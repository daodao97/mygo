//go:build !js

package mygo

import (
	"slices"
	"testing"
)

// NewTabs needs a tab, and every tab a window; on a desktop, which has
// no tab bar, the tabs' windows stay windows and the first one shows.
func TestNewTabs(t *testing.T) {
	if _, err := NewTabs(); err == nil {
		t.Fatal("NewTabs with no tabs said nothing")
	}
	if _, err := NewTabs(TabItem{Title: "Home"}); err == nil {
		t.Fatal("NewTabs with a tab of no window said nothing")
	}
	home, _ := testWindow(t, WindowOptions{Title: "Home", Hidden: true})
	web, _ := testWindow(t, WindowOptions{Title: "Web", Hidden: true})
	tabs, err := NewTabs(
		TabItem{Title: "Home", Window: home},
		TabItem{Title: "Web", Window: web},
	)
	if err != nil {
		t.Fatal(err)
	}
	if tabs.Selected() != 0 {
		t.Fatalf("the tab chosen is %d", tabs.Selected())
	}
	waitFor(t, "the first tab's window to show", func() bool { return home.IsVisible() })

	// Select shows the tab's window, and leaves out-of-range choices
	// alone.
	tabs.Select(1)
	waitFor(t, "the second tab's window to show", func() bool { return web.IsVisible() })
	if tabs.Selected() != 1 {
		t.Fatalf("the tab chosen is %d", tabs.Selected())
	}
	tabs.Select(-1)
	tabs.Select(5)
	if tabs.Selected() != 1 {
		t.Fatalf("an out-of-range select chose %d", tabs.Selected())
	}
	tabs.SetBadge(1, "3")

	// The tab bar's choice comes back through OnSelect, until it is
	// turned off; a choice of the tab already chosen says nothing.
	chose := make(chan int, 4)
	off := tabs.OnSelect(func(i int) { chose <- i })
	onMain(func() { tabsHandler{t: tabs}.TabSelected(0) })
	if i := <-chose; i != 0 || tabs.Selected() != 0 {
		t.Fatalf("chose %d, selected %d", i, tabs.Selected())
	}
	onMain(func() { tabsHandler{t: tabs}.TabSelected(0) })
	if n := len(chose); n != 0 {
		t.Fatalf("%d events for the tab already chosen", n)
	}
	off()
	onMain(func() { tabsHandler{t: tabs}.TabSelected(1) })
	if n := len(chose); n != 0 {
		t.Fatalf("%d events after off", n)
	}
}

// On a desktop, which has no stack of screens, Push shows the child and
// Pop closes it.
func TestWindowPushPop(t *testing.T) {
	parent, _ := testWindow(t, WindowOptions{Title: "List"})
	child, _ := testWindow(t, WindowOptions{Title: "Detail", Hidden: true})
	parent.Push(child)
	waitFor(t, "the pushed window to show", func() bool { return child.IsVisible() })
	child.Pop()
	waitFor(t, "the popped window to close", func() bool { return !slices.Contains(Windows(), child) })
	if child.native != nil {
		t.Fatal("the popped window is still attached")
	}
}
