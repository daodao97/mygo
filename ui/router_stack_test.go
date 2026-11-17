package ui

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

// stackHost is a system's stack of screens, as phones have, recording what
// the content tells it.
type stackHost struct {
	can, moves bool
	stacks     [][]platform.StackScreen
	shown      int
}

func (s *stackHost) CanStack() bool                          { return s.can }
func (s *stackHost) StackMoves() bool                        { return s.moves }
func (s *stackHost) SetStack(screens []platform.StackScreen) { s.stacks = append(s.stacks, screens) }
func (s *stackHost) StackShown()                             { s.shown++ }

// titles returns the titles of the stack last set, and how many times one was.
func (s *stackHost) titles() ([]string, int) {
	if len(s.stacks) == 0 {
		return nil, 0
	}
	var out []string
	for _, sc := range s.stacks[len(s.stacks)-1] {
		out = append(out, sc.Title)
	}
	return out, len(s.stacks)
}

func stackTester(s *stackHost, r *Router, in *bool) *Tester {
	tt := coreNewTester(func(c *context) {
		if in != nil {
			*in = r.coreInSystemStack(c)
		}
		r.coreView(c, func(rt *Route) {
			switch {
			case rt.Match("/"):
				rt.Title("Home")
			case rt.Match("/notes"):
				rt.Title("Notes")
			case rt.Match("/notes/{id}"):
				rt.Title("Note " + rt.Param("id"))
			}
			coreBox(c).Fill()
		})
	}, 400, 600)
	tt.h.stack = s
	return tt
}

// The history up to the page shown is the system's stack: a push pushes a
// screen and shows the page at once, the system moving it.
func TestRouterSystemStack(t *testing.T) {
	enableTouch()
	s := &stackHost{can: true, moves: true}
	r := NewRouter("/")
	r.SystemStack = true
	var in bool
	tt := stackTester(s, r, &in)
	tt.Frame()
	if got, n := s.titles(); !slices.Equal(got, []string{"Home"}) || n != 1 || !in {
		t.Fatalf("first frame: stack %q (%d sets), in the stack %v", got, n, in)
	}
	if s.shown != 1 {
		t.Errorf("shown %d times after the first stack", s.shown)
	}
	r.Push("/notes")
	tt.Frame()
	r.Push("/notes/7")
	tt.Frame()
	if got, n := s.titles(); !slices.Equal(got, []string{"Home", "Notes", "Note 7"}) || n != 3 {
		t.Fatalf("pushed: stack %q (%d sets)", got, n)
	}
	if r.view.leaving != nil {
		t.Errorf("the router slides its pages while the system moves them")
	}
	if tt.rt.edgeBack != nil {
		t.Errorf("the router takes the edge swipe the system's stack has")
	}
	// Frames that change nothing tell the surface nothing.
	tt.Frame()
	tt.Advance(time.Second)
	if _, n := s.titles(); n != 3 {
		t.Errorf("a frame changing nothing set the stack: %d sets", n)
	}
	// The user pops a screen: the router goes back, the surface hears of
	// the stack as the system has it, and once the page shows.
	shown := s.shown
	tt.rt.stackPopped(2)
	tt.Frame()
	if r.Path() != "/notes" {
		t.Fatalf("popped to two screens: at %s", r.Path())
	}
	if got, n := s.titles(); !slices.Equal(got, []string{"Home", "Notes"}) || n != 4 {
		t.Errorf("popped: stack %q (%d sets)", got, n)
	}
	if s.shown != shown+1 {
		t.Errorf("shown %d times, want %d", s.shown, shown+1)
	}
	// Going back from the view pops it.
	r.Back()
	tt.Frame()
	if got, _ := s.titles(); !slices.Equal(got, []string{"Home"}) {
		t.Errorf("back: stack %q", got)
	}
	// Replacing the page replaces its screen, of another key.
	r.Push("/notes")
	tt.Frame()
	key := s.stacks[len(s.stacks)-1][1].Key
	r.Replace("/notes/9")
	tt.Frame()
	last := s.stacks[len(s.stacks)-1]
	if len(last) != 2 || last[1].Title != "Note 9" || last[1].Key == key || last[0].Key != s.stacks[0][0].Key {
		t.Errorf("replaced: stack %+v, the replaced key %d", last, key)
	}
	// Without SystemStack, the stack goes.
	r.SystemStack = false
	tt.Frame()
	if got, _ := s.titles(); got != nil || in {
		t.Errorf("off: stack %q, in the stack %v", got, in)
	}
}

// Where the system shows the bar but leaves the pages to the content, as on
// Android, the router slides them and takes the system's back gesture.
func TestRouterSystemStackStill(t *testing.T) {
	enableTouch()
	s := &stackHost{can: true}
	r := NewRouter("/")
	r.SystemStack = true
	tt := stackTester(s, r, nil)
	tt.Frame()
	r.Push("/notes")
	tt.Frame()
	if r.view.leaving == nil {
		t.Errorf("the page doesn't slide in")
	}
	if tt.rt.edgeBack != r {
		t.Errorf("the router doesn't take the system's back gesture")
	}
	if got, _ := s.titles(); !slices.Equal(got, []string{"Home", "Notes"}) {
		t.Errorf("stack %q", got)
	}
}

// A window that can't show a stack, or a router without SystemStack, tells
// the surface nothing.
func TestRouterSystemStackNone(t *testing.T) {
	for _, c := range []struct {
		name   string
		can    bool
		system bool
	}{{"can't", false, true}, {"not asked", true, false}} {
		s := &stackHost{can: c.can, moves: true}
		r := NewRouter("/")
		r.SystemStack = c.system
		var in bool
		tt := stackTester(s, r, &in)
		tt.Frame()
		r.Push("/notes")
		tt.Frame()
		if len(s.stacks) != 0 || s.shown != 0 || in {
			t.Errorf("%s: %d stacks, shown %d times, in the stack %v", c.name, len(s.stacks), s.shown, in)
		}
		if r.view.leaving == nil {
			t.Errorf("%s: the page doesn't slide in", c.name)
		}
	}
}
