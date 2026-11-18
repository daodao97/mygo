package ui

import (
	"math"

	"github.com/egoist/mygo/internal/text"
)

// The layout is CSS flexbox, or grid (grid.go), with box-sizing:
// border-box. Sizes are border boxes (padding and border included, margins
// not); inf marks a size not known yet, such as the height of a column
// that grows with its content.

var inf = float32(math.Inf(1))

func finite(v float32) bool { return v < inf }

func (e *node) padX() float32 { return e.pad[1] + e.pad[3] + e.border[1] + e.border[3] }
func (e *node) padY() float32 { return e.pad[0] + e.pad[2] + e.border[0] + e.border[2] }

// contentX and contentY place the content box in the border box.
func (e *node) contentX() float32 { return e.pad[3] + e.border[3] }
func (e *node) contentY() float32 { return e.pad[0] + e.border[0] }

// m returns margin i, with automatic margins as 0.
func (e *node) m(i int) float32 {
	if isAuto(e.margin[i]) {
		return 0
	}
	return e.margin[i]
}

func (e *node) marginX() float32 { return e.m(1) + e.m(3) }
func (e *node) marginY() float32 { return e.m(0) + e.m(2) }

func (e *node) scrolls() bool { return e.flags&(flagScrollX|flagScrollY) != 0 }

// clampW and clampH apply the min and max sizes.
func (e *node) clampW(w, cbW float32) float32 {
	if v, ok := e.maxW.upper(base(cbW)); ok {
		w = min(w, v)
	}
	if v, ok := e.minW.lower(base(cbW)); ok {
		w = max(w, v)
	}
	// No narrower than its padding and border, as in CSS.
	return max(w, e.padX())
}

func (e *node) clampH(h, cbH float32) float32 {
	if v, ok := e.maxH.upper(base(cbH)); ok {
		h = min(h, v)
	}
	if v, ok := e.minH.lower(base(cbH)); ok {
		h = max(h, v)
	}
	return max(h, e.padY())
}

// base turns an unknown size into the -1 length.resolve takes for it.
func base(v float32) float32 {
	if finite(v) {
		return v
	}
	return -1
}

// layoutTree lays the frame out in a window of w×h DIPs and gives every
// element its box relative to the window.
func layoutTree(root *node, w, h float32) {
	rt := root.c.rt
	rt.consumed = false
	rt.beginLayout(root)
	layoutBox(root, w, h)
	// Lists build rows as they lay out, which may ask to come into view.
	for _, r := range root.c.reveal {
		rt.askReveal(r.e.id, r.o)
	}
	if len(rt.revealIDs) > 0 {
		revealAll(root, rt.revealIDs)
		rt.revealIDs = rt.revealIDs[:0]
	}
	if rt.late {
		// Rows built as their lists laid out handled their input as the
		// view's elements do, and the next frame shows what that changed;
		// what they put above the window is laid out with it.
		rt.lateInput()
		if ov := root.c.overlay; ov != nil {
			if ov.parent == nil {
				root.add(ov)
			}
			layoutAbsolute(root)
		}
	}
	rt.animateLayout(root, w, h)
	if ov := root.c.overlay; ov != nil {
		reattach(ov)
	}
	if rt.late {
		// Lists resized by their transitions built rows as they laid out
		// anew.
		rt.lateInput()
	}
	place(root, 0, 0)
	if rt.matches != nil {
		rt.matches.move()
	}
}

// lateInput forgets the input of the rows built as their lists laid out,
// which they handled as the view's elements do: the next frame shows what
// that changed.
func (rt *engine) lateInput() {
	rt.late = false
	rt.forgetInput()
	if rt.consumed {
		rt.animate()
	}
}

// place turns the boxes, laid out relative to their parents, into window
// coordinates, moving the content of scroll containers by their offset.
func place(e *node, x, y float32) {
	e.x += x
	e.y += y
	if e.stuck() {
		// Its scroll containers and parent are placed: it moves with what
		// is inside it to where it sticks.
		dx, dy := e.stickyShift(e.x, e.y, placedOrigin)
		e.x += dx
		e.y += dy
	}
	if e.hidden {
		// Not laid out: nor is what is inside it placed, whose scroll
		// offsets stay as they were for when it shows again.
		return
	}
	cx, cy := e.x, e.y
	if f := e.followX; f != nil {
		cx -= float32(f.followShift())
	}
	if e.scrolls() {
		// The content may no longer reach as far as the offset, as when a
		// page gives way to a shorter one, or the app asked for the end:
		// keep the offset within it.
		e.adoptScroll()
		s := e.st
		mx, my := e.maxScroll()
		if sx, sy := max(0, min(s.scrollX, mx)), max(0, min(s.scrollY, my)); sx != s.scrollX || sy != s.scrollY {
			s.beginMove(e.c.rt.frame)
			s.scrollTo(sx, sy)
		}
		if e.snap != SnapNone || s.snap != nil {
			// Where it rests, if it snaps (ScrollSnap).
			placeSnapper(e)
		}
		if t := e.track; t != nil {
			t.MaxX, t.MaxY = float32(mx), float32(my)
		}
		if s.movedIn(e.c.rt.frame) {
			// What was built read the old offset, as List's rows or the
			// app from its ScrollState: build the next frame with the new.
			e.c.rt.animate()
		}
		cx -= float32(e.scrollLeft()) + s.contentOverX()
		cy -= float32(s.scrollY-e.scrollBase) + s.contentOver()
	}
	for ch := e.first; ch != nil; ch = ch.next {
		// A copy of an element leaving the flow stays where the flow was.
		if ch.flags&flagAbsolute != 0 && ch.leaving != 1 {
			place(ch, e.x, e.y)
		} else {
			place(ch, cx, cy)
		}
	}
}

// insetAny reports whether any of the element's insets is set: they are
// what place it, and their unit tells, as the inspector's does.
func (e *node) insetAny() bool {
	return e.inset[0].u != unitAuto || e.inset[1].u != unitAuto || e.inset[2].u != unitAuto || e.inset[3].u != unitAuto
}

// relative returns how far the insets of an element in flow move it from
// where the layout put it, against a content box cw×ch. Where both Left
// and Right are set, Left wins, or Right in an element laid out right to
// left, as in CSS.
func (e *node) relative(cw, ch float32) (dx, dy float32) {
	if !e.insetSet {
		return 0, 0 // nothing placed it: it is where the layout put it
	}
	if e.sticky && e.flags&flagAbsolute == 0 {
		return 0, 0 // they hold it in its scroll container instead
	}
	left, lok := e.inset[3].resolve(base(cw))
	right, rok := e.inset[1].resolve(base(cw))
	if lok && !(rok && e.rtl()) {
		dx = left
	} else if rok {
		dx = -right
	}
	if v, ok := e.inset[0].resolve(base(ch)); ok {
		dy = v
	} else if v, ok := e.inset[2].resolve(base(ch)); ok {
		dy = -v
	}
	return dx, dy
}

// textParams returns how to lay out the element's text at a content width
// (0 for one line per paragraph).
func (e *node) textParams(width float32) text.Params {
	ts := e.resolvedText()
	style := text.Style{
		Family: ts.family, Size: ts.size, Weight: ts.weight, Italic: ts.italic, LineHeight: ts.lineHeight,
		LetterSpacing: ts.spacing, Features: ts.features, Lang: ts.lang, TabSize: ts.tabSize,
	}
	if ts.fixedLine && ts.lineHeight > 0 {
		style.LineHeight = ts.lineHeight / style.FontSize()
	}
	p := text.Params{Text: e.text, Width: width, MaxLines: e.maxLines, Style: style, Spans: e.textSpans(), NoWrap: e.noWrap, Balance: e.balance, Pretty: e.pretty, KeepSpaces: e.keepSpaces, Ellipsis: e.ellipsis, TruncateMiddle: e.middle, TruncateStart: e.cutStart, RTL: e.rtl()}
	switch {
	case ts.align == Center:
		p.Align = text.Center
	case e.rtl():
		// Lines start at the right of an element laid out right to left,
		// whatever they read, as in CSS, AppKit's cells and GTK's labels.
		// Left to right, a paragraph reading right to left starts at the
		// right, as Apple's guidelines align a paragraph by its language.
		p.Align = text.Right
		if ts.align == End {
			p.Align = text.Left
		}
	case ts.align == End:
		p.Align = text.End
	}
	return p
}

