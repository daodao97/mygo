package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/update"
)

type updateRoundTripper func(*http.Request) (*http.Response, error)

func (f updateRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUpdateGetRetry(t *testing.T) {
	dns := &net.DNSError{Err: "no such host", Name: "example.com", IsNotFound: true}
	for _, tc := range []struct {
		name     string
		statuses []int
		err      error
		requests int
		wantErr  bool
	}{
		{"recovers", []int{503, 0, 200}, dns, 3, false},
		{"not published", []int{404}, nil, 1, true},
		{"bad request", []int{400}, nil, 1, true},
		{"unauthorized", []int{401}, nil, 1, true},
		{"forbidden", []int{403}, nil, 1, true},
		{"request timeout", []int{408}, nil, 1, true},
		{"rate limited", []int{429}, nil, 3, true},
		{"server error", []int{500}, nil, 3, true},
		{"unavailable", []int{503}, nil, 3, true},
		{"DNS error", []int{0}, dns, 3, true},
		{"connection error", []int{0}, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}, 3, true},
		{"timeout", []int{0}, context.DeadlineExceeded, 3, true},
		{"connection closed", []int{0}, io.EOF, 3, true},
		{"canceled", []int{0}, context.Canceled, 1, true},
		{"other error", []int{0}, errors.New("invalid request"), 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := http.DefaultTransport
			wait := updateRetryWait
			t.Cleanup(func() { http.DefaultTransport, updateRetryWait = transport, wait })
			var delays []time.Duration
			updateRetryWait = func(d time.Duration) { delays = append(delays, d) }
			requests := 0
			http.DefaultTransport = updateRoundTripper(func(r *http.Request) (*http.Response, error) {
				status := tc.statuses[min(requests, len(tc.statuses)-1)]
				requests++
				if r.Method != http.MethodGet || r.URL.String() != "https://example.com/update.json" || r.Header.Get("User-Agent") != "mygo/"+version {
					t.Errorf("request = %s %s, headers %v", r.Method, r.URL, r.Header)
				}
				if status == 0 {
					return nil, tc.err
				}
				return &http.Response{
					StatusCode: status,
					Status:     http.StatusText(status),
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("published")),
				}, nil
			})
			resp, err := updateGet("https://example.com/update.json", time.Minute)
			if requests != tc.requests {
				t.Errorf("made %d requests, want %d", requests, tc.requests)
			}
			if want := ([]time.Duration{time.Second, 2 * time.Second})[:tc.requests-1]; !slices.Equal(delays, want) {
				t.Errorf("retry delays = %v, want %v", delays, want)
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("updateGet = %v, want error %v", err, tc.wantErr)
			}
			if tc.wantErr {
				if resp != nil {
					resp.Body.Close()
					t.Error("an unsuccessful request returned a response")
				}
				if tc.statuses[0] == http.StatusNotFound && !errors.Is(err, errNotPublished) {
					t.Errorf("404 returned %v, want errNotPublished", err)
				}
				if tc.statuses[0] == 0 && !errors.Is(err, tc.err) {
					t.Errorf("returned %v, want %v", err, tc.err)
				}
				return
			}
			defer resp.Body.Close()
			if b, err := io.ReadAll(resp.Body); err != nil || string(b) != "published" {
				t.Errorf("body = %q, %v", b, err)
			}
		})
	}
}

func TestUpdateGetRetryAfter(t *testing.T) {
	for _, tc := range []struct {
		header string
		first  time.Duration
		second time.Duration
	}{
		{"", time.Second, 2 * time.Second},
		{"5", 5 * time.Second, 5 * time.Second},
		{"3600", 30 * time.Second, 30 * time.Second},
		{"9223372036854775807", 30 * time.Second, 30 * time.Second},
		{"Fri, 31 Dec 9999 23:59:59 GMT", 30 * time.Second, 30 * time.Second},
		{"Sat, 01 Jan 2000 00:00:00 GMT", time.Second, 2 * time.Second},
		{"-1", time.Second, 2 * time.Second},
		{"invalid", time.Second, 2 * time.Second},
	} {
		t.Run(tc.header, func(t *testing.T) {
			transport, wait := http.DefaultTransport, updateRetryWait
			t.Cleanup(func() { http.DefaultTransport, updateRetryWait = transport, wait })
			var delays []time.Duration
			updateRetryWait = func(d time.Duration) { delays = append(delays, d) }
			http.DefaultTransport = updateRoundTripper(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Status:     "429 Too Many Requests",
					Header:     http.Header{"Retry-After": {tc.header}},
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			})
			if _, err := updateGet("https://example.com/update.json", time.Minute); err == nil {
				t.Fatal("accepted a 429 response")
			}
			if want := []time.Duration{tc.first, tc.second}; !slices.Equal(delays, want) {
				t.Errorf("retry delays = %v, want %v", delays, want)
			}
		})
	}
}

