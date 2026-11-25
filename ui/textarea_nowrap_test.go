package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

// noWrapURLs is a list of URLs, one a line, the first far wider than a
// text area 300 DIPs wide.
var noWrapURLs = "https://example.com/" + strings.Repeat("a-long-path/", 30) + "end\nhttps://example.org/short\nlast"

// noWrapArea builds a text area 300×120 with s, NoWrap or not, and returns
// its Tester, its state, its lines and its scroll.
func noWrapArea(t *testing.T, s *string, noWrap bool) (*Tester, *state, *TextLines, *ScrollState) {
	t.Helper()
	var lines TextLines
	var scroll ScrollState
	tt := NewTester(func(c *Context) {
		in := TextArea(c, s).Width(300).Height(120).TrackLines(&lines).TrackScroll(&scroll)
		if noWrap {
			in.NoWrap()
		}
	}, 400, 300)
	for _, st := range tt.rt.states {
		if st.editor != nil {
			return tt, st, &lines, &scroll
		}
	}
	t.Fatal("no text area")
	return nil, nil, nil, nil
}

// TestTextAreaNoWrapKeepsLines checks that a NoWrap text area keeps a line
// wider than it on one row, where a text area wraps it by default.
func TestTextAreaNoWrapKeepsLines(t *testing.T) {
	s := noWrapURLs
	_, wrapped, lines, _ := noWrapArea(t, &s, false)
	if rows := lines.Rows(0); len(rows) < 2 {
		t.Fatalf("a text area wraps a long line by default: rows %v", rows)
	}
	if wrapped.flags&flagScrollX != 0 || wrapped.contentW != float64(wrapped.w) {
		t.Errorf("a text area that wraps scrolls sideways, over %v", wrapped.contentW)
	}
	s2 := noWrapURLs
	_, st, lines, scroll := noWrapArea(t, &s2, true)
	for i := range lines.Count() {
		if rows := lines.Rows(i); len(rows) != 1 {
			t.Errorf("NoWrap: line %d on rows %v, want one", i, rows)
		}
	}
	if st.flags&flagScrollX == 0 {
		t.Error("a NoWrap text area does not scroll sideways")
	}
	if scroll.MaxX <= 0 || scroll.X != 0 {
		t.Errorf("NoWrap: scrolled to %v of %v sideways, want 0 of more", scroll.X, scroll.MaxX)
	}
	// A horizontal scroll bar, where the system shows the vertical one.
	if g := st.scrollbarGeometry(st.x, st.y, st.contentW, st.contentH, st.scrollX, st.scrollY); !g.horizontal {
		t.Error("NoWrap: no horizontal scroll bar")
	}
}

// TestTextAreaNoWrapCaretInView moves the caret to the end of a long line
// of a NoWrap text area and back, and checks that it stays in view, the
// area scrolling sideways with it, and that a press hits the rune shown
// under the pointer once scrolled.
func TestTextAreaNoWrapCaretInView(t *testing.T) {
	s := noWrapURLs
	tt, st, lines, scroll := noWrapArea(t, &s, true)
	ed := st.editor
	inView := func(what string) {
		t.Helper()
		x, _, _ := ed.area.caretAt(ed, ed.caret, ed.composeCaret)
		if at := x - ed.scrollX; at < 0 || at > ed.contentW {
			t.Errorf("%s: the caret at %v is out of the view %v wide (scrolled %v)", what, at, ed.contentW, ed.scrollX)
		}
		r := ed.caretRect(st)
		if box := (Rect{st.x, st.y, st.w, st.h}); r.X < box.X || r.X > box.X+box.W {
			t.Errorf("%s: input methods see the caret at %v, out of %v", what, r, box)
		}
	}
	tt.Press(20, 20)
	tt.Release(20, 20)
	tt.Key(Ctrl, KeyHome)
	if ed.caret != 0 || ed.scrollX != 0 {
		t.Fatalf("at the start, caret %d scrolled %v", ed.caret, ed.scrollX)
	}
	tt.Key(0, KeyEnd)
	end := strings.IndexByte(s, '\n')
	if ed.caret != end {
		t.Fatalf("End took the caret to %d, want the line's end %d", ed.caret, end)
	}
	if ed.scrollX <= 0 || scroll.X <= 0 {
		t.Fatalf("at the end of the long line the area scrolled %v (state %v)", ed.scrollX, scroll.X)
	}
	inView("End")
	if len(lines.Rows(0)) != 1 {
		t.Error("the long line wrapped")
	}

	// A press hits what the view shows, scrolled.
	x, y := st.x+ed.originX+100, st.y+ed.originY+5
	l := ed.area.paraLayout(ed, 0)
	want := l.PositionAt(100+ed.scrollX, 5).Index
	if unscrolled := l.PositionAt(100, 5).Index; want == unscrolled {
		t.Fatalf("the view is not scrolled far enough to tell: rune %d", want)
	}
	tt.Press(x, y)
	tt.Release(x, y)
	if ed.caret != want || ed.anchor != want {
		t.Errorf("a press at 100 DIPs in hit rune %d-%d, want %d", ed.anchor, ed.caret, want)
	}
	if bounds, _, ok := ed.client.BoundsForRange(TextInputRange{Start: want, End: want}); !ok || abs(bounds.X-(ed.originX+100)) > 8 {
		t.Errorf("the caret's bounds are at %v, want by the press at %v", bounds, ed.originX+100)
	}
	if i, ok := ed.client.IndexForPoint(Point{ed.originX + 100, ed.originY + 5}); !ok || i != want {
		t.Errorf("IndexForPoint at the press: %d, want %d", i, want)
	}

	// Home goes back to the start of the line, and the view with it.
	tt.Key(0, KeyHome)
	if ed.caret != 0 || ed.scrollX != 0 || scroll.X != 0 {
		t.Errorf("Home: caret %d, scrolled %v (state %v)", ed.caret, ed.scrollX, scroll.X)
	}
	// Down and up keep to the lines: the second is short, and the view
	// follows the caret to it.
	tt.Key(0, KeyEnd)
	tt.Key(0, KeyDown)
	if p := ed.buf.para(ed.caret); p != 1 {
		t.Fatalf("Down went to line %d", p)
	}
	inView("Down")
	// Typing at the end of the long line keeps the caret in view.
	tt.Key(0, KeyUp)
	tt.Key(0, KeyEnd)
	tt.Type(strings.Repeat("x", 40))
	inView("typing")
	if len(lines.Rows(0)) != 1 {
		t.Error("typing wrapped the long line")
	}
	// An input method's composition at the end shows too.
	tt.Compose(strings.Repeat("か", 20), 20)
	inView("composing")
}

