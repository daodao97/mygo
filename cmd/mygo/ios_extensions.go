package main

import (
	"debug/macho"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	iossupport "github.com/egoist/mygo/native/ios"
)

var iosTargetName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
var iosBundleName = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+$`)

func iosExtensionID(c *Config, e IOSExtension) string {
	if e.BundleIdentifier != "" {
		return e.BundleIdentifier
	}
	return c.Identifier + "." + e.Name
}
func iosMinimum(value string) string {
	if value == "" {
		return "15.0"
	}
	return value
}
func iosVersionParts(v string) ([3]int, error) {
	var parts [3]int
	if !regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`).MatchString(v) {
		return parts, fmt.Errorf("invalid iOS minimum version %q", v)
	}
	for i, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return parts, err
		}
		parts[i] = n
	}
	if parts[0] < 15 {
		return parts, fmt.Errorf("iOS 15.0 or later is required")
	}
	return parts, nil
}
func validateIOSNative(n IOSNative) error {
	for _, s := range n.Support {
		if s != "actions" && s != "shared-store" && s != "widgets" {
			return fmt.Errorf("unknown iOS native support %q", s)
		}
	}
	for _, s := range append(append([]string{}, n.SwiftConditions...), n.Frameworks...) {
		if !iosTargetName.MatchString(s) {
			return fmt.Errorf("invalid iOS native identifier %q", s)
		}
	}
	for _, paths := range [][]string{n.Sources, n.Resources, n.Libraries} {
		for _, p := range paths {
			if strings.TrimSpace(p) == "" || strings.ContainsAny(p, "\x00\r\n") {
				return fmt.Errorf("invalid native source/resource/library path")
			}
		}
	}
	return nil
}
func iosGroupEntitlements(groups []string, extra map[string]any) (map[string]any, error) {
	result := map[string]any{}
	for k, v := range extra {
		result[k] = v
	}
	if len(groups) > 0 {
		if result["com.apple.security.application-groups"] != nil {
			return nil, fmt.Errorf("configure appGroups or the application-groups entitlement, not both")
		}
		seen := map[string]bool{}
		for _, g := range groups {
			if !strings.HasPrefix(g, "group.") || !iosBundleName.MatchString(g) || seen[g] {
				return nil, fmt.Errorf("invalid or duplicate App Group %q", g)
			}
			seen[g] = true
		}
		result["com.apple.security.application-groups"] = groups
	}
	return result, nil
}
func validateIOSExtensions(c *Config) error {
	if err := validateIOSBackground(c); err != nil {
		return err
	}
	host, err := iosVersionParts(iosMinimum(c.IOS.MinimumSystemVersion))
	if err != nil {
		return err
	}
	if _, err = iosGroupEntitlements(c.IOS.AppGroups, c.IOS.Entitlements); err != nil {
		return err
	}
	if err = validateIOSNative(c.IOS.Native); err != nil {
		return err
	}
	names := map[string]bool{"mygoapp": true}
	ids := map[string]bool{strings.ToLower(c.Identifier): true}
	for _, e := range c.IOS.Extensions {
		id := iosExtensionID(c, e)
		if !iosTargetName.MatchString(e.Name) || names[strings.ToLower(e.Name)] {
			return fmt.Errorf("invalid or duplicate extension name %q", e.Name)
		}
		names[strings.ToLower(e.Name)] = true
		if !iosBundleName.MatchString(id) || !strings.HasPrefix(id, c.Identifier+".") || ids[strings.ToLower(id)] {
			return fmt.Errorf("extension %s needs a unique bundle identifier under %s", e.Name, c.Identifier)
		}
		ids[strings.ToLower(id)] = true
		if !iosBundleName.MatchString(e.ExtensionPoint) {
			return fmt.Errorf("extension %s needs an extensionPoint", e.Name)
		}
		min := e.MinimumSystemVersion
		if min == "" {
			min = iosMinimum(c.IOS.MinimumSystemVersion)
		}
		parts, err := iosVersionParts(min)
		if err != nil {
			return err
		}
		for i := range parts {
			if parts[i] > host[i] {
				break
			}
			if parts[i] < host[i] {
				return fmt.Errorf("extension %s minimum version is older than its host", e.Name)
			}
		}
		if e.Native.EntryPoint != "" {
			return fmt.Errorf("native.entryPoint is only supported on the host")
		}
		if err := validateIOSNative(e.Native); err != nil {
			return err
		}
		if _, err := iosGroupEntitlements(e.AppGroups, e.Entitlements); err != nil {
			return err
		}
		if c.IOS.Signing.Style == "manual" && c.IOS.Signing.ExportProfiles[id] == "" {
			return fmt.Errorf("manual signing requires ios.signing.exportProfiles[%q] for extension %s", id, e.Name)
		}
	}
	return nil
}