func TestUpdatesConfig(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	key := base64.StdEncoding.EncodeToString(pub)
	dir := t.TempDir()
	load := func(updates string) (*Config, error) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "mygo.json"), []byte(`{"name": "My App", "version": "1.2.0", "updates": `+updates+`}`), 0o644); err != nil {
			t.Fatal(err)
		}
		return loadConfig(dir)
	}
	c, err := load(`{"publicKey": "` + key + `", "github": "me/my-app"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.updateFeed("darwin-arm64"); got != "https://github.com/me/my-app/releases/latest/download/update-darwin-arm64.json" {
		t.Errorf("feed = %s", got)
	}
	if got := c.updateFile("my-app-1.2.0-darwin-arm64.tar.gz", false); got != "https://github.com/me/my-app/releases/download/v1.2.0/my-app-1.2.0-darwin-arm64.tar.gz" {
		t.Errorf("archive = %s", got)
	}
	// With a tag prefix, the repository may hold other releases: apps look
	// for the newest release tagged with it.
	c, err = load(`{"publicKey": "` + key + `", "github": "me/my-app", "tagPrefix": "desktop-v"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.updateFeed("windows-amd64"); got != "https://github.com/me/my-app/releases/download/desktop-v{version}/update-windows-amd64.json" {
		t.Errorf("tagged feed = %s", got)
	}
	if got := c.updateFile("my-app-1.2.0-windows-amd64.tar.gz", false); got != "https://github.com/me/my-app/releases/download/desktop-v1.2.0/my-app-1.2.0-windows-amd64.tar.gz" {
		t.Errorf("tagged archive = %s", got)
	}
	script := installScript(c)
	for _, line := range []string{
		"releases='https://github.com/me/my-app/releases/download/'\n",
		"api='https://api.github.com/repos/me/my-app/releases?per_page=100'\n",
		"tag_prefix='desktop-v'\n",
		"#   curl -fsSL https://github.com/me/my-app/releases/download/desktop-v1.2.0/install.sh | sh\n",
	} {
		if !strings.Contains(script, line) {
			t.Errorf("install.sh has no %q", line)
		}
	}
	c, err = load(`{"publicKey": "` + key + `", "url": "https://dl.example.com/my-app/"}`)
	if err != nil || c.updateFeed("linux-amd64") != "https://dl.example.com/my-app/update-linux-amd64.json" {
		t.Errorf("url feed = %s, %v", c.updateFeed("linux-amd64"), err)
	}
	if _, err := load(`{"publicKey": "` + key + `", "url": "https://dl.example.com/my-app", "s3": {"bucket": "downloads", "prefix": "my-app", "endpoint": "https://acc.r2.cloudflarestorage.com"}}`); err != nil {
		t.Error(err)
	}
	for _, bad := range []string{
		`{"github": "me/my-app"}`,
		`{"publicKey": "nope", "github": "me/my-app"}`,
		`{"publicKey": "` + key + `"}`,
		`{"publicKey": "` + key + `", "github": "me"}`,
		`{"publicKey": "` + key + `", "url": "http://dl.example.com"}`,
		`{"publicKey": "` + key + `", "url": "https://dl.example.com/my-app?token=example"}`,
		`{"publicKey": "` + key + `", "url": "https://dl.example.com/my-app#release"}`,
		`{"publicKey": "` + key + `", "url": "https://dl.example.com/my-app?"}`,
		`{"publicKey": "` + key + `", "url": "https://dl.example.com/my-app#"}`,
		`{"publicKey": "` + key + `", "github": "me/my-app", "deltas": -1}`,
		`{"publicKey": "` + key + `", "github": "me/my-app", "s3": {"bucket": "downloads"}}`,
		`{"publicKey": "` + key + `", "url": "https://dl.example.com", "s3": {}}`,
		`{"publicKey": "` + key + `", "url": "https://dl.example.com", "s3": {"bucket": "downloads", "endpoint": "acc.r2.cloudflarestorage.com"}}`,
		`{"publicKey": "` + key + `", "url": "https://dl.example.com", "s3": {"bucket": "downloads", "endpoint": "https://acc.r2.cloudflarestorage.com/downloads"}}`,
	} {
		if _, err := load(bad); err == nil {
			t.Errorf("accepted updates %s", bad)
		}
	}
}

func TestKeygen(t *testing.T) {
	dir := t.TempDir()
	if err := runKeygen([]string{"-o", dir}); err != nil {
		t.Fatal(err)
	}
	priv, _ := os.ReadFile(filepath.Join(dir, "mygo-update.key"))
	pub, _ := os.ReadFile(filepath.Join(dir, "mygo-update.pub"))
	sk, err := update.ParsePrivateKey(string(priv))
	if err != nil {
		t.Fatal(err)
	}
	if base64.StdEncoding.EncodeToString(sk.Public().(ed25519.PublicKey)) != strings.TrimSpace(string(pub)) {
		t.Error("the keys are not a pair")
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(filepath.Join(dir, "mygo-update.key")); info.Mode().Perm() != 0o600 {
			t.Errorf("secret key mode %v", info.Mode())
		}
	}
	if err := runKeygen([]string{"-o", dir}); err == nil {
		t.Error("keygen replaced existing keys without -force")
	}
}

// TestUpdateEndToEnd builds and publishes two versions of an app on a
// local server, installs the first and lets it update itself to the second
// with the delta update of the second.
func TestUpdateEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles programs")
	}
	dir := testModule(t, map[string]string{
		"main.go": `package main

import (
	"context"
	"fmt"
	"os"

	"github.com/egoist/mygo"
)

func main() {
	fmt.Println("version", mygo.App.Version())
	if os.Getenv("UPDATE") == "" {
		return
	}
	up, err := mygo.Updater.Check(context.Background())
	if err != nil || up == nil {
		fmt.Println("check:", up, err)
		os.Exit(1)
	}
	if err := up.Install(context.Background(), nil); err != nil {
		fmt.Println("install:", err)
		os.Exit(1)
	}
	fmt.Println("installed", up.Version, up.Notes)
}
`,
		"CHANGELOG.md": "# Changelog\n\n## 1.1.0\n\n- New things\n\n## 1.0.0\n\n- First\n",
	})
	pub, priv, _ := ed25519.GenerateKey(nil)
	t.Setenv("MYGO_UPDATER_PRIVATE_KEY", base64.StdEncoding.EncodeToString(priv))
	serve := t.TempDir()
	var mu sync.Mutex
	var requests []string
	files := http.FileServer(http.Dir(serve))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.Path)
		mu.Unlock()
		files.ServeHTTP(w, r)
	}))
	defer srv.Close()

	goos, goarch := runtime.GOOS, runtime.GOARCH
	target := goos + "-" + goarch
	build := func(version, out string) string {
		t.Helper()
		c, err := loadConfig(dir)
		if err != nil {
			t.Fatal(err)
		}
		c.Name, c.Version, c.Out = "Update Test", version, out
		c.Updates = &Updates{PublicKey: base64.StdEncoding.EncodeToString(pub), URL: srv.URL, TagPrefix: "v"}
		opts := buildOptions{sign: "-", skipDMG: true, work: t.TempDir()}
		if opts.pkg, err = packageDir(c); err != nil {
			t.Fatal(err)
		}
		if _, err := buildPlatform(c, goos, goarch, opts); err != nil {
			t.Fatal(err)
		}
		return filepath.Join(dir, out, target)
	}
	// publish copies the update files of a build to the server, and
	// returns its manifest.
	publish := func(dir string) update.Manifest {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, update.ManifestName(target)))
		if err != nil {
			t.Fatal(err)
		}
		var m update.Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		names := []string{filepath.Base(m.URL), update.ManifestName(target)}
		for _, d := range m.Deltas {
			names = append([]string{filepath.Base(d.URL)}, names...)
		}
		for _, name := range names {
			b, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(serve, name), b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return m
	}
	v1 := build("1.0.0", "dist1")
	if m := publish(v1); len(m.Deltas) != 0 || len(m.Previous) != 0 {
		t.Errorf("the first version has deltas %+v and previous %+v", m.Deltas, m.Previous)
	}
	v2 := build("1.1.0", "dist2")
	m := publish(v2)
	if m.Version != "1.1.0" || m.Notes != "- New things" {
		t.Errorf("manifest = %+v", m)
	}
	if len(m.Deltas) != 1 || m.Deltas[0].From != "1.0.0" || len(m.Previous) != 1 || m.Previous[0].Version != "1.0.0" {
		t.Fatalf("deltas %+v, previous %+v", m.Deltas, m.Previous)
	}
	if d := m.Deltas[0]; d.Size > m.Size/5 {
		t.Errorf("the delta update takes %d bytes, the archive %d", d.Size, m.Size)
	}

	// Install 1.0.0 as a user would, and run it.
	install := t.TempDir()
	exe := filepath.Join(install, "update-test")
	switch goos {
	case "darwin":
		if err := exec.Command("ditto", filepath.Join(v1, "Update Test.app"), filepath.Join(install, "Update Test.app")).Run(); err != nil {
			t.Fatal(err)
		}
		exe = filepath.Join(install, "Update Test.app", "Contents", "MacOS", "Update Test")
	case "windows":
		exe = filepath.Join(install, "Update Test.exe")
		fallthrough
	default:
		entries, _ := os.ReadDir(v1)
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".tar.gz") || strings.HasSuffix(e.Name(), ".json") || strings.HasSuffix(e.Name(), ".delta") {
				continue
			}
			if err := copyResource(filepath.Join(v1, e.Name()), filepath.Join(install, e.Name())); err != nil {
				t.Fatal(err)
			}
		}
	}
	run := func(env ...string) string {
		t.Helper()
		cmd := exec.Command(exe)
		cmd.Env = append(os.Environ(), env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", exe, err, out)
		}
		return string(out)
	}
	mu.Lock()
	requests = nil
	mu.Unlock()
	if out := run("UPDATE=1"); !strings.Contains(out, "version 1.0.0") || !strings.Contains(out, "installed 1.1.0 - New things") {
		t.Fatalf("updating printed:\n%s", out)
	}
	mu.Lock()
	if want := []string{"/" + update.ManifestName(target), "/" + filepath.Base(m.Deltas[0].URL)}; !slices.Equal(requests, want) {
		t.Errorf("the update downloaded %q, want %q", requests, want)
	}
	mu.Unlock()
	if goos == "darwin" {
		// The new bundle replaces the app's once the app has exited.
		for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
			if staged, _ := filepath.Glob(filepath.Join(install, ".*.update-*")); len(staged) == 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("the update was not installed once the app exited")
			}
		}
	}
	if out := run(); !strings.Contains(out, "version 1.1.0") {
		t.Errorf("after the update the app printed:\n%s", out)
	}
	if goos == "darwin" {
		if out, err := exec.Command("codesign", "--verify", "--deep", "--strict", filepath.Join(install, "Update Test.app")).CombinedOutput(); err != nil {
			t.Errorf("the updated bundle fails codesign: %v\n%s", err, out)
		}
	}
}

