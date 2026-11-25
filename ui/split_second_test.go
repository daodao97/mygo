package ui

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// TestSplitSecond sizes the second pane: it keeps its width at the end of
// the window, the first takes the rest, and the divider moves it with the
// pointer and the arrows, 40 DIPs from either edge (#1967).
func TestSplitSecond(t *testing.T) {
	size := float32(120)
	tt := coreNewTester(func(c *context) {
		coreSplit(c, &size, func() { coreText(c, "left") }, func() { coreText(c, "right") }).SplitSecond().Fill()
	}, 400, 200)
	if r := at(t, tt, "right"); r.X != 400-120 {
		t.Fatalf("the second pane starts at %v, want %v", r.X, 400-120)
	}
	if r := at(t, tt, "left"); r.X != 0 {
		t.Errorf("the first pane starts at %v", r.X)
	}
	// The divider, at 279, moves with the pointer: right narrows the
	// second pane.
	y := float32(100)
	drag(tt, 280, y, 330, y)
	if size != 70 {
		t.Errorf("dragged 50 DIPs right, the second pane is %v wide, want 70", size)
	}
	drag(tt, 330, y, 400, y)
	if size != 40 {
		t.Errorf("dragged to the right edge, the second pane is %v wide, want 40", size)
	}
	// Left widens it once the divider has the focus, as the line moves
	// the way the arrow points.
	tt.ClickAt(360, y)
	tt.Key(0, KeyLeft)
	if size != 50 {
		t.Errorf("Left on the divider: %v, want 50", size)
	}
	if r := at(t, tt, "right"); r.X != 350 {
		t.Errorf("the second pane follows to %v, want 350", r.X)
	}
	tt.Key(0, KeyRight)
	if size != 40 {
		t.Errorf("Right on the divider: %v, want 40", size)
	}
	drag(tt, 360, y, 0, y)
	if size != 400-1-40 {
		t.Errorf("dragged to the left edge, the second pane is %v wide, want %v", size, 400-1-40)
	}
	// The window grows: the second pane keeps its size, the first takes
	// the room.
	size = 120
	tt.SetSize(600, 200)
	if r := at(t, tt, "right"); r.X != 600-120 {
		t.Errorf("in a window 600 wide, the second pane starts at %v, want %v", r.X, 600-120)
	}
}

// TestSplitSecondKeepsOvershoot is TestSplitDragKeepsOvershoot for the
// second pane.
func TestSplitSecondKeepsOvershoot(t *testing.T) {
	size := float32(120)
	tt := coreNewTester(func(c *context) {
		coreSplit(c, &size, func() { coreText(c, "left") }, func() { coreText(c, "right") }).SplitSecond().Fill()
	}, 400, 200)
	x, y := float32(280), float32(100)
	tt.Press(x, y)
	// 80 DIPs to the second pane's least, 40, and 200 past it.
	tt.Move(x+280, y)
	if size != 40 {
		t.Fatalf("dragged past the least, the second pane is %v wide", size)
	}
	tt.Move(x+130, y)
	if size != 40 {
		t.Errorf("150 DIPs back, still 50 past the divider, the second pane is %v wide", size)
	}
	tt.Move(x+30, y)
	tt.Release(x+30, y)
	if size != 90 {
		t.Errorf("100 more back, 50 over the divider, the second pane is %v wide", size)
	}
}

// TestSplitSecondVertical sizes the bottom pane of SplitVertical.
func TestSplitSecondVertical(t *testing.T) {
	size := float32(60)
	tt := coreNewTester(func(c *context) {
		coreSplitVertical(c, &size, func() { coreText(c, "top") }, func() { coreText(c, "bottom") }).SplitSecond().Fill()
	}, 300, 200)
	if r := at(t, tt, "bottom"); r.Y != 200-60 {
		t.Fatalf("the second pane starts at %v, want %v", r.Y, 200-60)
	}
	drag(tt, 100, 140, 100, 100)
	if size != 100 {
		t.Errorf("dragged 40 DIPs up, the second pane is %v high, want 100", size)
	}
	tt.Key(0, KeyDown)
	if size != 90 {
		t.Errorf("Down on the divider: %v, want 90", size)
	}
}

