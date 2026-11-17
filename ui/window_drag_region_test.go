package ui

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// DragRegionAt tells a backend whether a finger landing would move the
// window, which Windows then leaves to the system's mouse (#1712): on a
// DragWindow handle, not on a button inside it, the content below, or a
// handle that scrolls, which the finger scrolls instead.
func TestDragRegionAt(t *testing.T) {
	closes := 0
	tt := NewTester(func(c *Context) {
		Column(c).Children(func() {
			Row(c).Size(400, 40).DragWindow().Children(func() {
				Box(c).Size(300, 40)
				if Button(c, "Close").Clicked() {
					closes++
				}
			})
			Row(c).Children(func() {
				Scroll(c.Key("sidebar")).Size(100, 260).DragWindow().Children(func() {
					Box(c).Size(100, 1000)
				})
				Scroll(c.Key("short")).Size(100, 260).DragWindow().Children(func() {
					Box(c).Size(100, 100)
				})
				Box(c).Size(200, 260)
			})
		})
	}, 400, 300)
	tt.Frame()
	for _, c := range []struct {
		name string
		x, y float64
		want bool
	}{
		{"the title bar", 100, 20, true},
		{"its button", 340, 20, false},
		{"a sidebar that scrolls", 50, 150, false},
		{"one with nothing to scroll", 150, 150, true},
		{"the content", 300, 150, false},
	} {
		if got := tt.rt.event(platform.SurfaceEvent{Kind: platform.DragRegionAt, X: c.x, Y: c.y}); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	if tt.rt.pressed != nil {
		t.Error("asking pressed an element")
	}
	// The button is where the test asks.
	tt.ClickAt(340, 20)
	tt.Frame()
	if closes != 1 {
		t.Errorf("the button was clicked %d times, want 1", closes)
	}
	tt.rt.close()
}
