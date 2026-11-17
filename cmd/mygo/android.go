package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"io/fs"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"

	androidHost "github.com/egoist/mygo/internal/mobile/android"
)

// The Android app is the Kotlin host (internal/mobile/android), a Gradle
// project mygo build writes into .mygo/android and builds, with the app's
// Go code built as a shared library for each ABI by the NDK's clang:
// Android loads Go into its process, which needs cgo to link. Gradle
// builds the .apk and the .aab with the SDK's tools.

// androidABIs are the ABIs apps ship for: what Google Play asks of apps
// with native code, and the emulators'.
var androidABIs = []struct{ abi, goarch, triple string }{
	{"arm64-v8a", "arm64", "aarch64-linux-android"},
	{"armeabi-v7a", "arm", "armv7a-linux-androideabi"},
	{"x86_64", "amd64", "x86_64-linux-android"},
}

// androidToolchain is the Android SDK, its NDK and the JDK Gradle runs on.
type androidToolchain struct {
	sdk, ndk, ndkBin, javaHome string
}

func newAndroidToolchain() (*androidToolchain, error) {
	t := &androidToolchain{sdk: findAndroidSDK()}
	if t.sdk == "" {
		return nil, errors.New("no Android SDK: install Android Studio, or set ANDROID_HOME")
	}
	ndk := findAndroidNDK(t.sdk)
	if ndk == "" {
		return nil, errors.New("no Android NDK: install it in Android Studio (SDK Manager › SDK Tools › NDK), or set ANDROID_NDK_HOME")
	}
	hosts, _ := filepath.Glob(filepath.Join(ndk, "toolchains", "llvm", "prebuilt", "*", "bin"))
	if len(hosts) == 0 {
		return nil, fmt.Errorf("the NDK in %s has no clang", ndk)
	}
	t.ndk, t.ndkBin = ndk, hosts[0]
	t.javaHome = findJavaHome()
	return t, nil
}

// findAndroidSDK returns the directory of the Android SDK, in ANDROID_HOME
// or its usual place, or "".
func findAndroidSDK() string {
	home, _ := os.UserHomeDir()
	for _, d := range []string{os.Getenv("ANDROID_HOME"), os.Getenv("ANDROID_SDK_ROOT"),
		filepath.Join(home, "Library", "Android", "sdk"), filepath.Join(home, "Android", "Sdk"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Android", "Sdk")} {
		if d != "" && isDir(filepath.Join(d, "platforms")) {
			return d
		}
	}
	return ""
}

// findAndroidNDK returns the directory of the NDK, in ANDROID_NDK_HOME or
// the newest of the SDK's, or "".
func findAndroidNDK(sdk string) string {
	if ndk := os.Getenv("ANDROID_NDK_HOME"); ndk != "" {
		return ndk
	}
	versions, _ := filepath.Glob(filepath.Join(sdk, "ndk", "*"))
	sort.Slice(versions, func(i, j int) bool { return versionLess(filepath.Base(versions[i]), filepath.Base(versions[j])) })
	if len(versions) == 0 {
		return ""
	}
	return versions[len(versions)-1]
}

// findJavaHome returns the JDK Gradle runs on, in JAVA_HOME or Android
// Studio's, or "": then Gradle looks for java itself.
func findJavaHome() string {
	if h := os.Getenv("JAVA_HOME"); h != "" {
		return h
	}
	home, _ := os.UserHomeDir()
	for _, d := range []string{"/Applications/Android Studio.app/Contents/jbr/Contents/Home",
		filepath.Join(home, "Applications", "Android Studio.app", "Contents", "jbr", "Contents", "Home"),
		"/opt/android-studio/jbr", filepath.Join(os.Getenv("ProgramFiles"), "Android", "Android Studio", "jbr")} {
		if isDir(d) {
			return d
		}
	}
	return ""
}

// sdkTool returns the path of a tool in a directory of the SDK, or its name
// to find on the PATH.
func sdkTool(dir, name string) string {
	if sdk := findAndroidSDK(); sdk != "" {
		p := filepath.Join(sdk, dir, name)
		if runtime.GOOS == "windows" {
			p += ".exe"
		}
		if fileExists(p) {
			return p
		}
	}
	return name
}

// versionLess orders NDK versions, 27.1.12297006 after 27.0.12077973.
func versionLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, _ := strconv.Atoi(pa[i])
		nb, _ := strconv.Atoi(pb[i])
		if na != nb {
			return na < nb
		}
	}
	return len(pa) < len(pb)
}

