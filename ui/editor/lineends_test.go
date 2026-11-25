package editor

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// written returns what b's WriteTo writes.
func written(t *testing.T, b *Buffer) string {
	t.Helper()
	var out strings.Builder
	if _, err := b.WriteTo(&out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// checkEnds checks that b keeps an ending for each of its line breaks,
// and counts them right.
func checkEnds(t *testing.T, b *Buffer) {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	codes := b.ends.codes.String()
	if len(codes) != b.text.LineCount()-1 {
		t.Fatalf("%d line ending codes for %d line breaks", len(codes), b.text.LineCount()-1)
	}
	if b.ends.crlf != strings.Count(codes, "c") || b.ends.cr != strings.Count(codes, "r") {
		t.Fatalf("counts %d, %d of %q", b.ends.crlf, b.ends.cr, codes)
	}
	if strings.Trim(codes, "ncr") != "" || strings.Contains(b.text.String(), "\r") {
		t.Fatalf("codes %q, text %q", codes, b.text.String())
	}
	kinds := 0
	for _, c := range "ncr" {
		if strings.ContainsRune(codes, c) {
			kinds++
		}
	}
	if b.ends.mixed() != (kinds > 1) {
		t.Fatalf("mixed %v for codes %q", b.ends.mixed(), codes)
	}
}

// checkReads checks that what b writes reads back as b's text: the same
// lines, written the same.
func checkReads(t *testing.T, b *Buffer) {
	t.Helper()
	w := written(t, b)
	r := NewBuffer(w)
	if r.LineCount() != b.LineCount() || r.String() != b.String() || written(t, r) != w {
		t.Fatalf("%q, %d lines, writes %q, which reads as %q, %d lines", b.String(), b.LineCount(), w, r.String(), r.LineCount())
	}
}

// A text opened and saved is written as it was read, whatever its line
// endings, and a lone \r breaks a line.
func TestLineEndsRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		in, text, ending string
	}{
		// The issue's three.
		{"first line\nsecond line\r\nthird line\n", "first line\nsecond line\nthird line\n", "\n"},
		{"first line\rsecond line\rthird line\r", "first line\nsecond line\nthird line\n", "\r"},
		{"first line\r\nsecond line\rthird line\r\n", "first line\nsecond line\nthird line\n", "\r\n"},
		{"", "", "\n"},
		{"one line", "one line", "\n"},
		{"a\nb\nc", "a\nb\nc", "\n"},
		{"a\r\nb\r\nc\r\n", "a\nb\nc\n", "\r\n"},
		{"a\r\r\n\n\rb", "a\n\n\n\nb", "\r"},
		{"\n\r", "\n\n", "\n"},
		{"\r\n\n", "\n\n", "\r\n"},
		{"\r", "\n", "\r"},
	} {
		b := NewBuffer(tc.in)
		checkEnds(t, b)
		if got := written(t, b); got != tc.in {
			t.Errorf("NewBuffer(%q) writes %q", tc.in, got)
		}
		if b.String() != tc.text || b.LineCount() != strings.Count(tc.text, "\n")+1 || b.LineEnding() != tc.ending {
			t.Errorf("NewBuffer(%q): text %q, %d lines, ending %q", tc.in, b.String(), b.LineCount(), b.LineEnding())
		}
		b.Reset(tc.in)
		if got := written(t, b); got != tc.in || b.Modified() {
			t.Errorf("Reset(%q) writes %q, modified %v", tc.in, got, b.Modified())
		}
	}
}

// Editing a line changes no other line's ending.
func TestLineEndsEditMixed(t *testing.T) {
	b := NewBuffer("first line\nsecond line\r\nthird line\n")
	b.Edit(Edit{Start: 0, End: 5, Text: "edited"})
	if got := written(t, b); got != "edited line\nsecond line\r\nthird line\n" {
		t.Fatalf("written %q", got)
	}
	b = NewBuffer("first line\r\nsecond line\rthird line\n")
	at := b.Offset(Pos{1, 0})
	b.Edit(Edit{Start: at, End: at + 6, Text: "2nd"})
	if got := written(t, b); got != "first line\r\n2nd line\rthird line\n" {
		t.Fatalf("written %q", got)
	}
	if !b.Undo() || written(t, b) != "first line\r\nsecond line\rthird line\n" || b.Modified() {
		t.Fatalf("after undo %q, modified %v", written(t, b), b.Modified())
	}
}

