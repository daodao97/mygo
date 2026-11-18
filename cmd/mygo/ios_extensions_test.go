package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIOSExtensionsConfig(t *testing.T) {
	for config, want := range map[string]string{
		`{"ios": {"extensions": [{"path": "build/PacketTunnel"}]}}`:                                               "not an app extension",
		`{"ios": {"extensions": [{"path": ""}]}}`:                                                                 "not an app extension",
		`{"ios": {"extensions": [{"path": "a/Tunnel.appex"}, {"path": "b/Tunnel.appex"}]}}`:                       "another extension is Tunnel.appex",
		`{"ios": {"extensions": [{"path": "Tunnel.appex"}, {"path": "Widget.appex"}, {"path": "Tunnel.appex"}]}}`: "extensions[2]",
	} {
		dir := t.TempDir()
		writeBytes(t, filepath.Join(dir, jsonConfig), []byte(config))
		if _, err := loadConfig(dir); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", config, err)
		}
	}
	dir := t.TempDir()
	writeBytes(t, filepath.Join(dir, jsonConfig), []byte(`{"identifier": "com.example.vpn", "ios": {"extensions": [
		{"path": "build/PacketTunnel.appex/", "provisioningProfile": "PacketTunnel App Store"}]}}`))
	c, err := loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.IOS.Extensions) != 1 || c.IOS.Extensions[0].ProvisioningProfile != "PacketTunnel App Store" {
		t.Errorf("%+v", c.IOS.Extensions)
	}
}

func TestExtensionIdentifier(t *testing.T) {
	for _, tc := range []struct{ app, id, want string }{
		{"com.example.vpn", "com.example.vpn.tunnel", "com.example.vpn.tunnel"},
		{"com.example.vpn.dev", "com.example.vpn.tunnel", "com.example.vpn.dev.tunnel"}, // the development app's
		{"com.example.vpn", "com.example.vpn.a.b", "com.example.vpn.a.b"},
	} {
		if got, err := extensionIdentifier("com.example.vpn", tc.app, tc.id); err != nil || got != tc.want {
			t.Errorf("%s in %s: %q, %v", tc.id, tc.app, got, err)
		}
	}
	for _, id := range []string{"com.example.vpntunnel", "com.example.vpn", "com.example.vpn.", "com.other.tunnel", ""} {
		if _, err := extensionIdentifier("com.example.vpn", "com.example.vpn", id); err == nil || !strings.Contains(err.Error(), "com.example.vpn.") {
			t.Errorf("%q: %v", id, err)
		}
	}
}

func TestExtensionEntitlements(t *testing.T) {
	p := &profile{team: "TEAM", appID: "PREFIX.com.example.vpn.*", entitlements: map[string]any{
		"application-identifier":                          "PREFIX.com.example.vpn.*",
		"com.apple.developer.networking.networkextension": []any{"packet-tunnel-provider"},
		associatedDomainsKey:                              "*",
	}}
	ent := p.extensionEntitlements("com.example.vpn.tunnel")
	if ent["application-identifier"] != "PREFIX.com.example.vpn.tunnel" {
		t.Errorf("application-identifier: %v", ent["application-identifier"])
	}
	if _, ok := ent["com.apple.developer.networking.networkextension"]; !ok {
		t.Errorf("the profile's capability is gone: %v", ent)
	}
	if _, ok := ent[associatedDomainsKey]; ok {
		t.Errorf("the profile's wildcard domains: %v", ent)
	}
	if p.entitlements["application-identifier"] != "PREFIX.com.example.vpn.*" {
		t.Error("the profile's own changed")
	}
	if got := (&profile{team: "TEAM"}).extensionEntitlements("x.y")["application-identifier"]; got != "TEAM.x.y" {
		t.Errorf("no app ID: %v", got)
	}
}

// recordIOSCodesign records the calls of codesign for the test, as the paths
// relative to dir they sign.
func recordIOSCodesign(t *testing.T, dir string) *[]string {
	t.Helper()
	var calls []string
	old := iosCodesign
	iosCodesign = func(args ...string) error {
		path, _ := filepath.Rel(dir, args[len(args)-1])
		calls = append(calls, strings.Join(append(args[:len(args)-1:len(args)-1], filepath.ToSlash(path)), " "))
		return nil
	}
	t.Cleanup(func() { iosCodesign = old })
	return &calls
}

