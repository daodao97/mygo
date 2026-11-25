//go:build linux && !android && (amd64 || arm64)

package linux

import "testing"

// TestPortalReducesMotion reads less motion from the settings portal as
// KDE's Instant animation speed gives it, reduced-motion 1, and from
// GNOME's animations turned off for portals without reduced-motion.
func TestPortalReducesMotion(t *testing.T) {
	if err := load(); err != nil {
		t.Skip(err)
	}
	var parse func(typ ptr, text *byte, limit, endptr, err ptr) ptr
	mustBind(libGLib, &parse, "g_variant_parse")
	for _, tc := range []struct {
		settings string
		want     bool
	}{
		{`{}`, false},
		{`{'org.freedesktop.appearance': {'reduced-motion': <uint32 1>}}`, true},
		{`{'org.freedesktop.appearance': {'reduced-motion': <uint32 0>}}`, false},
		{`{'org.gnome.desktop.interface': {'enable-animations': <false>}}`, true},
		{`{'org.gnome.desktop.interface': {'enable-animations': <true>}}`, false},
		// KDE Plasma 6 at Instant, and back.
		{`{'org.freedesktop.appearance': {'reduced-motion': <uint32 1>}, 'org.gnome.desktop.interface': {'enable-animations': <false>}}`, true},
		{`{'org.freedesktop.appearance': {'reduced-motion': <uint32 0>}, 'org.gnome.desktop.interface': {'enable-animations': <true>}}`, false},
		// A value of another type is none.
		{`{'org.freedesktop.appearance': {'reduced-motion': <true>}}`, false},
	} {
		all := parse(gVariantTypeNew(cs("a{sa{sv}}")), cs(tc.settings), 0, 0, 0)
		if all == 0 {
			t.Fatalf("%s: does not parse", tc.settings)
		}
		if got := portalReducesMotion(all); got != tc.want {
			t.Errorf("%s: reduces motion %v, want %v", tc.settings, got, tc.want)
		}
		gVariantUnref(all)
	}
}
