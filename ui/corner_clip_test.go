package ui

import (
	"testing"

	"github.com/egoist/mygo/internal/scene"
)

// TestRoundFrame: a window whose system keeps square corners has its frames cut
// to the corner radius: a rounded clip around everything the frame draws, with
// the background filled inside it, so that the raster renderer and the GPU's
// shader antialias the same arc.
func TestRoundFrame(t *testing.T) {
	s := &scene.Scene{Background: scene.Color{A: 40}}
	if !roundFrame(s, 12, 100, 100) {
		t.Fatal("not rounded")
	}
	if s.Background.A != 0 {
		t.Fatalf("background beyond the corners: %v", s.Background)
	}
	if len(s.Ops) != 2 {
		t.Fatalf("ops: %d", len(s.Ops))
	}
	if s.Ops[0].Kind != scene.OpPushClip || s.Ops[0].Radii != scene.Uniform(12) {
		t.Fatalf("clip: %v %v", s.Ops[0].Kind, s.Ops[0].Radii)
	}
	if s.Ops[1].Kind != scene.OpFill || s.Ops[1].Color.A != 40 {
		t.Fatalf("background fill: %v %v", s.Ops[1].Kind, s.Ops[1].Color)
	}
	if roundFrame(&scene.Scene{}, 60, 100, 100) {
		t.Fatal("rounded a frame too small for the radius")
	}
	if roundFrame(&scene.Scene{}, 0, 100, 100) {
		t.Fatal("rounded a square frame")
	}
}

// TestRoundedFrameCorners: the frames of a window that rounds its own
// corners show nothing beyond the arc, its opaque theme background
// included, and are square again while the radius is 0 (maximized).
func TestRoundedFrameCorners(t *testing.T) {
	radius := 16.0
	tt := coreNewTesterWith(func(c *context) {}, 200, 100, TesterOptions{Dark: true})
	tt.rt.corners = func() float64 { return radius }
	tt.SetScale(2)
	tt.Frame()
	bg := tt.rt.c.theme.Background
	img := tt.Image()
	for _, p := range [][2]int{{0, 0}, {399, 0}, {0, 199}, {399, 199}, {3, 3}} {
		if c := img.RGBAAt(p[0], p[1]); c.A != 0 {
			t.Errorf("corner %v: %v, want clear", p, c)
		}
	}
	if c := img.RGBAAt(200, 100); c.R != bg.R || c.G != bg.G || c.B != bg.B {
		t.Errorf("middle: %v, want %v", c, bg)
	}
	radius = 0
	tt.Frame()
	if c := tt.Image().RGBAAt(0, 0); c.R != bg.R || c.G != bg.G || c.B != bg.B || c.A != 255 {
		t.Errorf("square corner: %v, want %v", c, bg)
	}
}
