package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

func backFixture(t *testing.T, view func(*Context, *Router)) (*Tester, *Router, *time.Time) {
	t.Helper()
	r := NewRouter("/list")
	r.InteractiveBack, r.Transition = true, TransitionNone
	tt := NewTester(func(c *Context) { view(c, r) }, 320, 480)
	now := time.Now()
	tt.rt.clock = func() time.Time { return now }
	r.Push("/list/detail")
	tt.Frame()
	r.Transition = TransitionAuto
	return tt, r, &now
}

func finishBack(tt *Tester, now *time.Time) {
	*now = now.Add(250 * time.Millisecond)
	tt.Frame()
}

func TestInteractiveBackTracksAndCommits(t *testing.T) {
	tt, r, now := backFixture(t, func(c *Context, r *Router) {
		r.View(c, func(p *Route) { Text(c, p.Path()) })
	})
	before, _ := tt.Find("/list/detail")
	touch(tt, platform.PointerDown, 1, 8, 100)
	*now = now.Add(time.Second)
	touch(tt, platform.PointerMove, 1, 208, 100)
	moved, ok := tt.Find("/list/detail")
	if !ok || abs32(moved.X-before.X-200) > 1 || r.Path() != "/list/detail" || len(r.History()) != 2 {
		t.Fatalf("preview moved %v → %v, path=%s history=%v", before, moved, r.Path(), r.History())
	}
	*now = now.Add(200 * time.Millisecond)
	touch(tt, platform.PointerUp, 1, 208, 100)
	if r.Path() != "/list/detail" {
		t.Fatal("history changed before the settling animation completed")
	}
	finishBack(tt, now)
	if r.Path() != "/list" || r.CanGoBack() || !r.CanGoForward() || r.view.leaving != nil {
		t.Fatalf("completion: path=%s back=%v forward=%v", r.Path(), r.CanGoBack(), r.CanGoForward())
	}
	history := r.History()
	history[0] = "/mutated"
	if r.Path() != "/list" {
		t.Fatal("history snapshot aliases the router")
	}
	// A root-page edge gesture must not navigate beyond the stack.
	touch(tt, platform.PointerDown, 2, 8, 100)
	touch(tt, platform.PointerMove, 2, 250, 100)
	touch(tt, platform.PointerUp, 2, 250, 100)
	finishBack(tt, now)
	if r.Path() != "/list" || r.back != nil {
		t.Fatal("root page accepted a back gesture")
	}
}

func TestInteractiveBackCancellationPreservesEditor(t *testing.T) {
	for _, kind := range []platform.SurfaceEventKind{platform.PointerUp, platform.PointerCancel, platform.SurfaceBlur} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			value := "中文笔记 👋"
			tt, r, now := backFixture(t, func(c *Context, r *Router) {
				r.View(c, func(p *Route) {
					if p.Path() == "/list/detail" {
						TextInput(c, &value).Label("Editor").AutoFocus()
					} else {
						Text(c, "List")
					}
				})
			})
			focus := tt.rt.focused
			touch(tt, platform.PointerDown, 1, 8, 100)
			*now = now.Add(time.Second)
			touch(tt, platform.PointerMove, 1, 68, 100)
			if tt.rt.focused != focus || !tt.h.ime.Active {
				t.Fatal("preview blurred the editor")
			}
			*now = now.Add(200 * time.Millisecond)
			if kind == platform.SurfaceBlur {
				tt.send(platform.SurfaceEvent{Kind: kind})
			} else {
				touch(tt, kind, 1, 68, 100)
			}
			finishBack(tt, now)
			if r.Path() != "/list/detail" || value != "中文笔记 👋" || tt.rt.focused != focus {
				t.Fatal("cancel lost the page, draft or focus")
			}
			if kind != platform.SurfaceBlur && !tt.h.ime.Active {
				t.Fatal("cancel dismissed the keyboard")
			}
			if tt.rt.touch.active || tt.rt.pressed != nil {
				t.Fatal("cancel left a contact pressed")
			}
		})
	}
}

func TestInteractiveBackYieldsToScrollAndEdgeTap(t *testing.T) {
	clicks := 0
	var scroll ScrollState
	tt, r, now := backFixture(t, func(c *Context, r *Router) {
		r.View(c, func(p *Route) {
			Scroll(c).Fill().TrackScroll(&scroll).Children(func() {
				for i := range 20 {
					if Button(c, fmt.Sprint("Row ", i)).Height(48).FillWidth().Clicked() {
						clicks++
					}
				}
			})
		})
	})
	touch(tt, platform.PointerDown, 1, 8, 70)
	touch(tt, platform.PointerUp, 1, 8, 70)
	if clicks != 1 {
		t.Fatal("edge tap was lost")
	}
	touch(tt, platform.PointerDown, 2, 8, 200)
	*now = now.Add(20 * time.Millisecond)
	touch(tt, platform.PointerMove, 2, 9, 140)
	touch(tt, platform.PointerUp, 2, 9, 140)
	if scroll.Y < 50 || r.back != nil || r.Path() != "/list/detail" || clicks != 1 {
		t.Fatalf("vertical edge scroll: y=%v path=%s clicks=%d", scroll.Y, r.Path(), clicks)
	}
	touch(tt, platform.PointerDown, 3, 80, 200)
	touch(tt, platform.PointerMove, 3, 280, 200)
	touch(tt, platform.PointerUp, 3, 280, 200)
	if r.back != nil || r.Path() != "/list/detail" {
		t.Fatal("non-edge drag navigated")
	}
}

