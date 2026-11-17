package ui

import (
	"github.com/egoist/mygo/internal/gpu/gl"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/scene"
)

// newGPURenderer returns an OpenGL ES renderer drawing into the surface's
// ANativeWindow, with an EGL context of its own.
func newGPURenderer(n platform.SurfaceNative) (gpuRenderer, error) {
	if n.Window == 0 {
		return nil, nil
	}
	w, err := gl.NewWindow(n.Window)
	if err != nil {
		return nil, err
	}
	r, err := gl.New()
	if err != nil {
		w.Release()
		return nil, err
	}
	return &windowRenderer{Renderer: r, w: w}, nil
}

// windowRenderer draws a frame and shows it: an EGL window's buffers keep
// nothing from one frame to the next, so frames draw whole.
type windowRenderer struct {
	*gl.Renderer
	w *gl.Window
}

func (r *windowRenderer) Render(s *scene.Scene) error {
	r.w.MakeCurrent()
	if err := r.Renderer.Render(s); err != nil {
		return err
	}
	return r.w.Swap()
}

func (r *windowRenderer) Release() {
	r.w.MakeCurrent()
	r.Renderer.Release()
	r.w.Release()
}
