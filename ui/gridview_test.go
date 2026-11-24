package ui

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

func TestGridView(t *testing.T) {
	const n = 10000
	sel, opened, built := -1, -1, 0
	s := GridState{Selected: &sel, Label: func(i int) string { return fmt.Sprintf("Photo %d", i) }}
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Fill().Children(func() {
			if coreGridView(c, &s, n, 100, 80, func(i int) {
				built++
				coreTextf(c, "Photo %d", i)
			}).Grow(1).Submitted() {
				opened = sel
			}
		})
	}, 448, 400)
	// (448 + 8) / (100 + 8): 4 columns, each (448 - 3*8) / 4 = 106 wide.
	if s.cols != 4 {
		t.Fatalf("%d columns", s.cols)
	}
	built = 0
	tt.Frame()
	if built == 0 || built > 40 {
		t.Errorf("a frame built %d items of %d", built, n)
	}
	r, _ := tt.Find("Photo 5")
	tt.ClickAt(r.X, r.Y)
	if sel != 5 {
		t.Fatalf("a click chose %d", sel)
	}
	for _, step := range []struct {
		key  Key
		want int
	}{{KeyRight, 6}, {KeyDown, 10}, {KeyLeft, 9}, {KeyUp, 5}, {KeyUp, 1}, {KeyUp, 1}, {KeyEnd, n - 1}, {KeyHome, 0}} {
		tt.Key(0, step.key)
		if sel != step.want {
			t.Fatalf("%v chose %d, not %d", step.key, sel, step.want)
		}
	}
	tt.Key(0, KeyEnter)
	if opened != 0 {
		t.Errorf("Enter opened %d", opened)
	}
	// End shows the last item.
	tt.Key(0, KeyEnd)
	if !tt.HasText(fmt.Sprintf("Photo %d", n-1)) {
		t.Error("End did not show the last item")
	}
	// The cells share the width.
	cell := accessNode(t, func() *platform.AccessTree {
		tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
		return tt.h.access
	}(), platform.RoleListItem, fmt.Sprintf("Photo %d", n-1))
	if cell.Bounds.W != 106 || cell.Bounds.H != 80 {
		t.Errorf("a cell is %v", cell.Bounds)
	}
	// Assistive technology: a list of n items, the one chosen with the focus.
	tree := tt.h.access
	list := accessNode(t, tree, platform.RoleList, "")
	if list.SetSize != n || list.States&platform.AccessSelectable == 0 || cell.PosInSet != n || cell.SetSize != n || cell.States&platform.AccessChecked == 0 || tree.Focus != cell.ID {
		t.Errorf("the list %+v, the item %+v, the focus on %d", list, cell, tree.Focus)
	}
}

func TestGridViewChoosesSeveral(t *testing.T) {
	sel := -1
	var chosen Selection[string]
	names := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	s := GridState{Selected: &sel, Selection: &chosen, Key: func(i int) any { return names[i] }}
	tt := coreNewTester(func(c *context) {
		coreGridView(c, &s, len(names), 100, 60, func(i int) {
			coreText(c, names[i])
		}).Grow(1)
	}, 448, 300)
	click := func(mods Modifiers, name string) {
		r, _ := tt.Find(name)
		tt.ClickAtWith(mods, r.X, r.Y)
	}
	click(0, "b")
	click(Cmd, "d")
	if chosen.Len() != 2 || !chosen.Has("b") || !chosen.Has("d") {
		t.Fatalf("Cmd-click: %d chosen", chosen.Len())
	}
	click(Shift, "g")
	if chosen.Len() != 4 || !chosen.Has("e") || chosen.Has("b") {
		t.Fatalf("Shift-click from d to g: %d chosen", chosen.Len())
	}
	tt.Key(Shift, KeyUp)
	if sel != 2 || chosen.Len() != 2 || !chosen.Has("c") || !chosen.Has("d") {
		t.Fatalf("Shift-Up from g: chose %d, %d chosen", sel, chosen.Len())
	}
	tt.Key(Cmd, KeyA)
	if chosen.Len() != len(names) {
		t.Errorf("Cmd+A chose %d", chosen.Len())
	}
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	if n := accessNode(t, tt.h.access, platform.RoleList, ""); n.States&platform.AccessMultiselectable == 0 {
		t.Errorf("the list %+v", n)
	}
}

