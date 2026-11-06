//go:build ios || mygo_touch_elastic

package ui

import (
	"github.com/egoist/mygo/internal/platform"
	"math"
	"testing"
	"time"
)

func elasticFixture() (*Tester, *ScrollState) {
	scroll := new(ScrollState)
	tt := snapshotTester(func(c *Context) {
		Column(c).Fill().Children(func() {
			Text(c, "Fixed header").Height(40).Shrink(0)
			Scroll(c).Fill().TrackScroll(scroll).Children(func() {
				Text(c, "First row").Height(40).Shrink(0)
				Box(c).Height(1000).Shrink(0)
			})
			Text(c, "Fixed composer").Height(40).Shrink(0)
		})
	}, 320, 320)
	return tt, scroll
}

func TestTouchElasticMovesOnlyContentAndSettles(t *testing.T) {
	tt, scroll := elasticFixture()
	first, _ := tt.Find("First row")
	header, _ := tt.Find("Fixed header")
	composer, _ := tt.Find("Fixed composer")
	touch(tt, platform.PointerDown, 1, 150, 100)
	touch(tt, platform.PointerMove, 1, 150, 180)
	moved, _ := tt.Find("First row")
	h, _ := tt.Find("Fixed header")
	c, _ := tt.Find("Fixed composer")
	if scroll.Y != 0 || moved.Y <= first.Y || moved.Y >= first.Y+80 || header != h || composer != c {
		t.Fatalf("elastic drag: offset=%v first=%v moved=%v header=%v composer=%v", scroll.Y, first, moved, h, c)
	}
	touch(tt, platform.PointerUp, 1, 150, 180)
	tt.Advance(100 * time.Millisecond)
	returning, _ := tt.Find("First row")
	if returning.Y >= moved.Y || returning.Y <= first.Y {
		t.Fatalf("spring did not return smoothly: %v", returning)
	}
	tt.Advance(time.Second)
	settled, _ := tt.Find("First row")
	if settled != first || tt.rt.elastic != nil || tt.FramePending() {
		t.Fatalf("spring did not settle: %v elastic=%+v", settled, tt.rt.elastic)
	}
}

func TestTouchElasticReversalAndProgrammaticMove(t *testing.T) {
	tt, scroll := elasticFixture()
	touch(tt, platform.PointerDown, 1, 150, 100)
	touch(tt, platform.PointerMove, 1, 150, 160)
	touch(tt, platform.PointerMove, 1, 150, 80)
	if scroll.Y != 20 || tt.rt.elastic.y != 0 {
		t.Fatalf("reversal jumped: offset=%v elastic=%+v", scroll.Y, tt.rt.elastic)
	}
	touch(tt, platform.PointerMove, 1, 150, 180)
	touch(tt, platform.PointerUp, 1, 150, 180)
	scroll.Y = 100
	tt.Frame()
	tt.Advance(100 * time.Millisecond)
	if e := tt.rt.elastic; e != nil && e.y != 0 {
		t.Fatalf("programmatic move resurrected bounce: %+v", e)
	}
}

func TestTouchElasticMomentumStopsAtTableEdge(t *testing.T) {
	var outer, inner ScrollState
	tt := snapshotTester(func(c *Context) {
		Scroll(c).Fill().TrackScroll(&outer).Children(func() {
			ScrollHorizontal(c).Height(100).Shrink(0).TrackScroll(&inner).Children(func() { Text(c, "Table").Width(900).Height(100) })
			Box(c).Height(1000).Shrink(0)
		})
	}, 320, 240)
	inner.X = inner.MaxX
	tt.Frame()
	ev := platform.SurfaceEvent{Kind: platform.PointerScroll, Pointer: platform.PointerTouch, Precise: true, Momentum: true, X: 160, Y: 50, DX: 50}
	if tt.rt.event(ev) {
		t.Fatal("native momentum was not stopped at the edge")
	}
	tt.Frame()
	if tt.rt.elastic == nil || tt.rt.elastic.x <= 0 || inner.X != inner.MaxX || outer.Y != 0 {
		t.Fatalf("edge: inner=%+v outer=%+v elastic=%+v", inner, outer, tt.rt.elastic)
	}
	initial := tt.rt.elastic.x
	tt.Advance(100 * time.Millisecond)
	if tt.rt.event(ev) || tt.rt.elastic.x >= initial {
		t.Fatal("late momentum pumped the spring")
	}
	tt.Advance(time.Second)
	if tt.rt.elastic != nil || outer.Y != 0 {
		t.Fatal("table bounce retained displacement or moved its parent")
	}
}

