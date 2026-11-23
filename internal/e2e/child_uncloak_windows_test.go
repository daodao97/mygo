//go:build windows && (amd64 || arm64)

package e2e

import (
	"syscall"
	"testing"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/wait"
	win "github.com/egoist/mygo/internal/windows"
	"github.com/egoist/mygo/ui"
)

var getDWMWindowAttribute = syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmGetWindowAttribute")

// dwmCloak reads the actual DWM flags, including errors, on the main thread.
func dwmCloak(hwnd uintptr) (flags uint32, hr int32) {
	const dwmwaCloaked = 14
	r, _, _ := getDWMWindowAttribute.Call(hwnd, dwmwaCloaked, uintptr(unsafe.Pointer(&flags)), unsafe.Sizeof(flags))
	return flags, int32(r)
}

func nativeFrameRevealed(w *mygo.Window) (revealed bool) {
	mygo.RunOnMain(func() {
		hwnd := w.NativeHandle()
		_, drawn := win.TestCloaked(hwnd)
		flags, hr := dwmCloak(hwnd)
		revealed = drawn && hr >= 0 && flags == 0
	})
	return
}

func TestContentWindowUncloaksBeforeBecomingChild(t *testing.T) {
	work := mygo.Screen.PrimaryDisplay().WorkArea
	x, y := work.X+40, work.Y+40
	parent := newWindow(t, mygo.WindowOptions{
		X: x, Y: y, Width: 440, Height: 240, Frameless: true, AlwaysOnTop: true,
		Content: ui.View(func(c *ui.Context) { c.Root().Background(ui.RGB(0, 0, 255)) }),
	})
	eventually(t, "the parent shown with its first frame", func() bool { return nativeFrameRevealed(parent) })

	const width, height = 200, 120
	view := ui.View(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Box(c).Absolute().Left(80).Top(20).Size(80, 80).Background(ui.RGB(255, 0, 0))
	})
	user32 := syscall.NewLazyDLL("user32.dll")
	getLong, setLong := user32.NewProc("GetWindowLongPtrW"), user32.NewProc("SetWindowLongPtrW")
	setParent, setPos := user32.NewProc("SetParent"), user32.NewProc("SetWindowPos")
	const (
		gwlStyle        = -16
		wsChild         = 0x40000000
		wsPopup         = 0x80000000
		wsVisible       = 0x10000000
		swpNoSize       = 0x0001
		swpNoZOrder     = 0x0004
		swpNoActivate   = 0x0010
		swpFrameChanged = 0x0020
	)
	styleIndex := int32(gwlStyle)
	var child *mygo.Window
	var hwnd uintptr
	ready := make(chan struct{}, 1)
	mygo.RunOnMain(func() {
		child = newWindow(t, mygo.WindowOptions{
			X: x, Y: y, Width: width, Height: height,
			Hidden: true, Frameless: true, Transparent: true, Content: view,
		})
		child.OnReadyToShow(func() { ready <- struct{}{} })
		hwnd = child.NativeHandle()
		_, drawn := win.TestCloaked(hwnd)
		if flags, hr := dwmCloak(hwnd); hr < 0 || flags == 0 || child.IsVisible() || drawn {
			t.Errorf("before changing style: want hidden, cloaked and undrawn, got DWM flags %#x (HRESULT %#x), visible %t, drawn %t", flags, uint32(hr), child.IsVisible(), drawn)
			return
		}
		style, _, err := getLong.Call(hwnd, uintptr(styleIndex))
		if style == 0 {
			t.Errorf("GetWindowLongPtrW: %v", err)
			return
		}
		// A caller can set WS_VISIBLE along with WS_CHILD: ShowInactive
		// then finds the window visible even if DWM still cloaks it.
		if prev, _, err := setLong.Call(hwnd, uintptr(styleIndex), style&^wsPopup|wsChild|wsVisible); prev == 0 && err != syscall.Errno(0) {
			t.Errorf("SetWindowLongPtrW: %v", err)
			return
		}
		flags, hr := dwmCloak(hwnd)
		if flags != 0 || hr < 0 {
			t.Errorf("after setting WS_CHILD: DWM flags %#x (HRESULT %#x), want uncloaked", flags, uint32(hr))
		}
		// NULL can also be the previous parent. Proc.Call clears and
		// captures last-error around the call.
		if prev, _, err := setParent.Call(hwnd, parent.NativeHandle()); prev == 0 && err != syscall.Errno(0) {
			t.Errorf("SetParent: %v", err)
			return
		}
		if got, _, _ := user32.NewProc("GetParent").Call(hwnd); got != parent.NativeHandle() {
			t.Errorf("GetParent = %#x, want %#x", got, parent.NativeHandle())
		}
		if ok, _, err := setPos.Call(hwnd, 0, 0, 0, 0, 0, swpNoSize|swpNoZOrder|swpNoActivate|swpFrameChanged); ok == 0 {
			t.Errorf("SetWindowPos: %v", err)
			return
		}
		if !child.IsVisible() {
			t.Error("the WS_VISIBLE child should already be visible to Win32 before ShowInactive")
		}
		child.ShowInactive()
		if flags, hr := dwmCloak(hwnd); flags != 0 || hr < 0 {
			t.Errorf("after ShowInactive: DWM flags %#x (HRESULT %#x), want uncloaked", flags, uint32(hr))
		}
	})
	if t.Failed() {
		return
	}
	wait.For(t, "the child's first frame", ready)
	eventually(t, "the child shown with its first frame", func() bool { return child.IsVisible() && nativeFrameRevealed(child) })
	if w, h := child.Size(); w != width || h != height {
		t.Errorf("child window size = %dx%d, want %dx%d", w, h, width, height)
	}

	t.Run("pixels", func(t *testing.T) {
		if !canCompose() {
			t.Skip("DirectComposition is unavailable")
		}
		red := func(w *mygo.Window) bool {
			r, g, b, ok := screenColor(w, 120, 60)
			return ok && r > 220 && g < 35 && b < 35
		}
		blue := func(w *mygo.Window, x float64) bool {
			r, g, b, ok := screenColor(w, x, 60)
			return ok && b > 220 && r < 35 && g < 35
		}
		// Some displays (including Windows ARM64 CI) do not expose
		// DirectComposition pixels to GDI. Calibrate with a separate
		// top-level window so a broken child cannot make the test skip.
		control := newWindow(t, mygo.WindowOptions{
			X: x + width + 20, Y: y, Width: width, Height: height, AlwaysOnTop: true,
			Frameless: true, Transparent: true, Content: view,
		})
		eventually(t, "the control shown with its first frame", func() bool { return nativeFrameRevealed(control) })
		if !wait.Poll(func() bool { return red(control) && blue(control, 40) }) {
			t.Skip("desktop sampling cannot read the composited top-level control")
		}
		control.Destroy()
		eventually(t, "the red pane and the blue parent through the child's transparent area", func() bool {
			return red(child) && blue(child, 40)
		})
		child.Hide()
		eventually(t, "the blue parent where the child was", func() bool { return blue(parent, 120) })
		child.ShowInactive()
		eventually(t, "the child pixels restored", func() bool { return red(child) && blue(child, 40) })
	})

	child.Hide()
	if child.IsVisible() {
		t.Error("the child is visible after Hide")
	}
	child.ShowInactive()
	eventually(t, "the child shown again", func() bool { return child.IsVisible() && nativeFrameRevealed(child) })

	mygo.RunOnMain(func() {
		child.Destroy()
		if alive, _, _ := user32.NewProc("IsWindow").Call(hwnd); alive != 0 || !child.IsDestroyed() {
			t.Error("the destroyed child is still registered or has a live HWND")
		}
	})
	if !parent.IsVisible() {
		t.Error("destroying the child hid its parent")
	}
}

