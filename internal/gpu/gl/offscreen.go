//go:build linux && !android && (amd64 || arm64)

package gl

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/egoist/mygo/internal/scene"
)

// EGL enumerations.
const (
	eglPlatformSurfaceless   = 0x31DD // EGL_PLATFORM_SURFACELESS_MESA
	eglNone                  = 0x3038
	eglRenderableType        = 0x3040
	eglOpenGLBit             = 0x0008
	eglSurfaceType           = 0x3033
	eglPbufferBit            = 0x0001
	eglOpenGLAPI             = 0x30A2
	eglOpenGLESAPI           = 0x30A0
	eglOpenGLES3Bit          = 0x0040 // EGL_OPENGL_ES3_BIT_KHR
	eglContextMajorVersion   = 0x3098
	eglContextMinorVersion   = 0x30FB
	eglContextProfileMask    = 0x30FD
	eglContextCoreProfileBit = 0x1
)

// Offscreen is a GL context without a window, current on the thread that
// made it, which must stay locked to it, with a framebuffer bound: for
// drawing scenes in tests, as packages drawing effects do. EGL makes it
// with Mesa's surfaceless platform, which draws with llvmpipe without a
// GPU.
type Offscreen struct {
	display, context uintptr
	fb, tex          uint32
	w, h             int
	release          func()
	r                *Renderer
	// scratch holds the framebuffer's rows as GL reads them (Read).
	scratch []byte
}

// The functions of EGL NewOffscreen calls, loaded once.
var (
	eglOnce sync.Once
	errEGL  error

	eglGetPlatformDisplay func(platform uint32, native uintptr, attribs unsafe.Pointer) uintptr
	eglInitialize         func(display uintptr, major, minor *int32) uint32
	eglBindAPI            func(api uint32) uint32
	eglChooseConfig       func(display uintptr, attribs *int32, configs *uintptr, size int32, n *int32) uint32
	eglCreateContext      func(display, config, share uintptr, attribs *int32) uintptr
	eglMakeCurrent        func(display, draw, read, context uintptr) uint32
	eglDestroyContext     func(display, context uintptr) uint32
	eglTerminate          func(display uintptr) uint32
)

// The surfaceless display is the process's one: eglGetPlatformDisplay
// returns the same to every caller, and eglInitialize and eglTerminate
// count nothing, so one context's eglTerminate would tear the display
// down under the contexts other threads draw with (plugins/render's
// workers), which crashes Mesa. The offscreen contexts share it instead:
// the first initializes it, the last released terminates it.
var surfaceless struct {
	mu      sync.Mutex
	display uintptr
	refs    int
}

// loadEGL loads libEGL's functions, the first time it is called.
func loadEGL() error {
	eglOnce.Do(func() {
		lib, err := purego.Dlopen("libEGL.so.1", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			errEGL = fmt.Errorf("gl: no EGL: %w", err)
			return
		}
		for name, fn := range map[string]any{
			"eglGetPlatformDisplay": &eglGetPlatformDisplay, "eglInitialize": &eglInitialize, "eglBindAPI": &eglBindAPI,
			"eglChooseConfig": &eglChooseConfig, "eglCreateContext": &eglCreateContext, "eglMakeCurrent": &eglMakeCurrent,
			"eglDestroyContext": &eglDestroyContext, "eglTerminate": &eglTerminate,
		} {
			sym, err := purego.Dlsym(lib, name)
			if err != nil || sym == 0 {
				errEGL = fmt.Errorf("gl: EGL has no %s", name)
				return
			}
			purego.RegisterFunc(fn, sym)
		}
	})
	return errEGL
}

// acquireDisplay returns the surfaceless display, initialized, which
// releaseDisplay gives back.
func acquireDisplay() (uintptr, error) {
	surfaceless.mu.Lock()
	defer surfaceless.mu.Unlock()
	if surfaceless.refs == 0 {
		display := eglGetPlatformDisplay(eglPlatformSurfaceless, 0, nil)
		if display == 0 || eglInitialize(display, nil, nil) == 0 {
			return 0, errors.New("gl: no surfaceless EGL display")
		}
		surfaceless.display = display
	}
	surfaceless.refs++
	return surfaceless.display, nil
}

// releaseDisplay terminates the display once no context holds it.
func releaseDisplay() {
	surfaceless.mu.Lock()
	defer surfaceless.mu.Unlock()
	if surfaceless.refs--; surfaceless.refs == 0 {
		eglTerminate(surfaceless.display)
		surfaceless.display = 0
	}
}

