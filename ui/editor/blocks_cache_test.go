package editor

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func blockModel() *editor {
	ed := &editor{}
	ed.view.setBuffer(NewBuffer("head\none\ntwo\nthree\nfour\nend"))
	ed.view.lineHeight, ed.view.heights.lh = 20, 20
	return ed
}

func syncBlockRows(ed *editor) []block {
	ed.view.heights.setHidden(ed.hiddenLines())
	rows := ed.blockRows()
	ed.view.heights.setBlocks(rows)
	ed.view.heights.total()
	return rows
}

// Normalization, duplicate matching and folds cost no row rebuild or
// allocation once both scratch buffers have been used.
func TestBlocksSetEveryFrameStable(t *testing.T) {
	content := func(*ui.Context, ui.Element, Block) {}
	valid := Block{Line: 2, Height: 30, Content: content}
	invalid := []Block{{Line: -1, Content: content}, {Line: 6, Content: content}, {Line: 2}}
	for _, tc := range []struct {
		name  string
		bs    []Block
		lines []int
	}{
		{"empty", nil, nil},
		{"invalid", invalid, nil},
		{"single", []Block{valid}, []int{2}},
		{"duplicates", []Block{valid, {Line: 1, Above: true, Height: 10, Content: content}, valid}, []int{1, 2, 2}},
		{"filtered", append(slices.Clone(invalid), valid), []int{2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ed := blockModel()
			ed.folds = []fold{{0, 1}}
			rs := []Replacement{hide(ed.view.text.LineStart(1), ed.view.text.LineStart(4))}
			for range 3 {
				ed.setReplacements(rs)
				ed.setBlocks(tc.bs)
				syncBlockRows(ed)
			}
			rows := ed.blockRows()
			if len(rows) != len(tc.lines) {
				t.Fatalf("normalized %d rows, want %d", len(rows), len(tc.lines))
			}
			ids := map[int]bool{}
			for i, row := range rows {
				if row.line != tc.lines[i] || ids[row.from] {
					t.Fatalf("normalized order or duplicate ID is wrong: %+v", rows)
				}
				ids[row.from] = true
			}
			want := slices.Clone(rows)
			version := ed.view.replVersion
			stale, dirty, moved := false, false, false
			allocs := testing.AllocsPerRun(20, func() {
				ed.setReplacements(rs)
				ed.setBlocks(tc.bs)
				stale = stale || ed.view.blocksStale
				ed.view.heights.setHidden(ed.hiddenLines())
				next := ed.blockRows()
				moved = moved || len(rows) > 0 && &next[0] != &rows[0]
				ed.view.heights.setBlocks(next)
				dirty = dirty || ed.view.heights.dirty
				ed.view.heights.total()
				ed.standIn(2)
			})
			if ed.view.replVersion != version {
				t.Error("unchanged replacements invalidated block eligibility")
			}
			if stale || dirty || moved || !slices.Equal(ed.blockRows(), want) {
				t.Errorf("unchanged rows: stale=%v dirty=%v moved=%v", stale, dirty, moved)
			}
			if allocs != 0 {
				t.Errorf("unchanged set/layout/stand-in allocated %v times, want 0", allocs)
			}
		})
	}
}

