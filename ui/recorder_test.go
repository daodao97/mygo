package ui

import (
	"math"
	"testing"
	"time"
)

// TestRecorderSize makes frames of the size asked for, whatever the
// scale makes of it in DIPs.
func TestRecorderSize(t *testing.T) {
	for _, c := range []struct {
		w, h  int
		scale float32
	}{{1920, 1080, 1}, {1920, 1080, 2}, {1920, 1080, 1.5}, {1080, 1920, 3}, {1001, 777, 1.25}, {7, 3, 2.5}} {
		r := NewRecorder(func(c *Context) { Box(c).Fill() }, RecorderOptions{Width: c.w, Height: c.h, Scale: c.scale})
		s := r.Frame(0)
		if s.Width != c.w || s.Height != c.h {
			t.Errorf("%d×%d at scale %v: a frame of %d×%d", c.w, c.h, c.scale, s.Width, s.Height)
		}
		if w, h := r.Size(); w != c.w || h != c.h {
			t.Errorf("%d×%d at scale %v: Size %d×%d", c.w, c.h, c.scale, w, h)
		}
		r.Close()
	}
}

// TestRecorderClock moves what the view animates with the times of the
// frames, however fast they are made.
func TestRecorderClock(t *testing.T) {
	var value float32
	var now time.Time
	builds := 0
	show := false
	r := NewRecorder(func(c *Context) {
		builds++
		now = c.Now()
		target := float32(0)
		if show {
			target = 1
		}
		value = Box(c).Size(10, 10).AnimateWith("o", target, time.Second, Linear)
	}, RecorderOptions{Width: 100, Height: 100, FrameRate: 30})
	defer r.Close()
	r.Frame(0)
	start := now
	show = true
	r.Frame(time.Second / 30)
	for i := 2; i <= 16; i++ {
		before := builds
		r.Frame(time.Duration(i) * time.Second / 30)
		if builds == before {
			t.Fatalf("frame %d was not built anew", i)
		}
	}
	if d := now.Sub(start); d != 16*time.Second/30 {
		t.Errorf("the view's clock is %v after 16 frames", d)
	}
	// Half a second after the target changed at frame 1.
	if math.Abs(float64(value)-0.5) > 0.01 {
		t.Errorf("half way through a linear animation of a second: %v", value)
	}
	// A time earlier than the last frame's is that time.
	r.Frame(0)
	if d := now.Sub(start); d != 16*time.Second/30 {
		t.Errorf("the clock went back to %v", d)
	}
}

// TestRecorderSettles builds a frame again at its time while the view
// asks for it, as a view measuring what it built does.
func TestRecorderSettles(t *testing.T) {
	passes := 0
	measured := false
	r := NewRecorder(func(c *Context) {
		passes++
		if !measured {
			measured = true
			c.Invalidate()
		}
		Text(c, "x")
	}, RecorderOptions{Width: 50, Height: 50})
	defer r.Close()
	r.Frame(0)
	if passes != 2 {
		t.Errorf("%d passes for a frame that asked for one more", passes)
	}
	// A view that always asks stops after a few passes.
	passes = 0
	always := NewRecorder(func(c *Context) { passes++; c.Invalidate() }, RecorderOptions{Width: 50, Height: 50})
	defer always.Close()
	always.Frame(0)
	if passes > 8 {
		t.Errorf("%d passes for a view that always asks", passes)
	}
}

func TestRecorderClose(t *testing.T) {
	r := NewRecorder(func(c *Context) { Text(c, "x") }, RecorderOptions{Width: 50, Height: 50})
	r.Frame(0)
	r.Close()
	r.Close()
	defer func() {
		if recover() == nil {
			t.Error("Frame after Close")
		}
	}()
	r.Frame(time.Second)
}

func TestSpringProgress(t *testing.T) {
	for _, o := range []SpringOptions{Smooth.Spring, Snappy.Spring, Bouncy.Spring, {Stiffness: 100, Damping: 10}} {
		if p := o.Progress(0); p != 0 {
			t.Errorf("%+v starts at %v", o, p)
		}
		settle := o.SettleTime()
		if settle <= 0 || settle >= time.Minute {
			t.Errorf("%+v settles in %v", o, settle)
		}
		if p := o.Progress(settle); math.Abs(p-1) > 0.002 {
			t.Errorf("%+v is at %v once it settles", o, p)
		}
	}
	peak := 0.0
	for d := time.Duration(0); d < time.Second; d += 10 * time.Millisecond {
		peak = max(peak, Bouncy.Spring.Progress(d))
	}
	if peak <= 1.01 {
		t.Errorf("a bouncy spring does not overshoot: %v", peak)
	}
}
