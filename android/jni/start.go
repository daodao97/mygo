//go:build android && cgo

package jni

// The app-start seam: how an app's main runs when Java owns the process.
//
// 🔴 The model is gomobile's, and it is not a style choice — it is what the
// two runtimes allow. Go's own Android app package (x/mobile/app/android.go)
// says it in its header:
//
//	They are loaded by a running Java process. Before any entry point is
//	reached, a global constructor initializes the Go runtime, calling all Go
//	init functions. All cgo calls will block until this is complete.
//
// and its entry point then does:
//
//	func callMain(mainPC uintptr) { ... go callfn.CallFn(mainPC) }
//
// Two things follow, and both are enforced elsewhere in this repository:
//
//  1. A JNI entry point must return promptly. Android's JNI guidance is to
//     avoid blocking the UI thread across the boundary, so an app's main — a
//     loop that runs for the life of the process — can never be the body of
//     one. It runs on a goroutine of its own: that is the `go` in callMain.
//  2. The app's main is not the pump. The pump is the goroutine every backend
//     call runs on, and it is where frames are drawn from the surface's
//     events; if main blocked there, the events it waits for could never be
//     delivered (see Backend.Run).
//
// So Start spawns the goroutine and returns. The Java host's callbacks keep
// arriving on the pump, which draws the frames the app's main asked for.
var startFn func()

// Start registers the function that starts the app, and returns. It is called
// from an app's package init, which runs when the library is loaded — before
// any JNI call, so registration is never late.
//
// The function runs once, on a goroutine of its own, as soon as the host's
// first surface callback arrives.
func Start(fn func()) {
	if startFn == nil {
		startFn = fn
	}
}

// startApp spawns the app's main. A JNI entry point calls it — the first
// surface callback — which is why it returns immediately: the app's main runs
// until the process ends.
func startApp() {
	fn := startFn
	if fn == nil {
		return
	}
	startFn = nil
	androidLog("start: spawning the app's main goroutine")
	go func() {
		// When the app's main ends, by returning or by a panic, the process
		// ends with it: nothing can run that main again (see exitHost).
		defer exitHost()
		defer func() {
			if r := recover(); r != nil {
				androidLog("start: PANIC in the app's main: %v", r)
			}
		}()
		fn()
		androidLog("start: the app's main returned")
	}()
}
