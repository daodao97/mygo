package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIOSCapabilitiesRejectUnknownAPIs(t *testing.T) {
	c := &Config{IOS: IOS{Capabilities: []string{"camera", "microphone", "geolocation", "photos", "biometrics"}}}
	if err := validateIOSPackaging(c, buildOptions{}); err != nil {
		t.Fatal(err)
	}
	c.IOS.Capabilities = []string{"location"}
	if err := validateIOSPackaging(c, buildOptions{}); err == nil || !strings.Contains(err.Error(), "ios.capabilities") {
		t.Fatalf("unknown capability accepted: %v", err)
	}
}

// Compile the real bridges, including Keychain reads, and inspect Mach-O imports.
// A runtime guard cannot satisfy this test: disabled APIs must not be linked.
func TestIOSCapabilitiesExcludeProtectedImports(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("requires Xcode's iOS SDK")
	}
	sdk, err := exec.Command("xcrun", "--sdk", "iphonesimulator", "--show-sdk-path").Output()
	if err != nil {
		t.Skipf("iOS SDK unavailable: %v", err)
	}
	root := t.TempDir()
	bridge, err := filepath.Abs(filepath.Join("..", "..", "internal", "ios"))
	if err != nil {
		t.Fatal(err)
	}
	// Use actual exported Go prototypes when compiling the UIKit scanner too.
	generate := []string{"tool", "cgo", "-objdir", root, "--", "-x", "objective-c", "-fobjc-arc", "-isysroot", strings.TrimSpace(string(sdk)), "-target", "arm64-apple-ios15.0-simulator", "-I", bridge}
	for _, source := range []string{"backend.go", "mobile.go", "presentation.go"} {
		generate = append(generate, filepath.Join(bridge, source))
	}
	if out, err := exec.Command("go", generate...).CombinedOutput(); err != nil {
		t.Fatalf("generate native exports: %v\n%s", err, out)
	}
	protected := map[string][]string{
		"camera":      {"_AVMediaTypeVideo", "_OBJC_CLASS_$_AVCaptureSession"},
		"microphone":  {"_AVMediaTypeAudio"},
		"geolocation": {"_OBJC_CLASS_$_CLLocationManager"},
		"photos":      {"_OBJC_CLASS_$_PHPhotoLibrary"},
		"biometrics":  {"_OBJC_CLASS_$_LAContext"},
	}
	cases := [][]string{nil, {"camera"}, {"microphone"}, {"geolocation"}, {"photos"}, {"biometrics"}, {"camera", "microphone", "geolocation", "photos", "biometrics"}}
	for _, capabilities := range cases {
		name := strings.Join(capabilities, "+")
		if name == "" {
			name = "default"
		}
		t.Run(name, func(t *testing.T) {
			var imports strings.Builder
			for _, source := range []string{"permissions.m", "authentication.m", "secrets.m", "native.m"} {
				object := filepath.Join(root, name+"-"+source+".o")
				args := []string{"--sdk", "iphonesimulator", "clang", "-c", "-fobjc-arc", "-O2", "-isysroot", strings.TrimSpace(string(sdk)), "-target", "arm64-apple-ios15.0-simulator", "-I", root, "-I", bridge}
				// The default case also verifies direct builds with no CLI flags.
				if capabilities != nil {
					args = append(args, strings.Fields(iosCapabilityFlags(&Config{IOS: IOS{Capabilities: capabilities}}))...)
				}
				args = append(args, filepath.Join(bridge, source), "-o", object)
				if out, err := exec.Command("xcrun", args...).CombinedOutput(); err != nil {
					t.Fatalf("compile %s: %v\n%s", source, err, out)
				}
				out, err := exec.Command("xcrun", "nm", "-u", object).CombinedOutput()
				if err != nil {
					t.Fatalf("inspect %s: %v\n%s", source, err, out)
				}
				imports.Write(out)
			}
			for capability, symbols := range protected {
				want := false
				for _, enabled := range capabilities {
					want = want || enabled == capability
				}
				for _, symbol := range symbols {
					if got := strings.Contains(imports.String(), symbol); got != want {
						t.Errorf("import %s present=%v, want %v", symbol, got, want)
					}
				}
			}
			for _, retained := range []string{"_OBJC_CLASS_$_UNUserNotificationCenter", "_SecItemCopyMatching"} {
				if !strings.Contains(imports.String(), retained) {
					t.Errorf("ordinary service removed: %s", retained)
				}
			}
		})
	}
}
