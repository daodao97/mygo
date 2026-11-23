package editor

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
)

// defaultScrollMargin is how many lines the editor keeps above
// and below the caret as it reveals it, as Zed's vertical_scroll_margin.
const defaultScrollMargin = 3

// State is a code editor's state, kept by the app across
// frames: the buffer it edits, its options, and what the editor keeps of
// its own, as its selections, folds and scroll position. Set the options
// before building the editor; zero values are the defaults, those of Zed.
type State struct {
	// Buffer is the text the editor edits; one is made when nil.
	Buffer *Buffer
	// Font is the editor's font: the system's monospaced font of 13 DIPs
	// when zero.
	Font ui.Font
	// LineHeight is the height of lines as a multiple of the font's size,
	// 1.5 when 0.
	LineHeight float32
	// TabSize is how many columns a tab and a level of indentation take:
	// the language's, else 4, when 0.
	TabSize int
	// Indent says whether indenting inserts tabs or spaces: as the
	// language's files usually do (Go's tabs) by default.
	Indent IndentStyle
	// SoftWrap wraps lines at the editor's width, or at WrapColumn
	// columns when narrower.
	SoftWrap   bool
	WrapColumn int
	// LineNumbers shows the numbers of lines in the gutter, absolute by
	// default.
	LineNumbers LineNumbers
	// ReadOnly makes the text selectable and copyable, not editable.
	ReadOnly bool
	// Whitespace says where spaces and tabs show as dots and arrows:
	// within selections by default.
	Whitespace Whitespace
	// WrapGuides draws guides at these columns, as 80 and 100.
	WrapGuides []int
	// NoIndentGuides hides the lines that show indentation levels.
	NoIndentGuides bool
	// NoScrollPastEnd stops scrolling at the last line rather than
	// letting it scroll up to the top.
	NoScrollPastEnd bool
	// ScrollMargin is how many lines to keep above and below the
	// caret when scrolling it into view. Zero uses the default of three
	// lines, a positive value sets that many lines, and a negative value
	// removes the margin. The margin shrinks to fit the viewport and
	// stops at the scroll limits.
	ScrollMargin int
	// NoAutoClose stops closing brackets and quotes as they are typed.
	NoAutoClose bool
	// NoFolding hides the fold markers of the gutter and turns folding
	// off.
	NoFolding bool
	// NoActiveLine stops highlighting the lines with a caret.
	NoActiveLine bool
	// CursorShape is the shape of the carets: bars by default, or blocks
	// over the character after them, as Vim's normal mode has, or
	// underlines, as its pending operators do.
	CursorShape CursorShape
	// FitContent makes the editor as high as its lines, to show in a
	// scroll view or a list, rather than scrolling them itself. The
	// containers around it scroll it: its find bar and go to line bring
	// their text into view through them, and the bars stick to the top of
	// the part of the text those containers show.
	FitContent bool
	// KeepScrollbars keeps the thumbs of the scroll bars, and the marks
	// of the carets on the vertical one, showing while the text has
	// anywhere to scroll, rather than only while the pointer is over the
	// editor or drags a thumb, as ui's KeepScrollbars does for a scroll
	// container; they look the same either way.
	// NoScrollbars takes the scroll bars away, with the marks of changes,
	// matches and diagnostics on them, and their room, which goes to the
	// text; the wheel, the keys and SetScrollY still scroll it.
	KeepScrollbars bool
	NoScrollbars   bool
	// Scrollbar styles the editor's scroll bars, as a theme's
	// ScrollbarStyle does: their track and thumb colors, the widths of
	// their bars and thumbs, their corners and shortest thumb. Its zero
	// fields follow the theme's; its TrackWidth, 0, is 10 DIPs unless the
	// theme's style sets one, as the editor's bar has always been.
	Scrollbar ui.ScrollbarStyle
	// Placeholder shows while the buffer is empty.
	Placeholder string
	// Label names the editor for assistive technology.
	Label string
	// Theme colors the editor; DefaultTheme of the window's theme
	// when nil.
	Theme *Theme
	// Bindings add key bindings, or replace the default ones for their
	// keys (DefaultBindings).
	Bindings []Binding
	// DiffBase, when set, is the text the buffer's changes are shown
	// against in the gutter, as the version of a file in Git: bars mark
	// lines added and changed, and a mark where lines were deleted.
	DiffBase *Buffer

	// OnCompletion asks the app for completions where the caret is, as
	// a word is typed or Ctrl+Space pressed: answer with ShowCompletions,
	// at once or later.
	OnCompletion func(CompletionRequest)
	// OnHover asks the app what to show for the text under the pointer
	// once it rests there: answer with ShowHover.
	OnHover func(HoverRequest)
	// HoverDelay is how long the pointer rests on the text before the
	// hover is asked for: a tooltip's, 600 ms, when zero or less.
	HoverDelay time.Duration
	// NoPointerHover shows no hover as the pointer rests, an app's nor a
	// diagnostic's, as VS Code's editor.hover.enabled off: ShowHover, as
	// Cmd+K Cmd+I, still asks for one at the caret.
	NoPointerHover bool
	// OnDefinition is called for Cmd+click (Ctrl+click elsewhere) and F12
	// on a word, as to go to its definition.
	OnDefinition func(offset int)
	// NoDefinitionClick leaves Cmd+click to the editor's other clicks,
	// with no link under it: F12 and the context menu still go to the
	// definition.
	NoDefinitionClick bool
	// NoDefinitionLink shows no link under the pointer with Cmd held,
	// though Cmd+click still goes to the definition.
	NoDefinitionLink bool
	// OnGutterClick is called for a click on a line's number, as to set a
	// breakpoint.
	OnGutterClick func(line int)
	// ContextMenu adds items to the editor's context menu, after its
	// own.
	ContextMenu func(m *ui.Menu)
	// GutterMarks marks lines in the gutter, as breakpoints or
	// bookmarks: a dot of their color.
	GutterMarks []GutterMark
	// LineBackgrounds fill the rows of lines with colors, under the
	// text and the selections, as the statement a key runs: not the
	// gutter, nor the lines folds hide. They keep their line numbers as
	// the text is edited, unlike diagnostics and inlays, which move with
	// it: compute them again when the buffer's Version changes.
	LineBackgrounds []LineBackground
	// TextBackgrounds fill ranges of the text with colors, under the
	// text and over line backgrounds, as the words a merge changed:
	// across wrapped rows, tabs and wide characters as the editor lays
	// them out, not the lines folds hide. As LineBackgrounds, they stay
	// where they are as the text is edited: compute them again when the
	// buffer's Version changes.
	TextBackgrounds []TextBackground
	// OnInput sees each input event before the editor, as keys for a
	// mode of the app's such as Vim's: returning true takes it, and the
	// editor leaves it. Text typed at the carets comes as InputText;
	// what an input method composes, commits or replaces, as the accents
	// of press and hold, goes to the editor alone. The key after the
	// first of a chord of Bindings, as Cmd+K Cmd+0, is the chord's.
	OnInput func(ev ui.InputEvent) bool
	// NoInputMethod turns the system's input methods off while the
	// editor has the keyboard (ui's InputMethod): keys type the
	// characters of the keyboard's layout, which come to OnInput as
	// InputText, and nothing is composed, as Vim's normal mode wants
	// with a Chinese input method on. Browsers cannot turn input methods
	// off: there they compose in the text still.
	NoInputMethod bool

	// RenderFindBar, RenderGoToLine, RenderCompletions and RenderHover
	// build the editor's find bar, go to line bar, completions and hovers
	// in place of its own (DefaultFindBar, DefaultGoToLine,
	// DefaultCompletions and DefaultHover, which they can call to wrap
	// them): each is given the element the editor places, as a toolbar
	// over its top right corner or a list below the caret, to style and
	// fill, and a handle to what the part shows and does.
	RenderFindBar     func(c *ui.Context, bar ui.Element, f *FindBar)
	RenderGoToLine    func(c *ui.Context, bar ui.Element, g *GoToLineBar)
	RenderCompletions func(c *ui.Context, panel ui.Element, m *CompletionMenu)
	RenderHover       func(c *ui.Context, panel ui.Element, h *HoverCard)

	ed editor
}

