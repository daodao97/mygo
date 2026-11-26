package editor

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// newScrollTest makes an editor of text in a window 600 by 400, with
// the keyboard, and renders its first frame.
func newScrollTest(text string) (*State, *ui.Tester) {
	st := &State{Buffer: NewBuffer(text)}
	tt := ui.NewTester(func(c *ui.Context) {
		View(c, st).Fill().AutoFocus()
	}, 600, 400)
	tt.Frame()
	return st, tt
}

// scrolledRight scrolls the editor of st all the way right, failing when
// it does not scroll sideways.
func scrolledRight(t *testing.T, st *State, tt *ui.Tester) {
	t.Helper()
	if _, horizontal := st.ed.scrollbars(); !horizontal {
		t.Fatal("setup: expected a horizontal scrollbar")
	}
	tt.Scroll(250, 200, 10_000, 0)
	if st.ed.scrollX <= 0 || st.ed.scrollX != st.ed.maxScrollX() {
		t.Fatalf("setup: scrolled sideways to %v of %v", st.ed.scrollX, st.ed.maxScrollX())
	}
}

// fits fails unless the lines fit the editor again: no horizontal
// scroll bar, and the text at the left edge.
func fits(t *testing.T, st *State) {
	t.Helper()
	if _, horizontal := st.ed.scrollbars(); horizontal {
		t.Error("horizontal scrollbar is still enabled after the text fits")
	}
	if x := st.ed.scrollX; x != 0 {
		t.Errorf("horizontal offset = %v, want 0", x)
	}
}

// The reproduction of #2043: an app replacing a long line with a short
// one takes the horizontal scroll bar and offset away.
func TestHorizontalScrollbarAfterShortening(t *testing.T) {
	st := &State{Buffer: NewBuffer(strings.Repeat("x", 500))}
	tt := ui.NewTester(func(c *ui.Context) {
		View(c, st).Fill()
	}, 600, 400)
	tt.Frame()

	if _, horizontal := st.ed.scrollbars(); !horizontal {
		t.Fatal("setup: expected a horizontal scrollbar")
	}
	tt.Scroll(250, 200, 10_000, 0)
	if st.ed.scrollX <= 0 {
		t.Fatal("setup: expected a positive horizontal offset")
	}

	st.Buffer.Set("short")
	tt.Frame()

	if _, horizontal := st.ed.scrollbars(); horizontal {
		t.Error("horizontal scrollbar is still enabled after the text fits")
	}
	if x := st.ed.scrollX; x != 0 {
		t.Errorf("horizontal offset = %v, want 0", x)
	}
}

// Deleting most of the widest line by typing takes the bar away too.
func TestHorizontalScrollbarAfterDeleting(t *testing.T) {
	st, tt := newScrollTest(strings.Repeat("x", 500) + "\nshort")
	scrolledRight(t, st, tt)
	st.Select(5, 500)
	tt.Frame()
	tt.Key(0, ui.KeyBackspace)
	fits(t, st)
}

// A narrower line partly in view leaves the offset as far right as the
// widest line takes it, no farther.
func TestHorizontalScrollClampsToNarrowerLine(t *testing.T) {
	st, tt := newScrollTest(strings.Repeat("x", 500) + "\n" + strings.Repeat("y", 200))
	scrolledRight(t, st, tt)
	before := st.ed.scrollX
	st.Buffer.Edit(Edit{Start: 0, End: 500, Text: "x"})
	tt.Frame()
	if _, horizontal := st.ed.scrollbars(); !horizontal {
		t.Fatal("the line of 200 characters no longer scrolls sideways")
	}
	if m := st.ed.maxScrollX(); st.ed.scrollX != m || m <= 0 || m >= before {
		t.Errorf("horizontal offset = %v of %v, want the narrower line's end, short of %v", st.ed.scrollX, m, before)
	}
}

// longOutOfView is a text whose second line is wide and the others
// short, the wide line scrolled out of view.
func longOutOfView(t *testing.T) (*State, *ui.Tester) {
	t.Helper()
	st, tt := newScrollTest("first\n" + strings.Repeat("x", 500) + strings.Repeat("\nshort", 300))
	st.SetScrollY(st.LineTop(150))
	tt.Frame()
	if st.ed.shown[0] <= 1 {
		t.Fatal("setup: the long line is in view")
	}
	return st, tt
}

