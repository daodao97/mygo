package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	iosHost "github.com/egoist/mygo/internal/mobile/ios"
)

// The iOS app is the Swift host (internal/mobile/ios), compiled by swiftc
// and linked with the app's Go code built as a c-archive: GOOS=ios needs
// cgo to link, with Xcode's clang, so iOS apps build on a Mac with Xcode.
// No Xcode project: the CLI compiles, bundles, compiles the icon with
// actool and signs, as Xcode would.

// iosToolchain is Xcode's SDK of devices (iphoneos) or of the simulator.
type iosToolchain struct {
	simulator      bool
	sdkName, sdk   string
	clang, target  string
	minimumVersion string
}

func newIOSToolchain(c *Config, simulator bool) (*iosToolchain, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("iOS apps build on a Mac with Xcode")
	}
	t := &iosToolchain{simulator: simulator, sdkName: "iphoneos", minimumVersion: c.IOS.MinimumSystemVersion}
	if simulator {
		t.sdkName = "iphonesimulator"
	}
	var err error
	if t.sdk, err = xcrun("--sdk", t.sdkName, "--show-sdk-path"); err != nil {
		return nil, fmt.Errorf("no iOS SDK (install Xcode, then run xcode-select -s /Applications/Xcode.app): %v", err)
	}
	if t.clang, err = xcrun("--sdk", t.sdkName, "--find", "clang"); err != nil {
		return nil, err
	}
	t.target = "arm64-apple-ios" + t.minimumVersion
	if simulator {
		t.target += "-simulator"
	}
	return t, nil
}

func xcrun(args ...string) (string, error) {
	out, err := exec.Command("xcrun", args...).Output()
	return strings.TrimSpace(string(out)), err
}

