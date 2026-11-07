package terminal

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
)

func TestURLAt(t *testing.T) {
	row := "see https://example.com/a_(b)?q=1. and (http://x.org/y), mailto:me@x.org!"
	text := []rune(row)
	cols := make([]int, len(text))
	for i := range cols {
		cols[i] = i
	}
	for col, want := range map[int]string{
		0:  "",
		4:  "https://example.com/a_(b)?q=1",
		20: "https://example.com/a_(b)?q=1",
		33: "", // the period after it
		40: "http://x.org/y",
		58: "mailto:me@x.org",
	} {
		if got, _, _ := urlAt(text, cols, col); got != want {
			t.Errorf("urlAt(%d) = %q, want %q", col, got, want)
		}
	}
}

func TestPathAt(t *testing.T) {
	for _, c := range []struct {
		row  string
		col  int
		want string
	}{
		{"Updated internal/app/embed.go:51 and more", 10, "internal/app/embed.go:51"},
		{"  ⎿  Read /Users/me/repo/main.go (12 lines)", 12, "/Users/me/repo/main.go"},
		{"see ./notes/todo.txt.", 8, "./notes/todo.txt"},
		{"open ~/Downloads/clip.mp4", 10, "~/Downloads/clip.mp4"},
		{"main.go:250:3: undefined: x", 2, "main.go:250:3"},
		{"(src/a.go)", 3, "src/a.go"},
		{"write(notes.md)", 8, "notes.md"},
		{"notes.md (new file +2)", 2, "notes.md"},
		{"See notes.md.", 7, "notes.md"},
		{"notes.md:2:3", 2, "notes.md:2:3"},
		{"just words here", 5, ""},
		{"version 1.2.3 ok", 9, ""},
	} {
		text := []rune(c.row)
		cols := make([]int, len(text))
		for i := range cols {
			cols[i] = i
		}
		if got, _, _ := pathAt(text, cols, c.col); got != c.want {
			t.Errorf("pathAt(%q, %d) = %q, want %q", c.row, c.col, got, c.want)
		}
	}
}

