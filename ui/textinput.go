package ui

import (
	"github.com/egoist/mygo/internal/platform"
	"slices"
)

// TextInput creates a single-line text input editing *value.
func TextInput(c *Context, value *string) *Element { return textInput(c, value, false) }

// TextArea creates a multi-line text input editing *value.
func TextArea(c *Context, value *string) *Element { return textInput(c, value, true) }

func textInput(c *Context, value *string, multiline bool) *Element {
	t := c.theme
	e := textInputBase(c, value, multiline)
	e.Padding(t.Space(1.5), t.Space(2.5)).Radius(t.Radius).Background(t.Surface).Border(1, t.Border)
	if multiline {
		e.MinHeight(t.Space(20))
	}
	e.styleFn = func(e *Element) { inputBorder(t, e, e) }
	return e
}

func textInputBase(c *Context, value *string, multiline bool) *Element {
	e := c.newElement(kindInput)
	e.flags |= flagEditable | flagFocusable | flagHover
	e.widget = "TextInput"
	if multiline {
		e.widget = "TextArea"
		// It scrolls its text as a scroll container does its children.
		e.flags |= flagScrollY
	}
	st := e.st
	if st.editor == nil || st.editor.document != nil {
		st.editor = newEditor()
		st.editor.setText(*value)
		st.editor.caret, st.editor.anchor = st.editor.buf.n, st.editor.buf.n
	}
	ed := st.editor
	ed.value = value
	if ed.nativeDirty {
		ed.nativeDirty = false
		before := ed.nativeValue
		ed.nativeValue = ""
		if *value == before {
			if *value != ed.buf.s {
				st.changed = true
				c.rt.consumed = true
			}
			*value = ed.buf.s
		}
	}
	if ed.client == nil {
		ed.client = &widgetTextInput{ed: ed, rt: c.rt, id: e.id}
	}
	e.textClient = ed.client
	ed.multiline = multiline
	ed.inputOptions = platform.TextInputOptions{}
	if multiline && ed.area == nil {
		ed.area = &area{reveal: true}
	}
	// The input shares the string of *value: the same string is equal at
	// once, whatever its length.
	if ed.buf.s != *value {
		ed.setText(*value)
		ed.compose = ""
	}
	focused := c.rt.focused == e.id
	// Input queued while the input had the focus applies even when the
	// focus left before this frame, as with text typed right before Tab.
	if focused || len(ed.queue) > 0 {
		version := ed.buf.version
		ed.process(c, e)
		if ed.buf.version != version {
			if ed.buf.s != *value {
				st.changed = true
				c.rt.consumed = true
			}
			// Equal or not, the value shares the text's string again.
			*value = ed.buf.s
		}
	}
	if !focused {
		ed.compose = ""
	}
	// ReadOnly, Password and Lines say again for the next frame's input, as
	// the frame builds.
	ed.readOnly, ed.password = false, false
	ed.ranges = ed.ranges[:0]
	ed.lines = [2]int{}
	return e
}

// Placeholder shows s in an empty text input.
func (e *Element) Placeholder(s string) *Element {
	if e.st.editor != nil {
		e.st.editor.placeholder = s
	}
	return e
}

// ReadOnly makes a text input show its text without letting the user
// change it: the text can still be selected and copied, from the keyboard
// too, as it takes the focus, without a caret; assistive technology reads
// it as read-only.
func (e *Element) ReadOnly(on bool) *Element {
	if ed := e.st.editor; ed != nil && e.flags&flagEditable != 0 {
		ed.readOnly = on
		if on {
			ed.compose = ""
		}
	}
	return e
}

// Lines makes a text area as high as its text, wrapped at its width, from
// min lines up to max, past which it scrolls, as a message field grows with
// what is typed: TextArea(c, &draft).Lines(1, 8). Without it, a text area is
// as high as its paragraphs, three lines at least, unless given a height.
func (e *Element) Lines(min, max int) *Element {
	if ed := e.st.editor; ed != nil && ed.area != nil {
		ed.lines = [2]int{min, max}
	}
	return e
}