// resolvedText merges the text styles of the element and its ancestors.
func (e *node) resolvedText() textStyle {
	var out textStyle
	for p := e; p != nil && out.set != setAll; p = p.parent {
		t := &p.ts
		take := t.set &^ out.set
		if take == 0 {
			continue
		}
		if take&setFamily != 0 {
			out.family = t.family
		}
		if take&setSize != 0 {
			out.size = t.size
		}
		if take&setWeight != 0 {
			out.weight = t.weight
		}
		if take&setItalic != 0 {
			out.italic = t.italic
		}
		if take&setColor != 0 {
			out.color = t.color
		}
		if take&setLineHeight != 0 {
			out.lineHeight, out.fixedLine = t.lineHeight, t.fixedLine
		}
		if take&setAlign != 0 {
			out.align = t.align
		}
		if take&setUnderline != 0 {
			out.underline, out.wavy, out.dotted, out.solid = t.underline, t.wavy, t.dotted, t.solid
		}
		if take&setSpacing != 0 {
			out.spacing = t.spacing
		}
		if take&setFeatures != 0 {
			out.features = t.features
		}
		if take&setLang != 0 {
			out.lang = t.lang
		}
		if take&setStrike != 0 {
			out.strike = t.strike
		}
		if take&setDecoColor != 0 {
			out.decoColor = t.decoColor
		}
		if take&setDecoThick != 0 {
			out.decoThick = t.decoThick
		}
		if take&setDecoOffset != 0 {
			out.decoOffset = t.decoOffset
		}
		if take&setBackground != 0 {
			out.background = t.background
		}
		if take&setSelection != 0 {
			out.selection = t.selection
		}
		if take&setCaret != 0 {
			out.caret = t.caret
		}
		if take&setStroke != 0 {
			out.strokeWidth, out.strokeColor = t.strokeWidth, t.strokeColor
		}
		if take&setTabSize != 0 {
			out.tabSize = t.tabSize
		}
		if take&setShadow != 0 {
			out.shadow = t.shadow
		}
		out.set |= take
	}
	return out
}

// leafWidths returns the max-content and min-content widths of the
// element's own content (text, image, input), without padding.
func (e *node) leafWidths() (maxW, minW float32) {
	switch e.kind {
	case kindText:
		if e.text == "" {
			return 0, 0
		}
		// Measured once a frame: the layout asks for both widths a few
		// times.
		if !e.leafOK {
			e.leaf, e.leafOK = e.textWidths(), true
		}
		return e.leaf[0], e.leaf[1]
	case kindImage:
		w, _ := e.intrinsicSize()
		return w, 0
	case kindIcon:
		// Icons keep their size where room is short, as text does.
		w, _ := e.intrinsicSize()
		return w, w
	case kindInput:
		return 200, 0
	}
	return 0, 0
}

// intrinsic returns the element's max-content or min-content width, its
// border box.
func intrinsic(e *node, maxContent bool) float32 {
	if e.special {
		if e.form != nil {
			e.form.alignLabels()
		}
		if e.colFit != nil {
			e.colFit.apply()
		}
	}
	if v, ok := e.width.resolve(-1); ok {
		return e.clampW(v, inf)
	}
	return e.clampW(contentWidth(e, maxContent), inf)
}

// contentWidth returns the max-content or min-content width of the
// element's content and padding, before its minimum and maximum widths:
// its flex base size in a row.
func contentWidth(e *node, maxContent bool) float32 {
	if e.special {
		if e.form != nil {
			e.form.alignLabels()
		}
		if e.colFit != nil {
			e.colFit.apply()
		}
	}
	// Measured once a frame: each container measures its children, and
	// measuring them again for every container around them would take
	// as long as the tree is deep.
	i := b2i(!maxContent)
	if e.intrOK&(1<<i) != 0 {
		return e.intr[i]
	}
	w := measureIntrinsic(e, maxContent)
	e.intr[i] = w
	e.intrOK |= 1 << i
	return w
}

// forgetIntrinsic forgets the widths intrinsic measured of the element and
// of those around it, whose content it is, as what layout changes in it
// changes them: the rows a List builds, the labels a Form aligns.
func (e *node) forgetIntrinsic() {
	// What of its children changed, so what a measuring pass kept for the
	// commit pass is not what that pass would build now. The change is
	// counted, and every hold kept before it is stale.
	e.c.rt.flex.changes++
	for p := e; p != nil && p.intrOK != 0; p = p.parent {
		p.intrOK = 0
		p.keepMeasured()
	}
}

// measureIntrinsic measures the max-content or min-content width of an
// element sized by its content.
func measureIntrinsic(e *node, maxContent bool) float32 {
	var w float32
	switch {
	case e.kind != kindBox:
		mx, mn := e.leafWidths()
		w = mn
		if maxContent {
			w = mx
		}
	case e.grid:
		w = gridIntrinsic(e, maxContent)
		if !maxContent && e.scrolls() {
			w = 0
		}
	default:
		n := 0
		// A toolbar's overflow button is chrome, not content: it shows only
		// when controls overflow, so it must not widen the toolbar when
		// everything fits. Skipping it here (rather than relying on its
		// collapsed state) also keeps the measured width stable across
		// frames, as the button collapses and reappears during layout.
		var more *node
		if e.overflow != nil {
			for ch := e.first; ch != nil; ch = ch.next {
				more = ch
			}
		}
		for ch := e.first; ch != nil; ch = ch.next {
			if ch == more || ch.flags&flagAbsolute != 0 || ch.hidden {
				continue
			}
			cw := intrinsic(ch, maxContent)
			if e.row {
				// An item that cannot grow takes no more than its flex
				// base size, and one that cannot shrink no less (CSS
				// flexbox §9.9.1).
				// Neither goes below its minimum.
				if b, ok := ch.basis.resolve(-1); ok {
					b = ch.clampW(b, inf)
					if ch.grow == 0 {
						cw = max(min(cw, b), autoMinWidth(ch))
					}
					if ch.shrink == 0 {
						cw = max(cw, b)
					}
				}
			}
			cw += ch.marginX()
			if e.row && (maxContent || !e.wrap) {
				w += cw
				n++
			} else {
				w = max(w, cw)
			}
		}
		if e.row && n > 1 && (maxContent || !e.wrap) {
			w += e.gapX * float32(n-1)
		}
		if !maxContent && e.scrolls() {
			w = 0
		}
	}
	return w + e.padX()
}

// autoMinWidth returns the least width of an item of a row whose width
// is not known, as flexLayout finds it: its minimum, or its content's
// min-content width unless it scrolls or clips sideways, no wider than
// its width.
func autoMinWidth(c *node) float32 {
	if v, ok := c.minW.lower(-1); ok {
		return max(v, c.padX())
	}
	if c.scrolls() || c.flags&flagClipX != 0 {
		return c.padX()
	}
	w := intrinsic(c, false)
	if v, ok := c.width.resolve(-1); ok {
		w = min(w, v)
	}
	return max(w, c.padX())
}

// fitWidth returns the width of an element sized by its content within
// avail DIPs, against a containing block cbW wide.
func fitWidth(e *node, avail, cbW float32) float32 {
	if v, ok := e.width.resolve(base(cbW)); ok {
		return e.clampW(v, cbW)
	}
	if e.aspect > 0 {
		if h, ok := e.height.resolve(-1); ok {
			return e.clampW(h*e.aspect, cbW)
		}
	}
	mx := intrinsic(e, true)
	if !finite(avail) {
		return e.clampW(mx, cbW)
	}
	mn := intrinsic(e, false)
	return e.clampW(min(mx, max(mn, avail)), cbW)
}

