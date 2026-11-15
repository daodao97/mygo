package ui

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// A view sees every finger on an element, by an ID that stays the same
// while the finger moves, until it lifts or the system takes it, as a
// whiteboard recognizing gestures of its own needs (#1879).
func TestTouches(t *testing.T) {
	var seen []Touch
	tt := NewTester(func(c *Context) {
		Column(c).Children(func() {
			Box(c).Size(300, 20)
			canvas := Box(c).Size(300, 200).Children(func() {
				Box(c).Size(50, 50) // a finger landing on it lands on the canvas
			})
			seen = canvas.Touches()
			Box(c).Size(300, 80)
		})
	}, 300, 300)
	tt.Frame()
	if seen != nil {
		t.Fatalf("no finger down: %+v", seen)
	}
	type finger struct {
		id    uint64
		x, y  float32
		phase TouchPhase
	}
	check := func(step string, want ...finger) {
		t.Helper()
		var got []finger
		for _, f := range seen {
			got = append(got, finger{f.ID, f.X, f.Y, f.Phase})
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n got %+v\nwant %+v", step, got, want)
		}
	}

	tt.TouchDown(1, 10, 30) // on the box in the canvas
	check("one down", finger{1, 10, 10, TouchBegan})
	tt.TouchDown(2, 100, 70)
	tt.TouchDown(3, 200, 170)
	check("three down", finger{1, 10, 10, TouchStationary}, finger{2, 100, 50, TouchStationary}, finger{3, 200, 150, TouchBegan})
	tt.TouchDown(7, 150, 250) // below the canvas
	check("one elsewhere", finger{1, 10, 10, TouchStationary}, finger{2, 100, 50, TouchStationary}, finger{3, 200, 150, TouchStationary})

	tt.TouchMove(2, 120, 80)
	check("moved", finger{1, 10, 10, TouchStationary}, finger{2, 120, 60, TouchMoved}, finger{3, 200, 150, TouchStationary})
	// Moving out of the canvas, the finger is still the canvas's.
	tt.TouchMove(2, 120, 290)
	check("moved out", finger{1, 10, 10, TouchStationary}, finger{2, 120, 270, TouchMoved}, finger{3, 200, 150, TouchStationary})
	if tt.rt.event(platform.SurfaceEvent{Kind: platform.TouchMove, ID: 2, X: 120, Y: 290}); tt.FramePending() {
		t.Error("a move to where the finger is asked for a frame")
	}

	tt.TouchUp(1, 12, 34)
	check("lifted", finger{1, 12, 14, TouchEnded}, finger{2, 120, 270, TouchStationary}, finger{3, 200, 150, TouchStationary})
	tt.TouchCancel(3)
	check("canceled", finger{2, 120, 270, TouchStationary}, finger{3, 200, 150, TouchCanceled})
	tt.Frame()
	check("gone", finger{2, 120, 270, TouchStationary})

	// The platform gives the ID of a finger that lifted to the next: the
	// window gives it one of its own.
	tt.TouchDown(1, 40, 40)
	check("ID used again", finger{2, 120, 270, TouchStationary}, finger{5, 40, 20, TouchBegan})
	if s := seen[1]; s.StartX != 40 || s.StartY != 20 || s.Pressure != 0.5 {
		t.Errorf("start %v, %v, pressure %v, want 40, 20, 0.5", s.StartX, s.StartY, s.Pressure)
	}

	// A finger landing and moving between two builds comes as it began,
	// where it is, with where it landed.
	tt.rt.event(platform.SurfaceEvent{Kind: platform.TouchDown, ID: 8, X: 60, Y: 60, Pressure: 0.25})
	tt.rt.event(platform.SurfaceEvent{Kind: platform.TouchMove, ID: 8, X: 90, Y: 100, Pressure: 0.25})
	tt.Frame()
	if s := seen[len(seen)-1]; s.ID != 6 || s.Phase != TouchBegan || s.X != 90 || s.Y != 80 || s.StartX != 60 || s.StartY != 40 || s.Pressure != 0.25 {
		t.Errorf("landed and moved: %+v", s)
	}
	// One landing and lifting between two builds comes as it began, and
	// lifts in the build after.
	tt.rt.event(platform.SurfaceEvent{Kind: platform.TouchDown, ID: 9, X: 60, Y: 60})
	tt.rt.event(platform.SurfaceEvent{Kind: platform.TouchUp, ID: 9, X: 60, Y: 60})
	tt.Frame() // which builds twice (TestTouchesTapBetweenBuilds)
	if s := seen[len(seen)-1]; s.ID != 7 || s.Phase != TouchEnded {
		t.Errorf("landed and lifted, then built again: %+v", s)
	}
	// Unknown fingers, as those down before a view asked, do nothing.
	tt.TouchMove(42, 10, 10)
	tt.TouchUp(42, 10, 10)
	check("unknown", finger{2, 120, 270, TouchStationary}, finger{5, 40, 20, TouchStationary}, finger{6, 90, 80, TouchStationary})
}

