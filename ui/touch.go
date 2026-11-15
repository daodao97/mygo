package ui

import (
	"slices"

	"github.com/egoist/mygo/internal/platform"
)

// Touch is a finger on a touchscreen, on an element that asks about its
// fingers (Element.Touches).
type Touch struct {
	// ID tells the finger apart from the others for as long as it
	// touches, from TouchBegan to TouchEnded or TouchCanceled; the window
	// gives no other finger the same ID.
	ID uint64
	// X and Y are where the finger is, and StartX and StartY where it
	// landed, relative to the element's box.
	X, Y           float32
	StartX, StartY float32
	// Phase is what the finger did since the last build.
	Phase TouchPhase
	// Pressure is how hard the finger presses, from 0 to 1, where the
	// system tells it, else 0.5 while it touches, as Pointer Events'.
	Pressure float32
}

// TouchPhase is what a finger did since the last build (Touch.Phase).
type TouchPhase uint8

// Phases of a touch.
const (
	// TouchBegan is a finger that landed since the last build, and may
	// have moved since.
	TouchBegan TouchPhase = iota
	// TouchMoved is a finger that moved since the last build.
	TouchMoved
	// TouchStationary is a finger that stayed where it was.
	TouchStationary
	// TouchEnded is a finger that lifted since the last build, at X, Y:
	// the last build to see it. A finger that lands and lifts before a
	// build comes as TouchBegan in one build and TouchEnded in the next, so
	// that a view counting the fingers that land sees each one.
	TouchEnded
	// TouchCanceled is a finger the system took since the last build, as
	// for a gesture of its own at the screen's edge: no completed gesture,
	// and the last build to see it.
	TouchCanceled
)

// Touches returns the fingers on a touchscreen that landed on the element
// or on an element inside it, every one down and those that lifted or
// were canceled since the last build, in the order they landed, unless the
// element is disabled. Each keeps its ID for as long as it touches, so that
// a view tells them apart and recognizes gestures of its own, as a
// whiteboard turning the page as four fingers swipe or clearing it as five
// tap:
//
//	for _, t := range canvas.Touches() {
//		switch t.Phase {
//		case ui.TouchBegan, ui.TouchMoved:
//			strokes.Draw(t.ID, t.X, t.Y)
//		case ui.TouchEnded, ui.TouchCanceled:
//			strokes.End(t.ID)
//		}
//	}
//
// The fingers still do what they do without it, which apps that do not ask
// see alone: a tap clicks, one finger scrolls, two pinch. The window
// follows its fingers once a view asks: those down before, and lifting
// later, are not told of. Windows, Linux and browsers report touches;
// macOS has no touchscreens, and a trackpad's fingers, which are not points
// on the screen, report none. Elements asking for them get a frame
// whenever a finger lands, moves or lifts.
func (e *node) Touches() []Touch {
	if !e.hasState() || e.disabled() {
		return nil
	}
	rt := e.c.rt
	t, _ := rt.touchIn.(*touchTracker)
	if t == nil {
		// Set here, for apps that do not ask not to link it.
		t = &touchTracker{}
		rt.touchIn = t
	}
	s := e.st
	var out []Touch
	for i := range t.points {
		p := &t.points[i]
		if !slices.Contains(p.chain, s.id) {
			continue
		}
		touch := p.Touch
		touch.X, touch.Y = s.local(p.X, p.Y)
		touch.StartX, touch.StartY = s.local(p.StartX, p.StartY)
		out = append(out, touch)
	}
	return out
}

// touchInput follows the fingers on the window's touchscreen, once a view
// asked about them (Element.Touches), which sets it.
type touchInput interface {
	// event handles a TouchDown, TouchMove, TouchUp or TouchCancel, in
	// the content's DIPs.
	event(rt *engine, ev platform.SurfaceEvent)
	// endPass forgets the fingers that lifted, which the pass saw.
	endPass(rt *engine)
}

// touchTracker is the touchInput Element.Touches sets.
type touchTracker struct {
	// points are the fingers, in the order they landed, in the window's
	// DIPs.
	points []touchPoint
	// last is the last finger's ID.
	last uint64
}

type touchPoint struct {
	Touch
	// from is the platform's ID of the finger, which it may give
	// another once it lifts.
	from uint64
	// chain are the elements under the finger as it landed.
	chain []uint64
	// lift is TouchEnded or TouchCanceled for a finger that lifted, or was
	// taken, before a build saw it begin: it shows as TouchBegan to that
	// build, and lifts in the next.
	lift TouchPhase
}

func (t *touchTracker) event(rt *engine, ev platform.SurfaceEvent) {
	x, y := float32(ev.X), float32(ev.Y)
	pressure := float32(ev.Pressure)
	if pressure == 0 {
		pressure = 0.5
	}
	i := slices.IndexFunc(t.points, func(p touchPoint) bool { return p.from == ev.ID && p.Phase < TouchEnded && p.lift == 0 })
	switch ev.Kind {
	case platform.TouchDown:
		if i >= 0 {
			// Its end went unseen.
			t.points[i].Phase = TouchCanceled
		}
		t.last++
		t.points = append(t.points, touchPoint{
			Touch: Touch{ID: t.last, X: x, Y: y, StartX: x, StartY: y, Phase: TouchBegan, Pressure: pressure},
			from:  ev.ID, chain: rt.hitChain(x, y),
		})
	case platform.TouchMove:
		if i < 0 {
			return
		}
		p := &t.points[i]
		if p.X == x && p.Y == y && p.Pressure == pressure {
			return
		}
		p.X, p.Y, p.Pressure = x, y, pressure
		if p.Phase != TouchBegan {
			p.Phase = TouchMoved
		}
	default:
		if i < 0 {
			return
		}
		p := &t.points[i]
		phase := TouchCanceled
		if ev.Kind == platform.TouchUp {
			p.X, p.Y, phase = x, y, TouchEnded
		}
		if p.Phase == TouchBegan {
			p.lift = phase // as it began first, for a build to see
		} else {
			p.Phase = phase
		}
	}
	// Input, which the next frame builds even out of sight.
	rt.input = true
	rt.requestFrame()
}

func (t *touchTracker) endPass(rt *engine) {
	t.points = slices.DeleteFunc(t.points, func(p touchPoint) bool { return p.Phase >= TouchEnded })
	for i := range t.points {
		p := &t.points[i]
		if p.lift != 0 {
			// Seen to begin: it lifts in the next build.
			p.Phase, p.lift = p.lift, 0
			rt.input = true
			rt.askBuild() // in a frame, where requestFrame asks none
			continue
		}
		p.Phase = TouchStationary
	}
}