// heightAt returns the height of the element when it is w wide, against
// a containing block cbH high.
func heightAt(e *node, w, cbH float32) float32 {
	return e.clampH(naturalHeight(e, w, cbH), cbH)
}

// naturalHeight returns the height of the element when it is w wide,
// before its minimum and maximum heights: its flex base size in a column.
func naturalHeight(e *node, w, cbH float32) float32 {
	if v, ok := e.height.resolve(base(cbH)); ok {
		return v
	}
	if e.aspect > 0 {
		return w / e.aspect
	}
	// A box with nothing in it is as high as its padding, whichever width
	// it is measured at: contentHeight returns 0 for it, and the width
	// cannot change that. A leaf of a row that wraps is measured here,
	// then given the line's height, so this returns before the measures
	// it would be kept in, whose second read would find the same value.
	// A box that lays out by more than what it is built with (special) may
	// be a list with rows of its own, which contentHeight measures; the
	// flag says so without reading the list, which lies far away.
	if e.kind == kindBox && e.first == nil && !e.special {
		return e.padY()
	}
	for i := 0; i < e.nmeasure; i++ {
		if m := e.measures[i]; m.availW == w {
			return m.h
		}
	}
	h := contentHeight(e, w-e.padX()) + e.padY()
	if e.nmeasure < len(e.measures) {
		e.measures[e.nmeasure] = measure{availW: w, h: h}
		e.nmeasure++
	}
	return h
}

// contentHeight returns the height the content takes in a content box cw
// wide.
func contentHeight(e *node, cw float32) float32 {
	switch e.kind {
	case kindText:
		return e.textLayout(max(cw, 1)).Height
	case kindImage, kindIcon:
		if w, h := e.intrinsicSize(); w > 0 {
			return cw * h / w
		}
		return 0
	case kindInput:
		return e.inputHeight(cw)
	}
	if e.special {
		if f := e.list; f != nil && f.n > 0 {
			// A List is as high as all its rows, as far as the heights known
			// tell, whichever it built.
			s := f.s
			return float32(s.heights.top(f.n, float64(max(e.gapY, 0))) - float64(max(e.gapY, 0)))
		}
	}
	if e.first == nil {
		return 0
	}
	_, h := boxLayout(e, cw, inf, false)
	return h
}

// boxLayout lays out the children of a box, with flexbox or as a grid.
func boxLayout(e *node, cw, ch float32, commit bool) (usedW, usedH float32) {
	if e.special {
		if e.form != nil {
			e.form.alignLabels()
		}
		if e.colFit != nil {
			e.colFit.apply()
		}
	}
	if e.grid {
		return gridLayout(e, cw, ch, commit)
	}
	return flexLayout(e, cw, ch, commit)
}

// layoutBox gives the element its size and lays out its content, as it
// did in the last frame when it can (layoutcache.go).
func layoutBox(e *node, w, h float32) {
	rt := e.c.rt
	rt.laySeq++
	e.laid = rt.laySeq
	if e.lay&layReuse != 0 {
		if r := &e.st.lay; r.sized && r.w == w && r.h == h {
			reuseBox(e, r)
			return
		}
	}
	start := rt.laySeq
	rt.computed++
	computeBox(e, w, h)
	if e.lay&layOwner != 0 {
		keepBox(e, w, h, start)
	}
}

// computeBox gives the element its size and lays out its content.
func computeBox(e *node, w, h float32) {
	e.w, e.h = w, h
	cw, ch := max(w-e.padX(), 0), max(h-e.padY(), 0)
	switch e.kind {
	case kindText:
		// Lists may build paragraphs while laying out, after the view's pass.
		e.prepareSelectable()
		e.tl = e.textLayout(max(cw, 1))
		if ed := e.st.editor; ed != nil && e.flags&flagSelectable != 0 {
			// Selectable text hit-tests and selects in what it shows.
			ed.layout = e.tl
			ed.originX, ed.originY = e.contentX(), e.contentY()
		}
		return
	case kindInput:
		e.layoutInput(cw, ch)
		return
	case kindImage, kindIcon:
		return
	}
	if e.special {
		if e.list != nil {
			e.layoutList(w, h)
			return
		}
	}
	if e.first == nil && !e.scrolls() {
		// A box with nothing in it, most of a tree: its parent gave it
		// its size, and laying out no children would change nothing.
		return
	}
	if e.special && e.overflow != nil {
		e.layoutToolbar(cw)
	}
	lw, lh := cw, ch
	if e.flags&flagScrollX != 0 {
		lw = inf
	}
	if e.flags&flagScrollY != 0 {
		lh = inf
	}
	uw, uh := boxLayout(e, lw, lh, true)
	if e.flags&flagScrollX != 0 && uw < cw && e.rtl() {
		// Laid out from the right of what the content takes, which goes
		// at the right of the container.
		for c := e.first; c != nil; c = c.next {
			if c.flags&flagAbsolute == 0 {
				c.x += cw - uw
			}
		}
	}
	if e.baselines {
		alignBaselines(e)
	}
	if e.scrolls() {
		e.contentW = float64(max(uw, cw) + e.padX())
		e.contentH = float64(max(uh, ch) + e.padY())
	}
	layoutAbsolute(e)
}

// A flexItem is what the first walk over the children resolved of one,
// so that the passes after it read the item, which sits next to it in
// the scratch, and not the element again. It fills exactly 64 bytes, a
// cache line: a field more and the items of deep trees take longer to
// lay out than the walks over their elements they save.
type flexItem struct {
	e                    *node
	base, hyp            float32
	minMain, maxMain     float32
	main, cross          float32
	marginMain, marginCr float32
	// mainStart and crossStart are the margins at the start of the main
	// axis and of the cross one, the automatic ones as zero. The passes
	// after the first walk read them in the item, and not the element
	// again, where the first walk read them.
	mainStart, crossStart float32
	grow, shrink          float32 // what the line's flexible lengths grow and shrink it by
	// baseline is how far from the start of its margin box across the
	// line the baseline of an item aligned by its baseline (byBaseline)
	// is.
	baseline float32
	// flags are what the passes after the first walk know of the item:
	// crossKnown where the first pass found the cross size, which for
	// the others depends on their main size, stretch on those the
	// line's cross size is given to, frozen on those the flexible
	// lengths have resolved, byBase on those aligned by their
	// baselines, and laidOut on those of a row, laid out to find theirs.
	flags uint16
	// auto are the item's automatic margins: the ones at the start and
	// the end of its main axis, and the ones at the start and the end of
	// its cross axis. The first walk reads them with the margins it
	// reads the sizes of, and the passes after it read them here, in the
	// item, and not the element again.
	auto uint8
	// align is the item's own alignment across the line: the one it
	// gives itself, or the line's. The placement reads it in the item,
	// and not the element again.
	align Align
}

// The automatic margins of a flexItem, in bits.
const (
	autoMainStart uint8 = 1 << iota
	autoMainEnd
	autoCrossStart
	autoCrossEnd
)

// The flags of a flexItem, in bits so that the item stays within a
// cache line.
const (
	flagCrossKnown uint16 = 1 << iota
	flagStretch
	flagFrozen
	flagByBase
	flagLaidOut
	// flagUp and flagDown are how the clamping of the flexible lengths
	// moved the item: above its minimum, or below its maximum.
	flagUp
	flagDown
	// flagCrossAbs says the size of the item across the line is
	// absolute: a length in pixels, which is the same at any height, so
	// that a commit pass at another height takes it from the item, and
	// not from the element again.
	flagCrossAbs
	// flagCrossBound says the size of the item across the line is
	// bounded: it has a minimum or a maximum across the line, which the
	// pass that stretches it clamps it by, reading the element again.
	flagCrossBound
)

