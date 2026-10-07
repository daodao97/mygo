package ui

import "github.com/egoist/mygo/internal/platform"

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
		t.id, t.active, t.scrolling = ev.PointerID, true, false
		t.keyboardTap = false
		t.x, t.y, t.lastX, t.lastY = x, y, x, y
		t.back, t.backWidth, t.backLevel = rt.backAt(x, y)
		t.backEntry = nil
		if t.back != nil {
			t.backEntry = t.back.current()
			// Reserve the edge without blurring an editor. A stationary tap
			// is replayed on release; vertical movement can still scroll.
			return true
		}
	case platform.PointerMove:
		if !t.active || ev.PointerID != t.id {
			return true
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
			// Custom drags and text selection keep their press. Other widgets
			// yield to a scrollable ancestor once a finger starts moving.
			if rt.pressed == nil || rt.pressed.flags&(flagDraggable|flagEditable|flagTrackPointer) == 0 {
				for _, id := range rt.hitChain(t.x, t.y) {
					s := rt.states[id]
					if s != nil && (s.flags&flagScrollY != 0 && abs32(y-t.y) > 8 || s.flags&flagScrollX != 0 && abs32(x-t.x) > 8) {
						rt.cancelPointer()
						t.scrolling = true
						if rt.ime.state.Options.Dismiss == "on-drag" {
							rt.focused = 0
							rt.updateTextInput()
						}
						break
					}
				}
			}
		}
		if t.scrolling {
			rt.pointerX, rt.pointerY = t.x, t.y
			rt.scroll(t.lastX-x, t.lastY-y, 0, true)
			t.lastX, t.lastY = x, y
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
		t.active = false
		rt.pointerIn = false
		if r := t.back; r != nil {
			entry := t.backEntry
			t.back = nil
			t.backEntry = nil
			if r.back != nil {
				r.moveBack(x-t.x, rt.now())
				r.endBack(ev.Kind == platform.PointerCancel, rt.now())
			} else if ev.Kind == platform.PointerUp && r.current() == entry && abs32(x-t.x) <= 8 && abs32(y-t.y) <= 8 {
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

func (rt *engine) cancelPointer() {
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
