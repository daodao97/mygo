//go:build windows

package d3d11

import (
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/egoist/mygo/internal/pacing"
	"github.com/egoist/mygo/internal/scene"
	"github.com/egoist/mygo/internal/wait"
)

// TestUncappedSwapChain checks the swap chains of renderers made
// uncapped (MYGO_UNCAPPED): no frame latency object, at least three
// buffers, kept as the swap chain resizes, tearing allowed exactly where
// the system shows frames torn and the swap chain is the window's; and
// that frames following each other never wait for the display, whether
// they present or change nothing. The default, paced, keeps the object and
// no tearing. A frame of a new size begins a resize, whose frames present
// paced (uncapped.go): it waits for DWM within Render, and the frame after
// waits for nothing, unless Present, with DXGI_PRESENT_DO_NOT_WAIT, refused
// it for a refresh, as WARP on a CI runner does in 1 run of 300 to 1 of 50
// (DXGI_ERROR_WAS_STILL_DRAWING 2 to 13 times over 17 to 30 ms, the refresh
// 15.6 ms): the frame then waits to present (holdFrame), and the frame
// after waits for it.
func TestUncappedSwapChain(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer func(on bool) { pacing.Uncapped = on }(pacing.Uncapped)
	const scGetDesc1 = 18 // IDXGISwapChain1
	desc := func(r *Renderer) swapChainDesc1 {
		var d swapChainDesc1
		call(r.swapChain, scGetDesc1, uintptr(unsafe.Pointer(&d)))
		return d
	}
	for _, uncapped := range []bool{false, true} {
		pacing.Uncapped = uncapped
		for _, composed := range []bool{false, true} {
			newRenderer := New
			if composed {
				newRenderer = NewComposed
			}
			r, err := newRenderer(hiddenWindow(t, 200, 150))
			if err != nil {
				t.Skip("no Direct3D 11:", err)
			}
			posted := make(chan func(), 16)
			r.SetPost(func(fn func()) { posted <- fn })
			bg := scene.Color{R: 24, G: 24, B: 27, A: 255}
			if err := r.Render(&scene.Scene{Width: 200, Height: 150, Background: bg}); err != nil {
				r.Release()
				t.Fatal(err)
			}
			d := desc(r)
			waitable := d.Flags&swapChainWaitable != 0
			tearing := d.Flags&swapChainTearing != 0
			if !uncapped {
				if tearing || r.tearing || r.uncapped {
					t.Errorf("composed %v, paced: tearing %v (flag %v), uncapped %v", composed, r.tearing, tearing, r.uncapped)
				}
				if r.swapChain2 != 0 && (!waitable || r.lat.handle == 0) {
					t.Errorf("composed %v, paced: waitable %v, latency object %#x", composed, waitable, r.lat.handle)
				}
				r.Release()
				continue
			}
			wantTearing := !composed && tearingSupported(r.factory)
			if waitable || r.lat.handle != 0 || tearing != wantTearing || r.tearing != wantTearing || d.BufferCount < uncappedBuffers {
				t.Errorf("composed %v, uncapped: waitable %v, latency object %#x, tearing %v (renderer %v), want %v, %d buffers",
					composed, waitable, r.lat.handle, tearing, r.tearing, wantTearing, d.BufferCount)
			}
			if f := r.presentFlags(0); (f == presentTearing) != wantTearing || r.presentFlags(1) != 0 {
				t.Errorf("composed %v, uncapped: present flags %#x at once, %#x after a refresh", composed, f, r.presentFlags(1))
			}
			// Frames follow each other at once: changed, unchanged, and of
			// a new size, which resizes the swap chain with its flags.
			for i, s := range []*scene.Scene{
				{Width: 200, Height: 150, Background: scene.Color{R: 255, G: 255, B: 255, A: 255}},
				{Width: 200, Height: 150, Background: scene.Color{R: 255, G: 255, B: 255, A: 255}},
				{Width: 260, Height: 180, Background: bg},
			} {
				if !r.Ready(func() { t.Error("woken: a frame waited") }) {
					t.Fatalf("composed %v, uncapped: frame %d waits", composed, i)
				}
				if err := r.Render(s); err != nil {
					r.Release()
					t.Fatal(err)
				}
				if held := r.refreshing && i == 2 && r.unpresented && r.refused > 0; r.pending || r.refreshing && !held || len(posted) != 0 {
					t.Errorf("composed %v, uncapped, frame %d: pending %v, awaiting a refresh %v, %d posted (%s)", composed, i, r.pending, r.refreshing, len(posted), r.FrameInfo())
				} else if held {
					presentsHeld(t, r, posted)
				}
			}
			if d := desc(r); d.Flags != uint32(r.swapChainFlags()) || d.BufferCount < uncappedBuffers {
				t.Errorf("composed %v, uncapped, resized: flags %#x, want %#x, %d buffers", composed, d.Flags, r.swapChainFlags(), d.BufferCount)
			}
			r.Release()
		}
	}
}

// presentsHeld waits for the frame Present refused (holdFrame) to present,
// the frame after waiting for it, and checks that it did.
func presentsHeld(t *testing.T, r *Renderer, posted chan func()) {
	t.Helper()
	woke := false
	if r.Ready(func() { woke = true }) {
		t.Fatalf("composed %v: the frame after one waiting to present drew at once (%s)", r.composed, r.FrameInfo())
	}
	for !woke {
		wait.For(t, "the composition a frame waits for", posted)()
	}
	if r.unpresented || r.aside != "" || r.failed != nil || !r.Ready(func() { t.Error("woken: a frame waited") }) {
		t.Errorf("composed %v: once the frame after a held one could draw: unpresented %v, aside %q, failed %v; want presented", r.composed, r.unpresented, r.aside, r.failed)
	}
}

// TestUncappedResizePaced checks that uncapped (MYGO_UNCAPPED), frames
// present paced while the window changes size, as they would without it:
// a frame of a new size without tearing, the others without tearing too,
// each waiting for the composition that shows it, one that changed nothing
// for the next refresh, and none taking long (uncapped.go); and that once
// the swap chain settled, frames present uncapped again. Uncapped frames
// flooding DWM as it resized the window had a present take 2 s on a
// Radeon 780M; WARP, without a display, may not show that, but the frames
// are those of a live resize, the size changing every other frame, and no
// frame may take near as long. Real DXGI takes DXGI_PRESENT_DO_NOT_WAIT,
// and a frame of a new size that it refused for a refresh, as WARP on a
// busy runner may, waits to present (holdFrame) and presents before the
// frame after draws.
func TestUncappedResizePaced(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer func(on bool) { pacing.Uncapped = on }(pacing.Uncapped)
	defer func(d time.Duration) { settleDelay = d }(settleDelay)
	var composed atomic.Int32
	defer func(f func(_, _ time.Duration)) { awaitRefresh = f }(awaitRefresh)
	awaitRefresh = func(_, _ time.Duration) { composed.Add(1) }
	// Real DXGI presents, each call's flags and result recorded.
	var calls []presentResult
	defer func(f func(_, _, _ uintptr, _ *presentParameters) uintptr) { presentCall = f }(presentCall)
	present := presentCall
	presentCall = func(sc, sync, flags uintptr, params *presentParameters) uintptr {
		hr := present(sc, sync, flags, params)
		calls = append(calls, presentResult{flags, uint32(hr)})
		return hr
	}
	// A frame waiting to present presents once DXGI takes it, which it
	// does; TestUncappedHeldFrameGivenUp has one held maxHeld present
	// on another goroutine.
	defer func(d time.Duration) { maxHeld = d }(maxHeld)
	maxHeld = wait.Guard
	pacing.Uncapped = true
	// Frames come far sooner than half of it after the last change, so the
	// swap chain settles only once the size stops changing.
	settleDelay = time.Second
	for _, newRenderer := range []func(uintptr) (*Renderer, error){New, NewComposed} {
		r, err := newRenderer(hiddenWindow(t, 200, 150))
		if err != nil {
			t.Skip("no Direct3D 11:", err)
		}
		uncappedResize(t, r, &composed, &calls)
		r.Release()
	}
}

