//go:build darwin && !ios

package darwin

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var macClipboardWatch struct {
	once     sync.Once
	timer    id
	change   int
	notify   func()
	newTimer func(id, objc.SEL, float64, id, objc.SEL, id, bool) id
}

// AppKit has no public pasteboard-change notification. Poll only its change
// count while asked, never its types/data (which can invoke lazy providers).
func (clipboard) WatchChanges(changed func()) (func(), error) {
	w := &macClipboardWatch
	withPool(func() {
		w.once.Do(func() {
			purego.RegisterFunc(&w.newTimer, msgSendAddr)
			classDef("MyGoClipboardObserver", "NSObject", nil, []objc.MethodDef{
				method("clipboardTick:", func(_ id, _ objc.SEL, timer id) {
					if timer != w.timer || w.notify == nil {
						return
					}
					withPool(func() {
						change := sendInt(pasteboard(), "changeCount")
						if change != w.change {
							w.change = change
							w.notify()
						}
					})
				}),
			})
		})
		w.change = sendInt(pasteboard(), "changeCount")
		w.notify = changed
		observer := alloc("MyGoClipboardObserver")
		w.timer = retain(w.newTimer(class("NSTimer"), sel("timerWithTimeInterval:target:selector:userInfo:repeats:"), .25, observer, sel("clipboardTick:"), 0, true))
		release(observer) // NSTimer retains its target until invalidated.
		// A tenth of the interval, as Apple asks of repeating timers, lets
		// the system fire it with its other wakeups: nobody notices a
		// change 25 ms later, and the app idles in fewer of them.
		msgSetFloat(w.timer, sel("setTolerance:"), .025)
		send(send(class("NSRunLoop"), "mainRunLoop"), "addTimer:forMode:", uintptr(w.timer), kCFRunLoopCommonModes)
	})
	timer := w.timer
	return func() {
		if w.timer != timer {
			return
		}
		w.timer, w.notify = 0, nil
		send(timer, "invalidate")
		release(timer)
	}, nil
}