// A regular top-level transparent window must still wait for composition,
// so DWM cannot expose a blank window while its first frame is in flight.
func TestContentWindowTransparentTopLevelStaysCloakedUntilComposed(t *testing.T) {
	if !canCompose() {
		t.Skip("DirectComposition is unavailable")
	}
	var w *mygo.Window
	var release func()
	mygo.RunOnMain(func() {
		release = win.TestHoldReveal()
		w = newWindow(t, mygo.WindowOptions{
			Width: 200, Height: 120, Hidden: true, Frameless: true, Transparent: true,
			Content: ui.View(func(c *ui.Context) { c.Root().Background(ui.RGB(255, 0, 0)) }),
		})
		if c, drawn := win.TestCloaked(w.NativeHandle()); !c || drawn {
			t.Errorf("before ShowInactive: cloaked %t, drawn %t, want cloaked and undrawn", c, drawn)
		}
		w.ShowInactive()
	})
	t.Cleanup(func() {
		if release != nil {
			mygo.RunOnMain(release)
		}
	})
	eventually(t, "the top-level window's first frame drawn", func() bool { _, drawn := cloaked(w); return drawn })
	mygo.RunOnMain(func() {
		if flags, hr := dwmCloak(w.NativeHandle()); flags == 0 || hr < 0 {
			t.Errorf("before composition: DWM flags %#x (HRESULT %#x), want cloaked", flags, uint32(hr))
		}
		release()
	})
	release = nil
	eventually(t, "the top-level window revealed after composition", func() bool { return nativeFrameRevealed(w) })
}
