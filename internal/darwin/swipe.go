//go:build darwin && !ios

package darwin

import "math"

// The phases of a touchpad's scrolling gesture (NSEventPhase).
const (
	phaseBegan     = 1 << 0
	phaseChanged   = 1 << 2
	phaseEnded     = 1 << 3
	phaseCancelled = 1 << 4
)

// A two-finger swipe between pages goes back or forward once the fingers
// went swipeDistance DIPs sideways, and less than swipeSlope as far up
// or down (WebKit's minimumScrollEventRatioForSwipe).
const (
	swipeDistance = 60
	swipeSlope    = 0.5
)

// swipeTrack follows a touchpad's scrolling gesture while it may be a
// swipe between pages: with the touchpad set to swipe with two fingers,
// AppKit sends no swipeWithEvent: for it, only the scroll, which apps
// read themselves, as WebKit does. dx and dy are how far the scroll went,
// and taken whether the content took some of it sideways: a swipe over
// what scrolls sideways, or handles the scroll, stays a scroll.
type swipeTrack struct {
	on     bool
	dx, dy float64
	taken  bool
}

// swipeFromScrolls reports whether the touchpad swipes between pages
// with two fingers (System Settings, Trackpad, More Gestures).
var swipeFromScrolls = func() bool {
	return sendBool(class("NSEvent"), "isSwipeTrackingFromScrollEventsEnabled")
}

// swiping reports whether a touchpad's scroll of phase may be a swipe
// between pages, for the content to tell whether it takes it sideways:
// one of a gesture begun while the touchpad swipes between pages with two
// fingers. phase is 0 for a mouse's wheel and the momentum after a
// gesture.
func (t *swipeTrack) swiping(phase int) bool {
	if phase&phaseBegan != 0 {
		t.on = swipeFromScrolls()
		return t.on
	}
	return t.on && phase&(phaseChanged|phaseEnded|phaseCancelled) != 0
}

// scroll follows an event of a gesture swiping (above), of phase, which
// scrolled by dx, dy (scrollingDeltaX and Y) and which the content took
// sideways or not. The gesture ends a swipe as the fingers leave the
// touchpad, back if it scrolled toward the left as WebKit reads it:
// positive dx, the fingers to the right as the content follows them
// (natural scrolling), to the left otherwise.
func (t *swipeTrack) scroll(phase int, dx, dy float64, taken bool) (back, ok bool) {
	if phase&phaseBegan != 0 {
		*t = swipeTrack{on: true}
	}
	t.dx += dx
	t.dy += dy
	t.taken = t.taken || taken
	if phase&(phaseEnded|phaseCancelled) == 0 {
		return false, false
	}
	done := *t
	*t = swipeTrack{}
	if phase&phaseCancelled != 0 || done.taken || math.Abs(done.dx) < swipeDistance || math.Abs(done.dy) >= math.Abs(done.dx)*swipeSlope {
		return false, false
	}
	return done.dx > 0, true
}