// NewOffscreen makes an OpenGL 3.3 context, or with es an OpenGL ES 3.0
// one, current on the calling thread, and binds a framebuffer of w×h
// pixels. Contexts on several threads at once share EGL's display.
func NewOffscreen(es bool, w, h int) (_ *Offscreen, err error) {
	if err := loadEGL(); err != nil {
		return nil, err
	}
	// A variable of its own, not a named result: returning an error would
	// set that to nil before the undoing below runs.
	off := &Offscreen{w: w, h: h}
	var undo []func()
	off.release = func() {
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i]()
		}
	}
	defer func() {
		if err != nil {
			off.release()
		}
	}()
	display, err := acquireDisplay()
	if err != nil {
		return nil, err
	}
	off.display = display
	undo = append(undo, releaseDisplay)
	api, bit, contextAttribs := uint32(eglOpenGLAPI), int32(eglOpenGLBit), []int32{eglContextMajorVersion, 3, eglContextMinorVersion, 3, eglContextProfileMask, eglContextCoreProfileBit, eglNone}
	if es {
		api, bit, contextAttribs = eglOpenGLESAPI, eglOpenGLES3Bit, []int32{eglContextMajorVersion, 3, eglContextMinorVersion, 0, eglNone}
	}
	eglBindAPI(api)
	// Configurations are for windows unless asked otherwise.
	configAttribs := []int32{eglRenderableType, bit, eglSurfaceType, eglPbufferBit, eglNone}
	var config uintptr
	var n int32
	if eglChooseConfig(display, &configAttribs[0], &config, 1, &n) == 0 || n == 0 {
		return nil, errors.New("gl: no EGL configuration for the API")
	}
	context := eglCreateContext(display, config, 0, &contextAttribs[0])
	if context == 0 {
		return nil, errors.New("gl: no context of the version")
	}
	off.context = context
	// Releasing a context current on this thread leaves the others' as
	// they are: current is per thread.
	undo = append(undo, func() {
		eglMakeCurrent(display, 0, 0, 0)
		eglDestroyContext(display, context)
	})
	if eglMakeCurrent(display, 0, 0, context) == 0 {
		return nil, errors.New("gl: cannot make the context current without a surface")
	}
	if err := load(); err != nil {
		return nil, err
	}
	off.tex = newTexture(glRGBA8, glRGBA, w, h, nil, 0)
	glGenFramebuffers(1, &off.fb)
	glBindFramebuffer(glFramebuffer, off.fb)
	glFramebufferTexture2D(glFramebuffer, glColorAttachment0, glTexture2D, off.tex, 0)
	undo = append(undo, func() {
		if off.r != nil {
			off.r.Release()
		}
		glBindFramebuffer(glFramebuffer, 0)
		glDeleteFramebuffers(1, &off.fb)
		glDeleteTextures(1, &off.tex)
	})
	if status := glCheckFramebufferState(glFramebuffer); status != glFramebufferDone {
		return nil, fmt.Errorf("gl: framebuffer incomplete: %#x", status)
	}
	return off, nil
}

// Info names the context's version and renderer.
func (o *Offscreen) Info() string {
	return goString(glGetString(glVersion)) + ", " + goString(glGetString(glRenderer))
}

// Render draws s, which must be as large as the framebuffer, with a
// renderer of its own, and returns its pixels as the CPU renderer draws
// them: premultiplied BGRA rows from the top.
func (o *Offscreen) Render(s *scene.Scene) ([]byte, error) {
	if o.r == nil {
		r, err := New()
		if err != nil {
			return nil, err
		}
		o.r = r
	}
	if err := o.r.Render(s); err != nil {
		return nil, err
	}
	return ReadFramebuffer(o.w, o.h)
}

// Draw draws s, which must be as large as the framebuffer, with a
// renderer of its own, without reading it back: Read does, for videos
// (plugins/render), which build the next frame meanwhile.
func (o *Offscreen) Draw(s *scene.Scene) error {
	if o.r == nil {
		r, err := New()
		if err != nil {
			return err
		}
		o.r = r
	}
	if err := o.r.Render(s); err != nil {
		return err
	}
	return nil
}

// Read copies the framebuffer into dst, as the CPU renderer draws:
// premultiplied BGRA rows from the top, 4×width bytes each.
func (o *Offscreen) Read(dst []byte) error {
	n := 4 * o.w * o.h
	if len(dst) < n {
		return errors.New("gl: too small a buffer")
	}
	if len(o.scratch) != n {
		o.scratch = make([]byte, n)
	}
	glPixelStorei(glPackAlignment, 1)
	glReadPixels(0, 0, int32(o.w), int32(o.h), glRGBA, glUnsignedByte, unsafe.Pointer(&o.scratch[0]))
	glPixelStorei(glPackAlignment, 4)
	if e := glGetError(); e != glNoError {
		return fmt.Errorf("gl: reading the framebuffer failed (error %#x)", e)
	}
	// GL's rows go up, and its bytes are RGBA.
	row := 4 * o.w
	for y := range o.h {
		src, out := o.scratch[row*(o.h-1-y):][:row], dst[row*y:][:row]
		for x := 0; x < row; x += 4 {
			out[x], out[x+1], out[x+2], out[x+3] = src[x+2], src[x+1], src[x], src[x+3]
		}
	}
	return nil
}

// Release deletes the context and what it holds.
func (o *Offscreen) Release() { o.release() }
