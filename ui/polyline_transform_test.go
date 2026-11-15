package ui

import (
	"fmt"
	"slices"
	"testing"

	"github.com/egoist/mygo/internal/scene"
)

// The polyline's shared style and pixel transform must produce the same
// capsules as individual operations going through Painter.add, including
// a zero-length segment between two differently oriented segments.
func TestStrokePolylineTransformComposition(t *testing.T) {
	points := [][2]float32{{10, 10}, {13, 14}, {13, 14}, {17, 17}}
	for _, scale := range []float32{1, 1.25, 2} {
		for _, m := range []scene.Affine{
			{}, scene.Translation(.25, 3), scene.Scaling(1.7, .6), scene.Rotation(23),
			scene.Projective(1, .1, .2, 1, 2, 3, .001, -.002),
		} {
			t.Run(fmt.Sprintf("scale=%g/transform=%v", scale, m), func(t *testing.T) {
				var got, want scene.Scene
				rt := &engine{host: &windowHost{onGPU: true}}
				p := &Painter{rt: rt, s: &got, scale: scale, opacity: 1, transform: m}
				q := &Painter{rt: rt, s: &want, scale: scale, opacity: 1, transform: m}
				for i, c := range []Color{RGB(20, 60, 230), RGB(230, 40, 20), wideGreen} {
					width := float32(i+1) * 1.5
					p.StrokePolyline(points, width, c)
					w := width * scale
					wide := q.wide(c, Color{}, Color{})
					for _, segment := range []struct {
						length float32
						m      scene.Affine
					}{
						{5 * scale, scene.Matrix(.6, .8, -.8, .6, 10*scale, 10*scale)},
						{0, scene.Translation(13*scale, 14*scale)},
						{5 * scale, scene.Matrix(.8, .6, -.6, .8, 13*scale, 14*scale)},
					} {
						q.add(scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: -w / 2, Y: -w / 2, W: segment.length + w, H: w},
							Radii: scene.Uniform(w / 2), Color: c.scene(), Wide: wide, Opacity: 1, Transform: segment.m})
					}
				}
				if !slices.Equal(got.Ops, want.Ops) || !slices.Equal(got.Wide, want.Wide) {
					t.Fatal("polyline capsules differ from individually transformed fills")
				}
			})
		}
	}
}