// buildGo builds the app's Go code for abi into jniLibs/<abi>/libmygoapp.so.
func (t *androidToolchain) buildGo(ctx context.Context, c, dc *Config, tk *task, opts buildOptions, abi, jniLibs string) error {
	var a struct{ abi, goarch, triple string }
	for _, x := range androidABIs {
		if x.abi == abi {
			a = x
		}
	}
	env := []string{"GOOS=android", "GOARCH=" + a.goarch, "CGO_ENABLED=1", "CC=" + t.clang(c, abi),
		// 16 KB pages, which new devices have and Google Play asks for.
		"CGO_LDFLAGS=-Wl,-z,max-page-size=16384"}
	if a.goarch == "arm" {
		env = append(env, "GOARM=7")
	}
	ldflags := "-s -w" + packageFlags(dc)
	if !opts.debug {
		ldflags += " -X github.com/egoist/mygo.production=1"
	}
	ldflags += extraLDFlags()
	flags := []string{"-trimpath", "-buildmode=c-shared", "-ldflags", ldflags}
	flags = append(flags, productionTags(opts.debug)...)
	overlay, err := writeOverlay(filepath.Join(opts.work, "overlay-android.json"), opts.overlay)
	if err != nil {
		return err
	}
	if overlay != "" {
		flags = append(flags, "-overlay", overlay)
	}
	out := filepath.Join(jniLibs, abi, "libmygoapp.so")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	tk.set("compiling Go for " + abi)
	if err := goBuild(ctx, c, tk, out, env, flags...); err != nil {
		return err
	}
	os.Remove(strings.TrimSuffix(out, ".so") + ".h")
	return nil
}

// clang is the NDK's compiler for abi.
func (t *androidToolchain) clang(c *Config, abi string) string {
	triple := ""
	for _, x := range androidABIs {
		if x.abi == abi {
			triple = x.triple
		}
	}
	clang := filepath.Join(t.ndkBin, triple+strconv.Itoa(max(c.Android.MinSdk, 21))+"-clang")
	if runtime.GOOS == "windows" {
		clang += ".cmd"
	}
	return clang
}

// project writes the host's Gradle project into dir, where Gradle keeps
// its outputs between builds, and leaves files that did not change as
// they are.
func (t *androidToolchain) project(dir string) error {
	return fs.WalkDir(androidHost.Files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == "." {
			return err
		}
		dst := filepath.Join(dir, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := fs.ReadFile(androidHost.Files, path)
		if err != nil {
			return err
		}
		if old, err := os.ReadFile(dst); err == nil && bytes.Equal(old, data) {
			return nil
		}
		mode := os.FileMode(0o644)
		if filepath.Base(path) == "gradlew" {
			mode = 0o755
		}
		return os.WriteFile(dst, data, mode)
	})
}