// has reports whether the item has the flag f.
func (it *flexItem) has(f uint16) bool { return it.flags&f != 0 }

// autoCross reports whether the item has an automatic margin across the
// line, which keeps it from stretching and from lining up its baseline.
func (it *flexItem) autoCross() bool {
	return it.auto&(autoCrossStart|autoCrossEnd) != 0
}

type flexLine struct {
	start, end int
	main       float32 // sum of outer main sizes and gaps
	cross      float32
	pos        float32 // where it starts across the container
	baseline   float32 // of the items aligned by their baselines, below its top
}

// flexScratch holds the items and lines of the flex containers being laid
// out, a nested container's after its parent's, reusing their memory
// frame after frame.
type flexScratch struct {
	items []flexItem
	lines []flexLine
	// holds are what measuring passes resolved (saveHold), which the
	// commit passes at those sizes use instead of building them again
	// (takeHold): holdItems and holdLines hold what the entries point
	// into, so that keeping them costs one copy and no allocation once
	// the pools have grown to a frame's largest tree. dropHolds forgets
	// them at the start of a layout, for their elements' sake.
	holds         []flexHold
	holdItems     []flexItem
	holdLines     []flexLine
	holdSaved     int
	holdUsed      int
	holdRefreshed int
	// changes counts what changed in the tree: a hold keeps the count of
	// when it was measured, and is stale once the count is past it.
	changes uint64
}

// flexHold is what one measuring pass of one element resolved: its items
// and lines, where they sit in the pools, and the size it resolved them
// for. A commit pass at that size takes them instead of building them
// again; the element's hold says which entry is its.
type flexHold struct {
	e              *node
	items, lines   int32 // where they start in holdItems, holdLines
	nitems, nlines int32
	cw, ch         float32
	row            bool
	baselines      bool
	changes        uint64 // what changed in the tree when it was measured
}

// dropHolds forgets the holds of the last layout: their elements are gone,
// or are being built anew.
func (f *flexScratch) dropHolds() {
	clear(f.holdItems)
	clear(f.holdLines)
	f.holds, f.holdItems, f.holdLines = f.holds[:0], f.holdItems[:0], f.holdLines[:0]
}

// saveHold keeps what this measuring pass resolved of the in-flow
// children, for the commit pass at the same size: one copy into capacity
// kept from the last frame. It replaces what an earlier measuring pass of
// the same element kept, so that a frame measures its elements as many
// times as it did before without growing the pools.
func (e *node) saveHold(cw, ch float32, row, baselines bool, items []flexItem, lines []flexLine) {
	f := &e.c.rt.flex
	i := int(e.hold) - 1
	if i < 0 || i >= len(f.holds) || f.holds[i].e != e {
		f.holds = append(f.holds, flexHold{e: e})
		i, e.hold = len(f.holds)-1, int32(len(f.holds))
	}
	h := &f.holds[i]
	h.cw, h.ch, h.row, h.baselines = cw, ch, row, baselines
	h.changes = f.changes
	h.items, h.nitems = keepHold(&f.holdItems, items, h.items, h.nitems)
	h.lines, h.nlines = keepHold(&f.holdLines, lines, h.lines, h.nlines)
	f.holdSaved++
}

// keepHold copies v into pool, at start where an entry of ours as large as
// it already sits, or after what the pool holds; it returns where it went
// and how many. Slices of the pool another pass holds stay valid: a copy
// that grows the pool leaves them where they were.
func keepHold[T any](pool *[]T, v []T, start, n int32) (int32, int32) {
	if int(n) >= len(v) && int(start)+len(v) <= len(*pool) {
		// In its own space again, as the next frame's will be.
		copy((*pool)[start:start+int32(len(v))], v)
		return start, int32(len(v))
	}
	was := len(*pool)
	*pool = append(*pool, v...)
	return int32(was), int32(len(v))
}

// takeHold returns what the measuring pass of this element resolved for
// the size it is being committed at, and false when it has none: another
// size, another pass, what changed in the tree since, or what it kept is
// gone. A row's items and lines depend on the width alone, so a row
// committed at another height takes what its measuring pass kept at the
// width and crosses it again; a column's main axis is its height, which
// must be the height measured.
func (e *node) takeHold(cw, ch float32, row bool) (items []flexItem, lines []flexLine, baselines, sameCh, ok bool) {
	f := &e.c.rt.flex
	i := int(e.hold) - 1
	if i < 0 || i >= len(f.holds) {
		return nil, nil, false, false, false
	}
	h := &f.holds[i]
	if h.e != e || h.row != row || h.cw != cw || !row && h.ch != ch || h.changes != f.changes {
		return nil, nil, false, false, false
	}
	it, ln := h.items, h.lines
	if it+h.nitems > int32(len(f.holdItems)) || ln+h.nlines > int32(len(f.holdLines)) {
		return nil, nil, false, false, false // the pool was dropped under us
	}
	f.holdUsed++
	return f.holdItems[it : it+h.nitems], f.holdLines[ln : ln+h.nlines], h.baselines, h.ch == ch, true
}

// The margins of an item on the sides of its container's main and cross
// axes, by index into Element.margin: start (left or top) and end.
var (
	mainMargins  = [2][2]int{{0, 2}, {3, 1}} // [row] → start, end
	crossMargins = [2][2]int{{3, 1}, {0, 2}}
)

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// noHold turns off keeping what a measuring pass resolved, so that a test
// can lay a tree out with it and without it in one process
// (TestHoldKeepsLayout).
var noHold bool