// IndentStyle is what indenting inserts.
type IndentStyle uint8

const (
	IndentAuto IndentStyle = iota
	IndentSpaces
	IndentTabs
)

// LineNumbers is how the gutter numbers lines.
type LineNumbers uint8

const (
	LineNumbersOn LineNumbers = iota
	LineNumbersRelative
	LineNumbersOff
)

// Whitespace is where spaces and tabs show.
type Whitespace uint8

const (
	WhitespaceSelection Whitespace = iota
	WhitespaceNone
	WhitespaceAll
	WhitespaceTrailing
	// WhitespaceBoundary shows whitespace but single spaces between
	// words.
	WhitespaceBoundary
)

// CursorShape is the shape of an editor's carets, as Zed's CursorShape
// and the terminal's CursorStyle.
type CursorShape uint8

const (
	// CursorBar is a bar before the character at the caret.
	CursorBar CursorShape = iota
	// CursorBlock covers the character after the caret, a space's width
	// at the end of a line, translucent so that the character shows. An
	// editor without the keyboard focus outlines it, as Vim's and Zed's
	// do.
	CursorBlock
	// CursorUnderline underlines the character after the caret.
	CursorUnderline
)

// GutterMark marks a line in the gutter.
type GutterMark struct {
	Line  int
	Color ui.Color
}

