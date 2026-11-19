package ui

import (
	"bytes"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"testing"
	"weak"

	"github.com/egoist/mygo/internal/text"
	"github.com/egoist/mygo/internal/wait"
)

// testerTextsMade returns how many text systems Testers made so far.
func testerTextsMade() int {
	testerTexts.mu.Lock()
	defer testerTexts.mu.Unlock()
	return testerTexts.made
}

// testerTextIdle reports whether s is idle, for the next Tester to take.
func testerTextIdle(s *text.System) bool {
	testerTexts.mu.Lock()
	defer testerTexts.mu.Unlock()
	return s != nil && slices.Contains(testerTexts.idle, s)
}

// A Tester takes the text system an earlier Tester left rather than start
// one, which cost a suite of many Testers more than the first frame of a
// window of 250 texts each (#2085): one that Render ended, or one that
// became unreachable.
func TestTestersReuseTextSystems(t *testing.T) {
	// Testers of earlier tests may still end while this one runs: keep
	// every text system they leave, so that this Tester's stays.
	testerTexts.mu.Lock()
	max := maxIdleTesterTexts
	maxIdleTesterTexts = func() int { return 1 << 20 }
	testerTexts.mu.Unlock()
	t.Cleanup(func() {
		testerTexts.mu.Lock()
		maxIdleTesterTexts = max
		testerTexts.mu.Unlock()
	})

	coreRender(textHeavyView, 600, 400, 1)
	made := testerTextsMade()
	tt := coreNewTester(textHeavyView, 600, 400)
	if n := testerTextsMade() - made; n != 0 {
		t.Fatalf("a Tester after Render made %d text systems of its own; want it to take the one Render left", n)
	}
	if !tt.HasText("Lesson 1: shapes and colors") {
		t.Fatal("the Tester in a text system Render left doesn't show its text")
	}

	left := func() weak.Pointer[text.System] {
		tt := coreNewTester(textHeavyView, 600, 400)
		tt.Frame()
		return weak.Make(tt.h.text)
	}()
	wait.Until(t, "the text system of an unreachable Tester to be idle", func() bool {
		runtime.GC()
		return testerTextIdle(left.Value())
	})
	made = testerTextsMade()
	coreNewTester(textHeavyView, 600, 400)
	if n := testerTextsMade() - made; n != 0 {
		t.Fatalf("a Tester after an unreachable one made %d text systems of its own; want it to take the one left", n)
	}
	runtime.KeepAlive(tt)
}

// Testers that run at the same time, as parallel tests do, each have a
// text system of their own (#1722): an idle one goes to one Tester only.
func TestConcurrentTestersHaveTheirOwnTextSystems(t *testing.T) {
	// Some idle, from Testers that ended.
	for range 4 {
		coreRender(textHeavyView, 300, 200, 1)
	}
	const n = 8
	systems := make(chan *text.System, n)
	done := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			tt := coreNewTester(func(c *context) { coreText(c, fmt.Sprintf("tester %d", i)) }, 300, 200)
			systems <- tt.h.text
			// Keep it running until every Tester started.
			<-done
			tt.Frame()
			if !tt.HasText(fmt.Sprintf("tester %d", i)) {
				t.Errorf("tester %d doesn't show its text", i)
			}
		})
	}
	seen := map[*text.System]bool{}
	for range n {
		s := wait.For(t, "a Tester's text system", systems)
		if seen[s] {
			t.Error("two Testers running at once share a text system")
		}
		seen[s] = true
	}
	close(done)
	wg.Wait()
}

// A text system an ended Tester leaves starts as a new one for the next,
// which readies it (Reuse) as it takes it:
// with no language and empty atlases, whatever the Tester set and drew,
// and it draws a view as it did before.
func TestReusedTextSystemStartsAnew(t *testing.T) {
	before := coreRender(textHeavyView, 600, 400, 1)

	tt := coreNewTester(func(c *context) {
		coreColumn(c).Children(func() {
			for i := range 200 {
				coreText(c, fmt.Sprintf("%c%c 文字 %d", rune(0x4e00+i), rune(0x5e00+i), i)).FontSize(float32(10 + i%30))
			}
		})
	}, 1200, 4000)
	tt.SetScale(2)
	tt.SetLocale("ja")
	s := tt.h.text
	// Glyphs go to the mask atlas, or with ClearType (Windows) to the
	// color one.
	if s.MaskAtlas.W*s.MaskAtlas.H <= 256*256 && s.ColorAtlas.W*s.ColorAtlas.H <= 1 {
		t.Fatal("the fixture did not grow an atlas")
	}
	tt.end()
	s.Reuse() // as the Tester that takes it does
	if l := s.Language(); l != "" {
		t.Errorf("the text system left has language %q; want none", l)
	}
	if a := s.MaskAtlas; a.W != 256 || a.H != 256 {
		t.Errorf("the text system left has a %d×%d mask atlas; want a new one's 256×256", a.W, a.H)
	}
	if a := s.ColorAtlas; a.W != 1 || a.H != 1 {
		t.Errorf("the text system left has a %d×%d color atlas; want a new one's 1×1", a.W, a.H)
	}

	after := coreRender(textHeavyView, 600, 400, 1)
	if !bytes.Equal(before.Pix, after.Pix) {
		t.Error("a view draws otherwise in a text system an earlier Tester left")
	}
}
