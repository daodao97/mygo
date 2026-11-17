package ui

import (
	"testing"
)

// A style's zero fields follow the theme: the thumb's color and width,
// a bar six DIPs wider than the thumb, half its width as the corner, and
// 24 DIPs as the shortest thumb. A thumb wider than its bar is cut to it.
func TestScrollbarStyleResolve(t *testing.T) {
	th := *DarkTheme()
	th.Scrollbar, th.ScrollbarWidth = RGB(10, 20, 30), 8
	s := ScrollbarStyle{}.Resolve(&th)
	if s.TrackWidth != 14 || s.ThumbWidth != 8 || s.Radius != 4 || s.MinLength != 24 {
		t.Errorf("the defaults are %+v, want a 14 DIP bar with an 8 DIP thumb, radius 4, at least 24 long", s)
	}
	if s.Thumb != th.Scrollbar || s.Hover != th.Scrollbar || s.Active != th.Scrollbar || s.Track != Transparent {
		t.Errorf("the default colors are %+v, want the theme's thumb and no track", s)
	}
	track, hover := RGB(1, 2, 3), RGB(4, 5, 6)
	s = ScrollbarStyle{TrackWidth: 8, ThumbWidth: 20, Radius: 1, MinLength: 5, Track: track, Hover: hover}.Resolve(&th)
	if s.ThumbWidth != 8 {
		t.Errorf("a thumb %v wide in an 8 DIP bar stays %v wide", 20, s.ThumbWidth)
	}
	if s.Radius != 1 || s.MinLength != 5 || s.Track != track || s.Hover != hover || s.Active != th.Scrollbar {
		t.Errorf("the style is %+v, want its own radius, length, track and hover, and the theme's active", s)
	}
	if s := (ScrollbarStyle{Square: true}).Resolve(&th); s.Radius != 0 || !s.Square {
		t.Errorf("a square style has radius %v and square %v, want 0 and true", s.Radius, s.Square)
	}
	if s := (ScrollbarStyle{Square: true, Radius: 2}).Resolve(&th); s.Radius != 2 || s.Square {
		t.Errorf("a radius with square has radius %v and square %v, want 2 and false", s.Radius, s.Square)
	}
	if s := (ScrollbarStyle{Flush: true}).Resolve(&th); !s.Flush {
		t.Error("a flush style is not flush")
	}
}

// Flush leaves the thumb at the ends of its track, as VS Code's scroll
// bar, and OffsetAt undoes Thumb there.
func TestScrollbarStyleFlush(t *testing.T) {
	style := ScrollbarStyle{TrackWidth: 14, ThumbWidth: 14, Square: true, Flush: true, MinLength: 20}
	s := Scrollbar{Box: Rect{0, 0, 100, 100}, View: 100, Content: 1000, Style: style}
	if start, length := s.Thumb(); start != 0 || length != 20 {
		t.Fatalf("at the top the thumb is %v long at %v, want 20 at 0", length, start)
	}
	s.Offset = 900
	if start, _ := s.Thumb(); start != 80 {
		t.Errorf("at the end the thumb starts at %v, want 80", start)
	}
	if got := s.OffsetAt(80); got != 900 {
		t.Errorf("OffsetAt(80) = %v, want 900", got)
	}
	s.Style.Flush, s.Offset = false, 0
	if start, _ := s.Thumb(); start != 2 {
		t.Errorf("with a gap the thumb starts at %v, want 2", start)
	}
	s.Offset = 900
	if start, _ := s.Thumb(); start != 78 {
		t.Errorf("with a gap at the end the thumb starts at %v, want 78", start)
	}
}

// A Scrollbar paints the thumb a scroll container shows for the same
// view, content and offset, pixel for pixel, dragged or not; and OffsetAt
// undoes Thumb.
func TestScrollbarLikeContainer(t *testing.T) {
	const content = 600
	var offset float64
	var dragging bool
	tt := coreNewTester(func(c *context) {
		coreRow(c).Size(200, 200).Children(func() {
			sc := coreScroll(c).Size(100, 200).Children(func() { coreBox(c).Size(100, content) })
			sc.st.scrollY = offset
			coreBox(c).Size(100, 200).Draw(func(p *Painter, r Rect) {
				p.Scrollbar(Scrollbar{Box: r, View: r.H, Content: content, Offset: float32(offset)}, ScrollbarState{Dragging: dragging})
			})
		})
	}, 200, 200)
	same := func(what string) {
		t.Helper()
		img := tt.Image()
		s := img.Bounds().Dx() / 200
		painted := false
		for y := 0; y < 200*s; y++ {
			for x := 80 * s; x < 100*s; x++ {
				a, b := img.RGBAAt(x, y), img.RGBAAt(x+100*s, y)
				if a != b {
					t.Fatalf("%s: at (%d, %d) the container's bar is %v, the Scrollbar's %v", what, x, y, a, b)
				}
				painted = painted || a != img.RGBAAt(10*s, 10*s)
			}
		}
		if !painted {
			t.Fatalf("%s: no thumb painted", what)
		}
	}
	tt.Move(50, 100)
	tt.Frame()
	same("at the top")
	offset = 250
	tt.Frame()
	same("scrolled")
	// The container's thumb held: both widen.
	g := Scrollbar{Box: Rect{0, 0, 100, 200}, View: 200, Content: content, Offset: 250}
	start, n := g.Thumb()
	dragging = true
	tt.Press(95, start+n/2)
	tt.Frame()
	same("dragged")
	tt.Release(95, start+n/2)
	for _, off := range []float32{0, 100, 250, 400} {
		g.Offset = off
		start, _ := g.Thumb()
		if got := g.OffsetAt(start); got < off-0.01 || got > off+0.01 {
			t.Errorf("OffsetAt(Thumb()) at %v = %v", off, got)
		}
	}
	if got := g.OffsetAt(-100); got != 0 {
		t.Errorf("OffsetAt above the track = %v, want 0", got)
	}
	if got := g.OffsetAt(500); got != content-200 {
		t.Errorf("OffsetAt below the track = %v, want %v", got, content-200)
	}
}
