package richtext

import (
	"bytes"
	"fmt"
	"github.com/egoist/mygo/transfer"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// richTester builds a RichTextEditor of doc filling a window, focused at
// the end of its text.
func richTester(t *testing.T, doc *Document) (*ui.Tester, *bool) {
	t.Helper()
	changed := new(bool)
	tt := ui.NewTester(func(c *ui.Context) {
		if Editor(c, doc).Fill().Changed() {
			*changed = true
		}
	}, 500, 400)
	tt.Press(250, 380)
	tt.Release(250, 380)
	tt.Key(ui.Cmd, ui.KeyDown)
	return tt, changed
}

// runsOf describes the runs of a text: their text, marked with what they
// are, as "plain|*bold*|_italic_|[link]".
func runsOf(at Text) string {
	var parts []string
	var last Attributes
	for i, r := range at.Runs(0, at.Len()) {
		if i > 0 && r.Attributes == last && len(parts) > 0 {
			// A run of the next paragraph, as the one before.
			parts[len(parts)-1] = joinRun(parts[len(parts)-1], r.Text)
			continue
		}
		last = r.Attributes
		s := r.Text
		a := r.Attributes
		if a.Bold() {
			s = "*" + s + "*"
		}
		if a.Italic {
			s = "_" + s + "_"
		}
		if a.Underline {
			s = "~" + s + "~"
		}
		if a.Code {
			s = "`" + s + "`"
		}
		if a.Link != "" {
			s = "[" + s + "]"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "|")
}

// joinRun puts text inside the marks of a run runsOf described.
func joinRun(run, text string) string {
	i := strings.LastIndexFunc(run, func(r rune) bool { return !strings.ContainsRune("*_~`]", r) })
	return run[:i+1] + text + run[i+1:]
}

// typeRunes types s a rune at a time, as a keyboard does.
func typeRunes(tt *ui.Tester, s string) {
	for _, r := range s {
		tt.Type(string(r))
	}
}

// The caret blinks by the frames' time, and shows again as it moves.
func TestRichTextCaretBlinks(t *testing.T) {
	const blink = 530 * time.Millisecond
	tt, _ := richTester(t, NewDocument(Text{}))
	tt.Type("hello")
	shown := bytes.Clone(tt.Image().Pix)
	tt.Advance(blink)
	if bytes.Equal(tt.Image().Pix, shown) {
		t.Fatal("the caret still shows a blink on")
	}
	tt.Key(0, ui.KeyLeft)
	tt.Key(0, ui.KeyRight)
	if !bytes.Equal(tt.Image().Pix, shown) {
		t.Error("the caret does not show as it moved")
	}
}

func TestRichTextTyping(t *testing.T) {
	doc := NewDocument(Text{})
	tt, changed := richTester(t, doc)
	tt.Type("plain ")
	tt.Key(ui.Cmd, ui.KeyB)
	tt.Type("bold")
	tt.Key(ui.Cmd, ui.KeyB)
	tt.Type(" more")
	if got := runsOf(doc.Text()); got != "plain |*bold*| more" {
		t.Fatalf("runs %q", got)
	}
	if !*changed {
		t.Fatal("typing reports no change")
	}
	// Typing at the end of bold continues it; at its start, it does not.
	doc.SetSelection(10, 10)
	tt.Frame()
	tt.Type("er")
	doc.SetSelection(6, 6)
	tt.Frame()
	tt.Type("x")
	if got := runsOf(doc.Text()); got != "plain x|*bolder*| more" {
		t.Fatalf("runs %q", got)
	}
}

func TestRichTextLinkEdges(t *testing.T) {
	doc := NewDocument(TextOf(Paragraph{Runs: []Run{
		{Text: "see "}, {Text: "docs", Attributes: Attributes{Link: "https://x"}}, {Text: " now"},
	}}))
	tt, _ := richTester(t, doc)
	doc.SetSelection(8, 8)
	tt.Frame()
	tt.Type("!")
	doc.SetSelection(6, 6)
	tt.Frame()
	tt.Type("o")
	if got := runsOf(doc.Text()); got != "see |[doocs]|! now" {
		t.Fatalf("runs %q", got)
	}
}

func TestRichTextCaretFormatIsNoStep(t *testing.T) {
	doc := NewDocument(NewText("hello"))
	tt, _ := richTester(t, doc)
	tt.Key(ui.Cmd, ui.KeyB)
	if doc.CanUndo() {
		t.Fatal("⌘B at a caret made a step of undo")
	}
	if f := doc.Formatting(); f.Bold != FormatOn {
		t.Fatalf("typing attributes not bold: %+v", f)
	}
	// Moving the caret forgets them.
	tt.Key(0, ui.KeyLeft)
	if doc.Formatting().Bold != FormatOff {
		t.Fatal("moving the caret kept the typing attributes")
	}
}

// Backspace takes a combining mark, or sara am, alone, and an emoji
// sequence whole.
func TestRichTextBackspaceMarks(t *testing.T) {
	for _, c := range []struct{ s, want string }{
		{"aนี้", "aนี"},
		{"ทำ", "ท"},
		{"❤️", ""},
	} {
		doc := NewDocument(TextOf(Paragraph{Runs: []Run{{Text: c.s}}}))
		tt, _ := richTester(t, doc)
		n := doc.Text().Len()
		doc.SetSelection(n, n)
		tt.Frame()
		tt.Key(0, ui.KeyBackspace)
		if got := doc.Text().String(); got != c.want {
			t.Errorf("backspace in %q leaves %q, want %q", c.s, got, c.want)
		}
	}
}

func TestRichTextUndoRestoresAttributes(t *testing.T) {
	doc := NewDocument(TextOf(Paragraph{Runs: []Run{
		{Text: "a "}, {Text: "bold", Attributes: Attributes{Weight: 700}}, {Text: " word"},
	}}))
	tt, _ := richTester(t, doc)
	before := doc.Text()
	doc.SetSelection(2, 6)
	tt.Frame()
	tt.Key(0, ui.KeyBackspace)
	if got := doc.Text().String(); got != "a  word" {
		t.Fatalf("deleted to %q", got)
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if !doc.Text().Equal(before) {
		t.Fatalf("undo made %q", runsOf(doc.Text()))
	}
	if a, c := doc.Selection(); a != 2 || c != 6 {
		t.Fatalf("undo selected %d–%d", a, c)
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
	if got := doc.Text().String(); got != "a  word" {
		t.Fatalf("redo made %q", got)
	}
}

func TestRichTextFormatUndo(t *testing.T) {
	doc := NewDocument(NewText("one two"))
	tt, _ := richTester(t, doc)
	doc.SetSelection(4, 7)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyI)
	tt.Key(ui.Cmd, ui.KeyU)
	if got := runsOf(doc.Text()); got != "one |~_two_~" {
		t.Fatalf("runs %q", got)
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if got := runsOf(doc.Text()); got != "one |_two_" {
		t.Fatalf("after one undo %q", got)
	}
	if a, c := doc.Selection(); a != 4 || c != 7 {
		t.Fatalf("undo selected %d–%d", a, c)
	}
}

func TestRichTextLists(t *testing.T) {
	doc := NewDocument(Text{})
	tt, _ := richTester(t, doc)
	typeRunes(tt, "- milk")
	tt.Key(0, ui.KeyEnter)
	typeRunes(tt, "eggs")
	tt.Key(0, ui.KeyEnter)
	at := doc.Text()
	if got := at.String(); got != "milk\neggs\n" {
		t.Fatalf("text %q", got)
	}
	for i, want := range []ListKind{Bullets, Bullets, Bullets} {
		if got := at.ParagraphStyleAt(at.paraStart(i)).List; got != want {
			t.Fatalf("paragraph %d: list %d, want %d", i, got, want)
		}
	}
	// Enter on an empty item leaves the list.
	tt.Key(0, ui.KeyEnter)
	at = doc.Text()
	if at.String() != "milk\neggs\n" || at.ParagraphStyleAt(at.Len()).List != NoList {
		t.Fatalf("%q %+v", at.String(), at.ParagraphStyleAt(at.Len()))
	}
	// Tab indents an item, Backspace at its start outdents it, then takes
	// it out of the list.
	doc.SetSelection(5, 5)
	tt.Frame()
	tt.Key(0, ui.KeyTab)
	if got := doc.Text().ParagraphStyleAt(5).Indent; got != 1 {
		t.Fatalf("indent %d", got)
	}
	tt.Key(0, ui.KeyBackspace)
	tt.Key(0, ui.KeyBackspace)
	if st := doc.Text().ParagraphStyleAt(5); st.List != NoList || st.Indent != 0 {
		t.Fatalf("style %+v", st)
	}
}

func TestRichTextHeadings(t *testing.T) {
	doc := NewDocument(Text{})
	tt, _ := richTester(t, doc)
	typeRunes(tt, "## Title")
	tt.Key(0, ui.KeyEnter)
	typeRunes(tt, "body")
	at := doc.Text()
	if at.String() != "Title\nbody" || at.ParagraphStyleAt(0).Heading != 2 || at.ParagraphStyleAt(7).Heading != 0 {
		t.Fatalf("%q %+v %+v", at.String(), at.ParagraphStyleAt(0), at.ParagraphStyleAt(7))
	}
	// Undo takes the shortcut back to what was typed.
	doc2 := NewDocument(Text{})
	tt2, _ := richTester(t, doc2)
	typeRunes(tt2, "# ")
	tt2.Key(ui.Cmd, ui.KeyZ)
	if got := doc2.Text(); got.String() != "# " || got.ParagraphStyleAt(0).Heading != 0 {
		t.Fatalf("undo of the shortcut: %q", got.String())
	}
}

func TestRichTextInlineShortcuts(t *testing.T) {
	doc := NewDocument(Text{})
	tt, _ := richTester(t, doc)
	typeRunes(tt, "a **big** and `x` then")
	if got := runsOf(doc.Text()); got != "a |*big*| and |`x`| then" {
		t.Fatalf("runs %q", got)
	}
}

func TestRichTextCompose(t *testing.T) {
	doc := NewDocument(NewText("ab"))
	tt, _ := richTester(t, doc)
	tt.Key(ui.Cmd, ui.KeyB)
	tt.Compose("ni", 2)
	tt.Compose("你", 1)
	tt.Compose("", 0)
	tt.Type("你")
	if got := runsOf(doc.Text()); got != "ab|*你*" {
		t.Fatalf("runs %q", got)
	}
}

func TestRichTextAppCommands(t *testing.T) {
	doc := NewDocument(NewText("first\nsecond"))
	tt, _ := richTester(t, doc)
	doc.SetSelection(0, 3)
	doc.ToggleBold()
	doc.SetHeading(1)
	tt.Frame()
	at := doc.Text()
	if runsOf(at) != "*fir*|st\nsecond" || at.ParagraphStyleAt(0).Heading != 1 {
		t.Fatalf("%q %+v", runsOf(at), at.ParagraphStyleAt(0))
	}
	// The editor follows: typing replaces the selection the app set.
	tt.Type("X")
	if got := runsOf(doc.Text()); got != "*X*|st\nsecond" {
		t.Fatalf("runs %q", got)
	}
}

func TestRichTextFormatMenu(t *testing.T) {
	doc := NewDocument(NewText("one two"))
	tt, _ := richTester(t, doc)
	doc.SetSelection(0, 3)
	tt.Frame()
	tt.Command("bold")
	tt.Command("alignCenter")
	at := doc.Text()
	if runsOf(at) != "*one*| two" || at.ParagraphStyleAt(0).Align != ui.Center {
		t.Fatalf("%q %+v", runsOf(at), at.ParagraphStyleAt(0))
	}
}

// TestRichTextEdgesAsNSTextView checks the edges a Swift probe measured of
// NSTextView on macOS 27: Enter and Backspace after bold keep typing bold,
// a link extends only inside, never at its start.
func TestRichTextEdgesAsNSTextView(t *testing.T) {
	bold := Attributes{Weight: 700}
	link := Attributes{Link: "https://x"}
	cases := []struct {
		name  string
		text  Text
		caret int
		keys  func(tt *ui.Tester)
		want  string
	}{
		{"enter at the end of bold", TextOf(Paragraph{Runs: []Run{{Text: "plain "}, {Text: "bold", Attributes: bold}}}, Paragraph{Runs: []Run{{Text: "next"}}}), 10,
			func(tt *ui.Tester) { tt.Key(0, ui.KeyEnter); typeRunes(tt, "x") }, "plain |*bold\nx\n*|next"},
		{"backspace a bold character", TextOf(Paragraph{Runs: []Run{{Text: "plain "}, {Text: "B", Attributes: bold}}}), 7,
			func(tt *ui.Tester) { tt.Key(0, ui.KeyBackspace); typeRunes(tt, "x") }, "plain |*x*"},
		{"the start of a link", TextOf(Paragraph{Runs: []Run{{Text: "see "}, {Text: "docs", Attributes: link}}}), 4,
			func(tt *ui.Tester) { typeRunes(tt, "x") }, "see x|[docs]"},
		{"a link starting a paragraph", TextOf(Paragraph{Runs: []Run{{Text: "docs", Attributes: link}, {Text: " now"}}}), 0,
			func(tt *ui.Tester) { typeRunes(tt, "x") }, "x|[docs]| now"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := NewDocument(c.text)
			tt, _ := richTester(t, doc)
			doc.SetSelection(c.caret, c.caret)
			tt.Frame()
			c.keys(tt)
			if got := runsOf(doc.Text()); got != c.want {
				t.Fatalf("runs %q, want %q", got, c.want)
			}
		})
	}
}

// TestRichTextTypingIsOneStep types a word and undoes it whole, as
// NSTextView and TextArea do.
func TestRichTextTypingIsOneStep(t *testing.T) {
	doc := NewDocument(NewText(""))
	tt, _ := richTester(t, doc)
	typeRunes(tt, "abc")
	tt.Key(ui.Cmd, ui.KeyZ)
	if got := doc.Text().String(); got != "" {
		t.Fatalf("one undo left %q", got)
	}
}

// TestRichTextCursor checks the pointer's shape: an arrow over a task's
// box, which a click checks, an I-beam over text and a hand over links.
func TestRichTextCursor(t *testing.T) {
	doc := NewDocument(TextOf(
		Paragraph{Runs: []Run{{Text: "milk"}}, Style: ParagraphStyle{List: Tasks}},
		Paragraph{Runs: []Run{{Text: "see "}, {Text: "the site", Attributes: Attributes{Link: "https://example.com"}}}},
	))
	tt, _ := richTester(t, doc)
	find := func(want ui.Cursor) (float32, float32, bool) {
		for y := float32(2); y < 80; y += 2 {
			for x := float32(0); x < 200; x += 2 {
				tt.Move(x, y)
				if tt.Cursor() == want {
					return x, y, true
				}
			}
		}
		return 0, 0, false
	}
	x, y, ok := find(ui.CursorDefault)
	if !ok {
		t.Fatal("no arrow over the task's box")
	}
	tt.ClickAt(x, y)
	if !doc.Text().ParagraphStyleAt(0).Checked {
		t.Fatalf("a click where the arrow shows doesn't check the task")
	}
	tt.Move(x+60, y)
	if got := tt.Cursor(); got != ui.CursorText {
		t.Fatalf("cursor over the task's text %v, want an I-beam", got)
	}
	if _, _, ok := find(ui.CursorPointer); !ok {
		t.Fatal("no hand over the link")
	}
}

// TestRichTextComposeUndo checks that undo takes back what an input method
// committed at a caret, not the typing before it too.
func TestRichTextComposeUndo(t *testing.T) {
	doc := NewDocument(Text{})
	tt, _ := richTester(t, doc)
	typeRunes(tt, "hello")
	tt.Key(0, ui.KeyLeft)
	tt.Key(0, ui.KeyRight)
	tt.Compose("ni", 2)
	tt.Type("你")
	if got := doc.Text().String(); got != "hello你" {
		t.Fatalf("text %q", got)
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if got := doc.Text().String(); got != "hello" {
		t.Fatalf("after undo %q, want %q", got, "hello")
	}
}

// TestRichTextComposeDropped checks that a composition the app's selection
// drops leaves the caret where the app put it.
func TestRichTextComposeDropped(t *testing.T) {
	doc := NewDocument(Text{})
	tt, _ := richTester(t, doc)
	start, _ := tt.TextCaret()
	typeRunes(tt, "abc")
	tt.Compose("nihao", 5)
	doc.SetSelection(0, 0)
	tt.Frame()
	if got, _ := tt.TextCaret(); got.X != start.X {
		t.Fatalf("caret at x %v, want %v", got.X, start.X)
	}
	tt.Type("z")
	if got := doc.Text().String(); got != "zabc" {
		t.Fatalf("text %q", got)
	}
}

// TestRichTextReplaceCRLF checks that what an input method puts in with
// \r\n line endings goes in with \n, as typed and pasted text does: a
// composition committed, a range replaced, and a composition left as it
// is.
func TestRichTextReplaceCRLF(t *testing.T) {
	doc := NewDocument(Text{})
	tt, _ := richTester(t, doc)
	typeRunes(tt, "ab")
	tt.Compose("x", 1)
	tt.Type("c\r\nd")
	if got := doc.Text().String(); got != "abc\nd" {
		t.Fatalf("committed %q", got)
	}
	v := doc.shownIn.Value()
	v.ReplaceText(&ui.TextInputRange{Start: 0, End: 1}, "e\r\nf")
	tt.Frame()
	if got := doc.Text().String(); got != "e\nfbc\nd" {
		t.Fatalf("replaced %q", got)
	}
	if _, caret := doc.Selection(); caret != 3 {
		t.Fatalf("caret at %d, want 3", caret)
	}
	tt.Compose("g\r\nh", 4)
	v.UnmarkText()
	tt.Frame()
	if got := doc.Text().String(); got != "e\nfg\nhbc\nd" {
		t.Fatalf("left as it is %q", got)
	}
	if n := doc.Text().ParagraphCount(); n != 4 {
		t.Fatalf("%d paragraphs, want 4", n)
	}
}

// TestRichTextSetMarkedEmpty checks that SetMarkedText with an empty string
// and nil range from idle is a no-op (leaving selection and undo intact), and
// that clearing preedit before UnmarkText and ReplaceText (GTK commit order)
// joins the selection-replacement undo step.
func TestRichTextSetMarkedEmpty(t *testing.T) {
	checkCleared := func(t *testing.T, v *view, label string) {
		t.Helper()
		if v.composing || v.compose != "" || v.composeCaret != 0 || v.composeSelected != (ui.TextInputRange{}) {
			t.Fatalf("%s: composition state not cleared: composing=%v compose=%q caret=%d sel=%+v",
				label, v.composing, v.compose, v.composeCaret, v.composeSelected)
		}
		if mr, marked := v.MarkedRange(); marked {
			t.Fatalf("%s: MarkedRange() = %+v, true; want false", label, mr)
		}
	}

	t.Run("idle", func(t *testing.T) {
		doc := NewDocument(Text{})
		tt, _ := richTester(t, doc)
		typeRunes(tt, "hello")
		v := doc.shownIn.Value()
		v.SetMarkedText(nil, "", ui.TextInputRange{})
		tt.Frame()
		checkCleared(t, v, "idle")
		if got := doc.Text().String(); got != "hello" {
			t.Fatalf("text = %q, want %q", got, "hello")
		}
		tt.Key(ui.Cmd, ui.KeyZ)
		if got := doc.Text().String(); got != "" {
			t.Fatalf("after one undo = %q, want empty", got)
		}
	})

	t.Run("idle with active selection", func(t *testing.T) {
		doc := NewDocument(Text{})
		tt, _ := richTester(t, doc)
		typeRunes(tt, "hello")
		doc.SetSelection(1, 4)
		tt.Frame()
		v := doc.shownIn.Value()
		v.SetMarkedText(nil, "", ui.TextInputRange{})
		tt.Frame()
		checkCleared(t, v, "idle with active selection")
		if got := doc.Text().String(); got != "hello" {
			t.Fatalf("text = %q, want %q", got, "hello")
		}
		if a, c := doc.Selection(); a != 1 || c != 4 {
			t.Fatalf("selection = (%d, %d), want (1, 4)", a, c)
		}
		tt.Key(ui.Cmd, ui.KeyZ)
		if got := doc.Text().String(); got != "" {
			t.Fatalf("after one undo = %q, want empty", got)
		}
	})

	t.Run("GTK commit order", func(t *testing.T) {
		doc := NewDocument(Text{})
		tt, _ := richTester(t, doc)
		typeRunes(tt, "hello")
		doc.SetSelection(1, 4)
		tt.Frame()
		v := doc.shownIn.Value()
		v.SetMarkedText(nil, "ni", ui.TextInputRange{Start: 2, End: 2})
		v.SetMarkedText(nil, "", ui.TextInputRange{})
		v.UnmarkText()
		v.ReplaceText(nil, "你")
		tt.Frame()
		checkCleared(t, v, "GTK commit order")
		if got := doc.Text().String(); got != "h你o" {
			t.Fatalf("text = %q, want %q", got, "h你o")
		}
		if a, c := doc.Selection(); a != 2 || c != 2 {
			t.Fatalf("selection = (%d, %d), want (2, 2)", a, c)
		}
		tt.Key(ui.Cmd, ui.KeyZ)
		if got := doc.Text().String(); got != "hello" {
			t.Fatalf("after one undo = %q, want %q", got, "hello")
		}
		if a, c := doc.Selection(); a != 1 || c != 4 {
			t.Fatalf("after one undo selection = (%d, %d), want (1, 4)", a, c)
		}
	})
}

// TestRichTextEnterAtCheckedTask checks that Enter at the start of a
// checked task keeps it checked, with an empty item above, as Notes.
func TestRichTextEnterAtCheckedTask(t *testing.T) {
	doc := NewDocument(TextOf(Paragraph{Runs: []Run{{Text: "done"}}, Style: ParagraphStyle{List: Tasks, Checked: true}}))
	tt, _ := richTester(t, doc)
	doc.SetSelection(0, 0)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	at := doc.Text()
	if at.String() != "\ndone" || at.ParagraphStyleAt(0).Checked || !at.ParagraphStyleAt(1).Checked {
		t.Fatalf("%q: checked %v, %v", at.String(), at.ParagraphStyleAt(0).Checked, at.ParagraphStyleAt(1).Checked)
	}
}

// TestRichTextTaskBoxOnly checks that a click in the margin away from a
// task's box doesn't check it.
func TestRichTextTaskBoxOnly(t *testing.T) {
	doc := NewDocument(TextOf(Paragraph{Runs: []Run{{Text: "todo"}}, Style: ParagraphStyle{List: Tasks}}))
	tt, _ := richTester(t, doc)
	for x := float32(0); x < 40; x += 2 {
		tt.ClickAt(x, 300)
	}
	if doc.Text().ParagraphStyleAt(0).Checked {
		t.Fatal("a click below the task checked it")
	}
}

// TestRichTextUnderscoreInWord checks that snake_case stays as typed.
func TestRichTextUnderscoreInWord(t *testing.T) {
	doc := NewDocument(Text{})
	tt, _ := richTester(t, doc)
	typeRunes(tt, "snake_case_name and _this_")
	if got := runsOf(doc.Text()); got != "snake_case_name and |_this_" {
		t.Fatalf("runs %q", got)
	}
}

// TestRichTextCodeBlockSpace checks that a code block's last line gets its
// space below back when the line after it leaves the block.
func TestRichTextCodeBlockSpace(t *testing.T) {
	doc := NewDocument(TextOf(
		Paragraph{Runs: []Run{{Text: "code"}}, Style: ParagraphStyle{CodeBlock: true}},
		Paragraph{Style: ParagraphStyle{CodeBlock: true}},
		Paragraph{Runs: []Run{{Text: "after"}}},
	))
	tt, _ := richTester(t, doc)
	doc.SetSelection(5, 5)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	doc.SetSelection(doc.Text().Len(), doc.Text().Len())
	tt.Frame()
	got, _ := tt.TextCaret()
	fresh := NewDocument(doc.Text())
	ft, _ := richTester(t, fresh)
	want, _ := ft.TextCaret()
	if got.Y != want.Y {
		t.Fatalf("last line at y %v, want %v as laid out afresh", got.Y, want.Y)
	}
}

// TestRichTextParagraphKeys checks moving by paragraphs: ⌥↑↓ on macOS,
// Ctrl+↑↓ elsewhere.
func TestRichTextParagraphKeys(t *testing.T) {
	doc := NewDocument(NewText("one two\nthree four\nfive"))
	tt, _ := richTester(t, doc)
	mod := ui.Ctrl
	if runtime.GOOS == "darwin" {
		mod = ui.Alt
	}
	doc.SetSelection(12, 12)
	tt.Frame()
	tt.Key(mod, ui.KeyUp)
	if _, c := doc.Selection(); c != 8 {
		t.Fatalf("up to %d, want 8", c)
	}
	tt.Key(mod, ui.KeyUp)
	if _, c := doc.Selection(); c != 0 {
		t.Fatalf("up again to %d, want 0", c)
	}
	tt.Key(mod, ui.KeyDown)
	want := 7 // the end of the paragraph on macOS
	if runtime.GOOS != "darwin" {
		want = 8 // the start of the next elsewhere
	}
	if _, c := doc.Selection(); c != want {
		t.Fatalf("down to %d, want %d", c, want)
	}
}

// Up and Down go to the line above and below however much room there is
// between them, a heading's margin or Space, though it is over half a
// line: the caret never stays where it was.
func TestRichTextVerticalPastRoom(t *testing.T) {
	doc := NewDocument(NewText("one\ntwo\nthree"))
	theme := &Theme{Headings: [6]HeadingStyle{{Size: 1.5, Top: 2}}}
	tt := ui.NewTester(func(c *ui.Context) {
		Editor(c, doc, EditorOptions{Theme: theme, Space: func(p int) float32 {
			if p == 1 {
				return 40
			}
			return 0
		}}).Fill()
	}, 500, 400)
	tt.Press(250, 380)
	tt.Release(250, 380)
	doc.SetSelection(5, 5) // in "two"
	tt.Frame()
	for _, step := range []struct {
		key  ui.Key
		para int
	}{{ui.KeyUp, 0}, {ui.KeyDown, 1}, {ui.KeyDown, 2}, {ui.KeyUp, 1}} {
		tt.Key(0, step.key)
		if _, c := doc.Selection(); doc.Text().paraAt(c) != step.para {
			t.Fatalf("past Space: %v to %d, want paragraph %d", step.key, c, step.para)
		}
	}
	doc.SetSelection(9, 9) // in "three"
	doc.SetHeading(1)
	tt.Frame()
	tt.Key(0, ui.KeyUp)
	if _, c := doc.Selection(); doc.Text().paraAt(c) != 1 {
		t.Fatalf("up from a heading with a wide margin to %d, want paragraph 1", c)
	}
}

// TestRichTextVerticalCollapsesSelection checks that a plain Up or Down
// collapses a non-empty selection to its start (up) or end (down) before
// moving vertically, while Shift+Up/Down keeps extending from the caret.
func TestRichTextVerticalCollapsesSelection(t *testing.T) {
	doc := NewDocument(NewText("one\ntwo\nthree\nfour"))
	tt, _ := richTester(t, doc)

	// Backward selection from paragraph 2 ("th|ree") to paragraph 0 ("o|ne"):
	// plain Down collapses to the selection end in paragraph 2 and steps to paragraph 3.
	doc.SetSelection(10, 1)
	tt.Frame()
	tt.Key(0, ui.KeyDown)
	if a, c := doc.Selection(); a != c || doc.Text().paraAt(c) != 3 {
		t.Fatalf("backward selection + Down = (%d, %d) in paragraph %d, want collapsed in paragraph 3", a, c, doc.Text().paraAt(c))
	}

	// Forward selection from paragraph 1 ("t|wo") to paragraph 3 ("fo|ur"):
	// plain Up collapses to the selection start in paragraph 1 and steps to paragraph 0.
	doc.SetSelection(5, 16)
	tt.Frame()
	tt.Key(0, ui.KeyUp)
	if a, c := doc.Selection(); a != c || doc.Text().paraAt(c) != 0 {
		t.Fatalf("forward selection + Up = (%d, %d) in paragraph %d, want collapsed in paragraph 0", a, c, doc.Text().paraAt(c))
	}

	// Same-line backward selection on a uniform line: plain Down collapses
	// to the end of the selection so x stays at the line end, and plain
	// Up on a forward selection collapses to the start so x stays at 0.
	sameDoc := NewDocument(NewText("aaa\naaa\naaa"))
	st, _ := richTester(t, sameDoc)
	sameDoc.SetSelection(7, 4)
	st.Frame()
	st.Key(0, ui.KeyDown)
	if a, c := sameDoc.Selection(); a != c || c != 11 {
		t.Fatalf("same-line backward selection + Down = (%d, %d), want (11, 11)", a, c)
	}
	sameDoc.SetSelection(4, 7)
	st.Frame()
	st.Key(0, ui.KeyUp)
	if a, c := sameDoc.Selection(); a != c || c != 0 {
		t.Fatalf("same-line forward selection + Up = (%d, %d), want (0, 0)", a, c)
	}

	// Shift+Up still extends from the caret rather than collapsing.
	doc.SetSelection(5, 16)
	tt.Frame()
	tt.Key(ui.Shift, ui.KeyUp)
	if a, c := doc.Selection(); a != 5 || doc.Text().paraAt(c) != 2 {
		t.Fatalf("forward selection + Shift+Up = (%d, %d) in paragraph %d, want anchor 5 and caret in paragraph 2", a, c, doc.Text().paraAt(c))
	}

	t.Run("soft-wrap-boundary-affinity", func(t *testing.T) {
		// Paragraph 0 is "top"; paragraph 1 wraps across at least three visual
		// lines in a narrow 200px viewport.
		wrapDoc := NewDocument(NewText("top\n" + strings.Repeat("会议", 40)))
		wt := ui.NewTester(func(c *ui.Context) {
			Editor(c, wrapDoc).Fill()
		}, 200, 400)
		wt.Press(100, 380)
		wt.Release(100, 380)

		p1Start := 4 // after "top\n"
		wrapDoc.SetSelection(p1Start, p1Start)
		wt.Frame()
		wt.Key(0, ui.KeyEnd)
		_, line0End := wrapDoc.Selection()
		wt.Key(0, ui.KeyDown)
		_, line1End := wrapDoc.Selection()
		wt.Key(0, ui.KeyDown)
		_, line2End := wrapDoc.Selection()
		if !(p1Start < line0End && line0End < line1End && line1End < line2End) {
			t.Fatalf("expected >= 3 wrapped lines in paragraph 1, got ends (%d, %d, %d)", line0End, line1End, line2End)
		}

		// Forward selection ending at soft-wrap boundary (Home, Shift+End):
		// v.affinity is TextUpstream on line 0; plain Down must preserve it so
		// it steps onto line 1 instead of flipping to TextDownstream and
		// skipping to line 2.
		wrapDoc.SetSelection(p1Start, p1Start)
		wt.Frame()
		wt.Key(0, ui.KeyHome)
		wt.Key(ui.Shift, ui.KeyEnd)
		if a, c := wrapDoc.Selection(); a != p1Start || c != line0End {
			t.Fatalf("Home + Shift+End = (%d, %d), want (%d, %d)", a, c, p1Start, line0End)
		}
		wt.Key(0, ui.KeyDown)
		if a, c := wrapDoc.Selection(); a != c || c != line1End {
			t.Fatalf("soft-wrap forward selection + Down = (%d, %d), want (%d, %d) on line 1 (line 2 end is %d)", a, c, line1End, line1End, line2End)
		}

		// Backward selection whose caret (start edge) sits at the soft-wrap
		// boundary with TextUpstream: plain Up must preserve v.affinity on
		// v.caret so it steps up into paragraph 0 ("top") instead of flipping
		// to TextDownstream on line 1 and moving back onto line 0.
		wrapDoc.SetSelection(line0End+1, line0End+1)
		wt.Frame()
		wt.Key(ui.Shift, ui.KeyLeft) // (line0End, TextDownstream) on line 1
		wt.Key(ui.Shift, ui.KeyLeft) // (line0End, TextUpstream) on line 0
		if a, c := wrapDoc.Selection(); a != line0End+1 || c != line0End {
			t.Fatalf("Shift+Left across wrap = (%d, %d), want (%d, %d)", a, c, line0End+1, line0End)
		}
		wt.Key(0, ui.KeyUp)
		if a, c := wrapDoc.Selection(); a != c || wrapDoc.Text().paraAt(c) != 0 {
			t.Fatalf("soft-wrap backward selection + Up = (%d, %d) in paragraph %d, want collapsed in paragraph 0", a, c, wrapDoc.Text().paraAt(c))
		}
	})

	t.Run("resets-stale-desired-x", func(t *testing.T) {
		desiredDoc := NewDocument(NewText("aaaaaa\naaaaaa\naaaaaa"))
		dt, _ := richTester(t, desiredDoc)

		// Down from col 0 sets v.desiredX to col 0 and v.hasDesired = true.
		// Shift+End selects to the end of line 1 without clearing v.hasDesired.
		// Plain Down must collapse to line 1 end and reset v.hasDesired so it
		// steps to the end of line 2 (20), not col 0 of line 2 (14).
		desiredDoc.SetSelection(0, 0)
		dt.Frame()
		dt.Key(0, ui.KeyDown)
		if a, c := desiredDoc.Selection(); a != 7 || c != 7 {
			t.Fatalf("Down from 0 = (%d, %d), want (7, 7)", a, c)
		}
		dt.Key(ui.Shift, ui.KeyEnd)
		if a, c := desiredDoc.Selection(); a != 7 || c != 13 {
			t.Fatalf("Shift+End = (%d, %d), want (7, 13)", a, c)
		}
		dt.Key(0, ui.KeyDown)
		if a, c := desiredDoc.Selection(); a != c || c != 20 {
			t.Fatalf("Down after stale desiredX + Shift+End = (%d, %d), want (20, 20)", a, c)
		}

		// Symmetric upward case: Down from line 0 end sets v.desiredX to line end;
		// Shift+Home selects backward to col 0 without clearing v.hasDesired;
		// plain Up must collapse to col 0 and step to 0, not line 0 end (6).
		desiredDoc.SetSelection(6, 6)
		dt.Frame()
		dt.Key(0, ui.KeyDown)
		if a, c := desiredDoc.Selection(); a != 13 || c != 13 {
			t.Fatalf("Down from 6 = (%d, %d), want (13, 13)", a, c)
		}
		dt.Key(ui.Shift, ui.KeyHome)
		if a, c := desiredDoc.Selection(); a != 13 || c != 7 {
			t.Fatalf("Shift+Home = (%d, %d), want (13, 7)", a, c)
		}
		dt.Key(0, ui.KeyUp)
		if a, c := desiredDoc.Selection(); a != c || c != 0 {
			t.Fatalf("Up after stale desiredX + Shift+Home = (%d, %d), want (0, 0)", a, c)
		}
	})
}

// TestRichTextLinkOpensSafely clicks links: only those ui.SafeToOpen
// allows open, as "calc" would run a program on Windows.
func TestRichTextLinkOpensSafely(t *testing.T) {
	doc := NewDocument(TextOf(
		Paragraph{Runs: []Run{{Text: "run", Attributes: Attributes{Link: "calc"}}}},
		Paragraph{Runs: []Run{{Text: "site", Attributes: Attributes{Link: "https://example.com"}}}},
	))
	tt, _ := richTester(t, doc)
	for y := float32(2); y < 80; y += 6 { // apart, so that no click is a double click
		tt.Move(10, y)
		if tt.Cursor() == ui.CursorPointer {
			tt.ClickAt(10, y)
		}
	}
	got := tt.OpenedURLs()
	if len(got) == 0 || slices.ContainsFunc(got, func(u string) bool { return u != "https://example.com" }) {
		t.Errorf("opened %q", got)
	}
}

// TestRichTextLinkCanceledPress presses a link and has the system cancel
// the press: the link opens only for a click.
func TestRichTextLinkCanceledPress(t *testing.T) {
	doc := NewDocument(TextOf(
		Paragraph{Runs: []Run{{Text: "site", Attributes: Attributes{Link: "https://example.com"}}}},
	))
	tt, _ := richTester(t, doc)
	var x, y float32
	for y = 2; y < 40; y += 2 {
		if tt.Move(10, y); tt.Cursor() == ui.CursorPointer {
			x = 10
			break
		}
	}
	if x == 0 {
		t.Fatal("no link under the pointer")
	}
	tt.Press(x, y)
	tt.CancelPress()
	if got := tt.OpenedURLs(); len(got) != 0 {
		t.Fatalf("a canceled press opened %q", got)
	}
	tt.Advance(time.Second) // no double click
	tt.ClickAt(x, y)
	if got := tt.OpenedURLs(); !slices.Equal(got, []string{"https://example.com"}) {
		t.Fatalf("a click opened %q", got)
	}
}

// TestRichTextCJKWords deletes and moves by words in Chinese and
// Japanese: a character at a time, as they have no spaces (#668).
// Left and Right go through an offset with two caret places, the edge of
// a span padded with PaddingX (code within text here) or a line wrapped
// within a word, in two steps, and leave the paragraph only past its start
// or end.
func TestRichTextArrowsThroughTwoPlaces(t *testing.T) {
	doc := NewDocument(TextOf(
		Paragraph{Runs: []Run{{Text: "one"}}},
		Paragraph{Runs: []Run{{Text: "say "}, {Text: "ok", Attributes: Attributes{Code: true}}, {Text: " now"}}},
		Paragraph{Runs: []Run{{Text: strings.Repeat("会议", 40)}}},
		Paragraph{Runs: []Run{{Text: "three"}}},
	))
	tt := ui.NewTester(func(c *ui.Context) {
		Editor(c, doc, EditorOptions{Spans: func(_ int, _ ParagraphStyle, spans []ui.Span) []ui.Span {
			for i := range spans {
				if spans[i].Background.A != 0 {
					spans[i].PaddingX = 4
				}
			}
			return spans
		}}).Fill()
	}, 300, 400)
	tt.Press(150, 380)
	tt.Release(150, 380)
	walk := func(from int, key ui.Key, n int) []int {
		doc.SetSelection(from, from)
		tt.Frame()
		var at []int
		for range n {
			tt.Key(0, key)
			_, c := doc.Selection()
			at = append(at, c)
		}
		return at
	}
	// "say " is 4..8, "ok" 8..10, " now" 10..14.
	if got, want := walk(4, ui.KeyRight, 13), []int{5, 6, 7, 8, 8, 9, 10, 10, 11, 12, 13, 14, 15}; !slices.Equal(got, want) {
		t.Fatalf("Right through the code: %v, want %v", got, want)
	}
	if got, want := walk(14, ui.KeyLeft, 13), []int{13, 12, 11, 10, 10, 9, 8, 8, 7, 6, 5, 4, 3}; !slices.Equal(got, want) {
		t.Fatalf("Left through the code: %v, want %v", got, want)
	}
	// The wrapped paragraph is 15..95: each offset in turn, a wrap's twice.
	for _, key := range []ui.Key{ui.KeyRight, ui.KeyLeft} {
		from, dir, past := 15, 1, 96
		if key == ui.KeyLeft {
			from, dir, past = 95, -1, 14
		}
		at := walk(from, key, 120)
		last, twice := from, 0
		for _, c := range at {
			if c == past {
				break
			}
			if c == last {
				twice++
			} else if c != last+dir {
				t.Fatalf("%v through the wrapped paragraph went from %d to %d: %v", key, last, c, at)
			}
			last = c
		}
		if last != from+dir*80 || twice == 0 {
			t.Fatalf("%v through the wrapped paragraph: %v", key, at)
		}
	}
}

func TestRichTextCJKWords(t *testing.T) {
	word := ui.Ctrl
	if runtime.GOOS == "darwin" {
		word = ui.Alt
	}
	doc := NewDocument(TextOf(Paragraph{Runs: []Run{{Text: "Go语言，今日は"}}}))
	tt, _ := richTester(t, doc)
	tt.Key(word, ui.KeyBackspace)
	if got := doc.Text().String(); got != "Go语言，今日" {
		t.Fatalf("word backspace left %q", got)
	}
	for _, want := range []int{6, 5, 3, 2, 0} {
		tt.Key(word, ui.KeyLeft)
		if a, c := doc.Selection(); a != want || c != want {
			t.Fatalf("word left went to %d–%d, want %d", a, c, want)
		}
	}
}

// The scroll bar's thumb shows while the pointer is over the editor, as a
// scroll container's does, and not otherwise.
func TestScrollbarOnHover(t *testing.T) {
	doc := NewDocument(NewText(strings.Repeat("line\n", 200)))
	tt := ui.NewTester(func(c *ui.Context) { Editor(c, doc).Fill() }, 500, 400)
	thumb := func() bool {
		img := tt.Image()
		s := img.Bounds().Dx() / 500
		// Clear of the border's rounded corners.
		for y := 10 * s; y < 390*s; y++ {
			if img.RGBAAt(494*s, y) != img.RGBAAt(480*s, y) {
				return true
			}
		}
		return false
	}
	tt.Move(-10, -10)
	tt.Frame()
	if thumb() {
		t.Fatal("the thumb shows with the pointer away")
	}
	tt.Move(250, 200)
	tt.Frame()
	if !thumb() {
		t.Fatal("the thumb doesn't show with the pointer over the editor")
	}
	tt.Move(-10, -10)
	tt.Frame()
	if thumb() {
		t.Fatal("the thumb stayed as the pointer left")
	}
}

func TestAttachmentLinkBounds(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "editable", true: "read-only"}[readOnly], func(t *testing.T) {
			var boxes [2]ui.Rect
			var opened []string
			text := NewText("before ")
			for i, link := range []string{"first", "second"} {
				text = text.Append(string(ObjectReplacement), Attributes{Link: link, Attachment: &Attachment{
					Name: link, Width: 120, Height: 24,
					Draw: func(_ *ui.Painter, r ui.Rect) { boxes[i] = r },
				}})
			}
			text = text.Append(" after", Attributes{})
			doc := NewDocument(text)
			tt := ui.NewTester(func(c *ui.Context) {
				EditorBase(c, doc, EditorOptions{ReadOnly: readOnly, OnLink: func(s string) { opened = append(opened, s) }}).Fill()
			}, 500, 200)
			tt.Frame()
			for i, box := range boxes {
				for _, dx := range []float32{1, 20, 60, 119} {
					tt.Advance(time.Second)
					x, y := box.X+dx, box.Y+box.H/2
					tt.Move(x, y)
					if tt.Cursor() != ui.CursorPointer {
						t.Fatalf("attachment %d at %v has no pointer cursor", i, dx)
					}
					tt.ClickAt(x, y)
					want := []string{"first", "second"}[i]
					if len(opened) != 1 || opened[0] != want {
						t.Fatalf("attachment %d at %v: opened %v", i, dx, opened)
					}
					opened = nil
				}
			}
			for _, point := range []ui.Point{
				{X: boxes[0].X - 1, Y: boxes[0].Y + 12},
				{X: boxes[1].X + 121, Y: boxes[1].Y + 12},
				{X: boxes[0].X + 100, Y: boxes[0].Y + 30},
			} {
				tt.Advance(time.Second)
				tt.ClickAt(point.X, point.Y)
				if len(opened) != 0 {
					t.Fatalf("outside attachment at %+v opened %v", point, opened)
				}
			}
			// A drag within one half still has the same nearest caret.
			box := boxes[0]
			tt.Advance(time.Second)
			tt.Press(box.X+4, box.Y+12)
			tt.Move(box.X+30, box.Y+12)
			tt.Move(box.X+4, box.Y+12)
			tt.Release(box.X+4, box.Y+12)
			if len(opened) != 0 {
				t.Fatal("drag returning to the press opened the attachment")
			}
			tt.Advance(time.Second)
			tt.Press(box.X+4, box.Y+12)
			tt.Move(box.X+116, box.Y+12)
			tt.Release(box.X+116, box.Y+12)
			a, z := doc.Selection()
			if len(opened) != 0 || min(a, z) != 7 || max(a, z) != 8 {
				t.Fatalf("drag: opened=%v selection=%d,%d", opened, a, z)
			}
			if doc.Text().PlainText() != text.PlainText() {
				t.Fatal("pointer input changed the document")
			}
		})
	}
}

// OnKey is asked first: a key it takes does nothing in the editor, and one it lets go is the editor's as before.
func TestRichTextOnKey(t *testing.T) {
	doc := NewDocument(NewText("one\ntwo"))
	var asked []ui.Key
	tt := ui.NewTester(func(c *ui.Context) {
		Editor(c, doc, EditorOptions{OnKey: func(k ui.Key, _ ui.Modifiers) bool {
			asked = append(asked, k)
			return k == ui.KeyDown || k == ui.KeyEnter
		}}).Fill()
	}, 500, 400)
	tt.Press(10, 10)
	tt.Release(10, 10)
	doc.SetSelection(1, 1)
	tt.Frame()
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	if _, c := doc.Selection(); c != 1 || doc.Text().String() != "one\ntwo" {
		t.Fatalf("a key taken by OnKey moved or changed the text: caret %d, %q", c, doc.Text().String())
	}
	tt.Key(0, ui.KeyRight)
	if _, c := doc.Selection(); c != 2 {
		t.Fatalf("a key let go is not the editor's: caret %d", c)
	}
	if len(asked) != 3 {
		t.Fatalf("asked about %v", asked)
	}
	// what OnKey does to the document is kept: the editor does not give its own selection back over it
	tt2 := ui.NewTester(func(c *ui.Context) {
		Editor(c, doc, EditorOptions{OnKey: func(k ui.Key, _ ui.Modifiers) bool {
			if k == ui.KeyEnter {
				doc.Edit(func(ed *Edit) { ed.Replace(0, 3, NewText("ONE")); ed.Select(1, 1) })
				return true
			}
			return false
		}}).Fill()
	}, 500, 400)
	tt2.Press(10, 10)
	tt2.Release(10, 10)
	doc.SetSelection(3, 3)
	tt2.Frame()
	tt2.Key(0, ui.KeyEnter)
	tt2.Frame()
	if _, c := doc.Selection(); c != 1 || doc.Text().String() != "ONE\ntwo" {
		t.Fatalf("what OnKey did was undone: caret %d, %q", c, doc.Text().String())
	}
}

// OnCommand is asked about an edit command of the menus first, as a Mac's menu bar sends ⌘V: a command it takes does
// nothing in the editor, and what it does to the document is kept; one it lets go is the editor's as before.
func TestRichTextOnCommand(t *testing.T) {
	doc := NewDocument(NewText("one two"))
	var asked []string
	tt := ui.NewTester(func(c *ui.Context) {
		Editor(c, doc, EditorOptions{OnCommand: func(name string) bool {
			asked = append(asked, name)
			if name != "paste" {
				return false
			}
			doc.SetLink("https://example.com")
			return true
		}}).Fill()
	}, 500, 400)
	tt.Press(10, 10)
	tt.Release(10, 10)
	tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.Text, []byte("https://example.com")))))
	doc.SetSelection(0, 3)
	tt.Frame()
	tt.Command("paste")
	tt.Frame()
	if got := doc.Text().String(); got != "one two" {
		t.Fatalf("a command taken by OnCommand was the editor's too: %q", got)
	}
	if link := doc.Text().AttributesAt(1).Link; link != "https://example.com" {
		t.Fatalf("what OnCommand did was undone: link %q", link)
	}
	doc.SetSelection(0, 0)
	tt.Frame()
	tt.Command("selectAll")
	if a, c := doc.Selection(); a != 0 || c != 7 {
		t.Fatalf("a command let go is not the editor's: selection %d-%d", a, c)
	}
	if len(asked) != 2 || asked[0] != "paste" || asked[1] != "selectAll" {
		t.Fatalf("asked about %v", asked)
	}
}

