package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/internal/scene"
)

// aheadHost is a Tester's host that may ask for frames ahead, as
// Android's surface does (platform.FrameAheadSurface), and logs the frames
// asked for and presented, in order.
type aheadHost struct {
	*headless
	ahead bool
	log   []byte // 'r' for a frame asked for, 'p' for one presented
}

func (h *aheadHost) frameAhead() bool { return h.ahead }

func (h *aheadHost) requestFrame() {
	h.log = append(h.log, 'r')
	h.headless.requestFrame()
}

func (h *aheadHost) present(s *scene.Scene) {
	h.log = append(h.log, 'p')
	h.headless.present(s)
}

// aheadTester tests a scroll container under a host asking for frames
// ahead, or not.
func aheadTester(ahead bool) (*Tester, *aheadHost) {
	tt := coreNewTester(func(c *context) {
		coreScroll(c).Fill().Children(func() {
			coreBox(c).Height(5000).Shrink(0)
		})
	}, 200, 200)
	h := &aheadHost{headless: tt.h, ahead: ahead}
	tt.rt.host = h
	return tt, h
}

// TestFrameAhead checks that a frame asks for the next as it starts, before
// it presents, only while the view animates and only where the host asks
// for frames ahead; that the frame's end asks once more, for the host to
// coalesce; and that the frames stop once the view stops.
func TestFrameAhead(t *testing.T) {
	for _, ahead := range []bool{false, true} {
		tt, h := aheadTester(ahead)

		// A frame of a view that does not move asks for no frame.
		h.log = nil
		tt.Frame()
		if got := string(h.log); got != "p" {
			t.Errorf("ahead %v: still: %q, want %q", ahead, got, "p")
		}

		// A wheel's notch glides over frames: each frame of it asks for
		// the next, ahead of presenting where the host asks so.
		tt.Move(100, 100)
		tt.Wheel(100, 100, 0, 300)
		h.log = nil
		tt.Advance(time.Second / 60)
		if !tt.rt.animating {
			t.Fatalf("ahead %v: the glide does not animate", ahead)
		}
		want := "pr"
		if ahead {
			want = "rpr"
		}
		if got := string(h.log); got != want {
			t.Errorf("ahead %v: gliding: %q, want %q", ahead, got, want)
		}

		// Once the glide stops, no frame asks for another.
		tt.Advance(2 * time.Second)
		if tt.rt.animating || tt.asked() {
			t.Errorf("ahead %v: still animating after the glide (asked %v)", ahead, tt.asked())
		}
		h.log = nil
		tt.Advance(time.Second)
		if len(h.log) != 0 {
			t.Errorf("ahead %v: frames after the glide stopped: %q", ahead, h.log)
		}
	}
}

// TestFrameAheadOccluded checks that a window nothing of which shows asks
// for no frame ahead: what moves waits until it shows (next).
func TestFrameAheadOccluded(t *testing.T) {
	tt, h := aheadTester(true)
	tt.Move(100, 100)
	tt.Wheel(100, 100, 0, 300)
	tt.Advance(time.Second / 60)
	h.hidden = true
	h.log = nil
	tt.rt.runFrame()
	if got := string(h.log); got != "p" {
		t.Errorf("hidden: %q, want %q", got, "p")
	}
	if !tt.rt.held {
		t.Errorf("hidden: the glide is not held")
	}
}
