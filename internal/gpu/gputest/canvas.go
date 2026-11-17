package gputest

import (
	"math"

	"github.com/egoist/mygo/internal/scene"
)

// CanvasScene returns a 120×80 scene showing canvases (scene.NewCanvas): a
// 16×12 one drawn with fills, a rounded one, an image and a canvas of its
// own, shown 4 times as large pixelated, then smooth and flipped; and a
// canvas shown only within another, with nothing new to draw the second
// time the renderer draws the scene; and a canvas drawing a conic gradient,
// shown blurred, as a view blurred is (ui.Element.Blur).
func CanvasScene() *scene.Scene {
	s := &scene.Scene{Width: 120, Height: 80, Clear: scene.Color{R: 246, G: 247, B: 249, A: 255}}
	pix := make([]byte, 4*4*4)
	for i := range 16 {
		pix[4*i], pix[4*i+1], pix[4*i+2], pix[4*i+3] = byte(i*16), byte(255-i*16), 120, 255
	}
	img := scene.NewImageRGBA(4, 4, pix)

	inner := scene.NewCanvas(4, 4)
	inner.Scene.Ops = []scene.Op{{Kind: scene.OpFill, Rect: scene.Rect{W: 4, H: 4}, Color: scene.Color{R: 250, G: 204, B: 21, A: 255}},
		{Kind: scene.OpFill, Rect: scene.Rect{X: 1, Y: 1, W: 2, H: 2}, Color: scene.Color{R: 20, G: 24, B: 32, A: 255}}}

	c := scene.NewCanvas(16, 12)
	c.Scene.Ops = []scene.Op{
		{Kind: scene.OpFill, Rect: scene.Rect{W: 16, H: 12}, Color: scene.Color{R: 37, G: 99, B: 235, A: 255}},
		{Kind: scene.OpFill, Rect: scene.Rect{X: 1, Y: 1, W: 6, H: 6}, Radii: [4]float32{3, 3, 3, 3}, Color: scene.Color{R: 220, G: 40, B: 40, A: 200}},
		{Kind: scene.OpImage, Rect: scene.Rect{X: 9, Y: 1, W: 4, H: 4}, Image: img, Src: scene.Rect{W: 4, H: 4}},
		{Kind: scene.OpImage, Rect: scene.Rect{X: 2, Y: 7, W: 4, H: 4}, Image: inner, Src: scene.Rect{W: 4, H: 4}},
		{Kind: scene.OpFill, Rect: scene.Rect{X: 8, Y: 8, W: 8, H: 4}, Color: scene.Color{R: 255, G: 255, B: 255, A: 128}},
	}
	soft := scene.NewCanvas(40, 34)
	soft.Scene.Stops = []scene.Stop{{Color: scene.Color{R: 220, G: 40, B: 40, A: 255}, At: 0}, {Color: scene.Color{R: 37, G: 99, B: 235, A: 255}, At: 0.6},
		{Color: scene.Color{R: 220, G: 40, B: 40, A: 255}, At: 1}}
	soft.Scene.Ops = []scene.Op{
		{Kind: scene.OpFill, Rect: scene.Rect{X: 6, Y: 6, W: 28, H: 22}, Radii: [4]float32{6, 6, 6, 6}, Paint: scene.PaintConic,
			Gradient: [4]float32{20, 17, 0.3, 0}, Stops: [2]int32{0, 3}},
		{Kind: scene.OpFill, Rect: scene.Rect{X: 14, Y: 2, W: 4, H: 30}, Color: scene.Color{R: 20, G: 24, B: 32, A: 160}},
	}
	s.Canvases = []*scene.Image{c, soft}
	s.Ops = []scene.Op{
		{Kind: scene.OpImage, Rect: scene.Rect{X: 4, Y: 4, W: 64, H: 48}, Image: c, Src: scene.Rect{W: 16, H: 12}, Pixelated: true},
		{Kind: scene.OpImage, Rect: scene.Rect{X: 72, Y: 4, W: 44, H: 33}, Image: c, Src: scene.Rect{X: 16, W: -16, H: 12}},
		{Kind: scene.OpImage, Rect: scene.Rect{X: 74, Y: 42, W: 40, H: 34}, Image: soft, Src: scene.Rect{W: 40, H: 34}, Blur: 2.5},
		{Kind: scene.OpImage, Rect: scene.Rect{X: 4, Y: 56, W: 40, H: 20}, Image: soft, Src: scene.Rect{X: 0, Y: 34, W: 40, H: -34}, Blur: 5},
	}
	return s
}