// A line broken in two ends both halves as it ended; the last line, which
// has no ending, follows the line before it.
func TestLineEndsBreak(t *testing.T) {
	b := NewBuffer("ab\r\ncd\nef\rgh")
	b.Edit(Edit{Start: 1, End: 1, Text: "\n"})           // a|b
	b.Edit(Edit{Start: b.Offset(Pos{2, 1}), Text: "\n"}) // c|d
	b.Edit(Edit{Start: b.Offset(Pos{4, 1}), Text: "\n"}) // e|f
	b.Edit(Edit{Start: b.Offset(Pos{6, 1}), Text: "\n"}) // g|h
	if got, want := written(t, b), "a\r\nb\r\nc\nd\ne\rf\rg\rh"; got != want {
		t.Fatalf("written %q, want %q", got, want)
	}
	checkEnds(t, b)
	// A text with no line break takes the break as written.
	b = NewBuffer("ab")
	b.Edit(Edit{Start: 1, End: 1, Text: "\r\n"})
	if got := written(t, b); got != "a\r\nb" {
		t.Fatalf("written %q", got)
	}
}

// Enter in the editor breaks the line with the line's ending.
func TestLineEndsEnter(t *testing.T) {
	ct := newEditorTest(t, "", "")
	b := NewBuffer("one\r\ntwo\nthree")
	ct.st.Buffer = b
	ct.tt.Frame()
	ct.st.Select(1, 1)
	ct.tt.Frame()
	ct.key(0, ui.KeyEnter)
	ct.st.Select(6, 6) // t|wo
	ct.tt.Frame()
	ct.key(0, ui.KeyEnter)
	if got, want := written(t, b), "o\r\nne\r\nt\nwo\nthree"; got != want {
		t.Fatalf("written %q, want %q", got, want)
	}
	ct.run(Undo)
	ct.run(Undo)
	if got := written(t, b); got != "one\r\ntwo\nthree" || b.Modified() {
		t.Fatalf("after undo %q, modified %v", got, b.Modified())
	}
}

// Deleting a line break keeps the break after it, which ends the line
// left; deleting whole lines leaves the others' endings.
func TestLineEndsDeleteBreak(t *testing.T) {
	b := NewBuffer("a\r\nb\nc\rd")
	b.Edit(Edit{Start: 1, End: 2}) // a's break: ab ends as b did
	if got := written(t, b); got != "ab\nc\rd" {
		t.Fatalf("written %q", got)
	}
	b.Edit(Edit{Start: 2, End: 3}) // b's: abc ends as c did
	if got := written(t, b); got != "abc\rd" {
		t.Fatalf("written %q", got)
	}
	b = NewBuffer("a\r\nb\nc\rd")
	b.Edit(Edit{Start: b.Offset(Pos{1, 0}), End: b.Offset(Pos{2, 0})}) // line b
	if got := written(t, b); got != "a\r\nc\rd" {
		t.Fatalf("written %q", got)
	}
	b.Undo()
	if got := written(t, b); got != "a\r\nb\nc\rd" || b.Modified() {
		t.Fatalf("after undo %q, modified %v", got, b.Modified())
	}
}

