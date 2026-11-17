package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

// A handler that takes a long press takes the finger: it hears the lift,
// or its cancel, and nothing clicks.
func TestLongPressInputTakesTheFinger(t *testing.T) {
	var kinds []InputKind
	var canceled bool
	clicks := 0
	tt := NewTester(func(c *Context) {
		b := Box(c).Size(200, 200).HandleInput(func(ev InputEvent) bool {
			switch {
			case ev.Kind == InputLongPress:
				kinds = append(kinds, ev.Kind)
				return true
			case ev.Kind == InputPointerUp && len(kinds) > 0:
				kinds = append(kinds, ev.Kind)
				canceled = ev.Canceled
			}
			return false
		})
		if b.Clicked() {
			clicks++
		}
	}, 300, 300)
	tt.Frame()
	tt.FingerDown(50, 50)
	tt.Advance(600 * time.Millisecond)
	if len(kinds) != 1 || kinds[0] != InputLongPress {
		t.Fatalf("held: %v", kinds)
	}
	tt.FingerMove(53, 52)
	tt.FingerUp(53, 52)
	if len(kinds) != 2 || kinds[1] != InputPointerUp || canceled || clicks != 0 {
		t.Fatalf("lifted: %v canceled=%v clicks=%d", kinds, canceled, clicks)
	}

	kinds = nil
	tt.FingerDown(50, 50)
	tt.Advance(600 * time.Millisecond)
	tt.FingerCancel()
	if len(kinds) != 2 || !canceled || clicks != 0 {
		t.Fatalf("canceled: %v canceled=%v clicks=%d", kinds, canceled, clicks)
	}

	// A quick tap is a click, and no long press.
	kinds = nil
	tt.FingerDown(50, 50)
	tt.FingerUp(50, 50)
	tt.Frame()
	if len(kinds) != 0 || clicks != 1 {
		t.Fatalf("tap: %v clicks=%d", kinds, clicks)
	}
}

// A handler that does not take it leaves the long press to what it does.
func TestLongPressInputDeclined(t *testing.T) {
	heard, pressed := 0, false
	tt := NewTester(func(c *Context) {
		b := Box(c).Size(200, 200).HandleInput(func(ev InputEvent) bool {
			if ev.Kind == InputLongPress {
				heard++
			}
			return false
		})
		if b.LongPressed() {
			pressed = true
		}
	}, 300, 300)
	tt.Frame()
	tt.FingerDown(50, 50)
	tt.Advance(600 * time.Millisecond)
	tt.FingerUp(50, 50)
	tt.Frame()
	if heard != 1 || !pressed {
		t.Fatalf("heard %d, LongPressed %v", heard, pressed)
	}
}

// A finger on what asks for TouchPointer drives the pointer, and scrolls
// nothing under it.
func TestTouchPointerDrags(t *testing.T) {
	moves := 0
	var scroll ScrollState
	tt := NewTester(func(c *Context) {
		Scroll(c).Fill().TrackScroll(&scroll).Children(func() {
			Box(c).Size(300, 2000).TouchPointer().HandleInput(func(ev InputEvent) bool {
				if ev.Kind == InputPointerMove {
					moves++
				}
				return false
			})
		})
	}, 300, 300)
	tt.Frame()
	tt.FingerDown(100, 200)
	tt.FingerMove(100, 150)
	tt.FingerMove(100, 80)
	tt.FingerUp(100, 80)
	tt.Frame()
	if scroll.Y != 0 || moves == 0 {
		t.Fatalf("scroll %v, moves %d", scroll.Y, moves)
	}
}

