package raster

import (
	"bytes"
	"github.com/egoist/mygo/internal/scene"
	"testing"
)

func TestTransformedSceneDamageAndNestedClips(t *testing.T) {
	s := &scene.Scene{Width: 150, Height: 100}
	m := scene.Translation(70, 20).Mul(scene.Rotation(90))
	s.Ops = []scene.Op{
		{Kind: scene.OpPushClip, Rect: scene.Rect{W: 60, H: 40}, Transform: m},
		{Kind: scene.OpPushClip, Rect: scene.Rect{X: 5, Y: 5, W: 30, H: 20}, Transform: m, Radii: [4]float32{4, 4, 4, 4}},
		{Kind: scene.OpFill, Rect: scene.Rect{W: 100, H: 100}, Color: scene.Color{R: 255, A: 255}, Transform: m},
		{Kind: scene.OpPopClip}, {Kind: scene.OpPopClip},
	}
	var r Renderer
	defer r.Release()
	r.Render(s)
	p := r.Image.Pix[35*r.Image.Stride+4*55:][:4]
	if p[2] != 255 || p[3] != 255 {
		t.Fatalf("transformed clip interior: %v", p)
	}
	p = r.Image.Pix[30*r.Image.Stride+4*35:][:4]
	if p[3] != 0 {
		t.Fatal("paint escaped rotated clips")
	}
	if damage := r.Render(s); len(damage) != 0 {
		t.Fatalf("unchanged scene damaged %v", damage)
	}
	for i := range s.Ops {
		s.Ops[i].Transform = scene.Translation(20, 0).Mul(s.Ops[i].Transform)
	}
	r.Render(s)
	want := NewImage(s.Width, s.Height)
	Render(want, s)
	if !bytes.Equal(r.Image.Pix, want.Pix) {
		t.Fatal("incremental transformed drawing differs from full drawing")
	}
}

func TestTransformedThinBorderGradientAndImage(t *testing.T) {
	s := &scene.Scene{Width: 100, Height: 100}
	s.Ops = []scene.Op{{Kind: scene.OpFill, Rect: scene.Rect{W: 10, H: 10}, Color: scene.Color{R: 255, A: 255}, Color2: scene.Color{B: 255, A: 255}, Paint: scene.PaintLinear, Gradient: [4]float32{0, 0, 10, 0}, Border: scene.Uniform(1), BorderColor: scene.Color{G: 255, A: 255}, Transform: scene.Translation(20, 20).Mul(scene.Scaling(3, 2))}}
	m := NewImage(100, 100)
	Render(m, s)
	at := func(x, y int) []byte { return m.Pix[y*m.Stride+4*x:][:4] }
	if at(21, 30)[1] != 255 {
		t.Fatalf("scaled border: %v", at(21, 30))
	}
	if p := at(35, 30); p[0] < 100 || p[2] < 100 || p[1] != 0 {
		t.Fatalf("gradient not sampled locally: %v", p)
	}
	img := scene.NewImageRGBA(2, 1, []byte{255, 0, 0, 255, 0, 0, 255, 255})
	s.Ops = []scene.Op{{Kind: scene.OpImage, Rect: scene.Rect{W: 20, H: 10}, Src: scene.Rect{W: 2, H: 1}, Image: img, Transform: scene.Translation(50, 20).Mul(scene.Rotation(90))}}
	Render(m, s)
	if at(45, 21)[2] < 240 || at(45, 38)[0] < 240 {
		t.Fatalf("rotated image colors: %v %v", at(45, 21), at(45, 38))
	}
}

