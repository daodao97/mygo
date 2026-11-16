package ui

import (
	"slices"
	"testing"
	"time"
)

// moveList is a list of rows 40 high, of the items ids, in a list 400
// high, with a MoveAnimation of a 200 ms curve, and a clock the test
// moves.
type moveList struct {
	t   *testing.T
	ids []int
	s   ListState
	tt  *Tester
	now time.Time
}

func newMoveList(t *testing.T, n int, opts ...func(*moveList)) *moveList {
	m := &moveList{t: t, now: time.Unix(1000, 0)}
	for i := range n {
		m.ids = append(m.ids, i)
	}
	m.s = ListState{Key: func(i int) any { return m.ids[i] }, MoveAnimation: &Animation{Duration: 200 * time.Millisecond}}
	for _, o := range opts {
		o(m)
	}
	m.tt = coreNewTester(func(c *context) {
		coreList(c, &m.s, len(m.ids), func(i int) {
			coreBox(c).Height(40).Children(func() { coreTextf(c, "Item %d", m.ids[i]) })
		}).Grow(1)
	}, 300, 400)
	m.tt.rt.clock = func() time.Time { return m.now }
	m.tt.Frame()
	return m
}

// at returns where the row of item id shows, from the list's top.
func (m *moveList) at(id int) float32 {
	m.t.Helper()
	i := slices.Index(m.ids, id)
	r, ok := rowBox(m.tt, &m.s, i)
	if !ok {
		m.t.Fatalf("item %d is not shown", id)
	}
	return r.Y
}

// step moves the clock on by d and draws a frame.
func (m *moveList) step(d time.Duration) {
	m.now = m.now.Add(d)
	m.tt.Frame()
}

// covered reports whether the rows shown reach the list's top and its
// bottom.
func (m *moveList) covered() (top, bottom bool) {
	lo, hi := float32(400), float32(0)
	for i := range m.ids {
		if r, ok := rowBox(m.tt, &m.s, i); ok {
			lo, hi = min(lo, r.Y), max(hi, r.Y+r.H)
		}
	}
	return lo <= 0, hi >= 400
}

// TestListMoveAnimation checks that the rows of a list with a
// MoveAnimation glide from where they showed as rows above them go or
// come, close the gap of rows that went and open that of rows that came,
// and that scrolling and a desktop asking for less motion move them at
// once.
func TestListMoveAnimation(t *testing.T) {
	m := newMoveList(t, 50)
	// The first three rows go: the rows below show where they were, and
	// come up over the gap.
	m.ids = m.ids[3:]
	m.tt.Frame()
	if y := m.at(3); y != 120 {
		t.Fatalf("the rows went: item 3 shows at %v, not where it was", y)
	}
	m.step(50 * time.Millisecond)
	if y := m.at(3); !(y > 0 && y < 120) {
		t.Errorf("on the way, item 3 shows at %v", y)
	}
	// The gap closes at the top, where the rows went; the bottom stays
	// covered.
	if _, bottom := m.covered(); !bottom {
		t.Error("on the way, the rows leave a gap at the bottom")
	}
	m.step(time.Second)
	if y := m.at(3); y != 0 {
		t.Errorf("done, item 3 shows at %v", y)
	}

	// Two rows come below item 5: the rows below them show where they
	// were and go down; the rows above stay.
	k := slices.Index(m.ids, 5) + 1
	m.ids = slices.Insert(m.ids, k, 100, 101)
	m.tt.Frame()
	if a, b := m.at(5), m.at(6); a != 80 || b != 120 {
		t.Fatalf("rows came: item 5 at %v, item 6 at %v", a, b)
	}
	m.step(time.Second)
	if a, b, c := m.at(5), m.at(100), m.at(6); a != 80 || b != 120 || c != 200 {
		t.Errorf("done: item 5 at %v, item 100 at %v, item 6 at %v", a, b, c)
	}

	// Rows come above the top, and the app scrolls to them: the rows
	// shown slide down, the new ones coming in from above.
	m.ids = append([]int{200, 201, 202}, m.ids...)
	m.s.ScrollTo(0, Start)
	m.tt.Frame()
	if a, b := m.at(3), m.at(200); a != 0 || b != -120 {
		t.Fatalf("rows came above: item 3 at %v, item 200 at %v", a, b)
	}
	if top, bottom := m.covered(); !top || !bottom {
		t.Errorf("rows came above: the rows reach the top %v, the bottom %v", top, bottom)
	}
	m.step(time.Second)
	if a, b := m.at(3), m.at(200); a != 120 || b != 0 {
		t.Errorf("done: item 3 at %v, item 200 at %v", a, b)
	}

	// Scrolling moves the rows at once.
	m.tt.Move(100, 100)
	m.tt.Scroll(100, 100, 0, 80)
	m.step(16 * time.Millisecond)
	if y := m.at(3); y != 40 {
		t.Errorf("scrolled: item 3 at %v", y)
	}

	// Without motion, rows go to their places at once.
	m.tt.SetPreferences(Preferences{ReduceMotion: true})
	m.s.ScrollTo(0, Start)
	m.tt.Frame()
	m.ids = m.ids[2:]
	m.tt.Frame()
	if y := m.at(202); y != 0 {
		t.Errorf("reduced motion: item 202 at %v", y)
	}
}

