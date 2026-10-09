package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIOSIPACheckReadsAndVerifiesZIPCRC(t *testing.T) {
	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	entry, err := w.CreateHeader(&zip.FileHeader{Name: "Payload/A.app/x", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	const content = "mygo-crc-fixture"
	entry.Write([]byte(content))
	w.Close()
	data := buffer.Bytes()
	index := bytes.Index(data, []byte(content))
	if index < 0 {
		t.Fatal("missing fixture")
	}
	data[index] ^= 1
	root := t.TempDir()
	path := filepath.Join(root, "bad.ipa")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := extractIOSIPA(path, filepath.Join(root, "extracted")); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("CRC was not verified: %v", err)
	}
}

func TestIOSIPAExtractionRejectsUnsafeAndAmbiguousPackages(t *testing.T) {
	for _, names := range [][]string{
		{"../escape"}, {"/escape"}, {`Payload\..\escape`},
		{"Payload/A.app/x", "Payload/A.app/x"},
		{"Payload/A.app/x", "Payload/B.app/x"},
		{"missing-payload"},
	} {
		root := t.TempDir()
		file, err := os.Create(filepath.Join(root, "test.ipa"))
		if err != nil {
			t.Fatal(err)
		}
		w := zip.NewWriter(file)
		for _, name := range names {
			entry, err := w.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			entry.Write([]byte("fixture"))
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		file.Close()
		if _, err := extractIOSIPA(filepath.Join(root, "test.ipa"), filepath.Join(root, "extracted")); err == nil {
			t.Fatalf("unsafe package accepted: %v", names)
		}
		if _, err := os.Stat(filepath.Join(root, "escape")); !os.IsNotExist(err) {
			t.Fatal("escaped verification directory")
		}
	}
	for _, kind := range []string{"symlink", "oversized"} {
		root := t.TempDir()
		path := filepath.Join(root, "test.ipa")
		file, _ := os.Create(path)
		w := zip.NewWriter(file)
		h := &zip.FileHeader{Name: "Payload/A.app/unsafe", Method: zip.Store}
		if kind == "symlink" {
			h.SetMode(os.ModeSymlink | 0o777)
		} else {
			h.UncompressedSize64 = 3 << 30
		}
		if _, err := w.CreateRaw(h); err != nil {
			t.Fatal(err)
		}
		w.Close()
		file.Close()
		if _, err := extractIOSIPA(path, filepath.Join(root, "extracted")); err == nil {
			t.Fatalf("accepted %s", kind)
		}
	}
}

func TestIOSPrivacyCheckDetectsMissingRuntimeDeclarations(t *testing.T) {
	d := map[string]any{"NSPrivacyAccessedAPITypes": []any{map[string]any{"NSPrivacyAccessedAPIType": "NSPrivacyAccessedAPICategorySystemBootTime", "NSPrivacyAccessedAPITypeReasons": []any{"35F9.1"}}}}
	if err := iosCheckPrivacyImports(d, []string{"_mach_absolute_time"}); err != nil {
		t.Fatal(err)
	}
	if err := iosCheckPrivacyImports(d, []string{"_fstat$INODE64"}); err == nil {
		t.Fatal("missing file-timestamp reason accepted")
	}
	if err := iosCheckPrivacyImports(d, []string{"_statfs64"}); err == nil {
		t.Fatal("missing disk-space reason accepted")
	}
}

func TestIOSPurposeStringsRejectLocationUploadFailure(t *testing.T) {
	imports := []string{"_OBJC_CLASS_$_CLLocationManager"}
	for _, value := range []any{nil, true, 42, "", " \n\t", strings.Repeat("x", 4000)} {
		info := map[string]any{"NSLocationWhenInUseUsageDescription": value}
		if err := iosCheckPurposeStrings(info, imports); err == nil || !strings.Contains(err.Error(), "NSLocationWhenInUseUsageDescription") {
			t.Fatalf("invalid location purpose accepted (%T): %v", value, err)
		}
	}
	info := map[string]any{"NSLocationWhenInUseUsageDescription": "Show nearby places while using the app."}
	if err := iosCheckPurposeStrings(info, imports); err != nil {
		t.Fatal(err)
	}
	if err := iosCheckPurposeStrings(nil, []string{"_OBJC_CLASS_$_NSString", "_mach_absolute_time"}); err != nil {
		t.Fatalf("unrelated imports require permissions: %v", err)
	}
}

func TestIOSEncryptionDeclarationRejectsEmptyComplianceCode(t *testing.T) {
	for _, info := range []map[string]any{
		{"ITSAppUsesNonExemptEncryption": true},
		{"ITSAppUsesNonExemptEncryption": true, "ITSEncryptionExportComplianceCode": ""},
		{"ITSAppUsesNonExemptEncryption": true, "ITSEncryptionExportComplianceCode": " \n"},
		{"ITSAppUsesNonExemptEncryption": true, "ITSEncryptionExportComplianceCode": 123},
		{"ITSAppUsesNonExemptEncryption": true, "ITSEncryptionExportComplianceCode": " CODE "},
		{"ITSAppUsesNonExemptEncryption": "false"},
		{"ITSAppUsesNonExemptEncryption": false, "ITSEncryptionExportComplianceCode": "CODE"},
		{"ITSEncryptionExportComplianceCode": "CODE"},
	} {
		if _, err := iosCheckEncryptionDeclaration(info); err == nil {
			t.Fatalf("invalid encryption declaration accepted: %#v", info)
		}
	}
	for _, info := range []map[string]any{
		nil,
		{"ITSAppUsesNonExemptEncryption": false},
		{"ITSAppUsesNonExemptEncryption": true, "ITSEncryptionExportComplianceCode": "APPROVED-CODE"},
	} {
		if detail, err := iosCheckEncryptionDeclaration(info); err != nil || detail == "" {
			t.Fatalf("valid declaration rejected: %s, %v", detail, err)
		}
	}
}

func TestIOSPurposeStringsCoverLinkedPermissionAPIs(t *testing.T) {
	imports := []string{"_AVMediaTypeVideo", "_AVMediaTypeAudio", "_OBJC_CLASS_$_PHPhotoLibrary", "_OBJC_CLASS_$_LAContext"}
	info := map[string]any{
		"NSCameraUsageDescription":          "Test camera authorization.",
		"NSMicrophoneUsageDescription":      "Test microphone authorization.",
		"NSPhotoLibraryUsageDescription":    "Test photo library authorization.",
		"NSPhotoLibraryAddUsageDescription": "Test add-only photo authorization.",
		"NSFaceIDUsageDescription":          "Test local authentication.",
	}
	if err := iosCheckPurposeStrings(info, imports); err != nil {
		t.Fatal(err)
	}
	for key, value := range info {
		delete(info, key)
		if err := iosCheckPurposeStrings(info, imports); err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("missing %s accepted: %v", key, err)
		}
		info[key] = value
	}
}

func TestIOSManualSigningDoesNotUseAutomaticProfiles(t *testing.T) {
	c := &Config{Identifier: "com.example.app", Version: "1.0.0", IOS: IOS{Signing: IOSSigning{
		Style: "manual", Identity: "Apple Distribution", ProvisioningProfile: "Archive Profile",
		ExportProfiles: map[string]string{"com.example.app": "Store Profile"}, ExportCertificate: "Apple Distribution",
	}}}
	if err := validateIOSPackaging(c, buildOptions{iosExportMethod: "app-store-connect", iosTeam: "TEAM"}); err != nil {
		t.Fatal(err)
	}
	project := iosProject(c, "15.0", "TEAM")
	for _, want := range []string{"CODE_SIGN_STYLE = Manual", `PROVISIONING_PROFILE_SPECIFIER = "Archive Profile"`, `CODE_SIGN_IDENTITY = "Apple Distribution"`} {
		if !strings.Contains(project, want) {
			t.Fatalf("missing setting %s", want)
		}
	}
	d, err := parseIOSPlist(iosExportOptions(c, "app-store-connect", "TEAM"))
	if err != nil {
		t.Fatal(err)
	}
	if d["destination"] != "export" || d["signingStyle"] != "manual" || d["signingCertificate"] != "Apple Distribution" || d["provisioningProfiles"].(map[string]any)[c.Identifier] != "Store Profile" {
		t.Fatalf("incorrect export settings: %#v", d)
	}
	c.IOS.Signing.Identity = ""
	if validateIOSPackaging(c, buildOptions{}) == nil {
		t.Fatal("manual signing accepted without an identity")
	}
}

func TestIOSBuildNumberOverrideValidation(t *testing.T) {
	c := &Config{Version: "1.2.3", IOS: IOS{BuildNumber: "1"}}
	for _, number := range []string{"42", "42.1", "42.1.2"} {
		if err := overrideIOSBuildNumber(c, number); err != nil || c.IOS.BuildNumber != number || c.Version != "1.2.3" {
			t.Fatalf("override %q: %v", number, err)
		}
	}
	for _, number := range []string{"next", "-1", "1.2.3.4", "1.", "42\n", "v42"} {
		before := c.IOS.BuildNumber
		if err := overrideIOSBuildNumber(c, number); err == nil || c.IOS.BuildNumber != before {
			t.Fatalf("invalid override changed configuration: %q", number)
		}
	}
}

func TestIOSToolingStaysLocal(t *testing.T) {
	for _, command := range []string{"upload", "testflight", "build-number", "distribute", "status"} {
		if err := runIOS([]string{command}); err == nil || !strings.Contains(err.Error(), "unknown iOS command") {
			t.Fatalf("unexpected release integration %q: %v", command, err)
		}
	}
	for _, args := range [][]string{
		{"-platform", "ios/arm64", "-ios-build-number", "next", "../../examples/ios-native"},
		{"-platform", "ios/arm64", "-upload", "../../examples/ios-native"},
		{"-platform", "darwin/arm64", "-ios-build-number", "42", "../../examples/ios-native"},
	} {
		if err := runBuild(args); err == nil {
			t.Fatalf("invalid packaging flags accepted: %v", args)
		}
	}
}
