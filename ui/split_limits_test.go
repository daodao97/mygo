package ui

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// limitsPanes builds the panes of a split as boxes filling them, named
// first and second, so that their boxes are the panes'.
func limitsPanes(c *context) (first, second func()) {
	return func() { coreBox(c).Fill().Label("first") }, func() { coreBox(c).Fill().Label("second") }
}

// laidPanes returns the boxes the layout gave the panes of the split at the
// root of tt's view, which the window doesn't cut.
func laidPanes(tt *Tester) (first, second Rect) {
	in := tt.rt.c.root.first.first
	a, b := in.first, in.first.next.next
	return Rect{a.x, a.y, a.w, a.h}, Rect{b.x, b.y, b.w, b.h}
}

// TestSplitLimitsDrag stops the divider at either pane's limits, and keeps
// how far the pointer went past them: coming back moves the divider only
// once the pointer is over it again, as at the 40 DIPs every pane keeps
// (#1474).
func TestSplitLimitsDrag(t *testing.T) {
	size := float32(300)
	tt := coreNewTester(func(c *context) {
		first, second := limitsPanes(c)
		coreSplit(c, &size, first, second).SplitLimits(SplitLimits{FirstMin: 200, FirstMax: 400, SecondMin: 250}).Fill()
	}, 800, 200)
	x, y := float32(302), float32(100)
	tt.Press(x, y)
	// 100 DIPs to first's maximum, and 200 past it.
	tt.Move(x+300, y)
	if size != 400 {
		t.Fatalf("dragged past first's maximum, it is %v wide, want 400", size)
	}
	if r := at(t, tt, "second"); r.X != 401 || r.W != 399 {
		t.Errorf("first at its maximum, second is at %v, %v wide, want at 401, 399 wide", r.X, r.W)
	}
	tt.Move(x+150, y)
	if size != 400 {
		t.Errorf("150 DIPs back, still 50 past the divider, first is %v wide", size)
	}
	tt.Move(x+50, y)
	if size != 350 {
		t.Errorf("100 more back, 50 over the divider, first is %v wide, want 350", size)
	}
	// On to first's minimum, 150 DIPs on, and 100 past it.
	tt.Move(x-200, y)
	if size != 200 {
		t.Errorf("dragged past first's minimum, it is %v wide, want 200", size)
	}
	tt.Move(x-150, y)
	if size != 200 {
		t.Errorf("50 DIPs back, still 50 past the divider, first is %v wide", size)
	}
	tt.Move(x-50, y)
	if size != 250 {
		t.Errorf("100 more back, 50 over the divider, first is %v wide, want 250", size)
	}
	tt.Release(x-50, y)

	// Second's minimum stops it before first's maximum, in a window
	// narrower than both: 599 DIPs of room leave first 349.
	tt.SetSize(600, 200)
	x = 252
	tt.Press(x, y)
	tt.Move(x+200, y)
	if size != 349 {
		t.Errorf("dragged past second's minimum, first is %v wide, want 349", size)
	}
	if r := at(t, tt, "second"); r.W != 250 {
		t.Errorf("at its minimum, second is %v wide, want 250", r.W)
	}
	tt.Move(x+120, y)
	if size != 349 {
		t.Errorf("80 DIPs back, still 21 past the divider, first is %v wide", size)
	}
	tt.Move(x+80, y)
	if size != 330 {
		t.Errorf("40 more back, 19 over the divider, first is %v wide, want 330", size)
	}
	tt.Release(x+80, y)
}

