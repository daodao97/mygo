package editor

import (
	"math"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/internal/rope"
	"github.com/egoist/mygo/internal/text"
	"github.com/egoist/mygo/ui"
)

// view shows the lines of a Buffer: it keeps the display model in step
// with the text, lays out the lines in view a row at a time, shaping each
// row's text once and keeping it while frames draw it, and paints them
// with the colors of their tokens, in the bold and italic faces their
// styles ask for. An editor has one; a diff one per side.
type view struct {
	buf      *Buffer
	text     rope.Text
	version  uint64
	hl       Highlighter // the buffer's, as of the last frame
	hlGen    int         // counts the highlighters the view had
	readOnly bool

	font       ui.Font
	style      text.Style
	lineHeight float32
	ascent     float32
	descent    float32
	charWidth  float32 // the advance of "0", for columns
	textGen    uint64  // the text system's generation the widths are of
	tabSize    int
	wrap       bool
	wrapWidth  float32
	scale      float32
	theme      *Theme
	// sys is the text system of the window the editor draws in, which
	// build sets of the context each frame: a Tester's window shapes
	// apart from the others' shared system (#1722). A view styled before
	// a frame built it shapes in the shared one.
	sys *text.System
	// faces are the faces of the kinds of tokens, by the theme; styled
	// tells whether any is not the regular one.
	faces  [tokenKinds]uint8
	styled bool

	heights heights
	// inlays are texts shown in lines that are not part of them, sorted
	// by offset.
	inlays []inlay
	// inlayScratch keeps the inlays before they are set (keepInlays).
	inlayScratch []inlay
	// repls are the ranges of the text shown as something else, sorted
	// and apart (State.SetReplacements), and replScratch the slice the
	// next are kept in. replHidden are the lines they hide, while
	// replHiddenOK, and contents counts those that build elements.
	repls, replScratch []Replacement
	// replCross tells, by replacement, whether it may cross lines: true
	// until replacedLines looks, and again once a change reaches inside
	// it, so that an edit looks up the lines of those alone.
	replCross, crossScratch []bool
	replHidden              []lineSpan
	replHiddenOK            bool
	contents                int
	// replVersion changes only when replacement layout or source changes.
	replVersion uint64
	// blocks are the app's elements between lines, sorted by where they
	// go (State.SetBlocks), and blockScratch the slice the next are kept
	// in; nextBlock is the last id given. rows are what they take in the
	// heights, as of rowsHeight, rowsFolded and rowsRepls, unless block
	// identity, order, line, placement or effective height changed.
	blocks, blockScratch appBlocks
	nextBlock            int
	rows                 []block
	rowsHeight           float32
	rowsFolded           []lineSpan // independent snapshot of the fold cache
	rowsRepls            uint64
	blocksStale          bool

	lines  lineCache
	shaped shapeCache
	widths charWidths
	// measured are the widths of the widest lines laid out, which the
	// content scrolls sideways through.
	measured lineWidths
	wrapAt   int // the next line to measure the wraps of
	// wrapHidden are the lines hidden as wraps were measured last, which
	// were left: lines shown since are measured anew.
	wrapHidden []lineSpan
	// approx tells that a frame showed tokens from states the
	// highlighter has not confirmed yet.
	approx      bool
	tokScratch  []Token
	faceScratch []faceRun
	spanScratch []text.Span

	// faceWidths measure the bold and italic faces, by face, as widths
	// does the regular one (faceWidths[0] is unused).
	faceWidths [4]charWidths
	// lineStyles are the app's styles of lines, sorted by line
	// (State.SetLineStyles), laid out by the views of styleViews, one per
	// font and line height, which styleGen marks in use; stylesDirty asks
	// for the heights of their rows again (refreshLineHeights).
	lineStyles  []appLineStyle
	styleViews  map[lineStyleKey]*view
	styleGen    int
	stylesDirty bool
}

// inlay is text shown in a line at an offset, as an inlay hint or a
// suggestion.
type inlay struct {
	at       int
	text     string
	kind     int // 0 a hint, 1 a suggestion, inlayNoNewline a diff's mark, inlayTrailing text at a line's end
	ellipsis string
}

// inlayNoNewline is the kind of inlay a diff marks the last line of a
// text without a final newline with (markEnds).
const inlayNoNewline = 2

// inlayTrailing is the kind of inlay set with Inlay.Trailing.
const inlayTrailing = 3

// lineLayout is a line laid out for display.
type lineLayout struct {
	n     int
	start int    // the byte of the text it starts at
	text  string // the line, without its newline
	disp  string // as displayed: tabs expanded, inlays inserted
	segs  []dispSeg
	// reps are the replacements meeting the line as it was laid out,
	// which its segments of kind segReplace show (dispSeg.rep).
	reps  []Replacement
	rows  []rowLayout
	toks  []Token
	exact bool      // toks from a confirmed state
	faces []faceRun // the faces of toks, none when all are regular
	width float32
	used  uint64
	// owner is the view that laid it out: the editor's, or for a styled
	// line its style's (styledView), whose metrics its rows have.
	owner                      *view
	rowHeight, ascent, descent float32
}

// dispSeg maps a run of the displayed text to the line's: text, a tab
// expanded to spaces, an inlay, or what a replacement shows in place of
// bytes of the line, nothing for those of a replacement after the line
// it starts on.
type dispSeg struct {
	disp     int // the displayed byte it starts at
	from, to int // the bytes of the line it shows
	kind     uint8
	inlay    int
	rep      int  // the replacement, in lineLayout.reps
	ghost    bool // a suggestion or trailing text, which a caret at it shows before
}

const (
	segText uint8 = iota
	segTab
	segInlay
	segReplace
)

// rowLayout is a row of a line: a run of its displayed text, from its
// wrap indent, in pieces shaped apart for very long lines.
type rowLayout struct {
	from, to int
	indent   float32
	pieces   []rowPiece
	width    float32
}

type rowPiece struct {
	from, to int     // displayed bytes
	x        float32 // from the row's start
	l        *text.Layout
	ascii    bool
	width    float32
}

const pieceBytes = 2048

// The faces of text: the font's regular one, or bold, italic or both,
// as the styles of tokens ask.
const (
	faceBold uint8 = 1 << iota
	faceItalic
)

// faceRun is a run of a line's displayed text in a face, up to the
// displayed byte end; bytes past the last run are regular. A run of obj
// is of inline objects that wide, the elements replacements show.
type faceRun struct {
	end  int
	face uint8
	obj  float32
}

type lineCache struct {
	m     map[int]*lineLayout
	frame uint64
}

// shapeCache keeps the layouts of row texts two frames, as GPUI's
// line layout cache: those of the last frame, which the next one takes
// over as it uses them, and those of this one.
type shapeCache struct {
	style     text.Style
	sys       *text.System
	prev, cur map[shapeKey]*text.Layout
}

// shapeKey is a text and the spans of its faces (text.EncodeSpans).
type shapeKey struct{ text, spans string }

func (c *shapeCache) get(s string, style text.Style) *text.Layout {
	return c.getSpans(s, "", style)
}

