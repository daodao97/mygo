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
