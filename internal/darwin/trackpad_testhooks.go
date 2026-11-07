//go:build darwin

package darwin

import (
	"math"

	"github.com/ebitengine/purego"
)

// TestTrackpadDrag sends mouse events carrying NSEventSubtypeTouch, as an
// indirect trackpad does. It reports whether AppKit kept that subtype.
func TestTrackpadDrag(handle uintptr, points [][2]float64) bool {
	return testTrackpadMouse(handle, points, 1)
}

// TestTrackpadDoubleClick sends a trackpad mouse double click at (x, y).
func TestTrackpadDoubleClick(handle uintptr, x, y float64) bool {
	points := [][2]float64{{x, y}, {x, y}}
	return testTrackpadMouse(handle, points, 1) && testTrackpadMouse(handle, points, 2)
}

func testTrackpadMouse(handle uintptr, points [][2]float64, clicks int) (ok bool) {
	w := theBackend.byNSWindow[id(handle)]
	if w == nil || w.surface == nil || len(points) < 2 {
		return false
	}
	testSideOnce.Do(func() {
		purego.RegisterLibFunc(&cgEventSetIntegerValueField, libCG, "CGEventSetIntegerValueField")
	})
	testPinchOnce.Do(func() {
		purego.RegisterLibFunc(&cgEventSetType, libCG, "CGEventSetType")
		purego.RegisterLibFunc(&cgEventSetDoubleValueField, libCG, "CGEventSetDoubleValueField")
		purego.RegisterLibFunc(&cgEventSetLocation, libCG, "CGEventSetLocation")
	})
	withPool(func() {
		content := msgRect(send(w.win, "contentView"), sel("frame"))
		for i, p := range points {
			typ, name := uint(6), "mouseDragged:"
			if i == 0 {
				typ, name = 1, "mouseDown:"
			} else if i == len(points)-1 {
				typ, name = 2, "mouseUp:"
			}
			want := NSPoint{p[0], content.Size.Height - p[1]}
			ev := msgMouseEvent(class("NSEvent"), sel("mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:"),
				typ, want, 0, 0, sendInt(w.win, "windowNumber"), 0, 0, clicks, 1)
			cg := uintptr(send(ev, "CGEvent"))
			cgEventSetIntegerValueField(cg, 7, 3)             // kCGMouseEventSubtype: touch
			cgEventSetIntegerValueField(cg, 1, int64(clicks)) // kCGMouseEventClickState
			ev = send(class("NSEvent"), "eventWithCGEvent:", cg)
			if got := msgPoint(ev, sel("locationInWindow")); math.Abs(got.X-want.X) >= 0.5 || math.Abs(got.Y-want.Y) >= 0.5 {
				at := cgEventGetLocation(cg)
				cgEventSetLocation(cg, NSPoint{at.X - (got.X - want.X), at.Y + (got.Y - want.Y)})
				ev = send(class("NSEvent"), "eventWithCGEvent:", cg)
			}
			if ev == 0 || sendInt(ev, "type") != int(typ) || sendInt(ev, "subtype") != 3 || sendInt(ev, "clickCount") != clicks {
				return
			}
			send(w.surface.view, name, uintptr(ev))
		}
		ok = true
	})
	return ok
}
