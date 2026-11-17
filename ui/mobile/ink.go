package mobile

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// inkGrow is how long a ripple takes to cover what was pressed, and inkFade
// how long it takes to go after the press.
const (
	inkGrow = 225 * time.Millisecond
	inkFade = 150 * time.Millisecond
)

type inkState struct {
	x, y     float32
	at, gone time.Time
	on       bool
}

// Ink draws Material's ripple over the element as it is pressed, spreading
// from where the finger went down, and returns it. The element clips it to its
// corners if it clips.
func Ink(e ui.Element) ui.Element {
	s := ui.Local(e, inkKey{}, func() inkState { return inkState{} })
	pressed := e.Pressed()
	switch {
	case pressed && !s.on:
		s.on = true
		s.x, s.y, _ = e.PointerPosition()
		s.at, s.gone = time.Time{}, time.Time{}
	case !pressed && s.on:
		s.on = false
		s.gone = time.Time{}
	}
	color := ui.RGBA(128, 128, 128, 0.2)
	e.DrawOver(func(p *ui.Painter, r ui.Rect) {
		now := p.Now()
		if s.at.IsZero() {
			if !s.on && s.gone.IsZero() {
				return
			}
			s.at = now
		}
		if !s.on && s.gone.IsZero() {
			s.gone = now
		}
		alpha := float32(1)
		if !s.gone.IsZero() {
			alpha = 1 - float32(now.Sub(s.gone))/float32(inkFade)
			if alpha <= 0 {
				s.at = time.Time{}
				return
			}
		}
		grow := min(1, float32(now.Sub(s.at))/float32(inkGrow))
		grow = 1 - (1-grow)*(1-grow)
		far := float32(math.Hypot(float64(max(s.x, r.W-s.x)), float64(max(s.y, r.H-s.y))))
		radius := far * grow
		c := color
		c.A = uint8(float32(c.A) * alpha)
		// The element's box holds the circle.
		p.Clip(r, 0, func() {
			var path ui.Path
			path.Circle(r.X+s.x, r.Y+s.y, radius)
			p.FillPath(&path, c)
		})
		p.After(16 * time.Millisecond)
	})
	return e
}

type inkKey struct{}
