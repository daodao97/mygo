package editor

import (
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Tests of a viewer as high as its lines (FitContent), which scrolls in
// the containers around it: its find bar and go to line bring their text
// into view through them, and its bars stick to the top of the part of
// the text those containers show.

// longLines returns n lines of text, as the sides of a diff.
func longLines(n int) []string {
	a := make([]string, n)
	for i := range a {
		a[i] = "line " + strconv.Itoa(i)
	}
	return a
}

// fitDiff shows a diff of old and nw as high as its lines, its gaps
// expanded, in a scroll view, a frame or two in.
func fitDiff(t *testing.T, old, nw []string) (*DiffState, *ui.ScrollState, *ui.Tester) {
	t.Helper()
	d := &DiffState{Old: NewBuffer(strings.Join(old, "\n")), New: NewBuffer(strings.Join(nw, "\n")),
		Layout: DiffUnified, ExpandUnchanged: true, FitContent: true}
	var scroll ui.ScrollState
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Fill().TrackScroll(&scroll).Children(func() {
			DiffView(c, d).FillWidth()
		})
	}, 800, 600)
	tt.Frame()
	tt.Frame()
	return d, &scroll, tt
}

// fitEditor shows text in an editor as high as its lines, in a scroll
// view, a frame or two in.
func fitEditor(t *testing.T, text string) (*State, *ui.ScrollState, *ui.Tester) {
	t.Helper()
	st := &State{Buffer: NewBuffer(text), FitContent: true}
	var scroll ui.ScrollState
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Fill().TrackScroll(&scroll).Children(func() {
			View(c, st).Shrink(0)
		})
	}, 600, 400)
	tt.Frame()
	tt.Frame()
	return st, &scroll, tt
}

// showsLine reports whether the view shows the line at top, in the window
// h tall.
func showsLine(scroll *ui.ScrollState, top float32, h float32) bool {
	return float32(scroll.Y) <= top && float32(scroll.Y)+h >= top
}

// barInView reports whether the find bar shows, at the top of the view.
func barInView(tt *ui.Tester) bool {
	r, ok := tt.Find("Match case")
	return ok && r.W > 0 && r.H > 0 && r.Y >= 0 && r.Y <= 40
}

// goToBarInView reports whether the go to line bar shows, at the top of
// the view.
func goToBarInView(tt *ui.Tester) bool {
	r, ok := tt.Find("Go to line")
	return ok && r.W > 0 && r.H > 0 && r.Y >= 0 && r.Y <= 40
}

// A diff's find bar brings the match it selects into view: the viewer has
// no scroll of its own, so the containers around it scroll to the match.
func TestFitContentDiffFindScrollsToMatch(t *testing.T) {
	a := longLines(600)
	b := append([]string(nil), a...)
	b[500] = "needle in a haystack"
	d, scroll, tt := fitDiff(t, a, b)
	if scroll.Y != 0 {
		t.Fatalf("the view scrolled to %v before the search", scroll.Y)
	}
	d.Find("needle")
	tt.Frame()
	tt.Frame()
	ed := &d.eds[1]
	if len(ed.find.matches) != 1 {
		t.Fatalf("the search matched %v, want once", ed.find.matches)
	}
	if top := float32(ed.view.heights.top(500)); !showsLine(scroll, top, 600) {
		t.Fatalf("the view scrolled to %v, which doesn't show the match at %v", scroll.Y, top)
	}
}

// The find bar follows the matches the user asks for, and leaves the view
// where the user put it after.
func TestFitContentDiffFindFollowsTheMatches(t *testing.T) {
	a := longLines(600)
	b := append([]string(nil), a...)
	b[100] = "needle in a haystack"
	b[500] = "needle in a haystack"
	d := &DiffState{Old: NewBuffer(strings.Join(a, "\n")), New: NewBuffer(strings.Join(b, "\n")),
		Layout: DiffUnified, ExpandUnchanged: true, FitContent: true}
	var scroll ui.ScrollState
	ed := &d.eds[1]
	// ask is the user's Enter in the find bar, in the frame they press it.
	ask := false
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Fill().TrackScroll(&scroll).Children(func() {
			DiffView(c, d).FillWidth()
			if ask {
				ask = false
				ed.find.step(ed, 1)
			}
		})
	}, 800, 600)
	tt.Frame()
	tt.Frame()
	top := func(line int) float32 { return float32(ed.view.heights.top(line)) }
	d.Find("needle")
	tt.Frame()
	tt.Frame()
	if !showsLine(&scroll, top(100), 600) {
		t.Fatalf("the view scrolled to %v, which doesn't show the first match at %v", scroll.Y, top(100))
	}
	// The next match is further down: the view follows it.
	ask = true
	tt.Frame()
	tt.Frame()
	if !showsLine(&scroll, top(500), 600) {
		t.Fatalf("the view scrolled to %v, which doesn't show the second match at %v", scroll.Y, top(500))
	}
	// The view stays where the match is: it does not scroll on its own.
	at := scroll.Y
	tt.Frame()
	tt.Frame()
	if scroll.Y != at {
		t.Fatalf("the view scrolled from %v to %v on its own", at, scroll.Y)
	}
	// The user scrolls away from the match, and asks for the next one:
	// the view follows it again.
	scroll.Y = 0
	tt.Frame()
	tt.Frame()
	ask = true
	tt.Frame()
	tt.Frame()
	if !showsLine(&scroll, top(100), 600) {
		t.Fatalf("the view scrolled to %v, which doesn't show the match it went to at %v", scroll.Y, top(100))
	}
}