// Editing a line keeps the width of a long line out of view, which is
// still there: its scroll bar stays, with the caret on a short line.
func TestHorizontalScrollKeepsLinesOutOfView(t *testing.T) {
	st, tt := longOutOfView(t)
	at := st.Buffer.Offset(Pos{Line: 200})
	st.Select(at, at)
	tt.Frame()
	if _, horizontal := st.ed.scrollbars(); !horizontal {
		t.Fatal("setup: expected a horizontal scrollbar")
	}
	// An edit of another line, one of new lines above the long one, one
	// of lines between them, and one taking lines away.
	for _, e := range []Edit{
		{Start: st.Buffer.Offset(Pos{Line: 180}), Text: "z"},
		{Start: 0, Text: "a\nb\n"},
		{Start: st.Buffer.Offset(Pos{Line: 50}), Text: "c\nd\ne\n"},
		{Start: st.Buffer.Offset(Pos{Line: 10}), End: st.Buffer.Offset(Pos{Line: 40})},
	} {
		st.Buffer.Edit(e)
		tt.Frame()
		if _, horizontal := st.ed.scrollbars(); !horizontal {
			t.Fatalf("after %+v, no horizontal scrollbar, with the long line out of view", e)
		}
	}
	// The long line itself edited out of view: its width is not known,
	// and nothing in view scrolls sideways.
	long := strings.Index(st.Buffer.String(), strings.Repeat("x", 500))
	st.Buffer.Edit(Edit{Start: long, End: long + 500, Text: "x"})
	tt.Frame()
	fits(t, st)
}

// The caret on a long line scrolled out of view keeps the offset as an
// edit of another line goes by.
func TestHorizontalScrollKeepsOffsetOfCaretOutOfView(t *testing.T) {
	st, tt := newScrollTest(strings.Repeat("x", 500) + strings.Repeat("\nshort", 300))
	st.Select(60, 60)
	tt.Frame()
	tt.Scroll(250, 200, 300-st.ed.scrollX, 0)
	if st.ed.scrollX != 300 {
		t.Fatalf("setup: scrolled sideways to %v", st.ed.scrollX)
	}
	st.SetScrollY(st.LineTop(150))
	tt.Frame()
	at := st.Buffer.Offset(Pos{Line: 200})
	st.Buffer.Edit(Edit{Start: at, Text: "z"})
	tt.Frame()
	if st.ed.scrollX != 300 {
		t.Errorf("horizontal offset = %v after an edit of another line, want 300", st.ed.scrollX)
	}
}

// Scrolling sideways between an edit and the next frame keeps the range
// of the last frame, rather than the lines not laid out yet's.
func TestHorizontalScrollBetweenEditAndFrame(t *testing.T) {
	st, tt := newScrollTest(strings.Repeat("x", 500))
	scrolledRight(t, st, tt)
	before := st.ed.scrollX
	st.Buffer.Edit(Edit{Start: 500, Text: "y"})
	st.ensure()
	tt.Scroll(250, 200, -10, 0)
	if x := st.ed.scrollX; x < before-11 || x > before-9 {
		t.Errorf("horizontal offset = %v after scrolling 10 left of %v", x, before)
	}
}

// Another tab size makes lines of tabs narrower.
func TestHorizontalScrollAfterTabSize(t *testing.T) {
	st, tt := newScrollTest(strings.Repeat("\t", 60) + "x")
	st.TabSize = 8
	tt.Frame()
	scrolledRight(t, st, tt)
	st.TabSize = 1
	tt.Frame()
	fits(t, st)
}

// Taking a wide inlay away makes its line narrower.
func TestHorizontalScrollAfterInlay(t *testing.T) {
	st, tt := newScrollTest("short\nline")
	st.SetInlays([]Inlay{{Offset: 5, Text: strings.Repeat("x", 500)}})
	tt.Frame()
	scrolledRight(t, st, tt)
	st.SetInlays(nil)
	tt.Frame()
	fits(t, st)
}

// Setting inlays every frame forgets the widths of their lines alone: a
// long line out of view still scrolls sideways.
func TestHorizontalScrollInlaysEveryFrame(t *testing.T) {
	st, tt := longOutOfView(t)
	at := st.Buffer.Offset(Pos{Line: 160})
	for i := range 5 {
		st.SetInlays([]Inlay{{Offset: at, Text: strings.Repeat("i", i+1)}})
		tt.Frame()
		if _, horizontal := st.ed.scrollbars(); !horizontal {
			t.Fatalf("frame %d: no horizontal scrollbar, with the long line out of view", i)
		}
	}
}

// A replacement showing a long range as a few characters makes the line
// narrower.
func TestHorizontalScrollAfterReplacement(t *testing.T) {
	st, tt := newScrollTest(strings.Repeat("x", 500) + "\nshort")
	scrolledRight(t, st, tt)
	st.SetReplacements([]Replacement{{Start: 5, End: 500, Text: "…"}})
	tt.Frame()
	fits(t, st)
}

// A wider window brings the offset into its range.
func TestHorizontalScrollAfterWiderWindow(t *testing.T) {
	st, tt := newScrollTest(strings.Repeat("x", 100))
	scrolledRight(t, st, tt)
	tt.SetSize(2000, 400)
	fits(t, st)
}

