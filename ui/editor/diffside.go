package editor

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
)

// diffSide shows a side of a diff in an editor.
type diffSide struct {
	d       *DiffState
	old     bool
	unified bool
	hide    []lineSpan
	blks    []block
	// hideV counts how often the side's hidden lines changed (setSide):
	// what the diff's find keys its matches on, for the shown lines only.
	hideV int
}

func (s *diffSide) hidden() []lineSpan { return s.hide }

func (s *diffSide) blocks() []block { return s.blks }

// hunkOf returns the hunk holding line of the side, or -1.
func (s *diffSide) hunkOf(line int) int {
	hs := s.d.model.hunks
	if s.old {
		i := sort.Search(len(hs), func(i int) bool { return hs[i].A1 > line })
		if i < len(hs) && hs[i].A0 <= line {
			return i
		}
		return -1
	}
	i := sort.Search(len(hs), func(i int) bool { return hs[i].B1 > line })
	if i < len(hs) && hs[i].B0 <= line {
		return i
	}
	return -1
}

// empty reports whether the side's text is empty, as of a file added or
// deleted: the hunks count no lines of it, as git does, though its
// editor keeps one, which is not numbered, selected or commented on.
func (s *diffSide) empty() bool {
	if s.old {
		return s.d.eds[0].view.text.Len() == 0
	}
	return s.d.eds[1].view.text.Len() == 0
}

func (s *diffSide) colors(ed *editor) diffColors { return ed.theme.diff(isDark(ed.theme.Background)) }

func (s *diffSide) lineStyle(line int) lineLook {
	d := s.d
	var st lineLook
	ed := &d.eds[1]
	if s.old {
		ed = &d.eds[0]
	}
	if s.hunkOf(line) >= 0 {
		dc := s.colors(ed)
		if s.old {
			st.bg, st.gutter, st.number = dc.deletedLine, dc.deletedGutter, dc.deleted
			st.bar, st.stripes, st.sign = dc.deleted, true, "-"
		} else {
			st.bg, st.gutter, st.number = dc.addedLine, dc.addedGutter, dc.added
			st.bar, st.sign = dc.added, "+"
		}
		if d.NoBackground {
			st.bg, st.gutter = ui.Color{}, ui.Color{}
		}
		if d.Indicators != DiffBars {
			st.bar = ui.Color{}
		}
	}
	side := DiffNew
	if s.old {
		side = DiffOld
	}
	return s.selectedLook(ed, side, line, st)
}

// selectedLook returns st tinted as a line selected by its number
// (LineSelection) when the line of side is, or else st: a line of a
// side's text, or a deleted line of a unified view.
func (s *diffSide) selectedLook(ed *editor, side DiffSide, line int, st lineLook) lineLook {
	d := s.d
	if !d.hasSel || d.selected.Side != side || line < d.selected.Start || line > d.selected.End {
		return st
	}
	m := ed.theme.Modified
	bg := st.bg
	if bg.A == 0 {
		bg = ed.theme.Background
	}
	st.bg = mixLab(bg, m, 0.18)
	g := st.gutter
	if g.A == 0 {
		g = ed.theme.Background
	}
	st.gutter = mixLab(g, m, 0.3)
	st.number = m
	return st
}

func (s *diffSide) words(line int) []diffRange {
	d := s.d
	if d.Inline == DiffInlineNone {
		return nil
	}
	i := s.hunkOf(line)
	if i < 0 {
		return nil
	}
	w := d.model.wordsOf(i, d.Inline)
	if s.old {
		return w.oldWords[line]
	}
	return w.newWords[line]
}

func (s *diffSide) number(line int) int {
	if s.empty() {
		return -1
	}
	if pt := s.d.partial; pt != nil {
		return pt.fileLine(s.old || false, line) + 1
	}
	return line + 1
}

// paintBlock paints a block in r, whose top is at top in the content: a
// filler of rows a side lacks, hatched, or the deleted lines of a unified
// view. Separators leave their room to the elements over it
// (buildSeparators).
func (s *diffSide) paintBlock(p *ui.Painter, ed *editor, b *block, r ui.Rect, top float64) {
	ct := ed.theme
	switch b.kind {
	case blockSpacer:
		if b.from == 0 {
			return
		}
		// A filler: hatched at -45°, every 8 DIPs from its bottom-left
		// corner, as Pierre's. The corner is in float64: r's float32 is
		// a DIP off, or more, millions of DIPs down a file, which would
		// shake the stripes as the view scrolls.
		text := ui.Rect{X: r.X + ed.gutterW, Y: r.Y, W: r.W - ed.gutterW, H: r.H}
		corner := [2]float64{float64(text.X), float64(ed.r.Y) + top + float64(b.height) - ed.scrollY}
		c := ct.Text.Alpha(0.07)
		p.Clip(text, 0, func() {
			fillerStripes(p.Visible(), corner, func(from, to [2]float32) {
				p.StrokePolyline([][2]float32{from, to}, 1, c)
			})
		})
	case blockDeleted:
		s.paintDeleted(p, ed, b, r)
	}
}