// iosApp builds the app bundle for t into dir and returns its path. dc is
// the configuration to bundle (the development app's in mygo dev).
func (t *iosToolchain) buildApp(ctx context.Context, c, dc *Config, tk *task, opts buildOptions, dir string) (string, error) {
	work := filepath.Join(opts.work, "ios-"+t.sdkName)
	if err := os.MkdirAll(work, 0o755); err != nil {
		return "", err
	}
	// The native modules' generated Go, which the app's compiles with, is
	// fresh.
	if _, err := c.phoneModules("ios", opts.debug); err != nil {
		return "", err
	}
	lib := filepath.Join(work, "libapp.a")
	ldflags := "-s -w" + packageFlags(dc)
	if !opts.debug {
		ldflags += " -X github.com/egoist/mygo.production=1"
	}
	ldflags += extraLDFlags()
	flags := []string{"-trimpath", "-buildmode=c-archive", "-ldflags", ldflags}
	flags = append(flags, productionTags(opts.debug)...)
	overlay, err := writeOverlay(filepath.Join(work, "overlay.json"), opts.overlay)
	if err != nil {
		return "", err
	}
	if overlay != "" {
		flags = append(flags, "-overlay", overlay)
	}
	env := []string{"GOOS=ios", "GOARCH=arm64", "CGO_ENABLED=1", "CC=" + t.clang + " -target " + t.target + " -isysroot " + t.sdk}
	tk.set("compiling Go")
	if err := goBuild(ctx, c, tk, lib, env, flags...); err != nil {
		return "", err
	}

	tk.set("compiling the host")
	host := filepath.Join(work, "host")
	os.RemoveAll(host)
	if err := os.CopyFS(host, iosHost.Files); err != nil {
		return "", err
	}
	plugins, err := c.mobilePlugins("ios")
	if err != nil {
		return "", err
	}
	if err := plugins.writeSwift(filepath.Join(host, "Sources", "MyGoHost")); err != nil {
		return "", err
	}
	swift, _ := filepath.Glob(filepath.Join(host, "Sources", "MyGoHost", "*.swift"))
	bundle := filepath.Join(dir, dc.Name+".app")
	os.RemoveAll(bundle)
	if err := os.MkdirAll(bundle, 0o755); err != nil {
		return "", err
	}
	exe := dc.executableName()
	// The host compiles apart from what links it, and the development build
	// keeps what it compiled while its sources stay as they are: a change of
	// the app's Go code only links.
	var frameworks []string
	for _, f := range []string{"UIKit", "WebKit", "Metal", "QuartzCore", "CoreText", "CoreGraphics", "Security", "CoreFoundation",
		"UserNotifications", "UniformTypeIdentifiers", "Network", "AVFoundation"} {
		frameworks = append(frameworks, "-framework", f)
	}
	for _, f := range plugins.frameworks() {
		frameworks = append(frameworks, "-framework", f)
	}
	hostObject := filepath.Join(work, "host.o")
	if !opts.debug || !swiftCompiled(host, t.target, hostObject) {
		args := []string{"--sdk", t.sdkName, "swiftc", "-parse-as-library", "-module-name", "MyGoHost", "-target", t.target, "-sdk", t.sdk,
			"-import-objc-header", filepath.Join(host, "MyGo.h"), "-wmo", "-c", "-o", hostObject}
		if !opts.debug {
			args = append(args, "-O")
		}
		args = append(args, swift...)
		if out, err := exec.CommandContext(ctx, "xcrun", args...).CombinedOutput(); err != nil {
			return "", fmt.Errorf("swiftc failed: %v\n%s", err, bytes.TrimSpace(out))
		}
		noteSwiftCompiled(host, t.target, hostObject)
	}
	tk.set("linking")
	link := []string{"--sdk", t.sdkName, "swiftc", "-target", t.target, "-sdk", t.sdk, hostObject, lib, "-o", filepath.Join(bundle, exe), "-Xlinker", "-dead_strip", "-lresolv"}
	link = append(link, frameworks...)
	if t.simulator {
		flags, err := plugins.simulatorEntitlements(work)
		if err != nil {
			return "", err
		}
		link = append(link, flags...)
	}
	if out, err := exec.CommandContext(ctx, "xcrun", link...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("swiftc failed: %v\n%s", err, bytes.TrimSpace(out))
	}

	if err := t.iosModules(ctx, c, tk, work, bundle, opts.debug); err != nil {
		return "", err
	}

	tk.set("bundling")
	res, err := dc.appResources("ios", "arm64")
	if err != nil {
		return "", err
	}
	if err := copyResources(res, bundle); err != nil {
		return "", err
	}
	manifest, err := privacyManifest(c, filepath.Join(bundle, exe))
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(bundle, "PrivacyInfo.xcprivacy"), manifest, 0o644); err != nil {
		return "", err
	}
	info, err := t.infoPlist(dc, exe)
	if err != nil {
		return "", err
	}
	if dc.Icon != "" || dc.IOS.LaunchScreen != nil {
		assets, err := t.compileAssets(dc, work, bundle)
		if err != nil {
			return "", err
		}
		for k, v := range assets {
			info[k] = v
		}
	}
	if l := dc.IOS.LaunchScreen; l != nil {
		info["UILaunchScreen"] = launchScreenInfo(l)
	}
	if opts.debug && !t.simulator {
		// A device reaches the dev server over the network, unlike the
		// simulator, which shares the Mac's.
		info["NSAppTransportSecurity"] = map[string]any{"NSAllowsLocalNetworking": true}
		info["NSLocalNetworkUsageDescription"] = "Reaches the development server on this network."
	}
	for k, v := range plugins.infoPlist(dc.Identifier) {
		info[k] = v
	}
	addBackgroundModes(info, dc.IOS.BackgroundModes)
	if dc.IOS.MultipleWindows {
		info["NSUserActivityTypes"] = []any{"dev.mygo.window"} // Scenes, in the host
	}
	for k, v := range dc.IOS.InfoPlist {
		info[k] = v // the app's own keys win
	}
	if err := os.WriteFile(filepath.Join(bundle, "Info.plist"), plistBytes(info), 0o644); err != nil {
		return "", err
	}
	return bundle, nil
}

