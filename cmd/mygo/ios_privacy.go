package main

import (
	"fmt"
	"slices"
	"strings"
)

// Apple required-reason categories/reasons. Update with the current Apple list:
// https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype
var iosPrivacyReasons = map[string][]string{
	"NSPrivacyAccessedAPICategoryFileTimestamp":   {"DDA9.1", "C617.1", "3B52.1", "0A2A.1"},
	"NSPrivacyAccessedAPICategorySystemBootTime":  {"35F9.1", "8FFB.1", "3D61.1"},
	"NSPrivacyAccessedAPICategoryDiskSpace":       {"85F4.1", "E174.1", "7D9E.1", "B728.1"},
	"NSPrivacyAccessedAPICategoryActiveKeyboards": {"3EC4.1", "54BD.1"},
	"NSPrivacyAccessedAPICategoryUserDefaults":    {"CA92.1", "1C8F.1", "C56D.1", "AC6B.1"},
}

// https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacycollecteddatatypes/nsprivacycollecteddatatype
var iosPrivacyDataTypes = strings.Fields(`Name EmailAddress PhoneNumber PhysicalAddress OtherUserContactInfo
Health Fitness PaymentInfo CreditInfo OtherFinancialInfo PreciseLocation CoarseLocation SensitiveInfo Contacts
EmailsOrTextMessages PhotosorVideos AudioData GameplayContent CustomerSupport OtherUserContent BrowsingHistory
SearchHistory UserID DeviceID PurchaseHistory ProductInteraction AdvertisingData OtherUsageData CrashData
PerformanceData OtherDiagnosticData EnvironmentScanning Hands Head OtherDataTypes`)

func iosPrivacyManifest(c *Config) ([]byte, error) {
	p := c.IOS.Privacy
	// Go runtime measures intervals with mach_absolute_time. State/resources
	// inspect files in the sandbox; coordinated document imports inspect only
	// files the user selected. Do not add unrelated reasons or data collection.
	parts := []map[string]any{{
		"NSPrivacyAccessedAPITypes": []any{
			map[string]any{"NSPrivacyAccessedAPIType": "NSPrivacyAccessedAPICategorySystemBootTime", "NSPrivacyAccessedAPITypeReasons": []any{"35F9.1"}},
			map[string]any{"NSPrivacyAccessedAPIType": "NSPrivacyAccessedAPICategoryFileTimestamp", "NSPrivacyAccessedAPITypeReasons": []any{"C617.1", "3B52.1"}},
		},
	}}
	app := map[string]any{"NSPrivacyTracking": p.Tracking, "NSPrivacyTrackingDomains": stringValues(p.TrackingDomains)}
	apis, collected := []any{}, []any{}
	for _, a := range p.AccessedAPIs {
		apis = append(apis, map[string]any{"NSPrivacyAccessedAPIType": a.Category, "NSPrivacyAccessedAPITypeReasons": stringValues(a.Reasons)})
	}
	for _, d := range p.CollectedData {
		collected = append(collected, map[string]any{"NSPrivacyCollectedDataType": d.Type, "NSPrivacyCollectedDataTypeLinked": d.Linked,
			"NSPrivacyCollectedDataTypeTracking": d.Tracking, "NSPrivacyCollectedDataTypePurposes": stringValues(d.Purposes)})
	}
	app["NSPrivacyAccessedAPITypes"], app["NSPrivacyCollectedDataTypes"] = apis, collected
	parts = append(parts, app)
	for _, path := range p.Manifests {
		d, err := readIOSPlist(c.path(path))
		if err != nil {
			return nil, fmt.Errorf("ios.privacy.manifests: %w", err)
		}
		parts = append(parts, d)
	}
	d, err := mergeIOSPrivacy(parts)
	if err != nil {
		return nil, err
	}
	return iosPropertyList(d), nil
}

func stringValues(values []string) []any {
	a := make([]any, len(values))
	for i, v := range values {
		a[i] = v
	}
	return a
}

func privacyStrings(v any, key string, required bool) ([]string, error) {
	if v == nil && !required {
		return nil, nil
	}
	a, ok := v.([]any)
	if !ok || (required && len(a) == 0) {
		return nil, fmt.Errorf("%s must be a string array (nonempty required: %t)", key, required)
	}
	var result []string
	for _, item := range a {
		s, ok := item.(string)
		if !ok || strings.TrimSpace(s) == "" || strings.TrimSpace(s) != s {
			return nil, fmt.Errorf("%s contains an invalid string", key)
		}
		result = append(result, s)
	}
	slices.Sort(result)
	return slices.Compact(result), nil
}

func privacyKeys(row map[string]any, keys ...string) error {
	for key := range row {
		if !slices.Contains(keys, key) {
			return fmt.Errorf("unknown privacy entry key %q", key)
		}
	}
	return nil
}

