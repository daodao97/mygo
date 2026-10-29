//go:build android && cgo

package backend

import (
	"github.com/egoist/mygo/internal/platform"
)

// window is the app's one window: the Activity's surface. A phone has no
// window manager for the app to talk to, so the methods that would ask one
// (moving, resizing, stacking, full screen) are stubs — see window_gen.go —
// and the ones with meaning are here.
//
// It is shaped after internal/browser's window, which is in the same
// position: the host owns the one drawing area and the app only draws into
// it.
type window struct {
	b       *Backend
	h       platform.WindowHandler
	surface *Surface

	title   string
	bounds  platform.Rect
	visible bool
	focused bool
	closed  bool
}

// NewWindow makes the window. There is nothing to create: the Activity's
// surface exists before the app starts (it arrives through the first JNI
// callback, see Backend.OnSurfaceCreated), and this window is the app's view
// of it.
//
// 🔴 The surface is the backend's single one, not a new object. The pump is
// already drawing into it (frames from SurfaceEvent), and a second Surface
// would carry a second EGL state for the same ANativeWindow.
func (b *Backend) NewWindow(o *platform.WindowOptions, h platform.WindowHandler) (platform.Window, error) {
	b.mu.Lock()
	s := b.surface
	b.mu.Unlock()
	// The size asked for is only a hint on a phone: once the surface has a
	// size, that is the window's (see Backend.resized).
	bounds := platform.Rect{Width: o.Width, Height: o.Height}
	if s != nil && s.width > 0 && s.height > 0 {
		bounds = s.dipBounds()
	}
	w := &window{
		b:       b,
		h:       h,
		surface: s,
		title:   o.Title,
		bounds:  bounds,
	}
	b.mu.Lock()
	b.win = w
	b.mu.Unlock()
	// 🔴 The step input.go left open: every mapped event goes to this
	// window's handler, which is what turns a touch into a click on a
	// widget. Until it is set, DispatchInput drops what it maps (see
	// SurfaceSink) — which is exactly the "it draws but does not respond"
	// this wires up.
	SetSurfaceSink(func(ev platform.SurfaceEvent) bool { return h.SurfaceEvent(ev) })
	// A window that has just been made asks for a frame: this backend draws
	// the frames the content asks for and no others, so a content that has
	// not asked yet would never be run at all. It is also how the window
	// becomes visible — there is no Show to call on a phone.
	w.askFrame()
	return w, nil
}

func (w *window) Handle() uintptr { return 0 }

// Surface is the drawing area: the Activity's surface, which the backend
// already owns. It is nil only while no surface has arrived yet.
func (w *window) Surface() platform.Surface {
	if w.surface == nil {
		return nil
	}
	return w.surface
}

func (w *window) SetTitle(title string) { w.title = title }
func (w *window) Title() string         { return w.title }

func (w *window) SetBounds(r platform.Rect)        { w.bounds = r }
func (w *window) Bounds() platform.Rect            { return w.bounds }
func (w *window) SetContentBounds(r platform.Rect) { w.bounds = r }
func (w *window) ContentBounds() platform.Rect     { return w.bounds }

// SetRepresentedFile does nothing: the proxy icon it drives is a macOS title
// bar affordance. Android has no window chrome of its own to draw it in.
func (w *window) SetRepresentedFile(string) {}

// SetDocumentEdited does nothing, same reason as SetRepresentedFile: the
// "unsaved changes" dot lives in the close button, which is the platform's
// window decoration. The core marks the title instead.
func (w *window) SetDocumentEdited(bool) {}

// Show marks the window visible and asks for a frame. A phone's window is
// the whole screen and the Activity shows it; there is no separate step.
func (w *window) Show() {
	w.visible = true
	w.askFrame()
}

func (w *window) ShowInactive() { w.Show() }

// Hide is a no-op: an Activity cannot show nothing, and the surface keeps
// its last frame until something draws again.
func (w *window) Hide() {}

func (w *window) IsVisible() bool { return w.visible }

func (w *window) Focus() { w.focus() }
func (w *window) Blur()  { w.blur() }
func (w *window) IsFocused() bool {
	return w.focused
}

func (w *window) Close() {
	if w.closed {
		return
	}
	w.closed = true
	w.h.Closed()
}

// focus reports the window gaining the keyboard. The Activity's focus and
// the surface's own are one thing on a phone, so this is called as the
// surface comes up (see Backend.surfaceCreated) and by Focus.
func (w *window) focus() {
	if w.focused {
		return
	}
	w.focused = true
	w.h.Focused()
}

func (w *window) blur() {
	if !w.focused {
		return
	}
	w.focused = false
	w.h.Blurred()
}

// askFrame asks the content for a frame. The surface is where that request
// goes: the content draws when it is asked, and this backend shows what it
// drew (see Surface.PresentPixels).
func (w *window) askFrame() {
	if w.surface != nil {
		w.surface.RequestFrame()
	}
}
