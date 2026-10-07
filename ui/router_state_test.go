package ui

import (
	"encoding/json"
	"fmt"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

func TestRouterJSONRoundTrip(t *testing.T) {
	r := NewRouter("/home")
	r.Push("/中文?text=%F0%9F%91%8B")
	r.Push("/edit")
	r.Replace("/中文?text=%F0%9F%91%8B") // duplicate adjacent entries must survive
	r.Pop()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var restored Router
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Location() != r.Location() || restored.at != 1 || len(restored.entries) != 3 || !restored.CanGoForward() || restored.Query("text") != "👋" {
		t.Fatalf("restored: %s, index=%d, entries=%d", restored.Location(), restored.at, len(restored.entries))
	}
	if restored.InteractiveBack != (runtime.GOOS == "ios") {
		t.Fatal("fresh restored router lost platform defaults")
	}
	restored.Forward()
	if restored.at != 2 {
		t.Fatal("duplicate entry was dropped")
	}
	restored.Pop()
	restored.Push("/other")
	if restored.CanGoForward() || r.entries[2].loc != "/%E4%B8%AD%E6%96%87?text=%F0%9F%91%8B" {
		t.Fatal("push retained a forward branch or restoration aliased its source")
	}
}

func TestRouterJSONQuerySharingAndLegacy(t *testing.T) {
	var r Router
	if err := json.Unmarshal([]byte(`["/search?q=a","/search?q=b","/detail"]`), &r); err != nil {
		t.Fatal(err)
	}
	if r.at != 2 || r.entries[0].pages[0] != r.entries[1].pages[0] || r.entries[1].pages[0] == r.entries[2].pages[0] {
		t.Fatal("legacy locations lost query sharing or current index")
	}
	r.Go(-1000)
	if r.Query("q") != "a" || r.CanGoBack() || !r.CanGoForward() {
		t.Fatal("restored history cannot traverse")
	}
	for i := range 120 {
		r.Push(fmt.Sprintf("/page/%d", i))
	}
	r.Pop()
	data, _ := json.Marshal(&r)
	var bounded Router
	if err := json.Unmarshal(data, &bounded); err != nil || len(bounded.entries) != maxHistory || bounded.at != maxHistory-2 || !bounded.CanGoForward() {
		t.Fatalf("bounded restore: %v, index=%d", err, bounded.at)
	}
}

func TestRestoredRouterSharedLayouts(t *testing.T) {
	paths := []string{"/list/group/a?q=1", "/list/group/a?q=2", "/list/group/b", "/list/other/c", "/away", "/list/group/a"}
	for _, legacy := range []bool{false, true} {
		for _, start := range []int{0, 1, 2} {
			t.Run(fmt.Sprintf("legacy=%v/index=%d", legacy, start), func(t *testing.T) {
				var data []byte
				if legacy {
					data, _ = json.Marshal(paths)
				} else {
					data, _ = json.Marshal(routerSnapshot{Version: 1, Entries: paths, Index: start})
				}
				var r Router
				if err := json.Unmarshal(data, &r); err != nil {
					t.Fatal(err)
				}
				if legacy {
					r.Go(start - r.at)
				}
				r.Transition = TransitionNone
				var values [3]string
				tt := NewTester(func(c *Context) {
					values = [3]string{}
					r.View(c, func(p *Route) {
						if !p.Match("/list/{rest...}") {
							Text(c, "Other root")
							return
						}
						outer := Local(p.Page(), "outer", func() string { return "outer" })
						TextInput(c, outer).Label("Outer editor").Width(250)
						values[0] = *outer
						p.View(c, func(p *Route) {
							if !p.Match("/group/{rest...}") {
								Text(c, "Other section")
								return
							}
							inner := Local(p.Page(), "inner", func() string { return "inner" })
							TextInput(c, inner).Label("Inner editor").Width(250)
							values[1] = *inner
							p.View(c, func(p *Route) {
								leaf := Local(p.Page(), "leaf", func() string { return "leaf" })
								TextInput(c, leaf).Label("Leaf editor").Width(250)
								values[2] = *leaf
							})
						})
					})
				}, 375, 480)
				edit := func(label string) {
					t.Helper()
					if err := tt.Click(label); err != nil {
						t.Fatal(err)
					}
					tt.Type(" edited")
				}
				goTo := func(index int) { r.Go(index - r.at); tt.Frame() }
				edit("Outer editor")
				edit("Inner editor")
				outer, inner := values[0], values[1]
				for _, index := range []int{2, 1, 0, 2, 0} {
					goTo(index)
					if values[0] != outer || values[1] != inner {
						t.Fatalf("history index %d recreated shared layouts: %v", index, values)
					}
				}
				edit("Leaf editor")
				leaf := values[2]
				goTo(1)
				if values[2] != leaf {
					t.Fatal("query navigation recreated a nested leaf")
				}
				goTo(2)
				if values[2] != "leaf" {
					t.Fatal("different leaf paths shared editor state")
				}
				goTo(3)
				if values[0] != outer || values[1] != "" {
					t.Fatal("another section lost the outer layout or reused its nested layout")
				}
				goTo(5)
				if values != [3]string{"outer", "inner", "leaf"} {
					t.Fatal("independent history branch reused a previous layout")
				}
				goTo(0)
				if values != [3]string{outer, inner, leaf} {
					t.Fatal("returning to the original branch lost its state")
				}
				// After restoration, the gesture must move only the leaf view.
				goTo(2)
				r.InteractiveBack, r.Transition = true, TransitionAuto
				now := time.Now()
				tt.rt.clock = func() time.Time { return now }
				touch(tt, platform.PointerDown, 1, 8, 200)
				now = now.Add(time.Second)
				touch(tt, platform.PointerMove, 1, 230, 200)
				if r.back == nil || r.back.level != 2 {
					t.Fatal("restored back gesture moved a shared layout")
				}
				now = now.Add(200 * time.Millisecond)
				touch(tt, platform.PointerUp, 1, 230, 200)
				finishBack(tt, &now)
				if r.at != 1 || values != [3]string{outer, inner, leaf} {
					t.Fatal("restored gesture lost a shared layout or query leaf")
				}
			})
		}
	}
}

func TestRestoredRouterNavigationBeforeFirstView(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(fmt.Sprint("replace=", replace), func(t *testing.T) {
			var r Router
			if err := json.Unmarshal([]byte(`["/list/a","/list/b"]`), &r); err != nil {
				t.Fatal(err)
			}
			if replace {
				r.Replace("/list/c")
			} else {
				r.Push("/list/c")
			}
			r.Transition = TransitionNone
			var latest string
			tt := NewTester(func(c *Context) {
				r.View(c, func(p *Route) {
					if p.Match("/list/{rest...}") {
						draft := Local(p.Page(), "draft", func() string { return "default" })
						TextInput(c, draft).Label("Draft").Width(250)
						latest = *draft
						p.View(c, func(p *Route) { Text(c, p.Path()) })
					}
				})
			}, 320, 240)
			if err := tt.Click("Draft"); err != nil {
				t.Fatal(err)
			}
			tt.Type(" edited")
			want := latest
			r.Pop()
			tt.Frame()
			if latest != want {
				t.Fatal("navigation before the first restored frame lost a shared layout")
			}
		})
	}
}