func TestNativePanElasticReleaseAndReduceMotion(t *testing.T) {
	tt, scroll := elasticFixture()
	ev := platform.SurfaceEvent{Kind: platform.PointerScroll, Pointer: platform.PointerTouch, Precise: true, X: 150, Y: 100, DY: -60, Phase: "begin"}
	tt.send(ev)
	if tt.rt.elastic == nil || tt.rt.elastic.released {
		t.Fatal("native drag did not hold displacement")
	}
	ev.DY = 0
	ev.Phase = "cancel"
	tt.send(ev)
	tt.Advance(time.Second)
	if tt.rt.elastic != nil {
		t.Fatal("cancelled native pan did not settle")
	}
	tt.SetPreferences(Preferences{ReduceMotion: true})
	ev.DY = -60
	ev.Phase = "begin"
	tt.send(ev)
	if tt.rt.elastic != nil || scroll.Y != 0 {
		t.Fatal("reduce motion bounced at the edge")
	}
	ev.DY = 60
	ev.Phase = ""
	tt.send(ev)
	if scroll.Y != 60 {
		t.Fatal("reduce motion prevented ordinary scrolling")
	}
}

func TestElasticRubberBandAndClockIndependence(t *testing.T) {
	if a, b := rubberBand(100, 300), rubberBand(200, 300); a <= 0 || b-a >= a || b >= 200 || rubberBand(-100, 300) != -a {
		t.Fatalf("rubber band lost resistance: %v %v", a, b)
	}
	var positions [2]float64
	for i, step := range []time.Duration{time.Second / 60, time.Second / 120} {
		tt, _ := elasticFixture()
		touch(tt, platform.PointerDown, 1, 150, 100)
		touch(tt, platform.PointerMove, 1, 150, 180)
		touch(tt, platform.PointerUp, 1, 150, 180)
		elapsed := time.Duration(0)
		for elapsed+step < 200*time.Millisecond {
			tt.Advance(step)
			elapsed += step
		}
		tt.Advance(200*time.Millisecond - elapsed)
		positions[i] = tt.rt.elastic.y
	}
	if math.Abs(positions[0]-positions[1]) > 1e-6 {
		t.Fatalf("refresh-dependent spring: %v", positions)
	}
}

func TestTouchElasticVirtualListCanBeInterrupted(t *testing.T) {
	var list ListState
	var scroll ScrollState
	tt := snapshotTester(func(c *Context) {
		List(c, &list, 10000, func(i int) { Text(c, "Row").Height(40) }).Fill().TrackScroll(&scroll)
	}, 320, 320)
	touch(tt, platform.PointerDown, 1, 150, 100)
	touch(tt, platform.PointerMove, 1, 150, 180)
	first, last := list.Visible()
	row, _ := rowBox(tt, &list, 0)
	if first != 0 || last > 30 || row.Y <= 0 || scroll.Y != 0 {
		t.Fatalf("virtual list moved logical rows: %d..%d row=%v offset=%v", first, last, row, scroll.Y)
	}
	touch(tt, platform.PointerUp, 1, 150, 180)
	tt.Advance(50 * time.Millisecond)
	row, _ = rowBox(tt, &list, 0)
	touch(tt, platform.PointerDown, 2, 150, 100)
	tt.Advance(100 * time.Millisecond)
	held, _ := rowBox(tt, &list, 0)
	if held != row {
		t.Fatal("touching the returning list did not stop its spring")
	}
	touch(tt, platform.PointerMove, 2, 150, 80)
	reversed, _ := rowBox(tt, &list, 0)
	if reversed.Y >= held.Y {
		t.Fatal("new drag did not take over smoothly")
	}
	touch(tt, platform.PointerUp, 2, 150, 80)
	tt.Advance(time.Second)
	row, _ = rowBox(tt, &list, 0)
	if row.Y != 0 || scroll.Y != 0 || tt.rt.elastic != nil {
		t.Fatalf("virtual list failed to settle: row=%v scroll=%v", row, scroll.Y)
	}
}

func TestTouchElasticRightToLeft(t *testing.T) {
	var scroll ScrollState
	tt := snapshotTester(func(c *Context) {
		ScrollHorizontal(c).Fill().TrackScroll(&scroll).Children(func() { Text(c, "Wide row").Width(900).Height(100) })
	}, 320, 200)
	tt.SetDirection(RightToLeft)
	before, _ := tt.Find("Wide row")
	touch(tt, platform.PointerDown, 1, 200, 80)
	touch(tt, platform.PointerMove, 1, 120, 80)
	moved, _ := tt.Find("Wide row")
	if scroll.X != 0 || moved.X+moved.W >= before.X+before.W || moved.X+moved.W <= before.X+before.W-80 {
		t.Fatalf("RTL edge moved backwards: before=%v after=%v offset=%v", before, moved, scroll.X)
	}
	touch(tt, platform.PointerUp, 1, 120, 80)
	tt.Advance(time.Second)
	after, _ := tt.Find("Wide row")
	if after != before || tt.rt.elastic != nil {
		t.Fatal("RTL spring did not return")
	}
}