func extensionBundle(t *testing.T) (dir, bundle string, exts []*iosExtension) {
	dir = t.TempDir()
	src := filepath.Join(dir, "build", "PacketTunnel.appex")
	writeBytes(t, filepath.Join(src, "Info.plist"), []byte("<plist/>"))
	writeBytes(t, filepath.Join(src, "PacketTunnel"), []byte("binary"))
	os.Chmod(filepath.Join(src, "PacketTunnel"), 0o755)
	writeBytes(t, filepath.Join(src, "Frameworks", "Core.framework", "Core"), []byte("framework"))
	bundle = filepath.Join(dir, "App.app")
	writeBytes(t, filepath.Join(bundle, "App"), []byte("app"))
	writeBytes(t, filepath.Join(bundle, "Frameworks", "Module.framework", "Module"), []byte("module"))
	exts = []*iosExtension{{src: src, name: "PacketTunnel.appex", srcID: "com.example.vpn.tunnel", id: "com.example.vpn.tunnel"}}
	if err := embedExtensions(bundle, exts); err != nil {
		t.Fatal(err)
	}
	return dir, bundle, exts
}

func TestEmbedExtensions(t *testing.T) {
	_, bundle, exts := extensionBundle(t)
	exe := filepath.Join(bundle, "PlugIns", "PacketTunnel.appex", "PacketTunnel")
	// Windows keeps no executable bit, which only a Mac's build needs kept.
	if info, err := os.Stat(exe); err != nil || runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Errorf("the extension's executable: %v, %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(bundle, "PlugIns", "PacketTunnel.appex", "Frameworks", "Core.framework", "Core")); err != nil {
		t.Error(err)
	}
	// A new build replaces what was there.
	stale := filepath.Join(bundle, "PlugIns", "PacketTunnel.appex", "stale")
	writeBytes(t, stale, nil)
	if err := embedExtensions(bundle, exts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("the old extension's files stay")
	}
}

// The extension is signed, after its frameworks, with its own profile
// embedded and its entitlements, before the app, which is then verified
// with what it holds.
func TestSignWithExtensions(t *testing.T) {
	dir, bundle, exts := extensionBundle(t)
	work := filepath.Join(dir, "work")
	os.MkdirAll(work, 0o755)
	writeBytes(t, filepath.Join(dir, "app.mobileprovision"), []byte("app profile"))
	writeBytes(t, filepath.Join(dir, "tunnel.mobileprovision"), []byte("tunnel profile"))
	app := &profile{path: filepath.Join(dir, "app.mobileprovision"), name: "app", team: "TEAM", appID: "TEAM.com.example.vpn", identity: "CERT",
		entitlements: map[string]any{"application-identifier": "TEAM.com.example.vpn"}}
	exts[0].prof = &profile{path: filepath.Join(dir, "tunnel.mobileprovision"), name: "tunnel", team: "TEAM", appID: "TEAM.com.example.vpn.tunnel", identity: "CERT",
		entitlements: map[string]any{"application-identifier": "TEAM.com.example.vpn.tunnel", "com.apple.developer.networking.networkextension": []any{"packet-tunnel-provider"}}}
	calls := recordIOSCodesign(t, dir)
	c := &Config{root: dir, Identifier: "com.example.vpn"}
	if err := app.sign(work, bundle, c, exts); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"--force --sign CERT App.app/Frameworks/Module.framework",
		"--force --sign CERT App.app/PlugIns/PacketTunnel.appex/Frameworks/Core.framework",
		"--force --sign CERT --entitlements " + filepath.Join(work, "entitlements-PacketTunnel.plist") + " --generate-entitlement-der App.app/PlugIns/PacketTunnel.appex",
		"--force --sign CERT --entitlements " + filepath.Join(work, "entitlements.plist") + " --generate-entitlement-der App.app",
		"--verify --deep --strict App.app",
	}
	if strings.Join(*calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("codesign:\n%s\nwant:\n%s", strings.Join(*calls, "\n"), strings.Join(want, "\n"))
	}
	read := func(path string) string { return string(mustRead(t, path)) }
	if got := read(filepath.Join(bundle, "PlugIns", "PacketTunnel.appex", "embedded.mobileprovision")); got != "tunnel profile" {
		t.Errorf("the extension embeds %q", got)
	}
	if got := read(filepath.Join(bundle, "embedded.mobileprovision")); got != "app profile" {
		t.Errorf("the app embeds %q", got)
	}
	ent := read(filepath.Join(work, "entitlements-PacketTunnel.plist"))
	if !strings.Contains(ent, "<string>TEAM.com.example.vpn.tunnel</string>") || !strings.Contains(ent, "<string>packet-tunnel-provider</string>") {
		t.Errorf("the extension's entitlements: %s", ent)
	}
	if ent := read(filepath.Join(work, "entitlements.plist")); strings.Contains(ent, "packet-tunnel-provider") || !strings.Contains(ent, "<string>TEAM.com.example.vpn</string>") {
		t.Errorf("the app's entitlements: %s", ent)
	}
}