// LineBackground fills lines First to Last, from 0, with Color. The lines
// are those of the text when it is set: an edit adding or removing lines
// above them doesn't move them.
type LineBackground struct {
	First, Last int
	Color       ui.Color
}

// TextBackground fills bytes Start to End of the text with Color. Ranges
// past the end of the text, as an edit leaves them until they are
// computed again, are cut to it; empty ones fill nothing. A range across
// lines fills the text of each, not past its end.
type TextBackground struct {
	Start, End int
	Color      ui.Color
}

func (st *State) options() options {
	scrollMargin := defaultScrollMargin
	if st.ScrollMargin > 0 {
		scrollMargin = st.ScrollMargin
	} else if st.ScrollMargin < 0 {
		scrollMargin = 0
	}
	return options{
		readOnly: st.ReadOnly, font: st.Font, lineHeight: st.LineHeight, tabSize: st.TabSize, indent: st.Indent,
		wrap: st.SoftWrap, wrapColumn: st.WrapColumn, lineNumbers: st.LineNumbers, whitespace: st.Whitespace,
		wrapGuides: st.WrapGuides, indentGuides: !st.NoIndentGuides,
		scrollPastEnd: !st.NoScrollPastEnd && !st.FitContent, scrollMargin: scrollMargin,
		keepScrollbars: st.KeepScrollbars, hideScrollbars: st.NoScrollbars, scrollbar: st.Scrollbar,
		autoClose: !st.NoAutoClose, folding: !st.NoFolding, activeLine: !st.NoActiveLine, cursorShape: st.CursorShape, fitContent: st.FitContent,
		placeholder: st.Placeholder, label: st.Label, theme: st.Theme, bindings: st.Bindings, gutterMarks: st.GutterMarks,
		backgrounds: st.LineBackgrounds, textBgs: st.TextBackgrounds, diffBase: st.DiffBase, onCompletion: st.OnCompletion, onHover: st.OnHover, onDefinition: st.OnDefinition,
		hoverDelay: st.HoverDelay, noPointerHover: st.NoPointerHover,
		noDefinitionClick: st.NoDefinitionClick, noDefinitionLink: st.NoDefinitionLink,
		onGutterClick: st.OnGutterClick, contextMenu: st.ContextMenu, onInput: st.OnInput, noInputMethod: st.NoInputMethod, minDigits: 3,
		renderFindBar: st.RenderFindBar, renderGoToLine: st.RenderGoToLine,
		renderCompletions: st.RenderCompletions, renderHover: st.RenderHover,
	}
}

