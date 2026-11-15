package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestS3Signature signs the PUT Object example of the Signature Version 4
// documentation of S3.
func TestS3Signature(t *testing.T) {
	endpoint, _ := url.Parse("https://s3.amazonaws.com")
	c := &s3Client{bucket: "examplebucket", region: "us-east-1", endpoint: endpoint,
		accessKey: "AKIAIOSFODNN7EXAMPLE", secretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"}
	req, err := http.NewRequest(http.MethodPut, c.objectURL("test$file.text"), strings.NewReader("Welcome to Amazon S3."))
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Host != "examplebucket.s3.amazonaws.com" || req.URL.EscapedPath() != "/test%24file.text" {
		t.Fatalf("URL %s", req.URL)
	}
	req.Header.Set("Date", "Fri, 24 May 2013 00:00:00 GMT")
	req.Header.Set("X-Amz-Storage-Class", "REDUCED_REDUNDANCY")
	sum := sha256.Sum256([]byte("Welcome to Amazon S3."))
	c.sign(req, hex.EncodeToString(sum[:]), time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, " +
		"SignedHeaders=date;host;x-amz-content-sha256;x-amz-date;x-amz-storage-class, " +
		"Signature=98ad721746da40c64f1a55b78f14c238d841ea1380cd77a1b5971af0ece108bd"
	if got := req.Header.Get("Authorization"); got != want {
		t.Errorf("Authorization:\n%s\nwant\n%s", got, want)
	}
}

func TestS3URLs(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "id")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "eu-west-1")
	no := false
	for _, tt := range []struct {
		s3   S3
		want string
	}{
		{S3{Bucket: "downloads", Prefix: "/my-app/"}, "https://downloads.s3.eu-west-1.amazonaws.com/my-app/My%20App%201.2.0.dmg"},
		{S3{Bucket: "dl.example.com", Region: "us-west-2"}, "https://s3.us-west-2.amazonaws.com/dl.example.com/My%20App%201.2.0.dmg"},
		{S3{Bucket: "downloads", Endpoint: "https://acc.r2.cloudflarestorage.com/"}, "https://acc.r2.cloudflarestorage.com/downloads/My%20App%201.2.0.dmg"},
		{S3{Bucket: "downloads", Endpoint: "https://oss-cn-hangzhou.aliyuncs.com", PathStyle: &no}, "https://downloads.oss-cn-hangzhou.aliyuncs.com/My%20App%201.2.0.dmg"},
	} {
		c, err := newS3Client(&tt.s3)
		if err != nil {
			t.Fatal(err)
		}
		if got := c.objectURL(c.key("My App 1.2.0.dmg")); got != tt.want {
			t.Errorf("%+v: %s, want %s", tt.s3, got, tt.want)
		}
	}
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	if _, err := newS3Client(&S3{Bucket: "downloads"}); err == nil {
		t.Error("no error without credentials")
	}
}