// TestTransformedImageDrawsReleasedPixels checks that an image whose owner
// dropped its pixels (Bitmap.ReleasePixels) still draws with a transform:
// the op reads Pix, which the renderer must make again, as it does for an
// image drawn without one.
func TestTransformedImageDrawsReleasedPixels(t *testing.T) {
	s := &scene.Scene{Width: 100, Height: 100}
	img := scene.NewImageRGBA(2, 1, []byte{255, 0, 0, 255, 0, 0, 255, 255})
	img.Restore = func() []byte { return []byte{255, 0, 0, 255, 0, 0, 255, 255} }
	img.Dropped = func() { img.Pix = nil }
	img.Dropped()
	s.Ops = []scene.Op{{Kind: scene.OpImage, Rect: scene.Rect{W: 20, H: 10}, Src: scene.Rect{W: 2, H: 1}, Image: img, Transform: scene.Translation(50, 20).Mul(scene.Rotation(90))}}
	m := NewImage(100, 100)
	Render(m, s)
	at := func(x, y int) []byte { return m.Pix[y*m.Stride+4*x:][:4] }
	if at(45, 21)[2] < 240 || at(45, 38)[0] < 240 {
		t.Fatalf("rotated image colors: %v %v", at(45, 21), at(45, 38))
	}
}

// TestTransformedFastPathsDrawTheGeneralPixels checks that what the
// transformed path works out once per operation (the interior a shape
// covers whole, an image's footprint, a shadow's rows and the box casting
// it) draws the pixels of the general per-pixel path, byte for byte, under
// the maps transitions make: subpixel moves, scales, flips, rotations and
// perspective, through clips.
func TestTransformedFastPathsDrawTheGeneralPixels(t *testing.T) {
	pix := make([]byte, 4*37*23)
	for i := range pix {
		pix[i] = byte(i * 7919 >> 3)
	}
	for i := 3; i < len(pix); i += 4 {
		pix[i] = max(pix[i], pix[i-1], pix[i-2], pix[i-3]) // premultiplied
	}
	img := scene.NewImageRGBA(37, 23, pix)
	white, ink := scene.Color{R: 255, G: 255, B: 255, A: 255}, scene.Color{R: 15, G: 23, B: 42, A: 90}
	box := scene.Rect{X: 30, Y: 24, W: 120, H: 84}
	ops := map[string]scene.Op{
		"image":         {Kind: scene.OpImage, Rect: box, Src: scene.Rect{W: 37, H: 23}, Image: img, Opacity: 0.45},
		"image 1:1":     {Kind: scene.OpImage, Rect: scene.Rect{X: 30, Y: 24, W: 37, H: 23}, Src: scene.Rect{W: 37, H: 23}, Image: img},
		"image rounded": {Kind: scene.OpImage, Rect: box, Src: scene.Rect{X: 2, Y: 1, W: 30, H: 20}, Image: img, Radii: scene.Uniform(14)},
		"image cont":    {Kind: scene.OpImage, Rect: box, Src: scene.Rect{W: 37, H: 23}, Image: img, Radii: scene.Uniform(14), Continuous: true},
		"fill border":   {Kind: scene.OpFill, Rect: box, Color: white, Radii: scene.Uniform(14), Border: scene.Uniform(1.5), BorderColor: ink},
		"fill cont":     {Kind: scene.OpFill, Rect: box, Color: white, Radii: scene.Uniform(20), Continuous: true},
		"shadow":        {Kind: scene.OpShadow, Rect: scene.Rect{X: 30, Y: 34, W: 120, H: 84}, Cast: box, Radii: scene.Uniform(14), CastRadii: scene.Uniform(14), Blur: 28, Color: ink},
		"shadow cont":   {Kind: scene.OpShadow, Rect: box, Cast: box, Radii: scene.Uniform(14), CastRadii: scene.Uniform(14), Continuous: true, Blur: 9, Color: ink},
		"shadow sharp":  {Kind: scene.OpShadow, Rect: scene.Rect{X: 28, Y: 22, W: 120, H: 84}, Cast: box, Blur: 0.5, Color: ink},
		"shadow uncast": {Kind: scene.OpShadow, Rect: box, Radii: scene.Uniform(8), Blur: 12, Color: ink},
		"shadow inset":  {Kind: scene.OpShadow, Inset: true, Rect: scene.Rect{X: 36, Y: 30, W: 108, H: 72}, Cast: box, Radii: scene.Uniform(10), CastRadii: scene.Uniform(14), Blur: 16, Color: ink},
	}
	maps := map[string]scene.Affine{
		"subpixel":    scene.Translation(0.37, -0.26),
		"far":         scene.Translation(1500.62, 900.41), // of geometry far away
		"scale":       scene.Translation(90, 66).Mul(scene.Scaling(0.8, 0.8)).Mul(scene.Translation(-90, -66)),
		"scale 2x":    scene.Translation(-40.3, -20.2).Mul(scene.Scaling(2, 2)),
		"squash flip": scene.Translation(190, 10).Mul(scene.Scaling(-1.3, 0.7)),
		"rotate":      scene.Translation(90, 66).Mul(scene.Rotation(17)).Mul(scene.Translation(-90, -66)),
		"perspective": scene.Projective(1, 0.05, 0.02, 1, 3, 2, 0.0009, 0.0004),
	}
	clips := map[string][]scene.Op{
		"none":    nil,
		"rect":    {{Kind: scene.OpPushClip, Rect: scene.Rect{X: 20.5, Y: 10.25, W: 150, H: 100}}},
		"rounded": {{Kind: scene.OpPushClip, Rect: scene.Rect{X: 25, Y: 18, W: 140, H: 110}, Radii: scene.Uniform(30)}},
	}
	render := func(general bool, s *scene.Scene) []byte {
		generalTransforms = general
		defer func() { generalTransforms = false }()
		m := NewImage(s.Width, s.Height)
		Render(m, s)
		return m.Pix
	}
	for on, op := range ops {
		for mn, m := range maps {
			for cn, clip := range clips {
				for _, scale := range []float32{1, 2} {
					o := op
					o.Transform = scene.Scaling(scale, scale).Mul(m)
					if mn == "far" {
						o.Rect.X, o.Rect.Y = o.Rect.X-1500, o.Rect.Y-900
						o.Cast.X, o.Cast.Y = o.Cast.X-1500, o.Cast.Y-900
					}
					s := &scene.Scene{Width: int(200 * scale), Height: int(150 * scale), Clear: scene.Color{R: 246, G: 247, B: 249, A: 255}}
					for _, c := range clip {
						c.Transform = scene.Scaling(scale, scale)
						s.Ops = append(s.Ops, c)
					}
					s.Ops = append(s.Ops, o)
					for range clip {
						s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPopClip})
					}
					want, got := render(true, s), render(false, s)
					if !bytes.Equal(got, want) {
						t.Errorf("%s, %s, clip %s, %gx: pixels differ from the general path", on, mn, cn, scale)
					}
					if bytes.Equal(got, render(true, &scene.Scene{Width: s.Width, Height: s.Height, Clear: s.Clear})) {
						t.Errorf("%s, %s, clip %s, %gx: drew nothing", on, mn, cn, scale)
					}
				}
			}
		}
	}
}