// TestTextAreaNoWrapWheel scrolls a NoWrap text area sideways with a
// touchpad, and with Shift and a mouse's wheel.
func TestTextAreaNoWrapWheel(t *testing.T) {
	s := noWrapURLs
	tt, st, _, scroll := noWrapArea(t, &s, true)
	ed := st.editor
	tt.Scroll(st.x+50, st.y+50, 120, 0)
	tt.Frame()
	if scroll.X != 120 || ed.scrollX != 120 {
		t.Fatalf("the touchpad scrolled %v (area %v), want 120", scroll.X, ed.scrollX)
	}
	// The wheel turned sideways by Shift.
	tt.send(platform.SurfaceEvent{Kind: platform.PointerScroll, X: float64(st.x + 50), Y: float64(st.y + 50), DY: 60, Mods: platform.ModShift, Precise: true})
	tt.Frame()
	if scroll.X != 180 || ed.scrollX != 180 {
		t.Errorf("Shift and the wheel scrolled to %v (area %v), want 180", scroll.X, ed.scrollX)
	}
	// As far as the long line goes, with room for the caret after it.
	tt.Scroll(st.x+50, st.y+50, 1e6, 0)
	tt.Frame()
	l := ed.area.paraLayout(ed, 0)
	if want := l.Width + 1 - ed.contentW; abs(ed.scrollX-want) > 0.5 || abs(scroll.MaxX-want) > 0.5 {
		t.Errorf("scrolled to the end at %v of %v, want %v", ed.scrollX, scroll.MaxX, want)
	}
	// The press hits the text under it, and the caret stays where it is
	// in view: the view does not jump back.
	at := ed.scrollX
	tt.Press(st.x+ed.originX+10, st.y+ed.originY+5)
	tt.Release(st.x+ed.originX+10, st.y+ed.originY+5)
	if ed.scrollX != at {
		t.Errorf("a press moved the view from %v to %v", at, ed.scrollX)
	}
	if want := l.PositionAt(at+10, 5).Index; ed.caret != want {
		t.Errorf("the press hit rune %d, want %d", ed.caret, want)
	}
	// The app's ScrollState scrolls it too.
	scroll.X = 0
	tt.Frame()
	if ed.scrollX != 0 {
		t.Errorf("ScrollState.X = 0 left the area scrolled %v", ed.scrollX)
	}
}

// TestTextAreaNoWrapDrag selects past the right side of a NoWrap text
// area: the view scrolls along, frame after frame while the pointer stays
// out, and the selection follows it.
func TestTextAreaNoWrapDrag(t *testing.T) {
	s := noWrapURLs
	tt, st, _, _ := noWrapArea(t, &s, true)
	ed := st.editor
	y := st.y + ed.originY + 5
	tt.Press(st.x+ed.originX+5, y)
	tt.Key(Ctrl, KeyHome) // nothing of the caret's at the end to reveal
	tt.Release(st.x+ed.originX+5, y)
	tt.Press(st.x+ed.originX+5, y)
	tt.Move(st.x+st.w+20, y)
	first := ed.scrollX
	if first <= 0 {
		t.Fatalf("a drag past the right side did not scroll: %v", first)
	}
	tt.Advance(100 * time.Millisecond)
	if ed.scrollX <= first {
		t.Errorf("with the pointer still out, the view stayed at %v", ed.scrollX)
	}
	tt.Release(st.x+st.w+20, y)
	if a, z := ed.selection(); a != 0 || z <= 0 {
		t.Errorf("the drag selected %d-%d", a, z)
	}
	x, _, _ := ed.area.caretAt(ed, ed.caret, 0)
	if at := x - ed.scrollX; at < 0 || at > ed.contentW {
		t.Errorf("the selection's end at %v is out of view", at)
	}
}

