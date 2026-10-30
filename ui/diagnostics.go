package ui

import (
	"fmt"
	"log"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// maxWarnings is how many warnings the inspector keeps, the latest.
const maxWarnings = 50

// duplicateKey reports an element given the key of another element of
// the pass under the same parent: they have one ID, and so share one
// state, so that the pointer, the focus and scrolling meant for one go to
// the other. A Tester panics (engine.strict), so that tests fail where the
// key was given; apps log it once, and the inspector lists it.
func (rt *engine) duplicateKey(id uint64, k any) {
	msg := fmt.Sprintf("ui: two elements under one parent have the key %#v, and share one state: give each a key of its own", k)
	if at := callSite(); at != "" {
		msg += " (" + at + ")"
	}
	if rt.strict {
		panic(msg)
	}
	if rt.dupKeys == nil {
		rt.dupKeys = map[uint64]bool{}
	}
	if rt.dupKeys[id] {
		return
	}
	rt.dupKeys[id] = true
	log.Print(msg)
	rt.warn(msg)
}

// warn notes a warning for the inspector.
func (rt *engine) warn(msg string) {
	if len(rt.warnings) == maxWarnings {
		copy(rt.warnings, rt.warnings[1:])
		rt.warnings = rt.warnings[:maxWarnings-1]
	}
	rt.warnings = append(rt.warnings, msg)
	rt.insp.changed = true
}

// uiPackage prefixes the names of the functions of package ui.
const uiPackage = "github.com/egoist/mygo/ui."

// callSite returns the file and line of the innermost call outside package
// ui on the stack: the app's code that built an element, or "".
func callSite() string {
	var pcs [32]uintptr
	n := runtime.Callers(2, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		if f.Function != "" && !strings.HasPrefix(f.Function, uiPackage) && !strings.HasPrefix(f.Function, "runtime.") && !strings.HasPrefix(f.Function, "testing.") {
			return f.File + ":" + strconv.Itoa(f.Line)
		}
		if !more {
			return ""
		}
	}
}

// What an app can read about how the engine is doing: the mean of the
// latest frames, and the heap the Go runtime holds. MYGO_FRAME_STATS logs
// frames but writes to stderr and is off unless set; these readings cost a
// lock and a ReadMemStats.

// frameTiming tells that an app has read the frame times (FrameAverage,
// FrameReading), and frames are timed from then on. Until then a frame
// neither reads the clock nor takes the lock: most apps never read them,
// and the smallest frame is under a microsecond.
var frameTiming atomic.Bool

// diagFrames is how many of the latest frames FrameAverage covers.
const diagFrames = 120

// frameTimes are how long the latest frames took, in a ring of the last
// diagFrames, with how many the process drew in all. The frames of every
// window go in the same ring: an app draws one window's frames at a time,
// and a reading that mixed two windows says what the app spent, which is
// the honest answer for a heads-up display of the process.
var frameTimes struct {
	mu       sync.Mutex
	ring     [diagFrames]time.Duration
	next     int
	total    uint64
	repaints uint64
}

// recordFrameTime notes how long the frame that just ended took, called by
// engine.endFrame (runtime.go). repaint is true for a frame that painted
// again without rebuilding, which a display shows apart from the rest: those
// frames are cheap and a run of them is why a rate can look low.
func recordFrameTime(d time.Duration, repaint bool) {
	frameTimes.mu.Lock()
	frameTimes.ring[frameTimes.next] = d
	frameTimes.next = (frameTimes.next + 1) % diagFrames
	frameTimes.total++
	if repaint {
		frameTimes.repaints++
	}
	frameTimes.mu.Unlock()
}

// FrameAverage returns the mean time the latest frames took, over the last
// diagFrames of them, and 0 before the process has drawn one. It is the
// reading a heads-up display shows, in whatever unit that display wants:
//
//	ms := float64(ui.FrameAverage().Microseconds()) / 1000
//
// The frames a window only painted again count too (engine.repaintFrame),
// so a window that repaints as drawings move reports those frames as well.
//
// Frames are timed from the first call to FrameAverage or FrameReading on,
// so the first reading is 0: a display reading every frame shows a number
// from its next one.
func FrameAverage() time.Duration {
	frameTiming.Store(true)
	frameTimes.mu.Lock()
	defer frameTimes.mu.Unlock()
	n := int(frameTimes.total)
	if n > diagFrames {
		n = diagFrames
	}
	if n == 0 {
		return 0
	}
	var sum time.Duration
	for i := 0; i < n; i++ {
		sum += frameTimes.ring[i]
	}
	return sum / time.Duration(n)
}

// HeapBytes returns the bytes of heap the Go runtime holds live now, which
// is what a heads-up display shows: MemStats.HeapAlloc, the bytes of
// allocated heap objects.
//
// ReadMemStats stops the world to read it. The runtime metric
// /memory/classes/heap/objects:bytes is the same number for free, but as of
// the last update of the runtime's counters rather than as of now, so it
// can lag the allocations made since; HeapAlloc cannot.
func HeapBytes() int64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return int64(m.HeapAlloc)
}

