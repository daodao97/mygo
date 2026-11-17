package richtext

import (
	"slices"
	"time"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"
	"weak"

	"github.com/egoist/mygo/internal/l10n"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/rope"
	"github.com/egoist/mygo/ui"
)

// EditorOptions configure an Editor.
type EditorOptions struct {
	// OnInput can consume input before normal editing, e.g. a custom hold.
	// Return false to retain the editor's ordinary keyboard and pointer behavior.
	OnInput func(ui.InputEvent) bool

	// Label names the editor to assistive technology; "Rich text" when
	// empty.
	Label string
	// Placeholder shows while the text is empty.
	Placeholder string
	// Font is the text's: its zero fields take the theme's font and size.
	Font ui.Font
	// ReadOnly shows the text without letting the user change it: it is
	// selected and copied, and its links and tasks are clicked.
	ReadOnly bool
	// NoMarkdown turns off the Markdown shortcuts: "# " at the start of a
	// paragraph making a heading, **bold** making bold, and the others.
	NoMarkdown bool
	// OnPaste runs before keyboard or native-menu paste on the UI thread.
	// Returning true consumes the paste without changing the document unless
	// the callback edits it. Return false to use the editor's normal paste.
	// Use Services for clipboard access; do not retain a frame's Element.
	OnPaste func() bool
	// OnCopy runs before keyboard or native-menu copy/cut. Returning true
	// consumes the command. A cut hook owns deleting its selected content.
	OnCopy func(cut bool) bool
	// OnLink opens the URL of a link clicked, instead of the system,
	// which opens only those ui.SafeToOpen allows.
	OnLink func(url string)
	// OnLinkCommand runs for ⌘K (Ctrl+K), as an app asks for the URL of a
	// link in a popover and sets it with Document.SetLink. Without it,
	// ⌘K links a selected URL to itself.
	OnLinkCommand func()
	// TagStyle styles the runs that carry a Tag, as an app shows mentions
	// or comments.
	TagStyle func(tag string, s *ui.Span)
	// Theme sets how the text looks: colors, code, headings, lists and
	// quotations. What it leaves zero, and all of it when nil, takes the
	// window theme's DefaultTheme.
	Theme *Theme
	// Menu builds the context menu in place of the editor's own, as
	// Element.ContextMenu's function does: an app with a menu of its own
	// opens it there, and adds nothing to show none of the system's.
	Menu func(m *ui.Menu)
	// OnKey is asked about a key pressed in the editor before the editor
	// acts on it, as an app's popup over the text (a list of completions)
	// takes the arrows, Enter and Escape: true takes the key, and the
	// editor does nothing with it.
	OnKey func(key ui.Key, mods ui.Modifiers) bool
	// OnCommand is asked about an edit command of the menus ("paste",
	// "copy", "cut", "selectAll", "undo"...) before the editor does it,
	// as OnKey is about a key: a Mac's menu bar takes ⌘V and sends
	// "paste", which OnKey never sees. true takes the command, and the
	// editor does nothing with it.
	OnCommand func(name string) bool
	// Spans is given the spans paragraph p (from 0) of style is drawn as
	// and returns them as they are to be drawn: split, colored or spaced
	// otherwise, their text the same, as an app colors a block of code by
	// its language or spaces the letters. Change SpansVersion when what
	// it does changes, for the paragraphs to be laid out again.
	Spans        func(p int, style ParagraphStyle, spans []ui.Span) []ui.Span
	SpansVersion int
	// Space is given paragraph p (from 0) and returns the room in DIPs to
	// leave above it, more than the theme's, as a list opens a place
	// where an item dragged would go. Less than zero takes room away from
	// the theme's, down to none, as a rule sits closer to the text around
	// it than a line of text would. Above the first paragraph of a block
	// of code, the room is a margin outside the block's background. It is
	// asked as the paragraphs are laid out, each frame for those in view,
	// so that what it returns can change from frame to frame and the
	// paragraphs below move with it. While the editor has the keyboard and
	// is scrolled down its text, room that opens between the top of the
	// view and the caret moves the paragraphs above it up instead, so that
	// the line being typed stays.
	Space func(p int) float32
	// NoScrollbar draws no scroll bar of the editor's own, and a press
	// where it would be goes to the text: an app that makes the editor as
	// tall as its text, and scrolls it with a page around it, shows the
	// page's bar alone, even in a frame where the editor's height falls
	// short of its text and it scrolls a little itself. The editor still
	// scrolls to keep the caret in view, and with the wheel.
	NoScrollbar bool
}

// Editor creates an editor of doc's attributed text, as NSTextView edits
// attributed text: bold, italic, underline, strikethrough, code, fonts,
// sizes and colors, links, headings, lists, tasks, quotations, blocks of
// code, alignment and attachments, with the shortcuts of word processors
// (⌘B, ⌘I, ⌘U, ⌘K, ⇧⌘7, ⇧⌘8, ⌘], …), undo of formats with the text, rich
// copy and paste, and input methods composing in the typing attributes.
// The app keeps doc from frame to frame:
//
//	doc := richtext.NewDocument(richtext.NewText("Hello"))
//	// In the view:
//	if richtext.Editor(c, doc).Fill().Changed() {
//		app.save(doc.Text())
//	}
//
// A toolbar formats the selection through doc (doc.ToggleBold,
// doc.SetHeading, …) and shows its formats with doc.Formatting. Changed
// reports the edits the user made in the editor, not those the app made
// through doc. It lays out and draws only the paragraphs in view, so a
// long document edits as a short one.
func Editor(c *ui.Context, doc *Document, options ...EditorOptions) ui.Element {
	t := c.Theme()
	e := build(c, doc, options, true)
	e.Radius(t.Radius).Background(t.Surface).Border(1, t.Border).MinHeight(t.Space(30))
	if e.Focused() {
		e.Border(1, t.Accent)
	}
	return e
}

// EditorBase creates an Editor without padding, background, border or
// corners, for an editor of the app's own look.
func EditorBase(c *ui.Context, doc *Document, options ...EditorOptions) ui.Element {
	return build(c, doc, options, false)
}

