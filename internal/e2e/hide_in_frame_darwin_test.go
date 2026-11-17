//go:build darwin && !ios

package e2e

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

// TestHideOnScreenInFrame hides a window that shows on screen from its own
// content as it builds a frame, and the content goes on asking for a
// frame in each: the main thread still runs what is posted to it. The
// window server tells that the window left the screen a pass of the run
// loop later, and package ui holds no frame until then: drawn at once
// each, from the backend's queue, those frames never let that pass come.
// TestHideInFrame hides its window as soon as it has it, before it shows
// on screen unless the runner is slow, and hung only then.
func TestHideOnScreenInFrame(t *testing.T) {
	var self atomic.Pointer[mygo.Window]
	var hide, hidden, still atomic.Bool
	w := newWindow(t, mygo.WindowOptions{Width: 320, Height: 240, Content: ui.View(func(c *ui.Context) {
		if w := self.Load(); w != nil && hide.Load() && !hidden.Load() {
			w.Hide()
			hidden.Store(true)
		}
		if !still.Load() {
			c.AnimationFrame()
		}
		ui.Box(c).Fill().Background(ui.RGB(255, 255, 255))
	})})
	self.Store(w)
	waitOnScreen(w)
	hide.Store(true)
	eventually(t, "the window hiding itself", hidden.Load)
	ran := make(chan struct{})
	go func() {
		mygo.RunOnMain(func() {})
		close(ran)
	}()
	select {
	case <-ran:
	case <-time.After(wait.Guard):
		// Without frames to draw, the main thread runs the cleanups.
		still.Store(true)
		t.Fatalf("the main thread never ran what was posted to it after the window hid itself (waited %v)", wait.Guard)
	}
}
