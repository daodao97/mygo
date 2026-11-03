//go:build ios || mygo_ios_input_test

package ios

import (
	"unicode/utf8"

	"github.com/egoist/mygo/internal/platform"
)

// clientInput translates UIKit's bounded text proxy to an indexed client.
// The proxy's base stays fixed throughout an input-method composition.
type clientInput struct {
	client platform.TextInputClient
	base   int
	text   string
	pinned bool
	caret  int
}

func (s *clientInput) bind(client platform.TextInputClient) {
	if s.client != client {
		*s = clientInput{client: client}
	}
}

func (s *clientInput) snapshot(t platform.TextInputState) platform.TextInputState {
	if !s.pinned {
		context := platform.ClientTextContext(s.client)
		selection := platform.NormalizeTextRange(s.client.Selection().Range)
		if _, marked := s.client.MarkedRange(); !marked && (selection.Start < context.Base || selection.End > context.Base+platform.UTF16Len(context.Text)) {
			// UIKit's proxy must represent both handles, including Select All.
			text, actual := s.client.TextForRange(platform.TextRange{Start: max(0, selection.Start-1024), End: selection.End + 1024})
			context.Text, context.Base = text, actual.Start
			context.Start, context.End = selection.Start-actual.Start, selection.End-actual.Start
		}
		s.base, s.text = context.Base, context.Text
		t.Start = utf8.RuneCountInString(s.text[:platform.UTF16ByteOffset(s.text, context.Start)])
		t.End = utf8.RuneCountInString(s.text[:platform.UTF16ByteOffset(s.text, context.End)])
	} else {
		t.Start, t.End = s.caret, s.caret
	}
	t.Text, t.Multiline = s.text, true
	return t
}

func (s *clientInput) index(runeIndex int) int {
	r := []rune(s.text)
	return s.base + platform.UTF16Len(string(r[:max(0, min(runeIndex, len(r)))]))
}

func (s *clientInput) selectRange(start, end int) {
	if client, ok := s.client.(platform.TextInputSelectionClient); ok {
		client.SetSelection(platform.TextSelection{Range: platform.TextRange{Start: s.index(min(start, end)), End: s.index(max(start, end))}, Reversed: start > end})
	}
}

func (s *clientInput) clearPreedit() {
	if _, marked := s.client.MarkedRange(); marked {
		// Discard the old provisional text. A candidate commit is delivered
		// by the following snapshot diff, once, in the same undo transaction.
		s.client.SetMarkedText(nil, "", platform.TextRange{})
	}
}

func (s *clientInput) replaceSnapshot(text string) {
	before, after := []rune(s.text), []rune(text)
	a := 0
	for a < min(len(before), len(after)) && before[a] == after[a] {
		a++
	}
	z, end := len(before), len(after)
	for z > a && end > a && before[z-1] == after[end-1] {
		z--
		end--
	}
	if a != z || a != end {
		r := platform.TextRange{Start: s.index(a), End: s.index(z)}
		s.client.ReplaceText(&r, string(after[a:end]))
	}
	s.text = text
}

func (s *clientInput) commit(text string, caret int) {
	// Client edits can synchronously publish another native-input snapshot.
	// Keep that reentrant snapshot on UIKit's current document window.
	s.pinned, s.caret = true, caret
	s.clearPreedit()
	s.replaceSnapshot(text)
	s.selectRange(caret, caret)
	s.pinned = false
}

func (s *clientInput) compose(text string, start, end, caret int) {
	runes := []rune(text)
	start = max(0, min(start, len(runes)))
	end = max(start, min(end, len(runes)))
	s.pinned, s.caret = true, start
	s.clearPreedit()
	s.replaceSnapshot(string(runes[:start]) + string(runes[end:]))
	s.pinned, s.caret = true, start
	at := s.index(start)
	selected := platform.UTF16Len(string(runes[start:max(start, min(caret, end))]))
	s.client.SetMarkedText(&platform.TextRange{Start: at, End: at}, string(runes[start:end]), platform.TextRange{Start: selected, End: selected})
}

func (s *clientInput) geometry(q platform.TextGeometryQuery) platform.TextGeometry {
	r := platform.TextRange{Start: s.index(min(q.Start, q.End)), End: s.index(max(q.Start, q.End))}
	out := platform.TextGeometry{Valid: true}
	switch q.Kind {
	case "hit":
		index, ok := s.client.IndexForPoint(q.X, q.Y)
		out.Valid = ok
		out.Index = utf8.RuneCountInString(s.text[:platform.UTF16ByteOffset(s.text, max(0, index-s.base))])
	case "caret":
		var ok bool
		out.Caret, _, ok = s.client.BoundsForRange(platform.TextRange{Start: r.Start, End: r.Start})
		out.Valid = ok
	case "selection":
		if client, ok := s.client.(platform.TextInputSelectionGeometryClient); ok {
			out.Rects = client.SelectionRectsForRange(r)
		}
	default:
		out.Valid = false
	}
	return out
}