// An editor as high as its lines scrolls to the match its find bar
// selects, as the diff's does.
func TestFitContentEditorFindScrollsToMatch(t *testing.T) {
	st, scroll, tt := fitEditor(t, strings.Repeat("line\n", 300)+"needle\n")
	if scroll.Y != 0 {
		t.Fatalf("the view scrolled to %v before the search", scroll.Y)
	}
	st.Find("needle")
	tt.Frame()
	tt.Frame()
	if top := float32(st.LineTop(300)); !showsLine(scroll, top, 400) {
		t.Fatalf("the view scrolled to %v, which doesn't show the match at %v", scroll.Y, top)
	}
}

// A find bar opened anew brings its match into view again, even one the
// user scrolled away from since: Esc closes it, Cmd+F opens it.
func TestFitContentFindBarOpensOnItsMatchAgain(t *testing.T) {
	st, scroll, tt := fitEditor(t, strings.Repeat("line\n", 300)+"needle\n")
	st.Find("needle")
	tt.Frame()
	tt.Frame()
	if scroll.Y == 0 {
		t.Fatal("the find bar did not bring its match into view")
	}
	// The user scrolls back to the start, and closes the bar as Escape
	// does.
	scroll.Y = 0
	st.ed.find.open = false
	tt.Frame()
	tt.Frame()
	if scroll.Y != 0 {
		t.Fatalf("the view scrolled to %v after the bar closed", scroll.Y)
	}
	// Opened again, as Cmd+F does: the match comes into view again.
	st.ed.find.show(&st.ed, false)
	tt.Frame()
	tt.Frame()
	if scroll.Y == 0 {
		t.Fatal("the bar opened without bringing its match into view again")
	}
}

// An editor as high as its lines brings no text into view by itself: the
// view stays where the user put it.
func TestFitContentEditorStaysWhereTheUserPutIt(t *testing.T) {
	st := &State{Buffer: NewBuffer(strings.Repeat("line\n", 300)), FitContent: true}
	var scroll ui.ScrollState
	show := false
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Fill().TrackScroll(&scroll).Children(func() {
			ui.Box(c).Height(2000).Shrink(0)
			if show {
				View(c, st).Shrink(0)
			}
		})
	}, 600, 400)
	tt.Frame()
	// The user scrolls the view, and the editor shows in it after that.
	scroll.Y = 1000
	tt.Frame()
	show = true
	tt.Frame()
	tt.Frame()
	if scroll.Y != 1000 {
		t.Fatalf("the view scrolled to %v on its own", scroll.Y)
	}
}

// The find bar of a viewer as high as its lines stays at the top of the
// part of the text the containers around it show, rather than at the top
// of the whole text, which the scroll takes out of view.
func TestFitContentFindBarStaysInView(t *testing.T) {
	st, scroll, tt := fitEditor(t, strings.Repeat("line\n", 300))
	st.Find("line 200")
	tt.Frame()
	tt.Frame()
	if !barInView(tt) {
		r, _ := tt.Find("Match case")
		t.Fatalf("the find bar is at %v, not at the top of the view", r)
	}
	// Scrolled down, the text's top is out of view: the bar stays in it.
	scroll.Y = 1000
	tt.Frame()
	tt.Frame()
	if !barInView(tt) {
		r, _ := tt.Find("Match case")
		t.Fatalf("the find bar scrolled to %v, out of the view", r)
	}
	// Scrolled back to the start, it is where the text's top is.
	scroll.Y = 0
	tt.Frame()
	tt.Frame()
	if !barInView(tt) {
		r, _ := tt.Find("Match case")
		t.Fatalf("the find bar at %v back at the top", r)
	}
}

// The sticky find bar fits the editor it shows in, however narrow.
func TestFitContentFindBarFitsNarrowEditor(t *testing.T) {
	for _, w := range []int{320, 420, 700} {
		st := &State{Buffer: NewBuffer(strings.Repeat("line\n", 300)), FitContent: true}
		var scroll ui.ScrollState
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Scroll(c).Fill().TrackScroll(&scroll).Children(func() {
				View(c, st).Shrink(0)
			})
		}, w, 400)
		tt.Frame()
		st.Find("cat")
		tt.Frame()
		tt.Frame()
		r, ok := tt.Find("Find")
		if !ok || r.W == 0 {
			t.Fatalf("width %d: the find bar is not built", w)
		}
		if r.X < 0 || r.X+r.W > float32(w) {
			t.Fatalf("width %d: the find bar spans %v to %v, outside the editor", w, r.X, r.X+r.W)
		}
	}
}

