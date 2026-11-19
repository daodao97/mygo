package editor

import (
	"slices"
	"sort"
	"strconv"

	"github.com/egoist/mygo/ui"
)

// Replacement shows bytes Start to End of the text as something else in
// the editor's lines, as Zed's folds show their placeholders: Text, the
// element Content builds, or nothing. The text stays the buffer's: it
// copies, finds, undoes and reads to assistive technology as it is.
// Carets step over the range as over one character, and deleting next to
// it deletes all of it.
type Replacement struct {
	Start, End int
	// Text shows in place of the range, in the editor's font, in Color or
	// else the color of the text it replaces. With neither Text nor
	// Content, the range shows nothing.
	Text  string
	Color ui.Color
	// Content builds what shows in place of the range, in place of Text:
	// e is the element the editor places there, Width DIPs wide (a square
	// as high as a row when 0) and as high as a row, to style and fill as
	// the editor's parts are (RenderFindBar), and r the replacement as the
	// edits since it was set moved it. A press on it goes to the text, as
	// on the range's nearer end, unless Interactive.
	Content func(c *ui.Context, e ui.Element, r Replacement)
	Width   float32
	// Interactive keeps the presses on Content's element for what is in
	// it, as a check box's, rather than the text's, the wheel still
	// scrolling. Give a button KeepFocus to keep the typing in the editor.
	Interactive bool
	// Label names Content's element for assistive technology, as "Done"
	// for a check box drawn in place of "[x]".
	Label string
}

// sameLayout reports whether a and b lay their lines out alike, as their
// elements' functions, which only build, may differ.
func (a Replacement) sameLayout(b Replacement) bool {
	return a.Start == b.Start && a.End == b.End && a.Text == b.Text && a.Color == b.Color &&
		a.Width == b.Width && (a.Content == nil) == (b.Content == nil)
}

// shown returns what the replacement shows in a line's displayed text:
// an object replacement character for its element, which the text system
// gives its width (faceRun.obj), else its Text.
func (r *Replacement) shown() string {
	if r.Content != nil {
		return objectChar
	}
	return r.Text
}

// objectChar stands for an element in a line's displayed text.
const objectChar = "￼"

// setReplacements shows rs in place of their ranges, kept sorted and
// apart, and lays out again the lines whose replacements changed.
func (ed *editor) setReplacements(rs []Replacement) {
	v := &ed.view
	n := v.text.Len()
	in := v.replScratch[:0]
	for _, r := range rs {
		r.Start, r.End = max(0, min(r.Start, n)), max(0, min(r.End, n))
		if r.End > r.Start {
			in = append(in, r)
		}
	}
	slices.SortStableFunc(in, func(a, b Replacement) int { return a.Start - b.Start })
	out := in[:0]
	for _, r := range in {
		if k := len(out); k > 0 && r.Start < out[k-1].End {
			continue
		}
		out = append(out, r)
	}
	clear(in[len(out):])
	old, oldCross := v.repls, v.replCross
	// A range the last ones held holds the same bytes, and crosses lines
	// or not as it did.
	cross := slices.Grow(v.crossScratch[:0], len(out))[:len(out)]
	for k := range cross {
		cross[k] = true
	}
	// The lines of the replacements that are not in both, or not alike.
	changed := false
	for i, j := 0, 0; i < len(old) || j < len(out); {
		switch {
		case j >= len(out) || i < len(old) && old[i].Start < out[j].Start:
			changed = true
			v.forgetLines(old[i].Start, old[i].End)
			i++
		case i >= len(old) || out[j].Start < old[i].Start:
			changed = true
			v.forgetLines(out[j].Start, out[j].End)
			j++
		default:
			if !old[i].sameLayout(out[j]) {
				changed = true
				v.forgetLines(old[i].Start, old[i].End)
				v.forgetLines(out[j].Start, out[j].End)
			}
			if old[i].End == out[j].End {
				cross[j] = oldCross[i]
			}
			i++
			j++
		}
	}
	// The slice given last is the scratch of the next, its functions let go.
	clear(old)
	v.repls, v.replScratch = out, old[:0]
	v.replCross, v.crossScratch = cross, oldCross[:0]
	if changed {
		v.replsChanged()
	}
}

