//go:build !js

package ui

// watchFonts does nothing: fonts registered on desktops and phones are
// there at once.
func (h *windowHost) watchFonts() {}