// Fingers counted as they land and lift: a five-finger tap, as a
// whiteboard clears itself with.
func TestTouchesFiveFingerTap(t *testing.T) {
	down, most, cleared := map[uint64]bool{}, 0, 0
	tt := NewTester(func(c *Context) {
		for _, f := range Box(c).Size(400, 400).Touches() {
			switch f.Phase {
			case TouchBegan:
				down[f.ID] = true
				most = max(most, len(down))
			case TouchEnded, TouchCanceled:
				delete(down, f.ID)
				if len(down) == 0 && most == 5 && f.Phase == TouchEnded {
					cleared++
				}
				if len(down) == 0 {
					most = 0
				}
			}
		}
	}, 400, 400)
	tt.Frame()
	for i := range uint64(5) {
		tt.TouchDown(i, float32(50+50*i), 200)
	}
	for i := range uint64(5) {
		tt.TouchUp(i, float32(50+50*i), 200)
	}
	if cleared != 1 {
		t.Errorf("five fingers tapped: cleared %d times", cleared)
	}
}

// Views that ask about no fingers see no difference: touches build no
// frame, and the taps the backend makes of them click as they did.
func TestTouchesUnasked(t *testing.T) {
	clicks := 0
	tt := NewTester(func(c *Context) {
		if Box(c).Size(100, 100).Label("pad").Clicked() {
			clicks++
		}
	}, 200, 200)
	tt.Frame()
	for _, kind := range []platform.SurfaceEventKind{platform.TouchDown, platform.TouchMove, platform.TouchUp} {
		if tt.rt.event(platform.SurfaceEvent{Kind: kind, ID: 1, X: 50, Y: 50}); tt.FramePending() {
			t.Errorf("touch %d nothing asked about asked for a frame", kind)
		}
	}
	if tt.rt.touchIn != nil {
		t.Error("touches followed while nothing asked")
	}
	tt.UsePointer(PointerTouch, 0)
	tt.ClickAt(50, 50)
	if clicks != 1 {
		t.Errorf("a tap clicked %d times", clicks)
	}
}

// A disabled element tells of no finger.
func TestTouchesDisabled(t *testing.T) {
	disabled := false
	var seen []Touch
	tt := NewTester(func(c *Context) {
		seen = Box(c).Size(100, 100).Disabled(disabled).Touches()
	}, 200, 200)
	tt.Frame()
	tt.TouchDown(1, 50, 50)
	if len(seen) != 1 {
		t.Fatalf("enabled: %+v", seen)
	}
	if tt.rt.event(platform.SurfaceEvent{Kind: platform.TouchMove, ID: 1, X: 60, Y: 50}); !tt.FramePending() {
		t.Error("a finger moving asked for no frame")
	}
	disabled = true
	tt.Frame()
	if seen != nil {
		t.Errorf("disabled: %+v", seen)
	}
}

// A finger landing and lifting before a build, as a quick tap, or as
// Windows sends a two-finger tap once it decided it is no gesture of its
// own, is seen to begin in one build and to end in the next.
func TestTouchesTapBetweenBuilds(t *testing.T) {
	var phases []TouchPhase
	tt := NewTester(func(c *Context) {
		for _, f := range Box(c).Size(200, 200).Touches() {
			phases = append(phases, f.Phase)
		}
	}, 200, 200)
	tt.Frame()
	tt.rt.event(platform.SurfaceEvent{Kind: platform.TouchDown, ID: 1, X: 50, Y: 50})
	tt.rt.event(platform.SurfaceEvent{Kind: platform.TouchDown, ID: 2, X: 90, Y: 50})
	tt.rt.event(platform.SurfaceEvent{Kind: platform.TouchUp, ID: 1, X: 50, Y: 50})
	tt.rt.event(platform.SurfaceEvent{Kind: platform.TouchCancel, ID: 2})
	// The platform gives the ID to a finger landing before the build.
	tt.rt.event(platform.SurfaceEvent{Kind: platform.TouchDown, ID: 1, X: 120, Y: 50})
	for range 3 {
		tt.Frame()
	}
	want := []TouchPhase{TouchBegan, TouchBegan, TouchBegan, TouchEnded, TouchCanceled, TouchStationary}
	if len(phases) < len(want) || !reflect.DeepEqual(phases[:len(want)], want) {
		t.Errorf("phases %v, want %v", phases, want)
	}
}
