package ui

import (
	"github.com/egoist/mygo/internal/scene"
	"math"
	"time"
)

// refreshPull is how far a finger pulls content past its top, as it shows,
// to refresh, in DIPs; refreshing, the content rests that far down on iOS.
const refreshPull = 60

// RefreshState is the pull to refresh of a scroll container, kept in the
// app's state (PullToRefresh).
type RefreshState struct {
	Handle

	// Refreshing shows the indicator, spinning. A finger pulling the
	// content sets it; set it to false once the app has what it fetched,
	// which sends the indicator away, or to true to show it.
	Refreshing bool
	was        bool
}

// PullToRefresh lets a finger pull a scroll container's content past its top
// to refresh it, as with iOS's UIRefreshControl and Android's swipe to
// refresh, each drawn as on its system: an indicator shows, spinning while
// s.Refreshing. It reports whether the finger just asked to refresh: fetch
// what's new, and set s.Refreshing to false.
//
//	list := ui.Scroll(c).Fill()
//	if list.PullToRefresh(&app.refresh) {
//		go func() {
//			items := fetch()
//			win.Update(func() { app.items, app.refresh.Refreshing = items, false })
//		}()
//	}
func (e *node) PullToRefresh(s *RefreshState) bool {
	if !e.hasState() {
		return false
	}
	st, rt := e.st, e.c.rt
	st.refresh = s
	st.fingersFrame = rt.frame
	refreshPainter = (*Painter).paintRefresh
	if s.was != s.Refreshing {
		s.was = s.Refreshing
		if !st.refreshFired {
			rt.startBounce(st, 0, 0)
		}
	}
	if s.Refreshing {
		// The spinner turns.
		e.c.AnimationFrame()
	}
	fired := st.refreshFired
	st.refreshFired = false
	if fired {
		rt.consumed = true
		rt.haptic("light")
	}
	return fired
}

// refreshHold is where the content rests while refreshing, as over[1].
func (s *state) refreshHold() float32 {
	if s.refresh != nil && s.refresh.Refreshing {
		return -refreshPull
	}
	return 0
}

// movesContent reports whether pulling moves the content, as iOS's does: on
// Android, the indicator comes down over it.
func (s *RefreshState) movesContent() bool { return overscrollKind != overscrollStretch }

// contentOver is how far the content shows past its ends, down.
func (s *state) contentOver() float32 {
	if overscrollKind == overscrollStretch {
		return 0 // it stretches where it is (stretchOf)
	}
	if s.refresh != nil && !s.refresh.movesContent() {
		return 0
	}
	return s.over[1]
}

// contentOverX is how far the content shows past its ends, across.
func (s *state) contentOverX() float32 {
	if overscrollKind == overscrollStretch {
		return 0
	}
	return s.over[0]
}

// stretchShare is how much of the distance pulled past its end the content
// grows by, along the length of the view: as much as Android's, at the limits
// of setOver.
const stretchShare = 0.5

// stretchOf is the transform of what Android's stretch shows inside e, with its
// pivot at the end away from the pull: content past the top grows down from the
// top, and past the bottom up from the bottom. Its set is false without one.
func stretchOf(e *node) (t Transform, ox, oy float32) {
	s := e.st
	over := s.over
	if s.refresh != nil && over[1] < 0 {
		over[1] = 0 // the indicator comes down instead
	}
	if over == [2]float32{} || e.w <= 0 || e.h <= 0 {
		return Transform{}, 0, 0
	}
	sx := 1 + abs32(over[0])/e.w*stretchShare
	sy := 1 + abs32(over[1])/e.h*stretchShare
	ox, oy = e.x, e.y
	if over[0] > 0 {
		ox += e.w
	}
	if over[1] > 0 {
		oy += e.h
	}
	return Scaling(sx, sy), ox, oy
}

// stretchPainter is stretchOf where Android's stretch is on.
func stretchPainter(world scene.Affine, e *node) scene.Affine {
	if overscrollKind != overscrollStretch {
		return world
	}
	if t, ox, oy := stretchOf(e); t.set() {
		m := Translation(-ox, -oy).Then(t).Then(Translation(ox, oy))
		return world.Mul(m.flat())
	}
	return world
}

// touchIndicator returns how much a scroll indicator shows, from 0 to 1, as a
// finger moved the content a moment ago, and keeps frames coming until it
// fades.
func (s *state) touchIndicator(rt *engine) float32 {
	if s.barsUntil.IsZero() {
		return 0
	}
	left := s.barsUntil.Add(indicatorFade).Sub(rt.now())
	if left <= 0 {
		s.barsUntil = time.Time{}
		return 0
	}
	if until := s.barsUntil.Add(indicatorFade); until.After(rt.barsFadeAt) {
		rt.barsFadeAt = until
	}
	rt.animating = true
	return min(1, float32(left)/float32(indicatorFade))
}

// paintRefresh draws the indicator of a container's pull to refresh.
func (p *Painter) paintRefresh(e *node) {
	st := e.st
	r := st.refresh
	if r == nil {
		return
	}
	pull := -st.over[1]
	if pull <= 0 {
		return
	}
	frac := min(pull/refreshPull, 1)
	t := p.Theme()
	cx := e.x + e.w/2
	now := float64(e.c.rt.now().UnixNano()) / 1e9
	if r.movesContent() {
		// iOS's spokes, in the room the content leaves, which appear one by
		// one with the pull, and turn while refreshing.
		cy := e.y + pull/2
		const spokes = 12
		shown := spokes
		if !r.Refreshing {
			shown = int(frac * spokes)
		}
		turn := 0
		if r.Refreshing {
			turn = int(now*spokes) % spokes
		}
		for i := range shown {
			a := float64(i)/spokes*2*math.Pi - math.Pi/2
			fade := float32(0.25)
			if r.Refreshing {
				fade = 0.2 + 0.8*float32((i-turn+spokes*2)%spokes)/spokes
			} else if frac >= 1 {
				fade = 0.6
			}
			sin, cos := math.Sincos(a)
			x0, y0 := cx+float32(cos)*5, cy+float32(sin)*5
			x1, y1 := cx+float32(cos)*10, cy+float32(sin)*10
			c := t.TextMuted
			c.A = uint8(float32(c.A) * fade)
			var path Path
			path.MoveTo(x0, y0).LineTo(x1, y1)
			p.StrokePath(&path, 2, c)
		}
		return
	}
	// Android's disc, which comes down over the content with an arc that
	// grows with the pull, and goes round while refreshing.
	const radius = 20
	cy := e.y + min(pull, refreshPull*1.4) - radius
	p.Fill(Rect{cx - radius, cy - radius, 2 * radius, 2 * radius}, t.Surface, radius)
	start, sweep := float32(-math.Pi/2), frac*float32(math.Pi)*1.6
	if r.Refreshing {
		start = float32(math.Mod(now*4, 2*math.Pi))
		sweep = float32(math.Pi) * (1 + 0.5*float32(math.Sin(now*3)))
	}
	var path Path
	const steps = 24
	for i := range steps + 1 {
		a := float64(start + sweep*float32(i)/steps)
		sin, cos := math.Sincos(a)
		x, y := cx+float32(cos)*9, cy+float32(sin)*9
		if i == 0 {
			path.MoveTo(x, y)
		} else {
			path.LineTo(x, y)
		}
	}
	p.StrokePath(&path, 2.5, t.Accent)
}
