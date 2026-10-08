package ui

import (
	"github.com/egoist/mygo/internal/platform"
	"math"
)

// GestureEvent is a multi-touch update. Scale is multiplicative (1 means no
// change); Rotation is a delta in radians. X/Y are local to the element.
// Phase is begin, change, end or cancel; Kind is pinch or rotate.
type GestureEvent struct {
	Kind, Phase           string
	X, Y, Scale, Rotation float32
}
type gestureCapture struct {
	id              uint64
	scale, rotation float64
}

// Gestures opts an element into pinch/rotation input and returns updates since
// its last frame. A gesture stays with its starting element until it ends or
// that element disappears. Currently iOS supplies these events.
func (e *node) Gestures() []GestureEvent {
	e.gestureEnabled = true
	e.flags |= flagTrackPointer
	if len(e.st.gestures) > 0 {
		e.c.rt.consumed = true
	}
	return append([]GestureEvent(nil), e.st.gestures...)
}
func (rt *engine) gestureAt(x, y float32) uint64 {
	for _, id := range rt.hitChain(x, y) {
		s := rt.states[id]
		if s != nil && s.gestureEnabled && s.flags&(flagDisabled|flagInert) == 0 {
			return id
		}
	}
	return 0
}
func (rt *engine) gestureEvent(ev platform.SurfaceEvent) bool {
	if ev.Gesture != "pinch" && ev.Gesture != "rotate" {
		return false
	}
	if math.IsNaN(ev.Scale) || math.IsInf(ev.Scale, 0) || ev.Scale <= 0 || math.IsNaN(ev.Rotation) || math.IsInf(ev.Rotation, 0) {
		return false
	}
	if ev.Phase == "probe" {
		return rt.gestureAt(float32(ev.X), float32(ev.Y)) != 0
	}
	if rt.gestures == nil {
		rt.gestures = map[string]gestureCapture{}
	}
	capture := rt.gestures[ev.Gesture]
	if ev.Phase == "begin" {
		capture = gestureCapture{id: rt.gestureAt(float32(ev.X), float32(ev.Y)), scale: ev.Scale, rotation: ev.Rotation}
		if capture.id == 0 {
			return false
		}
		rt.cancelTouchBack()
		rt.cancelPointer()
		rt.touch.active, rt.touch.scrolling = false, false
	}
	s := rt.states[capture.id]
	if s == nil || s.seen != rt.frame || s.pass != rt.pass || !s.gestureEnabled || s.flags&(flagDisabled|flagInert) != 0 {
		delete(rt.gestures, ev.Gesture)
		return false
	}
	switch ev.Phase {
	case "begin", "change", "end", "cancel":
	default:
		return false
	}
	update := GestureEvent{Kind: ev.Gesture, Phase: ev.Phase, X: float32(ev.X) - s.x, Y: float32(ev.Y) - s.y, Scale: float32(ev.Scale / capture.scale), Rotation: float32(ev.Rotation - capture.rotation)}
	s.gestures = append(s.gestures, update)
	capture.scale, capture.rotation = ev.Scale, ev.Rotation
	rt.gestures[ev.Gesture] = capture
	if ev.Phase == "end" || ev.Phase == "cancel" {
		delete(rt.gestures, ev.Gesture)
	}
	rt.requestFrame()
	return true
}