// BenchmarkTransformed measures the transformed path on what transitions
// make of a window: a translucent layer moved by a fraction of a pixel,
// and a shadowed popover scaling in.
func BenchmarkTransformed(b *testing.B) {
	const w, h = 2240, 1440
	pix := make([]byte, 4*w*h)
	for i := range pix {
		pix[i] = byte(i)
	}
	img := scene.NewImageRGBA(w, h, pix)
	ink := scene.Color{R: 15, G: 23, B: 42, A: 36}
	for _, bc := range []struct {
		name string
		op   scene.Op
	}{
		{"layer subpixel", scene.Op{Kind: scene.OpImage, Rect: scene.Rect{W: w, H: h}, Src: scene.Rect{W: w, H: h}, Image: img, Opacity: 0.45, Transform: scene.Translation(0.37, 0)}},
		{"shadow scaled", scene.Op{Kind: scene.OpShadow, Rect: scene.Rect{X: 200, Y: 100, W: 480, H: 420}, Cast: scene.Rect{X: 200, Y: 80, W: 480, H: 420},
			Radii: scene.Uniform(28), CastRadii: scene.Uniform(28), Blur: 56, Color: ink,
			Transform: scene.Translation(440, 290).Mul(scene.Scaling(0.85, 0.85)).Mul(scene.Translation(-440, -290))}},
	} {
		b.Run(bc.name, func(b *testing.B) {
			s := &scene.Scene{Width: w, Height: h, Ops: []scene.Op{bc.op}}
			m := NewImage(w, h)
			for b.Loop() {
				Render(m, s)
			}
		})
	}
}