// A list with a MoveAnimation built again with the same rows leaves them
// in place, and asks for no frames.
func TestListMoveAnimationKeepsRowsInPlace(t *testing.T) {
	m := newMoveList(t, 30)
	m.tt.Frame()
	for _, id := range []int{0, 5, 9} {
		if y, want := m.at(id), float32(40*id); y != want {
			t.Errorf("item %d at %v, want %v", id, y, want)
		}
	}
	if m.tt.FramePending() {
		t.Error("rows in place still ask for frames")
	}
}

// A list showing its end keeps it in place as a row in view goes: the
// rows above it glide down over its gap, those below stay.
func TestListMoveAnimationAtTheEnd(t *testing.T) {
	m := newMoveList(t, 50)
	m.s.ScrollToEnd()
	m.tt.Frame()
	if a, b := m.at(44), m.at(46); a != 160 || b != 240 {
		t.Fatalf("at the end: item 44 at %v, item 46 at %v", a, b)
	}
	m.ids = slices.DeleteFunc(m.ids, func(id int) bool { return id == 45 })
	m.tt.Frame()
	if a, b := m.at(44), m.at(46); a != 160 || b != 240 {
		t.Fatalf("a row went: item 44 at %v, item 46 at %v", a, b)
	}
	m.step(50 * time.Millisecond)
	if a := m.at(44); !(a > 160 && a < 200) {
		t.Errorf("on the way, item 44 at %v", a)
	}
	m.step(time.Second)
	if a, b := m.at(44), m.at(46); a != 200 || b != 240 {
		t.Errorf("done: item 44 at %v, item 46 at %v", a, b)
	}
}

// Rows going in the frame the user scrolls glide from where the scroll
// alone would show them: the rows the change leaves in place scroll as
// the others.
func TestListMoveAnimationWhileScrolling(t *testing.T) {
	m := newMoveList(t, 50)
	m.ids = slices.DeleteFunc(m.ids, func(id int) bool { return id == 2 })
	m.tt.Move(100, 100)
	m.tt.Scroll(100, 100, 0, 40)
	m.tt.Frame()
	// Item 1 stays, scrolled up by 40; item 3, which closes the gap of
	// item 2, shows where the scroll alone would have it.
	if a, b := m.at(1), m.at(3); a != 0 || b != 80 {
		t.Fatalf("scrolled as a row went: item 1 at %v, item 3 at %v", a, b)
	}
	m.step(time.Second)
	if a, b := m.at(1), m.at(3); a != 0 || b != 40 {
		t.Errorf("done: item 1 at %v, item 3 at %v", a, b)
	}
}

// Moving through the rows without rows coming or going scrolls at once,
// as without a MoveAnimation.
func TestListMoveAnimationRevealsAtOnce(t *testing.T) {
	m := newMoveList(t, 50)
	m.s.ScrollTo(5, Start)
	m.tt.Frame()
	if y := m.at(5); y != 0 {
		t.Errorf("scrolled to item 5: it shows at %v", y)
	}
	if m.tt.FramePending() {
		t.Error("scrolled to item 5: the list still asks for frames")
	}
}

// The header pinned at the top of a list with a MoveAnimation stays
// there as the rows under it glide.
func TestListMoveAnimationKeepsTheHeaderPinned(t *testing.T) {
	m := newMoveList(t, 60, func(m *moveList) {
		m.s.Header = func(i int) bool { return m.ids[i]%10 == 0 }
		m.s.KeepOffset = true
	})
	m.s.ScrollTo(11, Start)
	m.tt.Frame()
	head := func() float32 {
		t.Helper()
		r, ok := rowBox(m.tt, &m.s, slices.Index(m.ids, 10))
		if !ok {
			t.Fatal("the header of item 10 is not shown")
		}
		return r.Y
	}
	if y := head(); y != 0 {
		t.Fatalf("scrolled: the header of item 10 at %v", y)
	}
	// Rows above go: the list keeps its offset, and the rows move up.
	m.ids = slices.DeleteFunc(m.ids, func(id int) bool { return id >= 1 && id <= 3 })
	m.tt.Frame()
	if m.at(12) == 40 {
		t.Fatal("the rows did not glide")
	}
	if y := head(); y != 0 {
		t.Errorf("rows gliding: the header of item 10 at %v", y)
	}
	m.step(time.Second)
	if y := head(); y != 0 {
		t.Errorf("done: the header of item 10 at %v", y)
	}
}
