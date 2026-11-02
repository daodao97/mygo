package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

func TestNestedTouchScrollLocksDirectionThroughMomentum(t *testing.T) {
	for _, horizontal := range []bool{false, true} {
		var outer, inner ScrollState
		clicks := 0
		tt := snapshotTester(func(c *Context) {
			Scroll(c).Fill().TrackScroll(&outer).Children(func() {
				ScrollHorizontal(c).FillWidth().Height(100).Shrink(0).TrackScroll(&inner).Children(func() {
					if Button(c, "Wide table content").Width(900).Height(100).Clicked() {
						clicks++
					}
				})
				Box(c).Height(1000).Shrink(0)
			})
		}, 320, 240)
		touch(tt, platform.PointerDown, 1, 200, 60)
		if horizontal {
			touch(tt, platform.PointerMove, 1, 150, 48)
			touch(tt, platform.PointerMove, 1, 100, 5)
			touch(tt, platform.PointerUp, 1, 100, 5)
		} else {
			touch(tt, platform.PointerMove, 1, 188, 10)
			touch(tt, platform.PointerMove, 1, 130, -40)
			touch(tt, platform.PointerUp, 1, 130, -40)
		}
		tt.send(platform.SurfaceEvent{Kind: platform.PointerScroll, PointerType: platform.PointerTouch, PointerID: 1,
			X: 200, Y: 60, DX: 30, DY: 40, Precise: true, Momentum: true})
		if horizontal && (inner.X != 130 || outer.Y != 0) || !horizontal && (inner.X != 0 || outer.Y != 140) || clicks != 0 {
			t.Fatalf("horizontal=%v: inner=%v outer=%v clicks=%d", horizontal, inner.X, outer.Y, clicks)
		}
		if horizontal {
			inner.X = inner.MaxX
			tt.Frame()
			tt.send(platform.SurfaceEvent{Kind: platform.PointerScroll, PointerType: platform.PointerTouch, PointerID: 1,
				X: 200, Y: 60, DX: 30, DY: 40, Precise: true, Momentum: true})
			if outer.Y != 0 {
				t.Fatal("horizontal momentum moved the conversation at the table edge")
			}
		}
	}
}

func TestCustomTouchScrollKeepsTwoAxes(t *testing.T) {
	var dx, dy float32
	tt := snapshotTester(func(c *Context) {
		Scroll(c).Fill().Children(func() {
			Box(c).FillWidth().Height(100).Shrink(0).TouchScroll().HandleInput(func(ev InputEvent) bool {
				if ev.Kind == InputScroll {
					dx += ev.DX
					dy += ev.DY
				}
				return true
			})
			Box(c).Height(1000).Shrink(0)
		})
	}, 320, 240)
	touch(tt, platform.PointerDown, 2, 200, 80)
	touch(tt, platform.PointerMove, 2, 160, 60)
	touch(tt, platform.PointerUp, 2, 160, 60)
	tt.send(platform.SurfaceEvent{Kind: platform.PointerScroll, PointerType: platform.PointerTouch, PointerID: 2,
		X: 200, Y: 80, DX: 10, DY: 20, Precise: true, Momentum: true})
	if dx != 50 || dy != 40 {
		t.Fatalf("custom two-axis input was constrained: %v, %v", dx, dy)
	}
}

func TestRouterTransitioningEndsWithAnimation(t *testing.T) {
	r := NewRouter("/list")
	tt := snapshotTester(func(c *Context) {
		r.View(c, func(p *Route) { Text(c, p.Path()) })
	}, 320, 240)
	if r.Transitioning() {
		t.Fatal("initial page is transitioning")
	}
	r.Push("/list/detail")
	if !r.Transitioning() {
		t.Fatal("pending navigation was not reported before the next frame")
	}
	tt.Frame()
	if !r.Transitioning() {
		t.Fatal("outgoing page was not reported during navigation")
	}
	tt.Advance(time.Second)
	if r.Transitioning() {
		t.Fatal("completed navigation retained outgoing page state")
	}
	r.Back()
	tt.Frame()
	if !r.Transitioning() {
		t.Fatal("back transition was not reported")
	}
	tt.Advance(time.Second)
	if r.Transitioning() || r.Path() != "/list" {
		t.Fatal("back transition did not finish")
	}
}
