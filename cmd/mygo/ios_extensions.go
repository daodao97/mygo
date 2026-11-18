package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
)

// App extensions (ios.extensions) are bundles the app builds apart, as a
// Network Extension's packet tunnel built with Xcode. The CLI copies each
// into the app's PlugIns/ and signs it, before the app, with its own
// provisioning profile, embedded in it, and that profile's entitlements.

// iosExtension is an extension of ios.extensions, as the app carries it.
type iosExtension struct {
	src    string // the .appex
	name   string // its directory in PlugIns
	srcID  string // the identifier it was built with
	id     string // its identifier in the app, which a development app's changes
	config IOSExtension
	prof   *profile // what signs it for devices
}

// iosExtensions reads the extensions of c's ios.extensions for the app dc
// bundles, c's or its development app.
func iosExtensions(c, dc *Config) ([]*iosExtension, error) {
	var out []*iosExtension
	for i, e := range c.IOS.Extensions {
		src := c.path(e.Path)
		raw, err := os.ReadFile(filepath.Join(src, "Info.plist"))
		if err != nil {
			return nil, fmt.Errorf("ios.extensions[%d]: %s is not a built app extension: %v", i, e.Path, err)
		}
		js, err := plistToJSON(raw)
		if err != nil {
			return nil, fmt.Errorf("ios.extensions[%d]: %s's Info.plist: %v", i, e.Path, err)
		}
		var info struct{ CFBundleIdentifier string }
		if err := json.Unmarshal(js, &info); err != nil {
			return nil, fmt.Errorf("ios.extensions[%d]: %s's Info.plist: %v", i, e.Path, err)
		}
		id, err := extensionIdentifier(c.Identifier, dc.Identifier, info.CFBundleIdentifier)
		if err != nil {
			return nil, fmt.Errorf("ios.extensions[%d]: %s: %v", i, e.Path, err)
		}
		out = append(out, &iosExtension{src: src, name: filepath.Base(src), srcID: info.CFBundleIdentifier, id: id, config: e})
	}
	return out, nil
}

// extensionIdentifier is the identifier in the app bundled as appID, of an
// extension built as id for the app identifier: iOS installs only the
// extensions whose identifiers start with their app's, so the development
// app, of an identifier of its own, carries them under it.
func extensionIdentifier(identifier, appID, id string) (string, error) {
	suffix, ok := strings.CutPrefix(id, identifier+".")
	if !ok || suffix == "" {
		return "", fmt.Errorf("its identifier %q does not start with %s, the app's and a dot", id, identifier+".")
	}
	return appID + "." + suffix, nil
}

// embedExtensions copies the extensions into the bundle's PlugIns.
func embedExtensions(bundle string, exts []*iosExtension) error {
	for _, e := range exts {
		dst := filepath.Join(bundle, "PlugIns", e.name)
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := copyResource(e.src, dst); err != nil {
			return err
		}
		if e.id != e.srcID {
			if err := runQuiet("plutil", "-replace", "CFBundleIdentifier", "-string", e.id, filepath.Join(dst, "Info.plist")); err != nil {
				return err
			}
		}
	}
	return nil
}

// pickExtensionProfiles finds the provisioning profile of each extension
// for dist, as pickProfile does the app's, with the certificate of app,
// the app's profile: the bundle has one signing identity.
func pickExtensionProfiles(c *Config, exts []*iosExtension, dist, device string, app *profile) error {
	ids := map[string]string{app.identity: app.identityName}
	for i, e := range exts {
		want := profileWant{team: c.IOS.Team, identifier: e.id, dist: dist, device: device}
		if dist == c.IOS.Distribution {
			want.name = e.config.ProvisioningProfile // the release's, as ios.provisioningProfile
		}
		p, err := findProfile(c, want, ids, fmt.Sprintf("ios.extensions[%d].provisioningProfile", i))
		if err != nil {
			return fmt.Errorf("%s: %v", e.name, err)
		}
		e.prof = p
	}
	return nil
}

// extensionEntitlements are those an extension is signed with: the
// profile's, whose wildcard app ID becomes the extension's identifier id.
func (p *profile) extensionEntitlements(id string) map[string]any {
	ent := maps.Clone(p.entitlements)
	if ent == nil {
		ent = map[string]any{}
	}
	ent["application-identifier"] = p.appIDPrefix() + "." + id
	// A profile's domains are what it allows, "*", not what to claim.
	delete(ent, associatedDomainsKey)
	return ent
}

// signExtensions signs the extensions in the bundle's PlugIns for devices,
// each with its profile embedded and its entitlements, after its own
// frameworks.
func signExtensions(work, bundle string, exts []*iosExtension) error {
	for _, e := range exts {
		dir := filepath.Join(bundle, "PlugIns", e.name)
		data, err := os.ReadFile(e.prof.path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "embedded.mobileprovision"), data, 0o644); err != nil {
			return err
		}
		ent := filepath.Join(work, "entitlements-"+strings.TrimSuffix(e.name, ".appex")+".plist")
		if err := os.WriteFile(ent, plistBytes(e.prof.extensionEntitlements(e.id)), 0o644); err != nil {
			return err
		}
		if err := signFrameworks(dir, e.prof.identity); err != nil {
			return err
		}
		if err := iosCodesign("--force", "--sign", e.prof.identity, "--entitlements", ent, "--generate-entitlement-der", dir); err != nil {
			return err
		}
	}
	return nil
}
