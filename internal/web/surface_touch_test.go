//go:build js

package web

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

type pointerEventRecorder struct {
	platform.WindowHandler
	event platform.SurfaceEvent
}

func (h *pointerEventRecorder) SurfaceEvent(event platform.SurfaceEvent) bool {
	h.event = event
	return true
}

func TestSurfaceForwardsTouchPointerEvents(t *testing.T) {
	for _, pointer := range []platform.PointerKind{platform.PointerMouse, platform.PointerPen, platform.PointerTouch} {
		for _, kind := range []platform.SurfaceEventKind{platform.PointerDown, platform.PointerMove, platform.PointerUp, platform.PointerCancel, platform.PointerLeave} {
			handler := &pointerEventRecorder{}
			surface := &surface{w: &window{h: handler}}
			event := platform.SurfaceEvent{Kind: kind, Pointer: pointer, X: 21, Y: 34, Button: 1}
			if !surface.send(event) {
				t.Fatal("event was not forwarded")
			}
			want := event
			want.Touch = pointer == platform.PointerTouch
			if !reflect.DeepEqual(handler.event, want) {
				t.Errorf("forwarded %v/%v = %+v, want %+v", pointer, kind, handler.event, want)
			}
		}
	}
}

// A finger the browser takes ends as a phone's does, with a release that
// clicks nothing, far from what it pressed; a mouse's or a pen's press is
// canceled where it was.
func TestCancelEvent(t *testing.T) {
	ev := cancelEvent(platform.PointerTouch, 21, 34, platform.ModShift)
	want := platform.SurfaceEvent{Kind: platform.PointerUp, X: -1e6, Y: -1e6, Mods: platform.ModShift, Pointer: platform.PointerTouch, Canceled: true}
	if !reflect.DeepEqual(ev, want) {
		t.Errorf("finger: %+v, want %+v", ev, want)
	}
	for _, pointer := range []platform.PointerKind{platform.PointerMouse, platform.PointerPen} {
		ev := cancelEvent(pointer, 21, 34, 0)
		want := platform.SurfaceEvent{Kind: platform.PointerCancel, X: 21, Y: 34, Pointer: pointer}
		if !reflect.DeepEqual(ev, want) {
			t.Errorf("%v: %+v, want %+v", pointer, ev, want)
		}
	}
}
