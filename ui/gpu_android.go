//go:build android && cgo

package ui

import (
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/internal/gpu/gl"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/probe"
	"github.com/egoist/mygo/internal/scene"
)

// gpu_android.go is the Android window's GPU renderer: the same OpenGL ES 3
// renderer Linux runs, on the EGL context the Android backend made current,
// plus the swap EGL leaves to whoever drew.
//
// 🔴 What this file does NOT do is make a context. The backend
// (internal/android/backend) owns EGL: it makes the display, the config, the
// context and the window surface, and it makes the surface current on the
// pump goroutine — the one gl.New runs on, because every frame is drawn
// there. So the context this renderer draws in is already current, and all
// this has to do is ask for a renderer.
//
// That split is why both platforms can share 1200 lines of renderer
// (internal/gpu/gl/gl.go) and differ only in where the entry points come
// from (load_linux.go: libepoxy, load_android.go: eglGetProcAddress).
//
// Until this file existed, ui/gpu_other.go answered for Android with "no GPU
// renderer for this platform yet", the window fell through to drawing in
// memory (ui/window.go), and every frame copied the whole 1440x3168 frame
// from the CPU to the window: about 10 ms a frame, which is what made a
// flagship phone look slow.
func newGPURenderer(n platform.SurfaceNative) (gpuRenderer, error) {
	if n.View == 0 {
		// No ANativeWindow: the surface is not there yet, and the window
		// draws in memory until the next frame tries again.
		return nil, nil
	}
	r, err := gl.New()
	if err != nil {
		return nil, err
	}
	liveSurface.Store(&n.Present)
	return &androidGPU{gl: r, surface: n.Present}, nil
}

// liveSurface is the surface the window is drawing into, for a measurement
// that has to reach it from outside the frame path (the lab's /debug/set).
//
// 🔴 Why a package-level pointer and not a method on the window: the knob it
// serves is a measurement, and a measurement has to be settable without a
// rebuild — a rebuild is a different binary, and a number read from a
// different binary is not the same measurement. The lab has an HTTP endpoint
// already (cmd/lab) and no window handle on the goroutine that serves it, so
// the surface is published here, once, where the renderer is made.
//
// It is written on the main thread when the renderer is created and read
// from the lab's HTTP goroutine, so it is an atomic. The pointer never goes
// stale in a way that matters: a surface that is gone is one whose swap
// interval no longer affects anything.
var liveSurface atomic.Pointer[platform.Surface]

// SetSwapInterval sets how many vsyncs the window's present waits for, on a
// surface that has one (Android's EGL — see platform.SwapIntervalSurface).
// It reports whether a surface was there to take it, so a caller can tell a
// platform without the knob from one that silently ignored it.
//
// Exported for measurement: with the default interval the present span reads
// the frame's pacing rather than its work, and 0 separates the two.
func SetSwapInterval(n int) bool {
	s := liveSurface.Load()
	if s == nil {
		return false
	}
	si, ok := (*s).(platform.SwapIntervalSurface)
	if !ok {
		return false
	}
	si.SetSwapInterval(n)
	return true
}

// The renderer draws into the framebuffer the EGL surface binds, which is
// the default one — nothing here asks for a framebuffer of its own.
//
// androidGPU makes the swap that EGL leaves to the drawing code.
type androidGPU struct {
	gl *gl.Renderer
	// surface shows the frames, when the backend handed it over. EGL's own
	// eglSwapBuffers is reached through it (platform.FramePresenter).
	surface platform.Surface
}

// Render draws the frame and has it shown.
//
// The second half is the part EGL makes necessary: gl.Render leaves a
// finished frame in the surface's back buffer and stops there, because on
// Linux the GtkGLArea swaps when its render signal returns. Nothing swaps
// for an EGLSurface, so a renderer that does not ask is a renderer whose
// frames never reach the screen — the surface keeps showing whatever was
// swapped last, which for this app was the red-and-green "nothing has
// drawn" landmark PresentPixels paints.
func (g *androidGPU) Render(s *scene.Scene) error {
	// The two halves of present are timed apart: submitting the scene is the
	// only part that scales with what the frame draws (and the only part with
	// anything to optimize), while the swap is the driver's and the
	// compositor's. Timing them as one number is what made "present" look
	// like a single mysterious cost.
	//
	// Both go to the frame phases and to the probe: the phases are what a
	// frame's shape is read from, and the probe is what the same span is read
	// as when a renderer is hunted — gpu/submit beside the rasterizer's
	// render/*, under one switch.
	//
	// The probe's span is ended with the duration the phase already took,
	// rather than timed again: a second clock read around the same call would
	// be measuring it twice and reporting two numbers for one thing.
	start := time.Now()
	err := g.gl.Render(s)
	submitted := time.Since(start)
	probe.At("gpu/submit", submitted)
	if err != nil {
		return err
	}
	if !gpuNoSwap {
		if p, ok := g.surface.(platform.FramePresenter); ok {
			start = time.Now()
			p.SetPresented()
			swapped := time.Since(start)
			// 🔴 This number is a wait, not work, and it is a symptom of a
			// late frame rather than the cost of presenting one. It tracks
			// the interval since the previous frame: on Adreno 830 it reads
			// 7.9 ms after a 55 ms gap and 0.4 ms after a 19 ms one, and a
			// page with no input at all reads 23 ms between frames with
			// 0.42 ms here. A frame that arrives late leaves buffers queued
			// behind it and the next submit reads that queue as its waiting.
			//
			// It is not EGL's vsync wait, whatever it looks like, and the
			// experiment that says so is eglSwapInterval(0) (see
			// platform.SwapIntervalSurface): the interval at 0 left it
			// unchanged, p50 0.38 ms and 9% of frames over 4 ms against 8%
			// at 1. Nor is it a floor — a page that draws on time draws with
			// almost no wait here. And it is not a second cost beside
			// gpu/clear, which waits on the same swap from the other side;
			// the two are one wait split by the GPU work between them.
			probe.At("gpu/swap", swapped)
		}
	}
	return nil
}

func (g *androidGPU) Release() { g.gl.Release() }

// gpuNoSwap skips the swap, to measure what it costs. A measurement switch,
// edited by hand: there is no runtime one because nothing but a measurement
// wants it.
var gpuNoSwap = false