// Line breaks pasted take the endings of those they replace, then the
// ending of the line they go in, whatever they were written with.
func TestLineEndsPaste(t *testing.T) {
	b := NewBuffer("a\r\nb\nc\rd")
	b.Edit(Edit{Start: 0, End: 0, Text: "x\ry\r\nz\n"}) // in line a
	if got := written(t, b); got != "x\r\ny\r\nz\r\na\r\nb\nc\rd" {
		t.Fatalf("written %q", got)
	}
	b = NewBuffer("a\r\nb\nc\rd")
	// Lines a to c, two breaks, replaced with three lines, three breaks.
	b.Edit(Edit{Start: 0, End: b.Offset(Pos{2, 1}), Text: "1\r\n2\r\n3\r\n4"})
	if got := written(t, b); got != "1\r\n2\n3\r4\rd" {
		t.Fatalf("written %q", got)
	}
	b.Undo()
	if got := written(t, b); got != "a\r\nb\nc\rd" {
		t.Fatalf("after undo %q", got)
	}
	b.Redo()
	if got := written(t, b); got != "1\r\n2\n3\r4\rd" {
		t.Fatalf("after redo %q", got)
	}
}

// SetLineEnding converts every line, and the lines undo and redo make.
func TestLineEndsConvert(t *testing.T) {
	b := NewBuffer("a\r\nb\nc\rd")
	b.Edit(Edit{Start: 1, End: 2}) // a's break
	b.SetLineEnding("\r\n")
	if got := written(t, b); got != "ab\r\nc\r\nd" || !b.Modified() || b.LineEnding() != "\r\n" {
		t.Fatalf("written %q, modified %v, ending %q", got, b.Modified(), b.LineEnding())
	}
	b.Undo()
	if got := written(t, b); got != "a\r\nb\r\nc\r\nd" {
		t.Fatalf("after undo %q", got)
	}
	b.MarkSaved()
	if b.Modified() {
		t.Fatal("modified after MarkSaved")
	}
	checkEnds(t, b)
}

// MixedLineEndings tells a text whose line breaks all end alike from one
// whose breaks mix endings, as edits, SetLineEnding and undo change them.
func TestLineEndsMixed(t *testing.T) {
	for _, tc := range []struct {
		in     string
		mixed  bool
		ending string
	}{
		{"", false, "\n"},
		{"one line", false, "\n"},
		{"a\n", false, "\n"},
		{"a\nb\nc", false, "\n"},
		{"a\r\nb\r\nc\r\n", false, "\r\n"},
		{"a\rb\rc", false, "\r"},
		{"a\nb\r\nc", true, "\n"},
		{"a\r\nb\r\nc\n", true, "\r\n"},
		{"a\nb\rc", true, "\n"},
		{"a\r\nb\rc", true, "\r\n"},
		{"a\nb\r\nc\rd", true, "\n"},
	} {
		b := NewBuffer(tc.in)
		if b.MixedLineEndings() != tc.mixed || b.LineEnding() != tc.ending {
			t.Errorf("%q: mixed %v, ending %q; want %v, %q", tc.in, b.MixedLineEndings(), b.LineEnding(), tc.mixed, tc.ending)
		}
		checkEnds(t, b)
	}

	// Deleting the one \n of a CRLF text leaves it uniform; undo mixes it
	// again.
	b := NewBuffer("a\r\nb\nc\r\nd")
	if !b.MixedLineEndings() {
		t.Fatal("a text with \\r\\n and \\n is not mixed")
	}
	b.Edit(Edit{Start: b.Offset(Pos{1, 0}), End: b.Offset(Pos{2, 0})}) // line b
	if got := written(t, b); got != "a\r\nc\r\nd" || b.MixedLineEndings() {
		t.Fatalf("written %q, mixed %v", got, b.MixedLineEndings())
	}
	b.Undo()
	if got := written(t, b); got != "a\r\nb\nc\r\nd" || !b.MixedLineEndings() {
		t.Fatalf("after undo %q, mixed %v", got, b.MixedLineEndings())
	}
	b.Redo()
	if b.MixedLineEndings() {
		t.Fatal("mixed after redo")
	}

	// Breaks pasted in a text with no breaks keep their endings: \r\n
	// alone leaves it uniform, \n and \r\n make it mixed, and undo
	// takes them back.
	b = NewBuffer("ab")
	b.Edit(Edit{Start: 1, End: 1, Text: "\r\n"})
	if got := written(t, b); got != "a\r\nb" || b.MixedLineEndings() {
		t.Fatalf("written %q, mixed %v", got, b.MixedLineEndings())
	}
	b = NewBuffer("ab")
	b.Edit(Edit{Start: 1, End: 1, Text: "1\n2\r\n"})
	if got := written(t, b); got != "a1\n2\r\nb" || !b.MixedLineEndings() {
		t.Fatalf("written %q, mixed %v", got, b.MixedLineEndings())
	}
	b.Undo()
	if got := written(t, b); got != "ab" || b.MixedLineEndings() {
		t.Fatalf("after undo %q, mixed %v", got, b.MixedLineEndings())
	}

	// Deleting a break joins its line to the next, which keeps the next
	// one's ending: the last \r\n gone, the text is uniform.
	b = NewBuffer("a\r\nb\nc")
	b.Edit(Edit{Start: 1, End: 2}) // a's break: ab ends as b did, \n
	if got := written(t, b); got != "ab\nc" || b.MixedLineEndings() {
		t.Fatalf("written %q, mixed %v", got, b.MixedLineEndings())
	}
	b.Undo()
	if !b.MixedLineEndings() {
		t.Fatal("not mixed after undo")
	}

	// SetLineEnding makes a mixed text uniform, and lines undo brings back
	// end with its ending, so the text stays uniform.
	b = NewBuffer("a\r\nb\nc\rd")
	b.Edit(Edit{Start: 1, End: 2}) // a's break
	if !b.MixedLineEndings() {
		t.Fatal("not mixed")
	}
	b.SetLineEnding("\r")
	if got := written(t, b); got != "ab\rc\rd" || b.MixedLineEndings() || b.LineEnding() != "\r" {
		t.Fatalf("written %q, mixed %v, ending %q", got, b.MixedLineEndings(), b.LineEnding())
	}
	b.Undo()
	if got := written(t, b); got != "a\rb\rc\rd" || b.MixedLineEndings() {
		t.Fatalf("after undo %q, mixed %v", got, b.MixedLineEndings())
	}
	checkEnds(t, b)
}