// An edit lays out the lines in view again, not every line the editor
// measured: the widths it keeps are of a few lines, however many it
// laid out.
func TestHorizontalScrollEditCost(t *testing.T) {
	var b strings.Builder
	for i := range 2000 {
		b.WriteString(strings.Repeat("w", 100+i%300))
		b.WriteByte('\n')
	}
	st, tt := newScrollTest(b.String())
	for y := 0; y < 2000; y += 20 {
		st.SetScrollY(st.LineTop(y))
		tt.Frame()
	}
	if n := len(st.ed.view.measured.e); n > maxLineWidths {
		t.Fatalf("%d widths kept, want at most %d", n, maxLineWidths)
	}
	st.SetScrollY(st.LineTop(1000))
	tt.Frame()
	widest := st.ed.contentWidth()
	at := st.Buffer.Offset(Pos{Line: 1010})
	st.Select(at, at)
	tt.Frame()
	tt.Type("z")
	shown := st.ed.shown[1] - st.ed.shown[0] + 1
	if n := len(st.ed.view.lines.m); n > shown+1 {
		t.Errorf("%d lines laid out after an edit, with %d in view", n, shown)
	}
	if w := st.ed.contentWidth(); w != widest {
		t.Errorf("content %v wide after an edit of a narrow line, want %v", w, widest)
	}
}

func TestLineWidths(t *testing.T) {
	var lw lineWidths
	lw.set(3, 100)
	lw.set(5, 300)
	lw.set(9, 200)
	lw.set(5, 250) // laid out again, the same text: as wide still
	if w := lw.widest(); w != 300 {
		t.Fatalf("widest %v, want 300", w)
	}
	// Two lines inserted at 4: 5 and 9 move to 7 and 11.
	lw.splice(4, 1, 3)
	lw.forget(7, 7)
	if w := lw.widest(); w != 200 {
		t.Fatalf("widest %v after the widest line was forgotten, want 200", w)
	}
	// Lines 10 to 12 taken away, 11 among them.
	lw.splice(10, 3, 1)
	if w := lw.widest(); w != 100 {
		t.Fatalf("widest %v after the lines were taken away, want 100", w)
	}
	lw.reset()
	if w := lw.widest(); w != 0 {
		t.Fatalf("widest %v after reset, want 0", w)
	}
	// Only the widest are kept.
	for i := range 3 * maxLineWidths {
		lw.set(i, float32(i))
	}
	if len(lw.e) != maxLineWidths {
		t.Fatalf("%d widths kept, want %d", len(lw.e), maxLineWidths)
	}
	for _, e := range lw.e {
		if e.line < 2*maxLineWidths {
			t.Fatalf("line %d kept, narrower than %d others", e.line, maxLineWidths)
		}
	}
}

// The reproduction of #2382: Shift turns a mouse's wheel sideways, as it
// does in scroll containers, both ways; the wheel alone still scrolls
// down, and a sideways or diagonal move with Shift keeps its own axes.
func TestShiftWheelScrollsSideways(t *testing.T) {
	st, tt := newScrollTest(strings.Repeat(strings.Repeat("x", 500)+"\n", 100))
	if _, horizontal := st.ed.scrollbars(); !horizontal {
		t.Fatal("setup: expected a horizontal scrollbar")
	}
	tt.HoldModifiers(ui.Shift)
	tt.Scroll(250, 200, 0, 60)
	if st.ed.scrollX != 60 || st.ed.scrollY != 0 {
		t.Fatalf("Shift and the wheel down scrolled to (%v, %v), want (60, 0)", st.ed.scrollX, st.ed.scrollY)
	}
	tt.Scroll(250, 200, 0, -20)
	if st.ed.scrollX != 40 || st.ed.scrollY != 0 {
		t.Errorf("Shift and the wheel up scrolled to (%v, %v), want (40, 0)", st.ed.scrollX, st.ed.scrollY)
	}
	tt.Scroll(250, 200, 10, 30)
	if st.ed.scrollX != 50 || st.ed.scrollY != 30 {
		t.Errorf("Shift and a diagonal move scrolled to (%v, %v), want (50, 30)", st.ed.scrollX, st.ed.scrollY)
	}
	tt.HoldModifiers(0)
	tt.Scroll(250, 200, 0, 30)
	if st.ed.scrollX != 50 || st.ed.scrollY != 60 {
		t.Errorf("the wheel alone scrolled to (%v, %v), want (50, 60)", st.ed.scrollX, st.ed.scrollY)
	}
}

// With its lines wrapped, an editor has nothing to scroll sideways: Shift
// and the wheel go on to the containers around it, which may.
func TestShiftWheelPassesOnWhenWrapped(t *testing.T) {
	st := &State{Buffer: NewBuffer(strings.Repeat(strings.Repeat("x", 500)+"\n", 100)), SoftWrap: true}
	outer := &ui.ScrollState{}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.ScrollHorizontal(c).TrackScroll(outer).Size(600, 400).Children(func() {
			View(c, st).Size(1200, 400)
		})
	}, 600, 400)
	tt.Frame()
	tt.HoldModifiers(ui.Shift)
	tt.Scroll(250, 200, 0, 60)
	tt.Frame()
	if st.ed.scrollY != 0 || outer.X == 0 {
		t.Errorf("Shift and the wheel scrolled the editor to %v and its container to %v, want 0 and more", st.ed.scrollY, outer.X)
	}
}
