package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A simulator's executable carries the identifier of the bundle it is in,
// which the keychain needs, with no plugin asking for anything too: in mygo
// dev that is the development app's, <id>.dev.
func TestSimulatorEntitlements(t *testing.T) {
	c := &Config{Identifier: "dev.mygo.x"}
	for _, tc := range []struct {
		dc   *Config
		want string
	}{
		{c, "dev.mygo.x"},
		{devConfig(c), "dev.mygo.x.dev"},
	} {
		for _, ps := range []mobilePlugins{
			nil,
			{{ID: "push", IOS: &pluginIOS{Entitlements: map[string]any{"aps-environment": "development"}}}},
		} {
			work := t.TempDir()
			flags, err := ps.simulatorEntitlements(work, tc.dc.Identifier)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(work, "simulator.entitlements")
			if len(flags) == 0 || flags[len(flags)-1] != path {
				t.Fatalf("flags: %v", flags)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			s := string(b)
			for _, want := range []string{
				"<key>application-identifier</key>\n\t<string>" + tc.want + "</string>",
				"<key>keychain-access-groups</key>\n\t<array>\n\t\t<string>" + tc.want + "</string>",
			} {
				if !strings.Contains(s, want) {
					t.Errorf("%s, %d plugins: no %q in\n%s", tc.want, len(ps), want, s)
				}
			}
			if len(ps) > 0 && !strings.Contains(s, "<key>aps-environment</key>") {
				t.Errorf("the plugin's entitlement is gone:\n%s", s)
			}
		}
	}
}
