package ui

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/scene"
)

// spillWires returns the i-th of the wires TestEveryPathDrawsWhenTheAtlasIsFull
// strokes in frame, a cubic curve 2300 pixels wide and 320 tall whose
// shape changes every frame, as a link of a node editor under a zoom
// that changes, and a function returning its point t of the way along.
func spillWires(i, frame int) (p Path, at func(t float32) (float32, float32)) {
	x0, y0 := 10+float32(frame)*0.37, float32(10+i*4)
	x1, y1 := float32(2310)-float32(frame)*0.21, y0+320
	// Each wire's own shape, whose mask no other shares.
	k := float32(i) * 0.37
	pts := [4][2]float32{{x0, y0}, {1160 - k, y0}, {1160 + k, y1}, {x1, y1}}
	p.MoveTo(pts[0][0], pts[0][1]).CubeTo(pts[1][0], pts[1][1], pts[2][0], pts[2][1], pts[3][0], pts[3][1])
	return p, func(t float32) (float32, float32) {
		u := 1 - t
		w := [4]float32{u * u * u, 3 * u * u * t, 3 * u * t * t, t * t * t}
		var x, y float32
		for j, pt := range pts {
			x, y = x+w[j]*pt[0], y+w[j]*pt[1]
		}
		return x, y
	}
}

// TestEveryPathDrawsWhenTheAtlasIsFull strokes 160 translucent wires that
// change shape every frame, 4 pixels apart, each mask 2300×320 pixels:
// together 118 million pixels of masks, and tiles of them, against the
// 16.8 million of the largest atlas, as in #1841 (2000 links of a node
// editor under a changing zoom). Paths that find no room in the atlas,
// even after the runtime made room and painted the frame again, draw on
// the CPU into a layer of the frame (pathSpill): in every frame, every
// wire shows all along.
func TestEveryPathDrawsWhenTheAtlasIsFull(t *testing.T) {
	const n = 160
	frame := 0
	tt := coreNewTester(func(c *context) {
		coreBox(c).Fill().Background(RGB(255, 255, 255)).Draw(func(p *Painter, r Rect) {
			for i := range n {
				path, _ := spillWires(i, frame)
				p.StrokePath(&path, 1.5, RGBA(20, 60, 230, 0.8))
			}
			// Drawn after the wires, over them: what a layer of them
			// draws stays under it.
			p.Fill(Rect{X: 1150, Y: 0, W: 20, H: 1000}, RGB(230, 40, 20), 0)
		})
	}, 2320, 980)
	missing := func(frame int) int {
		img := tt.Image()
		lost := 0
		for i := range n {
			_, at := spillWires(i, frame)
			for _, along := range []float32{0.05, 0.25, 0.4, 0.6, 0.75, 0.95} {
				x, y := at(along)
				found := false
				for dy := -1; dy <= 1 && !found; dy++ {
					for dx := -1; dx <= 1 && !found; dx++ {
						c := img.RGBAAt(int(x)+dx, int(y)+dy)
						found = c.B > 150 && c.R < 150
					}
				}
				if !found {
					if lost < 5 {
						t.Errorf("frame %d: wire %d is missing %v of the way along, at (%.0f, %.0f)", frame, i, along, x, y)
					}
					lost++
				}
			}
			// The fill drawn after the wires covers them.
			if x, y := at(0.5); x > 1151 && x < 1169 {
				if c := img.RGBAAt(int(x), int(y)); c.R < 200 || c.B > 80 {
					t.Errorf("frame %d: wire %d shows %v over the fill drawn after it", frame, i, c)
				}
			}
		}
		return lost
	}
	for frame = 1; frame <= 4; frame++ {
		tt.Frame()
		if lost := missing(frame); lost > 0 {
			t.Errorf("frame %d: %d of %d points along the wires are missing", frame, lost, 6*n)
		}
		// The wires lie apart: none shows darker than one coat of its
		// color, which a wire drawn twice would.
		img := tt.Image()
		for y := 0; y < img.Rect.Dy(); y++ {
			for x := 0; x < img.Rect.Dx(); x++ {
				if c := img.RGBAAt(x, y); c.R < 60 {
					t.Fatalf("frame %d: (%d, %d) is %v, darker than one coat of the wires' blue", frame, x, y, c)
				}
			}
		}
	}
	// It is not one frame painted again for room (System.MakeRoom) but
	// one paint, which draws the masks it has no room for on the CPU.
	paints := tt.rt.paints
	tt.Frame()
	if n := tt.rt.paints - paints; n != 1 {
		t.Errorf("a frame took %d paints", n)
	}
	// Logged once, and listed by the inspector.
	if w := spillWarnings(tt, "drawing paths on the CPU"); w != 1 {
		t.Errorf("drawing paths on the CPU was told %d times", w)
	}
}