// presentResult is a call to Present: its flags and what it returned.
type presentResult struct {
	flags uintptr
	hr    uint32
}

// lastPresent returns the last of calls, none for none.
func lastPresent(calls []presentResult) presentResult {
	if len(calls) == 0 {
		return presentResult{}
	}
	return calls[len(calls)-1]
}

// presentedNoWait reports whether the call presented with
// DXGI_PRESENT_DO_NOT_WAIT, and stillDrawing whether DXGI refused it.
func (c presentResult) presentedNoWait() bool {
	return c.flags&presentDoNotWait != 0 && !failed(uintptr(c.hr))
}

func (c presentResult) stillDrawing() bool {
	return c.flags&presentDoNotWait != 0 && c.hr == dxgiErrorWasStillDrawing
}

func uncappedResize(t *testing.T, r *Renderer, composed *atomic.Int32, calls *[]presentResult) {
	t.Helper()
	kind := "window"
	if r.composed {
		kind = "composition"
	}
	posted := make(chan func(), 16)
	r.SetPost(func(f func()) { posted <- f })
	// ready has the next frame wait until it can draw, as the window host
	// does, and reports whether it waited. A frame waiting to present is
	// tried again at each composition, which comes at once here: DXGI
	// takes it long before the guard.
	ready := func() bool {
		t.Helper()
		woke := false
		if r.Ready(func() { woke = true }) {
			return false
		}
		for end := time.Now().Add(wait.Guard); !woke; {
			if time.Now().After(end) {
				t.Fatalf("%s: the frame after one waiting to present never drew (waited %v)", kind, wait.Guard)
			}
			wait.For(t, kind+": the composition a frame waits for", posted)()
		}
		return true
	}
	const tooLong = 200 * time.Millisecond
	colors := []scene.Color{{R: 24, G: 24, B: 27, A: 255}, {R: 250, G: 250, B: 250, A: 255}}
	n := 0
	// frame draws and presents a frame of w×h, which changes what it starts
	// with and the background unless same, and reports whether it waited to
	// start: a swap chain for composition shows no background, and a frame
	// changing it alone would present nothing.
	frame := func(w, h int, same bool) bool {
		t.Helper()
		waited := ready()
		if !same {
			n++
		}
		start := time.Now()
		if err := r.Render(&scene.Scene{Width: w, Height: h, Clear: colors[n%2], Background: colors[n%2]}); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d > tooLong {
			t.Errorf("%s: a %dx%d frame took %v, resizing %v", kind, w, h, d, r.resizing)
		}
		return waited
	}
	wantTearing := !r.composed && tearingSupported(r.factory)
	frame(200, 150, false)
	if r.swapChain2 == 0 {
		t.Skip("no IDXGISwapChain2: the swap chain resizes whole at each size")
	}
	for range 3 {
		if frame(200, 150, false) || r.refreshing || r.resizing {
			t.Fatalf("%s: before the resize, a frame waited (refreshing %v, resizing %v)", kind, r.refreshing, r.resizing)
		}
	}
	if (r.presentFlags(0) == presentTearing) != wantTearing {
		t.Fatalf("%s: before the resize, present flags %#x", kind, r.presentFlags(0))
	}
	// A live resize: growing, shrinking within the buffers and growing past
	// them, the size changing every other frame.
	w, h := 200, 150
	held := 0
	for i := range 24 {
		switch {
		case i%6 == 5:
			w, h = w-11, h-5
		default:
			w, h = w+9, h+6
		}
		frame(w, h, false)
		if r.noWaitRefused {
			t.Fatalf("%s: the swap chain took no DXGI_PRESENT_DO_NOT_WAIT", kind)
		}
		if !r.resizing || r.presentFlags(0) != presentDoNotWait || r.pending {
			t.Fatalf("%s: a frame of a new size, %dx%d: resizing %v, present flags %#x, pending %v; want resizing, no tearing, DXGI_PRESENT_DO_NOT_WAIT, none pending",
				kind, w, h, r.resizing, r.presentFlags(0), r.pending)
		}
		info := r.FrameInfo()
		if !strings.Contains(info, "paced while resizing") {
			t.Fatalf("%s: a frame of a new size says %q", kind, info)
		}
		if r.unpresented {
			// DXGI refused it for a refresh: the frame waits to present,
			// the main thread back to the window's messages, and presents
			// before the frame after draws.
			held++
			if last := lastPresent(*calls); !r.refreshing || !last.stillDrawing() || !strings.Contains(info, "presenting once DXGI can") {
				t.Fatalf("%s: a frame of a new size, %dx%d, waiting to present: refreshing %v, last present %#x (flags %#x), says %q; want waiting for the composition, refused with DXGI_PRESENT_DO_NOT_WAIT",
					kind, w, h, r.refreshing, last.hr, last.flags, info)
			}
			ready()
			if last := lastPresent(*calls); r.unpresented || !last.presentedNoWait() {
				t.Fatalf("%s: a frame of a new size, %dx%d, waiting to present: unpresented %v, last present %#x (flags %#x) once the frame after can draw; want presented with DXGI_PRESENT_DO_NOT_WAIT",
					kind, w, h, r.unpresented, last.hr, last.flags)
			}
		} else if last := lastPresent(*calls); r.refreshing || !last.presentedNoWait() {
			t.Fatalf("%s: a frame of a new size, %dx%d: refreshing %v, last present %#x (flags %#x); want no wait, presented with DXGI_PRESENT_DO_NOT_WAIT",
				kind, w, h, r.refreshing, last.hr, last.flags)
		}
		before := composed.Load()
		frame(w, h, false)
		if !r.refreshing {
			t.Fatalf("%s: a %dx%d frame while resizing does not wait for its composition", kind, w, h)
		}
		if r.held {
			// It changes nothing, and so presents nothing.
			if !frame(w, h, true) {
				t.Fatalf("%s: a frame after one presented while resizing did not wait", kind)
			}
			if r.redrew != redrewNothing || !r.refreshing {
				t.Fatalf("%s: a frame that changed nothing while resizing: redrew %v, refreshing %v", kind, r.redrew, r.refreshing)
			}
		}
		ready()
		if composed.Load() == before {
			t.Fatalf("%s: frames while resizing waited for no composition", kind)
		}
	}
	if held > 0 {
		t.Logf("%s: %d of 24 frames of a new size waited to present, DXGI refusing them for a refresh", kind, held)
	}
	// Once the size stopped changing, a frame settles the swap chain, and
	// frames present uncapped again.
	time.Sleep(settleDelay/2 + 10*time.Millisecond)
	frame(w, h, false)
	if r.resizing || r.bw != w || r.bh != h {
		t.Fatalf("%s: settled: resizing %v, buffers %dx%d; want %dx%d", kind, r.resizing, r.bw, r.bh, w, h)
	}
	for range 3 {
		if frame(w, h, false) || r.refreshing {
			t.Fatalf("%s: after the resize, a frame waited (refreshing %v)", kind, r.refreshing)
		}
	}
	if (r.presentFlags(0) == presentTearing) != wantTearing || strings.Contains(r.FrameInfo(), "paced") {
		t.Fatalf("%s: after the resize, present flags %#x, frame %q", kind, r.presentFlags(0), r.FrameInfo())
	}
	if r.noWaitRefused {
		t.Errorf("%s: the swap chain took no DXGI_PRESENT_DO_NOT_WAIT", kind)
	}
}

