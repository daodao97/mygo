package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// mobilePlugin is what a package tells the CLI of its native code for phones,
// in its mygo-plugin.json, beside the libraries it may name:
//
//	{"mobile": {
//		"id": "location",
//		"ios": {
//			"sources": ["ios/*.swift"], "class": "LocationPlugin",
//			"frameworks": ["CoreLocation"],
//			"infoPlist": {"NSLocationWhenInUseUsageDescription": "Shows where you are."}
//		},
//		"android": {
//			"sources": ["android/*.kt"], "class": "LocationPlugin",
//			"permissions": ["ACCESS_FINE_LOCATION"],
//			"dependencies": ["androidx.biometric:biometric:1.1.0"],
//			"application": ["<service android:name=\"...\" />"]
//		}
//	}}
//
// Native code with its own types, which Go calls as any other Go
// interface, is a native module instead (see docs/native-modules.md).
//
// The Swift compiles into the host's module, the Kotlin into its package
// dev.mygo, and they implement MyGoPlugin: Go reaches them by id, with
// mygo.Native. An app that does not import the package has none of it.
type mobilePlugin struct {
	ID      string         `json:"id"`
	IOS     *pluginIOS     `json:"ios"`
	Android *pluginAndroid `json:"android"`

	dir string
}

type pluginIOS struct {
	Sources    []string       `json:"sources"`
	Class      string         `json:"class"`
	Frameworks []string       `json:"frameworks"`
	InfoPlist  map[string]any `json:"infoPlist"`
	// Entitlements are what its code needs the app signed with: a simulator's
	// build has them as they are, and a device's from the provisioning profile,
	// which must have the capability.
	Entitlements map[string]any `json:"entitlements"`
}

type pluginAndroid struct {
	Sources      []string `json:"sources"`
	Class        string   `json:"class"`
	Permissions  []string `json:"permissions"`
	Dependencies []string `json:"dependencies"`
	// Application are XML fragments the manifest's <application> takes.
	Application []string `json:"application"`
}

// mobilePlugins are the plugins of an app, in the order of their ids.
type mobilePlugins []*mobilePlugin

var pluginName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

// mobilePlugins lists the native code of the packages the app imports, for
// goos: ios or android.
func (c *Config) mobilePlugins(goos string) (mobilePlugins, error) {
	dirs, err := c.packageDirs(goos, "arm64")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out mobilePlugins
	for _, dir := range dirs {
		data, err := os.ReadFile(filepath.Join(dir, pluginFile))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		var f struct {
			Mobile *mobilePlugin `json:"mobile"`
		}
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Join(dir, pluginFile), err)
		}
		p := f.Mobile
		if p == nil || seen[p.ID] {
			continue
		}
		if !pluginName.MatchString(p.ID) {
			return nil, fmt.Errorf("%s: mobile.id %q must be a name", filepath.Join(dir, pluginFile), p.ID)
		}
		if p.IOS != nil && !pluginName.MatchString(p.IOS.Class) {
			return nil, fmt.Errorf("%s: mobile.ios.class %q must be a name", filepath.Join(dir, pluginFile), p.IOS.Class)
		}
		if p.Android != nil && !pluginName.MatchString(p.Android.Class) {
			return nil, fmt.Errorf("%s: mobile.android.class %q must be a name", filepath.Join(dir, pluginFile), p.Android.Class)
		}
		seen[p.ID] = true
		p.dir = dir
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// listEnv is the environment that go list sees the packages of goos with:
// phones link with cgo, without which it stops at the main package.
func listEnv(goos, goarch string) []string {
	env := []string{"GOOS=" + goos, "GOARCH=" + goarch}
	if goos == "ios" || goos == "android" {
		env = append(env, "CGO_ENABLED=1")
	}
	return env
}

// sourcesOf returns the files of a plugin's patterns, within its directory.
func (p *mobilePlugin) sourcesOf(patterns []string) ([]string, error) {
	var out []string
	for _, pat := range patterns {
		if strings.Contains(pat, "..") || filepath.IsAbs(pat) {
			return nil, fmt.Errorf("plugin %s: %q is outside its package", p.ID, pat)
		}
		m, err := filepath.Glob(filepath.Join(p.dir, filepath.FromSlash(pat)))
		if err != nil {
			return nil, err
		}
		out = append(out, m...)
	}
	return out, nil
}

