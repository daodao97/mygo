package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

var playTestKey = sync.OnceValue(func() *rsa.PrivateKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return key
})

// playServiceAccountJSON is a service account's key, of a test's token server.
func playServiceAccountJSON(t *testing.T, tokenURI string) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(playTestKey())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]string{
		"type":           "service_account",
		"client_email":   "ci@example.iam.gserviceaccount.com",
		"private_key_id": "key-1",
		"private_key":    string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"token_uri":      tokenURI,
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// playRequest is what the fake API received.
type playRequest struct {
	method, path, contentType, auth string
	body                            []byte
}

// fakePlay is Google's API and token server, recording requests, with a way
// to refuse some.
type fakePlay struct {
	*httptest.Server
	mu       sync.Mutex
	requests []playRequest
	refuse   string // the path suffix to answer 403 to
	sha      string // the hash the upload answers, by default the body's
}

func newFakePlay(t *testing.T) *fakePlay {
	f := &fakePlay{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		path := r.URL.Path
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
		f.mu.Lock()
		f.requests = append(f.requests, playRequest{r.Method, path, r.Header.Get("Content-Type"), r.Header.Get("Authorization"), body})
		refuse := f.refuse
		f.mu.Unlock()
		if refuse != "" && strings.HasSuffix(path, refuse) {
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `{"error": {"code": 403, "message": "The caller does not have permission"}}`)
			return
		}
		switch {
		case path == "/token":
			io.WriteString(w, `{"access_token": "the-token", "expires_in": 3599, "token_type": "Bearer"}`)
		case strings.HasSuffix(path, "/edits") && r.Method == http.MethodPost:
			io.WriteString(w, `{"id": "EDIT1", "expiryTimeSeconds": "1700000000"}`)
		case strings.HasSuffix(path, "/bundles?uploadType=media"):
			sum := sha256.Sum256(body)
			sha := hex.EncodeToString(sum[:])
			if f.sha != "" {
				sha = f.sha
			}
			io.WriteString(w, `{"versionCode": 10203, "sha256": "`+sha+`", "sha1": "x"}`)
		default:
			io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakePlay) client(t *testing.T) *playClient {
	t.Helper()
	account, err := parsePlayServiceAccount(playServiceAccountJSON(t, f.URL+"/token"))
	if err != nil {
		t.Fatal(err)
	}
	return &playClient{http: f.Client(), base: f.URL, account: account, pkg: "com.example.app",
		now: func() time.Time { return time.Unix(1_800_000_000, 0) }}
}

func writeTestAAB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app-1.2.3.aab")
	if err := os.WriteFile(path, []byte("the bundle's bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// An upload goes token, edit, bundle, track, commit, in that order and with
// these requests.
func TestPlayUpload(t *testing.T) {
	f := newFakePlay(t)
	file := writeTestAAB(t)
	var steps []string
	code, err := f.client(t).upload(context.Background(), playRelease{file: file, track: "beta", status: "completed", name: "1.2.3"},
		func(s string) { steps = append(steps, s) })
	if err != nil {
		t.Fatal(err)
	}
	if code != 10203 {
		t.Errorf("version code %d", code)
	}
	if len(steps) == 0 {
		t.Error("no progress")
	}
	const app = "/androidpublisher/v3/applications/com.example.app/edits"
	var got []string
	for _, r := range f.requests {
		got = append(got, r.method+" "+r.path)
	}
	want := []string{
		"POST /token",
		"POST " + app,
		"POST /upload" + app + "/EDIT1/bundles?uploadType=media",
		"PUT " + app + "/EDIT1/tracks/beta",
		"POST " + app + "/EDIT1:commit",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("requests:\n%s", strings.Join(got, "\n"))
	}
	for _, r := range f.requests[1:] {
		if r.auth != "Bearer the-token" {
			t.Errorf("%s: authorization %q", r.path, r.auth)
		}
	}
	if f.requests[0].auth != "" {
		t.Errorf("the token's request has a token: %q", f.requests[0].auth)
	}

	upload := f.requests[2]
	if upload.contentType != "application/octet-stream" || string(upload.body) != "the bundle's bytes" {
		t.Errorf("upload: %q %q", upload.contentType, upload.body)
	}

	var track struct {
		Track    string
		Releases []struct {
			Name, Status string
			VersionCodes []string
		}
	}
	if err := json.Unmarshal(f.requests[3].body, &track); err != nil {
		t.Fatal(err)
	}
	if track.Track != "beta" || len(track.Releases) != 1 {
		t.Fatalf("track: %s", f.requests[3].body)
	}
	if rel := track.Releases[0]; rel.Name != "1.2.3" || rel.Status != "completed" || !reflect.DeepEqual(rel.VersionCodes, []string{"10203"}) {
		t.Errorf("release: %+v", rel)
	}
	if f.requests[3].contentType != "application/json" {
		t.Errorf("track's type: %q", f.requests[3].contentType)
	}
}

// The assertion is a JWT that Google can check, by the scope asked and with
// the key of the service account.
func TestPlayAssertion(t *testing.T) {
	f := newFakePlay(t)
	c := f.client(t)
	if err := c.authenticate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.token != "the-token" {
		t.Errorf("token %q", c.token)
	}
	r := f.requests[0]
	if r.contentType != "application/x-www-form-urlencoded" {
		t.Errorf("type: %q", r.contentType)
	}
	form, err := url.ParseQuery(string(r.body))
	if err != nil {
		t.Fatal(err)
	}
	if form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
		t.Errorf("grant: %q", form.Get("grant_type"))
	}
	parts := strings.Split(form.Get("assertion"), ".")
	if len(parts) != 3 {
		t.Fatalf("assertion: %q", form.Get("assertion"))
	}
	decode := func(s string, v any) {
		data, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, v); err != nil {
			t.Fatal(err)
		}
	}
	var header map[string]string
	decode(parts[0], &header)
	if header["alg"] != "RS256" || header["typ"] != "JWT" || header["kid"] != "key-1" {
		t.Errorf("header: %v", header)
	}
	var claims struct {
		Iss, Scope, Aud string
		Iat, Exp        int64
	}
	decode(parts[1], &claims)
	if claims.Iss != "ci@example.iam.gserviceaccount.com" || claims.Scope != "https://www.googleapis.com/auth/androidpublisher" || claims.Aud != f.URL+"/token" {
		t.Errorf("claims: %+v", claims)
	}
	// Google takes an hour at most.
	if claims.Iat != 1_800_000_000 || claims.Exp != claims.Iat+3600 {
		t.Errorf("times: %+v", claims)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&playTestKey().PublicKey, crypto.SHA256, sum[:], sig); err != nil {
		t.Errorf("signature: %v", err)
	}
}

// A failure says Google's reason, commits nothing, and leaves no edit.
func TestPlayUploadFails(t *testing.T) {
	const app = "/androidpublisher/v3/applications/com.example.app/edits"
	for _, refuse := range []string{"/bundles?uploadType=media", "/tracks/internal", ":commit"} {
		f := newFakePlay(t)
		f.refuse = refuse
		_, err := f.client(t).upload(context.Background(), playRelease{file: writeTestAAB(t), track: "internal", status: "completed", name: "1"}, func(string) {})
		if err == nil || !strings.Contains(err.Error(), "does not have permission") || !strings.Contains(err.Error(), "403") {
			t.Errorf("%s: %v", refuse, err)
		}
		last := f.requests[len(f.requests)-1]
		if last.method != http.MethodDelete || last.path != app+"/EDIT1" {
			t.Errorf("%s: last request %s %s", refuse, last.method, last.path)
		}
		for _, r := range f.requests {
			if refuse != ":commit" && strings.HasSuffix(r.path, ":commit") {
				t.Errorf("%s: committed", refuse)
			}
		}
	}

	// A bundle that arrived otherwise goes no further.
	f := newFakePlay(t)
	f.sha = "00"
	_, err := f.client(t).upload(context.Background(), playRelease{file: writeTestAAB(t), track: "internal", status: "completed", name: "1"}, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "hash") {
		t.Errorf("hash: %v", err)
	}
	if last := f.requests[len(f.requests)-1]; last.method != http.MethodDelete {
		t.Errorf("last request %s %s", last.method, last.path)
	}

	// Signing in fails: there is no edit to delete.
	f = newFakePlay(t)
	f.refuse = "/token"
	_, err = f.client(t).upload(context.Background(), playRelease{file: writeTestAAB(t), track: "internal", status: "completed", name: "1"}, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "signing in") {
		t.Errorf("token: %v", err)
	}
	if len(f.requests) != 1 {
		t.Errorf("%d requests after no token", len(f.requests))
	}
}

func TestParsePlayServiceAccount(t *testing.T) {
	data := playServiceAccountJSON(t, "")
	a, err := parsePlayServiceAccount(data)
	if err != nil {
		t.Fatal(err)
	}
	if a.TokenURI != "https://oauth2.googleapis.com/token" {
		t.Errorf("default token URI: %q", a.TokenURI)
	}
	for _, bad := range []string{`not JSON`, `{}`, `{"client_email": "a", "private_key": "not PEM"}`,
		`{"client_email": "a", "private_key": "-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n"}`} {
		if _, err := parsePlayServiceAccount([]byte(bad)); err == nil {
			t.Errorf("%s was read", bad)
		}
	}
	// The variable holds a path, or the JSON, as a secret of CI.
	path := filepath.Join(t.TempDir(), "key.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{path, string(data)} {
		t.Setenv("MYGO_PLAY_SERVICE_ACCOUNT", v)
		if a, err := readPlayServiceAccount(); err != nil || a.ClientEmail == "" {
			t.Errorf("%v, %v", a, err)
		}
	}
	t.Setenv("MYGO_PLAY_SERVICE_ACCOUNT", "")
	if _, err := readPlayServiceAccount(); err == nil || !strings.Contains(err.Error(), "MYGO_PLAY_SERVICE_ACCOUNT") {
		t.Errorf("unset: %v", err)
	}
}

func TestAppStoreUploadArgs(t *testing.T) {
	k := ascCredentials{keyID: "ABC123", issuerID: "issuer-1", keyPath: "/k/key.p8"}
	got := altoolUploadArgs("/a/app.ipa", k)
	want := []string{"altool", "--upload-app", "-f", "/a/app.ipa", "-t", "ios", "--apiKey", "ABC123", "--apiIssuer", "issuer-1", "--output-format", "normal"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("args: %q", got)
	}
}

func TestAppStoreCredentials(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "whatever.p8")
	if err := os.WriteFile(key, []byte("the key"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYGO_ASC_KEY_ID", "")
	t.Setenv("MYGO_ASC_ISSUER_ID", "issuer")
	t.Setenv("MYGO_ASC_KEY", key)
	if _, err := ascFromEnv(); err == nil || !strings.Contains(err.Error(), "MYGO_ASC_KEY_ID") || strings.Contains(err.Error(), "MYGO_ASC_ISSUER_ID") {
		t.Errorf("no ID: %v", err)
	}
	t.Setenv("MYGO_ASC_KEY_ID", "../evil")
	if _, err := ascFromEnv(); err == nil {
		t.Error("a key ID that is a path was taken")
	}
	t.Setenv("MYGO_ASC_KEY_ID", "ABC123")
	k, err := ascFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	// altool looks for AuthKey_<id>.p8 in a directory.
	keys, err := ascKeyDir(k)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(keys)
	data, err := os.ReadFile(filepath.Join(keys, "AuthKey_ABC123.p8"))
	if err != nil || string(data) != "the key" {
		t.Errorf("%q, %v", data, err)
	}
	k.keyPath = filepath.Join(dir, "gone.p8")
	if _, err := ascKeyDir(k); err == nil || !strings.Contains(err.Error(), "MYGO_ASC_KEY") {
		t.Errorf("no key: %v", err)
	}
}

func TestStoreArtifact(t *testing.T) {
	root := t.TempDir()
	c := &Config{root: root, Name: "App", Slug: "my-app", Version: "1.2.3", Out: "dist"}
	path := filepath.Join(root, "dist", "android", "my-app-1.2.3.aab")
	if _, err := storeArtifact(c, "", "android", ".aab", "mygo build -platform android"); err == nil || !strings.Contains(err.Error(), "mygo build -platform android") {
		t.Errorf("not built: %v", err)
	}
	writeFiles(t, root, map[string]string{"dist/android/my-app-1.2.3.aab": "bundle", "other.apk": "apk"})
	got, err := storeArtifact(c, "", "android", ".aab", "")
	if err != nil || got != path {
		t.Errorf("%s, %v", got, err)
	}
	if got, err := storeArtifact(c, path, "android", ".aab", ""); err != nil || got != path {
		t.Errorf("given: %s, %v", got, err)
	}
	if _, err := storeArtifact(c, filepath.Join(root, "other.apk"), "android", ".aab", ""); err == nil || !strings.Contains(err.Error(), "an .aab") {
		t.Errorf("wrong kind: %v", err)
	}
	if _, err := storeArtifact(c, filepath.Join(root, "gone.aab"), "android", ".aab", ""); err == nil {
		t.Error("a file that is not there was taken")
	}
}
