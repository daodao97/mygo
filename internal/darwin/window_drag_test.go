//go:build darwin

package darwin

import "testing"

// A press that has not moved when its drag is asked goes to the window
// server, which moves the window: the drags that still reach the app are
// the window's, and MyGo leaves the window alone.
func TestWindowDragNative(t *testing.T) {
	var d windowDrag
	d.down(1, NSPoint{100, 50})
	if got := d.start(1, NSPoint{10, 500}); got != startNative {
		t.Fatalf("start = %v, want the window server's move", got)
	}
	// The window server moved the window by the time the drag came.
	if eat, move := d.dragged(NSPoint{140, 70}, NSPoint{50, 480}); !eat || move {
		t.Errorf("dragged while the window server moves the window = eat %v, move %v; want eaten, no move", eat, move)
	}
	if d.phase != dragNative {
		t.Errorf("phase = %v, want native", d.phase)
	}
	d.up()
	if d.phase != dragIdle {
		t.Errorf("phase after the release = %v, want idle", d.phase)
	}
}

// Issue #528: a quick press and drag has moved by the time the page asks
// for the drag. MyGo moves the window itself, at once to where the pointer
// is, and then with it.
func TestWindowDragMovedBeforeAsked(t *testing.T) {
	var d windowDrag
	d.down(1, NSPoint{100, 50})
	for _, p := range []NSPoint{{104, 52}, {120, 60}} {
		if eat, move := d.dragged(p, NSPoint{10, 500}); eat || move {
			t.Fatalf("a drag before the drag is asked = eat %v, move %v; want the content's", eat, move)
		}
	}
	if got := d.start(1, NSPoint{10, 500}); got != startManual {
		t.Fatalf("start = %v, want MyGo's move", got)
	}
	// Right 20, down 10 in Core Graphics' space: down 10 in AppKit's.
	if got, want := d.target(), (NSPoint{30, 490}); got != want {
		t.Errorf("target = %v, want %v", got, want)
	}
	eat, move := d.dragged(NSPoint{150, 40}, NSPoint{30, 490})
	if !eat || !move {
		t.Errorf("dragged = eat %v, move %v; want eaten and moved", eat, move)
	}
	if got, want := d.target(), (NSPoint{60, 510}); got != want {
		t.Errorf("target = %v, want %v", got, want)
	}
}

// The window server did not begin: the press's drags still reach the app
// and the window stays. Past the slop, MyGo moves the window itself.
func TestWindowDragNativeDidNotBegin(t *testing.T) {
	var d windowDrag
	origin := NSPoint{10, 500}
	d.down(1, NSPoint{100, 50})
	if got := d.start(1, origin); got != startNative {
		t.Fatalf("start = %v, want the window server's move", got)
	}
	if eat, move := d.dragged(NSPoint{101, 51}, origin); !eat || move {
		t.Errorf("dragged within the slop = eat %v, move %v; want eaten, no move", eat, move)
	}
	if eat, move := d.dragged(NSPoint{108, 50}, origin); !eat || !move {
		t.Errorf("dragged past the slop = eat %v, move %v; want eaten and moved", eat, move)
	}
	if d.phase != dragManual {
		t.Errorf("phase = %v, want manual", d.phase)
	}
	if got, want := d.target(), (NSPoint{18, 500}); got != want {
		t.Errorf("target = %v, want %v", got, want)
	}
}

// A drag is asked once a press, only for the window pressed, and only
// while the button is down.
func TestWindowDragAsked(t *testing.T) {
	var d windowDrag
	if got := d.start(1, NSPoint{}); got != startNone {
		t.Errorf("start without a press = %v, want none", got)
	}
	d.down(1, NSPoint{100, 50})
	if got := d.start(2, NSPoint{}); got != startNone {
		t.Errorf("start for another window = %v, want none", got)
	}
	if got := d.start(1, NSPoint{}); got != startNative {
		t.Errorf("start = %v, want the window server's move", got)
	}
	if got := d.start(1, NSPoint{}); got != startNone {
		t.Errorf("start again = %v, want none", got)
	}
	d.down(1, NSPoint{100, 50})
	d.up()
	if got := d.start(1, NSPoint{}); got != startNone {
		t.Errorf("start after the release = %v, want none", got)
	}
	// Drags of no press are the content's.
	if eat, move := d.dragged(NSPoint{200, 200}, NSPoint{}); eat || move {
		t.Errorf("dragged without a press = eat %v, move %v; want the content's", eat, move)
	}
	// A hand on a touchpad trembles: still enough for the window server.
	d.down(1, NSPoint{100, 50})
	d.dragged(NSPoint{100.5, 50.25}, NSPoint{})
	if got := d.start(1, NSPoint{}); got != startNative {
		t.Errorf("start after a tremble = %v, want the window server's move", got)
	}
}

func TestBelowMenuBar(t *testing.T) {
	for _, c := range []struct {
		origin NSPoint
		height float64
		want   NSPoint
	}{
		{NSPoint{10, 300}, 400, NSPoint{10, 300}}, // top at 700
		{NSPoint{10, 500}, 400, NSPoint{10, 475}}, // top at 900, past 875
		{NSPoint{-40, 475}, 400, NSPoint{-40, 475}},
	} {
		if got := belowMenuBar(c.origin, c.height, 875); got != c.want {
			t.Errorf("belowMenuBar(%v, %v, 875) = %v, want %v", c.origin, c.height, got, c.want)
		}
	}
}
