package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/internal/nativelib"
)

// TestNatives puts the native libraries that a package of the app names in
// its mygo-plugin.json among the app's resources, downloaded once into
// the user's cache: a universal app gets both architectures in one file.
func TestNatives(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	arm := thinMachO(binary.LittleEndian, 0x19)
	files := map[string][]byte{
		"/libx-darwin-arm64.dylib": arm,
		"/libx-darwin-amd64.dylib": withCPU(arm, cpuAMD64),
		"/libx-linux-amd64.so":     []byte("\x7fELF linux"),
		"/libx-bad.so":             []byte("not what the sum says"),
	}
	downloads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads++
		w.Write(files[r.URL.Path])
	}))
	defer srv.Close()
	entry := func(name, path string, content []byte) string {
		sum := sha256.Sum256(content)
		return fmt.Sprintf(`{"name": %q, "url": %q, "sha256": %q}`, name, srv.URL+path, hex.EncodeToString(sum[:]))
	}
	manifest := `{"libraries": [{"name": "libx", "files": {` +
		`"darwin-arm64": ` + entry("libx.dylib", "/libx-darwin-arm64.dylib", files["/libx-darwin-arm64.dylib"]) + `,` +
		`"darwin-amd64": ` + entry("libx.dylib", "/libx-darwin-amd64.dylib", files["/libx-darwin-amd64.dylib"]) + `,` +
		`"linux-amd64": ` + entry("libx.so", "/libx-linux-amd64.so", files["/libx-linux-amd64.so"]) + `,` +
		`"linux-arm64": ` + entry("libx.so", "/libx-bad.so", []byte("the right file")) + `}}]}`
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":              "module example.com/app\n\ngo 1.27\n",
		"main.go":             "package main\n\nimport _ \"example.com/app/lib\"\n\nfunc main() {}\n",
		"lib/lib.go":          "package lib\n",
		"lib/" + pluginFile:   manifest,
		"other/other.go":      "package other\n", // not imported
		"other/" + pluginFile: `{"libraries": [{"name": "unused", "files": {}}]}`,
		"mygo.json":           `{"name": "App"}`,
	})
	cache := t.TempDir()
	switch runtime.GOOS {
	case "darwin", "ios":
		t.Setenv("HOME", cache)
		cache = filepath.Join(cache, "Library", "Caches")
	case "windows":
		t.Setenv("LocalAppData", cache)
	default:
		t.Setenv("XDG_CACHE_HOME", cache)
	}
	c, err := loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}

	list, err := c.natives("linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].name != "libx.so" {
		t.Fatalf("natives = %+v", list)
	}
	if b, err := os.ReadFile(list[0].path); err != nil || !bytes.Equal(b, files["/libx-linux-amd64.so"]) {
		t.Errorf("the library holds %q, %v", b, err)
	}
	if !strings.HasPrefix(list[0].path, filepath.Join(cache, "mygo", "natives")+string(filepath.Separator)) {
		t.Errorf("the library is at %s, not in the cache %s", list[0].path, cache)
	}
	// It is downloaded once.
	if _, err := c.natives("linux", "amd64"); err != nil || downloads != 1 {
		t.Errorf("downloads = %d, %v", downloads, err)
	}

	natives, err := c.natives("darwin", "universal")
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.resourcesWith("darwin", "universal", natives)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range res {
		if r.name == "libx.dylib" {
			found = r.lipo != ""
		}
	}
	if !found {
		t.Errorf("no universal libx.dylib among %+v", res)
	}

	if _, err := c.natives("windows", "amd64"); err == nil || !strings.Contains(err.Error(), "libx, which has no build for windows/amd64") {
		t.Errorf("a platform without a build: %v", err)
	}
	if _, err := c.natives("linux", "arm64"); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Errorf("a download with another SHA-256: %v", err)
	}
	// A cached library that changed is downloaded again.
	if err := os.WriteFile(list[0].path, []byte("changed"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := c.natives("linux", "amd64"); err != nil || downloads != 5 {
		t.Errorf("downloads = %d, %v", downloads, err)
	}
	// Names and sums that would reach outside the cache are refused.
	for _, f := range []nativelib.File{
		{Name: "..", URL: srv.URL, SHA256: strings.Repeat("a", 64)},
		{Name: "lib/../../x", URL: srv.URL, SHA256: strings.Repeat("a", 64)},
		{Name: "x.so", URL: srv.URL, SHA256: strings.Repeat("../", 20) + "etc/"},
		{Name: "x.so", URL: srv.URL, SHA256: strings.Repeat("A", 64)},
	} {
		if _, err := fetchNative(f); err == nil || !strings.Contains(err.Error(), "invalid native library") {
			t.Errorf("fetchNative(%+v) = %v", f, err)
		}
	}
	// A resource of the app may not take the library's place.
	writeFiles(t, dir, map[string]string{"resources/libx.so": "mine"})
	if _, err := c.appResources("linux", "amd64"); err == nil || !strings.Contains(err.Error(), "would both be installed as libx.so") {
		t.Errorf("a resource named as the library: %v", err)
	}
}

// TestNativesFromSources builds a library of the app's own package from
// its Rust crate with cargo, puts it among the app's resources, and binds
// it. A package in the module cache downloads the published build instead,
// and fails without one.
func TestNativesFromSources(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list and cargo")
	}
	if _, err := exec.LookPath("cargo"); err != nil {
		t.Skip("cargo is not installed")
	}
	// go list reports the directories of packages with their links
	// resolved, as macOS's /var in /private/var.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeFiles(t, dir, map[string]string{
		"go.mod":            "module example.com/app\n\ngo 1.27\n",
		"main.go":           "package main\n\nimport _ \"example.com/app/lib\"\n\nfunc main() {}\n",
		"lib/lib.go":        "package lib\n",
		"lib/" + pluginFile: `{"libraries": [{"name": "adder", "build": {"cargo": "./rust"}}]}`,
		"lib/rust/Cargo.toml": "[package]\nname = \"adder\"\nversion = \"0.1.0\"\nedition = \"2021\"\n\n" +
			"[lib]\ncrate-type = [\"cdylib\"]\n",
		"lib/rust/src/lib.rs": "#[no_mangle]\npub extern \"C\" fn add(a: i32, b: i32) -> i32 { a + b }\n",
		"mygo.json":           `{"name": "App"}`,
	})
	c, err := loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	list, err := c.natives(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(nativelib.LocalDir(dir, runtime.GOOS, runtime.GOARCH), nativelib.FileName("adder", runtime.GOOS))
	if len(list) != 1 || list[0].path != want || list[0].name != filepath.Base(want) {
		t.Fatalf("natives = %+v, want %s", list, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "lib", "rust", "target")); err == nil {
		t.Error("cargo wrote into the crate")
	}
	if add, err := loadAdd(want); err != nil {
		t.Fatal(err)
	} else if add != nil && add(40, 2) != 42 {
		t.Errorf("add(40, 2) = %d", add(40, 2))
	}

	// A published build of these sources serves the app instead, until the
	// sources change.
	built, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(built) }))
	defer srv.Close()
	// A cache of its own, where macOS's is under $HOME, as rustup's and
	// cargo's homes are unless their variables name them.
	if home, err := os.UserHomeDir(); err == nil {
		for v, dir := range map[string]string{"RUSTUP_HOME": ".rustup", "CARGO_HOME": ".cargo"} {
			if os.Getenv(v) == "" {
				t.Setenv(v, filepath.Join(home, dir))
			}
		}
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LocalAppData", t.TempDir())
	build := &nativelib.Build{Cargo: "./rust"}
	sources, err := nativelib.SourceHash(filepath.Join(dir, "lib", "rust"), build)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(built)
	manifest := nativelib.Manifest{Libraries: []nativelib.Library{{Name: "adder", Build: build, Files: map[string]nativelib.File{
		runtime.GOOS + "-" + runtime.GOARCH: {Name: filepath.Base(want), URL: srv.URL, SHA256: hex.EncodeToString(sum[:]), SourceHash: sources},
	}}}}
	if err := writeManifest(filepath.Join(dir, "lib", pluginFile), manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(want); err != nil {
		t.Fatal(err)
	}
	if list, err := c.natives(runtime.GOOS, runtime.GOARCH); err != nil || len(list) != 1 || list[0].path == want {
		t.Fatalf("with a published build of the sources, natives = %+v, %v", list, err)
	}
	writeFiles(t, dir, map[string]string{"lib/rust/src/lib.rs": "#[no_mangle]\npub extern \"C\" fn add(a: i32, b: i32) -> i32 { a.wrapping_add(b) }\n"})
	if list, err := c.natives(runtime.GOOS, runtime.GOARCH); err != nil || len(list) != 1 || list[0].path != want {
		t.Fatalf("with changed sources, natives = %+v, %v", list, err)
	}

	// The same package in the module cache has no published build.
	writeFiles(t, dir, map[string]string{"lib/" + pluginFile: `{"libraries": [{"name": "adder", "build": {"cargo": "./rust"}}]}`})
	old := goEnv("GOMODCACHE")
	goEnvCache.Store("GOMODCACHE", dir)
	defer goEnvCache.Store("GOMODCACHE", old)
	if _, err := c.natives(runtime.GOOS, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), "has no build for") {
		t.Errorf("a package of the module cache: %v", err)
	}
}

