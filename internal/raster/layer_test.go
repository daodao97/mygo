package raster

import (
	"bytes"
	"image"
	"testing"

	"github.com/egoist/mygo/internal/scene"
)

// TestLayerDrawsOverWhatItHolds draws the ops of a scene, translucent
// fills and masks, in two parts with Layer.Over, the second over the
// first, as a layer of paths draws an atlas of masks after another: the
// pixels are those of the scene drawn whole, and Over leaves what lies
// outside the area it draws.
func TestLayerDrawsOverWhatItHolds(t *testing.T) {
	a := scene.NewAtlas(1, 64, 64)
	disc := make([]byte, 20*20)
	for y := range 20 {
		for x := range 20 {
			if dx, dy := x-10, y-10; dx*dx+dy*dy < 81 {
				disc[y*20+x] = 200
			}
		}
	}
	x, y, _ := a.AllocTransient(20, 20)
	a.Put(x, y, 20, 20, disc, 20)
	s := &scene.Scene{Width: 80, Height: 60, MaskAtlas: a}
	s.Glyphs = []scene.Glyph{
		{X: 10, Y: 10, W: 20, H: 20, U: uint16(x), V: uint16(y), UW: 20, VH: 20, Color: scene.Color{R: 20, G: 60, B: 230, A: 200}},
		{X: 18, Y: 14, W: 20, H: 20, U: uint16(x), V: uint16(y), UW: 20, VH: 20, Color: scene.Color{R: 230, G: 40, B: 20, A: 160}},
	}
	s.Ops = []scene.Op{
		{Kind: scene.OpFill, Rect: scene.Rect{X: 5, Y: 5, W: 50, H: 30}, Color: scene.Color{R: 40, G: 200, B: 60, A: 120}},
		{Kind: scene.OpGlyphs, Start: 0, End: 1},
		{Kind: scene.OpGlyphs, Start: 1, End: 2},
	}
	whole := NewImage(80, 60)
	Render(whole, s)
	parts := NewImage(80, 60)
	for i := range parts.Pix {
		parts.Pix[i] = 7 // what Over must leave outside its area
	}
	area := image.Rect(0, 0, 60, 40)
	for y := area.Min.Y; y < area.Max.Y; y++ {
		clear(parts.Pix[y*parts.Stride : y*parts.Stride+4*area.Max.X])
	}
	ops := s.Ops
	var l Layer
	for _, part := range [][]scene.Op{ops[:2], ops[2:]} {
		s.Ops = part
		l.Over(parts, s, area)
	}
	for y := range 60 {
		for x := range 80 {
			i := y*parts.Stride + 4*x
			want := whole.Pix[i : i+4]
			if !image.Pt(x, y).In(area) {
				want = []byte{7, 7, 7, 7}
			}
			if got := parts.Pix[i : i+4]; !bytes.Equal(got, want) {
				t.Fatalf("(%d, %d) is %v, not %v", x, y, got, want)
			}
		}
	}
}
