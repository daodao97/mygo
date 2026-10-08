package ui

import (
	"testing"
	"time"
)

func TestTextCaretFuncFollowsThePaintedFrame(t *testing.T) {
	var painted Rect
	x := float32(10)
	tt := newRepaintTester(func(c *Context) {
		Box(c).Fill().AutoFocus().HandleInput(func(InputEvent) bool { return true }).TextCaretFunc(func() Rect { return painted }).Draw(func(p *Painter, r Rect) {
			painted = Rect{X: x, Y: r.H - 24, W: 2, H: 20}
			p.AnimationFrame()
		})
	})
	check := func() {
		t.Helper()
		if got, active := tt.TextCaret(); !active || got != painted {
			t.Fatalf("input method uses a different frame: caret=%+v painted=%+v active=%v", got, painted, active)
		}
	}
	check()
	x = 45
	if built, _ := tt.frame(16 * time.Millisecond); built {
		t.Fatal("cursor-only repaint rebuilt the view")
	}
	check()
	tt.SetSize(390, 300)
	check()
}
