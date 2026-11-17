package ui

import (
	"math"

	"github.com/egoist/mygo/internal/platform"
)

// The fork's touch extensions on #940's touch engine (FORK.md): a long press
// an input handler takes (InputLongPress), elements a finger drives as a
// mouse (TouchPointer), panning that dismisses the keyboard
// (KeyboardDismiss), and two-finger gestures (Gestures). Each element asks
// for them in every frame, as for LongPressed.

// InputLongPress is a finger held still on the element for half a second, on
// a touch screen, which the element's input handler hears before the press
// does anything else. Taking it (returning true), the handler takes the
// finger: it hears InputPointerMove as the finger moves and InputPointerUp as
// it lifts, with Canceled as the system takes it, and nothing presses, pans
// or selects. Editable and selectable text hears it only once it calls
// TouchSelection.
const InputLongPress InputKind = 64

// KeyboardDismissMode is how panning dismisses the on-screen keyboard while
// a text input has the focus (KeyboardDismiss).
type KeyboardDismissMode string

const (
	// KeyboardDismissNone keeps the keyboard as a finger pans.
	KeyboardDismissNone KeyboardDismissMode = ""
	// KeyboardDismissOnDrag dismisses it as a finger starts panning.
	KeyboardDismissOnDrag KeyboardDismissMode = "on-drag"
	// KeyboardDismissInteractive dismisses it as a finger starts panning,
	// as KeyboardDismissOnDrag does: the keyboard does not follow the finger.
	KeyboardDismissInteractive KeyboardDismissMode = "interactive"
)

// TouchSelection has the element's input handler hear long presses on it
// (InputLongPress) where it is editable or selectable text, before a long
// press selects a word, for the frame it is called in.
func (e *node) TouchSelection() *node {
	if e.hasState() {
		e.st.touchHold = true
		e.st.touchFrame = e.c.rt.frame
	}
	return e
}

// TouchPointer has a finger on the element drive the pointer as a mouse does,
// rather than pan what scrolls under it: it presses at once, and its moves
// come as InputPointerMove, for the frame it is called in. A remote screen or
// a canvas that draws with the finger asks for it.
func (e *node) TouchPointer() *node {
	if e.hasState() {
		e.st.touchPointer = true
		e.st.touchFrame = e.c.rt.frame
	}
	return e
}

// KeyboardDismiss has a finger starting to pan anywhere dismiss the on-screen
// keyboard while the element, a text input, has the focus.
func (e *node) KeyboardDismiss(m KeyboardDismissMode) *node {
	if e.hasState() {
		e.st.keyboardDismiss = m
		e.st.touchFrame = e.c.rt.frame
	}
	return e
}

// GestureEvent is a two-finger update on a touch screen. Kind is pinch, rotate,
// pan (two fingers moving together) or tap (two fingers tapped at once, a
// begin then an end); Phase is begin, change, end or cancel. Scale is a
// pinch's since the last update (1 is no change), Rotation the radians it
// turned, and DX, DY how far a pan moved, in DIPs. X, Y are where the
// fingers are, in the element's units.
type GestureEvent struct {
	Kind, Phase           string
	X, Y, Scale, Rotation float32
	DX, DY                float32
}

type gestureCapture struct {
	id              uint64
	scale, rotation float64
	dx, dy          float64
}

// Gestures has the element take two fingers landing on it, and returns their
// updates since the last frame. A gesture stays with the element it started
// on until it ends or the element goes. Elements that ask for none leave two
// fingers to pinch what is under them (InputZoom).
func (e *node) Gestures() []GestureEvent {
	if !e.hasState() {
		return nil
	}
	s := e.st
	s.gestureEnabled = true
	s.touchFrame = e.c.rt.frame
	if len(s.gestures) == 0 {
		return nil
	}
	out := s.gestures
	s.gestures = nil
	e.c.rt.consumed = true
	return out
}

// forgetTouchAsks drops what an element no longer asks for.
func (s *state) forgetTouchAsks(frame uint64) {
	if s.touchFrame != frame {
		s.touchHold, s.touchPointer, s.gestureEnabled = false, false, false
		s.keyboardDismiss, s.gestures = KeyboardDismissNone, nil
	}
}

// inputHoldAt returns the innermost element under x, y whose input handler
// hears long presses (InputLongPress).
func (rt *engine) inputHoldAt(x, y float32) uint64 {
	for _, id := range rt.hitChain(x, y) {
		s := rt.states[id]
		if s == nil || s.input == nil || s.flags&flagDisabled != 0 {
			continue
		}
		if s.flags&(flagEditable|flagSelectable) != 0 && !s.touchHold {
			return 0
		}
		return id
	}
	return 0
}

