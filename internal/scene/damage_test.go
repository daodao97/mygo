package scene

import (
	"image"
	"math"
	"testing"
)

// TestDamageOfNestedTransformedClip checks that a clip under a
// transform that turns it, nested in a rounded one, changes all the
// rounded one cuts as it comes and goes: the GPU's renderers then cut the
// outer clip with a mask rather than in the shader, at its edges a unit in
// the last place apart. A clip only moved changes nothing around it, as
// the renderers cut it as any other.
func TestDamageOfNestedTransformedClip(t *testing.T) {
	outer := Op{Kind: OpPushClip, Rect: Rect{X: 10, Y: 10, W: 150, H: 150}, Radii: Uniform(8)}
	corner := Op{Kind: OpFill, Rect: Rect{X: 10, Y: 10, W: 20, H: 20}, Color: Color{R: 255, A: 255}}
	nested := []Op{
		{Kind: OpPushClip, Rect: Rect{W: 40, H: 40}, Transform: Translation(100, 100).Mul(Rotation(10))},
		{Kind: OpFill, Rect: Rect{X: 100, Y: 100, W: 20, H: 20}, Color: Color{G: 255, A: 255}},
		{Kind: OpPopClip},
	}
	without := &Scene{Width: 200, Height: 200, Ops: []Op{outer, corner, {Kind: OpPopClip}}}
	with := &Scene{Width: 200, Height: 200, Ops: append(append([]Op{outer, corner}, nested...), Op{Kind: OpPopClip})}
	for _, frames := range [][2]*Scene{{without, with}, {with, without}} {
		d := Differ{Reach: 2}
		d.Diff(frames[0])
		d.Remember(frames[0])
		damage, ok := d.Diff(frames[1])
		if !ok {
			t.Fatal("no damage found")
		}
		// The fill in the outer clip's corner, which the inner clip
		// does not reach.
		at := image.Rect(10, 10, 30, 30)
		hit := false
		for _, r := range damage {
			hit = hit || at.In(r)
		}
		if !hit {
			t.Errorf("%d to %d ops: damage %v misses %v", len(frames[0].Ops), len(frames[1].Ops), damage, at)
		}
	}
	nested[0].Transform = Translation(100.5, 100.25)
	with.Ops = append(append([]Op{outer, corner}, nested...), Op{Kind: OpPopClip})
	d := Differ{Reach: 2}
	d.Diff(without)
	d.Remember(without)
	damage, _ := d.Diff(with)
	for _, r := range damage {
		if r.Overlaps(image.Rect(10, 10, 30, 30)) {
			t.Errorf("a clip only moved coming: damage %v reaches the outer clip's corner", damage)
		}
	}
}

// TestDamageOfClipInRoundedCorners checks that a clip reaching into the
// corners of a rounded one changes all the rounded one cuts as it comes
// and goes, which the GPU's renderers then cut with a mask (#1418), and
// that one away from them changes only what it cuts.
func TestDamageOfClipInRoundedCorners(t *testing.T) {
	outer := Op{Kind: OpPushClip, Rect: Rect{X: 10, Y: 10, W: 150, H: 150}, Radii: Uniform(8)}
	corner := Op{Kind: OpFill, Rect: Rect{X: 10, Y: 10, W: 20, H: 20}, Color: Color{R: 255, A: 255}}
	nested := func(r Rect) []Op {
		return []Op{{Kind: OpPushClip, Rect: r}, {Kind: OpFill, Rect: r, Color: Color{G: 255, A: 255}}, {Kind: OpPopClip}}
	}
	without := &Scene{Width: 200, Height: 200, Ops: []Op{outer, corner, {Kind: OpPopClip}}}
	at := image.Rect(10, 10, 30, 30)
	for _, c := range []struct {
		r       Rect
		corners bool
	}{{Rect{X: 10, Y: 120, W: 150, H: 40}, true}, {Rect{X: 60, Y: 60, W: 40, H: 40}, false}} {
		with := &Scene{Width: 200, Height: 200, Ops: append(append([]Op{outer, corner}, nested(c.r)...), Op{Kind: OpPopClip})}
		for _, frames := range [][2]*Scene{{without, with}, {with, without}} {
			d := Differ{Reach: 2}
			d.Diff(frames[0])
			d.Remember(frames[0])
			damage, ok := d.Diff(frames[1])
			if !ok {
				t.Fatal("no damage found")
			}
			hit := false
			for _, r := range damage {
				hit = hit || r.Overlaps(at)
			}
			if hit != c.corners {
				t.Errorf("clip %v, %d to %d ops: damage %v reaches %v: %v", c.r, len(frames[0].Ops), len(frames[1].Ops), damage, at, hit)
			}
		}
	}
}