func TestInteractiveBackNavigationOverridesPreview(t *testing.T) {
	tt, r, now := backFixture(t, func(c *Context, r *Router) {
		r.View(c, func(p *Route) { Text(c, p.Path()) })
	})
	touch(tt, platform.PointerDown, 1, 8, 100)
	touch(tt, platform.PointerMove, 1, 230, 100)
	r.Push("/other")
	tt.Frame()
	touch(tt, platform.PointerUp, 1, 230, 100)
	finishBack(tt, now)
	if r.Path() != "/other" || r.back != nil {
		t.Fatal("stale gesture overrode navigation")
	}
}

func TestInteractiveBackSharedRouteLayout(t *testing.T) {
	view := func(c *Context, r *Router) {
		r.View(c, func(p *Route) {
			if p.Match("/list/{rest...}") {
				Text(c, "Shared header")
				p.View(c, func(p *Route) { Text(c, "Nested "+p.Path()) })
			}
		})
	}
	tt, r, now := backFixture(t, view)
	touch(tt, platform.PointerDown, 1, 8, 100)
	*now = now.Add(time.Second)
	touch(tt, platform.PointerMove, 1, 208, 100)
	header, _ := tt.Find("Shared header")
	if r.back == nil || header.X != 0 || r.back.level != 1 {
		t.Fatal("shared layout did not retain its header")
	}
	*now = now.Add(200 * time.Millisecond)
	touch(tt, platform.PointerUp, 1, 208, 100)
	finishBack(tt, now)
	if r.Path() != "/list" || !tt.HasText("Nested /") {
		t.Fatal("nested back did not commit")
	}
}

func TestInteractiveBackReleaseDecision(t *testing.T) {
	for _, tc := range []struct {
		name           string
		first, last    float32
		delay          time.Duration
		cancel, reduce bool
		complete       bool
	}{
		{name: "fast flick", first: 40, last: 90, delay: 20 * time.Millisecond, complete: true},
		{name: "held short drag", first: 40, last: 90, delay: 20 * time.Millisecond},
		{name: "reversed drag", first: 250, last: 90, delay: 100 * time.Millisecond},
		{name: "cancel beyond halfway", first: 230, last: 230, delay: 200 * time.Millisecond, cancel: true},
		{name: "reduced motion", first: 230, last: 230, delay: 200 * time.Millisecond, reduce: true, complete: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tt, r, now := backFixture(t, func(c *Context, r *Router) {
				r.View(c, func(p *Route) { Text(c, p.Path()) })
			})
			if tc.reduce {
				tt.SetPreferences(Preferences{ReduceMotion: true, TextScale: 1})
			}
			touch(tt, platform.PointerDown, 1, 8, 100)
			*now = now.Add(time.Second)
			touch(tt, platform.PointerMove, 1, tc.first, 100)
			*now = now.Add(tc.delay)
			touch(tt, platform.PointerMove, 1, tc.last, 100)
			if tc.name == "held short drag" {
				*now = now.Add(200 * time.Millisecond)
			}
			kind := platform.PointerUp
			if tc.cancel {
				kind = platform.PointerCancel
			}
			touch(tt, kind, 1, tc.last, 100)
			if tc.reduce && r.back != nil {
				t.Fatal("Reduce Motion left a settling animation")
			}
			finishBack(tt, now)
			want := "/list/detail"
			if tc.complete {
				want = "/list"
			}
			if r.Path() != want || r.back != nil {
				t.Fatalf("release: path=%s, want=%s, preview=%v", r.Path(), want, r.back)
			}
		})
	}
}

func TestInteractiveBackWaitsForNestedTransition(t *testing.T) {
	tt, r, now := backFixture(t, func(c *Context, r *Router) {
		r.View(c, func(p *Route) {
			if p.Match("/list/{rest...}") {
				p.View(c, func(p *Route) { Text(c, p.Path()) })
			}
		})
	})
	r.Push("/list/detail/edit")
	tt.Frame()
	touch(tt, platform.PointerDown, 1, 8, 100)
	touch(tt, platform.PointerMove, 1, 220, 100)
	touch(tt, platform.PointerUp, 1, 220, 100)
	if r.back != nil || r.Path() != "/list/detail/edit" {
		t.Fatal("edge gesture interrupted the nested transition")
	}
	*now = now.Add(time.Second)
	tt.Frame()
	touch(tt, platform.PointerDown, 2, 8, 100)
	*now = now.Add(time.Second)
	touch(tt, platform.PointerMove, 2, 220, 100)
	*now = now.Add(200 * time.Millisecond)
	touch(tt, platform.PointerUp, 2, 220, 100)
	finishBack(tt, now)
	if r.Path() != "/list/detail" {
		t.Fatal("settled nested view did not accept a back gesture")
	}
}