// The simulator signs extensions ad hoc, as it does frameworks, before the
// app.
func TestSignForSimulatorWithExtensions(t *testing.T) {
	dir, bundle, _ := extensionBundle(t)
	calls := recordIOSCodesign(t, dir)
	if err := signForSimulator(bundle); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"--force --sign - App.app/Frameworks/Module.framework",
		"--force --sign - App.app/PlugIns/PacketTunnel.appex/Frameworks/Core.framework",
		"--force --sign - App.app/PlugIns/PacketTunnel.appex",
		"--force --sign - App.app",
	}
	if strings.Join(*calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("codesign:\n%s\nwant:\n%s", strings.Join(*calls, "\n"), strings.Join(want, "\n"))
	}
}

// iosExtensions reads the identifier from the extension's Info.plist, with
// plutil (macOS).
func TestIOSExtensionsRead(t *testing.T) {
	if _, err := exec.LookPath("plutil"); err != nil {
		t.Skip("no plutil")
	}
	dir := t.TempDir()
	plist := func(id string) []byte {
		return plistBytes(map[string]any{"CFBundleIdentifier": id, "CFBundlePackageType": "XPC!"})
	}
	writeBytes(t, filepath.Join(dir, "build", "Tunnel.appex", "Info.plist"), plist("com.example.vpn.tunnel"))
	writeBytes(t, filepath.Join(dir, "build", "Other.appex", "Info.plist"), plist("com.other.tunnel"))
	c := &Config{root: dir, Identifier: "com.example.vpn", IOS: IOS{Extensions: []IOSExtension{{Path: "build/Tunnel.appex"}}}}
	exts, err := iosExtensions(c, devConfig(c))
	if err != nil {
		t.Fatal(err)
	}
	if len(exts) != 1 || exts[0].srcID != "com.example.vpn.tunnel" || exts[0].id != "com.example.vpn.dev.tunnel" || exts[0].name != "Tunnel.appex" {
		t.Errorf("%+v", exts[0])
	}
	bundle := filepath.Join(dir, "App.app")
	if err := embedExtensions(bundle, exts); err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(t, filepath.Join(bundle, "PlugIns", "Tunnel.appex", "Info.plist"))); !strings.Contains(got, "com.example.vpn.dev.tunnel") {
		t.Errorf("the development app's extension: %s", got)
	}
	c.IOS.Extensions = []IOSExtension{{Path: "build/Other.appex"}}
	if _, err := iosExtensions(c, c); err == nil || !strings.Contains(err.Error(), "does not start with com.example.vpn.") {
		t.Errorf("another app's: %v", err)
	}
	c.IOS.Extensions = []IOSExtension{{Path: "build/Missing.appex"}}
	if _, err := iosExtensions(c, c); err == nil || !strings.Contains(err.Error(), "not a built app extension") {
		t.Errorf("missing: %v", err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
