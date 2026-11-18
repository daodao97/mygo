package ui

import (
	"testing"
	"time"
)

// A finger pans what scrolls under it once it moves, and the press ends
// without a click; lifted as it moves, the content flings on and slows
// down to a stop.
func TestTouchPans(t *testing.T) {
	var s ScrollState
	clicks := 0
	tt := coreNewTester(func(c *context) {
		coreScroll(c).Fill().TrackScroll(&s).Children(func() {
			b := coreBox(c).Height(1000).Shrink(0)
			if b.Clicked() {
				clicks++
			}
		})
	}, 200, 200)
	// A tap clicks.
	tt.FingerDown(100, 150)
	tt.FingerUp(100, 150)
	tt.Frame()
	if clicks != 1 || s.Y != 0 {
		t.Fatalf("a tap: %d clicks, scrolled to %v", clicks, s.Y)
	}
	// Moving less than the slop is still a press.
	tt.Advance(time.Second)
	tt.FingerDown(100, 150)
	tt.FingerMove(100, 146)
	if s.Y != 0 {
		t.Errorf("within the slop: scrolled to %v", s.Y)
	}
	// Past it, the finger drags the content, which follows it from there.
	tt.FingerMove(100, 140)
	tt.Advance(16 * time.Millisecond)
	tt.FingerMove(100, 100)
	if s.Y != 40 {
		t.Errorf("dragged 40 up: scrolled to %v", s.Y)
	}
	tt.Advance(16 * time.Millisecond)
	tt.FingerMove(100, 60)
	tt.Advance(16 * time.Millisecond)
	tt.FingerUp(100, 20)
	tt.Frame()
	if clicks != 1 {
		t.Errorf("a pan clicked: %d clicks", clicks)
	}
	at := s.Y
	if at != 80 {
		t.Errorf("after the pan: at %v", at)
	}
	// It flings on, slower and slower, until it stops.
	tt.Advance(50 * time.Millisecond)
	if s.Y <= at {
		t.Errorf("no fling: at %v, was %v", s.Y, at)
	}
	tt.Advance(10 * time.Second)
	if tt.FramePending() || s.Y <= at || s.Y > 800 {
		t.Errorf("after the fling: at %v, frames pending %v", s.Y, tt.FramePending())
	}
	// A finger stops a fling and taps nothing.
	s.Y = 0
	tt.Frame()
	tt.Advance(time.Second)
	tt.FingerDown(100, 150)
	tt.Advance(16 * time.Millisecond)
	tt.FingerMove(100, 100)
	tt.Advance(16 * time.Millisecond)
	tt.FingerUp(100, 40)
	tt.Advance(16 * time.Millisecond)
	tt.FingerDown(100, 100)
	tt.FingerUp(100, 100)
	stopped := s.Y
	tt.Advance(time.Second)
	if s.Y != stopped || clicks != 1 {
		t.Errorf("a tap on a fling: moved from %v to %v, %d clicks", stopped, s.Y, clicks)
	}
}