// TestUncappedResizeWhileDWMStalls checks that uncapped (MYGO_UNCAPPED),
// no frame of a live resize waits long for DWM on the main thread, whose
// messages DWM may wait on, though DWM composes nothing for as long as the
// test runs: a frame of a new size waits for it at most two refreshes
// (waitShown), DwmFlush called on another thread, and the others not at
// all, from the uncapped frames before the resize, through its first frame
// and its last change, to the uncapped frames once the swap chain settled.
// A resize the app made with SetWindowPos had a present take 2 s on a
// Radeon 780M, at its first frame or after its last change, while the
// frame of a new size called DwmFlush on the main thread.
func TestUncappedResizeWhileDWMStalls(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer func(on bool) { pacing.Uncapped = on }(pacing.Uncapped)
	defer func(d time.Duration) { settleDelay = d }(settleDelay)
	pacing.Uncapped = true
	settleDelay = 400 * time.Millisecond
	// A frame DXGI refused waits to present for as long as it does here;
	// TestUncappedRefusedThroughSettle has one held maxHeld present on
	// another goroutine.
	defer func(d time.Duration) { maxHeld = d }(maxHeld)
	maxHeld = wait.Guard
	// A call to DwmFlush under way from another test returns first.
	if done := inFlightFlush(); done != nil {
		wait.For(t, "the window manager", done)
	}
	stalled := make(chan struct{})
	defer func(f func()) {
		close(stalled)
		// The call under way returns before DwmFlush is called again.
		for done := inFlightFlush(); done != nil; done = inFlightFlush() {
			<-done
		}
		dwmFlushCall = f
	}(dwmFlushCall)
	dwmFlushCall = func() { <-stalled }
	// Count the waits of frames of a new size, the only ones with no least.
	var shown atomic.Int32
	defer func(f func(_, _ time.Duration)) { awaitRefresh = f }(awaitRefresh)
	await := awaitRefresh
	awaitRefresh = func(least, most time.Duration) {
		if least == 0 {
			shown.Add(1)
		}
		await(least, most)
	}
	for _, newRenderer := range []func(uintptr) (*Renderer, error){New, NewComposed} {
		r, err := newRenderer(hiddenWindow(t, 200, 150))
		if err != nil {
			t.Skip("no Direct3D 11:", err)
		}
		resizeWhileDWMStalls(t, r, &shown)
		r.Release()
	}
}

// inFlightFlush returns what closes once the call to DwmFlush under way
// returns, nil for none.
func inFlightFlush() <-chan struct{} {
	dwmFlush.Lock()
	defer dwmFlush.Unlock()
	return dwmFlush.done
}

func resizeWhileDWMStalls(t *testing.T, r *Renderer, shown *atomic.Int32) {
	t.Helper()
	kind := "window"
	if r.composed {
		kind = "composition"
	}
	posted := make(chan func(), 16)
	r.SetPost(func(f func()) { posted <- f })
	// Two refreshes, the longest a frame waits, and time for a slow
	// machine to draw, far below the 2 s seen: WARP on a busy Windows
	// ARM64 runner has taken 235 ms for a frame of a new size.
	tooLong := 2*r.cost.refreshPeriod() + 500*time.Millisecond
	colors := []scene.Color{{R: 24, G: 24, B: 27, A: 255}, {R: 250, G: 250, B: 250, A: 255}}
	n := 0
	// frame draws a frame of w×h, changing the background, once it can, as
	// the window host has it wait: the waits for a composition end at their
	// bound.
	frame := func(w, h int) {
		t.Helper()
		woke := false
		if !r.Ready(func() { woke = true }) {
			for !woke {
				wait.For(t, kind+": the composition a frame waits for, or its bound", posted)()
			}
		}
		n++
		start := time.Now()
		if err := r.Render(&scene.Scene{Width: w, Height: h, Background: colors[n%2]}); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d > tooLong {
			t.Errorf("%s: a %dx%d frame took %v, resizing %v; want at most %v", kind, w, h, d, r.resizing, tooLong)
		}
	}
	frame(200, 150)
	if r.swapChain2 == 0 {
		t.Skip("no IDXGISwapChain2: the swap chain resizes whole at each size")
	}
	for range 10 {
		frame(200, 150) // uncapped, with tearing where the system has it
	}
	// Growing, past the buffers too, and shrinking within them.
	w, h := 200, 150
	for i := range 12 {
		if i%4 == 3 {
			w, h = w-13, h-7
		} else {
			w, h = w+17, h+9
		}
		before := shown.Load()
		frame(w, h)
		// It waits for DWM to show it once presented; one DXGI refused for
		// a refresh, as WARP on a busy runner may, waits to present
		// instead, and the frame after for it (holdFrame).
		want := before + 1
		if r.unpresented {
			want = before
		}
		if !r.resizing || shown.Load() != want {
			t.Fatalf("%s: a %dx%d frame of a new size: resizing %v, waiting to present %v, %d waits for DWM to show it; want resizing, %d",
				kind, w, h, r.resizing, r.unpresented, shown.Load()-before, want-before)
		}
		frame(w, h)
		frame(w, h)
	}
	// The frames after the last change, until one settles the swap chain.
	for end := time.Now().Add(settleDelay/2 + 20*time.Millisecond); time.Now().Before(end); {
		frame(w, h)
	}
	frame(w, h)
	if r.resizing || r.bw != w || r.bh != h {
		t.Fatalf("%s: settled: resizing %v, buffers %dx%d; want %dx%d", kind, r.resizing, r.bw, r.bh, w, h)
	}
	for range 10 {
		frame(w, h)
		if r.refreshing {
			t.Fatalf("%s: after the resize, a frame waits for a composition", kind)
		}
	}
}