// Heights own a value snapshot, not a reference to the caller's reused
// row buffer. Sorting must preserve the order of blocks at one place.
func TestBlocksHeightSnapshot(t *testing.T) {
	h := heights{lh: 20}
	h.setLines(4)
	bs := []block{
		{line: 2, below: true, height: 10, kind: blockApp, from: 1},
		{line: 1, height: 20, kind: blockApp, from: 2},
		{line: 1, height: 30, kind: blockApp, from: 3},
	}
	h.setBlocks(bs)
	h.total()
	bs[0].height = 40
	h.setBlocks(bs)
	if got, want := h.total(), float64(4*h.lh+40+20+30); got != want {
		t.Errorf("mutated height: %v, want %v", got, want)
	}
	bs[1], bs[2] = bs[2], bs[1]
	h.setBlocks(bs)
	if h.blocks[0].from != 3 || h.blocks[1].from != 2 {
		t.Errorf("duplicate order was lost: %+v", h.blocks)
	}
	bs[0].line, bs[0].below, bs[0].from = 0, false, 4
	h.setBlocks(bs)
	if h.blocks[0] != bs[0] {
		t.Errorf("mutated line, placement or ID was missed: %+v", h.blocks)
	}
	bs = bs[:1]
	h.setBlocks(bs)
	if len(h.blocks) != 1 || h.blocks[0] != bs[0] {
		t.Errorf("single block was missed: %+v", h.blocks)
	}
	h.setBlocks(nil)
	if len(h.blocks) != 0 || h.total() != float64(4*h.lh) {
		t.Errorf("empty blocks were missed: %+v", h.blocks)
	}
}

func TestBlocksRowsGeometryChanges(t *testing.T) {
	ed := blockModel()
	b := Block{Line: 1, Height: 10, Content: func(*ui.Context, ui.Element, Block) {}}
	ed.setBlocks([]Block{b})
	rows := syncBlockRows(ed)
	id := rows[0].from
	for _, change := range []struct {
		name string
		edit func()
	}{
		{"height", func() { b.Height = 30 }},
		{"placement", func() { b.Above = true }},
		{"line", func() { b.Line = 2 }},
		{"ID", func() { ed.setBlocks(nil); syncBlockRows(ed) }},
	} {
		t.Run(change.name, func(t *testing.T) {
			change.edit()
			ed.setBlocks([]Block{b})
			if !ed.view.blocksStale {
				t.Error("changed geometry did not mark rows stale")
			}
			next := syncBlockRows(ed)
			if next[0].line != b.Line || next[0].below == b.Above || next[0].height != b.Height {
				t.Fatalf("geometry was not updated: %+v", next)
			}
			if change.name != "height" && next[0].from == id {
				t.Error("the changed block kept the old ID")
			}
			id = next[0].from
			if got, want := ed.view.heights.total(), float64(6*ed.view.lineHeight+b.Height); got != want {
				t.Errorf("height %v, want %v", got, want)
			}
		})
	}
	// An edit within the same line does not change block rows.
	ed.view.buf.Edit(Edit{Start: 1, End: 1, Text: "x"})
	ed.view.sync(nil)
	if ed.view.blocksStale {
		t.Error("an edit with unchanged block lines marked rows stale")
	}
	// A new line moves the block, even though the row buffer is reused.
	ed.view.buf.Edit(Edit{Start: 0, End: 0, Text: "new\n"})
	ed.view.sync(nil)
	if got := syncBlockRows(ed)[0].line; got != b.Line+1 {
		t.Errorf("the edited block's line is %d, want %d", got, b.Line+1)
	}
}

// Only an unmeasured block uses Height or the line height. A new guess
// for an already measured block must not invalidate its effective row.
func TestBlocksRowsHeightInputs(t *testing.T) {
	ed := blockModel()
	b := Block{Line: 1, Content: func(*ui.Context, ui.Element, Block) {}}
	ed.setBlocks([]Block{b})
	rows := syncBlockRows(ed)
	if rows[0].height != ed.view.lineHeight {
		t.Fatal("the unmeasured block did not use the line height")
	}
	ed.view.lineHeight = 30
	if next := syncBlockRows(ed); next[0].height != 30 || &next[0] != &rows[0] {
		t.Fatalf("a new line height was missed or replaced the row buffer: %+v", next)
	}
	// Measuring the existing guess changes no geometry.
	ed.view.blocks[0].height, ed.view.blocks[0].measured = 30, true
	b.Height = 500
	ed.setBlocks([]Block{b})
	if ed.view.blocksStale {
		t.Error("a new guess changed a measured block's geometry")
	}
	ed.view.lineHeight = 40
	if got := syncBlockRows(ed)[0].height; got != 30 {
		t.Errorf("the measured block's height is %v, want 30", got)
	}
}

