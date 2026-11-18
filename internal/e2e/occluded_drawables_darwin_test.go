//go:build darwin && !ios

package e2e

import (
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/darwin"
	"github.com/egoist/mygo/ui"
)

// TestOccludedWindowGivesBackDrawables minimizes a window of native UI
// drawn with Metal, whose drawables, as large as the window, it gives back
// out of sight (#2040), and changes what it shows meanwhile: restored, it
// draws a frame at its size again, with the change.
func TestOccludedWindowGivesBackDrawables(t *testing.T) {
	var blue, builtBlue atomic.Bool
	w := newWindow(t, mygo.WindowOptions{Title: "Drawables out of sight", Width: 320, Height: 240, Content: ui.View(func(c *ui.Context) {
		color := ui.RGB(255, 0, 0)
		if blue.Load() {
			color = ui.RGB(0, 0, 255)
			builtBlue.Store(!c.WindowHidden())
		}
		ui.Box(c).Fill().Background(color)
	})})
	waitOnScreen(w)
	size := func() (width, height float64, ok bool) {
		mygo.RunOnMain(func() { width, height, ok = darwin.TestMetalDrawableSize(w.NativeHandle()) })
		return width, height, ok
	}
	// The first frame made the renderer, before the window showed.
	if _, _, ok := size(); !ok {
		t.Skip("the window draws without Metal")
	}
	full := func() bool {
		width, height, _ := size()
		return width > 1 && height > 1
	}
	eventually(t, "drawables at the window's size", func() bool {
		w.Update(func() {}) // a frame, in case the idle window gave them back already
		return full()
	})

	w.Minimize()
	eventually(t, "the window minimized", w.IsMinimized)
	eventually(t, "the drawables given back out of sight", func() bool {
		width, height, _ := size()
		return width == 1 && height == 1
	})
	blue.Store(true)
	w.Update(func() {})

	w.Restore()
	eventually(t, "a frame at the window's size, with the change, as it shows", func() bool { return builtBlue.Load() && full() })
}