// getSpans returns the layout of s with spans, as text.Params has them.
func (c *shapeCache) getSpans(s, spans string, style text.Style) *text.Layout {
	if c.style != style {
		c.style, c.prev, c.cur = style, nil, nil
	}
	if c.cur == nil {
		c.cur = map[shapeKey]*text.Layout{}
	}
	k := shapeKey{s, spans}
	if l, ok := c.cur[k]; ok {
		return l
	}
	if l, ok := c.prev[k]; ok {
		c.cur[k] = l
		return l
	}
	l := shapeText(orShared(c.sys), text.Params{Text: s, Style: style, NoWrap: true, KeepSpaces: true, Spans: spans})
	c.cur[k] = l
	return l
}

// shapeText shapes text for the view in the system of the window the
// editor draws in, which tests replace with a shaper of their own.
var shapeText = func(sys *text.System, p text.Params) *text.Layout { return sys.Shape(p) }

// orShared returns sys, or the shared text system for a view styled
// before a frame built it.
func orShared(sys *text.System) *text.System {
	if sys == nil {
		return text.Shared()
	}
	return sys
}

func (c *shapeCache) endFrame() {
	c.prev, c.cur = c.cur, c.prev
	if c.cur != nil {
		clear(c.cur)
	}
}

// charWidths measures characters for soft wraps and long lines
// without shaping them, as Zed's LineWrapper sums cached advances. A
// character shaped alone misses the room the text engine leaves around
// it in context, as Core Text does between Han and Latin letters and
// around CJK punctuation in Chinese and Japanese: the rows a line is laid
// out in are shaped to fit (wrapBreaks).
type charWidths struct {
	style text.Style
	sys   *text.System
	ascii [128]float32
	other map[rune]float32
	mono  float32 // the advance of every printable ASCII character, if one
}

func (w *charWidths) reset(style text.Style, sys *text.System) {
	if w.style == style && w.other != nil {
		return
	}
	*w = charWidths{style: style, sys: sys, other: map[rune]float32{}}
	for i := range w.ascii {
		w.ascii[i] = -1
	}
	l := shapeText(orShared(sys), text.Params{Text: asciiPrintableCode, Style: style, NoWrap: true, KeepSpaces: true})
	mono := float32(-1)
	if len(l.Lines) == 1 {
		line := l.Lines[0]
		ok := len(line.Glyphs) == len(asciiPrintableCode)
		for _, g := range line.Glyphs {
			if g.Cluster < len(asciiPrintableCode) {
				w.ascii[asciiPrintableCode[g.Cluster]] = g.Advance
				if mono < 0 {
					mono = g.Advance
				} else if math.Abs(float64(mono-g.Advance)) > 0.01 {
					ok = false
				}
			}
		}
		if ok {
			w.mono = mono
		}
	}
}

const asciiPrintableCode = " !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~"

func (w *charWidths) of(r rune) float32 {
	if r < 128 {
		if v := w.ascii[r]; v >= 0 {
			return v
		}
		if r < ' ' {
			return 0
		}
	}
	if v, ok := w.other[r]; ok {
		return v
	}
	l := shapeText(orShared(w.sys), text.Params{Text: string(r), Style: w.style, NoWrap: true, KeepSpaces: true})
	v := l.Width
	if len(w.other) < 4096 {
		w.other[r] = v
	}
	return v
}

// width returns the width of s.
func (w *charWidths) width(s string) float32 {
	if w.mono > 0 && isASCII(s) {
		return w.mono * float32(len(s))
	}
	var x float32
	for _, r := range s {
		x += w.of(r)
	}
	return x
}

// widthsOf returns the widths of a face.
func (v *view) widthsOf(face uint8) *charWidths {
	if face == 0 {
		return &v.widths
	}
	w := &v.faceWidths[face&3]
	w.reset(v.faceStyle(face), v.sys)
	return w
}

// faceStyle returns the style of the editor's font in a face.
func (v *view) faceStyle(face uint8) text.Style {
	s := v.style
	if face&faceBold != 0 {
		s.Weight = v.boldWeight()
	}
	if face&faceItalic != 0 {
		s.Italic = true
	}
	return s
}

// boldWeight is the weight of the bold face: 700, or black for a font
// already bold.
func (v *view) boldWeight() int {
	if v.style.Weight >= 700 {
		return 900
	}
	return 700
}

