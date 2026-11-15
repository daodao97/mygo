//go:build linux && (amd64 || arm64)

package linux

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// TestOverlayWindow makes a layer surface of a frameless window above
// others and out of the taskbar, and of no window with a frame, a parent or
// a role of its own.
func TestOverlayWindow(t *testing.T) {
	overlay := func() platform.WindowOptions {
		return platform.WindowOptions{AlwaysOnTop: true, SkipTaskbar: true, Frameless: true}
	}
	if o := overlay(); !overlayWindow(&o) {
		t.Error("a frameless always-on-top window out of the taskbar is no overlay")
	}
	for name, change := range map[string]func(*platform.WindowOptions){
		"not on top":     func(o *platform.WindowOptions) { o.AlwaysOnTop = false },
		"in the taskbar": func(o *platform.WindowOptions) { o.SkipTaskbar = false },
		"with a frame":   func(o *platform.WindowOptions) { o.Frameless = false },
		"a panel":        func(o *platform.WindowOptions) { o.Panel = true },
		"a flyout":       func(o *platform.WindowOptions) { o.Flyout = &platform.Flyout{} },
		"a child":        func(o *platform.WindowOptions) { o.Parent = &window{} },
	} {
		o := overlay()
		change(&o)
		if overlayWindow(&o) {
			t.Errorf("%s: an overlay", name)
		}
	}
}
