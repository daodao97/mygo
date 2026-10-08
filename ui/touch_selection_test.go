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
