//go:build darwin && !ios

package darwin

import (
	"testing"

	"github.com/ebitengine/purego"
	"github.com/egoist/mygo/internal/platform"
)

type swipeStep struct {
	phase  int
	dx, dy float64
	taken  bool
}

func TestSwipeTrack(t *testing.T) {
	gesture := func(dx, dy float64) []swipeStep {
		return []swipeStep{{phase: phaseBegan, dx: dx / 4, dy: dy / 4}, {phase: phaseChanged, dx: dx / 4, dy: dy / 4}, {phase: phaseChanged, dx: dx / 2, dy: dy / 2}, {phase: phaseEnded}}
	}
	cases := []struct {
		name     string
		steps    []swipeStep
		back, ok bool
	}{
		{"toward the left goes back", gesture(80, 10), true, true},
		{"toward the right goes forward", gesture(-80, -10), false, true},
		{"too short", gesture(40, 0), false, false},
		{"too steep", gesture(80, 40), false, false},
		{"vertical", gesture(0, 200), false, false},
		{"taken sideways", []swipeStep{{phase: phaseBegan, dx: 40}, {phase: phaseChanged, dx: 40, taken: true}, {phase: phaseEnded}}, false, false},
		{"cancelled", []swipeStep{{phase: phaseBegan, dx: 40}, {phase: phaseChanged, dx: 40}, {phase: phaseCancelled}}, false, false},
		{"back and forth", []swipeStep{{phase: phaseBegan, dx: 80}, {phase: phaseChanged, dx: -70}, {phase: phaseEnded}}, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var tr swipeTrack
			for i, st := range c.steps {
				back, ok := tr.scroll(st.phase, st.dx, st.dy, st.taken)
				last := i == len(c.steps)-1
				if !last && ok {
					t.Fatalf("step %d swiped before the fingers left", i)
				}
				if last && (back != c.back || ok != c.ok) {
					t.Fatalf("swipe = %v, %v, want %v, %v", back, ok, c.back, c.ok)
				}
			}
		})
	}
	// A taken gesture leaves the next one free.
	var tr swipeTrack
	tr.scroll(phaseBegan, 80, 0, true)
	tr.scroll(phaseEnded, 0, 0, false)
	tr.scroll(phaseBegan, 80, 0, false)
	if back, ok := tr.scroll(phaseEnded, 0, 0, false); !ok || !back {
		t.Fatalf("the gesture after a taken one: swipe = %v, %v, want back", back, ok)
	}
}

type swipeHandler struct {
	platform.WindowHandler
	sideways bool // whether the content takes scrolls sideways
	events   []platform.SurfaceEvent
}

func (h *swipeHandler) SurfaceEvent(ev platform.SurfaceEvent) bool {
	h.events = append(h.events, ev)
	return ev.Kind == platform.PointerScroll && h.sideways
}

func (h *swipeHandler) keys() []platform.SurfaceEvent {
	var keys []platform.SurfaceEvent
	for _, ev := range h.events {
		if ev.Kind == platform.KeyPressed || ev.Kind == platform.KeyReleased {
			keys = append(keys, ev)
		}
	}
	return keys
}

// TestTwoFingerSwipe sends a touchpad's scroll gesture, as AppKit does
// with the touchpad set to swipe between pages with two fingers, to a
// surface: it goes back once its fingers leave, unless the content took
// it sideways.
func TestTwoFingerSwipe(t *testing.T) {
	load()
	enabled := true
	defer func(f func() bool) { swipeFromScrolls = f }(swipeFromScrolls)
	swipeFromScrolls = func() bool { return enabled }
	var createScroll func(uintptr, uint32, uint32, int32) uintptr
	purego.RegisterLibFunc(&createScroll, libCG, "CGEventCreateScrollWheelEvent")
	var setInt func(uintptr, uint32, int64)
	purego.RegisterLibFunc(&setInt, libCG, "CGEventSetIntegerValueField")
	var release func(uintptr)
	purego.RegisterLibFunc(&release, libCG, "CFRelease")
	withPool(func() {
		view := msgInitRect(send(class("NSView"), "alloc"), sel("initWithFrame:"), NSRect{Size: NSSize{Width: 400, Height: 300}})
		defer send(view, "release")
		swipe := func(h *swipeHandler, dx int32) {
			s := &surface{w: &window{h: h}, view: view}
			// kCGScrollPhaseBegan, Changed, Ended
			for i, phase := range []int64{1, 2, 2, 4} {
				d := dx
				if i == 3 {
					d = 0
				}
				cg := createScroll(0, 0, 1, 0) // kCGScrollEventUnitPixel
				// The wheels' second axis, varargs of
				// CGEventCreateScrollWheelEvent, set as fields.
				setInt(cg, 12, int64(d)) // kCGScrollWheelEventDeltaAxis2
				setInt(cg, 97, int64(d)) // kCGScrollWheelEventPointDeltaAxis2
				setInt(cg, 99, phase)    // kCGScrollWheelEventScrollPhase
				ev := send(class("NSEvent"), "eventWithCGEvent:", cg)
				release(cg)
				if i == 0 && msgFloat(ev, sel("scrollingDeltaX")) != float64(dx) {
					t.Fatalf("scrollingDeltaX = %v, want %v", msgFloat(ev, sel("scrollingDeltaX")), dx)
				}
				scrollWheel(s, ev)
			}
		}
		h := &swipeHandler{}
		swipe(h, 30)
		keys := h.keys()
		if len(keys) != 2 || keys[0].Kind != platform.KeyPressed || keys[0].Key != platform.KeyBack || keys[1].Kind != platform.KeyReleased || keys[1].Key != platform.KeyBack {
			t.Fatalf("keys after a swipe toward the left = %+v, want KeyBack pressed and released", keys)
		}
		for _, ev := range h.events {
			if ev.Kind == platform.PointerScroll && !ev.Swipe {
				t.Fatalf("scroll of a swipe = %+v, want Swipe", ev)
			}
		}
		h = &swipeHandler{}
		swipe(h, -30)
		if keys := h.keys(); len(keys) != 2 || keys[0].Key != platform.KeyForward {
			t.Fatalf("keys after a swipe toward the right = %+v, want KeyForward", keys)
		}
		h = &swipeHandler{sideways: true}
		swipe(h, 30)
		if keys := h.keys(); len(keys) != 0 {
			t.Fatalf("keys after a swipe the content took = %+v, want none", keys)
		}
		enabled = false
		h = &swipeHandler{}
		swipe(h, 30)
		if keys := h.keys(); len(keys) != 0 {
			t.Fatalf("keys after a swipe with swiping between pages off = %+v, want none", keys)
		}
		for _, ev := range h.events {
			if ev.Swipe {
				t.Fatalf("scroll with swiping between pages off = %+v, want no Swipe", ev)
			}
		}
	})
}