// A finger that lands on a fling stops it and taps nothing, but pans once
// it moves past the slop, as UIKit's scroll views follow the finger that
// stops their deceleration: flicking again and again keeps the content
// going (#2003).
func TestTouchPansAfterCatchingFling(t *testing.T) {
	var s ScrollState
	clicks := 0
	tt := coreNewTester(func(c *context) {
		coreScroll(c).Fill().TrackScroll(&s).Children(func() {
			if coreBox(c).Height(5000).Shrink(0).Clicked() {
				clicks++
			}
		})
	}, 200, 200)
	flick := func() {
		tt.FingerDown(100, 150)
		tt.Advance(16 * time.Millisecond)
		tt.FingerMove(100, 100)
		tt.Advance(16 * time.Millisecond)
		tt.FingerUp(100, 40)
	}
	flick()
	// Whole frames (60 Hz), for the frame of the finger's landing not to
	// come too soon after the last, which TestTouchFlingSurvivesFrameTooSoon
	// tests.
	tt.Advance(3 * (time.Second / 60))
	if !tt.rt.flinging.on {
		t.Fatalf("no fling after a flick: at %v", s.Y)
	}
	// The finger stops it.
	tt.FingerDown(100, 150)
	caught := s.Y
	tt.Advance(50 * time.Millisecond)
	if tt.rt.flinging.on || s.Y != caught {
		t.Fatalf("the finger did not stop the fling: at %v, was %v", s.Y, caught)
	}
	// Within the slop it moves nothing; past it, the content follows it
	// from there.
	tt.FingerMove(100, 146)
	tt.FingerMove(100, 140)
	if s.Y != caught {
		t.Errorf("within and at the slop: at %v, was %v", s.Y, caught)
	}
	tt.Advance(16 * time.Millisecond)
	tt.FingerMove(100, 100)
	if s.Y != caught+40 {
		t.Errorf("dragged 40 up after catching the fling: at %v, want %v", s.Y, caught+40)
	}
	// Lifted as it moves, it flings again, and clicked nothing.
	tt.Advance(16 * time.Millisecond)
	tt.FingerUp(100, 40)
	if !tt.rt.flinging.on {
		t.Errorf("no fling after the second flick")
	}
	tt.Advance(10 * time.Second)
	if clicks != 0 {
		t.Errorf("flicks clicked: %d clicks", clicks)
	}
	// A finger that catches a fling and lifts without moving still taps
	// nothing, and stays swallowed within the slop.
	s.Y = 0
	tt.Frame()
	tt.Advance(time.Second)
	flick()
	tt.Advance(16 * time.Millisecond)
	tt.FingerDown(100, 100)
	tt.FingerMove(100, 95)
	stopped := s.Y
	tt.FingerUp(100, 95)
	tt.Advance(time.Second)
	if s.Y != stopped || clicks != 0 {
		t.Errorf("a tap on a fling: moved from %v to %v, %d clicks", stopped, s.Y, clicks)
	}
}

// A frame too soon after the last to move a fling (its decay rounds to
// none in float32) leaves the fling going, its time adding up to the next
// frame, rather than ending it as if the content had reached its end
// (#2003).
func TestTouchFlingSurvivesFrameTooSoon(t *testing.T) {
	var s ScrollState
	tt := coreNewTester(func(c *context) {
		coreScroll(c).Fill().TrackScroll(&s).Children(func() {
			coreBox(c).Height(5000).Shrink(0)
		})
	}, 200, 200)
	tt.FingerDown(100, 150)
	tt.Advance(16 * time.Millisecond)
	tt.FingerMove(100, 100)
	tt.Advance(16 * time.Millisecond)
	tt.FingerUp(100, 40)
	f := &tt.rt.flinging
	if !f.on {
		t.Fatalf("no fling after a flick")
	}
	at, vy, was := f.at, f.vy, s.Y
	tt.rt.flingStep(at.Add(time.Nanosecond))
	if !f.on {
		t.Fatalf("a frame 1ns after the last ended the fling")
	}
	if f.at != at || f.vy != vy || s.Y != was {
		t.Errorf("a frame that moved nothing changed the fling: at %v (was %v), vy %v (was %v), scrolled to %v (was %v)", f.at, at, f.vy, vy, s.Y, was)
	}
	tt.Advance(50 * time.Millisecond)
	if s.Y <= was {
		t.Errorf("the fling did not go on: at %v, was %v", s.Y, was)
	}
}

// A scroll container reaching the bottom of the safe area goes on under
// the bottom inset, as UIKit's under a floating tab bar, and scrolls as
// far again; what does not scroll stays clear of the insets.
func TestScrollUnderBottomInset(t *testing.T) {
	var s ScrollState
	var box *node
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Fill().Children(func() {
			box = coreBox(c).Height(20).Shrink(0)
			coreScroll(c).Grow(1).TrackScroll(&s).Children(func() {
				coreBox(c).Height(1000).Shrink(0)
			})
		})
	}, 200, 400)
	tt.SetInsets(30, 0, 50, 0)
	if box.y != 30 {
		t.Errorf("the box is at %v, under the top inset", box.y)
	}
	s.Y = 10000
	tt.Frame()
	tt.Frame()
	// 400 high: 30 above, 20 of box, the scroll from 50 to the bottom
	// edge, 350 high, whose end comes out 50 above it.
	if want := float32(1000 - (400 - 50 - 50)); s.Y != want {
		t.Errorf("scrolled to the end at %v, want %v", s.Y, want)
	}
}