// Panning takes the focus from a text input that asks, and from no other.
func TestKeyboardDismissOnPan(t *testing.T) {
	for _, mode := range []KeyboardDismissMode{KeyboardDismissNone, KeyboardDismissOnDrag, KeyboardDismissInteractive} {
		value := ""
		tt := NewTester(func(c *Context) {
			Column(c).Fill().Children(func() {
				TextInput(c, &value).Height(44).AutoFocus().KeyboardDismiss(mode)
				Scroll(c).Grow(1).Children(func() { Box(c).Size(300, 2000) })
			})
		}, 300, 400)
		tt.Frame()
		if tt.rt.focused == 0 {
			t.Fatalf("%q: the input took no focus", mode)
		}
		tt.FingerDown(150, 250)
		tt.FingerMove(150, 200)
		tt.FingerMove(150, 120)
		tt.FingerUp(150, 120)
		tt.Frame()
		if kept := tt.rt.focused != 0; kept != (mode == KeyboardDismissNone) {
			t.Errorf("%q: focus kept %v", mode, kept)
		}
	}
}

// Two fingers come to what asks for Gestures as deltas from where they
// began, and to nothing else.
func TestGestures(t *testing.T) {
	var got []GestureEvent
	tt := NewTester(func(c *Context) {
		Column(c).Children(func() {
			got = append(got, Box(c).Size(200, 200).Gestures()...)
			Box(c).Size(200, 100)
		})
	}, 300, 300)
	tt.Frame()
	send := func(kind, phase string, x, y, scale, dx, dy float64) bool {
		return tt.rt.event(platform.SurfaceEvent{Kind: platform.SurfaceGesture, Gesture: kind, Phase: phase, X: x, Y: y, Scale: scale, DX: dx, DY: dy})
	}
	if send("pinch", "probe", 100, 250, 1, 0, 0) {
		t.Fatal("a probe outside was taken")
	}
	if !send("pinch", "probe", 100, 100, 1, 0, 0) {
		t.Fatal("a probe inside was not taken")
	}
	send("pinch", "begin", 100, 100, 1, 0, 0)
	send("pan", "begin", 100, 100, 1, 0, 0)
	send("pinch", "change", 100, 100, 1.5, 0, 0)
	send("pinch", "change", 100, 100, 3, 0, 0)
	send("pan", "change", 110, 100, 1, 10, 4)
	send("pan", "change", 115, 100, 1, 15, 4)
	tt.Frame()
	var scales, dxs []float32
	for _, g := range got {
		if g.Phase != "change" {
			continue
		}
		switch g.Kind {
		case "pinch":
			scales = append(scales, g.Scale)
		case "pan":
			dxs = append(dxs, g.DX)
		}
	}
	if len(scales) != 2 || scales[0] != 1.5 || scales[1] != 2 || len(dxs) != 2 || dxs[0] != 10 || dxs[1] != 5 {
		t.Fatalf("scales %v, pans %v (%+v)", scales, dxs, got)
	}
	got = nil
	send("pinch", "end", 100, 100, 1, 0, 0)
	if !send("tap", "begin", 100, 100, 1, 0, 0) || !send("tap", "end", 100, 100, 1, 0, 0) {
		t.Fatal("a tap was not taken")
	}
	tt.Frame()
	if len(got) != 3 || got[2].Kind != "tap" || got[2].Phase != "end" {
		t.Fatalf("ended and tapped: %+v", got)
	}
}

// A window that extends into the safe area keeps its root out of the insets'
// padding, and still reads them.
func TestExtendIntoSafeArea(t *testing.T) {
	for _, own := range []bool{false, true} {
		var top, bottom float32
		tt := NewTester(func(c *Context) {
			top, _, bottom, _ = c.SafeArea()
			Box(c).Size(100, 20).Label("first")
		}, 300, 600)
		tt.SetExtendIntoSafeArea(own)
		tt.SetInsets(44, 0, 300, 0)
		r, ok := tt.Find("first")
		if !ok {
			t.Fatal("no box")
		}
		want := float32(44)
		if own {
			want = 0
		}
		if r.Y != want || top != 44 || bottom != 300 {
			t.Errorf("own=%v: box at %v, want %v; safe area %v, %v", own, r.Y, want, top, bottom)
		}
	}
}