// TestSplitSecondShrinks keeps both panes in a window made smaller: the
// second keeps its size while the first gives way down to its 40 DIPs,
// then the second gives way, and takes its size back when there is room
// again; the divider moves from where it shows.
func TestSplitSecondShrinks(t *testing.T) {
	size := float32(100)
	tt := coreNewTester(func(c *context) {
		coreSplitVertical(c, &size, func() { coreText(c, "top") }, func() { coreText(c, "bottom") }).SplitSecond().Fill()
	}, 300, 600)
	tt.SetSize(300, 300)
	if r := at(t, tt, "bottom"); r.Y != 300-100 {
		t.Fatalf("in a window 300 high, the second pane starts at %v, want 200", r.Y)
	}
	tt.SetSize(300, 120)
	if r := at(t, tt, "bottom"); r.Y != 41 {
		t.Fatalf("in a window 120 high, the second pane starts at %v, want 41", r.Y)
	}
	if size != 100 {
		t.Errorf("shrinking the window changed the size to %v", size)
	}
	// Assistive technology hears where the divider shows, from the top.
	tt.rt.accessibilityOn()
	tt.Frame()
	dividers := 0
	for _, n := range tt.h.access.Nodes {
		if n.Role == platform.RoleSplitter {
			dividers++
			if n.Now != 40 || n.Min != 40 || n.Max != 79 {
				t.Errorf("the divider says %v of %v to %v, want 40 of 40 to 79", n.Now, n.Min, n.Max)
			}
		}
	}
	if dividers != 1 {
		t.Errorf("assistive technology sees %d dividers", dividers)
	}
	tt.SetSize(300, 300)
	if r := at(t, tt, "bottom"); r.Y != 200 {
		t.Errorf("made high again, the second pane starts at %v, want 200", r.Y)
	}
	tt.SetSize(300, 120)
	// The divider shows at 40, the second pane 79 high.
	drag(tt, 100, 41, 100, 61)
	if size != 59 {
		t.Errorf("dragged 20 DIPs down from where it shows, the second pane is %v high, want 59", size)
	}
}

// TestSplitSecondSideways sizes the second pane left to right, where it is
// on the right, and right to left, where it is on the left.
func TestSplitSecondSideways(t *testing.T) {
	for _, rtl := range []bool{false, true} {
		size := float32(100)
		view := func(c *context) {
			coreSplit(c, &size, func() { coreText(c, "first") }, func() { coreText(c, "second") }).SplitSecond().Fill()
		}
		tt := coreNewTester(view, 400, 200)
		if rtl {
			tt = rtlTester(view, 400, 200)
		}
		first, second := at(t, tt, "first"), at(t, tt, "second")
		if !rtl && (second.X != 300 || first.X != 0) || rtl && (second.X >= 100 || first.X < 101) {
			t.Fatalf("rtl %v: the first pane's text is at %v, the second's at %v", rtl, first.X, second.X)
		}
		// The divider, 100 from the second pane's edge.
		x := float32(300)
		if rtl {
			x = 100
		}
		// Toward the first pane, which widens the second.
		to := x - 30
		if rtl {
			to = x + 30
		}
		drag(tt, x, 100, to, 100)
		if size != 130 {
			t.Errorf("rtl %v: dragged 30 DIPs toward the first pane, the second is %v wide, want 130", rtl, size)
		}
		// Left moves the divider left: the second pane widens left to
		// right, and narrows right to left.
		tt.Key(0, KeyLeft)
		if want := map[bool]float32{false: 140, true: 120}[rtl]; size != want {
			t.Errorf("rtl %v: Left made the second pane %v wide, want %v", rtl, size, want)
		}
		// With too little room, the first pane gives way first.
		tt.SetSize(150, 200)
		if r := at(t, tt, "first"); !rtl && r.X != 0 || rtl && r.X < 150-40 {
			t.Errorf("rtl %v: in a window 150 wide, the first pane's text is at %v", rtl, r.X)
		}
	}
}

// TestSplitSecondPercent makes *p the second pane's percentage.
func TestSplitSecondPercent(t *testing.T) {
	secondPercent := float32(25)
	tt := coreNewTester(func(c *context) {
		coreSplitVerticalPercent(c, &secondPercent, func() { coreText(c, "top") }, func() { coreText(c, "bottom") }).SplitSecond().Fill()
	}, 300, 401)
	if r := at(t, tt, "bottom"); r.Y != 301 {
		t.Fatalf("a quarter of 400 for the second pane: it starts at %v, want 301", r.Y)
	}
	drag(tt, 100, 300, 100, 200)
	if secondPercent != 50 {
		t.Errorf("dragged to the middle, the second pane takes %v%%, want 50", secondPercent)
	}
	// Up widens it by 10 DIPs of 400.
	tt.Key(0, KeyUp)
	if secondPercent != 52.5 {
		t.Errorf("Up from the middle: %v, want 52.5", secondPercent)
	}
}

