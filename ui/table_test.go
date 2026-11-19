package ui

import (
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

// tableView shows a table of files: name, kind and size.
func tableView(s *ListState, cols []TableColumn, names []string) func(c *context) {
	return func(c *context) {
		coreColumn(c).Fill().Padding(10).Children(func() {
			coreTable(c, s, cols, len(names), func(row, col int) {
				switch col {
				case 0:
					coreText(c, names[row]).SingleLine()
				case 1:
					coreText(c, "Document")
				case 2:
					coreTextf(c, "%d KB", row+1)
				}
			}).Grow(1)
		})
	}
}

func TestTableSort(t *testing.T) {
	sort := SortOrder{Column: "Size"}
	sel := -1
	s := ListState{Sort: &sort, Selected: &sel}
	cols := []TableColumn{{Title: "Name", Sortable: true}, {Title: "Kind", Width: 100}, {Title: "Size", Width: 80, Align: End, Sortable: true}}
	tt := coreNewTester(tableView(&s, cols, []string{"a.txt", "b.txt"}), 500, 300)
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	if n := accessNode(t, tt.h.access, platform.RoleColumnHeader, "Size"); n.States&platform.AccessSortAscending == 0 {
		t.Errorf("the column sorted by: %+v", n)
	}
	tt.Click("Name")
	if sort != (SortOrder{Column: "Name"}) {
		t.Fatalf("a click on Name: %+v", sort)
	}
	tt.Click("Name")
	if sort != (SortOrder{Column: "Name", Descending: true}) {
		t.Fatalf("a second click on Name: %+v", sort)
	}
	tree := tt.h.access
	if n := accessNode(t, tree, platform.RoleColumnHeader, "Name"); n.States&platform.AccessSortDescending == 0 || n.Actions&platform.ActionPress == 0 {
		t.Errorf("Name, sorted descending: %+v", n)
	}
	if n := accessNode(t, tree, platform.RoleColumnHeader, "Size"); n.States&(platform.AccessSortAscending|platform.AccessSortDescending) != 0 {
		t.Errorf("Size, no longer sorted by: %+v", n)
	}
	// A column that does not sort.
	tt.Click("Kind")
	if sort != (SortOrder{Column: "Name", Descending: true}) || sel != -1 {
		t.Errorf("a click on Kind: %+v, chose %d", sort, sel)
	}
}

// header returns the box of a column's header.
func header(t *testing.T, tt *Tester, title string) platform.RectF {
	t.Helper()
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	return accessNode(t, tt.h.access, platform.RoleColumnHeader, title).Bounds
}

func TestTableResize(t *testing.T) {
	s := ListState{}
	cols := []TableColumn{{Title: "Name"}, {Title: "Kind", Width: 100, MinWidth: 60}, {Title: "Size", Width: 80, Fixed: true}}
	tt := coreNewTester(tableView(&s, cols, []string{"a.txt", "a much longer name.txt"}), 500, 300)
	kind := header(t, tt, "Kind")
	// Dragging the right edge of Kind's header.
	edge := float32(kind.X+kind.W) - 2
	tt.Press(edge, float32(kind.Y+kind.H/2))
	tt.Move(edge+30, float32(kind.Y+kind.H/2))
	tt.Release(edge+30, float32(kind.Y+kind.H/2))
	if w, h := s.Columns.Widths["Kind"], header(t, tt, "Kind"); w != 130 || h.W != 130 || float32(h.X+h.W)-2 != edge+30 {
		t.Fatalf("dragged 30 DIPs wider: %v, the header %v", w, h)
	}
	// The column before it, sharing the room left, kept its width, for the
	// edge to follow the pointer.
	if w := s.Columns.Widths["Name"]; w != 300 {
		t.Errorf("Name took %v", w)
	}
	edge += 30
	// No narrower than its least.
	tt.Press(edge, float32(kind.Y+kind.H/2))
	tt.Move(edge-130, float32(kind.Y+kind.H/2))
	tt.Release(edge-130, float32(kind.Y+kind.H/2))
	if w := header(t, tt, "Kind").W; w != 60 {
		t.Errorf("dragged narrower than its least: %v", w)
	}
	// A double click fits it to its cells, and the cells follow.
	kind = header(t, tt, "Kind")
	edge = float32(kind.X+kind.W) - 2
	tt.ClickAt(edge, float32(kind.Y+kind.H/2))
	tt.ClickAt(edge, float32(kind.Y+kind.H/2))
	doc, _ := tt.Find("Document")
	if w := float32(header(t, tt, "Kind").W); w < doc.W+20 || w > doc.W+21 || w < 70 {
		t.Errorf("fitted to %v for a text of %v", w, doc.W)
	}
	// A fixed column has no edge to drag.
	size := header(t, tt, "Size")
	tt.Press(float32(size.X+size.W-1), float32(size.Y+size.H/2))
	tt.Move(float32(size.X+size.W+40), float32(size.Y+size.H/2))
	tt.Release(float32(size.X+size.W+40), float32(size.Y+size.H/2))
	if w := header(t, tt, "Size").W; w != 80 {
		t.Errorf("a fixed column resized to %v", w)
	}
}

func TestTableReorder(t *testing.T) {
	sort := SortOrder{}
	s := ListState{Sort: &sort}
	cols := []TableColumn{{Title: "Name", Fixed: true}, {Title: "Kind", Width: 100, Sortable: true}, {Title: "Size", Width: 80}}
	tt := coreNewTester(tableView(&s, cols, []string{"a.txt"}), 500, 300)
	kind := header(t, tt, "Kind")
	y := float32(kind.Y + kind.H/2)
	// Kind dragged past the middle of Size.
	x := float32(kind.X + 20)
	tt.Press(x, y)
	for _, d := range []float32{5, 20, 50, 60} {
		tt.Move(x+d, y)
	}
	tt.Release(x+60, y)
	if got := s.Columns.Order; len(got) != 3 || got[1] != "Size" || got[2] != "Kind" {
		t.Fatalf("the order: %q", got)
	}
	if sort.Column != "" {
		t.Errorf("dragging the header sorted by %q", sort.Column)
	}
	kind, size := header(t, tt, "Kind"), header(t, tt, "Size")
	doc, _ := tt.Find("Document")
	if !(size.X < kind.X) || float64(doc.X) < kind.X {
		t.Errorf("Size at %v, Kind at %v, its cell at %v", size.X, kind.X, doc.X)
	}
	// Name is fixed: it neither moves nor lets others past it.
	tt.Press(float32(size.X+10), y)
	for _, d := range []float32{-5, -40, -100, -200} {
		tt.Move(float32(size.X+10)+d, y)
	}
	tt.Release(float32(size.X+10)-200, y)
	if got := s.Columns.Order; got[0] != "Name" {
		t.Errorf("moved past a fixed column: %q", got)
	}
	// The release's own move moves the column too, and is no click.
	x = float32(header(t, tt, "Kind").X + 20)
	tt.Press(x, y)
	tt.Move(x-5, y)
	tt.Move(x-20, y)
	tt.Release(x-60, y)
	if got := s.Columns.Order; got[1] != "Kind" || got[2] != "Size" {
		t.Errorf("Kind let go past the middle of Size: %q", got)
	}
	if sort.Column != "" {
		t.Errorf("dragging the header sorted by %q", sort.Column)
	}
}

// Changed reports a column moved or resized once the user lets go, when
// the layout is new, and a column fitted to its cells, not as the pointer
// moves: the app saves ListState.Columns once per gesture.
func TestTableColumnsChanged(t *testing.T) {
	s := ListState{}
	cols := []TableColumn{{Title: "Name"}, {Title: "Kind", Width: 100}, {Title: "Size", Width: 80}}
	names := []string{"a.txt"}
	changes := 0
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Fill().Padding(10).Children(func() {
			if coreTable(c, &s, cols, len(names), func(row, col int) {
				coreText(c, []string{names[row], "Document", "1 KB"}[col]).SingleLine()
			}).Grow(1).Changed() {
				changes++
			}
		})
	}, 500, 300)
	expect := func(n int, what string) {
		t.Helper()
		if changes != n {
			t.Fatalf("%s: Changed reported %d times, want %d (%+v)", what, changes, n, s.Columns)
		}
	}
	kind := header(t, tt, "Kind")
	y := float32(kind.Y + kind.H/2)

	// Resizing Kind by its edge: reported on release only.
	edge := float32(kind.X+kind.W) - 2
	tt.Press(edge, y)
	tt.Move(edge+10, y)
	tt.Move(edge+30, y)
	expect(0, "resizing, held")
	tt.Release(edge+30, y)
	expect(1, "resized")
	if w := s.Columns.Widths["Kind"]; w != 130 {
		t.Fatalf("Kind is %v wide", w)
	}
	// A press on the edge that comes back where it started changes nothing.
	edge += 30
	tt.Press(edge, y)
	tt.Move(edge+20, y)
	tt.Move(edge, y)
	tt.Release(edge, y)
	expect(1, "resized and back")

	// A double click on the edge fits the column to its cells: one change,
	// and none for a column already fitted.
	tt.ClickAt(edge, y)
	tt.ClickAt(edge, y)
	expect(2, "fitted")
	kind = header(t, tt, "Kind")
	edge = float32(kind.X+kind.W) - 2
	tt.ClickAt(edge, y)
	tt.ClickAt(edge, y)
	expect(2, "fitted again")

	// Moving Kind past Size: reported on release only.
	x := float32(kind.X + 20)
	tt.Press(x, y)
	for _, d := range []float32{5, 20, 50, 60} {
		tt.Move(x+d, y)
	}
	if got := s.Columns.Order; len(got) != 3 || got[2] != "Kind" {
		t.Fatalf("the order while held: %q", got)
	}
	expect(2, "moving, held")
	tt.Release(x+60, y)
	expect(3, "moved")
	// A drag that leaves the column where it was changes nothing.
	x = float32(header(t, tt, "Kind").X + 20)
	tt.Press(x, y)
	tt.Move(x-5, y)
	tt.Move(x-10, y)
	tt.Release(x-10, y)
	if got := s.Columns.Order; got[2] != "Kind" {
		t.Fatalf("Kind moved: %q", got)
	}
	expect(3, "dragged in place")
	// Nor does a click on a header that does not sort.
	tt.Click("Size")
	expect(3, "clicked")
}