// manifest is the app's AndroidManifest.xml: its permissions and URL
// schemes over the host's.
func androidManifest(c *Config, plugins mobilePlugins) []byte {
	perms := []string{"android.permission.INTERNET", "android.permission.ACCESS_NETWORK_STATE", "android.permission.POST_NOTIFICATIONS"}
	for _, p := range append(plugins.permissions(), c.Android.Permissions...) {
		if !strings.Contains(p, ".") {
			p = "android.permission." + p
		}
		if !slices.Contains(perms, p) {
			perms = append(perms, p)
		}
	}
	var b strings.Builder
	b.WriteString(`<manifest xmlns:android="http://schemas.android.com/apk/res/android">` + "\n")
	for _, p := range perms {
		fmt.Fprintf(&b, "    <uses-permission android:name=%q />\n", p)
	}
	theme := "Theme.MyGo"
	if c.Android.Splash != "" {
		theme = androidSplashTheme
	}
	b.WriteString(`    <application
        android:label="${mygoAppName}"
        android:icon="${mygoIcon}"
        android:supportsRtl="true"
        android:enableOnBackInvokedCallback="true"
        android:theme="@style/` + theme + `">
        <activity
            android:name="dev.mygo.MyGoActivity"
            android:exported="true"
            android:launchMode="singleTask"
            android:configChanges="orientation|screenSize|screenLayout|smallestScreenSize|keyboard|keyboardHidden|navigation|uiMode|fontScale|density|locale|layoutDirection"
            android:windowSoftInputMode="adjustResize">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
`)
	for _, s := range c.URLSchemes {
		fmt.Fprintf(&b, `            <intent-filter>
                <action android:name="android.intent.action.VIEW" />
                <category android:name="android.intent.category.DEFAULT" />
                <category android:name="android.intent.category.BROWSABLE" />
                <data android:scheme=%q />
            </intent-filter>
`, s)
	}
	for _, h := range c.Android.AppLinks {
		// One filter a host: Android verifies the hosts of a filter together.
		fmt.Fprintf(&b, `            <intent-filter android:autoVerify="true">
                <action android:name="android.intent.action.VIEW" />
                <category android:name="android.intent.category.DEFAULT" />
                <category android:name="android.intent.category.BROWSABLE" />
                <data android:scheme="https" android:host=%q />
            </intent-filter>
`, h)
	}
	b.WriteString("        </activity>\n")
	for _, x := range plugins.manifestApplication() {
		b.WriteString(x + "\n")
	}
	b.WriteString("    </application>\n</manifest>\n")
	return []byte(b.String())
}

// androidIconSizes are the sizes of a launcher icon, in pixels at each
// density: 48dp. Those of an adaptive icon's layers are 108dp.
var androidIconSizes = map[string]int{"mdpi": 48, "hdpi": 72, "xhdpi": 96, "xxhdpi": 144, "xxxhdpi": 192}

// androidIcons writes the launcher icon at each density into res, with the
// adaptive icon of Android 8.0 and later over it. It reports whether the app
// has one.
func androidIcons(c *Config, res string) (bool, error) {
	var legacy image.Image
	if c.Icon != "" {
		src, err := os.ReadFile(c.path(c.Icon))
		if err != nil {
			return false, err
		}
		if legacy, err = png.Decode(bytes.NewReader(src)); err != nil {
			return false, fmt.Errorf("%s: %v", c.Icon, err)
		}
	}
	if a := c.Android.AdaptiveIcon; a != nil {
		fg, err := writeAdaptiveIcon(c, a, res)
		if err != nil {
			return false, err
		}
		if legacy == nil {
			// Systems older than Android 8.0 show the icon's two layers as one.
			if legacy, err = flattenAdaptiveIcon(fg, a.Background); err != nil {
				return false, err
			}
		}
	}
	if legacy == nil {
		return false, nil
	}
	for density, size := range androidIconSizes {
		if err := writeIconPNG(filepath.Join(res, "mipmap-"+density, "ic_launcher.png"), resize(legacy, size)); err != nil {
			return false, err
		}
	}
	return true, nil
}

func writeIconPNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// writeAdaptiveIcon writes the layers of an adaptive icon into res, and
// returns its foreground.
func writeAdaptiveIcon(c *Config, a *AndroidAdaptiveIcon, res string) (image.Image, error) {
	src, err := os.ReadFile(c.path(a.Foreground))
	if err != nil {
		return nil, err
	}
	if err := validateIcon(src); err != nil {
		return nil, fmt.Errorf("android.adaptiveIcon.foreground: %w", err)
	}
	fg, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("%s: %v", a.Foreground, err)
	}
	for density, size := range androidIconSizes {
		if err := writeIconPNG(filepath.Join(res, "mipmap-"+density, "ic_launcher_foreground.png"), resize(fg, size*108/48)); err != nil {
			return nil, err
		}
	}
	files := map[string]string{
		filepath.Join("mipmap-anydpi-v26", "ic_launcher.xml"): `<adaptive-icon xmlns:android="http://schemas.android.com/apk/res/android">
    <background android:drawable="@color/ic_launcher_background" />
    <foreground android:drawable="@mipmap/ic_launcher_foreground" />
</adaptive-icon>
`,
		filepath.Join("values", "mygo_icon.xml"): fmt.Sprintf("<resources>\n    <color name=\"ic_launcher_background\">%s</color>\n</resources>\n", a.Background),
	}
	return fg, writeResFiles(res, files)
}

// flattenAdaptiveIcon draws what launchers show of an adaptive icon, the
// middle 72/108 of its foreground, over its background.
func flattenAdaptiveIcon(fg image.Image, background string) (image.Image, error) {
	col, err := parseHexColor(background)
	if err != nil {
		return nil, err
	}
	b := fg.Bounds()
	inset := b.Dx() * 18 / 108
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx()-2*inset, b.Dy()-2*inset))
	draw.Draw(out, out.Bounds(), image.NewUniform(color.NRGBA{col[0], col[1], col[2], 255}), image.Point{}, draw.Src)
	draw.Draw(out, out.Bounds(), fg, b.Min.Add(image.Pt(inset, inset)), draw.Over)
	return out, nil
}

// androidSplashTheme is the style the manifest names with android.splash:
// the host's theme, which a resource of the same name could not replace in
// its source set, with the splash screen's color.
const androidSplashTheme = "Theme.MyGo.Splash"

// writeAndroidSplash writes the style of the splash screen into res. Android
// 12 and later take its color from windowSplashScreenBackground, and
// earlier systems show the window's background while the app starts.
func writeAndroidSplash(res, background string) error {
	style := func(items string) string {
		return `<resources>
    <style name="` + androidSplashTheme + `" parent="Theme.MyGo">
        <item name="android:windowBackground">@color/mygo_splash</item>
` + items + `    </style>
</resources>
`
	}
	return writeResFiles(res, map[string]string{
		filepath.Join("values", "mygo_splash_color.xml"): fmt.Sprintf("<resources>\n    <color name=\"mygo_splash\">%s</color>\n</resources>\n", background),
		filepath.Join("values", "mygo_splash.xml"):       style(""),
		filepath.Join("values-v31", "mygo_splash.xml"):   style("        <item name=\"android:windowSplashScreenBackground\">@color/mygo_splash</item>\n"),
	})
}