// infoPlist is the app's Info.plist, with the keys of the build that App
// Store validation checks.
func (t *iosToolchain) infoPlist(c *Config, exe string) (map[string]any, error) {
	sdkVersion, err := xcrun("--sdk", t.sdkName, "--show-sdk-version")
	if err != nil {
		return nil, err
	}
	sdkBuild, _ := xcrun("--sdk", t.sdkName, "--show-sdk-build-version")
	xcode, xcodeBuild := xcodeVersion()
	platform := "iPhoneOS"
	if t.simulator {
		platform = "iPhoneSimulator"
	}
	var orient []any
	allows := func(o string) bool { return len(c.IOS.Orientations) == 0 || stringsContain(c.IOS.Orientations, o) }
	if allows("portrait") {
		orient = append(orient, "UIInterfaceOrientationPortrait")
	}
	if allows("landscape") {
		orient = append(orient, "UIInterfaceOrientationLandscapeLeft", "UIInterfaceOrientationLandscapeRight")
	}
	m := map[string]any{
		"CFBundleIdentifier":                       c.Identifier,
		"CFBundleExecutable":                       exe,
		"CFBundleName":                             c.Name,
		"CFBundleDisplayName":                      c.Name,
		"CFBundlePackageType":                      "APPL",
		"CFBundleShortVersionString":               c.Version,
		"CFBundleVersion":                          c.IOS.buildNumber(c.Version),
		"CFBundleInfoDictionaryVersion":            "6.0",
		"CFBundleDevelopmentRegion":                "en",
		"CFBundleSupportedPlatforms":               []any{platform},
		"MinimumOSVersion":                         t.minimumVersion,
		"LSRequiresIPhoneOS":                       true,
		"UIDeviceFamily":                           []any{1.0, 2.0},
		"UIRequiredDeviceCapabilities":             []any{"arm64"},
		"UILaunchScreen":                           map[string]any{},
		"CADisableMinimumFrameDurationOnPhone":     true, // 120 Hz on ProMotion iPhones
		"UIApplicationSupportsIndirectInputEvents": true,
		"UIApplicationSceneManifest":               map[string]any{"UIApplicationSupportsMultipleScenes": c.IOS.MultipleWindows},
		"UISupportedInterfaceOrientations":         orient,
		"UISupportedInterfaceOrientations~ipad": []any{"UIInterfaceOrientationPortrait", "UIInterfaceOrientationPortraitUpsideDown",
			"UIInterfaceOrientationLandscapeLeft", "UIInterfaceOrientationLandscapeRight"},
		"ITSAppUsesNonExemptEncryption": false,
		"DTPlatformName":                t.sdkName,
		"DTPlatformVersion":             sdkVersion,
		"DTSDKName":                     t.sdkName + sdkVersion,
		"DTSDKBuild":                    sdkBuild,
		"DTPlatformBuild":               sdkBuild,
		"DTXcode":                       xcode,
		"DTXcodeBuild":                  xcodeBuild,
		"DTCompiler":                    "com.apple.compilers.llvm.clang.1_0",
	}
	if len(c.URLSchemes) > 0 {
		var schemes []any
		for _, s := range c.URLSchemes {
			schemes = append(schemes, s)
		}
		m["CFBundleURLTypes"] = []any{map[string]any{"CFBundleURLName": c.Identifier, "CFBundleURLSchemes": schemes}}
	}
	if c.Copyright != "" {
		m["NSHumanReadableCopyright"] = c.Copyright
	}
	return m, nil
}

func stringsContain(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// buildNumber is CFBundleVersion: the build number set, else the version.
func (o IOS) buildNumber(version string) string {
	if o.BuildNumber != "" {
		return o.BuildNumber
	}
	return version
}

// xcodeVersion returns DTXcode (26.1 → "2610") and the build.
func xcodeVersion() (string, string) {
	out, err := exec.Command("xcodebuild", "-version").Output()
	if err != nil {
		return "", ""
	}
	var ver, build string
	for _, l := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(l, "Xcode "); ok {
			ver = v
		}
		if b, ok := strings.CutPrefix(l, "Build version "); ok {
			build = b
		}
	}
	p := strings.Split(ver, ".")
	for len(p) < 3 {
		p = append(p, "0")
	}
	major, _ := strconv.Atoi(p[0])
	return fmt.Sprintf("%02d%s%s", major, p[1][:1], p[2][:1]), build
}

// compileAssets compiles the app's icon and the launch screen's color and
// image into an asset catalog in the bundle, and returns the keys of
// Info.plist that name them.
func (t *iosToolchain) compileAssets(c *Config, work, bundle string) (map[string]any, error) {
	catalog := filepath.Join(work, "Assets.xcassets")
	os.RemoveAll(catalog)
	if err := os.MkdirAll(catalog, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(catalog, "Contents.json"), []byte(assetContents), 0o644); err != nil {
		return nil, err
	}
	partial := filepath.Join(work, "assets-info.plist")
	args := []string{"actool", "--compile", bundle, "--platform", t.sdkName,
		"--minimum-deployment-target", t.minimumVersion, "--target-device", "iphone", "--target-device", "ipad",
		"--output-partial-info-plist", partial, "--output-format", "human-readable-text"}
	if c.Icon != "" {
		if err := writeIconSet(c, catalog); err != nil {
			return nil, err
		}
		args = append(args, "--app-icon", "AppIcon")
	}
	if l := c.IOS.LaunchScreen; l != nil {
		if err := writeLaunchAssets(c, l, catalog); err != nil {
			return nil, err
		}
	}
	out, err := exec.Command("xcrun", append(args, catalog)...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("actool could not compile the app's assets: %v\n%s", err, bytes.TrimSpace(out))
	}
	js, err := exec.Command("plutil", "-convert", "json", "-o", "-", partial).Output()
	if err != nil {
		return nil, err
	}
	var keys map[string]any
	if err := json.Unmarshal(js, &keys); err != nil {
		return nil, err
	}
	return keys, nil
}

