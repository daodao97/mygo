package ui

import (
	"github.com/egoist/mygo/internal/platform"
	"time"
)

// touchEvent arbitrates a primary touch between a press and a scroll.
// Extra contacts are ignored by widgets while the primary contact owns input.
func (rt *engine) touchEvent(ev platform.SurfaceEvent) bool {
	x, y := float32(ev.X), float32(ev.Y)
	t := &rt.touch
	switch ev.Kind {
	case platform.PointerDown:
		if t.active {
			return true
		}
		rt.holdElastic()
		t.id, t.active, t.scrolling = ev.PointerID, true, false
		t.keyboardTap = false
		t.selecting, t.holdUntil = false, time.Time{}
		t.scrollTarget, t.scrollAxis = 0, 0
		if t.text != nil {
			t.text.reset()
		}
		t.x, t.y, t.lastX, t.lastY = x, y, x, y
		t.back, t.backWidth, t.backLevel = rt.backAt(x, y)
		t.backEntry = nil
		if t.back != nil {
			t.backEntry = t.back.current()
			// Reserve the edge without blurring an editor. A stationary tap
			// is replayed on release; vertical movement can still scroll.
			return true
		}
		if s := rt.handler(rt.hitChain(x, y)); s != nil && s.touchScroll {
			// Delay the press and focus until this contact becomes a tap.
			t.scrollTarget = s.id
			if s.touchSelection {
				t.holdUntil = rt.now().Add(500 * time.Millisecond)
				rt.requestFrame()
			}
			return true
		}
		if t.text != nil && t.text.reserve(x, y) {
			return true
		}
	case platform.PointerLongPress:
		if t.active && ev.PointerID == t.id && !t.scrolling {
			rt.beginTouchTextSelection()
		}
		return true
	case platform.PointerMove:
		if !t.active || ev.PointerID != t.id {
			return true
		}
		if t.selecting {
			return false
		}
		if r := t.back; r != nil {
			if r.back != nil {
				r.moveBack(x-t.x, rt.now())
				return true
			}
			dx, dy := x-t.x, y-t.y
			if r.current() != t.backEntry || !r.CanGoBack() {
				return true // navigation superseded this reserved contact
			}
			if dx > 8 && dx > abs32(dy) {
				r.back = &routeBack{from: r.current(), to: r.entries[r.at-1], width: t.backWidth, level: t.backLevel, moved: rt.now()}
				rt.cancelPointer()
				r.moveBack(dx, rt.now())
				return true
			}
			if abs32(dx) <= 8 && abs32(dy) <= 8 {
				return true
			}
			t.back = nil // vertical/leftward motion yields to scrolling
		}
		if !t.scrolling && (abs32(x-t.x) > 8 || abs32(y-t.y) > 8) {
			t.holdUntil = time.Time{}
			if t.scrollTarget != 0 {
				t.scrolling = true
				if abs32(y-t.y) > 8 && abs32(y-t.y) > abs32(x-t.x) {
					rt.dismissKeyboardOnDrag()
				}
			}
			// Custom drags and text selection keep their press. Other widgets
			// yield to a scrollable ancestor once a finger starts moving.
			if t.scrollTarget == 0 && (rt.pressed == nil || rt.pressed.flags&(flagDraggable|flagEditable|flagTrackPointer) == 0) {
				horizontal := abs32(x-t.x) > abs32(y-t.y)
				for _, id := range rt.hitChain(t.x, t.y) {
					s := rt.states[id]
					if s == nil {
						continue
					}
					axis := uint8(0)
					if horizontal && s.flags&flagScrollX != 0 && s.contentW > float64(s.w) {
						axis = 1
					} else if !horizontal && s.flags&flagScrollY != 0 && s.contentH > float64(s.h) {
						axis = 2
					}
					if axis != 0 {
						rt.cancelPointer()
						t.scrolling, t.scrollAxis = true, axis
						if axis == 2 {
							rt.dismissKeyboardOnDrag()
						}
						break
					}
				}
			}
		}
		if t.scrolling {
			rt.pointerX, rt.pointerY = t.x, t.y
			dx, dy := t.lastX-x, t.lastY-y
			if t.scrollAxis == 1 {
				dy = 0
			} else if t.scrollAxis == 2 {
				dx = 0
			}
			rt.scrollInput(dx, dy, 0, true, false, true)
			t.lastX, t.lastY = x, y
			return true
		}
		if t.scrollTarget != 0 || rt.pendingTouchText() {
			return true
		}
		t.lastX, t.lastY = x, y
	case platform.PointerUp, platform.PointerCancel:
		if !t.active || ev.PointerID != t.id {
			return true
		}
		if t.keyboardTap && !t.scrolling && ev.Kind == platform.PointerUp {
			rt.focused = 0
		}
		t.keyboardTap = false
		rt.releaseElastic()
		t.active = false
		t.holdUntil = time.Time{}
		rt.pointerIn = false
		if t.selecting {
			t.selecting = false
			return false
		}
		if t.text != nil && t.text.finish(ev.Kind) {
			return true
		}
		if id := t.scrollTarget; id != 0 {
			t.scrollTarget = 0
			if !t.scrolling && ev.Kind == platform.PointerUp && rt.handler(rt.hitChain(x, y)) == rt.states[id] {
				rt.pointerMove(t.x, t.y)
				rt.pointerDown(t.x, t.y, ev.Button, 0, ev.Clicks)
				return false
			}
			return true
		}
		if r := t.back; r != nil {
			entry := t.backEntry
			t.back = nil
			t.backEntry = nil
			if r.back != nil {
				r.moveBack(x-t.x, rt.now())
				r.endBack(ev.Kind == platform.PointerCancel, rt.now())
			} else if ev.Kind == platform.PointerUp && r.current() == entry && abs32(x-t.x) <= 8 && abs32(y-t.y) <= 8 {
				if rt.touchTextAt(t.x, t.y) != nil {
					return true
				}
				rt.pointerMove(t.x, t.y)
				rt.pointerDown(t.x, t.y, ev.Button, 0, ev.Clicks)
				return false // the ordinary Up completes the reserved tap
			}
			return true
		}
		if t.scrolling {
			rt.cancelPointer()
			return true
		}
	}
	return false
}

