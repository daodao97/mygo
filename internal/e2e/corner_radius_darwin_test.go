//go:build darwin

package e2e

import (
	"testing"

	"github.com/egoist/mygo"
)

// TestCornerRadius reads the corners macOS rounds windows with: a titled window's the system's, larger with
// TitleBarHiddenInset's toolbar than without (26 and 16 on macOS 26), a frameless one's its CornerRadius, or the system's
// without one.
func TestCornerRadius(t *testing.T) {
	plain := newWindow(t, mygo.WindowOptions{Width: 300, Height: 200, TitleBarStyle: mygo.TitleBarHidden})
	inset := newWindow(t, mygo.WindowOptions{Width: 300, Height: 200, TitleBarStyle: mygo.TitleBarHiddenInset})
	rp, ri := plain.CornerRadius(), inset.CornerRadius()
	if rp <= 0 || ri < rp {
		t.Errorf("titled: %v without a toolbar, %v with one; want both rounded, the toolbar's larger", rp, ri)
	}
	t.Logf("corners: %v without a toolbar, %v with one", rp, ri)
	rounded := newWindow(t, mygo.WindowOptions{Width: 300, Height: 200, Frameless: true, CornerRadius: 12})
	if r := rounded.CornerRadius(); r != 12 {
		t.Errorf("frameless with CornerRadius 12: %v", r)
	}
	plainFrameless := newWindow(t, mygo.WindowOptions{Width: 300, Height: 200, Frameless: true})
	if r := plainFrameless.CornerRadius(); r != rp { // titled under its hidden title bar: the system's corners
		t.Errorf("frameless without CornerRadius: %v, want the system's %v", r, rp)
	}
}
