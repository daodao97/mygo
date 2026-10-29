//go:build android && cgo

// Package backend is the Android implementation of platform.Backend: the
// Go half of the libmygo c-shared library an Android Activity loads with
// System.loadLibrary("mygo").
//
// It lives in its own package rather than in internal/android, where the
// JNI //export functions are, because -buildmode=c-shared requires a main
// package and a main package cannot be imported: the library's entry point
// (internal/android) and the backend both need the same Backend value, so
// the Backend has to be the importable side.
//
// # Threading
//
// Android is unlike the desktop backends in one way that shapes this whole
// file: the event loop belongs to the Activity, not to us. The Activity
// owns the process' main thread, its Looper and the Choreographer, and it
// calls into Go through the //export functions of package main whenever it
// wants to (surface created/changed/destroyed, touch, key, size). So this
// backend never runs an event loop of its own and never pumps a native
// queue: it parks a single goroutine, the pump, on a channel of work
// items, and every Go call that touches backend state — and, once package
// ui is wired in, every call into package ui — happens on that goroutine,
// where IsMainThread is true.
//
// The //export functions run on whatever thread Java called them on. They
// must not touch Go state there: a cgo callback that enters the scheduler
// and then blocks on the main-thread machinery deadlocks. They only call
// Post (OnSurfaceCreated, OnSurfaceChanged, ...), which hands the work to
// the pump.
package backend

import (
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/unsupported"
)

// Backend is the Android implementation of platform.Backend.
//
// It embeds internal/unsupported's Backend, which answers every method the
// skeleton does not: menus, trays, dialogs, notifications, global
// shortcuts and window creation are all the stubs a platform without a
// native implementation gets, exactly as internal/browser embeds them.
// The Activity owns the window and hands us a Surface; mygo's own windows
// are not how an Android app is shown, so NewWindow stays unsupported.
type Backend struct {
	*unsupported.Backend

	mu sync.Mutex
	h  platform.AppHandler

	// tasks carries work to the pump goroutine. It is buffered so the
	// //export functions, which run on Java's threads, rarely block.
	tasks chan func()

	// started is true once a pump goroutine exists; running, while it is
	// inside its loop.
	started atomic.Bool
	running atomic.Bool

	// threadID is the OS thread the pump is locked to (see IsMainThread).
	threadID atomic.Int64

	// signaled coalesces Signal until the pump runs the Dispatch it asks
	// for.
	signaled atomic.Bool

	// readyFired guards the single AppHandler.Ready call.
	readyFired atomic.Bool

	// quit is closed by Quit; the pump returns on it.
	quit     chan struct{}
	quitOnce sync.Once

	// wake carries Wake to a Step that is blocked.
	wake chan struct{}

	// egl is the EGL state of the process: the display and the context
	// are made once and kept across surface changes (see egl.go).
	egl *eglState
	// surface is the drawing surface of the Activity's window; it exists
	// from the first surfaceCreated until the backend quits.
	surface *Surface
	// surfaceRenewed tells that a surface has already arrived, so the next
	// one is a replacement and the window is told (SurfaceRenew). renewPending
	// carries that from surfaceCreated to surfaceChanged, which delivers it
	// once the EGLSurface exists. See surfaceChanged.
	surfaceRenewed bool
	renewPending   bool
	// win is the app's window, once App.Run's NewWindow has made it. The
	// backend owns it because the surface events that drive it arrive here.
	win *window
	// frameQueued coalesces frame requests: a request made while one is
	// already queued is the same frame (see frame.go).
	frameQueued atomic.Bool

	// safeArea is the room the device's chrome takes, which Java reports
	// through Native.onSafeArea. Guarded by mu: it is written on Java's
	// thread (the JNI callback) and read on the pump's.
	safeArea platform.SafeArea

	// dark is the system's night mode as Java last reported it
	// (Native.onTheme); darkKnown tells the first report from a change.
	// themeSource is what Theme.SetSource asked for, guarded by mu. See
	// lifecycle.go.
	dark        atomic.Bool
	darkKnown   atomic.Bool
	themeSource string
}

