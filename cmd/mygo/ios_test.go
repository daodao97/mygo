package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIOSBundleMetadata(t *testing.T) {
	c := &Config{Name: "Go & UI", Identifier: "com.example.native", Version: "1.2.3", URLSchemes: []string{"native"}, IOS: IOS{InfoPlist: map[string]any{"NSCameraUsageDescription": "Scan & save"}}}
	data := string(iosInfoPlist(c, "15.0"))
	for _, want := range []string{"Go &amp; UI", "com.example.native", "1.2.3", "CFBundleURLTypes", "native", "Scan &amp; save", "UILaunchStoryboardName", "UIApplicationSceneManifest", "MyGoScene", "UIInterfaceOrientationLandscapeRight"} {
		if !strings.Contains(data, want) {
			t.Errorf("iOS plist is missing %q", want)
		}
	}
	if strings.Contains(data, "NSPrincipalClass") || strings.Contains(data, "NSHighResolutionCapable") {
		t.Fatal("iOS inherited AppKit metadata")
	}
}

func TestIOSProjectLinksHostedEntry(t *testing.T) {
	p := iosProject(&Config{Identifier: "com.example.native"}, "15.0", "TEAMID")
	for _, want := range []string{"-force_load", "$(PROJECT_DIR)/libmygo.a", "UIKit", "Metal", "CoreText", "AVFoundation", "Photos", "PhotosUI", "UniformTypeIdentifiers", "CoreLocation", "UserNotifications", `DEVELOPMENT_TEAM = "TEAMID"`, "Resources"} {
		if !strings.Contains(p, want) {
			t.Errorf("iOS host is missing %q", want)
		}
	}
	if strings.Contains(p, "path = Resources;") || !strings.Contains(p, "path = MyGoResources;") {
		t.Fatal("a flat iOS bundle must not have a root Resources directory")
	}
	if strings.Contains(p, "ASSETCATALOG_COMPILER_APPICON_NAME") {
		t.Fatal("unconfigured icon must not reference an app icon set")
	}
	p = iosProject(&Config{Icon: "icon.png"}, "15.0", "TEAMID")
	for _, want := range []string{"ASSETCATALOG_COMPILER_APPICON_NAME = AppIcon", "path = Assets.xcassets", "folder.assetcatalog", "fileRef = A00000000000000000000020"} {
		if !strings.Contains(p, want) {
			t.Fatalf("configured icon missing %q", want)
		}
	}
}

