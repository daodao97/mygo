package ui

import (
	"bytes"
	"testing"
	"time"
)

// TestTesterStartsAt starts two Testers' clocks at the same time: a
// drawing that reads the time draws the same pixels in both, byte for byte.
func TestTesterStartsAt(t *testing.T) {
	start := time.Date(2026, 1, 2, 3, 4, 5, 6_000_000, time.UTC)
	var drawn []time.Time
	view := func(c *context) {
		coreBox(c).Size(60, 12).Draw(func(p *Painter, r Rect) {
			drawn = append(drawn, p.Now())
			p.Fill(Rect{r.X + float32(p.Now().UnixMilli()%50), r.Y, 8, r.H}, RGB(0, 0, 0), 0)
		})
	}
	shot := func() []byte {
		tt := coreNewTesterWith(view, 100, 20, TesterOptions{Now: start})
		if !tt.Now().Equal(start) {
			t.Fatalf("the clock starts at %v", tt.Now())
		}
		tt.Advance(1234 * time.Millisecond)
		if !tt.Now().Equal(start.Add(1234 * time.Millisecond)) {
			t.Fatalf("after Advance: %v", tt.Now())
		}
		return tt.Image().Pix
	}
	a, b := shot(), shot()
	if !bytes.Equal(a, b) {
		t.Error("two runs from the same time draw differently")
	}
	if len(drawn) == 0 || !drawn[0].Equal(start) {
		t.Errorf("the drawing reads %v", drawn)
	}
}