// assetContents is the Contents.json of what holds no assets of its own.
const assetContents = `{"info":{"author":"mygo","version":1}}`

// writeIconSet writes the app's icon into the asset catalog.
func writeIconSet(c *Config, catalog string) error {
	src, err := os.ReadFile(c.path(c.Icon))
	if err != nil {
		return err
	}
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return fmt.Errorf("%s: %v", c.Icon, err)
	}
	var b bytes.Buffer
	// The App Store takes no transparency in an iOS icon.
	if err := png.Encode(&b, opaqueImage(resize(img, 1024))); err != nil {
		return err
	}
	contents := `{"images":[{"filename":"icon.png","idiom":"universal","platform":"ios","size":"1024x1024"}],"info":{"author":"mygo","version":1}}`
	return writeAssetSet(filepath.Join(catalog, "AppIcon.appiconset"), contents, "icon.png", b.Bytes())
}

// The names in the asset catalog of what the launch screen shows.
const (
	launchColorAsset = "LaunchBackground"
	launchImageAsset = "LaunchImage"
)

// launchScreenInfo is the UILaunchScreen of Info.plist, whose values name
// assets: it takes no color or file of its own.
func launchScreenInfo(l *IOSLaunchScreen) map[string]any {
	m := map[string]any{}
	if l.Background != "" {
		m["UIColorName"] = launchColorAsset
	}
	if l.Image != "" {
		m["UIImageName"] = launchImageAsset
	}
	return m
}

// writeLaunchAssets writes the launch screen's color and image into the
// asset catalog.
func writeLaunchAssets(c *Config, l *IOSLaunchScreen, catalog string) error {
	if l.Background != "" {
		col, err := parseHexColor(l.Background)
		if err != nil {
			return err
		}
		contents, err := json.Marshal(map[string]any{
			"colors": []any{map[string]any{"idiom": "universal", "color": map[string]any{
				"color-space": "srgb",
				"components": map[string]string{
					"red":   fmt.Sprintf("%.3f", float64(col[0])/255),
					"green": fmt.Sprintf("%.3f", float64(col[1])/255),
					"blue":  fmt.Sprintf("%.3f", float64(col[2])/255),
					"alpha": "1.000",
				},
			}}},
			"info": map[string]any{"author": "mygo", "version": 1},
		})
		if err != nil {
			return err
		}
		if err := writeAssetSet(filepath.Join(catalog, launchColorAsset+".colorset"), string(contents), "", nil); err != nil {
			return err
		}
	}
	if l.Image != "" {
		src, err := os.ReadFile(c.path(l.Image))
		if err != nil {
			return err
		}
		if _, err := png.DecodeConfig(bytes.NewReader(src)); err != nil {
			return fmt.Errorf("ios.launchScreen.image %s must be a PNG: %v", l.Image, err)
		}
		// At 3x: the sharpest screens' pixels, which others scale down.
		contents := `{"images":[{"filename":"launch.png","idiom":"universal","scale":"3x"}],"info":{"author":"mygo","version":1}}`
		if err := writeAssetSet(filepath.Join(catalog, launchImageAsset+".imageset"), contents, "launch.png", src); err != nil {
			return err
		}
	}
	return nil
}