func TestIOSIconAssets(t *testing.T) {
	root, host := t.TempDir(), t.TempDir()
	c := &Config{root: root, Icon: "icon.png"}
	src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	src.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 128})
	src.SetNRGBA(1, 1, color.NRGBA{0, 0, 255, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, src); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.path(c.Icon), b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := iosIconAssets(c, host); err != nil {
		t.Fatal(err)
	}
	set := filepath.Join(host, "Assets.xcassets", "AppIcon.appiconset")
	data, err := os.ReadFile(filepath.Join(set, "Contents.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Images []struct{ Filename, Idiom, Platform, Size string }
	}
	if err := json.Unmarshal(data, &catalog); err != nil || len(catalog.Images) != 1 {
		t.Fatalf("catalog: %s, %v", data, err)
	}
	entry := catalog.Images[0]
	if entry.Idiom != "universal" || entry.Platform != "ios" || entry.Size != "1024x1024" {
		t.Fatalf("bad app icon descriptor: %+v", entry)
	}
	data, err = os.ReadFile(filepath.Join(set, entry.Filename))
	if err != nil {
		t.Fatal(err)
	}
	icon, err := png.Decode(bytes.NewReader(data))
	if err != nil || icon.Bounds() != image.Rect(0, 0, 1024, 1024) {
		t.Fatalf("icon size/decode: %v", err)
	}
	for _, p := range []image.Point{{0, 0}, {512, 512}, {1023, 1023}} {
		_, _, _, alpha := icon.At(p.X, p.Y).RGBA()
		if alpha != 65535 {
			t.Fatal("iOS default app icon has transparency")
		}
	}
	if got := color.NRGBAModel.Convert(icon.At(0, 0)).(color.NRGBA); got != (color.NRGBA{255, 127, 127, 255}) {
		t.Fatalf("transparent pixels not composited over white: %v", got)
	}
	if err := os.WriteFile(c.path(c.Icon), []byte("bad PNG"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := iosIconAssets(c, t.TempDir()); err == nil {
		t.Fatal("invalid PNG accepted")
	}
	blank := t.TempDir()
	if err := iosIconAssets(&Config{}, blank); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(blank, "Assets.xcassets")); !os.IsNotExist(err) {
		t.Fatal("unconfigured icon generated a catalog")
	}
}

func TestIOSLaunchAssets(t *testing.T) {
	host := t.TempDir()
	compile := func(dir string) {
		t.Helper()
		if runtime.GOOS != "darwin" || os.Getenv("MYGO_E2E") != "1" {
			return
		}
		out, err := exec.Command("xcrun", "ibtool", "--compile", filepath.Join(dir, "LaunchScreen.storyboardc"), "--minimum-deployment-target", "15.0", filepath.Join(dir, "LaunchScreen.storyboard")).CombinedOutput()
		if err != nil {
			t.Fatalf("compiling launch screen: %v\n%s", err, out)
		}
	}
	c := &Config{Name: `Go & "UI"`, IOS: IOS{LaunchScreen: IOSLaunchScreen{BackgroundColor: "#123456", ForegroundColor: "#abcdef", FadeDurationMs: 250}}}
	if err := iosLaunchAssets(c, host); err != nil {
		t.Fatal(err)
	}
	compile(host)
	data, err := os.ReadFile(filepath.Join(host, "LaunchScreen.storyboard"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct{ XMLName xml.Name }
	if err := xml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Go &amp; &#34;UI&#34;", `toolsVersion=`, `launchScreen="YES"`, `red="0.070588"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("launch storyboard missing %q", want)
		}
	}
	logo, err := os.Open(filepath.Join(host, "LaunchLogo.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer logo.Close()
	if _, err := png.DecodeConfig(logo); err != nil {
		t.Fatal(err)
	}
	c.IOS.LaunchScreen.BackgroundColor = "bad"
	if err := iosLaunchAssets(c, host); err == nil {
		t.Fatal("invalid color accepted")
	}
	c.IOS.LaunchScreen.BackgroundColor = ""
	c.IOS.LaunchScreen.FadeDurationMs = -1
	if err := iosLaunchAssets(c, host); err == nil {
		t.Fatal("negative fade accepted")
	}
	c.IOS.LaunchScreen.FadeDurationMs = 5001
	if err := iosLaunchAssets(c, host); err == nil {
		t.Fatal("excessive fade accepted")
	}
	c.IOS.LaunchScreen.FadeDurationMs = 0
	c.IOS.LaunchScreen.Image = filepath.Join(host, "LaunchLogo.png")
	withLogo := t.TempDir()
	if err := iosLaunchAssets(c, withLogo); err != nil {
		t.Fatal(err)
	}
	compile(withLogo)
	if !strings.Contains(iosLaunchStoryboard(c), `id="launch-logo"`) {
		t.Fatal("configured logo is missing")
	}
	bad := filepath.Join(host, "bad.png")
	if err := os.WriteFile(bad, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	c.IOS.LaunchScreen.Image = bad
	if err := iosLaunchAssets(c, host); err == nil {
		t.Fatal("non-PNG launch image accepted")
	}
}

func TestIOSSigningDocumentsAndExport(t *testing.T) {
	c := &Config{Name: "Native", Identifier: "com.example.native", Version: "1.0.0", IOS: IOS{BuildNumber: "7", AssociatedDomains: []string{"applinks:example.com"}, Entitlements: map[string]any{"aps-environment": "development"}}, FileAssociations: []FileAssociation{{Ext: []string{"txt", "mygo-note"}, Name: "Text", MimeType: "text/plain", Role: "Viewer"}}}
	if err := validateIOSPackaging(c, buildOptions{iosArchive: true}); err != nil {
		t.Fatal(err)
	}
	host := iosProject(c, "15.0", "TEAM")
	for _, want := range []string{"name = Release", "CODE_SIGN_ENTITLEMENTS = MyGo.entitlements", "SKIP_INSTALL = NO"} {
		if !strings.Contains(host, want) {
			t.Fatalf("missing archive setting %q", want)
		}
	}
	entitlements := string(iosEntitlements(c))
	if !strings.Contains(entitlements, "<array>") || !strings.Contains(entitlements, "<string>applinks:example.com</string>") {
		t.Fatalf("invalid entitlement array: %s", entitlements)
	}
	plist := string(iosInfoPlist(c, "15.0"))
	for _, want := range []string{"CFBundleDocumentTypes", "UTImportedTypeDeclarations", "LSItemContentTypes", "public.filename-extension", "text/plain", "<string>7</string>"} {
		if !strings.Contains(plist, want) {
			t.Fatalf("missing bundle field %q", want)
		}
	}
	for _, o := range []buildOptions{{iosArchive: true, iosSimulator: true}, {iosArchive: true, debug: true}, {iosArchive: true, iosDevice: "UDID"}, {iosExportMethod: "invalid", iosTeam: "TEAM"}, {iosExportMethod: "app-store-connect"}} {
		if validateIOSPackaging(c, o) == nil {
			t.Fatalf("invalid archive accepted %+v", o)
		}
	}
	for _, method := range []string{"debugging", "release-testing", "app-store-connect", "enterprise"} {
		if err := validateIOSPackaging(c, buildOptions{iosExportMethod: method, iosTeam: "TEAM"}); err != nil {
			t.Fatal(err)
		}
		options := string(iosExportOptions(c, method, "TEAM"))
		if !strings.Contains(options, "<key>destination</key>\n\t<string>export</string>") || strings.Contains(options, "<string>upload</string>") {
			t.Fatalf("export could upload: %s", options)
		}
	}
	c.IOS.BuildNumber = "1-beta"
	if validateIOSPackaging(c, buildOptions{iosArchive: true}) == nil {
		t.Fatal("invalid build number")
	}
	c.IOS.BuildNumber = "8"
	c.IOS.AssociatedDomains = []string{"https://example.com"}
	if validateIOSPackaging(c, buildOptions{}) == nil {
		t.Fatal("invalid domain")
	}
}
