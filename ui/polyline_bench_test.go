package ui

import (
	"math"
	"testing"

	"github.com/egoist/mygo/internal/gpu"
	"github.com/egoist/mygo/internal/scene"
)

var polylineWorkloads = []string{"edit", "pan", "cover50", "cover90", "perspective"}

const polylineSegments = 6000
const polylineWidth, polylineHeight = 1280, 720

// Each workload starts with the same synthetic curve and owns its source,
// scratch and scene. Every paint resets the scene and calls the production
// Painter; there is no prebuilt display list or reduced set of samples.
func newPolylineWorkload(mode string) (*scene.Scene, func(int)) {
	s := &scene.Scene{}
	rt := &engine{host: &windowHost{onGPU: true}}
	p := &Painter{rt: rt, s: s, scale: 1, opacity: 1}
	source := make([][2]float32, polylineSegments+1)
	scratch := make([][2]float32, len(source))
	for i := range source {
		x := float32(i) / polylineSegments
		source[i] = [2]float32{30 + 1220*x, 360 + 240*float32(math.Sin(float64(x)*18))}
	}
	return s, func(frame int) {
		s.Reset(polylineWidth, polylineHeight, scene.Color{R: 245, G: 245, B: 245, A: 255})
		copy(scratch, source)
		step := float32(frame % 2)
		transform := Transform{}
		switch mode {
		case "edit":
			scratch[polylineSegments/2][1] += 7 * step
		case "perspective":
			transform = RotationY(12 + step).Then(Perspective(1800))
		default:
			// The covered cases pan exactly as the uncovered control does.
			transform = Translation(step, 0)
		}
		p.Transform(transform, polylineWidth/2, polylineHeight/2, func() {
			p.StrokePolyline(scratch, 1.5, RGB(20, 100, 220))
		})
		clear(scratch)
		cover := float32(0)
		switch mode {
		case "cover50":
			cover = .5
		case "cover90":
			cover = .9
		}
		if cover > 0 {
			p.Fill(Rect{W: polylineWidth * cover, H: polylineHeight}, RGB(230, 230, 230), 0)
		}
	}
}

// BenchmarkStrokePolylineScene times CPU painting, scene differencing and
// GPU instance construction for 6,000 segments at 1280×720, scale 1. It
// excludes layout, GPU submission and presentation. Covered cases put an
// opaque panel over 50% or 90% of the viewport; work counts describe what
// remains. Each iteration changes the input, including the covered cases.
func BenchmarkStrokePolylineScene(b *testing.B) {
	for _, mode := range polylineWorkloads {
		b.Run(mode, func(b *testing.B) {
			s, paint := newPolylineWorkload(mode)
			builder := gpu.Builder{Classes: true, ClipSkip: true}
			defer builder.Release()
			d := scene.Differ{Reach: 2}
			defer d.Release()
			var area, culled, instances int64
			frame := func(i int) {
				paint(i)
				damage, known := d.Diff(s)
				if known {
					for _, r := range damage {
						area += int64(r.Dx() * r.Dy())
					}
				} else {
					area += int64(s.Width * s.Height)
				}
				builder.Build(s, nil)
				culled += int64(builder.Culled)
				instances += int64(len(builder.Instances))
				d.Remember(s)
			}
			for i := range 4 {
				frame(i)
			}
			area, culled, instances = 0, 0, 0
			b.ReportAllocs()
			i := 4
			for b.Loop() {
				frame(i)
				i++
			}
			b.ReportMetric(float64(polylineSegments), "segments/op")
			b.ReportMetric(float64(area)/float64(b.N), "damage-px/op")
			b.ReportMetric(float64(culled)/float64(b.N), "culled/op")
			b.ReportMetric(float64(instances)/float64(b.N), "instances/op")
		})
	}
}
