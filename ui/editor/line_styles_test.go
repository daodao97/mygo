package editor

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// styledFrames runs frames until the editor asks for none, as styled
// lines lay out and the heights follow.
func (ct *editorTest) styledFrames() {
	ct.t.Helper()
	ct.tt.Frame()
	for n := 0; ct.tt.FramePending(); n++ {
		if n > 10 {
			ct.t.Fatal("the frames don't end")
		}
		ct.tt.Frame()
	}
}

// A styled line's rows are as high as its font makes them; carets, hits
// and the lines after it follow, and the style moves with its line.
func TestLineStylesGeometry(t *testing.T) {
	ct := newEditorTest(t, "", "one\nheading\nlast")
	ct.st.SetLineStyles([]LineStyle{{Line: 1, Font: ui.Font{Size: 28}, LineHeight: 1.5}})
	ct.styledFrames()
	ed := &ct.st.ed
	v := &ed.view
	if l := v.line(1); l.rowHeight != 42 {
		t.Fatalf("the styled line's rows are %v high, want 42", l.rowHeight)
	}
	if got, want := ed.rowTop(2), float64(v.lineHeight)+42; got != want {
		t.Fatalf("the last line is at %v, want %v", got, want)
	}
	for _, line := range []int{0, 1, 2} {
		at := v.text.LineStart(line) + 1
		r, ok := ed.caretRect(at)
		if !ok {
			t.Fatalf("no caret on line %d", line)
		}
		if want := v.rowHeight(line); r.H != want {
			t.Fatalf("the caret on line %d is %v high, want %v", line, r.H, want)
		}
		if got := ed.offsetAt(r.X, float64(r.Y+r.H/2)); got != at {
			t.Fatalf("a press on line %d went to %d, want %d", line, got, at)
		}
	}
	ct.st.Buffer.Edit(Edit{Start: 0, End: 0, Text: "new\n"})
	ct.styledFrames()
	if got := ct.st.LineStyles(); len(got) != 1 || got[0].Line != 2 {
		t.Fatalf("the styles moved to %+v", got)
	}
	ct.st.SetLineStyles(nil)
	ct.styledFrames()
	if v.rowHeight(2) != v.lineHeight {
		t.Fatal("the style's height outlived it")
	}
	if len(v.styleViews) != 0 {
		t.Fatalf("%d views kept for styles no line has", len(v.styleViews))
	}
}

// A styled line wraps in its own font, and a replacement's element in it
// is as high as its rows, and as wide without a Width.
func TestLineStylesWrapAndReplacements(t *testing.T) {
	ct := newEditorTest(t, "", "first\n"+strings.Repeat("heading ", 30)+"\nlast")
	ct.st.SoftWrap = true
	ct.st.SetLineStyles([]LineStyle{{Line: 1, Font: ui.Font{Size: 30}}})
	ct.st.SetReplacements([]Replacement{{Start: 6, End: 13, Content: func(c *ui.Context, e ui.Element, _ Replacement) {
		e.Children(func() { ui.Text(c, "H") })
	}}})
	ct.styledFrames()
	v := &ct.st.ed.view
	l := v.line(1)
	if len(l.rows) < 2 {
		t.Fatal("the heading didn't wrap")
	}
	if g := v.heights.geometry(1); g.below-g.rows != float64(len(l.rows))*float64(l.rowHeight) {
		t.Fatalf("the wrapped line's rows take %v, want %v", g.below-g.rows, float64(len(l.rows))*float64(l.rowHeight))
	}
	placed := ct.st.ed.placeReplacements(0, 10000, nil)
	if len(placed) != 1 || placed[0].rect.H != l.rowHeight || placed[0].rect.W != l.rowHeight {
		t.Fatalf("the element is %+v, want %v square", placed, l.rowHeight)
	}
}

// The heights agree with finding lines by height, whatever lines are
// styled, hidden or have blocks.
func TestLineStylesHeightsAgree(t *testing.T) {
	r := rand.New(rand.NewPCG(4, 7))
	h := heights{lh: 20, n: 100, dirty: true}
	for i := range 100 {
		if r.IntN(3) == 0 {
			h.styles = append(h.styles, heightEntry{i, float32(10 + r.IntN(40))})
		}
	}
	h.setHidden([]lineSpan{{10, 15}, {40, 43}})
	h.setBlocks([]block{{line: 20, height: 70}, {line: 60, height: 30}})
	for i := range 100 {
		if h.isHidden(i) {
			continue
		}
		g := h.geometry(i)
		if got := h.lineAt((g.rows + g.below) / 2); got != i {
			t.Fatalf("line %d's rows are found as line %d's", i, got)
		}
		if g.bottom != h.top(i+1) {
			t.Fatalf("a gap after line %d: %v, next at %v", i, g.bottom, h.top(i+1))
		}
	}
}

// Setting styles again lays out again only the lines whose style changed.
func TestLineStylesKeepLayouts(t *testing.T) {
	ct := newEditorTest(t, "", "one\ntwo\nthree")
	styles := []LineStyle{{Line: 0, Font: ui.Font{Size: 20}}, {Line: 1, Font: ui.Font{Size: 25}}}
	ct.st.SetLineStyles(styles)
	ct.styledFrames()
	before := ct.st.ed.view.line(0)
	styles[1].Font.Size = 30
	ct.st.SetLineStyles(styles)
	ct.styledFrames()
	if ct.st.ed.view.line(0) != before {
		t.Fatal("a line whose style didn't change was laid out again")
	}
	ct.st.SetLineStyles(styles)
	ct.styledFrames()
	if ct.st.ed.view.line(0) != before {
		t.Fatal("the same styles laid the line out again")
	}
}

// A folded styled line takes no height.
func TestLineStylesFolded(t *testing.T) {
	ct := newEditorTest(t, "go", "func a() {\n\txx()\n}\nz")
	ct.st.SetLineStyles([]LineStyle{{Line: 1, Font: ui.Font{Size: 50}}})
	ct.styledFrames()
	ct.st.Fold(0)
	ct.styledFrames()
	if got, want := ct.st.ed.view.heights.total(), 3*float64(ct.st.ed.view.lineHeight); got != want {
		t.Fatalf("folded, the text is %v high, want %v", got, want)
	}
}

// A styled line keeps its syntax colors.
func TestLineStylesTokens(t *testing.T) {
	ct := newEditorTest(t, "go", "package a\n// a comment\nvar x = 1\n")
	ct.st.SetLineStyles([]LineStyle{{Line: 1, Font: ui.Font{Size: 26}}})
	ct.styledFrames()
	if l := ct.st.ed.view.line(1); len(l.toks) == 0 {
		t.Fatal("the styled line has no tokens")
	}
}

// The gutter numbers a styled line in the editor's font, on the baseline
// of its first row.
func TestLineStylesGutterBaseline(t *testing.T) {
	theme := &Theme{Background: ui.RGB(255, 255, 255), LineNumber: ui.RGB(255, 0, 0), ActiveLineNumber: ui.RGB(255, 0, 0), Text: ui.RGB(0, 0, 0)}
	ct := newEditorTest(t, "", "x\n1111\nz")
	ct.st.Theme, ct.st.NoActiveLine = theme, true
	ct.st.SetLineStyles([]LineStyle{{Line: 1, Font: ui.Font{Size: 40}}})
	ct.styledFrames()
	ed := &ct.st.ed
	img := ct.tt.Image()
	s := float32(img.Bounds().Dx()) / 600
	// The lowest row of red in the gutter beside the line, and of black
	// ink in its text: both stand on the baseline.
	lowest := func(x0, x1 float32, ink func(r, g, b uint8) bool) float32 {
		g := ed.view.heights.geometry(1)
		top, bottom := ed.r.Y+float32(g.rows), ed.r.Y+float32(g.below)
		low := float32(-1)
		for y := top; y < bottom; y += 0.5 {
			for x := x0; x < x1; x += 0.5 {
				c := img.RGBAAt(int(x*s), int(y*s))
				if ink(c.R, c.G, c.B) {
					low = y
				}
			}
		}
		return low
	}
	number := lowest(ed.r.X, ed.r.X+ed.gutterW, func(r, g, b uint8) bool { return r > 200 && g < 80 && b < 80 })
	text := lowest(ed.r.X+ed.textX, ed.r.X+ed.textX+80, func(r, g, b uint8) bool { return r < 60 && g < 60 && b < 60 })
	if number < 0 || text < 0 {
		t.Fatalf("no ink: number %v, text %v", number, text)
	}
	if d := number - text; d < -1.5 || d > 1.5 {
		t.Fatalf("the number's baseline is at %v, the text's at %v", number, text)
	}
	if lh := float32(ed.view.lineHeight); number-(ed.r.Y+float32(ed.view.heights.geometry(1).rows)) < lh {
		t.Fatalf("the number is at the top of the line, not on its baseline")
	}
}