// TestWriteDeltas makes delta updates from the versions a published
// manifest lists.
func TestWriteDeltas(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	t.Setenv("MYGO_UPDATER_PRIVATE_KEY", base64.StdEncoding.EncodeToString(priv))
	code := make([]byte, 50_000)
	for i := range code {
		code[i] = byte(i * i >> 3)
	}
	// app writes the app of a version into dir.
	app := func(dir, version string) []string {
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "app"), append(slices.Clone(code), version...), 0o755)
		os.WriteFile(filepath.Join(dir, "LICENSE"), []byte("MIT"), 0o644)
		return []string{"LICENSE", "app"}
	}
	serve := t.TempDir()
	var mu sync.Mutex
	var requests []string
	files := http.FileServer(http.Dir(serve))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.Path)
		mu.Unlock()
		files.ServeHTTP(w, r)
	}))
	defer srv.Close()
	// Published: 1.1.0, and before it 1.0.0, a 2.0.0 that cannot be, and
	// 0.9.0, whose archive is signed with another key.
	published := update.Manifest{Version: "1.1.0"}
	for _, v := range []string{"1.1.0", "1.0.0", "2.0.0", "0.9.0"} {
		dir := filepath.Join(t.TempDir(), v)
		entries := app(dir, v)
		var archive bytes.Buffer
		if err := update.WriteArchive(&archive, dir, entries); err != nil {
			t.Fatal(err)
		}
		name := "app-" + v + ".tar.gz"
		os.WriteFile(filepath.Join(serve, name), archive.Bytes(), 0o644)
		sum := sha256.Sum256(archive.Bytes())
		a := update.Archive{Version: v, URL: srv.URL + "/" + name, Size: int64(archive.Len()), Signature: update.Sign(priv, sum[:])}
		if v == "0.9.0" {
			_, other, _ := ed25519.GenerateKey(nil)
			a.Signature = update.Sign(other, sum[:])
		}
		if v == "1.1.0" {
			published.URL, published.Size, published.Signature = a.URL, a.Size, a.Signature
		} else {
			published.Previous = append(published.Previous, a)
		}
	}
	data, _ := json.Marshal(published)
	os.WriteFile(filepath.Join(serve, update.ManifestName("linux-amd64")), data, 0o644)

	build := func(deltas int) (update.Manifest, []string) {
		t.Helper()
		stage := t.TempDir()
		entries := app(stage, "1.2.0")
		c := &Config{root: t.TempDir(), Name: "App", Version: "1.2.0", Updates: &Updates{PublicKey: base64.StdEncoding.EncodeToString(pub), URL: srv.URL, Deltas: &deltas}}
		mu.Lock()
		requests = nil
		mu.Unlock()
		written, err := writeArchive(c, stage, "linux-amd64", entries)
		if err != nil {
			t.Fatal(err)
		}
		var m update.Manifest
		data, _ := os.ReadFile(written[len(written)-1])
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		// Each delta makes the new version of the old.
		for _, d := range m.Deltas {
			old := filepath.Join(t.TempDir(), "old")
			app(old, d.From)
			f, err := os.Open(filepath.Join(stage, filepath.Base(d.URL)))
			if err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(t.TempDir(), "new")
			err = update.ApplyDelta(f, d.Size, d.From, "1.2.0", old, out)
			f.Close()
			if err != nil {
				t.Fatalf("the delta from %s: %v", d.From, err)
			}
			if b, _ := os.ReadFile(filepath.Join(out, "app")); !bytes.Equal(b, append(slices.Clone(code), "1.2.0"...)) {
				t.Errorf("the delta from %s made another app", d.From)
			}
		}
		mu.Lock()
		defer mu.Unlock()
		return m, slices.Clone(requests)
	}
	versions := func(m update.Manifest) (deltas, previous []string) {
		for _, d := range m.Deltas {
			deltas = append(deltas, d.From)
		}
		for _, a := range m.Previous {
			previous = append(previous, a.Version)
		}
		return deltas, previous
	}

	m, _ := build(3)
	if deltas, previous := versions(m); !slices.Equal(deltas, []string{"1.1.0", "1.0.0"}) || !slices.Equal(previous, []string{"1.1.0", "1.0.0"}) {
		t.Errorf("3 deltas: from %v, previous %v", deltas, previous)
	}
	if d := m.Deltas[0]; d.URL != srv.URL+"/app-1.1.0-to-1.2.0-linux-amd64.delta" || d.Size > m.Size/4 {
		t.Errorf("delta %+v, archive of %d bytes", d, m.Size)
	}
	m, _ = build(1)
	if deltas, previous := versions(m); !slices.Equal(deltas, []string{"1.1.0"}) || previous != nil {
		t.Errorf("1 delta: from %v, previous %v", deltas, previous)
	}
	if m, requests := build(0); m.Deltas != nil || m.Previous != nil || requests != nil {
		t.Errorf("no deltas: %+v, requested %v", m, requests)
	}
}