// flexLayout lays out the in-flow children in a content box cw×ch (inf
// when unknown) and returns the size they take. With commit it gives them
// their boxes (relative to e) and lays them out in turn.
func flexLayout(e *node, cw, ch float32, commit bool) (usedW, usedH float32) {
	row := e.row
	mainSize, crossSize := ch, cw
	gap, crossGap := e.gapY, e.gapX
	if row {
		mainSize, crossSize = cw, ch
		gap, crossGap = e.gapX, e.gapY
	}
	align := e.align
	if align == alignAuto {
		align = Stretch
	}
	scratch := &e.c.rt.flex
	firstItem, firstLine := len(scratch.items), len(scratch.lines)
	baselines := align == Baseline // whether children align by their baselines
	defer func() {
		// Laid out again only when they change (layoutcache.go), the
		// items let go of their elements.
		clear(scratch.items[firstItem:])
		scratch.items, scratch.lines = scratch.items[:firstItem], scratch.lines[:firstLine]
	}()
	var items []flexItem
	var lines []flexLine
	r := b2i(row)
	ms, me := mainMargins[r][0], mainMargins[r][1]
	cs, ce := crossMargins[r][0], crossMargins[r][1]
	taken, sameCh := false, false
	if commit && e.hold != 0 && !noHold { // nothing to take for most: one compare
		var bl bool
		if items, lines, bl, sameCh, taken = e.takeHold(cw, ch, row); taken {
			baselines = baselines || bl
		}
	}
	if !taken {
		// Whether the items break into lines, and the line they are on:
		// they break as they come, and the lines are read again by the
		// passes that follow.
		wrap := e.wrap && finite(mainSize)
		cur := flexLine{}
		for c := e.first; c != nil; c = c.next {
			if c.flags&flagAbsolute != 0 || c.hidden {
				continue
			}
			it := flexItem{e: c}
			// The margins of the item, and which of them are automatic:
			// read here with the margins the layout reads the sizes of,
			// so that the passes after this one read them in the item
			// and not the element again.
			mStart, mEnd := c.margin[ms], c.margin[me]
			cStart, cEnd := c.margin[cs], c.margin[ce]
			if isAuto(mStart) {
				it.auto |= autoMainStart
				mStart = 0
			}
			if isAuto(mEnd) {
				it.auto |= autoMainEnd
				mEnd = 0
			}
			if isAuto(cStart) {
				it.auto |= autoCrossStart
				cStart = 0
			}
			if isAuto(cEnd) {
				it.auto |= autoCrossEnd
				cEnd = 0
			}
			it.mainStart, it.crossStart = mStart, cStart
			it.marginMain, it.marginCr = mStart+mEnd, cStart+cEnd
			// The alignment across the line, which the placement reads in
			// the item, and not the element again.
			it.align = selfAlign(c, align)
			stretch := stretches(c, it.align, it.autoCross())
			// resolveFlexible reads the flexible lengths of every item of
			// the line in every round: it reads the item, which it has in
			// order, and not the element again.
			it.grow = c.grow
			var own float32 // the size along the main axis the element gives itself
			var owns bool
			if row {
				own, owns = c.width.resolve(base(cw))
				// Across the row it is as high as it says, or as the row where
				// it stretches to its one line, which measuring it first would
				// not change; the others are as high as their width, known
				// once the line's flexible lengths are, makes them.
				v, ok := c.height.resolve(base(ch))
				if stretch && !ok && c.aspect == 0 {
					it.flags |= flagStretch
				}
				if ok {
					it.cross = c.clampH(v, ch)
					it.flags |= flagCrossKnown
					if c.height.u == unitPx {
						it.flags |= flagCrossAbs
					}
				} else if it.has(flagStretch) && finite(ch) && !e.wrap {
					it.cross = c.clampH(ch-it.marginCr, ch)
					it.flags |= flagCrossKnown
				}
				if c.minH.set != 0 || c.maxH.set != 0 {
					it.flags |= flagCrossBound
				}
			} else {
				own, owns = c.height.resolve(base(ch))
				// Across the column it is as wide as it says, as the column
				// where it stretches to it, or as its content. In a column
				// that wraps it fits its content, and stretches only to its
				// line, as wide as the widest there.
				v, ok := c.width.resolve(base(cw))
				if stretch && !ok {
					it.flags |= flagStretch
				}
				if ok {
					it.cross = c.clampW(v, cw)
				} else if it.has(flagStretch) && finite(cw) && !e.wrap {
					it.cross = c.clampW(cw-it.marginCr, cw)
				} else {
					it.cross = fitWidth(c, cw-it.marginCr, cw)
				}
				if c.minW.set != 0 || c.maxW.set != 0 {
					it.flags |= flagCrossBound
				}
				it.flags |= flagCrossKnown
			}
			// The flex base size.
			if v, ok := c.basis.resolve(base(mainSize)); ok {
				it.base = v
			} else if owns {
				it.base = own
			} else if row {
				it.base = contentWidth(c, true)
			} else {
				it.base = naturalHeight(c, it.cross, ch)
			}
			// The automatic minimum is the content's, unless it scrolls or
			// clips that way. Measuring the content cannot see the main size
			// this container has, so a maximum that is a percentage of it does
			// not bind there: clamp the minimum by the maximum here, where the
			// main size the maximum resolved against is known. A minimum the
			// element asked for itself still wins over the maximum.
			it.minMain, it.maxMain = 0, inf
			if row {
				if v, ok := c.maxW.upper(base(cw)); ok {
					it.maxMain = v
				}
				if v, ok := c.minW.lower(base(cw)); ok {
					it.minMain = v
				} else if !c.scrolls() && c.flags&flagClipX == 0 {
					it.minMain = min(intrinsic(c, false), it.maxMain)
					if owns {
						it.minMain = min(it.minMain, own)
					}
				}
			} else {
				if v, ok := c.maxH.upper(base(ch)); ok {
					it.maxMain = v
				}
				if v, ok := c.minH.lower(base(ch)); ok {
					it.minMain = v
				} else if !c.scrolls() && c.flags&flagClipY == 0 && c.grow == 0 {
					it.minMain = min(heightAt(c, it.cross, inf), it.maxMain)
					if owns {
						it.minMain = min(it.minMain, own)
					}
				}
			}
			pad := c.padY()
			if row {
				pad = c.padX()
			}
			// No size along it goes below the padding and border, as in CSS.
			it.base = max(it.base, pad)
			it.minMain, it.maxMain = max(it.minMain, pad), max(it.maxMain, pad)
			it.hyp = max(it.minMain, min(it.base, it.maxMain))
			// What it shrinks by is what it has of its own, less its
			// padding and border (CSS flexbox §9.7): the passes that
			// resolve the flexible lengths read it in the item.
			it.shrink = c.shrink * max(it.base-pad, 0)
			baselines = baselines || c.self == Baseline
			// After the child's measures, which lay out containers in it.
			scratch.items = append(scratch.items, it)
			// The line the item is on, broken as the items come: the
			// items of a line are read again by the passes that follow,
			// and the walk over them is one fewer.
			n := len(scratch.items) - firstItem - 1
			outer := it.hyp + it.marginMain
			if wrap && n > cur.start && cur.main+gap+outer > mainSize {
				cur.end = n
				scratch.lines = append(scratch.lines, cur)
				cur = flexLine{start: n}
			}
			if n > cur.start {
				cur.main += gap
			}
			cur.main += outer
		}
		items = scratch.items[firstItem:]

		if len(items) > 0 {
			cur.end = len(items)
			scratch.lines = append(scratch.lines, cur)
		}
		lines = scratch.lines[firstLine:]

		// Resolve the flexible lengths of each line.
		for li := range lines {
			ln := &lines[li]
			its := items[ln.start:ln.end]
			if !finite(mainSize) {
				// The line takes the size each item has of its own: the
				// flexible lengths are not resolved without a size to
				// resolve them against.
				for i := range its {
					its[i].main = its[i].hyp
				}
				continue
			}
			gaps := gap * float32(len(its)-1)
			resolveFlexible(its, mainSize-gaps, ln.main-gaps < mainSize-gaps)
			ln.main = gaps
			for _, it := range its {
				ln.main += it.main + it.marginMain
			}
		}

	}
	if taken && row && !sameCh {
		e.c.rt.flex.holdRefreshed++
		// What the measuring pass knew of the children's cross sizes was
		// known at the height it measured; a row committed at another
		// height knows it again, before the cross sizes are taken. A
		// column is only taken at the height it was measured at.
		for i := range items {
			it := &items[i]
			if it.has(flagCrossAbs) && !it.has(flagCrossBound) {
				// Its size across the line is absolute and unbounded: it
				// is the same at this height as at the height it was
				// measured at, and the element is not read again.
				continue
			}
			c := it.e
			stretch := stretches(c, it.align, it.autoCross())
			v, ok := c.height.resolve(base(ch))
			it.flags &^= flagStretch
			if stretch && !ok && c.aspect == 0 {
				it.flags |= flagStretch
			}
			switch {
			case ok:
				it.cross = c.clampH(v, ch)
				it.flags |= flagCrossKnown
			case it.has(flagStretch) && finite(ch) && !e.wrap:
				it.cross = c.clampH(ch-it.marginCr, ch)
				it.flags |= flagCrossKnown
			}
		}
	}
	if !commit && !noHold {
		// What is worth keeping: a row's children depend on the width
		// alone, so its commit pass takes them at another height; a
		// column's depend on the height, which the commit pass has and
		// the measuring pass did not, unless it scrolls in Y, which
		// leaves its height unknown. A container that scrolls in X has
		// no width to take them at.
		if e.flags&flagScrollX == 0 && (row || e.flags&flagScrollY != 0) {
			e.saveHold(cw, ch, row, baselines, items, lines)
		}
	}
	// Cross sizes: natural ones first, then stretched ones fill the line.
	for li := range lines {
		ln := &lines[li]
		for i := ln.start; i < ln.end; i++ {
			it := &items[i]
			if !it.has(flagCrossKnown) {
				it.cross = heightAt(it.e, it.main, ch)
			}
			ln.cross = max(ln.cross, it.cross+it.marginCr)
		}
		// The items aligned by their baselines line them up: the line
		// is as high as the highest baseline above and the deepest
		// descent below.
		// Across a column, their left edges are their baselines, as in
		// Chrome, even laid out right to left.
		ascent, descent := -inf, -inf
		for i := ln.start; i < ln.end && baselines; i++ {
			it := &items[i]
			c := it.e
			if !byBaselineItem(c, it.align, e, it.autoCross()) {
				continue
			}
			switch {
			case row:
				layoutBox(c, it.main, it.cross)
				it.flags |= flagLaidOut
				b, ok := boxBaseline(c)
				if !ok {
					b = it.cross
				}
				it.baseline = c.m(0) + b
			case e.rtl():
				it.baseline = c.m(1) + it.cross
			default:
				it.baseline = c.m(3)
			}
			it.flags |= flagByBase
			ascent = max(ascent, it.baseline)
			descent = max(descent, it.cross+it.marginCr-it.baseline)
		}
		if finite(-ascent) {
			ln.baseline = ascent
			ln.cross = max(ln.cross, ascent+descent)
		}
	}
	if len(lines) == 1 && finite(crossSize) && !e.wrap {
		lines[0].cross = crossSize
	}
	// The lines across the container, as AlignContent places them.
	var linesCross float32
	for li, ln := range lines {
		if li > 0 {
			linesCross += crossGap
		}
		linesCross += ln.cross
	}
	var crossStart, crossBetween float32
	if ac := e.alignContent; ac != alignAuto && finite(crossSize) && len(lines) > 0 && (e.wrap || len(lines) > 1) {
		free := crossSize - linesCross
		if ac == Stretch {
			if free > 0 {
				for li := range lines {
					lines[li].cross += free / float32(len(lines))
				}
				linesCross = crossSize
			}
		} else {
			crossStart, crossBetween = justifyOffsets(ac, max(free, 0), len(lines))
			if free < 0 && (ac == Center || ac == End) {
				crossStart, _ = justifyOffsets(ac, free, len(lines))
			}
		}
	}
	for li := range lines {
		ln := &lines[li]
		for i := ln.start; i < ln.end; i++ {
			it := &items[i]
			if !it.has(flagStretch) {
				continue
			}
			if it.has(flagCrossBound) {
				// Its size across the line is bounded: the bounds are of
				// the element, which is read again for them.
				if row {
					it.cross = it.e.clampH(ln.cross-it.marginCr, ch)
				} else {
					it.cross = it.e.clampW(ln.cross-it.marginCr, cw)
				}
			} else if v := ln.cross - it.marginCr; v > it.cross {
				// Unbounded, it fills the line: the line is at least as
				// large as what the item already has, so it grows to it
				// and never shrinks, as the clamp would not either.
				it.cross = v
			}
		}
	}

	// The size the lines take.
	var usedMain float32
	for _, ln := range lines {
		usedMain = max(usedMain, ln.main)
	}
	usedCross := linesCross
	if row {
		usedW, usedH = usedMain, usedCross
	} else {
		usedW, usedH = usedCross, usedMain
	}
	if !commit {
		return usedW, usedH
	}

	// Place the items, along the main axis from its start (the right of a
	// reversed row, the bottom of a reversed column), and the lines across
	// it from theirs (the bottom of a row wrapping in reverse). Right to
	// left, a row starts at the right, as does a column across it.
	flipMain := e.reverse != (row && e.rtl())
	flipCross := e.wrapReverse != (!row && e.rtl())
	contentMain := mainSize
	if !finite(contentMain) {
		contentMain = usedMain
	}
	contentCross := crossSize
	if !finite(contentCross) {
		contentCross = usedCross
	}
	// The automatic margins of the items, which the first walk read of
	// them, told by the ends they are at now that the axis may be
	// flipped.
	autoStart, autoEnd := uint8(autoMainStart), uint8(autoMainEnd)
	if flipMain {
		autoStart, autoEnd = autoEnd, autoStart
	}
	autoStartC, autoEndC := uint8(autoCrossStart), uint8(autoCrossEnd)
	if flipCross {
		autoStartC, autoEndC = autoEndC, autoStartC
	}
	crossPos := crossStart
	for _, ln := range lines {
		its := items[ln.start:ln.end]
		free := max(contentMain-ln.main, 0)
		// Automatic margins take the free space first.
		autos := 0
		for i := range its {
			it := &its[i]
			autos += b2i(it.auto&autoMainStart != 0) + b2i(it.auto&autoMainEnd != 0)
		}
		pos, between := justifyOffsets(e.justify, free, len(its))
		auto := float32(0)
		if autos > 0 {
			pos, between, auto = 0, 0, free/float32(autos)
		}
		for i := range its {
			it := &its[i]
			c := it.e
			// The margins at the start and the end of the main axis, as
			// the axis may be flipped: the item carries them, and the
			// element is not read again here.
			startM, endM := it.mainStart, it.marginMain-it.mainStart
			if flipMain {
				startM, endM = endM, startM
			}
			if it.auto&autoStart != 0 {
				startM = auto
			}
			if it.auto&autoEnd != 0 {
				endM = auto
			}
			mainOff := pos + startM
			if flipMain {
				mainOff = contentMain - mainOff - it.main
			}
			var crossOff float32
			free := ln.cross - it.cross - it.marginCr
			switch autoS, autoE := it.auto&autoStartC != 0, it.auto&autoEndC != 0; {
			case autoS && autoE:
				crossOff = max(free, 0) / 2
			case autoS:
				crossOff = max(free, 0)
			case autoE:
			case it.has(flagByBase):
				crossOff = ln.baseline - it.baseline
			default:
				switch it.align {
				case Center:
					crossOff = free / 2
				case End:
					crossOff = free
				}
			}
			crossStart := it.crossStart
			if flipCross {
				crossStart = it.marginCr - crossStart
			}
			crossOff += crossPos + crossStart
			if flipCross {
				crossOff = contentCross - crossOff - it.cross
			}
			if row {
				c.x = e.contentX() + mainOff
				c.y = e.contentY() + crossOff
				if !it.has(flagLaidOut) {
					layoutBox(c, it.main, it.cross)
				}
			} else {
				c.x = e.contentX() + crossOff
				c.y = e.contentY() + mainOff
				layoutBox(c, it.cross, it.main)
			}
			dx, dy := c.relative(cw, ch)
			c.x += dx
			c.y += dy
			pos += startM + it.main + endM + gap + between
		}
		crossPos += ln.cross + crossGap + crossBetween
	}
	return usedW, usedH
}