// TestSplitLimitsSecondMax makes first take the room second can't, at its
// maximum, as the use case of #2041: a side panel between 280 and 480 DIPs
// after a chat no narrower than 360.
func TestSplitLimitsSecondMax(t *testing.T) {
	size := float32(500)
	tt := coreNewTester(func(c *context) {
		first, second := limitsPanes(c)
		coreSplit(c, &size, first, second).SplitLimits(SplitLimits{FirstMin: 360, SecondMin: 280, SecondMax: 480}).Fill()
	}, 1201, 200)
	// 1200 DIPs of room: second at 480 leaves first 720.
	if r := at(t, tt, "first"); r.W != 720 {
		t.Fatalf("second at its maximum, first is %v wide, want 720", r.W)
	}
	if r := at(t, tt, "second"); r.X != 721 || r.W != 480 {
		t.Fatalf("second is at %v, %v wide, want at 721, 480 wide", r.X, r.W)
	}
	// The divider moves from where it shows: toward first, second would
	// pass its maximum, and the divider waits there until the pointer comes
	// back over it.
	x, y := float32(722), float32(100)
	tt.Press(x, y)
	tt.Move(x-100, y)
	if size != 720 {
		t.Errorf("dragged toward first, past second's maximum, first is %v wide, want 720", size)
	}
	tt.Move(x+30, y)
	if size != 750 {
		t.Errorf("back over the divider and 30 on, first is %v wide, want 750", size)
	}
	tt.Release(x+30, y)
	if r := at(t, tt, "second"); r.W != 450 {
		t.Errorf("second is %v wide, want 450", r.W)
	}
	// The arrows stop at second's maximum too.
	tt.Key(0, KeyLeft)
	tt.Key(0, KeyLeft)
	tt.Key(0, KeyLeft)
	tt.Key(0, KeyLeft)
	if size != 720 {
		t.Errorf("Left four times from 750, first is %v wide, want 720", size)
	}
}

// TestSplitLimitsKeys moves the divider with the arrows as far as the
// limits, side by side and one above the other.
func TestSplitLimitsKeys(t *testing.T) {
	for _, vertical := range []bool{false, true} {
		size := float32(205)
		tt := coreNewTester(func(c *context) {
			first, second := limitsPanes(c)
			l := SplitLimits{FirstMin: 200, FirstMax: 300}
			if vertical {
				coreSplitVertical(c, &size, first, second).SplitLimits(l).Fill()
			} else {
				coreSplit(c, &size, first, second).SplitLimits(l).Fill()
			}
		}, 600, 600)
		back, forth := KeyLeft, KeyRight
		if vertical {
			back, forth = KeyUp, KeyDown
			tt.ClickAt(300, 207)
		} else {
			tt.ClickAt(207, 300)
		}
		tt.Key(0, back)
		if size != 200 {
			t.Errorf("vertical %v: back 10 DIPs from 205, first is %v, want its minimum, 200", vertical, size)
		}
		tt.Key(0, back)
		if size != 200 {
			t.Errorf("vertical %v: back past its minimum, first is %v", vertical, size)
		}
		for range 12 {
			tt.Key(0, forth)
		}
		if size != 300 {
			t.Errorf("vertical %v: forth past its maximum, first is %v, want 300", vertical, size)
		}
		r := at(t, tt, "second")
		if !vertical && r.X != 301 || vertical && r.Y != 301 {
			t.Errorf("vertical %v: second starts at %v", vertical, r)
		}
	}
}

// TestSplitLimitsVerticalDrag is TestSplitLimitsDrag one pane above the
// other.
func TestSplitLimitsVerticalDrag(t *testing.T) {
	size := float32(150)
	tt := coreNewTester(func(c *context) {
		first, second := limitsPanes(c)
		coreSplitVertical(c, &size, first, second).SplitLimits(SplitLimits{FirstMin: 100, SecondMin: 200, SecondMax: 300}).Fill()
	}, 300, 501)
	// 500 DIPs of room, second at most 300: first is at least 200.
	if r := at(t, tt, "second"); r.Y != 201 || r.H != 300 {
		t.Fatalf("second is at %v, %v high, want at 201, 300 high", r.Y, r.H)
	}
	x, y := float32(100), float32(202)
	tt.Press(x, y)
	tt.Move(x, y+200)
	if size != 300 {
		t.Errorf("dragged down past second's minimum, first is %v high, want 300", size)
	}
	tt.Move(x, y+150)
	if size != 300 {
		t.Errorf("back to 50 DIPs past the divider, first is %v high", size)
	}
	tt.Move(x, y+50)
	if size != 250 {
		t.Errorf("back over the divider and 50 on, first is %v high, want 250", size)
	}
	tt.Move(x, y-100)
	if size != 200 {
		t.Errorf("dragged up past second's maximum, first is %v high, want 200", size)
	}
	tt.Release(x, y-100)
}

