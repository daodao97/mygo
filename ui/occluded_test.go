package ui

import (
	"bytes"
	"testing"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/surface"
)

// occludedSurface is a testSurface that can be out of sight, counting the
// frames it is asked for.
type occludedSurface struct {
	testSurface
	occluded  bool
	requested int
}

func (s *occludedSurface) Occluded() bool { return s.occluded }
func (s *occludedSurface) RequestFrame()  { s.requested++ }

// occludedHost is a window host on an occludedSurface, showing count, with
// the number of views built and of times the window was ready to show.
type occludedHost struct {
	*windowHost
	s                    *occludedSurface
	count, builds, ready int
}

func newOccludedHost(t *testing.T, occluded bool) *occludedHost {
	t.Setenv("MYGO_GPU", "0")
	o := &occludedHost{s: &occludedSurface{occluded: occluded}}
	o.windowHost = &windowHost{conn: &surface.Conn{
		Surface: o.s,
		Ready:   func() { o.ready++ },
		Post:    func(f func()) { f() },
	}}
	o.rt = newRuntime(func(c *context) {
		o.builds++
		coreTextf(c, "%d", o.count)
	}, o.windowHost)
	return o
}

func (o *occludedHost) frame() { o.event(platform.SurfaceEvent{Kind: platform.SurfaceFrame}) }

func (o *occludedHost) shows() string {
	if len(o.rt.labels) == 0 {
		return ""
	}
	return o.rt.labels[0].text
}

// TestCaptureBuildsHeldFrame checks that capturing a window out of sight
// shows what changed since its last frame, which waited for it to show;
// the window then waits again, and draws once it shows.
func TestCaptureBuildsHeldFrame(t *testing.T) {
	o := newOccludedHost(t, false)
	o.frame()
	o.s.occluded = true
	o.count = 1
	o.rt.changed()
	o.frame()
	if o.builds != 1 || !o.rt.held {
		t.Fatalf("a change out of sight: %d builds, held %v", o.builds, o.rt.held)
	}
	if w, h, pix := o.capture(); w == 0 || h == 0 || len(pix) == 0 || o.builds != 2 || o.shows() != "1" {
		t.Fatalf("captured %dx%d after %d builds, showing %q: the change not built", w, h, o.builds, o.shows())
	}
	// The surface's frame the capture asked for waits for the window.
	o.frame()
	if o.builds != 2 || !o.rt.held {
		t.Fatalf("after the capture, out of sight: %d builds, held %v", o.builds, o.rt.held)
	}
	o.s.occluded = false
	asked := o.s.requested
	o.event(platform.SurfaceEvent{Kind: platform.SurfaceShown})
	if o.s.requested == asked {
		t.Fatal("shown again, it asked for no frame")
	}
}

// TestCaptureBuildsFrameAskedOutOfSight checks that capturing a window out
// of sight whose surface draws no frame, as macOS's of a hidden window or
// behind the locked screen, shows what changed since its last frame, which
// never came; and that a capture after it, with nothing changed, builds
// nothing more.
func TestCaptureBuildsFrameAskedOutOfSight(t *testing.T) {
	o := newOccludedHost(t, false)
	o.frame()
	o.s.occluded = true
	o.count = 1
	o.rt.changed()
	if w, _, _ := o.capture(); w == 0 || o.builds != 2 || o.shows() != "1" {
		t.Fatalf("captured after %d builds, showing %q: the change not built", o.builds, o.shows())
	}
	o.capture()
	if o.builds != 2 {
		t.Errorf("a capture of what was captured built %d more", o.builds-2)
	}
	o.count = 2
	o.rt.changed()
	if o.capture(); o.builds != 3 || o.shows() != "2" {
		t.Errorf("a change after the capture: %d builds, showing %q", o.builds, o.shows())
	}
}

// newDrawingHost is a window host in sight on an occludedSurface, whose
// view draws with draw, counting the views built.
func newDrawingHost(t *testing.T, draw func(p *Painter, r Rect)) *occludedHost {
	t.Setenv("MYGO_GPU", "0")
	o := &occludedHost{s: &occludedSurface{}}
	o.windowHost = &windowHost{conn: &surface.Conn{Surface: o.s, Post: func(f func()) { f() }}}
	o.rt = newRuntime(func(c *context) {
		o.builds++
		coreBox(c).Size(200, 100).Background(RGB(255, 255, 255)).Draw(draw)
	}, o.windowHost)
	return o
}

