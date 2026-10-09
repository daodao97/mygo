package main

import (
	"archive/zip"
	"debug/macho"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type iosCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

type iosCheckReport struct {
	Artifact     string             `json:"artifact"`
	BundleID     string             `json:"bundleID"`
	Version      string             `json:"version"`
	BuildNumber  string             `json:"buildNumber"`
	Distribution bool               `json:"distribution"`
	Symbols      *iosSymbolEvidence `json:"symbols,omitempty"`
	Checks       []iosCheck         `json:"checks"`
}

func iosWriteCheckReport(app, symbols, out, name string, unsigned, distribution bool) error {
	report, err := iosCheckApp(app, symbols, unsigned, distribution)
	report.Artifact = filepath.Base(app)
	data, encodeErr := json.MarshalIndent(report, "", "  ")
	if encodeErr != nil {
		return encodeErr
	}
	if writeErr := os.WriteFile(filepath.Join(out, name), append(data, '\n'), 0o644); writeErr != nil {
		return writeErr
	}
	if err != nil {
		return fmt.Errorf("iOS artifact preflight: %w", err)
	}
	return nil
}

func iosCheckApp(app, symbols string, unsigned, distribution bool) (iosCheckReport, error) {
	r := iosCheckReport{Artifact: app, Distribution: distribution}
	var failures []error
	check := func(name, detail string, err error) {
		if err != nil {
			detail = err.Error()
			failures = append(failures, fmt.Errorf("%s: %w", name, err))
		}
		r.Checks = append(r.Checks, iosCheck{Name: name, Passed: err == nil, Detail: detail})
	}
	info, err := readIOSPlist(filepath.Join(app, "Info.plist"))
	if err != nil {
		check("metadata", "", err)
		return r, errors.Join(failures...)
	}
	r.BundleID, _ = info["CFBundleIdentifier"].(string)
	r.Version, _ = info["CFBundleShortVersionString"].(string)
	r.BuildNumber, _ = info["CFBundleVersion"].(string)
	executable, _ := info["CFBundleExecutable"].(string)
	if executable == "" || filepath.Base(executable) != executable || executable == "." || executable == ".." {
		check("executable", "", fmt.Errorf("invalid bundle executable"))
		return r, errors.Join(failures...)
	}
	binary := filepath.Join(app, executable)
	metadataErr := validateIOSPackaging(&Config{Version: r.Version, IOS: IOS{BuildNumber: r.BuildNumber}}, buildOptions{iosArchive: true})
	if r.BundleID == "" || !strings.Contains(r.BundleID, ".") {
		metadataErr = fmt.Errorf("missing reverse-DNS bundle identifier")
	}
	check("metadata", r.BundleID+" "+r.Version+" ("+r.BuildNumber+")", metadataErr)
	file, err := macho.Open(binary)
	if err == nil {
		defer file.Close()
		if file.Cpu != macho.CpuArm64 {
			err = fmt.Errorf("expected arm64, got %v", file.Cpu)
		}
	}
	check("architecture", "arm64 Mach-O", err)
	var imports []string
	importErr := err
	if file != nil {
		imports, importErr = file.ImportedSymbols()
	}
	privacy, privacyErr := readIOSPlist(filepath.Join(app, "PrivacyInfo.xcprivacy"))
	if privacyErr == nil {
		_, privacyErr = mergeIOSPrivacy([]map[string]any{privacy})
	}
	if privacyErr == nil && file != nil {
		if importErr != nil {
			privacyErr = importErr
		} else {
			privacyErr = iosCheckPrivacyImports(privacy, imports)
		}
	}
	check("privacy", "valid manifest and declarations for detected required-reason imports", privacyErr)
	if unsigned {
		if distribution {
			check("signature", "", fmt.Errorf("App Store builds cannot be unsigned"))
		} else {
			check("signature", "unsigned build explicitly permitted", nil)
		}
	} else {
		check("signature", "strict codesign verification", command("codesign", "--verify", "--deep", "--strict", app))
	}
	if symbols != "" {
		e, err := iosVerifySymbols(binary, symbols)
		r.Symbols = &e
		check("symbols", e.Symbolized, err)
	} else if distribution {
		check("symbols", "", fmt.Errorf("provide the matching dSYM with -symbols"))
	}
	if distribution {
		purposeErr := importErr
		if purposeErr == nil {
			purposeErr = iosCheckPurposeStrings(info, imports)
		}
		check("purpose-strings", "nonempty purpose strings for detected protected-resource imports", purposeErr)
		deviceTarget := false
		if file != nil {
			for _, load := range file.Loads {
				raw := load.Raw()
				// LC_BUILD_VERSION / PLATFORM_IOS, from Mach-O loader.h.
				if len(raw) >= 12 && file.ByteOrder.Uint32(raw[:4]) == 0x32 && file.ByteOrder.Uint32(raw[8:12]) == 2 {
					deviceTarget = true
				}
			}
		}
		if !deviceTarget {
			check("device-target", "", fmt.Errorf("distribution requires an iOS device Mach-O, not a simulator binary"))
		} else {
			check("device-target", "iOS device build platform", nil)
		}
		_, icon := info["CFBundleIcons"]
		if !icon {
			check("icon", "", fmt.Errorf("missing compiled app icon"))
		} else {
			check("icon", "compiled app icon metadata", nil)
		}
		encryptionDetail, encryptionErr := iosCheckEncryptionDeclaration(info)
		check("encryption", encryptionDetail, encryptionErr)
		check("distribution-profile", "App Store profile, application identifier, expiry and distribution identity", iosCheckDistributionProfile(app, r.BundleID))
	}
	return r, errors.Join(failures...)
}

// Omitting the declaration deliberately leaves Apple's questionnaire in the
// app release flow. Declaring non-exempt encryption needs Apple's approved code;
// accepting YES alone would produce an IPA rejected for an empty code.
func iosCheckEncryptionDeclaration(info map[string]any) (string, error) {
	value, declared := info["ITSAppUsesNonExemptEncryption"]
	code, hasCode := info["ITSEncryptionExportComplianceCode"]
	if !declared {
		if hasCode {
			return "", fmt.Errorf("ITSEncryptionExportComplianceCode requires ITSAppUsesNonExemptEncryption=true")
		}
		return "encryption questionnaire deferred to App Store Connect; no exemption declared", nil
	}
	nonExempt, ok := value.(bool)
	if !ok {
		return "", fmt.Errorf("ITSAppUsesNonExemptEncryption must be a boolean")
	}
	if !nonExempt {
		if hasCode {
			return "", fmt.Errorf("an exemption declaration cannot include ITSEncryptionExportComplianceCode")
		}
		return "app declares exempt encryption; classification remains app-specific", nil
	}
	approved, ok := code.(string)
	if !ok || strings.TrimSpace(approved) == "" || approved != strings.TrimSpace(approved) {
		return "", fmt.Errorf("ITSAppUsesNonExemptEncryption=true requires Apple's approved ITSEncryptionExportComplianceCode; otherwise omit both keys and complete the App Store Connect questionnaire")
	}
	return "non-exempt encryption code present; match against Apple's approved documentation before upload", nil
}

// Apple's static validation also checks SDK references, even when the app does
// not request that resource at runtime. These imports identify MyGo's permission
// bridge APIs; other SDKs and localized purpose strings need their own review.
func iosCheckPurposeStrings(info map[string]any, imports []string) error {
	required := map[string]bool{}
	for _, symbol := range imports {
		switch strings.TrimPrefix(symbol, "_") {
		case "OBJC_CLASS_$_CLLocationManager":
			required["NSLocationWhenInUseUsageDescription"] = true
		case "AVMediaTypeVideo":
			required["NSCameraUsageDescription"] = true
		case "AVMediaTypeAudio":
			required["NSMicrophoneUsageDescription"] = true
		case "OBJC_CLASS_$_PHPhotoLibrary":
			required["NSPhotoLibraryUsageDescription"] = true
			required["NSPhotoLibraryAddUsageDescription"] = true
		case "OBJC_CLASS_$_LAContext":
			required["NSFaceIDUsageDescription"] = true
		}
	}
	var failures []error
	for _, key := range []string{"NSCameraUsageDescription", "NSMicrophoneUsageDescription", "NSLocationWhenInUseUsageDescription", "NSPhotoLibraryUsageDescription", "NSPhotoLibraryAddUsageDescription", "NSFaceIDUsageDescription"} {
		if !required[key] {
			continue
		}
		purpose, ok := info[key].(string)
		if !ok || strings.TrimSpace(purpose) == "" || len(purpose) >= 4000 {
			failures = append(failures, fmt.Errorf("%s requires a nonempty purpose string shorter than 4000 bytes in ios.infoPlist", key))
		}
	}
	return errors.Join(failures...)
}

func iosCheckPrivacyImports(privacy map[string]any, imports []string) error {
	declared := map[string]bool{}
	rows, _ := privacyArray(privacy, "NSPrivacyAccessedAPITypes")
	for _, row := range rows {
		a := row.(map[string]any)
		declared[a["NSPrivacyAccessedAPIType"].(string)] = true
	}
	for _, name := range imports {
		name = strings.Split(strings.TrimPrefix(name, "_"), "$")[0]
		category := ""
		switch name {
		case "mach_absolute_time":
			category = "SystemBootTime"
		case "stat", "stat64", "fstat", "fstat64", "fstatat", "lstat", "lstat64":
			category = "FileTimestamp"
		case "statfs", "statfs64", "statvfs", "fstatfs", "fstatfs64", "fstatvfs":
			category = "DiskSpace"
		}
		if category != "" && !declared["NSPrivacyAccessedAPICategory"+category] {
			return fmt.Errorf("import %s lacks required-reason category %s", name, category)
		}
	}
	return nil
}

func iosCheckDistributionProfile(app, bundleID string) error {
	out, err := exec.Command("security", "cms", "-D", "-i", filepath.Join(app, "embedded.mobileprovision")).Output()
	if err != nil {
		return fmt.Errorf("cannot decode embedded provisioning profile: %w", err)
	}
	profile, err := parseIOSPlist(out)
	if err != nil {
		return err
	}
	if _, ok := profile["ProvisionedDevices"]; ok {
		return fmt.Errorf("device-limited profile cannot be used for App Store Connect")
	}
	if profile["ProvisionsAllDevices"] == true {
		return fmt.Errorf("enterprise profile cannot be used for App Store Connect")
	}
	expires, _ := profile["ExpirationDate"].(string)
	date, err := time.Parse(time.RFC3339, expires)
	if err != nil || !date.After(time.Now()) {
		return fmt.Errorf("provisioning profile is expired or lacks an expiry date")
	}
	entitlements, _ := profile["Entitlements"].(map[string]any)
	appID, _ := entitlements["application-identifier"].(string)
	if !strings.HasSuffix(appID, "."+bundleID) || entitlements["get-task-allow"] == true || entitlements["beta-reports-active"] != true {
		return fmt.Errorf("profile does not authorize this app for App Store/TestFlight")
	}
	out, err = exec.Command("codesign", "-d", "--verbose=4", app).CombinedOutput()
	if err != nil {
		return fmt.Errorf("cannot inspect signing identity: %w", err)
	}
	if !strings.Contains(string(out), "Authority=Apple Distribution:") && !strings.Contains(string(out), "Authority=iPhone Distribution:") {
		return fmt.Errorf("app is not signed with an Apple distribution identity")
	}
	out, err = exec.Command("codesign", "-d", "--entitlements", ":-", app).Output()
	if err != nil {
		return fmt.Errorf("cannot read app signing entitlements: %w", err)
	}
	actual, err := parseIOSPlist(out)
	if err != nil {
		return err
	}
	if actual["application-identifier"] != appID || actual["get-task-allow"] == true || actual["beta-reports-active"] != true {
		return fmt.Errorf("signed app entitlements do not match the App Store profile")
	}
	if actual["com.apple.developer.team-identifier"] != entitlements["com.apple.developer.team-identifier"] {
		return fmt.Errorf("app/profile teams differ")
	}
	if push := actual["aps-environment"]; push != nil && (push != "production" || push != entitlements["aps-environment"]) {
		return fmt.Errorf("App Store push entitlement must match the production profile")
	}
	return nil
}

// extractIOSIPA rejects traversal, symlinks and unbounded expansion before
// writing anything outside its temporary verification directory.
func extractIOSIPA(path, destination string) (string, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer z.Close()
	var size uint64
	if len(z.File) > 100000 {
		return "", fmt.Errorf("IPA contains too many entries")
	}
	for _, f := range z.File {
		if !filepath.IsLocal(f.Name) || strings.Contains(f.Name, "\\") || f.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("unsafe IPA entry %q", f.Name)
		}
		if f.UncompressedSize64 > 2<<30 || size > 2<<30-f.UncompressedSize64 {
			return "", fmt.Errorf("IPA exceeds 2 GB expanded limit")
		}
		size += f.UncompressedSize64
		target := filepath.Join(destination, f.Name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}
		input, err := f.Open()
		if err != nil {
			return "", err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, f.Mode().Perm()&0o777)
		if err != nil {
			input.Close()
			return "", err
		}
		_, err = io.Copy(output, io.LimitReader(input, int64(f.UncompressedSize64)+1))
		closeErr := output.Close()
		input.Close()
		if err != nil || closeErr != nil {
			return "", errors.Join(err, closeErr)
		}
	}
	return iosSingleApp(filepath.Join(destination, "Payload"))
}