// forgetLines lays out again the lines holding bytes a to b, and measures
// their wraps anew.
func (v *view) forgetLines(a, b int) {
	for _, child := range v.styleViews {
		child.forgetLines(a, b)
	}
	from, to := v.text.LineOf(a), v.text.LineOf(b)
	if v.lines.m != nil {
		if to-from > len(v.lines.m) {
			for k := range v.lines.m {
				if k >= from && k <= to {
					delete(v.lines.m, k)
				}
			}
		} else {
			for line := from; line <= to; line++ {
				delete(v.lines.m, line)
			}
		}
	}
	v.heights.wraps.forget(from, to)
	v.measured.forget(from, to)
	v.wrapAt = min(v.wrapAt, from)
	v.replHiddenOK = false
}

// replsChanged notes that the replacements changed or moved.
func (v *view) replsChanged() {
	v.replVersion++
	v.replHiddenOK = false
	v.contents = 0
	for i := range v.repls {
		if v.repls[i].Content != nil {
			v.contents++
		}
	}
}

// moveRepls moves the replacements through changes of the text, as
// selections move: the text inserted at their edges left out. Those whose
// text was deleted go.
func (v *view) moveRepls(cs []change) {
	if len(v.repls) == 0 {
		return
	}
	out := v.repls[:0]
	c := 0
	for i, r := range v.repls {
		// Only a change reaching inside it changes the bytes it holds.
		for c < len(cs) && cs[c].oldEnd <= r.Start {
			c++
		}
		touched := c < len(cs) && cs[c].oldStart < r.End
		r.Start, r.End = mapOffset(r.Start, cs, biasRight), mapOffset(r.End, cs, biasLeft)
		if r.End > r.Start {
			v.replCross[len(out)] = v.replCross[i] || touched
			out = append(out, r)
		}
	}
	clear(v.repls[len(out):])
	v.repls, v.replCross = out, v.replCross[:len(out)]
	v.replsChanged()
}

// replsIn returns the replacements meeting the line from byte a to its
// end, b: those holding some of its bytes, its newline among them, or
// starting at its end.
func (v *view) replsIn(a, b int) []Replacement {
	i := sort.Search(len(v.repls), func(i int) bool { return v.repls[i].End > a })
	j := i
	for j < len(v.repls) && v.repls[j].Start <= b {
		j++
	}
	return v.repls[i:j]
}

// replacedLines returns the lines replacements hide: those one covers
// from their start through their newline, but the line it starts on when
// it shows something there.
func (v *view) replacedLines() []lineSpan {
	if v.replHiddenOK {
		return v.replHidden
	}
	v.replHiddenOK = true
	v.replHidden = v.replHidden[:0]
	t := &v.text
	for i := range v.repls {
		// Most stay within a line, known without looking in the text.
		if !v.replCross[i] {
			continue
		}
		r := &v.repls[i]
		a, b := t.LineOf(r.Start), t.LineOf(r.End)
		if b == a {
			v.replCross[i] = false
			continue
		}
		first := a
		if r.Start > t.LineStart(a) || r.shown() != "" {
			first++
		}
		last := b - 1
		if first > last {
			continue
		}
		if k := len(v.replHidden); k > 0 && first <= v.replHidden[k-1].to+1 {
			v.replHidden[k-1].to = max(v.replHidden[k-1].to, last)
			continue
		}
		v.replHidden = append(v.replHidden, lineSpan{first, last})
	}
	return v.replHidden
}

// replaced returns the replacement holding off inside it, past its start.
func (v *view) replaced(off int) (Replacement, bool) {
	i := sort.Search(len(v.repls), func(i int) bool { return v.repls[i].End > off })
	if i < len(v.repls) && v.repls[i].Start < off {
		return v.repls[i], true
	}
	return Replacement{}, false
}