// TestUncappedResizePresentRefused checks that uncapped (MYGO_UNCAPPED),
// while the window changes size, no frame waits in Present long on the
// main thread: frames present with DXGI_PRESENT_DO_NOT_WAIT, and one that
// Present refuses for a refresh (DXGI_ERROR_WAS_STILL_DRAWING) is left to
// present after the next composition, the frame after waiting for it, or
// gives way to a frame drawn meanwhile, as one of a new size within
// SetWindowPos (holdFrame). A resize the app made with SetWindowPos every
// 50 ms had a present of a frame redrawn in part take 2 s on a Radeon
// 780M, most likely waiting in Present for DWM to give a buffer back.
// Present here refuses for as long as the test says, as it would while DWM
// is held up, and only then; a frame waits for as long as it refuses
// (TestUncappedHeldFrameGivenUp bounds that). The paced default, and frames
// once the swap chain settled, present without the flag.
func TestUncappedResizePresentRefused(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer func(on bool) { pacing.Uncapped = on }(pacing.Uncapped)
	defer func(d time.Duration) { settleDelay = d }(settleDelay)
	// The swap chain settles only once the test stops changing the size.
	settleDelay = time.Second
	defer func(f func(_, _ time.Duration)) { awaitRefresh = f }(awaitRefresh)
	awaitRefresh = func(_, _ time.Duration) {}
	// A hidden window's frame latency object is not signaled: paced frames
	// wait for it no longer than this.
	defer func(ms uintptr) { latencyTimeout = ms }(latencyTimeout)
	latencyTimeout = 20
	// A frame waits to present for as long as Present refuses it.
	defer func(d time.Duration) { maxHeld = d }(maxHeld)
	maxHeld = wait.Guard
	var p refusingPresent
	defer func(f func(_, _, _ uintptr, _ *presentParameters) uintptr) { presentCall = f }(presentCall)
	presentCall = p.call(presentCall)
	for _, uncapped := range []bool{false, true} {
		pacing.Uncapped = uncapped
		for _, newRenderer := range []func(uintptr) (*Renderer, error){New, NewComposed} {
			r, err := newRenderer(hiddenWindow(t, 200, 150))
			if err != nil {
				t.Skip("no Direct3D 11:", err)
			}
			p = refusingPresent{}
			presentRefused(t, r, &p)
			r.Release()
		}
	}
}

// refusingPresent stands for Present: while refuse, it refuses presents
// with DXGI_PRESENT_DO_NOT_WAIT, counting them, and, given stall, waits
// that long in those without it, as DXGI would for DWM, counting them
// (waited); it counts those it took (presented); flags are those of every
// call. With untilWaits, it stops refusing once that many calls waited,
// ResizeBuffers too (resize), as DXGI on a Radeon 780M took no frame with
// the flag after a resize's last ones were refused, for seconds, until the
// app made a call that waited; waitedOnMain counts the calls that waited on
// main, the test's thread, and stalling those waiting now; acceptedAt is
// when it stopped refusing so, and takenAt when it took a frame after. The
// calls may come from another goroutine (goAside): fakeMu guards what they
// change.
type refusingPresent struct {
	refuse             bool
	stall              time.Duration
	refused, presented int
	waited             int
	flags              []uintptr
	untilWaits         int
	main               uintptr
	waitedOnMain       int
	stalling           int
	acceptedAt         time.Time
	takenAt            time.Time
}

var fakeMu sync.Mutex

// threadID returns the calling thread's ID.
var threadID = func() func() uintptr {
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId")
	return func() uintptr {
		id, _, _ := proc.Call()
		return id
	}
}()

// call returns what stands for Present, calling present for the calls it
// takes.
func (p *refusingPresent) call(present func(_, _, _ uintptr, _ *presentParameters) uintptr) func(_, _, _ uintptr, _ *presentParameters) uintptr {
	return func(sc, sync, flags uintptr, params *presentParameters) uintptr {
		fakeMu.Lock()
		p.flags = append(p.flags, flags)
		if flags&presentDoNotWait != 0 {
			if p.refuse {
				p.refused++
				fakeMu.Unlock()
				return dxgiErrorWasStillDrawing
			}
			// Only the test refuses: real DXGI, which may refuse too on a
			// busy runner (TestUncappedResizePaced), takes the frame as it
			// would without the flag.
			flags &^= presentDoNotWait
			fakeMu.Unlock()
		} else {
			fakeMu.Unlock()
			p.wait()
		}
		fakeMu.Lock()
		p.presented++
		if !p.acceptedAt.IsZero() && p.takenAt.IsZero() {
			p.takenAt = time.Now()
		}
		fakeMu.Unlock()
		return present(sc, sync, flags, params)
	}
}

// resize returns what stands for ResizeBuffers, which waits as Present
// without the flag does, calling resizeBuffers.
func (p *refusingPresent) resize(resizeBuffers func(_, _, _, _, _ uintptr) uintptr) func(_, _, _, _, _ uintptr) uintptr {
	return func(sc, n, w, h, flags uintptr) uintptr {
		p.wait()
		return resizeBuffers(sc, n, w, h, flags)
	}
}

// wait waits stall while Present refuses frames, and stops refusing once
// untilWaits calls waited.
func (p *refusingPresent) wait() {
	fakeMu.Lock()
	if !p.refuse || p.stall == 0 {
		fakeMu.Unlock()
		return
	}
	p.waited++
	if p.main != 0 && threadID() == p.main {
		p.waitedOnMain++
	}
	stall := p.stall
	p.stalling++
	fakeMu.Unlock()
	time.Sleep(stall)
	fakeMu.Lock()
	p.stalling--
	if p.untilWaits > 0 && p.waited >= p.untilWaits {
		p.refuse, p.acceptedAt, p.takenAt = false, time.Now(), time.Time{}
	}
	fakeMu.Unlock()
}

// state returns a copy of p, taken while no call changes it.
func (p *refusingPresent) state() refusingPresent {
	fakeMu.Lock()
	defer fakeMu.Unlock()
	c := *p
	c.flags = append([]uintptr(nil), p.flags...)
	return c
}

// noWait reports whether a call to Present since the from'th had
// DXGI_PRESENT_DO_NOT_WAIT.
func (p *refusingPresent) noWait(from int) bool {
	for _, f := range p.flags[from:] {
		if f&presentDoNotWait != 0 {
			return true
		}
	}
	return false
}