// New returns the Android backend, creating it on first use.
//
// 🔴 It is a process-wide singleton, not a factory, and that is the whole
// point: an Android process has one Activity and one surface, and two callers
// must be the same object. They are libmygo's JNI entry points (the host's
// callbacks: surface, touch, keys, IME) and package mygo's own backend()
// (the app: windows, events, App.Run). With one Backend each, the pump's
// thread and the surface live in the instance the host fills, while the app
// talks to an empty one — the app never sees a frame and App.Run refuses
// because IsMainThread answers false for a pump that instance never ran.
//
// Every other backend may be a factory: a desktop process can hold several
// windows and tests want fresh state. Android has neither.
func New() *Backend {
	once.Do(func() {
		the = &Backend{
			Backend: unsupported.New(),
			tasks:   make(chan func(), 64),
			quit:    make(chan struct{}),
			wake:    make(chan struct{}, 1),
		}
	})
	return the
}

var (
	once sync.Once
	the  *Backend
)

// Name identifies the backend.
func (b *Backend) Name() string { return "android/egl" }

// Init records the app handler and the app name.
//
// There is no native application to prepare here: Android started the
// process and its Activity before the library was loaded, so Init only
// remembers what to call back and records the thread it runs on. mygo calls
// it on the main goroutine, which is the goroutine that goes on to call
// Run, so IsMainThread answers true for the caller before the pump's first
// iteration.
func (b *Backend) Init(h platform.AppHandler, opts platform.AppOptions) error {
	b.mu.Lock()
	b.h = h
	b.mu.Unlock()
	// 🔴 The thread to remember depends on who got here first, and getting it
	// wrong deadlocks the app instead of failing loudly.
	//
	// The desktop model, which the comment below used to describe alone: there
	// is no pump yet, mygo calls Init on the main goroutine, and that same
	// goroutine goes on to call Run — so recording the caller is what makes
	// IsMainThread true before the pump's first iteration.
	//
	// Android's order is the other one. Java drives the library from the first
	// surface callback, so the pump is already up and has recorded the thread
	// every backend call runs on. Overwriting it with the caller here (an
	// app's own goroutine, see android/jni.Start) makes IsMainThread false for
	// the pump — and then AppHandler.Ready, which the pump fires, runs the
	// app's WhenReady callback on the pump; NewWindow there sees "not the main
	// thread" and waits in waitReady for a ready that is being delivered right
	// now. The app hangs with its first frame never asked for.
	if !b.running.Load() {
		b.threadID.Store(int64(syscall.Gettid()))
	}
	// A pump may already have started, when Java drove the library before
	// mygo's App.Run (see Post): let it fire Ready now that there is a
	// handler to receive it.
	if b.running.Load() {
		b.Post(b.markReady)
	}
	_ = opts // the name is only used by the desktop backends so far
	return nil
}

// Run runs the pump until Quit and blocks, as platform.Backend asks.
//
// It does not create an event loop: Run is the pump, a goroutine parked on
// the work queue the JNI callbacks fill, so the loop it blocks on is the
// one driven by the Java callbacks. Run fires AppHandler.Ready once, so
// the app's windows can be opened, and then runs queued work until Quit.
//
// Nothing calls Run in this skeleton: the library's Java host drives the
// backend through the //export functions, whose first Post starts the pump
// itself. When mygo's App.Run is used instead, it calls Init and then Run,
// and Run is the pump on the main goroutine.
// Run blocks until the app quits, as platform.Backend asks — but on Android
// the loop it blocks on is the pump's, not one of its own.
//
// 🔴 Run must not run the pump itself, and this is the trap it avoids: mygo
// calls Run on the main thread, which on Android is the pump (see
// IsMainThread), so an App.Run that looped there would hold the one goroutine
// every backend call has to run on. Frames are drawn from SurfaceEvent
// (Event), and the input method, the clipboard and every other platform call
// is queued the same way — with Run looping on the pump, none of them run
// again, the app draws no frame, and an InputConnection request goes
// unanswered until its timeout. The symptom is a live, focused, black app
// whose log shows the host's callbacks and nothing else.
//
// So Run only makes sure the pump is up — it is already, started by the first
// JNI call — and then waits for it to quit. AppHandler.Ready has fired by
// then (the pump fires it once a handler is set), which is when the app opens
// its window, and the frame that follows is drawn on the pump like the rest.
func (b *Backend) Run() error {
	if b.started.CompareAndSwap(false, true) {
		// Nothing has driven the library yet: the pump is ours to start.
		go b.pump()
	}
	<-b.quit
	return nil
}