// overReplacements moves a caret going from off to to past a replacement
// to rather falls inside of, to its end going forward, else its start,
// so that carets step over replacements as over one character.
func (ed *editor) overReplacements(off, to int) int {
	if r, ok := ed.view.replaced(to); ok {
		if to > off {
			return r.End
		}
		return r.Start
	}
	return to
}

// stepTo returns where a caret s moves, fn telling where it goes from a
// place: past what replacements show, as past one character, and past
// those showing nothing within a line, as hidden marks, as if they were
// not there, leaving or reaching them, so that no step leaves the caret
// where it shows; and past the lines folds and replacements hide.
func (ed *editor) stepTo(s sel, fn func(sel) int) int {
	off := s.head
	to := fn(s)
	if to == off {
		return to
	}
	fwd := to > off
	if from, ok := ed.view.passHidden(off, fwd); ok {
		s.head = from
		if s.empty() || s.anchor == off {
			s.anchor = from
		}
		off, to = from, fn(s)
	}
	for range 4 {
		next := ed.overReplacements(off, ed.overFolds(off, to))
		if (next > off) == fwd && next != off {
			next, _ = ed.view.passHidden(next, fwd)
		}
		if next == to {
			break
		}
		to = next
	}
	return to
}

// passHidden returns where a caret at off going forward, or back, is past
// the replacements next to it there that show nothing within a line, and
// whether there were any.
func (v *view) passHidden(off int, fwd bool) (int, bool) {
	moved := false
	for range len(v.repls) {
		var i int
		if fwd {
			i = sort.Search(len(v.repls), func(i int) bool { return v.repls[i].Start >= off })
		} else {
			i = sort.Search(len(v.repls), func(i int) bool { return v.repls[i].End >= off })
		}
		if i >= len(v.repls) {
			break
		}
		r := &v.repls[i]
		if fwd && r.Start != off || !fwd && r.End != off || !v.hides(r) {
			break
		}
		if fwd {
			off = r.End
		} else {
			off = r.Start
		}
		moved = true
	}
	return off, moved
}

// hides reports whether r shows nothing within a line, as hidden marks,
// where its start and end show at one place.
func (v *view) hides(r *Replacement) bool {
	return r.shown() == "" && v.text.LineOf(r.Start) == v.text.LineOf(r.End)
}

// deletion returns the bytes deleting from caret s to where fn takes it
// deletes: from past the replacements next to the caret that show
// nothing within a line, as hidden marks, to the text beyond them, and
// all of a replacement it ends inside, as one character.
func (ed *editor) deletion(s sel, fn func(sel) int) (int, int) {
	to := fn(s)
	if to != s.head {
		if from, ok := ed.view.passHidden(s.head, to > s.head); ok {
			s.anchor, s.head = from, from
			to = fn(s)
		}
	}
	to = ed.overReplacements(s.head, to)
	return min(to, s.head), max(to, s.head)
}

// objectWidth returns how wide the element of a replacement is.
func (v *view) objectWidth(r *Replacement) float32 {
	if r.Width > 0 {
		return r.Width
	}
	return v.lineHeight
}

// placed is where an element is: a replacement in content coordinates,
// or a block in view coordinates. Offsets and block indices stay separate.
type placed struct {
	start      int // the replacement's byte offset
	blockIndex int // the index in view.blocks when block is true
	block      bool
	rect       ui.Rect
}