func presentRefused(t *testing.T, r *Renderer, p *refusingPresent) {
	t.Helper()
	kind := "window"
	if r.composed {
		kind = "composition"
	}
	if !r.uncapped {
		kind += ", paced"
	}
	posted := make(chan func(), 16)
	r.SetPost(func(f func()) { posted <- f })
	// A refresh, which Present is tried for, and time for a slow machine
	// to draw, far below the 2 s seen.
	tooLong := r.cost.refreshPeriod() + 200*time.Millisecond
	colors := []scene.Color{{R: 24, G: 24, B: 27, A: 255}, {R: 250, G: 250, B: 250, A: 255}}
	n := 0
	// render draws a frame of w×h, changing what it starts with and the
	// background, at once, as the window host does within SetWindowPos: a
	// swap chain for composition shows no background, and a frame changing
	// it alone would present nothing.
	render := func(w, h int) {
		t.Helper()
		n++
		start := time.Now()
		if err := r.Render(&scene.Scene{Width: w, Height: h, Clear: colors[n%2], Background: colors[n%2]}); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); r.uncapped && d > tooLong {
			t.Errorf("%s: a %dx%d frame took %v, resizing %v; want at most %v", kind, w, h, d, r.resizing, tooLong)
		}
	}
	// ready has the next frame wait until it can draw, as the window host
	// does, running what the renderer posts meanwhile.
	ready := func() {
		t.Helper()
		woke := false
		if r.Ready(func() { woke = true }) {
			return
		}
		for !woke {
			wait.For(t, kind+": what the frame after waits for", posted)()
		}
	}
	frame := func(w, h int) {
		t.Helper()
		ready()
		render(w, h)
	}
	frame(200, 150)
	if r.swapChain2 == 0 {
		t.Skip("no IDXGISwapChain2: the swap chain resizes whole at each size")
	}
	for range 3 {
		frame(200, 150)
	}
	if p.noWait(0) {
		t.Fatalf("%s: before the resize, a present with DXGI_PRESENT_DO_NOT_WAIT: %#x", kind, p.flags)
	}
	w, h := 200, 150
	if !r.uncapped {
		// Paced, a live resize presents as it did, whatever Present says.
		p.refuse = true
		for range 4 {
			w, h = w+17, h+9
			frame(w, h)
			frame(w, h)
		}
		if !r.resizing || p.noWait(0) || p.refused != 0 || r.unpresented {
			t.Fatalf("%s: resizing %v, presents with DXGI_PRESENT_DO_NOT_WAIT %v, %d refused, unpresented %v; want resizing, none",
				kind, r.resizing, p.noWait(0), p.refused, r.unpresented)
		}
		return
	}

	// A frame of a new size, which Present refuses for longer than a
	// refresh, returns to the window's messages within about that, and
	// waits to present.
	p.refuse = true
	w, h = w+17, h+9
	ready()
	from := len(p.flags)
	render(w, h)
	if !r.resizing || !r.unpresented || !r.refreshing || p.refused == 0 || !p.noWait(from) {
		t.Fatalf("%s: a %dx%d frame of a new size Present refused: resizing %v, unpresented %v, refreshing %v, %d refused, flags %#x; want resizing, unpresented, refreshing, refused with DXGI_PRESENT_DO_NOT_WAIT",
			kind, w, h, r.resizing, r.unpresented, r.refreshing, p.refused, p.flags[from:])
	}
	if info := r.FrameInfo(); !strings.Contains(info, "Present still drawing") || !strings.Contains(info, "presenting once DXGI can") {
		t.Errorf("%s: a frame Present refused says %q", kind, info)
	}
	woke := false
	if r.Ready(func() { woke = true }) {
		t.Fatalf("%s: the frame after one waiting to present is ready", kind)
	}
	// Tried again after the composition, it waits again while refused.
	refused := p.refused
	start := time.Now()
	wait.For(t, kind+": the composition the frame waits for", posted)()
	if d := time.Since(start); d > tooLong {
		t.Errorf("%s: trying the frame again took %v; want at most %v", kind, d, tooLong)
	}
	if woke || !r.unpresented || !r.refreshing || p.refused == refused {
		t.Fatalf("%s: tried again while refused: woken %v, unpresented %v, refreshing %v, %d more refused; want waiting, unpresented, refused",
			kind, woke, r.unpresented, r.refreshing, p.refused-refused)
	}
	// Once DXGI takes it, it presents, and the frame after waits for the
	// composition that shows it.
	p.refuse = false
	presented := p.presented
	for !woke {
		wait.For(t, kind+": the frame's present and its composition", posted)()
	}
	if r.unpresented || p.presented != presented+1 {
		t.Fatalf("%s: once taken: unpresented %v, %d presents; want one", kind, r.unpresented, p.presented-presented)
	}

	// A frame not of a new size, refused, gives way to the frame of the
	// next size, drawn whole at once within SetWindowPos, and is never
	// presented.
	p.refuse = true
	frame(w, h)
	if !r.unpresented {
		t.Fatalf("%s: a %dx%d frame Present refused does not wait to present", kind, w, h)
	}
	p.refuse = false
	presented = p.presented
	w, h = w-13, h-7
	render(w, h)
	if r.unpresented || r.redrew != redrewWhole || p.presented != presented+1 {
		t.Fatalf("%s: a %dx%d frame of a new size after one refused: unpresented %v, redrew %v, %d presents; want whole, one",
			kind, w, h, r.unpresented, r.redrew, p.presented-presented)
	}
	ready()
	if p.presented != presented+1 {
		t.Fatalf("%s: the frame refused presented after the one that took its place", kind)
	}

	// The frames after the last change, until one settles the swap chain,
	// then present without the flag.
	for end := time.Now().Add(settleDelay/2 + 20*time.Millisecond); time.Now().Before(end); {
		frame(w, h)
	}
	frame(w, h)
	if r.resizing {
		t.Fatalf("%s: the swap chain did not settle", kind)
	}
	from = len(p.flags)
	for range 3 {
		frame(w, h)
	}
	if p.noWait(from) || r.unpresented || r.noWaitRefused {
		t.Fatalf("%s: settled: presents with DXGI_PRESENT_DO_NOT_WAIT %v, unpresented %v, the flag refused %v; want none",
			kind, p.noWait(from), r.unpresented, r.noWaitRefused)
	}
}

// TestUncappedHeldFrameGivenUp checks that uncapped (MYGO_UNCAPPED), a
// frame that Present goes on refusing while the window changes size, as
// at the end of a resize, waits maxHeld at most, and then presents
// without DXGI_PRESENT_DO_NOT_WAIT on another goroutine (presentAside),
// and that frames go on: the frame after, which waits for it, draws then;
// with none waiting, the window is asked for a frame (askFrame). Without
// that bound, a SetWindowPos resize on a Radeon 780M held its last frame,
// and nothing drew again. TestUncappedRefusedThroughSettle has that
// present wait, and Present refuse past the settle.
func TestUncappedHeldFrameGivenUp(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer func(on bool) { pacing.Uncapped = on }(pacing.Uncapped)
	defer func(d time.Duration) { settleDelay = d }(settleDelay)
	settleDelay = time.Second
	defer func(d time.Duration) { maxHeld = d }(maxHeld)
	defer func(f func(_, _ time.Duration)) { awaitRefresh = f }(awaitRefresh)
	awaitRefresh = func(_, _ time.Duration) {}
	asked := 0
	defer func(f func(uintptr)) { askFrame = f }(askFrame)
	askFrame = func(uintptr) { asked++ }
	var p refusingPresent
	defer func(f func(_, _, _ uintptr, _ *presentParameters) uintptr) { presentCall = f }(presentCall)
	presentCall = p.call(presentCall)
	pacing.Uncapped = true
	for _, newRenderer := range []func(uintptr) (*Renderer, error){New, NewComposed} {
		r, err := newRenderer(hiddenWindow(t, 200, 150))
		if err != nil {
			t.Skip("no Direct3D 11:", err)
		}
		p = refusingPresent{}
		heldGivenUp(t, r, &p, &asked)
		r.Release()
	}
}

