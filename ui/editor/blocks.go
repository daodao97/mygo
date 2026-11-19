package editor

import (
	"slices"
	"sort"
	"strconv"

	"github.com/egoist/mygo/ui"
)

// Block is an element between the lines of the text, as Zed's blocks: an
// image under the line naming it, say. It is no part of the text, which
// stays the buffer's. On a line an empty replacement hides, starting at a
// line boundary, it shows in place of the lines the replacement covers,
// as a table drawn in place of its source.
type Block struct {
	// Line is the line it goes with, from 0.
	Line int
	// Above puts it above the line, else it goes below.
	Above bool
	// Content builds it: e is the element the editor places there, as
	// wide as the text layout (including WrapColumn), and as high as
	// what Content puts in it. Style and fill it as the editor's parts
	// (RenderFindBar). b is the block as edits since it was set moved it.
	// A press on it goes to the text, unless Interactive.
	Content func(c *ui.Context, e ui.Element, b Block)
	// Height is how high it is taken to be until it has shown, as only
	// those in view are laid out: a row when 0. Its element then says.
	Height float32
	// Interactive keeps the presses on Content's element for what is in
	// it, as a button's, rather than the text's, the wheel still
	// scrolling. Give a button KeepFocus to keep the typing in the editor.
	Interactive bool
	// Label names Content's element for assistive technology.
	Label string
}

// appBlock is a block the app gave, with what the editor keeps of it.
type appBlock struct {
	Block
	at       int // where its line starts, which edits move
	id       int // names its element from frame to frame
	height   float32
	measured bool // height is what the element's layout said
}

// appBlocks compares blocks by pointer. Sorting through a view's slice
// field avoids boxing a temporary slice header for sort.Interface.
type appBlocks []appBlock

func (bs *appBlocks) Len() int { return len(*bs) }
func (bs *appBlocks) Less(i, j int) bool {
	return compareBlocks(&(*bs)[i], &(*bs)[j]) < 0
}
func (bs *appBlocks) Swap(i, j int) { (*bs)[i], (*bs)[j] = (*bs)[j], (*bs)[i] }

// rowHeight is the measured height, or the app's guess until measured.
func (b *appBlock) rowHeight(lineHeight float32) float32 {
	if b.measured {
		return b.height
	}
	if b.Height > 0 {
		return b.Height
	}
	return lineHeight
}

// builtBlock is the element the last build made of view.blocks[i], whose
// layout says how high the block is.
type builtBlock struct {
	i, id int
	e     ui.Element
}

func compareBlocks(a, b *appBlock) int {
	if a.Line != b.Line {
		return a.Line - b.Line
	}
	switch {
	case a.Above == b.Above:
		return 0
	case a.Above:
		return -1
	}
	return 1
}

// setBlocks shows bs, sorted by where they go. Those at the place of one
// shown, in order, keep its element and the height it measured, so that
// setting them again as the text changes costs no layout.
func (ed *editor) setBlocks(bs []Block) {
	v := &ed.view
	n := v.text.LineCount()
	in := v.blockScratch[:0]
	for _, b := range bs {
		if b.Content != nil && b.Line >= 0 && b.Line < n {
			in = append(in, appBlock{Block: b})
		}
	}
	v.blockScratch = in
	if !sort.IsSorted(&v.blockScratch) {
		sort.Stable(&v.blockScratch)
	}
	old := v.blocks
	i := 0
	for j := range in {
		b := &in[j]
		for i < len(old) && compareBlocks(&old[i], b) < 0 {
			i++
		}
		if i < len(old) && compareBlocks(&old[i], b) == 0 {
			// sync already settled this line's start after any edits.
			b.at, b.id, b.height, b.measured = old[i].at, old[i].id, old[i].height, old[i].measured
			i++
			continue
		}
		v.nextBlock++
		b.id, b.at = v.nextBlock, v.text.LineStart(b.Line)
	}
	// UI callbacks and options always update, but only effective row
	// changes invalidate geometry. Duplicate identities match in order.
	same := len(in) == len(old)
	if same {
		for j := range in {
			a, b := &in[j], &old[j]
			if a.id != b.id || compareBlocks(a, b) != 0 || a.rowHeight(v.lineHeight) != b.rowHeight(v.lineHeight) {
				same = false
				break
			}
		}
	}
	if !same {
		v.blocksStale = true
	}
	// The slice given last is the scratch of the next, its functions let go.
	clear(old)
	v.blocks, v.blockScratch = in, old[:0]
}

// moveBlocks moves the blocks through changes of the text, by where
// their lines start: settleBlocks finds their lines once the text is the
// last of the changes'.
func (v *view) moveBlocks(cs []change) {
	for i := range v.blocks {
		v.blocks[i].at = mapOffset(v.blocks[i].at, cs, biasRight)
	}
}

