//go:build linux && (amd64 || arm64)

package linux

import "testing"

// TestPortalOnX11 binds global shortcuts through the portal under XWayland,
// where the desktop offers it, and grabs keys on a real X server.
func TestPortalOnX11(t *testing.T) {
	for _, c := range []struct {
		name            string
		wayland         string
		offered, portal bool
	}{
		{"X11 session", "", true, false},
		{"XWayland with the portal", "wayland-0", true, true},
		{"XWayland without the portal", "wayland-0", false, false},
	} {
		asked := false
		got := portalOnX11(func(k string) string {
			if k == "WAYLAND_DISPLAY" {
				return c.wayland
			}
			return ""
		}, func() bool { asked = true; return c.offered })
		if got != c.portal {
			t.Errorf("%s: through the portal %v, want %v", c.name, got, c.portal)
		}
		if c.wayland == "" && asked {
			t.Errorf("%s: asked the desktop for the portal", c.name)
		}
	}
}