func privacyArray(d map[string]any, key string) ([]any, error) {
	if d[key] == nil {
		return nil, nil
	}
	a, ok := d[key].([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", key)
	}
	return a, nil
}

func privacyBool(d map[string]any, key string) (bool, error) {
	if d[key] == nil {
		return false, nil
	}
	b, ok := d[key].(bool)
	if !ok {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return b, nil
}

func mergeIOSPrivacy(parts []map[string]any) (map[string]any, error) {
	apis := map[string][]string{}
	data := map[string]map[string]any{}
	var domains []string
	tracking := false
	for _, part := range parts {
		for key := range part {
			if !slices.Contains([]string{"NSPrivacyTracking", "NSPrivacyTrackingDomains", "NSPrivacyAccessedAPITypes", "NSPrivacyCollectedDataTypes"}, key) {
				return nil, fmt.Errorf("unknown privacy manifest key %q", key)
			}
		}
		flag, err := privacyBool(part, "NSPrivacyTracking")
		if err != nil {
			return nil, err
		}
		d, err := privacyStrings(part["NSPrivacyTrackingDomains"], "NSPrivacyTrackingDomains", false)
		if err != nil {
			return nil, err
		}
		if len(d) > 0 && !flag {
			return nil, fmt.Errorf("tracking domains require NSPrivacyTracking=true")
		}
		for _, name := range d {
			if strings.ContainsAny(name, "/: \t\r\n") {
				return nil, fmt.Errorf("tracking domain must be a domain name: %q", name)
			}
		}
		tracking, domains = tracking || flag, append(domains, d...)
		a, err := privacyArray(part, "NSPrivacyAccessedAPITypes")
		if err != nil {
			return nil, err
		}
		for _, item := range a {
			row, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("privacy API entry must be a dictionary")
			}
			if err := privacyKeys(row, "NSPrivacyAccessedAPIType", "NSPrivacyAccessedAPITypeReasons"); err != nil {
				return nil, err
			}
			category, _ := row["NSPrivacyAccessedAPIType"].(string)
			reasons, err := privacyStrings(row["NSPrivacyAccessedAPITypeReasons"], "NSPrivacyAccessedAPITypeReasons", true)
			if err != nil {
				return nil, err
			}
			for _, reason := range reasons {
				if !slices.Contains(iosPrivacyReasons[category], reason) {
					return nil, fmt.Errorf("unsupported required reason %q for %q; check the current Apple reason list", reason, category)
				}
			}
			apis[category] = append(apis[category], reasons...)
		}
		a, err = privacyArray(part, "NSPrivacyCollectedDataTypes")
		if err != nil {
			return nil, err
		}
		for _, item := range a {
			row, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("privacy data entry must be a dictionary")
			}
			if err := privacyKeys(row, "NSPrivacyCollectedDataType", "NSPrivacyCollectedDataTypeLinked", "NSPrivacyCollectedDataTypeTracking", "NSPrivacyCollectedDataTypePurposes"); err != nil {
				return nil, err
			}
			if row["NSPrivacyCollectedDataTypeLinked"] == nil || row["NSPrivacyCollectedDataTypeTracking"] == nil {
				return nil, fmt.Errorf("collected-data entries must declare linked and tracking booleans")
			}
			name, _ := row["NSPrivacyCollectedDataType"].(string)
			kind, validPrefix := strings.CutPrefix(name, "NSPrivacyCollectedDataType")
			if !validPrefix || !slices.Contains(iosPrivacyDataTypes, kind) {
				return nil, fmt.Errorf("invalid privacy data type %q", name)
			}
			purposes, err := privacyStrings(row["NSPrivacyCollectedDataTypePurposes"], "NSPrivacyCollectedDataTypePurposes", true)
			if err != nil {
				return nil, err
			}
			for _, purpose := range purposes {
				if !slices.Contains([]string{"NSPrivacyCollectedDataTypePurposeThirdPartyAdvertising", "NSPrivacyCollectedDataTypePurposeDeveloperAdvertising", "NSPrivacyCollectedDataTypePurposeAnalytics", "NSPrivacyCollectedDataTypePurposeProductPersonalization", "NSPrivacyCollectedDataTypePurposeAppFunctionality", "NSPrivacyCollectedDataTypePurposeOther"}, purpose) {
					return nil, fmt.Errorf("invalid privacy purpose %q", purpose)
				}
			}
			linked, err := privacyBool(row, "NSPrivacyCollectedDataTypeLinked")
			if err != nil {
				return nil, err
			}
			tracked, err := privacyBool(row, "NSPrivacyCollectedDataTypeTracking")
			if err != nil {
				return nil, err
			}
			if tracked && !flag {
				return nil, fmt.Errorf("tracking data requires NSPrivacyTracking=true")
			}
			if old := data[name]; old != nil {
				linked, tracked = linked || old["NSPrivacyCollectedDataTypeLinked"].(bool), tracked || old["NSPrivacyCollectedDataTypeTracking"].(bool)
				previous, _ := privacyStrings(old["NSPrivacyCollectedDataTypePurposes"], "purposes", true)
				purposes = append(purposes, previous...)
			}
			slices.Sort(purposes)
			data[name] = map[string]any{"NSPrivacyCollectedDataType": name, "NSPrivacyCollectedDataTypeLinked": linked,
				"NSPrivacyCollectedDataTypeTracking": tracked, "NSPrivacyCollectedDataTypePurposes": stringValues(slices.Compact(purposes))}
		}
	}
	apiRows, dataRows := []any{}, []any{}
	var categories, names []string
	for category := range apis {
		categories = append(categories, category)
	}
	for name := range data {
		names = append(names, name)
	}
	slices.Sort(categories)
	slices.Sort(names)
	slices.Sort(domains)
	for _, category := range categories {
		reasons := apis[category]
		slices.Sort(reasons)
		apiRows = append(apiRows, map[string]any{"NSPrivacyAccessedAPIType": category, "NSPrivacyAccessedAPITypeReasons": stringValues(slices.Compact(reasons))})
	}
	for _, name := range names {
		dataRows = append(dataRows, data[name])
	}
	return map[string]any{"NSPrivacyTracking": tracking, "NSPrivacyTrackingDomains": stringValues(slices.Compact(domains)),
		"NSPrivacyAccessedAPITypes": apiRows, "NSPrivacyCollectedDataTypes": dataRows}, nil
}
