package main

import (
	"fmt"
	iossupport "github.com/egoist/mygo/native/ios"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var iosCSymbol = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateIOSBackground(c *Config) error {
	if len(c.IOS.BackgroundTasks) == 0 {
		return nil
	}
	if c.IOS.Native.EntryPoint != "" {
		return fmt.Errorf("ios.backgroundTasks requires the generated entry point; remove ios.native.entryPoint")
	}
	seen := map[string]bool{}
	symbols := map[string]string{}
	for _, j := range c.IOS.BackgroundTasks {
		if !iosBundleName.MatchString(j.Identifier) || !strings.HasPrefix(j.Identifier, c.Identifier+".") || seen[j.Identifier] {
			return fmt.Errorf("invalid or duplicate background task identifier %q", j.Identifier)
		}
		seen[j.Identifier] = true
		for i, s := range []string{j.Prepare, j.Run, j.Cancel, j.NextDelay} {
			kind := []string{"void", "int", "void", "double"}[i]
			if !iosCSymbol.MatchString(s) || (symbols[s] != "" && symbols[s] != kind) {
				return fmt.Errorf("invalid or conflicting background callback %q", s)
			}
			symbols[s] = kind
		}
	}
	// Generated declarations own the identifier list. Other background modes may coexist.
	if c.IOS.InfoPlist["BGTaskSchedulerPermittedIdentifiers"] != nil {
		return fmt.Errorf("configure ios.backgroundTasks instead of BGTaskSchedulerPermittedIdentifiers")
	}
	if v := c.IOS.InfoPlist["UIBackgroundModes"]; v != nil {
		switch a := v.(type) {
		case []string:
		case []any:
			for _, s := range a {
				if _, ok := s.(string); !ok {
					return fmt.Errorf("UIBackgroundModes must contain strings")
				}
			}
		default:
			return fmt.Errorf("UIBackgroundModes must be a string array")
		}
	}
	return nil
}
func iosBackgroundPlist(c *Config, d map[string]any) {
	if len(c.IOS.BackgroundTasks) == 0 {
		return
	}
	ids := []string{}
	for _, j := range c.IOS.BackgroundTasks {
		ids = append(ids, j.Identifier)
	}
	d["BGTaskSchedulerPermittedIdentifiers"] = ids
	modes := []string{}
	switch v := d["UIBackgroundModes"].(type) {
	case []string:
		modes = append(modes, v...)
	case []any:
		for _, s := range v {
			if str, ok := s.(string); ok {
				modes = append(modes, str)
			}
		}
	}
	found := false
	for _, m := range modes {
		found = found || m == "fetch"
	}
	if !found {
		modes = append(modes, "fetch")
	}
	d["UIBackgroundModes"] = modes
}
func iosPrepareBackground(c *Config, host string, t *iosNativeTarget) error {
	if len(c.IOS.BackgroundTasks) == 0 {
		return nil
	}
	base := filepath.Join("Native", t.name, "Background")
	if err := os.MkdirAll(filepath.Join(host, base), 0755); err != nil {
		return err
	}
	for _, name := range []string{"MyGoBackgroundRefresh.m", "MyGoBackgroundRefresh.h"} {
		b, e := iossupport.Sources.ReadFile("background/" + name)
		if e != nil {
			return e
		}
		rel := filepath.Join(base, name)
		if e = os.WriteFile(filepath.Join(host, rel), b, 0644); e != nil {
			return e
		}
		if strings.HasSuffix(name, ".m") {
			t.sources = append(t.sources, rel)
		} else {
			t.headers = append(t.headers, rel)
		}
	}
	var b strings.Builder
	b.WriteString("#import \"MyGoBackgroundRefresh.h\"\n")
	for _, j := range c.IOS.BackgroundTasks {
		fmt.Fprintf(&b, "extern void %s(void);\nextern int %s(void);\nextern void %s(void);\nextern double %s(void);\n", j.Prepare, j.Run, j.Cancel, j.NextDelay)
	}
	b.WriteString("void MyGoRegisterConfiguredBackgroundTasks(void) {\n")
	for _, j := range c.IOS.BackgroundTasks {
		fmt.Fprintf(&b, " MyGoRegisterBackgroundRefresh(\"%s\", %s, %s, %s, %s);\n", j.Identifier, j.Prepare, j.Run, j.Cancel, j.NextDelay)
	}
	b.WriteString("}\n")
	rel := filepath.Join(base, "MyGoConfiguredBackgroundTasks.m")
	if err := os.WriteFile(filepath.Join(host, rel), []byte(b.String()), 0644); err != nil {
		return err
	}
	t.sources = append(t.sources, rel)
	t.native.Frameworks = append(t.native.Frameworks, "BackgroundTasks")
	main := "#import <Foundation/Foundation.h>\nextern int mygo_ios_start(void);\nextern void MyGoRegisterConfiguredBackgroundTasks(void);\nint main(int argc, char **argv) { @autoreleasepool { MyGoRegisterConfiguredBackgroundTasks(); } return mygo_ios_start(); }\n"
	return os.WriteFile(filepath.Join(host, "main.m"), []byte(main), 0644)
}
