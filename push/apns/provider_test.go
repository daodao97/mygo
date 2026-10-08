package apns

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func providerFixture(t *testing.T) (string, ProviderConfig) {
	t.Helper()
	if runtime.GOOS == "windows" || runtime.GOOS == "ios" {
		t.Skip("managed key files require POSIX owner-only permissions")
	}
	dir := t.TempDir()
	key := testConfig(t)
	cfg := ProviderConfig{TeamID: key.TeamID, KeyID: key.KeyID, Topic: "dev.example.app", Environment: Sandbox, KeyFile: filepath.Join(dir, "apns.p8")}
	if err := writeProviderFile(cfg.KeyFile, key.PrivateKey); err != nil {
		t.Fatal(err)
	}
	saveProviderFixture(t, dir, cfg)
	return dir, cfg
}

func saveProviderFixture(t *testing.T, dir string, cfg ProviderConfig) {
	t.Helper()
	raw, _ := json.MarshalIndent(cfg, "", "  ")
	if err := writeProviderFile(filepath.Join(dir, "push-config.json"), raw); err != nil {
		t.Fatal(err)
	}
}

func TestManagedProviderDefaultsTopicAndRetainsAuthenticationOnReload(t *testing.T) {
	dir, cfg := providerFixture(t)
	p, err := OpenProvider(dir)
	if err != nil {
		t.Fatal(err)
	}
	client := p.client
	token, _ := client.token(false)
	// A metadata rewrite must not rotate JWT or HTTP/2 connections.
	raw, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(dir, "push-config.json"), raw, 0600)
	if err := p.Reload(); err != nil || p.client != client {
		t.Fatal("unchanged credentials recreated client", err)
	}
	if got, _ := p.client.token(false); got != token {
		t.Fatal("reload rotated authentication token")
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apns-topic") != cfg.Topic || r.ProtoMajor != 2 {
			t.Error("managed provider lost topic or HTTP/2")
		}
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	p.client.http, p.client.endpoint = server.Client(), server.URL
	n := testNotice()
	n.Topic = ""
	if _, err := p.Send(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	n.Topic = "dev.another.app"
	if _, err := p.Send(context.Background(), n); err == nil {
		t.Fatal("cross-application send allowed")
	}
	cfg.Topic = "dev.another.app"
	saveProviderFixture(t, dir, cfg)
	if err := p.Reload(); err == nil {
		t.Fatal("reload changed provider application")
	}
	if _, err := p.Send(context.Background(), testNotice()); err == nil {
		t.Fatal("failed reload retained active sender")
	}
}

func TestManagedProviderRotationPermissionAndRecovery(t *testing.T) {
	dir, cfg := providerFixture(t)
	p, err := OpenProvider(dir)
	if err != nil {
		t.Fatal(err)
	}
	before := p.client
	replacement := testConfig(t)
	writeProviderFile(cfg.KeyFile, replacement.PrivateKey)
	if err := p.Reload(); err != nil || p.client == before {
		t.Fatal("key rotation was not loaded", err)
	}
	os.Chmod(cfg.KeyFile, 0644)
	if err := p.Reload(); err == nil || p.client != nil {
		t.Fatal("public-readable key accepted")
	}
	os.Chmod(cfg.KeyFile, 0600)
	if err := p.Reload(); err != nil || p.client == nil {
		t.Fatal("sender did not recover", err)
	}
	for _, field := range []string{"team", "environment", "store", "topic"} {
		bad := cfg
		switch field {
		case "team":
			bad.TeamID = "invalid"
		case "environment":
			bad.Environment = "invalid"
		case "store":
			bad.KeychainService = "another.application.apns"
		case "topic":
			bad.Topic = "app'; shell injection"
		}
		saveProviderFixture(t, dir, bad)
		if _, err := OpenProvider(dir); err == nil {
			t.Errorf("invalid %s accepted", field)
		}
	}
	// Errors and metadata never contain the PEM key.
	raw, _ := os.ReadFile(filepath.Join(dir, "push-config.json"))
	if strings.Contains(string(raw), "PRIVATE KEY") {
		t.Fatal("provider metadata contains private key")
	}
}

func TestImportProviderRejectsInvalidCredentialsBeforeWriting(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "uncreated")
	if err := ImportProvider(dir, ProviderConfig{TeamID: "TEAM123456", KeyID: "KEY1234567", Topic: "dev.example.app", Environment: Sandbox}, []byte("invalid private key")); err == nil {
		t.Fatal("invalid key imported")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("invalid import wrote provider state")
	}
}