func heldGivenUp(t *testing.T, r *Renderer, p *refusingPresent, asked *int) {
	t.Helper()
	kind := "window"
	if r.composed {
		kind = "composition"
	}
	posted := make(chan func(), 16)
	r.SetPost(func(f func()) { posted <- f })
	colors := []scene.Color{{R: 24, G: 24, B: 27, A: 255}, {R: 250, G: 250, B: 250, A: 255}}
	n := 0
	// render draws a frame of w×h, changing what it starts with and the
	// background, at once.
	render := func(w, h int) {
		t.Helper()
		n++
		if err := r.Render(&scene.Scene{Width: w, Height: h, Clear: colors[n%2], Background: colors[n%2]}); err != nil {
			t.Fatal(err)
		}
	}
	// pump runs what the renderer posts until done.
	pump := func(what string, done func() bool) {
		t.Helper()
		for !done() {
			wait.For(t, kind+": "+what, posted)()
		}
	}
	// waiting has the frame after wait as the window host does, and
	// returns whether it can draw.
	waiting := func() func() bool {
		t.Helper()
		woke := false
		if r.Ready(func() { woke = true }) {
			t.Fatalf("%s: the frame after one waiting to present is ready", kind)
		}
		return func() bool { return woke }
	}
	ready := func() {
		t.Helper()
		woke := false
		if !r.Ready(func() { woke = true }) {
			pump("what the frame after waits for", func() bool { return woke })
		}
	}
	frame := func(w, h int) {
		t.Helper()
		ready()
		render(w, h)
	}
	frame(200, 150)
	if r.swapChain2 == 0 {
		t.Skip("no IDXGISwapChain2: the swap chain resizes whole at each size")
	}
	for range 3 {
		frame(200, 150)
	}
	w, h := 200, 150
	// resizeEnd draws a frame of a new size, which presents, and then one
	// that Present refuses for good, the resize's last.
	resizeEnd := func() {
		t.Helper()
		p.refuse = false
		w, h = w+17, h+9
		frame(w, h)
		p.refuse = true
		frame(w, h)
		if !r.resizing || !r.unpresented {
			t.Fatalf("%s: a %dx%d frame Present refused: resizing %v, unpresented %v; want resizing, waiting to present", kind, w, h, r.resizing, r.unpresented)
		}
	}
	// presentedAside checks that the frame refused presented once, without
	// DXGI_PRESENT_DO_NOT_WAIT, on another goroutine.
	presentedAside := func(presented, calls int) {
		t.Helper()
		if r.unpresented || r.aside != "" || p.presented != presented+1 || len(p.flags) == calls || p.flags[len(p.flags)-1]&presentDoNotWait != 0 || !r.gaveUp {
			t.Fatalf("%s: a frame refused for %v: unpresented %v, aside %q, %d presents, flags %#x, gave up %v; want presented once, without DXGI_PRESENT_DO_NOT_WAIT, paced after",
				kind, maxHeld, r.unpresented, r.aside, p.presented-presented, p.flags[calls:], r.gaveUp)
		}
	}
	// next draws the frame after one presented so, which Present takes, and
	// checks that it presents, and that frames go on.
	next := func(presented int) {
		t.Helper()
		p.refuse = false
		render(w, h)
		if info := r.FrameInfo(); p.presented != presented+2 || !strings.Contains(info, "after Present waited on another thread") {
			t.Fatalf("%s: the frame after one presented on another thread: %d presents, says %q; want two, after one presented so", kind, p.presented-presented, info)
		}
		if !r.gaveUp || !r.resizing {
			t.Fatalf("%s: the frame after one presented on another thread, taken: gave up %v, resizing %v; want paced until the swap chain settles",
				kind, r.gaveUp, r.resizing)
		}
		for range 3 {
			frame(w, h)
		}
		if p.presented != presented+5 || r.unpresented {
			t.Fatalf("%s: frames after one presented on another thread: %d presents, unpresented %v; want 5", kind, p.presented-presented, r.unpresented)
		}
	}

	// The frame after waits for the frame refused, which presents on
	// another goroutine once it waited maxHeld.
	maxHeld = 50 * time.Millisecond
	resizeEnd()
	heldAt, presented, before, calls := r.heldAt, p.presented, *asked, len(p.flags)
	pump("the frame refused presented, and the frame after woken", waiting())
	if d := time.Since(heldAt); d < maxHeld || *asked != before {
		t.Fatalf("%s: the frame after one refused woke %v after it, %d frames asked; want after %v, none", kind, d, *asked-before, maxHeld)
	}
	presentedAside(presented, calls)
	next(presented)

	// With no frame waiting for it, the window is asked for one.
	resizeEnd()
	presented, before, calls = p.presented, *asked, len(p.flags)
	pump("the frame refused presented, and a frame asked for", func() bool { return *asked != before })
	if r.refreshing {
		t.Fatalf("%s: a frame refused, none waiting: waiting for a refresh; want nothing waited for", kind)
	}
	presentedAside(presented, calls)
	if !r.Ready(func() { t.Errorf("%s: the frame asked for waited", kind) }) {
		t.Fatalf("%s: the frame asked for after one presented on another thread waits", kind)
	}
	next(presented)
}

