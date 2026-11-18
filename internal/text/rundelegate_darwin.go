//go:build darwin

package text

import (
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

// The leaves of rundelegate_darwin_*.s.
func runDelegateDealloc()
func runDelegateAscent()
func runDelegateDescent()
func runDelegateWidth()
func runDelegateFuncs() (dealloc, ascent, descent, width uintptr)

// runDelegateCallbacks is a CTRunDelegateCallbacks, version 1.
type runDelegateCallbacks struct {
	version                         int
	dealloc, ascent, descent, width uintptr
}

// runDelegate holds Core Text's run delegates, bound once with Core Text
// (loadCoreText) and only read after: CTRunDelegateCreate, the name of
// their attribute, and their callbacks, which a package variable keeps in
// place.
var runDelegate struct {
	create    uintptr // CTRunDelegateCreate
	attribute uintptr // kCTRunDelegateAttributeName
	callbacks runDelegateCallbacks
}

// initDelegates finds Core Text's run delegates in the framework text.
func initDelegates(text uintptr, optional func(lib uintptr, name string) uintptr) {
	if sym, err := purego.Dlsym(text, "CTRunDelegateCreate"); err == nil {
		runDelegate.create = sym
	}
	runDelegate.attribute = optional(text, "kCTRunDelegateAttributeName")
	dealloc, ascent, descent, width := runDelegateFuncs()
	runDelegate.callbacks = runDelegateCallbacks{version: 1, dealloc: dealloc, ascent: ascent, descent: descent, width: width}
}

// delegates makes the run delegates of an engine's text, which give
// inline objects their size, from metrics it keeps, pinned while Core
// Text lays out the text (done). Each engine has its own: the engines of
// Testers lay out text on goroutines of their own, at the same time.
type delegates struct {
	metrics [][3]float64
	pinner  runtime.Pinner
}

// reset lets go of the metrics of the last text laid out, and makes room
// for n objects.
func (d *delegates) reset(n int) {
	d.pinner.Unpin()
	d.metrics = make([][3]float64, n)
	if n > 0 {
		d.pinner.Pin(&d.metrics[0])
	}
}

// done unpins the metrics once Core Text let go of the text's delegates,
// with its typesetter and lines: an engine dropped with metrics pinned,
// as a Tester's is, would panic as its pinner is collected.
func (d *delegates) done() { d.pinner.Unpin() }

// delegate returns a run delegate, owned, giving object k its height
// above the baseline, its descent below it and its width.
func (d *delegates) delegate(k int, width, height, descent float32) uintptr {
	if runDelegate.create == 0 || runDelegate.attribute == 0 || k >= len(d.metrics) {
		return 0
	}
	d.metrics[k] = [3]float64{float64(height), float64(descent), float64(width)}
	r, _, _ := purego.SyscallN(runDelegate.create, uintptr(unsafe.Pointer(&runDelegate.callbacks)), uintptr(unsafe.Pointer(&d.metrics[k])))
	return r
}