// view is the state of an editor: the text it shows, which follows its
// document, the layouts of its paragraphs, where it is scrolled to, the
// selection and an input method's composition.
type view struct {
	doc  *Document
	opts EditorOptions
	// shown is the text laid out, of the document's version and
	// selVersion.
	shown      Text
	version    uint64
	selVersion uint64
	paras      []paraView
	hs         heights
	laid       int
	look       look
	base       ui.Font
	// width is the content's, which the layouts wrap at; padX and padY
	// the room around the text in the element.
	width, viewH float32
	// laidLook is the look the layouts were made in.
	laidLook   lookKey
	padX, padY float32
	rect       ui.Rect
	// clip is the part of the element the painter's clip showed in the
	// last paint, from its top, while clipped is true: an editor as tall
	// as its text, in a scroll container, shows a window of it.
	clip    [2]float32
	clipped bool
	scroll  float64
	// The view keeps its place by paragraph anchor: its top is anchorOff
	// below the anchor's, or, while anchorText, the view keeps the
	// anchor's first line where it was when its text was at anchorY in
	// the content (the caret's paragraph, while the editor is focused).
	anchor      int
	anchorOff   float64
	anchorText  bool
	anchorY     float64
	reveal      bool
	first, last int
	// The selection, in runes: anchor and caret, and the caret's visual
	// edge.
	selAnchor, caret int
	affinity         ui.TextAffinity
	desiredX         float32
	hasDesired       bool
	// An input method's composition, shown at the caret.
	compose         string
	composeCaret    int
	composeSelected ui.TextInputRange
	composing       bool
	// mode is how the next edit joins the steps of undo, coalesce whether
	// typing goes on.
	mode     editMode
	coalesce bool
	// The pointer: a drag selecting by runes, words or paragraphs, from
	// where it started; a scroll bar's thumb held; a link pressed.
	dragging   bool
	dragUnit   int
	dragStart  [2]int
	pressMods  ui.Modifiers
	pressLink  string
	pressPoint ui.Point
	// cursor is the pointer's shape where it hovers: an arrow over task
	// boxes and the scroll bar, a hand over links, an I-beam over text.
	cursor    ui.Cursor
	thumb     bool
	inView    bool // the pointer is over the editor, which shows the scroll bar
	thumbGrab float32
	// barStyle is how the scroll bar looks, the theme's as of the last
	// paint.
	barStyle   ui.ScrollbarStyle
	focused    bool
	blinkStart time.Time
	services   ui.Services
	// changed is set by an edit of the user, which the next build reports.
	changed bool
	// numbers caches the numbers of the items of numbered lists, for the
	// text of numbersOf.
	numbers   map[int]int
	numbersOf Text
	// placed is room for the boxes of the paragraphs painted, which moved
	// compares with those the app read.
	placed []ParagraphBox
	// read is set as the app reads this editor's paragraphs in view
	// (VisibleParagraphs), which readBoxes holds, and readCaret as it reads
	// its caret (CaretRect), which readCaretRect holds: for its next paint
	// to tell whether it moved them. Under the document's lock.
	read, readCaret bool
	readBoxes       []ParagraphBox
	readCaretRect   ui.Rect
}

type editMode uint8

const (
	newStep editMode = iota
	typingStep
	sameStep
)

// paraView is what an editor made of a paragraph: its layout, for the
// runs, style and composition it was made of, the UTF-16 offset of each of
// its runes, and its height with the room around it, 0 until laid out.
type paraView struct {
	layout *ui.TextLayout
	units  []int
	// marker is the list marker painted beside the paragraph, laid out.
	marker       string
	markerLayout *ui.TextLayout
	id           uint64
	runs         *attrRun
	nruns        int
	style        ParagraphStyle
	compose      string
	at           int
	h            float32
	// space is the space above and below it, in h.
	space float32
	// indent is how far its first line goes in (Theme.FirstLineIndent),
	// as its layout indents it (ui.TextShape.Indent).
	indent float32
}

// look is how the text of an editor looks: its font and the theme's
// colors, set as it builds.
type look struct {
	Theme
	size   float32
	indent float32  // DIPs a level of indentation takes
	border ui.Color // around an attachment not loaded yet
	// width is the room of the paragraph being laid out, which
	// attachments shrink to.
	width    float32
	tagStyle func(tag string, s *ui.Span)
}

func build(c *ui.Context, doc *Document, options []EditorOptions, styled bool) ui.Element {
	if doc == nil {
		panic("richtext: Editor requires a non-nil Document")
	}
	t := c.Theme()
	e := ui.Box(c).Focusable().Clip()
	v := ui.Local(e, "richtext", func() view { return view{cursor: ui.CursorText} })
	if v.doc != doc {
		*v = view{doc: doc, reveal: true}
		doc.lock()
		v.selAnchor, v.caret = doc.anchor, doc.caret
		v.selVersion = doc.selVersion
		if was := doc.shownIn.Value(); was != nil && was.doc == doc && was.version == doc.version {
			v.heightsOf(was)
		}
		doc.mu.Unlock()
	}
	// the editor built last shows the document for what the app reads
	// next (VisibleParagraphs, CaretRect): with two editors of one
	// document, each one's are read right after it is built
	doc.lock()
	doc.shownIn = weak.Make(v)
	doc.mu.Unlock()
	v.opts = EditorOptions{}
	if len(options) > 0 {
		v.opts = options[0]
	}
	v.services = c.Services()
	v.setLook(t, styled)
	v.sync()
	v.inView = e.Hovered()
	focused := e.Focused()
	if focused != v.focused {
		v.focused = focused
		v.blinkStart = c.Now()
		if !focused {
			v.dragging, v.thumb = false, false
		}
	}
	if v.changed {
		v.changed = false
		e.SetChanged()
	}
	label := v.opts.Label
	if label == "" {
		label = l10n.In(c.Locale()).RichText()
	}
	e.Role(ui.RoleTextField).Label(label).Access(v.access)
	if !v.opts.ReadOnly {
		e.HandleTextInput(v)
	}
	e.HandleInput(v.input)
	e.Cursor(v.cursor)
	e.Draw(v.paint)
	if v.opts.Menu != nil {
		e.ContextMenu(v.opts.Menu)
	} else {
		e.ContextMenu(v.menu)
	}
	if styled {
		e.FocusRing(false)
	}
	if focused && !v.opts.ReadOnly {
		// The caret blinks for half a minute after the last change.
		const blink = 530 * time.Millisecond
		if phase := c.Now().Sub(v.blinkStart); phase < 30*time.Second {
			c.After(blink - phase%blink)
		}
	}
	return e
}

