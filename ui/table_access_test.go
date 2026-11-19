package ui_test

import (
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"
)

// A table that builds only the columns in view tells assistive technology
// how many it has and which place each cell and header has among them, for
// the cells to be paired with their headers, not with the ones the gap
// left them beside.
func TestTableVirtualColumnsAccess(t *testing.T) {
	s := ui.ListState{VirtualColumns: true}
	cols := make([]ui.TableColumn, 60)
	for j := range cols {
		cols[j] = ui.TableColumn{Title: "H" + strconv.Itoa(j), Width: 100}
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Table(c, &s, cols, 5, func(row, col int) {
			ui.Text(c, "r"+strconv.Itoa(row)+"c"+strconv.Itoa(col)).SingleLine()
		}).Grow(1).Label("Sheet")
	}, 400, 300)
	tt.Frame()
	tt.Scroll(200, 150, 3000, 0)
	tt.Frame()
	tt.Frame()
	if table := accessOf(t, tt, ui.RoleTable, "Sheet"); table.ColumnCount != 60 {
		t.Errorf("the table says %d columns, want all 60", table.ColumnCount)
	}
	h := accessOf(t, tt, ui.RoleColumnHeader, "H31")
	if h.ColumnIndex != 32 {
		t.Errorf("H31 is column %d, want 32", h.ColumnIndex)
	}
	// The cells under it, where it is, are in its column.
	under := 0
	for _, a := range tt.AccessAll() {
		if a.Role == ui.RoleCell && a.Bounds.X == h.Bounds.X {
			under++
			if a.ColumnIndex != h.ColumnIndex {
				t.Errorf("a cell under H31 is column %d, want %d", a.ColumnIndex, h.ColumnIndex)
			}
		}
	}
	if under == 0 {
		t.Error("no cell under H31 in the tree")
	}
}
