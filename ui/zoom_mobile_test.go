package ui

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func TestZoomMobileInsetsAndTouch(t *testing.T) {
	zoom := float32(1)
	var insets SafeArea
	var events []InputEvent
	tt := NewTester(func(c *Context) {
		c.SetZoom(zoom)
		insets = c.SafeArea()
		Box(c).Absolute().Left(60).Top(20).Size(30, 30).Focusable().TouchSelection().
			TextCaret(Rect{5, 6, 1, 12}).HandleInput(func(ev InputEvent) bool {
			if ev.Kind == InputPointerDown || ev.Kind == InputPointerUp || ev.Kind == InputScroll {
				events = append(events, ev)
			}
			return true
		})
	}, 320, 200)
	defer tt.rt.close()
	tt.SetSafeArea(SafeArea{Top: 24, Right: 8, Bottom: 40, Left: 12})
	zoom = 2
	tt.Frame()
	if insets != (SafeArea{Top: 12, Right: 4, Bottom: 20, Left: 6}) {
		t.Fatalf("safe area did not follow the frame's zoom: %+v", insets)
	}
	touch(tt, platform.PointerDown, 1, 140, 60)
	touch(tt, platform.PointerUp, 1, 140, 60)
	if len(events) != 2 || events[0].Kind != InputPointerDown || events[1].Kind != InputPointerUp || events[0].X != 10 || events[0].Y != 10 {
		t.Fatalf("zoomed tap missed the custom widget: %+v", events)
	}
	if !tt.h.ime.Active || tt.h.ime.Caret != (platform.RectF{X: 130, Y: 52, W: 2, H: 24}) {
		t.Fatalf("custom caret is not in window DIPs: %+v", tt.h.ime)
	}
	events = nil
	touch(tt, platform.PointerDown, 2, 140, 70)
	touch(tt, platform.PointerMove, 2, 140, 30)
	touch(tt, platform.PointerUp, 2, 140, 30)
	var scrolled float32
	for _, ev := range events {
		if ev.Kind != InputScroll || !ev.Precise {
			t.Fatalf("zoomed scroll became a selection: %+v", events)
		}
		scrolled += ev.DY
	}
	if scrolled != 20 {
		t.Fatalf("zoomed touch scrolled %v content DIPs, want 20", scrolled)
	}
}

func TestZoomNativeTextGeometry(t *testing.T) {
	value := "Hello world"
	tt := snapshotTester(func(c *Context) {
		c.SetZoom(2)
		Column(c).Padding(10).Children(func() {
			TextInput(c, &value).Size(120, 40).AutoFocus()
		})
	}, 320, 200)
	defer tt.rt.close()
	state := tt.h.ime
	if state.Bounds != (platform.RectF{X: 20, Y: 20, W: 240, H: 80}) {
		t.Fatalf("native field bounds are not in window DIPs: %+v", state.Bounds)
	}
	for _, index := range []int{0, 3, len(value)} {
		g := tt.rt.textGeometry(platform.TextGeometryQuery{ID: state.ID, Kind: "caret", Start: index})
		if !g.Valid || g.Caret.W != 2 || g.Caret.H <= 0 {
			t.Fatalf("missing zoomed caret at %d: %+v", index, g)
		}
		hit := tt.rt.textGeometry(platform.TextGeometryQuery{ID: state.ID, Kind: "hit", X: g.Caret.X, Y: g.Caret.Y + g.Caret.H/2})
		if !hit.Valid || hit.Index != index {
			t.Fatalf("zoomed caret hit %d instead of %d", hit.Index, index)
		}
	}
	s := tt.rt.states[tt.rt.focused]
	if state.Caret != tt.rt.toWindow(platformRect(s.editor.caretRect(s))) {
		t.Fatalf("snapshot caret is not in window DIPs: %+v", state.Caret)
	}
	g := tt.rt.textGeometry(platform.TextGeometryQuery{ID: state.ID, Kind: "selection", End: len(value)})
	if !g.Valid || len(g.Rects) == 0 {
		t.Fatal("zoomed native selection has no geometry")
	}
	for _, r := range g.Rects {
		if r.X < state.Bounds.X || r.Y < state.Bounds.Y || r.X+r.W > state.Bounds.X+state.Bounds.W || r.Y+r.H > state.Bounds.Y+state.Bounds.H {
			t.Fatalf("zoomed selection escaped the native field: %+v", r)
		}
	}
}
