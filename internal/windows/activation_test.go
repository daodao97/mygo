//go:build windows && (amd64 || arm64)

package windows

import (
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

type recordingWindowHandler struct {
	platform.WindowHandler
	events []platform.SurfaceEvent
}

func (h *recordingWindowHandler) Focused() {}
func (h *recordingWindowHandler) Blurred() {}

func (h *recordingWindowHandler) SurfaceEvent(ev platform.SurfaceEvent) bool {
	h.events = append(h.events, ev)
	return true
}

func TestMouseActivateConsumeActivationClick(t *testing.T) {
	b := &Backend{
		windows: map[uintptr]*window{},
	}
	for _, enabled := range []bool{false, true} {
		w := &window{
			b:       b,
			opts:    &platform.WindowOptions{ConsumeActivationClick: enabled},
			surface: &surface{},
		}
		w.surface.w = w

		// Client area activation press while inactive.
		res, handled := w.message(wmMouseActivate, 0, htClient|uintptr(wmLButtonDown)<<16)
		if enabled {
			if !handled || res != maActivateAndEat {
				t.Fatalf("enabled=true, inactive, htClient: got (%d, %v), want (%d, true)", res, handled, maActivateAndEat)
			}
			if w.surface.ateUp != wmLButtonUp {
				t.Fatalf("surface.ateUp = %#x, want %#x", w.surface.ateUp, wmLButtonUp)
			}
		} else if handled {
			t.Fatalf("enabled=false, inactive, htClient: expected unhandled, got (%d, %v)", res, handled)
		}

		// Once active, WM_MOUSEACTIVATE on the client area must not eat subsequent clicks.
		w.active = true
		w.surface.ateUp = 0
		if res, handled := w.message(wmMouseActivate, 0, htClient|uintptr(wmLButtonDown)<<16); handled {
			t.Fatalf("enabled=%v, active, htClient: expected unhandled, got (%d, %v)", enabled, res, handled)
		}
		w.active = false

		// Non-client areas (title bar, window controls, resize edges) keep default activation.
		for _, ht := range []uintptr{htCaption, htMinButton, htMaxButton, htClose, htTop, htTopLeft} {
			w.surface.ateUp = 0
			res, handled := w.message(wmMouseActivate, 0, ht|uintptr(wmLButtonDown)<<16)
			if handled {
				t.Fatalf("enabled=%v, ht=%d: expected unhandled, got (%d, %v)", enabled, ht, res, handled)
			}
			if w.surface.ateUp != 0 {
				t.Fatalf("enabled=%v, ht=%d: unexpected ateUp=%#x", enabled, ht, w.surface.ateUp)
			}
		}
	}
}

func TestSurfaceEatActivationRelease(t *testing.T) {
	h := &recordingWindowHandler{}
	b := &Backend{
		windows:  map[uintptr]*window{},
		surfaces: map[uintptr]*surface{},
	}
	w := &window{b: b, h: h, opts: &platform.WindowOptions{ConsumeActivationClick: true}}
	s := &surface{w: w, hwnd: 1234}
	w.surface = s

	for _, tc := range []struct {
		down uint16
		up   uint32
	}{
		{wmLButtonDown, wmLButtonUp},
		{wmRButtonDown, wmRButtonUp},
		{wmMButtonDown, wmMButtonUp},
		{wmXButtonDown, wmXButtonUp},
	} {
		h.events = nil
		if res, handled := w.message(wmMouseActivate, 0, htClient|uintptr(tc.down)<<16); !handled || res != maActivateAndEat {
			t.Fatalf("down=%#x: got (%d, %v), want (%d, true)", tc.down, res, handled, maActivateAndEat)
		}
		if res, handled := s.message(s.hwnd, tc.up, 0, 0); !handled || res != 0 {
			t.Fatalf("up=%#x: expected eaten (0, true), got (%d, %v)", tc.up, res, handled)
		}
		if len(h.events) != 0 {
			t.Fatalf("up=%#x: expected no surface events, got %+v", tc.up, h.events)
		}
		if s.ateUp != 0 {
			t.Fatalf("up=%#x: expected ateUp cleared, got %#x", tc.up, s.ateUp)
		}
	}

	// A stale ateUp (e.g. pointer released outside the window) is cleared by the next press.
	s.eatActivationRelease(wmLButtonDown)
	s.message(s.hwnd, wmLButtonDown, 1, 0)
	if s.ateUp != 0 {
		t.Fatalf("expected ateUp cleared on wmLButtonDown, got %#x", s.ateUp)
	}
	s.message(s.hwnd, wmLButtonUp, 0, 0)
	if len(h.events) != 2 || h.events[0].Kind != platform.PointerDown || h.events[1].Kind != platform.PointerUp {
		t.Fatalf("expected normal down+up after cleared ateUp, got %+v", h.events)
	}
}

func TestSyncWebViewEnabled(t *testing.T) {
	registerClasses()
	b := &Backend{
		windows:      map[uintptr]*window{},
		captions:     map[uintptr]*captionBar{},
		topEdges:     map[uintptr]*window{},
		surfaces:     map[uintptr]*surface{},
		captionFonts: map[int]captionFont{},
	}
	prev := theBackend
	theBackend = b
	defer func() { theBackend = prev }()

	hwnd := createWindow(0, windowClass, "test", wsOverlappedWindow|wsClipChildren, 0, 0, 300, 200, 0)
	if hwnd == 0 {
		t.Fatal("createWindow failed")
	}
	defer procDestroyWindow.Call(hwnd)

	const cw0Class = "Chrome_WidgetWin_0"
	wc := wndClassEx{
		WndProc:   procDefWindowProcW.Addr(),
		Instance:  instance(),
		ClassName: u16(cw0Class),
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	atom, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom != 0 {
		defer user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(u16(cw0Class))), instance())
	}
	child := createWindow(0, cw0Class, "", wsChild|wsVisible, 0, 0, 300, 200, hwnd)
	if child == 0 {
		t.Fatal("createWindow child failed")
	}
	defer procDestroyWindow.Call(child)

	var moveFocusCalls int
	var ctlVtbl [ctlMoveFocus + 1]uintptr
	ctlVtbl[ctlMoveFocus] = syscall.NewCallback(func(this, reason uintptr) uintptr {
		moveFocusCalls++
		return sOK
	})
	ctlPtr := &ctlVtbl[0]
	var pinner runtime.Pinner
	pinner.Pin(&ctlVtbl[0])
	pinner.Pin(&ctlPtr)
	defer pinner.Unpin()

	isEnabled := user32.NewProc("IsWindowEnabled")
	w := &window{
		b:          b,
		h:          &recordingWindowHandler{},
		hwnd:       hwnd,
		controller: uintptr(unsafe.Pointer(&ctlPtr)),
		opts:       &platform.WindowOptions{ConsumeActivationClick: true},
	}
	b.windows[hwnd] = w
	defer delete(b.windows, hwnd)

	if got := w.webViewChild(); got != child {
		t.Fatalf("webViewChild() = %#x, want %#x", got, child)
	}
	w.message(wmActivate, 1, 0)
	if !w.active {
		t.Fatal("expected w.active true after WA_ACTIVE")
	}
	if r, _, _ := isEnabled.Call(child); r == 0 {
		t.Fatal("expected child enabled while active")
	}
	w.message(wmActivate, waInactive, 0)
	if w.active {
		t.Fatal("expected w.active false after WA_INACTIVE")
	}
	if r, _, _ := isEnabled.Call(child); r != 0 {
		t.Fatal("expected child disabled while inactive")
	}
	// While disabled, RealChildWindowFromPoint skips child and hits hwnd,
	// so WM_MOUSEACTIVATE reaches w.message and returns MA_ACTIVATEANDEAT.
	res, cls := TestMouseActivate(hwnd, 50, 50)
	if cls != windowClass || res != maActivateAndEat {
		t.Fatalf("TestMouseActivate while inactive = (%d, %q), want (%d, %q)", res, cls, maActivateAndEat, windowClass)
	}
	w.message(wmActivate, 1, 0)
	if r, _, _ := isEnabled.Call(child); r == 0 {
		t.Fatal("expected child re-enabled after WA_ACTIVE")
	}

	// Simulate an activation click on an inactive page window: child stays
	// disabled across WA_CLICKACTIVE until the matching button-up arrives.
	w.message(wmActivate, waInactive, 0)
	if res, handled := w.message(wmMouseActivate, hwnd, htClient|uintptr(wmLButtonDown)<<16); !handled || res != maActivateAndEat {
		t.Fatalf("wmMouseActivate on page = (%d, %v), want (%d, true)", res, handled, maActivateAndEat)
	}
	w.message(wmActivate, 2, 0) // WA_CLICKACTIVE
	if r, _, _ := isEnabled.Call(child); r != 0 {
		t.Fatal("expected child kept disabled until activation release arrives")
	}
	if res, handled := w.message(wmLButtonUp, 0, 0); !handled || res != 0 {
		t.Fatalf("wmLButtonUp after activation press = (%d, %v), want (0, true)", res, handled)
	}
	if r, _, _ := isEnabled.Call(child); r == 0 {
		t.Fatal("expected child re-enabled after activation release")
	}
	if moveFocusCalls != 1 {
		t.Fatalf("expected focusWebView called once after activation release, got %d", moveFocusCalls)
	}

	// Capture loss clears atePageUp and re-enables child so it cannot stay disabled.
	w.message(wmActivate, waInactive, 0)
	w.message(wmMouseActivate, hwnd, htClient|uintptr(wmLButtonDown)<<16)
	w.message(wmActivate, 2, 0)
	w.message(wmCaptureChanged, 0, 0)
	if w.atePageUp != 0 {
		t.Fatalf("expected atePageUp cleared on wmCaptureChanged, got %#x", w.atePageUp)
	}
	if r, _, _ := isEnabled.Call(child); r == 0 {
		t.Fatal("expected child re-enabled after wmCaptureChanged")
	}

	// Non-click activation (WA_ACTIVE, e.g. Alt+Tab) clears stale atePageUp and re-enables child.
	w.message(wmActivate, waInactive, 0)
	w.message(wmMouseActivate, hwnd, htClient|uintptr(wmLButtonDown)<<16)
	w.message(wmActivate, 1, 0) // WA_ACTIVE
	if w.atePageUp != 0 {
		t.Fatalf("expected atePageUp cleared on WA_ACTIVE, got %#x", w.atePageUp)
	}
	if r, _, _ := isEnabled.Call(child); r == 0 {
		t.Fatal("expected child re-enabled after WA_ACTIVE")
	}
}
