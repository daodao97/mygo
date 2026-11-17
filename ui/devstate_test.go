//go:build !mygo_noinspector

package ui

import (
	"encoding/json"
	"strconv"
	"testing"
)

// A window comes back from what it wrote down: scroll offsets, what widgets keep,
// and where a router is.
func TestDevStateRoundTrip(t *testing.T) {
	defer func(a func(*engine, *state), b func(*state, any, any), c func(*Router, *node)) {
		restoreElement, restoreLocal, noteRouter = a, b, c
	}(restoreElement, restoreLocal, noteRouter)
	restoreElement, restoreLocal, noteRouter = keptElementFor, keptLocalFor, noteDevRouter

	type sheet struct {
		open  bool
		count int
	}
	show := false
	view := func(r *Router, y *float32, s **sheet) func(*context) {
		return func(c *context) {
			if !show {
				return // until the window is ready to take it back
			}
			e := coreScroll(c).Fill().Key("list")
			*s = coreLocal(e, "sheet", func() sheet { return sheet{count: 1} })
			e.Children(func() {
				coreBox(c).Height(1000).Shrink(0)
				*y = float32(e.st.scrollY)
				r.coreView(c, func(rt *Route) { coreBox(c).Height(20) })
			})
		}
	}

	r := NewRouter("/a")
	var y float32
	var s *sheet
	first := coreNewTester(view(r, &y, &s), 200, 200)
	first.rt.dev = &devState{routers: map[uint64]*Router{}}
	show = true
	first.Frame()
	r.Push("/a/b?x=1")
	s.open, s.count = true, 7
	first.Scroll(100, 100, 0, 300)
	first.Frame()
	first.Frame()
	if y == 0 {
		t.Fatal("it didn't scroll")
	}
	data := first.rt.keptNow()

	var kept keptWindow
	if err := json.Unmarshal(data, &kept); err != nil {
		t.Fatal(err)
	}
	r2 := NewRouter("/a")
	var y2 float32
	var s2 *sheet
	show = false
	second := coreNewTester(view(r2, &y2, &s2), 200, 200)
	second.rt.dev = &devState{kept: &kept, routers: map[uint64]*Router{}}
	show = true
	second.Frame()
	second.Frame()
	if y2 != y {
		t.Errorf("scroll: %v, want %v", y2, y)
	}
	if s2 == nil || !s2.open || s2.count != 7 {
		t.Errorf("local: %+v", s2)
	}
	if r2.Path() != "/a/b" || !r2.CanGoBack() {
		t.Errorf("router: at %s", r2.Path())
	}
}

// A route the change of the code removed leaves no blank page: the router starts
// where it would have.
func TestDevStateRouteGone(t *testing.T) {
	defer func(a func(*Router, *node), b func(*Router, *node)) { noteRouter, routerBuilt = a, b }(noteRouter, routerBuilt)
	noteRouter, routerBuilt = noteDevRouter, devRouterBuilt
	show := false
	r := NewRouter("/home")
	tt := coreNewTester(func(c *context) {
		if !show {
			return
		}
		r.coreView(c, func(rt *Route) {
			if rt.Match("/home") {
				coreText(c, "home")
			}
		})
	}, 200, 200)
	tt.rt.dev = &devState{routers: map[uint64]*Router{}}
	show = true
	// Every router's element has one ID in this view: the kept state names it once built.
	tt.Frame()
	var id uint64
	for k := range tt.rt.dev.routers {
		id = k
	}
	tt.rt.dev.routers = map[uint64]*Router{}
	tt.rt.dev.kept = &keptWindow{Routers: map[string]*keptRouter{
		strconv.FormatUint(id, 10): {Locs: []string{"/removed"}},
	}}
	r2 := NewRouter("/home")
	r = r2
	tt.Frame()
	tt.Frame()
	if r2.Path() != "/home" {
		t.Errorf("at %s", r2.Path())
	}
	if !tt.HasText("home") {
		t.Errorf("shows %q", tt.Texts())
	}
}

// Without mygo dev, nothing of it runs.
func TestDevStateOff(t *testing.T) {
	h := windowHost{rt: &engine{}}
	attachDevState(&h)
	if h.rt.dev != nil {
		t.Error("attached")
	}
}
