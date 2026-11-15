package ui

import (
	"bytes"
	"fmt"
	"image/color"
	"math"
	"slices"
	"testing"

	"github.com/egoist/mygo/internal/raster"
)

// The accelerated case records the scene a GPU would receive, then draws it
// with the Tester's CPU renderer. It does not exercise a GPU driver.
func polylineDrawing(draw func(*Painter), accelerated bool, scale float32) *Tester {
	tt := NewTester(func(c *Context) {
		Box(c).Fill().Background(RGB(255, 255, 255)).Draw(func(p *Painter, _ Rect) {
			if accelerated {
				host := p.rt.host
				p.rt.host = &windowHost{onGPU: true}
				defer func() { p.rt.host = host }()
			}
			draw(p)
		})
	}, 200, 100)
	tt.SetScale(scale)
	return tt
}

// A caller may reuse its points as soon as StrokePolyline returns. Both
// another call in the same frame and the next frame must keep earlier
// drawings intact, including masks used by the CPU and translucent lines.
func TestStrokePolylineScratchOwnership(t *testing.T) {
	for _, accelerated := range []bool{false, true} {
		for _, alpha := range []float32{1, .4} {
			for _, scale := range []float32{1, 2} {
				t.Run(fmt.Sprintf("gpu=%v/alpha=%g/scale=%g", accelerated, alpha, scale), func(t *testing.T) {
					frame := 0
					scratch := make([][2]float32, 4)
					draw := func(reuse bool) func(*Painter) {
						return func(p *Painter) {
							lines := [][][2]float32{
								{{10, 20}, {30, 55 + float32(frame)*2}, {55, 15}, {80, 45}},
								{{120, 65}, {145, 25}, {180, 80}},
							}
							for i, points := range lines {
								input := points
								if reuse {
									input = scratch[:len(points)]
									copy(input, points)
								}
								p.StrokePolyline(input, 4, RGBA(uint8(20+i*180), 60, 230, alpha))
								if !slices.Equal(input, points) {
									t.Fatal("StrokePolyline changed the caller's points")
								}
								if reuse {
									clear(scratch)
								}
							}
						}
					}
					got := polylineDrawing(draw(true), accelerated, scale)
					want := polylineDrawing(draw(false), accelerated, scale)
					for ; frame < 4; frame++ {
						got.Frame()
						// Repaint the reference whole, with independent point storage.
						want.h.img.Invalidate()
						want.Frame()
						if !bytes.Equal(got.Image().Pix, want.Image().Pix) {
							t.Fatalf("frame %d: reusing caller storage changes the pixels", frame)
						}
					}
				})
			}
		}
	}
}

// A narrow spike and a gap occupy little of the image. Check them locally
// so missing either cannot hide in an average pixel error, then compare
// partial redraws after a point edit with a full render, byte for byte.
func TestStrokePolylineSpikeGapAndEdit(t *testing.T) {
	for _, accelerated := range []bool{false, true} {
		for _, scale := range []float32{1, 2} {
			for _, transformed := range []bool{false, true} {
				t.Run(fmt.Sprintf("gpu=%v/scale=%g/transform=%v", accelerated, scale, transformed), func(t *testing.T) {
					points := make([][2]float32, 181)
					for i := range points {
						points[i] = [2]float32{10 + float32(i), 70}
					}
					for i := 75; i <= 105; i++ {
						points[i] = [2]float32{float32(math.NaN()), 0}
					}
					points[90] = [2]float32{100, 25} // isolated, between two breaks
					points[91] = [2]float32{0, float32(math.Inf(1))}
					points[40][1] = 15
					tt := polylineDrawing(func(p *Painter) {
						draw := func() { p.StrokePolyline(points, 5, RGB(20, 60, 230)) }
						if transformed {
							p.Transform(Scaling(.8, .75).Then(Translation(10, 8)), 0, 0, draw)
						} else {
							draw()
						}
					}, accelerated, scale)
					probe := func(x, y float32, want color.RGBA) {
						t.Helper()
						if transformed {
							x, y = .8*x+10, .75*y+8
						}
						px, py := int(x*scale), int(y*scale)
						if got := tt.Image().RGBAAt(px, py); got != want {
							t.Errorf("at (%d,%d): %v, want %v", px, py, got, want)
						}
					}
					white, blue := color.RGBA{255, 255, 255, 255}, color.RGBA{20, 60, 230, 255}
					for _, tip := range []float32{15, 35, 15} {
						points[40][1] = tip
						tt.Frame()
						probe(50, tip+2, blue)
						probe(100, 70, white) // disconnected runs must not join
						probe(100, 25, white) // an isolated point draws nothing
						if tip == 35 {
							probe(50, 17, white) // the old spike must be erased
						}
						s := &tt.rt.scene
						whole := raster.NewImage(s.Width, s.Height)
						raster.Render(whole, s)
						if !bytes.Equal(tt.Image().Pix, whole.RGBA()) {
							t.Fatalf("tip at %g: partial redraw differs from a full render", tip)
						}
					}
				})
			}
		}
	}
}
