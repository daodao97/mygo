package ui

import (
	"runtime"
	"slices"
	"sync"

	"github.com/egoist/mygo/internal/text"
)

// testerTexts are the text systems of Testers that ended, which the next
// Testers take rather than start their own (#2085). A Tester shapes and
// paints in a text system of its own, so that Testers that run at the same
// time, as parallel tests do, neither race over nor invalidate each
// other's (#1722); but starting one costs much more than drawing in it
// (its engine, on Linux a Pango font map, and the shaping of every text
// it shows), which a test suite of many Testers paid each time. A Tester
// ends as it becomes unreachable, or as Render returns.
//
// Each text system has one Tester at a time: a Tester takes one that no
// other Tester draws in, or makes one, and only gives it back once it
// ends. Reuse makes it as a new one is to its next Tester, but for the
// fonts and layouts it made, which depend on no window.
var testerTexts struct {
	mu   sync.Mutex
	idle []*text.System
	// made counts the text systems Testers made, for tests.
	made int
}

// maxIdleTesterTexts is how many idle text systems the Testers keep: as
// many as Testers usually run at once, which go test runs as many of as
// GOMAXPROCS (-parallel). Those over it go, which a burst of parallel
// Testers makes. testerTexts.mu guards it, which tests set.
var maxIdleTesterTexts = func() int { return runtime.GOMAXPROCS(0) }

// takeTesterText returns a text system for a new Tester: an idle one, or a
// new one if all are taken.
func takeTesterText() *text.System {
	p := &testerTexts
	p.mu.Lock()
	defer p.mu.Unlock()
	if n := len(p.idle); n > 0 {
		s := p.idle[n-1]
		p.idle[n-1] = nil
		p.idle = p.idle[:n-1]
		p.mu.Unlock()
		s.Reuse()
		p.mu.Lock()
		return s
	}
	p.made++
	return text.New()
}

// leaveTesterText takes back the text system of a Tester that ended, or
// lets it go if enough are idle. The next Tester readies it (System.Reuse):
// this runs as a cleanup, while another Tester may be counting a frame's
// allocations (testing.AllocsPerRun counts the process's), so it
// allocates nothing once the pool has room.
func leaveTesterText(s *text.System) {
	p := &testerTexts
	p.mu.Lock()
	defer p.mu.Unlock()
	max := maxIdleTesterTexts()
	if len(p.idle) >= max {
		return
	}
	// Room for all at once, the first time.
	if room := min(max, 256); cap(p.idle) < room {
		p.idle = slices.Grow(p.idle, room-len(p.idle))
	}
	p.idle = append(p.idle, s)
}

// keepText has the Tester's text system go back to the Testers after it
// once the Tester is unreachable: nothing that draws in it is reachable
// then, as the Tester's runtime and window refer to it (their clocks).
func (t *Tester) keepText() {
	t.textLeft = runtime.AddCleanup(t, leaveTesterText, t.h.text)
}

// end gives the Tester's text system back at once, for Render, which
// never uses the Tester again.
func (t *Tester) end() {
	t.textLeft.Stop()
	leaveTesterText(t.h.text)
}