func writeResFiles(res string, files map[string]string) error {
	for name, content := range files {
		path := filepath.Join(res, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// versionCode is the integer version Google Play orders uploads by.
func (o Android) versionCode(version string) int {
	if o.VersionCode > 0 {
		return o.VersionCode
	}
	p := strings.SplitN(strings.SplitN(version, "-", 2)[0], ".", 3)
	n := 0
	for i, mul := range []int{10000, 100, 1} {
		if i < len(p) {
			v, _ := strconv.Atoi(p[i])
			n += v * mul
		}
	}
	return max(n, 1)
}

// gradle builds the app with the host's project in dir: tasks such as
// :app:assembleRelease, with the app's Go libraries in jniLibs and its
// resources, manifest and icons in work.
func (t *androidToolchain) gradle(ctx context.Context, c, dc *Config, dir, work, jniLibs string, tasks ...string) error {
	res := filepath.Join(work, "res")
	os.RemoveAll(res)
	hasIcon, err := androidIcons(dc, res)
	if err != nil {
		return err
	}
	if dc.Android.Splash != "" {
		if err := writeAndroidSplash(res, dc.Android.Splash); err != nil {
			return err
		}
	}
	plugins, err := c.mobilePlugins("android")
	if err != nil {
		return err
	}
	if err := plugins.writeKotlin(filepath.Join(dir, "mygo", "src", "main", "java", "dev", "mygo")); err != nil {
		return err
	}
	manifest := filepath.Join(work, "AndroidManifest.xml")
	if err := os.WriteFile(manifest, androidManifest(dc, plugins), 0o644); err != nil {
		return err
	}
	assets := filepath.Join(work, "assets")
	os.RemoveAll(assets)
	list, err := dc.appResources("android", "arm64")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(assets, "resources"), 0o755); err != nil {
		return err
	}
	if err := copyResources(list, filepath.Join(assets, "resources")); err != nil {
		return err
	}
	debug := slices.Contains(tasks, ":app:assembleDebug")
	mods, err := c.phoneModules("android", debug)
	if err != nil {
		return err
	}
	props := map[string]string{
		"mygoNativeModules":      androidModules(mods),
		"mygoNdk":                t.ndk,
		"mygoAbis":               strings.Join(abisIn(jniLibs), ","),
		"mygoApplicationId":      dc.Identifier,
		"mygoAppName":            dc.Name,
		"mygoVersionCode":        strconv.Itoa(dc.Android.versionCode(dc.Version)),
		"mygoVersionName":        dc.Version,
		"mygoMinSdk":             strconv.Itoa(dc.Android.MinSdk),
		"mygoJniLibs":            jniLibs,
		"mygoAssets":             assets,
		"mygoRes":                res,
		"mygoManifest":           manifest,
		"mygoPluginDependencies": strings.Join(plugins.dependencies(), ","),
	}
	if hasIcon {
		props["mygoIcon"] = "@mipmap/ic_launcher"
	}
	if ks := dc.Android.Keystore; ks != "" {
		pass := os.Getenv("MYGO_ANDROID_KEYSTORE_PASSWORD")
		if pass == "" {
			return errors.New("android.keystore needs its password in MYGO_ANDROID_KEYSTORE_PASSWORD")
		}
		props["mygoKeystore"] = c.path(ks)
		props["mygoKeystorePassword"] = pass
		props["mygoKeyAlias"] = dc.Android.KeyAlias
		if kp := os.Getenv("MYGO_ANDROID_KEY_PASSWORD"); kp != "" {
			props["mygoKeyPassword"] = kp
		}
	}
	args := []string{"-q", "--console=plain"}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var env []string
	for _, k := range keys {
		// Passwords go in the environment, out of the command line that
		// other users' ps shows.
		env = append(env, "ORG_GRADLE_PROJECT_"+k+"="+props[k])
	}
	args = append(args, tasks...)
	gradlew := filepath.Join(dir, "gradlew")
	if runtime.GOOS == "windows" {
		gradlew += ".bat"
	}
	cmd := exec.CommandContext(ctx, gradlew, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "ANDROID_HOME="+t.sdk)
	if t.javaHome != "" {
		cmd.Env = append(cmd.Env, "JAVA_HOME="+t.javaHome)
	}
	cmd.Env = append(cmd.Env, env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gradle failed: %v\n%s", err, bytes.TrimSpace(out))
	}
	return nil
}

// abisIn returns the ABIs that Go was built for in jniLibs, which the
// native modules' C builds for too.
func abisIn(jniLibs string) []string {
	var abis []string
	entries, _ := os.ReadDir(jniLibs)
	for _, e := range entries {
		if e.IsDir() {
			abis = append(abis, e.Name())
		}
	}
	return abis
}

// buildAndroid builds the app for Android: an .apk and an .aab for Google
// Play, for every ABI, or one (android/arm64, android/arm, android/amd64).
func buildAndroid(c *Config, opts buildOptions, arch string) ([]string, error) {
	t, err := newAndroidToolchain()
	if err != nil {
		return nil, err
	}
	var abis []string
	for _, a := range androidABIs {
		if arch == "" || arch == a.goarch || arch == a.abi {
			abis = append(abis, a.abi)
		}
	}
	if len(abis) == 0 {
		return nil, fmt.Errorf("unknown Android architecture %q: arm64, arm or amd64", arch)
	}
	dir := c.path(filepath.Join(".mygo", "android"))
	if err := t.project(dir); err != nil {
		return nil, err
	}
	work := filepath.Join(opts.work, "android")
	jniLibs := filepath.Join(work, "jniLibs")
	os.RemoveAll(jniLibs)
	tk := con.start("Building for Android")
	err = func() error {
		// Their generated Go, which the app's compiles with, is fresh.
		if _, err := c.phoneModules("android", opts.debug); err != nil {
			return err
		}
		for _, abi := range abis {
			if err := t.buildGo(context.Background(), c, c, tk, opts, abi, jniLibs); err != nil {
				return err
			}
		}
		tk.set("building the APK and the bundle (Gradle)")
		return t.gradle(context.Background(), c, c, dir, work, jniLibs, ":app:assembleRelease", ":app:bundleRelease")
	}()
	if err := tk.end(err, "Built for Android"); err != nil {
		return nil, err
	}
	out := c.path(filepath.Join(c.Out, "android"))
	if err := os.MkdirAll(out, 0o755); err != nil {
		return nil, err
	}
	name := c.slug() + "-" + c.Version
	var artifacts []string
	for src, dst := range map[string]string{
		filepath.Join(dir, "app", "build", "outputs", "apk", "release", "app-release.apk"):    name + ".apk",
		filepath.Join(dir, "app", "build", "outputs", "bundle", "release", "app-release.aab"): name + ".aab",
	} {
		data, err := os.ReadFile(src)
		if err != nil {
			return nil, err
		}
		p := filepath.Join(out, dst)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, p)
	}
	sort.Strings(artifacts)
	if c.Android.Keystore == "" {
		warnf("Signed with the debug key: set android.keystore for Google Play")
	}
	return artifacts, nil
}

// adb runs adb of the SDK.
func adb(serial string, args ...string) *exec.Cmd {
	if serial != "" {
		args = append([]string{"-s", serial}, args...)
	}
	return exec.Command(sdkTool("platform-tools", "adb"), args...)
}

// androidDevice finds the device or emulator to run on: id if set (adb's
// serial, the name of a running emulator's AVD or a device's model), else
// the first adb reaches. An AVD that is not running starts.
func androidDevice(ctx context.Context, id string) (*mobileDevice, error) {
	devs, err := adbDevices()
	if err != nil {
		return nil, fmt.Errorf("adb: %v (install the SDK's platform tools)", err)
	}
	d, err := pickAndroidDevice(devs, id)
	if err == nil {
		return &mobileDevice{platform: "android", id: d.serial, name: d.displayName()}, nil
	}
	avds := listAVDs()
	// One adb lists is starting or stuck, and a second would not start.
	running := slices.ContainsFunc(devs, func(d adbDevice) bool { return d.avd == id })
	if id != "" && !running && slices.Contains(avds, id) {
		booted, err := startAVD(ctx, id)
		if err != nil {
			return nil, err
		}
		return &mobileDevice{platform: "android", id: booted.serial, name: id}, nil
	}
	if id == "" && len(avds) > 0 {
		return nil, fmt.Errorf("%v; or start an emulator with -device %s", err, avds[0])
	}
	return nil, err
}

// buildAndroidDev builds the debug APK for the device's ABI.
func (s *devSession) buildAndroidDev(ctx context.Context, c, dc *Config, t *task, opts buildOptions, stage string) (string, error) {
	tc, err := newAndroidToolchain()
	if err != nil {
		return "", err
	}
	abiOut, err := adb(s.device.id, "shell", "getprop", "ro.product.cpu.abi").Output()
	if err != nil {
		return "", fmt.Errorf("adb: %v", err)
	}
	abi := strings.TrimSpace(string(abiOut))
	dir := c.path(filepath.Join(".mygo", "android"))
	if err := tc.project(dir); err != nil {
		return "", err
	}
	jniLibs := filepath.Join(opts.work, "jniLibs")
	os.RemoveAll(jniLibs)
	if _, err := c.phoneModules("android", true); err != nil {
		return "", err
	}
	if err := tc.buildGo(ctx, c, dc, t, opts, abi, jniLibs); err != nil {
		return "", err
	}
	// Only Go changed: the app the device has takes the new library, without
	// Gradle.
	key, keyErr := androidHostKey(c, dc, dir)
	if keyErr == nil && key == s.androidHost && s.installedOn == s.device.id {
		lib := filepath.Join(stage, dc.slug()+".lib")
		if err := os.MkdirAll(lib, 0o755); err != nil {
			return "", err
		}
		data, err := os.ReadFile(filepath.Join(jniLibs, abi, "libmygoapp.so"))
		if err != nil {
			return "", err
		}
		return lib, os.WriteFile(filepath.Join(lib, "libmygoapp.so"), data, 0o755)
	}
	s.androidHost = key
	s.installedOn = s.device.id
	t.set("building the APK (Gradle)")
	if err := tc.gradle(ctx, c, dc, dir, opts.work, jniLibs, ":app:assembleDebug"); err != nil {
		return "", err
	}
	apk := filepath.Join(stage, dc.slug()+".apk")
	data, err := os.ReadFile(filepath.Join(dir, "app", "build", "outputs", "apk", "debug", "app-debug.apk"))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(apk, 0o755); err != nil {
		return "", err
	}
	// A directory holding the APK, as replacePath and hashTree take.
	return apk, os.WriteFile(filepath.Join(apk, "app.apk"), data, 0o644)
}

// launchAndroid installs the APK and starts the app with env, its log the
// output of the process it returns.
func (s *devSession) launchAndroid(dc *Config, app string, env []string) (*exec.Cmd, func(), error) {
	adb(s.device.id, "shell", "am", "force-stop", dc.Identifier).Run()
	if lib := filepath.Join(app, "libmygoapp.so"); fileExists(lib) {
		if err := s.pushAndroidLib(dc, lib); err != nil {
			return nil, nil, err
		}
	} else {
		apk := filepath.Join(app, "app.apk")
		if out, err := adb(s.device.id, "install", "-r", "-t", apk).CombinedOutput(); err != nil {
			return nil, nil, fmt.Errorf("adb install: %v\n%s", err, bytes.TrimSpace(out))
		}
		// The library of the APK, not the last one pushed.
		adb(s.device.id, "shell", "run-as", dc.Identifier, "rm", "-rf", "files/mygo-dev").Run()
	}
	adb(s.device.id, "logcat", "-c").Run()
	quoted := strings.ReplaceAll(strings.Join(env, "\n"), "'", `'\''`)
	if out, err := adb(s.device.id, "shell", "am start -W -n "+dc.Identifier+"/dev.mygo.MyGoActivity --es mygo.env '"+quoted+"'").CombinedOutput(); err != nil {
		return nil, nil, fmt.Errorf("adb am start: %v\n%s", err, bytes.TrimSpace(out))
	}
	// The app's log: what it writes (the tag mygo) and its crashes.
	cmd := adb(s.device.id, "logcat", "-v", "raw", "-s", "mygo:V", "AndroidRuntime:E", "DEBUG:V")
	serial, id := s.device.id, dc.Identifier
	return cmd, func() { adb(serial, "shell", "am", "force-stop", id).Run() }, nil
}

// pushAndroidLib gives the debuggable app on the device a new library of the
// app's Go code, which the host loads rather than its own (DevLibrary).
func (s *devSession) pushAndroidLib(dc *Config, lib string) error {
	tmp := "/data/local/tmp/mygo-" + dc.Identifier + ".so"
	if out, err := adb(s.device.id, "push", lib, tmp).CombinedOutput(); err != nil {
		return fmt.Errorf("adb push: %v\n%s", err, bytes.TrimSpace(out))
	}
	script := "mkdir -p files/mygo-dev && rm -f files/mygo-dev/libmygoapp.so && cp " + tmp + " files/mygo-dev/libmygoapp.so && chmod 500 files/mygo-dev/libmygoapp.so"
	if out, err := adb(s.device.id, "shell", "run-as "+dc.Identifier+" sh -c '"+script+"'").CombinedOutput(); err != nil {
		return fmt.Errorf("adb run-as: %v\n%s", err, bytes.TrimSpace(out))
	}
	adb(s.device.id, "shell", "rm", "-f", tmp).Run()
	return nil
}

// androidHostKey hashes what an APK holds besides the app's Go code: the host's
// project and the plugins' code in dir, the manifest, the resources and the
// configuration. While it stays, a change of the Go code needs no Gradle.
func androidHostKey(c, dc *Config, dir string) (string, error) {
	h := sha256.New()
	cfg, err := json.Marshal(dc)
	if err != nil {
		return "", err
	}
	h.Write(cfg)
	plugins, err := c.mobilePlugins("android")
	if err != nil {
		return "", err
	}
	h.Write(androidManifest(dc, plugins))
	for _, sub := range []string{"mygo/src", "mygo/build.gradle.kts", "app/build.gradle.kts", "app/src"} {
		if err := hashIfExists(h, filepath.Join(dir, filepath.FromSlash(sub))); err != nil {
			return "", err
		}
	}
	mods, err := c.phoneModules("android", true)
	if err != nil {
		return "", err
	}
	if err := hashModules(h, mods); err != nil {
		return "", err
	}
	for _, p := range plugins {
		if p.Android == nil {
			continue
		}
		files, err := p.sourcesOf(p.Android.Sources)
		if err != nil {
			return "", err
		}
		for _, f := range files {
			if err := hashIfExists(h, f); err != nil {
				return "", err
			}
		}
	}
	res, err := dc.appResources("android", "arm64")
	if err != nil {
		return "", err
	}
	for _, r := range res {
		if err := hashIfExists(h, r.src); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashIfExists(h io.Writer, path string) error {
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	return hashTree(h, path)
}

// reachDevURL returns the URL at which the device reaches the dev server
// at u: the simulator shares the Mac's network, an iOS device reaches it over
// the network, which the server must listen on, and Android's devices reach
// it at the same port through adb reverse.
func (d *mobileDevice) reachDevURL(u string) (string, error) {
	if d.platform != "android" {
		if !d.physical {
			return u, nil
		}
		ip, err := lanAddress()
		if err != nil {
			return u, fmt.Errorf("%s reaches the dev server over the network, and this computer has none: %v", d.name, err)
		}
		reached, err := lanDevURL(u, ip)
		if err != nil {
			return u, err
		}
		if reached != u {
			warnf("%s loads the dev server at %s: it must listen on the network, as vite --host", d.name, reached)
		}
		return reached, nil
	}
	p, err := url.Parse(u)
	if err != nil {
		return u, err
	}
	port := p.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[p.Scheme]
	}
	if out, err := adb(d.id, "reverse", "tcp:"+port, "tcp:"+port).CombinedOutput(); err != nil {
		return u, fmt.Errorf("adb reverse: %v\n%s", err, bytes.TrimSpace(out))
	}
	if h := p.Hostname(); h != "localhost" && h != "127.0.0.1" {
		return u, nil
	}
	p.Host = "localhost:" + port
	return p.String(), nil
}

// lanDevURL replaces a local host of a URL with ip, what another machine
// reaches this one at. Other hosts stay.
func lanDevURL(u, ip string) (string, error) {
	p, err := url.Parse(u)
	if err != nil {
		return u, err
	}
	switch p.Hostname() {
	case "localhost", "127.0.0.1", "::1":
	default:
		return u, nil
	}
	if port := p.Port(); port != "" {
		p.Host = net.JoinHostPort(ip, port)
	} else {
		p.Host = ip
	}
	return p.String(), nil
}

// lanAddress returns this computer's address on its network. Connecting a
// UDP socket sends nothing: it only picks the route.
func lanAddress() (string, error) {
	conn, err := net.Dial("udp4", "192.0.2.1:9")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}