// TestTextAreaNoWrapRTL scrolls a NoWrap text area laid out right to left,
// whose long line starts at its right and goes past its left: the view
// shows its start, and scrolls left to its end.
func TestTextAreaNoWrapRTL(t *testing.T) {
	s := strings.Repeat("שלום עולם ", 40) + "\nקצר"
	var scroll ScrollState
	tt := NewTester(func(c *Context) {
		TextArea(c, &s).Width(300).Height(120).NoWrap().TrackScroll(&scroll)
	}, 400, 300)
	tt.SetDirection(RightToLeft)
	tt.Frame()
	var st *state
	for _, x := range tt.rt.states {
		if x.editor != nil {
			st = x
		}
	}
	ed := st.editor
	l := ed.area.paraLayout(ed, 0)
	if len(l.Lines) != 1 || l.Lines[0].X >= 0 {
		t.Fatalf("the long line is laid out %v, want one line going past the left", l.Lines)
	}
	if ed.scrollX != 0 || scroll.X != 0 || scroll.MaxX <= 0 {
		t.Fatalf("the view starts scrolled %v (state %v of %v), want at the start", ed.scrollX, scroll.X, scroll.MaxX)
	}
	// Scrolling toward the end goes left.
	tt.Scroll(st.x+50, st.y+50, -100, 0)
	tt.Frame()
	if scroll.X != 100 || ed.scrollX != -100 {
		t.Errorf("scrolled left to %v (area %v), want 100 from the right", scroll.X, ed.scrollX)
	}
	tt.Press(st.x+st.w-20, st.y+ed.originY+5)
	tt.Release(st.x+st.w-20, st.y+ed.originY+5)
	tt.Key(0, KeyEnd)
	x, _, _ := ed.area.caretAt(ed, ed.caret, 0)
	if at := x - ed.scrollX; at < 0 || at > ed.contentW || ed.scrollX != l.Lines[0].X {
		t.Errorf("at the end, the caret at %v of the view scrolled %v, want %v", at, ed.scrollX, l.Lines[0].X)
	}
}

// TestTextAreaNoWrapStateless checks that NoWrap tells a text area by the
// control it is, not by its state: a buffer's text area scrolls sideways
// too, and Key may follow NoWrap on other elements.
func TestTextAreaNoWrapStateless(t *testing.T) {
	buf := NewTextBuffer(noWrapURLs)
	tt := NewTester(func(c *Context) {
		Text(c, "label").NoWrap().Key("label")
		TextAreaBuffer(c.Key("area"), buf).NoWrap().Width(300).Height(120)
	}, 400, 300)
	tt.Frame()
	for _, st := range tt.rt.states {
		if st.editor != nil {
			if st.flags&flagScrollX == 0 || st.contentW <= float64(st.w) {
				t.Errorf("a NoWrap buffer area does not scroll sideways: flags %b, content %v of %v", st.flags, st.contentW, st.w)
			}
			return
		}
	}
	t.Fatal("no text area")
}

// TestTextAreaNoWrapEditsExtent checks that edits which add and remove
// paragraphs keep each paragraph's extent with it: the content stays as
// wide as an area built afresh with the same text measures it.
func TestTextAreaNoWrapEditsExtent(t *testing.T) {
	long := strings.Repeat("wide ", 40)
	s := "a\n" + long + "\nb\nc"
	tt, st, _, scroll := noWrapArea(t, &s, true)
	check := func(what string) {
		t.Helper()
		tt.Frame()
		fresh := s
		_, _, _, want := noWrapArea(t, &fresh, true)
		if scroll.MaxX != want.MaxX {
			t.Errorf("%s: scrolls %v sideways, an area built afresh %v", what, scroll.MaxX, want.MaxX)
		}
	}
	tt.Press(20, 20)
	tt.Release(20, 20)
	tt.Key(Ctrl, KeyHome)
	tt.Key(0, KeyEnd)
	tt.Key(0, KeyEnter) // a paragraph after "a"
	tt.Key(0, KeyEnter)
	check("two paragraphs added before the wide one")
	tt.Key(0, KeyBackspace)
	tt.Key(0, KeyBackspace)
	check("removed again")
	tt.Key(0, KeyDown)
	tt.Key(0, KeyHome)
	for range 20 {
		tt.Key(Shift, KeyRight)
	}
	tt.Key(0, KeyBackspace) // the wide one narrower
	check("the widest paragraph narrowed")
	tt.Key(Ctrl, KeyHome)
	tt.Key(Shift|Ctrl, KeyEnd)
	tt.Key(0, KeyBackspace)
	check("all deleted")
	if st.contentW > float64(st.w) {
		t.Errorf("an empty area is %v wide in a box %v wide", st.contentW, st.w)
	}
}