// A diff as high as its lines keeps its find bar in view as the search
// takes the view to a match further down, and takes its clicks there.
func TestFitContentDiffFindKeepsTheBarInView(t *testing.T) {
	a := longLines(600)
	b := append([]string(nil), a...)
	b[500] = "needle in a haystack"
	d, scroll, tt := fitDiff(t, a, b)
	d.Find("needle")
	tt.Frame()
	tt.Frame()
	ed := &d.eds[1]
	if top := float32(ed.view.heights.top(500)); !showsLine(scroll, top, 600) {
		t.Fatalf("the view scrolled to %v, which doesn't show the match at %v", scroll.Y, top)
	}
	if !barInView(tt) {
		r, _ := tt.Find("Match case")
		t.Fatalf("the find bar is at %v after the search, not at the top of the view", r)
	}
	// The bar takes its clicks where it shows.
	if err := tt.Click("Match case"); err != nil {
		t.Fatal(err)
	}
	if !ed.find.caseSens {
		t.Fatal("a click on Match case did not reach the bar")
	}
}

// The go to line bar of a viewer as high as its lines stays at the top of
// the part of the text the containers around it show, as the find bar
// does.
func TestFitContentGoToLineStaysInView(t *testing.T) {
	st, scroll, tt := fitEditor(t, strings.Repeat("line\n", 300))
	st.Run(GoToLine)
	tt.Frame()
	tt.Frame()
	if !goToBarInView(tt) {
		r, _ := tt.Find("Go to line")
		t.Fatalf("the go to line bar is at %v, not at the top of the view", r)
	}
	// Scrolled down, the text's top is out of view: the bar stays in it.
	scroll.Y = 1000
	tt.Frame()
	tt.Frame()
	if !goToBarInView(tt) {
		r, _ := tt.Find("Go to line")
		t.Fatalf("the go to line bar scrolled to %v, out of the view", r)
	}
	// Scrolled back to the start, it is where the text's top is.
	scroll.Y = 0
	tt.Frame()
	tt.Frame()
	if !goToBarInView(tt) {
		r, _ := tt.Find("Go to line")
		t.Fatalf("the go to line bar at %v back at the top", r)
	}
}

// Go to line takes a viewer as high as its lines to the line: the
// containers around it scroll to the line, which the viewer cannot.
func TestFitContentGoToLineScrollsToTheLine(t *testing.T) {
	st, scroll, tt := fitEditor(t, strings.Repeat("line\n", 300))
	st.Run(GoToLine)
	tt.Frame()
	st.ed.goTo.text = "250"
	(&GoToLineBar{ed: &st.ed}).Go()
	tt.Frame()
	tt.Frame()
	if top := float32(st.LineTop(249)); !showsLine(scroll, top, 400) {
		t.Fatalf("the view scrolled to %v, which doesn't show line 250 at %v", scroll.Y, top)
	}
}

// State.GoTo takes a viewer as high as its lines to the line too.
func TestFitContentGoToScrollsToTheLine(t *testing.T) {
	st, scroll, tt := fitEditor(t, strings.Repeat("line\n", 300))
	st.GoTo(Pos{Line: 249})
	tt.Frame()
	tt.Frame()
	if top := float32(st.LineTop(249)); !showsLine(scroll, top, 400) {
		t.Fatalf("the view scrolled to %v, which doesn't show line 250 at %v", scroll.Y, top)
	}
}

// The find bar of a diff sticks within its own diff as the view shows the
// diffs after it: a page of diffs, as a commit's is, shows the bar of the
// diff whose text is in view, not over the next one.
func TestFitContentFindBarLeavesWithItsDiff(t *testing.T) {
	a := strings.Repeat("line\n", 150)
	b := a + "changed\n"
	d1 := &DiffState{Old: NewBuffer(a), New: NewBuffer(b), FitContent: true, ExpandUnchanged: true}
	d2 := &DiffState{Old: NewBuffer(a), New: NewBuffer(b), FitContent: true, ExpandUnchanged: true}
	var scroll ui.ScrollState
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Fill().TrackScroll(&scroll).Children(func() {
			DiffView(c, d1).FillWidth()
			DiffView(c, d2).FillWidth()
		})
	}, 800, 400)
	tt.Frame()
	tt.Frame()
	d1.Find("line")
	tt.Frame()
	tt.Frame()
	if !barInView(tt) {
		r, _ := tt.Find("Match case")
		t.Fatalf("the first diff's bar is at %v, not at the top of the view", r)
	}
	// The second diff's text is what shows: the first one's bar goes with
	// the first diff, out of view.
	scroll.Y = 3200
	tt.Frame()
	tt.Frame()
	if r, _ := tt.Find("Match case"); r.W != 0 || r.H != 0 {
		t.Fatalf("the first diff's bar is at %v over the second diff", r)
	}
}
