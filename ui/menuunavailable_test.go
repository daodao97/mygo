package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

// unavailableApp is a row with a drawn menu whose disabled item says why,
// and the items chosen from it.
type unavailableApp struct{ chosen []string }

const runTip = "Connect to run queries"

func (a *unavailableApp) view(c *context) {
	coreColumn(c).Fill().Padding(20).Children(func() {
		row := coreRow(c).Padding(8).Focusable().Label("Report").Children(func() { coreText(c, "Report.pdf") })
		row.PopoverMenu(func(m *Menu) {
			if m.Item("Open").Chosen() {
				a.chosen = append(a.chosen, "Open")
			}
			if m.Item("Run").Disabled(true).Tooltip(runTip).Chosen() {
				a.chosen = append(a.chosen, "Run")
			}
			m.Submenu("Export", func(m *Menu) { m.Item("CSV") }).Disabled(true)
			if m.Item("Copy").Chosen() {
				a.chosen = append(a.chosen, "Copy")
			}
		})
	})
}

// reachUnavailable sets whether the keys reach unavailable menu items for
// the test, as on Windows or as elsewhere.
func reachUnavailable(t *testing.T, on bool) {
	saved := menusReachUnavailable
	menusReachUnavailable = on
	t.Cleanup(func() { menusReachUnavailable = saved })
}

// TestMenuUnavailableItem shows a drawn menu's disabled item, dimmed and
// read as unavailable, with its tip as the pointer rests on it, on every
// platform; choosing it, or its submenu, does nothing and keeps the menu
// open, and its tip with it.
func TestMenuUnavailableItem(t *testing.T) {
	for _, reach := range []bool{false, true} {
		reachUnavailable(t, reach)
		a := &unavailableApp{}
		tt, now := clockTester(a.view, 400, 300)
		tick := ticker(tt, now)
		tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
		if err := tt.RightClick("Report.pdf"); err != nil {
			t.Fatal(err)
		}
		if n := accessNode(t, tt.h.access, platform.RoleMenuItem, "Run"); n.States&platform.AccessDisabled == 0 {
			t.Errorf("reach %v: Run is not read as unavailable", reach)
		}
		tt.Move(center(at(t, tt, "Run")))
		tick(700 * time.Millisecond)
		if !tt.HasText(runTip) {
			t.Fatalf("reach %v: no tip once the pointer rested on the disabled item", reach)
		}
		tt.Click("Run")
		tick(100 * time.Millisecond)
		if len(a.chosen) != 0 || !menuShows(tt) || !tt.HasText(runTip) {
			t.Fatalf("reach %v: a click on it chose %q, menu shows %v, tip %v", reach, a.chosen, menuShows(tt), tt.HasText(runTip))
		}
		tt.Move(center(at(t, tt, "Export")))
		tick(100 * time.Millisecond)
		if tt.HasText("CSV") {
			t.Fatalf("reach %v: the disabled submenu opened", reach)
		}
	}
}

// TestMenuUnavailableItemKeys skips a disabled item with the keys, as
// macOS's and Linux's menus do, and reaches it on Windows, as Windows'
// do; Enter on it does nothing there.
func TestMenuUnavailableItemKeys(t *testing.T) {
	for _, tc := range []struct {
		reach bool
		down  string
	}{{false, "Copy"}, {true, "Run"}} {
		reachUnavailable(t, tc.reach)
		a := &unavailableApp{}
		tt := coreNewTester(a.view, 400, 300)
		tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
		tt.Click("Report.pdf")
		tt.Key(0, KeyContextMenu)
		wantFocus(t, tt, "Open")
		tt.Key(0, KeyDown)
		wantFocus(t, tt, tc.down)
		if tc.reach {
			tt.Advance(time.Second)
			if !tt.HasText(runTip) {
				t.Error("reach: no tip once the keys rested on the disabled item")
			}
			tt.Key(0, KeyEnter)
			if len(a.chosen) != 0 || !menuShows(tt) {
				t.Errorf("reach: Enter on the disabled item chose %q, menu shows %v", a.chosen, menuShows(tt))
			}
		}
	}
}