// TestSplitLimitsResize keeps both panes within their limits as the window
// changes, and *size as it was.
func TestSplitLimitsResize(t *testing.T) {
	size := float32(300)
	tt := coreNewTester(func(c *context) {
		first, second := limitsPanes(c)
		coreSplit(c, &size, first, second).SplitLimits(SplitLimits{FirstMin: 200, SecondMin: 250, SecondMax: 400}).Fill()
	}, 1001, 200)
	if r := at(t, tt, "second"); r.X != 601 || r.W != 400 {
		t.Errorf("1000 DIPs of room: second at %v, %v wide, want at 601, 400 wide", r.X, r.W)
	}
	tt.SetSize(601, 200)
	if r := at(t, tt, "second"); r.X != 301 || r.W != 300 {
		t.Errorf("600 DIPs of room: second at %v, %v wide, want at 301, 300 wide", r.X, r.W)
	}
	tt.SetSize(501, 200)
	if r := at(t, tt, "second"); r.X != 251 || r.W != 250 {
		t.Errorf("500 DIPs of room: second at %v, %v wide, want at 251, 250 wide", r.X, r.W)
	}
	// Assistive technology hears the size that shows, and the range the
	// limits leave.
	tt.rt.accessibilityOn()
	tt.Frame()
	for _, n := range tt.h.access.Nodes {
		if n.Role == platform.RoleSplitter && (n.Min != 200 || n.Max != 250 || n.Now != 250) {
			t.Errorf("the divider says %v from %v to %v, want 250 from 200 to 250", n.Now, n.Min, n.Max)
		}
	}
	tt.SetSize(801, 200)
	if r := at(t, tt, "second"); r.X != 401 || r.W != 400 {
		t.Errorf("800 DIPs of room: second at %v, %v wide, want at 401, 400 wide", r.X, r.W)
	}
	if size != 300 {
		t.Errorf("resizing the window changed the size to %v", size)
	}
}

// TestSplitLimitsPercent bounds the panes of the percent variants in DIPs:
// they share the room in proportion within their limits.
func TestSplitLimitsPercent(t *testing.T) {
	p := float32(50)
	l := SplitLimits{FirstMax: 300, SecondMin: 100}
	tt := coreNewTester(func(c *context) {
		first, second := limitsPanes(c)
		coreSplitPercent(c, &p, first, second).SplitLimits(l).Fill()
	}, 1001, 200)
	if r := at(t, tt, "second"); r.X != 301 || r.W != 700 {
		t.Fatalf("half of 1000 past first's maximum: second at %v, %v wide, want at 301, 700 wide", r.X, r.W)
	}
	tt.SetSize(401, 200)
	if r := at(t, tt, "second"); r.X != 201 || r.W != 200 {
		t.Errorf("half of 400: second at %v, %v wide, want at 201, 200 wide", r.X, r.W)
	}
	if p != 50 {
		t.Errorf("resizing the window changed the percentage to %v", p)
	}
	// Past second's minimum, the divider stops and waits.
	x, y := float32(202), float32(100)
	tt.Press(x, y)
	tt.Move(x+200, y)
	if p != 75 {
		t.Errorf("dragged past second's minimum, first takes %v%%, want 75", p)
	}
	tt.Move(x+120, y)
	if p != 75 {
		t.Errorf("80 DIPs back, still 20 past the divider, first takes %v%%", p)
	}
	tt.Move(x+60, y)
	if p != 65 {
		t.Errorf("60 more back, 40 over the divider, first takes %v%%, want 65", p)
	}
	tt.Release(x+60, y)
	for range 5 {
		tt.Key(0, KeyRight)
	}
	if p != 75 {
		t.Errorf("Right past second's minimum, first takes %v%%, want 75", p)
	}

	// A pane at no share of the room still takes what the other can't.
	q := float32(0)
	tt = coreNewTester(func(c *context) {
		first, second := limitsPanes(c)
		coreSplitVerticalPercent(c, &q, first, second).SplitLimits(SplitLimits{SecondMax: 300}).Fill()
	}, 200, 501)
	if r := at(t, tt, "second"); r.Y != 201 || r.H != 300 {
		t.Errorf("first at 0%%, second at most 300: second at %v, %v high, want at 201, 300 high", r.Y, r.H)
	}
	q = 100
	tt.Frame()
	if r := at(t, tt, "second"); r.Y != 461 || r.H != 40 {
		t.Errorf("first at 100%%: second at %v, %v high, want at 461, 40 high", r.Y, r.H)
	}
}