// settleBlocks attaches each block to the line at its mapped offset.
// When its line is deleted, it attaches to the surviving line at that
// position. Above stays unchanged.
func (v *view) settleBlocks() {
	if len(v.blocks) == 0 {
		return
	}
	for i := range v.blocks {
		b := &v.blocks[i]
		line := v.text.LineOf(b.at)
		if line != b.Line {
			v.blocksStale = true
		}
		b.Line = line
		b.at = v.text.LineStart(b.Line)
	}
	// Lines joined bring blocks above and below to one line.
	if !sort.IsSorted(&v.blocks) {
		sort.Stable(&v.blocks)
		v.blocksStale = true
	}
}

// blockRows returns the rows of the app's blocks, for the heights. A
// block stays on a hidden line only when it stands in for a replacement,
// and goes with one a fold hides. The row buffer is reused; heights keep
// their own value snapshot, so in-place updates cannot be missed.
func (ed *editor) blockRows() []block {
	v := &ed.view
	if len(v.blocks) == 0 && len(v.rows) == 0 {
		v.blocksStale = false
		return v.rows
	}
	spans := ed.hiddenSpans()
	if !v.blocksStale && v.rowsHeight == v.lineHeight && v.rowsRepls == v.replVersion && slices.Equal(spans, v.rowsFolded) {
		return v.rows
	}
	v.blocksStale, v.rowsHeight, v.rowsRepls = false, v.lineHeight, v.replVersion
	v.rowsFolded = append(v.rowsFolded[:0], spans...)
	folded := heights{hidden: spans}
	rows := slices.Grow(v.rows[:0], len(v.blocks))[:len(v.blocks)]
	for i := range v.blocks {
		b := &v.blocks[i]
		// The block itself supplies standIn's existence condition.
		_, sticky := v.standInReplacement(b.Line)
		rows[i] = block{line: b.Line, below: !b.Above, height: b.rowHeight(v.lineHeight), kind: blockApp, from: b.id, to: i,
			sticky: sticky && !folded.isHidden(b.Line)}
	}
	v.rows = rows
	return rows
}

// standIn returns the replacement a block shows in place of, on line: one
// showing nothing, from the start of a line through a newline, whose
// lines hold a block not folded away. Carets stop at its start, on the
// block, as at the start of what any replacement shows.
func (ed *editor) standIn(line int) (Replacement, bool) {
	v := &ed.view
	if len(v.blocks) == 0 {
		return Replacement{}, false
	}
	r, ok := v.standInReplacement(line)
	if !ok {
		return Replacement{}, false
	}
	t := &v.text
	first, end := t.LineOf(r.Start), t.LineOf(r.End)
	folded := heights{hidden: ed.hiddenSpans()}
	j := sort.Search(len(v.blocks), func(j int) bool { return v.blocks[j].Line >= first })
	for ; j < len(v.blocks) && v.blocks[j].Line < end; j++ {
		if !folded.isHidden(v.blocks[j].Line) {
			return r, true
		}
	}
	return Replacement{}, false
}

// standInReplacement is the common eligibility condition for a sticky
// block and standIn: an empty replacement hiding whole lines.
func (v *view) standInReplacement(line int) (Replacement, bool) {
	if len(v.repls) == 0 {
		return Replacement{}, false
	}
	t := &v.text
	off := t.LineStart(line)
	i := sort.Search(len(v.repls), func(i int) bool { return v.repls[i].End > off })
	if i >= len(v.repls) || v.repls[i].Start > off {
		return Replacement{}, false
	}
	r := v.repls[i]
	first, end := t.LineOf(r.Start), t.LineOf(r.End)
	if r.shown() != "" || r.Start != t.LineStart(first) || end <= line {
		return Replacement{}, false
	}
	return r, true
}

// standInWithin returns the replacement of the block standing in nearest
// the start of the lines of sp going forward, else nearest their end.
func (ed *editor) standInWithin(sp lineSpan, fwd bool) (Replacement, bool) {
	v := &ed.view
	i := sort.Search(len(v.blocks), func(i int) bool { return v.blocks[i].Line >= sp.from })
	j := sort.Search(len(v.blocks), func(i int) bool { return v.blocks[i].Line > sp.to })
	for k := range j - i {
		n := i + k
		if !fwd {
			n = j - 1 - k
		}
		if r, ok := ed.standIn(v.blocks[n].Line); ok {
			return r, true
		}
	}
	return Replacement{}, false
}

// standInBox returns the top and the bottom of what stands in for r's
// lines, in the content.
func (ed *editor) standInBox(r Replacement) (top, bottom float64) {
	t := &ed.view.text
	h := &ed.view.heights
	return h.top(t.LineOf(r.Start)), h.top(t.LineOf(r.End))
}

