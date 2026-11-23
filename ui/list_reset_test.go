package ui

import "testing"

// An app may reset its ListState between frames, as it reloads its rows.
// The last frame's elements still point into the state until the next build,
// and what reads them before it (assistive technology, a right-click, a drag
// choosing rows) must not panic.

func TestAccessAfterListStateReset(t *testing.T) {
	var list ListState
	sel := 0
	list.Selected = &sel
	tt := NewTester(func(c *Context) {
		List(c, &list, 3, func(i int) { Text(c, "Row "+string(rune('0'+i))) }).Grow(1)
	}, 300, 400)
	tt.Click("Row 1") // the focus on the list, which the tree follows to its row
	list = ListState{}
	tt.rt.accessTree()
	tt.Frame()
}

func TestRightClickAfterListStateReset(t *testing.T) {
	var list ListState
	var sel Selection[int]
	list.Selection = &sel
	tt := NewTester(func(c *Context) {
		List(c, &list, 3, func(i int) {
			Text(c, "Row "+string(rune('0'+i))).ContextMenu(func(m *Menu) { m.Item("Delete") })
		}).Grow(1)
	}, 300, 400)
	tt.Frame()
	x, y, err := tt.targetCenter("Row 1", 1)
	if err != nil {
		t.Fatal(err)
	}
	list = ListState{}
	tt.RightClickAt(x, y)
}

func TestDragSelectAfterListStateReset(t *testing.T) {
	var list ListState
	var sel Selection[int]
	list.Selection, list.DragSelect = &sel, true
	tt := NewTester(func(c *Context) {
		List(c, &list, 30, func(i int) { Text(c, "Row "+string(rune('0'+i))) }).Height(100)
	}, 300, 400)
	tt.Frame()
	x, y, err := tt.targetCenter("Row 1", 1)
	if err != nil {
		t.Fatal(err)
	}
	tt.Press(x, y)
	tt.Move(x, y+90) // at the bottom edge, which scrolls each frame
	tt.Frame()
	list = ListState{}
	tt.Frame()
}
