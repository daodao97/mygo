//go:build linux && !android && (amd64 || arm64)

package linux

import "sync"

// WebKitGTK on GTK 3 (2.50 and later) zooms every page by the screen's
// resolution over 96, gdk_screen_get_resolution, which it takes for the
// font DPI. GDK reports -1 while the resolution is unset, as on Wayland
// sessions whose settings have no text scaling factor (no
// org.gnome.desktop.interface schema and no settings portal answering
// for it): WebKit then zooms pages by -1/96, and a page's viewport comes
// out negative (innerWidth -78048 for 813 pixels), so it never lays out
// (#2175, https://bugs.webkit.org/show_bug.cgi?id=332681).
//
// fixScreenResolution gives such a screen GTK's own default, 96, which
// GTK already draws its text at when the resolution is unset, before the
// first web context reads it. A resolution the session set stays.

// minResolution is the lowest resolution taken as set on purpose: half
// of 96, the smallest text scaling factor GNOME offers. Below it are
// unset (-1), zero, and the 1 or 2 that sessions were seen to report
// while starting.
const minResolution = 48

var (
	resolutionOnce         sync.Once
	gdkScreenGetResolution func(screen ptr) float64
	gdkScreenSetResolution func(screen ptr, dpi float64)
)

// saneResolution reports the resolution the screen should have, and
// whether it differs from r.
func saneResolution(r float64) (float64, bool) {
	if r >= minResolution {
		return r, false
	}
	return 96, true
}

// fixScreenResolution runs on the main thread, the first time webKit is
// called there with WebKitGTK loaded, before any web context exists.
func fixScreenResolution() {
	if !bind(libGDK, &gdkScreenGetDefault, "gdk_screen_get_default") ||
		!bind(libGDK, &gdkScreenGetResolution, "gdk_screen_get_resolution") ||
		!bind(libGDK, &gdkScreenSetResolution, "gdk_screen_set_resolution") {
		return
	}
	screen := gdkScreenGetDefault()
	if screen == 0 {
		return
	}
	if r, changed := saneResolution(gdkScreenGetResolution(screen)); changed {
		gdkScreenSetResolution(screen, r)
	}
}
