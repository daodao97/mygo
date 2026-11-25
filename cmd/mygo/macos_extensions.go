package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
)

// App extensions of macOS (macos.extensions) are bundles the app builds
// apart, as a share extension built with Xcode, signed with their own
// entitlements: macOS loads an app extension only in the App Sandbox,
// which the app itself need not be in. writeBundle copies each into the
// app's Contents/PlugIns, and codesign signs it before the app, with the
// app's identity and the entitlements it was signed with (appCodesigning).

// embedMacOSExtensions copies the extensions of c's macos.extensions into
// the PlugIns of contents, the bundle's Contents. The identifier in each
// one's Info.plist must start with the app's and a dot, as Apple asks of
// an app extension.
func embedMacOSExtensions(c *Config, contents string) error {
	for i, e := range c.MacOS.Extensions {
		src := c.path(e.Path)
		raw, err := os.ReadFile(filepath.Join(src, "Contents", "Info.plist"))
		if err != nil {
			return fmt.Errorf("macos.extensions[%d]: %s is not a built app extension: %v", i, e.Path, err)
		}
		info, err := readPlist(raw)
		if err != nil {
			return fmt.Errorf("macos.extensions[%d]: %s's Info.plist: %v", i, e.Path, err)
		}
		id, _ := info["CFBundleIdentifier"].(string)
		if _, err := extensionIdentifier(c.Identifier, c.Identifier, id); err != nil {
			return fmt.Errorf("macos.extensions[%d]: %s: %v", i, e.Path, err)
		}
		if err := copyResource(src, filepath.Join(contents, "PlugIns", filepath.Base(src))); err != nil {
			return err
		}
	}
	return nil
}

// readPlist reads a property list whose root is a dictionary: XML in Go,
// which builds off macOS need, and a binary one with plutil, which only
// macOS has.
func readPlist(raw []byte) (map[string]any, error) {
	if !bytes.HasPrefix(raw, []byte("bplist")) {
		return parsePlist(raw)
	}
	js, err := plistToJSON(raw)
	if errors.Is(err, exec.ErrNotFound) {
		return nil, errors.New("reading a binary property list needs macOS: convert it to XML with plutil -convert xml1")
	}
	if err != nil {
		return nil, err
	}
	var d map[string]any
	return d, json.Unmarshal(js, &d)
}

// appCodesigning returns the arguments of each codesign that signs the
// bundle app, in order, the app with the property list entitlements, if
// any.
//
// An app without app extensions is signed at once with --deep, which signs
// the code in Contents/Frameworks with the same options. --deep would sign
// the extensions in Contents/PlugIns with the app's entitlements, or none,
// in place of their own, and macOS does not load an extension without the
// App Sandbox. So an app with extensions is signed from the inside out:
// each item of Contents/Frameworks and Contents/PlugIns, with what it holds
// (--deep), keeping the entitlements it was signed with, then the app
// alone, whose signature seals them.
func appCodesigning(app, identity, entitlements string, production bool) ([][]string, error) {
	plugins, err := bundleItems(app, "PlugIns")
	if err != nil {
		return nil, err
	}
	if len(plugins) == 0 {
		return [][]string{codesignArgs(app, identity, entitlements, production, true)}, nil
	}
	frameworks, err := bundleItems(app, "Frameworks")
	if err != nil {
		return nil, err
	}
	var calls [][]string
	for _, path := range slices.Concat(frameworks, plugins) {
		calls = append(calls, append(nestedCodesignArgs(identity, "", production), "--deep", path))
	}
	return append(calls, codesignArgs(app, identity, entitlements, production, false)), nil
}

// bundleItems returns the paths of what the directory dir of the bundle
// app's Contents holds, sorted, or none without it.
func bundleItems(app, dir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(app, "Contents", dir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		paths = append(paths, filepath.Join(app, "Contents", dir, e.Name()))
	}
	return paths, nil
}
