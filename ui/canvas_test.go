package ui

import (
	"bytes"
	"image/color"
	"testing"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/surface"
)

// A canvas of 8×4 pixels, drawn once, shows scaled up 4 times, pixelated:
// red and blue halves, each pixel a block of 4×4.
func TestCanvas(t *testing.T) {
	cv := NewCanvas(8, 4)
	cv.Bitmap().Pixelated()
	draws := 0
	drawn := false
	tt := coreNewTester(func(c *context) {
		coreBox(c).Size(32, 16).Draw(func(p *Painter, r Rect) {
			if !drawn {
				drawn = true
				draws++
				cv.Draw(p, func(cp *Painter) {
					cp.Fill(Rect{W: 4, H: 4}, RGB(255, 0, 0), 0)
					cp.Fill(Rect{X: 4, W: 4, H: 4}, RGB(0, 0, 255), 0)
					// A pixel of green, which pixelated scaling keeps whole.
					cp.Fill(Rect{X: 1, Y: 1, W: 1, H: 1}, RGB(0, 255, 0), 0)
				})
			}
			p.Image(cv.Bitmap(), r, FillBox)
		})
	}, 32, 16)
	img := tt.Image()
	at := func(x, y int) color.RGBA { return img.RGBAAt(x, y) }
	for _, c := range []struct {
		x, y int
		want color.RGBA
	}{
		{2, 2, color.RGBA{255, 0, 0, 255}},
		{6, 6, color.RGBA{0, 255, 0, 255}},
		{9, 9, color.RGBA{255, 0, 0, 255}},
		{20, 8, color.RGBA{0, 0, 255, 255}},
		{31, 15, color.RGBA{0, 0, 255, 255}},
	} {
		if got := at(c.x, c.y); got != c.want {
			t.Errorf("pixel %d,%d = %v, want %v", c.x, c.y, got, c.want)
		}
	}
	// Frames after keep what it holds without drawing it anew.
	tt.Frame()
	if draws != 1 || at(20, 8) != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("after another frame: %d draws, %v", draws, at(20, 8))
	}
}

// A canvas shows another, drawn in the same frame, and text.
func TestNestedCanvas(t *testing.T) {
	inner, outer := NewCanvas(4, 4), NewCanvas(8, 8)
	tt := coreNewTester(func(c *context) {
		coreBox(c).Size(80, 40).Draw(func(p *Painter, r Rect) {
			outer.Draw(p, func(op *Painter) {
				inner.Draw(p, func(ip *Painter) {
					ip.Fill(Rect{W: 4, H: 4}, RGB(0, 200, 0), 0)
				})
				op.Image(inner.Bitmap(), Rect{X: 2, Y: 2, W: 4, H: 4}, FillBox)
			})
			p.Image(outer.Bitmap(), Rect{X: r.X, Y: r.Y, W: 8, H: 8}, FillBox)
			text := NewCanvas(60, 20)
			text.Draw(p, func(tp *Painter) { tp.Text(2, 2, "Hi", 14, RGB(255, 255, 255)) })
			p.Image(text.Bitmap(), Rect{X: r.X + 10, Y: r.Y, W: 60, H: 20}, FillBox)
		})
	}, 80, 40)
	img := tt.Image()
	if got := img.RGBAAt(4, 4); got != (color.RGBA{0, 200, 0, 255}) {
		t.Errorf("inner canvas in outer: %v", got)
	}
	if got := img.RGBAAt(0, 0); got.A != 0 && got != (color.RGBA{255, 255, 255, 255}) {
		// The theme's background shows where the outer canvas is clear.
		t.Logf("outside the inner canvas: %v", got)
	}
	lit := 0
	for y := 0; y < 20; y++ {
		for x := 10; x < 70; x++ {
			if c := img.RGBAAt(x, y); c.R > 200 && c.G > 200 && c.B > 200 {
				lit++
			}
		}
	}
	if lit == 0 {
		t.Error("no text drawn in the canvas")
	}
}

// canvasHost is a window host drawing on a GPU renderer that draws
// nothing, whose view shows a canvas, drawn once with draw, under what
// over paints. A capture of it, drawn in memory, is the first to draw the
// canvas from its scene, as a renderer does that has not its texture: one
// made anew, or one that freed its textures while the window was idle.
func canvasHost(t *testing.T, draw func(cp *Painter), over func(p *Painter, r Rect)) *occludedHost {
	t.Setenv("MYGO_GPU", "")
	newGPU = func(platform.SurfaceNative) (gpuRenderer, error) { return &testGPU{}, nil }
	t.Cleanup(func() { newGPU = newGPURenderer })
	cv := NewCanvas(200, 100)
	drawn := false
	o := &occludedHost{s: &occludedSurface{}}
	o.windowHost = &windowHost{conn: &surface.Conn{Surface: o.s, Post: func(f func()) { f() }}}
	o.rt = newRuntime(func(c *context) {
		o.builds++
		coreBox(c).Size(200, 100).Background(RGB(255, 255, 255)).Draw(func(p *Painter, r Rect) {
			if !drawn {
				drawn = true
				cv.Draw(p, draw)
			}
			p.Image(cv.Bitmap(), r, FillBox)
			if over != nil {
				over(p, r)
			}
		})
	}, o.windowHost)
	return o
}