// TestGridViewShiftAfterItemsRemoved checks Shift with an arrow and a
// Shift-click after the app took items out, as those chosen deleted: the
// item Shift chose from may be past the items left, and the grid asks the
// app for the key of none of those.
func TestGridViewShiftAfterItemsRemoved(t *testing.T) {
	sel := -1
	var chosen Selection[string]
	var names []string
	s := GridState{Selected: &sel, Selection: &chosen, Key: func(i int) any {
		if i < 0 || i >= len(names) {
			t.Errorf("the grid asked for the key of item %d of %d", i, len(names))
			return ""
		}
		return names[i]
	}}
	tt := coreNewTester(func(c *context) {
		coreGridView(c, &s, len(names), 100, 60, func(i int) {
			coreText(c, names[i])
		}).Grow(1)
	}, 448, 300)
	click := func(mods Modifiers, name string) {
		t.Helper()
		r, ok := tt.Find(name)
		if !ok {
			t.Fatalf("no item %q", name)
		}
		tt.ClickAtWith(mods, r.X, r.Y)
	}
	all := func() {
		names = []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"}
		chosen.Clear()
		tt.Frame()
	}

	// Shift chose b to j from j, which the app deletes: k, now the second
	// item, is chosen alone, and Shift with an arrow chooses from there.
	all()
	click(0, "j")
	click(Shift, "b")
	if sel != 1 || chosen.Len() != 9 {
		t.Fatalf("Shift-click from j to b: chose %d, %d chosen", sel, chosen.Len())
	}
	names = []string{"a", "k", "l"}
	chosen.Clear()
	tt.Frame()
	tt.Key(Shift, KeyRight)
	if sel != 2 || chosen.Len() != 1 || !chosen.Has("l") {
		t.Errorf("Shift-Right after the items chosen went: chose %d, %d chosen", sel, chosen.Len())
	}
	tt.Key(Shift, KeyLeft)
	if sel != 1 || chosen.Len() != 2 || !chosen.Has("k") || !chosen.Has("l") {
		t.Errorf("Shift-Left then: chose %d, %d chosen", sel, chosen.Len())
	}

	// The item clicked is past the items left.
	all()
	click(0, "j")
	names = names[:3]
	chosen.Clear()
	tt.Frame()
	tt.Key(Shift, KeyRight)
	if sel != 0 || chosen.Len() != 1 || !chosen.Has("a") {
		t.Errorf("Shift-Right with the item chosen gone: chose %d, %d chosen", sel, chosen.Len())
	}

	// A Shift-click then.
	all()
	click(0, "j")
	names = names[:3]
	chosen.Clear()
	tt.Frame()
	click(Shift, "c")
	if sel != 2 || chosen.Len() != 1 || !chosen.Has("c") {
		t.Errorf("Shift-click with the item chosen gone: chose %d, %d chosen", sel, chosen.Len())
	}
}

func TestGridViewResizes(t *testing.T) {
	sel := 13
	s := GridState{Selected: &sel}
	tt := coreNewTester(func(c *context) {
		coreGridView(c, &s, 100, 100, 80, func(i int) {
			coreTextf(c, "Item %d", i)
		}).Grow(1)
	}, 448, 300)
	if s.cols != 4 {
		t.Fatalf("%d columns", s.cols)
	}
	tt.SetSize(664, 300)
	// (664 + 8) / 108: 6 columns.
	if s.cols != 6 {
		t.Fatalf("after widening, %d columns", s.cols)
	}
	tt.Click("Item 13")
	tt.Key(0, KeyDown)
	if sel != 19 {
		t.Errorf("Down from 13 in 6 columns chose %d", sel)
	}
}

func TestGridViewTypeToChoose(t *testing.T) {
	names := []string{"Apple", "Banana", "Cherry", "Chestnut", "Avocado", "Citrus"}
	sel := -1
	s := GridState{Selected: &sel, Label: func(i int) string { return names[i] }}
	tt := coreNewTester(func(c *context) {
		coreGridView(c, &s, len(names), 100, 60, func(i int) { coreText(c, names[i]) }).Grow(1).AutoFocus()
	}, 448, 400)
	tt.Key(0, KeyA)
	if sel != 0 {
		t.Fatalf("A with no item chosen went to %d", sel)
	}
	pause := func() {
		for _, st := range tt.rt.states {
			st.typedAt = time.Time{}
		}
	}
	pause()
	// The same letter again goes to the next item starting with it.
	var went []int
	for range 4 {
		tt.Key(0, KeyC)
		went = append(went, sel)
	}
	if !slices.Equal(went, []int{2, 3, 5, 2}) {
		t.Fatalf("c, c, c, c went to %v", went)
	}
	// Letters typed quickly add up.
	pause()
	went = went[:0]
	for _, k := range []Key{KeyC, KeyH, KeyE, KeyR} {
		tt.Key(0, k)
		went = append(went, sel)
	}
	if !slices.Equal(went, []int{3, 3, 3, 2}) {
		t.Fatalf("c, h, e, r went to %v", went)
	}
	// The app took items out from under the choice: from the first again.
	pause()
	sel = 10
	tt.Key(0, KeyA)
	if sel != 0 {
		t.Fatalf("A with item 10 of 6 chosen went to %d", sel)
	}
}
