//go:build linux && !android && (amd64 || arm64)

package linux

import (
	"slices"

	"github.com/egoist/mygo/internal/platform"
)

// The fingers' taps, scrolls and pinches are platform.TouchGesture's
// (surface.go gives it GTK's touches).

// touchPoints numbers the fingers on a surface for the views that follow
// each (platform.TouchDown): GDK's sequences, which it frees and gives the
// fingers after, become IDs counted from 1.
type touchPoints struct {
	down []touchPoint
	last uint64
}

type touchPoint struct {
	seq uintptr
	id  uint64
}

// event returns the kind and the finger's ID of the touch event of a
// GdkEventTouch of type typ (GDK_TOUCH_BEGIN 37, UPDATE 38, END 39, CANCEL
// 40) of seq, and false for a sequence that did not begin here.
func (p *touchPoints) event(typ int32, seq uintptr) (kind platform.SurfaceEventKind, id uint64, ok bool) {
	i := slices.IndexFunc(p.down, func(t touchPoint) bool { return t.seq == seq })
	switch {
	case typ == 37 && i < 0:
		p.last++
		p.down = append(p.down, touchPoint{seq, p.last})
		return platform.TouchDown, p.last, true
	case i < 0, typ == 37:
		return 0, 0, false
	case typ == 38:
		return platform.TouchMove, p.down[i].id, true
	}
	id = p.down[i].id
	p.down = slices.Delete(p.down, i, i+1)
	if typ == 40 {
		return platform.TouchCancel, id, true
	}
	return platform.TouchUp, id, true
}

// reset forgets the fingers down, whose ends went unseen, returning their
// IDs, which the surface cancels.
func (p *touchPoints) reset() []uint64 {
	var ids []uint64
	for _, t := range p.down {
		ids = append(ids, t.id)
	}
	p.down = p.down[:0]
	return ids
}
