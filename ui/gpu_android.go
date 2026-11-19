package ui

import (
	"errors"

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
// nothing from one frame to the next, so frames draw whole. It outlives
// the SurfaceView's surface (renewableRenderer): as the app goes to the
// background and comes back, the EGL context and the renderer's
// programs, atlases and textures stay, and only the EGL surface is made
// anew, in milliseconds rather than the second making them again took,
// past the second Android waits for the window's first draw (#2055).
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

var _ renewableRenderer = (*windowRenderer)(nil)

func (r *windowRenderer) Detach() { r.w.Detach() }

func (r *windowRenderer) Attach(n platform.SurfaceNative) error {
	if n.Window == 0 {
		return errors.New("ui: no window to draw into")
	}
	return r.w.Attach(n.Window)
}