// longPressInput offers t's long press to the input handler under it, and
// reports whether it took the finger.
func (rt *engine) longPressInput(t *touchPan) bool {
	id := t.inputHolds
	t.inputHolds = 0
	s := rt.states[id]
	if s == nil || s.input == nil {
		return false
	}
	if !rt.deliver(s, InputEvent{Kind: InputLongPress, Mods: rt.mods}) {
		return false
	}
	rt.haptic("medium")
	// The press ends with nothing told: its handler took the finger.
	if p := rt.pressed; p != nil {
		if p.editor != nil {
			p.editor.release()
		}
		rt.selection.dragging = false
		p.pressed = false
		rt.pressed = nil
		rt.setHover(nil)
	}
	t.inputHeld, t.holds, t.waiting = id, 0, false
	rt.requestFrame()
	return true
}

// heldEvent sends what a finger an input handler took does on to it.
func (rt *engine) heldEvent(ev platform.SurfaceEvent, x, y float32) bool {
	t := &rt.touch
	if t.inputHeld == 0 || !t.down {
		return false
	}
	switch ev.Kind {
	case platform.PointerMove:
		if s := rt.states[t.inputHeld]; s != nil && s.input != nil {
			rt.pointerX, rt.pointerY = x, y
			rt.deliver(s, InputEvent{Kind: InputPointerMove, Mods: rt.mods})
		}
		return true
	case platform.PointerUp:
		id := t.inputHeld
		*t = touchPan{}
		if !ev.Canceled {
			rt.pointerX, rt.pointerY = x, y
		}
		rt.endHeld(id, ev.Canceled)
		return true
	}
	return false
}

// endHeld tells the handler that took a finger that it lifted, or was taken.
func (rt *engine) endHeld(id uint64, canceled bool) {
	if s := rt.states[id]; s != nil && s.input != nil {
		rt.deliver(s, InputEvent{Kind: InputPointerUp, Mods: rt.mods, Canceled: canceled})
	}
	rt.requestFrame()
}

// touchPointerAt reports whether a finger at x, y drives the pointer
// (TouchPointer).
func (rt *engine) touchPointerAt(x, y float32) bool {
	for _, id := range rt.hitChain(x, y) {
		if s := rt.states[id]; s != nil && s.touchPointer && s.flags&flagDisabled == 0 {
			return true
		}
	}
	return false
}

// dismissKeyboardOnPan takes the focus from the text input that asked
// (KeyboardDismiss) as a finger starts panning.
func (rt *engine) dismissKeyboardOnPan() {
	if s := rt.states[rt.focused]; s != nil && s.keyboardDismiss != KeyboardDismissNone {
		rt.focused = 0
		rt.requestFrame()
	}
}

// gestureAt returns the innermost element under x, y that takes gestures.
func (rt *engine) gestureAt(x, y float32) uint64 {
	for _, id := range rt.hitChain(x, y) {
		if s := rt.states[id]; s != nil && s.gestureEnabled && s.flags&flagDisabled == 0 {
			return id
		}
	}
	return 0
}

// gestureEvent handles a SurfaceGesture: a probe reports whether an element
// takes a gesture at its place, and the phases of one go to the element it
// began on, as deltas.
func (rt *engine) gestureEvent(ev platform.SurfaceEvent) bool {
	switch ev.Gesture {
	case "pinch", "rotate", "pan", "tap":
	default:
		return false
	}
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	if !finite(ev.Scale) || ev.Scale <= 0 || !finite(ev.Rotation) || !finite(ev.DX) || !finite(ev.DY) {
		return false
	}
	if ev.Phase == "probe" {
		return rt.gestureAt(float32(ev.X), float32(ev.Y)) != 0
	}
	switch ev.Phase {
	case "begin", "change", "end", "cancel":
	default:
		return false
	}
	if rt.forkGestures == nil {
		rt.forkGestures = map[string]gestureCapture{}
	}
	capture := rt.forkGestures[ev.Gesture]
	if ev.Phase == "begin" {
		capture = gestureCapture{id: rt.gestureAt(float32(ev.X), float32(ev.Y)), scale: ev.Scale, rotation: ev.Rotation, dx: ev.DX, dy: ev.DY}
		if capture.id == 0 {
			return false
		}
		if rt.touch.down {
			rt.cancelTouch()
		}
	}
	s := rt.states[capture.id]
	if s == nil || s.seen != rt.frame || !s.gestureEnabled || s.flags&flagDisabled != 0 {
		delete(rt.forkGestures, ev.Gesture)
		return false
	}
	x, y := s.local(float32(ev.X), float32(ev.Y))
	s.gestures = append(s.gestures, GestureEvent{
		Kind: ev.Gesture, Phase: ev.Phase, X: x, Y: y,
		Scale:    float32(ev.Scale / capture.scale),
		Rotation: float32(ev.Rotation - capture.rotation),
		DX:       float32(ev.DX - capture.dx), DY: float32(ev.DY - capture.dy),
	})
	capture.scale, capture.rotation, capture.dx, capture.dy = ev.Scale, ev.Rotation, ev.DX, ev.DY
	rt.forkGestures[ev.Gesture] = capture
	if ev.Phase == "end" || ev.Phase == "cancel" {
		delete(rt.forkGestures, ev.Gesture)
	}
	rt.buildNext()
	return true
}
