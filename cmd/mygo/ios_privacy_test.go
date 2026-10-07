package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestIOSPrivacyMergePreservesFrameworkAndDependencyDeclarations(t *testing.T) {
	c := &Config{root: t.TempDir(), IOS: IOS{Privacy: IOSPrivacy{
		AccessedAPIs:  []IOSPrivacyAPI{{Category: "NSPrivacyAccessedAPICategoryFileTimestamp", Reasons: []string{"C617.1", "DDA9.1"}}},
		CollectedData: []IOSPrivacyData{{Type: "NSPrivacyCollectedDataTypeName", Purposes: []string{"NSPrivacyCollectedDataTypePurposeAppFunctionality"}}},
		Manifests:     []string{"dependency.xcprivacy"},
	}}}
	dep := map[string]any{"NSPrivacyAccessedAPITypes": []any{map[string]any{
		"NSPrivacyAccessedAPIType": "NSPrivacyAccessedAPICategoryUserDefaults", "NSPrivacyAccessedAPITypeReasons": []any{"CA92.1"},
	}}, "NSPrivacyCollectedDataTypes": []any{map[string]any{
		"NSPrivacyCollectedDataType": "NSPrivacyCollectedDataTypeName", "NSPrivacyCollectedDataTypeLinked": true,
		"NSPrivacyCollectedDataTypeTracking": false, "NSPrivacyCollectedDataTypePurposes": []any{"NSPrivacyCollectedDataTypePurposeAnalytics"},
	}}}
	if err := os.WriteFile(filepath.Join(c.root, "dependency.xcprivacy"), iosPropertyList(dep), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := iosPrivacyManifest(c)
	if err != nil {
		t.Fatal(err)
	}
	second, err := iosPrivacyManifest(c)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("privacy output is not reproducible", err)
	}
	d, err := parseIOSPlist(first)
	if err != nil {
		t.Fatal(err)
	}
	apis := d["NSPrivacyAccessedAPITypes"].([]any)
	got := map[string]any{}
	for _, row := range apis {
		a := row.(map[string]any)
		got[a["NSPrivacyAccessedAPIType"].(string)] = a["NSPrivacyAccessedAPITypeReasons"]
	}
	want := map[string]any{
		"NSPrivacyAccessedAPICategoryFileTimestamp":  []any{"3B52.1", "C617.1", "DDA9.1"},
		"NSPrivacyAccessedAPICategorySystemBootTime": []any{"35F9.1"},
		"NSPrivacyAccessedAPICategoryUserDefaults":   []any{"CA92.1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("API declarations: %#v", got)
	}
	data := d["NSPrivacyCollectedDataTypes"].([]any)
	if len(data) != 1 {
		t.Fatal("duplicate data type was not merged")
	}
	row := data[0].(map[string]any)
	if row["NSPrivacyCollectedDataTypeLinked"] != true || !reflect.DeepEqual(row["NSPrivacyCollectedDataTypePurposes"], []any{"NSPrivacyCollectedDataTypePurposeAnalytics", "NSPrivacyCollectedDataTypePurposeAppFunctionality"}) {
		t.Fatalf("collection declaration lost: %#v", row)
	}
	if d["NSPrivacyTracking"] != false {
		t.Fatal("framework introduced tracking")
	}
}

func TestIOSPrivacyRejectsInvalidDeclarations(t *testing.T) {
	for _, p := range []IOSPrivacy{
		{AccessedAPIs: []IOSPrivacyAPI{{Category: "NSPrivacyAccessedAPICategorySystemBootTime", Reasons: []string{"C617.1"}}}},
		{AccessedAPIs: []IOSPrivacyAPI{{Category: "NSPrivacyAccessedAPICategoryFileTimestamp"}}},
		{TrackingDomains: []string{"tracker.example.com"}},
		{Tracking: true, TrackingDomains: []string{"https://tracker.example.com"}},
		{CollectedData: []IOSPrivacyData{{Type: "NSPrivacyCollectedDataTypeName"}}},
		{CollectedData: []IOSPrivacyData{{Type: "NSPrivacyCollectedDataTypeName", Tracking: true, Purposes: []string{"NSPrivacyCollectedDataTypePurposeAnalytics"}}}},
	} {
		if _, err := iosPrivacyManifest(&Config{IOS: IOS{Privacy: p}}); err == nil {
			t.Fatalf("accepted invalid privacy declaration: %+v", p)
		}
	}
	if _, err := mergeIOSPrivacy([]map[string]any{{"NSPrivacyAccessedAPITypes": "not an array"}}); err == nil {
		t.Fatal("invalid property type accepted")
	}
	if _, err := mergeIOSPrivacy([]map[string]any{{"NSPrivacyTrackng": false}}); err == nil {
		t.Fatal("unknown key accepted")
	}
}

func TestIOSPlistRejectsAmbiguousAndMalformedInput(t *testing.T) {
	for _, input := range []string{
		`<plist><dict><key>x</key><true/><key>x</key><false/></dict></plist>`,
		`<plist><dict><key>x</key></dict></plist>`,
		`<plist><array/></plist>`,
		`<plist><dict/></plist><plist><dict/></plist>`,
		`<plist><dict><key>x</key><null/></dict></plist>`,
		`<plist><dict><key>x</key><true>yes</true></dict></plist>`,
	} {
		if _, err := parseIOSPlist([]byte(input)); err == nil {
			t.Fatalf("accepted malformed plist %s", input)
		}
	}
}