// Selections returns the editor's selections, in order.
func (st *State) Selections() []Selection {
	st.ensure()
	out := make([]Selection, len(st.ed.sels))
	for i, s := range st.ed.sels {
		out[i] = Selection{Anchor: s.anchor, Head: s.head}
	}
	return out
}

// Selection returns the newest selection, the one input methods and
// scrolling follow.
func (st *State) Selection() Selection {
	st.ensure()
	p := st.ed.primary()
	return Selection{Anchor: p.anchor, Head: p.head}
}

// SetSelections sets the selections, the last the newest, and scrolls
// it into view.
func (st *State) SetSelections(sels ...Selection) {
	st.ensure()
	if len(sels) == 0 {
		return
	}
	n := st.ed.view.text.Len()
	out := make([]sel, len(sels))
	for i, s := range sels {
		out[i] = st.ed.newSel(max(0, min(s.Anchor, n)), max(0, min(s.Head, n)))
	}
	st.ed.setSels(out, revealFit)
}

// Select selects the text from start to end, a caret alone at end when
// they are equal.
func (st *State) Select(start, end int) { st.SetSelections(Selection{Anchor: start, Head: end}) }

// Cursor returns the offset of the newest caret.
func (st *State) Cursor() int { return st.Selection().Head }

// CursorPos returns the line and column of the newest caret.
func (st *State) CursorPos() Pos {
	st.ensure()
	return posOf(st.ed.view.text, st.ed.primary().head)
}

// SelectedText returns the text of the selections, joined by newlines.
func (st *State) SelectedText() string {
	st.ensure()
	return st.ed.selectedText()
}

// Insert replaces each selection with text, as typing or pasting it.
func (st *State) Insert(text string) {
	st.ensure()
	st.ed.retry(func() bool { st.ed.paste(text, false); return true })
}

// Run runs a command as its key would, and reports whether it did
// something.
func (st *State) Run(cmd Command) bool {
	st.ensure()
	return st.ed.retry(func() bool { return st.ed.run(cmd) })
}

// GoTo puts the caret at a line and column, from 0, and scrolls it to
// the middle of the editor: an editor as high as its lines has no scroll
// of its own, so the containers around it take the line into view.
func (st *State) GoTo(p Pos) {
	st.ensure()
	off := offsetOf(st.ed.view.text, p)
	st.ed.setSels([]sel{st.ed.newSel(off, off)}, revealCenter)
	st.ed.askRevealNow(off)
}

// ScrollTo scrolls a line to the top of the editor.
func (st *State) ScrollTo(line int) {
	st.ensure()
	st.ed.setScroll(st.ed.view.heights.top(line))
}

// ScrollY returns how far the editor's lines are scrolled up, in DIPs, 0
// for an editor as high as its lines (FitContent); SetScrollY scrolls
// them, as far as they scroll, as editors side by side scroll together.
func (st *State) ScrollY() float64 { return st.ed.scrollY }

func (st *State) SetScrollY(y float64) {
	st.ensure()
	st.ed.setScroll(y)
}

// ViewportHeight returns the height of the part of the text the editor
// shows, in DIPs, as the last frame laid it out: from ScrollY to ScrollY
// plus ViewportHeight, against LineTop, is what is in view, as a minimap
// beside the editor shows it. An editor as high as its lines
// (FitContent) is that high.
func (st *State) ViewportHeight() float64 { return float64(st.ed.viewH) }

// LineTop returns the top of a line, from 0, in DIPs below the top of the
// first, as the last frame laid the lines out: less ScrollY, it is where
// the line shows in the editor. The lines a fold hides take no room, and
// LineTop of the count of lines is the height of them all. With SoftWrap,
// the lines of a long text far from the view may take the rows they are
// estimated to for a few frames, until they are measured.
func (st *State) LineTop(line int) float64 {
	st.ensure()
	return st.ed.view.heights.top(line)
}