// setLook sets how the text looks, from the editor's font and the theme.
func (v *view) setLook(t *ui.Theme, styled bool) {
	f := v.opts.Font
	if f.Size <= 0 {
		f.Size = t.FontSize
	}
	if f.Size <= 0 {
		f.Size = 14
	}
	if f.Family == "" {
		f.Family = t.Font
	}
	v.base = f
	def := defaultTheme(t)
	th := resolve(v.opts.Theme, &def)
	v.look = look{
		Theme:    th,
		size:     f.Size,
		indent:   f.Size * th.Indent,
		border:   t.Border,
		tagStyle: v.opts.TagStyle,
	}
	if styled {
		v.padX, v.padY = th.Padding[0], th.Padding[1]
	} else {
		v.padX, v.padY = 0, 0
	}
}

// ContentHeight returns how tall the text is in the editor showing the
// document, in DIPs, with the room around it: the paragraphs laid out
// measured, the others as tall as they were when last laid out, or
// estimated. An app that sizes an editor to its text, in a scroll
// container of its own, sizes it to this. ok is false until an editor
// showed the document, as in the first frame.
//
// Call it on the main thread, as the view builds.
func (d *Document) ContentHeight() (h float32, ok bool) {
	d.lock()
	v := d.shownIn.Value()
	d.mu.Unlock()
	if v == nil || v.doc != d || v.width <= 0 {
		return 0, false
	}
	v.sync()
	v.layout(v.width, v.viewH)
	return float32(v.hs.top(len(v.paras))) + 2*v.padY, true
}

// heightsOf starts a new editor of a document from the heights of the
// paragraphs the editor built before it measured, as an app that builds
// its tree anew for another theme builds its editor: the new one lays out
// what shows, not every paragraph again, which for an editor as tall as
// a long text is all of them. The layouts are made anew, in its look.
func (v *view) heightsOf(was *view) {
	v.shown, v.version = was.shown, was.version
	v.paras = make([]paraView, len(was.paras))
	for i, pv := range was.paras {
		v.paras[i] = paraView{h: pv.h, space: pv.space}
	}
	v.width, v.viewH, v.laidLook = was.width, was.viewH, was.laidLook
	v.clip, v.clipped = was.clip, was.clipped
	v.resetHeights()
}

// sync gets the editor in step with its document: the paragraphs that
// changed give up their layouts, and a selection the app set is taken.
func (v *view) sync() {
	v.doc.lock()
	defer v.doc.mu.Unlock()
	v.lockedSync()
}

// lockedSync is sync, with the document's lock held. It returns the change
// it took, and whether there was one the log described.
func (v *view) lockedSync() (change, bool) {
	d := v.doc
	var c change
	moved := false
	var steps []change
	if d.version != v.version {
		c, steps, moved = d.since(v.version)
		if moved && v.version != 0 && len(v.paras) == v.shown.ParagraphCount() {
			v.replaced(c, steps, d.text)
		} else {
			if v.anchorText {
				v.setScroll(v.scroll)
			}
			v.shown = d.text
			v.paras = make([]paraView, d.text.ParagraphCount())
			v.laid = 0
			v.resetHeights()
			moved = false
		}
		v.shown, v.version = d.text, d.version
		n := v.shown.Len()
		if moved {
			// Through each change in turn, as the document moved its own
			// selection: a paragraph moved past the caret leaves it where
			// it was in its text.
			upperAnchor, upperCaret := v.selAnchor > v.caret, v.caret > v.selAnchor
			for _, s := range steps {
				v.selAnchor = mapPosition(v.selAnchor, s.start, s.oldEnd-s.start, s.newEnd-s.start, upperAnchor)
				v.caret = mapPosition(v.caret, s.start, s.oldEnd-s.start, s.newEnd-s.start, upperCaret)
			}
		}
		v.selAnchor, v.caret = min(v.selAnchor, n), min(v.caret, n)
	}
	if d.selVersion != v.selVersion {
		v.selVersion = d.selVersion
		v.selAnchor, v.caret = d.anchor, d.caret
		v.affinity = ui.TextDownstream
		v.hasDesired = false
		v.compose, v.composing = "", false
		v.composeCaret, v.composeSelected = 0, ui.TextInputRange{}
		v.reveal = true
	}
	return c, moved
}

// replaced takes a change of the text, made of steps: the paragraphs from
// the one holding c.start to the one holding c.oldEnd of the text shown
// give way to those of now, without layouts.
func (v *view) replaced(c change, steps []change, now Text) {
	was := v.shown
	pa, pz := was.paraAt(c.start), was.paraAt(c.oldEnd)
	npz := now.paraAt(c.newEnd)
	stay := v.scroll - v.hs.top(pa)
	for _, p := range v.paras[pa : pz+1] {
		if p.layout != nil {
			v.laid--
		}
	}
	if npz == pz {
		// As many paragraphs: they give up their heights in place, as a
		// keystroke's does, rather than summing all the others again.
		for p := pa; p <= pz; p++ {
			if h := v.paras[p].h; h > 0 {
				v.hs.add(p, -float64(h), 1)
			}
			v.paras[p] = paraView{}
		}
	} else {
		added := make([]paraView, npz-pa+1)
		v.paras = slices.Replace(v.paras, pa, pz+1, added...)
		v.resetHeights()
	}
	v.shown = now
	switch {
	case v.anchor > pz:
		v.anchor += npz - pz
	case v.anchor < pa:
	default:
		at := was.paraStart(v.anchor)
		if v.anchor == pa && c.start > at {
			break // within the anchor's paragraph
		}
		// The change reaches the anchor's start. When it's made of
		// several replacements, as a paragraph moved from above the
		// anchor to below it in one edit, or undoing that, and the view
		// isn't at the top of the text, follow the anchor's start through
		// them: a paragraph that starts there still keeps the view. One
		// replacement goes as it always has.
		p, ok := 0, false
		if len(steps) > 1 && v.scroll > 0 {
			p, ok = follow(at, steps, now, !v.anchorText)
		}
		switch {
		case ok:
			v.anchor = p
		case v.anchor == pa:
		case v.anchorText:
			// The caret's paragraph, joined to one above or replaced
			// with it: the view stays.
			v.anchor, v.anchorOff, v.anchorText = pa, stay, false
		default:
			v.anchor, v.anchorOff = pa, 0
		}
	}
	v.numbersOf = Text{}
}

