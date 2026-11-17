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
// OpenGL ES 3 that draws into it.
type Window struct {
	display, context, surface uintptr
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
	var config uintptr
	var n int32
	if eglChooseConfig(w.display, &attribs[0], &config, 1, &n) == 0 || n == 0 {
		return nil, fmt.Errorf("gl: EGL has no configuration of OpenGL ES 3 (%#x)", eglGetError())
	}
	ctxAttribs := []int32{eglContextVersion, 3, eglNone}
	if w.context = eglCreateContext(w.display, config, 0, &ctxAttribs[0]); w.context == 0 {
		return nil, fmt.Errorf("gl: EGL cannot make a context (%#x)", eglGetError())
	}
	if w.surface = eglCreateWindowSurface(w.display, config, native, nil); w.surface == 0 {
		eglDestroyContext(w.display, w.context)
		return nil, fmt.Errorf("gl: EGL cannot make a window's surface (%#x)", eglGetError())
	}
	if !w.MakeCurrent() {
		w.Release()
		return nil, fmt.Errorf("gl: EGL cannot make the context current (%#x)", eglGetError())
	}
	return w, nil
}

// MakeCurrent makes the window's context current on this thread, drawing
// into its surface.
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