func iosSingleApp(dir string) (string, error) {
	apps, err := filepath.Glob(filepath.Join(dir, "*.app"))
	if err != nil {
		return "", err
	}
	if len(apps) != 1 {
		return "", fmt.Errorf("expected one app in %s, found %d", dir, len(apps))
	}
	return apps[0], nil
}

func runIOSCheck(args []string) error {
	flags := newFlags("ios check", "", "Verify an iOS app/archive/IPA, signatures, privacy and Go source symbols. Distribution checks require a matching dSYM and an App Store profile.")
	app := flags.String("app", "", "app bundle to check")
	archive := flags.String("archive", "", "Xcode archive to check")
	ipa := flags.String("ipa", "", "IPA to extract and check")
	symbols := flags.String("symbols", "", "matching app dSYM (automatic for archives)")
	distribution := flags.Bool("distribution", false, "require App Store/TestFlight signing and metadata")
	unsigned := flags.Bool("unsigned", false, "permit an explicitly unsigned build")
	jsonOutput := flags.Bool("json", false, "print a machine-readable report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("iOS artifact checks require macOS and Xcode")
	}
	count := 0
	for _, value := range []string{*app, *archive, *ipa} {
		if value != "" {
			count++
		}
	}
	if count != 1 || len(flags.Args()) != 0 {
		return fmt.Errorf("specify exactly one of -app, -archive, -ipa")
	}
	if *archive != "" {
		var err error
		*app, err = iosSingleApp(filepath.Join(*archive, "Products", "Applications"))
		if err != nil {
			return err
		}
		if *symbols == "" {
			*symbols = filepath.Join(*archive, "dSYMs", filepath.Base(*app)+".dSYM")
		}
	}
	if *ipa != "" {
		dir, err := os.MkdirTemp("", "mygo-ipa-check-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		*app, err = extractIOSIPA(*ipa, dir)
		if err != nil {
			return err
		}
	}
	report, err := iosCheckApp(*app, *symbols, *unsigned, *distribution)
	if *ipa != "" {
		report.Artifact = *ipa
	}
	if *archive != "" {
		report.Artifact = *archive
	}
	if *jsonOutput {
		if encodeErr := json.NewEncoder(os.Stdout).Encode(report); encodeErr != nil {
			return encodeErr
		}
	} else {
		for _, check := range report.Checks {
			mark := "PASS"
			if !check.Passed {
				mark = "FAIL"
			}
			fmt.Printf("%s %-22s %s\n", mark, check.Name, check.Detail)
		}
	}
	return err
}
