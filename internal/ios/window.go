//go:build ios && cgo

package ios

/*
#include "native.h"
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

type window struct {
	b       *Backend
	h       platform.WindowHandler
	id      uint64
	view    uintptr
	title   string
	visible bool
	input   platform.TextInputState
}

func (w *window) Handle() uintptr           { return w.view }
func (w *window) Surface() platform.Surface { return w }
func (w *window) WatchDrawable(drawable uintptr) {
	C.mygo_ios_watch_drawable(C.uintptr_t(w.view), C.uintptr_t(drawable))
}
func (w *window) Native() platform.SurfaceNative {
	return platform.SurfaceNative{View: w.view, Layer: uintptr(C.mygo_ios_layer(C.uintptr_t(w.view)))}
}
func (w *window) Size() (float64, float64, float64) {
	var x, y, s C.double
	C.mygo_ios_size(C.uintptr_t(w.view), &x, &y, &s)
	return float64(x), float64(y), float64(s)
}
func (w *window) RefreshRate() float64 { return float64(C.mygo_ios_hz(C.uintptr_t(w.view))) }
func (w *window) RequestFrame() {
	if w.view != 0 {
		C.mygo_ios_frame(C.uintptr_t(w.view))
	}
}
func (w *window) PresentPixels(p []byte, stride, width, height int) {
	if len(p) > 0 {
		C.mygo_ios_pixels(C.uintptr_t(w.view), unsafe.Pointer(&p[0]), C.int(stride), C.int(width), C.int(height))
	}
}
func (w *window) SetCursor(platform.Cursor) {}
func (w *window) NativeTextSelection() bool { return bool(C.mygo_ios_native_selection()) }
func (w *window) SetTextInput(t platform.TextInputState) {
	if t.Options != w.input.Options || t.Password != w.input.Password {
		data, _ := json.Marshal(t.Options)
		cString(string(data), func(p *C.char) { C.mygo_ios_input_options(C.uintptr_t(w.view), p) })
	}
	w.input = t
	C.mygo_ios_input_history(C.uintptr_t(w.view), C.bool(t.CanUndo), C.bool(t.CanRedo))
	cString(fmt.Sprint(t.ID), func(id *C.char) {
		r := t.Bounds
		C.mygo_ios_input_bounds(C.uintptr_t(w.view), id, C.double(r.X), C.double(r.Y), C.double(r.W), C.double(r.H))
	})
	cString(t.Text, func(p *C.char) {
		r := t.Caret
		C.mygo_ios_input(C.uintptr_t(w.view), C.bool(t.Active), C.bool(t.ReadOnly), C.bool(t.Password), C.bool(t.Multiline), p, C.int(t.Start), C.int(t.End), C.double(r.X), C.double(r.Y), C.double(r.W), C.double(r.H))
	})
}
func (w *window) Occluded() bool { return !w.visible || !w.b.active }
func (w *window) Bounds() platform.Rect {
	x, y, _ := w.Size()
	return platform.Rect{Width: int(x), Height: int(y)}
}
func (w *window) ContentBounds() platform.Rect { return w.Bounds() }
func (w *window) SetTitle(s string)            { w.title = s }
func (w *window) Title() string                { return w.title }
func (w *window) SetBackgroundColor(c platform.Color) {
	C.mygo_ios_background(C.uintptr_t(w.view), C.double(c.R)/255, C.double(c.G)/255, C.double(c.B)/255, C.double(c.A)/255)
}
func (w *window) Show() {
	w.visible = true
	C.mygo_ios_show(C.uintptr_t(w.view), true)
	w.h.Focused()
	w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceFocus})
	w.RequestFrame()
}
func (w *window) ShowInactive() { w.Show() }
func (w *window) Hide() {
	w.visible = false
	C.mygo_ios_show(C.uintptr_t(w.view), false)
	w.h.Blurred()
	w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceBlur})
}
func (w *window) IsVisible() bool { return w.visible }
func (w *window) Focus()          { w.Show() }
func (w *window) Blur() {
	w.h.Blurred()
	w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceBlur})
}
func (w *window) IsFocused() bool    { return w.visible && w.b.active }
func (w *window) IsFullScreen() bool { return true }
func (w *window) Close() {
	if w.view == 0 {
		return
	}
	v := w.view
	w.view = 0
	w.b.window = nil
	C.mygo_ios_close(C.uintptr_t(v))
	w.h.Closed()
}

var _ platform.Window = (*window)(nil)
var _ platform.Surface = (*window)(nil)
