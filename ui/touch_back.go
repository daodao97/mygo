package ui

import (
	"math"
	"time"
)

// Going back with a gesture, in a Router: iOS's swipe from the left edge,
// which UIKit's stacks have and native UI draws for itself, and Android's
// predictive back, which the system reports (platform.SurfaceBackGesture).
// The page shown follows the finger off the page before it, which a finger
// let go past half the way, or thrown, goes back to.

// edgeReach is how far from the screen's left edge a finger starts going
// back, in DIPs, as UIKit's edge pan.
const edgeReach = 24

// swipeThrow is the speed of a finger, in DIPs a second, at which it goes back
// whatever how far it came.
const swipeThrow = 500

// noteBack has the frame's first Router take gestures while it can go back.
func (r *Router) noteBack(c *context) {
	// The system's stack takes its own gesture.
	if !r.DisableBackGesture && r.CanGoBack() && r.Transition == TransitionAuto && !c.rt.preferences().ReduceMotion && !r.stackMoves {
		c.rt.edgeBack = r
	}
}

// peeking reports whether a gesture shows the page before: while it swipes,
// and as it goes back to rest.
func (r *Router) peeking(c *context) bool {
	if r.settling {
		c.AnimationFrame()
	}
	return r.swiping || r.settling
}

// startEdgeSwipe takes a finger that came from the left edge for going back,
// on iOS: on Android, its edge swipe is the system's.
func (rt *engine) startEdgeSwipe(t *touchPan) bool {
	r := rt.edgeBack
	if r == nil || t.x0 > edgeReach || overscrollKind != overscrollBounce || rt.layoutDir() {
		return false
	}
	if t.waiting {
		t.waiting = false // it never pressed
	} else {
		rt.cancelPress()
	}
	t.edge = true
	r.swiping, r.settling = true, false
	return true
}

// layoutDir reports whether the window lays out right to left, whose edge
// swipe this does not take.
func (rt *engine) layoutDir() bool { return rt.host.rightToLeft() }

// moveEdgeSwipe follows the finger.
func (rt *engine) moveEdgeSwipe(t *touchPan, x float32) {
	r := rt.edgeBack
	if r == nil {
		return
	}
	w, _, _ := rt.host.size()
	if w > 0 {
		r.swipe = max(0, min((x-t.x0)/w, 1))
	}
	rt.swipeRouter = r
	rt.requestFrame()
}

// endEdgeSwipe lets go at a speed to the right, in DIPs a second.
func (rt *engine) endEdgeSwipe(speed float32) {
	r := rt.swipeRouter
	if r == nil {
		r = rt.edgeBack
	}
	if r == nil {
		return
	}
	rt.swipeRouter = nil
	r.swiping = false
	if r.swipe > 0.5 || speed > swipeThrow {
		r.commitSwipe()
		return
	}
	r.giveUpSwipe(rt)
}

// commitSwipe goes back, the slide going on from where the gesture left it.
func (r *Router) commitSwipe() { r.Back() }

// endSwipe ends a gesture as the history moves: the slide goes on from where
// the gesture left the page.
func (r *Router) endSwipe() {
	if !r.swiping && !r.settling {
		return
	}
	r.resume = r.swipe
	r.swipe, r.swiping, r.settling = 0, false, false
	if r.rt != nil && r.rt.swipeRouter == r {
		r.rt.swipeRouter = nil
	}
}

// giveUpSwipe sends the page back over the one before.
func (r *Router) giveUpSwipe(rt *engine) {
	r.swiping = false
	r.settling = true
	now := rt.now()
	r.settle = channel{value: float64(r.swipe), target: float64(r.swipe), at: now}
	headFor(&r.settle, 0, Animation{Duration: slideTime, Ease: EaseOut}, now)
	rt.swipeRouter = r
	rt.requestFrame()
}

// settleSwipe moves on what a gesture gave up.
func (rt *engine) settleSwipe(now time.Time) {
	r := rt.swipeRouter
	if r == nil || !r.settling {
		return
	}
	r.settle.advance(now)
	r.swipe = float32(math.Max(r.settle.value, 0))
	if !r.settle.moving && !r.settle.pending {
		r.swipe, r.settling = 0, false
		rt.swipeRouter = nil
		rt.requestFrame()
		return
	}
	rt.animating = true
}

// backGesture follows the system's gesture of going back. Letting go goes
// back with the back key, which commits it (Router.keys); or it comes as
// cancelled.
func (rt *engine) backGesture(phase string, progress float32) {
	r := rt.edgeBack
	if rt.swipeRouter != nil {
		r = rt.swipeRouter
	}
	if r == nil {
		return
	}
	switch phase {
	case "started":
		// What is open takes back first (Escape): the page stays.
		if rt.modal != 0 || rt.takesKey(0, KeyEscape) {
			return
		}
		r.swiping, r.settling = true, false
		r.swipe = 0
		rt.swipeRouter = r
	case "progress":
		if !r.swiping {
			return
		}
		// The system's progress follows the finger with a curve: the page
		// goes part of the way, as the back gesture's preview does.
		r.swipe = progress * 0.35
	case "cancelled":
		if r.swiping {
			r.giveUpSwipe(rt)
		}
	}
	rt.requestFrame()
}

// takesKey reports whether the frame's elements take a key.
func (rt *engine) takesKey(mods Modifiers, key Key) bool {
	_, found := routeKey(rt.nextRegs, rt.focusChain(), keyEvent{mods: mods, key: key})
	return found
}

// tellBack tells the surface whether the frame takes the system's back: what
// has a page to go back to, or takes its key or Escape, as what is open.
func (rt *engine) tellBack() {
	on := rt.edgeBack != nil || rt.modal != 0 || rt.takesKey(0, KeyBack) || rt.takesKey(0, KeyEscape)
	if on == rt.backSent {
		return
	}
	rt.backSent = on
	if s := rt.touchSurfaceOf(); s != nil {
		s.SetBackAvailable(on)
	}
}
