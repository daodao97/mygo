package ui

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/internal/text"
)

// TestPreserveSpaces checks that spaces keep their advance on a text that
// asks for it (CSS Text 3, white-space: break-spaces), and are dropped at
// the end of a line otherwise (white-space: normal).
func TestPreserveSpaces(t *testing.T) {
	tt := coreNewTester(func(c *context) {
		coreRow(c).Children(func() {
			coreText(c, "  ").Label("gap").PreserveSpaces()
			coreText(c, "a").Label("a1")
		})
		coreRow(c).Children(func() {
			coreText(c, "  ").Label("plain gap")
			coreText(c, "a").Label("a2")
		})
		coreRow(c).Children(func() {
			coreText(c, "a  ").Label("kept").PreserveSpaces()
		})
		coreRow(c).Children(func() {
			coreText(c, "a  ").Label("dropped")
		})
	}, 300, 200)
	rect := func(l string) Rect {
		r, ok := tt.Find(l)
		if !ok {
			t.Fatalf("no element %q", l)
		}
		return r
	}
	if g, a := rect("gap"), rect("a1"); g.W <= 0 || a.X <= g.X {
		t.Errorf("a text of spaces is %v wide, and the next text starts at %v", g.W, a.X-g.X)
	}
	if g := rect("plain gap"); g.W != 0 {
		t.Errorf("without it, a text of spaces is %v wide", g.W)
	}
	if k, d := rect("kept"), rect("dropped"); k.W <= d.W {
		t.Errorf("trailing spaces: %v wide kept, %v dropped", k.W, d.W)
	}
}

// TestTruncateMiddle cuts a path in its middle in the room layout gives
// it beside an icon, keeping its start and its file's name, and again in
// the room a narrower window gives it.
func TestTruncateMiddle(t *testing.T) {
	const path = "src/components/settings/appearance/theme/picker.go"
	var label *node
	tt := coreNewTester(func(c *context) {
		coreRow(c).Gap(8).Padding(0, 10).Children(func() {
			coreBox(c).Width(16).Height(16)
			label = coreText(c, path).SingleLine().TruncateMiddle().Shrink(1).MinWidth(0)
		})
	}, 240, 100)
	check := func(room float32) {
		t.Helper()
		l := label.tl
		line := l.Lines[0]
		first, last := line.Glyphs[0], line.Glyphs[len(line.Glyphs)-1]
		mark := slices.IndexFunc(line.Glyphs, func(g text.Glyph) bool { return g.Runes == 0 })
		if !l.Truncated || l.Params.Width != room || line.Width > room || first.Cluster != 0 || last.Cluster+last.Runes != len(path) || mark <= 0 || mark == len(line.Glyphs)-1 {
			t.Errorf("in %v (laid out in %v), truncated %v, %v wide, the ellipsis glyph %d of %d, ending with %+v", room, l.Params.Width, l.Truncated, line.Width, mark, len(line.Glyphs), last)
		}
	}
	check(240 - 2*10 - 16 - 8)
	wide := label.tl.Lines[0].Width
	tt.SetSize(180, 100)
	check(180 - 2*10 - 16 - 8)
	if label.tl.Lines[0].Width >= wide {
		t.Errorf("narrower, the label is %v wide, as wide as %v", label.tl.Lines[0].Width, wide)
	}
}

// TestTruncateStart cuts a path at its start in the room layout gives it
// beside an icon, keeping its file's name and the folders nearest it,
// again in the room a narrower window gives it, and not at all where a
// wider one gives it room. A label that asks for its middle to be cut
// too has its start cut alike.
func TestTruncateStart(t *testing.T) {
	const path = "src/components/settings/appearance/theme/picker.go"
	var label, both *node
	tt := coreNewTester(func(c *context) {
		coreRow(c).Gap(8).Padding(0, 10).Children(func() {
			coreBox(c).Width(16).Height(16)
			label = coreText(c, path).SingleLine().TruncateStart().Shrink(1).MinWidth(0)
		})
		coreRow(c).Gap(8).Padding(0, 10).Children(func() {
			coreBox(c).Width(16).Height(16)
			both = coreText(c, path).SingleLine().TruncateMiddle().TruncateStart().Shrink(1).MinWidth(0)
		})
	}, 240, 100)
	check := func(label *node, room float32) {
		t.Helper()
		l := label.tl
		line := l.Lines[0]
		first, last := line.Glyphs[0], line.Glyphs[len(line.Glyphs)-1]
		marks := 0
		for _, g := range line.Glyphs {
			if g.Runes == 0 {
				marks++
			}
		}
		if !l.Truncated || l.Params.Width != room || line.Width > room || first.Runes != 0 || first.Cluster != 0 || marks != 1 || last.Cluster+last.Runes != len(path) {
			t.Errorf("in %v (laid out in %v), truncated %v, %v wide, %d ellipsis glyphs, starting with %+v, ending with %+v", room, l.Params.Width, l.Truncated, line.Width, marks, first, last)
		}
	}
	check(label, 240-2*10-16-8)
	check(both, 240-2*10-16-8)
	wide := label.tl.Lines[0].Width
	tt.SetSize(180, 100)
	check(label, 180-2*10-16-8)
	if label.tl.Lines[0].Width >= wide {
		t.Errorf("narrower, the label is %v wide, as wide as %v", label.tl.Lines[0].Width, wide)
	}
	tt.SetSize(1000, 100)
	if l := label.tl; l.Truncated || l.Lines[0].Glyphs[0].Runes == 0 {
		t.Errorf("with room for it all, truncated %v, starting with %+v", l.Truncated, l.Lines[0].Glyphs[0])
	}
}
