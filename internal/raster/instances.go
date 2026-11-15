package raster

import (
	"image"
	"math"
	"sync/atomic"

	"github.com/egoist/mygo/internal/scene"
)

// instancer gives each quad of the effects of a scene that paint quads
// (scene.EffectOp.Instances) an effect op of its own, reusing its memory
// from scene to scene.
type instancer struct {
	s       scene.Scene
	ops     []scene.Op
	effects []scene.EffectOp
	bounds  []image.Rectangle
}

// split is instancer.split once EnableInstances was called.
var split atomic.Pointer[func(in *instancer, s *scene.Scene, bounds []image.Rectangle) (*scene.Scene, []image.Rectangle)]

// EnableInstances makes renderers draw the effects that paint quads
// (scene.EffectOp.Instances), which they leave out before. Package ui calls
// it as it paints one (Painter.ShaderInstances), on any goroutine: apps
// that never do carry none of the code.
func EnableInstances() {
	if split.Load() == nil {
		f := (*instancer).split
		split.CompareAndSwap(nil, &f)
	}
}

// Split is split once EnableInstances was called, and s and bounds before.
func (in *instancer) Split(s *scene.Scene, bounds []image.Rectangle) (*scene.Scene, []image.Rectangle) {
	if f := split.Load(); f != nil {
		return (*f)(in, s, bounds)
	}
	return s, bounds
}

// split returns s and bounds, where each op of s draws, or, when an effect
// of s paints quads, copies of them with an effect op for each quad in
// its place, holding it alone (scene.EffectOp.Instance), with its bounds:
// the CPU renderer readies an effect for each op it draws (EffectPixels),
// so it draws each quad as a shader drawing it alone would. Quads beyond
// the op's bounds are left out. The copies hold until the next split.
func (in *instancer) split(s *scene.Scene, bounds []image.Rectangle) (*scene.Scene, []image.Rectangle) {
	first := -1
	for i := range s.Ops {
		if instanced(s, &s.Ops[i]) {
			first = i
			break
		}
	}
	if first < 0 {
		return s, bounds
	}
	in.ops = append(in.ops[:0], s.Ops[:first]...)
	in.bounds = append(in.bounds[:0], bounds[:first]...)
	in.effects = append(in.effects[:0], s.Effects...)
	for i := first; i < len(s.Ops); i++ {
		op := s.Ops[i]
		if !instanced(s, &op) {
			in.ops, in.bounds = append(in.ops, op), append(in.bounds, bounds[i])
			continue
		}
		fx := s.Effects[op.Start]
		n := int(fx.InstanceWords)
		for k := range fx.InstanceCount() {
			quad, _ := fx.InstanceRect(k)
			b := bounds[i].Intersect(quadBounds(op.Transform, quad))
			if quad.Empty() || b.Empty() {
				continue
			}
			one := fx
			one.Instances = fx.Instances[k*n : (k+1)*n : (k+1)*n]
			one.Instance = fx.Instance + int32(k)
			q := op
			q.Rect, q.Start = quad, int32(len(in.effects))
			in.effects = append(in.effects, one)
			in.ops, in.bounds = append(in.ops, q), append(in.bounds, b)
		}
	}
	in.s = *s
	in.s.Ops, in.s.Effects = in.ops, in.effects
	return &in.s, in.bounds
}

// instanced reports whether op of s is an effect painting quads.
func instanced(s *scene.Scene, op *scene.Op) bool {
	return op.Kind == scene.OpEffect && op.Start >= 0 && int(op.Start) < len(s.Effects) && s.Effects[op.Start].InstanceCount() > 0
}

// quadBounds returns the pixels a quad under m may draw, with a few to
// spare for antialiasing, as the bounds of ops have.
func quadBounds(m scene.Affine, quad scene.Rect) image.Rectangle {
	r := m.Bounds(quad)
	const spare = 4
	return image.Rect(int(math.Floor(float64(r.X)))-spare, int(math.Floor(float64(r.Y)))-spare,
		int(math.Ceil(float64(r.X+r.W)))+spare, int(math.Ceil(float64(r.Y+r.H)))+spare)
}
