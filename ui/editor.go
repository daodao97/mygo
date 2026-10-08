package ui

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/text"
)

type editKind uint8

const (
	editKey editKind = iota
	editInsert
	editCompose
	editCommand
	editSelect
)

type editEvent struct {
	kind  editKind
	mods  Modifiers
	key   Key
	text  string
	caret int
	// replace makes an insertion or a composition take the runes from to
	// to instead of the selection, as an input method asked.
	replace                bool
	snapshot               bool
	from, to               int
	markedStart, markedEnd int
}

// editor is the state of a text input: the text, the caret, the selection,
// the composition of an input method, undo history and what the last frame
// laid out.
type editor struct {
	buf       buffer
	graphemes graphemes
	editorSelection
	compose            string
	composeCaret       int
	composeSelected    TextInputRange
	compositionActive  bool // one undo transaction for preedit and its commit
	client             *widgetTextInput
	value              *string
	nativeDirty        bool
	nativeValue        string // bound value before the next widget build publishes input
	document           *TextBuffer
	published          TextSnapshot
	bufferDirty        bool
	nativeBufferBefore TextSnapshot
	queue              []editEvent
	multiline          bool
	readOnly           bool   // selectable text: selected and copied, not edited
	source             string // the text of selectable text
	password           bool
	inputOptions       platform.TextInputOptions
	// leaveEmptyBackspace leaves Backspace to shortcuts while the text is
	// empty, as a token field's input does to take out a token.
	leaveEmptyBackspace bool
	// lines are a text area's least and most lines (Lines), as high as its
	// wrapped text between them; zero for the paragraphs, at least three.
	lines       [2]int
	placeholder string
	// ranges style runs of the text (TextRanges), sorted by Start.
	ranges []TextRange
	// layout is what the last frame laid out of a single-line input or a
	// selectable text; area lays out a text area (textarea.go).
	layout           *text.Layout
	layoutVersion    uint64
	layoutGeneration uint64
	layoutCompose    string
	layoutPassword   bool
	area             *area
	// display maps runes of the text to runes of the layout, which shows
	// bullets for passwords and holds the composition.
	scrollX          float32
	originX, originY float32 // content box, relative to the element
	desiredX         float32
	hasDesired       bool
	undo, redo       []undoStep
	lastEdit         time.Time
	coalesce         bool
	dragging         bool
	dragUnit         int // 1 rune, 2 word, 3 line
	dragStart        [2]int
	pressMods        Modifiers
	contentW         float32
}

func newEditor() *editor { return &editor{} }

func (ed *editor) String() string { return ed.buf.string() }

// edit replaces the runes from a to z with s, and tells the area.
func (ed *editor) edit(a, z int, s string) {
	first, old, after := ed.buf.replace(a, z, s)
	if ed.area != nil {
		ed.area.edited(&ed.buf, first, old, after)
	}
}

func (ed *editor) insert(s string) {
	if ed.readOnly {
		return
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	a, b := ed.selection()
	ranges := ed.selectedRanges()
	ed.record(a == b && utf8.RuneCountInString(s) == 1)
	if len(ranges) == 0 {
		ed.replace(ed.caret, ed.caret, s)
	} else {
		for i := len(ranges) - 1; i >= 0; i-- {
			r := ranges[i]
			insert := ""
			if i == 0 {
				insert = s
			}
			ed.replace(r.Start, r.End, insert)
		}
	}
	ed.hasDesired = false
}

func (ed *editor) deleteRange(a, b int) {
	if a == b || ed.readOnly {
		return
	}
	ed.record(false)
	ed.replace(a, b, "")
	ed.hasDesired = false
}

func (ed *editor) commitCompose() {
	if ed.compose != "" {
		s := ed.compose
		ed.compose = ""
		ed.replace(ed.caret, ed.caret, s)
		if ed.client != nil {
			ed.client.publish()
		}
	}
	ed.composeCaret, ed.composeSelected = 0, TextInputRange{}
	ed.compositionActive = false
}

// snapshot applies only the changed runes of a native editor's surrounding
// text. Repeated IME snapshots must not create edits for unchanged context,
// replace the whole document in undo history, or invalidate every paragraph.
func (ed *editor) snapshot(ev editEvent) {
	if ed.readOnly {
		return
	}
	a, b := ed.selection()
	if ev.replace {
		a, b = min(ev.from, ed.buf.n), min(ev.to, ed.buf.n)
	}
	before, after := []rune(ed.buf.slice(a, b)), []rune(ev.text)
	prefix := 0
	for prefix < min(len(before), len(after)) && before[prefix] == after[prefix] {
		prefix++
	}
	x, y := len(before), len(after)
	for x > prefix && y > prefix && before[x-1] == after[y-1] {
		x--
		y--
	}
	if x != prefix || y != prefix {
		ed.record(x == prefix && y-prefix == 1)
		ed.replace(a+prefix, a+x, string(after[prefix:y]))
		ed.hasDesired = false
	}
	caret := max(0, min(a+ev.caret, ed.buf.n))
	if ed.caret != caret || ed.anchor != caret {
		ed.move(caret, false)
	} else if ed.area != nil {
		ed.area.reveal = true
	}
}

// process applies the input queued for the editor.
func (ed *editor) process(c *context, e *node) {
	st := e.st
	for _, ev := range ed.queue {
		if ev.replace && !ev.snapshot {
			ed.anchor, ed.caret = min(ev.from, ed.buf.n), min(ev.to, ed.buf.n)
		}
		switch ev.kind {
		case editSelect:
			ed.editorSelection = editorSelection{anchor: max(0, min(ev.from, ed.buf.n)), caret: max(0, min(ev.to, ed.buf.n))}
			if ed.area != nil {
				ed.area.reveal = true
			}
		case editKey:
			ed.commitCompose()
			ed.key(c, st, ev)
		case editInsert:
			ed.compose = ""
			if ev.snapshot {
				ed.snapshot(ev)
			} else {
				ed.insert(ev.text)
			}
		case editCompose:
			if ed.readOnly {
				break
			}
			if ed.area != nil {
				ed.area.reveal = true
			}
			if ev.snapshot {
				text := []rune(ev.text)
				ed.coalesce = false
				a, b := max(0, min(ev.markedStart, len(text))), max(0, min(ev.markedEnd, len(text)))
				b = max(a, b)
				ed.snapshot(editEvent{replace: ev.replace, from: ev.from, to: ev.to, text: string(text[:a]) + string(text[b:]), caret: a})
				ed.compose = string(text[a:b])
				ed.composeCaret = max(0, min(ev.caret-a, b-a))
				break
			}
			ed.compose = ev.text
			ed.composeCaret = max(0, min(ev.caret, utf8.RuneCountInString(ev.text)))
			if ev.text != "" {
				a, b := ed.selection()
				if a != b {
					ed.deleteRange(a, b)
				}
			}
		case editCommand:
			ed.commitCompose()
			ed.command(c, ev.text)
		}
	}
	clear(ed.queue)
	ed.queue = ed.queue[:0]
	if ed.dragging && st.pressed {
		rt := c.rt
		ed.drag(rt.pointerX-st.x, rt.pointerY-st.y)
	}
}
