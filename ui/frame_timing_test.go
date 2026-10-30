package ui

import "testing"

// TestFrameTimingStartsWithFirstReading checks that frames are timed only
// once an app reads the frame times: until then a frame takes no lock and
// reads no clock for them.
func TestFrameTimingStartsWithFirstReading(t *testing.T) {
	was := frameTiming.Load()
	frameTiming.Store(false)
	t.Cleanup(func() { frameTiming.Store(was) })

	tt := coreNewTester(func(c *context) { coreColumn(c).Size(200, 100) }, 200, 100)
	frameTimes.mu.Lock()
	before := frameTimes.total
	frameTimes.mu.Unlock()
	tt.Frame()
	tt.Frame()
	frameTimes.mu.Lock()
	untimed := frameTimes.total
	frameTimes.mu.Unlock()
	if untimed != before {
		t.Fatalf("%d frames timed before any reading", untimed-before)
	}

	FrameReading()
	tt.Frame()
	if got := FrameReading().Frames; got != untimed+1 {
		t.Fatalf("Frames = %d after one frame since the first reading, want %d", got, untimed+1)
	}
}