// runsWidth returns the width of displayed bytes a to b of s, whose
// faces are faces.
func (v *view) runsWidth(s string, faces []faceRun, a, b int) float32 {
	if len(faces) == 0 {
		return v.widths.width(s[a:b])
	}
	var x float32
	prev := 0
	for _, r := range faces {
		from, to := max(prev, a), min(r.end, b)
		prev = r.end
		switch {
		case from >= to:
		case r.obj > 0:
			x += r.obj * float32(utf8.RuneCountInString(s[from:to]))
		default:
			x += v.widthsOf(r.face).width(s[from:to])
		}
	}
	if from := max(prev, a); from < b {
		x += v.widths.width(s[from:b])
	}
	return x
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// setStyle sets the font and the line height, and the scale of the
// display, which line heights snap to.
func (v *view) setStyle(f ui.Font, lineHeight, scale float32, tabSize int) {
	if f.Family == "" {
		f.Family = "monospace"
	}
	if f.Size <= 0 {
		f.Size = 13
	}
	style := styleOf(f)
	if lineHeight <= 0 {
		lineHeight = 1.5
	}
	lh := float32(math.Round(float64(f.Size*lineHeight*scale))) / scale
	tabSize = max(1, min(tabSize, 16))
	sys := orShared(v.sys)
	v.shaped.sys = sys
	// A new generation of the text system shapes text anew, as when the
	// language the app shows changes, which the editor's text, naming
	// none, is shaped in: what was measured and shaped is to be again.
	gen := sys.Generation()
	if v.style == style && v.lineHeight == lh && v.tabSize == tabSize && v.scale == scale && v.textGen == gen {
		return
	}
	if v.textGen != gen {
		v.textGen = gen
		v.widths.other = nil
		v.faceWidths = [4]charWidths{}
		v.shaped.prev, v.shaped.cur = nil, nil
	}
	v.font, v.style, v.lineHeight, v.tabSize, v.scale = f, style, lh, tabSize, scale
	v.heights.lh = lh
	v.heights.dirty = true
	a, d, _ := sys.Metrics(style)
	v.ascent, v.descent = a, d
	v.widths.reset(style, sys)
	v.charWidth = v.widths.of('0')
	// Another font, tab size or line height, which replacements' elements
	// are as wide as, makes lines of other widths.
	v.measured.reset()
	v.lines.m = nil
	v.stylesDirty = true
	if v.wrap {
		v.heights.wraps.reset(v.text.LineCount(), true)
		v.wrapAt = 0
	}
}

// setTheme sets the theme, laying out lines anew when the faces of
// tokens change.
func (v *view) setTheme(ct *Theme) {
	v.theme = ct
	var faces [tokenKinds]uint8
	styled := false
	for k, ts := range ct.Tokens {
		if ts.Bold {
			faces[k] |= faceBold
		}
		if ts.Italic {
			faces[k] |= faceItalic
		}
		styled = styled || faces[k] != 0
	}
	if faces != v.faces {
		v.faces, v.styled = faces, styled
		v.lines.m = nil
		v.measured.reset()
	}
}

// setWrap turns soft wrapping on at width, or off.
func (v *view) setWrap(on bool, width float32) {
	width = float32(math.Floor(float64(width)))
	if on == v.wrap && (!on || width == v.wrapWidth) {
		return
	}
	v.wrap, v.wrapWidth = on, width
	v.stylesDirty = true
	v.heights.wraps.reset(v.text.LineCount(), on)
	v.heights.dirty = true
	v.wrapAt = 0
	v.lines.m = nil
	v.measured.reset()
}

// setBuffer shows another buffer.
func (v *view) setBuffer(b *Buffer) {
	if v.buf == b {
		return
	}
	v.buf = b
	v.text, v.version = b.snapshot()
	v.hl = nil
	v.lines.m = nil
	v.measured.reset()
	v.heights.setLines(v.text.LineCount())
	v.heights.wraps.reset(v.text.LineCount(), v.wrap)
	v.wrapAt = 0
}

// sync follows the buffer's edits, calling moved with the changes of
// each version so the owner moves what it keeps of the text. It returns
// false when the buffer changed past what its log keeps, and everything
// was taken anew.
func (v *view) sync(moved func(cs []change)) bool {
	if v.buf == nil {
		return true
	}
	changes, version, text, ok := v.buf.changesTo(v.version)
	if version == v.version && ok {
		v.text = text
		return true
	}
	old := v.text
	v.text, v.version = text, version
	v.lines.m = nil
	// The highlighter hears of the edits before a line of the text is
	// laid out, as moving the caret into view does between frames.
	v.followHighlighter()
	if !ok {
		// Too far behind the buffer's log to follow its edits: the
		// changes of the lines between the text it had and the text.
		cs := changesBetween(old, text)
		v.heights.setLines(text.LineCount())
		v.heights.wraps.reset(text.LineCount(), v.wrap)
		v.wrapAt = 0
		for _, c := range cs {
			v.measured.splice(c.newLine, c.oldEndLine-c.oldLine+1, c.newEndLine-c.newLine+1)
		}
		v.measured.trim(text.LineCount())
		for i := range v.inlays {
			v.inlays[i].at = mapOffset(v.inlays[i].at, cs, biasRight)
		}
		v.moveRepls(cs)
		v.moveBlocks(cs)
		v.settleBlocks()
		v.moveLineStyles(cs)
		v.settleLineStyles()
		if moved != nil {
			moved(cs)
		}
		return false
	}
	for _, cs := range changes {
		for _, c := range cs {
			// Lines from the change's first are measured anew.
			v.heights.wraps.splice(c.newLine, c.oldEndLine-c.oldLine+1, c.newEndLine-c.newLine+1)
			v.measured.splice(c.newLine, c.oldEndLine-c.oldLine+1, c.newEndLine-c.newLine+1)
			v.wrapAt = min(v.wrapAt, c.newLine)
		}
		for i := range v.inlays {
			v.inlays[i].at = mapOffset(v.inlays[i].at, cs, biasRight)
		}
		v.moveRepls(cs)
		v.moveBlocks(cs)
		v.moveLineStyles(cs)
		if moved != nil {
			moved(cs)
		}
	}
	v.settleBlocks()
	v.settleLineStyles()
	v.measured.trim(text.LineCount())
	v.heights.setLines(text.LineCount())
	v.heights.dirty = true
	return true
}

// advanceSyntax has the buffer's highlighter work toward the tokens of
// the lines up to upTo, for at most budget, and reports whether they are
// ready.
func (v *view) advanceSyntax(upTo int, budget time.Duration) bool {
	if v.buf == nil {
		return true
	}
	if hl := v.followHighlighter(); hl != nil {
		return hl.Highlight(upTo, budget)
	}
	return true
}

// followHighlighter has the buffer's highlighter follow its edits, and
// returns it, nil for plain text.
func (v *view) followHighlighter() Highlighter {
	hl := v.buf.highlighter()
	if hl != v.hl {
		// Another language: the lines laid out with the old one's tokens
		// go, and what was found of its brackets.
		v.hl = hl
		v.lines.m = nil
		v.hlGen++
		if v.styled {
			// Its tokens may show in other faces, of other widths.
			v.measured.reset()
		}
	}
	return hl
}

// line returns line n laid out.
func (v *view) line(n int) *lineLayout {
	// A styled line, as its style's view lays it out.
	if s := v.lineStyle(n); s != nil {
		child := v.styledView(s.LineStyle)
		l := child.line(n)
		if child.approx {
			// Its tokens are a guess: frames go on until they are known.
			v.approx, child.approx = true, false
		}
		v.heights.wraps.set(n, len(l.rows))
		v.measured.set(n, l.width)
		return l
	}
	if v.lines.m == nil {
		v.lines.m = map[int]*lineLayout{}
	}
	if l, ok := v.lines.m[n]; ok {
		l.used = v.lines.frame
		if !l.exact {
			// A guess: the highlighter may know better now. The layout
			// depends on the tokens only through their faces.
			l.toks, l.exact = v.tokens(n, l.text, l.toks[:0])
			if v.styled {
				v.faceScratch = v.facesOf(l, v.faceScratch[:0])
				if !slices.Equal(v.faceScratch, l.faces) {
					delete(v.lines.m, n)
					v.measured.forget(n, n)
					return v.line(n)
				}
			}
		}
		return l
	}
	l := &lineLayout{n: n, used: v.lines.frame, owner: v, rowHeight: v.lineHeight, ascent: v.ascent, descent: v.descent}
	l.start = v.text.LineStart(n)
	l.text = v.text.Slice(l.start, v.text.LineEnd(n))
	l.toks, l.exact = v.tokens(n, l.text, nil)
	v.display(l)
	v.ellipsizeTrailing(l)
	if len(l.reps) > 0 {
		// The view's own, which the next replacements are kept in.
		l.reps = slices.Clone(l.reps)
	}
	if v.styled || len(l.reps) > 0 {
		v.faceScratch = v.facesOf(l, v.faceScratch[:0])
		if len(v.faceScratch) > 0 {
			l.faces = slices.Clone(v.faceScratch)
		}
	}
	v.layoutRows(l)
	v.lines.m[n] = l
	v.measured.set(n, l.width)
	return l
}

// lineWidths keeps the widths of the widest lines laid out, by line, which
// the content scrolls sideways through: an edit forgets those of the lines
// it changes and moves the others, so that a line out of view keeps its
// width while its text and what shows in it are the same, and laying out
// the lines in view after an edit tells how wide the content is. It keeps
// only the widest few, so that following an edit costs as much in a long
// text as in a short one: a narrower line is known again as it is laid
// out (#2043).
type lineWidths struct {
	e []lineWidth // at most maxLineWidths, in no order
}

type lineWidth struct {
	line  int
	width float32
}

// maxLineWidths is how many lines lineWidths keeps the widths of.
const maxLineWidths = 64

// set notes that line is w wide. A line laid out again is as wide as it
// was until forgotten, as a piece of it shaped as it showed may have made
// it wider.
func (lw *lineWidths) set(line int, w float32) {
	least := -1
	for i := range lw.e {
		if e := &lw.e[i]; e.line == line {
			e.width = max(e.width, w)
			return
		}
		if least < 0 || lw.e[i].width < lw.e[least].width {
			least = i
		}
	}
	switch {
	case w <= 0:
	case len(lw.e) < maxLineWidths:
		lw.e = append(lw.e, lineWidth{line, w})
	case w > lw.e[least].width:
		lw.e[least] = lineWidth{line, w}
	}
}

// forget forgets the widths of the lines from to to, inclusive, as what
// shows in them changed.
func (lw *lineWidths) forget(from, to int) {
	lw.e = slices.DeleteFunc(lw.e, func(e lineWidth) bool { return e.line >= from && e.line <= to })
}

// splice follows a change of lines, as wrapRows.splice: the lines from at
// to at+removed replaced by added ones, not measured.
func (lw *lineWidths) splice(at, removed, added int) {
	lw.forget(at, at+removed-1)
	for i := range lw.e {
		if lw.e[i].line >= at+removed {
			lw.e[i].line += added - removed
		}
	}
}

// trim forgets the lines from n on, past the end of a text of n lines.
func (lw *lineWidths) trim(n int) { lw.forget(n, math.MaxInt) }

// reset forgets the widths of all lines, as they all change.
func (lw *lineWidths) reset() { lw.e = lw.e[:0] }

// widest returns the width of the widest line known.
func (lw *lineWidths) widest() float32 {
	w := float32(0)
	for _, e := range lw.e {
		w = max(w, e.width)
	}
	return w
}

// endFrame forgets the lines and layouts no recent frame used.
func (v *view) endFrame() {
	for _, child := range v.styleViews {
		child.endFrame()
	}
	v.lines.frame++
	if len(v.lines.m) > 512 {
		for k, l := range v.lines.m {
			if v.lines.frame-l.used > 2 {
				delete(v.lines.m, k)
			}
		}
	}
	v.shaped.endFrame()
}

// tokens returns the tokens of line n, whose text is s, and whether they
// are final.
func (v *view) tokens(n int, s string, dst []Token) ([]Token, bool) {
	if v.hl == nil {
		return dst[:0], true
	}
	toks, ok := v.hl.Tokens(n, s, dst[:0])
	if !ok {
		v.approx = true
	}
	return toks, ok
}

// facesOf appends to dst the faces of l's tokens, by displayed bytes,
// with inlays regular, the text replacements show in the face of what
// they replace and the elements they show as objects, and returns none
// when all are regular.
func (v *view) facesOf(l *lineLayout, dst []faceRun) []faceRun {
	dst = dst[:0]
	styled := false
	if v.styled {
		for _, t := range l.toks {
			f := v.faces[t.Kind]
			styled = styled || f != 0
			e := l.toDisp(t.End, false)
			if n := len(dst); n > 0 && (dst[n-1].face == f || dst[n-1].end >= e) {
				dst[n-1].end = max(dst[n-1].end, e)
				continue
			}
			dst = append(dst, faceRun{end: e, face: f})
		}
	}
	if !styled {
		dst = dst[:0]
	}
	objects := false
	for i, sg := range l.segs {
		n := segLen(l, i)
		switch {
		case n == 0:
		case sg.kind == segInlay && styled:
			dst = setRun(dst, sg.disp, sg.disp+n, faceRun{})
		case sg.kind == segReplace && l.reps[sg.rep].Content != nil:
			dst = setRun(dst, sg.disp, sg.disp+n, faceRun{obj: v.objectWidth(&l.reps[sg.rep])})
			objects = true
		case sg.kind == segReplace && styled:
			dst = setRun(dst, sg.disp, sg.disp+n, faceRun{face: v.faces[kindAt(l.toks, sg.from)]})
		}
	}
	if !styled && !objects {
		return dst[:0]
	}
	return dst
}

// setRun makes displayed bytes a to b of runs f, bytes past the last run
// being regular.
func setRun(runs []faceRun, a, b int, f faceRun) []faceRun {
	if a >= b {
		return runs
	}
	out := make([]faceRun, 0, len(runs)+3)
	add := func(e int, r faceRun) {
		r.end = e
		n := len(out)
		switch {
		case n > 0 && out[n-1].end >= e:
		case n > 0 && out[n-1].face == r.face && out[n-1].obj == r.obj:
			out[n-1].end = e
		default:
			out = append(out, r)
		}
	}
	prev := 0
	for _, r := range runs {
		if prev < a {
			add(min(r.end, a), r)
		}
		if r.end > a && prev < b {
			add(min(r.end, b), f)
		}
		if r.end > b {
			add(r.end, r)
		}
		prev = r.end
	}
	if prev < b {
		if prev < a {
			add(a, faceRun{})
		}
		add(b, f)
	}
	return out
}

// spansOf returns the spans of the faces of displayed bytes a to b of
// l, for text.Params, "" when they are all regular.
func (v *view) spansOf(l *lineLayout, a, b int) string {
	return v.spansIn(l.disp, l.faces, a, b)
}

// spansIn returns the spans of the faces of bytes a to b of the displayed
// text s, whose faces are faces.
func (v *view) spansIn(s string, faces []faceRun, a, b int) string {
	if len(faces) == 0 {
		return ""
	}
	spans := v.spanScratch[:0]
	styled := false
	runes, prev := 0, 0
	for _, r := range faces {
		from, to := max(prev, a), min(r.end, b)
		prev = r.end
		if from >= to {
			continue
		}
		runes += utf8.RuneCountInString(s[from:to])
		sp := text.Span{End: runes, Italic: r.face&faceItalic != 0}
		if r.face&faceBold != 0 {
			sp.Weight = v.boldWeight()
		}
		if r.obj > 0 {
			// Standing on the baseline as the font does, so that the row's
			// metrics stay the font's.
			sp.ObjectWidth, sp.ObjectHeight, sp.ObjectDescent = r.obj, v.ascent, v.descent
		}
		spans = append(spans, sp)
		styled = styled || r.face != 0 || r.obj > 0
	}
	v.spanScratch = spans
	if !styled {
		return ""
	}
	return text.EncodeSpans(spans)
}

// display makes the displayed text of l: tabs expanded to the next tab
// stop, by columns counted in characters, inlays inserted, and the
// ranges of replacements showing what they show in their place.
func (v *view) display(l *lineLayout) {
	s := l.text
	inl := v.inlaysIn(l.start, l.start+len(s))
	l.reps = v.replsIn(l.start, l.start+len(s))
	l.segs = l.segs[:0]
	if strings.IndexByte(s, '\t') < 0 && len(inl) == 0 && len(l.reps) == 0 {
		l.disp, l.segs = s, nil
		return
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	col := 0
	textFrom := 0
	ii, ri := 0, 0
	flush := func(to int) {
		if to > textFrom {
			l.segs = append(l.segs, dispSeg{disp: b.Len(), from: textFrom, to: to, kind: segText})
			b.WriteString(s[textFrom:to])
		}
		textFrom = to
	}
	emitInlays := func(at int) {
		for ii < len(inl) && inl[ii].at-l.start <= at {
			in := inl[ii]
			if in.at-l.start < at {
				// Inside a replacement, passed over.
				ii++
				continue
			}
			flush(at)
			l.segs = append(l.segs, dispSeg{disp: b.Len(), from: at, to: at, kind: segInlay, inlay: ii, ghost: in.kind == 1 || in.kind == inlayTrailing})
			b.WriteString(in.text)
			col += utf8.RuneCountInString(in.text)
			ii++
		}
	}
	for i := 0; ; {
		emitInlays(i)
		// The replacements from here: what one shows, at its start, and
		// nothing for the rest of one from a line before.
		for ri < len(l.reps) && max(l.reps[ri].Start-l.start, 0) == i {
			rp := &l.reps[ri]
			to := min(rp.End-l.start, len(s))
			flush(i)
			shown := ""
			if rp.Start >= l.start {
				shown = rp.shown()
			}
			l.segs = append(l.segs, dispSeg{disp: b.Len(), from: i, to: to, kind: segReplace, rep: ri})
			b.WriteString(shown)
			col += utf8.RuneCountInString(shown)
			textFrom = to
			ri++
			if to > i {
				i = to
				emitInlays(i)
			}
		}
		if i >= len(s) {
			break
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == '\t' {
			flush(i)
			w := v.tabSize - col%v.tabSize
			if col >= maxTabColumn {
				w = 1
			}
			l.segs = append(l.segs, dispSeg{disp: b.Len(), from: i, to: i + 1, kind: segTab})
			b.WriteString(spaces(w))
			col += w
			textFrom = i + 1
		} else {
			col++
		}
		i += size
	}
	flush(len(s))
	l.disp = b.String()
}

// maxTabColumn is where tabs stop expanding, which bounds the work
// of a line, as Zed's MAX_EXPANSION_COLUMN.
const maxTabColumn = 256

func spaces(n int) string {
	const s = "                "
	if n <= len(s) {
		return s[:n]
	}
	return strings.Repeat(" ", n)
}

func (v *view) inlaysIn(a, b int) []inlay {
	i := sort.Search(len(v.inlays), func(i int) bool { return v.inlays[i].at >= a })
	j := i
	for j < len(v.inlays) && v.inlays[j].at <= b {
		j++
	}
	return v.inlays[i:j]
}

// toDisp maps a byte of the line to its displayed byte; after places an
// offset at an inlay after it, and one inside a replacement after what it
// shows, else before.
func (l *lineLayout) toDisp(col int, after bool) int {
	if l.segs == nil {
		return col
	}
	out := len(l.disp)
	for i := len(l.segs) - 1; i >= 0; i-- {
		s := l.segs[i]
		switch s.kind {
		case segText:
			if col >= s.from && col <= s.to {
				return s.disp + col - s.from
			}
		case segTab:
			if col == s.from {
				return s.disp
			}
			if col == s.to {
				return s.disp + segLen(l, i)
			}
		case segReplace:
			if col == s.to && s.to > s.from || col > s.from && col < s.to && after {
				return s.disp + segLen(l, i)
			}
			if col >= s.from && col < s.to {
				return s.disp
			}
		case segInlay:
			if col == s.from {
				if after && !s.ghost {
					return s.disp + segLen(l, i)
				}
				out = s.disp
				continue
			}
		}
		if s.from < col {
			break
		}
	}
	return out
}

func segLen(l *lineLayout, i int) int {
	if i+1 < len(l.segs) {
		return l.segs[i+1].disp - l.segs[i].disp
	}
	return len(l.disp) - l.segs[i].disp
}

// fromDisp maps a displayed byte to the byte of the line, the nearest
// edge for a byte inside a tab or an inlay.
func (l *lineLayout) fromDisp(d int) int {
	if l.segs == nil {
		return min(d, len(l.text))
	}
	i := sort.Search(len(l.segs), func(i int) bool { return l.segs[i].disp > d }) - 1
	if i < 0 {
		return 0
	}
	s := l.segs[i]
	switch s.kind {
	case segText:
		return min(s.from+d-s.disp, s.to)
	case segTab:
		if d-s.disp > segLen(l, i)/2 {
			return s.to
		}
		return s.from
	case segReplace:
		// The nearer end of what it shows; the start of what shows
		// nothing, at the end of the line, as text follows it otherwise.
		if n := segLen(l, i); n > 0 && d-s.disp > n/2 {
			return s.to
		}
		return s.from
	}
	return s.from
}

// ellipsizeTrailing shortens marked text at the end of a line to the room
// left on the source text's last display row. Only the layout is shortened;
// the inlay and source buffer retain their complete text.
func (v *view) ellipsizeTrailing(l *lineLayout) {
	if !v.wrap || v.wrapWidth <= 0 || len(l.segs) == 0 {
		return
	}
	segment := l.segs[len(l.segs)-1]
	if segment.kind != segInlay {
		return
	}
	inlays := v.inlaysIn(l.start, l.start+len(l.text))
	if segment.inlay >= len(inlays) {
		return
	}
	inlay := inlays[segment.inlay]
	if inlay.kind != inlayTrailing || inlay.ellipsis == "" {
		return
	}
	displayed := l.disp
	prefix, trailing := displayed[:segment.disp], displayed[segment.disp:]
	// Replacements can be inline objects with an application-provided width.
	// Measure their real faces before the normal face pass below shortens l.
	faces := v.facesOf(l, nil)
	breaks := v.wrapBreaks(prefix, faces, len(prefix) <= fitBytes)
	start, indent := 0, float32(0)
	if len(breaks) > 0 {
		start = breaks[len(breaks)-1]
		indent = v.wrapIndent(prefix)
	}
	used := indent + v.runsWidth(displayed, faces, start, segment.disp)
	trailingWidth := func(end int) float32 {
		return v.runsWidth(displayed, faces, segment.disp, segment.disp+end)
	}
	if used+trailingWidth(len(trailing)) <= v.wrapWidth {
		return
	}
	ellipsisWidth := v.shaped.get(inlay.ellipsis, v.style).Width
	if ellipsisWidth > v.wrapWidth-used {
		l.disp = prefix
		l.segs = l.segs[:len(l.segs)-1]
		return
	}
	cut := 0
	for index := range trailing {
		if index > 0 && used+trailingWidth(index)+ellipsisWidth <= v.wrapWidth {
			cut = index
		}
	}
	l.disp = prefix + trailing[:cut] + inlay.ellipsis
}

// replaced reports whether byte col of the line is in a replacement,
// which shows in its place.
func (l *lineLayout) replaced(col int) bool {
	if len(l.reps) == 0 {
		return false
	}
	for _, s := range l.segs {
		if s.kind == segReplace && col >= s.from && col < s.to {
			return true
		}
	}
	return false
}

// layoutRows wraps the displayed text of l into rows and shapes them.
func (v *view) layoutRows(l *lineLayout) {
	l.rows = l.rows[:0]
	fit := len(l.disp) <= fitBytes
	breaks := v.wrapBreaks(l.disp, l.faces, fit)
	indent := float32(0)
	if len(breaks) > 0 {
		indent = v.wrapIndent(l.disp)
	}
	from := 0
	for i := 0; i <= len(breaks); i++ {
		to := len(l.disp)
		if i < len(breaks) {
			to = breaks[i]
		}
		row := rowLayout{from: from, to: to}
		if i > 0 {
			row.indent = indent
		}
		// The rows of a wrapped line are shaped as they show, not all
		// of a long line's at once, but those wrapBreaks shaped to fit.
		v.shapeRow(l, &row, len(breaks) > 0, fit)
		l.rows = append(l.rows, row)
		l.width = max(l.width, row.indent+row.width)
		from = to
	}
	if v.wrap {
		v.heights.wraps.set(l.n, len(l.rows))
	}
}

// shapeRow shapes a row of l, in pieces of at most pieceBytes, placed by
// their widths: only the first piece of the row is shaped at once when
// the row is long, the others as they show (piece). A row of a wrapped
// line is shaped as it shows too (lazy), unless it was to fit (fitted).
func (v *view) shapeRow(l *lineLayout, row *rowLayout, lazy, fitted bool) {
	s := l.disp
	text := s[row.from:row.to]
	if lazy && fitted && !v.summed(s, l.faces, row.from, row.to) {
		lazy = false
	}
	if len(text) <= pieceBytes && lazy {
		w := v.runsWidth(s, l.faces, row.from, row.to)
		row.pieces = append(row.pieces[:0], rowPiece{from: row.from, to: row.to, ascii: isASCII(text), width: w})
		row.width = w
		return
	}
	if len(text) <= pieceBytes {
		lay := v.shaped.getSpans(text, v.spansOf(l, row.from, row.to), v.style)
		row.pieces = append(row.pieces[:0], rowPiece{from: row.from, to: row.to, l: lay, ascii: isASCII(text), width: lay.Width})
		row.width = lay.Width
		return
	}
	row.pieces = row.pieces[:0]
	x := float32(0)
	for a := row.from; a < row.to; {
		b := min(a+pieceBytes, row.to)
		for b < row.to && !utf8.RuneStart(s[b]) {
			b++
		}
		w := v.runsWidth(s, l.faces, a, b)
		row.pieces = append(row.pieces, rowPiece{from: a, to: b, x: x, ascii: isASCII(s[a:b]), width: w})
		x += w
		a = b
	}
	row.width = x
}

// piece returns the layout of a row's piece, shaping it the first time.
func (v *view) piece(l *lineLayout, p *rowPiece) *text.Layout {
	if l.owner != nil && l.owner != v {
		return l.owner.piece(l, p)
	}
	if p.l == nil {
		p.l = v.shaped.getSpans(l.disp[p.from:p.to], v.spansOf(l, p.from, p.to), v.style)
		// The text engine may set a piece's characters farther apart than
		// their advances, which placed the pieces, add up to, as Core Text
		// does between Han and Latin letters: an unwrapped line is as wide
		// as its last piece ends, where a caret at its end shows.
		if w := p.x + p.l.Width; !v.wrap && p.to == len(l.disp) && w > l.width {
			l.width = w
			v.measured.set(l.n, w)
		}
	}
	return p.l
}

// wrapBreaks returns where the displayed text s breaks into rows when
// wrapping, as Zed's LineWrapper: summing the advances of characters,
// breaking after whitespace or between a word and what is not, anywhere
// in CJK but before closing punctuation and after opening punctuation,
// and inside a word too long for a row. faces are those of s, if known.
//
// With fit, a row is then shaped as it is painted, and ends at a place
// to break before while it is wider than the wrap width: the room the
// text engine leaves between characters in context, as Core Text does
// between Han and Latin letters and around CJK punctuation in Chinese
// and Japanese text, is in no character shaped alone. Laying a line out
// fits its rows, which it shapes to paint them anyway; measuring the
// wraps of lines out of view only sums, and may count too few rows until
// they show.
func (v *view) wrapBreaks(s string, faces []faceRun, fit bool) []int {
	if !v.wrap || v.wrapWidth <= 0 {
		return nil
	}
	if len(faces) == 0 && v.widths.mono > 0 && float32(len(s))*v.widths.mono <= v.wrapWidth && (!fit || isASCII(s)) {
		return nil
	}
	var breaks []int
	indent := float32(0)
	for start := 0; ; {
		end := v.rowEnd(s, faces, start, indent)
		if fit {
			end = v.fitRow(s, faces, start, end, indent)
		}
		if end >= len(s) {
			return breaks
		}
		if len(breaks) == 0 {
			indent = v.wrapIndent(s)
		}
		breaks = append(breaks, end)
		start = end
	}
}

// rowEnd returns where the row of s from start ends, x into it after
// its indent, by the advances of its characters: at the last place to
// break before the first character past the wrap width, else before that
// character, or at the end of s.
func (v *view) rowEnd(s string, faces []faceRun, start int, x float32) int {
	fi := sort.Search(len(faces), func(k int) bool { return faces[k].end > start })
	cw := &v.widths
	var obj float32
	last := -1 // the last place to break
	var prev rune
	for i, r := range s[start:] {
		i += start
		if i > start && canBreak(prev, r) {
			last = i
		}
		if len(faces) > 0 {
			for fi < len(faces) && faces[fi].end <= i {
				fi++
			}
			cw, obj = &v.widths, 0
			if fi < len(faces) {
				cw, obj = v.widthsOf(faces[fi].face), faces[fi].obj
			}
		}
		w := cw.of(r)
		if obj > 0 {
			w = obj
		}
		if x+w > v.wrapWidth && i > start {
			if last > start {
				return last
			}
			return i
		}
		x += w
		prev = r
	}
	return len(s)
}

// fitBytes is the longest line whose rows are shaped to fit as it is laid
// out: the rows of longer ones are shaped only as they show.
const fitBytes = 16 << 10

// fitRow returns where the row of s from start to end ends so that,
// shaped as it is painted, it fits in the wrap width after indent: end,
// else the last place to break before, else a character before. Rows of
// monospaced ASCII are as wide as their advances sum to.
func (v *view) fitRow(s string, faces []faceRun, start, end int, indent float32) int {
	for end-start <= pieceBytes && !v.summed(s, faces, start, end) {
		l := v.shaped.getSpans(s[start:end], v.spansIn(s, faces, start, end), v.style)
		if indent+l.Width <= v.wrapWidth {
			break
		}
		at := lastBreak(s, start, end)
		if at <= start {
			_, n := utf8.DecodeLastRuneInString(s[start:end])
			if end-n <= start {
				break
			}
			at = end - n
		}
		end = at
	}
	return end
}

// summed tells whether bytes a to b of s, whose faces are faces, are as
// wide as their characters' advances sum to: monospaced ASCII, whose
// shaping needs no context.
func (v *view) summed(s string, faces []faceRun, a, b int) bool {
	if v.widths.mono <= 0 || !isASCII(s[a:b]) {
		return false
	}
	prev := 0
	for _, r := range faces {
		if r.end > a && prev < b && (r.obj > 0 || r.face != 0 && v.widthsOf(r.face).mono <= 0) {
			return false
		}
		prev = r.end
	}
	return true
}

// lastBreak returns the last place to break in s between start and end,
// or -1.
func lastBreak(s string, start, end int) int {
	last := -1
	var prev rune
	for i, r := range s[start:end] {
		if i > 0 && canBreak(prev, r) {
			last = start + i
		}
		prev = r
	}
	return last
}

// wrapIndent is how far rows after the first are indented: as far as the
// line, but no more than half the wrap width.
func (v *view) wrapIndent(s string) float32 {
	n := len(s) - len(strings.TrimLeft(s, " "))
	w := v.widths.width(s[:n])
	return min(w, v.wrapWidth/2)
}

func canBreak(prev, r rune) bool {
	// Kinsoku: closing punctuation does not start a row, nor opening
	// punctuation end one.
	if closing(r) || opening(prev) {
		return false
	}
	if prev == ' ' || prev == '\t' {
		return r != ' ' && r != '\t'
	}
	if isCJK(r) || isCJK(prev) {
		// Nor ASCII punctuation, next to CJK.
		return !(r < utf8.RuneSelf && strings.ContainsRune(".,;:!?)]}", r)) && !(prev < utf8.RuneSelf && strings.ContainsRune("([{", prev))
	}
	return isWordRune(prev) != isWordRune(r) && r != ' ' && r != '\t' && !strings.ContainsRune(".,;:)]}>'\"", r)
}

// closing tells whether r is punctuation that does not start a row:
// CJK commas, stops and closing brackets, the right quotation marks,
// ellipses and middle dots, as JIS X 4051 and the Chinese rules of
// line breaking have them.
func closing(r rune) bool {
	if !maybePunct(r) {
		return false
	}
	switch r {
	case '、', '。', '，', '．', '：', '；', '？', '！', '）', '］', '｝', '」', '』', '】', '〕', '〉', '》', '〗', '〙', '〛', '｠', '〞', '〟',
		'’', '”', '・', '･', '…', '‥', '｡', '､', '｣':
		return true
	}
	return false
}

// maybePunct tells whether r is in the blocks of the punctuation that
// closing and opening know, which Han characters and ASCII are not.
func maybePunct(r rune) bool {
	return r >= '‘' && r <= '…' || r >= '、' && r <= '〟' || r == '・' || r >= '！' && r <= '･'
}

// opening tells whether r is punctuation that does not end a row: CJK
// opening brackets and the left quotation marks.
func opening(r rune) bool {
	if !maybePunct(r) {
		return false
	}
	switch r {
	case '（', '［', '｛', '「', '『', '【', '〔', '〈', '《', '〖', '〘', '〚', '｟', '〝', '‘', '“', '｢':
		return true
	}
	return false
}

func isCJK(r rune) bool {
	return r >= 0x2e80 && r <= 0x9fff || r >= 0xac00 && r <= 0xd7af || r >= 0xf900 && r <= 0xfaff || r >= 0xff00 && r <= 0xffef
}

// measureWraps measures the wraps of lines not measured yet, from those
// around near, for at most budget, and reports whether all are measured.
// Lines hidden, as the unchanged ones a diff folds away, show no rows,
// measured or not: they are left until they show, which a long file's
// diff, measured anew at each width a window is resized through, would
// else spend most of its frames on.
func (v *view) measureWraps(near int, budget time.Duration) bool {
	w := &v.heights.wraps
	if !w.on {
		return true
	}
	n := v.text.LineCount()
	hidden := v.heights.hidden
	if !slices.Equal(v.wrapHidden, hidden) {
		v.wrapHidden = append(v.wrapHidden[:0], hidden...)
		v.wrapAt = 0
	}
	if w.known >= n || v.wrapAt >= n {
		return true
	}
	start := time.Now()
	measure := func(i, at int, s string) bool {
		if w.extra[i] != wrapUnknown {
			return true
		}
		if v.lineStyle(i) != nil {
			v.line(i)
			return true
		}
		disp := s
		var faces []faceRun
		if strings.IndexByte(s, '\t') >= 0 || len(v.inlaysIn(at, at+len(s))) > 0 || len(v.replsIn(at, at+len(s))) > 0 {
			l := &lineLayout{n: i, start: at, text: s}
			v.display(l)
			disp = l.disp
			if len(l.reps) > 0 {
				// The widths of the elements replacements show.
				faces = v.facesOf(l, nil)
			}
		}
		w.set(i, len(v.wrapBreaks(disp, faces, false))+1)
		return true
	}
	// Those around near first, so what shows is right at once.
	a := v.text.LineStart(near)
	lo := max(near-200, v.text.LineOf(a-64<<10))
	hi := min(near+200, v.text.LineOf(a+64<<10)+1)
	v.text.EachLine(lo, hi, func(i, at int, s string) bool {
		if !v.heights.isHidden(i) {
			measure(i, at, s)
		}
		return true
	})
	count, bytes := 0, 0
	for v.wrapAt < n {
		from := v.wrapAt
		// Past the hidden lines from here, up to the next.
		k := sort.Search(len(hidden), func(k int) bool { return hidden[k].to >= from })
		if k < len(hidden) && hidden[k].from <= from {
			v.wrapAt = hidden[k].to + 1
			continue
		}
		to := n
		if k < len(hidden) {
			to = min(n, hidden[k].from)
		}
		stopped := false
		v.text.EachLine(from, to, func(i, at int, s string) bool {
			measure(i, at, s)
			v.wrapAt = i + 1
			count++
			bytes += len(s)
			if count&255 != 0 && bytes < 64<<10 {
				return true
			}
			bytes = 0
			stopped = time.Since(start) > budget
			return !stopped
		})
		if stopped {
			break
		}
		v.wrapAt = max(v.wrapAt, to)
	}
	return v.wrapAt >= n
}

// rowOf returns the row of l holding displayed byte d, the row it ends
// rather than the next for a byte at a wrap when upstream.
func (l *lineLayout) rowOf(d int, upstream bool) int {
	for i := range l.rows {
		r := &l.rows[i]
		if d < r.to || i == len(l.rows)-1 || d == r.to && upstream {
			return i
		}
	}
	return 0
}

// xOf returns the x of displayed byte d in row r, from the row's start.
func (v *view) xOf(l *lineLayout, r *rowLayout, d int) float32 {
	d = max(r.from, min(d, r.to))
	for i := range r.pieces {
		p := &r.pieces[i]
		if d < p.to || i == len(r.pieces)-1 {
			lay := v.piece(l, p)
			idx := d - p.from
			if !p.ascii {
				idx = utf8.RuneCountInString(l.disp[p.from:d])
			}
			x, _, _ := lay.Caret(idx)
			return r.indent + p.x + x
		}
	}
	return r.indent
}

// dispAt returns the displayed byte in row r nearest to x, from the row's
// start.
func (v *view) dispAt(l *lineLayout, r *rowLayout, x float32) int {
	x -= r.indent
	d := r.from
	for i := range r.pieces {
		p := &r.pieces[i]
		if x < p.x+p.width || i == len(r.pieces)-1 {
			lay := v.piece(l, p)
			idx := lay.IndexAt(x-p.x, 0)
			if p.ascii {
				d = min(p.from+idx, p.to)
			} else {
				d = p.from + byteOfRune(l.disp[p.from:p.to], idx)
			}
			break
		}
	}
	// The end of a row a line wraps after is the start of the next, where
	// a caret shows: on this row, it is before the row's last character,
	// as Zed clips there, before the marks of that character too.
	if d >= r.to && r.to < len(l.disp) && r.to > r.from {
		d = r.to
		for d > r.from {
			c, size := utf8.DecodeLastRuneInString(l.disp[r.from:d])
			d -= size
			if !extends(c) {
				break
			}
		}
	}
	return d
}

// byteOfRune returns the byte of s rune i starts at.
func byteOfRune(s string, i int) int {
	for at := range s {
		if i == 0 {
			return at
		}
		i--
	}
	return len(s)
}

// runColors colors a row's glyphs: the color of each run of the
// displayed text.
type runColors struct {
	ends   []int // displayed bytes each run ends at
	colors []ui.Color
}

// colorsOf makes the colors of l's tokens, by displayed bytes, with
// inlays muted, and the text replacements show in their Color, else that
// of the token they replace.
func (v *view) colorsOf(l *lineLayout, out *runColors) {
	out.ends, out.colors = out.ends[:0], out.colors[:0]
	for _, t := range l.toks {
		out.ends = append(out.ends, l.toDisp(t.End, false))
		out.colors = append(out.colors, v.theme.TokenColor(t.Kind))
	}
	for i, sg := range l.segs {
		switch sg.kind {
		case segInlay:
			out.overlay(sg.disp, sg.disp+segLen(l, i), v.theme.LineNumber)
		case segReplace:
			if n := segLen(l, i); n > 0 && l.reps[sg.rep].Content == nil {
				c := l.reps[sg.rep].Color
				if c.A == 0 {
					c = v.theme.TokenColor(kindAt(l.toks, sg.from))
				}
				out.overlay(sg.disp, sg.disp+n, c)
			}
		}
	}
}

// overlay colors displayed bytes a to b c.
func (cc *runColors) overlay(a, b int, c ui.Color) {
	if a >= b {
		return
	}
	var ends []int
	var colors []ui.Color
	add := func(e int, c ui.Color) {
		if n := len(ends); n > 0 && ends[n-1] >= e {
			return
		}
		ends, colors = append(ends, e), append(colors, c)
	}
	prev := 0
	for i, e := range cc.ends {
		col := cc.colors[i]
		if e <= a || prev >= b {
			add(e, col)
		} else {
			if prev < a {
				add(a, col)
			}
			add(min(e, b), c)
			if e > b {
				add(e, col)
			}
		}
		prev = e
	}
	if prev < b {
		if prev < a {
			add(a, ui.Color{})
		}
		add(b, c)
	}
	cc.ends, cc.colors = ends, colors
}

func (cc *runColors) at(d int, def ui.Color) ui.Color {
	i := sort.SearchInts(cc.ends, d+1)
	if i < len(cc.colors) && cc.colors[i].A > 0 {
		return cc.colors[i]
	}
	return def
}

// paintRow paints the glyphs of row r of l, its start at x and its top at
// y, colored by cc, or by override when given.
func (v *view) paintRow(p *ui.Painter, l *lineLayout, r *rowLayout, x, y float32, cc *runColors, override func(d int, c ui.Color) ui.Color) {
	clip := p.Visible()
	// A styled line's rows have its style's metrics; rows made to paint
	// alone, as the gutter's numbers, the view's.
	rowHeight, ascent, descent := l.rowHeight, l.ascent, l.descent
	if rowHeight <= 0 {
		rowHeight, ascent, descent = v.lineHeight, v.ascent, v.descent
	}
	baseline := y + (rowHeight-(ascent+descent))/2 + ascent
	def := v.theme.Text
	for pi := range r.pieces {
		pc := &r.pieces[pi]
		px := x + r.indent + pc.x
		if px+pc.width < clip.X-64 || px > clip.X+clip.W+64 {
			continue
		}
		lay := v.piece(l, pc)
		var bytesOf []int
		if !pc.ascii {
			bytesOf = runeBytes(l.disp[pc.from:pc.to])
		}
		p.TextLine(lay, px, baseline, func(cluster int) ui.Color {
			d := pc.from + cluster
			if bytesOf != nil && cluster < len(bytesOf) {
				d = pc.from + bytesOf[cluster]
			}
			c := cc.at(d, def)
			if override != nil {
				c = override(d, c)
			}
			return c
		})
	}
}

// runeBytes returns the byte each rune of s starts at.
func runeBytes(s string) []int {
	out := make([]int, 0, len(s))
	for i := range s {
		out = append(out, i)
	}
	return out
}

// rangeRects returns the rectangles, relative to row r's start, covering
// displayed bytes a to b of the row.
func (v *view) rangeRects(l *lineLayout, r *rowLayout, a, b int, out []ui.Rect) []ui.Rect {
	a, b = max(a, r.from), min(b, r.to)
	if a > b {
		return out
	}
	if a == b {
		return out
	}
	for pi := range r.pieces {
		pc := &r.pieces[pi]
		pa, pb := max(a, pc.from), min(b, pc.to)
		if pa >= pb {
			continue
		}
		lay := v.piece(l, pc)
		ra, rb := pa-pc.from, pb-pc.from
		if !pc.ascii {
			ra = utf8.RuneCountInString(l.disp[pc.from:pa])
			rb = ra + utf8.RuneCountInString(l.disp[pa:pb])
		}
		for _, rc := range lay.SelectionVisual(ra, rb, false) {
			out = append(out, ui.Rect{X: r.indent + pc.x + rc.X, Y: 0, W: rc.W, H: l.rowHeight})
		}
	}
	return out
}

// mergeRects joins rectangles that touch, which selections of runs
// return in pieces.
func mergeRects(rs []ui.Rect) []ui.Rect {
	if len(rs) < 2 {
		return rs
	}
	slices.SortFunc(rs, func(a, b ui.Rect) int {
		if a.X < b.X {
			return -1
		}
		if a.X > b.X {
			return 1
		}
		return 0
	})
	out := rs[:1]
	for _, r := range rs[1:] {
		last := &out[len(out)-1]
		if r.X <= last.X+last.W+0.5 {
			last.W = max(last.W, r.X+r.W-last.X)
			continue
		}
		out = append(out, r)
	}
	return out
}

// styleOf returns the text system's style of a font.
func styleOf(f ui.Font) text.Style {
	return text.Style{Family: f.Family, Size: f.Size, Weight: f.Weight, Italic: f.Italic, Features: f.Features}
}

func isWordRune(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }
