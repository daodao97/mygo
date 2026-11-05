package terminal

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestCommandClickIndentedTableFileContinuation(t *testing.T) {
	loadLib(t)
	var opened []linkTarget
	term, err := New(Options{Conn: newPipe(), OpenLink: func(link string) bool { opened = append(opened, openedTarget(link)); return true }})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 960, 260)
	v := term.v
	first := "/Users/daodao/work/bihu/aicoding-"
	start := v.cols - len(first) - 3
	for _, indent := range []int{10, 26, 40} {
		prefix := "HTTP Client timeout"
		prefix += strings.Repeat(" ", start-len(prefix)-1) + "("
		tail := "relay/api/transport.go:21)"
		term.Feed([]byte("\x1b[H\x1b[2J" + prefix + first + "\r\n" + strings.Repeat(" ", indent) + tail))
		tt.Frame()
		want := linkTarget{Path: first + "relay/api/transport.go", Line: 21}
		for _, cell := range [][2]int{{start + 3, 0}, {indent + 3, 1}, {indent + len(tail) - 3, 1}} {
			x := float32(v.ox+cell[0]*v.cellW+v.cellW/2) / v.scale
			y := float32(v.oy+cell[1]*v.cellH+v.cellH/2) / v.scale
			opened = nil
			tt.ClickAtWith(ui.Cmd, x, y)
			if len(opened) != 1 || opened[0] != want {
				t.Fatalf("indent %d click %v: got %+v, want %+v", indent, cell, opened, want)
			}
		}
		term.mu.Lock()
		x := float32(v.ox+2*v.cellW+v.cellW/2) / v.scale
		y := float32(v.oy+v.cellH+v.cellH/2) / v.scale
		padding := v.linkAt(x, y)
		x = float32(v.ox+(indent+3)*v.cellW+v.cellW/2) / v.scale
		hover := v.linkAt(x, y)
		term.mu.Unlock()
		if padding.valid() || hover.row != 1 || hover.cellA < indent || hover.cellB > indent+len(tail)-1 {
			t.Fatalf("table padding became a link or underline: padding=%+v hover=%+v", padding, hover)
		}
	}

	// The same table cell may occupy more than two separately painted rows.
	indent := 26
	middle := strings.Repeat("b", v.cols-indent-4) + "-"
	last := "relay.go:324:2"
	term.Feed([]byte("\x1b[H\x1b[2J" + strings.Repeat(" ", start) + first + "\x1b[2;1H" + strings.Repeat(" ", indent) + middle + "\x1b[3;1H" + strings.Repeat(" ", indent) + last))
	tt.Frame()
	for _, cell := range [][2]int{{start + 2, 0}, {indent + 2, 1}, {indent + 2, 2}} {
		x := float32(v.ox+cell[0]*v.cellW+v.cellW/2) / v.scale
		y := float32(v.oy+cell[1]*v.cellH+v.cellH/2) / v.scale
		opened = nil
		tt.ClickAtWith(ui.Cmd, x, y)
		want := linkTarget{Path: first + middle + "relay.go", Line: 324, Column: 2}
		if len(opened) != 1 || opened[0] != want {
			t.Fatalf("three-row table click %v: got %+v, want %+v", cell, opened, want)
		}
	}
	// A different indentation or a new absolute reference is a separate link.
	for _, next := range []string{"other.go:12", "/tmp/other.go:12"} {
		firstRow := "/tmp/" + strings.Repeat("a", v.cols-9) + "-"
		term.Feed([]byte("\x1b[H\x1b[2J" + firstRow + "\r\n" + strings.Repeat(" ", indent) + next))
		tt.Frame()
		opened = nil
		x := float32(v.ox+(indent+2)*v.cellW+v.cellW/2) / v.scale
		y := float32(v.oy+v.cellH+v.cellH/2) / v.scale
		tt.ClickAtWith(ui.Cmd, x, y)
		want := linkTarget{Path: strings.TrimSuffix(next, ":12"), Line: 12}
		if len(opened) != 1 || opened[0] != want {
			t.Fatalf("separate table row %q became %+v", next, opened)
		}
	}
}