func TestPublishS3(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "id")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	t.Setenv("AWS_SESSION_TOKEN", "token")
	type object struct{ path, typ, cache string }
	var (
		mu  sync.Mutex
		put []object
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		sum := sha256.Sum256(body)
		switch {
		case r.Method != http.MethodPut || r.ContentLength != int64(len(body)):
			t.Errorf("%s %s with %d bytes of %d", r.Method, r.URL, len(body), r.ContentLength)
		case r.Header.Get("X-Amz-Content-Sha256") != hex.EncodeToString(sum[:]):
			t.Errorf("%s: the hash of another payload", r.URL)
		case r.Header.Get("X-Amz-Security-Token") != "token" || !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=id/"):
			t.Errorf("%s: not signed: %v", r.URL, r.Header)
		}
		switch {
		case strings.Contains(r.URL.Path, "denied"):
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>Access Denied</Message></Error>`)
			return
		case strings.Contains(r.URL.Path, "elsewhere"):
			w.Header().Set("Location", "/moved")
			w.Header().Set("X-Amz-Bucket-Region", "eu-west-1")
			w.WriteHeader(http.StatusMovedPermanently)
			return
		}
		mu.Lock()
		put = append(put, object{r.URL.EscapedPath(), r.Header.Get("Content-Type"), r.Header.Get("Cache-Control")})
		mu.Unlock()
	}))
	defer srv.Close()

	dist := t.TempDir()
	var artifacts []string
	for _, name := range []string{"update-darwin-arm64.json", "app-1.2.0.dmg", "app-1.2.0-darwin-arm64.tar.gz", "app-1.1.0-to-1.2.0-darwin-arm64.delta", "app-setup-1.2.0.exe", "App.exe", "empty.deb", "app-1.2.0-1.x86_64.rpm"} {
		p := filepath.Join(dist, name)
		content := name
		if name == "empty.deb" {
			content = ""
		}
		os.WriteFile(p, []byte(content), 0o644)
		artifacts = append(artifacts, p)
	}
	for _, target := range []string{"linux-amd64", "linux-arm64"} {
		p := filepath.Join(dist, target, "install.sh")
		os.Mkdir(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755)
		artifacts = append(artifacts, p)
	}
	c := &Config{root: t.TempDir(), Version: "1.2.0", Updates: &Updates{URL: "https://dl.example.com/app", S3: &S3{Bucket: "downloads", Prefix: "app", Endpoint: srv.URL}}}
	if err := publish(c, artifacts); err != nil {
		t.Fatal(err)
	}
	want := []object{
		{"/downloads/app/app-1.2.0.dmg", "application/x-apple-diskimage", ""},
		{"/downloads/app/app-1.2.0-darwin-arm64.tar.gz", "application/gzip", ""},
		{"/downloads/app/app-1.1.0-to-1.2.0-darwin-arm64.delta", "application/octet-stream", ""},
		{"/downloads/app/app-setup-1.2.0.exe", "application/vnd.microsoft.portable-executable", ""},
		{"/downloads/app/empty.deb", "application/vnd.debian.binary-package", ""},
		{"/downloads/app/app-1.2.0-1.x86_64.rpm", "application/x-rpm", ""},
		{"/downloads/app/install.sh", "text/plain; charset=utf-8", "no-cache"},
		{"/downloads/app/update-darwin-arm64.json", "application/json", "no-cache"},
	}
	if !slices.Equal(put, want) {
		t.Errorf("uploaded\n%v\nwant\n%v", put, want)
	}

	c.Updates.S3.Prefix = "denied"
	if err := publish(c, artifacts); err == nil || !strings.Contains(err.Error(), "AccessDenied: Access Denied") {
		t.Errorf("error %v", err)
	}
	c.Updates.S3.Prefix = "elsewhere"
	if err := publish(c, artifacts); err == nil || !strings.Contains(err.Error(), "301 Moved Permanently (the bucket is in eu-west-1: set updates.s3.region)") {
		t.Errorf("error %v", err)
	}
}

// TestArchivesFromS3 gets the archives of earlier versions from the
// bucket of updates.s3 when its credentials are set, rather than from
// updates.url.
func TestArchivesFromS3(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "id")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	t.Setenv("AWS_SESSION_TOKEN", "")
	var (
		mu  sync.Mutex
		got []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.Host+r.URL.EscapedPath())
		mu.Unlock()
		bucket := strings.HasPrefix(r.URL.Path, "/downloads/")
		switch {
		case r.Method != http.MethodGet:
			t.Errorf("%s %s", r.Method, r.URL)
		case bucket && (r.Header.Get("X-Amz-Content-Sha256") != emptyPayloadHash || !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=id/")):
			t.Errorf("%s: not signed: %v", r.URL, r.Header)
		case r.URL.Path == "/downloads/app/missing.tar.gz":
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code></Error>`)
			return
		}
		io.WriteString(w, "archive")
	}))
	defer srv.Close()
	served := srv.URL + "/app"
	c := &Config{Updates: &Updates{URL: served, S3: &S3{Bucket: "downloads", Prefix: "app", Endpoint: srv.URL}}}
	read := func(get archiveGetter, u string) error {
		t.Helper()
		resp, err := get(t.Context(), u)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if b, _ := io.ReadAll(resp.Body); string(b) != "archive" {
			t.Errorf("%s: %q", u, b)
		}
		return nil
	}
	get := c.archiveGetter()
	for _, u := range []string{served + "/App%201.0.0.tar.gz", srv.URL + "/elsewhere/app-0.9.0.tar.gz"} {
		if err := read(get, u); err != nil {
			t.Fatal(err)
		}
	}
	if err := read(get, served+"/missing.tar.gz"); err != errNotPublished {
		t.Errorf("a missing archive: %v", err)
	}
	host := strings.TrimPrefix(srv.URL, "http://")
	if want := []string{host + "/downloads/app/App%201.0.0.tar.gz", host + "/elsewhere/app-0.9.0.tar.gz", host + "/downloads/app/missing.tar.gz"}; !slices.Equal(got, want) {
		t.Errorf("requested %v, want %v", got, want)
	}
	// Without credentials, from updates.url.
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	got = nil
	if err := read(c.archiveGetter(), served+"/app-1.0.0.tar.gz"); err != nil {
		t.Fatal(err)
	}
	if want := []string{host + "/app/app-1.0.0.tar.gz"}; !slices.Equal(got, want) {
		t.Errorf("without credentials, requested %v", got)
	}
}
