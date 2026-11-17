//go:build linux && !android && (amd64 || arm64)

package e2e

import (
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// TestInputRegion clicks a desktop pet: a frameless, transparent window
// over another, whose input region takes the clicks on its character and
// lets those elsewhere through to the window behind, through the X
// server's input shape, as a user's would be.
func TestInputRegion(t *testing.T) {
	var behind, pet, character atomic.Int32
	var drawn atomic.Int32
	counted := func(n *atomic.Int32, e ui.Element) {
		if e.Clicked() {
			n.Add(1)
		}
		e.Draw(func(*ui.Painter, ui.Rect) { drawn.Add(1) })
	}
	back := newWindow(t, mygo.WindowOptions{Title: "Behind", X: 120, Y: 120, Width: 320, Height: 240, Content: ui.View(func(c *ui.Context) {
		counted(&behind, ui.Box(c).Fill().Background(ui.RGB(240, 240, 240)))
	})})
	front := newWindow(t, mygo.WindowOptions{Title: "Pet", X: 120, Y: 120, Width: 320, Height: 240, Frameless: true, Transparent: true, AlwaysOnTop: true, Content: ui.View(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		root := ui.Box(c).Fill()
		counted(&pet, root)
		root.Children(func() {
			counted(&character, ui.Box(c).Absolute().Left(20).Top(30).Size(60, 50).Background(ui.RGB(220, 30, 40)))
		})
	})})
	eventually(t, "both windows drawn", func() bool { return back.IsVisible() && front.IsVisible() && drawn.Load() >= 3 })
	front.SetInputRegion([]mygo.Rectangle{{X: 20, Y: 30, Width: 60, Height: 50}})

	if !click(front, 50, 50) {
		t.Skip("surface click automation unavailable")
	}
	eventually(t, "the character clicked", func() bool { return character.Load() == 1 })
	onPet := pet.Load() // the character's click, if it reaches the root too
	click(front, 200, 150)
	eventually(t, "the window behind clicked", func() bool { return behind.Load() == 1 })
	if n := pet.Load(); n != onPet {
		t.Fatalf("the pet took %d clicks off the character", n-onPet)
	}

	// An empty region lets all of it through, and nil none of it.
	front.SetInputRegion([]mygo.Rectangle{})
	click(front, 50, 50)
	eventually(t, "the window behind clicked through the character", func() bool { return behind.Load() == 2 })
	front.SetInputRegion(nil)
	click(front, 200, 150)
	eventually(t, "the pet clicked again", func() bool { return pet.Load() == onPet+1 })
	if n := behind.Load(); n != 2 {
		t.Fatalf("the window behind took %d clicks, want 2", n)
	}
}

// TestInputRegionPage shapes the input of a window showing a page, which
// has no native UI's surface.
func TestInputRegionPage(t *testing.T) {
	w := newWindow(t, mygo.WindowOptions{Title: "Page region", Width: 240, Height: 160})
	eventually(t, "the window shown", w.IsVisible)
	w.SetInputRegion([]mygo.Rectangle{{X: 10, Y: 10, Width: 50, Height: 40}})
	w.SetIgnoreMouseEvents(true)
	w.SetIgnoreMouseEvents(false)
	w.SetInputRegion(nil)
}
