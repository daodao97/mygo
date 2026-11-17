//go:build windows && (amd64 || arm64)

package windows

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// touchHandler records what a surface sends, and answers DragRegionAt
// for the points in drag, as a title bar the content draws.
type touchHandler struct {
	platform.WindowHandler
	events []platform.SurfaceEvent
	drag   platform.RectF
}

func (h *touchHandler) SurfaceEvent(ev platform.SurfaceEvent) bool {
	if ev.Kind == platform.DragRegionAt {
		return h.drag.Contains(ev.X, ev.Y)
	}
	h.events = append(h.events, ev)
	return false
}

// TestFingerMessages feeds a surface a finger's WM_POINTER messages, as
// pointerMessage reads them: a drag scrolls rather than presses, a tap
// clicks and a long one right-clicks, a canceled finger or one another
// window takes taps nothing, and a finger landing on a drag region is
// left to DefWindowProc, whose mouse moves the window (#1712). Every
// finger the gesture takes also comes on its own, by its pointer ID
// (#1884).
func TestFingerMessages(t *testing.T) {
	const (
		primary   = pointerMessageFlagPrimary | pointerMessageFlagInContact
		inContact = pointerMessageFlagInContact
		canceled  = pointerMessageFlagCanceled
	)
	type msg struct {
		m     uint32
		id    uint16
		flags uint16
		x, y  float64
		t     uint32
		lost  bool // WM_POINTERCAPTURECHANGED
	}
	down := func(id, flags uint16, x, y float64, t uint32) msg {
		return msg{wmPointerDown, id, flags, x, y, t, false}
	}
	move := func(id uint16, x, y float64) msg { return msg{wmPointerUpdate, id, inContact, x, y, 0, false} }
	up := func(id, flags uint16, x, y float64, t uint32) msg { return msg{wmPointerUp, id, flags, x, y, t, false} }
	lost := func(id uint16) msg { return msg{lost: true, id: id} }
	const (
		touchDown   = platform.TouchDown
		touchMove   = platform.TouchMove
		touchUp     = platform.TouchUp
		touchCancel = platform.TouchCancel
		press       = platform.PointerDown
		release     = platform.PointerUp
		scroll      = platform.PointerScroll
		zoom        = platform.PointerZoom
		leave       = platform.PointerLeave
	)
	for _, c := range []struct {
		name  string
		msgs  []msg
		taken []bool
		want  []platform.SurfaceEventKind
		check func(t *testing.T, evs []platform.SurfaceEvent)
	}{
		{
			name:  "drag",
			msgs:  []msg{down(7, primary, 100, 200, 0), move(7, 100, 190), move(7, 100, 150), up(7, 0, 100, 150, 300)},
			taken: []bool{true, true, true, true},
			want:  []platform.SurfaceEventKind{touchDown, touchMove, scroll, touchMove, scroll, touchUp, leave},
			check: func(t *testing.T, evs []platform.SurfaceEvent) {
				// Past the slop, the content follows the finger from
				// where it landed.
				var dy float64
				for _, ev := range evs {
					if ev.Kind == scroll {
						if !ev.Precise || ev.Pointer != platform.PointerTouch {
							t.Errorf("scroll %+v: not precise, or not a finger's", ev)
						}
						dy += ev.DY
					}
				}
				if dy != 50 {
					t.Errorf("scrolled %v, want 50", dy)
				}
			},
		},
		{
			name:  "tap",
			msgs:  []msg{down(7, primary, 100, 200, 0), move(7, 102, 201), up(7, 0, 102, 201, 100)},
			taken: []bool{true, true, true},
			want:  []platform.SurfaceEventKind{touchDown, touchMove, touchUp, press, release, leave},
			check: func(t *testing.T, evs []platform.SurfaceEvent) {
				p := evs[3]
				if p.Button != 0 || p.Clicks != 1 || p.X != 100 || p.Y != 200 || p.Pointer != platform.PointerTouch {
					t.Errorf("tap's press %+v, want the first button once where it landed, by a finger", p)
				}
				if evs[0].ID != 7 || evs[2].ID != 7 {
					t.Errorf("the finger's IDs %d and %d, want its pointer ID 7", evs[0].ID, evs[2].ID)
				}
			},
		},
		{
			name:  "press and hold",
			msgs:  []msg{down(7, primary, 100, 200, 1000), up(7, 0, 100, 200, 1600)},
			taken: []bool{true, true},
			want:  []platform.SurfaceEventKind{touchDown, touchUp, press, release, leave},
			check: func(t *testing.T, evs []platform.SurfaceEvent) {
				if evs[2].Button != 1 {
					t.Errorf("held press of button %d, want 1", evs[2].Button)
				}
			},
		},
		{
			name: "pinch",
			msgs: []msg{
				down(7, primary, 100, 200, 0), down(8, inContact, 200, 200, 10),
				move(8, 300, 200), up(8, 0, 300, 200, 100), up(7, 0, 100, 200, 110),
			},
			taken: []bool{true, true, true, true, true},
			want:  []platform.SurfaceEventKind{touchDown, touchDown, touchMove, scroll, zoom, touchUp, touchUp, leave},
		},
		{
			name:  "canceled",
			msgs:  []msg{down(7, primary, 100, 200, 0), up(7, canceled, 100, 200, 50)},
			taken: []bool{true, true},
			want:  []platform.SurfaceEventKind{touchDown, touchCancel, leave},
		},
		{
			name:  "taken by another window",
			msgs:  []msg{down(7, primary, 100, 200, 0), lost(7), up(7, 0, 100, 200, 50)},
			taken: []bool{true, true, false},
			want:  []platform.SurfaceEventKind{touchDown, leave},
		},
		{
			// DefWindowProc's mouse presses there, and the content's
			// StartDrag moves the window; pointerMessage sends the
			// finger itself.
			name:  "on a drag region",
			msgs:  []msg{down(7, primary, 100, 20, 0), move(7, 100, 80), up(7, 0, 100, 80, 50)},
			taken: []bool{false, false, false},
		},
		{
			// A second finger lands on the content while the first moves
			// the window.
			name:  "beside one on a drag region",
			msgs:  []msg{down(7, primary, 100, 20, 0), down(8, inContact, 100, 200, 10), up(8, 0, 100, 200, 60), up(7, 0, 100, 80, 70)},
			taken: []bool{false, true, true, false},
			want:  []platform.SurfaceEventKind{touchDown, touchUp, press, release, leave},
		},
		{
			name:  "new touches",
			msgs:  []msg{down(7, primary, 100, 200, 0), down(9, primary, 300, 200, 1000), up(9, 0, 300, 200, 1050), up(7, 0, 100, 200, 1060)},
			taken: []bool{true, true, true, false},
			// The first finger's end went unseen: the second taps alone.
			want: []platform.SurfaceEventKind{touchDown, touchDown, touchUp, press, release, leave},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := &touchHandler{drag: platform.RectF{W: 400, H: 40}}
			s := &surface{w: &window{h: h}}
			for i, mm := range c.msgs {
				wp := uintptr(mm.flags)<<16 | uintptr(mm.id)
				var took bool
				if mm.lost {
					took = s.touchCaptureLost(wp)
				} else {
					took = s.fingerMessage(mm.m, wp, mm.x, mm.y, 0.5, mm.t)
				}
				if took != c.taken[i] {
					t.Errorf("message %d (%#x of %d): taken %v, want %v", i, mm.m, mm.id, took, c.taken[i])
				}
			}
			var kinds []platform.SurfaceEventKind
			for _, ev := range h.events {
				kinds = append(kinds, ev.Kind)
			}
			if !slices.Equal(kinds, c.want) {
				t.Fatalf("sent %v, want %v", kinds, c.want)
			}
			if c.check != nil {
				c.check(t, h.events)
			}
		})
	}
}
