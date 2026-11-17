package mobile

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func advance(tt *ui.Tester, d time.Duration) {
	for range int(d / (16 * time.Millisecond)) {
		tt.Advance(16 * time.Millisecond)
	}
}

// A sheet comes up to its detent, a finger drags it among them, and down and
// away closes it.
func TestSheet(t *testing.T) {
	open := true
	var top float32
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill()
		Sheet(c, &open, SheetOptions{Detents: []float32{0.5, 1}}, func() {
			top = ui.Box(c).Fill().Bounds().Y
		})
	}, 400, 800)
	tt.Frame()
	advance(tt, 2*time.Second)
	if top < 390 || top > 420 {
		t.Fatalf("opened: its content shows at %v", top)
	}
	// Up to the tall detent.
	tt.FingerDown(200, 430)
	advance(tt, 16*time.Millisecond)
	for y := float32(420); y >= 40; y -= 20 {
		tt.FingerMove(200, y)
		tt.Frame()
		advance(tt, 16*time.Millisecond)
	}
	advance(tt, 200*time.Millisecond)
	tt.FingerUp(200, 40)
	advance(tt, 2*time.Second)
	if top > 40 {
		t.Errorf("dragged up: its content shows at %v", top)
	}
	// Down and away.
	tt.FingerDown(200, 60)
	advance(tt, 16*time.Millisecond)
	for y := float32(80); y <= 790; y += 30 {
		tt.FingerMove(200, y)
		tt.Frame()
		advance(tt, 16*time.Millisecond)
	}
	advance(tt, 200*time.Millisecond)
	tt.FingerUp(200, 790)
	advance(tt, 2*time.Second)
	if open {
		t.Error("dragged away, it stays open")
	}
}

// A tap outside a sheet closes it.
func TestSheetDismiss(t *testing.T) {
	open := true
	tt := ui.NewTester(func(c *ui.Context) {
		Sheet(c, &open, SheetOptions{}, func() {})
	}, 400, 800)
	tt.Frame()
	advance(tt, 2*time.Second)
	tt.FingerDown(200, 100)
	tt.FingerUp(200, 100)
	tt.Frame()
	if open {
		t.Error("a tap outside left it open")
	}
}

// A tap on what a sheet holds is that of what it holds.
func TestSheetContent(t *testing.T) {
	open, clicks := true, 0
	tt := ui.NewTester(func(c *ui.Context) {
		Sheet(c, &open, SheetOptions{}, func() {
			if ui.Box(c).Fill().Clicked() {
				clicks++
			}
		})
	}, 400, 800)
	tt.Frame()
	advance(tt, 2*time.Second)
	tt.FingerDown(200, 600)
	tt.FingerUp(200, 600)
	tt.Frame()
	if clicks != 1 || !open {
		t.Errorf("%d clicks, open %v", clicks, open)
	}
}

// A throw goes to the next page; a short move stays.
func TestPageView(t *testing.T) {
	var s PageState
	built := map[int]bool{}
	tt := ui.NewTester(func(c *ui.Context) {
		PageView(c, &s, 5, func(i int) { built[i] = true; ui.Box(c).Fill() }).Fill()
	}, 400, 400)
	tt.Frame()
	tt.Frame()
	if built[3] {
		t.Errorf("built far pages: %v", built)
	}
	swipe := func(from, to float32) {
		tt.FingerDown(from, 200)
		advance(tt, 16*time.Millisecond)
		n := 6
		for i := 1; i <= n; i++ {
			tt.FingerMove(from+(to-from)*float32(i)/float32(n), 200)
			tt.Frame()
			tt.Advance(16 * time.Millisecond)
		}
		tt.FingerUp(to, 200)
		advance(tt, 2*time.Second)
	}
	swipe(350, 100)
	if s.Page != 1 {
		t.Fatalf("a swipe left: page %d", s.Page)
	}
	swipe(100, 350)
	if s.Page != 0 {
		t.Errorf("a swipe right: page %d", s.Page)
	}
	s.Page = 3
	tt.Frame()
	advance(tt, 2*time.Second)
	if s.Page != 3 {
		t.Errorf("set to 3: page %d", s.Page)
	}
}

// A wheel lands on an item.
func TestWheel(t *testing.T) {
	v, changes := 0, 0
	tt := ui.NewTester(func(c *ui.Context) {
		if Wheel(c, &v, []string{"a", "b", "c", "d", "e", "f"}).Changed() {
			changes++
		}
	}, 200, 180)
	tt.Frame()
	tt.Frame()
	tt.FingerDown(100, 90)
	advance(tt, 16*time.Millisecond)
	for y := float32(80); y >= 10; y -= 10 {
		tt.FingerMove(100, y)
		tt.Frame()
		advance(tt, 16*time.Millisecond)
	}
	advance(tt, 200*time.Millisecond)
	tt.FingerUp(100, 10)
	advance(tt, 2*time.Second)
	if v != 2 || changes != 1 {
		t.Errorf("spun 80: item %d, %d changes", v, changes)
	}
}

// Swiped, a row shows its actions, whose tap runs and closes it.
func TestSwipeRow(t *testing.T) {
	var s SwipeState
	deleted := false
	tt := ui.NewTester(func(c *ui.Context) {
		SwipeRow(c, &s, SwipeOptions{Trailing: []SwipeAction{{Label: "Delete", Destructive: true, OnTap: func() { deleted = true }}}},
			func() { ui.Box(c).Height(60).Shrink(0).FillWidth() })
	}, 300, 200)
	tt.Frame()
	tt.FingerDown(280, 30)
	advance(tt, 16*time.Millisecond)
	for x := float32(270); x >= 170; x -= 20 {
		tt.FingerMove(x, 30)
		tt.Frame()
		tt.Advance(16 * time.Millisecond)
	}
	advance(tt, 200*time.Millisecond)
	tt.FingerUp(170, 30)
	advance(tt, 2*time.Second)
	if !s.open {
		t.Fatalf("swiped: offset %+v", s.offset)
	}
	tt.FingerDown(260, 30)
	tt.FingerUp(260, 30)
	tt.Frame()
	if !deleted {
		t.Error("tapping the action did nothing")
	}
}

// A sheet stays at its detent as the window turns, whose height is another.
func TestSheetTurns(t *testing.T) {
	open := true
	var top float32
	tt := ui.NewTester(func(c *ui.Context) {
		Sheet(c, &open, SheetOptions{Detents: []float32{0.5, 1}}, func() {
			top = ui.Box(c).Fill().Bounds().Y
		})
	}, 400, 800)
	tt.Frame()
	advance(tt, 2*time.Second)
	tt.SetSize(800, 300)
	tt.Frame()
	advance(tt, 2*time.Second)
	if !open {
		t.Fatal("turned: it closed")
	}
	if top < 130 || top > 170 {
		t.Errorf("turned: its content shows at %v of 300", top)
	}
}

// Opened again as it closes, it opens.
func TestSheetReopens(t *testing.T) {
	open := true
	var top float32
	tt := ui.NewTester(func(c *ui.Context) {
		Sheet(c, &open, SheetOptions{Detents: []float32{0.5}}, func() {
			top = ui.Box(c).Fill().Bounds().Y
		})
	}, 400, 800)
	tt.Frame()
	advance(tt, 2*time.Second)
	open = false
	tt.Frame()
	advance(tt, 50*time.Millisecond)
	open = true
	tt.Frame()
	advance(tt, 2*time.Second)
	if !open || top > 420 {
		t.Errorf("reopened: open %v, its content shows at %v", open, top)
	}
}