func TestPublishGitHub(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as gh")
	}
	bin, dist := t.TempDir(), t.TempDir()
	calls := filepath.Join(bin, "calls")
	// gh: the release does not exist yet; every other call succeeds.
	script := "#!/bin/sh\necho \"$@\" >> " + calls + "\n[ \"$2\" = view ] && exit 1\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var artifacts []string
	for _, name := range []string{"app-1.2.0.dmg", "app-1.2.0-darwin-arm64.tar.gz", "app-1.1.0-to-1.2.0-darwin-arm64.delta", "update-darwin-arm64.json", "app-setup-1.2.0.exe", "App.exe"} {
		p := filepath.Join(dist, name)
		os.WriteFile(p, nil, 0o644)
		artifacts = append(artifacts, p)
	}
	os.Mkdir(filepath.Join(dist, "App.app"), 0o755)
	artifacts = append(artifacts, filepath.Join(dist, "App.app"))
	for _, target := range []string{"linux-amd64", "linux-arm64"} {
		p := filepath.Join(dist, target, "install.sh")
		os.Mkdir(filepath.Dir(p), 0o755)
		os.WriteFile(p, nil, 0o755)
		artifacts = append(artifacts, p)
	}
	c := &Config{root: t.TempDir(), Version: "1.2.0", Updates: &Updates{GitHub: "me/app", TagPrefix: "v"}}
	if err := publishGitHub(c, artifacts); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(calls)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[1], "release create v1.2.0 --repo me/app --draft") {
		t.Fatalf("gh calls:\n%s", b)
	}
	if !strings.Contains(lines[2], "app-1.2.0.dmg") || !strings.Contains(lines[2], "app-setup-1.2.0.exe") || !strings.Contains(lines[2], ".delta") || strings.Count(lines[2], "install.sh") != 1 || strings.Contains(lines[2], "App.exe ") || strings.Contains(lines[2], "update-") {
		t.Errorf("first upload: %s", lines[2])
	}
	if !strings.HasSuffix(lines[3], "update-darwin-arm64.json") {
		t.Errorf("the manifests must go last: %s", lines[3])
	}

	// Tagged releases leave the repository's latest release to the others.
	os.Remove(calls)
	c.Updates.TagPrefix = "desktop-v"
	if err := publishGitHub(c, artifacts); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(calls)
	if lines := strings.Split(strings.TrimSpace(string(b)), "\n"); len(lines) != 4 || !strings.HasPrefix(lines[1], "release create desktop-v1.2.0 --repo me/app --draft") || !strings.HasSuffix(lines[1], " --latest=false") {
		t.Errorf("gh calls:\n%s", b)
	}
}

