package richtext

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The bars of a quotation's paragraphs join as one: rounded at the
// quotation's ends only, so that no notch shows where two paragraphs
// meet, even at the bar's outer edges.
func TestQuoteBarsJoin(t *testing.T) {
	doc := NewDocument(TextOf(
		para("Before"),
		Paragraph{Style: ParagraphStyle{Quote: true}, Runs: []Run{{Text: "One"}}},
		Paragraph{Style: ParagraphStyle{Quote: true}, Runs: []Run{{Text: "Two"}}},
		para("After"),
	))
	red := ui.RGB(255, 0, 0)
	boxes := new([]ParagraphBox)
	tt := openTester(t, func(c *ui.Context) {
		*boxes = doc.VisibleParagraphs()
		EditorBase(c, doc, EditorOptions{Font: ui.Font{Size: 10},
			Theme: &Theme{QuoteBar: red, QuoteBarWidth: 6, ParagraphSpacing: 0.8}}).Fill()
	}, 300, 200)
	tt.Frame()
	if len(*boxes) != 4 {
		t.Fatalf("%d paragraphs in view, want 4", len(*boxes))
	}
	one, two := (*boxes)[1], (*boxes)[2]
	img := tt.Image()
	s := float32(img.Bounds().Dx()) / 300
	isRed := func(x, y int) bool {
		c := img.RGBAAt(x, y)
		return c.R > 200 && c.G < 60 && c.B < 60
	}
	row := int((one.Top + one.Height/2) * s)
	first, last := -1, -1
	for x := 0; x < img.Bounds().Dx()/2; x++ {
		if isRed(x, row) {
			if first < 0 {
				first = x
			}
			last = x
		}
	}
	if first < 0 {
		t.Fatal("no bar beside the first quoted line")
	}
	// The bar's outermost columns, where rounded ends would leave a notch,
	// are red all the way from the middle of one line to the next.
	for _, x := range []int{first, last} {
		for y := row; y < int((two.Top+two.Height/2)*s); y++ {
			if !isRed(x, y) {
				t.Fatalf("the bar has a notch at x %d, y %d (DIP %v), between the paragraphs at %v and %v", x, y, float32(y)/s, one.Top, two.Top)
			}
		}
	}
	// Its ends stay round: the corner pixel at the top of the quotation is
	// not filled.
	top := row
	for top > 0 && isRed((first+last)/2, top-1) {
		top--
	}
	if isRed(first, top) {
		t.Error("the quotation's first bar is square at its top end")
	}
}
