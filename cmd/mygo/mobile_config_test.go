package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func solidPNG(t *testing.T, size int, c color.Color) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func writeBytes(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMobileConfigValidate(t *testing.T) {
	for config, want := range map[string]string{
		`{"ios": {"associatedDomains": ["https://example.com"]}}`:                 "associatedDomains",
		`{"ios": {"launchScreen": {}}}`:                                           "launchScreen",
		`{"ios": {"launchScreen": {"background": "red"}}}`:                        "launchScreen.background",
		`{"ios": {"backgroundModes": ["not one"]}}`:                               "backgroundModes",
		`{"android": {"appLinks": ["https://example.com"]}}`:                      "appLinks",
		`{"android": {"adaptiveIcon": {"background": "#000"}}}`:                   "foreground",
		`{"android": {"adaptiveIcon": {"foreground": "a.png"}}}`:                  "background",
		`{"android": {"adaptiveIcon": {"foreground": "a", "background": "#12"}}}`: "background",
		`{"android": {"splash": "#12345g"}}`:                                      "splash",
	} {
		dir := t.TempDir()
		writeBytes(t, filepath.Join(dir, jsonConfig), []byte(config))
		if _, err := loadConfig(dir); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", config, err)
		}
	}
	dir := t.TempDir()
	writeBytes(t, filepath.Join(dir, jsonConfig), []byte(`{
		"ios": {"associatedDomains": ["example.com", "webcredentials:example.com", "applinks:*.example.com?mode=developer"],
			"launchScreen": {"background": "#102030"}, "backgroundModes": ["audio"]},
		"android": {"appLinks": ["example.com", "*.example.org"], "splash": "#fff",
			"adaptiveIcon": {"foreground": "fg.png", "background": "#102030"}}
	}`))
	if _, err := loadConfig(dir); err != nil {
		t.Error(err)
	}
	if got, err := parseHexColor("#1a2"); err != nil || got != [3]uint8{0x11, 0xaa, 0x22} {
		t.Errorf("#1a2: %v, %v", got, err)
	}
}

func TestAndroidManifestOfConfig(t *testing.T) {
	c := &Config{Android: Android{AppLinks: []string{"example.com", "*.example.org"}}, URLSchemes: []string{"myapp"}}
	m := string(androidManifest(c, nil))
	if n := strings.Count(m, `android:autoVerify="true"`); n != 2 {
		t.Errorf("%d verified filters:\n%s", n, m)
	}
	for _, want := range []string{`<data android:scheme="https" android:host="example.com" />`, `android:host="*.example.org"`,
		`<data android:scheme="myapp" />`, `android:theme="@style/Theme.MyGo"`} {
		if !strings.Contains(m, want) {
			t.Errorf("no %s in:\n%s", want, m)
		}
	}
	// A custom scheme's filter is not verified: it has no host to ask.
	if i := strings.Index(m, `android:scheme="myapp"`); strings.Contains(m[strings.LastIndex(m[:i], "<intent-filter"):i], "autoVerify") {
		t.Errorf("the scheme's filter is verified:\n%s", m)
	}
	c.Android.Splash = "#102030"
	if m := string(androidManifest(c, nil)); !strings.Contains(m, `android:theme="@style/`+androidSplashTheme+`"`) {
		t.Errorf("no splash theme:\n%s", m)
	}
}