func (rt *engine) dismissKeyboardOnDrag() {
	if rt.ime.state.Active && rt.ime.state.Options.Dismiss == "on-drag" {
		rt.focused = 0
		rt.updateTextInput()
	}
}

func (rt *engine) advanceTouch() {
	t := &rt.touch
	if !t.active || t.scrolling || t.holdUntil.IsZero() || rt.now().Before(t.holdUntil) {
		return
	}
	if rt.pendingTouchText() {
		rt.beginTouchTextSelection()
		t.holdUntil = time.Time{}
		return
	}
	t.holdUntil = time.Time{}
	s := rt.states[t.scrollTarget]
	if s == nil || !s.touchSelection || s.flags&(flagDisabled|flagInert) != 0 || rt.handler(rt.hitChain(t.x, t.y)) != s {
		return
	}
	rt.pointerX, rt.pointerY = t.x, t.y
	if rt.deliver(s, InputEvent{Kind: InputLongPress, Button: 0}) {
		t.scrollTarget, t.selecting = 0, true
		rt.pressed, rt.pressButton = s, 0
		s.pressed = true
	}
}

func (rt *engine) cancelPointer() {
	rt.selection.dragging = false
	if s := rt.pressed; s != nil {
		if s.input != nil {
			rt.deliver(s, InputEvent{Kind: InputPointerCancel, Button: rt.pressButton, Mods: rt.mods})
		}
		s.pressed = false
		if s.editor != nil {
			s.editor.dragging = false
		}
		rt.pressed = nil
	}
	rt.scrollDrag.st = nil
	rt.dragCancel()
	rt.setHover(nil)
	rt.requestFrame()
}

// Selectable installs the touch selection policy; other native views do
// not link its hit testing, focus and word-selection implementation.
type touchTextPolicy interface {
	hit(float32, float32) *state
	reset()
	reserve(float32, float32) bool
	begin()
	finish(platform.SurfaceEventKind) bool
	pending() bool
	started() bool
}

func (rt *engine) pendingTouchText() bool { return rt.touch.text != nil && rt.touch.text.pending() }
func (rt *engine) touchTextAt(x, y float32) *state {
	if rt.touch.text == nil {
		return nil
	}
	return rt.touch.text.hit(x, y)
}
func (rt *engine) beginTouchTextSelection() {
	if rt.touch.text != nil {
		rt.touch.text.begin()
	}
}