// follow returns the paragraph of now that starts where the one starting
// at rune at did, following the changes in turn, and whether there is one:
// no change took that start away or joined it to the paragraph before.
// When a change deletes from that start on, it's the paragraph that
// starts there now.
// With above set, what is inserted right where it starts goes above it, as
// scroll anchoring keeps the first node in view below what is added before
// it (the paragraph an undo puts back above the view), unless that leaves
// its start inside a paragraph (text typed at its start). Otherwise what
// is inserted there goes in it, as at the caret's paragraph, whose line
// moves down at a Return at its start.
func follow(at int, changes []change, now Text, above bool) (int, bool) {
	for _, before := range [...]bool{above, false} {
		if p, ok := shiftStart(at, changes, before); ok && p <= now.Len() {
			if i := now.paraAt(p); now.paraStart(i) == p {
				return i, true
			}
		}
		if !above {
			break
		}
	}
	return 0, false
}

// shiftStart returns where the start of a paragraph at goes through the
// changes, and false when one takes it away. What is inserted right at it
// goes before it when before is set, and after it otherwise.
func shiftStart(at int, changes []change, before bool) (int, bool) {
	for _, c := range changes {
		switch {
		case c.start < at && c.oldEnd <= at, before && c.start == at && c.oldEnd == at:
			at += c.newEnd - c.oldEnd
		case c.start >= at:
		default:
			return 0, false
		}
	}
	return at, true
}

// resetHeights sums the heights of the paragraphs laid out.
func (v *view) resetHeights() {
	hs := make([]float32, len(v.paras))
	for i := range v.paras {
		hs[i] = v.paras[i].h
	}
	v.hs.reset(hs)
	v.estimate()
}

// estimate sets the height of paragraphs not laid out yet to the average
// of those that are, or a line of text.
func (v *view) estimate() {
	m, unknown := v.hs.totals()
	if n := len(v.paras) - unknown; n > 0 {
		v.hs.est = m / float64(n)
	} else {
		v.hs.est = float64(v.look.size) * 1.3
	}
}

// para returns paragraph p of the text shown.
func (v *view) para(p int) *richPara { return seqAt(v.shown.seq(), p) }

// paraFont returns the font of a paragraph of style st.
func (v *view) paraFont(st ParagraphStyle) ui.Font {
	f := v.base
	switch {
	case st.Heading > 0:
		h := v.look.heading(st.Heading)
		f.Size *= h.Size
		f.Weight = h.Weight
		if h.LineHeight > 0 {
			f.LineHeight = h.LineHeight
		}
	case st.CodeBlock:
		f.Family = v.look.CodeFont
		f.Size *= v.look.CodeSize
	}
	return f
}

// lookKey is what of the look the layouts of paragraphs are made of.
type lookKey struct {
	base  ui.Font
	theme Theme
	tags  bool
	spans bool
	ver   int
}

func (v *view) lookKey() lookKey {
	return lookKey{base: v.base, theme: v.look.Theme, tags: v.look.tagStyle != nil, spans: v.opts.Spans != nil, ver: v.opts.SpansVersion}
}

// heading returns the style of headings of level, from 1.
func (look *look) heading(level int) *HeadingStyle {
	return &look.Headings[min(max(level, 1), 6)-1]
}

// insets returns the room left and right of paragraph p: its indentation,
// its list's marker, a quotation's bar, a block of code's padding.
func (v *view) insets(st ParagraphStyle) (left, right float32) {
	look := &v.look
	left = float32(st.Indent) * look.indent
	if st.List != NoList {
		left += look.indent
	}
	if st.Quote {
		left += look.indent * 0.75 * float32(st.quoteDepth())
	}
	if st.CodeBlock {
		left += look.size * look.CodeBlockPadding[0]
		right += look.size * look.CodeBlockPadding[0]
	}
	return left, right
}

// body reports whether a paragraph of style st is of the body's text,
// which Theme.FirstLineIndent and ParagraphSpacing set apart: not a
// heading, an item of a list or code.
func body(st ParagraphStyle) bool {
	return st.Heading == 0 && st.List == NoList && !st.CodeBlock
}

// firstLine returns how far the first line of paragraph p goes in: a body
// paragraph aligned at the start after another of the same quotation and
// level takes Theme.FirstLineIndent, as printed prose indents all but the
// first paragraph after a heading, a list, a centered scene break or an
// empty line. It looks at the paragraph before only, so that it costs
// O(log n) as the paragraph is laid out.
func (v *view) firstLine(p int, st ParagraphStyle) float32 {
	if v.look.FirstLineIndent <= 0 || p == 0 || !body(st) || st.Align != ui.Start {
		return 0
	}
	prev := v.para(p - 1)
	ps := prev.style
	if prev.n == 0 || !body(ps) || ps.Align != ui.Start || ps.quoteDepth() != st.quoteDepth() || ps.Indent != st.Indent {
		return 0
	}
	return v.look.size * v.look.FirstLineIndent
}

// space returns the room above and below paragraph p: a heading's
// margins, a block of code's padding at its ends, the spacing between
// body paragraphs, above the second.
func (v *view) space(p int, st ParagraphStyle) (above, below float32) {
	look := &v.look
	switch {
	case body(st):
		if look.ParagraphSpacing > 0 && p > 0 && body(v.para(p-1).style) {
			above = look.size * look.ParagraphSpacing
		}
	case st.Heading > 0:
		h := look.heading(st.Heading)
		if p > 0 {
			above = look.size * h.Size * h.Top
		}
		below = look.size * h.Bottom
	case st.CodeBlock:
		if p == 0 || !v.para(p-1).style.CodeBlock {
			above = look.size * look.CodeBlockPadding[1]
		}
		if p+1 >= len(v.paras) || !v.para(p+1).style.CodeBlock {
			below = look.size * look.CodeBlockPadding[1]
		}
	}
	if v.opts.Space != nil {
		above = max(0, above+v.opts.Space(p))
	}
	return above, below
}

