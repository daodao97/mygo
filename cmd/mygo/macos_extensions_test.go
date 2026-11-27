package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestMacOSExtensionsConfig(t *testing.T) {
	for config, want := range map[string]string{
		`{"macos": {"extensions": [{"path": "build/ShareExtension"}]}}`:                                           "not an app extension",
		`{"macos": {"extensions": [{"path": ""}]}}`:                                                               "not an app extension",
		`{"macos": {"extensions": [{"path": "a/Share.appex"}, {"path": "b/Share.appex"}]}}`:                       "another extension is Share.appex",
		`{"macos": {"extensions": [{"path": "Share.appex"}, {"path": "Action.appex"}, {"path": "Share.appex"}]}}`: "extensions[2]",
	} {
		dir := t.TempDir()
		writeBytes(t, filepath.Join(dir, jsonConfig), []byte(config))
		if _, err := loadConfig(dir); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", config, err)
		}
	}
	dir := t.TempDir()
	writeBytes(t, filepath.Join(dir, jsonConfig), []byte(`{"identifier": "com.example.app", "macos": {"extensions": [{"path": "build/Share.appex/"}]}}`))
	c, err := loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.MacOS.Extensions) != 1 || c.MacOS.Extensions[0].Path != "build/Share.appex/" {
		t.Errorf("%+v", c.MacOS.Extensions)
	}
}

// macOSExtensionProject writes a project whose app, com.example.app,
// carries build/Share.appex, of the identifier id, and returns its
// configuration and an executable for writeBundle.
func macOSExtensionProject(t *testing.T, id string) (c *Config, bin string) {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		jsonConfig:                               `{"name": "App", "identifier": "com.example.app", "macos": {"extensions": [{"path": "build/Share.appex"}]}}`,
		"build/Share.appex/Contents/Info.plist":  string(infoPlist(&Config{Name: "Share", Identifier: id}, "Share", "", "")),
		"build/Share.appex/Contents/MacOS/Share": "extension",
		"app":                                    "executable",
	})
	if err := os.Chmod(filepath.Join(dir, "build", "Share.appex", "Contents", "MacOS", "Share"), 0o755); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	return c, filepath.Join(dir, "app")
}

// The app bundle carries the extension in Contents/PlugIns, as it was
// built; the development app carries none.
func TestMacOSExtensionsBundle(t *testing.T) {
	c, bin := macOSExtensionProject(t, "com.example.app.share")
	app, err := writeBundle(c, t.TempDir(), bin, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ext := filepath.Join(app, "Contents", "PlugIns", "Share.appex")
	if data, err := os.ReadFile(filepath.Join(ext, "Contents", "Info.plist")); err != nil || !strings.Contains(string(data), "<string>com.example.app.share</string>") {
		t.Errorf("the extension's Info.plist: %s, %v", data, err)
	}
	// Windows keeps no executable bit, which only a Mac's build needs kept.
	if info, err := os.Stat(filepath.Join(ext, "Contents", "MacOS", "Share")); err != nil || runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Errorf("the extension's executable: %v, %v", info, err)
	}

	writeBytes(t, bin, []byte("executable"))
	dev, err := writeBundle(devConfig(c), t.TempDir(), bin, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dev, "Contents", "PlugIns")); !os.IsNotExist(err) {
		t.Errorf("the development app carries the extensions: %v", err)
	}
	if len(c.MacOS.Extensions) != 1 {
		t.Error("devConfig changed the app's configuration")
	}
}

func TestMacOSExtensionsRefused(t *testing.T) {
	for _, id := range []string{"com.example.share", "com.example.appshare", "com.example.app", "com.example.app.", ""} {
		c, bin := macOSExtensionProject(t, id)
		if _, err := writeBundle(c, t.TempDir(), bin, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "does not start with com.example.app.") {
			t.Errorf("%q: %v", id, err)
		}
	}

	c, bin := macOSExtensionProject(t, "com.example.app.share")
	if err := os.RemoveAll(c.path("build")); err != nil {
		t.Fatal(err)
	}
	if _, err := writeBundle(c, t.TempDir(), bin, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "macos.extensions[0]: build/Share.appex is not a built app extension") {
		t.Errorf("no extension: %v", err)
	}

	if runtime.GOOS != "darwin" {
		c, bin := macOSExtensionProject(t, "com.example.app.share")
		writeBytes(t, c.path("build/Share.appex/Contents/Info.plist"), []byte("bplist00"))
		if _, err := writeBundle(c, t.TempDir(), bin, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "plutil -convert xml1") {
			t.Errorf("a binary Info.plist off macOS: %v", err)
		}
	}
}