// Rect returns where the caret at byte offset off shows, in DIPs from the
// top left of the editor's box, as the last frame laid it out, and whether
// its line shows: a line a fold hides doesn't. It is as high as a row; the
// row of off, with SoftWrap. A hosted frame around text draws its edges
// from the Rect of its first and last offsets, in an element's DrawOver.
// For a line far from the view with SoftWrap, Y may be an estimate for a
// few frames, as LineTop's is.
func (st *State) Rect(off int) (ui.Rect, bool) {
	st.ensure()
	off = max(0, min(off, st.ed.view.text.Len()))
	r, ok := st.ed.caretRect(off)
	if !ok {
		return ui.Rect{}, false
	}
	r.X += st.ed.textX - st.ed.scrollX
	r.Y -= float32(st.ed.scrollY)
	return r, true
}

// Find opens the find bar with query, and selects its first match after
// the caret.
func (st *State) Find(query string) {
	st.ensure()
	f := &st.ed.find
	f.open, f.query, f.stale = true, query, true
	f.step(&st.ed, 1)
}

// FindMatches returns how many matches the find bar has, and which is
// chosen, from 0.
func (st *State) FindMatches() (count, current int) {
	st.ensure()
	f := &st.ed.find
	if !f.open {
		return 0, 0
	}
	f.search(&st.ed)
	return len(f.matches), f.current
}

// Fold folds the lines indented under line, up to its closing bracket,
// or the run of imports it starts, as the gutter's marker does; Unfold
// unfolds them.
func (st *State) Fold(line int) {
	st.ensure()
	if !st.ed.isFolded(line) {
		st.ed.toggleFoldLine(line)
	}
}

func (st *State) Unfold(line int) {
	st.ensure()
	if st.ed.isFolded(line) {
		st.ed.toggleFoldLine(line)
	}
}

// FoldImports folds the first run of imports of the text, or the first
// import alone by its brackets, as editors can on opening a file
// (Language's Imports).
func (st *State) FoldImports() {
	st.ensure()
	ed := &st.ed
	l := ed.language()
	if l == nil || l.Imports == nil {
		return
	}
	r := &ed.view.text
	for line := range r.LineCount() {
		if !ed.isImport(l, line) {
			continue
		}
		if !ed.isFolded(line) {
			ed.toggleFoldLine(line)
		}
		return
	}
}

// Folded returns the lines folds start at.
func (st *State) Folded() []int {
	var out []int
	for _, f := range st.ed.folds {
		out = append(out, f.start)
	}
	slices.Sort(out)
	return out
}

// SetDiagnostics shows diagnostics of the text at version, the buffer's
// Version when they were made: those of an older version move with the
// edits since, as a language server's that arrive late.
func (st *State) SetDiagnostics(version uint64, ds []Diagnostic) {
	st.ensure()
	ed := &st.ed
	ed.sync()
	ds = slices.Clone(ds)
	if version != ed.view.version {
		// Moved through the edits since, up to the text the editor shows.
		entries, _, ok := st.Buffer.entriesSince(version)
		if !ok {
			return
		}
		for _, e := range entries {
			if e.version > ed.view.version {
				break
			}
			for i := range ds {
				d := &ds[i]
				d.Start, d.End = mapOffset(d.Start, e.changes, biasRight), mapOffset(d.End, e.changes, biasLeft)
				d.End = max(d.End, d.Start)
			}
		}
	}
	slices.SortStableFunc(ds, func(a, b Diagnostic) int { return a.Start - b.Start })
	ed.diags = ds
}

// Diagnostics returns the diagnostics shown, where the edits since moved
// them.
func (st *State) Diagnostics() []Diagnostic { return slices.Clone(st.ed.diags) }

// ShowCompletions shows completions of the word before the caret, which
// starts at offset start (the request's Offset less its Prefix), or -1
// for the word there now. Typing filters them; an empty list closes them.
func (st *State) ShowCompletions(start int, items []Completion) {
	st.ensure()
	st.ed.sync()
	st.ed.showCompletions(start, items)
}

// ShowHover shows what the text from start to end is: text in the
// editor's font, and what build adds, as a hover of a language server.
func (st *State) ShowHover(start, end int, text string, build func(c *ui.Context)) {
	st.ensure()
	h := &st.ed.hover
	h.shown, h.start, h.end, h.text, h.build = true, start, end, text, build
	if h.diags == nil {
		h.diags = st.ed.diagnosticsAt(start)
	}
}

