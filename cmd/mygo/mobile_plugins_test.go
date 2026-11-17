package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A plugin's native code goes into the host's sources with a registry, and
// what it asks of the app adds up.
func TestMobilePlugins(t *testing.T) {
	dir := t.TempDir()
	write := func(name, s string) {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ios/A.swift", "class APlugin {}")
	write("android/A.kt", "class APlugin")
	ps := mobilePlugins{
		{ID: "a", dir: dir,
			IOS:     &pluginIOS{Sources: []string{"ios/*.swift"}, Class: "APlugin", Frameworks: []string{"CoreLocation"}, InfoPlist: map[string]any{"UIBackgroundModes": []any{"audio"}, "K": "v"}},
			Android: &pluginAndroid{Sources: []string{"android/*.kt"}, Class: "APlugin", Permissions: []string{"CAMERA"}, Dependencies: []string{"x:y:1"}}},
		{ID: "b", dir: dir,
			IOS:     &pluginIOS{Class: "BPlugin", Frameworks: []string{"CoreLocation", "Photos"}, InfoPlist: map[string]any{"UIBackgroundModes": []any{"fetch"}}},
			Android: &pluginAndroid{Class: "BPlugin", Dependencies: []string{"x:y:1", "z:w:2"}, Application: []string{"<service />"}}},
	}
	swift := t.TempDir()
	if err := ps.writeSwift(swift); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(swift, "Plugin_a_A.swift")); err != nil {
		t.Error(err)
	}
	reg, _ := os.ReadFile(filepath.Join(swift, "PluginRegistry.swift"))
	if !strings.Contains(string(reg), `PluginHost.register("a", APlugin())`) || !strings.Contains(string(reg), `PluginHost.register("b", BPlugin())`) {
		t.Errorf("registry: %s", reg)
	}
	if got := ps.frameworks(); len(got) != 2 {
		t.Errorf("frameworks: %v", got)
	}
	if got := ps.infoPlist("dev.mygo.x")["UIBackgroundModes"].([]any); len(got) != 2 {
		t.Errorf("background modes: %v", got)
	}
	kt := t.TempDir()
	if err := os.WriteFile(filepath.Join(kt, "Plugin_gone_X.kt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ps.writeKotlin(kt); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(kt, "Plugin_gone_X.kt")); err == nil {
		t.Error("a plugin no longer imported stays")
	}
	if _, err := os.Stat(filepath.Join(kt, "Plugin_a_A.kt")); err != nil {
		t.Error(err)
	}
	if got := ps.dependencies(); len(got) != 2 {
		t.Errorf("dependencies: %v", got)
	}
	m := string(androidManifest(&Config{}, ps))
	if !strings.Contains(m, "android.permission.CAMERA") || !strings.Contains(m, "<service />") {
		t.Errorf("manifest: %s", m)
	}
	// A package may not reach outside itself.
	bad := &mobilePlugin{ID: "c", dir: dir}
	if _, err := bad.sourcesOf([]string{"../x.swift"}); err == nil {
		t.Error("a source outside the package")
	}
}

// What decides whether Gradle runs changes with what an APK holds, not with the
// app's Go code.
func TestSwiftStamp(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.swift")
	if err := os.WriteFile(src, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	obj := filepath.Join(t.TempDir(), "host.o")
	if swiftCompiled(dir, "arm64", obj) {
		t.Error("nothing compiled yet")
	}
	if err := os.WriteFile(obj, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	noteSwiftCompiled(dir, "arm64", obj)
	if !swiftCompiled(dir, "arm64", obj) {
		t.Error("just compiled")
	}
	if swiftCompiled(dir, "x86_64", obj) {
		t.Error("another target")
	}
	if err := os.WriteFile(src, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if swiftCompiled(dir, "arm64", obj) {
		t.Error("a source changed")
	}
}