// TestUncappedRefusedThroughSettle checks that uncapped (MYGO_UNCAPPED),
// when Present goes on refusing frames after the window's last change of
// size, as DXGI did on a Radeon 780M until the app made a call that
// waited, the window recovers, and no call that may wait is made on the
// main thread: the frame refused for maxHeld presents without
// DXGI_PRESENT_DO_NOT_WAIT on another goroutine (presentAside), waiting
// as long as DXGI has it, and the frame after waits for it; until DXGI
// takes a frame at once, the buffers resize, to settle or to grow, on
// another goroutine too (resizeAside); a frame that comes meanwhile, as
// within SetWindowPos, is put off, and frames draw again once the call
// returned; then the swap chain settles, and frames present uncapped
// again, with tearing where the system has it. Here Present refuses
// frames with the flag until a call to Present without it, or to
// ResizeBuffers, waited stall, and only then: on the Radeon 780M, DXGI
// took no frame with the flag for seconds after a resize's last frames
// were refused (before maxHeld bounded them), and a frame that then
// waited in ResizeBuffers and Present took 994 ms, frames coming after
// it. At 0338404, which put such a frame off until DXGI took a frame with
// the flag again and an event query said the GPU was done, never making a
// call that waits, 4 resizes of 70 put every frame off for good, and
// nothing presented again; this test then fails with frames put off for
// good. While Present refuses, or a call waits stall, no frame, nor
// anything the renderer posts, may take over a refresh and 200 ms; once
// DXGI takes frames again, a frame presents within that, and frames
// present uncapped within recoverBy.
func TestUncappedRefusedThroughSettle(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer func(on bool) { pacing.Uncapped = on }(pacing.Uncapped)
	defer func(d time.Duration) { settleDelay = d }(settleDelay)
	settleDelay = 400 * time.Millisecond
	defer func(d time.Duration) { maxHeld = d }(maxHeld)
	maxHeld = 50 * time.Millisecond
	defer func(f func(_, _ time.Duration)) { awaitRefresh = f }(awaitRefresh)
	awaitRefresh = func(_, _ time.Duration) {}
	// The frames come as an animation's: none needs asking for.
	defer func(f func(uintptr)) { askFrame = f }(askFrame)
	askFrame = func(uintptr) {}
	var p refusingPresent
	defer func(f func(_, _, _ uintptr, _ *presentParameters) uintptr) { presentCall = f }(presentCall)
	presentCall = p.call(presentCall)
	defer func(f func(_, _, _, _, _ uintptr) uintptr) { resizeBuffersCall = f }(resizeBuffersCall)
	resizeBuffersCall = p.resize(resizeBuffersCall)
	pacing.Uncapped = true
	for _, newRenderer := range []func(uintptr) (*Renderer, error){New, NewComposed} {
		r, err := newRenderer(hiddenWindow(t, 200, 150))
		if err != nil {
			t.Skip("no Direct3D 11:", err)
		}
		// Release waited for the calls made on another goroutine.
		p = refusingPresent{stall: 300 * time.Millisecond, main: threadID()}
		refusedThroughSettle(t, r, &p)
		r.Release()
	}
}

// recoverBy is how soon after DXGI takes frames again frames present
// uncapped, the swap chain settled (TestUncappedRefusedThroughSettle).
const recoverBy = time.Second