// TestCaptureAfterAtlasMoved checks that capturing a window after another
// window made room in the glyph atlases they share (System.MakeRoom) shows
// its text: the window's last frame drew its glyphs from where they were
// before, which then held the other window's (#1920). A capture after
// that, with nothing changed, builds nothing more.
func TestCaptureAfterAtlasMoved(t *testing.T) {
	b := newDrawingHost(t, func(p *Painter, r Rect) {
		p.Text(r.X+4, r.Y+30, "Hello, world", 20, RGB(0, 0, 0))
	})
	b.frame()
	_, _, want := b.capture()
	builds := b.builds
	// Another window drawing more glyphs than the atlas has room for, at
	// new sizes each frame, as one zooming in and out. ASCII, which every
	// font the tests may get has (a CI runner may have no CJK font).
	size := float32(40)
	a := newDrawingHost(t, func(p *Painter, r Rect) {
		for s := size; s < size+40; s += 4 {
			for c := '!'; c <= '~'; c++ {
				p.Text(r.X, r.Y+40, string(c), s, RGB(0, 0, 0))
			}
		}
	})
	for range 20 {
		a.frame()
		if atlasGenerations(b.last) != b.rt.atlases {
			break
		}
		size += 40
	}
	if atlasGenerations(b.last) == b.rt.atlases {
		t.Fatal("the other window never made room in the atlases")
	}
	if _, _, got := b.capture(); !bytes.Equal(got, want) || b.builds != builds+1 {
		t.Errorf("captured after %d builds, equal to before: %v", b.builds-builds, bytes.Equal(got, want))
	}
	builds = b.builds
	if b.capture(); b.builds != builds {
		t.Errorf("a capture of what was captured built %d more", b.builds-builds)
	}
}

// TestHiddenWindowShowsAfterCapture checks that a window made hidden,
// which is out of sight until shown, still draws its first frame to its
// surface, and so becomes ready to show, though a capture built a frame
// first: OnReadyToShow(w.Show) shows such windows.
func TestHiddenWindowShowsAfterCapture(t *testing.T) {
	o := newOccludedHost(t, true)
	o.capture()
	o.frame()
	if o.ready != 1 {
		t.Fatalf("ready to show %d times after its first frame on the surface (%d builds)", o.ready, o.builds)
	}
}

// firstFrameSurface is an occludedSurface told of the first frame
// (platform.FirstFrameSurface), with the times the window was ready to
// show by then.
type firstFrameSurface struct {
	occludedSurface
	o           *occludedHost
	firstFrames []int
}

func (s *firstFrameSurface) FirstFrame() { s.firstFrames = append(s.firstFrames, s.o.ready) }

// TestFirstFrameTold checks that a surface waiting for the content's first
// frame, as Windows' keeps its window out of sight until then, is told
// once, as the frame reached it, before the window is ready to show: not
// for a capture's frame, which never reaches it, nor for the frames after.
// It is told again of the first frame on its new native objects, after a
// SurfaceRenew, as Android's host holds the window's first draw until it
// shows one; the window is not ready to show again.
func TestFirstFrameTold(t *testing.T) {
	o := newOccludedHost(t, false)
	s := &firstFrameSurface{o: o}
	o.s, o.conn.Surface = &s.occludedSurface, s
	o.capture()
	if len(s.firstFrames) != 0 {
		t.Fatalf("told of the first frame after a capture")
	}
	o.frame()
	o.count = 1
	o.rt.changed()
	o.frame()
	if len(s.firstFrames) != 1 || s.firstFrames[0] != 0 || o.ready != 1 {
		t.Fatalf("told of the first frame %d times, when ready to show %v times; ready %d times", len(s.firstFrames), s.firstFrames, o.ready)
	}

	o.event(platform.SurfaceEvent{Kind: platform.SurfaceRenew})
	if len(s.firstFrames) != 1 {
		t.Fatalf("renewed: told of a first frame before one")
	}
	o.frame()
	o.count = 2
	o.rt.changed()
	o.frame()
	if len(s.firstFrames) != 2 || o.ready != 1 {
		t.Fatalf("renewed: told of the first frame %d times in all, ready %d times", len(s.firstFrames), o.ready)
	}
}

// TestOccludedHoldsWithAccessibility checks that a window out of sight
// holds its frames though assistive technology once read it, as utilities
// reading every window's accessibility do; and that it builds the frame
// waiting when assistive technology turns on, so that it reads what the
// window shows now.
func TestOccludedHoldsWithAccessibility(t *testing.T) {
	o := newOccludedHost(t, false)
	o.frame()
	o.event(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	o.frame()
	built := o.builds
	o.s.occluded = true
	o.count = 1
	o.rt.changed()
	o.frame()
	if o.builds != built || !o.rt.held {
		t.Fatalf("a change out of sight, accessibility on: built %d more, held %v", o.builds-built, o.rt.held)
	}

	// Assistive technology turning on while a frame waits.
	o = newOccludedHost(t, false)
	o.frame()
	o.s.occluded = true
	o.count = 1
	o.rt.changed()
	o.frame()
	asked := o.s.requested
	o.event(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	if o.s.requested == asked {
		t.Fatal("accessibility on while a frame waits: no frame asked for")
	}
	o.frame()
	if o.shows() != "1" {
		t.Errorf("accessibility reads %q, not what the window shows now", o.shows())
	}
}
