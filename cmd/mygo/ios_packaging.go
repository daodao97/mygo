package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

func iosBuildNumber(c *Config) string {
	if c.IOS.BuildNumber != "" {
		return c.IOS.BuildNumber
	}
	return c.Version
}

func overrideIOSBuildNumber(c *Config, number string) error {
	if number == "" {
		return nil
	}
	if !regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,2}$`).MatchString(number) {
		return fmt.Errorf("-ios-build-number must be numeric with up to three components")
	}
	c.IOS.BuildNumber = number
	return nil
}
func validateIOSPackaging(c *Config, o buildOptions) error {
	for _, capability := range c.IOS.Capabilities {
		switch capability {
		case "camera", "microphone", "geolocation", "photos", "biometrics":
		default:
			return fmt.Errorf("invalid ios.capabilities value %q", capability)
		}
	}
	s := c.IOS.Signing
	if s.Style != "" && s.Style != "automatic" && s.Style != "manual" {
		return fmt.Errorf("ios.signing.style must be automatic or manual")
	}
	if s.Style == "manual" && (s.Identity == "" || s.ProvisioningProfile == "") {
		return fmt.Errorf("manual iOS signing requires identity and provisioningProfile")
	}
	if s.Style != "manual" && (s.ProvisioningProfile != "" || len(s.ExportProfiles) > 0) {
		return fmt.Errorf("provisioning profiles require ios.signing.style=manual")
	}
	archive := o.iosArchive || o.iosExportMethod != ""
	if archive && (o.iosSimulator || o.iosDevice != "" || o.debug) {
		return fmt.Errorf("iOS archives require a Release device target without -debug, -ios-simulator or -ios-device")
	}
	if o.iosExportMethod != "" {
		switch o.iosExportMethod {
		case "debugging", "release-testing", "app-store-connect", "enterprise":
		default:
			return fmt.Errorf("invalid -ios-export-method %q", o.iosExportMethod)
		}
		if o.iosTeam == "" {
			return fmt.Errorf("iOS IPA export requires -ios-team or ios.developmentTeam")
		}
	}
	if archive && !regexp.MustCompile(`^\d+(\.\d+){0,2}$`).MatchString(iosBuildNumber(c)) {
		return fmt.Errorf("iOS archives require a numeric ios.buildNumber with up to three components")
	}
	if archive && !regexp.MustCompile(`^\d+(\.\d+){0,2}$`).MatchString(c.Version) {
		return fmt.Errorf("iOS archives require a numeric app version")
	}
	for _, d := range c.IOS.AssociatedDomains {
		service, domain, ok := strings.Cut(d, ":")
		if !ok || domain == "" || strings.ContainsAny(domain, "/ \t\n") {
			return fmt.Errorf("invalid ios.associatedDomains value %q", d)
		}
		switch service {
		case "applinks", "webcredentials", "activitycontinuation":
		default:
			return fmt.Errorf("invalid associated domain service %q", service)
		}
	}
	if len(c.IOS.AssociatedDomains) > 0 && c.IOS.Entitlements["com.apple.developer.associated-domains"] != nil {
		return fmt.Errorf("configure associated domains in ios.associatedDomains or ios.entitlements, not both")
	}
	if e := c.IOS.Entitlements["aps-environment"]; e != nil && e != "development" && e != "production" {
		return fmt.Errorf("aps-environment must be development or production")
	}
	return nil
}

// Explicit zeroes keep inherited compiler flags from enabling an undeclared API.
func iosCapabilityFlags(c *Config) string {
	enabled := make(map[string]bool, len(c.IOS.Capabilities))
	for _, capability := range c.IOS.Capabilities {
		enabled[capability] = true
	}
	var flags strings.Builder
	for _, capability := range []string{"camera", "microphone", "geolocation", "photos", "biometrics"} {
		value := "0"
		if enabled[capability] {
			value = "1"
		}
		fmt.Fprintf(&flags, " -DMYGO_IOS_%s=%s", strings.ToUpper(capability), value)
	}
	return flags.String()
}
func iosPropertyList(d map[string]any) []byte {
	var b bytes.Buffer
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<plist version=\"1.0\">\n")
	writePlistValue(&b, d, "")
	b.WriteString("</plist>\n")
	return b.Bytes()
}
func iosEntitlements(c *Config) []byte {
	d := map[string]any{}
	for k, v := range c.IOS.Entitlements {
		d[k] = v
	}
	if c.IOS.PushNotifications && d["aps-environment"] == nil {
		d["aps-environment"] = "development"
	}
	if len(c.IOS.AssociatedDomains) > 0 {
		d["com.apple.developer.associated-domains"] = c.IOS.AssociatedDomains
	}
	return iosPropertyList(d)
}
func iosExportOptions(c *Config, method, team string) []byte {
	d := map[string]any{"method": method, "teamID": team, "signingStyle": "automatic", "destination": "export", "manageAppVersionAndBuildNumber": false}
	s := c.IOS.Signing
	if s.Style == "manual" {
		d["signingStyle"] = "manual"
		profiles := map[string]any{c.Identifier: s.ProvisioningProfile}
		for bundle, profile := range s.ExportProfiles {
			profiles[bundle] = profile
		}
		d["provisioningProfiles"] = profiles
	}
	if s.ExportCertificate != "" {
		d["signingCertificate"] = s.ExportCertificate
	}
	return iosPropertyList(d)
}

// Each extension declares an imported UTI. An app does not claim ownership of
// somebody else's file format; matching extension/MIME tags identify it.
func iosDocumentTypes(c *Config, d map[string]any) {
	if len(c.FileAssociations) == 0 {
		return
	}
	var docs, types []any
	for i, a := range c.FileAssociations {
		role := a.Role
		if role == "" {
			role = "Editor"
		}
		var ids []string
		for j, ext := range a.Ext {
			id := iosStandardDocumentType(ext)
			if id == "" {
				id = fmt.Sprintf("%s.document-%d-%d", c.Identifier, i, j)
				tags := map[string]any{"public.filename-extension": ext}
				if a.MimeType != "" {
					tags["public.mime-type"] = a.MimeType
				}
				types = append(types, map[string]any{"UTTypeIdentifier": id, "UTTypeDescription": a.Name, "UTTypeConformsTo": []string{"public.data"}, "UTTypeTagSpecification": tags})
			}
			ids = append(ids, id)
		}

		docs = append(docs, map[string]any{"CFBundleTypeName": a.Name, "CFBundleTypeRole": role, "LSHandlerRank": "Alternate", "LSItemContentTypes": ids})
	}
	d["CFBundleDocumentTypes"] = docs
	if len(types) > 0 {
		d["UTImportedTypeDeclarations"] = types
	}
}

func iosStandardDocumentType(ext string) string {
	return map[string]string{"txt": "public.plain-text", "text": "public.plain-text", "json": "public.json", "xml": "public.xml", "html": "public.html", "htm": "public.html", "csv": "public.comma-separated-values-text", "pdf": "com.adobe.pdf", "png": "public.png", "jpg": "public.jpeg", "jpeg": "public.jpeg", "gif": "com.compuserve.gif", "heic": "public.heic", "svg": "public.svg-image", "rtf": "public.rtf", "zip": "public.zip-archive"}[strings.ToLower(ext)]
}