// Spans is given each paragraph's spans to draw them otherwise, its text the same; the layouts it made are kept until
// SpansVersion changes.
func TestRichTextSpans(t *testing.T) {
	doc := NewDocument(NewText("plain\ncolored"))
	calls, ver, red := 0, 0, false
	draw := func(c *ui.Context) {
		opts := EditorOptions{SpansVersion: ver}
		if red {
			opts.Spans = func(_ int, st ParagraphStyle, spans []ui.Span) []ui.Span {
				calls++
				for i := range spans {
					spans[i].Color = ui.RGB(220, 0, 0)
					spans[i].LetterSpacing = 2
				}
				return spans
			}
		}
		Editor(c, doc, opts).Fill()
	}
	tt := ui.NewTester(draw, 500, 300)
	tt.Frame()
	plain := bytes.Clone(tt.Image().Pix)
	red = true
	tt.Frame()
	if calls == 0 || bytes.Equal(tt.Image().Pix, plain) {
		t.Fatalf("the spans were not drawn otherwise: %d calls", calls)
	}
	was := calls
	tt.Frame()
	if calls != was {
		t.Fatalf("laid out again with nothing changed: %d calls", calls-was)
	}
	ver++
	tt.Frame()
	if calls == was {
		t.Fatal("not laid out again when the version changed")
	}
	if doc.Text().String() != "plain\ncolored" {
		t.Fatal("the text changed")
	}
}