// placeReplacements appends to dst the elements of the replacements in
// the lines from y0 to y1 of the content, with their boxes.
func (ed *editor) placeReplacements(y0, y1 float64, dst []placed) []placed {
	v := &ed.view
	h := &v.heights
	if v.contents == 0 || h.n == 0 {
		return dst
	}
	first, last := h.lineAt(y0), min(h.lineAt(y1), h.n-1)
	reps := v.replsIn(v.text.LineStart(first), v.text.LineEnd(last))
	for i := range reps {
		r := &reps[i]
		if r.Content == nil {
			continue
		}
		line := v.text.LineOf(r.Start)
		if line < first || h.isHidden(line) {
			continue
		}
		l := v.line(line)
		d := l.toDisp(r.Start-l.start, false)
		ri := l.rowOf(d, false)
		x := v.xOf(l, &l.rows[ri], d)
		y := ed.rowTop(line) + float64(ri)*float64(l.rowHeight)
		dst = append(dst, placed{start: r.Start, rect: ui.Rect{X: x, Y: float32(y), W: l.owner.objectWidth(r), H: l.rowHeight}})
	}
	return dst
}

// buildElements builds the elements of the replacements and the blocks
// in view, in a box over the text that clips them to it.
func (ed *editor) buildElements(c *ui.Context) {
	v := &ed.view
	ed.placed, ed.built = ed.placed[:0], ed.built[:0]
	if v.contents == 0 && len(v.blocks) == 0 || ed.r.W <= 0 && !ed.o.fitContent {
		return
	}
	y0, y1 := ed.buildRange()
	ed.placed = ed.placeReplacements(y0, y1, ed.placed)
	ed.placed = ed.placeBlocks(y0, y1, ed.placed)
	ed.placedScroll = [2]float64{ed.scrollY, float64(ed.scrollX)}
	if len(ed.placed) == 0 {
		return
	}
	h := float32(ed.viewH)
	if ed.o.fitContent {
		h = float32(v.heights.total())
	}
	ui.Box(c).Key("code elements").Absolute().Left(ed.gutterW).Top(0).
		Width(max(0, ed.laidW-ed.gutterW)).Height(h).Clip().PassThrough().Children(func() {
		for _, p := range ed.placed {
			if p.block {
				ed.buildBlock(c, p)
				continue
			}
			r, ok := v.replacementAt(p.start)
			if !ok {
				continue
			}
			e := ui.Box(c).Key("replacement "+strconv.Itoa(p.start)).Absolute().
				Left(ed.textX-ed.gutterW+p.rect.X-ed.scrollX).Top(float32(float64(p.rect.Y)-ed.scrollY)).
				Size(p.rect.W, p.rect.H)
			ed.describeElement(e, r.Label, r.Interactive)
			r.Content(c, e, r)
		}
	})
}

// describeElement gives an element of the app's over the text its role
// and name, and keeps the presses on it from the text when interactive.
func (ed *editor) describeElement(e ui.Element, label string, interactive bool) {
	if label != "" {
		// A picture of what the text says, or what holds the controls
		// that act on it; Content can say otherwise.
		role := ui.RoleImage
		if interactive {
			role = ui.RoleGroup
		}
		e.Role(role).Label(label)
	}
	if interactive {
		e.Cursor(ui.CursorDefault).KeepFocus().HandleInput(ed.wheelOnly)
	}
}

// replacementAt returns the replacement starting at off.
func (v *view) replacementAt(off int) (Replacement, bool) {
	i := sort.Search(len(v.repls), func(i int) bool { return v.repls[i].Start >= off })
	if i < len(v.repls) && v.repls[i].Start == off {
		return v.repls[i], true
	}
	return Replacement{}, false
}

// elementsMoved reports whether the app's elements no longer match their
// last placement, as after scrolling, resizing or measuring a block. The
// caller schedules a build to place them again.
func (ed *editor) elementsMoved() bool {
	if ed.view.contents == 0 && len(ed.view.blocks) == 0 && len(ed.placed) == 0 {
		return false
	}
	y0, y1 := ed.buildRange()
	ed.placeScratch = ed.placeReplacements(y0, y1, ed.placeScratch[:0])
	ed.placeScratch = ed.placeBlocks(y0, y1, ed.placeScratch)
	moved := !slices.Equal(ed.placeScratch, ed.placed) ||
		len(ed.placed) > 0 && ed.placedScroll != [2]float64{ed.scrollY, float64(ed.scrollX)}
	return moved
}
