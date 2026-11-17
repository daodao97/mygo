package mobile

import (
	"math"

	"github.com/egoist/mygo/internal/platform"
)

// twoFingers is the fork's two-finger gestures (FORK.md, ui.Element.Gestures):
// where an element under the fingers takes them, they come as pinch, rotate,
// pan and tap, from where they started, rather than as PointerZoom.
type twoFingers struct {
	on           bool
	at           float64 // when the second finger landed
	x, y, spread float64 // their center and spread then
	turn         float64 // how far they turned since
	moved        bool
}

func (s *surface) gesture(kind, phase string, x, y, scale, rotation, dx, dy float64) bool {
	return s.send(platform.SurfaceEvent{Kind: platform.SurfaceGesture, Gesture: kind, Phase: phase, X: x, Y: y, Scale: scale, Rotation: rotation, DX: dx, DY: dy, Touch: true})
}

// twoStart asks whether what is under two fingers that landed takes them.
func (s *surface) twoStart(at float64) {
	cx, cy := s.center()
	s.two = twoFingers{at: at, x: cx, y: cy, spread: s.spread()}
	s.pinchTurning = false
	if !s.gesture("pinch", "probe", cx, cy, 1, 0, 0, 0) {
		return
	}
	s.two.on = true
	for _, kind := range []string{"pinch", "rotate", "pan"} {
		s.gesture(kind, "begin", cx, cy, 1, 0, 0, 0)
	}
}

func (s *surface) twoMove() {
	cx, cy := s.center()
	d := s.spread()
	if math.Hypot(cx-s.two.x, cy-s.two.y) > 10 || math.Abs(d-s.two.spread) > 10 {
		s.two.moved = true
	}
	s.two.turn += s.turned()
	scale := 1.0
	if s.two.spread > 0 && d > 0 {
		scale = d / s.two.spread
	}
	s.gesture("pinch", "change", cx, cy, scale, 0, 0, 0)
	s.gesture("rotate", "change", cx, cy, 1, s.two.turn, 0, 0)
	s.gesture("pan", "change", cx, cy, 1, 0, cx-s.two.x, cy-s.two.y)
	s.pinchDist = d
}

// twoEnd ends the gestures as one of the two fingers lifts: two that landed
// and lifted together, still, tap.
func (s *surface) twoEnd(canceled bool, at float64) {
	cx, cy := s.center()
	phase := "end"
	if canceled {
		phase = "cancel"
	}
	for _, kind := range []string{"pinch", "rotate", "pan"} {
		s.gesture(kind, phase, cx, cy, 1, 0, 0, 0)
	}
	if !canceled && !s.two.moved && at-s.two.at < 0.3 && s.gesture("tap", "begin", cx, cy, 1, 0, 0, 0) {
		s.gesture("tap", "end", cx, cy, 1, 0, 0, 0)
	}
	s.two = twoFingers{}
}