func TestAndroidAdaptiveIcon(t *testing.T) {
	dir := t.TempDir()
	writeBytes(t, filepath.Join(dir, "fg.png"), solidPNG(t, 432, color.NRGBA{255, 0, 0, 255}))
	c := &Config{root: dir, Android: Android{AdaptiveIcon: &AndroidAdaptiveIcon{Foreground: "fg.png", Background: "#102030"}}}
	res := filepath.Join(dir, "res")
	has, err := androidIcons(c, res)
	if err != nil || !has {
		t.Fatalf("icons: %v, %v", has, err)
	}
	xml, err := os.ReadFile(filepath.Join(res, "mipmap-anydpi-v26", "ic_launcher.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(xml), "@mipmap/ic_launcher_foreground") || !strings.Contains(string(xml), "@color/ic_launcher_background") {
		t.Errorf("adaptive icon: %s", xml)
	}
	values, err := os.ReadFile(filepath.Join(res, "values", "mygo_icon.xml"))
	if err != nil || !strings.Contains(string(values), `<color name="ic_launcher_background">#102030</color>`) {
		t.Errorf("color: %s, %v", values, err)
	}
	for density, size := range map[string]int{"mdpi": 108, "hdpi": 162, "xhdpi": 216, "xxhdpi": 324, "xxxhdpi": 432} {
		data, err := os.ReadFile(filepath.Join(res, "mipmap-"+density, "ic_launcher_foreground.png"))
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width != size {
			t.Errorf("%s: %dx%d, %v", density, cfg.Width, cfg.Height, err)
		}
		// Older systems' icon, from the same layers without an icon.
		if _, err := os.Stat(filepath.Join(res, "mipmap-"+density, "ic_launcher.png")); err != nil {
			t.Error(err)
		}
	}

	// The two layers as one: the foreground's middle over the background.
	half := image.NewNRGBA(image.Rect(0, 0, 108, 108))
	half.Set(54, 54, color.NRGBA{255, 0, 0, 255})
	flat, err := flattenAdaptiveIcon(half, "#102030")
	if err != nil {
		t.Fatal(err)
	}
	if b := flat.Bounds(); b.Dx() != 72 || b.Dy() != 72 {
		t.Errorf("flattened to %v", b)
	}
	if got := color.NRGBAModel.Convert(flat.At(0, 0)); got != (color.NRGBA{0x10, 0x20, 0x30, 255}) {
		t.Errorf("background %v", got)
	}
	if got := color.NRGBAModel.Convert(flat.At(36, 36)); got != (color.NRGBA{255, 0, 0, 255}) {
		t.Errorf("foreground %v", got)
	}

	// Not square.
	img := image.NewNRGBA(image.Rect(0, 0, 10, 20))
	var b bytes.Buffer
	png.Encode(&b, img)
	writeBytes(t, filepath.Join(dir, "fg.png"), b.Bytes())
	if _, err := androidIcons(c, res); err == nil || !strings.Contains(err.Error(), "square") {
		t.Errorf("not square: %v", err)
	}
}

func TestAndroidIconsWithoutOne(t *testing.T) {
	if has, err := androidIcons(&Config{root: t.TempDir()}, t.TempDir()); has || err != nil {
		t.Errorf("%v, %v", has, err)
	}
}

// The style has its own name: the host's source set has Theme.MyGo, which
// another of the same name there would clash with.
func TestAndroidSplash(t *testing.T) {
	res := t.TempDir()
	if err := writeAndroidSplash(res, "#102030"); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(res, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if s := read("values-v31/mygo_splash.xml"); !strings.Contains(s, `android:windowSplashScreenBackground">@color/mygo_splash`) || !strings.Contains(s, `parent="Theme.MyGo"`) {
		t.Errorf("v31: %s", s)
	}
	if s := read("values/mygo_splash.xml"); strings.Contains(s, "windowSplashScreenBackground") || !strings.Contains(s, "windowBackground") {
		t.Errorf("before Android 12, that attribute is unknown: %s", s)
	}
	if s := read("values/mygo_splash_color.xml"); !strings.Contains(s, `<color name="mygo_splash">#102030</color>`) {
		t.Errorf("color: %s", s)
	}
}

func TestLaunchScreen(t *testing.T) {
	dir := t.TempDir()
	writeBytes(t, filepath.Join(dir, "logo.png"), solidPNG(t, 30, color.White))
	c := &Config{root: dir}
	catalog := filepath.Join(dir, "Assets.xcassets")
	l := &IOSLaunchScreen{Background: "#ff0080", Image: "logo.png"}
	if err := writeLaunchAssets(c, l, catalog); err != nil {
		t.Fatal(err)
	}
	info := launchScreenInfo(l)
	if info["UIColorName"] != launchColorAsset || info["UIImageName"] != launchImageAsset {
		t.Errorf("info: %v", info)
	}
	data, err := os.ReadFile(filepath.Join(catalog, launchColorAsset+".colorset", "Contents.json"))
	if err != nil {
		t.Fatal(err)
	}
	var set struct {
		Colors []struct {
			Idiom string
			Color struct {
				ColorSpace string `json:"color-space"`
				Components map[string]string
			}
		}
	}
	if err := json.Unmarshal(data, &set); err != nil {
		t.Fatal(err)
	}
	comp := set.Colors[0].Color.Components
	if set.Colors[0].Color.ColorSpace != "srgb" || comp["red"] != "1.000" || comp["green"] != "0.000" || comp["blue"] != "0.502" || comp["alpha"] != "1.000" {
		t.Errorf("color: %s", data)
	}
	if _, err := os.Stat(filepath.Join(catalog, launchImageAsset+".imageset", "launch.png")); err != nil {
		t.Error(err)
	}
	// One or the other.
	if info := launchScreenInfo(&IOSLaunchScreen{Background: "#000"}); len(info) != 1 || info["UIColorName"] == nil {
		t.Errorf("only a color: %v", info)
	}
	writeBytes(t, filepath.Join(dir, "not.png"), []byte("not a PNG"))
	if err := writeLaunchAssets(c, &IOSLaunchScreen{Image: "not.png"}, catalog); err == nil || !strings.Contains(err.Error(), "PNG") {
		t.Errorf("not a PNG: %v", err)
	}
}

func TestAddBackgroundModes(t *testing.T) {
	info := map[string]any{"UIBackgroundModes": []any{"audio", "fetch"}}
	addBackgroundModes(info, []string{"fetch", "remote-notification"})
	got := info["UIBackgroundModes"].([]any)
	if len(got) != 3 || got[2] != "remote-notification" {
		t.Errorf("merged: %v", got)
	}
	empty := map[string]any{}
	addBackgroundModes(empty, nil)
	if _, ok := empty["UIBackgroundModes"]; ok {
		t.Error("no modes wrote the key")
	}
	// plistBytes takes what it makes.
	if !bytes.Contains(plistBytes(info), []byte("<string>remote-notification</string>")) {
		t.Error("plist")
	}
}

func TestChooseProfile(t *testing.T) {
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	later, soon := now.AddDate(1, 0, 0), now.AddDate(0, 1, 0)
	ids := map[string]string{"CERT": "Apple Development: Jane"}
	make1 := func(name, kind, appID string, expires time.Time, certs []string, devices ...string) *profile {
		return &profile{name: name, uuid: "uuid-" + name, kind: kind, appID: appID, team: "TEAM", expires: expires, certs: certs, devices: devices}
	}
	cert := []string{"CERT"}
	store := make1("store", "app-store", "TEAM.com.example.app.dev", later, cert)
	wild := make1("wild", "development", "TEAM.*", later, cert, "PHONE")
	exact := make1("exact", "development", "TEAM.com.example.app.dev", later, cert, "PHONE")
	expired := make1("expired", "development", "TEAM.com.example.app.dev", now.AddDate(0, 0, -1), cert, "PHONE")
	other := make1("other device", "development", "TEAM.com.example.app.dev", later, cert, "ANOTHER")
	noCert := make1("no certificate", "development", "TEAM.com.example.app.dev", later, []string{"ELSEWHERE"}, "PHONE")
	otherTeam := make1("other team", "development", "OTHER.com.example.app.dev", later, cert, "PHONE")
	want := profileWant{team: "TEAM", identifier: "com.example.app.dev", dist: "development", device: "PHONE", now: now}

	// The closest fit of the kind asked for, not a store profile for the
	// very identifier.
	if p, _ := chooseProfile([]*profile{store, wild, exact}, ids, want); p != exact {
		t.Errorf("chose %+v", p)
	}
	if p, _ := chooseProfile([]*profile{store, wild}, ids, want); p != wild {
		t.Errorf("wildcard: chose %+v", p)
	}
	if p, _ := chooseProfile([]*profile{wild}, map[string]string{"CERT": "x"}, profileWant{identifier: "com.example.app.dev", dist: "development", device: "NOT LISTED", now: now}); p != nil {
		t.Errorf("a profile without the device: %+v", p)
	}
	// Whatever else there is, it says why it would not do.
	p, near := chooseProfile([]*profile{store, expired, other, noCert, otherTeam}, ids, want)
	if p != nil {
		t.Errorf("chose %+v", p)
	}
	joined := strings.Join(near, "; ")
	for _, s := range []string{`"store" (app-store)`, "expired 2026-10-31", "does not list this device", "not in the keychain"} {
		if !strings.Contains(joined, s) {
			t.Errorf("no %q in %s", s, joined)
		}
	}
	if strings.Contains(joined, "other team") {
		t.Errorf("another team's is not close: %s", joined)
	}
	// Named, its kind does not matter.
	named := want
	named.name, named.device, named.dist = "store", "", "app-store"
	if p, _ := chooseProfile([]*profile{wild, store}, ids, named); p != store {
		t.Errorf("named: %+v", p)
	}
	named.name, named.dist = "uuid-exact", "app-store"
	if p, _ := chooseProfile([]*profile{wild, exact}, ids, named); p != exact {
		t.Errorf("by UUID: %+v", p)
	}
	// As close, the latest to expire.
	a := make1("a", "development", "TEAM.*", soon, cert, "PHONE")
	b := make1("b", "development", "TEAM.*", later, cert, "PHONE")
	if p, _ := chooseProfile([]*profile{a, b}, ids, want); p != b {
		t.Errorf("later: %+v", p)
	}
	if p, _ := chooseProfile([]*profile{b, a}, ids, want); p != b {
		t.Errorf("later, in the other order: %+v", p)
	}
	// Enterprise profiles list no devices.
	ent := make1("enterprise", "enterprise", "TEAM.*", later, cert)
	entWant := want
	entWant.dist = "enterprise"
	if p, _ := chooseProfile([]*profile{ent}, ids, entWant); p != ent {
		t.Errorf("enterprise: %+v", p)
	}
}

// A development build does not take the profile named for the release.
func TestNamedProfile(t *testing.T) {
	o := IOS{Distribution: "app-store", ProvisioningProfile: "Store"}
	if got := o.namedProfile("app-store"); got != "Store" {
		t.Errorf("release: %q", got)
	}
	if got := o.namedProfile("development"); got != "" {
		t.Errorf("development: %q", got)
	}
	o.Distribution = "development"
	if got := o.namedProfile("development"); got != "Store" {
		t.Errorf("a development app's: %q", got)
	}
}

func TestSigningEntitlements(t *testing.T) {
	p := &profile{name: "dev", team: "TEAM", entitlements: map[string]any{
		"application-identifier":              "TEAM.*",
		"get-task-allow":                      true,
		"keychain-access-groups":              []any{"TEAM.*"},
		associatedDomainsKey:                  "*",
		"com.apple.developer.team-identifier": "TEAM",
	}}
	c := &Config{Identifier: "com.example.app"}
	ent, err := p.signingEntitlements(c)
	if err != nil {
		t.Fatal(err)
	}
	if ent["application-identifier"] != "TEAM.com.example.app" || ent["get-task-allow"] != true {
		t.Errorf("entitlements: %v", ent)
	}
	// The profile's wildcard is what it allows, not what the app claims.
	if _, ok := ent[associatedDomainsKey]; ok {
		t.Errorf("no domains asked for: %v", ent)
	}
	if p.entitlements["application-identifier"] != "TEAM.*" {
		t.Error("the profile's own changed")
	}
	c.IOS.AssociatedDomains = []string{"example.com", "webcredentials:example.com", "applinks:*.example.org"}
	ent, err = p.signingEntitlements(c)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(ent[associatedDomainsKey])
	if string(got) != `["applinks:example.com","webcredentials:example.com","applinks:*.example.org"]` {
		t.Errorf("domains: %s", got)
	}
	out := string(plistBytes(ent))
	if !strings.Contains(out, "<string>applinks:example.com</string>") || !strings.Contains(out, "<true/>") {
		t.Errorf("plist: %s", out)
	}
	delete(p.entitlements, associatedDomainsKey)
	if p.allowsAssociatedDomains() {
		t.Error("allowsAssociatedDomains")
	}
	if _, err := p.signingEntitlements(c); err == nil || !strings.Contains(err.Error(), "Associated Domains") {
		t.Errorf("without the capability: %v", err)
	}
}
