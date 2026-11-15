package shader_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

// instancesSource draws a quad from its data, or from its uniforms when
// drawn alone (Draw), where instanceData and instanceIndex are 0: a color,
// stripes as many as its index plus one across its rectangle, and a ring
// at its edge.
var instancesSource = `package main

var Color vec4
var Index int

func stripes(pos vec2) float {
	i := Index + instanceIndex()
	return step(0.5, fract(pos.x/rectSize().x*float(i+1)))
}

func Fragment(dst vec4, pos vec2) vec4 {
	c := Color + vec4(instanceData(0), instanceData(1), instanceData(2), instanceData(3))
	c.rgb *= mix(0.5, 1.0, stripes(pos))
	d := min(min(pos.x, pos.y), min(rectSize().x-pos.x, rectSize().y-pos.y))
	if d < 2 {
		c.rgb = vec3(instanceData(4) + Color.a*0.25)
	}
	return c
}
`

// instancesQuads are quads that overlap, between pixels at scale 1.5, with
// their colors and, after, the gray of their rings, a quarter of their
// alpha, as Color gives it drawn alone.
func instancesQuads() ([]ui.Rect, []float32) {
	var rects []ui.Rect
	var data []float32
	for i := range 12 {
		x, y := 6.3+float32(i%4)*41.7, 5.6+float32(i/4)*27.9
		rects = append(rects, ui.Rect{X: x, Y: y, W: 50.4, H: 31.1})
		k := float32(i) / 11
		data = append(data, k, 1-k, 0.5, 1, 0.25)
	}
	// Half transparent, over the others.
	rects = append(rects, ui.Rect{X: 40.2, Y: 30.7, W: 90.5, H: 40.3})
	data = append(data, 0.2, 0.1, 0.4, 0.5, 0.125)
	return rects, data
}

// drawQuads draws the quads in a box rotated by turn, each alone (Draw)
// or all in one draw (DrawInstances).
func drawQuads(s *shader.Shader, alone bool, turn float32) func(c *ui.Context) {
	rects, data := instancesQuads()
	const stride = 5
	return func(c *ui.Context) {
		ui.Box(c).Fill().Background(ui.RGB(255, 255, 255)).Rotate(turn).Draw(func(p *ui.Painter, r ui.Rect) {
			if !alone {
				s.DrawInstances(p, shader.Instances{Rects: rects, Data: data, Stride: stride}, shader.Options{Radius: 6})
				return
			}
			for i, q := range rects {
				d := data[i*stride:]
				s.Draw(p, q, shader.Options{Radius: 6, Uniforms: map[string]any{
					"Color": d[:4], "Index": i,
				}})
			}
		})
	}
}

// The CPU draws the quads of one DrawInstances as it draws each alone with
// Draw, pixel for pixel: their rectangles, corners, data, indices and
// order, under a transform too.
func TestDrawInstancesAsDraws(t *testing.T) {
	s := shader.MustCompile(instancesSource)
	for _, turn := range []float32{0, 7} {
		images := [2][]byte{}
		for k, alone := range []bool{true, false} {
			tt := ui.NewTester(drawQuads(s, alone, turn), 200, 100)
			tt.SetScale(1.5)
			images[k] = tt.Image().Pix
		}
		if !bytes.Equal(images[0], images[1]) {
			n := 0
			for i := range images[0] {
				if images[0][i] != images[1][i] {
					n++
				}
			}
			t.Errorf("rotated %v°: DrawInstances differs from Draw in %d bytes", turn, n)
		}
	}
}

