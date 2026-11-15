//go:build windows

package render

import (
	"github.com/egoist/mygo/internal/gpu/d3d11"
	"github.com/egoist/mygo/internal/scene"
)

// d3dDrawer draws frames with Direct3D 11, into a window that never shows,
// whose frames it reads back through a staging texture.
type d3dDrawer struct {
	f    *d3d11.Frames
	gpu  string
	pix  []byte
	w, h int
}

// newGPUDrawer returns a Direct3D 11 drawer. Without a GPU, Direct3D draws
// with WARP, on the CPU, slower than MyGo's own CPU renderer, which
// redraws only what changed: Auto takes that instead, GPU takes WARP.
func newGPUDrawer(w, h int, required bool) (drawer, error) {
	f, err := d3d11.NewFrames(w, h)
	if err != nil {
		return nil, err
	}
	if f.Software() && !required {
		f.Release()
		return nil, nil
	}
	return &d3dDrawer{f: f, gpu: f.Name(), w: w, h: h}, nil
}

func (d *d3dDrawer) submit(s *scene.Scene) error { return d.f.Draw(s) }

func (d *d3dDrawer) pixels() ([]byte, error) {
	if n := d.w * d.h * 4; len(d.pix) != n {
		d.pix = make([]byte, n)
	}
	return d.pix, d.f.Read(d.pix, d.w, d.h)
}

func (d *d3dDrawer) name() string { return "Direct3D 11 (" + d.gpu + ")" }
func (d *d3dDrawer) release()     { d.f.Release() }
