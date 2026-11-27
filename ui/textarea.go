package ui

import (
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/internal/scene"
	"github.com/egoist/mygo/internal/text"
)

// area lays out and shows the text of a TextArea a paragraph at a time, as
// a List does its rows: each paragraph keeps its layout until an edit
// changes it, only those in view are laid out, and the heights of the
// others are estimated (heights) until they are. Text laid out whole, as
// text elements and single-line inputs are, makes the same lines, since
// the text system breaks lines a paragraph at a time too.
//
// The area scrolls as a scroll container does (flagScrollY): the wheel and
// the scroll bar move the state's offset, which the area keeps by a
// paragraph and how far into it the view starts (anchor, anchorOff), so
// that the view stays put as the heights above it are measured.
//
// An area that keeps its lines whole (NoWrap) scrolls sideways too, as a
// single-line input does, by the editor's scrollX: the x in the layouts at
// the left of the view, 0 where they start. Lines wider than the view go
// past its right, or its left where they align to the right, as far as
// the paragraphs measured so far reach (ext).
type area struct {
	hs heights
	// params are those of the layouts, without their text; version is
	// the buffer's the heights are of, and line a line of an empty text.
	params  text.Params
	version uint64
	line    text.Line

	scroll     float64 // the content's y at the top of the view
	anchor     int
	anchorOff  float64
	lastScroll float64 // the state's offset after the last layout
	// reveal scrolls the caret into view at the next layout, after an
	// edit, a move of the caret or a press.
	reveal bool
	// first and last are the paragraphs the view shows; laid counts those
	// with a layout.
	first, last int
	laid        int
	// wrapped is the height of the whole text wrapped at a width, for a text
	// area that grows with it (Lines), and the params it is of.
	wrapped        float32
	wrappedParams  text.Params
	wrappedVersion uint64
	wrappedCompose string

	// noWrap is the layouts': the lines break only at newlines. ext holds
	// how far left and right of the view's edges at 0 the lines of each
	// paragraph laid out reach, lo and hi the farthest of them while
	// extOK, as a scroll bar knows the heights measured.
	noWrap bool
	ext    [][2]float32
	lo, hi float32
	extOK  bool
	// lastScrollX is the state's horizontal offset after the last
	// layout, which counted it from xFrom, xMax DIPs from the far end
	// right to left.
	lastScrollX float64
	xFrom, xMax float32
	rtlScrollX  bool
}

// wrappedHeight returns the height of the text, with an input method's
// composition, wrapped at the content width cw.
func (a *area) wrappedHeight(e *node, ed *editor, cw float32) float32 {
	params := e.textParams(max(caretRoom(cw), 1))
	if ed.buf.indexed {
		params.KeepSpaces, params.MaxLines = true, 0
		if params != a.wrappedParams || a.wrappedVersion != ed.buf.version || a.wrappedCompose != ed.compose {
			empty := params
			empty.Text = ""
			limit := float32(ed.lines[1]) * ed.sys.Layout(empty).Lines[0].Height
			var height float32
			for p := 0; p < len(ed.buf.paras) && height < limit; p++ {
				part := params
				part.Text = ed.buf.text(p)
				if ed.compose != "" && p == ed.buf.para(ed.caret) {
					at := runeOffset(part.Text, ed.caret-ed.buf.start(p))
					part.Text = part.Text[:at] + ed.compose + part.Text[at:]
				}
				part.Spans = ed.rangeKey(ed.buf.start(p), ed.buf.end(p))
				height += ed.sys.Shape(part).Height
			}
			a.wrapped, a.wrappedParams, a.wrappedVersion = height, params, ed.buf.version
			a.wrappedCompose = ed.compose
		}
		return a.wrapped
	}
	params.Text = ed.displayText()
	params.KeepSpaces, params.MaxLines = true, 0
	// Bold runs take more room, so the text may wrap sooner.
	params.Spans = ed.rangeKey(0, ed.buf.n)
	if params != a.wrappedParams {
		a.wrappedParams = params
		a.wrapped = ed.sys.Layout(params).Height
	}
	return a.wrapped
}

// caretRoom returns the width the paragraphs of a text area are laid out
// in, in a content box cw wide: its last column stays for the caret, as in
// a single-line input, since a caret on the box's right edge, where a line
// laid out right to left starts, would be clipped away.
func caretRoom(cw float32) float32 {
	if cw > 1 {
		return cw - 1
	}
	return cw
}

