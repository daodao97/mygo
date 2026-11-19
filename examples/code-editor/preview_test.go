package main

import (
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

// The marks hide, but on the lines with a caret.
func TestLivePreview(t *testing.T) {
	st := &editor.State{Buffer: editor.NewBuffer("a **b** c\n# Title\n- [ ] task\n- item\n")}
	st.Select(0, 0)
	shown := func(rs []editor.Replacement) []string {
		var out []string
		for _, r := range rs {
			out = append(out, st.Buffer.Slice(r.Start, r.End))
		}
		return out
	}
	rs, _ := livePreview(st)
	got := shown(rs)
	want := []string{"# ", "- [ ]", "-"}
	if len(got) != len(want) {
		t.Fatalf("replaced %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("replaced %q, want %q", got, want)
		}
	}
	// The caret off the first line, its marks hide too.
	st.Select(st.Buffer.Len(), st.Buffer.Len())
	rs, _ = livePreview(st)
	if got := shown(rs); len(got) != 5 || got[0] != "**" || got[1] != "**" {
		t.Fatalf("replaced %q", got)
	}
}

// A table shows in place of its lines, and an image under its line, but
// the source of a table with a caret shows.
func TestLivePreviewBlocks(t *testing.T) {
	src := "top\n\n![Cat](cat.png)\n\n| a | b |\n| - | - |\n| 1 | 2 |\n\nend\n"
	st := &editor.State{Buffer: editor.NewBuffer(src)}
	st.Select(0, 0)
	rs, blocks := livePreview(st)
	if len(rs) != 1 || st.Buffer.Slice(rs[0].Start, rs[0].End) != "| a | b |\n| - | - |\n| 1 | 2 |\n" {
		t.Fatalf("replaced %+v", rs)
	}
	if len(blocks) != 2 || blocks[0].Line != 2 || blocks[0].Label != "Cat" || blocks[1].Line != 4 || blocks[1].Label != "Table" {
		t.Fatalf("blocks %+v", blocks)
	}
	at := st.Buffer.Offset(editor.Pos{Line: 5})
	st.Select(at, at)
	rs, blocks = livePreview(st)
	if len(rs) != 0 || len(blocks) != 1 {
		t.Fatalf("with a caret in the table: %+v, %+v", rs, blocks)
	}
}

// Headings show larger by their level, those in code not at all.
func TestHeadingStyles(t *testing.T) {
	b := editor.NewBuffer("# Title\ntext\n### Small\n```\n# not a heading\n```\n")
	got := headingStyles(b)
	if len(got) != 2 || got[0].Line != 0 || got[1].Line != 2 || got[0].Font.Size <= got[1].Font.Size {
		t.Fatalf("styles %+v", got)
	}
}