// stretches reports whether c stretches across the line of a row (or a
// column), as the alignment self gives itself asks, which icons do not,
// as they keep their size, nor elements with automatic margins across it.
func stretches(c *node, self Align, autoCross bool) bool {
	return self == Stretch && c.kind != kindIcon && !autoCross
}

// byBaseline reports whether c, a child of the row (or grid) e, lines up
// its baseline with its siblings', as aligning by align asks: not where
// automatic margins place it across, nor in a row wrapping in reverse,
// where Baseline is Start.
func byBaseline(c *node, align Align, e *node) bool {
	m := crossMargins[b2i(e.row && !e.grid)]
	if e.grid {
		m = [2]int{0, 2}
	}
	return selfAlign(c, align) == Baseline && !isAuto(c.margin[m[0]]) && !isAuto(c.margin[m[1]]) && !e.wrapReverse
}

// byBaselineItem reports whether the element c of the item lines up its
// baseline with its siblings', as the alignment the item gives itself
// says: the same as byBaseline, which reads it of the element, and the
// item's automatic margins across the line say.
func byBaselineItem(c *node, self Align, e *node, autoCross bool) bool {
	return self == Baseline && !autoCross && !e.wrapReverse
}

// boxBaseline returns how far below the top of an element laid out its
// first baseline is, as CSS's: that of its first line of text, or that of
// its first child (in a row, of the first child aligned by its baseline),
// which a child without one takes from its bottom; false for an element
// with neither text nor children.
func boxBaseline(e *node) (float32, bool) {
	switch e.kind {
	case kindText:
		if e.tl != nil && len(e.tl.Lines) > 0 {
			return e.contentY() + e.tl.Lines[0].Baseline, true
		}
		return 0, false
	case kindInput:
		if ed := e.st.editor; ed != nil {
			if l := ed.firstLine(); l != nil {
				return ed.originY + l.Baseline, true
			}
		}
		return 0, false
	case kindImage, kindIcon:
		return 0, false
	}
	align := e.align
	if align == alignAuto {
		align = Stretch
	}
	var first *node
	for ch := e.first; ch != nil; ch = ch.next {
		if ch.flags&flagAbsolute != 0 || ch.hidden {
			continue
		}
		if first == nil {
			first = ch
		}
		if !e.row || e.grid {
			break
		}
		if byBaseline(ch, align, e) {
			first = ch
			break
		}
	}
	if first == nil {
		return 0, false
	}
	b, ok := boxBaseline(first)
	if !ok {
		b = first.h
	}
	return first.y + b, true
}

