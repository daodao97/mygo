package terminal

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTapLinksOpenOnRelease(t *testing.T) {
	loadLib(t)
	for _, osc := range []bool{false, true} {
		conn := newPipe()
		term, err := New(Options{Conn: conn, OpenLinksOnTap: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { term.Close() })
		url := "https://example.com/" + strings.Repeat("a", 35)
		text := url
		if osc {
			text = "\x1b]8;;" + url + "\x1b\\打开链接\x1b]8;;\x1b\\"
		}
		term.Feed([]byte("\x1b[?1002h\x1b[?1006h" + text))
		tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 260, 160)
		row := 1
		if osc {
			row = 0
		}
		x, y := cellCenter(term, 2, row)
		tt.SetClipboard("keep clipboard")
		tt.Press(x, y)
		if len(tt.OpenedURLs()) != 0 {
			t.Fatal("link opened before release")
		}
		tt.Release(x, y)
		if !slices.Equal(tt.OpenedURLs(), []string{url}) {
			t.Fatalf("opened links = %q", tt.OpenedURLs())
		}
		if tt.Clipboard() != "keep clipboard" || conn.take(term) != "" {
			t.Fatal("link tap copied text or reached the remote program")
		}
	}
}

func TestTapLinksCancelAndKeepLongPressSelection(t *testing.T) {
	loadLib(t)
	for _, action := range []string{"tap", "move back", "release moved", "cancel", "long press", "scroll", "output changed"} {
		t.Run(action, func(t *testing.T) {
			conn := newPipe()
			opened := 0
			var term *Terminal
			term, err := New(Options{Conn: conn, OpenLinksOnTap: true, OpenLink: func(string) bool {
				_ = term.Text() // Callback must run outside the emulator lock.
				opened++
				return true
			}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { term.Close() })
			term.Feed([]byte("https://example.com/test"))
			tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 400, 160)
			x, y := cellCenter(term, 3, 0)
			tt.Press(x, y)
			switch action {
			case "move back":
				tt.Move(x+20, y)
				tt.Move(x, y)
			case "release moved":
				x += 20
			case "cancel":
				term.v.input(ui.InputEvent{Kind: ui.InputPointerCancel, X: x, Y: y})
			case "long press":
				term.v.input(ui.InputEvent{Kind: ui.InputLongPress, X: x, Y: y})
			case "scroll":
				tt.Scroll(x, y, 0, 20)
			case "output changed":
				term.Feed([]byte("\x1b[H\x1b[2Jhttps://different.example/test"))
				tt.Frame()
			}
			if action != "cancel" {
				tt.Release(x, y)
			}
			want := 0
			if action == "tap" {
				want = 1
			}
			if opened != want {
				t.Fatalf("opened %d links, want %d", opened, want)
			}
			if action == "long press" {
				if text, selected := term.SelectedText(); !selected || text == "" {
					t.Fatal("long press did not select link text")
				}
			}
			if term.v.linkTap != nil {
				t.Fatal("completed contact retained a pending link")
			}
		})
	}
}
