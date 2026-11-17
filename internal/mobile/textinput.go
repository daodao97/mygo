package mobile

import (
	"math"

	"github.com/egoist/mygo/internal/mobile/wire"
	"github.com/egoist/mygo/internal/platform"
)

// The keyboard edits the document of the input with the focus through its
// TextInputClient, by UTF-16 offsets, which are what UITextInput and
// InputConnection count in. The host keeps a copy of the text around the
// selection, for the keyboard's many questions, and gets it afresh with the
// answer to each of its edits, and from SetTextInput when the content
// changes it.

// textWindow is how many UTF-16 units either side of the selection the host
// gets: a document may be large, and a keyboard reads around the caret.
const textWindow = 1 << 12

// The hosts' names of what platform.TextTraits numbers.
var (
	keyboardNames    = [...]string{"", "email", "number", "decimal", "phone", "url", "search"}
	returnNames      = [...]string{"", "done", "go", "next", "search", "send"}
	capitalizeNames  = [...]string{"", "none", "words", "sentences", "characters"}
	autocorrectNames = [...]string{"", "on", "off"}
	autofillNames    = [...]string{"", "username", "password", "newPassword", "oneTimeCode", "email", "name", "phone", "address", "postalCode", "creditCard"}
)

// SetTextInput shows the keyboard while an element takes text, and gives the
// host its traits and the text around the caret.
func (s *surface) SetTextInput(t platform.TextInputState) {
	if !t.Active && s.text.Active {
		s.marked = ""
	}
	if t.Client != s.text.Client {
		s.textSerial++ // another input: keyboards forget what they knew
	}
	s.text = t
	s.syncText()
}

// syncText tells the host the state of the text input, if it changed.
func (s *surface) syncText() {
	t := s.text
	st := s.snapshot()
	m := wire.SetTextInput{ID: s.w.id, Active: t.Active, Show: int(t.ShowKeyboard), Serial: s.textSerial, Document: t.Client != nil,
		X: st.X, Y: st.Y, Width: st.Width, Height: st.Height,
		Multiline: t.Traits.Multiline, Secure: t.Traits.Secure, Keyboard: keyboardNames[t.Traits.Keyboard], ReturnKey: returnNames[t.Traits.ReturnKey],
		Capitalize: capitalizeNames[t.Traits.Capitalize], Autocorrect: autocorrectNames[t.Traits.Autocorrect], Autofill: autofillNames[t.Traits.Autofill],
		Text: st.Text, Base: st.Base, Length: st.Length, SelStart: st.SelStart, SelEnd: st.SelEnd, MarkStart: st.MarkStart, MarkEnd: st.MarkEnd}
	if m == s.textSent {
		return
	}
	s.textSent = m
	s.w.b.call(&m, nil)
}

// snapshot is the input's state now, as the host takes it.
func (s *surface) snapshot() wire.TextStateReply {
	t := s.text
	r := wire.TextStateReply{MarkStart: -1, MarkEnd: -1, X: t.Caret.X, Y: t.Caret.Y, Width: t.Caret.W, Height: t.Caret.H}
	c := t.Client
	if !t.Active || c == nil {
		return r
	}
	sel := c.Selection().Range
	r.SelStart, r.SelEnd = sel.Start, sel.End
	if m, ok := c.MarkedRange(); ok {
		r.MarkStart, r.MarkEnd = m.Start, m.End
	}
	// A range past the document's end is taken back to it.
	_, end := c.TextForRange(platform.TextRange{Start: math.MaxInt32, End: math.MaxInt32})
	r.Length = end.End
	var around platform.TextRange
	r.Text, around = c.TextForRange(platform.TextRange{Start: max(0, sel.Start-textWindow), End: sel.End + textWindow})
	r.Base = around.Start
	caret := sel.End
	if b, _, ok := c.BoundsForRange(platform.TextRange{Start: caret, End: caret}); ok {
		r.X, r.Y, r.Width, r.Height = b.X, b.Y, b.W, b.H
	}
	return r
}

// typed hands the keyboard's text to the content without a document, or a
// new line or a tab as its key, which submits a field or moves the focus.
func (s *surface) typed(text string) {
	if s.marked != "" {
		s.marked = ""
		s.send(platform.SurfaceEvent{Kind: platform.TextComposition})
	}
	s.send(platform.SurfaceEvent{Kind: platform.TextInput, Text: text})
}

func (s *surface) key(k platform.Key) {
	s.send(platform.SurfaceEvent{Kind: platform.KeyPressed, Key: k, PhysicalKey: k})
	s.send(platform.SurfaceEvent{Kind: platform.KeyReleased, Key: k, PhysicalKey: k})
}

// replace puts text in a range of the document, or in place of the marked
// text or the selection. A return or a tab typed is its key, as desktop
// keyboards send them.
func (s *surface) replace(r *platform.TextRange, text string) {
	c := s.text.Client
	if r == nil {
		marked := false
		if c != nil {
			_, marked = c.MarkedRange()
		}
		switch {
		case text == "\n" && !marked:
			s.key(platform.KeyEnter)
			return
		case text == "\t" && !marked:
			s.key(platform.KeyTab)
			return
		}
	}
	if c != nil {
		c.ReplaceText(r, text)
		return
	}
	if text != "" {
		s.typed(text)
	}
}

// deleteAround deletes units before and after the selection, which only
// keys reach where the document does not show the host: a password's text.
func (s *surface) deleteAround(before, after int) {
	c := s.text.Client
	if c == nil || s.text.Traits.Secure {
		for range before {
			s.key(platform.KeyBackspace)
		}
		for range after {
			s.key(platform.KeyDelete)
		}
		return
	}
	sel := c.Selection().Range
	if after > 0 {
		c.ReplaceText(&platform.TextRange{Start: sel.End, End: sel.End + after}, "")
	}
	if before > 0 {
		c.ReplaceText(&platform.TextRange{Start: max(0, sel.Start-before), End: sel.Start}, "")
	}
}