// TestSpilledPathsMatchTheAtlas draws paths on the CPU, into layers of the
// frame (spillEveryPath), and again from the atlas: fills by both rules,
// strokes with caps, joins and dashes, translucent ones over others, in
// linear and conic gradients, turned by a transform and cut by a rounded
// clip, with a fill drawn between them. The pixels are the same, within
// two levels of 255 where a layer rounds what it holds to 8 bits.
func TestSpilledPathsMatchTheAtlas(t *testing.T) {
	blue, red := RGB(20, 60, 230), RGB(230, 40, 20)
	view := func(c *context) {
		coreBox(c).Fill().Background(RGB(255, 255, 255)).Draw(func(p *Painter, r Rect) {
			var star, ring, wave, spike, disc Path
			star.MoveTo(60, 8).LineTo(92, 110).LineTo(8, 46).LineTo(112, 46).LineTo(28, 110).Close()
			p.FillPath(&star, blue)
			ring.Circle(180, 60, 50).Circle(180, 60, 25).FillRule(EvenOdd)
			p.FillPath(&ring, RGBA(230, 40, 20, 0.6))
			wave.MoveTo(140, 100).CubeTo(270, -40, 300, 200, 330, 20).QuadTo(380, 140, 250, 30)
			p.StrokePath(&wave, 7.5, RGBA(20, 60, 230, 0.5))
			p.StrokePathGradient(&wave, 2, LinearGradient{Angle: 90, From: blue, To: red})
			// A fill between paths: those after it draw over it.
			p.Fill(Rect{X: 200, Y: 120, W: 60, H: 60}, RGBA(40, 200, 60, 0.7), 6)
			disc.Circle(240, 170, 35)
			p.FillPathConicGradient(&disc, ConicGradient{Stops: []GradientStop{{red, 0}, {blue, 0.5}, {red, 1}}})
			spike.MoveTo(140, 200).LineTo(170, 150).LineTo(176, 210).LineTo(200, 160)
			p.StrokePath(spike.Cap(SquareCap).Join(MiterJoin).MiterLimit(8).Dash(2, 40, 6), 5, blue)
			p.Transform(Rotation(17), 60, 170, func() {
				var box Path
				box.MoveTo(20, 140).LineTo(100, 140).LineTo(100, 200).LineTo(20, 200).Close()
				p.StrokePath(&box, 3, RGBA(230, 40, 20, 0.8))
			})
			p.Clip(Rect{X: 290, Y: 100, W: 80, H: 110}, 20, func() {
				var x Path
				x.MoveTo(280, 90).LineTo(380, 220).MoveTo(380, 90).LineTo(280, 220)
				p.StrokePath(&x, 9, RGBA(20, 60, 230, 0.7))
			})
		})
	}
	render := func(spill bool) *image.RGBA {
		defer func(was bool) { spillEveryPath = was }(spillEveryPath)
		spillEveryPath = spill
		tt := coreNewTester(view, 380, 220)
		tt.SetScale(2)
		layers, masks := 0, 0
		for _, op := range tt.rt.scene.Ops {
			switch {
			case op.Kind == scene.OpImage && !op.Rect.Empty():
				layers++
			case op.Kind == scene.OpGlyphs:
				masks++
			}
		}
		// The fill and the clip end a layer.
		if spill && (layers != 3 || masks != 0) {
			t.Errorf("spilled, the paths draw in %d layers and %d ops of masks, not 3 layers", layers, masks)
		}
		return tt.Image()
	}
	atlas, spilled := render(false), render(true)
	worst, at := 0, 0
	for i := range atlas.Pix {
		d := int(atlas.Pix[i]) - int(spilled.Pix[i])
		if d = max(d, -d); d > worst {
			worst, at = d, i/4
		}
	}
	if worst > 2 {
		w := atlas.Rect.Dx()
		t.Errorf("spilled paths differ by up to %d levels from paths drawn from the atlas, at (%d, %d): %v and %v",
			worst, at%w, at/w, atlas.RGBAAt(at%w, at/w), spilled.RGBAAt(at%w, at/w))
	}
}