// fillerStripes calls fn with the stripes of a filler whose bottom-left
// corner is at corner, from a at v's bottom to b at its top, v being what
// shows of it: lines at -45°, up to the right, every 8 DIPs along its
// bottom from the corner. They are those that cross v alone, and their
// pieces from v's bottom to its top, all of a size so that they share a
// mask without a GPU: a filler across a whole file added is millions of
// DIPs high. Stroke each alone: on a GPU, a translucent line of one
// segment takes no mask. That one would be as wide and as high as v is
// high, rasterized again as its height changes, and drawn that large for
// each stripe.
func fillerStripes(v ui.Rect, corner [2]float64, fn func(a, b [2]float32)) {
	if v.W <= 0 || v.H <= 0 {
		return
	}
	// A stripe is where x+y is the corner's, give or take 8s: s, crossing
	// v when s-y1 < x1 and s-y0 > x0.
	x0, y0 := float64(v.X), float64(v.Y)
	x1, y1 := x0+float64(v.W), y0+float64(v.H)
	phase := corner[0] + corner[1]
	lo, hi := x0+y0, x1+y1
	for s := phase + 8*(math.Floor((lo-phase)/8)+1); s < hi; s += 8 {
		fn([2]float32{float32(s - y1), float32(y1)}, [2]float32{float32(s - y0), float32(y0)})
	}
}

// paintDeleted paints the lines a unified view shows deleted: the old
// text's, tinted, with their numbers in the gutter.
func (s *diffSide) paintDeleted(p *ui.Painter, ed *editor, b *block, r ui.Rect) {
	d := s.d
	ov := &d.oldView
	ov.setTheme(ed.theme)
	dc := s.colors(ed)
	lh := ed.view.lineHeight
	ox := r.X + ed.textX - ed.scrollX
	text := ui.Rect{X: r.X + ed.gutterW, Y: r.Y, W: r.W - ed.gutterW, H: r.H}
	var cc runColors
	var rects []ui.Rect
	words := d.model.wordsOf(b.hunk, d.Inline)
	// The lines in the clip, from the first whose rows or annotations
	// reach into it, by the tops prepare kept: a run as long as a file
	// deleted is not laid out from its first line.
	type placed struct {
		line int
		y, h float32
		l    *lineLayout
	}
	var lines []placed
	tops := d.deletedTops[b.hunk]
	vis := p.Visible()
	y0, y1 := float64(vis.Y-r.Y), float64(vis.Y+vis.H-r.Y)
	first := b.from + max(0, sort.Search(len(tops), func(i int) bool { return tops[i] > y0 })-1)
	last := first
	for last < b.to && tops[last-b.from] <= y1 {
		last++
	}
	ready := ov.advanceSyntax(last, 4*time.Millisecond)
	ov.approx = false
	for line := first; line < last; line++ {
		l := ov.line(line)
		y := r.Y + float32(tops[line-b.from])
		h := float32(len(l.rows)) * lh
		if y+h >= vis.Y {
			lines = append(lines, placed{line, y, h, l})
		}
	}
	for _, pl := range lines {
		look := lineLook{number: dc.deleted}
		if !d.NoBackground {
			look.bg, look.gutter = dc.deletedLine, dc.deletedGutter
		}
		look = s.selectedLook(ed, DiffOld, pl.line, look)
		if look.bg.A != 0 {
			p.Fill(ui.Rect{X: text.X, Y: pl.y, W: text.W, H: pl.h}, look.bg, 0)
		}
		if look.gutter.A != 0 {
			p.Fill(ui.Rect{X: r.X, Y: pl.y, W: ed.gutterW, H: pl.h}, look.gutter, 0)
		}
		if !d.NoLineNumbers {
			n := pl.line + 1
			if pt := d.partial; pt != nil {
				n = pt.fileLine(true, pl.line) + 1
			}
			num := strconv.Itoa(n)
			lay := ed.view.shaped.get(num, ed.view.style)
			nr := rowLayout{pieces: []rowPiece{{to: len(num), l: lay, ascii: true, width: lay.Width}}, to: len(num)}
			var nc runColors
			col := look.number
			ed.view.paintRow(p, &lineLayout{disp: num}, &nr, r.X+ed.gutterW-4-ed.view.charWidth*0.5-lay.Width, pl.y, &nc, func(int, ui.Color) ui.Color { return col })
		}
		if d.Indicators == DiffBars {
			ed.paintBar(p, ui.Rect{X: r.X, Y: pl.y, W: 4, H: pl.h}, lineLook{bar: dc.deleted, stripes: true})
		}
	}
	p.Clip(text, 0, func() {
		for _, pl := range lines {
			l := pl.l
			ov.colorsOf(l, &cc)
			if d.Indicators == DiffClassic {
				s.paintSign(p, ed, "-", r.X+ed.textX-ed.view.charWidth*1.5, pl.y, dc.deleted)
			}
			for ri := range l.rows {
				row := &l.rows[ri]
				ry := pl.y + float32(ri)*lh
				// Only the rows in view: a long line wraps into
				// thousands, each shaped and painted otherwise.
				if ry > p.Visible().Y+p.Visible().H || ry+lh < p.Visible().Y {
					continue
				}
				if d.Inline != DiffInlineNone {
					for _, w := range words.oldWords[pl.line] {
						rects = mergeRects(ov.rangeRects(l, row, l.toDisp(w.start, false), l.toDisp(w.end, false), rects[:0]))
						for _, rc := range rects {
							p.Fill(ui.Rect{X: ox + rc.X, Y: ry, W: rc.W, H: lh}, dc.deletedWord, 3)
						}
					}
				}
				ov.paintRow(p, l, row, ox, ry, &cc, nil)
			}
		}
	})
	// Frames go on while the highlighter works, or the lines show its
	// guesses, as the editor's own lines do.
	if !ready || ov.approx {
		p.AnimationFrame()
	}
}