// A column dragged past several others stays the one dragged: its header
// keeps the press as the others move aside.
func TestTableReorderFar(t *testing.T) {
	s := ListState{}
	cols := []TableColumn{{Title: "Name"}, {Title: "Kind", Width: 80}, {Title: "Size", Width: 80}, {Title: "Date", Width: 80}}
	tt := coreNewTester(tableView(&s, cols, []string{"a.txt"}), 600, 300)
	kind := header(t, tt, "Kind")
	y := float32(kind.Y + kind.H/2)
	x := float32(kind.X + 20)
	tt.Press(x, y)
	for d := float32(5); d <= 180; d += 15 {
		tt.Move(x+d, y)
	}
	tt.Release(x+180, y)
	if got := s.Columns.Order; len(got) != 4 || got[1] != "Size" || got[2] != "Date" || got[3] != "Kind" {
		t.Fatalf("the order after dragging Kind past Size and Date: %q", got)
	}
}

// The last column sharing the room left ends at the table's end: its
// header has no edge, as AppKit's. A last column with a width keeps its.
func TestTableLastColumnEdge(t *testing.T) {
	// edge reports whether the end of a column's header resizes it, its
	// pointer moving there and dragging it 30 DIPs out.
	edge := func(tt *Tester, s *ListState, title string, rtl bool) bool {
		h := header(t, tt, title)
		x, y, dx := float32(h.X+h.W)-2, float32(h.Y+h.H/2), float32(-30)
		if rtl {
			x, dx = float32(h.X)+2, 30
		}
		tt.Move(x, y)
		cursor := tt.Cursor() == CursorResizeEW
		before := header(t, tt, title).W
		tt.Press(x, y)
		tt.Move(x+dx, y)
		tt.Release(x+dx, y)
		after := header(t, tt, title).W
		if cursor != (after != before) {
			t.Errorf("%s: the resize pointer %v, but dragged from %v to %v", title, cursor, before, after)
		}
		return cursor
	}
	for _, rtl := range []bool{false, true} {
		s := ListState{}
		cols := []TableColumn{{Title: "Name", Width: 150}, {Title: "Kind", Width: 100}, {Title: "Size"}}
		tt := coreNewTester(tableView(&s, cols, []string{"a.txt"}), 500, 300)
		if rtl {
			tt.SetDirection(RightToLeft)
		}
		if edge(tt, &s, "Size", rtl) {
			t.Errorf("rtl %v: the last column, sharing the room left, has an edge", rtl)
		}
		if _, ok := s.Columns.Widths["Size"]; ok {
			t.Errorf("rtl %v: the last column took a width: %v", rtl, s.Columns.Widths)
		}
		if !edge(tt, &s, "Kind", rtl) {
			t.Errorf("rtl %v: a column with a width before it has no edge", rtl)
		}
		// Moved before the others, it has its edge back, and the last one,
		// with a width, keeps its own.
		s.Columns.Order = []string{"Size", "Name", "Kind"}
		tt.Frame()
		if !edge(tt, &s, "Size", rtl) || !edge(tt, &s, "Kind", rtl) {
			t.Errorf("rtl %v: moved first, Size or the last column, Kind, has no edge", rtl)
		}
	}
}

func TestTableScrollsSideways(t *testing.T) {
	s := ListState{}
	cols := []TableColumn{{Title: "Name", Width: 200}, {Title: "Kind", Width: 200}, {Title: "Size", Width: 200}}
	names := make([]string, 50)
	for i := range names {
		names[i] = "file"
	}
	tt := coreNewTester(tableView(&s, cols, names), 400, 300)
	if tt.HasText("Size") {
		size := header(t, tt, "Size")
		if size.X < 390 {
			t.Fatalf("the third column of 200 shows at %v in a table 380 wide", size.X)
		}
	}
	tt.Scroll(200, 150, 300, 0)
	size := header(t, tt, "Size")
	cell, _ := tt.Find("1 KB")
	if size.X > 300 || float64(cell.X) != size.X+10 {
		t.Errorf("scrolled sideways: the header at %v, the text of its cell at %v", size.X, cell.X)
	}
	// Down still moves the rows, and the header stays.
	before := size.Y
	tt.Scroll(200, 150, 0, 300)
	if size := header(t, tt, "Size"); size.Y != before || s.first == 0 {
		t.Errorf("scrolled down: the header at %v (was %v), first row %d", size.Y, before, s.first)
	}
}