// canvasShape fills a shape of curves, which is drawn as a mask, bent by n.
func canvasShape(p *Painter, n int, c Color) {
	var path Path
	path.MoveTo(10, 10).CubeTo(60, float32(90+n), 120, -20, 190, 80).LineTo(10, float32(80+n)).Close()
	p.FillPath(&path, c)
}

// TestCanvasKeepsGlyphs checks that a canvas drawn once shows its text and
// its shapes when a renderer first draws it after a frame of another
// window made room in the glyph atlases every window shares
// (System.MakeRoom), which moves or forgets what that frame does not draw:
// the canvas's scene would draw what took their places.
func TestCanvasKeepsGlyphs(t *testing.T) {
	for name, draw := range map[string]func(cp *Painter){
		"text":  func(cp *Painter) { cp.Text(4, 30, "Hello, world", 20, RGB(0, 0, 0)) },
		"shape": func(cp *Painter) { canvasShape(cp, 0, RGB(200, 0, 0)) },
	} {
		t.Run(name, func(t *testing.T) {
			first := canvasHost(t, draw, nil)
			first.frame()
			_, _, want := first.capture()

			b := canvasHost(t, draw, nil)
			b.frame()
			// Another window drawing more glyphs than the atlas has room
			// for, at new sizes each frame (see TestCaptureAfterAtlasMoved).
			size := float32(40)
			a := newDrawingHost(t, func(p *Painter, r Rect) {
				for s := size; s < size+40; s += 4 {
					for c := '!'; c <= '~'; c++ {
						p.Text(r.X, r.Y+40, string(c), s, RGB(0, 0, 0))
					}
				}
			})
			for range 20 {
				a.frame()
				if atlasGenerations(b.last) != b.rt.atlases {
					break
				}
				size += 40
			}
			if atlasGenerations(b.last) == b.rt.atlases {
				t.Fatal("the other window never made room in the atlases")
			}
			if _, _, got := b.capture(); !bytes.Equal(got, want) {
				t.Error("drawn after the atlases moved, the canvas shows something else")
			}
		})
	}
}

// canvasArcs counts the arcs canvasArc drew, each of a width of its own:
// masks are cached by their shape, and one seen in an earlier frame, of
// any test, is no longer drawn for its frame alone.
var canvasArcs int

// canvasArc strokes an arc from (x0, y) to (x1, y) through (cx, cy), a
// little wider than width, drawn as a mask.
func canvasArc(p *Painter, x0, x1, y, cx, cy, width float32, c Color) {
	canvasArcs++
	var path Path
	p.StrokePath(path.MoveTo(x0, y).QuadTo(cx, 2*cy-y, x1, y), width+float32(canvasArcs)/65536, c)
}

// TestCanvasKeepsMasks checks that a canvas drawn once shows its shapes
// when a renderer first draws it some frames later. A mask drawn for the
// first time lasts for its frame alone (System.Mask), and the masks of the
// frames since took its place: the canvas's scene would draw those.
func TestCanvasKeepsMasks(t *testing.T) {
	n := 0
	h := canvasHost(t, func(cp *Painter) { canvasArc(cp, 30, 90, 50, 60, 35, 20, RGB(200, 0, 0)) }, func(p *Painter, r Rect) {
		if n > 0 {
			// A larger arc, another each frame, beside the canvas's.
			canvasArc(p, 110, 190, 80, 150, 30, 30, RGB(0, 0, 200))
		}
	})
	for ; n < 4; n++ {
		uses := h.rt.text.TransientUses()
		h.rt.changed()
		h.frame()
		if h.rt.text.TransientUses() == uses {
			t.Fatalf("frame %d: its arc was not drawn as a mask of its frame alone", n)
		}
	}
	w, _, pix := h.capture()
	white := color.RGBA{255, 255, 255, 255}
	for _, c := range []struct {
		x, y int
		want color.RGBA
	}{
		{60, 35, color.RGBA{200, 0, 0, 255}}, // the middle of the canvas's arc
		{34, 46, color.RGBA{200, 0, 0, 255}}, // near its end
		{60, 54, white},                      // under it, within its mask
		{24, 30, white},
	} {
		i := 4 * (c.y*w + c.x)
		if got := (color.RGBA{pix[i], pix[i+1], pix[i+2], pix[i+3]}); got != c.want {
			t.Errorf("drawn some frames later, pixel %d,%d = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}
