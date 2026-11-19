package editor

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestStickyBlocksFastPaths(t *testing.T) {
	a := block{kind: blockDeleted, height: 10, sticky: true, from: 1}
	b := block{kind: blockAnnotation, height: 20, sticky: true, from: 2}
	c := block{kind: blockSeparator, height: 30, sticky: true, from: 3}
	x := block{kind: blockSpacer, height: 40, from: 4}
	for _, tc := range []struct {
		name   string
		in     []block
		want   []block
		allocs float64
	}{
		{"nil", nil, nil, 0},
		{"empty", make([]block, 0, 3), nil, 0},
		{"one-sticky", []block{a}, []block{a}, 0},
		{"one-hidden", []block{x}, nil, 0},
		{"all-sticky", []block{a, b, c}, []block{a, b, c}, 0},
		{"none", []block{x, x, x}, nil, 0},
		{"mixed", []block{x, a, x, b, c, x}, []block{a, b, c}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := slices.Clone(tc.in[:cap(tc.in)])
			var got []block
			allocs := testing.AllocsPerRun(20, func() { got = stickyBlocks(tc.in) })
			if allocs != tc.allocs {
				t.Errorf("filter allocated %v times, want %v", allocs, tc.allocs)
			}
			if !slices.Equal(got, tc.want) || !slices.Equal(tc.in[:cap(tc.in)], before) {
				t.Fatalf("filter changed its input or order: got %+v, input %+v", got, tc.in)
			}
			if len(got) == len(tc.in) {
				if cap(got) != cap(tc.in) || cap(got) > 0 && &got[:cap(got)][0] != &tc.in[:cap(tc.in)][0] {
					t.Error("an unfiltered slice must retain its original backing array")
				}
			} else if len(got) > 0 {
				// Mixed diff blocks need their own filtered slice, not an
				// in-place compaction that corrupts the height snapshot.
				got[0].height++
				if !slices.Equal(tc.in[:cap(tc.in)], before) {
					t.Error("mixed output aliases and mutates the input")
				}
			}
		})
	}
}

// A hidden-only text pass needs no geometry, even when a height cache
// is stale. Mixed blocks make accidental geometry allocate as well.
func TestPaintLinesSkipsHiddenGeometry(t *testing.T) {
	ed := blockModel()
	h := &ed.view.heights
	h.setLines(ed.view.text.LineCount())
	h.setHidden([]lineSpan{{1, 4}})
	h.setBlocks([]block{
		{line: 1, kind: blockDeleted, height: 10, sticky: true},
		{line: 1, kind: blockSpacer, height: 100},
		{line: 1, kind: blockSeparator, height: 20, sticky: true},
		{line: 1, below: true, kind: blockAnnotation, height: 30, sticky: true},
		{line: 1, below: true, kind: blockHeader, height: 100},
		{line: 1, below: true, kind: blockApp, height: 40, sticky: true},
	})
	before := slices.Clone(h.blocks)
	total := h.total()
	h.dirty = true
	f := frame{first: 1, last: 4}
	allocs := testing.AllocsPerRun(20, func() { ed.paintLines(nil, ui.Rect{}, &f) })
	if allocs != 0 || !h.dirty {
		t.Errorf("hidden text painting allocated %v times or consumed the stale geometry cache (dirty=%v)", allocs, h.dirty)
	}
	// Block consumers still get correct geometry after the text pass.
	// Filtering above and below must preserve the mixed diff's order.
	above, below := h.blocksOf(1)
	if !slices.Equal(above, []block{before[0], before[2]}) || !slices.Equal(below, []block{before[3], before[5]}) {
		t.Fatalf("mixed hidden blocks: above %+v, below %+v", above, below)
	}
	g := h.geometry(1)
	top := float64(h.lh)
	rows := top + float64(before[0].height+before[2].height)
	bottom := rows + float64(before[3].height+before[5].height)
	if !g.hidden || g.top != top || g.rows != rows || g.below != rows || g.bottom != bottom || h.dirty || h.total() != total || !slices.Equal(h.blocks, before) {
		t.Fatalf("the hidden paint pass or filtering corrupted cached geometry: %+v", g)
	}
	// Changing hidden lines must expose all blocks again, without a
	// filtered slice having removed the nonsticky entries from storage.
	h.setHidden(nil)
	above, below = h.blocksOf(1)
	if !slices.Equal(above, before[:3]) || !slices.Equal(below, before[3:]) {
		t.Fatal("a hidden-line change lost the previously filtered blocks")
	}
	g = h.geometry(1)
	if g.hidden || g.below-g.rows != float64(h.lh) || !slices.Equal(h.blocks, before) {
		t.Fatalf("the shown line reused stale hidden geometry: %+v", g)
	}
}
