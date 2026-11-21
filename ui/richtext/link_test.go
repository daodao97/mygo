package richtext

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func linkTester(t *testing.T, doc *Document, width int) (*ui.Tester, *[]string) {
	t.Helper()
	opened := new([]string)
	tt := ui.NewTester(func(c *ui.Context) {
		EditorBase(c, doc, EditorOptions{OnLink: func(url string) {
			*opened = append(*opened, url)
		}}).Fill()
	}, width, 200)
	tt.Frame()
	return tt, opened
}

// linkCaret returns the caret at a rune in window coordinates.
func linkCaret(t *testing.T, doc *Document, tt *ui.Tester, i int) ui.Rect {
	t.Helper()
	doc.SetSelection(i, i)
	tt.Frame()
	r, ok := doc.CaretRect()
	if !ok {
		t.Fatal("caret is not in view")
	}
	return r
}

func hoverLink(t *testing.T, tt *ui.Tester, x, y float32, want string) {
	t.Helper()
	tt.Move(x, y)
	if got := tt.Cursor(); (got == ui.CursorPointer) != (want != "") {
		t.Fatalf("hover at (%v, %v) has cursor %v, want link %q", x, y, got, want)
	}
}

func clickLink(t *testing.T, tt *ui.Tester, opened *[]string, x, y float32, want string) {
	t.Helper()
	hoverLink(t, tt, x, y, want)
	before := len(*opened)
	tt.ClickAt(x, y)
	if want == "" {
		if len(*opened) != before {
			t.Fatalf("click at (%v, %v) opened %q", x, y, (*opened)[before:])
		}
		return
	}
	if len(*opened) != before+1 || (*opened)[before] != want {
		t.Fatalf("click at (%v, %v) opened %q, want %q", x, y, (*opened)[before:], want)
	}
}

func TestRichTextLinkHitBounds(t *testing.T) {
	const url = "https://example.com"
	doc := NewDocument(TextOf(Paragraph{Runs: []Run{{Text: "site", Attributes: Attributes{Link: url}}}}))
	tt, opened := linkTester(t, doc, 500)
	start := linkCaret(t, doc, tt, 0)
	end := linkCaret(t, doc, tt, 4)
	center := (start.X + end.X) / 2
	lineY := start.Y + start.H/2
	clickLink(t, tt, opened, center, lineY, url)
	clickLink(t, tt, opened, end.X+40, lineY, "")
	clickLink(t, tt, opened, start.X-2, lineY, "")
	clickLink(t, tt, opened, center, start.Y-1, "")
	clickLink(t, tt, opened, center, start.Y+start.H+40, "")
}

func TestRichTextWrappedLinkHitBounds(t *testing.T) {
	const url = "https://example.com"
	doc := NewDocument(TextOf(Paragraph{Runs: []Run{{Text: "abcdefghij", Attributes: Attributes{Link: url}}}}))
	tt, opened := linkTester(t, doc, 55)
	tt.Press(1, 1)
	tt.Release(1, 1)
	first := linkCaret(t, doc, tt, 0)
	wrapped := -1
	for i := 1; i <= 10; i++ {
		if r := linkCaret(t, doc, tt, i); r.Y > first.Y {
			wrapped = i
			break
		}
	}
	if wrapped <= 0 || wrapped >= 10 {
		t.Fatalf("link wrapped after rune %d", wrapped)
	}
	before := linkCaret(t, doc, tt, wrapped-1)
	after := linkCaret(t, doc, tt, wrapped+1)
	center := (first.X + before.X) / 2
	firstY := first.Y + first.H/2
	hoverLink(t, tt, center, firstY, url)
	beforeOpened := len(*opened)
	tt.Press(center, firstY)
	tt.Release(center, firstY)
	if got := (*opened)[beforeOpened:]; len(got) != 1 || got[0] != url {
		t.Fatalf("click on the first line opened %q, want %q", got, url)
	}
	linkEnd := linkCaret(t, doc, tt, 10)
	clickLink(t, tt, opened, (after.X+linkEnd.X)/2, after.Y+after.H/2, url)
	clickLink(t, tt, opened, max(before.X, after.X)+30, first.Y+first.H/2, "")
}

func TestRichTextMidParagraphLinkHitBounds(t *testing.T) {
	const url = "https://example.com"
	doc := NewDocument(TextOf(Paragraph{Runs: []Run{
		{Text: "before "},
		{Text: "site", Attributes: Attributes{Link: url}},
		{Text: " after"},
	}}))
	tt, opened := linkTester(t, doc, 500)
	a := linkCaret(t, doc, tt, 7)
	z := linkCaret(t, doc, tt, 11)
	clickLink(t, tt, opened, (a.X+z.X)/2, a.Y+a.H/2, url)
	clickLink(t, tt, opened, z.X+30, a.Y+a.H/2, "")
}

func TestRichTextComposedLinkHitBounds(t *testing.T) {
	const url = "https://example.com"
	doc := NewDocument(TextOf(
		Paragraph{Runs: []Run{{Text: "site", Attributes: Attributes{Link: url}}}},
		Paragraph{Runs: []Run{{Text: "next", Attributes: Attributes{Link: url + "/next"}}}},
	))
	tt, opened := linkTester(t, doc, 500)
	before := linkCaret(t, doc, tt, 0)
	mid := linkCaret(t, doc, tt, 2)
	end := linkCaret(t, doc, tt, 4)
	nextStart := linkCaret(t, doc, tt, 5)
	nextEnd := linkCaret(t, doc, tt, 9)
	// Focus the editor, place caret inside the first link, and start an active composition.
	lineY := before.Y + before.H/2
	tt.ClickAt(end.X+40, lineY)
	doc.SetSelection(2, 2)
	tt.Frame()
	tt.Compose("你好", 2)
	tt.Frame()
	// The prefix before the composition stays clickable; the composition text does not.
	hoverLink(t, tt, (before.X+mid.X)/2, lineY, url)
	afterCompose, ok := doc.CaretRect()
	if !ok || afterCompose.X <= mid.X {
		t.Fatalf("composition caret at %v, want x > %v", afterCompose, mid.X)
	}
	hoverLink(t, tt, (mid.X+afterCompose.X)/2, lineY, "")
	// The shifted suffix after the composition stays clickable; trailing empty space does not.
	suffixW := end.X - mid.X
	hoverLink(t, tt, afterCompose.X+suffixW/2, lineY, url)
	hoverLink(t, tt, afterCompose.X+suffixW+40, lineY, "")
	// A composition in paragraph 0 does not shift links in paragraph 1.
	clickLink(t, tt, opened, (nextStart.X+nextEnd.X)/2, nextStart.Y+nextStart.H/2, url+"/next")
}

func TestRichTextLinkBelowLastParagraph(t *testing.T) {
	const url = "https://example.com"
	doc := NewDocument(TextOf(
		Paragraph{Runs: []Run{{Text: "above"}}},
		Paragraph{Runs: []Run{{Text: "site", Attributes: Attributes{Link: url}}}},
	))
	tt, opened := linkTester(t, doc, 500)
	a := linkCaret(t, doc, tt, 6)
	z := linkCaret(t, doc, tt, 10)
	boxes := doc.VisibleParagraphs()
	box := boxes[len(boxes)-1]
	clickLink(t, tt, opened, (a.X+z.X)/2, box.Top+box.Height+20, "")
}