// TestSplitLimitsConflict follows the documented priority where the limits
// can't all hold: a minimum wins over a maximum and over the room, and
// room beyond both maximums stays empty.
func TestSplitLimitsConflict(t *testing.T) {
	type pane struct{ x, w float32 }
	for _, tc := range []struct {
		name          string
		percent       bool
		limits        SplitLimits
		width         int
		size          float32
		first, second pane
	}{
		{"first's minimum over its maximum", false, SplitLimits{FirstMin: 300, FirstMax: 200}, 801, 100, pane{0, 300}, pane{301, 500}},
		{"second's minimum over its maximum", false, SplitLimits{SecondMin: 300, SecondMax: 200}, 801, 600, pane{0, 500}, pane{501, 300}},
		{"both minimums over the room", false, SplitLimits{FirstMin: 300, SecondMin: 300}, 401, 350, pane{0, 300}, pane{301, 300}},
		{"both minimums over the room, in percent", true, SplitLimits{FirstMin: 300, SecondMin: 300}, 401, 50, pane{0, 300}, pane{301, 300}},
		{"room beyond both maximums", false, SplitLimits{FirstMax: 200, SecondMax: 300}, 1001, 500, pane{0, 200}, pane{201, 300}},
		{"room beyond both maximums, in percent", true, SplitLimits{FirstMax: 200, SecondMax: 300}, 1001, 50, pane{0, 200}, pane{201, 300}},
	} {
		size := tc.size
		tt := coreNewTester(func(c *context) {
			first, second := limitsPanes(c)
			if tc.percent {
				coreSplitPercent(c, &size, first, second).SplitLimits(tc.limits).Fill()
			} else {
				coreSplit(c, &size, first, second).SplitLimits(tc.limits).Fill()
			}
		}, tc.width, 200)
		// Laid out, as the window may cut second.
		a, b := laidPanes(tt)
		if a.X != tc.first.x || a.W != tc.first.w {
			t.Errorf("%s: first at %v, %v wide, want at %v, %v wide", tc.name, a.X, a.W, tc.first.x, tc.first.w)
		}
		if b.X != tc.second.x || b.W != tc.second.w {
			t.Errorf("%s: second at %v, %v wide, want at %v, %v wide", tc.name, b.X, b.W, tc.second.x, tc.second.w)
		}
		// The divider doesn't move from there.
		x := tc.second.x + 1
		drag(tt, x, 100, x+30, 100)
		drag(tt, x, 100, x-30, 100)
		if _, b := laidPanes(tt); b.X != tc.second.x {
			t.Errorf("%s: dragged both ways, second is at %v, want %v", tc.name, b.X, tc.second.x)
		}
	}
}

// TestSplitLimitsReset puts first at the size SplitReset chose within its
// limits, which don't depend on the room.
func TestSplitLimitsReset(t *testing.T) {
	size := float32(250)
	tt := coreNewTester(func(c *context) {
		first, second := limitsPanes(c)
		coreSplit(c, &size, first, second).SplitLimits(SplitLimits{FirstMin: 200, FirstMax: 300}).SplitReset(500).Fill()
	}, 800, 200)
	tt.ClickAt(252, 100)
	tt.ClickAt(252, 100)
	if size != 300 {
		t.Errorf("double click to reset to 500: first is %v, want its maximum, 300", size)
	}
}