// writeSwift puts the plugins' Swift in the host's sources, and the registry
// that registers them.
func (ps mobilePlugins) writeSwift(dir string) error {
	var reg strings.Builder
	reg.WriteString("// Written by mygo build: the plugins the app's packages name.\nfunc registerPlugins() {\n")
	for _, p := range ps {
		if p.IOS == nil {
			continue
		}
		files, err := p.sourcesOf(p.IOS.Sources)
		if err != nil {
			return err
		}
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				return err
			}
			name := "Plugin_" + p.ID + "_" + filepath.Base(f)
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
				return err
			}
		}
		fmt.Fprintf(&reg, "    PluginHost.register(%q, %s())\n", p.ID, p.IOS.Class)
	}
	reg.WriteString("}\n")
	return os.WriteFile(filepath.Join(dir, "PluginRegistry.swift"), []byte(reg.String()), 0o644)
}

// writeKotlin puts the plugins' Kotlin in the host's sources, and the registry
// that registers them.
func (ps mobilePlugins) writeKotlin(dir string) error {
	gone, _ := filepath.Glob(filepath.Join(dir, "Plugin_*"))
	for _, f := range gone {
		os.Remove(f)
	}
	var reg strings.Builder
	reg.WriteString("// Written by mygo build: the plugins the app's packages name.\npackage dev.mygo\n\ninternal fun registerPlugins() {\n")
	for _, p := range ps {
		if p.Android == nil {
			continue
		}
		files, err := p.sourcesOf(p.Android.Sources)
		if err != nil {
			return err
		}
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				return err
			}
			name := "Plugin_" + p.ID + "_" + filepath.Base(f)
			if _, err := writeIfChanged(filepath.Join(dir, name), data); err != nil {
				return err
			}
		}
		fmt.Fprintf(&reg, "    PluginHost.register(%q, %s())\n", p.ID, p.Android.Class)
	}
	reg.WriteString("}\n")
	_, err := writeIfChanged(filepath.Join(dir, "PluginRegistry.kt"), []byte(reg.String()))
	return err
}

// entitlements are what the plugins need the app signed with.
func (ps mobilePlugins) entitlements() map[string]any {
	out := map[string]any{}
	for _, p := range ps {
		if p.IOS != nil {
			for k, v := range p.IOS.Entitlements {
				out[k] = v
			}
		}
	}
	return out
}

func (ps mobilePlugins) frameworks() []string {
	var out []string
	for _, p := range ps {
		if p.IOS != nil {
			for _, f := range p.IOS.Frameworks {
				if !slices.Contains(out, f) {
					out = append(out, f)
				}
			}
		}
	}
	return out
}

// expandIdentifier puts the app's identifier in v's strings.
func expandIdentifier(v any, identifier string) any {
	switch v := v.(type) {
	case string:
		return strings.ReplaceAll(v, "$(IDENTIFIER)", identifier)
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = expandIdentifier(x, identifier)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, x := range v {
			out[k] = expandIdentifier(x, identifier)
		}
		return out
	}
	return v
}

// infoPlist is what the plugins add to the app's Info.plist. Lists add up, as
// the background modes that several ask for. $(IDENTIFIER) in a string is the
// app's, for what the system wants prefixed with it.
func (ps mobilePlugins) infoPlist(identifier string) map[string]any {
	out := map[string]any{}
	for _, p := range ps {
		if p.IOS == nil {
			continue
		}
		for k, v := range p.IOS.InfoPlist {
			v = expandIdentifier(v, identifier)
			if list, ok := v.([]any); ok {
				if old, ok := out[k].([]any); ok {
					v = append(old, list...)
				}
			}
			out[k] = v
		}
	}
	return out
}

func (ps mobilePlugins) permissions() []string {
	var out []string
	for _, p := range ps {
		if p.Android != nil {
			out = append(out, p.Android.Permissions...)
		}
	}
	return out
}

func (ps mobilePlugins) dependencies() []string {
	var out []string
	for _, p := range ps {
		if p.Android != nil {
			for _, d := range p.Android.Dependencies {
				if !slices.Contains(out, d) {
					out = append(out, d)
				}
			}
		}
	}
	return out
}

func (ps mobilePlugins) manifestApplication() []string {
	var out []string
	for _, p := range ps {
		if p.Android != nil {
			out = append(out, p.Android.Application...)
		}
	}
	return out
}