func TestEditableTextInTable(t *testing.T) {
	names := []string{"notes.txt", "photo.jpeg", "README"}
	sel, opened, renames := -1, -1, 0
	s := ListState{Selected: &sel}
	cols := []TableColumn{{Title: "Name"}, {Title: "Size", Width: 80}}
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Fill().Padding(10).Children(func() {
			if coreTable(c, &s, cols, len(names), func(row, col int) {
				if col == 0 {
					if coreEditableText(c, &names[row]).Changed() {
						renames++
					}
				} else {
					coreText(c, "1 KB")
				}
			}).Grow(1).Submitted() {
				opened = sel
			}
		})
	}, 400, 300)
	rename := func() {
		if runtime.GOOS == "darwin" {
			tt.Key(0, KeyEnter)
		} else {
			tt.Key(0, KeyF2)
		}
	}
	tt.Click("photo.jpeg")
	rename()
	if opened != -1 {
		t.Fatalf("the key editing the name opened the row")
	}
	// The name before its extension is selected: typing replaces it.
	tt.Type("beach")
	tt.Key(0, KeyEnter)
	if names[1] != "beach.jpeg" || renames != 1 {
		t.Fatalf("renamed to %q (%d)", names[1], renames)
	}
	if !tt.Focused("beach.jpeg") {
		t.Error("the table did not take the focus back")
	}
	// Escape goes back.
	tt.Key(0, KeyF2)
	tt.Type("gone")
	tt.Key(0, KeyEscape)
	if names[1] != "beach.jpeg" || !tt.HasText("beach.jpeg") {
		t.Errorf("Escape left %q", names[1])
	}
	// A click on the name of the row chosen edits it, once a double click
	// is out of the question; the focus leaving keeps what was typed.
	tt.Click("beach.jpeg")
	if tt.HasText("beach.jpeg") != true {
		t.Fatal("the click edited at once")
	}
	tt.Advance(renameDelay + 50*time.Millisecond)
	tt.Type("sea")
	tt.Click("notes.txt")
	if names[1] != "sea.jpeg" || sel != 0 {
		t.Errorf("after a click elsewhere: %q, chose %d", names[1], sel)
	}
	// A double click opens, and does not edit.
	tt.Click("notes.txt")
	tt.Click("notes.txt")
	tt.Advance(renameDelay + 50*time.Millisecond)
	tt.Type("x")
	if opened != 0 || names[0] != "notes.txt" {
		t.Errorf("a double click: opened %d, the name %q", opened, names[0])
	}
}

func TestEditableText(t *testing.T) {
	title := "Untitled.md"
	changes := 0
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Gap(8).Children(func() {
			if coreEditableText(c, &title).Changed() {
				changes++
			}
			coreButton(c, "Other")
		})
	}, 400, 300)
	r, _ := tt.Find("Untitled.md")
	tt.ClickAt(r.X+5, r.Y+5)
	tt.ClickAt(r.X+5, r.Y+5)
	tt.Type("Notes")
	tt.Key(0, KeyTab)
	if title != "Notes.md" || changes != 1 {
		t.Fatalf("after Tab: %q (%d)", title, changes)
	}
	tt.Click("Notes.md")
	tt.Key(0, KeyEnter)
	tt.Key(Cmd, KeyA)
	tt.Type("Plan")
	tt.Key(0, KeyEnter)
	if title != "Plan" || !tt.Focused("Plan") {
		t.Errorf("after Enter: %q, focused %v", title, tt.Focused("Plan"))
	}
}

func TestTableRoundRows(t *testing.T) {
	names := []string{"a.txt", "b.txt", "c.txt"}
	cols := []TableColumn{{Title: "Name"}, {Title: "Kind", Width: 100}, {Title: "Size", Width: 80, Align: End}}
	// radius returns the corners of row i's box, or of the box inside it
	// that shows the pointer over it in a table that does not choose.
	radius := func(s *ListState, i int, inner bool) [4]float32 {
		t.Helper()
		for _, r := range s.frame.rows {
			if r.i == i {
				if inner {
					return r.e.first.radius
				}
				return r.e.radius
			}
		}
		t.Fatalf("row %d not built", i)
		return [4]float32{}
	}
	r := LightTheme().Radius
	square, round := [4]float32{}, [4]float32{r, r, r, r}

	sel := 1
	s := ListState{Selected: &sel}
	coreNewTester(tableView(&s, cols, names), 500, 300)
	if got := radius(&s, 1, false); got != square {
		t.Errorf("a chosen row by default: %v", got)
	}
	s = ListState{Selected: &sel, RoundRows: true}
	coreNewTester(tableView(&s, cols, names), 500, 300)
	if got := radius(&s, 1, false); got != round {
		t.Errorf("a chosen row with RoundRows: %v", got)
	}

	// Rows chosen together make one block.
	var chosen Selection[string]
	chosen.Add("a.txt")
	chosen.Add("b.txt")
	s = ListState{Selection: &chosen, Key: func(i int) any { return names[i] }, RoundRows: true}
	coreNewTester(tableView(&s, cols, names), 500, 300)
	if got := radius(&s, 0, false); got != [4]float32{r, r, 0, 0} {
		t.Errorf("the first of two rows chosen: %v", got)
	}
	if got := radius(&s, 1, false); got != [4]float32{0, 0, r, r} {
		t.Errorf("the second of two rows chosen: %v", got)
	}

	// A table that does not choose shows the pointer over its rows.
	s = ListState{}
	coreNewTester(tableView(&s, cols, names), 500, 300)
	if got := radius(&s, 0, true); got != square {
		t.Errorf("a row of a table that does not choose, by default: %v", got)
	}
	s = ListState{RoundRows: true}
	coreNewTester(tableView(&s, cols, names), 500, 300)
	if got := radius(&s, 0, true); got != round {
		t.Errorf("a row of a table that does not choose, with RoundRows: %v", got)
	}
}

// With SortBy, a Cmd-click sorts by several columns: it adds a column,
// ascending, after those sorted by, makes it descending, then takes it
// out. A click sorts by one column again.
func TestTableSortBy(t *testing.T) {
	var by []SortOrder
	s := ListState{SortBy: &by}
	cols := []TableColumn{{Title: "Name", Sortable: true}, {Title: "Kind", Width: 100, Sortable: true}, {Title: "Size", Width: 80, Sortable: true}}
	tt := coreNewTester(tableView(&s, cols, []string{"a.txt", "b.txt"}), 500, 300)
	check := func(step string, want ...SortOrder) {
		t.Helper()
		if !slices.Equal(by, want) {
			t.Fatalf("%s: %+v, want %+v", step, by, want)
		}
	}
	tt.Click("Name")
	check("a click on Name", SortOrder{Column: "Name"})
	tt.ClickWith(Cmd, "Kind")
	check("a Cmd-click on Kind", SortOrder{Column: "Name"}, SortOrder{Column: "Kind"})
	kept := by
	tt.ClickWith(Cmd, "Size")
	check("a Cmd-click on Size", SortOrder{Column: "Name"}, SortOrder{Column: "Kind"}, SortOrder{Column: "Size"})
	if len(kept) != 2 {
		t.Fatalf("a click changed the slice the app kept: %+v", kept)
	}
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	if n := accessNode(t, tt.h.access, platform.RoleColumnHeader, "Size"); n.States&platform.AccessSortAscending == 0 || n.Label != "Size" {
		t.Errorf("Size, sorted by after Name and Kind: %+v", n)
	}
	tt.ClickWith(Cmd, "Kind")
	check("a second Cmd-click on Kind", SortOrder{Column: "Name"}, SortOrder{Column: "Kind", Descending: true}, SortOrder{Column: "Size"})
	tt.ClickWith(Cmd, "Kind")
	check("a third Cmd-click on Kind", SortOrder{Column: "Name"}, SortOrder{Column: "Size"})
	// The first column goes the same way, the next taking its place.
	tt.ClickWith(Cmd, "Name")
	check("a Cmd-click on Name", SortOrder{Column: "Name", Descending: true}, SortOrder{Column: "Size"})
	tt.ClickWith(Cmd, "Name")
	check("a second Cmd-click on Name", SortOrder{Column: "Size"})
	// A click sorts by one column again, and on the first reverses it.
	tt.ClickWith(Cmd, "Kind")
	tt.Click("Kind")
	check("a click on Kind", SortOrder{Column: "Kind"})
	tt.ClickWith(Cmd, "Name")
	tt.Click("Kind")
	check("a click on Kind, first", SortOrder{Column: "Kind", Descending: true})
}

