package ui

import (
	"math"
	"strings"
	"testing"
)

// replaceTester builds one text input bound to *s and replaces its whole
// text with *next, once, as the next frame is built. Changed reports a change
// in one pass of a frame: changed collects it, for the test to clear.
func replaceTester(s, next *string, changed *bool, multiline bool) *Tester {
	return coreNewTester(func(c *context) {
		var e *node
		if multiline {
			e = coreTextArea(c, s).Fill()
		} else {
			e = coreTextInput(c, s).Fill()
		}
		if *next != "" {
			e.ReplaceText(0, math.MaxInt, *next)
			*next = ""
		}
		if e.Changed() {
			*changed = true
		}
	}, 400, 300)
}

// TestReplaceTextUndo checks that a text the app puts in place of the
// user's is a step of undo of its own: Undo brings the user's words back,
// and Redo the app's text.
func TestReplaceTextUndo(t *testing.T) {
	for _, multiline := range []bool{false, true} {
		s, next, changed := "", "", false
		tt := replaceTester(&s, &next, &changed, multiline)
		tt.Press(20, 15)
		tt.Release(20, 15)
		tt.Type("my words")
		tt.Frame()
		next, changed = "Generated title", false
		tt.Frame()
		if s != "Generated title" || !changed {
			t.Fatalf("multiline %v: replaced to %q, changed %v", multiline, s, changed)
		}
		changed = false
		tt.Frame()
		if changed {
			t.Errorf("multiline %v: the next frame reports the change again", multiline)
		}
		ed := textAreaOrInput(tt).editor
		if ed.caret != ed.buf.n || ed.anchor != ed.caret {
			t.Errorf("multiline %v: caret %d, anchor %d after %d bytes", multiline, ed.caret, ed.anchor, ed.buf.n)
		}
		tt.Key(Cmd, KeyZ)
		if s != "my words" {
			t.Errorf("multiline %v: undo made %q", multiline, s)
		}
		tt.Key(Cmd|Shift, KeyZ)
		if s != "Generated title" {
			t.Errorf("multiline %v: redo made %q", multiline, s)
		}
		// Typing at once after it is a step of its own.
		tt.Type("!")
		tt.Frame()
		tt.Key(Cmd, KeyZ)
		if s != "Generated title" {
			t.Errorf("multiline %v: undoing the typing made %q", multiline, s)
		}
		tt.rt.close()
	}
}

// TestReplaceTextAfterTyping checks that what was typed in the frame that
// replaces the text is taken first, so Undo brings it back.
func TestReplaceTextAfterTyping(t *testing.T) {
	s, next, changed := "", "", false
	tt := replaceTester(&s, &next, &changed, true)
	defer tt.rt.close()
	tt.Press(20, 15)
	tt.Release(20, 15)
	tt.Type("ab")
	next = "new"
	tt.Frame()
	if s != "new" {
		t.Fatalf("replaced to %q", s)
	}
	tt.Key(Cmd, KeyZ)
	if s != "ab" {
		t.Errorf("undo made %q", s)
	}
}

// TestReplaceTextSame checks that replacing the text with itself makes no
// step and no change.
func TestReplaceTextSame(t *testing.T) {
	s, next, changed := "", "", false
	tt := replaceTester(&s, &next, &changed, false)
	defer tt.rt.close()
	tt.Press(20, 15)
	tt.Release(20, 15)
	tt.Type("same")
	tt.Frame()
	ed := textAreaOrInput(tt).editor
	steps := len(ed.undo)
	next, changed = "same", false
	tt.Frame()
	if changed || len(ed.undo) != steps {
		t.Errorf("changed %v, %d steps, want %d", changed, len(ed.undo), steps)
	}
}

// TestReplaceTextLimits checks that a single-line input takes line breaks
// as spaces and that MaxLength does not cut the text.
func TestReplaceTextLimits(t *testing.T) {
	s := ""
	next := ""
	tt := coreNewTester(func(c *context) {
		e := coreTextInput(c, &s).MaxLength(3).Fill()
		if next != "" {
			e.ReplaceText(0, math.MaxInt, next)
			next = ""
		}
	}, 400, 300)
	defer tt.rt.close()
	next = "one\ntwo"
	tt.Frame()
	if s != "one two" {
		t.Errorf("replaced to %q", s)
	}
}