// pump is the single goroutine every backend call runs on. It is locked to
// an OS thread so IsMainThread, which compares thread ids, is stable, and
// so a renderer that needs a fixed thread gets one.
func (b *Backend) pump() error {
	runtime.LockOSThread()
	b.threadID.Store(int64(syscall.Gettid()))
	b.running.Store(true)
	if b.handler() != nil {
		b.markReady()
	}
	defer func() {
		b.running.Store(false)
		b.mu.Lock()
		if b.egl != nil {
			b.egl.release()
		}
		if b.surface != nil && b.surface.window != nil {
			releaseWindow(b.surface.window)
			b.surface.window = nil
		}
		b.mu.Unlock()
	}()
	for {
		select {
		case fn := <-b.tasks:
			fn()
		case <-b.quit:
			return nil
		}
	}
}

// Post schedules fn on the pump goroutine, starting the pump when nothing
// has yet. Every entry point that runs on a Java thread goes through it;
// touching backend or ui state on the calling thread instead is the cgo
// callback deadlock this indirection exists to avoid.
func (b *Backend) Post(fn func()) {
	if b.started.CompareAndSwap(false, true) {
		go b.pump()
	}
	select {
	case b.tasks <- fn:
	case <-b.quit:
	}
}

// Quit makes Run return, as platform.Backend asks. Releasing the EGL state
// is left to the pump's exit (pump's deferred cleanup), on the thread that
// owns it.
func (b *Backend) Quit() {
	b.quitOnce.Do(func() { close(b.quit) })
}

// IsMainThread reports whether the caller is the pump goroutine, which is
// this backend's "UI thread": Android's own main thread is Java's, and no
// Go code ever runs there. Safe to call from any goroutine.
//
// Before the pump starts it reports whether the caller is the goroutine
// Init recorded — mygo's main goroutine, the one that goes on to call Run.
func (b *Backend) IsMainThread() bool {
	tid := int64(syscall.Gettid())
	if b.running.Load() {
		return b.threadID.Load() == tid
	}
	return b.started.Load() && b.threadID.Load() == tid
}

// Signal asks the backend to run AppHandler.Dispatch on the pump as soon
// as it can, as platform.Backend asks. Safe to call from any goroutine.
//
// Calls coalesce: while a Dispatch is queued, a second Signal does nothing,
// so a burst of postMain calls becomes one drain (mainLoop.drain runs
// everything queued).
func (b *Backend) Signal() {
	if !b.signaled.CompareAndSwap(false, true) {
		return
	}
	b.Post(func() {
		b.signaled.Store(false)
		if h := b.handler(); h != nil {
			h.Dispatch()
		}
	})
}

// Step blocks until Wake or Quit, or runs one queued task if one is ready,
// as platform.Backend asks. mygo calls it from await so a goroutine
// producing a value gets to run while the main thread waits. Main thread
// only.
func (b *Backend) Step() {
	select {
	case fn := <-b.tasks:
		fn()
		return
	default:
	}
	select {
	case <-b.wake:
	case <-b.quit:
	}
}

// Wake unblocks a Step that is waiting. Safe to call from any goroutine.
func (b *Backend) Wake() {
	select {
	case b.wake <- struct{}{}:
	default:
	}
}

// handler returns the app handler, if Init has run.
func (b *Backend) handler() platform.AppHandler {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.h
}

// markReady fires AppHandler.Ready once, on the pump.
func (b *Backend) markReady() {
	if !b.readyFired.CompareAndSwap(false, true) {
		return
	}
	if h := b.handler(); h != nil {
		h.Ready()
	}
}