func TestTableHeaderMenu(t *testing.T) {
	built, chosen := -1, -1
	s := ListState{Columns: TableLayout{Order: []string{"Size", "Name", "Kind"}}}
	cols := []TableColumn{{Title: "Name"}, {Title: "Kind", Width: 100}, {Title: "Size", Width: 80}}
	tt := coreNewTester(tableView(&s, cols, []string{"a.txt"}), 500, 300)
	// Without HeaderMenu, a header has no menu.
	tt.RightClick("Kind")
	if m := tt.Menu(); m != nil {
		t.Fatalf("a menu without HeaderMenu: %q", m)
	}
	s.HeaderMenu = func(m *Menu, col int) {
		built = col
		if m.Item("Hide Column").Chosen() {
			chosen = col
		}
	}
	tt.Frame()
	// Kind shows third, and is the second of the columns.
	tt.RightClick("Kind")
	if m := tt.Menu(); len(m) != 1 || m[0] != "Hide Column" || built != 1 {
		t.Fatalf("the menu %q, built for column %d", m, built)
	}
	if err := tt.ChooseMenuItem("Hide Column"); err != nil {
		t.Fatal(err)
	}
	if chosen != 1 {
		t.Errorf("chose the item for column %d", chosen)
	}
}

// Set with SortBy, Sort follows its first column, for the app to read
// the column that sorts first; a Shift-click adds a column as a Cmd-click
// does, as web grids do.
func TestTableSortFollowsSortBy(t *testing.T) {
	var by []SortOrder
	var sort SortOrder
	s := ListState{Sort: &sort, SortBy: &by}
	cols := []TableColumn{{Title: "Name", Sortable: true}, {Title: "Kind", Width: 100, Sortable: true}}
	tt := coreNewTester(tableView(&s, cols, []string{"a.txt", "b.txt"}), 500, 300)
	check := func(step string, first SortOrder, want ...SortOrder) {
		t.Helper()
		if !slices.Equal(by, want) || sort != first {
			t.Fatalf("%s: SortBy %+v, Sort %+v; want %+v, %+v", step, by, sort, want, first)
		}
	}
	tt.Click("Name")
	check("a click on Name", SortOrder{Column: "Name"}, SortOrder{Column: "Name"})
	tt.ClickWith(Shift, "Kind")
	check("a Shift-click on Kind", SortOrder{Column: "Name"}, SortOrder{Column: "Name"}, SortOrder{Column: "Kind"})
	tt.ClickWith(Cmd, "Name")
	tt.ClickWith(Shift, "Name")
	check("Name taken out", SortOrder{Column: "Kind"}, SortOrder{Column: "Kind"})
	tt.ClickWith(Shift, "Kind")
	tt.ClickWith(Shift, "Kind")
	check("sorted by none", SortOrder{})
	// The app setting SortBy itself.
	by = []SortOrder{{Column: "Kind", Descending: true}}
	tt.Frame()
	check("SortBy set by the app", SortOrder{Column: "Kind", Descending: true}, SortOrder{Column: "Kind", Descending: true})
}

// The header past the columns has HeaderMenu's menu for column -1, which
// does not open when it has no items.
func TestTableHeaderMenuPastTheColumns(t *testing.T) {
	built := 0
	items := true
	s := ListState{HeaderMenu: func(m *Menu, col int) {
		built = col
		if items {
			m.Item("Reset Columns")
		}
	}}
	cols := []TableColumn{{Title: "Name", Width: 120}, {Title: "Kind", Width: 100}}
	tt := coreNewTester(tableView(&s, cols, []string{"a.txt"}), 500, 300)
	kind, _ := tt.Find("Kind")
	tt.RightClickAt(450, kind.Y+kind.H/2)
	if m := tt.Menu(); len(m) != 1 || m[0] != "Reset Columns" || built != -1 {
		t.Fatalf("past the columns: the menu %q, built for column %d", m, built)
	}
	tt.CloseMenu()
	items = false
	tt.RightClickAt(450, kind.Y+kind.H/2)
	if m := tt.Menu(); m != nil {
		t.Fatalf("a menu without items opened: %q", m)
	}
	items = true
	tt.RightClick("Kind")
	if built != 1 {
		t.Fatalf("a column's header built the menu for column %d", built)
	}
}

// CellMenu gives the cells a context menu over the whole cell, its padding
// too, told the row and the column as the cell builder receives them.
func TestTableCellMenu(t *testing.T) {
	var s ListState
	cols := []TableColumn{{Title: "A", Width: 100, Fixed: true}, {Title: "B", Width: 100, Fixed: true}}
	gotRow, gotCol := -1, -1
	s.CellMenu = func(m *Menu, row, col int) {
		gotRow, gotCol = row, col
		if col == 1 {
			m.Item("Open")
		}
	}
	tt := NewTester(func(c *Context) {
		Table(c, &s, cols, 4, func(row, col int) { Textf(c, "r%dc%d", row, col) }).Grow(1)
	}, 400, 300)
	tt.Frame()
	r, _ := tt.Find("r2c1")
	tt.RightClickAt(r.X+r.W+20, r.Y+r.H/2) // past the text, in the cell
	if m := tt.Menu(); len(m) != 1 || m[0] != "Open" || gotRow != 2 || gotCol != 1 {
		t.Fatalf("right-click in r2c1 past its text: menu %q, row %d, col %d", m, gotRow, gotCol)
	}
	tt.CloseMenu()
	r, _ = tt.Find("r1c0")
	tt.RightClickAt(r.X+2, r.Y+r.H/2)
	if m := tt.Menu(); m != nil || gotRow != 1 || gotCol != 0 {
		t.Fatalf("a menu built without items opened: %q (row %d, col %d)", m, gotRow, gotCol)
	}
}

// HeaderCell gets each column's header cell as the table builds it: what
// it builds goes in the cell after the title, and col is the column's
// index in the columns given to Table.
func TestTableHeaderCell(t *testing.T) {
	hovered := -1
	s := ListState{}
	cols := []TableColumn{{Title: "Name"}, {Title: "Kind", Width: 100}, {Title: "Size", Width: 80}}
	tt := coreNewTester(func(c *context) {
		s.HeaderCell = func(cell Element, col int) {
			coreTextf(c, "mark %d", col)
			if cell.Hovered() {
				hovered = col
			}
		}
		coreColumn(c).Fill().Padding(10).Children(func() {
			coreTable(c, &s, cols, 1, func(row, col int) { coreText(c, "a.txt") }).Grow(1)
		})
	}, 500, 300)
	if !tt.HasText("mark 0") || !tt.HasText("mark 2") {
		t.Fatalf("marks: %q", tt.Texts())
	}
	name, _ := tt.Find("Name")
	mark, _ := tt.Find("mark 0")
	if mark.X < name.X+name.W || mark.Y > name.Y+name.H {
		t.Errorf("the mark at %+v, not after the title at %+v", mark, name)
	}
	size, _ := tt.Find("Size")
	tt.Move(size.X+2, size.Y+size.H/2)
	tt.Frame()
	if hovered != 2 {
		t.Fatalf("hovered %d", hovered)
	}
}

// With RowHeight every row is that high, a short one as a tall one would
// be; the header keeps the theme's height.
func TestTableRowHeight(t *testing.T) {
	names := []string{"a.txt", "b.txt", "c.txt"}
	cols := []TableColumn{{Title: "Name"}, {Title: "Size", Width: 80, Align: End}}
	s := ListState{}
	tt := coreNewTester(tableView(&s, cols, names), 500, 300)
	r0, _ := rowBox(tt, &s, 0)
	if want := LightTheme().Space(8); r0.H != want {
		t.Fatalf("a row by default is %v high, want %v", r0.H, want)
	}
	s = ListState{RowHeight: 22}
	tt = coreNewTester(tableView(&s, cols, names), 500, 300)
	for i := range names {
		r, _ := rowBox(tt, &s, i)
		if r.H != 22 {
			t.Errorf("row %d is %v high, want 22", i, r.H)
		}
		if i > 0 {
			prev, _ := rowBox(tt, &s, i-1)
			if r.Y != prev.Y+22 {
				t.Errorf("row %d starts at %v, after one at %v", i, r.Y, prev.Y)
			}
		}
	}
	// The header above the first row keeps the theme's height.
	if r, _ := rowBox(tt, &s, 0); r.Y != r0.Y {
		t.Errorf("the first row starts at %v, not %v below the header", r.Y, r0.Y)
	}
	if !tt.HasText("b.txt") {
		t.Errorf("texts %q", tt.Texts())
	}
}

