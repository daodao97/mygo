package ui

import (
	"github.com/egoist/mygo/internal/platform"
	"math"
	"testing"
)

func TestGestureCaptureAndDeltas(t *testing.T) {
	var updates []GestureEvent
	visible := true
	tt := NewTester(func(c *Context) {
		if visible {
			pad := Box(c).Label("pad").Size(200, 200)
			updates = append(updates, pad.Gestures()...)
		}
	}, 320, 240)
	send := func(kind, phase string, x, scale, rotation float64) {
		tt.send(platform.SurfaceEvent{Kind: platform.SurfaceGesture, Gesture: kind, Phase: phase, X: x, Y: 50, Scale: scale, Rotation: rotation})
	}
	send("pinch", "begin", 50, 1, 0)
	send("pinch", "change", 280, 1.5, 0) // outside pad: retain capture
	send("rotate", "begin", 50, 1, 0)
	send("rotate", "change", 280, 1, .4)
	send("pinch", "end", 280, 3, 0)
	if len(updates) != 5 || updates[1].Scale != 1.5 || updates[4].Scale != 2 || math.Abs(float64(updates[3].Rotation)-.4) > 1e-6 {
		t.Fatalf("deltas %+v", updates)
	}
	before := len(updates)
	send("pinch", "change", 50, 2, 0)
	if len(updates) != before {
		t.Fatal("update after end retargeted")
	}
	visible = false
	tt.Frame()
	send("rotate", "change", 50, 1, .7)
	if len(updates) != before {
		t.Fatal("gesture reached a removed element")
	}
}
func TestGestureCancelsPressAndIgnoresOrdinaryWidgets(t *testing.T) {
	clicks := 0
	var updates []GestureEvent
	tt := NewTester(func(c *Context) {
		e := Button(c, "pad").Size(200, 150)
		updates = append(updates, e.Gestures()...)
		if e.Clicked() {
			clicks++
		}
	}, 320, 240)
	touch(tt, platform.PointerDown, 1, 50, 50)
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceGesture, Gesture: "pinch", Phase: "begin", X: 50, Y: 50, Scale: 1})
	touch(tt, platform.PointerUp, 1, 50, 50)
	if clicks != 0 || tt.rt.pressed != nil || tt.rt.touch.active || len(updates) != 1 {
		t.Fatal("multi-touch activated its cancelled press")
	}
	plain := NewTester(func(c *Context) { Button(c, "plain").Size(200, 150) }, 320, 240)
	if plain.rt.gestureEvent(platform.SurfaceEvent{Gesture: "pinch", Phase: "probe", X: 50, Y: 50, Scale: 1}) {
		t.Fatal("unregistered widget stole a gesture")
	}
}
func TestKeyboardDismissOnScroll(t *testing.T) {
	value := "draft"
	clicks := 0
	tt := NewTester(func(c *Context) {
		Scroll(c).Fill().Children(func() {
			TextInput(c, &value).Label("editor").Height(44).AutoFocus().InputOptions(InputOptions{Dismiss: KeyboardDismissOnDrag})
			for i := range 15 {
				if Button(c, string(rune('a'+i))).Height(44).Clicked() {
					clicks++
				}
			}
		})
	}, 320, 240)
	// Touch outside the editor without first stealing its focus.
	touch(tt, platform.PointerDown, 1, 310, 150)
	touch(tt, platform.PointerMove, 1, 310, 100)
	touch(tt, platform.PointerUp, 1, 310, 100)
	if tt.h.ime.Active || value != "draft" || clicks != 0 {
		t.Fatalf("dismiss: active %v value %q clicks %d", tt.h.ime.Active, value, clicks)
	}
}

func TestInteractiveKeyboardKeepsFocusUntilNativeDismissal(t *testing.T) {
	value := "draft"
	tt := NewTester(func(c *Context) {
		Column(c).Fill().Children(func() {
			TextInput(c, &value).Label("editor").Height(44).AutoFocus().InputOptions(InputOptions{Dismiss: KeyboardDismissInteractive})
			Button(c, "button").Height(44)
			Box(c).Grow(1)
		})
	}, 320, 240)
	focus := tt.rt.focused
	touch(tt, platform.PointerDown, 1, 50, 65)
	touch(tt, platform.PointerCancel, 1, 50, 65) // UIKit took the contact for its pan
	if tt.rt.focused != focus || !tt.h.ime.Active {
		t.Fatal("native keyboard pan lost its first responder")
	}
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceKeyboardDismiss})
	if tt.h.ime.Active || value != "draft" {
		t.Fatal("native dismissal lost draft or retained keyboard")
	}
	tt.Click("editor")
	touch(tt, platform.PointerDown, 2, 50, 65)
	touch(tt, platform.PointerUp, 2, 50, 65)
	if tt.h.ime.Active {
		t.Fatal("ordinary tap did not dismiss keyboard")
	}
}
