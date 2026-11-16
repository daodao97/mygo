package e2e

import (
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/darwin"
	"github.com/egoist/mygo/ui"
)

// A panel's native UI hears the pointer while another app is active, as
// a panel never makes its app active.
func TestPanelHoverWhileInactive(t *testing.T) {
	var moves atomic.Int32
	var drawn atomic.Bool
	view := func(c *ui.Context) {
		ui.Box(c).Fill().HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputPointerMove {
				moves.Add(1)
			}
			return false
		}).Draw(func(*ui.Painter, ui.Rect) { drawn.Store(true) })
	}
	// The app activates as it starts: done before another app is.
	other := newWindow(t, mygo.WindowOptions{Title: "Other", Width: 200, Height: 100})
	other.Focus()
	eventually(t, "the app active", func() bool { active, _ := appActive(); return active })
	panel := newWindow(t, mygo.WindowOptions{Title: "Panel hover", Width: 300, Height: 200, Panel: true, Hidden: true, Content: ui.View(view)})
	if !deactivateApp() {
		t.Skip("cannot activate another app")
	}
	// The tests after this one find the app active, as TestPanel needs.
	t.Cleanup(func() {
		activateApp()
		eventually(t, "the app active again", func() bool { active, _ := appActive(); return active })
	})
	eventually(t, "the app deactivated", func() bool { active, _ := appActive(); return !active })
	panel.ShowInactive()
	eventually(t, "the first frame", drawn.Load)

	if active, _ := appActive(); active {
		t.Fatal("showing the panel activated the app")
	}
	// The window server may place the panel after ShowInactive returns:
	// the pointer moves over it until the panel hears it.
	b := panel.ContentBounds()
	dx := 0.0
	eventually(t, "the pointer moving over the panel", func() bool {
		var ok bool
		mygo.RunOnMain(func() { ok = darwin.TestPostMouseMove(float64(b.X)+float64(b.Width)/2+dx, float64(b.Y)+float64(b.Height)/2) })
		if !ok {
			t.Skip("this process may not post events (Accessibility)")
		}
		dx = 10 - dx
		return moves.Load() > 0
	})
	if active, _ := appActive(); active {
		t.Error("the panel activated the app")
	}
}