// The On* methods below are what package main's //export functions call.
// Each one only queues its work: they run on the thread Java chose, and the
// pump is the only place backend state is touched.

// OnSurfaceCreated adopts the ANativeWindow of the Activity's surface and
// builds the EGLSurface for it. window is the ANativeWindow* that
// C.ANativeWindow_fromSurface returned; the backend takes it over and
// releases it in OnSurfaceDestroyed, after the EGLSurface is gone.
func (b *Backend) OnSurfaceCreated(window unsafe.Pointer) {
	if window == nil {
		return
	}
	b.Post(func() { b.surfaceCreated(window) })
}

// OnSurfaceChanged records the surface's new size and density and shows a
// frame.
func (b *Backend) OnSurfaceChanged(w, h, dpi int) {
	b.Post(func() { b.surfaceChanged(w, h, dpi) })
}

// OnSurfaceDestroyed drops the EGLSurface and the ANativeWindow reference.
// The EGL context survives (see egl.go), so coming back from the background
// keeps every GL object and does not flash a blank frame.
func (b *Backend) OnSurfaceDestroyed() {
	b.Post(func() { b.surfaceDestroyed() })
}

// OnSizeChanged records a new size without a new density, for
// nativeOnSizeChanged, which Android reports separately from the surface's
// own change.
func (b *Backend) OnSizeChanged(w, h int) {
	b.Post(func() { b.sizeChanged(w, h) })
}

// OnTouch receives a touch event. The skeleton only forwards it: there is
// no ui content to deliver a platform.SurfaceEvent to yet (that is the
// WindowHandler.SurfaceEvent path, and package ui is not wired on Android).
// Keeping the export means the Java side can send events from day one.
func (b *Backend) OnTouch(action int, x, y float32, eventTimeMs int64) {
	b.Post(func() { b.touch(action, x, y, eventTimeMs) })
}

// OnKey receives a key event, forwarded exactly like OnTouch until package
// ui is wired.
func (b *Backend) OnKey(keyCode, action, metaState int) {
	b.Post(func() { b.key(keyCode, action, metaState) })
}

// OnSafeArea records the room the device's chrome takes, in DIPs, which
// Java reports from the window's WindowInsets.
//
// 🔴 It stores before posting, unlike the On* methods above, and that is
// deliberate. Those queue work that *changes* the backend, and the pump is
// where backend state may be touched. This one changes a value that a frame
// reads, and the frame it must be visible to is the one the same Java
// callback asks for — so the store happens on the calling thread (under mu,
// which is what makes it safe there) and the post only requests the frame.
// Queueing the store instead would let a frame be built from the previous
// insets, which on a rotation is a page laid out for the old orientation.
func (b *Backend) OnSafeArea(top, right, bottom, left int) {
	b.mu.Lock()
	b.safeArea = platform.SafeArea{Top: top, Right: right, Bottom: bottom, Left: left}
	b.mu.Unlock()
	// Ask for the frame this is for, through the window's handler — the
	// same path a title bar change takes. The window may not exist yet (the
	// first insets arrive with the surface, before App.Run makes one), and
	// then there is nothing to ask: the frame that runs once it does reads
	// the value stored above.
	b.Post(func() {
		if b.win != nil {
			b.win.h.SafeAreaChanged()
		}
	})
}

// surfaceCreated (pump) adopts the ANativeWindow of a (re)created surface. The
// EGL display and context are made once and kept; only the EGLSurface is
// rebuilt, which is the whole point of the split (see egl.go).
//
// The window is told its renderer is stale from surfaceChanged, not here: the
// EGLSurface does not exist until then (see adoptSurface), and a renewal that
// arrives first makes the window build a renderer with no current context —
// gl.New fails with "OpenGL 0.0 is too old" and the window drops to drawing in
// memory. surfaceChanged is also where the surface genuinely comes up, so it is
// the earliest point a renewal can succeed.
func (b *Backend) surfaceCreated(window unsafe.Pointer) {
	b.adoptSurface(window)
}

