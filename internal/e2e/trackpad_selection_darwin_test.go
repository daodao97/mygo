//go:build darwin

package e2e

import (
	"slices"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/darwin"
	"github.com/egoist/mygo/ui"
)

func TestTrackpadMouseDragSelectsText(t *testing.T) {
	var frames atomic.Int32
	w := newWindow(t, mygo.WindowOptions{Title: "Trackpad selection", Width: 400, Height: 200, TitleBarStyle: mygo.TitleBarHidden, Content: ui.View(func(c *ui.Context) {
		frames.Add(1)
		ui.Column(c).Fill().Padding(20).Gap(20).Selectable().Children(func() {
			ui.Text(c, "First paragraph.").Height(30)
			ui.Text(c, "Second paragraph.").Height(30)
		})
	})})
	previousMenu := mygo.App.Menu()
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{{Role: mygo.RoleEditMenu}}))
	defer mygo.App.SetMenu(previousMenu)
	clipboard := mygo.Clipboard.ReadText()
	defer mygo.Clipboard.WriteText(clipboard)
	w.Focus()
	waitOnScreen(w)
	eventually(t, "the trackpad window focused", w.IsFocused)
	eventually(t, "a frame", func() bool { return frames.Load() > 0 })
	for _, reverse := range []bool{false, true} {
		points := [][2]float64{{20, 28}, {200, 78}, {380, 78}}
		if reverse {
			slices.Reverse(points)
		}
		before := frames.Load()
		var ok bool
		mygo.RunOnMain(func() { ok = darwin.TestTrackpadDrag(w.NativeHandle(), points) })
		if !ok {
			t.Fatal("AppKit did not preserve the trackpad mouse subtype")
		}
		eventually(t, "a frame after the trackpad drag", func() bool { return frames.Load() > before })
		enabled, err := menuItemEnabled(w, "Edit", "Copy")
		if err != nil || !enabled {
			t.Fatalf("trackpad mouse drag did not create a selection: Copy enabled=%v, error=%v", enabled, err)
		}
		mygo.Clipboard.WriteText("before copy")
		if err := activateMenu(w, "Edit", "Copy"); err != nil {
			t.Fatal(err)
		}
		eventually(t, "trackpad selection copied", func() bool { return mygo.Clipboard.ReadText() == "First paragraph.\nSecond paragraph." })
	}
	// A trackpad double click must select a word immediately, without a hold.
	before := frames.Load()
	var ok bool
	mygo.RunOnMain(func() { ok = darwin.TestTrackpadDoubleClick(w.NativeHandle(), 25, 28) })
	if !ok {
		t.Fatal("AppKit did not preserve the double-click subtype")
	}
	eventually(t, "a frame after the double click", func() bool { return frames.Load() > before })
	mygo.Clipboard.WriteText("before double-click copy")
	if err := activateMenu(w, "Edit", "Copy"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "trackpad double-click word copied", func() bool { return mygo.Clipboard.ReadText() == "First" })
}