// Menu builds the editor's context menu in place of its own: a right click shows the app's items, not the editor's.
func TestRichTextMenu(t *testing.T) {
	doc := NewDocument(NewText("some text"))
	tt := ui.NewTester(func(c *ui.Context) {
		Editor(c, doc, EditorOptions{Menu: func(m *ui.Menu) { m.Item("Ask the app") }}).Fill()
	}, 500, 300)
	tt.RightClickAt(40, 20)
	if got := tt.Menu(); len(got) != 1 || got[0] != "Ask the app" {
		t.Fatalf("the menu: %q", got)
	}
	plain := ui.NewTester(func(c *ui.Context) { Editor(c, NewDocument(NewText("some text"))).Fill() }, 500, 300)
	plain.RightClickAt(40, 20)
	if got := plain.Menu(); len(got) > 0 && got[0] == "Ask the app" {
		t.Fatalf("without Menu, the app's item: %q", got)
	}
}

func TestRichTextEditMenu(t *testing.T) {
	doc := NewDocument(NewText("some text"))
	tt := ui.NewTester(func(c *ui.Context) { Editor(c, doc).Fill() }, 500, 300)
	tt.RightClickAt(40, 20)
	if got := tt.Menu(); !slices.Contains(got, "Copy") || !slices.Contains(got, "Paste") || !slices.Contains(got, "Select All") {
		t.Fatalf("the editor's own menu: %q", got)
	}
	if err := tt.ChooseMenuItem("Select All"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a, z := doc.Selection(); min(a, z) != 0 || max(a, z) != len("some text") {
		t.Fatalf("Select All selected %d…%d", a, z)
	}

	// EditItems puts the editor's commands in the app's own menu.
	app := ui.NewTester(func(c *ui.Context) {
		Editor(c, NewDocument(NewText("some text")), EditorOptions{Menu: func(m *ui.Menu) {
			m.Item("Ask the app")
			m.Separator()
			m.EditItems()
		}}).Fill()
	}, 500, 300)
	app.RightClickAt(40, 20)
	if got := app.Menu(); len(got) < 3 || got[0] != "Ask the app" || !slices.Contains(got, "Copy") {
		t.Fatalf("the app's menu with EditItems: %q", got)
	}
}

// A right-click outside the selection gives the editor the focus and puts
// the caret where it is, as in a TextArea, so that the menu acts there; one
// in the selection keeps it.
func TestRichTextRightClickMovesCaret(t *testing.T) {
	doc := NewDocument(NewText("first paragraph\nsecond paragraph"))
	seen := -1
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			ui.Button(c, "Other")
			Editor(c, doc, EditorOptions{Menu: func(m *ui.Menu) {
				_, seen = doc.Selection()
				m.EditItems()
			}}).Fill()
		})
	}, 500, 400)
	doc.SetSelection(32, 32)
	if err := tt.Click("Other"); err != nil {
		t.Fatal(err)
	}
	ed, ok := tt.Find("Rich text")
	if !ok {
		t.Fatal("no editor")
	}
	p := doc.VisibleParagraphs()[0]
	y := ed.Y + p.Top + p.Height/2
	tt.RightClickAt(30, y)
	if _, caret := doc.Selection(); caret <= 0 || caret >= len("first paragraph") {
		t.Errorf("caret at %d after a right-click in the first paragraph", caret)
	}
	if _, caret := doc.Selection(); seen != caret {
		t.Errorf("the menu saw the caret at %d, it is at %d", seen, caret)
	}
	if !tt.Focused("Rich text") {
		t.Error("the editor did not take the focus")
	}
	if len(tt.Menu()) == 0 {
		t.Error("no menu")
	}

	tt.Key(0, ui.KeyEscape)
	doc.SetSelection(0, len("first paragraph"))
	tt.Frame()
	tt.RightClickAt(30, y)
	if a, z := doc.Selection(); a != 0 || z != len("first paragraph") {
		t.Errorf("a right-click in the selection made it %d…%d", a, z)
	}
}