// TestSpilledPathsRepaintInParts strokes the wires of
// TestEveryPathDrawsWhenTheAtlasIsFull in two drawings that move every
// frame, which frames paint again by themselves (repaintParts), with a
// bar between them: each drawing's wires spill into layers of its own,
// so that the first's show under the bar and the second's over it, in
// every frame, painted in parts.
func TestSpilledPathsRepaintInParts(t *testing.T) {
	const n = 160
	start := time.Unix(1000, 0)
	frameAt := func(now time.Time) int { return int(now.Sub(start) / (8 * time.Millisecond)) }
	wires := func(from, to int) func(p *Painter, r Rect) {
		return func(p *Painter, r Rect) {
			p.AnimationFrame()
			frame := frameAt(p.Now())
			for i := from; i < to; i++ {
				path, _ := spillWires(i, frame)
				p.StrokePath(&path, 1.5, RGBA(20, 60, 230, 0.8))
			}
		}
	}
	tt := &repaintTester{clock: start}
	tt.Tester = coreNewTester(func(c *context) {
		tt.builds++
		coreBox(c).Fill().Background(RGB(255, 255, 255)).Children(func() {
			coreBox(c).Absolute().Left(0).Top(0).Size(2320, 980).Draw(wires(0, n/2))
			coreBox(c).Absolute().Left(1150).Top(0).Size(20, 980).Background(RGB(230, 40, 20))
			coreBox(c).Absolute().Left(0).Top(0).Size(2320, 980).Draw(wires(n/2, n))
		})
	}, 2320, 980)
	tt.rt.clock = func() time.Time { return tt.clock }
	tt.Frame()
	parts := tt.rt.partPaints
	for range 6 {
		if built, _ := tt.frame(8 * time.Millisecond); built {
			t.Fatal("a frame built the view")
		}
		frame := frameAt(tt.clock)
		img := tt.Image()
		for i := range n {
			_, at := spillWires(i, frame)
			for _, along := range []float32{0.1, 0.3, 0.7, 0.9} {
				x, y := at(along)
				found := false
				for dy := -1; dy <= 1 && !found; dy++ {
					for dx := -1; dx <= 1 && !found; dx++ {
						c := img.RGBAAt(int(x)+dx, int(y)+dy)
						found = c.B > 150 && c.R < 150
					}
				}
				if !found {
					t.Fatalf("frame %d: wire %d is missing %v of the way along, at (%.0f, %.0f)", frame, i, along, x, y)
				}
			}
			x, y := at(0.5)
			under := i < n/2
			found := false
			for dy := -1; dy <= 1 && !found; dy++ {
				c := img.RGBAAt(int(x), int(y)+dy)
				found = !under && c.B > 150 && c.R < 120
			}
			if c := img.RGBAAt(int(x), int(y)); under && (c.R < 220 || c.B > 30) || !under && !found {
				t.Fatalf("frame %d: wire %d shows %v where it crosses the bar, drawn under it: %v", frame, i, c, under)
			}
		}
	}
	if tt.rt.partPaints == parts {
		t.Error("no frame painted the drawings in parts")
	}
}

// spillWarnings counts the warnings of tt that say what.
func spillWarnings(tt *Tester, what string) int {
	n := 0
	for _, w := range tt.rt.warnings {
		if strings.Contains(w, what) {
			n++
		}
	}
	return n
}

// TestPathsNotDrawnAreLogged strokes a line too long to place its points
// in float32 (pathSpanMax), frame after frame: that it is not drawn is
// logged, once.
func TestPathsNotDrawnAreLogged(t *testing.T) {
	tt := coreNewTester(func(c *context) {
		coreBox(c).Fill().Draw(func(p *Painter, r Rect) {
			var line Path
			line.MoveTo(0, 10).LineTo(1e7, 10)
			p.StrokePath(&line, 2, RGB(20, 60, 230))
			p.StrokePath(&line, 3, RGB(20, 60, 230))
		})
	}, 200, 100)
	tt.Frame()
	tt.Frame()
	if w := spillWarnings(tt, "a path was not drawn"); w != 1 {
		t.Errorf("a path not drawn was told %d times", w)
	}
}