// A finger on what scrolls presses only once pressDelay passed without
// it moving: a pan that starts sooner presses nothing.
func TestTouchPressDelay(t *testing.T) {
	pressed := false
	tt := coreNewTester(func(c *context) {
		coreScroll(c).Fill().Children(func() {
			b := coreBox(c).Height(1000).Shrink(0)
			b.Clicked()
			pressed = b.Pressed()
		})
	}, 200, 200)
	tt.FingerDown(100, 100)
	tt.Frame()
	if pressed {
		t.Error("pressed before the delay")
	}
	tt.Advance(pressDelay + 20*time.Millisecond)
	if !pressed {
		t.Error("not pressed after the delay")
	}
	tt.FingerUp(100, 100)
	tt.Advance(time.Second)
	tt.FingerDown(100, 150)
	tt.Advance(16 * time.Millisecond)
	tt.FingerMove(100, 120)
	tt.Advance(pressDelay * 2)
	if pressed {
		t.Error("a pan pressed")
	}
	tt.FingerUp(100, 100)
}

// A finger keeps to the way it first went, and to the container that scrolls
// that way: a list across inside a list down takes only what goes across.
func TestTouchLocksAxis(t *testing.T) {
	var outer, inner ScrollState
	tt := coreNewTester(func(c *context) {
		coreScroll(c).Fill().TrackScroll(&outer).Children(func() {
			coreScrollHorizontal(c).Height(100).Shrink(0).TrackScroll(&inner).Children(func() {
				coreBox(c).Width(1000).Height(100).Shrink(0)
			})
			coreBox(c).Height(1000).Shrink(0)
		})
	}, 200, 200)
	tt.FingerDown(100, 50)
	tt.Advance(time.Millisecond)
	tt.FingerMove(70, 52) // across
	tt.Advance(16 * time.Millisecond)
	tt.FingerMove(40, 90) // then diagonally: it keeps going across
	tt.Frame()
	if inner.X != 30 || outer.Y != 0 {
		t.Errorf("across: inner %v, outer %v", inner.X, outer.Y)
	}
	tt.FingerUp(40, 90)
	tt.Advance(5 * time.Second)
	inner.X = 0
	tt.Frame()
	tt.FingerDown(100, 150)
	tt.Advance(time.Millisecond)
	tt.FingerMove(102, 120) // down the list, from outside the inner one
	tt.Advance(16 * time.Millisecond)
	tt.FingerMove(102, 100)
	tt.Frame()
	if inner.X != 0 || outer.Y != 20 {
		t.Errorf("down: inner %v, outer %v", inner.X, outer.Y)
	}
}

// Past the end the content follows the finger less and less far, and springs
// back as it lifts.
func TestTouchOverscrolls(t *testing.T) {
	defer func(k overscroll) { overscrollKind = k }(overscrollKind)
	overscrollKind = overscrollBounce
	var s ScrollState
	var top float32
	tt := coreNewTester(func(c *context) {
		coreScroll(c).Fill().TrackScroll(&s).Children(func() {
			b := coreBox(c).Height(1000).Shrink(0)
			top = b.st.y
		})
	}, 200, 200)
	tt.Frame()
	tt.FingerDown(100, 50)
	tt.Advance(time.Millisecond)
	tt.FingerMove(100, 60)
	tt.Advance(16 * time.Millisecond)
	tt.FingerMove(100, 110) // 50 past the top
	tt.Frame()
	tt.Frame()
	if s.Y != 0 {
		t.Fatalf("scrolled to %v", s.Y)
	}
	if top <= 0 || top >= 50 {
		t.Fatalf("pulled 50 past the top: the content shows %v down", top)
	}
	pulled := top
	tt.Advance(200 * time.Millisecond) // a finger at rest throws nothing
	tt.FingerUp(100, 110)
	tt.Advance(100 * time.Millisecond)
	tt.Frame()
	if top >= pulled {
		t.Errorf("as it lifts, it should spring back: %v from %v", top, pulled)
	}
	for range 200 {
		tt.Advance(16 * time.Millisecond)
	}
	tt.Frame()
	if top != 0 {
		t.Errorf("after the spring, it shows at %v", top)
	}
	// Without overscroll it stops at the end.
	overscrollKind = overscrollNone
	tt.FingerDown(100, 50)
	tt.Advance(time.Millisecond)
	tt.FingerMove(100, 60)
	tt.FingerMove(100, 110)
	tt.Frame()
	tt.Frame()
	if top != 0 {
		t.Errorf("without overscroll it shows at %v", top)
	}
	tt.FingerUp(100, 110)
}