// wideTable shows a table of n rows and count columns of width wide, the
// first of them frozen of those that frozen says, whose cell r2c5 is row 2's
// in column 5, and counts the cells built, by column.
func wideTable(s *ListState, count int, width float32, frozen int, built map[int]int) func(c *context) {
	cols := make([]TableColumn, count)
	for j := range cols {
		cols[j] = TableColumn{Title: "H" + strconv.Itoa(j), Width: width, Frozen: j < frozen}
	}
	return func(c *context) {
		coreColumn(c).Fill().Children(func() {
			coreTable(c, s, cols, 20, func(row, col int) {
				if built != nil {
					built[col]++
				}
				coreText(c, "r"+strconv.Itoa(row)+"c"+strconv.Itoa(col)).SingleLine()
			}).Grow(1)
		})
	}
}

// builtColumns returns the columns whose cells were built, in order.
func builtColumns(built map[int]int) []int {
	var cols []int
	for j := range built {
		cols = append(cols, j)
	}
	slices.Sort(cols)
	return cols
}

func TestTableVirtualColumnsBuildsThoseInView(t *testing.T) {
	for _, virtual := range []bool{false, true} {
		built := map[int]int{}
		s := ListState{VirtualColumns: virtual}
		tt := coreNewTester(wideTable(&s, 100, 100, 0, built), 400, 300)
		tt.Frame()
		clear(built)
		tt.Frame()
		cols := builtColumns(built)
		switch {
		case !virtual && len(cols) != 100:
			t.Errorf("off: built %d columns, want all", len(cols))
		case virtual && (cols[0] != 0 || cols[len(cols)-1] > 8 || len(cols) < 4):
			t.Errorf("on, at the start: built columns %v", cols)
		}
	}
}

// The columns in view are those built as the table scrolls, and the
// headers and the cells stay as aligned as with every column built.
func TestTableVirtualColumnsScrolls(t *testing.T) {
	built := map[int]int{}
	s := ListState{VirtualColumns: true}
	tt := coreNewTester(wideTable(&s, 100, 100, 0, built), 400, 300)
	tt.Frame()
	refS := ListState{}
	ref := coreNewTester(wideTable(&refS, 100, 100, 0, nil), 400, 300)
	for _, dx := range []float32{3000, 300, -1000, 6000, 20000, -20000} {
		tt.Scroll(200, 150, dx, 0)
		ref.Scroll(200, 150, dx, 0)
		tt.Frame()
		ref.Frame()
		if scrollXOf(&s) != scrollXOf(&refS) {
			t.Fatalf("scrolled by %v: at %v, want %v", dx, scrollXOf(&s), scrollXOf(&refS))
		}
		x := int(scrollXOf(&s) / 100)
		for _, j := range []int{x, x + 1, x + 2} {
			name := "r2c" + strconv.Itoa(j)
			got, ok := tt.Find(name)
			want, wok := ref.Find(name)
			if !ok || !wok || got != want {
				t.Fatalf("scrolled by %v: %s at %v (%v), want %v (%v)", dx, name, got, ok, want, wok)
			}
			head := header(t, tt, "H"+strconv.Itoa(j))
			if float64(got.X) != head.X+10 {
				t.Errorf("scrolled by %v: %s at %v, its header at %v", dx, name, got.X, head.X)
			}
		}
	}
	// Not all of them: the columns far from view are not built.
	clear(built)
	tt.Scroll(200, 150, -1e6, 0)
	tt.Frame()
	if cols := builtColumns(built); len(cols) > 9 {
		t.Errorf("at the start again: built %v", cols)
	}
}

// A column sharing the room left cannot be placed before the table is
// laid out: every column is built then, and with right to left text.
func TestTableVirtualColumnsNeedsWidths(t *testing.T) {
	built := map[int]int{}
	s := ListState{VirtualColumns: true}
	cols := []TableColumn{{Title: "A"}, {Title: "B", Width: 300}, {Title: "C", Width: 300}, {Title: "D", Width: 300}}
	tt := coreNewTester(func(c *context) {
		coreTable(c, &s, cols, 3, func(row, col int) {
			built[col]++
			coreText(c, "x").SingleLine()
		}).Grow(1)
	}, 400, 300)
	tt.Frame()
	tt.Frame()
	if cols := builtColumns(built); len(cols) != 4 {
		t.Errorf("with a flexible column: built %v", cols)
	}
}

// The header cannot be dragged away from the columns not built: it is
// built whole, so that moving a column and the accessibility tree see them
// all.
func TestTableVirtualColumnsBuildsAllHeaders(t *testing.T) {
	s := ListState{VirtualColumns: true}
	tt := coreNewTester(wideTable(&s, 100, 100, 0, nil), 400, 300)
	tt.Frame()
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	for _, j := range []int{0, 50, 99} {
		accessNode(t, tt.h.access, platform.RoleColumnHeader, "H"+strconv.Itoa(j))
	}
}

// Frozen columns stay at the table's start as the others scroll under
// them: their headers and their cells keep their places.
func TestTableFrozenColumns(t *testing.T) {
	for _, virtual := range []bool{false, true} {
		s := ListState{VirtualColumns: virtual}
		tt := coreNewTester(wideTable(&s, 40, 100, 2, nil), 500, 300)
		tt.Frame()
		h0, h1 := header(t, tt, "H0"), header(t, tt, "H1")
		c0, _ := tt.Find("r3c0")
		c1, _ := tt.Find("r3c1")
		tt.Scroll(250, 150, 1050, 0)
		tt.Frame()
		if got := header(t, tt, "H0"); got != h0 {
			t.Errorf("virtual %v: H0 moved from %v to %v", virtual, h0, got)
		}
		if got := header(t, tt, "H1"); got != h1 {
			t.Errorf("virtual %v: H1 moved from %v to %v", virtual, h1, got)
		}
		if got, _ := tt.Find("r3c0"); got != c0 {
			t.Errorf("virtual %v: r3c0 moved from %v to %v", virtual, c0, got)
		}
		if got, _ := tt.Find("r3c1"); got != c1 {
			t.Errorf("virtual %v: r3c1 moved from %v to %v", virtual, c1, got)
		}
		if scrollXOf(&s) != 1050 {
			t.Fatalf("virtual %v: scrolled to %v", virtual, scrollXOf(&s))
		}
		// The others went by as far as the table scrolled, header and cell.
		head := header(t, tt, "H12")
		if cell, ok := tt.Find("r3c12"); !ok || float64(cell.X) != head.X+10 || head.X != h0.X-1050+1200 {
			t.Errorf("virtual %v: H12 at %v, r3c12 at %v (%v)", virtual, head.X, cell.X, ok)
		}
	}
}

// What the pinned columns cover cannot be clicked: the click lands in the
// pinned column, where it shows, not in the one scrolled under it.
func TestTableFrozenHitTest(t *testing.T) {
	s := &ListState{CellSelect: true, VirtualColumns: true}
	sel := &Selection[int]{}
	s.Selection = sel
	cols := []TableColumn{{Title: "#", RowHeader: true, Width: 40, Frozen: true}, {Title: "A", Width: 100, Frozen: true}}
	for k := range 30 {
		cols = append(cols, TableColumn{Title: string(rune('a'+k%26)) + strconv.Itoa(k), Width: 100})
	}
	tt := NewTester(func(c *Context) {
		Table(c, s, cols, 10, func(row, col int) {
			Textf(c, "r%dc%d", row, col)
		}).Grow(1)
	}, 500, 300)
	tt.Frame()
	tt.Scroll(250, 150, 430, 0)
	tt.Frame()
	// Column 5 (x 540 of the row) is at 540-430 = 110: under A (40-140).
	x, y := rowCenter(t, tt, "r2c1")
	tt.ClickAt(x+60, y) // by the right of A's text, where column 3 is under it
	wantBlock(t, s, sel, "click over the pinned A", []int{2}, ColumnRange{Cursor: "A", Anchor: "A"}, []string{"A"})
	// A column in view at the right (column 8, at 740-430) takes its click.
	x, y = rowCenter(t, tt, "r2c8")
	tt.ClickAt(x, y)
	wantBlock(t, s, sel, "click on r2c8", []int{2}, ColumnRange{Cursor: "g6", Anchor: "g6"}, []string{"g6"})
}

// Keys move the cursor to a column not built, which scrolls into view past
// the pinned ones.
func TestTableFrozenRevealsCursor(t *testing.T) {
	s := &ListState{CellSelect: true, VirtualColumns: true}
	sel := &Selection[int]{}
	s.Selection = sel
	cols := []TableColumn{{Title: "A", Width: 100, Frozen: true}}
	for k := range 40 {
		cols = append(cols, TableColumn{Title: "C" + strconv.Itoa(k), Width: 100})
	}
	tt := NewTester(func(c *Context) {
		Table(c, s, cols, 5, func(row, col int) {
			Textf(c, "r%dc%d", row, col)
		}).Grow(1)
	}, 500, 300)
	tt.Frame()
	tt.Click("r0c0")
	for range 20 {
		tt.Key(0, KeyRight)
		tt.Frame()
	}
	if s.Cells.Cursor != "C19" {
		t.Fatalf("the cursor is in %q", s.Cells.Cursor)
	}
	cell, ok := tt.Find("r0c20")
	pinned, _ := tt.Find("r0c0")
	if !ok || cell.X < pinned.X+100 || cell.X > 500 {
		t.Errorf("the cursor's cell at %v (%v), under the pinned one at %v or out of the view", cell.X, ok, pinned.X)
	}
}

// Frozen columns move like any other: the pinned run is the leading
// Frozen columns of the order the layout gives, and only a run from the
// first is pinned. Fixed ones stay where they are.
func TestTableFrozenArrangeAndGeometry(t *testing.T) {
	cols := []TableColumn{{Title: "A", Width: 50, Frozen: true}, {Title: "B", Width: 60, Frozen: true}, {Title: "C", Width: 70}, {Title: "D", Width: 80, Frozen: true}, {Title: "E", Width: 90}}
	layout := TableLayout{}
	order := layout.arrange(cols)
	if !slices.Equal(order, []int{0, 1, 2, 3, 4}) {
		t.Errorf("arrange: %v", order)
	}
	starts, n, w := tableGeometry(&layout, cols, order)
	if n != 2 || w != 110 || len(starts) != 6 || starts[2] != 110 || starts[5] != 350 {
		t.Errorf("geometry: starts %v, %d frozen, %v wide", starts, n, w)
	}
	// Moved: D takes the first place, then B, A; C and E follow.
	layout = TableLayout{Order: []string{"D", "B", "A", "E", "C"}}
	order = layout.arrange(cols)
	if !slices.Equal(order, []int{3, 1, 0, 4, 2}) {
		t.Errorf("arrange after the move: %v", order)
	}
	starts, n, w = tableGeometry(&layout, cols, order)
	if n != 3 || w != 190 || starts[3] != 190 {
		t.Errorf("pinned run after the move: starts %v, %d frozen, %v wide", starts, n, w)
	}
	// An unfrozen column first pins nothing, and a Frozen one after it
	// scrolls.
	layout = TableLayout{Order: []string{"C", "A", "B", "D", "E"}}
	order = layout.arrange(cols)
	if _, n, w = tableGeometry(&layout, cols, order); n != 0 || w != 0 {
		t.Errorf("an unfrozen first column: %d frozen, %v wide", n, w)
	}
	// A Fixed column stays where it is, Frozen or not.
	fixed := []TableColumn{{Title: "A", Width: 50, Fixed: true, Frozen: true}, {Title: "B", Width: 60, Frozen: true}, {Title: "C", Width: 70}}
	layout = TableLayout{Order: []string{"C", "B", "A"}}
	if order = layout.arrange(fixed); !slices.Equal(order, []int{0, 2, 1}) {
		t.Errorf("a fixed column moved: %v", order)
	}
	// One sharing the room left cannot pin, nor can a column after it.
	flex := []TableColumn{{Title: "A", Frozen: true}, {Title: "B", Width: 60, Frozen: true}}
	if starts, n, _ := tableGeometry(&TableLayout{}, flex, []int{0, 1}); n != 0 || starts != nil {
		t.Errorf("a frozen column sharing the room left: starts %v, %d frozen", starts, n)
	}
}

// A Frozen column is dragged to another place and resized like any other,
// and the pinned run follows the order after the move.
func TestTableFrozenMovesAndResizes(t *testing.T) {
	s := ListState{}
	cols := []TableColumn{{Title: "Name", Width: 100, Frozen: true}, {Title: "Kind", Width: 100, Frozen: true}, {Title: "Size", Width: 80}}
	tt := coreNewTester(tableView(&s, cols, []string{"a.txt"}), 500, 300)
	name := header(t, tt, "Name")
	y := float32(name.Y + name.H/2)
	// Name dragged past the middle of Kind.
	x := float32(name.X + 20)
	tt.Press(x, y)
	for _, d := range []float32{5, 20, 50, 60} {
		tt.Move(x+d, y)
	}
	tt.Release(x+60, y)
	if got := s.Columns.Order; len(got) != 3 || got[0] != "Kind" || got[1] != "Name" || got[2] != "Size" {
		t.Fatalf("the order after moving a frozen column: %q", got)
	}
	order := s.Columns.arrange(cols)
	if _, n, w := tableGeometry(&s.Columns, cols, order); n != 2 || w != 200 {
		t.Errorf("pinned run after the move: %d frozen, %v wide", n, w)
	}
	// Moved past the unfrozen Size, it leaves the pinned run.
	name = header(t, tt, "Name")
	x = float32(name.X + 20)
	tt.Press(x, y)
	for _, d := range []float32{5, 20, 50, 60} {
		tt.Move(x+d, y)
	}
	tt.Release(x+60, y)
	if got := s.Columns.Order; got[2] != "Name" {
		t.Fatalf("the order after the second move: %q", got)
	}
	order = s.Columns.arrange(cols)
	if _, n, w := tableGeometry(&s.Columns, cols, order); n != 1 || w != 100 {
		t.Errorf("pinned run once Name left it: %d frozen, %v wide", n, w)
	}
	// Kind, still pinned, resizes from the edge of its header.
	kind := header(t, tt, "Kind")
	tt.Press(float32(kind.X+kind.W-1), y)
	tt.Move(float32(kind.X+kind.W+20), y)
	tt.Release(float32(kind.X+kind.W+40), y)
	if w := header(t, tt, "Kind").W; w <= 100 {
		t.Errorf("a frozen column stayed %v wide", w)
	}
}

// Tables that ask for neither are as they were: every column built, none
// pinned.
func TestTableWithoutVirtualOrFrozenIsUnchanged(t *testing.T) {
	built := map[int]int{}
	s := ListState{}
	tt := coreNewTester(wideTable(&s, 30, 100, 0, built), 400, 300)
	tt.Frame()
	if n := len(builtColumns(built)); n != 30 {
		t.Errorf("built %d of 30 columns", n)
	}
	tt.Scroll(200, 150, 500, 0)
	tt.Frame()
	if h := header(t, tt, "H0"); h.X > -400 {
		t.Errorf("H0 at %v after scrolling 500: it should have scrolled away", h.X)
	}
}

// scrollXOf returns how far the table s shows is scrolled sideways.
func scrollXOf(s *ListState) float64 { return s.frame.e.st.scrollX }

