//go:build darwin

package render

import (
	"github.com/egoist/mygo/internal/gpu/metal"
	"github.com/egoist/mygo/internal/scene"
)

// metalDrawer draws frames with Metal, into a texture whose pixels it
// reads back, or into the encoder's pixel buffers (avSceneSink).
type metalDrawer struct {
	f    *metal.Frames
	gpu  string
	pix  []byte
	w, h int
}

func newGPUDrawer(w, h int, _ bool) (drawer, error) {
	f, err := metal.NewFrames()
	if err != nil {
		return nil, err
	}
	return &metalDrawer{f: f, gpu: f.Name(), w: w, h: h}, nil
}

func (d *metalDrawer) submit(s *scene.Scene) error { return d.f.Draw(s) }

func (d *metalDrawer) pixels() ([]byte, error) {
	if err := d.f.Wait(); err != nil {
		return nil, err
	}
	if n := d.w * d.h * 4; len(d.pix) != n {
		d.pix = make([]byte, n)
	}
	return d.pix, d.f.Read(d.pix)
}

func (d *metalDrawer) name() string { return "Metal (" + d.gpu + ")" }
func (d *metalDrawer) release()     { d.f.Release() }