// adoptSurface takes the ANativeWindow of a (re)created surface and notes that
// the window's renderer is stale, for surfaceChanged to deliver. See
// surfaceCreated for the whole story.
func (b *Backend) adoptSurface(window unsafe.Pointer) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.egl == nil {
		b.egl = &eglState{}
	}
	if err := b.egl.init(); err != nil {
		log.Printf("mygo/android: egl: %v", err)
		return
	}
	if b.surface == nil {
		b.surface = &Surface{b: b, egl: b.egl}
	}
	b.surface.window = window
	// 🔴 A new ANativeWindow means the ui layer's GPU renderer must be made
	// again, and nothing else tells it so.
	//
	// The Activity destroys and recreates its Surface on every lock and every
	// background/foreground switch, and the ANativeWindow* that arrives here is
	// a different object each time (see MygoView's surfaceDestroyed). The ui
	// layer's windowHost, though, drew its first frame long ago: it still holds
	// the renderer it made then and sees no reason to make another, so it draws
	// into an ANativeWindow that is gone. Every frame fails, render() drops the
	// renderer for good (the retry is a minute away, and never comes while the
	// app is idle), and the app draws in memory from then on — the whole 18 MB
	// frame copied through the CPU, with the logo page that first frame shows.
	//
	// SurfaceRenew is the one event whose meaning is exactly this ("the native
	// objects a GPU renderer draws into changed"); Windows sends it when its
	// web views take over the frame (windows/webview_embed.go), and it makes
	// the window drop its renderer and ask for a frame. Android never sent it,
	// which is why the two platforms differed here.
	//
	// Only a surface after the first renews: the first one arrives before
	// App.Run and the window it would reach does not exist yet, and the
	// window's own NewWindow asks for its first frame regardless.
	renew := b.surfaceRenewed
	b.surfaceRenewed = true
	b.renewPending = renew
	// 🔴 The EGLSurface is NOT made here, and that is the fix for a real
	// EGL_BAD_ALLOC (0x3003) on a 1440x3168 phone.
	//
	// A surfaceCreated arrives before the window has a size: the Surface's
	// buffers are not allocated until the surface has one, which is what
	// surfaceChanged reports — and a driver asked to allocate window
	// buffers for an unsized window answers EGL_BAD_ALLOC, not a
	// configuration error. This function used to call createWindow here
	// anyway (the comment right below admitted the size was not known
	// yet), the call failed, and the app drew every frame in memory for
	// the rest of its life: about 10 ms a frame copying 18 MB from the CPU
	// to the window.
	//
	// gio's GioView ignores surfaceCreated outright — "surfaceChanged is
	// guaranteed to be called immediately after this" — and makes its
	// EGLSurface there. surfaceChanged below does the same thing now.
}

// surfaceChanged (pump) records the size, makes the EGLSurface, and shows one
// frame — the frame that proves the pipeline end to end: EGL context,
// EGLSurface, clear, swap, display.
func (b *Backend) surfaceChanged(w, h, dpi int) {
	b.mu.Lock()
	if b.surface == nil || b.surface.window == nil {
		b.mu.Unlock()
		return
	}
	b.surface.width, b.surface.height = w, h
	if dpi > 0 {
		b.surface.dpi = dpi
	}
	// The window has a size now, so this is where the EGLSurface is made —
	// see surfaceCreated above for why it cannot be made before.
	if b.egl.surface == nil {
		if err := b.egl.createWindow(b.surface.window); err != nil {
			b.mu.Unlock()
			log.Printf("mygo/android: egl: %v", err)
			return
		}
	}
	renew := b.renewPending
	b.renewPending = false
	b.mu.Unlock()
	// 🔴 Delivered outside the lock: b.deliver takes b.mu too, and a
	// sync.Mutex is not reentrant — renewing from inside the lock deadlocked
	// the pump on the first background switch.
	//
	// Delivered here rather than in surfaceCreated because only now does the
	// EGLSurface exist: a renew before it makes the window build a renderer
	// with no current context, and gl.New fails ("OpenGL 0.0 is too old"),
	// dropping the app to drawing in memory — the very symptom this fixes.
	if renew {
		log.Printf("mygo/android: new surface, renewing the window's renderer")
		b.deliver(platform.SurfaceEvent{Kind: platform.SurfaceRenew})
	}
	b.resized(w, h)
}