// span returns how a run with attributes a looks in a paragraph of style
// st.
func (look *look) span(text string, a Attributes, st ParagraphStyle) ui.Span {
	s := ui.Span{Text: text, Font: a.Font, Size: a.Size, Weight: a.Weight, Italic: a.Italic, Color: a.Color,
		Underline: a.Underline, Strikethrough: a.Strikethrough, Background: a.Background}
	if a.Code {
		s.Font = look.CodeFont
		if s.Size == 0 {
			s.Size = look.size * look.CodeSize
		}
		if s.Background.A == 0 {
			s.Background = look.CodeBackground
		}
		// A pill hugging the code, as Markdown views show it.
		s.Radius = max(3, s.Size/4)
	}
	if a.Link != "" {
		if s.Color.A == 0 {
			s.Color = look.Link
		}
		s.Underline = true
	}
	if st.List == Tasks && st.Checked {
		s.Strikethrough = true
		if s.Color.A == 0 {
			s.Color = look.Muted
		}
	}
	if st.Quote && s.Color.A == 0 {
		s.Color = look.QuoteText
	}
	if (a.Code || st.CodeBlock) && s.Color.A == 0 && look.Code != look.Text {
		s.Color = look.Code
	}
	if st.Heading > 0 && s.Color.A == 0 {
		if c := look.heading(st.Heading).Color; c != look.Text {
			s.Color = c
		}
	}
	if a.Tag != "" && look.tagStyle != nil {
		look.tagStyle(a.Tag, &s)
	}
	if at := a.Attachment; at != nil {
		// A box of the attachment's size, scaled down to the width.
		w, h := at.size()
		if look.width > 0 && w > look.width {
			w, h = look.width, h*look.width/w
		}
		s.ObjectWidth, s.ObjectHeight = w, h
	}
	return s
}

// lineOf returns the text of paragraph p, without its newline.
func (v *view) lineOf(p int) string { return rope.TextOf(v.shown.text).Line(p) }

// paraLayout returns paragraph p laid out, with the composition at the
// caret: it keeps the layout until the paragraph's runs or style change.
func (v *view) paraLayout(p int) *paraView {
	pv := &v.paras[p]
	para := v.para(p)
	compose, at := "", 0
	if v.compose != "" {
		if cp := v.shown.paraAt(v.caret); cp == p {
			compose, at = v.compose, v.caret-v.shown.paraStart(p)
		}
	}
	var first *attrRun
	if len(para.runs) > 0 {
		first = unsafe.SliceData(para.runs)
	}
	indent := v.firstLine(p, para.style)
	if pv.layout != nil && pv.id == para.id && pv.runs == first && pv.nruns == len(para.runs) && pv.style == para.style && pv.compose == compose && pv.at == at && pv.indent == indent {
		// The space around it follows its neighbours, which may have
		// changed without it: a code block's last line, the first heading,
		// a body paragraph after one that became a heading.
		if above, below := v.space(p, para.style); above+below != pv.space {
			h := pv.h - pv.space + above + below
			v.hs.add(p, float64(h-pv.h), 0)
			pv.h, pv.space = h, above+below
		}
		return pv
	}
	composed := compose
	left, right := v.insets(para.style)
	v.look.width = max(v.width-left-right, 1)
	line := v.lineOf(p)
	var spans []ui.Span
	var typing Attributes
	if compose != "" {
		v.doc.lock()
		typing = v.doc.typingAttributes()
		v.doc.mu.Unlock()
	}
	off, done := 0, 0
	add := func(n int, a Attributes) {
		end := off
		for k := 0; k < n && end < len(line); k++ {
			_, size := utf8.DecodeRuneInString(line[end:])
			end += size
		}
		if end > off {
			spans = append(spans, v.look.span(line[off:end], a, para.style))
		}
		off = end
	}
	for _, run := range para.runs {
		n := min(run.n, para.n-done)
		if compose != "" && done <= at && at < done+n {
			add(at-done, run.a)
			spans = append(spans, v.look.span(compose, typing, para.style))
			add(done+n-at, run.a)
			compose = ""
		} else {
			add(n, run.a)
		}
		done += n
		if done >= para.n {
			break
		}
	}
	if compose != "" {
		spans = append(spans, v.look.span(compose, typing, para.style))
	}
	if v.opts.Spans != nil {
		spans = v.opts.Spans(p, para.style, spans)
	}
	// The indent is on the side the paragraph starts at, the right of one
	// reading right to left.
	shape := ui.TextShape{Width: v.look.width, Align: para.style.Align, Indent: indent}
	l := ui.ShapeRichTextWith(spans, v.paraFont(para.style), shape)
	units := pv.units[:0]
	units = append(units, 0)
	for _, s := range spans {
		for _, r := range s.Text {
			units = append(units, units[len(units)-1]+utf16.RuneLen(r))
		}
	}
	above, below := v.space(p, para.style)
	_, lh := l.Size()
	h := above + lh + below
	if pv.layout == nil {
		v.laid++
	}
	if pv.h > 0 {
		v.hs.add(p, float64(h-pv.h), 0)
	} else {
		v.hs.add(p, float64(h), -1)
	}
	*pv = paraView{layout: l, units: units, id: para.id, runs: first, nruns: len(para.runs), style: para.style,
		compose: composed, at: at, h: h, space: above + below, indent: indent}
	return pv
}

// sized returns paragraph p with its height: laid out, unless it has a
// height but no layout and is out of what the painter's clip shows,
// where its height is enough. An editor as tall as its text has every
// paragraph in its view; what is out of the clip is laid out as it comes
// into it.
func (v *view) sized(p int) *paraView {
	pv := &v.paras[p]
	if pv.layout == nil && pv.h > 0 && v.clipped {
		lo, hi := v.scroll+float64(v.clip[0]), v.scroll+float64(v.clip[1])
		if v.hs.top(p+1) < lo || v.hs.top(p) > hi {
			return pv
		}
	}
	return v.paraLayout(p)
}

// maxLaid is how many paragraphs keep their layouts out of view.
const maxLaid = 512

// origin returns where the layout of paragraph p starts in the content.
func (v *view) origin(p int) (x float32, y float64) {
	st := v.para(p).style
	left, _ := v.insets(st)
	above, _ := v.space(p, st)
	return left, v.hs.top(p) + float64(above)
}

// local returns where rune i of paragraph p is in its layout, which holds
// the composition at the caret.
func (v *view) local(p, i int) int {
	l := i - v.shown.paraStart(p)
	if v.compose != "" && i > v.caret && v.shown.paraAt(v.caret) == p {
		l += utf8.RuneCountInString(v.compose)
	}
	return l
}