// paintSign paints a classic + or - before a line.
func (s *diffSide) paintSign(p *ui.Painter, ed *editor, sign string, x, y float32, c ui.Color) {
	v := &ed.view
	lay := v.shaped.get(sign, v.style)
	row := rowLayout{pieces: []rowPiece{{to: 1, l: lay, ascii: true, width: lay.Width}}, to: 1}
	var cc runColors
	v.paintRow(p, &lineLayout{disp: sign}, &row, x, y, &cc, func(int, ui.Color) ui.Color { return c })
}

// blockInput takes presses on the numbers of lines, which select them,
// and on the deleted lines of a unified view. Separators and buttons are
// elements of their own (buildSeparators, buildBlocks).
func (s *diffSide) blockInput(ed *editor, b *block, ev ui.InputEvent, r ui.Rect) bool {
	d := s.d
	side := DiffNew
	if s.old {
		side = DiffOld
	}
	if b != nil {
		switch b.kind {
		case blockDeleted:
			// Lines of the old text: their numbers select them.
			if ev.X < ed.gutterW && d.LineSelection && ev.Kind == ui.InputPointerDown {
				s.pressLine(ed, DiffOld, d.deletedLineAt(b, float64(ev.Y-r.Y)), ev.Mods)
				return true
			}
			return true
		}
		return false
	}
	// A press on the gutter of a line, in r.
	if ev.Kind != ui.InputPointerDown {
		return false
	}
	_, cy := ed.toContent(ev.X, ev.Y)
	line := ed.view.heights.lineAt(cy)
	if d.LineSelection && !s.empty() {
		s.pressLine(ed, side, line, ev.Mods)
		return true
	}
	return false
}

// pressLine selects the line whose number is pressed, or extends the
// selection to it with Shift, and has the drag that follows select the
// lines it goes over (dragLine).
func (s *diffSide) pressLine(ed *editor, side DiffSide, line int, mods ui.Modifiers) {
	d := s.d
	d.selectLine(side, line, mods&ui.Shift != 0)
	if d.hasSel {
		ed.startDrag(func(_ float32, cy float64) { s.dragLine(ed, cy) })
	}
}

// dragLine extends the lines selected to the line at height y of the
// content: a deleted line of a unified view, or a line of the side's
// text. Blocks that hold no line, as annotations, leave them be.
func (s *diffSide) dragLine(ed *editor, y float64) {
	d := s.d
	if b, r, ok := ed.blockAt(y); ok {
		if b.kind == blockDeleted {
			d.dragLine(DiffOld, d.deletedLineAt(b, y-ed.scrollY-float64(r.Y)))
		}
		return
	}
	if s.empty() {
		return
	}
	side := DiffNew
	if s.old {
		side = DiffOld
	}
	d.dragLine(side, ed.view.heights.lineAt(y))
}

