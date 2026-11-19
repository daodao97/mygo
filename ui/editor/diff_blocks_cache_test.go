package editor

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// preparedBlocksDiff has separated changes with annotations on both
// sides. With no context, separators share a position with deleted rows
// and the trailing separator follows the last annotation and spacer.
func preparedBlocksDiff(t testing.TB, layout DiffLayout, n int) (*DiffState, *ui.Tester) {
	t.Helper()
	lines := make([]string, n)
	for i := range lines {
		lines[i] = "line " + strconv.Itoa(i)
	}
	old := strings.Join(lines, "\n") + "\n"
	var anns []DiffAnnotation
	for i := 5; i < n; i += 20 {
		lines[i] += " changed"
		anns = append(anns, DiffAnnotation{Side: DiffOld, Line: i, Height: 10}, DiffAnnotation{Side: DiffNew, Line: i, Height: 30})
	}
	d := &DiffState{Old: NewBuffer(old), New: NewBuffer(strings.Join(lines, "\n") + "\n"), Layout: layout,
		ContextLines: -1, Annotations: anns}
	d.RenderAnnotation = func(c *ui.Context, a DiffAnnotation) { ui.Box(c).Height(a.Height) }
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 1000, 600)
	tt.Frame()
	for i := 0; tt.FramePending(); i++ {
		if i > 10 {
			t.Fatal("the diff frames do not settle")
		}
		tt.Frame()
	}
	return d, tt
}

// Prepared diffs are already ordered when heights takes its value
// snapshot. An unchanged frame must not copy or sort through scratch.
func TestDiffPreparedBlocksStable(t *testing.T) {
	for _, layout := range []DiffLayout{DiffSplit, DiffUnified} {
		t.Run(strconv.Itoa(int(layout)), func(t *testing.T) {
			d, tt := preparedBlocksDiff(t, layout, 40)
			gaps := 0
			for _, g := range d.gapList {
				if g.hidden > 0 {
					gaps++
				}
			}
			if gaps < 2 || len(d.Hunks()) != 2 {
				t.Fatalf("the fixture has %d collapsed gaps and %d hunks", gaps, len(d.Hunks()))
			}
			first := 0
			if layout == DiffUnified {
				first = 1
			}
			var pointers [2]*block
			var snapshots [2][]block
			for side := first; side < 2; side++ {
				bs := d.sides[side].blocks()
				seps, anns := 0, 0
				for i, b := range bs {
					if b.kind == blockSeparator {
						seps++
					}
					if b.kind == blockAnnotation {
						anns++
					}
					if i > 0 {
						prev := bs[i-1]
						if prev.line > b.line || prev.line == b.line && prev.below && !b.below {
							t.Errorf("side %d is not prepared in height order: %+v then %+v", side, prev, b)
						}
					}
				}
				if seps < 2 || anns == 0 {
					t.Fatalf("side %d has %d separators and %d annotations", side, seps, anns)
				}
				kinds := func(line int, below bool) []blockKind {
					var out []blockKind
					for _, b := range bs {
						if b.line == line && b.below == below {
							out = append(out, b.kind)
						}
					}
					return out
				}
				above := []blockKind{blockSeparator}
				if layout == DiffUnified {
					above = append(above, blockDeleted)
				}
				for _, line := range []int{5, 25} {
					if got := kinds(line, false); !slices.Equal(got, above) {
						t.Errorf("side %d line %d: above order %v, want %v", side, line, got, above)
					}
				}
				tail := []blockKind{blockAnnotation, blockSeparator}
				if layout == DiffSplit && side == 0 {
					tail = []blockKind{blockAnnotation, blockSpacer, blockSeparator}
				}
				if got := kinds(25, true); !slices.Equal(got, tail) {
					t.Errorf("side %d: annotation/spacer/late order %v, want %v", side, got, tail)
				}
				pointers[side], snapshots[side] = &bs[0], slices.Clone(bs)
				h := &d.eds[side].view.heights
				h.setBlocks(bs)
				h.total()
				if &h.blocks[0] == &bs[0] {
					t.Fatal("heights must own a snapshot, not alias the prepared input")
				}
				// A canary catches copies/sorts even when they allocate nothing
				// and the final sorted values leave heights clean.
				h.scratch = make([]block, len(bs))
				for i := range h.scratch {
					h.scratch[i] = block{line: -1, height: -1}
				}
			}
			for range 5 {
				tt.Frame()
				// Exercise setBlocks even if the UI skips an idle draw.
				for side := first; side < 2; side++ {
					d.eds[side].view.heights.setBlocks(d.sides[side].blocks())
				}
			}
			for side := first; side < 2; side++ {
				h := &d.eds[side].view.heights
				for _, b := range h.scratch {
					if b != (block{line: -1, height: -1}) {
						t.Errorf("side %d copied or sorted unchanged prepared blocks through scratch", side)
						break
					}
				}
				if h.dirty || !slices.Equal(h.blocks, snapshots[side]) {
					t.Errorf("side %d changed its height snapshot in an unchanged frame", side)
				}
			}
			// Even a fresh preparation with equal values reuses the immutable
			// side blocks and does not invalidate the viewer's elements.
			version := d.blocksV
			d.prepared = diffPrepared{}
			d.prepare()
			for side := first; side < 2; side++ {
				bs := d.sides[side].blocks()
				if &bs[0] != pointers[side] || !slices.Equal(bs, snapshots[side]) || d.blocksV != version {
					t.Errorf("side %d replaced or mutated equal prepared blocks", side)
				}
			}
		})
	}
}

// Repeated height updates of a prepared diff with 100 changes, collapsed
// gaps and annotations on both sides. This isolates the unchanged block
// path, without timing the CPU renderer.
func BenchmarkDiffPreparedBlockHeights(b *testing.B) {
	for _, layout := range []DiffLayout{DiffSplit, DiffUnified} {
		b.Run(strconv.Itoa(int(layout)), func(b *testing.B) {
			d, _ := preparedBlocksDiff(b, layout, 2000)
			first := 0
			if layout == DiffUnified {
				first = 1
			}
			b.ReportAllocs()
			for b.Loop() {
				for side := first; side < 2; side++ {
					h := &d.eds[side].view.heights
					h.setBlocks(d.sides[side].blocks())
					h.total()
				}
			}
		})
	}
}