// standInCaretRect returns the full caret cell in text-content coordinates.
// Blocks stay fixed horizontally, so scrollX cancels the text's offset
// when consumers place this cell in the viewport. Underlines paint only
// the bottom of the cell; IME anchors and reveal still use its full height.
// Keep the whole cell in the margin, with a pixel before opaque content.
func (ed *editor) standInCaretRect(r Replacement) ui.Rect {
	top, bottom := ed.standInBox(r)
	width := float32(2)
	if ed.o.cursorShape == CursorBlock || ed.o.cursorShape == CursorUnderline {
		width = min(ed.view.charWidth, max(0, ed.textX-ed.gutterW-1))
	}
	return ui.Rect{X: ed.scrollX - 1 - width, Y: float32(top), W: width, H: float32(bottom - top)}
}

// paintStandIns paints what shows of the selections on the blocks
// standing in for lines in view: as selected behind those a selection
// covers, and the caret of those it stands at, tall as the block, in the
// margin before it.
func (ed *editor) paintStandIns(p *ui.Painter, r ui.Rect, f *frame) {
	v := &ed.view
	if len(v.blocks) == 0 || len(v.repls) == 0 {
		return
	}
	ct := ed.theme
	selColor := ct.Selection
	if !ed.focused {
		selColor = ct.InactiveSelection
	}
	reps := v.replsIn(v.text.LineStart(f.first), v.text.LineEnd(f.last))
	for i := range reps {
		rep := reps[i]
		if _, ok := ed.standIn(v.text.LineOf(rep.Start)); !ok {
			continue
		}
		top, bottom := ed.standInBox(rep)
		y := r.Y + float32(top-ed.scrollY)
		h := float32(bottom - top)
		for _, s := range ed.sels {
			if !s.empty() && s.start() <= rep.Start && s.end() >= rep.End {
				p.Fill(ui.Rect{X: r.X + ed.gutterW, Y: y, W: r.W - ed.gutterW, H: h}, selColor, 0)
			}
			if f.caretsOn && s.head == rep.Start {
				cr := ed.standInCaretRect(rep)
				cr.X += r.X + ed.textX - ed.scrollX
				cr.Y = r.Y + float32(float64(cr.Y)-ed.scrollY)
				ed.paintCaret(p, cr)
			}
		}
	}
}

// placeBlocks appends to dst the blocks of the lines from y0 to y1 of the
// content, with where they are in the view.
func (ed *editor) placeBlocks(y0, y1 float64, dst []placed) []placed {
	h := &ed.view.heights
	if len(ed.view.blocks) == 0 || h.n == 0 {
		return dst
	}
	ed.eachBlock(h.lineAt(y0), min(h.lineAt(y1), h.n-1), func(b *block, top float64) {
		if b.kind == blockApp {
			dst = append(dst, placed{blockIndex: b.to, block: true, rect: ui.Rect{Y: float32(top - ed.scrollY), W: ed.textWidth(), H: b.height}})
		}
	})
	return dst
}

// buildBlock builds the element of the block p placed, which its layout
// measures.
func (ed *editor) buildBlock(c *ui.Context, p placed) {
	v := &ed.view
	if p.blockIndex >= len(v.blocks) {
		return
	}
	b := &v.blocks[p.blockIndex]
	e := ui.Box(c).Key("block " + strconv.Itoa(b.id)).Absolute().
		Left(ed.textX - ed.gutterW).Top(p.rect.Y).
		Width(p.rect.W)
	ed.describeElement(e, b.Label, b.Interactive)
	built := builtBlock{p.blockIndex, b.id, e}
	// What the last frame laid out, for a frame that paints none, as of
	// an editor as high as its content out of view.
	ed.measureBlock(built)
	ed.built = append(ed.built, built)
	b.Content(c, e, b.Block)
}

// measureBlock has a block as high as its element was laid out, in the
// frame the element belongs to.
func (ed *editor) measureBlock(built builtBlock) {
	v := &ed.view
	if built.i >= len(v.blocks) || v.blocks[built.i].id != built.id || !built.e.Valid() {
		return
	}
	r := built.e.LayoutBounds()
	if r.W <= 0 {
		return
	}
	b := &v.blocks[built.i]
	if b.rowHeight(v.lineHeight) != r.H {
		v.blocksStale = true
	}
	b.height, b.measured = r.H, true
}

// measureBlocks has the blocks built as high as this frame laid them
// out, before their lines are placed around them.
func (ed *editor) measureBlocks() {
	for _, b := range ed.built {
		ed.measureBlock(b)
	}
}