func TestFileAndURLMatches(t *testing.T) {
	for _, tc := range []struct {
		text, click string
		want        linkTarget
	}{
		{"error: src/main.go:42:5: failed", "main", linkTarget{Path: "src/main.go", Line: 42, Column: 5}},
		{"at (./src/main.ts(12,3))", "main", linkTarget{Path: "./src/main.ts", Line: 12, Column: 3}},
		{"see ~/work/中文.go#L8C2", "中文", linkTarget{Path: "~/work/中文.go", Line: 8, Column: 2}},
		{`File "/tmp/my project/中文.go":13:2`, "project", linkTarget{Path: "/tmp/my project/中文.go", Line: 13, Column: 2}},
		{"(../README.md),", "README", linkTarget{Path: "../README.md"}},
		{"已保存新版图标 (assets/branding/terminal-icon.png)，使用新版", "terminal", linkTarget{Path: "assets/branding/terminal-icon.png"}},
		{"terminal-icon.prompt.txt)也已保存。", "terminal", linkTarget{Path: "terminal-icon.prompt.txt"}},
		{"已保存（assets/branding/中文.png），继续", "中文", linkTarget{Path: "assets/branding/中文.png"}},
		{"见：src/main.go:42:5，错误", "main", linkTarget{Path: "src/main.go", Line: 42, Column: 5}},
		{"错误 (src/main.ts(12,3))，继续", "main", linkTarget{Path: "src/main.ts", Line: 12, Column: 3}},
		{"已保存 (assets/icon_(copy).png)，继续", "icon", linkTarget{Path: "assets/icon_(copy).png"}},
		{`文件 "assets/图标（新版），副本.png"，已保存`, "新版", linkTarget{Path: "assets/图标（新版），副本.png"}},
		{"文档（https://example.com/a_(b)），继续", "example", linkTarget{URL: "https://example.com/a_(b)"}},
		{"Makefile:4", "Make", linkTarget{Path: "Makefile", Line: 4}},
		{"目录 /tmp/project/", "project", linkTarget{Path: "/tmp/project/"}},
		{"🙂 中文 https://example.com/a_(b)?q=1.", "example", linkTarget{URL: "https://example.com/a_(b)?q=1"}},
		{"see file:///tmp/a%20b.go:4", "a%20", linkTarget{URL: "file:///tmp/a%20b.go:4"}},
		{"plain words without a link", "words", linkTarget{}},
		{"main.go:0", "main", linkTarget{Path: "main.go"}}, // upstream ignores a line 0
	} {
		t.Run(tc.text, func(t *testing.T) {
			text := []rune(tc.text)
			cols := make([]int, len(text))
			col := 0
			for i, r := range text {
				cols[i] = col
				col++
				if r > 0x2fff {
					col++
				}
			}
			i := len([]rune(tc.text[:strings.Index(tc.text, tc.click)]))
			if got := matchLink(text, cols, cols[i]).linkTarget; got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestFileLinkProseBoundaries(t *testing.T) {
	text := []rune("已保存 (assets/branding/terminal-icon.png)，继续")
	cols := make([]int, len(text))
	for i := range cols {
		cols[i] = i
	}
	for i, r := range text {
		if !strings.ContainsRune("()，", r) {
			continue
		}
		if got := matchLink(text, cols, i); got.valid() {
			t.Errorf("punctuation %q matched %+v", r, got.linkTarget)
		}
	}
}

func TestCommandClickWrappedLinksAndMouseTracking(t *testing.T) {
	loadLib(t)
	conn := newPipe()
	var opened []linkTarget
	var term *Terminal
	var err error
	term, err = New(Options{Conn: conn, OpenLink: func(link string) bool {
		// Calling the emulator here also verifies the callback runs unlocked.
		_ = term.Text()
		opened = append(opened, openedTarget(link))
		return true
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 220, 180)
	v := term.v
	point := func(col, row int) (float32, float32) {
		return float32(v.ox+col*v.cellW+v.cellW/2) / v.scale, float32(v.oy+row*v.cellH+v.cellH/2) / v.scale
	}
	url := "https://example.com/" + strings.Repeat("a", v.cols+10)
	term.Feed([]byte("\x1b[?1000h\x1b[?1006h" + url))
	tt.Frame()
	x, y := point(2, 1)
	tt.SetClipboard("keep clipboard")
	tt.ClickAtWith(ui.Cmd, x, y)
	if len(opened) != 1 || opened[0].URL != url {
		t.Fatalf("wrapped URL = %+v", opened)
	}
	if sent := conn.take(term); sent != "" {
		t.Fatalf("Command click reached program: %q", sent)
	}
	if tt.Clipboard() != "keep clipboard" {
		t.Fatal("link click replaced clipboard")
	}
	if tt.Cursor() != ui.CursorPointer || !v.hover.valid() {
		t.Fatal("Command hover not discoverable")
	}
	// Hard newlines must not concatenate unrelated text into a URL.
	term.Feed([]byte("\x1b[H\x1b[2Jhttps://example.com/\r\nsrc/main.go:17:3"))
	tt.Frame()
	x, y = point(5, 1)
	tt.ClickAtWith(ui.Cmd, x, y)
	if len(opened) != 2 || opened[1] != (linkTarget{Path: "src/main.go", Line: 17, Column: 3}) {
		t.Fatalf("file location = %+v", opened)
	}
	// Ordinary pointer clicks still reach mouse-aware terminal programs.
	x, y = point(1, 0)
	tt.ClickAt(x, y)
	if sent := conn.take(term); !strings.Contains(sent, "\x1b[<0;") {
		t.Fatalf("normal mouse reporting broken: %q", sent)
	}
	// OSC 8 display labels retain their URL rather than becoming file paths.
	term.Feed([]byte("\x1b[H\x1b[2J\x1b]8;;https://example.com/hidden\x07label\x1b]8;;\x07"))
	tt.Frame()
	x, y = point(2, 0)
	tt.ClickAtWith(ui.Cmd, x, y)
	if len(opened) != 3 || opened[2].URL != "https://example.com/hidden" {
		t.Fatalf("OSC 8 = %+v", opened)
	}
	// Link opening works on history rows too.
	term.Feed([]byte("\r\n" + strings.Repeat("history\r\n", 50)))
	term.mu.Lock()
	term.term.ScrollToTop()
	term.mu.Unlock()
	tt.Frame()
	x, y = point(2, 0)
	tt.ClickAtWith(ui.Cmd, x, y)
	if len(opened) != 4 || opened[3].URL != "https://example.com/hidden" {
		t.Fatalf("history link = %+v", opened)
	}
}

func TestCommandClickTUIPaintedFileContinuations(t *testing.T) {
	loadLib(t)
	var opened []linkTarget
	term, err := New(Options{Conn: newPipe(), OpenLink: func(link string) bool { opened = append(opened, openedTarget(link)); return true }})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 960, 240)
	v := term.v
	click := func(col, row int) linkTarget {
		t.Helper()
		x := float32(v.ox+col*v.cellW+v.cellW/2) / v.scale
		y := float32(v.oy+row*v.cellH+v.cellH/2) / v.scale
		opened = nil
		tt.ClickAtWith(ui.Cmd, x, y)
		if len(opened) != 1 {
			t.Fatalf("click (%d,%d): opened %v", col, row, opened)
		}
		return opened[0]
	}
	for _, prefix := range []string{"file:///tmp/", "/tmp/", "./images/"} {
		t.Run(prefix, func(t *testing.T) {
			first := prefix + strings.Repeat("a", v.cols-len(prefix)-len("Saved to: ")-len("/exec-")-3) + "/exec-"
			last := "0e64fee3-e2b3-4f2a-99c7-7619d408d347.png"
			term.Feed([]byte("\x1b[H\x1b[2JSaved to: " + first + "\r\n  " + last))
			tt.Frame()
			want := linkTarget{Path: first + last}
			if strings.HasPrefix(prefix, "file://") {
				want = linkTarget{URL: first + last}
			}
			for _, cell := range [][2]int{{12, 0}, {v.cols - 5, 0}, {4, 1}, {len(last), 1}} {
				if got := click(cell[0], cell[1]); got != want {
					t.Fatalf("got %+v, want %+v", got, want)
				}
			}
			for _, suffix := range []string{"),", ")，继续", ")也已保存。", "），继续"} {
				term.Feed([]byte("\x1b[H\x1b[2JSaved to: (" + first + "\r\n  " + last + suffix))
				tt.Frame()
				for _, cell := range [][2]int{{12, 0}, {4, 1}, {len(last), 1}} {
					if got := click(cell[0], cell[1]); got != want {
						t.Fatalf("suffix %q: got %+v, want %+v", suffix, got, want)
					}
				}
			}
		})
	}
	// A TUI can use cursor-addressed painting, without any wrap flag/newline.
	first := "/tmp/" + strings.Repeat("a", v.cols-8) + "-"
	middle := strings.Repeat("b", v.cols-4) + "-"
	term.Feed([]byte("\x1b[H\x1b[2J" + first + "\x1b[2;1H" + middle + "\x1b[3;1Hfinal.png:12:3"))
	tt.Frame()
	want := linkTarget{Path: first + middle + "final.png", Line: 12, Column: 3}
	for row := range 3 {
		if got := click(4, row); got != want {
			t.Fatalf("three-row path = %+v, want %+v", got, want)
		}
	}
	term.Feed([]byte("\x1b[H\x1b[2J(" + first + "\x1b[2;1H" + middle + "\x1b[3;1Hfinal.png(12,3))，继续"))
	tt.Frame()
	for row := range 3 {
		if got := click(4, row); got != want {
			t.Fatalf("wrapped compiler location = %+v, want %+v", got, want)
		}
	}
	for _, tc := range []struct {
		first, second string
		want          linkTarget
	}{
		{"https://example.com/" + strings.Repeat("a", v.cols-23), "other.png", linkTarget{Path: "other.png"}},
		{"/tmp/" + strings.Repeat("a", v.cols-14) + ".png", "other.png", linkTarget{Path: "other.png"}},
		{first, "/tmp/other.png", linkTarget{Path: "/tmp/other.png"}},
		{"file:///tmp/short-", "other.png", linkTarget{Path: "other.png"}},
	} {
		term.Feed([]byte("\x1b[H\x1b[2J" + tc.first + "\r\n" + tc.second))
		tt.Frame()
		if got := click(4, 1); got != tc.want {
			t.Fatalf("unrelated rows %q/%q became %+v", tc.first, tc.second, got)
		}
	}
}

func TestPathAtSpan(t *testing.T) {
	text := []rune("write(notes.md)")
	cols := make([]int, len(text))
	for i := range cols {
		cols[i] = i
	}
	m := matchLink(text, cols, 8)
	if !m.valid() {
		t.Fatal("file reference did not match")
	}
	m.setCells(cols, 0)
	if m.Path != "notes.md" || m.cellA != 6 || m.cellB != 14 {
		t.Fatalf("path = %q %d..%d, want notes.md 6..14", m.Path, m.cellA, m.cellB)
	}
}

func TestURLAtSpan(t *testing.T) {
	text := []rune("go to https://x.org/a. now")
	cols := make([]int, len(text))
	for i := range cols {
		cols[i] = i
	}
	m := matchLink(text, cols, 10)
	if !m.valid() {
		t.Fatal("URL did not match")
	}
	m.setCells(cols, 0)
	if m.URL != "https://x.org/a" || m.cellA != 6 || m.cellB != 21 {
		t.Fatalf("URL = %q %d..%d, want https://x.org/a 6..21", m.URL, m.cellA, m.cellB)
	}
}

// openedTarget reads what OpenLink was given back into its URL, or its path
// and place in the file.
func openedTarget(link string) linkTarget {
	text := []rune(link)
	cols := make([]int, len(text))
	for i := range cols {
		cols[i] = i
	}
	return matchLink(text, cols, 0).linkTarget
}
