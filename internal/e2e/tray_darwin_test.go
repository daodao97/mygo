//go:build darwin && !ios

package e2e

import (
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/darwin"
	"github.com/egoist/mygo/ui"
)

// TestTrayPopoverToggle opens a focusable popover from a tray icon's click,
// as examples/menubar-flyout does, and closes it with a click on the icon,
// which does not open it again, or elsewhere, each time it opens, with
// clicks the window server delivers as the user's (#1050). macOS 26 asks
// the popover's delegate on the release; where AppKit asks on the press,
// as the reporter's macOS does, a press on the icon closes it too, which
// the test asks as AppKit would, with the button held over the icon.
func TestTrayPopoverToggle(t *testing.T) {
	tray, err := mygo.NewTray(mygo.TrayOptions{Icon: squarePNG(), ToolTip: "popover"})
	if err != nil {
		t.Skip("no tray: ", err)
	}
	defer tray.Destroy()
	waitTrayLaidOut(tray)
	elsewhere := newWindow(t, mygo.WindowOptions{Width: 300, Height: 200})
	waitOnScreen(elsewhere)

	var panel *mygo.Window // main thread only
	var shown atomic.Pointer[mygo.Window]
	var clicks, opened, closed atomic.Int32
	toggle := func() {
		if panel != nil {
			panel.Close()
			return
		}
		win := mygo.NewFlyout(mygo.FlyoutOptions{Tray: tray, Placement: mygo.PlacementBottom, Gap: 4, Width: 200, Height: 120,
			Focusable: true, Shadow: true, Popover: true, Content: ui.View(func(c *ui.Context) {})})
		panel = win
		shown.Store(win)
		opened.Add(1)
		win.OnClosed(func() {
			if panel == win {
				panel = nil
			}
			closed.Add(1)
		})
	}
	tray.OnClick(func() {
		clicks.Add(1)
		toggle()
	})

	buttons := func() (downs, ups int) {
		mygo.RunOnMain(func() { downs, ups = darwin.TestMouseButtons() })
		return downs, ups
	}
	buttons() // counting from here
	// post presses or releases the button at (x, y) on screen, and waits for
	// the app to handle it: the event monitor counts it, but for a release
	// the icon's button took tracking its press, which clicks it.
	post := func(x, y float64, down bool) {
		t.Helper()
		d, u := buttons()
		k := clicks.Load()
		var ok bool
		mygo.RunOnMain(func() { ok = darwin.TestPostMouse(x, y, down) })
		if !ok {
			t.Skip("this process may not post events (Accessibility)")
		}
		eventually(t, "the app took the button's event", func() bool {
			downs, ups := buttons()
			return down && downs > d || !down && (ups > u || clicks.Load() > k)
		})
		mygo.RunOnMain(func() {}) // after the event, and what it posted
	}
	click := func(x, y float64) {
		t.Helper()
		post(x, y, true)
		post(x, y, false)
	}
	icon := func() (x, y float64) {
		b := tray.Bounds()
		return float64(b.X) + float64(b.Width)/2, float64(b.Y) + float64(b.Height)/2
	}
	clickIcon := func() { click(icon()) }
	clickElsewhere := func() {
		b := elsewhere.ContentBounds()
		click(float64(b.X)+float64(b.Width)/2, float64(b.Y)+float64(b.Height)/2)
	}
	open := func() {
		t.Helper()
		n := opened.Load()
		clickIcon()
		eventually(t, "the icon's click opened the popover", func() bool { return opened.Load() > n && shown.Load().IsVisible() })
	}
	// closes closes the open popover with close, and checks that it stays
	// closed once the app handled what close did.
	closes := func(what string, close func()) {
		t.Helper()
		n, c := opened.Load(), closed.Load()
		close()
		eventually(t, "the popover closed by "+what, func() bool { return closed.Load() > c })
		mygo.RunOnMain(func() {}) // after what the close posted
		if opened.Load() != n || shown.Load().IsVisible() {
			t.Fatalf("%s opened the popover again", what)
		}
	}
	for range 2 {
		open()
		closes("a click elsewhere", clickElsewhere)
		open()
		closes("a click on the icon", clickIcon)
		open()
		closes("a click elsewhere after the icon's", clickElsewhere)
	}

	// A press on the icon, which the button tracks here, as AppKit asks
	// where it closes the popover on presses.
	open()
	c := closed.Load()
	x, y := icon()
	post(x, y, true)
	var ok bool
	mygo.RunOnMain(func() { ok = darwin.TestClosePopover(shown.Load().NativeHandle()) })
	if !ok {
		t.Fatal("the flyout shows no popover")
	}
	eventually(t, "the popover closed by a press on the icon", func() bool { return closed.Load() > c })
	// The button, which AppKit leaves the press to here, clicks as it is
	// released: the popover opens again.
	post(x, y, false)
	mygo.RunOnMain(func() {
		if panel != nil {
			panel.Close()
		}
	})
	open()
	closes("a click elsewhere after a press on the icon", clickElsewhere)
}

// TestTrayMonospacedTitle keeps a tray item's width as the letters of its
// title change, the title keeping its length (#225): with only its digits
// at one width, "M" is wider than "B", and a rate that changes units
// resizes the item, which shifts its icon and the items beside it.
func TestTrayMonospacedTitle(t *testing.T) {
	width := func(mono bool, title string) int {
		t.Helper()
		tray, err := mygo.NewTray(mygo.TrayOptions{Title: title, MonospacedTitle: mono})
		if err != nil {
			t.Skip("no tray: ", err)
		}
		defer tray.Destroy()
		waitTrayLaidOut(tray)
		return tray.Bounds().Width
	}
	const busy, idle = "↓999M ↑999M", "↓\u2007\u20070B ↑\u2007\u20070B"
	if a, b := width(true, busy), width(true, idle); a == 0 || a != b {
		t.Errorf("monospaced, the item is %d wide with %q and %d with %q, want one width", a, busy, b, idle)
	}
	// What the test rests on: the system's font draws these at two widths.
	if a, b := width(false, busy), width(false, idle); a == b {
		t.Errorf("in the system's font the item is %d wide with both titles: they tell nothing", a)
	}
}
