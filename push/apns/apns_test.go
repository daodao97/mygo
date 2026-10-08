package apns

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	data, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return Config{KeyID: "KEY1234567", TeamID: "TEAM123456", PrivateKey: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: data}), Environment: Sandbox}
}
func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)
	cfg := testConfig(t)
	cfg.HTTPClient = server.Client()
	client, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	client.endpoint = server.URL
	return client
}
func testNotice() Notification {
	return Notification{DeviceToken: strings.Repeat("ab", 32), Topic: "dev.example.app", CollapseID: "event", Payload: Payload{ID: "event", Title: "Completed", Body: "Task", Group: "tasks", Data: map[string]string{"session": "pane"}}}
}
func TestProviderTokenSignatureCacheAndValidation(t *testing.T) {
	c, err := New(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	token, err := c.token(false)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatal("invalid JWT")
	}
	header, _ := base64.RawURLEncoding.DecodeString(parts[0])
	claims, _ := base64.RawURLEncoding.DecodeString(parts[1])
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	var h map[string]string
	json.Unmarshal(header, &h)
	var p map[string]any
	json.Unmarshal(claims, &p)
	if h["alg"] != "ES256" || h["kid"] != c.config.KeyID || p["iss"] != c.config.TeamID || len(sig) != 64 {
		t.Fatal("incorrect claims/signature")
	}
	hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(&c.key.PublicKey, hash[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("invalid ES256 signature")
	}
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, _ := c.token(false)
			if got != token {
				t.Error("token not cached across concurrent senders")
			}
		}()
	}
	wg.Wait()
	c.issued = time.Now().Add(-51 * time.Minute)
	fresh, _ := c.token(false)
	if fresh == token {
		t.Fatal("expired token retained")
	}
	bad := testConfig(t)
	bad.TeamID = "invalid"
	if _, err := New(bad); err == nil {
		t.Fatal("accepted invalid team")
	}
	bad = testConfig(t)
	key, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	data, _ := x509.MarshalPKCS8PrivateKey(key)
	bad.PrivateKey = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: data})
	if _, err := New(bad); err == nil {
		t.Fatal("accepted P-384")
	}
}
func TestHTTP2AlertHeadersAndMyGoPayload(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 || r.Method != "POST" || r.URL.Path != "/3/device/"+testNotice().DeviceToken {
			t.Error("wrong transport")
		}
		for k, v := range map[string]string{"apns-topic": "dev.example.app", "apns-push-type": "alert", "apns-priority": "10", "apns-collapse-id": "event"} {
			if r.Header.Get(k) != v {
				t.Errorf("wrong %s", k)
			}
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "bearer ") {
			t.Error("missing authorization")
		}
		var p map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&p) != nil {
			t.Error("invalid JSON")
		}
		var data map[string]string
		json.Unmarshal(p["mygo"], &data)
		if data["session"] != "pane" || string(p["mygoID"]) != `"event"` {
			t.Error("native notification data contract broken")
		}
		w.Header().Set("apns-id", "response-id")
	})
	result, err := c.Send(context.Background(), testNotice())
	if err != nil || result.ID != "response-id" {
		t.Fatal(result, err)
	}
}
func TestProviderFailureTimestampAndExpiredTokenRefresh(t *testing.T) {
	calls := 0
	tokens := []string{}
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		tokens = append(tokens, r.Header.Get("Authorization"))
		if calls == 1 {
			w.WriteHeader(403)
			w.Write([]byte(`{"reason":"ExpiredProviderToken"}`))
			return
		}
		w.WriteHeader(410)
		w.Write([]byte(`{"reason":"Unregistered","timestamp":1700000000123}`))
	})
	_, err := c.Send(context.Background(), testNotice())
	var response *Error
	if !errors.As(err, &response) || !response.Unregistered() || response.Timestamp.UnixMilli() != 1700000000123 || calls != 2 || tokens[0] == tokens[1] {
		t.Fatal("incorrect refresh or invalidation")
	}
	if strings.Contains(err.Error(), testNotice().DeviceToken) {
		t.Fatal("private token in error")
	}
}
func TestRetryAfterHonorsCancellationAndPermanentErrorsDoNotRetry(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(503)
		w.Write([]byte(`{"reason":"ServiceUnavailable"}`))
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := c.Send(ctx, testNotice())
	if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
		t.Fatal("retry ignored cancellation")
	}
	calls = 0
	c = testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(400)
		w.Write([]byte(`{"reason":"BadDeviceToken"}`))
	})
	_, err = c.Send(context.Background(), testNotice())
	if err == nil || calls != 1 {
		t.Fatal("permanent error retried")
	}
}
func TestRejectInvalidPayloadAndPreventRedirectLeaks(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Redirect(w, r, "https://example.com/", 302)
	})
	n := testNotice()
	n.Payload.Body = strings.Repeat("x", 4096)
	if _, err := c.Send(context.Background(), n); err == nil || calls != 0 {
		t.Fatal("oversized payload sent")
	}
	n = testNotice()
	n.DeviceToken = "nothex"
	if _, err := c.Send(context.Background(), n); err == nil || calls != 0 {
		t.Fatal("invalid token sent")
	}
	if _, err := c.Send(context.Background(), testNotice()); err == nil || calls != 1 {
		t.Fatal("redirect followed")
	}
}
