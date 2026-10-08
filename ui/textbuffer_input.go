package ui

// TextInputBuffer creates a single-line control editing indexed storage.
// It shares TextInput's selection, composition, undo and styling behavior.
func TextInputBuffer(c *Context, value *TextBuffer) *Element {
	return styledBufferInput(c, value, false)
}

// TextAreaBuffer creates a multiline control editing indexed storage.
// Ordinary edits and native queries never materialize the whole document.
// Layout retains the paragraphs in view, as TextArea does.
func TextAreaBuffer(c *Context, value *TextBuffer) *Element { return styledBufferInput(c, value, true) }

// TextInputBufferBase and TextAreaBufferBase are the unstyled buffer controls.
func TextInputBufferBase(c *Context, value *TextBuffer) *Element {
	return bufferInputBase(c, value, false)
}
func TextAreaBufferBase(c *Context, value *TextBuffer) *Element {
	return bufferInputBase(c, value, true)
}

func styledBufferInput(c *Context, value *TextBuffer, multiline bool) *Element {
	t := c.theme
	e := bufferInputBase(c, value, multiline)
	e.Padding(t.Space(1.5), t.Space(2.5)).Radius(t.Radius).Background(t.Surface).Border(1, t.Border)
	if multiline {
		e.MinHeight(t.Space(20))
	}
	e.styleFn = func(e *Element) { inputBorder(t, e, e) }
	return e
}

func (ed *editor) loadBuffer(s TextSnapshot) {
	clear(ed.undo)
	clear(ed.redo)
	clear(ed.queue)
	ed.undo, ed.redo, ed.queue = nil, nil, nil
	ed.compose, ed.compositionActive = "", false
	ed.composeCaret, ed.composeSelected = 0, TextInputRange{}
	ed.buf.setSnapshot(s)
	ed.published = s
	ed.caret = min(ed.caret, ed.buf.n)
	ed.anchor = min(ed.anchor, ed.buf.n)
	ed.visualSelection, ed.selected = false, nil
	ed.coalesce = false
	if ed.area != nil {
		ed.area.reveal = true
	}
}

func (ed *editor) syncBuffer() {
	if ed.document == nil {
		return
	}
	snapshot := ed.document.Snapshot()
	if snapshot != ed.published {
		ed.loadBuffer(snapshot)
	}
}

func bufferInputBase(c *Context, value *TextBuffer, multiline bool) *Element {
	if value == nil {
		panic("ui: a buffer input requires a non-nil TextBuffer")
	}
	e := c.newElement(kindInput)
	e.flags |= flagEditable | flagFocusable | flagHover
	e.widget = "TextInputBuffer"
	if multiline {
		e.widget = "TextAreaBuffer"
		e.flags |= flagScrollY
	}
	st := e.st
	if st.editor == nil || st.editor.document != value {
		st.editor = newEditor()
		st.editor.document = value
		st.editor.loadBuffer(value.Snapshot())
		st.editor.caret, st.editor.anchor = st.editor.buf.n, st.editor.buf.n
	}
	ed := st.editor
	ed.syncBuffer()
	ed.multiline = multiline
	if multiline && ed.area == nil {
		ed.area = &area{reveal: true}
	}
	if ed.client == nil {
		ed.client = &widgetTextInput{ed: ed, rt: c.rt, id: e.id}
	}
	e.textClient = ed.client
	if ed.bufferDirty {
		st.changed = true
		c.rt.consumed = true
		ed.bufferDirty = false
	}
	if c.rt.focused == e.id || len(ed.queue) > 0 {
		before := ed.buf.version
		ed.process(c, e)
		if ed.buf.version != before {
			ed.client.publish()
			st.changed = true
			c.rt.consumed = true
			ed.bufferDirty = false
		}
	}
	if c.rt.focused != e.id {
		ed.compose = ""
	}
	ed.readOnly, ed.password = false, false
	ed.ranges = ed.ranges[:0]
	ed.lines = [2]int{}
	return e
}