// A finger held still long-presses, which clicks nothing; moved first, it
// does not.
func TestTouchLongPress(t *testing.T) {
	long, clicks := 0, 0
	tt := coreNewTester(func(c *context) {
		b := coreBox(c).Fill()
		if b.LongPressed() {
			long++
		}
		if b.Clicked() {
			clicks++
		}
	}, 200, 200)
	tt.Frame()
	tt.FingerDown(50, 50)
	tt.Advance(400 * time.Millisecond)
	if long != 0 {
		t.Fatal("too soon")
	}
	tt.Advance(200 * time.Millisecond)
	tt.Frame()
	if long != 1 {
		t.Fatalf("long presses: %d", long)
	}
	tt.FingerUp(50, 50)
	tt.Frame()
	if clicks != 0 {
		t.Errorf("a long press clicked %d times", clicks)
	}
	// A tap clicks, and long-presses nothing.
	tt.Advance(time.Second)
	tt.FingerDown(50, 50)
	tt.Advance(50 * time.Millisecond)
	tt.FingerUp(50, 50)
	tt.Advance(time.Second)
	tt.Frame()
	if long != 1 || clicks != 1 {
		t.Errorf("a tap: %d long, %d clicks", long, clicks)
	}
}

// A sideways Draggable in a list takes what goes sideways, and leaves what
// goes down to the list.
func TestTouchDraggableAxisInScroll(t *testing.T) {
	var s ScrollState
	var off Point
	tt := coreNewTester(func(c *context) {
		coreScroll(c).Fill().TrackScroll(&s).Children(func() {
			coreBox(c).Height(100).Shrink(0).Draggable(&off, DraggableOptions{Axis: Horizontal})
			coreBox(c).Height(1000).Shrink(0)
		})
	}, 200, 200)
	tt.Frame()
	// Down: the list.
	tt.FingerDown(100, 50)
	tt.Advance(time.Millisecond)
	tt.FingerMove(101, 30)
	tt.Advance(16 * time.Millisecond)
	tt.FingerMove(101, 20)
	tt.Frame()
	if off.X != 0 || s.Y != 10 {
		t.Errorf("down: dragged %v, scrolled %v", off.X, s.Y)
	}
	tt.FingerUp(101, 20)
	tt.Advance(5 * time.Second)
	s.Y = 0
	tt.Frame()
	// Across: the row.
	tt.FingerDown(100, 50)
	tt.Advance(time.Millisecond)
	tt.FingerMove(70, 51)
	tt.Frame()
	tt.Advance(16 * time.Millisecond)
	tt.FingerMove(40, 51)
	tt.Frame()
	if off.X >= 0 || s.Y != 0 {
		t.Errorf("across: dragged %v, scrolled %v", off.X, s.Y)
	}
	tt.FingerUp(40, 51)
}

// Pulling a container past its top by enough refreshes it.
func TestPullToRefresh(t *testing.T) {
	defer func(k overscroll) { overscrollKind = k }(overscrollKind)
	overscrollKind = overscrollBounce
	var r RefreshState
	fired := 0
	tt := coreNewTester(func(c *context) {
		e := coreScroll(c).Fill()
		if e.PullToRefresh(&r) {
			fired++
		}
		e.Children(func() { coreBox(c).Height(50).Shrink(0) })
	}, 200, 200)
	tt.Frame()
	tt.FingerDown(100, 20)
	tt.Advance(time.Millisecond)
	tt.FingerMove(100, 30)
	tt.Advance(16 * time.Millisecond)
	tt.FingerMove(100, 60) // not far enough
	tt.FingerUp(100, 60)
	for range 100 {
		tt.Advance(16 * time.Millisecond)
	}
	if fired != 0 || r.Refreshing {
		t.Fatalf("a short pull fired %d times", fired)
	}
	tt.FingerDown(100, 20)
	tt.Advance(time.Millisecond)
	tt.FingerMove(100, 30)
	tt.Advance(16 * time.Millisecond)
	tt.FingerMove(100, 190)
	tt.FingerUp(100, 190)
	tt.Frame()
	if fired != 1 || !r.Refreshing {
		t.Fatalf("a long pull: fired %d, refreshing %v", fired, r.Refreshing)
	}
	// It rests with the indicator showing until the app is done.
	for range 100 {
		tt.Advance(16 * time.Millisecond)
	}
	st := tt.rt.states[tt.rt.lastScrollerForTest()]
	if st == nil || st.over[1] != -refreshPull {
		t.Fatalf("while refreshing: over %+v", st)
	}
	r.Refreshing = false
	tt.Frame()
	for range 100 {
		tt.Advance(16 * time.Millisecond)
	}
	if st.over[1] != 0 {
		t.Errorf("after refreshing: over %v", st.over[1])
	}
}