// Space leaves room above a paragraph, more than the theme's: the paragraphs from it down move by as much, in the
// frame it changes, and back when it goes.
func TestRichTextSpace(t *testing.T) {
	doc := NewDocument(NewText("one\ntwo\nthree"))
	var room float32
	tt := openTester(t, func(c *ui.Context) {
		Editor(c, doc, EditorOptions{Space: func(p int) float32 {
			if p == 1 {
				return room
			}
			return 0
		}}).Fill()
	}, 500, 400)
	tt.Frame()
	before := doc.VisibleParagraphs()
	room = 40
	tt.Frame()
	tt.Frame()
	after := doc.VisibleParagraphs()
	if after[0].Top != before[0].Top || after[1].Top != before[1].Top+40 || after[2].Top != before[2].Top+40 {
		t.Fatalf("room above the second: %+v, then %+v", before, after)
	}
	room = 0
	tt.Frame()
	tt.Frame()
	if back := doc.VisibleParagraphs(); back[1].Top != before[1].Top || back[2].Top != before[2].Top {
		t.Fatalf("the room gone: %+v, then %+v", before, back)
	}
}

// With NoScrollbar the editor draws no scroll bar, the pointer over it or
// not, and a press where the bar would be places the caret in the text.
func TestNoScrollbar(t *testing.T) {
	doc := NewDocument(NewText(strings.Repeat("line\n", 200)))
	tt := ui.NewTester(func(c *ui.Context) { Editor(c, doc, EditorOptions{NoScrollbar: true}).Fill() }, 500, 400)
	tt.Move(250, 200)
	tt.Frame()
	img := tt.Image()
	s := img.Bounds().Dx() / 500
	for y := 10 * s; y < 390*s; y++ {
		if img.RGBAAt(494*s, y) != img.RGBAAt(480*s, y) {
			t.Fatal("a scroll bar shows")
		}
	}
	tt.ClickAt(495, 30)
	if a, z := doc.Selection(); a != z || a == 0 {
		t.Fatalf("a press on the bar's place did not place the caret in the text: %d, %d", a, z)
	}
}

