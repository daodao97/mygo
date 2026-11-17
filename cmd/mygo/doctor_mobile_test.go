package main

import (
	"strings"
	"testing"
)

func lineOf(t *testing.T, lines []doctorLine, name string) doctorLine {
	t.Helper()
	for _, l := range lines {
		if l.name == name {
			return l
		}
	}
	t.Fatalf("no line %s in %+v", name, lines)
	return doctorLine{}
}

func TestIOSDoctorLines(t *testing.T) {
	// Without Xcode, one line says what to do.
	lines := iosDoctorLines(iosFacts{})
	if len(lines) != 1 || lines[0].good || !strings.Contains(lines[0].detail, "App Store") {
		t.Errorf("no Xcode: %+v", lines)
	}
	lines = iosDoctorLines(iosFacts{developerDir: "/Library/Developer/CommandLineTools"})
	if len(lines) != 1 || lines[0].good || !strings.Contains(lines[0].detail, "command line tools") {
		t.Errorf("command line tools: %+v", lines)
	}

	f := iosFacts{xcode: "Xcode 27.1", xcodeBuild: "27A9275", developerDir: "/Applications/Xcode.app/Contents/Developer"}
	lines = iosDoctorLines(f)
	for _, name := range []string{"ios sdk", "simulators", "signing", "profiles"} {
		if l := lineOf(t, lines, name); l.good || !strings.Contains(l.detail, ":") {
			t.Errorf("missing %s has no hint: %+v", name, l)
		}
	}
	if !lineOf(t, lines, "xcode").good {
		t.Errorf("xcode: %+v", lines)
	}

	f.sdk, f.simulatorSDK = "27.1", "27.1"
	f.simulatorsKnown = true
	f.simulators = []deviceEntry{{State: stateBooted}, {State: stateShutdown}, {State: stateBooted}}
	f.developmentIDs, f.distributionIDs, f.profiles = 1, 2, 3
	lines = iosDoctorLines(f)
	for name, want := range map[string]string{
		"ios sdk":    "27.1 (simulator 27.1)",
		"simulators": "3 available, 2 booted",
		"signing":    "1 development, 2 distribution",
		"profiles":   "3 provisioning",
	} {
		if l := lineOf(t, lines, name); !l.good || !strings.Contains(l.detail, want) {
			t.Errorf("%s: %+v", name, l)
		}
	}
}

func TestParseXcodeVersion(t *testing.T) {
	v, b := parseXcodeVersion("Xcode 27.1\nBuild version 27A9275\n")
	if v != "Xcode 27.1" || b != "27A9275" {
		t.Errorf("%q %q", v, b)
	}
}

func TestCountSigningIdentities(t *testing.T) {
	const sha = "0123456789ABCDEF0123456789ABCDEF01234567"
	out := `
  1) ` + sha + ` "Apple Development: Jane Doe (AAAAAAAAAA)"
  2) ` + sha + ` "iPhone Developer: Jane Doe (AAAAAAAAAA)"
  3) ` + sha + ` "Apple Distribution: Acme (BBBBBBBBBB)"
  4) ` + sha + ` "Developer ID Application: Acme (BBBBBBBBBB)"
     4 valid identities found
`
	if dev, dist := countSigningIdentities(out); dev != 2 || dist != 1 {
		t.Errorf("%d development, %d distribution", dev, dist)
	}
}

func TestAndroidDoctorLines(t *testing.T) {
	lines := androidDoctorLines(androidFacts{})
	if len(lines) != 1 || lines[0].good || !strings.Contains(lines[0].detail, "ANDROID_HOME") {
		t.Errorf("no SDK: %+v", lines)
	}

	f := androidFacts{sdk: "/sdk"}
	lines = androidDoctorLines(f)
	for name, hint := range map[string]string{"adb": "Platform-Tools", "ndk": "ANDROID_NDK_HOME", "jdk": "JAVA_HOME", "emulators": "Android Emulator"} {
		if l := lineOf(t, lines, name); l.good || !strings.Contains(l.detail, hint) {
			t.Errorf("%s: %+v", name, l)
		}
	}

	f = androidFacts{sdk: "/sdk", platforms: []string{"35", "36"}, adb: "Android Debug Bridge version 1.0.41", ndk: "27.1.12297006",
		java: "21.0.8", javaMajor: 21, emulator: true, avds: []string{"Pixel_9"}, devicesRead: true,
		devices: []adbDevice{{serial: "a", state: "device"}, {serial: "b", state: "offline"}}}
	lines = androidDoctorLines(f)
	for name, want := range map[string]string{"android": "API 36", "ndk": "27.1", "jdk": "21.0.8", "emulators": "Pixel_9", "devices": "1 connected"} {
		if l := lineOf(t, lines, name); !l.good || !strings.Contains(l.detail, want) {
			t.Errorf("%s: %+v", name, l)
		}
	}

	f.java, f.javaMajor = "1.8.0_392", 8
	if l := lineOf(t, androidDoctorLines(f), "jdk"); l.good || !strings.Contains(l.detail, "too old") {
		t.Errorf("old JDK: %+v", l)
	}
	f.devices = []adbDevice{{serial: "a", state: "unauthorized"}}
	if l := lineOf(t, androidDoctorLines(f), "devices"); l.good || !strings.Contains(l.detail, "USB debugging") {
		t.Errorf("refusing: %+v", l)
	}
	// A device is not something to have: none is fine.
	f.devices = nil
	if l := lineOf(t, androidDoctorLines(f), "devices"); !l.good {
		t.Errorf("no device: %+v", l)
	}
}

func TestParseToolVersions(t *testing.T) {
	if got := parseNDKVersion("Pkg.Desc = Android NDK\nPkg.Revision = 27.1.12297006\nPkg.BaseRevision = 27.0\n"); got != "27.1.12297006" {
		t.Errorf("ndk: %q", got)
	}
	for out, major := range map[string]int{
		"openjdk version \"21.0.8\" 2025-07-15\nOpenJDK Runtime": 21,
		"java version \"1.8.0_392\"\n":                           8,
		"openjdk version \"17\" 2021-09-14":                      17,
		"The operation couldn’t be completed.":                   0,
	} {
		if _, got := parseJavaVersion(out); got != major {
			t.Errorf("%q: %d, want %d", out, got, major)
		}
	}
}
