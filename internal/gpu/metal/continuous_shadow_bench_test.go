//go:build darwin

package metal

import (
	"testing"
	"time"

	"github.com/egoist/mygo/internal/gpu/gputest"
	"github.com/egoist/mygo/internal/scene"
)

// BenchmarkContinuousShadowCompare alternates full and shortcut formulas in
// one process, ABBA per iteration, after warming both renderers. GPU metrics
// are per frame; ns/op and allocations include all four offscreen frames.
func BenchmarkContinuousShadowCompare(b *testing.B) {
	oldSync := syncPipelines
	syncPipelines = true
	defer func() { syncPipelines = oldSync }()
	small := gputest.Continuously(gputest.BoxesScene(2560, 1600, 600, false))
	for i := range small.Ops {
		o := &small.Ops[i]
		o.Rect.X *= 2
		o.Rect.Y *= 2
		o.Rect.W *= 2
		o.Rect.H *= 2
		o.Blur *= 2
		for j := range o.Radii {
			o.Radii[j] *= 2
		}
		if o.Kind == scene.OpShadow {
			o.Cast = small.Ops[i+1].Rect
			o.Cast.X *= 2
			o.Cast.Y *= 2
			o.Cast.W *= 2
			o.Cast.H *= 2
			for j := range o.CastRadii {
				o.CastRadii[j] = 12
			}
		}
	}
	for _, c := range []struct {
		name string
		s    *scene.Scene
	}{
		{"small_continuous", small},
		{"cards", gputest.CardsScene(2560, 1600)},
		{"circular_control", gputest.BoxesScene(1280, 800, 600, false)},
	} {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			var o [2]*Offscreen
			for i := range o {
				var e error
				o[i], e = NewOffscreen(c.s.Width, c.s.Height)
				if e != nil {
					b.Skip("no Metal:", e)
				}
				defer o[i].Release()
				o[i].r.source = shaderSource
				if i == 0 {
					o[i].r.source = referenceCornerSource()
				}
			}
			for range 20 {
				for i := range o {
					if _, e := o[i].Render(c.s); e != nil {
						b.Fatal(e)
					}
				}
			}
			var elapsed [2]time.Duration
			for b.Loop() {
				for _, i := range []int{0, 1, 1, 0} {
					d, e := o[i].Render(c.s)
					if e != nil {
						b.Fatal(e)
					}
					elapsed[i] += d
				}
			}
			b.ReportMetric(elapsed[0].Seconds()*1000/float64(b.N*2), "old-GPU-ms")
			b.ReportMetric(elapsed[1].Seconds()*1000/float64(b.N*2), "new-GPU-ms")
			b.ReportMetric(float64(elapsed[0])/float64(elapsed[1]), "speedup")
		})
	}
}