func refusedThroughSettle(t *testing.T, r *Renderer, p *refusingPresent) {
	t.Helper()
	kind := "window"
	if r.composed {
		kind = "composition"
	}
	// A refresh, which Present is tried for, and time for a slow machine
	// to draw, far below the 994 ms seen.
	tooLong := r.cost.refreshPeriod() + 200*time.Millisecond
	// DXGI takes frames again once a call waited: the frame refused waits
	// maxHeld, Present tried a refresh, and the call then waits stall.
	waitBy := maxHeld + 2*tooLong + p.stall
	posted := make(chan func(), 16)
	r.SetPost(func(f func()) { posted <- f })
	colors := []scene.Color{{R: 24, G: 24, B: 27, A: 255}, {R: 250, G: 250, B: 250, A: 255}}
	n := 0
	// timed runs what the main thread runs, a frame or what the renderer
	// posted, and checks that it returns soon when it began while Present
	// refused, or a call waited stall: what a wait on DXGI on the main
	// thread, the call itself or one waiting for it on another goroutine,
	// would hold past tooLong. Before and after, nothing here waits stall,
	// and a frame makes the calls an uncapped one makes, Present waiting
	// for the GPU (uncapped.go), which WARP on a busy CI runner takes past
	// tooLong now and then (305 ms, of a frame presenting uncapped again).
	timed := func(what string, f func()) {
		t.Helper()
		s := p.state()
		start := time.Now()
		f()
		if d := time.Since(start); (s.refuse || s.stalling != 0) && d > tooLong {
			t.Errorf("%s: %s took %v (resizing %v, gave up %v, aside %q); want at most %v", kind, what, d, r.resizing, r.gaveUp, r.aside, tooLong)
		}
	}
	// render draws a frame of w×h at once, changing what it starts with
	// and the background, as an animation does.
	render := func(w, h int) {
		t.Helper()
		n++
		timed("a frame", func() {
			if err := r.Render(&scene.Scene{Width: w, Height: h, Clear: colors[n%2], Background: colors[n%2]}); err != nil {
				t.Fatal(err)
			}
		})
	}
	// said counts what the frames said of the calls made on another
	// goroutine.
	said := map[string]int{}
	note := func() {
		t.Helper()
		info := r.FrameInfo()
		for _, what := range []string{"Present", "ResizeBuffers"} {
			if strings.Contains(info, "put off while "+what+" waits on another thread") {
				said["put off, "+what]++
			}
			if strings.Contains(info, "after "+what+" waited on another thread") {
				said["after "+what]++
			}
		}
		if r.putOff != strings.Contains(info, "put off") {
			t.Errorf("%s: put off %v, says %q", kind, r.putOff, info)
		}
	}
	// putOffNow draws a frame while a call waits on another goroutine, as
	// the window does within SetWindowPos, which does not wait for Ready,
	// and checks that it is put off.
	putOffNow := func(w, h int) {
		t.Helper()
		what := r.aside
		if what == "" {
			return
		}
		render(w, h)
		note()
		if !r.putOff || r.aside != what {
			t.Fatalf("%s: a frame while %s waits on another thread: put off %v, aside %q; want put off", kind, what, r.putOff, r.aside)
		}
	}
	// ready has the next frame wait until it can draw, as the window host
	// does, running what the renderer posts meanwhile, and drawing a frame
	// at once while a call waits on another goroutine (putOffNow).
	ready := func(w, h int) {
		t.Helper()
		woke := false
		if r.Ready(func() { woke = true }) {
			return
		}
		for !woke {
			timed("what the renderer posted", wait.For(t, kind+": what the frame after waits for", posted))
			putOffNow(w, h)
		}
	}
	frame := func(w, h int) {
		t.Helper()
		ready(w, h)
		render(w, h)
		note()
	}
	frame(200, 150)
	if r.swapChain2 == 0 {
		t.Skip("no IDXGISwapChain2: the swap chain resizes whole at each size")
	}
	for range 3 {
		frame(200, 150)
	}
	wantTearing := !r.composed && tearingSupported(r.factory)
	// uncappedAgain checks that frames present uncapped, and with tearing
	// where the system has it.
	uncappedAgain := func(w, h int) {
		t.Helper()
		from := len(p.flags)
		for range 5 {
			frame(w, h)
			if r.refreshing || r.unpresented {
				t.Fatalf("%s: after the resize, a frame waits (refreshing %v, unpresented %v)", kind, r.refreshing, r.unpresented)
			}
		}
		if p.noWait(from) || (r.presentFlags(0) == presentTearing) != wantTearing || strings.Contains(r.FrameInfo(), "paced") {
			t.Fatalf("%s: after the resize: DXGI_PRESENT_DO_NOT_WAIT %v, present flags %#x, says %q; want uncapped, tearing %v",
				kind, p.noWait(from), r.presentFlags(0), r.FrameInfo(), wantTearing)
		}
		for _, f := range p.flags[from:] {
			if wantTearing && f != presentTearing {
				t.Fatalf("%s: after the resize, a present with flags %#x; want tearing", kind, f)
			}
		}
	}
	// refuse has Present refuse frames with DXGI_PRESENT_DO_NOT_WAIT
	// until waits more calls waited.
	refuse := func(waits int) time.Time {
		fakeMu.Lock()
		defer fakeMu.Unlock()
		p.refuse, p.untilWaits, p.acceptedAt, p.takenAt = true, p.waited+waits, time.Time{}, time.Time{}
		return time.Now()
	}
	// recovers draws frames of w×h, as an animation does, until frames
	// present uncapped again, and checks that DXGI takes frames again
	// within waitBy of refusing them, a call having waited off the main
	// thread, then presents a frame within tooLong, and frames present
	// uncapped within recoverBy.
	recovers := func(w, h int, refusedAt time.Time) {
		t.Helper()
		frames := 0
		for r.resizing || r.gaveUp || r.aside != "" {
			s := p.state()
			if s.refuse && time.Since(refusedAt) > waitBy {
				t.Fatalf("%s: frames put off for good: Present still refuses %v after it began, %d frames since, %d calls waited (%d on the main thread), resizing %v, gave up %v, aside %q, says %q",
					kind, time.Since(refusedAt), frames, s.waited, s.waitedOnMain, r.resizing, r.gaveUp, r.aside, r.FrameInfo())
			}
			if !s.refuse && time.Since(s.acceptedAt) > recoverBy {
				t.Fatalf("%s: not recovered %v after DXGI took frames again: %d frames since Present refused, resizing %v, gave up %v, aside %q, says %q",
					kind, time.Since(s.acceptedAt), frames, r.resizing, r.gaveUp, r.aside, r.FrameInfo())
			}
			frame(w, h)
			frames++
		}
		s := p.state()
		if s.refuse || s.waitedOnMain != 0 || s.takenAt.IsZero() || s.takenAt.Sub(s.acceptedAt) > tooLong {
			t.Fatalf("%s: recovered: refusing %v, %d calls waited on the main thread, a frame taken %v after DXGI took frames again (taken %v); want none, within %v",
				kind, s.refuse, s.waitedOnMain, s.takenAt.Sub(s.acceptedAt), !s.takenAt.IsZero(), tooLong)
		}
		if r.bw != w || r.bh != h || r.w != w || r.h != h {
			t.Fatalf("%s: recovered: showing %dx%d in %dx%d buffers; want %dx%d, settled", kind, r.w, r.h, r.bw, r.bh, w, h)
		}
		t.Logf("%s: %d frames until recovered, %v after DXGI took frames again; frames said %v", kind, frames, time.Since(s.acceptedAt), said)
	}

	// A live resize, growing past the buffers, whose last frame Present
	// refuses, and every one after, until a call waited: the frame refused
	// presents on another goroutine, which waits, the frames meanwhile put
	// off, and the swap chain then settles, ResizeBuffers called on
	// another goroutine too. Every frame of the resize is of a new size, as
	// the user drags the window's border, so that the swap chain does not
	// settle before its last however long the runner takes between two;
	// and Present refuses from the last on, so that it refused a frame
	// before the swap chain settles. Refusing only from the frame after,
	// the settle came first where the runner took settleDelay/2 from the
	// resize's last change to that frame (Windows ARM64): with no frame
	// refused, the swap chain settled as it does, ResizeBuffers called on
	// the main thread, which then waited stall. The test, not the
	// renderer, had DXGI start refusing at the settle, which nothing tells
	// the renderer.
	w, h := 200, 150
	for range 6 {
		w, h = w+23, h+11
		frame(w, h)
		w--
		frame(w, h)
	}
	if !r.resizing || r.gaveUp {
		t.Fatalf("%s: resizing %v, gave up %v; want resizing", kind, r.resizing, r.gaveUp)
	}
	refusedAt := refuse(1)
	w--
	frame(w, h)
	if !r.resizing || !r.unpresented {
		t.Fatalf("%s: the resize's last frame, %dx%d, Present refused: resizing %v, unpresented %v; want resizing, waiting to present", kind, w, h, r.resizing, r.unpresented)
	}
	recovers(w, h, refusedAt)
	if said["put off, Present"] == 0 || said["after Present"] == 0 || said["put off, ResizeBuffers"] == 0 || said["after ResizeBuffers"] == 0 {
		t.Fatalf("%s: frames said %v; want frames put off while Present and ResizeBuffers waited on another thread, and frames after", kind, said)
	}
	uncappedAgain(w, h)

	// Another resize whose last frame Present refuses until two calls
	// waited: the frame refused presents on another goroutine, which
	// waits, and Present still refuses; a frame of a size the buffers do
	// not hold, drawn at once as within SetWindowPos, resizes them on
	// another goroutine, which waits too, and draws once it did.
	w, h = w+5, h+3
	frame(w, h)
	waited := p.state().waited
	refusedAt = refuse(2)
	// The window goes on changing size within the buffers, as the user
	// drags its border, so that the swap chain does not settle.
	for i := 0; p.state().waited == waited; i++ {
		if time.Since(refusedAt) > waitBy {
			t.Fatalf("%s: no call waited %v after Present refused (aside %q, gave up %v)", kind, time.Since(refusedAt), r.aside, r.gaveUp)
		}
		w += 1 - 2*(i%2) // a pixel wider, then back
		frame(w, h)
	}
	bw, bh := r.bw, r.bh
	w2, h2 := bw+40, bh+20
	render(w2, h2)
	note()
	if !r.putOff || r.aside != "ResizeBuffers" || r.w != w || r.h != h {
		t.Fatalf("%s: a %dx%d frame past the %dx%d buffers while Present refuses: put off %v, aside %q, showing %dx%d; want put off while ResizeBuffers waits on another thread, showing %dx%d",
			kind, w2, h2, bw, bh, r.putOff, r.aside, r.w, r.h, w, h)
	}
	putOffNow(w2, h2)
	// The window changes size again, within the new buffers, as the user
	// drags on: without, the frame after the call settles the swap chain,
	// as the call waited past settleDelay/2.
	w2--
	frame(w2, h2)
	if info := r.FrameInfo(); r.putOff || r.w != w2 || r.h != h2 || r.bw < w2 || r.bh < h2 || !r.resizing || !strings.Contains(info, "after ResizeBuffers waited on another thread") {
		t.Fatalf("%s: a %dx%d frame once ResizeBuffers returned: put off %v, showing %dx%d in %dx%d buffers, resizing %v, says %q; want drawn, resizing",
			kind, w2, h2, r.putOff, r.w, r.h, r.bw, r.bh, r.resizing, info)
	}
	recovers(w2, h2, refusedAt)
	if s := p.state(); s.waited != s.untilWaits {
		t.Fatalf("%s: %d calls waited, want %d", kind, s.waited, s.untilWaits)
	}
	uncappedAgain(w2, h2)
}
