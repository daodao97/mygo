//go:build windows

package windows

import (
	"runtime"
	"testing"
)

// Exiting destroys the windows of MyGo's classes, and with the hidden
// application window the windows others made its children, as an input
// method's helper window, so none of them is left for ExitProcess (#1896).
func TestExitingDestroysWindows(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := registerClasses(); err != nil {
		t.Fatal(err)
	}
	app := createWindow(0, appClass, "", wsOverlapped, 0, 0, 0, 0, 0)
	win := createWindow(0, windowClass, "", wsOverlapped, 0, 0, 100, 100, 0)
	child := createWindow(0, "STATIC", "", wsChild, 0, 0, 0, 0, app)
	other := createWindow(0, "STATIC", "", wsOverlapped, 0, 0, 0, 0, 0)
	if app == 0 || win == 0 || child == 0 || other == 0 {
		t.Fatal("cannot create the windows")
	}
	defer procDestroyWindow.Call(other)
	b := &Backend{appHwnd: app}
	b.Exiting()
	for _, w := range []struct {
		name string
		hwnd uintptr
	}{{"application window", app}, {"MyGoWindow", win}, {"application window's child", child}} {
		if r, _, _ := procIsWindow.Call(w.hwnd); r != 0 {
			t.Errorf("the %s is still there", w.name)
		}
	}
	if r, _, _ := procIsWindow.Call(other); r == 0 {
		t.Error("a window of another class was destroyed")
	}
	if b.appHwnd != 0 {
		t.Error("appHwnd is still set")
	}
	b.Exiting() // once only
}
