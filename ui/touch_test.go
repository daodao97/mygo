package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
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

// Selected read-only text has handles, as an input's selection does: a long
// press selects a word, and dragging the end handle takes the selection on.
func TestReadOnlySelectionHandles(t *testing.T) {
	tt := NewTester(func(c *Context) {
		Column(c).Selectable().Padding(20).Children(func() {
			Text(c, "alpha bravo charlie delta echo")
		})
	}, 400, 200)
	tt.Frame()
	r, ok := tt.Find("alpha bravo charlie delta echo")
	if !ok {
		t.Fatal("no text")
	}
	// Hold "bravo".
	x, y := r.X+r.W*0.27, r.Y+r.H/2
	tt.FingerDown(x, y)
	tt.Advance(700 * time.Millisecond)
	tt.FingerUp(x, y)
	tt.Frame()
	a, b, selected := tt.rt.textSelectionBounds()
	if !selected {
		t.Fatal("a long press selected nothing")
	}
	start, end, ok := tt.rt.selectionHandles()
	if !ok || end.X <= start.X {
		t.Fatalf("handles %v %v %v", start, end, ok)
	}
	// The menu points at the word, not at the top of the paragraph.
	if s := tt.rt.states[a.id]; s != nil {
		mx, my := tt.rt.editMenuPoint(s)
		if my != start.Y || mx < start.X || mx > end.X {
			t.Fatalf("menu at %v, %v; selection %v to %v", mx, my, start, end)
		}
	}
	if h := tt.rt.handleAt(float32(end.X), float32(end.Y+end.H)); h != handleEnd {
		t.Fatalf("the end handle is not where it draws: %d", h)
	}
	was := b.at
	tt.FingerDown(float32(end.X), float32(end.Y+end.H))
	for _, nx := range []float32{float32(end.X) + 20, r.X + r.W - 2} {
		tt.FingerMove(nx, y+12)
		tt.Frame()
	}
	tt.FingerUp(r.X+r.W-2, y+12)
	tt.Frame()
	a2, b2, selected := tt.rt.textSelectionBounds()
	if !selected || a2 != a || b2.at <= was {
		t.Fatalf("dragging the end handle: from %v-%v to %v-%v", a, b, a2, b2)
	}
}

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

// Text that asks for TouchSelection: a hold its handler takes does not
// focus it, a tap does, and text that does not ask keeps its long press.
func TestTouchSelectionDefersThePress(t *testing.T) {
	value, held := "", 0
	ask := true
	tt := NewTester(func(c *Context) {
		in := TextInput(c, &value).Label("Message").Size(300, 44).HandleInput(func(ev InputEvent) bool {
			if ev.Kind == InputLongPress {
				held++
				return true
			}
			return false
		})
		if ask && !in.Focused() {
			in.TouchSelection()
		}
	}, 320, 200)
	tt.Frame()
	r, _ := tt.Find("Message")
	x, y := r.X+r.W/2, r.Y+r.H/2
	tt.FingerDown(x, y)
	tt.Advance(time.Second)
	tt.FingerUp(x, y)
	tt.Frame()
	if held != 1 || tt.Focused("Message") {
		t.Fatalf("hold: held %d, focused %v", held, tt.Focused("Message"))
	}
	tt.FingerDown(x, y)
	tt.FingerUp(x, y)
	tt.Advance(200 * time.Millisecond)
	if !tt.Focused("Message") {
		t.Fatal("a tap did not focus the input")
	}
	// Focused, it no longer asks: the hold is the text's.
	tt.FingerDown(x, y)
	tt.Advance(time.Second)
	tt.FingerUp(x, y)
	tt.Frame()
	if held != 1 || !tt.Focused("Message") {
		t.Fatalf("focused hold: held %d, focused %v", held, tt.Focused("Message"))
	}
}

// Each ask holds for the frame that asks: asking for another every frame
// keeps no earlier one.
func TestTouchAsksLastAFrame(t *testing.T) {
	ask := true
	var id uint64
	tt := NewTester(func(c *Context) {
		e := Box(c).Size(100, 100).KeyboardDismiss(KeyboardDismissOnDrag)
		if ask {
			e.TouchSelection().TouchPointer()
		}
		id = uint64(e.ID())
	}, 200, 200)
	tt.Frame()
	s := tt.rt.states[id]
	if s == nil || !s.touchHold || !s.touchPointer || s.keyboardDismiss != KeyboardDismissOnDrag {
		t.Fatalf("asked: %+v", s)
	}
	ask = false
	tt.Frame()
	if s.touchHold || s.touchPointer || s.keyboardDismiss != KeyboardDismissOnDrag {
		t.Fatal("an ask outlived its frame")
	}
}

