package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/internal/nativegen"
)

// Phones build an app's native modules with it, from their sources: on
// iOS their Swift, with xcodebuild, into frameworks of the app; on Android
// their Kotlin, a Gradle library each, into the app's Gradle build. Phones
// take no libraries built elsewhere, as desktops do: iOS signs what it
// runs with the app, and Android's libraries are the app's own build.

// phoneModule is a native module that an app for a phone builds with it.
type phoneModule struct {
	m     *nativegen.Module
	local bool // outside the module cache: its directory may be written
	// deps are the modules it uses, which build before it.
	deps []*phoneModule
}

// phoneModules returns the native modules of the app for goos (ios,
// android) implemented in its language there, those a module uses before
// it. It regenerates their generated files for a debug build, and checks
// them for a release.
func (c *Config) phoneModules(goos string, debug bool) ([]*phoneModule, error) {
	dirs, err := appNativeModules(c, goos, "arm64")
	if err != nil {
		return nil, err
	}
	lang := map[string]string{"ios": "swift", "android": "kotlin"}[goos]
	var list []*phoneModule
	byDir := map[string]*phoneModule{}
	var add func(m *nativegen.Module) (*phoneModule, error)
	add = func(m *nativegen.Module) (*phoneModule, error) {
		if p, ok := byDir[m.Dir]; ok {
			return p, nil
		}
		p := &phoneModule{m: m, local: isLocalPackage(m.Dir)}
		byDir[m.Dir] = p
		for _, path := range slices.Sorted(maps.Keys(m.Deps)) {
			d, err := add(m.Deps[path])
			if err != nil {
				return nil, err
			}
			if d != nil {
				p.deps = append(p.deps, d)
			}
		}
		if moduleLang(m.Dir, goos, "") != lang {
			// Unsupported here: package native says so as it loads.
			byDir[m.Dir] = nil
			return nil, nil
		}
		list = append(list, p)
		return p, nil
	}
	for _, dir := range dirs {
		rel := relPathTo(c.root, dir)
		if debug {
			r, err := nativegen.Generate(dir)
			if err != nil {
				return nil, fmt.Errorf("native module %s:\n%w", rel, err)
			}
			for _, w := range r.Written {
				logf("Wrote %s", cyan(relPathTo(c.root, w)))
			}
		} else {
			stale, err := nativegen.Stale(dir)
			if err != nil {
				return nil, fmt.Errorf("native module %s:\n%w", rel, err)
			}
			if len(stale) > 0 {
				return nil, fmt.Errorf("the generated files of native module %s are older than its interfaces (%s): regenerate them with mygo dev, or mygo native check %s, and commit them", rel, relPathTo(c.root, stale[0]), rel)
			}
		}
		m, err := nativegen.Load(dir)
		if err != nil {
			return nil, fmt.Errorf("native module %s:\n%w", rel, err)
		}
		if _, err := add(m); err != nil {
			return nil, err
		}
	}
	return list, nil
}

