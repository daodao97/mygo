//go:build ios || mygo_touch_elastic

package ui

import (
	"math"
	"time"
)

// Logical offsets stay clamped, including for virtual lists. Only placement
// uses the rubber-band displacement, so headers and sibling controls stay put.
type elasticScroll struct {
	id           uint64
	x, y         float64
	fromX, fromY float64
	released     bool
	at           time.Time
}

func elasticAccepts(s *state, dx, dy float32) bool {
	return dx != 0 && s.flags&flagScrollX != 0 && s.contentW > float64(s.w) ||
		dy != 0 && s.flags&flagScrollY != 0 && s.contentH > float64(s.h)
}

func (rt *engine) holdElastic() {
	if e := rt.elastic; e != nil {
		e.released = false
	}
}

func (rt *engine) releaseElastic() {
	if e := rt.elastic; e != nil && !e.released {
		e.fromX, e.fromY, e.at, e.released = e.x, e.y, rt.now(), true
		rt.requestFrame()
	}
}

func elasticAxis(position, over, delta, reach, extent float64) (float64, float64) {
	next := position + over + delta
	clamped := max(0, min(next, max(0, reach)))
	return clamped, max(-extent*4, min(next-clamped, extent*4))
}

func (rt *engine) elasticBy(s *state, dx, dy float32, momentum bool) bool {
	if rt.preferences().ReduceMotion {
		moved := scrollBy(s, dx, dy)
		if moved {
			rt.requestFrame()
		}
		return moved
	}
	e := rt.elastic
	if e == nil || e.id != s.id {
		e = &elasticScroll{id: s.id}
		rt.elastic = e
	}
	// Once momentum reaches an edge, stop its source. Repeated inertial events
	// must not pump a returning spring or hand off to the surrounding page.
	if momentum && (e.x != 0 || e.y != 0) {
		rt.releaseElastic()
		return false
	}
	if s.rtl {
		dx = -dx
	}
	x, y := s.scrollX, s.scrollY
	if s.flags&flagScrollX != 0 && s.contentW > float64(s.w) {
		x, e.x = elasticAxis(x, e.x, float64(dx), s.contentW-float64(s.w), float64(s.w))
	}
	if s.flags&flagScrollY != 0 && s.contentH > float64(s.h) {
		y, e.y = elasticAxis(y, e.y, float64(dy), s.contentH-float64(s.h), float64(s.h))
	}
	s.wheel = nil
	if s.track != nil {
		s.track.glide = nil
	}
	s.scrollTo(x, y)
	e.released = false
	if momentum && (e.x != 0 || e.y != 0) {
		// Convert the incoming step to a bounded impact displacement. Subsequent
		// momentum stops; the clock, not further scroll events, returns the edge.
		if e.x != 0 {
			e.x += math.Copysign(math.Min(math.Abs(float64(dx))*2, float64(s.w)*.25), e.x)
		}
		if e.y != 0 {
			e.y += math.Copysign(math.Min(math.Abs(float64(dy))*2, float64(s.h)*.25), e.y)
		}
		rt.releaseElastic()
	}
	rt.requestFrame()
	return !momentum || e.x == 0 && e.y == 0
}

func rubberBand(over, extent float64) float32 {
	if extent <= 0 {
		return 0
	}
	distance := math.Abs(over) * .55
	return float32(math.Copysign(distance*extent/(extent+distance), over))
}

func (rt *engine) elasticOffset(s *state) (float32, float32) {
	e := rt.elastic
	if e == nil || e.id != s.id {
		return 0, 0
	}
	if rt.preferences().ReduceMotion {
		rt.elastic = nil
		return 0, 0
	}
	// Content changes or programmatic scrolling can remove the touched edge.
	if e.x < 0 && s.scrollX > 0 || e.x > 0 && s.scrollX < s.contentW-float64(s.w) || s.contentW <= float64(s.w) {
		e.x, e.fromX = 0, 0
	}
	if e.y < 0 && s.scrollY > 0 || e.y > 0 && s.scrollY < s.contentH-float64(s.h) || s.contentH <= float64(s.h) {
		e.y, e.fromY = 0, 0
	}
	x, y := rubberBand(e.x, float64(s.w)), rubberBand(e.y, float64(s.h))
	if s.rtl {
		x = -x
	}
	return x, y
}

func (rt *engine) advanceElastic(now time.Time) {
	e := rt.elastic
	if e == nil || !e.released {
		return
	}
	if rt.states[e.id] == nil || rt.preferences().ReduceMotion {
		rt.elastic = nil
		return
	}
	// Analytical critically damped return: independent of refresh rate and
	// exact after a long pause, without accumulating numerical integration error.
	elapsed := math.Max(0, now.Sub(e.at).Seconds())
	decay := (1 + 18*elapsed) * math.Exp(-18*elapsed)
	e.x, e.y = e.fromX*decay, e.fromY*decay
	if math.Abs(e.x) < .1 && math.Abs(e.y) < .1 {
		rt.elastic = nil
		return
	}
	rt.animate()
}