// global returns the rune of the text at rune l of the layout of
// paragraph p.
func (v *view) global(p, l int) int {
	start := v.shown.paraStart(p)
	if v.compose != "" && v.shown.paraAt(v.caret) == p {
		n, c := utf8.RuneCountInString(v.compose), v.caret-start
		switch {
		case l > c+n:
			l -= n
		case l > c:
			l = c
		}
	}
	return min(start+l, start+v.para(p).n)
}

// runeOfUnit returns the rune of a paragraph's layout at UTF-16 unit u.
func (pv *paraView) runeOfUnit(u int) int {
	i, found := slices.BinarySearch(pv.units, u)
	if !found && i > 0 {
		i--
	}
	return min(i, len(pv.units)-1)
}

// caretAt returns the caret before rune i, extra runes into a composition
// there, in the content: its x, top and height.
func (v *view) caretAt(i, extra int, affinity ui.TextAffinity) (x float32, y float64, h float32) {
	p := v.shown.paraAt(i)
	pv := v.paraLayout(p)
	l := min(v.local(p, i)+extra, len(pv.units)-1)
	r := pv.layout.CaretAt(ui.TextCaretPosition{Index: pv.units[l], Affinity: affinity})
	ox, oy := v.origin(p)
	return ox + r.X, oy + float64(r.Y), r.H
}

// positionAt returns the caret position closest to (x, y) in the content.
func (v *view) positionAt(x float32, y float64) (int, ui.TextAffinity) {
	p, l, aff := v.layoutPositionAt(x, y)
	if p < 0 {
		return v.shown.Len(), ui.TextUpstream
	}
	return v.global(p, l), aff
}

// layoutPositionAt returns the paragraph closest to (x, y) in the content
// and the rune of its layout there, which holds the composition; p is -1
// past the paragraphs.
func (v *view) layoutPositionAt(x float32, y float64) (p, l int, aff ui.TextAffinity) {
	n := len(v.paras)
	y = max(y, 0)
	p = v.hs.at(y)
	for range n {
		pv := v.paraLayout(p)
		top := v.hs.top(p)
		switch {
		case y < top && p > 0:
			p--
		case y >= top+float64(pv.h) && p < n-1:
			p++
		default:
			ox, oy := v.origin(p)
			pos := pv.layout.PositionAt(ui.Point{X: x - ox, Y: float32(y - oy)})
			return p, pv.runeOfUnit(pos.Index), pos.Affinity
		}
	}
	return -1, 0, ui.TextUpstream
}

// lineEdges returns the edges of the visual line holding rune i.
func (v *view) lineEdges(i int) (int, int) {
	p := v.shown.paraAt(i)
	pv := v.paraLayout(p)
	r := pv.layout.CaretAt(ui.TextCaretPosition{Index: pv.units[min(v.local(p, i), len(pv.units)-1)], Affinity: v.affinity})
	mid := r.Y + r.H/2
	a := pv.layout.PositionAt(ui.Point{X: -1e6, Y: mid})
	z := pv.layout.PositionAt(ui.Point{X: 1e6, Y: mid})
	s, e := v.global(p, pv.runeOfUnit(a.Index)), v.global(p, pv.runeOfUnit(z.Index))
	if s > e {
		s, e = e, s
	}
	return s, e
}

// setScroll scrolls the content to y, keeping the place by the paragraph
// there.
func (v *view) setScroll(y float64) {
	v.scroll = y
	v.anchor = v.hs.at(y)
	v.anchorOff = y - v.hs.top(v.anchor)
	v.anchorText = false
}

// scrollTo scrolls the content to y, kept between its ends, and reports
// whether it moved.
func (v *view) scrollTo(y float64) bool {
	y = max(0, min(y, v.hs.top(len(v.paras))-float64(v.viewH)))
	if y == v.scroll {
		return false
	}
	v.setScroll(y)
	return true
}

// anchorCaret keeps the place by the caret's paragraph while the editor
// has the keyboard and that paragraph is in view, as browsers prefer the
// focused editable element to keep in place (CSS Scroll Anchoring's
// priority candidates): what grows between the top of the view and the
// caret, room an app leaves with Space or an edit, moves the text above
// it up, and the line being typed stays. An editor scrolled to its top
// stays there, as one that shows all of its text does.
func (v *view) anchorCaret() {
	if !v.focused || v.opts.ReadOnly || v.scroll <= 0 {
		return
	}
	p := v.shown.paraAt(v.caret)
	if p < v.first || p > v.last {
		return
	}
	_, y := v.origin(p)
	v.anchor, v.anchorText, v.anchorY = p, true, y
}

// layout lays out the paragraphs in view of a content box w×h, and those
// the caret is in when it is to be revealed.
func (v *view) layout(w, h float32) {
	if key := v.lookKey(); w != v.width || key != v.laidLook {
		// Another width, font or theme: every paragraph anew. At the same
		// width the heights are kept, those of another font scaled to it,
		// until each paragraph is laid out again: those in view at once, the
		// others as they come into view (sized), so that an editor as tall
		// as a long text changes its theme or its font in the time it takes
		// to lay out what shows.
		keep := w == v.width && v.width > 0
		scale := float32(1)
		if was := v.laidLook.base.Size; keep && was > 0 && key.base.Size != was {
			scale = key.base.Size / was
		}
		v.laidLook = key
		for i := range v.paras {
			if keep {
				v.paras[i] = paraView{h: v.paras[i].h * scale, space: v.paras[i].space * scale}
			} else {
				v.paras[i] = paraView{}
			}
		}
		v.laid = 0
		v.width = w
		v.resetHeights()
	}
	v.viewH = h
	v.estimate()
	v.place(h)
	if v.reveal {
		v.reveal = false
		_, y, ch := v.caretAt(v.caret, v.composeCaret, v.affinity)
		switch {
		case y < v.scroll:
			v.setScroll(y)
		case y+float64(ch) > v.scroll+float64(h):
			p := v.shown.paraAt(v.caret)
			for q, room := p-1, y+float64(ch)-v.hs.top(p); q >= 0 && room < float64(h); q-- {
				room += float64(v.paraLayout(q).h)
			}
			_, y, ch = v.caretAt(v.caret, v.composeCaret, v.affinity)
			v.setScroll(y + float64(ch) - float64(h))
		}
		v.place(h)
	}
	total := v.hs.top(len(v.paras))
	if s := max(0, min(v.scroll, total-float64(h))); s != v.scroll {
		v.setScroll(s)
		v.place(h)
	}
	v.anchorCaret()
	if v.laid > maxLaid+v.last-v.first {
		v.forget()
	}
}

