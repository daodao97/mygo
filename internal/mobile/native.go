//go:build ios || android

package mobile

import "github.com/egoist/mygo/internal/nativehost"

// Native modules' main-thread methods and views run on the UI thread, in
// order with Go's calls of the host.
func init() {
	nativehost.OnUI = func(fn func()) {
		b := current.Load()
		if b == nil || b.t == nil || onUIThread() {
			fn()
			return
		}
		b.t.run(fn, true)
	}
	nativehost.PostUI = func(fn func()) {
		b := current.Load()
		if b == nil || b.t == nil {
			fn()
			return
		}
		b.t.run(fn, false)
	}
}
