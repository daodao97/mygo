package main

import (
	"os"
	"path/filepath"
)

// signForSimulator signs the bundle for a simulator, with no identity. Entitlements
// don't go in such a signature, which the simulator then refuses to launch: they're
// in the executable (simulatorEntitlements), as Xcode has them.
func signForSimulator(bundle string) error {
	if err := signFrameworks(bundle, "-"); err != nil {
		return err
	}
	return runQuiet("codesign", "--force", "--sign", "-", bundle)
}

// simulatorEntitlements returns the flags that link what the plugins need the app
// signed with into a simulator's executable.
func (ps mobilePlugins) simulatorEntitlements(work string) ([]string, error) {
	ent := ps.entitlements()
	if len(ent) == 0 {
		return nil, nil
	}
	path := filepath.Join(work, "simulator.entitlements")
	if err := os.WriteFile(path, plistBytes(ent), 0o644); err != nil {
		return nil, err
	}
	return []string{"-Xlinker", "-sectcreate", "-Xlinker", "__TEXT", "-Xlinker", "__entitlements", "-Xlinker", path}, nil
}

// signFrameworks signs the frameworks of the bundle, which iosModules
// built from the Swift of native modules.
func signFrameworks(bundle, identity string) error {
	dirs, err := filepath.Glob(filepath.Join(bundle, "Frameworks", "*.framework"))
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if err := runQuiet("codesign", "--force", "--sign", identity, dir); err != nil {
			return err
		}
	}
	return nil
}
