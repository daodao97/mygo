// Package mobile holds what phones' interfaces are made of and desktops'
// aren't: sheets that come up from the bottom edge, pages and rows that a
// finger swipes, wheels that it spins, bars at the screen's edges, and the
// press of Material's ink, each looking as on its system.
//
// It is built on package ui's public API, as any app could be, and apps that
// don't import it don't pay for it. It works with a mouse too, and on
// desktops, with the look of the system that Current chooses.
package mobile

import (
	"runtime"

	"github.com/egoist/mygo/ui"
)

// Look is the design a widget follows.
type Look uint8

const (
	// Automatic chooses by the system the app runs on.
	Automatic Look = iota
	// Cupertino follows Apple's Human Interface Guidelines.
	Cupertino
	// Material follows Google's Material Design.
	Material
)

var look = Automatic

// SetLook makes the widgets follow a design whatever the system, as an app
// with one design everywhere, or to see the other system's on this one.
func SetLook(l Look) { look = l }

// Current returns the look widgets follow: Cupertino on Apple's systems
// unless SetLook said otherwise, Material elsewhere.
func Current() Look {
	if look != Automatic {
		return look
	}
	switch runtime.GOOS {
	case "ios", "darwin":
		return Cupertino
	}
	return Material
}

// overlayRadius is the corner of what comes up from the bottom of the screen.
func overlayRadius() float32 {
	if Current() == Cupertino {
		return 14
	}
	return 28
}

// scrim is the color that dims what is under a sheet or a drawer.
func scrim(c *ui.Context, fraction float32) ui.Color {
	return ui.RGBA(0, 0, 0, 0.4*max(0, min(fraction, 1)))
}

// separator is the color of the lines between rows.
func separator(t *ui.Theme) ui.Color { return t.Border }