// A tap on a button that keeps the input's focus, as a chat's Send, is no tap
// on its caret: it shows no edit menu.
func TestKeepFocusTapIsNoCaretTap(t *testing.T) {
	value, sent := "", 0
	tt := NewTester(func(c *Context) {
		Column(c).Gap(20).Padding(20).Children(func() {
			TextInput(c, &value).Label("Message").Size(300, 44)
			if Button(c, "Send").KeepFocus().Clicked() {
				sent++
			}
		})
	}, 400, 300)
	tt.Frame()
	r, _ := tt.Find("Message")
	tt.FingerDown(r.X+r.W/2, r.Y+r.H/2)
	tt.FingerUp(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	if !tt.Focused("Message") {
		t.Fatal("the input took no focus")
	}
	caretAt := tt.rt.touchText.caretAt
	tt.Advance(time.Second)
	b, _ := tt.Find("Send")
	tt.FingerDown(b.X+b.W/2, b.Y+b.H/2)
	tt.FingerUp(b.X+b.W/2, b.Y+b.H/2)
	tt.Frame()
	if sent != 1 || !tt.Focused("Message") {
		t.Fatalf("sent %d, focused %v", sent, tt.Focused("Message"))
	}
	if tt.rt.touchText.caretAt != caretAt {
		t.Fatal("a tap on Send was taken for a tap on the caret")
	}
}

// A tap on the caret shows the edit menu, the next puts it away, as UIKit's
// text fields do.
func TestCaretTapTogglesEditMenu(t *testing.T) {
	value := "hello"
	tt := NewTester(func(c *Context) {
		Column(c).Padding(20).Children(func() { TextInput(c, &value).Label("Message").Size(300, 44) })
	}, 400, 200)
	tt.Frame()
	r, _ := tt.Find("Message")
	x, y := r.X+r.W-10, r.Y+r.H/2
	tap := func() {
		tt.FingerDown(x, y)
		tt.FingerUp(x, y)
		tt.Advance(500 * time.Millisecond)
	}
	tap() // focuses it, and places the caret
	if tt.rt.touchText.menu {
		t.Fatal("the first tap showed the menu")
	}
	for i, want := range []bool{true, false, true} {
		tap()
		if tt.rt.touchText.menu != want {
			t.Fatalf("tap %d on the caret: menu %v, want %v", i+2, tt.rt.touchText.menu, want)
		}
	}
	tt.Type("x")
	if tt.rt.touchText.menu {
		t.Fatal("typing left the menu")
	}
}

// Copy from the edit menu of selected read-only text copies it and deselects it.
func TestMenuCopyDeselectsReadOnlyText(t *testing.T) {
	tt := NewTester(func(c *Context) {
		Column(c).Selectable().Padding(20).Children(func() {
			Text(c, "alpha bravo charlie")
		})
	}, 400, 200)
	tt.Frame()
	r, _ := tt.Find("alpha bravo charlie")
	x, y := r.X+r.W*0.5, r.Y+r.H/2
	tt.FingerDown(x, y)
	tt.Advance(700 * time.Millisecond)
	tt.FingerUp(x, y)
	tt.Frame()
	if _, _, selected := tt.rt.textSelectionBounds(); !selected || !tt.rt.touchText.menu {
		t.Fatalf("long press: selected %v, menu %v", selected, tt.rt.touchText.menu)
	}
	var want string
	for _, p := range tt.rt.selectedTexts(tt.rt.selection.scope) {
		want += string([]rune(p.Text)[p.Start:p.End])
	}
	tt.Command("copy")
	tt.Frame()
	if got := tt.Clipboard(); got == "" || got != want {
		t.Fatalf("copied %q, selected %q", got, want)
	}
	if _, _, selected := tt.rt.textSelectionBounds(); selected {
		t.Fatal("the copied text stayed selected")
	}
	if _, _, ok := tt.rt.selectionHandles(); ok {
		t.Fatal("the handles stayed")
	}
}