// maxLaid is how many paragraphs keep their layouts out of view, beyond
// those in view: those further from it give theirs up, keeping their
// heights.
const maxLaid = 512

// edited follows an edit of the buffer: the paragraphs from first, which
// were old, are now after others, without a layout.
func (a *area) edited(b *buffer, first int, old []paragraph, after int) {
	for _, o := range old {
		if o.layout != nil {
			a.laid--
		}
	}
	if a.version+1 == b.version {
		a.version = b.version
		a.editedExt(first, len(old), after, len(b.paras))
		if len(old) == after {
			for k, o := range old {
				if o.h > 0 {
					a.hs.add(first+k, -float64(o.h), 1)
				}
			}
		} else {
			a.hs.reset(b.paras)
		}
	}
	switch {
	case a.anchor >= first+len(old):
		a.anchor += after - len(old)
	case a.anchor > first:
		a.anchor, a.anchorOff = first, 0
	}
	a.reveal = true
}

// sync gets the heights in step with the buffer and the layouts' params,
// which a new width or style changes.
func (a *area) sync(ed *editor, b *buffer, params text.Params) {
	if params != a.params {
		for i := range b.paras {
			b.paras[i].layout, b.paras[i].h = nil, 0
		}
		a.laid = 0
		a.params = params
		a.noWrap = params.NoWrap
		a.version = 0
		a.ext = a.ext[:0]
		empty := params
		empty.Text = ""
		a.line = ed.sys.Layout(empty).Lines[0]
	}
	if a.version != b.version || len(a.hs.measured) != len(b.paras)+1 {
		if a.version != b.version {
			a.laid = 0 // a new text: its paragraphs have no layouts
			a.ext = a.ext[:0]
		}
		a.hs.reset(b.paras)
		a.version = b.version
		a.anchor = min(a.anchor, len(b.paras)-1)
	}
	if a.noWrap && len(a.ext) != len(b.paras) {
		// Measured anew as they are laid out.
		a.ext = slices.Grow(a.ext[:0], len(b.paras))[:len(b.paras)]
		clear(a.ext)
		for i := range b.paras {
			if l := b.paras[i].layout; l != nil {
				a.ext[i] = lineExtent(l)
			}
		}
		a.extOK = false
	}
	m, unknown := a.hs.totals()
	if n := len(b.paras) - unknown; n > 0 {
		a.hs.est = m / float64(n)
	} else {
		a.hs.est = float64(a.line.Height)
	}
}

// paraLayout returns the layout of paragraph p, with the composition of an
// input method in the paragraph of the caret.
func (a *area) paraLayout(ed *editor, p int) *text.Layout {
	b := &ed.buf
	pr := &b.paras[p]
	compose := ""
	if ed.compose != "" && p == b.para(ed.caret) {
		compose = ed.compose
	}
	spans := ed.rangeKey(b.start(p), b.end(p))
	if pr.layout != nil && pr.compose == compose && pr.spans == spans {
		return pr.layout
	}
	t := b.text(p)
	if compose != "" {
		at := runeOffset(t, ed.caret-b.start(p))
		t = t[:at] + compose + t[at:]
	}
	params := a.params
	// A paragraph's layout outlives edits elsewhere. Its text must not
	// pin the old document when the buffer replaces it with a new string.
	if !b.indexed && len(t) < b.byteLen() {
		t = strings.Clone(t)
	}
	params.Text, params.Spans = t, spans
	l := ed.sys.Shape(params)
	if pr.layout == nil {
		a.laid++
	}
	if pr.h > 0 {
		a.hs.add(p, float64(l.Height-pr.h), 0)
	} else {
		a.hs.add(p, float64(l.Height), -1)
	}
	pr.layout, pr.compose, pr.spans, pr.h = l, compose, spans, l.Height
	if a.noWrap && p < len(a.ext) {
		a.measureX(p, l)
	}
	return l
}

// lineExtent returns how far left of 0 and right of it the lines of l go.
func lineExtent(l *text.Layout) [2]float32 {
	var x [2]float32
	for i := range l.Lines {
		line := &l.Lines[i]
		x[0], x[1] = min(x[0], line.X), max(x[1], line.X+line.Width)
	}
	return x
}

// measureX keeps how far the lines of paragraph p, laid out as l, reach.
func (a *area) measureX(p int, l *text.Layout) {
	x, was := lineExtent(l), a.ext[p]
	a.ext[p] = x
	if !a.extOK {
		return
	}
	if was[0] == a.lo && x[0] > was[0] || was[1] == a.hi && x[1] < was[1] {
		a.extOK = false // it was the widest, and is no longer
		return
	}
	a.lo, a.hi = min(a.lo, x[0]), max(a.hi, x[1])
}