// place lays out the paragraphs in view, from the anchor down, and keeps
// the view where the anchor is.
func (v *view) place(h float32) {
	n := len(v.paras)
	v.anchor = max(0, min(v.anchor, n-1))
	v.sized(v.anchor)
	// top is where the view's top is in the content. Kept by the anchor's
	// text, it moves as much as that text did since it was anchored, which
	// is nothing at all when nothing moved it.
	top := func() float64 { return v.hs.top(v.anchor) + v.anchorOff }
	if v.anchorText {
		above, _ := v.space(v.anchor, v.para(v.anchor).style)
		top = func() float64 { return v.scroll + (v.hs.top(v.anchor) + float64(above) - v.anchorY) }
	}
	v.first = v.anchor
	for v.first > 0 && top()-v.hs.top(v.first) < 0 {
		v.first--
		v.sized(v.first)
	}
	v.setScroll(max(0, top()))
	y := v.hs.top(v.first)
	p := v.first
	for ; p < n; p++ {
		y += float64(v.sized(p).h)
		if y >= v.scroll+float64(h) {
			break
		}
	}
	v.last = min(p, n-1)
}

// ParagraphBox is where a paragraph is in an editor, as
// Document.VisibleParagraphs gives it.
type ParagraphBox struct {
	// Index is the paragraph's, from 0, and Start the rune it starts at.
	Index, Start int
	// Top and Height are those of its lines, in DIPs from the top of the
	// editor's element, scrolled: the room above and below the paragraph
	// (a heading's margins, the spacing between paragraphs) is left out,
	// so that a mark at Top lines up with its first line. Top is below 0
	// for a paragraph scrolled partly out at the top.
	Top, Height float32
}

// VisibleParagraphs returns the paragraphs in view in the editor showing
// the document, from the first to the last, those partly in view
// included, where the editor places them in this frame: an app lines up
// a gutter beside it with them, as a timeline or the numbers of
// paragraphs. A frame in which they move from where this said, as the
// editor scrolls, its width changes or it reveals the caret, builds
// another, so that what read them follows. It lays out only the
// paragraphs in view, as painting the editor does.
//
// Call it on the main thread, as the view builds. It returns nil until an
// editor showed the document, as in the first frame.
func (d *Document) VisibleParagraphs() []ParagraphBox {
	d.lock()
	v := d.shownIn.Value()
	d.mu.Unlock()
	var boxes []ParagraphBox
	if v != nil && v.doc == d && v.width > 0 {
		v.sync()
		v.layout(v.width, v.viewH)
		boxes = v.boxes(nil)
	}
	d.lock()
	if v != nil && v.doc == d {
		v.read = true
		v.readBoxes = append(v.readBoxes[:0], boxes...)
	} else {
		d.readNone = true
	}
	d.mu.Unlock()
	return boxes
}

// CaretRect returns the caret of the editor showing the document, where
// the editor paints it in this frame, focused or not: a line as high as
// the caret's, in DIPs from the top left of the editor's element,
// scrolled. Place a popup at its bottom, as completions under what is
// typed after a "/" or an "@". A frame in which the caret moves from where
// this said, as the user types, the editor scrolls or its width changes,
// builds another, as one that moves the paragraphs VisibleParagraphs gave
// does, so that the popup follows. ok is false until an editor showed the
// document, and while the caret is scrolled out of view.
//
// Call it on the main thread, as the view builds.
func (d *Document) CaretRect() (r ui.Rect, ok bool) {
	d.lock()
	v := d.shownIn.Value()
	d.mu.Unlock()
	if v != nil && v.doc == d && v.width > 0 {
		v.sync()
		v.layout(v.width, v.viewH)
		r, ok = v.caretRect()
	}
	d.lock()
	if v != nil && v.doc == d {
		v.readCaret, v.readCaretRect = true, r
	} else {
		d.readNone = true
	}
	d.mu.Unlock()
	return r, ok
}

// ScrollBy scrolls the editor showing the document by dy DIPs, down when
// dy is positive, as far as its text goes, and reports whether it moved:
// an app that drags something of its own over the text (a block, by a
// handle beside it) scrolls the editor near its top and bottom edges, as
// the editor does while the user drags a selection there, and stops at an
// end. VisibleParagraphs and CaretRect read after it give where it
// scrolled to, and it asks for a frame to paint it.
//
// Call it on the main thread, as the view builds or in an event's
// handler. Before an editor showed the document, as in the first frame,
// it does nothing.
func (d *Document) ScrollBy(dy float32) bool {
	v := d.laidOut()
	if v == nil || !v.scrollTo(v.scroll+float64(dy)) {
		return false
	}
	v.services.Invalidate()
	return true
}

// ScrollToParagraph scrolls the editor showing the document to paragraph i
// (from 0), lined up as align says, as ui.Element's ScrollIntoView lines
// an element up: ui.ScrollNearest scrolls as little as shows it, and
// ui.ScrollStart, ui.ScrollCenter and ui.ScrollEnd line its start, center
// or end up with the editor's. Its box is that of its lines, as in
// VisibleParagraphs, without the room above and below it. It goes no
// further than the text does, and an i out of range is the first or the
// last paragraph. It leaves the caret where it is.
//
// Call it on the main thread, as the view builds or in an event's
// handler. Before an editor showed the document, as in the first frame,
// it does nothing.
func (d *Document) ScrollToParagraph(i int, align ui.ScrollAlign) {
	if align > ui.ScrollEnd {
		panic("richtext: ScrollToParagraph alignments are ui.ScrollNearest, ui.ScrollStart, ui.ScrollCenter or ui.ScrollEnd")
	}
	v := d.laidOut()
	if v == nil || len(v.paras) == 0 {
		return
	}
	i = max(0, min(i, len(v.paras)-1))
	h := float64(v.viewH)
	// The paragraphs above it that the view may show are laid out first,
	// for their heights, as revealing the caret does.
	pv := v.paraLayout(i)
	for q, room := i-1, float64(pv.h); q >= 0 && room < h; q-- {
		room += float64(v.paraLayout(q).h)
	}
	_, y := v.origin(i)
	size := float64(pv.h - pv.space)
	to := v.scroll
	switch align {
	case ui.ScrollStart:
		to = y
	case ui.ScrollCenter:
		to = y + size/2 - h/2
	case ui.ScrollEnd:
		to = y + size - h
	default:
		// What is taller than the view shows from its start.
		start, end := y-v.scroll, y+size-v.scroll
		switch {
		case start < 0:
			to = y
		case end > h:
			to = v.scroll + min(end-h, start)
		}
	}
	if v.scrollTo(to) {
		v.services.Invalidate()
	}
}

