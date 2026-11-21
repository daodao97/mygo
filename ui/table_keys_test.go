package ui

import (
	"fmt"
	"testing"
)

// focusedName returns the accessible name of the element holding the focus.
func focusedName(tt *Tester) string {
	for _, a := range tt.AccessAll() {
		if a.Focused {
			return a.Name
		}
	}
	return ""
}

// tabTo presses Tab until the element named name has the focus.
func tabTo(t *testing.T, tt *Tester, name string) {
	t.Helper()
	var seen []string
	for range 12 {
		if focusedName(tt) == name {
			return
		}
		tt.Key(0, KeyTab)
		tt.Frame()
		seen = append(seen, focusedName(tt))
	}
	t.Fatalf("Tab never reached %q: %q", name, seen)
}

// A focused button in a row of a list choosing rows clicks on Enter, as it
// does in one that does not, rather than the list taking the key (#2145).
func TestEnterOnAButtonInARow(t *testing.T) {
	for _, kind := range []string{"list", "table", "grid"} {
		for _, chooses := range []bool{false, true} {
			t.Run(fmt.Sprint(kind, chooses), func(t *testing.T) {
				sel := -1
				st := ListState{}
				gs := GridState{}
				if chooses {
					st.Selected, gs.Selected = &sel, &sel
				}
				clicks, submitted := 0, 0
				cols := []TableColumn{{Title: "Name"}, {Title: "Action"}}
				tt := coreNewTester(func(c *context) {
					edit := func(row int) {
						if coreButton(c, fmt.Sprint("Edit ", row)).Clicked() {
							clicks++
						}
					}
					coreColumn(c).Fill().Children(func() {
						var e *node
						switch kind {
						case "list":
							e = coreList(c, &st, 2, func(i int) {
								coreRow(c).Children(func() { coreText(c, "row"); edit(i) })
							})
						case "table":
							e = coreTable(c, &st, cols, 2, func(row, col int) {
								if col == 0 {
									coreText(c, "row")
									return
								}
								edit(row)
							})
						case "grid":
							e = coreGridView(c, &gs, 2, 120, 60, func(i int) { edit(i) })
						}
						e.Height(200)
						if e.Submitted() {
							submitted++
						}
					})
				}, 600, 400)
				tt.Frame()
				tabTo(t, tt, "Edit 1")
				tt.Key(0, KeyEnter)
				tt.Frame()
				if clicks != 1 || submitted != 0 {
					t.Errorf("Enter on Edit 1: %d clicks, %d submitted", clicks, submitted)
				}
				tt.Key(0, KeySpace)
				tt.Frame()
				if clicks != 2 {
					t.Errorf("Space on Edit 1: %d clicks", clicks)
				}
			})
		}
	}
}

// A sortable header is a button to the keyboard: Tab reaches it, and Enter
// or Space sorts by it, in a table choosing rows too; a click leaves the
// focus with the table (#2146).
func TestSortableHeaderByKeyboard(t *testing.T) {
	sort := SortOrder{}
	sel := -1
	st := ListState{Sort: &sort, Selected: &sel, Label: func(i int) string { return fmt.Sprint("row ", i) }}
	cols := []TableColumn{{Title: "Name", Sortable: true}, {Title: "Kind"}, {Title: "Price", Sortable: true}}
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Fill().Children(func() {
			coreButton(c, "Before")
			coreTable(c, &st, cols, 3, func(row, col int) { coreTextf(c, "%d %d", row, col) }).Height(200)
			coreButton(c, "After")
		})
	}, 600, 400)
	tt.Frame()
	tt.Click("Price")
	if sort != (SortOrder{Column: "Price"}) {
		t.Fatalf("a click on Price: %+v", sort)
	}
	if name := focusedName(tt); name == "Price" || name == "Before" {
		t.Errorf("a click on Price focused %q, not the table", name)
	}
	// The keys moving the choice still go to the table.
	tt.Key(0, KeyDown)
	tt.Frame()
	if sel != 0 {
		t.Errorf("Down after the click chose %d", sel)
	}
	tt.Key(0, KeyEnter) // submits the row, which sorts nothing
	tt.Frame()
	if sort != (SortOrder{Column: "Price"}) {
		t.Errorf("Enter on the table sorted: %+v", sort)
	}
	tabTo(t, tt, "Name")
	tt.Key(0, KeyEnter)
	tt.Frame()
	if sort != (SortOrder{Column: "Name"}) {
		t.Errorf("Enter on Name: %+v", sort)
	}
	tt.Key(0, KeySpace)
	tt.Frame()
	if sort != (SortOrder{Column: "Name", Descending: true}) {
		t.Errorf("Space on Name: %+v", sort)
	}
	if a, _ := tt.AccessByRole(RoleColumnHeader, "Name"); !a.Focusable || !a.Focused || a.Sort == SortNone {
		t.Errorf("Name to assistive technology: %+v", a)
	}
	if a, _ := tt.AccessByRole(RoleColumnHeader, "Kind"); a.Focusable {
		t.Errorf("Kind, which does not sort, is focusable")
	}
	// Kind, which does not sort, is no stop: Tab goes on to Price, then
	// past the table.
	tt.Key(0, KeyTab)
	tt.Frame()
	if name := focusedName(tt); name != "Price" {
		t.Errorf("Tab from Name went to %q", name)
	}
	tt.Key(0, KeyTab)
	tt.Frame()
	if name := focusedName(tt); name != "After" {
		t.Errorf("Tab from Price went to %q", name)
	}
}
