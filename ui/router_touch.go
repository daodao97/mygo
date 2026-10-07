package ui

import "time"

// A back gesture previews the previous entry without changing history or
// focus. The touch is reserved until it settles; only completion navigates.
type routeBack struct {
	from, to                  *routeEntry
	width, progress, velocity float32
	lastX                     float32
	moved, start              time.Time
	settling, complete        bool
	fromProgress              float32
	level                     int
}

func (rt *engine) backAt(x, y float32) (*Router, float32, int) {
	for _, id := range rt.hitChain(x, y) {
		s := rt.states[id]
		if s == nil || s.router == nil {
			continue
		}
		r := s.router
		view := &r.view
		if s.routerLevel > 0 {
			view, _ = s.locals["view"].(*routeView)
		}
		if view == nil || view.leaving != nil {
			continue // wait for this view's existing navigation to settle
		}
		if r.InteractiveBack && r.CanGoBack() && r.back == nil && r.view.leaving == nil &&
			x >= s.x && x-s.x <= 24 && s.w > 0 {
			from, to := r.current(), r.entries[r.at-1]
			level := 0
			for level < len(from.pages) && level < len(to.pages) && from.pages[level] == to.pages[level] {
				level++
			}
			if level == s.routerLevel {
				return r, s.w, level
			}
		}
	}
	return nil, 0, 0
}

func (r *Router) moveBack(x float32, now time.Time) {
	g := r.back
	if g == nil || g.settling {
		return
	}
	if dt := now.Sub(g.moved).Seconds(); dt > 0 && x != g.lastX {
		g.velocity = (x - g.lastX) / float32(dt)
	}
	g.progress = min(max(x/g.width, 0), 1)
	if x != g.lastX {
		g.lastX, g.moved = x, now
	}
	r.rt.requestFrame()
}

func (r *Router) endBack(cancelled bool, now time.Time) {
	g := r.back
	if g == nil || g.settling {
		return
	}
	velocity := g.velocity
	if now.Sub(g.moved) > 100*time.Millisecond {
		velocity = 0
	}
	g.complete = !cancelled && g.progress+velocity/g.width*0.15 >= 0.5
	g.settling, g.start, g.fromProgress = true, now, g.progress
	r.rt.requestFrame()
}

func (rt *engine) cancelTouchBack() {
	if r := rt.touch.back; r != nil {
		r.endBack(true, rt.now())
		rt.touch.back = nil
		rt.touch.backEntry = nil
		rt.touch.active = false
	}
}

func (r *Router) advanceBack(c *Context) {
	g := r.back
	if g.from != r.current() || !r.CanGoBack() || r.entries[r.at-1] != g.to {
		r.back = nil
		return
	}
	progress := g.progress
	if g.settling {
		target := float32(0)
		if g.complete {
			target = 1
		}
		duration := 180 * time.Millisecond
		elapsed := c.now.Sub(g.start)
		if elapsed >= duration || r.Transition == TransitionNone || c.rt.preferences().ReduceMotion {
			r.back = nil
			if g.complete {
				r.suppressTransitionAt = c.rt.frame
				r.Pop()
			}
			return
		}
		progress = g.fromProgress + (target-g.fromProgress)*EaseOut(float32(elapsed)/float32(duration))
		c.AnimationFrame()
	}
	g.progress = progress
}

func (r *Router) buildBack(c *Context, box *Element, v *routeView, parent *Route, fn func(*Route)) {
	g := r.back
	progress := g.progress
	box.Clip()
	bg := backdrop(box)
	box.Children(func() {
		lower := r.page(c, parent, g.to, fn, true)
		// Keep the current page's focus/IME alive during a cancellable
		// preview. Touch arbitration owns the contact and prevents clicks.
		upper := r.page(c, parent, g.from, fn, false)
		upper.Absolute().Top(0).Fill().Background(bg).LeftPercent(100*progress).Shadow(0, 0, 16, 0, RGBA(0, 0, 0, 0.15))
		lower.LeftPercent(-30 * (1 - progress)).Background(bg).DrawOver(func(p *Painter, rc Rect) {
			p.Fill(rc, RGBA(0, 0, 0, 0.06*(1-progress)), 0)
		})
		r.keep(c, box, v, g.level, g.from)
	})
}