func (s *surface) command(cmd string) {
	switch cmd {
	case "newline":
		s.key(platform.KeyEnter)
	case "tab":
		s.key(platform.KeyTab)
	case "deleteBackward":
		s.key(platform.KeyBackspace)
	case "deleteForward":
		s.key(platform.KeyDelete)
	case "left":
		s.key(platform.KeyLeft)
	case "right":
		s.key(platform.KeyRight)
	case "up":
		s.key(platform.KeyUp)
	case "down":
		s.key(platform.KeyDown)
	default:
		s.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: cmd})
	}
}

func rangeOf(has bool, start, end int) *platform.TextRange {
	if !has {
		return nil
	}
	return &platform.TextRange{Start: start, End: end}
}

// textEvent handles an event of the keyboard. Those that edit answer with
// the state after the edit, which the keyboard goes on to read.
func (b *Backend) textEvent(op uint64, r *wire.Reader, w *wire.Writer) {
	sf := func(id int) *surface {
		if win := b.windows[id]; win != nil {
			return win.s
		}
		return nil
	}
	state := func(s *surface) {
		if !wire.EventWaits(op) {
			return
		}
		var st wire.TextStateReply
		if s != nil {
			st = s.snapshot()
			s.acknowledge(st)
		}
		st.Encode(w)
	}
	switch op {
	case wire.EventTextReplace:
		var e wire.TextReplaceEvent
		e.Decode(r)
		s := sf(e.ID)
		if s != nil {
			s.replace(rangeOf(e.HasRange, e.Start, e.End), e.Text)
		}
		state(s)
	case wire.EventTextMark:
		var e wire.TextMarkEvent
		e.Decode(r)
		s := sf(e.ID)
		if s != nil {
			if c := s.text.Client; c != nil {
				c.SetMarkedText(rangeOf(e.HasRange, e.Start, e.End), e.Text, platform.TextRange{Start: e.SelStart, End: e.SelEnd})
			} else {
				s.marked = e.Text
				s.send(platform.SurfaceEvent{Kind: platform.TextComposition, Text: e.Text, Caret: platform.RunesInUTF16(e.Text, e.SelEnd)})
			}
		}
		state(s)
	case wire.EventTextUnmark:
		var e wire.TextUnmarkEvent
		e.Decode(r)
		s := sf(e.ID)
		if s != nil {
			if c := s.text.Client; c != nil {
				c.UnmarkText()
			} else if s.marked != "" {
				text := s.marked
				s.marked = ""
				s.send(platform.SurfaceEvent{Kind: platform.TextComposition})
				s.typed(text)
			}
		}
		state(s)
	case wire.EventTextDelete:
		var e wire.TextDeleteEvent
		e.Decode(r)
		s := sf(e.ID)
		if s != nil {
			s.deleteAround(e.Before, e.After)
		}
		state(s)
	case wire.EventTextSelect:
		var e wire.TextSelectEvent
		e.Decode(r)
		s := sf(e.ID)
		if s != nil {
			if c, ok := s.text.Client.(platform.TextSelectionSetter); ok {
				c.SetTextSelection(platform.TextRange{Start: e.Start, End: e.End})
			}
		}
		state(s)
	case wire.EventTextCommand:
		var e wire.TextCommandEvent
		e.Decode(r)
		s := sf(e.ID)
		if s != nil {
			s.command(e.Command)
		}
		state(s)
	case wire.EventTextState:
		var e wire.TextStateEvent
		e.Decode(r)
		state(sf(e.ID))
	case wire.EventTextRange:
		var e wire.TextRangeEvent
		e.Decode(r)
		reply := wire.TextRangeReply{}
		if s := sf(e.ID); s != nil && s.text.Client != nil {
			var got platform.TextRange
			reply.Text, got = s.text.Client.TextForRange(platform.TextRange{Start: e.Start, End: e.End})
			reply.Start, reply.End = got.Start, got.End
		}
		reply.Encode(w)
	case wire.EventTextBounds:
		var e wire.TextBoundsEvent
		e.Decode(r)
		reply := wire.TextBoundsReply{}
		if s := sf(e.ID); s != nil && s.text.Client != nil {
			if b, got, ok := s.text.Client.BoundsForRange(platform.TextRange{Start: e.Start, End: e.End}); ok {
				reply.OK, reply.X, reply.Y, reply.Width, reply.Height, reply.End = true, b.X, b.Y, b.W, b.H, got.End
			}
		}
		reply.Encode(w)
	case wire.EventTextIndexAt:
		var e wire.TextIndexAtEvent
		e.Decode(r)
		reply := wire.TextIndexAtReply{}
		if s := sf(e.ID); s != nil && s.text.Client != nil {
			reply.Index, reply.OK = s.text.Client.IndexForPoint(e.X, e.Y)
		}
		reply.Encode(w)
	case wire.EventTextInputEnded:
		// The content keeps its selection and the focus as the keyboard goes,
		// as UIKit's text fields do until the app ends editing.
	}
}

// acknowledge notes the state the host gets with an answer, which the next
// frame need not send again.
func (s *surface) acknowledge(st wire.TextStateReply) {
	m := &s.textSent
	m.Text, m.Base, m.Length, m.SelStart, m.SelEnd, m.MarkStart, m.MarkEnd = st.Text, st.Base, st.Length, st.SelStart, st.SelEnd, st.MarkStart, st.MarkEnd
	m.X, m.Y, m.Width, m.Height = st.X, st.Y, st.Width, st.Height
}