// OwnedCanvasScene returns a 96×32 scene showing a canvas that keeps the
// bitmaps of its glyphs in atlases of its own (scene.Scene.Own): a disc of
// the mask atlas and, when color, a glyph of the color atlas, painted with
// the frame's atlases, which hold other bitmaps in their places since, as
// after a frame made room in them. The frame draws those next to the
// canvas. Without color the canvas has a mask atlas of its own alone, and
// the frame's color atlas.
func OwnedCanvasScene(color bool) *scene.Scene {
	s := &scene.Scene{Width: 96, Height: 32, Clear: scene.Color{R: 246, G: 247, B: 249, A: 255}}
	mask, colors := scene.NewAtlas(1, 64, 64), scene.NewAtlas(4, 64, 32)
	s.MaskAtlas, s.ColorAtlas = mask, colors
	disc, bars := make([]byte, 16*16), make([]byte, 16*16)
	for y := range 16 {
		for x := range 16 {
			dx, dy := float64(x)-7.5, float64(y)-7.5
			disc[y*16+x] = byte(255 * clamp(7.5-math.Sqrt(dx*dx+dy*dy)))
			if x%4 < 2 {
				bars[y*16+x] = 255
			}
		}
	}
	block := func(r, g, b byte) []byte {
		pix := make([]byte, 10*10*4)
		for i := 0; i < len(pix); i += 4 {
			pix[i], pix[i+1], pix[i+2], pix[i+3] = r, g, b, 255
		}
		return pix
	}
	put := func(maskPix, colorPix []byte) (m, c [2]uint16) {
		x, y, _ := mask.Alloc(16, 16)
		mask.Put(x, y, 16, 16, maskPix, 16)
		m = [2]uint16{uint16(x), uint16(y)}
		x, y, _ = colors.Alloc(10, 10)
		colors.Put(x, y, 10, 10, colorPix, 40)
		return m, [2]uint16{uint16(x), uint16(y)}
	}
	ink := scene.Color{R: 20, G: 24, B: 32, A: 255}
	glyphs := func(m, c [2]uint16, x float32) []scene.Glyph {
		return []scene.Glyph{{X: x, Y: 4, W: 16, H: 16, U: m[0], V: m[1], UW: 16, VH: 16, Color: ink},
			{X: x + 20, Y: 7, W: 10, H: 10, U: c[0], V: c[1], UW: 10, VH: 10, Color: scene.Color{A: 255}, Colored: true}}
	}

	c := scene.NewCanvas(40, 24)
	cs := c.Scene
	cs.MaskAtlas, cs.ColorAtlas = mask, colors
	m, k := put(disc, block(240, 160, 20))
	cs.Glyphs = glyphs(m, k, 4)
	if !color {
		cs.Glyphs = cs.Glyphs[:1]
	}
	cs.Ops = []scene.Op{{Kind: scene.OpFill, Rect: scene.Rect{W: 40, H: 24}, Color: scene.Color{R: 255, G: 255, B: 255, A: 255}},
		{Kind: scene.OpGlyphs, Start: 0, End: int32(len(cs.Glyphs))}}
	if !cs.Own(&scene.Owned{}) {
		panic("gputest: the canvas's glyphs fit no atlas")
	}
	mask.Reset()
	colors.Reset()
	m, k = put(bars, block(37, 99, 235))
	s.Glyphs = glyphs(m, k, 52)
	s.Canvases = []*scene.Image{c}
	s.Ops = []scene.Op{
		{Kind: scene.OpImage, Rect: scene.Rect{X: 4, Y: 4, W: 40, H: 24}, Image: c, Src: scene.Rect{W: 40, H: 24}},
		{Kind: scene.OpGlyphs, Start: 0, End: 2},
	}
	return s
}

// OwnedCanvasFrames is how many frames a test draws an OwnedCanvasScene
// for, readying it for each after the first with OwnedCanvasNext.
const OwnedCanvasFrames = 3

// OwnedCanvasNext readies an OwnedCanvasScene for the frame after frame,
// counted from 0: the second only shows the canvas, and the third draws
// it anew on another ground, from its own atlases again.
func OwnedCanvasNext(s *scene.Scene, frame int) {
	c := s.Ops[0].Image
	switch frame {
	case 0:
		s.Canvases = nil
	case 1:
		c.Scene.Ops[0].Color = scene.Color{R: 254, G: 240, B: 138, A: 255}
		c.Changed()
		s.Canvases = []*scene.Image{c}
	}
}
