//go:build darwin

package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/internal/gpu/metal"
)

// BenchmarkStrokePolylineGPU paints the same changing inputs as
// BenchmarkStrokePolylineScene and draws them with Metal into a texture,
// waiting for each frame. GPU-ms comes from command buffer timestamps;
// encode-ms is CPU encoding time. Offscreen renders the whole target, so
// this measures neither damage-limited window drawing nor presentation.
func BenchmarkStrokePolylineGPU(b *testing.B) {
	for _, mode := range polylineWorkloads {
		b.Run(mode, func(b *testing.B) {
			o, err := metal.NewOffscreen(polylineWidth, polylineHeight)
			if err != nil {
				b.Skip("no Metal:", err)
			}
			defer o.Release()
			s, paint := newPolylineWorkload(mode)
			var gpuTime, encoded time.Duration
			frame := func(i int) {
				paint(i)
				d, err := o.Render(s)
				if err != nil {
					b.Fatal(err)
				}
				gpuTime += d
				encoded += o.Encoded()
			}
			for i := range 4 {
				frame(i)
			}
			gpuTime, encoded = 0, 0
			b.ReportAllocs()
			i := 4
			for b.Loop() {
				frame(i)
				i++
			}
			b.ReportMetric(gpuTime.Seconds()*1000/float64(b.N), "GPU-ms")
			b.ReportMetric(encoded.Seconds()*1000/float64(b.N), "encode-ms")
		})
	}
}