// TestSplitSecondReset puts the second pane at SplitReset's size on a
// double click.
func TestSplitSecondReset(t *testing.T) {
	size := float32(120)
	tt := coreNewTester(func(c *context) {
		coreSplit(c, &size, func() { coreText(c, "left") }, func() { coreText(c, "right") }).SplitSecond().SplitReset(220).Fill()
	}, 400, 200)
	drag(tt, 280, 100, 330, 100)
	if size != 70 {
		t.Fatalf("dragged 50 DIPs right, the second pane is %v wide", size)
	}
	tt.ClickAt(330, 100)
	tt.ClickAt(330, 100)
	if size != 220 {
		t.Errorf("double click on the divider: second pane %v, want 220", size)
	}
	if r := at(t, tt, "right"); r.X != 400-220 {
		t.Errorf("the second pane starts at %v after the reset, want %v", r.X, 400-220)
	}
}

// TestSplitSecondThroughTheFacade sizes the second pane from the first
// frame through the public API, whose constructor applies input before
// the chained SplitSecond runs, and Changed reports a move.
func TestSplitSecondThroughTheFacade(t *testing.T) {
	size := float32(120)
	changed := 0
	tt := NewTester(func(c *Context) {
		if Split(c, &size, func() { Text(c, "left") }, func() { Text(c, "right") }).SplitSecond().Fill().Changed() {
			changed++
		}
	}, 400, 200)
	if r, _ := tt.Find("right"); r.X != 400-120 {
		t.Fatalf("on the first frame, the second pane starts at %v, want %v", r.X, 400-120)
	}
	drag(tt, 280, 100, 230, 100)
	if size != 170 {
		t.Errorf("dragged 50 DIPs left, the second pane is %v wide, want 170", size)
	}
	if changed == 0 {
		t.Error("Changed did not report the move")
	}
	if r, _ := tt.Find("right"); r.X != 400-170 {
		t.Errorf("the second pane starts at %v, want %v", r.X, 400-170)
	}
}

// TestSplitSecondGap sizes the second pane with SplitGap's room between
// the panes, whichever of the two the app calls first: the drag, the
// arrows and the divider's accessible position all go by it.
func TestSplitSecondGap(t *testing.T) {
	for _, gapFirst := range []bool{false, true} {
		size := float32(100)
		var a, b Rect
		var div *node
		tt := coreNewTester(func(c *context) {
			e := coreSplit(c, &size, func() {
				a = coreBox(c).Fill().Bounds()
			}, func() {
				b = coreBox(c).Fill().Bounds()
			})
			if gapFirst {
				e.SplitGap(8).SplitSecond()
			} else {
				e.SplitSecond().SplitGap(8)
			}
			e.Fill()
			div = c.splitBuilt(e).div
		}, 300, 100)
		tt.Frame()
		if a != (Rect{0, 0, 192, 100}) || b != (Rect{200, 0, 100, 100}) {
			t.Fatalf("gap first %v: panes at %v and %v, want {0 0 192 100} and {200 0 100 100}", gapFirst, a, b)
		}
		if div.accRange[1] != 300-8-40 || div.accRange[2] != 192 {
			t.Errorf("gap first %v: the divider reads %v of %v, want 192 of %v", gapFirst, div.accRange[2], div.accRange[1], 300-8-40)
		}
		drag(tt, 196, 50, 166, 50)
		tt.Frame()
		if size != 130 || b.X != 170 {
			t.Errorf("gap first %v: dragged 30 left, the second pane is %v wide at %v, want 130 at 170", gapFirst, size, b.X)
		}
		tt.Key(0, KeyRight)
		tt.Frame()
		if size != 120 || a.W != 172 {
			t.Errorf("gap first %v: Right: the second pane is %v, the first %v, want 120 and 172", gapFirst, size, a.W)
		}
		drag(tt, 176, 50, 0, 50)
		if size != 300-8-40 {
			t.Errorf("gap first %v: dragged to the left edge, the second pane is %v wide, want %v", gapFirst, size, 300-8-40)
		}
	}
}
