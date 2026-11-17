package terminal

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
)

func TestMobileLongPressSelectsAndExpandsWithoutRemoteMouse(t *testing.T) {
	loadLib(t)
	conn := newPipe()
	term, err := New(Options{CopyOnSelect: CopyOnSelectClipboard, Conn: conn, FixedCols: 80, FixedRows: 20})
	if err != nil {
		t.Fatal(err)
	}
	term.Feed([]byte("\x1b[?1002h\x1b[?1006hhello 中文🙂 world"))
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 393, 300)
	x, y := cellCenter(term, 2, 0)
	term.v.input(ui.InputEvent{Kind: ui.InputLongPress, X: x, Y: y})
	tt.Frame()
	if text, ok := term.SelectedText(); !ok || text != "hello" {
		t.Fatalf("long press selected %q", text)
	}
	if _, visible := term.SelectionAnchor(); visible {
		t.Fatal("selection actions obstructed the active drag")
	}
	x1, y1 := cellCenter(term, 15, 0)
	term.v.input(ui.InputEvent{Kind: ui.InputPointerMove, X: x1, Y: y1, Button: 0})
	term.v.input(ui.InputEvent{Kind: ui.InputPointerUp, X: x1, Y: y1, Button: 0})
	tt.Frame()
	if copied := tt.Clipboard(); !strings.Contains(copied, "hello 中文🙂 world") {
		t.Fatalf("expanded selection copied %q", copied)
	}
	if anchor, visible := term.SelectionAnchor(); !visible || anchor.X != x1 || anchor.Y != y1 || anchor.H <= 0 {
		t.Fatalf("released selection has no usable context anchor: %+v, %v", anchor, visible)
	}
	term.SelectAll()
	if text, ok := term.SelectedText(); !ok || !strings.Contains(text, "hello 中文🙂 world") {
		t.Fatalf("select all lost the displayed text: %q", text)
	}
	if data := conn.take(term); data != "" {
		t.Fatalf("mobile selection reached remote program: %q", data)
	}
	term.v.input(ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y, Button: 0})
	term.v.input(ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: y, Button: 0})
	tt.Frame()
	term.Close()
}