// TestAccessDisabledMenuButtonTip hides an AccessDisabled menu button's tip
// as it is pressed: its menu still opens, as a menu button's does.
func TestAccessDisabledMenuButtonTip(t *testing.T) {
	tt, now := clockTester(func(c *context) {
		coreColumn(c).Fill().Padding(40).AlignItems(Start).Children(func() {
			coreButton(c, "More").AccessDisabled(true).Tooltip("Read only").PopoverMenuButton(func(m *Menu) { m.Item("Copy") })
		})
	}, 400, 300)
	tick := ticker(tt, now)
	tt.Move(center(at(t, tt, "More")))
	tick(time.Second)
	if !tt.HasText("Read only") {
		t.Fatal("no tip on the menu button")
	}
	tt.ClickAt(center(at(t, tt, "More")))
	tick(100 * time.Millisecond)
	if !tt.HasText("Copy") || tt.HasText("Read only") {
		t.Errorf("pressed: menu shows %v, tip %v", tt.HasText("Copy"), tt.HasText("Read only"))
	}
}

// TestMenuUnavailableSubmenu reads a disabled submenu item as one with a
// submenu, as the system's menus do, which never opens.
func TestMenuUnavailableSubmenu(t *testing.T) {
	for _, reach := range []bool{false, true} {
		reachUnavailable(t, reach)
		tt := coreNewTester((&unavailableApp{}).view, 400, 300)
		tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
		tt.Click("Report.pdf")
		tt.Key(0, KeyContextMenu)
		n := accessNode(t, tt.h.access, platform.RoleMenuItem, "Export")
		if n.States&platform.AccessExpandable == 0 || n.States&platform.AccessDisabled == 0 {
			t.Errorf("reach %v: Export reads as states %b", reach, n.States)
		}
		if reach {
			tt.Key(0, KeyEnd)
			tt.Key(0, KeyUp) // Export
			tt.Key(0, KeyRight)
			if tt.HasText("CSV") {
				t.Error("Right opened the disabled submenu")
			}
		}
		tt.Move(center(at(t, tt, "Export")))
		tt.ClickAt(center(at(t, tt, "Export")))
		if tt.HasText("CSV") {
			t.Errorf("reach %v: the pointer opened the disabled submenu", reach)
		}
	}
}

// TestMenuUnavailablePointerSkip clears the highlight, and closes the
// submenu open, as the pointer moves onto an item the keys skip, as Qt's
// menus do: Enter then chooses nothing.
func TestMenuUnavailablePointerSkip(t *testing.T) {
	reachUnavailable(t, false)
	a := &unavailableApp{}
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Fill().Padding(20).Children(func() {
			coreRow(c).Padding(8).Focusable().Label("Report").Children(func() { coreText(c, "Report.pdf") }).PopoverMenu(func(m *Menu) {
				m.Submenu("More", func(m *Menu) { m.Item("Deeper") })
				if m.Item("Run").Disabled(true).Chosen() {
					a.chosen = append(a.chosen, "Run")
				}
				if m.Item("Copy").Chosen() {
					a.chosen = append(a.chosen, "Copy")
				}
			})
		})
	}, 400, 300)
	tt.RightClick("Report.pdf")
	tt.Move(center(at(t, tt, "More")))
	if !tt.HasText("Deeper") {
		t.Fatal("More's submenu did not open")
	}
	tt.Move(center(at(t, tt, "Run")))
	if tt.HasText("Deeper") {
		t.Error("More's submenu stayed open with the pointer on Run")
	}
	tt.Key(0, KeyEnter)
	if len(a.chosen) != 0 || !tt.HasText("Copy") {
		t.Errorf("Enter chose %q, menu shows %v", a.chosen, tt.HasText("Copy"))
	}
}
