//go:build darwin && !ios

package darwin

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// TestOcclusionChange tells a popover flyout's surface shown and hidden once
// for each change, though its window's occlusion, popoverDidShow: and
// forgetting its window may each tell the same, over repeated shows (#2053).
func TestOcclusionChange(t *testing.T) {
	var shown bool
	for i, c := range []struct {
		visible bool
		want    platform.SurfaceEventKind
		ok      bool
	}{
		{false, 0, false},                     // made hidden: nothing to tell
		{true, platform.SurfaceShown, true},   // the window's occlusion as it shows
		{true, 0, false},                      // popoverDidShow:
		{false, platform.SurfaceHidden, true}, // the window's occlusion as it hides
		{false, 0, false},                     // popoverDidClose: forgets the window
		{true, platform.SurfaceShown, true},   // popoverDidShow:, the occlusion not told
		{true, 0, false},                      // the window's occlusion after it
		{false, platform.SurfaceHidden, true}, // popoverDidClose: forgets the window
	} {
		kind, ok := occlusionChange(&shown, c.visible)
		if kind != c.want || ok != c.ok || shown != c.visible {
			t.Errorf("%d: visible %v: %v %v, shown %v; want %v %v", i, c.visible, kind, ok, shown, c.want, c.ok)
		}
	}
}