// What takes clicks inside a sideways Draggable leaves what goes sideways to
// it, and takes no click for it.
func TestTouchDraggableAroundClickable(t *testing.T) {
	var off Point
	clicks := 0
	tt := coreNewTester(func(c *context) {
		coreBox(c).Fill().Draggable(&off, DraggableOptions{Axis: Horizontal}).Children(func() {
			if coreBox(c).Fill().Clicked() {
				clicks++
			}
		})
	}, 200, 200)
	tt.Frame()
	tt.FingerDown(150, 50)
	tt.Advance(time.Millisecond)
	tt.FingerMove(120, 50)
	tt.Frame()
	tt.Advance(16 * time.Millisecond)
	tt.FingerMove(80, 50)
	tt.Frame()
	tt.Advance(200 * time.Millisecond)
	tt.FingerUp(80, 50)
	tt.Frame()
	if clicks != 0 || off.X >= -30 {
		t.Errorf("%d clicks, dragged to %v", clicks, off.X)
	}
}

// A finger holding a word chooses it; the handles at its ends drag them.
func TestTouchTextHandles(t *testing.T) {
	defer func(k overscroll) { overscrollKind = k }(overscrollKind)
	overscrollKind = overscrollBounce
	v := "hello big world"
	var in *node
	tt := coreNewTester(func(c *context) {
		in = coreTextInput(c, &v).Fill().AutoFocus()
	}, 300, 60)
	tt.Frame()
	tt.Frame()
	tt.FingerDown(20, 30)
	tt.Advance(600 * time.Millisecond)
	tt.FingerUp(20, 30)
	tt.Frame()
	if got := in.st.editor.selectedText(); got != "hello" {
		t.Fatalf("held: chose %q", got)
	}
	_, _, end, sel, ok := tt.rt.handles()
	if !ok || sel.End != 5 {
		t.Fatalf("handles: %v %v", ok, sel)
	}
	// The end handle, dragged to the end of the text.
	x, y := float32(end.X), float32(end.Y+end.H)
	tt.FingerDown(x, y)
	if tt.rt.touchText.held != handleEnd {
		t.Fatalf("near the end handle, it holds %d", tt.rt.touchText.held)
	}
	tt.FingerMove(x+200, y)
	tt.Frame()
	tt.FingerUp(x+200, y)
	tt.Frame()
	if got := in.st.editor.selectedText(); got != "hello big world" {
		t.Errorf("dragged to the end: chose %q", got)
	}
}

// The keyboard coming up shows the input in use.
func TestKeyboardRevealsFocus(t *testing.T) {
	v := ""
	var s ScrollState
	tt := coreNewTester(func(c *context) {
		coreScroll(c).Fill().TrackScroll(&s).Children(func() {
			coreBox(c).Height(500).Shrink(0)
			coreTextInput(c, &v).AutoFocus()
		})
	}, 300, 600)
	tt.Frame()
	tt.Frame()
	y0 := s.Y
	tt.SetInsets(0, 0, 300, 0)
	tt.Frame()
	tt.Frame()
	if s.Y <= y0 {
		t.Errorf("under the keyboard: scrolled from %v to %v", y0, s.Y)
	}
}

