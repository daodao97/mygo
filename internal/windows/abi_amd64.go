//go:build windows

package windows

import (
	"math"
	"unsafe"
)

// On x64, structs larger than 8 bytes are passed by reference.
func putBounds(controller uintptr, r rect) {
	comCall(controller, ctlPutBounds, uintptr(unsafe.Pointer(&r)))
}

// Go's Windows x64 calls copy the first integer arguments to the XMM
// registers as well, so a double can be passed as its bits.
func putZoom(w *window, f float64) {
	comCall(w.controller, ctlPutZoomFactor, uintptr(math.Float64bits(f)))
}

const zoomByScript = false

// userZoomed takes the controller's zoom as the page's after it changed,
// and reports whether it is new: a zoom the user chose (Ctrl+wheel, or
// Ctrl+0 back to SetZoom's), or theirs dropped as the page navigated.
// SetZoom's own is w.zoom already.
func (w *window) userZoomed() bool {
	var f float64
	comCall(w.controller, ctlGetZoomFactor, uintptr(unsafe.Pointer(&f)))
	if f <= 0 || f == w.zoom {
		return false
	}
	w.zoom = f
	return true
}
