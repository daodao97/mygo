package ui

import (
	"testing"
	"time"
)

func TestRouterStackTransitionsFollowHistory(t *testing.T) {
	r := NewRouter("/thread")
	r.Transition = TransitionStack
	r.Animation = &Animation{Duration: 100 * time.Millisecond, Ease: Linear}
	labels := map[string]*node{}
	tt, now := clockTester(func(c *context) {
		r.coreView(c, func(page *Route) { labels[page.Path()] = coreText(c, "Page "+page.Path()) })
	}, 400, 300)
	for _, step := range []struct {
		action    func()
		path      string
		direction int
	}{
		{func() { r.Push("/automations") }, "/automations", 1},
		{func() { r.Push("/settings") }, "/settings", 1},
		{r.Back, "/automations", -1},
		{r.Forward, "/settings", 1},
		{r.Back, "/automations", -1},
		{r.Back, "/thread", -1},
		{func() { r.Push("/approval") }, "/approval", 1},
	} {
		previous := r.Path()
		step.action()
		tt.Frame()
		if r.Path() != step.path || r.view.dir != step.direction || r.view.leaving == nil {
			t.Fatalf("navigation to %s: path=%s direction=%d", step.path, r.Path(), r.view.dir)
		}
		*now = now.Add(50 * time.Millisecond)
		tt.Frame()
		x := at(t, tt, "Page "+step.path).X
		if step.direction < 0 {
			// The page above it slides out while the previous page returns.
			x = labels[previous].Bounds().X
		}
		if x < 199 || x > 201 {
			t.Fatalf("halfway to %s: moving page x=%v, want 200", step.path, x)
		}
		*now = now.Add(60 * time.Millisecond)
		tt.Frame()
		if x := at(t, tt, "Page "+step.path).X; x != 0 || r.Transitioning() {
			t.Fatalf("settled at %s: x=%v transitioning=%v", step.path, x, r.Transitioning())
		}
	}
}

func TestRouterStackReducedMotionUsesFade(t *testing.T) {
	r := NewRouter("/thread")
	r.Transition = TransitionStack
	tt, now := clockTester(func(c *context) {
		r.coreView(c, func(page *Route) { coreText(c, "Page "+page.Path()) })
	}, 400, 300)
	tt.SetPreferences(Preferences{ReduceMotion: true})
	r.Push("/automations")
	tt.Frame()
	if r.view.dir != 0 || at(t, tt, "Page /automations").X != 0 {
		t.Fatal("reduced motion should fade the new page without sliding")
	}
	*now = now.Add(time.Second)
	tt.Frame()
	if r.Transitioning() || !tt.HasText("Page /automations") {
		t.Fatal("reduced-motion navigation did not finish")
	}
}
