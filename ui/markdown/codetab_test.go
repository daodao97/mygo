package markdown

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestCodeBlockTabs indents code with tabs of 4 columns, as the editor
// does, on every system: without a tab size, a tab is as wide as the
// system's text stack sets it, a single space in a browser.
func TestCodeBlockTabs(t *testing.T) {
	src := "```\nab\n```\n\n```\na b\n```\n\n```\n\tb\n```"
	// At a size where 4 spaces aren't the 28 points of macOS's own stops.
	o := &Options{Theme: &Theme{FontSize: 24}}
	tt := ui.NewTester(func(c *ui.Context) { View(c, src, o) }, 600, 400)
	width := func(s string) float32 {
		r, ok := tt.Find(s)
		if !ok {
			t.Fatalf("no %q in %q", s, tt.Texts())
		}
		return r.W
	}
	space := width("a b") - width("ab")
	letter := width("ab") / 2
	if d := width("\tb") - (4*space + letter); d < -0.5 || d > 0.5 {
		t.Errorf("a tab and a letter are %v wide, want 4 spaces of %v and a letter of %v", width("\tb"), space, letter)
	}
}
