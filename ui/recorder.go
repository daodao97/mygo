package ui

import (
	"math"
	"time"

	"github.com/egoist/mygo/internal/scene"
)

// A Recorder builds the frames of a view without a window, one after
// another at the times its caller gives, as the frames of a video are
// made (plugins/render). Each frame is built anew at its own time, which
// the view's clock reads: what moves with time (Animate, Spring, a
// Transition, After) is where it would be on screen at that time, however
// long a frame takes to make, and the same frames come every time.
//
// A Recorder is not safe for concurrent use: one goroutine makes its
// frames. Recorders on different goroutines run in parallel, each with a
// text system of its own, as Testers do.
type Recorder struct {
	rt *engine
	h  *recordHost
	// start is the time of the first frame, and now the time of the
	// frame being made.
	start, now time.Time
	closed     bool
}

// RecorderOptions describe the window a Recorder's view is built in.
type RecorderOptions struct {
	// Width and Height are the size of the frames, in device pixels.
	Width, Height int
	// Scale is how many device pixels a DIP is, 1 when zero: the view
	// lays out in Width/Scale × Height/Scale DIPs.
	Scale float32
	// Dark builds the view in the dark appearance, and Preferences with
	// the desktop's settings that controls follow.
	Dark        bool
	Preferences Preferences
	// Locale is the language the view shows (Context.Locale), "" for the
	// system's default, and Direction lays the view out right to left
	// when it is RightToLeft.
	Locale    string
	Direction Direction
	// FrameRate is how many frames a second the Recorder is asked for,
	// which the view reads as the display's refresh rate: 60 when zero.
	FrameRate float64
}

// recordHost is the window of a Recorder: a Tester's, whose frames are
// kept for the caller to draw rather than drawn on the CPU.
type recordHost struct{ *headless }

func (h recordHost) present(s *scene.Scene) { h.last = s }

// recordEpoch is the time of a Recorder's first frame: the same for every
// recording, for the same frames.
var recordEpoch = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

// NewRecorder returns a Recorder of view, in frames of the size o gives.
func coreNewRecorder(view func(c *context), o RecorderOptions) *Recorder {
	if o.Width <= 0 || o.Height <= 0 {
		panic("ui: a Recorder's frames must have a size")
	}
	scale := o.Scale
	if scale == 0 {
		scale = 1
	}
	if !(scale > 0) || math.IsInf(float64(scale), 0) {
		panic("ui: a Recorder's Scale must be positive")
	}
	hz := float32(o.FrameRate)
	if hz <= 0 {
		hz = 60
	}
	h := &headless{scale: scale, hz: hz, prefs: platformPreferences(o.Preferences), dark: o.Dark,
		lang: o.Locale, rtl: o.Direction == RightToLeft}
	h.w, h.h = dipsFor(o.Width, scale), dipsFor(o.Height, scale)
	// Frames come as the Recorder asks for them, not at a display's
	// refresh: the engine never waits for one to pace what moves.
	h.atOnce = true
	r := &Recorder{h: &recordHost{h}, start: recordEpoch, now: recordEpoch}
	r.rt = newRuntime(view, r.h)
	if o.Locale != "" {
		r.rt.text.SetLanguage(o.Locale)
	}
	r.rt.clock = r.Now
	h.clock = r.Now
	return r
}

// dipsFor returns the DIPs that make px device pixels at scale, as the
// engine rounds them up (engine.pixels): px/scale, or just below it where
// float32 would make one pixel more.
func dipsFor(px int, scale float32) float32 {
	d := float32(px) / scale
	for int(math.Ceil(float64(d*scale))) > px {
		d = math.Nextafter32(d, 0)
	}
	return d
}

// Now returns the time of the frame being made, or of the last one: the
// view's clock.
func (r *Recorder) Now() time.Time { return r.now }

// Frame builds, lays out and paints the frame at time at from the first
// frame's, and returns it for a renderer to draw: it stays as it is until
// the next call. Times go forward from frame to frame, as a window's do;
// one earlier than the last frame's is taken as that time.
//
// A frame that asks to be built again at once, as a view that measures
// what it built and builds again (Context.Invalidate), is built again at
// the same time, a few times at most, so that each frame shows what it
// settles on.
func (r *Recorder) Frame(at time.Duration) *scene.Scene {
	if r.closed {
		panic("ui: Recorder used after Close")
	}
	if t := r.start.Add(at); t.After(r.now) {
		r.now = t
	}
	h := r.h.headless
	// Every frame is built anew, at its time: the timers of After and
	// Painter.After need not fire.
	h.afterAt, h.afterFrom, h.repaintAt = time.Time{}, time.Time{}, time.Time{}
	r.rt.changed()
	const passes = 8
	for range passes {
		h.requested.Store(false)
		h.invalidated.Store(false)
		r.runLater()
		r.rt.runFrame()
		if !h.invalidated.Load() && len(h.later) == 0 {
			break
		}
	}
	h.requested.Store(false)
	return h.last
}

// runLater runs what the view asked to run after the frame.
func (r *Recorder) runLater() {
	h := r.h.headless
	for len(h.later) > 0 {
		fn := h.later[0]
		h.later = h.later[1:]
		fn()
	}
}

// Size returns the size of the frames in device pixels.
func (r *Recorder) Size() (w, h int) { return r.rt.pixels(r.h.w, r.h.h, r.h.scale) }

// Close drops what the view keeps, its states and Locals, and the last
// frame.
func (r *Recorder) Close() {
	if r.closed {
		return
	}
	r.closed = true
	r.rt.close()
	r.h.last = nil
}

var _ host = recordHost{}
