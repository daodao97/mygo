package platform

import (
	"math"
	"slices"
)

// Fingers on a touchscreen do what the mouse and a touchpad do, as on
// phones: a tap clicks where it lands, and one held there LongPress or
// longer right-clicks, as Windows's press and hold does; a finger moving
// further than TouchSlop scrolls the content under it along with it,
// and two pinch to zoom between them and scroll as they move together.
// The systems make the first finger the mouse, which pressed where it
// landed and selected text as it moved: backends take the touches
// instead and give them to a TouchGesture (GTK's touch events on Linux,
// WM_POINTER's on Windows).
const (
	TouchSlop = 8   // DIPs
	LongPress = 500 // milliseconds
)

// TouchGesture follows the fingers on a surface, by ids the backend
// gives them, and turns them into surface events. Its zero value has no
// finger down.
type TouchGesture struct {
	// fingers are down, in the order they landed; the first two count.
	fingers []touchFinger
	// tap tells that the only finger down stayed within TouchSlop of
	// where it landed, at startX, startY at time start.
	tap            bool
	startX, startY float64
	start          uint32
	// x, y is where the fingers last scrolled from: the finger, or the
	// midpoint of two, dist apart.
	x, y, dist float64
}

type touchFinger struct {
	id   uintptr
	x, y float64
}

// Reset forgets the fingers down, as when the first finger of new
// touches lands and those before ended unseen, as under a grab.
func (g *TouchGesture) Reset() { g.fingers, g.tap = g.fingers[:0], false }

// Holds reports whether the finger id is down.
func (g *TouchGesture) Holds(id uintptr) bool { return g.find(id) >= 0 }

// Begin puts a finger down at x, y, in DIPs, at time t, in milliseconds.
func (g *TouchGesture) Begin(id uintptr, x, y float64, t uint32) []SurfaceEvent {
	if g.find(id) >= 0 {
		return nil
	}
	g.fingers = append(g.fingers, touchFinger{id, x, y})
	g.tap = len(g.fingers) == 1
	if g.tap {
		g.startX, g.startY, g.start = x, y, t
	}
	g.anchor()
	return nil
}

// Move moves a finger to x, y.
func (g *TouchGesture) Move(id uintptr, x, y float64) []SurfaceEvent {
	i := g.find(id)
	if i < 0 {
		return nil
	}
	g.fingers[i].x, g.fingers[i].y = x, y
	var out []SurfaceEvent
	switch {
	case i >= 2:
	case len(g.fingers) == 1:
		if g.tap && math.Hypot(x-g.startX, y-g.startY) < TouchSlop {
			break
		}
		g.tap = false
		out = g.scroll(out, x, y)
	default:
		a, b := g.fingers[0], g.fingers[1]
		mx, my := (a.x+b.x)/2, (a.y+b.y)/2
		out = g.scroll(out, mx, my)
		if d := math.Hypot(a.x-b.x, a.y-b.y); d > 0 {
			if g.dist > 0 && d != g.dist {
				out = append(out, SurfaceEvent{Kind: PointerZoom, X: mx, Y: my, Scale: d / g.dist})
			}
			g.dist = d
		}
	}
	return out
}

// End lifts a finger at x, y at time t; canceled, as when the system
// takes the touch for its own gesture, it neither moves nor taps.
func (g *TouchGesture) End(id uintptr, x, y float64, t uint32, canceled bool) []SurfaceEvent {
	i := g.find(id)
	if i < 0 {
		return nil
	}
	var out []SurfaceEvent
	if !canceled {
		out = g.Move(id, x, y)
	}
	g.fingers = slices.Delete(g.fingers, i, i+1)
	if len(g.fingers) > 0 {
		g.anchor()
		return out
	}
	if g.tap && !canceled {
		button := 0
		if t-g.start >= LongPress {
			button = 1
		}
		out = append(out,
			SurfaceEvent{Kind: PointerDown, X: g.startX, Y: g.startY, Button: button},
			SurfaceEvent{Kind: PointerUp, X: g.startX, Y: g.startY, Button: button})
	}
	g.tap = false
	// Nothing hovers once the fingers are gone.
	return append(out, SurfaceEvent{Kind: PointerLeave})
}

// scroll scrolls the content at the last point along to x, y, following
// the fingers: the other way from a scroll bar's thumb.
func (g *TouchGesture) scroll(out []SurfaceEvent, x, y float64) []SurfaceEvent {
	if x != g.x || y != g.y {
		out = append(out, SurfaceEvent{Kind: PointerScroll, X: g.x, Y: g.y, DX: g.x - x, DY: g.y - y, Precise: true})
	}
	g.x, g.y = x, y
	return out
}

// anchor scrolls and pinches from where the fingers are now.
func (g *TouchGesture) anchor() {
	a := g.fingers[0]
	g.x, g.y, g.dist = a.x, a.y, 0
	if len(g.fingers) > 1 {
		b := g.fingers[1]
		g.x, g.y, g.dist = (a.x+b.x)/2, (a.y+b.y)/2, math.Hypot(a.x-b.x, a.y-b.y)
	}
}

func (g *TouchGesture) find(id uintptr) int {
	return slices.IndexFunc(g.fingers, func(f touchFinger) bool { return f.id == id })
}