// iosModules builds the Swift of the app's native modules for t with
// xcodebuild, which builds Swift packages for iOS as Xcode does (with
// their dependencies, binary frameworks and resources), and puts their
// frameworks into the bundle's Frameworks, where package native loads
// them, and their resource bundles into the bundle.
func (t *iosToolchain) iosModules(ctx context.Context, c *Config, tk *task, work, bundle string, debug bool) error {
	mods, err := c.phoneModules("ios", debug)
	if err != nil || len(mods) == 0 {
		return err
	}
	if _, err := exec.LookPath("xcodebuild"); err != nil {
		return errors.New("the app's native modules are Swift packages, which xcodebuild builds for iOS: install Xcode")
	}
	config := "Release"
	if debug {
		config = "Debug"
	}
	dest := "generic/platform=iOS"
	if t.simulator {
		dest = "generic/platform=iOS Simulator"
	}
	frameworks := filepath.Join(bundle, "Frameworks")
	if err := os.MkdirAll(frameworks, 0o755); err != nil {
		return err
	}
	products := map[*phoneModule]string{}
	for _, p := range mods {
		name := p.m.Name
		tk.set("building the Swift of native module " + p.m.Package)
		derived := filepath.Join(work, "modules", name)
		src := filepath.Join(p.m.Dir, "swift")
		if !p.local {
			// The module cache is read-only, and xcodebuild writes into the
			// package's directory.
			src = filepath.Join(derived, "src")
			if err := copyPath(filepath.Join(p.m.Dir, "swift"), src); err != nil {
				return err
			}
			if err := makeWritable(src); err != nil {
				return err
			}
		}
		out := filepath.Join(derived, "Build", "Products", config+"-"+t.sdkName)
		os.RemoveAll(out)
		args := []string{"-scheme", name, "-destination", dest, "-derivedDataPath", derived, "-configuration", config,
			"ARCHS=arm64", "ONLY_ACTIVE_ARCH=NO", "IPHONEOS_DEPLOYMENT_TARGET=" + t.minimumVersion,
			"-skipPackagePluginValidation", "-skipMacroValidation", "-quiet"}
		// The modules it uses, built before it: their Swift modules and
		// frameworks.
		var swiftFlags, ldFlags []string
		for _, d := range p.deps {
			dp := products[d]
			swiftFlags = append(swiftFlags, "-I", dp)
			ldFlags = append(ldFlags, "-F", dp, "-F", filepath.Join(dp, "PackageFrameworks"), "-framework", d.m.Name)
		}
		if len(swiftFlags) > 0 {
			args = append(args, "OTHER_SWIFT_FLAGS=$(inherited) "+strings.Join(swiftFlags, " "), "OTHER_LDFLAGS=$(inherited) "+strings.Join(ldFlags, " "))
		}
		args = append(args, "build")
		cmd := exec.CommandContext(ctx, "xcodebuild", args...)
		cmd.Dir = src
		var log bytes.Buffer
		cmd.Stdout, cmd.Stderr = &log, &log
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("xcodebuild of native module %s for iOS: %v\n%s", p.m.Package, err, swiftProblems(log.String()))
		}
		products[p] = out
		found := false
		for _, dir := range []string{out, filepath.Join(out, "PackageFrameworks")} {
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				path := filepath.Join(dir, e.Name())
				switch filepath.Ext(e.Name()) {
				case ".framework":
					if e.Name() == name+".framework" {
						found = true
					}
					if err := copyPath(path, filepath.Join(frameworks, e.Name())); err != nil {
						return err
					}
				case ".bundle":
					if err := copyPath(path, filepath.Join(bundle, e.Name())); err != nil {
						return err
					}
				}
			}
		}
		if !found {
			return fmt.Errorf("xcodebuild made no %s.framework of native module %s: keep its product %s a dynamic library in Package.swift", name, p.m.Package, name)
		}
	}
	return nil
}

// makeWritable lets the files under dir, copied from the module cache, be
// written.
func makeWritable(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return err
		}
		return os.Chmod(path, info.Mode().Perm()|0o200)
	})
}

// androidModules returns the Gradle property that adds the Kotlin of the
// app's native modules to its build (mygoNativeModules): for each its
// project's name, its directory and the projects of the modules it uses.
func androidModules(mods []*phoneModule) string {
	names := map[*phoneModule]string{}
	used := map[string]bool{}
	var entries []string
	for _, p := range mods {
		name := "native-" + strings.ReplaceAll(p.m.Package, "_", "-")
		for i := 2; used[name]; i++ {
			name = fmt.Sprintf("native-%s-%d", strings.ReplaceAll(p.m.Package, "_", "-"), i)
		}
		used[name] = true
		names[p] = name
		var deps []string
		for _, d := range p.deps {
			deps = append(deps, names[d])
		}
		entries = append(entries, name+"|"+filepath.ToSlash(filepath.Join(p.m.Dir, "kotlin"))+"|"+strings.Join(deps, " "))
	}
	return strings.Join(entries, ",")
}

// hashModules hashes the Kotlin of the modules, which is in the APK, not
// in what only Go changes.
func hashModules(h io.Writer, mods []*phoneModule) error {
	for _, p := range mods {
		fmt.Fprintf(h, "%s\x00", p.m.Dir)
		if err := hashIfExists(h, filepath.Join(p.m.Dir, "kotlin")); err != nil {
			return err
		}
	}
	return nil
}