// writeAssetSet writes a set of the asset catalog, with its file if it has
// one.
func writeAssetSet(dir, contents, file string, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if file != "" {
		if err := os.WriteFile(filepath.Join(dir, file), data, 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(dir, "Contents.json"), []byte(contents), 0o644)
}

// addBackgroundModes adds modes to the UIBackgroundModes of an Info.plist,
// which the app's plugins may have asked for too.
func addBackgroundModes(info map[string]any, modes []string) {
	if len(modes) == 0 {
		return
	}
	list, _ := info["UIBackgroundModes"].([]any)
	for _, m := range modes {
		if !slices.Contains(list, any(m)) {
			list = append(list, m)
		}
	}
	info["UIBackgroundModes"] = list
}

func plistBytes(m map[string]any) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
`)
	writePlistValue(&b, m, "")
	b.WriteString("</plist>\n")
	return b.Bytes()
}

// privacyAPIs are the APIs Apple asks an app to give a reason for using
// (its privacy manifest, PrivacyInfo.xcprivacy, without which App Store
// Connect turns an app down), each category with MyGo's reason: a C
// function the binary imports, or an Objective-C method it names. Go's
// runtime alone uses two: mach_absolute_time (its clock) and stat.
var privacyAPIs = []struct {
	category, reason string
	symbols          []string
	selectors        []string
}{
	// C617.1: timestamps of files in the app's container.
	{"NSPrivacyAccessedAPICategoryFileTimestamp", "C617.1",
		[]string{"stat", "fstat", "fstatat", "lstat", "getattrlist", "getattrlistbulk", "fgetattrlist", "getattrlistat"},
		[]string{"creationDate", "modificationDate", "fileModificationDate", "contentModificationDateKey", "creationDateKey"}},
	// 35F9.1: the time between events in the app.
	{"NSPrivacyAccessedAPICategorySystemBootTime", "35F9.1", []string{"mach_absolute_time"}, []string{"systemUptime"}},
	// E174.1: whether there is room to write files.
	{"NSPrivacyAccessedAPICategoryDiskSpace", "E174.1",
		[]string{"statfs", "statvfs", "fstatfs", "fstatvfs"}, []string{"volumeAvailableCapacityKey", "systemFreeSize"}},
	// CA92.1: values only the app reads and writes.
	{"NSPrivacyAccessedAPICategoryUserDefaults", "CA92.1", []string{"OBJC_CLASS_$_NSUserDefaults"}, nil},
}

// privacyManifest is the PrivacyInfo.xcprivacy of the executable bin: the
// app's own (ios/PrivacyInfo.xcprivacy in the project), for what it
// collects or tracks, else the reasons for the APIs bin uses, without
// tracking or data collected.
func privacyManifest(c *Config, bin string) ([]byte, error) {
	if own, err := os.ReadFile(c.path(filepath.Join("ios", "PrivacyInfo.xcprivacy"))); err == nil {
		return own, nil
	}
	out, err := exec.Command("xcrun", "nm", "-u", bin).Output()
	if err != nil {
		return nil, err
	}
	imported := map[string]bool{}
	for _, l := range strings.Split(string(out), "\n") {
		s := strings.TrimPrefix(strings.TrimSpace(l), "_")
		if i := strings.IndexByte(s, '$'); i > 0 && !strings.HasPrefix(s, "OBJC_") {
			s = s[:i] // stat$INODE64
		}
		imported[s] = true
	}
	data, err := os.ReadFile(bin)
	if err != nil {
		return nil, err
	}
	accessed := []any{}
	for _, api := range privacyAPIs {
		used := false
		for _, s := range api.symbols {
			used = used || imported[s] || imported[s+"64"]
		}
		for _, s := range api.selectors {
			used = used || bytes.Contains(data, []byte("\x00"+s+"\x00")) || bytes.Contains(data, []byte("\x00"+s+":\x00"))
		}
		if used {
			accessed = append(accessed, map[string]any{
				"NSPrivacyAccessedAPIType":        api.category,
				"NSPrivacyAccessedAPITypeReasons": []any{api.reason},
			})
		}
	}
	return plistBytes(map[string]any{
		"NSPrivacyTracking":           false,
		"NSPrivacyTrackingDomains":    []any{},
		"NSPrivacyCollectedDataTypes": []any{},
		"NSPrivacyAccessedAPITypes":   accessed,
	}), nil
}

// privateSelectors are Objective-C methods of Apple's that are not
// public, which App Review turns apps down for naming. An iOS build must
// name none of them: checkPrivateAPIs looks for them in the executable.
var privateSelectors = regexp.MustCompile(`\x00(_set[A-Z][A-Za-z]*:|_webProcessIdentifier|_isPlayingAudio|_mediaMutedState|_sessionStateData|_setPageMuted:|_hiddenPage[A-Za-z]*|_togglePictureInPicture)\x00`)

// checkPrivateAPIs fails when the executable names a private method.
func checkPrivateAPIs(bin string) error {
	data, err := os.ReadFile(bin)
	if err != nil {
		return err
	}
	if m := privateSelectors.Find(data); m != nil {
		return fmt.Errorf("%s names Apple's private method %s, which App Review turns down", filepath.Base(bin), bytes.Trim(m, "\x00"))
	}
	return nil
}

// buildIOS builds the app for iOS: a signed .ipa for devices (and the App
// Store), or with simulator an .app for the simulator.
func buildIOS(c *Config, opts buildOptions, simulator bool) ([]string, error) {
	t, err := newIOSToolchain(c, simulator)
	if err != nil {
		return nil, err
	}
	var prof *profile
	if !simulator {
		// Before building: no point building for a profile that isn't there.
		if prof, err = pickProfile(c, c.IOS.Distribution, ""); err != nil {
			return nil, err
		}
		if _, err := prof.signingEntitlements(c); err != nil {
			return nil, err
		}
		logf("Signing with %q (%s, team %s) as %s", prof.name, prof.kind, prof.team, prof.identityName)
	}
	name := "ios-arm64"
	if simulator {
		name = "ios-simulator"
	}
	out := c.path(filepath.Join(c.Out, name))
	stage := out + ".staging"
	os.RemoveAll(stage)
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	tk := con.start("Building for iOS" + map[bool]string{true: " (simulator)"}[simulator])
	bundle, err := t.buildApp(context.Background(), c, c, tk, opts, stage)
	if err == nil {
		err = checkPrivateAPIs(filepath.Join(bundle, c.executableName()))
	}
	if err == nil {
		tk.set("signing")
		if simulator {
			err = signForSimulator(bundle)
		} else {
			err = prof.sign(opts.work, bundle, c)
		}
	}
	artifact := bundle
	if err == nil && !simulator {
		artifact = filepath.Join(stage, c.slug()+"-"+c.Version+".ipa")
		err = packIPA(opts.work, bundle, artifact)
		os.RemoveAll(bundle)
	}
	if err := tk.end(err, "Built for iOS"); err != nil {
		return nil, err
	}
	if err := replacePath(stage, out); err != nil {
		return nil, err
	}
	return []string{filepath.Join(out, filepath.Base(artifact))}, nil
}

func runQuiet(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %v\n%s", name, err, bytes.TrimSpace(out))
	}
	return nil
}

func packIPA(work, bundle, ipa string) error {
	stage := filepath.Join(work, "ipa")
	os.RemoveAll(stage)
	payload := filepath.Join(stage, "Payload")
	if err := os.MkdirAll(payload, 0o755); err != nil {
		return err
	}
	if err := runQuiet("ditto", bundle, filepath.Join(payload, filepath.Base(bundle))); err != nil {
		return err
	}
	os.Remove(ipa)
	cmd := exec.Command("ditto", "-c", "-k", "--sequesterRsrc", "--keepParent", "Payload", ipa)
	cmd.Dir = stage
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ditto failed: %v\n%s", err, out)
	}
	return nil
}

// Provisioning profiles.

type profile struct {
	path, name, uuid, team, appID string
	kind                          string // app-store, ad-hoc, development, enterprise
	expires                       time.Time
	certs                         []string // the SHA-1s of the certificates it allows
	identity, identityName        string   // the keychain's identity that signs with it
	devices                       []string // the UDIDs it lists, for development and ad hoc
	entitlements                  map[string]any
}

func profileDirs() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, "Library", "Developer", "Xcode", "UserData", "Provisioning Profiles"),
		filepath.Join(home, "Library", "MobileDevice", "Provisioning Profiles"),
	}
}

// readProfile decodes a .mobileprovision, a signed property list.
func readProfile(path string) (*profile, error) {
	raw, err := exec.Command("security", "cms", "-D", "-i", path).Output()
	if err != nil {
		return nil, fmt.Errorf("%s: cannot decode it", path)
	}
	js, err := plistToJSON(raw)
	if err != nil {
		return nil, err
	}
	var d struct {
		Name, UUID            string
		TeamIdentifier        []string
		ExpirationDate        time.Time
		ProvisionsAllDevices  bool
		ProvisionedDevices    []string
		DeveloperCertificates []string
		Entitlements          map[string]any
	}
	if err := json.Unmarshal(js, &d); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	p := &profile{path: path, name: d.Name, uuid: d.UUID, expires: d.ExpirationDate, devices: d.ProvisionedDevices, entitlements: d.Entitlements}
	if len(d.TeamIdentifier) > 0 {
		p.team = d.TeamIdentifier[0]
	}
	p.appID, _ = d.Entitlements["application-identifier"].(string)
	switch {
	case d.ProvisionsAllDevices:
		p.kind = "enterprise"
	case d.Entitlements["get-task-allow"] == true:
		p.kind = "development"
	case len(d.ProvisionedDevices) > 0:
		p.kind = "ad-hoc"
	default:
		p.kind = "app-store"
	}
	for _, c := range d.DeveloperCertificates {
		der, err := base64.StdEncoding.DecodeString(c)
		if err == nil {
			sum := sha1.Sum(der)
			p.certs = append(p.certs, strings.ToUpper(hex.EncodeToString(sum[:])))
		}
	}
	return p, nil
}

// plistToJSON converts a property list to JSON, with dates and data as
// strings.
func plistToJSON(raw []byte) ([]byte, error) {
	cmd := exec.Command("plutil", "-convert", "json", "-r", "-o", "-", "-")
	cmd.Stdin = bytes.NewReader(raw)
	out, err := cmd.Output()
	if err == nil {
		return out, nil
	}
	// plutil's JSON takes no dates or data: turn them into strings first.
	s := regexp.MustCompile(`<date>([^<]*)</date>`).ReplaceAll(raw, []byte("<string>$1</string>"))
	s = regexp.MustCompile(`(?s)<data>(.*?)</data>`).ReplaceAllFunc(s, func(m []byte) []byte {
		inner := regexp.MustCompile(`\s+`).ReplaceAll(m[6:len(m)-7], nil)
		return append(append([]byte("<string>"), inner...), "</string>"...)
	})
	cmd = exec.Command("plutil", "-convert", "json", "-o", "-", "-")
	cmd.Stdin = bytes.NewReader(s)
	return cmd.Output()
}

// identities are the keychain's code signing identities, by SHA-1.
func identities() map[string]string {
	out, _ := exec.Command("security", "find-identity", "-v", "-p", "codesigning").Output()
	m := map[string]string{}
	re := regexp.MustCompile(`\d+\) ([0-9A-F]{40}) "(.+)"`)
	for _, l := range strings.Split(string(out), "\n") {
		if s := re.FindStringSubmatch(l); s != nil {
			m[s[1]] = s[2]
		}
	}
	return m
}

// matches tells how well the profile's app ID (TEAM.id or TEAM.prefix.*)
// fits id: 0 not at all, more for a closer fit.
func (p *profile) matches(team, id string) int {
	pt, pid, ok := strings.Cut(p.appID, ".")
	if !ok || (team != "" && pt != team) {
		return 0
	}
	if pid == id {
		return 1000
	}
	if prefix, ok := strings.CutSuffix(pid, "*"); ok && strings.HasPrefix(id, prefix) {
		return 1 + len(prefix)
	}
	return 0
}

// profileWant is what a build asks of a provisioning profile.
type profileWant struct {
	team, identifier string
	name             string // a name or UUID the configuration gives, or ""
	dist             string // the kind it must be, unless it is named
	device           string // the UDID it must list, or ""
	now              time.Time
}

// chooseProfile picks the closest fit among profiles: unexpired, for the
// identifier, whose certificate is among the keychain's identities (by
// SHA-1), the latest to expire of those as close. It also returns why the
// others that came close did not fit.
func chooseProfile(all []*profile, ids map[string]string, w profileWant) (*profile, []string) {
	var best *profile
	bestScore := 0
	var near []string
	for _, p := range all {
		if w.name != "" && p.name != w.name && p.uuid != w.name {
			continue
		}
		score := p.matches(w.team, w.identifier)
		if score == 0 {
			continue
		}
		if p.kind != w.dist && w.name == "" {
			near = append(near, fmt.Sprintf("%q (%s)", p.name, p.kind))
			continue
		}
		if p.expires.Before(w.now) {
			near = append(near, fmt.Sprintf("%q (expired %s)", p.name, p.expires.Format("2006-01-02")))
			continue
		}
		if w.device != "" && p.kind != "enterprise" && !slices.Contains(p.devices, w.device) {
			near = append(near, fmt.Sprintf("%q (it does not list this device)", p.name))
			continue
		}
		for _, cert := range p.certs {
			if name, ok := ids[cert]; ok {
				p.identity, p.identityName = cert, name // the SHA-1: names can be ambiguous
				if score > bestScore || score == bestScore && best != nil && p.expires.After(best.expires) {
					best, bestScore = p, score
				}
				break
			}
		}
		if p.identity == "" {
			near = append(near, fmt.Sprintf("%q (its certificate is not in the keychain)", p.name))
		}
	}
	return best, near
}

// pickProfile finds the provisioning profile to sign with for dist: the one
// ios.provisioningProfile names, else the closest fit for the app's
// identifier installed. device is the UDID it must list, if any. A profile
// named for a release does not sign a development build, which asks for
// another kind and identifier.
func pickProfile(c *Config, dist, device string) (*profile, error) {
	want := c.IOS.namedProfile(dist)
	var all []*profile
	if strings.HasSuffix(want, ".mobileprovision") {
		p, err := readProfile(c.path(want))
		if err != nil {
			return nil, err
		}
		all, want = []*profile{p}, ""
	} else {
		for _, d := range profileDirs() {
			files, _ := filepath.Glob(filepath.Join(d, "*.mobileprovision"))
			for _, f := range files {
				if p, err := readProfile(f); err == nil {
					all = append(all, p)
				}
			}
		}
	}
	best, near := chooseProfile(all, identities(), profileWant{team: c.IOS.Team, identifier: c.Identifier, name: want, dist: dist, device: device, now: time.Now()})
	if best != nil {
		return best, nil
	}
	msg := fmt.Sprintf("no %s provisioning profile for %s", dist, c.Identifier)
	if len(near) > 0 {
		sort.Strings(near)
		msg += "; close ones: " + strings.Join(near, ", ")
	}
	return nil, errors.New(msg + ". Make one for the identifier in Xcode (Settings › Accounts) or at developer.apple.com, and set ios.team, or ios.provisioningProfile in " + c.configName())
}

// namedProfile is the profile the configuration names for dist: it names the
// release's, of ios.distribution.
func (o IOS) namedProfile(dist string) string {
	if dist != o.Distribution {
		return ""
	}
	return o.ProvisioningProfile
}

// associatedDomains are the values of the entitlement of ios.associatedDomains.
func (o IOS) associatedDomains() []any {
	var out []any
	for _, d := range o.AssociatedDomains {
		if !strings.Contains(d, ":") {
			d = "applinks:" + d
		}
		out = append(out, d)
	}
	return out
}

// associatedDomainsKey is the entitlement of universal links.
const associatedDomainsKey = "com.apple.developer.associated-domains"

// allowsAssociatedDomains reports whether the profile has the capability,
// which profiles of a wildcard app ID cannot have.
func (p *profile) allowsAssociatedDomains() bool {
	_, ok := p.entitlements[associatedDomainsKey]
	return ok
}

// signingEntitlements are those the app is signed with: the profile's, whose
// wildcards become the app's identifier and domains.
func (p *profile) signingEntitlements(c *Config) (map[string]any, error) {
	ent := make(map[string]any, len(p.entitlements)+1)
	for k, v := range p.entitlements {
		ent[k] = v
	}
	// A wildcard profile's app ID becomes the app's.
	prefix, _, ok := strings.Cut(p.appID, ".")
	if !ok {
		prefix = p.team // older app IDs have a prefix of their own
	}
	ent["application-identifier"] = prefix + "." + c.Identifier
	// A plugin's need shows now, not as the app first asks for it on a phone.
	if plugins, err := c.mobilePlugins("ios"); err == nil {
		for k := range plugins.entitlements() {
			if _, ok := p.entitlements[k]; !ok {
				return nil, fmt.Errorf("a plugin of the app needs the entitlement %s, which %q has not: turn on its capability for %s at developer.apple.com, then make the profile again", k, p.name, c.Identifier)
			}
		}
	}
	delete(ent, associatedDomainsKey)
	if domains := c.IOS.associatedDomains(); len(domains) > 0 {
		if !p.allowsAssociatedDomains() {
			return nil, fmt.Errorf("ios.associatedDomains needs the capability Associated Domains, which %q has not: turn it on for %s at developer.apple.com, then make the profile again", p.name, c.Identifier)
		}
		ent[associatedDomainsKey] = domains
	}
	return ent, nil
}

// sign embeds the profile, signs the bundle with its entitlements and
// verifies the signature.
func (p *profile) sign(work, bundle string, c *Config) error {
	data, err := os.ReadFile(p.path)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(bundle, "embedded.mobileprovision"), data, 0o644); err != nil {
		return err
	}
	entitlements, err := p.signingEntitlements(c)
	if err != nil {
		return err
	}
	ent := filepath.Join(work, "entitlements.plist")
	if err := os.WriteFile(ent, plistBytes(entitlements), 0o644); err != nil {
		return err
	}
	if err := signFrameworks(bundle, p.identity); err != nil {
		return err
	}
	if err := runQuiet("codesign", "--force", "--sign", p.identity, "--entitlements", ent, "--generate-entitlement-der", bundle); err != nil {
		return err
	}
	return runQuiet("codesign", "--verify", "--deep", "--strict", bundle)
}

var _ fs.FS = iosHost.Files

// opaqueImage draws img over white, for icons that take no transparency.
func opaqueImage(img image.Image) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(b)
	draw.Draw(out, b, image.White, image.Point{}, draw.Src)
	draw.Draw(out, b, img, b.Min, draw.Over)
	return out
}

// mobilePlatform returns the build of a platform of phones: ios (or
// ios/arm64) for devices, ios/simulator, android, android/<arch>.
func mobilePlatform(p string) (func(*Config, buildOptions) ([]string, error), bool) {
	switch p {
	case "ios", "ios/arm64":
		return func(c *Config, o buildOptions) ([]string, error) { return buildIOS(c, o, false) }, true
	case "ios/simulator":
		return func(c *Config, o buildOptions) ([]string, error) { return buildIOS(c, o, true) }, true
	}
	if p == "android" || strings.HasPrefix(p, "android/") {
		_, arch, _ := strings.Cut(p, "/")
		return func(c *Config, o buildOptions) ([]string, error) { return buildAndroid(c, o, arch) }, true
	}
	return nil, false
}
