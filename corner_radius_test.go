//go:build !js

package mygo

import "testing"

// TestWindowCornerRadius: a frameless window's corners are its CornerRadius, a titled one's what the system says (the
// fake none until told), for content inset from the edges to round its own concentric with them.
func TestWindowCornerRadius(t *testing.T) {
	rounded, _ := testWindow(t, WindowOptions{Frameless: true, CornerRadius: 12})
	if r := rounded.CornerRadius(); r != 12 {
		t.Errorf("frameless with CornerRadius 12: %v", r)
	}
	titled, fw := testWindow(t, WindowOptions{TitleBarStyle: TitleBarHiddenInset})
	if r := titled.CornerRadius(); r != 0 {
		t.Errorf("titled, before the system said: %v", r)
	}
	onMain(func() { fw.SetCornerRadius(26) })
	if r := titled.CornerRadius(); r != 26 {
		t.Errorf("titled, the system's 26: %v", r)
	}
}
