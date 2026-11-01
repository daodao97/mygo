package ui

import (
	"image/color"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func TestScrollbarsFalseRetainsContentInteraction(t *testing.T) {
	for _, tc := range []struct {
		name         string
		container    func(*Context) Element
		x, y         int
		wantX, wantY float32
	}{
		{"vertical", Scroll, 94, 10, 0, 30},
		{"horizontal", ScrollHorizontal, 10, 94, 20, 0},
		{"both", ScrollBoth, 94, 10, 20, 30},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hide, clicks := false, 0
			var offset ScrollState
			tt := NewTester(func(c *Context) {
				sc := tc.container(c).Size(100, 100).TrackScroll(&offset)
				if hide {
					sc.Scrollbars(false)
				}
				sc.Children(func() {
					if Box(c).Size(400, 300).Background(Hex("#ffffff")).Clicked() {
						clicks++
					}
				})
			}, 100, 100)
			tt.Move(float32(tc.x), float32(tc.y))
			white := color.RGBA{255, 255, 255, 255}
			if tt.Image().RGBAAt(tc.x, tc.y) == white {
				t.Fatal("visible thumb was not painted")
			}
			tt.ClickAt(float32(tc.x), float32(tc.y))
			if clicks != 0 {
				t.Fatal("visible thumb did not intercept its click")
			}
			hide = true
			tt.Frame()
			if got := tt.Image().RGBAAt(tc.x, tc.y); got != white {
				t.Fatalf("hidden thumb still painted: %v", got)
			}
			tt.ClickAt(float32(tc.x), float32(tc.y))
			if clicks != 1 {
				t.Fatal("hidden thumb still intercepted content")
			}
			tt.Scroll(50, 50, 20, 30)
			if offset.X != tc.wantX || offset.Y != tc.wantY {
				t.Fatalf("wheel scrolling lost: %+v", offset)
			}
			if tc.wantY > 0 {
				before := offset.Y
				touch(tt, platform.PointerDown, 1, 50, 70)
				touch(tt, platform.PointerMove, 1, 50, 30)
				touch(tt, platform.PointerUp, 1, 50, 30)
				if offset.Y <= before || clicks != 1 {
					t.Fatalf("touch scrolling lost or clicked content: %+v, %d", offset, clicks)
				}
			}
			hide = false
			offset.X, offset.Y = 0, 0
			tt.Frame()
			tt.Move(float32(tc.x), float32(tc.y))
			if tt.Image().RGBAAt(tc.x, tc.y) == white {
				t.Fatal("Scrollbars(true) did not restore thumb")
			}
		})
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