// SetInlays shows inlay hints in the lines, as types and parameter
// names: they move with the edits until set again.
func (st *State) SetInlays(inlays []Inlay) {
	st.ensure()
	st.ed.setInlays(inlays)
}

// SetReplacements shows ranges of the text as something else, as text,
// as an element, or as nothing, as a Markdown editor hides the marks of
// emphasis and draws a check box in place of "[ ]": they move with the
// edits until set again, as inlays do. Set them again as the text or the
// selections change, as such an editor shows the marks of the lines with
// a caret. Empty ranges, and ranges overlapping one before them, are left
// out.
//
// A range may cover lines: the lines it covers from their start through
// their newline show no rows, as folded ones, but the one it starts on
// when it shows something there; the line it ends in shows what follows
// it.
func (st *State) SetReplacements(rs []Replacement) {
	st.ensure()
	st.ed.setReplacements(rs)
}

// Replacements returns the replacements shown, where the edits since
// moved them.
func (st *State) Replacements() []Replacement { return slices.Clone(st.ed.view.repls) }

// SetBlocks shows elements between the lines of the text, as an image
// under the line naming it: each as high as its element is laid out, and
// only those in view built. They move with the edits until set again, a
// block going with its line's text. Deleting that line attaches the block
// to the surviving line at that position, with Above unchanged.
// Set them again as the text changes: those at the place of one shown
// keep their elements and heights, so this is cheap in every frame.
// Blocks of lines that don't exist are left out.
//
// A block on a line a replacement hides shows in place of the lines the
// replacement covers, as a table drawn in place of its source; a fold
// hides the blocks of its lines.
func (st *State) SetBlocks(blocks []Block) {
	st.ensure()
	st.ed.setBlocks(blocks)
}

// Blocks returns the blocks shown, where the edits since moved them.
func (st *State) Blocks() []Block {
	st.ensure()
	out := make([]Block, len(st.ed.view.blocks))
	for i := range out {
		out[i] = st.ed.view.blocks[i].Block
	}
	return out
}

// SetLineStyles draws lines in other fonts, as a Markdown editor's
// headings in larger type: each line's rows are as high as its style
// makes them, and only the lines in view are laid out still. They move
// with the edits until set again, a style going with its line's text.
// Set them again as the text changes: setting the same styles costs
// nothing, and a change lays out again only the lines whose style
// changed. Styles of lines that don't exist are left out, and the last
// given for a line wins. A diff's sides take none.
//
// The gutter numbers styled lines in the editor's font, on the baseline
// of their first row, and a replacement's element without a Width is as
// wide as the line's rows are high.
func (st *State) SetLineStyles(styles []LineStyle) {
	st.ensure()
	st.ed.setLineStyles(styles)
}

// LineStyles returns the styles shown, where the edits since moved them.
func (st *State) LineStyles() []LineStyle {
	st.ensure()
	out := make([]LineStyle, len(st.ed.view.lineStyles))
	for i, s := range st.ed.view.lineStyles {
		out[i] = s.LineStyle
	}
	return out
}

// SetSuggestion shows text at offset, muted, as an edit prediction:
// Tab inserts it, and an edit or a move of the caret drops it. An empty
// text takes it away.
func (st *State) SetSuggestion(offset int, text string) {
	st.ensure()
	st.ed.setSuggestion(offset, text)
}

// Focused reports whether the editor had the keyboard focus in the last
// frame.
func (st *State) Focused() bool { return st.ed.focused }

// VisibleLines returns the first and the last line, from 0, that the
// last frame showed, those hidden by folds among them.
func (st *State) VisibleLines() (first, last int) { return st.ed.shown[0], st.ed.shown[1] }

// ensure readies the state for methods called before the editor is
// built.
func (st *State) ensure() {
	if st.Buffer == nil {
		st.Buffer = NewBuffer("")
	}
	ed := &st.ed
	if ed.view.buf != st.Buffer || ed.sels == nil {
		ed.init(st.Buffer, st.options(), ui.LightTheme())
	}
	ed.sync()
}