type iosNativeTarget struct {
	name, minimum, bundle, point           string
	native                                 IOSNative
	sources, resources, libraries, headers []string
	resourceTypes                          map[string]string
}

func iosPrepareNative(c *Config, host string, privacy []byte) ([]iosNativeTarget, error) {
	targets := []iosNativeTarget{{name: "MyGoApp", minimum: iosMinimum(c.IOS.MinimumSystemVersion), bundle: c.Identifier, native: c.IOS.Native}}
	for _, e := range c.IOS.Extensions {
		min := e.MinimumSystemVersion
		if min == "" {
			min = iosMinimum(c.IOS.MinimumSystemVersion)
		}
		targets = append(targets, iosNativeTarget{name: e.Name, minimum: min, bundle: iosExtensionID(c, e), point: e.ExtensionPoint, native: e.Native})
		dir := filepath.Join(host, "Extensions", e.Name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
		d := map[string]any{}
		for k, v := range e.InfoPlist {
			d[k] = v
		}
		d["CFBundleExecutable"] = e.Name
		d["CFBundleIdentifier"] = iosExtensionID(c, e)
		d["CFBundleName"] = e.Name
		d["CFBundlePackageType"] = "XPC!"
		d["CFBundleShortVersionString"] = c.Version
		d["CFBundleVersion"] = iosBuildNumber(c)
		ext := map[string]any{}
		if supplied, ok := d["NSExtension"].(map[string]any); ok {
			for k, v := range supplied {
				ext[k] = v
			}
		}
		ext["NSExtensionPointIdentifier"] = e.ExtensionPoint
		d["NSExtension"] = ext
		ent, err := iosGroupEntitlements(e.AppGroups, e.Entitlements)
		if err != nil {
			return nil, err
		}
		for name, data := range map[string][]byte{"Info.plist": iosPropertyList(d), "Target.entitlements": iosPropertyList(ent), "PrivacyInfo.xcprivacy": privacy} {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
				return nil, err
			}
		}
	}
	for i := range targets {
		t := &targets[i]
		base := filepath.Join("Native", t.name)
		t.resourceTypes = map[string]string{}
		for j, p := range t.native.Sources {
			rel := filepath.Join(base, fmt.Sprint(j), filepath.Base(p))
			dest := filepath.Join(host, rel)
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return nil, err
			}
			if err := iosCopyNative(c.path(p), dest); err != nil {
				return nil, fmt.Errorf("native source %s: %w", p, err)
			}
			err := filepath.WalkDir(dest, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				r, err := filepath.Rel(host, path)
				if err != nil {
					return err
				}
				switch strings.ToLower(filepath.Ext(path)) {
				case ".swift", ".m", ".mm", ".c", ".cc", ".cpp":
					t.sources = append(t.sources, r)
				case ".h", ".hpp":
					t.headers = append(t.headers, filepath.Dir(r))
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
		}
		for j, p := range t.native.Resources {
			rel := filepath.Join(base, "Resources", fmt.Sprint(j), filepath.Base(p))
			if err := os.MkdirAll(filepath.Dir(filepath.Join(host, rel)), 0755); err != nil {
				return nil, err
			}
			if err := iosCopyNative(c.path(p), filepath.Join(host, rel)); err != nil {
				return nil, err
			}
			t.resources = append(t.resources, rel)
			info, err := os.Stat(filepath.Join(host, rel))
			if err != nil {
				return nil, err
			}
			if info.IsDir() {
				t.resourceTypes[rel] = "folder"
			}
			if strings.HasSuffix(rel, ".xcassets") {
				t.resourceTypes[rel] = "folder.assetcatalog"
			}
			if strings.HasSuffix(rel, ".storyboard") {
				t.resourceTypes[rel] = "file.storyboard"
			}
			if strings.HasSuffix(rel, ".xib") {
				t.resourceTypes[rel] = "file.xib"
			}
		}
		for j, p := range t.native.Libraries {
			if strings.ToLower(filepath.Ext(p)) != ".a" {
				return nil, fmt.Errorf("native libraries must be static .a archives: %s", p)
			}
			rel := filepath.Join(base, "Libraries", fmt.Sprint(j), filepath.Base(p))
			if err := os.MkdirAll(filepath.Dir(filepath.Join(host, rel)), 0755); err != nil {
				return nil, err
			}
			if err := iosCopyNative(c.path(p), filepath.Join(host, rel)); err != nil {
				return nil, err
			}
			t.libraries = append(t.libraries, rel)
		}
		support := map[string]string{"actions": "MyGoActions.swift", "shared-store": "MyGoSharedStore.swift", "widgets": "MyGoWidgets.swift"}
		seen := map[string]bool{}
		for _, s := range t.native.Support {
			if seen[s] {
				continue
			}
			seen[s] = true
			data, err := iossupport.Sources.ReadFile(support[s])
			if err != nil {
				return nil, err
			}
			rel := filepath.Join(base, support[s])
			if err := os.MkdirAll(filepath.Dir(filepath.Join(host, rel)), 0755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(filepath.Join(host, rel), data, 0644); err != nil {
				return nil, err
			}
			t.sources = append(t.sources, rel)
		}
		if t.name != "MyGoApp" {
			t.resources = append(t.resources, filepath.Join("Extensions", t.name, "PrivacyInfo.xcprivacy"))
		}
		sort.Strings(t.sources)
		sort.Strings(t.headers)
	}
	if c.IOS.Native.EntryPoint != "" {
		data, err := os.ReadFile(c.path(c.IOS.Native.EntryPoint))
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(host, "main.m"), data, 0644); err != nil {
			return nil, err
		}
	}
	if err := iosPrepareBackground(c, host, &targets[0]); err != nil {
		return nil, err
	}
	return targets, nil
}

// Extend the small single-target project without introducing an XcodeGen runtime
// dependency. All references are relative to its exported, self-contained host.
func iosNativeProject(base string, c *Config, team string, targets []iosNativeTarget) string {
	var objects strings.Builder
	next := 0
	id := func() string { next++; return fmt.Sprintf("B%023X", next) }
	obj := func(isa, body string) string {
		v := id()
		fmt.Fprintf(&objects, " %s = {isa = %s; %s };\n", v, isa, body)
		return v
	}
	var group, products, projectTargets, deps, embeds []string
	buildFiles := func(paths []string, types map[string]string) []string {
		var out []string
		for _, p := range paths {
			filetype := "text"
			switch strings.ToLower(filepath.Ext(p)) {
			case ".swift":
				filetype = "sourcecode.swift"
			case ".m":
				filetype = "sourcecode.c.objc"
			case ".mm":
				filetype = "sourcecode.cpp.objcpp"
			case ".c":
				filetype = "sourcecode.c.c"
			case ".cpp", ".cc":
				filetype = "sourcecode.cpp.cpp"
			}
			if kind := types[p]; kind != "" {
				filetype = kind
			}
			ref := obj("PBXFileReference", fmt.Sprintf("path = %s; sourceTree = \"<group>\"; lastKnownFileType = %s;", strconv.Quote(filepath.ToSlash(p)), filetype))
			group = append(group, ref)
			out = append(out, obj("PBXBuildFile", "fileRef = "+ref+";"))
		}
		return out
	}
	list := func(s []string) string { return strings.Join(s, ", ") }
	quoted := func(s []string) string {
		a := make([]string, len(s))
		for i, v := range s {
			a[i] = strconv.Quote(v)
		}
		return "(" + list(a) + ")"
	}
	for _, t := range targets {
		sources := buildFiles(t.sources, nil)
		resources := buildFiles(t.resources, t.resourceTypes)
		frameworks := append([]string{}, t.native.Frameworks...)
		for _, s := range t.native.Support {
			switch s {
			case "actions":
				frameworks = append(frameworks, "AppIntents")
			case "widgets":
				frameworks = append(frameworks, "WidgetKit")
			}
		}
		flags := []string{"$(inherited)"}
		for _, f := range frameworks {
			linkage := "-framework"
			version, _ := iosVersionParts(t.minimum)
			if f == "AppIntents" && version[0] < 16 {
				linkage = "-weak_framework"
			}
			flags = append(flags, linkage, f)
		}
		for _, l := range t.libraries {
			flags = append(flags, "-force_load", "$(PROJECT_DIR)/"+filepath.ToSlash(l))
		}
		flags = append(flags, t.native.LinkerFlags...)
		headers := []string{"$(inherited)"}
		for _, p := range t.headers {
			headers = append(headers, "$(PROJECT_DIR)/"+filepath.ToSlash(p))
		}
		settings := ` LD_RUNPATH_SEARCH_PATHS = ("$(inherited)", "@executable_path/Frameworks", "@executable_path/../../Frameworks");` + " SWIFT_VERSION = 5.0; CLANG_ENABLE_MODULES = YES; HEADER_SEARCH_PATHS = " + quoted(headers) + "; SWIFT_ACTIVE_COMPILATION_CONDITIONS = " + quoted(append([]string{"$(inherited)"}, t.native.SwiftConditions...)) + ";"
		if t.name == "MyGoApp" {
			embedSwift := false
			for _, target := range targets {
				for _, source := range target.sources {
					embedSwift = embedSwift || strings.HasSuffix(source, ".swift")
				}
			}
			if embedSwift {
				settings += " ALWAYS_EMBED_SWIFT_STANDARD_LIBRARIES = YES;"
			}

			base = strings.ReplaceAll(base, "PRODUCT_NAME = MyGoApp;", "PRODUCT_NAME = MyGoApp;"+settings)
			// Host flags already contain the platform frameworks and the Go archive.
			extra := strings.TrimSuffix(strings.TrimPrefix(quoted(flags[1:]), "("), ")")
			if extra != "" {
				base = strings.ReplaceAll(base, `"-lresolv");`, `"-lresolv", `+extra+`);`)
			}
			if len(sources) > 0 {
				base = strings.Replace(base, "files = (A00000000000000000000014)", "files = (A00000000000000000000014, "+list(sources)+")", 1)
			}
			if len(resources) > 0 {
				base = strings.Replace(base, "files = (A00000000000000000000015,", "files = ("+list(resources)+", A00000000000000000000015,", 1)
			}
			continue
		}
		sourcePhase := obj("PBXSourcesBuildPhase", "buildActionMask = 2147483647; files = ("+list(sources)+"); runOnlyForDeploymentPostprocessing = 0;")
		resourcePhase := obj("PBXResourcesBuildPhase", "buildActionMask = 2147483647; files = ("+list(resources)+"); runOnlyForDeploymentPostprocessing = 0;")
		product := obj("PBXFileReference", fmt.Sprintf("path = %s; sourceTree = BUILT_PRODUCTS_DIR; explicitFileType = \"wrapper.app-extension\";", strconv.Quote(t.name+".appex")))
		products = append(products, product)
		sign := "CODE_SIGN_STYLE = Automatic; CODE_SIGN_IDENTITY = \"Apple Development\";"
		if c.IOS.Signing.Style == "manual" {
			sign = "CODE_SIGN_STYLE = Manual; PROVISIONING_PROFILE_SPECIFIER = " + strconv.Quote(c.IOS.Signing.ExportProfiles[t.bundle]) + ";"
		}
		if c.IOS.Signing.Identity != "" {
			sign += " CODE_SIGN_IDENTITY = " + strconv.Quote(c.IOS.Signing.Identity) + ";"
		}
		common := fmt.Sprintf(`PRODUCT_NAME = %s; PRODUCT_BUNDLE_IDENTIFIER = %s; INFOPLIST_FILE = %s; CODE_SIGN_ENTITLEMENTS = %s; IPHONEOS_DEPLOYMENT_TARGET = %s; TARGETED_DEVICE_FAMILY = "1,2"; APPLICATION_EXTENSION_API_ONLY = YES; SKIP_INSTALL = YES; CLANG_ENABLE_OBJC_ARC = YES; ENABLE_BITCODE = NO; DEBUG_INFORMATION_FORMAT = "dwarf-with-dsym"; GCC_GENERATE_DEBUGGING_SYMBOLS = YES; DEVELOPMENT_TEAM = %s; OTHER_LDFLAGS = %s; %s %s`, strconv.Quote(t.name), strconv.Quote(t.bundle), strconv.Quote("Extensions/"+t.name+"/Info.plist"), strconv.Quote("Extensions/"+t.name+"/Target.entitlements"), strconv.Quote(t.minimum), strconv.Quote(team), quoted(flags), sign, settings)
		debug := obj("XCBuildConfiguration", "name = Debug; buildSettings = {"+common+"};")
		release := obj("XCBuildConfiguration", "name = Release; buildSettings = {"+common+" DEPLOYMENT_POSTPROCESSING = YES; STRIP_INSTALLED_PRODUCT = YES; STRIP_STYLE = non-global;};")
		config := obj("XCConfigurationList", "buildConfigurations = ("+debug+", "+release+"); defaultConfigurationIsVisible = 0; defaultConfigurationName = Release;")
		target := obj("PBXNativeTarget", fmt.Sprintf("name = %s; productName = %s; productReference = %s; productType = \"com.apple.product-type.app-extension\"; buildConfigurationList = %s; buildPhases = (%s, %s); buildRules = (); dependencies = ();", strconv.Quote(t.name), strconv.Quote(t.name), product, config, sourcePhase, resourcePhase))
		projectTargets = append(projectTargets, target)
		proxy := obj("PBXContainerItemProxy", "containerPortal = A00000000000000000000001; proxyType = 1; remoteGlobalIDString = "+target+"; remoteInfo = "+strconv.Quote(t.name)+";")
		deps = append(deps, obj("PBXTargetDependency", "target = "+target+"; targetProxy = "+proxy+";"))
		embeds = append(embeds, obj("PBXBuildFile", "fileRef = "+product+"; settings = {ATTRIBUTES = (RemoveHeadersOnCopy, CodeSignOnCopy);};"))
	}
	if len(embeds) > 0 {
		phase := obj("PBXCopyFilesBuildPhase", "buildActionMask = 2147483647; dstPath = \"\"; dstSubfolderSpec = 13; files = ("+list(embeds)+"); name = \"Embed App Extensions\"; runOnlyForDeploymentPostprocessing = 0;")
		base = strings.Replace(base, "buildPhases = (A00000000000000000000011, A00000000000000000000012)", "buildPhases = (A00000000000000000000011, A00000000000000000000012, "+phase+")", 1)
		base = strings.Replace(base, "dependencies = ();", "dependencies = ("+list(deps)+");", 1)
		base = strings.Replace(base, "targets = (A00000000000000000000005)", "targets = (A00000000000000000000005, "+list(projectTargets)+")", 1)
		base = strings.Replace(base, "children = (A00000000000000000000009)", "children = (A00000000000000000000009, "+list(products)+")", 1)
	}
	if len(group) > 0 {
		base = strings.Replace(base, "children = (A00000000000000000000007,", "children = ("+list(group)+", A00000000000000000000007,", 1)
	}
	return strings.Replace(base, " }; rootObject =", objects.String()+" }; rootObject =", 1)
}

func iosCheckExtensions(app, hostID, version, build string) error {
	dir := filepath.Join(app, "PlugIns")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".appex") || !entry.IsDir() {
			return fmt.Errorf("unexpected embedded extension %s", entry.Name())
		}
		bundle := filepath.Join(dir, entry.Name())
		p, err := readIOSPlist(filepath.Join(bundle, "Info.plist"))
		if err != nil {
			return err
		}
		id, _ := p["CFBundleIdentifier"].(string)
		if !strings.HasPrefix(id, hostID+".") || !iosBundleName.MatchString(id) || seen[id] {
			return fmt.Errorf("invalid embedded extension identifier %q", id)
		}
		seen[id] = true
		if p["CFBundleShortVersionString"] != version || p["CFBundleVersion"] != build {
			return fmt.Errorf("extension %s version differs from its host", id)
		}
		ext, ok := p["NSExtension"].(map[string]any)
		if !ok || ext["NSExtensionPointIdentifier"] == nil || p["CFBundlePackageType"] != "XPC!" {
			return fmt.Errorf("invalid extension metadata for %s", id)
		}
		executable, _ := p["CFBundleExecutable"].(string)
		if executable == "" || filepath.Base(executable) != executable || executable == "." || executable == ".." {
			return fmt.Errorf("invalid extension executable")
		}
		binary, err := macho.Open(filepath.Join(bundle, executable))
		if err != nil {
			return err
		}
		valid := binary.Cpu == macho.CpuArm64
		binary.Close()
		if !valid {
			return fmt.Errorf("extension %s is not arm64", id)
		}
		privacy, err := readIOSPlist(filepath.Join(bundle, "PrivacyInfo.xcprivacy"))
		if err != nil {
			return err
		}
		if _, err := mergeIOSPrivacy([]map[string]any{privacy}); err != nil {
			return err
		}
	}
	return nil
}

// Preserve internal relative links, but reject links that would make the
// exported native project depend on files outside its copied inputs.
func iosCopyNative(source, destination string) error {
	if err := copyResource(source, destination); err != nil {
		return err
	}
	root, err := filepath.EvalSymlinks(destination)
	if err != nil {
		return err
	}
	return filepath.WalkDir(destination, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink == 0 {
			return nil
		}
		link, err := os.Readlink(path)
		if err != nil {
			return err
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, resolved)
		if err != nil {
			return err
		}
		if filepath.IsAbs(link) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("native input contains an external symlink: %s", path)
		}
		return nil
	})
}