func selfAlign(c *node, align Align) Align {
	if c.self != alignAuto {
		return c.self
	}
	return align
}

// resolveFlexible grows or shrinks the items of a line to fill space,
// honoring their minimum and maximum sizes (CSS flexbox §9.7).
func resolveFlexible(its []flexItem, space float32, growing bool) {
	// The first walk freezes the items that cannot grow or shrink, and
	// sums what the others grow or shrink by, in one walk: the walk that
	// freezes them is the walk that sums them, and it reads the item,
	// which it has in order, and not the element again.
	free, sum := space, float32(0)
	for i := range its {
		it := &its[i]
		it.flags &^= flagFrozen
		// What every item starts from, which the rounds below give to
		// the ones they resolve: where the factors sum to zero, no round
		// runs, and an item that cannot grow or shrink keeps this.
		it.main = it.hyp
		factor := it.grow
		if !growing {
			factor = it.shrink
		}
		if factor == 0 || (growing && it.base > it.hyp) || (!growing && it.base < it.hyp) {
			it.flags |= flagFrozen
			free -= it.main + it.marginMain
			continue
		}
		free -= it.base + it.marginMain
		sum += factor
	}
	for range its {
		if sum == 0 {
			break
		}
		var violation float32
		for i := range its {
			it := &its[i]
			if it.has(flagFrozen) {
				continue
			}
			if growing {
				it.main = it.base + free*it.grow/sum
			} else {
				it.main = it.base + free*it.shrink/sum
			}
			clamped := max(it.minMain, min(it.main, it.maxMain))
			// How far clamping moved it, and which way: the rounds after
			// this one freeze the items clamped the way the total moved.
			it.flags &^= flagUp | flagDown
			if clamped > it.main {
				it.flags |= flagUp
				violation += clamped - it.main
			} else if clamped < it.main {
				it.flags |= flagDown
				violation += clamped - it.main
			}
			it.main = clamped
		}
		if violation == 0 {
			break
		}
		// Freeze the items clamped the way the total moved: by their
		// minimums or by their maximums, the minimum winning where it is
		// the larger.
		for i := range its {
			it := &its[i]
			if !it.has(flagFrozen) && (violation > 0 && it.has(flagUp) || violation < 0 && it.has(flagDown)) {
				it.flags |= flagFrozen
			}
		}
		// What is left to grow or shrink, and the room left for it.
		free, sum = space, 0
		for _, it := range its {
			if it.has(flagFrozen) {
				free -= it.main + it.marginMain
			} else {
				free -= it.base + it.marginMain
				if growing {
					sum += it.grow
				} else {
					sum += it.shrink
				}
			}
		}
	}
}

// justifyOffsets returns where the first item goes and the extra space
// between items.
func justifyOffsets(j Align, free float32, n int) (start, between float32) {
	switch j {
	case Center:
		return free / 2, 0
	case End:
		return free, 0
	case SpaceBetween:
		if n > 1 {
			return 0, free / float32(n-1)
		}
	case SpaceAround:
		if n > 0 {
			return free / float32(n) / 2, free / float32(n)
		}
	case SpaceEvenly:
		return free / float32(n+1), free / float32(n+1)
	}
	return 0, 0
}

// layoutAbsolute places the absolute children in the padding box. Right
// to left, as in CSS, one with neither Left nor Right goes to the right,
// and Right wins over Left where its width leaves no room for both.
func layoutAbsolute(e *node) {
	if !e.absoluteChild {
		return // no child out of the flow to place
	}
	pw, ph := e.w-e.border[1]-e.border[3], e.h-e.border[0]-e.border[2]
	for c := e.first; c != nil; c = c.next {
		if c.flags&flagAbsolute == 0 || c.leaving != 0 || c.hidden {
			// Copies of elements leaving are placed by their transitions.
			continue
		}
		if c.attach != 0 {
			if c.popover != nil && c.popover.inHidden() {
				// Nowhere to attach to: it hides with its target.
				journalKeep(c.c.rt, &c.hidden)
				c.hidden = true
				continue
			}
			if c.popover != nil {
				attachTo(c, e, pw, ph)
			} else {
				attach(c, e, pw, ph)
			}
			continue
		}
		pw, ph := pw, ph
		var ox, oy float32
		if s := c.placeOn; s != nil {
			pw, ph = s.w, s.h
			ox, oy = s.x-e.border[3], s.y-e.border[0]
		}
		top, tok := c.inset[0].resolve(ph)
		right, rok := c.inset[1].resolve(pw)
		bottom, bok := c.inset[2].resolve(ph)
		left, lok := c.inset[3].resolve(pw)
		var w float32
		if v, ok := c.width.resolve(pw); ok {
			w = c.clampW(v, pw)
		} else if lok && rok {
			w = c.clampW(pw-left-right-c.marginX(), pw)
		} else {
			w = fitWidth(c, pw-c.marginX(), pw)
		}
		var h float32
		if v, ok := c.height.resolve(ph); ok {
			h = c.clampH(v, ph)
		} else if tok && bok {
			h = c.clampH(ph-top-bottom-c.marginY(), ph)
		} else {
			h = heightAt(c, w, ph)
		}
		x := left + c.m(3)
		rtl := c.rtl()
		if !lok && (rok || rtl) {
			x = pw - right - w - c.m(1)
		} else if lok && rok {
			// Automatic margins share the room left between the insets.
			free := pw - left - right - w - c.marginX()
			switch autoL, autoR := isAuto(c.margin[3]), isAuto(c.margin[1]); {
			case autoL && autoR:
				x += max(free, 0) / 2
			case autoL:
				x += max(free, 0)
			case !autoR && rtl:
				x += free
			}
		}
		y := top + c.m(0)
		if !tok && bok {
			y = ph - bottom - h - c.m(2)
		} else if tok && bok {
			free := ph - top - bottom - h - c.marginY()
			switch autoT, autoB := isAuto(c.margin[0]), isAuto(c.margin[2]); {
			case autoT && autoB:
				y += max(free, 0) / 2
			case autoT:
				y += max(free, 0)
			}
		}
		c.x, c.y = e.border[3]+ox+x, e.border[0]+oy+y
		layoutBox(c, w, h)
	}
}

// attachTo places c, a child of e whose padding box is pw×ph, attached to
// a point of the box of the element it is a popover of (AttachTo).
func attachTo(c, e *node, pw, ph float32) {
	var w float32
	if v, ok := c.width.resolve(pw); ok {
		w = c.clampW(v, pw)
	} else {
		w = fitWidth(c, pw, pw)
	}
	var h float32
	if v, ok := c.height.resolve(ph); ok {
		h = c.clampH(v, ph)
	} else {
		h = heightAt(c, w, ph)
	}
	t := laidOutVisualBox(c.popover)
	// No taller than the room the window has: one that scrolls, as a
	// select's long list of options, scrolls, and children that shrink,
	// as a scroll container inside a panel of SelectBase's, fit.
	_, ay, _, sy := c.attachFractions()
	h = min(h, roomAlong(t.Y, t.H, beside(ay, sy), c.m(0)+c.m(2), c.c.h, c.windowEdge()))
	c.x, c.y = attachedAt(c, e, t, pw, ph, w, h)
	layoutBox(c, w, h)
}

