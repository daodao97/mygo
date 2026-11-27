//go:build !js

package mygo

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func menuLabels(items []*MenuItem) string {
	var labels []string
	for _, it := range items {
		labels = append(labels, it.Label)
	}
	return strings.Join(labels, ",")
}

// Insert puts an item where it says, clamped to the menu's ends, finds it
// by ID, and a menu in use is shown again with the insert in it.
func TestMenuInsert(t *testing.T) {
	m := NewMenu([]*MenuItem{
		{ID: "a", Label: "A"},
		{ID: "b", Label: "B"},
	})
	m.Insert(1, &MenuItem{ID: "c", Label: "C"})
	if got := menuLabels(m.Items()); got != "A,C,B" {
		t.Fatalf("after the insert the menu is %q", got)
	}
	m.Insert(-1, &MenuItem{ID: "first", Label: "First"})
	m.Insert(99, &MenuItem{ID: "last", Label: "Last"})
	if got := menuLabels(m.Items()); got != "First,A,C,B,Last" {
		t.Fatalf("out-of-range inserts gave %q", got)
	}
	if m.ItemByID("c") == nil {
		t.Fatal("the inserted item is not found by ID")
	}

	w, fw := testWindow(t, WindowOptions{})
	w.SetMenu(m)
	m.Insert(0, &MenuItem{ID: "top", Label: "Top"})
	// The menu is installed again on the main thread, before this runs.
	// Read it there but check it here: t.Fatal on the main thread would end
	// the event loop's goroutine and hang the tests after this one.
	menu := onMainValue(fw.Menu)
	if menu == nil || len(menu.Items) != 6 || menu.Items[0].Label != "Top" {
		t.Fatalf("the window's menu was not shown again: %+v", menu)
	}
}

// The setters change what the item is and what the backend is told: a
// hidden item, a changed shortcut and label, and back again.
func TestMenuItemState(t *testing.T) {
	m := NewMenu([]*MenuItem{{ID: "act", Label: "Act", Accelerator: "CmdOrCtrl+A"}})
	act := m.ItemByID("act")
	if !act.IsEnabled() || !act.IsVisible() {
		t.Fatal("the item starts disabled or hidden")
	}

	act.SetVisible(false)
	if act.IsVisible() {
		t.Fatal("the item is still visible")
	}
	act.SetEnabled(false)
	act.SetAccelerator("CmdOrCtrl+B")
	act.SetLabel("Do")
	onMain(func() {}) // the updates reach the backend on the main thread
	updates := fb.MenuUpdates()
	last := updates[len(updates)-1]
	if last.ID != act.uid || last.Visible || last.Enabled ||
		last.Accelerator != "CmdOrCtrl+B" || last.Label != "Do" {
		t.Fatalf("the setters did not reach the backend: %+v", last)
	}

	act.SetVisible(true)
	if !act.IsVisible() {
		t.Fatal("the item did not come back")
	}
	if act.IsEnabled() {
		t.Fatal("the item came back enabled")
	}
}

// SetChecked on a radio item unchecks the radio items beside it, and the
// whole group reaches the backend; a checkbox checks alone.
func TestMenuItemRadio(t *testing.T) {
	m := NewMenu([]*MenuItem{
		{Label: "View", Submenu: []*MenuItem{
			{ID: "grid", Label: "Grid", Type: MenuItemCheckbox},
			{ID: "small", Label: "Small", Type: MenuItemRadio, Checked: true},
			{ID: "large", Label: "Large", Type: MenuItemRadio},
		}},
	})
	grid, small, large := m.ItemByID("grid"), m.ItemByID("small"), m.ItemByID("large")
	if !small.IsChecked() || large.IsChecked() || grid.IsChecked() {
		t.Fatal("the menu starts checked wrong")
	}
	large.SetChecked(true)
	if !large.IsChecked() || small.IsChecked() {
		t.Fatal("the radio group did not move")
	}
	small.SetChecked(true)
	if !small.IsChecked() || large.IsChecked() {
		t.Fatal("the radio group did not move back")
	}
	grid.SetChecked(true)
	if !grid.IsChecked() {
		t.Fatal("the checkbox did not check")
	}

	onMain(func() {}) // the updates reach the backend on the main thread
	// The group's items were all told: the item checked, its sibling
	// unchecked.
	var checked, sibling *platform.MenuItem
	for _, u := range fb.MenuUpdates() {
		if u.ID == small.uid {
			checked = u
		}
		if u.ID == large.uid {
			sibling = u
		}
	}
	if checked == nil || !checked.Checked {
		t.Fatalf("the checked radio did not reach the backend: %+v", checked)
	}
	if sibling == nil || sibling.Checked {
		t.Fatalf("the sibling radio did not reach the backend: %+v", sibling)
	}
}