// Random edits, with breaks of every kind, keep an ending for each line,
// and undo and redo make the texts they were, byte for byte.
func TestLineEndsUndoRandom(t *testing.T) {
	r := rand.New(rand.NewPCG(17, 95))
	pieces := []string{"", "x", "yz", "\n", "\r\n", "\r", "a\rb\r\nc\nd", "\n\n", "\r\r"}
	for round := range 40 {
		orig := ""
		for range r.IntN(12) {
			orig += pieces[r.IntN(len(pieces))]
		}
		b := NewBuffer(orig)
		var states []string // the text before each step of undo
		for range 1 + r.IntN(15) {
			before, v := written(t, b), b.Version()
			n := b.Len()
			at := r.IntN(n + 1)
			end := min(n, at+r.IntN(5))
			edits := []Edit{{Start: at, End: end, Text: pieces[r.IntN(len(pieces))]}}
			if r.IntN(3) == 0 && end < n {
				a2 := end + r.IntN(n-end+1)
				edits = append(edits, Edit{Start: a2, End: min(n, a2+r.IntN(3)), Text: pieces[r.IntN(len(pieces))]})
			}
			b.Edit(edits...)
			if b.Version() != v {
				states = append(states, before)
			}
			checkEnds(t, b)
			checkReads(t, b)
		}
		last := written(t, b)
		for i := len(states) - 1; i >= 0; i-- {
			if !b.Undo() {
				break
			}
			if got := written(t, b); got != states[i] {
				t.Fatalf("round %d: undo to %q, want %q", round, got, states[i])
			}
			checkEnds(t, b)
			checkReads(t, b)
		}
		if b.CanUndo() || b.Modified() || written(t, b) != orig {
			t.Fatalf("round %d: undone to %q, want %q", round, written(t, b), orig)
		}
		for b.Redo() {
		}
		if got := written(t, b); got != last {
			t.Fatalf("round %d: redo to %q, want %q", round, got, last)
		}
		checkEnds(t, b)
		// A remote edit moves the history past it, endings and all.
		b.EditRemote(Edit{Start: 0, End: 0, Text: "r\r\n"})
		w := written(t, b)
		if !strings.HasSuffix(w, last) {
			t.Fatalf("round %d: remote edit made %q of %q", round, w, last)
		}
		for b.Undo() {
			checkEnds(t, b)
			checkReads(t, b)
		}
		if got, want := written(t, b), w[:len(w)-len(last)]+orig; got != want {
			t.Fatalf("round %d: undone past a remote edit to %q, want %q", round, got, want)
		}
	}
}