// TestSplitLimitsThroughTheFacade sets the limits through the public API,
// whose constructor applies the input before the chained SplitLimits of
// that build runs: the limits the build before set are the ones the input
// applies by, and the layout this build's.
func TestSplitLimitsThroughTheFacade(t *testing.T) {
	size := float32(300)
	limits := true
	tt := NewTester(func(c *Context) {
		e := Split(c, &size, func() { Box(c).Fill().Label("first") }, func() { Box(c).Fill().Label("second") })
		if limits {
			e.SplitLimits(SplitLimits{FirstMax: 350})
		}
		e.Fill()
	}, 800, 200)
	drag(tt, 302, 100, 502, 100)
	if size != 350 {
		t.Errorf("dragged past first's maximum: %v, want 350", size)
	}
	if r, _ := tt.Find("second"); r.X != 351 {
		t.Errorf("second starts at %v, want 351", r.X)
	}
	// Without them, the split is as it was.
	limits = false
	tt.Frame()
	drag(tt, 352, 100, 552, 100)
	if size != 550 {
		t.Errorf("without limits, dragged 200 DIPs right: %v, want 550", size)
	}
}

// TestSplitLimitsAllocs builds a split with limits with no more allocations
// a frame than one without, and adds none to a split without them.
func TestSplitLimitsAllocs(t *testing.T) {
	frame := func(l *SplitLimits) float64 {
		size := float32(300)
		tt := coreNewTester(func(c *context) {
			e := coreSplit(c, &size, func() { coreText(c, "first") }, func() { coreText(c, "second") })
			if l != nil {
				e.SplitLimits(*l)
			}
			e.Fill()
		}, 800, 200)
		tt.Frame()
		return testing.AllocsPerRun(50, tt.Frame)
	}
	without, with := frame(nil), frame(&SplitLimits{FirstMin: 200, FirstMax: 400, SecondMin: 100, SecondMax: 500})
	if with > without {
		t.Errorf("a frame of a split with limits allocates %v times, %v without", with, without)
	}
}

// TestSplitLimitsSecond bounds the panes of a split whose *size is the
// second pane's (SplitSecond), 8 DIPs apart (SplitGap), in whichever order
// the app calls the three: a side panel between 280 and 480 DIPs wide
// after a chat no narrower than 360 (#2041).
func TestSplitLimitsSecond(t *testing.T) {
	type mod func(e *node)
	mods := map[string]mod{
		"second": func(e *node) { e.SplitSecond() },
		"gap":    func(e *node) { e.SplitGap(8) },
		"limits": func(e *node) { e.SplitLimits(SplitLimits{FirstMin: 360, SecondMin: 280, SecondMax: 480}) },
	}
	for _, order := range [][3]string{
		{"second", "gap", "limits"}, {"second", "limits", "gap"}, {"gap", "second", "limits"},
		{"gap", "limits", "second"}, {"limits", "second", "gap"}, {"limits", "gap", "second"},
	} {
		size := float32(300)
		var div *node
		tt := coreNewTester(func(c *context) {
			first, second := limitsPanes(c)
			e := coreSplit(c, &size, first, second)
			for _, m := range order {
				mods[m](e)
			}
			e.Fill()
			div = c.splitBuilt(e).div
		}, 1000, 200)
		tt.Frame()
		if a, b := laidPanes(tt); a.W != 692 || b.X != 700 || b.W != 300 {
			t.Fatalf("%v: panes %v and %v, want the second at 700, 300 wide", order, a, b)
		}
		if div.accRange != [3]float64{992 - 480, 992 - 280, 692} {
			t.Errorf("%v: the divider reads %v, want 692 in 512..712", order, div.accRange)
		}
		// Left widens the second pane up to its maximum.
		y := float32(100)
		drag(tt, 696, y, 396, y)
		if size != 480 {
			t.Errorf("%v: dragged past the second's maximum, it is %v wide, want 480", order, size)
		}
		drag(tt, 516, y, 916, y)
		if size != 280 {
			t.Errorf("%v: dragged past the second's minimum, it is %v wide, want 280", order, size)
		}
		// A narrower window: the first keeps its minimum, the second gives
		// way down to its own, keeping *size.
		size = 450
		tt.SetSize(700, 200)
		if a, b := laidPanes(tt); a.W != 360 || b.W != 332 || size != 450 {
			t.Errorf("%v: in 700 DIPs, panes %v and %v (size %v), want 360 and 332 wide", order, a, b, size)
		}
	}
}