// reattach moves the elements of the overlay ov attached to others
// (AttachTo) to where those show in this frame, after transitions. It
// keeps their size and what is inside them, which transitions moved too,
// and the distance their own transition has them from their place, as an
// Enter's X and Y.
func reattach(ov *node) {
	pw, ph := ov.w-ov.border[1]-ov.border[3], ov.h-ov.border[0]-ov.border[2]
	for c := ov.first; c != nil; c = c.next {
		if c.popover == nil || c.attach == 0 || c.flags&flagAbsolute == 0 || c.leaving != 0 || c.hidden {
			continue
		}
		c.x, c.y = attachedAt(c, ov, laidOutVisualBox(c.popover), pw, ph, c.w, c.h)
		if r := ov.c.rt.trans[c.id]; r != nil && r.entering&movePos != 0 {
			c.x += r.shown.x - r.target.x
			c.y += r.shown.y - r.target.y
		}
	}
}

// attachedAt returns where c, w×h and a child of e whose padding box is
// pw×ph, goes relative to e, attached to t, the box of its target.
// It notes the anchors it puts together (AttachedAnchors), and puts the
// transform origin on the element's (AttachedOrigin).
func attachedAt(c, e *node, t Rect, pw, ph, w, h float32) (x, y float32) {
	ax, ay, sx, sy := c.attachFractions()
	edge := c.windowEdge()
	x, fx := alongTarget(t.X, t.W, ax, sx, w, c.m(3)-c.m(1), c.c.w, edge)
	y, fy := alongTarget(t.Y, t.H, ay, sy, h, c.m(0)-c.m(2), c.c.h, edge)
	c.placed, c.flips = c.attach.flipped(fx, fy), 0
	if fx {
		c.flips |= flipAcross
	}
	if fy {
		c.flips |= flipDown
	}
	if c.hasState() {
		c.st.placed, c.st.asked = c.placed, c.attach
	}
	if c.originAttached {
		if fx {
			sx = 1 - sx
		}
		if fy {
			sy = 1 - sy
		}
		rt := c.c.rt
		journalKeep(rt, &c.originX)
		journalKeep(rt, &c.originY)
		journalKeep(rt, &c.originSet)
		c.originX, c.originY, c.originSet = sx, sy, true
	}
	dx, dy := c.relative(pw, ph)
	ox, oy := laidOutOrigin(e)
	return x + dx - ox, y + dy - oy
}

// attachFractions returns the points of its target's box and of its own
// that AttachTo puts together, as fractions of the boxes' sizes.
func (c *node) attachFractions() (ax, ay, sx, sy float32) {
	at, self := c.attach.anchors()
	at, self = at.physical(c.rtl()), self.physical(c.rtl())
	ax, ay = at.fractions()
	sx, sy = self.fractions()
	return ax, ay, sx, sy
}

// windowMargin keeps what overlays place apart from the window's edges,
// unless the element says otherwise (AttachEdgeMargin).
const windowMargin = 4

// windowEdge returns how far AttachTo keeps c from the window's edges.
func (c *node) windowEdge() float32 {
	if c.edgeMarginSet {
		return c.edgeMargin
	}
	return windowMargin
}

// roomAlong returns the most an element attached to target can be along
// an axis of the window, limit long, and stay in it, edge from its ends:
// the larger side of target for one placed beside it, else the window.
// An attached element is no taller (attachTo).
func roomAlong(t0, tsize float32, beside bool, margins, limit, edge float32) float32 {
	room := limit - 2*edge - margins
	if beside {
		room = max(t0, limit-(t0+tsize)) - edge - margins
	}
	return max(room, 0)
}

// beside reports whether an element with its point at fraction self on
// its target's point at fraction at goes before or after the target.
func beside(at, self float32) bool { return at == 1 && self == 0 || at == 0 && self == 1 }

// alongTarget returns where an element size long goes along an axis of the
// window, limit long, with its point at fraction self on target's point at
// fraction at, moved by off: on the other side of target, or the other way
// along it, where that overflows the window, less edge at each end, less
// (flipped), then moved into it along target.
func alongTarget(t0, tsize, at, self, size, off, limit, edge float32) (p float32, flipped bool) {
	pos := func(at, self, off float32) float32 { return t0 + at*tsize - self*size + off }
	over := func(p float32) float32 {
		return max(0, edge-p) + max(0, p+size-(limit-edge))
	}
	p = pos(at, self, off)
	if over(p) > 0 {
		if q := pos(1-at, 1-self, -off); over(q) < over(p) {
			p, flipped = q, true
		}
	}
	if !beside(at, self) && over(p) > 0 {
		p = max(edge, min(p, limit-edge-size))
	}
	return p, flipped
}

// laidOutOrigin returns where place will put the box of an element laid
// out, in the window: its position and those of the elements around it,
// less the offsets of the scroll containers among them, and where sticky
// ones among them stick.
func laidOutOrigin(e *node) (x, y float32) {
	x, y = flowOrigin(e)
	if e.stuck() {
		dx, dy := e.stickyShift(x, y, laidOutOrigin)
		x += dx
		y += dy
	}
	return x, y
}

// flowOrigin returns where the flow puts an element laid out, in the
// window, as laidOutOrigin does, but for where it sticks itself.
func flowOrigin(e *node) (x, y float32) {
	p := e.parent
	if p == nil {
		return e.x, e.y
	}
	x, y = laidOutOrigin(p)
	if e.flags&flagAbsolute == 0 || e.leaving == 1 {
		if f := p.followX; f != nil {
			x -= float32(f.followShift())
		}
		if p.scrolls() {
			x -= float32(p.scrollLeft())
			y -= float32(p.st.scrollY - p.scrollBase)
		}
	}
	return x + e.x, y + e.y
}

// laidOutBox returns where place, or commit for an inline element, will
// put the box of an element of the frame, in the window, as its overlays
// are laid out; that of the last frame for one this frame did not build.
func laidOutBox(e *node) Rect {
	if e.st.seen != e.c.rt.frame || e.parent == nil {
		return e.Bounds()
	}
	if !e.isInline() {
		x, y := laidOutOrigin(e)
		return Rect{x, y, e.w, e.h}
	}
	// The box around its text in the paragraph's layout.
	start, end := e.runes[0], e.runes[1]
	para := e.parent
	for para.isInline() {
		start, end = start+para.runes[0], end+para.runes[0]
		para = para.parent
	}
	x, y := laidOutOrigin(para)
	x, y = x+para.contentX(), y+para.contentY()
	if para.tl == nil {
		return Rect{x, y, 0, 0}
	}
	var box Rect
	for i, r := range para.tl.Selection(start, end) {
		if f := (Rect{x + r.X, y + r.Y, r.W, r.H}); i == 0 {
			box = f
		} else {
			box = union(box, f)
		}
	}
	return box
}

// attach places c, attached to a point of the padding box of its parent e,
// pw×ph (Attach).
func attach(c, e *node, pw, ph float32) {
	var w float32
	if v, ok := c.width.resolve(pw); ok {
		w = c.clampW(v, pw)
	} else {
		w = fitWidth(c, pw, pw)
	}
	var h float32
	if v, ok := c.height.resolve(ph); ok {
		h = c.clampH(v, ph)
	} else {
		h = heightAt(c, w, ph)
	}
	at, self := c.attach.anchors()
	at, self = at.physical(c.rtl()), self.physical(c.rtl())
	ax, ay := at.fractions()
	sx, sy := self.fractions()
	dx, dy := c.relative(pw, ph)
	c.x = e.border[3] + ax*pw - sx*w + dx
	c.y = e.border[0] + ay*ph - sy*h + dy
	layoutBox(c, w, h)
}