// A reading, for a heads-up display: not just the mean, because the mean is
// the one number that hides what a display exists to show. A run of 120
// frames at 3 ms with one at 300 ms reads as 5.5 ms mean and looks fine,
// while every one of those users saw the hitch. The worst recent frame is
// the number that catches it, and the last frame is the one that says
// whether the app is moving at all.
//
// Frames counts what the process drew since it started, so a display can
// tell "12 fps because the app is idle and repainting nothing" from "12 fps
// because every frame costs 80 ms" — the first is a number that should not
// worry anyone.

// Reading is what FrameReading returns: how the engine's frames are going.
type Reading struct {
	// Last is how long the frame that ended most recently took, or 0 if
	// none has: a window that never drew a frame reports 0.
	Last time.Duration
	// Mean is the mean of the latest diagFrames ended frames, as
	// FrameAverage returns.
	Mean time.Duration
	// Worst is the longest of those same frames: the hitch the mean hides.
	Worst time.Duration
	// FPS is how many frames a second the Mean pace amounts to, and 0 when
	// nothing has been drawn. It is derived, not measured over a wall-clock
	// second: a window whose frames are user-driven draws none while it is
	// idle, and a wall-clock rate would report the idle as slowness.
	FPS float64
	// PacePerSecond is 1/second of the Mean frame, and 0 when nothing has
	// been drawn.
	//
	// 🔴 It is a pace, not an observed rate, and the name says so because the
	// old one did not. It answers "if frames kept coming back to back at this
	// pace, how many would a second hold" — which is `1e9 / Mean` and no
	// more. It is NOT how many frames the window drew in a second: a page
	// that draws nothing while it is still has a Mean of 2.5 ms and this
	// reads 400, while the frames on screen came 63 ms apart. Read as a rate
	// it says the app runs at 400 fps on a 120 Hz panel, which is impossible
	// and was believed for a while.
	//
	// For a rate, divide the count of frames by the wall-clock span they came
	// in — the frame ring's `at` is what that needs. This field is for
	// answering "how much does one frame cost", where the reciprocal is the
	// honest shape.
	PacePerSecond float64
	// Frames is how many frames the process has drawn in all.
	Frames uint64
	// Repaints is how many of them were frames that painted again without
	// rebuilding (engine.repaintFrame): a window showing a drawing move.
	Repaints uint64
}

