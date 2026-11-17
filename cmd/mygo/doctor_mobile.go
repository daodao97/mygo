package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// doctorLine is a line of mygo doctor about phones. What is missing there
// only warns, with how to fix it: desktop apps need none of it.
type doctorLine struct {
	name   string
	good   bool
	detail string
}

// print writes the line in doctor's style.
func (l doctorLine) print() {
	mark := "\033[32m✓\033[0m"
	if !l.good {
		mark = "\033[33m!\033[0m"
	}
	fmt.Printf("%s %-10s %s\n", mark, l.name, l.detail)
}

// iosFacts is what doctor finds out for iOS apps.
type iosFacts struct {
	xcode, xcodeBuild string // of xcodebuild -version, "" without Xcode
	developerDir      string // of xcode-select -p, "" without command line tools
	sdk, simulatorSDK string
	simulatorsKnown   bool // simctl answered
	simulators        []deviceEntry
	developmentIDs    int // identities that sign for devices
	distributionIDs   int // and for the App Store
	profiles          int
}

// iosDoctorLines says what facts mean, and how to fix what is missing.
func iosDoctorLines(f iosFacts) []doctorLine {
	const installXcode = "install Xcode from the App Store, then run sudo xcode-select -s /Applications/Xcode.app"
	if f.xcode == "" {
		detail := "not found (needed for iOS apps): " + installXcode
		if f.developerDir != "" {
			detail = "only the command line tools, in " + f.developerDir + " (iOS apps need Xcode): " + installXcode
		}
		return []doctorLine{{"xcode", false, detail}}
	}
	lines := []doctorLine{{"xcode", true, f.xcode + " (" + f.xcodeBuild + "), in " + f.developerDir}}
	if f.sdk == "" {
		lines = append(lines, doctorLine{"ios sdk", false, "not found: run xcodebuild -downloadPlatform iOS, or install it in Xcode (Settings › Components)"})
	} else {
		lines = append(lines, doctorLine{"ios sdk", true, f.sdk + " (simulator " + or(f.simulatorSDK, "not found") + ")"})
	}
	booted := 0
	for _, d := range f.simulators {
		if d.State == stateBooted {
			booted++
		}
	}
	if !f.simulatorsKnown || len(f.simulators) == 0 {
		lines = append(lines, doctorLine{"simulators", false, "none: add an iOS runtime in Xcode (Settings › Components)"})
	} else {
		lines = append(lines, doctorLine{"simulators", true, fmt.Sprintf("%d available, %d booted", len(f.simulators), booted)})
	}
	if f.developmentIDs+f.distributionIDs == 0 {
		lines = append(lines, doctorLine{"signing", false, "no Apple Development or Distribution identity (needed for devices and the App Store): sign in to your team in Xcode (Settings › Accounts), then Manage Certificates"})
	} else {
		lines = append(lines, doctorLine{"signing", true, fmt.Sprintf("%d development, %d distribution identities", f.developmentIDs, f.distributionIDs)})
	}
	if f.profiles == 0 {
		lines = append(lines, doctorLine{"profiles", false, "no provisioning profiles (needed for devices and the App Store): Xcode makes them (Settings › Accounts), or developer.apple.com"})
	} else {
		lines = append(lines, doctorLine{"profiles", true, fmt.Sprintf("%d provisioning profiles", f.profiles)})
	}
	return lines
}

// parseXcodeVersion reads xcodebuild -version.
func parseXcodeVersion(out string) (version, build string) {
	for _, l := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(l, "Build version "); ok {
			build = strings.TrimSpace(v)
		} else if strings.HasPrefix(l, "Xcode ") {
			version = strings.TrimSpace(l)
		}
	}
	return version, build
}

// countSigningIdentities counts the lines of security find-identity that
// sign for devices, and for the App Store.
func countSigningIdentities(out string) (development, distribution int) {
	re := regexp.MustCompile(`^\s*\d+\) [0-9A-F]{40} "(Apple Development|iPhone Developer|Apple Distribution|iPhone Distribution)`)
	for _, l := range strings.Split(out, "\n") {
		m := re.FindStringSubmatch(l)
		switch {
		case m == nil:
		case m[1] == "Apple Development" || m[1] == "iPhone Developer":
			development++
		default:
			distribution++
		}
	}
	return development, distribution
}

// probeIOS asks the tools. On a Mac without Xcode, xcrun may offer to
// install the command line tools; xcode-select -p, asked first, does not.
func probeIOS() iosFacts {
	var f iosFacts
	if out, err := toolOutput("xcode-select", "-p"); err == nil {
		f.developerDir = strings.TrimSpace(string(out))
	}
	if f.developerDir == "" {
		return f
	}
	if out, err := toolOutput("xcodebuild", "-version"); err == nil {
		f.xcode, f.xcodeBuild = parseXcodeVersion(string(out))
	}
	if f.xcode == "" {
		return f
	}
	if out, err := toolOutput("xcrun", "--sdk", "iphoneos", "--show-sdk-version"); err == nil {
		f.sdk = strings.TrimSpace(string(out))
	}
	if out, err := toolOutput("xcrun", "--sdk", "iphonesimulator", "--show-sdk-version"); err == nil {
		f.simulatorSDK = strings.TrimSpace(string(out))
	}
	if sims, err := iosSimulators(); err == nil {
		f.simulators, f.simulatorsKnown = sims, true
	}
	if out, err := toolOutput("security", "find-identity", "-v", "-p", "codesigning"); err == nil {
		f.developmentIDs, f.distributionIDs = countSigningIdentities(string(out))
	}
	for _, d := range profileDirs() {
		files, _ := filepath.Glob(filepath.Join(d, "*.mobileprovision"))
		f.profiles += len(files)
	}
	return f
}