// TextSelection returns the selection of a text input as offsets in runes
// into its text, the caret where start equals end, as the app reads it to
// complete the word being typed.
func (e *Element) TextSelection() (start, end int) {
	ed := e.st.editor
	if ed == nil {
		return 0, 0
	}
	return ed.selection()
}

// SetTextSelection selects the runes of a text input from start to end, or
// puts the caret at start when they are equal, and scrolls it into view: the
// caret after a word the app completed. The offsets are kept within the text.
func (e *Element) SetTextSelection(start, end int) *Element {
	ed := e.st.editor
	if ed == nil {
		return e
	}
	n := ed.buf.n
	start, end = min(max(start, 0), n), min(max(end, 0), n)
	ed.editorSelection = editorSelection{anchor: start, caret: end}
	ed.hasDesired = false
	if ed.area != nil {
		ed.area.reveal = true
	}
	e.c.rt.requestFrame()
	return e
}

// Composing reports whether an input method composes text in a text
// input, as Pinyin before a candidate is chosen: its value holds the text
// once composed. Keys typed meanwhile are the input method's, as Enter
// choosing a candidate: they submit nothing and press no shortcut.
func (e *Element) Composing() bool {
	ed := e.st.editor
	return ed != nil && ed.compose != ""
}

// Password hides what a text input holds, in the frames that call it: an
// input that stops calling it shows its text again, as a field's eye
// button does. It does nothing to a text area: as on every platform, only
// single-line fields hide their text.
func (e *Element) Password() *Element {
	if ed := e.st.editor; ed != nil && !ed.multiline {
		ed.password = true
	}
	return e
}

// Selectable lets the user select and copy text by dragging, double-clicking
// a word, triple-clicking a line, or using Shift with the arrows. Its context
// menu has Copy and Select All; Cmd+C copies the selection.
//
// On a container, its Text and RichText descendants share one selection.
// Copy joins their selected text with newlines, in the order they were built.
// Nested Selectable containers have independent selections. Text inside
// controls, such as buttons and text inputs, keeps the control's interaction.
// Unselectable excludes a subtree. On a Text or RichText alone, the selection
// stays within that paragraph. Inline elements share their paragraph's
// selection; set Selectable on the paragraph.
func (e *Element) Selectable() *Element {
	if e.isInline() {
		return e
	}
	if e.kind == kindText || e.kind == kindBox {
		e.flags &^= flagUnselectable
		e.flags |= flagSelectable
	}
	return e
}

// Unselectable excludes a paragraph or container subtree from a surrounding
// Selectable container. A Selectable container nested inside it can provide
// a selection of its own. For inline elements, set it on their paragraph.
func (e *Element) Unselectable() *Element {
	if e.isInline() {
		return e
	}
	e.flags &^= flagSelectable
	e.flags |= flagUnselectable
	return e
}

// TextRange styles runes Start up to End of a text input's text: their
// color and, when Weight is set, their weight, as a message field shows
// the mentions in what is typed.
type TextRange struct {
	Start, End int
	Color      Color
	Weight     int
}

// TextRanges styles runs of a text input's text, in the frames that call
// it, with ranges that do not overlap. They follow the text as it is: an
// app that finds them in the text finds them again as it changes. A
// password shows none, nor a paragraph while an input method composes in
// it.
func (e *Element) TextRanges(ranges ...TextRange) *Element {
	ed := e.st.editor
	if ed == nil || e.flags&flagEditable == 0 {
		return e
	}
	for _, r := range ranges {
		if r.End > r.Start {
			ed.ranges = append(ed.ranges, r)
		}
	}
	slices.SortFunc(ed.ranges, func(a, b TextRange) int { return a.Start - b.Start })
	return e
}