// A line ending in \r followed by an empty line ending in \n would be
// written \r\n, one line break: the empty line's break becomes \r\n,
// and undo puts it back.
func TestLineEndsCRBeforeEmptyLine(t *testing.T) {
	b := NewBuffer("a\rb\nc")
	b.Edit(Edit{Start: 2, End: 3}) // b
	if got := written(t, b); got != "a\r\r\nc" || b.LineCount() != 3 {
		t.Fatalf("written %q, %d lines", got, b.LineCount())
	}
	checkReads(t, b)
	b.Undo()
	if got := written(t, b); got != "a\rb\nc" || b.Modified() {
		t.Fatalf("after undo %q, modified %v", got, b.Modified())
	}
	b.Redo()
	if got := written(t, b); got != "a\r\r\nc" {
		t.Fatalf("after redo %q", got)
	}
	// Enter at the start of a line ending in \n, after one ending in \r.
	b = NewBuffer("a\rb\n")
	b.Edit(Edit{Start: 2, End: 2, Text: "\n"})
	if got := written(t, b); got != "a\r\r\nb\n" {
		t.Fatalf("written %q", got)
	}
	checkReads(t, b)
	b.Undo()
	if got := written(t, b); got != "a\rb\n" || b.Modified() {
		t.Fatalf("after undo %q, modified %v", got, b.Modified())
	}
	// Empty lines in a row: only the first changes.
	b = NewBuffer("a\rb\n\n\nc")
	b.Edit(Edit{Start: 2, End: 3})
	if got := written(t, b); got != "a\r\r\n\n\nc" {
		t.Fatalf("written %q", got)
	}
	checkReads(t, b)
}

// Moving lines up and down moves their endings with them.
func TestLineEndsMoveLines(t *testing.T) {
	ct := newEditorTest(t, "", "")
	b := NewBuffer("a\r\nb\nc\rd")
	ct.st.Buffer = b
	ct.tt.Frame()
	// One at a time, checking each.
	move := func(start, end int, cmd Command, want string) {
		t.Helper()
		ct.st.Select(start, end)
		ct.tt.Frame()
		ct.run(cmd)
		if got := written(t, b); got != want {
			t.Fatalf("%v: written %q, want %q", cmd, got, want)
		}
		checkEnds(t, b)
		checkReads(t, b)
	}
	move(0, 0, MoveLinesDown, "b\na\r\nc\rd")
	move(2, 2, MoveLinesDown, "b\nc\ra\r\nd")
	// The last line has no ending: d takes the one of the line it passes.
	move(4, 4, MoveLinesDown, "b\nc\rd\r\na")
	// Two lines, b and c, down past d.
	move(0, 3, MoveLinesDown, "d\r\nb\nc\ra")
	move(2, 2, MoveLinesUp, "b\nd\r\nc\ra")
	for range 5 {
		ct.run(Undo)
	}
	if got := written(t, b); got != "a\r\nb\nc\rd" || b.Modified() {
		t.Fatalf("after undo %q, modified %v", got, b.Modified())
	}
	for range 5 {
		ct.run(Redo)
	}
	if got := written(t, b); got != "b\nd\r\nc\ra" {
		t.Fatalf("after redo %q", got)
	}
}
