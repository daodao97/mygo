package editor

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Both sort paths must preserve same-position order without allocating
// an interface wrapper on each call, including genuinely unsorted input.
func TestBlocksPointerSortingStableNoAlloc(t *testing.T) {
	content := func(*ui.Context, ui.Element, Block) {}
	bs := []Block{
		{Line: 4, Content: content, Label: "four-first"},
		{Line: 1, Content: content, Label: "one-below-first"},
		{Line: 3, Above: true, Content: content, Label: "three-first"},
		{Line: 1, Above: true, Content: content, Label: "one-above"},
		{Line: 4, Content: content, Label: "four-second"},
		{Line: 1, Content: content, Label: "one-below-second"},
		{Line: 3, Above: true, Content: content, Label: "three-second"},
		{Line: 0, Content: content, Label: "zero"},
	}
	t.Run("unsorted-setters", func(t *testing.T) {
		ed := blockModel()
		for range 3 {
			ed.setBlocks(bs)
			syncBlockRows(ed)
		}
		rows := ed.blockRows()
		want := slices.Clone(rows)
		stale, moved := false, false
		allocs := testing.AllocsPerRun(20, func() {
			ed.setBlocks(bs)
			stale = stale || ed.view.blocksStale
			next := syncBlockRows(ed)
			moved = moved || &next[0] != &rows[0]
		})
		if allocs != 0 || stale || moved || !slices.Equal(ed.blockRows(), want) {
			t.Fatalf("repeated unsorted input: allocations %v, stale %v, moved %v", allocs, stale, moved)
		}
		labels := []string{"zero", "one-above", "one-below-first", "one-below-second", "three-first", "three-second", "four-first", "four-second"}
		for i, label := range labels {
			if ed.view.blocks[i].Label != label {
				t.Fatalf("block %d: label %q, want %q", i, ed.view.blocks[i].Label, label)
			}
		}
	})
	t.Run("settled-joins", func(t *testing.T) {
		ed := blockModel()
		ed.setBlocks(bs)
		v := &ed.view
		seed := slices.Clone(v.blocks)
		var want []int
		// Joining the lines puts all blocks above first, then below,
		// preserving the original order within each placement.
		for _, above := range []bool{true, false} {
			for i := range seed {
				if seed[i].Above == above {
					want = append(want, seed[i].id)
				}
			}
		}
		allocs := testing.AllocsPerRun(20, func() {
			copy(v.blocks, seed)
			v.blocksStale = false
			for i := range v.blocks {
				v.blocks[i].at = 0 // all mapped to the surviving line
			}
			v.settleBlocks()
		})
		if allocs != 0 || !v.blocksStale {
			t.Fatalf("settling joined lines: allocations %v, stale %v", allocs, v.blocksStale)
		}
		for i, id := range want {
			if b := &v.blocks[i]; b.id != id || b.Line != 0 || b.at != 0 {
				t.Fatalf("settled block %d: id %d at line %d offset %d, want id %d at line 0", i, b.id, b.Line, b.at, id)
			}
		}
	})
}