// A document shown in two editors gives each one's paragraphs in view
// when they are read right after it is built, as an app that shows one
// page in two panes lines each up with its own; and what each painted is
// told apart, so the frames settle.
func TestVisibleParagraphsPerEditor(t *testing.T) {
	doc := NewDocument(NewText(strings.Repeat("a word or two, again and again, to wrap at a narrow width; ", 6) + "\nsecond\nthird"))
	var wide, narrow []ParagraphBox
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Fill().Children(func() {
			EditorBase(c, doc).Width(600).Height(400)
			wide = doc.VisibleParagraphs()
			EditorBase(c, doc).Width(150).Height(400)
			narrow = doc.VisibleParagraphs()
		})
	}, 800, 400)
	tt.Frame()
	tt.Frame()
	if len(wide) < 2 || len(narrow) < 2 {
		t.Fatalf("boxes: %d and %d", len(wide), len(narrow))
	}
	if narrow[0].Height <= wide[0].Height {
		t.Fatalf("the narrow editor's first paragraph is %v high, the wide one's %v: one editor's boxes for both", narrow[0].Height, wide[0].Height)
	}
	if tt.FramePending() {
		t.Fatal("the frames do not settle")
	}
}

// An editor as tall as its text, in a scroll container (a page that
// scrolls as a whole), paints the paragraphs the container shows when it
// is scrolled down the text, though it paints only those, and gives each
// paragraph in its box with the rune it starts at.
func TestTallEditorInScroll(t *testing.T) {
	var b strings.Builder
	var starts []int
	at := 0
	for i := 0; i < 300; i++ {
		line := fmt.Sprintf("paragraph %d: çok güzel, %s", i, strings.Repeat("é", i%7))
		starts = append(starts, at)
		at += utf8.RuneCountInString(line) + 1
		b.WriteString(line + "\n")
	}
	doc := NewDocument(NewText(strings.TrimSuffix(b.String(), "\n")))
	var boxes []ParagraphBox
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Fill().Children(func() {
			Editor(c, doc, EditorOptions{NoScrollbar: true}).FillWidth().Height(300 * 40)
			boxes = doc.VisibleParagraphs()
		})
	}, 500, 400)
	tt.Frame()
	tt.Frame()
	if len(boxes) != len(starts) {
		t.Fatalf("%d boxes in an editor as tall as its %d paragraphs", len(boxes), len(starts))
	}
	for i, bx := range boxes {
		if bx.Index != i || bx.Start != starts[i] {
			t.Fatalf("box %d: paragraph %d at rune %d, want %d", i, bx.Index, bx.Start, starts[i])
		}
	}
	tt.Wheel(250, 200, 0, 2000)
	tt.Advance(time.Second)
	img := tt.Image()
	s := img.Bounds().Dx() / 500
	ground := img.RGBAAt(498*s, 2*s)
	ink := 0
	for y := 0; y < 400*s; y += 2 {
		for x := 0; x < 300*s; x += 2 {
			if img.RGBAAt(x, y) != ground {
				ink++
			}
		}
	}
	if ink < 2000 {
		t.Fatalf("scrolled down the text, the view shows no text (%d marked pixels)", ink)
	}
}

