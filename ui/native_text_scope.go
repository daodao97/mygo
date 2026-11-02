package ui

import (
	"strings"

	"github.com/egoist/mygo/internal/platform"
)

// nativeTextScope gives UIKit one read-only document for a Selectable
// container. Paragraphs keep their own rich layouts; only indices are joined.
// The snapshot is rebuilt only when its paragraphs or their text change.
type nativeTextScope struct {
	scope uint64
	text  string
	count int
	parts []nativeTextPart
}

type nativeTextPart struct {
	id         uint64
	text       string
	start, end int
}

func (d *nativeTextScope) sync(rt *engine) {
	i, same := 0, true
	for _, s := range rt.texts {
		if s.textScope != d.scope {
			continue
		}
		if i >= len(d.parts) || d.parts[i].id != s.id || d.parts[i].text != s.editor.source {
			same = false
		}
		i++
	}
	if same && i == len(d.parts) {
		return
	}
	d.parts, d.count = d.parts[:0], 0
	var text strings.Builder
	for _, s := range rt.texts {
		if s.textScope != d.scope {
			continue
		}
		if len(d.parts) > 0 {
			text.WriteByte('\n')
			d.count++
		}
		start := d.count
		text.WriteString(s.editor.source)
		d.count += s.editor.buf.n
		d.parts = append(d.parts, nativeTextPart{id: s.id, text: s.editor.source, start: start, end: d.count})
	}
	d.text = text.String()
}

func (d *nativeTextScope) index(p textPoint) int {
	for _, part := range d.parts {
		if part.id == p.id {
			return part.start + max(0, min(p.at, part.end-part.start))
		}
	}
	return 0
}

func (d *nativeTextScope) point(index int) textPoint {
	index = max(0, min(index, d.count))
	for i, part := range d.parts {
		if index <= part.end || i == len(d.parts)-1 {
			return textPoint{part.id, max(0, min(index-part.start, part.end-part.start))}
		}
	}
	return textPoint{}
}

func makeNativeTextScope(scope uint64) nativeSelectionScope {
	return &nativeTextScope{scope: scope}
}

func (d *nativeTextScope) owns(scope uint64) bool { return d.scope == scope }

func (d *nativeTextScope) input(rt *engine, s *state, t platform.TextInputState) platform.TextInputState {
	d.sync(rt)
	a, z := textPoint{s.id, s.editor.anchor}, textPoint{s.id, s.editor.caret}
	if rt.selection.scope == s.textScope {
		a, z = rt.selection.anchor, rt.selection.caret
	}
	t.Text, t.Start, t.End = d.text, d.index(a), d.index(z)
	if t.Start > t.End {
		t.Start, t.End = t.End, t.Start
	}
	if scope := rt.states[s.textScope]; scope != nil {
		t.Bounds = rt.toWindow(platformRect(Rect{scope.vx, scope.vy, scope.vw, scope.vh}))
	}
	if caret, ok := rt.nativeScopeCaret(d.point(t.End)); ok {
		t.Caret = caret
	}
	return t
}

func (rt *engine) nativeScopeCaret(p textPoint) (platform.RectF, bool) {
	s := rt.states[p.id]
	if s == nil || s.editor == nil || s.editor.layout == nil {
		return platform.RectF{}, false
	}
	ed := s.editor
	x, y, h := ed.layout.Caret(ed.displayIndex(p.at))
	r := transformRect(s.world, Rect{s.x + ed.originX + x - ed.scrollX, s.y + ed.originY + y, 1, h})
	return rt.toWindow(platformRect(r)), true
}

func (d *nativeTextScope) geometry(rt *engine, q platform.TextGeometryQuery) platform.TextGeometry {
	result := platform.TextGeometry{Valid: true}
	start, end := max(0, min(q.Start, d.count)), max(0, min(q.End, d.count))
	switch q.Kind {
	case "hit":
		z := rt.zoomFactor()
		p := rt.textSelectionAt(d.scope, float32(q.X)/z, float32(q.Y)/z)
		if p.id == 0 {
			return platform.TextGeometry{}
		}
		result.Index = d.index(p)
	case "caret":
		var ok bool
		result.Caret, ok = rt.nativeScopeCaret(d.point(start))
		if !ok {
			return platform.TextGeometry{}
		}
	case "selection":
		if start > end {
			start, end = end, start
		}
		if start == end {
			return result
		}
		for _, part := range d.parts {
			if end <= part.start || start >= part.end {
				continue
			}
			s := rt.states[part.id]
			if s == nil || s.editor == nil || s.editor.layout == nil || s.vw <= 0 || s.vh <= 0 {
				continue
			}
			ed := s.editor
			from, to := max(start-part.start, 0), min(end-part.start, ed.buf.n)
			visible := Rect{s.vx, s.vy, s.vw, s.vh}
			for _, segment := range ed.layout.Selection(ed.displayIndex(from), ed.displayIndex(to)) {
				r := transformRect(s.world, Rect{s.x + ed.originX + segment.X - ed.scrollX, s.y + ed.originY + segment.Y, segment.W, segment.H})
				if r = intersect(r, visible); r.W > 0 && r.H > 0 {
					result.Rects = append(result.Rects, platform.TextSelectionRect{RectF: rt.toWindow(platformRect(r)), RTL: ed.layout.Lines[0].RTL})
				}
			}
		}
	default:
		return platform.TextGeometry{}
	}
	return result
}