func TestNativeTargets(t *testing.T) {
	if list, err := nativeTargets("all"); err != nil || len(list) != 6 {
		t.Errorf("all = %v, %v", list, err)
	}
	if list, err := nativeTargets("linux-amd64,windows-arm64"); err != nil || len(list) != 2 {
		t.Errorf("a list = %v, %v", list, err)
	}
	if _, err := nativeTargets("plan9-amd64"); err == nil {
		t.Error("accepted an unknown platform")
	}
	repo, tag, err := githubRelease("https://github.com/me/app/releases/download/adder-v1")
	if err != nil || repo != "me/app" || tag != "adder-v1" {
		t.Errorf("githubRelease = %q, %q, %v", repo, tag, err)
	}
	for _, bad := range []string{"https://example.com/me/app/releases/download/v1", "https://github.com/me/app", "::"} {
		if _, _, err := githubRelease(bad); err == nil {
			t.Errorf("githubRelease(%q) accepted it", bad)
		}
	}
}

// TestPackagesInDirectoryWithSpaces finds the packages of a project whose
// path has spaces, as go list prints it.
func TestPackagesInDirectoryWithSpaces(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	dir := filepath.Join(t.TempDir(), "My  Projects", "app")
	writeFiles(t, dir, map[string]string{
		"go.mod":            "module example.com/app\n\ngo 1.27\n",
		"main.go":           "package main\n\nimport _ \"example.com/app/lib\"\n\nfunc main() {}\n",
		"lib/lib.go":        "package lib\n",
		"lib/" + pluginFile: `{"libraries": [{"name": "libx", "files": {}}]}`,
		"mygo.json":         `{"name": "App"}`,
	})
	c, err := loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	lib := filepath.Join(dir, "lib")
	if list, err := c.nativePackages("linux", "amd64"); err != nil || len(list) != 1 || !sameDir(list[0].dir, lib) {
		t.Errorf("the app's native packages: %v, %v", list, err)
	}
	if list, err := nativePackagesIn(dir); err != nil || len(list) != 1 || !sameDir(list[0].dir, lib) {
		t.Errorf("the project's native packages: %v, %v", list, err)
	}
	if dirs := compiledDirs(c); !slices.ContainsFunc(dirs, func(d string) bool { return sameDir(d, lib) }) {
		t.Errorf("the app's packages lack %s: %q", lib, dirs)
	}
}
