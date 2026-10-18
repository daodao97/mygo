package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIOSExtensionValidation(t *testing.T) {
	c := Config{Identifier: "org.example.app", Version: "1.2.0", IOS: IOS{AppGroups: []string{"group.org.example.app"}, Extensions: []IOSExtension{{Name: "Status", ExtensionPoint: "com.apple.widgetkit-extension", MinimumSystemVersion: "18.0"}}}}
	if err := validateIOSPackaging(&c, buildOptions{}); err != nil {
		t.Fatal(err)
	}
	bad := []IOSExtension{
		{Name: "MyGoApp", ExtensionPoint: "com.apple.widgetkit-extension"},
		{Name: "Bad/name", ExtensionPoint: "com.apple.widgetkit-extension"},
		{Name: "Status", ExtensionPoint: "com.apple.widgetkit-extension", BundleIdentifier: "org.other.Status"},
		{Name: "Status"},
		{Name: "Status", ExtensionPoint: "com.apple.widgetkit-extension", MinimumSystemVersion: "14.0"},
		{Name: "Status", ExtensionPoint: "com.apple.widgetkit-extension", Native: IOSNative{Support: []string{"mystery"}}},
	}
	for _, e := range bad {
		copy := c
		copy.IOS.Extensions = []IOSExtension{e}
		if err := validateIOSPackaging(&copy, buildOptions{}); err == nil {
			t.Fatalf("accepted %+v", e)
		}
	}
	c.IOS.Signing = IOSSigning{Style: "manual", Identity: "Apple Development", ProvisioningProfile: "host"}
	if err := validateIOSPackaging(&c, buildOptions{}); err == nil {
		t.Fatal("missing extension profile accepted")
	}
	c.IOS.Signing.ExportProfiles = map[string]string{"org.example.app.Status": "extension"}
	if err := validateIOSPackaging(&c, buildOptions{}); err != nil {
		t.Fatal(err)
	}
}
func TestIOSNativeStageAndProject(t *testing.T) {
	root := t.TempDir()
	host := filepath.Join(root, "host")
	if err := os.MkdirAll(host, 0755); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string]string{"shared/Actions.swift": "import Foundation\n", "tunnel/Provider.m": "#import \"Bridge.h\"\n", "tunnel/Bridge.h": "void core(void);", "libcore.a": "archive-fixture", "entry.m": "int main(){ return 0; }"} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	c := Config{root: root, Identifier: "org.example.app", Version: "2.0.0", IOS: IOS{BuildNumber: "7", AppGroups: []string{"group.org.example.app"}, Native: IOSNative{Sources: []string{"shared"}, Support: []string{"actions"}, EntryPoint: "entry.m"}, Extensions: []IOSExtension{
		{Name: "Controls", ExtensionPoint: "com.apple.widgetkit-extension", MinimumSystemVersion: "18.0", Native: IOSNative{Sources: []string{"shared"}, Support: []string{"actions", "widgets"}, SwiftConditions: []string{"EXTENSION"}}},
		{Name: "Tunnel", ExtensionPoint: "com.apple.networkextension.packet-tunnel", Native: IOSNative{Sources: []string{"tunnel"}, Libraries: []string{"libcore.a"}, Frameworks: []string{"NetworkExtension"}}, AppGroups: []string{"group.org.example.app"}},
	}}}
	targets, err := iosPrepareNative(&c, host, []byte("privacy-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	project := iosNativeProject(iosProject(&c, "15.0", "TEAM"), &c, "TEAM", targets)
	for _, want := range []string{"Embed App Extensions", "PBXTargetDependency", "Controls.appex", "Tunnel.appex", "org.example.app.Tunnel", "MyGoActions.swift", "MyGoWidgets.swift", "libcore.a", "sourcecode.swift", "HEADER_SEARCH_PATHS", "APPLICATION_EXTENSION_API_ONLY = YES", "18.0", "-weak_framework", "ALWAYS_EMBED_SWIFT_STANDARD_LIBRARIES", "LD_RUNPATH_SEARCH_PATHS"} {
		if !strings.Contains(project, want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(project, root) {
		t.Fatal("project references nonportable source tree")
	}
	info, err := os.ReadFile(filepath.Join(host, "Extensions/Tunnel/Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"com.apple.networkextension.packet-tunnel", "2.0.0", "7"} {
		if !strings.Contains(string(info), want) {
			t.Errorf("missing metadata %s", want)
		}
	}
	ent, _ := os.ReadFile(filepath.Join(host, "Extensions/Tunnel/Target.entitlements"))
	if !strings.Contains(string(ent), "group.org.example.app") {
		t.Fatal("missing group")
	}
	main, _ := os.ReadFile(filepath.Join(host, "main.m"))
	if string(main) != "int main(){ return 0; }" {
		t.Fatal("entry override ignored")
	}
}

func TestIOSNativeCopyRejectsExternalLinks(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "input")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "external.h"), []byte("external"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../external.h", filepath.Join(source, "link.h")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if err := iosCopyNative(source, filepath.Join(root, "copy")); err == nil {
		t.Fatal("export accepted external source dependency")
	}
	if err := os.Remove(filepath.Join(source, "link.h")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "real.h"), []byte("internal"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.h", filepath.Join(source, "link.h")); err != nil {
		t.Fatal(err)
	}
	if err := iosCopyNative(source, filepath.Join(root, "portable")); err != nil {
		t.Fatal(err)
	}
}
