package terminal

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestScrollbarCancelStopsDragging(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe()})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 200)
	term.Feed([]byte(strings.Repeat("a line of scrollback\r\n", 100)))
	tt.Frame()
	v := term.v
	v.input(ui.InputEvent{Kind: ui.InputPointerDown, X: 395, Y: 188})
	if !v.barHeld {
		t.Fatal("the scroll bar did not take the drag")
	}
	v.input(ui.InputEvent{Kind: ui.InputPointerCancel, X: 395, Y: 188})
	offset := func() uint64 {
		term.mu.Lock()
		defer term.mu.Unlock()
		return term.term.Scrollbar().Offset
	}
	before := offset()
	v.input(ui.InputEvent{Kind: ui.InputPointerMove, X: 395, Y: 10})
	if got := offset(); got != before {
		t.Fatalf("moving after cancellation scrolled from %d to %d", before, got)
	}
}

func TestScrollbarKeepsExistingTextDrag(t *testing.T) {
	loadLib(t)
	for _, tracked := range []bool{false, true} {
		name := "local selection"
		if tracked {
			name = "program mouse tracking"
		}
		t.Run(name, func(t *testing.T) {
			conn := newPipe()
			term, err := New(Options{Conn: conn})
			if err != nil {
				t.Fatal(err)
			}
			defer term.Close()
			tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 400, 200)
			term.Feed([]byte(strings.Repeat("a line of scrollback\r\n", 100)))
			if tracked {
				term.Feed([]byte("\x1b[?1002h\x1b[?1006h"))
			}
			tt.Frame()
			x, y := cellCenter(term, 1, 1)
			tt.Press(x, y)
			tt.Move(395, y)
			tt.Release(395, y)
			if tracked {
				got := conn.take(term)
				if !strings.HasSuffix(got, "m") {
					t.Fatalf("the program lost the drag release over the scroll bar: %q", got)
				}
			} else if _, ok := term.SelectedText(); !ok {
				t.Fatal("the text drag lost its selection over the scroll bar")
			}
		})
	}
}
