package ui

import (
	"slices"

	"github.com/egoist/mygo/internal/platform"
)

// A Router's pages as the system's own stack of screens, on phones
// (Router.SystemStack): the history up to the page shown is the stack,
// each entry a screen of it, under the system's navigation bar with the
// page's title and a back button. On iOS the system moves the screens
// itself, and follows a finger going back from the left edge: the window
// holds one surface, which the host moves from screen to screen, leaving
// a snapshot of the page it showed in the screen going out of sight, as
// Hotwire Native does with its one web view. The router then shows each
// page at once. On Android, whose apps move their pages themselves, the
// router slides them as anywhere, under the system's toolbar.

// routerStack is the state of a window's stack.
type routerStack struct {
	// router is the Router the frame's pass built with SystemStack, the
	// first of the window, nil for none.
	router *Router
	// sent is what the surface was last told the stack is.
	sent []platform.StackScreen
	// due tells the surface, after the frame presents, that it shows the
	// page the stack shows last.
	due bool
}

// stackSurfaceOf returns the surface of the window as a StackSurface, nil
// where it is none.
func (rt *engine) stackSurfaceOf() platform.StackSurface {
	if h, ok := rt.host.(interface{ stackSurface() platform.StackSurface }); ok {
		return h.stackSurface()
	}
	return nil
}

// noteStack makes r, built with SystemStack, the router of the window's
// stack, unless another is, where the window can show one.
func (r *Router) noteStack(c *context) {
	r.stackMoves = false
	if !r.SystemStack || c.rt.stack.router != nil {
		return
	}
	s := c.rt.stackSurfaceOf()
	if s == nil || !s.CanStack() {
		return
	}
	c.rt.stack.router = r
	r.stackMoves = s.StackMoves()
}

// InSystemStack reports whether the system shows r's pages as its own
// stack of screens (SystemStack), with its navigation bar: the view then
// shows no bar of its own over them, nor a back button.
func (r *Router) coreInSystemStack(c *context) bool {
	if !r.SystemStack {
		return false
	}
	if c.rt.stack.router != nil {
		return c.rt.stack.router == r
	}
	s := c.rt.stackSurfaceOf()
	return s != nil && s.CanStack()
}

// syncStack tells the surface the stack the frame shows, before it
// presents: the surface keeps what it shows of the page going out of
// sight, which the frame replaces.
func (rt *engine) syncStack() {
	var screens []platform.StackScreen
	if r := rt.stack.router; r != nil {
		r.current()
		for _, e := range r.entries[:r.at+1] {
			screens = append(screens, platform.StackScreen{Key: r.entryKey(e), Title: e.title})
		}
	}
	if slices.Equal(screens, rt.stack.sent) {
		return
	}
	s := rt.stackSurfaceOf()
	if s == nil {
		return
	}
	rt.stack.sent = screens
	rt.stack.due = true
	s.SetStack(screens)
}

// stackShown tells the surface, once the frame presented, that it shows
// the page the stack shows last.
func (rt *engine) stackShown() {
	if !rt.stack.due {
		return
	}
	rt.stack.due = false
	if s := rt.stackSurfaceOf(); s != nil {
		s.StackShown()
	}
}

// stackPopped follows the user taking screens off the stack, n staying:
// the router goes back to the page of the last. The frame tells the
// surface the stack again: a system that pops its screens itself (iOS)
// finds it as it has it, one that shows the bar alone (Android) shows the
// page's title.
func (rt *engine) stackPopped(n int) {
	if n < 1 || n >= len(rt.stack.sent) {
		return
	}
	rt.stack.due = true
	if r := rt.stack.router; r != nil && r.at >= n-1 {
		r.Go(n - 1 - r.at)
	}
	rt.requestFrame()
}

// entryKey returns the key of entry e, telling its screen apart from the
// others in a stack.
func (r *Router) entryKey(e *routeEntry) uint64 {
	if e.key == 0 {
		r.keyed++
		e.key = r.keyed
	}
	return e.key
}