// Fold spans must not share the buffer that hiddenLines appends and
// merges replacements into, or the snapshot blockRows keeps of folds.
func TestBlocksFoldCacheIsSeparate(t *testing.T) {
	ed := blockModel()
	ed.setBlocks([]Block{{Line: 2, Content: func(*ui.Context, ui.Element, Block) {}}})
	r := hide(ed.view.text.LineStart(1), ed.view.text.LineStart(4))
	ed.setReplacements([]Replacement{r})
	ed.folds = []fold{{0, 1}}
	ed.o.decor = &diffSide{hide: []lineSpan{{3, 4}}}
	if !syncBlockRows(ed)[0].sticky {
		t.Fatal("a replacement was mistaken for a fold")
	}
	if got := ed.hiddenSpans(); !slices.Equal(got, []lineSpan{{1, 1}}) {
		t.Fatalf("replacement spans corrupted folds: %+v", got)
	}
	// Mutate a fold in place, then remove it without changing capacity.
	ed.folds[0].end = 2
	if syncBlockRows(ed)[0].sticky {
		t.Error("an in-place fold change was missed")
	}
	ed.folds = ed.folds[:0]
	if !syncBlockRows(ed)[0].sticky {
		t.Error("removing a fold was missed")
	}
	for _, text := range []string{"shown", "", "shown"} {
		r.Text = text
		ed.setReplacements([]Replacement{r})
		if got, want := syncBlockRows(ed)[0].sticky, text == ""; got != want {
			t.Errorf("replacement %q: sticky %v, want %v", text, got, want)
		}
	}
	r.Text = ""
	r.Start++
	ed.setReplacements([]Replacement{r})
	if syncBlockRows(ed)[0].sticky {
		t.Error("moving the replacement start inside a line was missed")
	}
	r.Start--
	ed.setReplacements([]Replacement{r})
	if !syncBlockRows(ed)[0].sticky {
		t.Error("restoring the replacement's whole-line start was missed")
	}
	ed.setReplacements(nil)
	if syncBlockRows(ed)[0].sticky {
		t.Error("removing a replacement was missed")
	}
}

// Updating callbacks and accessibility/input options does not change
// row geometry, but every update must still reach the block's UI.
func TestBlocksSetUpdatesContent(t *testing.T) {
	ct := newEditorTest(t, "", "one\ntwo")
	ct.st.SetBlocks([]Block{tall(0, false, 40, "Original")})
	ct.settle()
	rows := ct.st.ed.blockRows()
	id := rows[0].from
	clicks := 0
	updated := tall(0, false, 40, "Updated")
	updated.Interactive = true
	updated.Content = func(c *ui.Context, e ui.Element, b Block) {
		e.Children(func() {
			if ui.ButtonBase(c).Size(80, 40).KeepFocus().Label("New content").Clicked() {
				clicks++
			}
		})
	}
	ct.st.SetBlocks([]Block{updated})
	if ct.st.ed.view.blocksStale {
		t.Error("a content-only update marked row geometry stale")
	}
	ct.settle()
	if _, ok := ct.tt.Find("Original"); ok {
		t.Error("the original label is still built")
	}
	if _, ok := ct.tt.Find("Updated"); !ok {
		t.Error("the updated label is not built")
	}
	if err := ct.tt.Click("New content"); err != nil {
		t.Fatal(err)
	}
	ct.settle()
	if clicks != 1 {
		t.Errorf("the updated interactive content received %d clicks", clicks)
	}
	if next := ct.st.ed.blockRows(); &next[0] != &rows[0] || next[0].from != id {
		t.Error("a content-only update replaced the row or its ID")
	}
}