// A pinned cell takes the background of the nearest element around it
// that has one, and one that is translucent, as a chosen or hovered row's,
// is composited over the ones behind it, down to an opaque one: taken as
// it is, it would let the cells scrolling under the pinned one show through.
func TestFrozenStyleIsOpaque(t *testing.T) {
	th := LightTheme()
	page := RGB(250, 250, 250)
	tint := RGBA(0, 0, 255, 0.2)
	chain := func(cell Color, around ...Color) *node {
		var parent *node
		for i := len(around) - 1; i >= 0; i-- {
			parent = &node{parent: parent, bg: around[i]}
		}
		return &node{parent: parent, bg: cell}
	}
	for _, tc := range []struct {
		name string
		e    *node
		want Color
	}{
		{"an opaque row", chain(Transparent, page), page},
		{"a translucent row over the page", chain(Transparent, tint, page), tint.Over(page)},
		{"a translucent row, the page's none", chain(Transparent, tint), tint.Over(th.Background)},
		{"nothing around it", chain(Transparent), th.Background},
		{"a translucent cell in a translucent row", chain(tint, tint, page), tint.Over(tint.Over(page))},
		{"a cell of its own", chain(RGB(1, 2, 3), page), RGB(1, 2, 3)},
	} {
		frozenStyle(th, false)(tc.e)
		if tc.e.bg != tc.want || tc.e.bg.A != 255 {
			t.Errorf("%s: pinned cell is %+v, want the opaque %+v", tc.name, tc.e.bg, tc.want)
		}
	}
}

// Right to left, the window follows the scroll as it does left to right:
// the columns from the start, at the right, are built, and the others as it
// scrolls, counted from the start.
func TestTableVirtualColumnsRightToLeft(t *testing.T) {
	built := map[int]int{}
	s := ListState{VirtualColumns: true}
	tt := coreNewTester(wideTable(&s, 100, 100, 0, built), 400, 300)
	tt.SetDirection(RightToLeft)
	tt.Frame()
	clear(built)
	tt.Frame()
	if cols := builtColumns(built); cols[0] != 0 || cols[len(cols)-1] > 8 || len(cols) < 4 {
		t.Errorf("at the start: built columns %v", cols)
	}
	tt.Scroll(200, 150, -3000, 0) // right to left, the start is on the right

	tt.Frame()
	clear(built)
	tt.Frame()
	cols := builtColumns(built)
	if cols[0] < 20 || cols[len(cols)-1] > 40 || len(cols) > 12 {
		t.Errorf("scrolled to 3000: built columns %v", cols)
	}
	// One at that place shows.
	if _, ok := tt.Find("r1c" + strconv.Itoa(cols[len(cols)/2])); !ok {
		t.Errorf("a column in the window is not shown: texts %q", tt.Texts())
	}
}

// Right to left, the pinned columns are at the right, and revealing a
// column scrolls the table as little as shows it in what they leave.
func TestCellSelectRevealsColumnsRightToLeftFrozen(t *testing.T) {
	s := ListState{CellSelect: true, Selection: &Selection[int]{}}
	cols := make([]TableColumn, 20)
	for j := range cols {
		cols[j] = TableColumn{Title: string(rune('A' + j)), Width: 100, Frozen: j < 2}
	}
	tt := NewTester(func(c *Context) {
		Table(c, &s, cols, 5, func(row, col int) {
			Textf(c, "r%dc%d", row, col)
		}).Grow(1)
	}, 500, 300)
	tt.SetDirection(RightToLeft)
	tt.Frame()
	tt.Click("r1c0")
	for range 12 {
		tt.Key(0, KeyLeft)
	}
	h, list := tt.rt.states[s.tableCols[s.colIndex(s.Cells.Cursor)].head], s.frame.e.st
	// What the pinned two leave, at the right, is 300 of the 500.
	if s.Cells.Cursor != "M" || h.x < list.x || h.x+h.w > list.x+list.w-s.frozenW {
		t.Fatalf("Left 12 times from A right to left: cursor %q, its header at %v..%v, the view %v..%v less %v pinned",
			s.Cells.Cursor, h.x, h.x+h.w, list.x, list.x+list.w, s.frozenW)
	}
	// Back at the first column that scrolls, next to the pinned ones: it is
	// not left under them.
	for range 10 {
		tt.Key(0, KeyRight)
	}
	h = tt.rt.states[s.tableCols[s.colIndex(s.Cells.Cursor)].head]
	if s.Cells.Cursor != "C" || h.x < list.x || h.x+h.w > list.x+list.w-s.frozenW {
		t.Fatalf("Right 10 times: cursor %q, its header at %v..%v, the view %v..%v less %v pinned",
			s.Cells.Cursor, h.x, h.x+h.w, list.x, list.x+list.w, s.frozenW)
	}
}

// A pinned header follows the pointer as the others do, scrolled or not:
// Left would replace the inset that holds it in place.
func TestTableFrozenHeaderFollowsThePointer(t *testing.T) {
	for _, scrolled := range []bool{false, true} {
		s := ListState{}
		cols := []TableColumn{{Title: "A", Width: 100, Frozen: true}, {Title: "B", Width: 100, Frozen: true}, {Title: "C", Width: 100}, {Title: "D", Width: 300}, {Title: "E", Width: 300}}
		tt := coreNewTester(tableView(&s, cols, []string{"a.txt"}), 500, 300)
		if scrolled {
			tt.Scroll(300, 150, 150, 0)
			tt.Frame()
		}
		b := header(t, tt, "B")
		y, x := float32(b.Y+b.H/2), float32(b.X+20)
		tt.Press(x, y)
		for _, d := range []float32{5, 20, 40} {
			tt.Move(x+d, y)
		}
		if got := header(t, tt, "B"); got.X != b.X+40 {
			t.Errorf("scrolled %v: B at %v after a 40 DIP drag from %v, want %v", scrolled, got.X, b.X, b.X+40)
		}
		tt.Release(x+40, y)
	}
}

// A List keeps the row holding the focus built as it scrolls out of view,
// and VirtualColumns the column: a cell holding the keyboard keeps it. It
// lets the column go once the keyboard is elsewhere.
func TestTableVirtualColumnsKeepTheFocusedColumn(t *testing.T) {
	built := map[int]int{}
	s := ListState{VirtualColumns: true}
	cols := make([]TableColumn, 60)
	for j := range cols {
		cols[j] = TableColumn{Title: "H" + strconv.Itoa(j), Width: 100}
	}
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Fill().Children(func() {
			coreTable(c, &s, cols, 5, func(row, col int) {
				built[col]++
				if row == 1 && col == 1 {
					coreBox(c).Key("editor").Focusable().Children(func() { coreText(c, "editor") })
					return
				}
				coreText(c, "r"+strconv.Itoa(row)+"c"+strconv.Itoa(col)).SingleLine()
			}).Grow(1)
		})
	}, 400, 300)
	tt.Frame()
	tt.Click("editor")
	tt.Frame()
	tt.Scroll(200, 150, 3000, 0)
	tt.Frame()
	clear(built)
	tt.Frame()
	if _, ok := built[1]; !ok || !tt.HasText("editor") {
		t.Fatalf("the column holding the focus, scrolled out of view, is gone: built %v", builtColumns(built))
	}
	// The keyboard goes to a cell in view, and the column goes with it.
	tt.Click("r1c30")
	tt.Frame()
	clear(built)
	tt.Frame()
	if _, ok := built[1]; ok {
		t.Errorf("the column is kept built once the keyboard left it: built %v", builtColumns(built))
	}
}