// An editor as tall as its text, in a scroll container, lays out again
// only the paragraphs the container shows when its theme or its font
// changes, or when the app builds it anew, keeping the others' heights
// (those of another font scaled) until they come into view; its height,
// ContentHeight, stays what the text takes.
func TestTallEditorNewLook(t *testing.T) {
	paras := make([]Paragraph, 3000)
	for i := range paras {
		paras[i] = para(fmt.Sprintf("Paragraph %d, long enough to wrap in a window this narrow at times.", i))
	}
	doc := NewDocument(TextOf(paras...))
	dark := &Theme{Text: ui.Hex("#eeeeee")}
	var th *Theme
	size, tree := float32(14), 0
	tt := ui.NewTester(func(c *ui.Context) {
		c.Key(fmt.Sprint("tree", tree))
		ui.Scroll(c).Fill().Children(func() {
			ed := Editor(c, doc, EditorOptions{NoScrollbar: true, Theme: th, Font: ui.Font{Size: size}}).FillWidth()
			h, ok := doc.ContentHeight()
			if !ok {
				h = 3000 * 60
			}
			ed.Height(h)
		})
	}, 500, 400)
	tt.Frame()
	tt.Frame()
	v := doc.shownIn.Value()
	tall, _ := doc.ContentHeight()
	if v.laid < len(paras) || tall < 3000*14 {
		t.Fatalf("the first frames laid out %d of %d paragraphs, %v tall", v.laid, len(paras), tall)
	}
	shows := func(what string) {
		t.Helper()
		v := doc.shownIn.Value()
		if v.laid > 100 {
			t.Fatalf("%s: %d paragraphs laid out again", what, v.laid)
		}
	}
	th = dark
	tt.Frame()
	shows("another theme")
	if h, _ := doc.ContentHeight(); h != tall {
		t.Fatalf("another theme: %v tall, was %v", h, tall)
	}
	tree++
	tt.Frame()
	if doc.shownIn.Value() == v {
		t.Fatal("the tree built anew kept its editor")
	}
	shows("an editor built anew")
	if h, _ := doc.ContentHeight(); h != tall {
		t.Fatalf("an editor built anew: %v tall, was %v", h, tall)
	}
	size = 21
	tt.Frame()
	shows("another font")
	if h, _ := doc.ContentHeight(); h < tall*1.3 {
		t.Fatalf("a larger font: %v tall, was %v", h, tall)
	}
	tt.Wheel(250, 200, 0, 20000)
	tt.Advance(time.Second)
	v = doc.shownIn.Value()
	far := 0
	for i := 300; i < len(v.paras); i++ {
		if v.paras[i].layout != nil {
			far++
		}
	}
	if far == 0 || v.laid > 300 {
		t.Fatalf("scrolled down the text, %d paragraphs there laid out, %d in all", far, v.laid)
	}
}
