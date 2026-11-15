package ui

import (
	"fmt"
	"math"
	"testing"

	"github.com/egoist/mygo/internal/scene"
)

// BenchmarkStrokePolylinePaint records changing opaque polylines in a GPU
// scene. It measures only the CPU Painter work, including its transform,
// with small lines as controls; it does not build GPU instances or draw.
func BenchmarkStrokePolylinePaint(b *testing.B) {
	for _, n := range []int{1, 8, 6000} {
		for _, tc := range []struct {
			name string
			m    Transform
		}{
			{"plain", Transform{}},
			{"affine", Scaling(1.1, .8).Then(Rotation(5))},
			{"perspective", RotationY(12).Then(Perspective(1800))},
		} {
			b.Run(fmt.Sprintf("%d/%s", n, tc.name), func(b *testing.B) {
				points := make([][2]float32, n+1)
				for i := range points {
					x := float32(i) / float32(n)
					points[i] = [2]float32{30 + 1220*x, 360 + 240*float32(math.Sin(float64(x)*18))}
				}
				var s scene.Scene
				p := &Painter{rt: &engine{host: &windowHost{onGPU: true}}, s: &s, scale: 1, opacity: 1}
				y := points[n/2][1]
				paint := func(i int) {
					s.Reset(1280, 720, scene.Color{})
					points[n/2][1] = y + float32(i%2)
					p.Transform(tc.m, 640, 360, func() { p.StrokePolyline(points, 1.5, RGB(20, 100, 220)) })
				}
				for i := range 4 {
					paint(i)
				}
				b.ReportAllocs()
				i := 4
				for b.Loop() {
					paint(i)
					i++
				}
			})
		}
	}
}