// laidOut returns the editor showing the document, in step with it and
// laid out as it was last painted, or nil before one painted it.
func (d *Document) laidOut() *view {
	d.lock()
	v := d.shownIn.Value()
	d.mu.Unlock()
	if v == nil || v.doc != d || v.width <= 0 {
		return nil
	}
	v.sync()
	v.layout(v.width, v.viewH)
	return v
}

// caretRect returns where the editor paints the caret, in its element,
// and whether that is in view.
func (v *view) caretRect() (ui.Rect, bool) {
	if len(v.paras) == 0 {
		return ui.Rect{}, false
	}
	x, y, h := v.caretAt(v.caret, v.composeCaret, v.affinity)
	r := ui.Rect{X: v.padX + x, Y: v.padY + float32(y-v.scroll), W: 1, H: h}
	return r, r.Y+r.H > 0 && r.Y < v.rect.H
}

// boxes appends the boxes of the paragraphs in view to out.
func (v *view) boxes(out []ParagraphBox) []ParagraphBox {
	if v.first > v.last || v.first >= len(v.paras) {
		return out
	}
	// Each start is counted on from the one before, as paint does: found
	// from the start of the text, each cost as much as the text is long.
	ps := v.shown.paraStart(v.first)
	for i := v.first; i <= v.last && i < len(v.paras); i, ps = i+1, ps+v.para(i).n+1 {
		pv := v.sized(i)
		_, y := v.origin(i)
		out = append(out, ParagraphBox{Index: i, Start: ps, Top: v.padY + float32(y-v.scroll), Height: pv.h - pv.space})
	}
	return out
}

// moved asks for another frame when the app read the paragraphs in view
// and they are no longer where it read them, as painting placed them
// otherwise: the frame after builds the app's view with them again.
func (v *view) moved() {
	d := v.doc
	d.lock()
	read, was := v.read, v.readBoxes
	readCaret, caretWas := v.readCaret, v.readCaretRect
	v.read, v.readCaret = false, false
	if d.readNone { // read before any editor showed it: what this one placed is news
		d.readNone, read, was = false, true, nil
	}
	d.mu.Unlock()
	if read {
		v.placed = v.boxes(v.placed[:0])
		if !slices.Equal(v.placed, was) {
			v.services.Invalidate()
			return
		}
	}
	if readCaret {
		if r, _ := v.caretRect(); r != caretWas {
			v.services.Invalidate()
		}
	}
}

// forget drops the layouts of the paragraphs far from view, keeping their
// heights.
func (v *view) forget() {
	lo, hi := v.first-maxLaid/4, v.last+maxLaid/4
	for i := range v.paras {
		if (i < lo || i > hi) && v.paras[i].layout != nil {
			v.paras[i].layout, v.paras[i].units = nil, nil
			v.paras[i].runs = nil
			v.laid--
		}
	}
}

// contentPoint returns the point of the content at (x, y) of the element.
func (v *view) contentPoint(x, y float32) (float32, float64) {
	return x - v.padX, float64(y-v.padY) + v.scroll
}

// selection returns the selection, in order.
func (v *view) selection() (int, int) { return min(v.selAnchor, v.caret), max(v.selAnchor, v.caret) }

// access describes the editor to assistive technology: its text, and the
// selection.
func (v *view) access(n *platform.AccessNode) {
	n.Value = v.shown.String()
	n.SelStart, n.SelEnd = v.selection()
	n.States |= platform.AccessMultiline
	if v.opts.ReadOnly {
		n.States |= platform.AccessReadOnly
	}
	if v.shown.Len() == 0 {
		n.Placeholder = v.opts.Placeholder
	}
	n.TextRuns = v.accessRuns(n.TextRuns[:0])
}

// accessRuns appends the runs of the text that have formats, as screen
// readers read them: the attributes as they show, and the headings.
func (v *view) accessRuns(out []platform.AccessTextRun) []platform.AccessTextRun {
	for start, p := range v.shown.Paragraphs() {
		font := v.paraFont(p.Style)
		at := start
		for _, r := range p.Runs {
			n := utf8.RuneCountInString(r.Text)
			s := v.look.span(r.Text, r.Attributes, p.Style)
			run := platform.AccessTextRun{Start: at, End: at + n, Family: s.Font, Size: s.Size, Weight: s.Weight,
				Italic: s.Italic, Underline: s.Underline, Strikethrough: s.Strikethrough, Link: r.Attributes.Link, Heading: p.Style.Heading}
			if run.Family == "" && font.Family != v.base.Family {
				run.Family = font.Family
			}
			if run.Size == 0 && font.Size != v.base.Size {
				run.Size = font.Size
			}
			if run.Weight == 0 && font.Weight > 0 {
				run.Weight = font.Weight
			}
			if s.Color.A > 0 {
				run.Color = [4]uint8{s.Color.R, s.Color.G, s.Color.B, s.Color.A}
			}
			if s.Background.A > 0 {
				run.Background = [4]uint8{s.Background.R, s.Background.G, s.Background.B, s.Background.A}
			}
			if run != (platform.AccessTextRun{Start: run.Start, End: run.End}) {
				// Neighbors alike are one run, or screen readers announce a
				// change of format that is not.
				if k := len(out) - 1; k >= 0 && out[k].End == run.Start && sameFormat(out[k], run) {
					out[k].End = run.End
				} else {
					out = append(out, run)
				}
			}
			at += n
		}
	}
	return out
}

// sameFormat reports whether two runs have the same formats.
func sameFormat(a, b platform.AccessTextRun) bool {
	a.Start, a.End, b.Start, b.End = 0, 0, 0, 0
	return a == b
}
