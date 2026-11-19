//go:build js

package ui

// watchFonts has the window build and lay out a frame anew whenever a
// face registered has loaded (RegisterFont), which browsers do in the
// background: its text was measured with other fonts until then. detach
// stops it.
func (h *windowHost) watchFonts() {
	h.stopFonts = h.textSystem().WatchFonts(func() { h.post(h.rt.changed) })
}