// A finger leaves no hover behind: what scrolls does not keep its bars.
func TestTouchLeavesNoHover(t *testing.T) {
	tt := coreNewTester(func(c *context) {
		coreScroll(c).Fill().Children(func() { coreBox(c).Height(1000).Shrink(0) })
	}, 200, 200)
	tt.Frame()
	tt.FingerDown(100, 150)
	tt.Advance(time.Millisecond)
	tt.FingerMove(100, 130)
	tt.Advance(16 * time.Millisecond)
	tt.FingerMove(100, 100)
	tt.Advance(200 * time.Millisecond)
	tt.FingerUp(100, 100)
	tt.Advance(5 * time.Second)
	tt.Frame()
	if len(tt.rt.hover) != 0 {
		t.Errorf("hovering %v after the finger lifted", tt.rt.hover)
	}
}

// A finger from the left edge takes a Router's page back, following it; let
// go short of half the way, the page comes back.
func TestTouchEdgeSwipeBack(t *testing.T) {
	defer func(k overscroll) { overscrollKind = k }(overscrollKind)
	overscrollKind = overscrollBounce
	r := NewRouter("/a")
	var c2 *context
	tt := coreNewTester(func(c *context) {
		c2 = c
		r.coreView(c, func(rt *Route) { coreBox(c).Fill() })
	}, 400, 400)
	_ = c2
	r.Push("/a/b")
	tt.Frame()
	tt.Advance(time.Second)
	// Short of half the way: it stays.
	tt.FingerDown(10, 200)
	tt.Advance(time.Millisecond)
	tt.FingerMove(30, 200)
	tt.Frame()
	tt.Advance(16 * time.Millisecond)
	tt.FingerMove(100, 200)
	tt.Frame()
	if !r.swiping || r.swipe <= 0 {
		t.Fatalf("following: swiping %v, %v", r.swiping, r.swipe)
	}
	tt.Advance(200 * time.Millisecond)
	tt.FingerUp(100, 200)
	for range 60 {
		tt.Advance(16 * time.Millisecond)
	}
	if r.Path() != "/a/b" || r.swipe != 0 || r.settling {
		t.Fatalf("given up: at %s, swipe %v, settling %v", r.Path(), r.swipe, r.settling)
	}
	// Past it: back.
	tt.FingerDown(10, 200)
	tt.Advance(time.Millisecond)
	tt.FingerMove(30, 200)
	tt.Frame()
	tt.Advance(16 * time.Millisecond)
	tt.FingerMove(300, 200)
	tt.Frame()
	tt.Advance(200 * time.Millisecond)
	tt.FingerUp(300, 200)
	tt.Frame()
	if r.Path() != "/a" {
		t.Errorf("past half the way: at %s", r.Path())
	}
}

// Without a page to go back to, an edge swipe is any other finger.
func TestTouchEdgeSwipeNowhere(t *testing.T) {
	defer func(k overscroll) { overscrollKind = k }(overscrollKind)
	overscrollKind = overscrollBounce
	r := NewRouter("/a")
	tt := coreNewTester(func(c *context) {
		r.coreView(c, func(rt *Route) { coreBox(c).Fill() })
	}, 400, 400)
	tt.Frame()
	tt.FingerDown(10, 200)
	tt.Advance(time.Millisecond)
	tt.FingerMove(30, 200)
	tt.Frame()
	if r.swiping {
		t.Error("swiping with nowhere to go")
	}
	tt.FingerUp(30, 200)
}

// A finger nearly on a small button takes it; one on nothing does nothing, and
// one between two buttons takes the one under it.
func TestTouchReach(t *testing.T) {
	clicks := 0
	tt := coreNewTester(func(c *context) {
		coreBox(c).Fill().Children(func() {
			if coreBox(c).Absolute().Left(100).Top(100).Size(20, 20).Clicked() {
				clicks++
			}
		})
	}, 300, 300)
	tt.Frame()
	tt.FingerDown(95, 110)
	tt.FingerUp(95, 110)
	tt.Frame()
	if clicks != 1 {
		t.Errorf("nearly on it: %d clicks", clicks)
	}
	tt.Advance(time.Second)
	tt.FingerDown(50, 50)
	tt.FingerUp(50, 50)
	tt.Frame()
	if clicks != 1 {
		t.Errorf("far from it: %d clicks", clicks)
	}
}