// A masked clip changes all it cuts when its mask changes: the stops of
// its gradient, or the pixels of its image; and nothing when they are the
// same, in another frame's scene.
func TestDamageOfMaskedClip(t *testing.T) {
	img := NewImageRGBA(4, 4, make([]byte, 64))
	build := func(alpha uint8) *Scene {
		return &Scene{Width: 100, Height: 100, Stops: []Stop{{Color: Color{A: alpha}}, {Color: Color{}, At: 1}}, Ops: []Op{
			{Kind: OpPushClip, Rect: Rect{X: 10, Y: 10, W: 40, H: 40}, Paint: PaintLinearStops, Gradient: [4]float32{10, 10, 50, 10}, Stops: [2]int32{0, 2}},
			{Kind: OpFill, Rect: Rect{X: 10, Y: 10, W: 40, H: 40}, Color: Color{R: 255, A: 255}},
			{Kind: OpPopClip},
			{Kind: OpPushClip, Rect: Rect{X: 60, Y: 60, W: 4, H: 4}, Image: img, Src: Rect{W: 4, H: 4}},
			{Kind: OpFill, Rect: Rect{X: 60, Y: 60, W: 4, H: 4}, Color: Color{G: 255, A: 255}},
			{Kind: OpPopClip},
		}}
	}
	d := Differ{Reach: 1}
	d.Diff(build(255))
	d.Remember(build(255))
	if damage, ok := d.Diff(build(255)); !ok || len(damage) != 0 {
		t.Fatalf("the same masks: damage %v, %v", damage, ok)
	}
	d.Remember(build(255))
	if damage, ok := d.Diff(build(128)); !ok || len(damage) != 1 || !image.Rect(10, 10, 50, 50).In(damage[0]) {
		t.Errorf("other stops: damage %v, %v", damage, ok)
	}
	d.Remember(build(128))
	img.Changed()
	if damage, ok := d.Diff(build(128)); !ok || len(damage) != 1 || !image.Rect(60, 60, 64, 64).In(damage[0]) {
		t.Errorf("the image changed: damage %v, %v", damage, ok)
	}
}

// An effect painting quads (EffectOp.Instances) damages where it paints
// when a quad's data changes, though the scene reuses its words next
// frame (Words), and nothing when the quads stay the same.
func TestDamageOfInstances(t *testing.T) {
	fx := &Effect{Name: "quads"}
	words := make([]uint32, 2*(InstanceHead+1))
	build := func(data float32) *Scene {
		for i, q := range []Rect{{X: 10, Y: 10, W: 20, H: 20}, {X: 50, Y: 40, W: 20, H: 20}} {
			w := words[i*(InstanceHead+1):]
			for k, v := range []float32{q.X, q.Y, q.W, q.H, q.W, q.H, data} {
				w[k] = math.Float32bits(v)
			}
		}
		return &Scene{Width: 100, Height: 100, Words: words,
			Ops:     []Op{{Kind: OpEffect, Rect: Rect{X: 10, Y: 10, W: 60, H: 50}}},
			Effects: []EffectOp{{Effect: fx, Instances: words, InstanceWords: InstanceHead + 1}}}
	}
	d := Differ{Reach: 1}
	d.Diff(build(1))
	d.Remember(build(1))
	if damage, ok := d.Diff(build(1)); !ok || len(damage) != 0 {
		t.Fatalf("the same quads: damage %v, %v", damage, ok)
	}
	d.Remember(build(1))
	if damage, ok := d.Diff(build(2)); !ok || len(damage) != 1 || !image.Rect(10, 10, 70, 60).In(damage[0]) {
		t.Errorf("other data: damage %v, %v", damage, ok)
	}
}
