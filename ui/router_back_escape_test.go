package ui

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// Android's back comes as Escape, then the Back key, at once (the phones'
// host): where nothing takes the Escape and a Router takes the Back, the
// page it goes back to has neither, though it takes Escape itself, as a
// bar of tabs leaving its tab.
func TestRouterBackLeavesEscape(t *testing.T) {
	r := NewRouter("/")
	r.Transition = TransitionNone
	escapes := 0
	tt := coreNewTester(func(c *context) {
		r.coreView(c, func(rt *Route) {
			switch {
			case rt.Match("/"):
				if c.Shortcut(0, KeyEscape) {
					escapes++
				}
				coreText(c, "Home")
			default:
				coreText(c, "Detail")
			}
		})
	}, 400, 300)
	r.Push("/detail")
	tt.Frame()
	if !tt.HasText("Detail") {
		t.Fatalf("shows %q", tt.Texts())
	}
	for _, k := range []platform.Key{platform.KeyEscape, platform.KeyBack} {
		tt.rt.event(platform.SurfaceEvent{Kind: platform.KeyPressed, Key: k})
		tt.rt.event(platform.SurfaceEvent{Kind: platform.KeyReleased, Key: k})
	}
	tt.settle()
	if !tt.HasText("Home") || escapes != 0 {
		t.Fatalf("shows %q, after %d escapes", tt.Texts(), escapes)
	}
	// Escape alone still reaches it.
	tt.Key(0, KeyEscape)
	if escapes != 1 {
		t.Fatalf("Escape: %d escapes", escapes)
	}
}