// The Cursor's column is kept too, for the keys to move from its cell.
func TestTableVirtualColumnsKeepTheCursorColumn(t *testing.T) {
	tt, s, _ := cellTable(t, 8, 30, 100)
	s.VirtualColumns = true
	tt.Frame()
	tt.Click("r1c2")
	tt.Frame()
	tt.Scroll(200, 150, 1500, 0)
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find("r1c18"); !ok {
		t.Fatalf("the table did not scroll: texts %q", tt.Texts())
	}
	if _, ok := tt.Find("r1c2"); !ok {
		t.Fatalf("the cell of the Cursor %q, scrolled out of view, is not built", s.Cells.Cursor)
	}
}

// A pinned header under the pointer is opaque with a theme whose hover
// color is translucent, as it is without the pointer: the headers scrolling
// under it would show through.
func TestTableFrozenHeaderHoveredIsOpaque(t *testing.T) {
	sort := SortOrder{}
	s := ListState{Sort: &sort}
	cols := []TableColumn{{Title: "A", Width: 100, Frozen: true, Sortable: true}, {Title: "B", Width: 400}}
	// The node of the frame built last, whose look the paint settled.
	var head *node
	s.HeaderCell = func(e Element, j int) {
		if j == 0 {
			head = e.node()
		}
	}
	th := *LightTheme()
	th.SurfaceHover = RGBA(0, 0, 0, 0.1)
	tt := coreNewTester(func(c *context) {
		c.SetTheme(&th)
		coreColumn(c).Fill().Children(func() {
			coreTable(c, &s, cols, 3, func(row, col int) { coreText(c, "x") }).Grow(1)
		})
	}, 300, 200)
	tt.Frame()
	a := header(t, tt, "A")
	tt.Move(float32(a.X+a.W/2), float32(a.Y+a.H/2))
	tt.Frame()
	tt.Frame()
	if head == nil || head.styleFn == nil {
		t.Fatal("no pinned header built")
	}
	// Painting restores what its style sets: run it as the paint does.
	head.styleFn(head)
	if !head.Hovered() || head.bg.A != 255 {
		t.Errorf("the hovered pinned header's background %+v (hovered %v), want an opaque one", head.bg, head.Hovered())
	}
}

// The first pinned cell of a row with rounded corners (RoundRows) takes the
// corners at the row's start, which its square background would cover; the
// others, and the cells of a header row, are as they were.
func TestFrozenStyleTakesTheRowsStartCorners(t *testing.T) {
	th := LightTheme()
	for _, rtl := range []bool{false, true} {
		row := &node{radius: [4]float32{6, 7, 8, 9}}
		if rtl {
			row.dir = dirRTL
		}
		first, second := &node{parent: row, dir: row.dir}, &node{parent: row, dir: row.dir}
		frozenStyle(th, true)(first)
		frozenStyle(th, false)(second)
		want := [4]float32{6, 0, 0, 9}
		if rtl {
			want = [4]float32{0, 7, 8, 0}
		}
		if first.radius != want {
			t.Errorf("rtl %v: the first pinned cell's corners %v, want %v", rtl, first.radius, want)
		}
		if second.radius != [4]float32{} {
			t.Errorf("rtl %v: a later pinned cell's corners %v, want none", rtl, second.radius)
		}
	}
}

// A table without VirtualColumns does not read which cell holds the
// keyboard: each read asks for another frame whenever the focus moves.
func TestTableReadsTheFocusOnlyForVirtualColumns(t *testing.T) {
	for _, virtual := range []bool{false, true} {
		s := ListState{VirtualColumns: virtual}
		cols := []TableColumn{{Title: "A", Width: 100}, {Title: "B", Width: 100}}
		tt := coreNewTester(func(c *context) {
			coreColumn(c).Fill().Children(func() {
				coreTable(c, &s, cols, 3, func(row, col int) {
					coreText(c, "r"+strconv.Itoa(row)+"c"+strconv.Itoa(col))
				}).Grow(1)
			})
		}, 400, 300)
		tt.Frame()
		tt.Frame()
		if got := tt.rt.withinRead; got != virtual {
			t.Errorf("VirtualColumns %v: the frame read FocusWithin %v, want %v", virtual, got, virtual)
		}
	}
}

// gapIDs returns the IDs of the blocks standing for the columns not built,
// in the order of the tree.
func gapIDs(root *node) []uint64 {
	var ids []uint64
	var walk func(n *node)
	walk = func(n *node) {
		if k, ok := n.key.(string); ok && strings.HasPrefix(k, "\x00gap") {
			ids = append(ids, n.id)
		}
		for ch := n.first; ch != nil; ch = ch.next {
			walk(ch)
		}
	}
	walk(root)
	return ids
}

// The block after the columns built stays the same element as the table
// scrolls, as the one before: keyed by where the run it stands for ends it
// would be another as the window moves, and lose its state.
func TestTableVirtualColumnsGapsKeepTheirKeys(t *testing.T) {
	s := ListState{VirtualColumns: true}
	cols := make([]TableColumn, 60)
	for j := range cols {
		cols[j] = TableColumn{Title: "H" + strconv.Itoa(j), Width: 100}
	}
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Fill().Children(func() {
			coreTable(c, &s, cols, 1, func(row, col int) { coreText(c, "r"+strconv.Itoa(col)).SingleLine() }).Grow(1)
		})
	}, 400, 300)
	tt.Frame()
	tt.Scroll(200, 150, 1500, 0)
	tt.Frame()
	tt.Frame()
	before := gapIDs(tt.rt.c.root)
	tt.Scroll(200, 150, 200, 0)
	tt.Frame()
	tt.Frame()
	after := gapIDs(tt.rt.c.root)
	if len(before) != 2 || !slices.Equal(before, after) {
		t.Errorf("the blocks' IDs changed as the table scrolled: %v, then %v", before, after)
	}
}

// A column with no title or ID, a checkbox column say, is kept built while
// it holds the keyboard as the others are.
func TestTableVirtualColumnsKeepTheFocusedUntitledColumn(t *testing.T) {
	s := ListState{VirtualColumns: true}
	cols := make([]TableColumn, 60)
	for j := range cols {
		cols[j] = TableColumn{Title: "H" + strconv.Itoa(j), Width: 100}
	}
	cols[1] = TableColumn{Width: 100}
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Fill().Children(func() {
			coreTable(c, &s, cols, 5, func(row, col int) {
				if row == 1 && col == 1 {
					coreBox(c).Key("check").Focusable().Children(func() { coreText(c, "check") })
					return
				}
				coreText(c, "r"+strconv.Itoa(row)+"c"+strconv.Itoa(col)).SingleLine()
			}).Grow(1)
		})
	}, 400, 300)
	tt.Frame()
	tt.Click("check")
	tt.Frame()
	tt.Scroll(200, 150, 3000, 0)
	tt.Frame()
	tt.Frame()
	if !tt.HasText("check") {
		t.Fatal("the untitled column holding the focus, scrolled out of view, is gone")
	}
}

// The column a drag over the cells started in is kept built, for the drag
// to go on from it as the table scrolls away.
func TestTableVirtualColumnsKeepTheDragColumn(t *testing.T) {
	tt, s, _ := cellTable(t, 8, 30, 100)
	s.VirtualColumns = true
	tt.Frame()
	x, y := rowCenter(t, tt, "r1c2")
	x2, _ := rowCenter(t, tt, "r1c3")
	tt.Press(x, y)
	tt.Move(x+3, y)
	// The pointer moves the Cursor on, to the column under it.
	tt.Move(x2+60, y)
	tt.Frame()
	if !s.dragSel.on || s.dragSel.fromCol == s.Cells.Cursor {
		t.Fatalf("the drag is not on, or the Cursor did not leave its column: on %v, from %q, Cursor %q", s.dragSel.on, s.dragSel.fromCol, s.Cells.Cursor)
	}
	tt.Scroll(200, 150, 1500, 0)
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find("r1c18"); !ok {
		t.Fatalf("the table did not scroll: texts %q", tt.Texts())
	}
	if _, ok := tt.Find("r1c2"); !ok {
		t.Fatalf("the column the drag started in (%q), scrolled out of view, is not built", s.dragSel.fromCol)
	}
}
