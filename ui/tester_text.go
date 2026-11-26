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
// GOMAXPROCS (-parallel). Those over it, which a burst of parallel Testers
// leaves, close as the next Tester starts. testerTexts.mu guards it, which
// tests set.
var maxIdleTesterTexts = func() int { return runtime.GOMAXPROCS(0) }

// testerTextsRoom is how many text systems of ended Testers wait for the
// next Tester at most; those over it go, and close once unreachable.
const testerTextsRoom = 256

// takeTesterText returns a text system for a new Tester: an idle one, or a
// new one if all are taken. It closes the idle ones over
// maxIdleTesterTexts here, on the goroutine starting the Tester: let go,
// they would close once unreachable, on the runtime's cleanup goroutine,
// while another Tester may be counting a frame's allocations
// (testing.AllocsPerRun counts the process's), and closing an engine
// allocates.
func takeTesterText() *text.System {
	p := &testerTexts
	p.mu.Lock()
	var s *text.System
	if n := len(p.idle); n > 0 {
		s = p.idle[n-1]
		p.idle[n-1] = nil
		p.idle = p.idle[:n-1]
	} else {
		p.made++
	}
	var over []*text.System
	if max := maxIdleTesterTexts(); len(p.idle) > max {
		over = slices.Clone(p.idle[max:])
		clear(p.idle[max:])
		p.idle = p.idle[:max]
	}
	p.mu.Unlock()
	for _, o := range over {
		o.CloseEngine()
	}
	if s == nil {
		return text.New()
	}
	s.Reuse()
	return s
}

// leaveTesterText takes back the text system of a Tester that ended, for
// the next Tester, which readies it (System.Reuse), or closes it if enough
// are idle. This runs as a cleanup, while another Tester may be counting a
// frame's allocations, so it allocates nothing, and closes nothing, once
// the pool has room.
func leaveTesterText(s *text.System) {
	p := &testerTexts
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.idle) >= testerTextsRoom {
		return
	}
	// Room for all at once, the first time.
	if cap(p.idle) < testerTextsRoom {
		p.idle = slices.Grow(p.idle, testerTextsRoom-len(p.idle))
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