// FrameReading returns the current reading. It takes one lock; a display
// reading it once a frame or once a second costs nothing. As with
// FrameAverage, frames are timed from the first reading on.
func FrameReading() Reading {
	frameTiming.Store(true)
	frameTimes.mu.Lock()
	defer frameTimes.mu.Unlock()
	n := int(frameTimes.total)
	if n > diagFrames {
		n = diagFrames
	}
	var r Reading
	r.Frames, r.Repaints = frameTimes.total, frameTimes.repaints
	if n == 0 {
		return r
	}
	var sum, worst time.Duration
	for i := 0; i < n; i++ {
		d := frameTimes.ring[i]
		sum += d
		if d > worst {
			worst = d
		}
	}
	r.Mean = sum / time.Duration(n)
	r.Worst = worst
	// The newest frame is the one before next, which is where the ring
	// writes next.
	r.Last = frameTimes.ring[(frameTimes.next-1+diagFrames)%diagFrames]
	if r.Mean > 0 {
		r.FPS = 1e9 / float64(r.Mean.Nanoseconds())
		r.PacePerSecond = 1e9 / float64(r.Mean.Nanoseconds())
	}
	return r
}

// HeapReading returns what HeapBytes does plus what the runtime holds that
// the heap alone does not say: the bytes it has taken from the OS with no
// way to give back on some platforms, and what the collector has run.
//
// A display that shows HeapAlloc alone reads as a small number that moves
// little, while the pool under it only ever grows — the two are the same
// story told by the two numbers a wasm build in particular needs, where the
// linear memory is the pool.
type HeapReading struct {
	// Alloc is MemStats.HeapAlloc: the live heap objects, as HeapBytes
	// returns.
	Alloc int64
	// Sys is MemStats.Sys: the bytes the runtime has taken from the OS for
	// every purpose. On wasm this is what the linear memory holds.
	Sys int64
	// TotalAlloc is MemStats.TotalAlloc: every byte allocated since the
	// start, freed or not. A rising TotalAlloc with a flat Alloc is churn,
	// not a leak.
	TotalAlloc int64
	// NumGC and PauseTotal are the collections and the time they took.
	NumGC      uint32
	PauseTotal time.Duration
	// Goroutines is how many goroutines run.
	Goroutines int

	// The heap, split the way the runtime splits it. HeapSys is the arena
	// the runtime took from the OS; HeapInuse is the part of it holding
	// spans, which is the memory the process is really paying for; HeapIdle
	// is the part it holds and is not using, which on a platform that can
	// give pages back (Linux, Windows) is not residence but on one that
	// cannot (wasm) is. HeapReleased is what it has given back.
	//
	// A display that shows Alloc alone reads as small and quiet: Alloc is
	// live objects, and a program that allocates and frees a lot has a
	// small Alloc over a large arena. The split is what says which.
	HeapSys      int64
	HeapInuse    int64
	HeapIdle     int64
	HeapReleased int64
	HeapObjects  uint64
	StackSys     int64
	// MSpanSys, MCacheSys, BuckHashSys, GCSys and OtherSys are the
	// runtime's own bookkeeping, all of it small except GCSys's write
	// barriers and OtherSys's profile buffers.
	MSpanSys, MCacheSys, BuckHashSys, GCSys, OtherSys int64
}

// HeapReadingNow returns the current heap reading. ReadMemStats stops the
// world, so calling this every frame on a large heap is itself a cost; once
// a second is what a display wants.
func HeapReadingNow() HeapReading {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return HeapReading{
		Alloc:      int64(m.HeapAlloc),
		Sys:        int64(m.Sys),
		TotalAlloc: int64(m.TotalAlloc),
		NumGC:      m.NumGC,
		PauseTotal: time.Duration(m.PauseTotalNs),
		Goroutines: runtime.NumGoroutine(),

		HeapSys:      int64(m.HeapSys),
		HeapInuse:    int64(m.HeapInuse),
		HeapIdle:     int64(m.HeapIdle),
		HeapReleased: int64(m.HeapReleased),
		HeapObjects:  m.HeapObjects,
		StackSys:     int64(m.StackSys),
		MSpanSys:     int64(m.MSpanSys),
		MCacheSys:    int64(m.MCacheSys),
		BuckHashSys:  int64(m.BuckHashSys),
		GCSys:        int64(m.GCSys),
		OtherSys:     int64(m.OtherSys),
	}
}
