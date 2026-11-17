//go:build darwin

package e2e

import (
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/darwin"
	"github.com/egoist/mygo/ui"
)

// TestCloseOrdersOutFirst closes a window of native UI that shows, with
// Destroy and as its close button does, and checks that it was ordered
// out by the time it told it closed, which follows its content going:
// AppKit's animation fades the window as it orders it out, which faded
// an empty window when the content went first (#1931).
func TestCloseOrdersOutFirst(t *testing.T) {
	for _, c := range []struct {
		name  string
		close func(w *mygo.Window, handle uintptr)
	}{
		{"Destroy", func(w *mygo.Window, _ uintptr) { w.Destroy() }},
		{"close button", func(_ *mygo.Window, handle uintptr) {
			mygo.RunOnMain(func() { darwin.TestPerformClose(handle) })
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := newWindow(t, mygo.WindowOptions{Width: 320, Height: 240, Content: ui.View(func(c *ui.Context) {
				ui.Box(c).Fill().Background(ui.RGB(255, 255, 255))
			})})
			waitOnScreen(w)
			handle := w.NativeHandle()
			var closed, orderedIn atomic.Bool
			w.OnClosed(func() {
				orderedIn.Store(darwin.TestOrderedIn(handle))
				closed.Store(true)
			})
			c.close(w, handle)
			eventually(t, "the window closing", closed.Load)
			if orderedIn.Load() {
				t.Error("the window was still ordered in once its content went")
			}
		})
	}
}