// androidFacts is what doctor finds out for Android apps.
type androidFacts struct {
	sdk         string
	platforms   []string
	adb         string // its version line, "" without it
	ndk         string // its version, or its directory
	java        string
	javaMajor   int
	emulator    bool // the SDK's emulator is there
	avds        []string
	devices     []adbDevice
	devicesRead bool // adb answered
}

// androidDoctorLines says what facts mean, and how to fix what is missing.
func androidDoctorLines(f androidFacts) []doctorLine {
	if f.sdk == "" {
		return []doctorLine{{"android", false, "no SDK (needed for Android apps): install Android Studio, or set ANDROID_HOME"}}
	}
	detail := f.sdk
	if n := len(f.platforms); n > 0 {
		detail += " (API " + f.platforms[n-1] + ")"
	}
	lines := []doctorLine{{"android", true, detail}}
	if f.adb == "" {
		lines = append(lines, doctorLine{"adb", false, "not found: install Android SDK Platform-Tools in Android Studio (SDK Manager › SDK Tools)"})
	} else {
		lines = append(lines, doctorLine{"adb", true, f.adb})
	}
	if f.ndk == "" {
		lines = append(lines, doctorLine{"ndk", false, "not found: install it in Android Studio (SDK Manager › SDK Tools › NDK), or set ANDROID_NDK_HOME"})
	} else {
		lines = append(lines, doctorLine{"ndk", true, f.ndk})
	}
	switch {
	case f.java == "":
		lines = append(lines, doctorLine{"jdk", false, "not found: Android Studio has one, or set JAVA_HOME to a JDK 17 or later"})
	case f.javaMajor < 17:
		lines = append(lines, doctorLine{"jdk", false, f.java + " is too old: set JAVA_HOME to a JDK 17 or later, as Android Studio's"})
	default:
		lines = append(lines, doctorLine{"jdk", true, f.java})
	}
	if len(f.avds) == 0 {
		hint := "none: make a virtual device in Android Studio (Device Manager)"
		if !f.emulator {
			hint = "no emulator: install Android Emulator in Android Studio (SDK Manager › SDK Tools), then make a virtual device"
		}
		lines = append(lines, doctorLine{"emulators", false, hint})
	} else {
		lines = append(lines, doctorLine{"emulators", true, fmt.Sprintf("%d: %s", len(f.avds), strings.Join(f.avds, ", "))})
	}
	if f.devicesRead {
		ready, refusing := 0, 0
		for _, d := range f.devices {
			switch d.state {
			case "device":
				ready++
			case "unauthorized":
				refusing++
			}
		}
		switch {
		case refusing > 0:
			lines = append(lines, doctorLine{"devices", false, fmt.Sprintf("%d connected, %d not allowing this computer: accept USB debugging on the device", ready, refusing)})
		case ready > 0:
			lines = append(lines, doctorLine{"devices", true, fmt.Sprintf("%d connected", ready)})
		default:
			lines = append(lines, doctorLine{"devices", true, "none connected (mygo dev -device <emulator> starts one)"})
		}
	}
	return lines
}

// probeAndroid asks the tools.
func probeAndroid() androidFacts {
	var f androidFacts
	f.sdk = findAndroidSDK()
	if f.sdk == "" {
		return f
	}
	f.platforms = androidPlatforms(f.sdk)
	adbTool := sdkTool("platform-tools", "adb")
	if out, err := toolOutput(adbTool, "--version"); err == nil {
		f.adb, _, _ = strings.Cut(string(out), "\n")
		f.adb = strings.TrimSpace(f.adb)
		if devs, err := adbDevices(); err == nil {
			f.devices, f.devicesRead = devs, true
		}
	}
	if ndk := findAndroidNDK(f.sdk); ndk != "" {
		f.ndk = filepath.Base(ndk)
		if data, err := os.ReadFile(filepath.Join(ndk, "source.properties")); err == nil {
			f.ndk = or(parseNDKVersion(string(data)), f.ndk)
		}
	}
	java := "java"
	if h := findJavaHome(); h != "" {
		java = filepath.Join(h, "bin", "java")
	}
	// java -version writes to its standard error.
	ctx, cancel := context.WithTimeout(context.Background(), toolTimeout)
	defer cancel()
	if out, err := exec.CommandContext(ctx, java, "-version").CombinedOutput(); err == nil {
		if v, major := parseJavaVersion(string(out)); v != "" {
			f.java, f.javaMajor = v+" ("+java+")", major
		}
	}
	emulator := sdkTool("emulator", "emulator")
	if _, err := exec.LookPath(emulator); err == nil {
		f.emulator = true
		f.avds = listAVDs()
	}
	return f
}