// sizeChanged (pump) is surfaceChanged without a density.
func (b *Backend) sizeChanged(w, h int) {
	b.mu.Lock()
	if b.surface == nil || b.surface.window == nil {
		b.mu.Unlock()
		return
	}
	b.surface.width, b.surface.height = w, h
	b.mu.Unlock()
	b.resized(w, h)
}

// resized (pump) shows a surface at its new size.
//
// 🔴 Once the app has a window, the window draws the frame, not the
// placeholder. The placeholder fill says "nothing has drawn yet", and
// presenting it over a live app flashed it on every rotation and, with
// adjustResize, every time the keyboard came up. The window is told as iOS
// tells it (goIOSResize): Resized, SurfaceResize, then a frame at the new
// size.
//
// The window also takes the surface here when it has none: a window made
// before the first surface arrived copied a nil one in NewWindow and would
// never have asked for a frame.
func (b *Backend) resized(width, height int) {
	b.mu.Lock()
	win, s := b.win, b.surface
	b.mu.Unlock()
	if s == nil {
		return
	}
	if win == nil || win.closed {
		s.PresentPixels(nil, 0, width, height)
		return
	}
	if win.surface == nil {
		win.surface = s
	}
	win.bounds = s.dipBounds()
	win.h.Resized()
	b.deliver(platform.SurfaceEvent{Kind: platform.SurfaceResize})
	win.askFrame()
}

// dipBounds is the surface's size in DIPs, which is what Window.Bounds
// reports: on a phone the window is the surface.
func (s *Surface) dipBounds() platform.Rect {
	w, h, _ := s.Size()
	return platform.Rect{Width: int(w + 0.5), Height: int(h + 0.5)}
}

// surfaceDestroyed (pump) drops the EGLSurface, then the ANativeWindow
// reference: the EGLSurface still points at the window until it is gone, so
// the order matters. The EGL context stays alive.
func (b *Backend) surfaceDestroyed() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.egl != nil {
		b.egl.destroyWindow()
	}
	if b.surface != nil && b.surface.window != nil {
		releaseWindow(b.surface.window)
		b.surface.window = nil
	}
}

// touch (pump) is where a touch would become a platform.SurfaceEvent once
// package ui is wired; it is a no-op stub until then.
// touch (pump) is the single-pointer path: Native.nativeOnTouch, which the
// app's view used before MygoView forwarded whole events through MygoInput.
// Both are kept working: a host that calls the old entry still moves things.
//
// It is the only pointer in its event, and its x and y are in view pixels —
// see TouchBatch for the batch path and input.go for what the numbers mean.
func (b *Backend) touch(action int, x, y float32, eventTimeMs int64) {
	TouchBatch(TouchInput{
		Action:      action,
		ActionIndex: 0,
		EventTimeMs: eventTimeMs,
		Pointers:    []Pointer{{ID: 0, X: x, Y: y}},
	})
}

// key (pump) is where a key would become a platform.SurfaceEvent once
// package ui is wired; it is a no-op stub until then.
// key (pump) is the single-key path, nativeOnKey: the same relationship to
// the key path MygoView forwards (MygoInput.onKeyEvent → KeyBatch) that touch
// has to TouchBatch.
func (b *Backend) key(keyCode, action, metaState int) {
	KeyBatch(KeyEventInput{KeyCode: keyCode, Action: action, MetaState: metaState})
}

// Backend must satisfy the platform contract.
var _ platform.Backend = (*Backend)(nil)

// StartedForLog and RunningForLog expose the pump's two flags, for the start
// diagnostics in android/jni. They are diagnostics, not API: whether the pump
// is up decides whether an app's startup may run.
func (b *Backend) StartedForLog() bool { return b.started.Load() }
func (b *Backend) RunningForLog() bool { return b.running.Load() }
