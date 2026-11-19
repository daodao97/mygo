package gl

import (
	"errors"
	"fmt"

	"github.com/ebitengine/purego"
)

// EGL's functions, which make the contexts of OpenGL ES and the surfaces
// of the windows they draw into, on Android.
var (
	eglGetDisplay          func(id uintptr) uintptr
	eglInitialize          func(display uintptr, major, minor *int32) uint32
	eglChooseConfig        func(display uintptr, attribs *int32, configs *uintptr, size int32, n *int32) uint32
	eglCreateContext       func(display, config, share uintptr, attribs *int32) uintptr
	eglCreateWindowSurface func(display, config, window uintptr, attribs *int32) uintptr
	eglMakeCurrent         func(display, draw, read, context uintptr) uint32
	eglSwapBuffers         func(display, surface uintptr) uint32
	eglDestroySurface      func(display, surface uintptr) uint32
	eglDestroyContext      func(display, context uintptr) uint32
	eglGetCurrentContext   func() uintptr
	eglGetError            func() int32
)

const (
	eglNone           = 0x3038
	eglRenderableType = 0x3040
	eglOpenGLES3Bit   = 0x40
	eglSurfaceType    = 0x3033
	eglWindowBit      = 0x4
	eglRedSize        = 0x3024
	eglGreenSize      = 0x3023
	eglBlueSize       = 0x3022
	eglAlphaSize      = 0x3021
	eglContextVersion = 0x3098
)

func loadEGL() error {
	lib, err := purego.Dlopen("libEGL.so", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("gl: cannot load libEGL: %w", err)
	}
	for name, f := range map[string]any{
		"eglGetDisplay": &eglGetDisplay, "eglInitialize": &eglInitialize, "eglChooseConfig": &eglChooseConfig,
		"eglCreateContext": &eglCreateContext, "eglCreateWindowSurface": &eglCreateWindowSurface,
		"eglMakeCurrent": &eglMakeCurrent, "eglSwapBuffers": &eglSwapBuffers, "eglDestroySurface": &eglDestroySurface,
		"eglDestroyContext": &eglDestroyContext, "eglGetCurrentContext": &eglGetCurrentContext, "eglGetError": &eglGetError,
	} {
		sym, err := purego.Dlsym(lib, name)
		if err != nil || sym == 0 {
			return fmt.Errorf("gl: libEGL lacks %s", name)
		}
		purego.RegisterFunc(f, sym)
	}
	return nil
}

// Window is a window's surface (an ANativeWindow) and the context of
// OpenGL ES 3 that draws into it. The context, with the programs and
// textures made in it, outlives the surface: Detach lets go of the
// surface, as Android destroys the SurfaceView's as the app goes to the
// background, and Attach makes one on the next ANativeWindow with the
// same configuration, rather than a new context whose programs compile
// again (#2055).
type Window struct {
	display, config, context, surface uintptr
}

// NewWindow makes a context and a surface for an ANativeWindow, the
// context current.
func NewWindow(native uintptr) (*Window, error) {
	if native == 0 {
		return nil, errors.New("gl: no window")
	}
	if err := load(); err != nil {
		return nil, err
	}
	w := &Window{display: eglGetDisplay(0)}
	if w.display == 0 || eglInitialize(w.display, nil, nil) == 0 {
		return nil, fmt.Errorf("gl: EGL has no display (%#x)", eglGetError())
	}
	attribs := []int32{eglRenderableType, eglOpenGLES3Bit, eglSurfaceType, eglWindowBit,
		eglRedSize, 8, eglGreenSize, 8, eglBlueSize, 8, eglAlphaSize, 8, eglNone}
	var n int32
	if eglChooseConfig(w.display, &attribs[0], &w.config, 1, &n) == 0 || n == 0 {
		return nil, fmt.Errorf("gl: EGL has no configuration of OpenGL ES 3 (%#x)", eglGetError())
	}
	ctxAttribs := []int32{eglContextVersion, 3, eglNone}
	if w.context = eglCreateContext(w.display, w.config, 0, &ctxAttribs[0]); w.context == 0 {
		return nil, fmt.Errorf("gl: EGL cannot make a context (%#x)", eglGetError())
	}
	if err := w.Attach(native); err != nil {
		w.Release()
		return nil, err
	}
	return w, nil
}

// Attach makes a surface for an ANativeWindow, after Detach, with the
// configuration the context was made with, and makes the context current
// drawing into it. On an error the window has no surface, its context
// still made: Release lets go of it.
func (w *Window) Attach(native uintptr) error {
	if native == 0 {
		return errors.New("gl: no window")
	}
	if w.context == 0 {
		return errors.New("gl: the window was released")
	}
	w.Detach()
	if w.surface = eglCreateWindowSurface(w.display, w.config, native, nil); w.surface == 0 {
		return fmt.Errorf("gl: EGL cannot make a window's surface (%#x)", eglGetError())
	}
	if !w.MakeCurrent() {
		err := fmt.Errorf("gl: EGL cannot make the context current (%#x)", eglGetError())
		w.Detach()
		return err
	}
	return nil
}

// Detach lets go of the surface, keeping the context and what was made
// in it: the ANativeWindow is about to go. No context is current after.
func (w *Window) Detach() {
	if w.display == 0 {
		return
	}
	eglMakeCurrent(w.display, 0, 0, 0)
	if w.surface != 0 {
		eglDestroySurface(w.display, w.surface)
		w.surface = 0
	}
}

// MakeCurrent makes the window's context current on this thread, drawing
// into its surface; without one (Detach), only where EGL takes a context
// with no surface (EGL_KHR_surfaceless_context), to delete its objects.
func (w *Window) MakeCurrent() bool {
	return eglMakeCurrent(w.display, w.surface, w.surface, w.context) != 0
}

// Swap shows the frame drawn into the surface.
func (w *Window) Swap() error {
	if eglSwapBuffers(w.display, w.surface) == 0 {
		return fmt.Errorf("gl: EGL cannot show the frame (%#x)", eglGetError())
	}
	return nil
}

// Release lets go of the surface and the context; the ANativeWindow stays
// its owner's.
func (w *Window) Release() {
	if w.display == 0 {
		return
	}
	eglMakeCurrent(w.display, 0, 0, 0)
	if w.surface != 0 {
		eglDestroySurface(w.display, w.surface)
	}
	if w.context != 0 {
		eglDestroyContext(w.display, w.context)
	}
	*w = Window{}
}