// Reveal and all centering paths use the styled row's full height, not
// the editor's default row height. Oversized rows center at their top.
func TestLineStylesCaretRevealAndCenter(t *testing.T) {
	for _, tc := range []struct {
		name   string
		height float32
	}{{"fits", 120}, {"small_margin", 360}, {"taller_than_viewport", 600}} {
		t.Run(tc.name, func(t *testing.T) {
			ct := newEditorTest(t, "", strings.Repeat("top\n", 20)+"heading\n"+strings.Repeat("end\n", 100))
			ct.st.ScrollMargin = 3
			ct.st.SetLineStyles([]LineStyle{{Line: 20, Font: ui.Font{Size: 40}, LineHeight: tc.height / 40}})
			ct.styledFrames()
			ed := &ct.st.ed
			at := ct.st.Buffer.Offset(Pos{Line: 20})
			top := float64(20 * ed.view.lineHeight)
			cr, ok := ed.caretRect(at)
			if !ok || cr.H != tc.height || float64(cr.Y) != top {
				t.Fatalf("styled caret %+v (%v), want height %v at %v", cr, ok, tc.height, top)
			}
			ct.st.Select(at, at)
			ct.styledFrames()
			margin := min(3*ed.view.lineHeight, max(0, (ed.viewH-min(tc.height, ed.viewH))/2))
			if want := top + float64(tc.height+margin-ed.viewH); ed.scrollY != want {
				t.Fatalf("reveal scrolled to %v, want %v for the whole styled row and margin %v", ed.scrollY, want, margin)
			}
			center := max(0, (ed.viewH-tc.height)/2)
			want := top - float64(center)
			ct.run(ScrollCursorCenter)
			if ed.scrollY != want {
				t.Fatalf("center command scrolled to %v, want %v", ed.scrollY, want)
			}
			ct.st.SetScrollY(0)
			ed.reveal = revealCenter
			ct.styledFrames()
			if ed.scrollY != want {
				t.Fatalf("center reveal scrolled to %v, want %v", ed.scrollY, want)
			}
			ct.st.Select(0, 0)
			ct.st.SetScrollY(0)
			ct.styledFrames()
			ct.run(GoToLine)
			ed.goTo.text = "21" // Go-to line numbers start at 1.
			ct.styledFrames()
			if ed.goTo.preview != 20 || ed.scrollY != want || ct.st.Cursor() != 0 {
				t.Fatalf("go-to preview %d at %v (caret %d), want line 20 at %v without moving the caret", ed.goTo.preview, ed.scrollY, ct.st.Cursor(), want)
			}
		})
	}
}

// Another buffer takes none of the styles of the last.
func TestLineStylesNewBuffer(t *testing.T) {
	ct := newEditorTest(t, "", "one\ntwo")
	ct.st.SetLineStyles([]LineStyle{{Line: 1, Font: ui.Font{Size: 30}}})
	ct.styledFrames()
	ct.st.Buffer = NewBuffer("a\nb\nc")
	ct.styledFrames()
	v := &ct.st.ed.view
	if len(ct.st.LineStyles()) != 0 || v.rowHeight(1) != v.lineHeight {
		t.Fatal("the styles outlived their buffer")
	}
}
