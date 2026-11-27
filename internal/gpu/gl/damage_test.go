//go:build linux && !android && (amd64 || arm64)

package gl

import (
	"bytes"
	"testing"

	"github.com/egoist/mygo/internal/gpu/gputest"
	"github.com/egoist/mygo/internal/scene"
)

// TestPartialFramesMatchWhole draws random scenes changing a little at a
// time into a framebuffer that keeps what was drawn, as a GtkGLArea's,
// each frame only where it changed, and requires the bytes of each frame
// drawn whole, over an opaque color and a translucent one. Now and then
// the framebuffer loses what it held, as GTK makes the area's buffers
// anew, which Invalidate tells.
func TestPartialFramesMatchWhole(t *testing.T) {
	seeds := uint64(12)
	if testing.Short() {
		seeds = 6
	}
	for _, api := range apis {
		t.Run(api.name, func(t *testing.T) {
			read := offscreen(t, api.es, 320, 240)
			var outer int32
			glGetIntegerv(glFramebufferBinding, &outer)
			// The frames drawn whole go into a framebuffer of their own.
			tex := newTexture(glRGBA8, glRGBA, 320, 240, nil, 0)
			var fb uint32
			glGenFramebuffers(1, &fb)
			glBindFramebuffer(glFramebuffer, fb)
			glFramebufferTexture2D(glFramebuffer, glColorAttachment0, glTexture2D, tex, 0)
			defer func() {
				glDeleteFramebuffers(1, &fb)
				glDeleteTextures(1, &tex)
			}()
			frames, partial := 0, 0
			for seed := range seeds {
				m := gputest.NewChanging(seed)
				if seed%3 == 2 {
					m.Clear = scene.Color{R: 20, G: 40, B: 200, A: 77}
				}
				s := m.Scene()
				// Renderers of their own, as each scene has an atlas of its own.
				r, err := New()
				if err != nil {
					t.Fatal(err)
				}
				r.Partial = true
				whole, err := New()
				if err != nil {
					t.Fatal(err)
				}
				for step := range 30 {
					what := "first"
					if step > 0 {
						what = m.Change(s)
					}
					if step%11 == 10 {
						// GTK made the buffers anew: what they hold is undefined.
						glBindFramebuffer(glFramebuffer, uint32(outer))
						glClearColor(1, 0, 1, 1)
						glClear(glColorBufferBit)
						r.Invalidate()
						what += ", invalidated"
					}
					glBindFramebuffer(glFramebuffer, uint32(outer))
					if err := r.Render(s); err != nil {
						t.Fatal(err)
					}
					got := read()
					frames++
					part := "whole"
					if r.redrew != redrewWhole {
						partial++
						part = r.part.String()
					}
					glBindFramebuffer(glFramebuffer, fb)
					if err := whole.Render(s); err != nil {
						t.Fatal(err)
					}
					want := read()
					if !bytes.Equal(got, want) {
						n := 0
						for i := range got {
							if got[i] != want[i] {
								n++
							}
						}
						t.Fatalf("seed %d, step %d (%s), redrew %s: %d bytes differ from the frame drawn whole", seed, step, what, part, n)
					}
				}
				r.Release()
				whole.Release()
			}
			if partial < frames/3 {
				t.Errorf("%d frames of %d drawn in part", partial, frames)
			}
		})
	}
}

// TestPartialFramesApart checks that two changes in opposite corners, a
// spinner's and a progress bar's, redraw two rectangles, not all between,
// and the bytes of the frame drawn whole.
func TestPartialFramesApart(t *testing.T) {
	for _, api := range apis {
		t.Run(api.name, func(t *testing.T) {
			read := offscreen(t, api.es, 320, 240)
			s := gputest.NewChanging(5).Scene()
			s.Ops = append(s.Ops,
				scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: 4, Y: 4, W: 12, H: 12}, Radii: scene.Uniform(6), Color: scene.Color{R: 200, A: 255}},
				scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: 250, Y: 226, W: 60, H: 6}, Radii: scene.Uniform(3), Color: scene.Color{B: 200, A: 255}})
			r, err := New()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Release()
			r.Partial = true
			whole, err := New()
			if err != nil {
				t.Fatal(err)
			}
			defer whole.Release()
			for step := range 4 {
				if step > 0 {
					n := len(s.Ops)
					s.Ops[n-2].Color.G += 60
					s.Ops[n-1].Rect.W -= 7
				}
				if err := r.Render(s); err != nil {
					t.Fatal(err)
				}
				got := read()
				if step > 0 && (r.redrew != redrewPart || len(r.part.Rects) != 2) {
					t.Errorf("step %d redrew %v: %s", step, r.redrew, r.part.String())
				}
				if err := whole.Render(s); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, read()) {
					t.Fatalf("step %d, redrew %s: differs from the frame drawn whole", step, r.part.String())
				}
			}
		})
	}
}

// TestUnchangedFramesDrawNothing checks that a renderer drawing frames
// whole into buffers that keep nothing, as an EGL window surface's on
// Android, draws nothing for a frame the same as the last one, leaving the
// framebuffer as it was so the surface is not swapped and keeps showing
// the last frame drawn (Unchanged), and that what shows is always the
// frame drawn whole; after Invalidate it draws (#2339).
func TestUnchangedFramesDrawNothing(t *testing.T) {
	for _, api := range apis {
		t.Run(api.name, func(t *testing.T) {
			read := offscreen(t, api.es, 320, 240)
			m := gputest.NewChanging(3)
			s := m.Scene()
			r, err := New()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Release()
			r.Unchanged = true
			whole, err := New()
			if err != nil {
				t.Fatal(err)
			}
			defer whole.Release()
			// shown is what the surface shows: the last frame drawn.
			var shown []byte
			drew := 0
			for step := range 16 {
				what := "first"
				switch {
				case step%4 == 1:
					what = "the same"
				case step%4 == 3:
					what = "the same, invalidated"
					r.Invalidate()
				case step > 0:
					what = m.Change(s)
				}
				// The buffer drawn into holds what no frame draws.
				glClearColor(1, 0, 1, 1)
				glClear(glColorBufferBit)
				before := read()
				if err := r.Render(s); err != nil {
					t.Fatal(err)
				}
				got := read()
				switch {
				case r.Drew():
					drew++
					shown = got
				case !bytes.Equal(got, before):
					t.Fatalf("step %d (%s): drew into the framebuffer, though Drew says not", step, what)
				case step == 0 || step%4 == 3:
					// A change may change nothing that shows; these must draw.
					t.Fatalf("step %d (%s): drew nothing", step, what)
				}
				if step%4 == 1 && r.Drew() {
					t.Fatalf("step %d (%s): drew the frame again", step, what)
				}
				if err := whole.Render(s); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(shown, read()) {
					t.Fatalf("step %d (%s): what shows differs from the frame drawn whole", step, what)
				}
			}
			if drew > 12 {
				t.Errorf("drew %d frames of 16", drew)
			}
		})
	}
}