// editedExt follows an edit for the extents of the paragraphs: n paragraphs
// from first, now after others, unmeasured, of total.
func (a *area) editedExt(first, n, after, total int) {
	if !a.noWrap {
		return
	}
	if len(a.ext) != total-after+n {
		a.ext = a.ext[:0] // sync measures them anew
		return
	}
	for _, x := range a.ext[first : first+n] {
		if x[0] < 0 && x[0] == a.lo || x[1] > 0 && x[1] == a.hi {
			a.extOK = false
		}
	}
	// The after paragraphs in place of the n, unmeasured, the rest moved
	// along (by hand rather than slices.Replace, which grows every app).
	old := len(a.ext)
	end := old - n + after
	if end > old {
		a.ext = slices.Grow(a.ext, end-old)[:end]
	}
	copy(a.ext[first+after:end], a.ext[first+n:old])
	clear(a.ext[first : first+after])
	a.ext = a.ext[:end]
}

// extents returns how far left and right of the view's edges at 0 the
// lines of the paragraphs measured reach.
func (a *area) extents() (lo, hi float32) {
	if !a.extOK {
		a.lo, a.hi = 0, 0
		for _, x := range a.ext {
			a.lo, a.hi = min(a.lo, x[0]), max(a.hi, x[1])
		}
		a.extOK = true
	}
	return a.lo, a.hi
}

// local returns where rune i of paragraph p is in its layout, which holds
// the composition after the caret.
func (a *area) local(ed *editor, p, i int) int {
	l := i - ed.buf.start(p)
	if ed.compose != "" && i > ed.caret && p == ed.buf.para(ed.caret) {
		l += utf8.RuneCountInString(ed.compose)
	}
	return l
}

// global returns the rune of the text at index l of the layout of
// paragraph p.
func (a *area) global(ed *editor, p, l int) int {
	start := ed.buf.start(p)
	if ed.compose != "" && p == ed.buf.para(ed.caret) {
		n, c := utf8.RuneCountInString(ed.compose), ed.caret-start
		switch {
		case l > c+n:
			l -= n
		case l > c:
			l = c
		}
	}
	return min(start+l, ed.buf.end(p))
}

// caretAt returns the caret before rune i, extra runes into a
// composition there: its x, top and height in the content.
func (a *area) caretAt(ed *editor, i, extra int) (x float32, y float64, h float32) {
	p := ed.buf.para(i)
	l := a.paraLayout(ed, p)
	affinity := text.Downstream
	if i == ed.caret {
		affinity = ed.caretAffinity
	}
	x, ly, h := l.CaretAt(text.CaretPosition{Index: a.local(ed, p, i) + extra, Affinity: affinity})
	return x, a.hs.top(p) + float64(ly), h
}

// lineEdges returns the edges of the visual line holding rune i, the one
// the caret is drawn on when i is the caret.
func (a *area) lineEdges(ed *editor, i int) (int, int) {
	p := ed.buf.para(i)
	l := a.paraLayout(ed, p)
	affinity := text.Downstream
	if i == ed.caret {
		affinity = ed.caretAffinity
	}
	line := &l.Lines[l.CaretLine(text.CaretPosition{Index: a.local(ed, p, i), Affinity: affinity})]
	return a.global(ed, p, line.Start), a.global(ed, p, line.End)
}

// firstLine returns the first line of the text, or a line of an empty text
// while the first paragraph has no layout.
func (a *area) firstLine(b *buffer) *text.Line {
	if l := b.paras[0].layout; l != nil {
		return &l.Lines[0]
	}
	return &a.line
}

// indexAt returns the rune of the caret position closest to (x, y) in the
// content.
func (a *area) indexAt(ed *editor, x float32, y float64) int {
	return a.positionAt(ed, x, y).Index
}

func (a *area) positionAt(ed *editor, x float32, y float64) text.CaretPosition {
	return a.hitPosition(ed, x, y, false)
}

func (a *area) documentPositionAt(ed *editor, x float32, y float64) text.CaretPosition {
	return a.hitPosition(ed, x, y, true)
}