// Each quad's shader reads its own data and index, and its rectangle's
// size; the data of none reaches another.
func TestDrawInstancesData(t *testing.T) {
	s := shader.MustCompile(`package main

func Fragment(dst vec4, pos vec2) vec4 {
	if instanceIndex() == 2 && rectSize().x == 30 {
		return vec4(0, 0, 1, 1)
	}
	return vec4(instanceData(0), instanceData(15), instanceData(16), 1)
}
`)
	data := make([]float32, 3*16)
	data[0], data[16+15], data[32+15] = 1, 1, 1
	view := func(c *ui.Context) {
		ui.Box(c).Fill().Background(ui.RGB(255, 255, 255)).Draw(func(p *ui.Painter, r ui.Rect) {
			s.DrawInstances(p, shader.Instances{
				Rects:  []ui.Rect{{X: 0, Y: 0, W: 20, H: 20}, {X: 20, Y: 0, W: 20, H: 20}, {X: 40, Y: 0, W: 30, H: 20}},
				Data:   data,
				Stride: 16,
			}, shader.Options{})
			// Alone, a shader reads no instance.
			s.Draw(p, ui.Rect{X: 0, Y: 30, W: 30, H: 20}, shader.Options{})
		})
	}
	img := ui.NewTester(view, 80, 60).Image()
	at := func(x, y int) [4]uint8 {
		o := img.PixOffset(x, y)
		return [4]uint8(img.Pix[o : o+4])
	}
	for _, c := range []struct {
		x, y int
		want [4]uint8
	}{
		{10, 10, [4]uint8{255, 0, 0, 255}},     // instance 0: data[0]
		{30, 10, [4]uint8{0, 255, 0, 255}},     // instance 1: its data[15]
		{55, 10, [4]uint8{0, 0, 255, 255}},     // instance 2, 30 DIPs wide
		{15, 40, [4]uint8{0, 0, 0, 255}},       // Draw: no data
		{75, 10, [4]uint8{255, 255, 255, 255}}, // nothing drawn
	} {
		if got := at(c.x, c.y); got != c.want {
			t.Errorf("at (%d, %d): %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

// DrawInstances draws at most MaxInstances quads, with at most
// MaxInstanceStride floats each, of as many as the quads need, and no
// shader reading its backdrop; it draws nothing for no quads.
func TestDrawInstancesCaps(t *testing.T) {
	s := shader.MustCompile("package main\nfunc Fragment(dst vec4, pos vec2) vec4 { return vec4(instanceData(0)) }\n")
	backdrop := shader.MustCompile("package main\nfunc Fragment(dst vec4, pos vec2) vec4 { return backdropAt(pos) }\n")
	if shader.MaxInstanceStride != 16 || shader.MaxInstances != 1<<16 {
		t.Errorf("the caps are %d floats and %d instances, not those the documentation gives", shader.MaxInstanceStride, shader.MaxInstances)
	}
	rect := ui.Rect{W: 1, H: 1}
	many := make([]ui.Rect, shader.MaxInstances+1)
	for _, c := range []struct {
		s    *shader.Shader
		in   shader.Instances
		want string
	}{
		{s, shader.Instances{Rects: []ui.Rect{rect}, Data: make([]float32, 17), Stride: 17}, "stride is 17"},
		{s, shader.Instances{Rects: []ui.Rect{rect}, Stride: -1}, "stride is -1"},
		{s, shader.Instances{Rects: many}, "beyond 65536"},
		{s, shader.Instances{Rects: []ui.Rect{rect, rect}, Data: make([]float32, 3), Stride: 2}, "3 floats of data for 2"},
		{backdrop, shader.Instances{Rects: []ui.Rect{rect}}, "reading its backdrop"},
		{s, shader.Instances{Rects: many[:shader.MaxInstances], Data: make([]float32, shader.MaxInstances*16), Stride: 16}, ""},
		{s, shader.Instances{}, ""},
	} {
		var got any
		ui.NewTester(func(ctx *ui.Context) {
			ui.Box(ctx).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
				defer func() { got = recover() }()
				c.s.DrawInstances(p, c.in, shader.Options{})
			})
		}, 4, 4).Image()
		switch {
		case c.want == "" && got != nil:
			t.Errorf("%d instances of %d: %v, want no panic", len(c.in.Rects), c.in.Stride, got)
		case c.want != "" && !strings.Contains(fmt.Sprint(got), c.want):
			t.Errorf("%d instances of %d: %v, want a panic saying %q", len(c.in.Rects), c.in.Stride, got, c.want)
		}
	}
	// The last of MaxInstances quads draws, its index whole.
	last := shader.MustCompile(fmt.Sprintf("package main\nfunc Fragment(dst vec4, pos vec2) vec4 { return vec4(0, step(%d, float(instanceIndex())), instanceData(0), 1) }\n", shader.MaxInstances-1))
	quads, data := many[:shader.MaxInstances], make([]float32, shader.MaxInstances)
	quads[len(quads)-1], data[len(data)-1] = ui.Rect{W: 4, H: 4}, 1
	img := ui.NewTester(func(ctx *ui.Context) {
		ui.Box(ctx).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
			last.DrawInstances(p, shader.Instances{Rects: quads, Data: data, Stride: 1}, shader.Options{})
		})
	}, 4, 4).Image()
	if got := [4]uint8(img.Pix[:4]); got != [4]uint8{0, 255, 255, 255} {
		t.Errorf("the last of %d quads: %v, want cyan, its index and data", shader.MaxInstances, got)
	}
}