// buildBlocks builds what blocks hold of elements in view: annotations,
// the actions of hunks and the gutter's button.
func (s *diffSide) buildBlocks(c *ui.Context, ed *editor) {
	d := s.d
	v := &ed.view
	h := &v.heights
	if ed.r.W <= 0 {
		return
	}
	y0, y1 := ed.buildRange()
	first, last := h.lineAt(y0), h.lineAt(y1)
	side := DiffNew
	if s.old {
		side = DiffOld
	}
	// Annotations under their lines, after one another on a line.
	if d.RenderAnnotation != nil && len(d.Annotations) > 0 {
		stacked := map[[2]int]float64{} // by where they go: 0 a line, 1 a deleted line
		for _, a := range d.Annotations {
			var top float64
			var at [2]int
			line := a.Line
			if s.unified && a.Side == DiffOld {
				if nl, ok := d.model.newLineOf(a.Line); ok {
					line = nl
				} else {
					line = -1
				}
			}
			switch {
			case a.Side == side || s.unified && a.Side == DiffOld && line >= 0:
				if line < first || line > last {
					continue
				}
				g := h.geometry(line)
				if g.hidden {
					continue
				}
				top = g.below
				// After the blocks below it that come first.
				_, below := h.blocksOf(line)
				for _, b := range below {
					if b.kind == blockAnnotation {
						break
					}
					top += float64(b.height)
				}
				at = [2]int{0, line}
			case s.unified && a.Side == DiffOld:
				// In the block of deleted lines it is in.
				found := false
				for line := first; line <= last && !found; line++ {
					above, below := h.blocksOf(line)
					g := h.geometry(line)
					for _, blks := range [][]block{above, below} {
						bt := g.top
						if len(blks) > 0 && blks[0].below {
							bt = g.below
						}
						for _, b := range blks {
							if b.kind == blockDeleted && a.Line >= b.from && a.Line < b.to {
								// Under the line's rows.
								rows := float64(float32(d.oldView.heights.wraps.rows(a.Line)) * v.lineHeight)
								top, found = bt+d.deletedTops[b.hunk][a.Line-b.from]+rows, true
								break
							}
							bt += float64(b.height)
						}
						if found {
							break
						}
					}
				}
				if !found {
					continue
				}
				at = [2]int{1, a.Line}
			default:
				continue
			}
			top += stacked[at]
			n := stacked[at]
			stacked[at] += float64(a.Height)
			a := a
			box := ui.Box(c).Key(fmt.Sprint("annotation ", a.Side, " ", a.Line, " ", a.Key, " ", n)).Absolute().
				Left(ed.gutterW).Top(float32(top - ed.scrollY)).Width(ed.r.W - ed.gutterW).Height(a.Height).HandleInput(shield)
			box.Children(func() { d.RenderAnnotation(c, a) })
		}
	}
	// Revert and Keep on the hunk under the pointer, or the caret's.
	if d.HunkActions && !s.old {
		active := d.activeHunk()
		for i, hk := range d.model.hunks {
			if i != active {
				continue
			}
			line := hk.B0
			if line > last+1 || hk.B1 < first && hk.B0 < first {
				continue
			}
			var top float64
			if s.unified && hk.A1 > hk.A0 {
				if line >= v.text.LineCount() {
					line = v.text.LineCount() - 1
					top = h.geometry(line).below
				} else {
					top = h.geometry(line).top
					above, _ := h.blocksOf(line)
					for _, b := range above {
						if b.kind == blockDeleted {
							break
						}
						top += float64(b.height)
					}
				}
			} else {
				line = min(line, v.text.LineCount()-1)
				top = h.geometry(line).rows
			}
			if top < y0-float64(v.lineHeight) || top > y1 {
				continue
			}
			bar := ui.Row(c).Key(fmt.Sprint("hunk actions ", i)).Absolute().Right(ed.trackWidth() + 6).Top(float32(top-ed.scrollY) + 2).
				Cursor(ui.CursorDefault).KeepFocus().HandleInput(shield)
			render := d.RenderHunkActions
			if render == nil {
				render = DefaultHunkActions
			}
			render(c, bar, &DiffHunkActions{Index: i, Hunk: DiffHunk{hk.A0, hk.A1, hk.B0, hk.B1}, d: d})
		}
	}
	// The gutter's button, on the line under the pointer.
	if d.OnGutterButton != nil && !s.empty() && ed.pointerIn && ed.hoverLine >= 0 && ed.hoverLine < v.text.LineCount() && ed.pointer.X < ed.gutterW+40 {
		g := h.geometry(ed.hoverLine)
		if !g.hidden {
			line := DiffLineRange{Side: side, Start: ed.hoverLine, End: ed.hoverLine}
			ui.Box(c).Key("gutter button").Absolute().Left(2).Top(float32(g.rows-ed.scrollY) + 2).HandleInput(shield).Children(func() {
				btn := ui.ButtonBase(c).Size(16, v.lineHeight-4).Cursor(ui.CursorPointer).KeepFocus()
				render := d.RenderGutterButton
				if render == nil {
					render = DefaultGutterButton
				}
				render(c, btn, line)
				if btn.Clicked() {
					d.OnGutterButton(line)
				}
			})
		}
	}
}