func TestRouterJSONRejectsInvalidStateAtomically(t *testing.T) {
	bad := []string{
		`null`, `{}`, `{"version":2,"entries":["/"]}`, `{"version":1,"entries":[]}`, `[]`,
		`{"version":1,"entries":["/"],"index":-1}`, `{"version":1,"entries":["/"],"index":1}`,
		`["relative"]`, `["https://example.com"]`, `["//example.com"]`, `["/bad%ZZ"]`, `["/a/../b"]`, `["/a#fragment"]`,
	}
	tooMany := make([]string, maxHistory+1)
	for i := range tooMany {
		tooMany[i] = "/"
	}
	oversized, _ := json.Marshal(tooMany)
	bad = append(bad, string(oversized))
	for _, input := range bad {
		t.Run(input, func(t *testing.T) {
			r := NewRouter("/original")
			r.Push("/original/detail")
			r.Transition, r.InteractiveBack = TransitionFade, true
			original, _ := json.Marshal(r)
			if err := json.Unmarshal([]byte(input), r); err == nil {
				t.Fatal("invalid state accepted")
			}
			after, _ := json.Marshal(r)
			if string(original) != string(after) || r.Transition != TransitionFade || !r.InteractiveBack {
				t.Fatal("invalid restore changed the live router")
			}
		})
	}
}

func TestRouterLiveRestoreAndReset(t *testing.T) {
	r := NewRouter("/old")
	r.Transition, r.InteractiveBack = TransitionNone, true
	tt := NewTester(func(c *Context) { r.View(c, func(p *Route) { Text(c, p.Path()) }) }, 320, 240)
	oldID, host := r.current().pages[0], r.rt
	if err := json.Unmarshal([]byte(`{"version":1,"entries":["/new","/new/detail"],"index":0}`), r); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("/new") || tt.HasText("/old") || r.current().pages[0] == oldID || r.rt != host || r.Transition != TransitionNone || !r.InteractiveBack {
		t.Fatal("live restore lost configuration, attachment or reused old element state")
	}
	r.Forward()
	tt.Frame()
	r.Reset("../")
	tt.Frame()
	if r.Path() != "/" || r.CanGoBack() || r.CanGoForward() || !tt.HasText("/") {
		t.Fatal("Reset did not clear both history directions")
	}
	r.Push("/child")
	r.Reset("/child")
	if !slices.Equal(r.History(), []string{"/child"}) {
		t.Fatal("Reset of the current location retained history")
	}
}

func TestRouterGestureCheckpointUsesOwnedHistory(t *testing.T) {
	tt, r, now := backFixture(t, func(c *Context, r *Router) { r.View(c, func(p *Route) { Text(c, p.Path()) }) })
	check := func(want string, forward bool) {
		t.Helper()
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		var restored Router
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatal(err)
		}
		if restored.Path() != want || restored.CanGoForward() != forward {
			t.Fatalf("checkpoint: %s, forward=%v", restored.Path(), restored.CanGoForward())
		}
	}
	touch(tt, platform.PointerDown, 1, 8, 100)
	*now = now.Add(time.Second)
	touch(tt, platform.PointerMove, 1, 230, 100)
	check("/list/detail", false)
	*now = now.Add(200 * time.Millisecond)
	touch(tt, platform.PointerUp, 1, 230, 100)
	finishBack(tt, now)
	check("/list", true)
}