// An app without extensions is signed at once, with --deep, as it always
// was. An app with extensions is signed from the inside out, its
// extensions keeping their entitlements, and the app last without --deep,
// which would sign them with the app's.
func TestAppCodesigning(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "App.app")
	writeFiles(t, app, map[string]string{
		"Contents/MacOS/App":                      "app",
		"Contents/Frameworks/libmodule.dylib":     "module",
		"Contents/Frameworks/Core.framework/Core": "framework",
		"Contents/Resources/bin/helper":           "helper",
	})
	calls := func(identity, entitlements string) []string {
		t.Helper()
		got, err := appCodesigning(app, identity, entitlements, true)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, args := range got {
			path, _ := filepath.Rel(dir, args[len(args)-1])
			out = append(out, strings.Join(append(args[:len(args)-1:len(args)-1], filepath.ToSlash(path)), " "))
		}
		return out
	}
	const id = "Developer ID Application: X"
	if got, want := calls(id, "e.plist"), []string{
		"--force --deep --sign " + id + " --options runtime --timestamp --entitlements e.plist App.app",
	}; !slices.Equal(got, want) {
		t.Errorf("without extensions:\n%s", strings.Join(got, "\n"))
	}

	writeFiles(t, app, map[string]string{"Contents/PlugIns/Share.appex/Contents/MacOS/Share": "extension"})
	if got, want := calls(id, "e.plist"), []string{
		"--force --sign " + id + " --options runtime --timestamp --preserve-metadata=entitlements --deep App.app/Contents/Frameworks/Core.framework",
		"--force --sign " + id + " --options runtime --timestamp --preserve-metadata=entitlements --deep App.app/Contents/Frameworks/libmodule.dylib",
		"--force --sign " + id + " --options runtime --timestamp --preserve-metadata=entitlements --deep App.app/Contents/PlugIns/Share.appex",
		"--force --sign " + id + " --options runtime --timestamp --entitlements e.plist App.app",
	}; !slices.Equal(got, want) {
		t.Errorf("with extensions:\n%s", strings.Join(got, "\n"))
	}
	if got, want := calls("-", ""), []string{
		"--force --sign - --preserve-metadata=entitlements --deep App.app/Contents/Frameworks/Core.framework",
		"--force --sign - --preserve-metadata=entitlements --deep App.app/Contents/Frameworks/libmodule.dylib",
		"--force --sign - --preserve-metadata=entitlements --deep App.app/Contents/PlugIns/Share.appex",
		"--force --sign - App.app",
	}; !slices.Equal(got, want) {
		t.Errorf("ad hoc, with extensions:\n%s", strings.Join(got, "\n"))
	}

	if err := os.RemoveAll(filepath.Join(app, "Contents", "Frameworks")); err != nil {
		t.Fatal(err)
	}
	if got, want := calls("-", ""), []string{
		"--force --sign - --preserve-metadata=entitlements --deep App.app/Contents/PlugIns/Share.appex",
		"--force --sign - App.app",
	}; !slices.Equal(got, want) {
		t.Errorf("with extensions, without frameworks:\n%s", strings.Join(got, "\n"))
	}
}

// TestSignMacOSExtensions signs an app with an extension, which keeps the
// App Sandbox it was signed with, while the app gets its own entitlements
// and its signature seals the extension.
func TestSignMacOSExtensions(t *testing.T) {
	if _, err := exec.LookPath("codesign"); err != nil || runtime.GOOS != "darwin" {
		t.Skip("needs codesign")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	plist := func(key string) string {
		return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>` + key + `</key><true/></dict></plist>
`
	}
	c, bin := macOSExtensionProject(t, "com.example.app.share")
	c.MacOS.Entitlements = "app.plist"
	writeFiles(t, c.root, map[string]string{
		"app.plist":   plist("com.apple.security.cs.allow-jit"),
		"share.plist": plist("com.apple.security.app-sandbox"),
	})
	src := c.path("build/Share.appex")
	if err := copyFile(exe, filepath.Join(src, "Contents", "MacOS", "Share"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("codesign", "--force", "--sign", "-", "--entitlements", c.path("share.plist"), src).CombinedOutput(); err != nil {
		t.Fatalf("codesign the extension: %v\n%s", err, out)
	}
	if err := copyFile(exe, bin, 0o755); err != nil {
		t.Fatal(err)
	}
	app, err := writeBundle(c, t.TempDir(), bin, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// A library, as native modules put in Contents/Frameworks.
	if err := os.MkdirAll(filepath.Join(app, "Contents", "Frameworks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(exe, filepath.Join(app, "Contents", "Frameworks", "libmodule.dylib"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := codesign(c, app, "-", true); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("codesign", "--verify", "--deep", "--strict", app).CombinedOutput(); err != nil {
		t.Errorf("the app does not verify: %v\n%s", err, out)
	}
	entitlements := func(path string) string {
		out, err := exec.Command("codesign", "--display", "--entitlements", "-", "--xml", path).CombinedOutput()
		if err != nil {
			t.Errorf("codesign --display %s: %v\n%s", path, err, out)
		}
		return string(out)
	}
	if out := entitlements(filepath.Join(app, "Contents", "PlugIns", "Share.appex")); !strings.Contains(out, "app-sandbox") || strings.Contains(out, "allow-jit") {
		t.Errorf("the extension's entitlements are not its own:\n%s", out)
	}
	if out := entitlements(app); !strings.Contains(out, "allow-jit") {
		t.Errorf("the app lacks its entitlements:\n%s", out)
	}
}
