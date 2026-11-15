//go:build darwin

package e2e

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/darwin"
	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

// TestPopoverFlyoutsReuseNSPopovers opens a popover flyout, closes it and
// opens another, as a menu bar app does at each click of its tray icon:
// the second shows in the first one's NSPopover. On macOS 26 a released
// NSPopover leaves its material's backdrop behind, a few MB each.
func TestPopoverFlyoutsReuseNSPopovers(t *testing.T) {
	parent := newWindow(t, mygo.WindowOptions{Width: 300, Height: 200})
	eventually(t, "anchor window visible", parent.IsVisible)
	open := func() (*mygo.Window, uintptr) {
		t.Helper()
		fl := mygo.NewFlyout(mygo.FlyoutOptions{Parent: parent, Anchor: mygo.Rectangle{X: 100, Y: 20, Width: 80, Height: 24}, Popover: true, Width: 220, Height: 120, Content: ui.View(func(c *ui.Context) { ui.Text(c, "Popover reuse test") })})
		eventually(t, "popover window adopted", func() bool { return fl.NativeHandle() != 0 })
		var popover uintptr
		mygo.RunOnMain(func() { popover = darwin.TestPopover(fl.NativeHandle()) })
		if popover == 0 {
			t.Fatal("the flyout shows in no NSPopover")
		}
		return fl, popover
	}
	first, before := open()
	closed := make(chan struct{}, 1)
	first.OnClosed(func() { closed <- struct{}{} })
	first.Close()
	wait.For(t, "first flyout closed", closed)
	eventually(t, "the first popover finished closing", func() bool {
		var closing bool
		mygo.RunOnMain(func() { closing = darwin.TestPopoverClosing(before) })
		return !closing
	})
	second, after := open()
	defer second.Destroy()
	if after != before {
		t.Fatalf("the second popover flyout shows in a new NSPopover (%#x), not the first one's (%#x)", after, before)
	}
	if !second.IsVisible() {
		t.Fatal("the second popover flyout does not show")
	}
}

// TestPopoverFlyoutClosedAsItHides hides a popover flyout and closes it
// while its popover's animation runs, then opens another at once. The
// first popover still shows, and tells it closed only as the animation
// ends: the second flyout shows in another one, and stays open after that.
func TestPopoverFlyoutClosedAsItHides(t *testing.T) {
	parent := newWindow(t, mygo.WindowOptions{Width: 300, Height: 200})
	eventually(t, "anchor window visible", parent.IsVisible)
	open := func() *mygo.Window {
		return mygo.NewFlyout(mygo.FlyoutOptions{Parent: parent, Anchor: mygo.Rectangle{X: 100, Y: 20, Width: 80, Height: 24}, Popover: true, Width: 220, Height: 120, Content: ui.View(func(c *ui.Context) { ui.Text(c, "Popover reuse test") })})
	}
	closing := func(popover uintptr) (closing bool) {
		mygo.RunOnMain(func() { closing = darwin.TestPopoverClosing(popover) })
		return closing
	}
	first := open()
	eventually(t, "the first popover shown", first.IsVisible)
	// On one turn of the main thread, which its popover's telling it
	// closed cannot come in the middle of: the second flyout takes its
	// NSPopover as NewFlyout makes it.
	var before uintptr
	var wasClosing bool
	var second *mygo.Window
	mygo.RunOnMain(func() {
		before = darwin.TestPopover(first.NativeHandle())
		first.Hide()
		first.Close()
		wasClosing = darwin.TestPopoverClosing(before)
		second = open()
	})
	defer second.Destroy()
	eventually(t, "the second popover's window adopted", func() bool { return second.NativeHandle() != 0 })
	if !wasClosing {
		t.Skip("the first popover finished closing at once, and is the second one's")
	}
	var after uintptr
	mygo.RunOnMain(func() { after = darwin.TestPopover(second.NativeHandle()) })
	if after == before {
		t.Fatal("the second flyout shows in the first one's NSPopover, which is still closing")
	}
	eventually(t, "the first popover closed", func() bool { return !closing(before) })
	mygo.RunOnMain(func() {}) // after what its close posted
	if second.IsDestroyed() || !second.IsVisible() {
		t.Fatal("the first popover's close closed the second flyout")
	}
}
