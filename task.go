package mygo

import (
	"github.com/egoist/mygo/async"
	"time"
)

// NewTask delivers work/progress on the UI thread and cancels on background
// entry. Resume enables new work; it never silently reruns a canceled request.
// Call Close to detach lifecycle listeners and Cancel when leaving a page.
func NewTask[T any](notify func(async.State[T])) *async.Task[T] {
	needsApp("NewTask")
	t := async.New(func(fn func()) { postMain(fn) }, notify)
	off := App.OnLifecycleChanged(func(state LifecycleState) {
		if state == LifecycleActive {
			t.Resume()
		} else if state == LifecycleBackground {
			t.Pause()
		}
	})
	offQuit := App.OnQuit(t.Close)
	t.SetCleanup(func() { off(); offQuit() })
	onMain(func() {
		if App.Lifecycle() == LifecycleBackground {
			t.Pause()
		}
	})
	return t
}

// NewPoller also requires SetVisible(true) before it starts. It keeps timers
// suspended when the view is hidden or the application is in the background.
func NewPoller[T any](interval time.Duration, work async.Work[T], notify func(async.State[T])) *async.Poller[T] {
	needsApp("NewPoller")
	t := async.New(func(fn func()) { postMain(fn) }, notify)
	p := async.NewPoller(t, interval, work)
	off := App.OnLifecycleChanged(func(s LifecycleState) { p.SetForeground(s == LifecycleActive) })
	offQuit := App.OnQuit(p.Close)
	t.SetCleanup(func() { off(); offQuit() })
	onMain(func() { p.SetForeground(App.Lifecycle() == LifecycleActive) })
	return p
}
