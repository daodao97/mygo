package ui

import (
	"github.com/egoist/mygo/internal/platform"
	"testing"
	"time"
)

func TestLongPressCapturesSelectionWithoutKeyboard(t *testing.T) {
	var events []InputEvent
	tt := NewTester(func(c *Context) {
		Box(c).Fill().Focusable().TouchSelection().TextCaret(Rect{W: 1, H: 20}).HandleInput(func(ev InputEvent) bool {
			events = append(events, ev)
			return true
		})
	}, 320, 240)
	now := time.Now()
	tt.rt.clock = func() time.Time { return now }
	touch(tt, platform.PointerDown, 1, 120, 100)
	now = now.Add(499 * time.Millisecond)
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceFrame})
	if len(events) != 0 || tt.h.ime.Active {
		t.Fatal("hold selected early or opened the keyboard")
	}
	now = now.Add(time.Millisecond)
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceFrame})
	if len(events) != 1 || events[0].Kind != InputLongPress || tt.h.ime.Active || tt.rt.pressed == nil {
		t.Fatalf("long press not captured: %+v", events)
	}
	touch(tt, platform.PointerMove, 1, 220, 150)
	touch(tt, platform.PointerUp, 1, 220, 150)
	if len(events) != 3 || events[1].Kind != InputPointerMove || events[2].Kind != InputPointerUp {
		t.Fatalf("selection became a scroll: %+v", events)
	}
	if tt.h.ime.Active || tt.rt.pressed != nil || tt.rt.touch.active {
		t.Fatal("selection retained contact or keyboard focus")
	}
}

func TestScrollAndCancelCannotBecomeLongPress(t *testing.T) {
	for _, cancel := range []platform.SurfaceEventKind{platform.PointerMove, platform.PointerCancel, platform.SurfaceBlur, platform.SurfaceResize} {
		holds := 0
		tt := NewTester(func(c *Context) {
			Box(c).Fill().TouchSelection().HandleInput(func(ev InputEvent) bool {
				if ev.Kind == InputLongPress {
					holds++
				}
				return true
			})
		}, 320, 240)
		now := time.Now()
		tt.rt.clock = func() time.Time { return now }
		touch(tt, platform.PointerDown, 1, 120, 180)
		if cancel == platform.SurfaceBlur || cancel == platform.SurfaceResize {
			tt.send(platform.SurfaceEvent{Kind: cancel})
		} else {
			touch(tt, cancel, 1, 120, 100)
		}
		now = now.Add(time.Second)
		tt.send(platform.SurfaceEvent{Kind: platform.SurfaceFrame})
		if holds != 0 {
			t.Fatalf("%v became a hold", cancel)
		}
	}
}

func TestCustomTouchScrollDismissesKeyboardAndSurvivesResize(t *testing.T) {
	for _, dy := range []float32{-40, 40} {
		var total float32
		tt := NewTester(func(c *Context) {
			Box(c).Fill().Focusable().TouchSelection().TextCaret(Rect{W: 1, H: 20}).InputOptions(InputOptions{Dismiss: KeyboardDismissOnDrag}).HandleInput(func(ev InputEvent) bool {
				if ev.Kind == InputScroll {
					total += ev.DY
				}
				return true
			})
		}, 320, 240)
		touch(tt, platform.PointerDown, 1, 120, 100)
		touch(tt, platform.PointerUp, 1, 120, 100)
		if !tt.h.ime.Active {
			t.Fatal("tap did not open keyboard")
		}
		touch(tt, platform.PointerDown, 2, 120, 100)
		touch(tt, platform.PointerMove, 2, 120, 100+dy)
		if tt.h.ime.Active {
			t.Fatal("vertical scroll retained keyboard")
		}
		tt.send(platform.SurfaceEvent{Kind: platform.SurfaceResize})
		touch(tt, platform.PointerMove, 2, 120, 100+2*dy)
		touch(tt, platform.PointerUp, 2, 120, 100+2*dy)
		if total != -2*dy || tt.rt.touch.active {
			t.Fatalf("keyboard resize interrupted scrolling: %v", total)
		}
	}
}

func TestHorizontalPanAndLongPressKeepKeyboard(t *testing.T) {
	for _, hold := range []bool{false, true} {
		tt := NewTester(func(c *Context) {
			Box(c).Fill().Focusable().TouchSelection().TextCaret(Rect{W: 1, H: 20}).InputOptions(InputOptions{Dismiss: KeyboardDismissOnDrag}).HandleInput(func(ev InputEvent) bool { return true })
		}, 320, 240)
		now := time.Now()
		tt.rt.clock = func() time.Time { return now }
		touch(tt, platform.PointerDown, 1, 120, 100)
		touch(tt, platform.PointerUp, 1, 120, 100)
		touch(tt, platform.PointerDown, 2, 120, 100)
		if hold {
			now = now.Add(time.Second)
			tt.send(platform.SurfaceEvent{Kind: platform.SurfaceFrame})
			touch(tt, platform.PointerMove, 2, 120, 140)
		} else {
			touch(tt, platform.PointerMove, 2, 180, 100)
		}
		if !tt.h.ime.Active {
			t.Fatal("selection or horizontal pan dismissed keyboard")
		}
		touch(tt, platform.PointerUp, 2, 180, 140)
	}
}

func TestNativeTouchPanDismissesKeyboardOnlyInOnDragMode(t *testing.T) {
	for _, mode := range []KeyboardDismissMode{KeyboardDismissNone, KeyboardDismissInteractive, KeyboardDismissOnDrag} {
		tt := NewTester(func(c *Context) {
			Box(c).Fill().AutoFocus().TextCaret(Rect{W: 1, H: 20}).InputOptions(InputOptions{Dismiss: mode}).HandleInput(func(ev InputEvent) bool { return true })
		}, 320, 240)
		tt.send(platform.SurfaceEvent{Kind: platform.PointerScroll, PointerType: platform.PointerMouse, X: 120, Y: 100, DY: 20, Precise: true})
		if !tt.h.ime.Active {
			t.Fatal("mouse/trackpad scroll dismissed software keyboard")
		}
		tt.send(platform.SurfaceEvent{Kind: platform.PointerScroll, PointerType: platform.PointerTouch, X: 120, Y: 100, DY: 20, Precise: true})
		if tt.h.ime.Active != (mode != KeyboardDismissOnDrag) {
			t.Fatalf("unexpected keyboard state for %s", mode)
		}
	}
}
