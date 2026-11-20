package terminal

import (
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

// TestURLAtNotASCII: a URL ends where text that is not ASCII starts, as
// CJK or full-width punctuation after it with no space.
func TestURLAtNotASCII(t *testing.T) {
	for row, want := range map[string]string{
		"https://example.com/a的说明":      "https://example.com/a",
		"https://example.com/a，然后":      "https://example.com/a",
		"见https://example.com/a。":       "https://example.com/a",
		"（https://example.com/a）":       "https://example.com/a",
		"https://example.com/%E4%BD%A0": "https://example.com/%E4%BD%A0",
		"mailto:me@x.org，谢谢":            "mailto:me@x.org",
	} {
		text := []rune(row)
		cols := make([]int, len(text))
		for i := range cols {
			cols[i] = i
		}
		col := len([]rune(row[:strings.Index(row, ":")]))
		if got, _, _ := urlAt(text, cols, col); got != want {
			t.Errorf("urlAt(%q) = %q, want %q", row, got, want)
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
		// Windows paths, as a whole wherever the pointer is on them.
		{`  ⎿  Read D:\dev\carrel\internal\desktop\links.go`, 30, `D:\dev\carrel\internal\desktop\links.go`},
		{`  ⎿  Read D:\dev\carrel\internal\desktop\links.go`, 13, `D:\dev\carrel\internal\desktop\links.go`},
		{`D:/dev/carrel/main.go:12:3: undefined: x`, 4, `D:/dev/carrel/main.go:12:3`},
		{`error at D:\dev\app\main.go(12,5): bad`, 20, `D:\dev\app\main.go:12:5`},
		{`copied \\server\share\report.pdf`, 12, `\\server\share\report.pdf`},
		{`out: C:\Users\me\a.txt.`, 9, `C:\Users\me\a.txt`},
		{`C:/Users/me/.codex/images/exec-1.png`, 0, `C:/Users/me/.codex/images/exec-1.png`},
		{`C:/Users/me/.codex/images/exec-1.png`, 20, `C:/Users/me/.codex/images/exec-1.png`},
		{`time 12:30:45 ok`, 7, ""},
		{`copied \\server\share\reports`, 12, `\\server\share\reports`},
		// A backslash elsewhere is an escape, not a separator.
		{`echo hello\nworld`, 8, ""},
		{`printf %s\t%s`, 9, ""},
		{`grep \d+ file`, 6, ""},
		{`\section intro`, 3, ""},
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

func TestPathAtSpan(t *testing.T) {
	text := []rune("write(notes.md)")
	cols := make([]int, len(text))
	for i := range cols {
		cols[i] = i
	}
	if path, x0, x1 := pathAt(text, cols, 8); path != "notes.md" || x0 != 6 || x1 != 14 {
		t.Errorf("pathAt = %q %d..%d, want notes.md 6..14", path, x0, x1)
	}
}

func TestURLAtSpan(t *testing.T) {
	text := []rune("go to https://x.org/a. now")
	cols := make([]int, len(text))
	for i := range cols {
		cols[i] = i
	}
	if url, x0, x1 := urlAt(text, cols, 10); url != "https://x.org/a" || x0 != 6 || x1 != 21 {
		t.Errorf("urlAt = %q %d..%d, want https://x.org/a 6..21", url, x0, x1)
	}
}

// A Windows path is a link as a whole, not its last name: the span runs from
// its drive to its end, wherever the pointer is on it.
func TestPathAtWindowsSpan(t *testing.T) {
	row := `  ⎿  Read D:\dev\carrel\main.go (12 lines)`
	path := `D:\dev\carrel\main.go`
	text := []rune(row)
	cols := make([]int, len(text))
	for i := range cols {
		cols[i] = i
	}
	start := len([]rune(row[:strings.Index(row, path)]))
	end := start + len([]rune(path))
	for _, col := range []int{start, start + 3, end - 1} {
		if got, x0, x1 := pathAt(text, cols, col); got != path || x0 != start || x1 != end {
			t.Errorf("pathAt(%d) = %q %d..%d, want %q %d..%d", col, got, x0, x1, path, start, end)
		}
	}
}
