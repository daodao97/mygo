package editor

import (
	"strings"
	"testing"
)

// Rect puts an offset where the text shows in the editor's box: right of
// the gutter, a row per line, a column per character, and moved by the
// scroll; a folded line has none.
func TestStateRect(t *testing.T) {
	ct := newEditorTest(t, "", "ab\ncdef\ng\nh|")
	st := ct.st
	r0, ok := st.Rect(0)
	if !ok || r0.X != st.ed.textX || r0.Y != 0 {
		t.Fatalf("Rect(0) = %v %v, want X %v Y 0", r0, ok, st.ed.textX)
	}
	if r0.H != st.ed.view.lineHeight {
		t.Fatalf("Rect height %v, want a row %v", r0.H, st.ed.view.lineHeight)
	}
	r3, _ := st.Rect(3)
	if r3.X != r0.X || r3.Y != st.ed.view.lineHeight {
		t.Fatalf("Rect(3), the start of line 1 = %v, want X %v Y %v", r3, r0.X, st.ed.view.lineHeight)
	}
	r5, _ := st.Rect(5)
	if got := r5.X - r3.X; got != 2*st.ed.view.charWidth {
		t.Fatalf("two columns in take %v, want %v", got, 2*st.ed.view.charWidth)
	}
	// Past the end clamps to the end.
	end, _ := st.Rect(st.ed.view.text.Len())
	if far, _ := st.Rect(1 << 20); far != end {
		t.Fatalf("Rect past the end = %v, want %v", far, end)
	}
	// Folds hide a line's rect.
	ct = newEditorTest(t, "go", "func a() {\n\tx()\n}\nz|")
	ct.st.Fold(0)
	ct.tt.Frame()
	if _, ok := ct.st.Rect(len("func a() {\n")); ok {
		t.Fatal("a folded line has a Rect")
	}
	if r, ok := ct.st.Rect(len("func a() {\n\tx()\n}\n")); !ok || r.Y != 2*ct.st.ed.view.lineHeight {
		t.Fatalf("the line after the fold is at %v %v, want two rows down", r, ok)
	}
	// Soft wrap puts a later offset of a long line on a lower row.
	ct = newEditorTest(t, "", strings.Repeat("x", 150)+"|")
	ct.st.SoftWrap = true
	ct.tt.Frame()
	ct.tt.Frame()
	a, _ := ct.st.Rect(0)
	b, _ := ct.st.Rect(149)
	if b.Y <= a.Y {
		t.Fatalf("a wrapped line's end at Y %v isn't below its start %v", b.Y, a.Y)
	}
}

// TextX is where an unscrolled line starts, and stays there as the text
// scrolls sideways.
func TestStateTextX(t *testing.T) {
	ct := newEditorTest(t, "", strings.Repeat("x", 400)+"\n|")
	st := ct.st
	st.ed.scrollX = 0
	r, _ := st.Rect(0)
	if x := st.TextX(); x <= 0 || r.X != x {
		t.Fatalf("TextX %v, want the unscrolled start of a line %v, right of the gutter", x, r.X)
	}
	st.ed.scrollX = 100
	r, _ = st.Rect(0)
	if x := st.TextX(); r.X != x-100 {
		t.Fatalf("scrolled 100 right, TextX %v and the start of a line %v", x, r.X)
	}
}

// Scrolled, a Rect moves with the text, left and up.
func TestStateRectScrolled(t *testing.T) {
	ct := newEditorTest(t, "", strings.Repeat("x", 400)+"\n"+strings.Repeat("y\n", 60)+"|")
	st := ct.st
	st.SetScrollY(0)
	before, _ := st.Rect(300)
	st.ed.scrollX = 100
	st.SetScrollY(2 * float64(st.ed.view.lineHeight))
	after, _ := st.Rect(300)
	if before.X-after.X != 100 {
		t.Fatalf("scrolled 100 right, X moved %v", before.X-after.X)
	}
	if before.Y-after.Y != 2*st.ed.view.lineHeight {
		t.Fatalf("scrolled two rows, Y moved %v", before.Y-after.Y)
	}
}
