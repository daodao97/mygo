package ui

import (
	"unicode/utf8"

	"github.com/egoist/mygo/internal/platform"
)

// A container installs this bridge only when Selectable is used. Native
// scope snapshots and their indexing do not grow apps without selectable text.
type nativeSelectionScope interface {
	owns(uint64) bool
	input(*engine, *state, platform.TextInputState) platform.TextInputState
	point(int) textPoint
	geometry(*engine, platform.TextGeometryQuery) platform.TextGeometry
}

func platformRect(r Rect) platform.RectF {
	return platform.RectF{X: float64(r.X), Y: float64(r.Y), W: float64(r.W), H: float64(r.H)}
}

func (rt *engine) nativeSelection(s *state) bool {
	h, ok := rt.host.(interface{ nativeTextSelection() bool })
	if !ok || !h.nativeTextSelection() || !rt.windowFocused || s.editor == nil || s.editor.password || s.editor.compose != "" {
		return false
	}
	if rt.focused == s.id {
		return true
	}
	// UIKit paints one shared selection over all paragraphs of a message.
	f := rt.states[rt.focused]
	return s.textScope != 0 && f != nil && f.textScope == s.textScope && f.editor != nil && f.editor.readOnly
}

// textGeometry reads the same layouts used to paint the text. The native
// proxy only sees a bounded surrounding-text window; translate its rune
// indices back into the editor before answering, and reject stale field IDs.
func (rt *engine) textGeometry(q platform.TextGeometryQuery) platform.TextGeometry {
	s := rt.states[rt.focused]
	if !rt.ime.state.Active || q.ID != rt.ime.state.ID || s == nil || s.id != q.ID ||
		s.editor == nil || s.editor.password || s.editor.compose != "" {
		return platform.TextGeometry{}
	}
	if d := rt.ime.scope; d != nil && d.owns(s.textScope) {
		return d.geometry(rt, q)
	}
	ed := s.editor
	n := utf8.RuneCountInString(rt.ime.state.Text)
	start := rt.ime.base + max(0, min(q.Start, n))
	end := rt.ime.base + max(0, min(q.End, n))
	ox, oy := s.x+ed.originX, s.y+ed.originY
	result := platform.TextGeometry{Valid: true}
	switch q.Kind {
	case "hit":
		z := rt.zoomFactor()
		result.Index = max(0, min(ed.hit(float32(q.X)/z-s.x, float32(q.Y)/z-s.y)-rt.ime.base, n))
	case "caret":
		var r Rect
		if a := ed.area; a != nil && a.version != 0 {
			x, y, h := a.caretAt(ed, start, 0)
			r = Rect{ox + x, oy + float32(y-a.scroll), 1, h}
		} else if ed.layout != nil {
			x, y, h := ed.layout.Caret(ed.displayIndex(start))
			r = Rect{ox + x - ed.scrollX, oy + y, 1, h}
		} else {
			return platform.TextGeometry{}
		}
		result.Caret = rt.toWindow(platformRect(r))
	case "selection":
		if start > end {
			start, end = end, start
		}
		visible := Rect{s.vx, s.vy, s.vw, s.vh}
		appendRect := func(r Rect, rtl bool) {
			if r = intersect(r, visible); r.W > 0 && r.H > 0 {
				result.Rects = append(result.Rects, platform.TextSelectionRect{RectF: rt.toWindow(platformRect(r)), RTL: rtl})
			}
		}
		if a := ed.area; a != nil {
			// Invisible paragraphs need no selection visuals and must not be
			// shaped merely because a large document was selected.
			for p := a.first; p <= a.last && p < len(ed.buf.paras); p++ {
				pr := &ed.buf.paras[p]
				if pr.layout == nil || end <= pr.rune || start > ed.buf.end(p) {
					continue
				}
				for _, r := range pr.layout.SelectionOn(max(start-pr.rune, 0), min(end-pr.rune, ed.buf.end(p)-pr.rune), end > ed.buf.end(p) && p < len(ed.buf.paras)-1) {
					appendRect(Rect{ox + r.X, oy + float32(a.hs.top(p)-a.scroll) + r.Y, r.W, r.H}, pr.layout.Lines[0].RTL)
				}
			}
		} else if ed.layout != nil {
			for _, r := range ed.layout.Selection(ed.displayIndex(start), ed.displayIndex(end)) {
				appendRect(Rect{ox + r.X - ed.scrollX, oy + r.Y, r.W, r.H}, ed.layout.Lines[0].RTL)
			}
		}
	default:
		return platform.TextGeometry{}
	}
	return result
}