// TestFetchSignedRetry retries a download that stalls, as a CDN can on a
// file it has not cached, or loses its connection, but not one that fails
// for good.
func TestFetchSignedRetry(t *testing.T) {
	stall, wait := fetchStall, updateRetryWait
	t.Cleanup(func() { fetchStall, updateRetryWait = stall, wait })
	fetchStall = 500 * time.Millisecond
	waitPastStall, waits := false, 0
	updateRetryWait = func(time.Duration) {
		waits++
		if waitPastStall {
			time.Sleep(2 * fetchStall) // as a long Retry-After does
		}
	}
	pub, priv, _ := ed25519.GenerateKey(nil)
	archive := bytes.Repeat([]byte("archive "), 10_000)
	sum := sha256.Sum256(archive)
	var (
		mu        sync.Mutex
		responses []string
		requests  int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		response := responses[min(requests, len(responses)-1)]
		requests++
		mu.Unlock()
		switch response {
		case "403", "404", "429", "503":
			code, _ := strconv.Atoi(response)
			w.WriteHeader(code)
			return
		case "short":
			w.Write(archive[:1000])
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(archive)))
		if response == "ok" {
			w.Write(archive)
			return
		}
		w.Write(archive[:1000])
		w.(http.Flusher).Flush()
		if response == "drop" {
			panic(http.ErrAbortHandler)
		}
		<-r.Context().Done() // stall
	}))
	defer srv.Close()
	get := (&Config{Updates: &Updates{URL: srv.URL}}).archiveGetter()
	for _, tc := range []struct {
		responses     []string
		requests      int
		err           string
		waitPastStall bool
	}{
		{[]string{"stall", "stall", "ok"}, 3, "", false},
		{[]string{"stall"}, fetchAttempts, "no data for", false},
		{[]string{"drop", "ok"}, 2, "", false},
		{[]string{"503", "ok"}, 2, "", false},
		{[]string{"429", "ok"}, 2, "", true},
		{[]string{"403"}, 1, "403", false},
		{[]string{"404"}, 1, "not published", false},
		{[]string{"short"}, 1, "has 1000 bytes", false},
	} {
		mu.Lock()
		responses, requests, waitPastStall, waits = tc.responses, 0, tc.waitPastStall, 0
		mu.Unlock()
		path := filepath.Join(t.TempDir(), "archive.tar.gz")
		err := fetchSigned(get, srv.URL+"/app-1.0.0.tar.gz", int64(len(archive)), update.Sign(priv, sum[:]), pub, path)
		if tc.err == "" && err != nil || tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)) {
			t.Errorf("%v: %v, want %q", tc.responses, err, tc.err)
		}
		if requests != tc.requests {
			t.Errorf("%v: %d requests, want %d", tc.responses, requests, tc.requests)
		}
		// A wait of get's retries is not a stall, which would add one of fetchSigned's.
		if tc.waitPastStall && waits != tc.requests-1 {
			t.Errorf("%v: waited %d times, want %d", tc.responses, waits, tc.requests-1)
		}
	}
}