// TestReplaceTextCRLF checks that a line break of two characters is one,
// as in a paste: one space of a single-line input and a line feed of a text
// area, and that a text the input holds already, once its line breaks are
// taken so, makes no step and no change.
func TestReplaceTextCRLF(t *testing.T) {
	for _, multiline := range []bool{false, true} {
		want := "one two three"
		if multiline {
			want = "one\ntwo\nthree"
		}
		s, next, changed := "", "", false
		tt := replaceTester(&s, &next, &changed, multiline)
		next = "one\r\ntwo\rthree"
		tt.Frame()
		if s != want {
			t.Errorf("multiline %v: replaced to %q, want %q", multiline, s, want)
		}
		ed := textAreaOrInput(tt).editor
		steps := len(ed.undo)
		next, changed = "one\r\ntwo\rthree", false
		tt.Frame()
		if s != want || changed || len(ed.undo) != steps {
			t.Errorf("multiline %v: the same text made %q, changed %v, %d steps, want %d", multiline, s, changed, len(ed.undo), steps)
		}
		tt.rt.close()
	}
}

// TestReplaceTextBuffer replaces the text of an input editing a TextBuffer:
// the buffer takes it, and Undo brings the user's text back.
func TestReplaceTextBuffer(t *testing.T) {
	for _, multiline := range []bool{false, true} {
		b := NewTextBuffer("")
		next := ""
		tt := coreNewTester(func(c *context) {
			e := bufferInputBase(c, b, multiline).Fill().AutoFocus()
			if next != "" {
				e.ReplaceText(0, math.MaxInt, next)
				next = ""
			}
		}, 400, 140)
		tt.Type("mine")
		tt.Frame()
		theirs := strings.Repeat("theirs ", 3)
		next = theirs
		tt.Frame()
		if b.String() != theirs {
			t.Fatalf("multiline %v: buffer holds %q", multiline, b.String())
		}
		tt.Key(Cmd, KeyZ)
		if b.String() != "mine" {
			t.Errorf("multiline %v: undo left %q", multiline, b.String())
		}
		tt.Key(Cmd|Shift, KeyZ)
		if b.String() != theirs {
			t.Errorf("multiline %v: redo left %q", multiline, b.String())
		}
		tt.rt.close()
	}
}

// TestReplaceTextRange completes the word before the caret and inserts at
// it, each a step of undo, the caret after the text put in.
func TestReplaceTextRange(t *testing.T) {
	s := ""
	var edit func(e *node)
	tt := coreNewTester(func(c *context) {
		e := coreTextInput(c, &s).Fill()
		if edit != nil {
			edit(e)
			edit = nil
		}
	}, 400, 300)
	defer tt.rt.close()
	tt.Press(20, 15)
	tt.Release(20, 15)
	tt.Type("hi @br")
	tt.Frame()
	edit = func(e *node) {
		_, caret := e.TextSelection()
		e.ReplaceText(caret-3, caret, "@brodie ")
	}
	tt.Frame()
	ed := textAreaOrInput(tt).editor
	if s != "hi @brodie " || ed.caret != 11 || ed.anchor != 11 {
		t.Fatalf("completed to %q, caret %d, anchor %d", s, ed.caret, ed.anchor)
	}
	ed.anchor, ed.caret = 2, 2
	edit = func(e *node) {
		a, z := e.TextSelection()
		e.ReplaceText(a, z, " 👋")
	}
	tt.Frame()
	if s != "hi 👋 @brodie " || ed.caret != 4 {
		t.Fatalf("inserted to %q, caret %d", s, ed.caret)
	}
	tt.Key(Cmd, KeyZ)
	if s != "hi @brodie " {
		t.Errorf("first undo made %q", s)
	}
	tt.Key(Cmd, KeyZ)
	if s != "hi @br" {
		t.Errorf("second undo made %q", s)
	}
	// Offsets out of the text or the wrong way round are kept within it.
	edit = func(e *node) { e.ReplaceText(math.MaxInt, -1, "x") }
	tt.Frame()
	if s != "x" {
		t.Errorf("clamped replace made %q", s)
	}
}

// textAreaOrInput returns the state of the only text input of tt.
func textAreaOrInput(tt *Tester) *state {
	for _, st := range tt.rt.states {
		if st.editor != nil {
			return st
		}
	}
	return nil
}