func (a *area) hitPosition(ed *editor, x float32, y float64, virtual bool) text.CaretPosition {
	if a.version != ed.buf.version {
		// The app replaced the text since the layout, and a drag that
		// goes on looks for the pointer in the new one before it is
		// laid out: the heights are still the old paragraphs'.
		a.sync(ed, &ed.buf, a.params)
	}
	n := len(ed.buf.paras)
	y = max(y, 0)
	p := a.hs.at(y)
	// Laying the paragraph out may show it is not as high as estimated.
	for range n {
		l := a.paraLayout(ed, p)
		top := a.hs.top(p)
		switch {
		case y < top && p > 0:
			p--
		case y >= top+float64(l.Height) && p < n-1:
			p++
		default:
			position := l.PositionAt(x, float32(y-top))
			if virtual {
				position.Index += ed.buf.start(p)
				if ed.compose != "" && p > ed.buf.para(ed.caret) {
					position.Index += utf8.RuneCountInString(ed.compose)
				}
			} else {
				position.Index = a.global(ed, p, position.Index)
			}
			return position
		}
	}
	return text.CaretPosition{Index: ed.buf.n, Affinity: text.Upstream}
}

// setScroll scrolls the content to y, keeping the place by the paragraph
// there.
func (a *area) setScroll(y float64) {
	a.scroll = y
	a.anchor = a.hs.at(y)
	a.anchorOff = y - a.hs.top(a.anchor)
}

// layout lays the text area out in a content box of cw×ch: the
// paragraphs in view, the caret's when it is to be revealed, and where the
// view is.
func (a *area) layout(e *node, cw, ch float32) {
	ed := e.st.editor
	b := &ed.buf
	params := e.textParams(caretRoom(cw))
	params.KeepSpaces, params.MaxLines = true, 0
	a.sync(ed, b, params)

	st := e.st
	if st.scrollY != a.lastScroll {
		a.setScroll(st.scrollY) // the wheel or the scroll bar
	}
	reveal := a.reveal
	// The text shows through the padding, inside the border.
	above, below := e.pad[0], e.pad[2]
	a.place(ed, ch, above, below)
	if a.reveal {
		a.reveal = false
		_, y, h := a.caretAt(ed, ed.caret, ed.composeCaret)
		switch {
		case y < a.scroll:
			a.setScroll(y)
		case y+float64(h) > a.scroll+float64(ch):
			// Measure the paragraphs above the caret's that the view will
			// show, so that the caret ends at its bottom whatever their
			// heights were estimated.
			p := b.para(ed.caret)
			for q, room := p-1, y+float64(h)-a.hs.top(p); q >= 0 && room < float64(ch); q-- {
				room += float64(a.paraLayout(ed, q).Height)
			}
			_, y, h = a.caretAt(ed, ed.caret, ed.composeCaret)
			a.setScroll(y + float64(h) - float64(ch))
		}
		a.place(ed, ch, above, below)
	}
	total := a.hs.top(len(b.paras))
	if s := max(0, min(a.scroll, total-float64(ch))); s != a.scroll {
		a.setScroll(s)
		a.place(ed, ch, above, below)
		total = a.hs.top(len(b.paras))
	}
	scrollX, contentW := 0.0, float64(e.w)
	if a.noWrap {
		scrollX, contentW = a.layoutX(e, ed, cw, reveal)
	} else {
		ed.scrollX, a.lastScrollX = 0, 0
	}
	st.scrollTo(scrollX, a.scroll)
	a.lastScroll = a.scroll
	e.contentW, e.contentH = contentW, total+float64(e.padY())
	if a.laid > maxLaid+a.last-a.first {
		a.forget(b)
	}
}

// layoutX keeps the view of a text area whose lines are whole on the
// caret, sideways, as layout does down, when it is to be revealed, and
// returns the state's horizontal offset and the content's width. The
// state counts the offset from the content's left, or right to left from
// its right, as a scroll container's.
func (a *area) layoutX(e *node, ed *editor, cw float32, reveal bool) (float64, float64) {
	st := e.st
	if st.scrollX != a.lastScrollX {
		// The wheel, the scroll bar or ScrollTo, by the last layout's
		// content.
		x := float32(st.scrollX)
		if a.rtlScrollX {
			x = a.xMax - x
		}
		ed.scrollX = a.xFrom + x
	}
	if reveal {
		// As a single-line input, the caret's column stays in view at
		// the right.
		switch x, _, _ := a.caretAt(ed, ed.caret, ed.composeCaret); {
		case x < ed.scrollX:
			ed.scrollX = x
		case x > ed.scrollX+cw-1:
			ed.scrollX = x - cw + 1
		}
	}
	lo, hi := a.extents()
	// The view's edges, and the caret's room after a line longer than it.
	right := cw
	if hi > cw {
		right = hi + 1
	}
	ed.scrollX = max(lo, min(ed.scrollX, right-cw))
	a.xFrom, a.xMax, a.rtlScrollX = lo, right-lo-cw, e.rtl()
	x := ed.scrollX - lo
	if a.rtlScrollX {
		x = a.xMax - x
	}
	a.lastScrollX = float64(x)
	return a.lastScrollX, float64(right - lo + e.padX())
}

