//go:build linux && (amd64 || arm64)

package render

import (
	"strings"

	"github.com/egoist/mygo/internal/gpu/gl"
	"github.com/egoist/mygo/internal/scene"
)

// glDrawer draws frames with OpenGL in a context of EGL without a
// display (Mesa's surfaceless platform), current on the rendering thread.
type glDrawer struct {
	o    *gl.Offscreen
	info string
	pix  []byte
	w, h int
}

// newGPUDrawer returns an OpenGL 3.3 drawer, else an OpenGL ES 3.0 one.
// Mesa without a GPU draws with llvmpipe, on the CPU, slower than MyGo's
// own CPU renderer, which redraws only what changed: Auto takes that
// instead, GPU takes llvmpipe.
func newGPUDrawer(w, h int, required bool) (drawer, error) {
	var first error
	for _, es := range []bool{false, true} {
		o, err := gl.NewOffscreen(es, w, h)
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		info := o.Info()
		if soft := strings.Contains(info, "llvmpipe") || strings.Contains(info, "softpipe") || strings.Contains(info, "SwiftShader"); soft && !required {
			o.Release()
			return nil, nil
		}
		return &glDrawer{o: o, info: info, w: w, h: h}, nil
	}
	return nil, first
}

func (d *glDrawer) submit(s *scene.Scene) error { return d.o.Draw(s) }

func (d *glDrawer) pixels() ([]byte, error) {
	if n := d.w * d.h * 4; len(d.pix) != n {
		d.pix = make([]byte, n)
	}
	return d.pix, d.o.Read(d.pix)
}

func (d *glDrawer) name() string { return "OpenGL (" + d.info + ")" }
func (d *glDrawer) release()     { d.o.Release() }
