//go:build windows

package windows

import (
	"cmp"
	"strconv"
	"unsafe"
)

// On ARM64, structs of up to 16 bytes are passed in registers.
func putBounds(controller uintptr, r rect) {
	comCall(controller, ctlPutBounds,
		uintptr(uint32(r.Left))|uintptr(uint32(r.Top))<<32,
		uintptr(uint32(r.Right))|uintptr(uint32(r.Bottom))<<32)
}

// Go's Windows ARM64 calls do not set the floating point registers, which
// ICoreWebView2Controller::put_ZoomFactor reads: zoom with CSS instead,
// over the controller's zoom, which the user's Ctrl+wheel sets, so that
// the page shows at f in all.
func putZoom(w *window, f float64) {
	w.Eval("document.documentElement.style.zoom=" + strconv.FormatFloat(f/cmp.Or(w.userZoom, 1), 'f', -1, 64))
}

const zoomByScript = true

// userZoomed takes the controller's zoom, which only the user changes
// here (Ctrl+wheel), or WebView2 as the page navigates to another site,
// after it changed, and reports whether it is new. The page's zoom is
// the CSS one times it.
func (w *window) userZoomed() bool {
	var f float64
	comCall(w.controller, ctlGetZoomFactor, uintptr(unsafe.Pointer(&f)))
	old := cmp.Or(w.userZoom, 1)
	if f <= 0 || f == old {
		return false
	}
	w.zoom *= f / old
	w.userZoom = f
	return true
}