// place lays out the paragraphs in view, from the anchor down, with those
// showing in the bands above and below the view, and keeps the view where
// the anchor is.
func (a *area) place(ed *editor, ch, above, below float32) {
	n := len(ed.buf.paras)
	a.anchor = max(0, min(a.anchor, n-1))
	a.paraLayout(ed, a.anchor)
	a.first = a.anchor
	for a.first > 0 && a.hs.top(a.anchor)+a.anchorOff-a.hs.top(a.first) < float64(above) {
		a.first--
		a.paraLayout(ed, a.first)
	}
	// The heights from the first paragraph shown are known now: the place
	// is by the paragraph the offset falls in.
	a.setScroll(max(0, a.hs.top(a.anchor)+a.anchorOff))
	y := a.hs.top(a.first)
	p := a.first
	for ; p < n; p++ {
		y += float64(a.paraLayout(ed, p).Height)
		if y >= a.scroll+float64(ch+below) {
			break
		}
	}
	a.last = min(p, n-1)
}

// forget drops the layouts of the paragraphs far from view, keeping their
// heights.
func (a *area) forget(b *buffer) {
	lo, hi := a.first-maxLaid/4, a.last+maxLaid/4
	for i := range b.paras {
		if (i < lo || i > hi) && b.paras[i].layout != nil {
			b.paras[i].layout = nil
			a.laid--
		}
	}
}

// paint paints the paragraphs in view, the selection, the composition and
// the caret, with the content's top at oy.
func (a *area) paint(e *node, p *Painter, ox, oy float32) {
	ed := e.st.editor
	b := &ed.buf
	t := p.Theme()
	ts := e.resolvedText()
	focused := e.Focused()
	last := len(b.paras) - 1
	for i := a.first; i <= a.last; i++ {
		l := a.paraLayout(ed, i)
		y := oy + float32(a.hs.top(i)-a.scroll)
		start, end := b.start(i), b.end(i)
		_, sp := ed.rangeSpans(start, end)
		p.textBackgrounds(l, ox, y, ts, sp)
		if focused {
			for _, selected := range ed.selectedRanges() {
				sa, sz := selected.Start, selected.End
				if sa == sz || sa > end || sz < start || sz == start && i > 0 && sa < start {
					continue
				}
				from, to := a.local(ed, i, max(sa, start)), a.local(ed, i, min(sz, end))
				for _, r := range l.SelectionVisual(from, to, sz > end && i < last) {
					p.Fill(Rect{ox + r.X, y + r.Y, r.W, r.H}, ts.selectionColor(t), 0)
				}
			}
		}
		p.textForeground(l, ox, y, ts.color, ts, sp)
		if ed.compose != "" && b.para(ed.caret) == i {
			c := ed.caret - start
			for _, r := range l.Selection(c, c+utf8.RuneCountInString(ed.compose)) {
				p.Fill(Rect{ox + r.X, y + r.Y + r.H - 2, r.W, 1}, ts.color, 0)
			}
		}
	}
	if focused && !ed.readOnly {
		rt := e.c.rt
		phase := e.c.now.Sub(rt.blinkStart)
		const blink = 530 * time.Millisecond
		if (phase/blink)%2 == 0 && ed.caretWidth > 0 {
			x, y, h := a.caretAt(ed, ed.caret, ed.composeCaret)
			caret := ts.caretColor(t)
			p.add(scene.Op{Kind: scene.OpFill, Rect: p.snap(Rect{ox + x, oy + float32(y-a.scroll), 0, h}), Color: caret.scene(), Wide: p.wide(caret, Color{}, Color{}), Opacity: p.opacity})
			op := &p.s.Ops[len(p.s.Ops)-1]
			op.Rect.W = max(round(ed.caretWidth*p.scale), 1)
		}
		if phase < 30*time.Second {
			e.c.After(blink - phase%blink)
		}
	}
}
