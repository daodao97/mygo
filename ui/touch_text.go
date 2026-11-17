package ui

import (
	"runtime"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

// A finger edits text as phones have it: a word chosen by holding it, handles
// at the selection's ends that drag them, with the system's magnifier, and the
// system's menu of what to do with it. The handles draw over everything, and
// take the fingers near them before what is under them.

// handleReach is how far from a handle, in DIPs, a finger takes it, and
// caretHandleFor how long Android's caret handle shows after the caret moved.
const (
	handleReach    = 22
	caretHandleFor = 4 * time.Second
)

// Which handle a finger holds.
const (
	handleNone = iota
	handleStart
	handleEnd
	handleCaret
)

// touchText is the handles of the focused text input.
type touchText struct {
	// id is the input that fingers last placed the selection or caret of, 0
	// for none: keys hide its handles.
	id uint64
	// caretAt is when the caret handle started showing.
	caretAt time.Time
	// held is the handle a finger holds.
	held int
}

// handles returns the input whose handles show and the places of its selection
// or caret, in the window: where it starts and ends.
func (rt *engine) handles() (a *textInputAdapter, start, end platform.RectF, sel platform.TextRange, ok bool) {
	t := &rt.touchText
	if t.id == 0 || t.id != rt.focused {
		return nil, start, end, sel, false
	}
	s := rt.states[t.id]
	if s == nil || s.textAdapter == nil || s.textClient == nil || s.editor != nil && s.editor.password {
		return nil, start, end, sel, false
	}
	a = s.textAdapter
	sel = a.Selection().Range
	var got bool
	if start, _, got = a.BoundsForRange(platform.TextRange{Start: sel.Start, End: sel.Start}); !got {
		return nil, start, end, sel, false
	}
	if end, _, got = a.BoundsForRange(platform.TextRange{Start: sel.End, End: sel.End}); !got {
		return nil, start, end, sel, false
	}
	// Hidden as what holds the text scrolled it away.
	r := transformRect(s.world, Rect{s.x, s.y, s.w, s.h})
	w := rt.toWindow(platform.RectF{X: float64(r.X), Y: float64(r.Y), W: float64(r.W), H: float64(r.H)})
	if end.Y+end.H < w.Y-1 || start.Y > w.Y+w.H+1 {
		return nil, start, end, sel, false
	}
	return a, start, end, sel, true
}

// caretHandles reports whether a caret, with nothing selected, has a handle:
// Android's do, for a while after it moved; iOS has none.
func (rt *engine) caretHandle(now time.Time) bool {
	t := &rt.touchText
	return overscrollKind == overscrollStretch && !t.caretAt.IsZero() && now.Sub(t.caretAt) < caretHandleFor
}

// paintHandles draws the selection's handles over what the window shows.
func (rt *engine) paintHandles(p *Painter) {
	_, a, b, sel, ok := rt.handles()
	if !ok {
		return
	}
	color := p.Theme().Accent
	cupertino := runtime.GOOS == "ios" || overscrollKind == overscrollBounce
	if sel.Start == sel.End {
		if rt.caretHandle(p.Now()) || rt.touchText.held == handleCaret {
			drawTeardrop(p, float32(b.X), float32(b.Y+b.H), color, 0)
			p.After(time.Second)
		}
		return
	}
	if cupertino {
		// A bar along the line, with a knob at its outer end.
		bar := func(r platform.RectF, top bool) {
			x, y0, y1 := float32(r.X), float32(r.Y), float32(r.Y+r.H)
			p.Fill(Rect{x - 1, y0, 2, y1 - y0}, color, 1)
			cy := y1 + 4
			if top {
				cy = y0 - 4
			}
			p.Fill(Rect{x - 5, cy - 5, 10, 10}, color, 5)
		}
		bar(a, true)
		bar(b, false)
		return
	}
	// Android's teardrops under the line, leaning out of the selection.
	drawTeardrop(p, float32(a.X), float32(a.Y+a.H), color, -1)
	drawTeardrop(p, float32(b.X), float32(b.Y+b.H), color, 1)
}

// drawTeardrop draws Android's text handle with its point at x, y: straight
// down from the point (lean 0), or leaning to a side.
func drawTeardrop(p *Painter, x, y float32, c Color, lean float32) {
	const r = 9
	cx, cy := x+lean*r, y+r
	p.Fill(Rect{cx - r, cy - r + 2, 2 * r, 2 * r}, c, r)
	var path Path
	if lean == 0 {
		path.MoveTo(x, y).LineTo(x+r*0.8, y+r*0.6).LineTo(x-r*0.8, y+r*0.6).Close()
	} else {
		path.MoveTo(x, y).LineTo(x+lean*r*1.6, y+2).LineTo(x, y+r*1.4).Close()
	}
	p.FillPath(&path, c)
}

// handleAt returns the handle a finger going down at x, y takes.
func (rt *engine) handleAt(x, y float32) int {
	_, a, b, sel, ok := rt.handles()
	if !ok {
		return handleNone
	}
	near := func(px, py float64) bool {
		dx, dy := float64(x)-px, float64(y)-py
		return dx*dx+dy*dy < handleReach*handleReach
	}
	if sel.Start == sel.End {
		if rt.caretHandle(rt.now()) && near(b.X, b.Y+b.H+9) {
			return handleCaret
		}
		return handleNone
	}
	// The nearer one, as they may touch.
	if near(b.X, b.Y+b.H) {
		return handleEnd
	}
	if near(a.X, a.Y) || near(a.X, a.Y+a.H) {
		return handleStart
	}
	return handleNone
}

// dragHandle moves the selection's end a finger holds to under it.
func (rt *engine) dragHandle(x, y float32) {
	a, _, _, sel, ok := rt.handles()
	if !ok {
		return
	}
	t := &rt.touchText
	// The line above the finger, which hides what is under it.
	wx, wy := rt.windowPoint(x, y)
	i, found := a.IndexForPoint(wx, wy-12)
	if !found {
		return
	}
	to := platform.TextRange{Start: sel.Start, End: sel.End}
	switch t.held {
	case handleCaret:
		to = platform.TextRange{Start: i, End: i}
	case handleStart:
		if i >= sel.End {
			i = sel.End - 1
		}
		to.Start = i
	case handleEnd:
		if i <= sel.Start {
			i = sel.Start + 1
		}
		to.End = i
	}
	if to != sel {
		a.SetTextSelection(to)
		rt.haptic("selection")
	}
	rt.touchSurface(func(s platform.TouchSurface) { s.ShowMagnifier(wx, wy) })
	t.caretAt = rt.now()
	rt.requestFrame()
}

// windowPoint returns the content's x, y in the window.
func (rt *engine) windowPoint(x, y float32) (float64, float64) {
	r := rt.toWindow(platform.RectF{X: float64(x), Y: float64(y)})
	return r.X, r.Y
}

// touchSurface calls fn with the window's touch surface, where it has one.
func (rt *engine) touchSurface(fn func(platform.TouchSurface)) {
	if h, ok := rt.host.(interface{ touchSurface() platform.TouchSurface }); ok {
		if s := h.touchSurface(); s != nil {
			fn(s)
		}
	}
}

// touchSurfaceOf returns the window's touch surface, nil without one.
func (rt *engine) touchSurfaceOf() platform.TouchSurface {
	if h, ok := rt.host.(interface{ touchSurface() platform.TouchSurface }); ok {
		return h.touchSurface()
	}
	return nil
}

// showEditMenu shows the system's menu of editing commands over the
// selection of the input with the focus.
func (rt *engine) showEditMenu(s *state) {
	var actions []string
	for _, name := range [...]string{"cut", "copy", "paste", "selectAll"} {
		if rt.commandEnabled(name) {
			actions = append(actions, name)
		}
	}
	if len(actions) == 0 {
		return
	}
	var x, y float64
	if _, a, b, _, ok := rt.handles(); ok {
		x, y = (a.X+b.X)/2, a.Y
	} else {
		r := transformRect(s.world, Rect{s.x, s.y, s.w, s.h})
		w := rt.toWindow(platform.RectF{X: float64(r.X + r.W/2), Y: float64(r.Y)})
		x, y = w.X, w.Y
	}
	rt.touchSurface(func(ts platform.TouchSurface) { ts.ShowEditMenu(x, y, actions) })
}

// textTapped notes a finger's tap on the text input s, which it placed the
// caret of: tapped again on the caret, the menu shows what can be done there.
func (rt *engine) textTapped(s *state, focused bool, sel platform.TextRange) {
	t := &rt.touchText
	t.id = s.id
	t.caretAt = rt.now()
	if focused && s.textAdapter != nil && s.textAdapter.Selection().Range == sel && sel.Start == sel.End {
		rt.showEditMenu(s)
	}
}

// textKeyed hides the handles as keys type: what they point at moved.
func (rt *engine) textKeyed() { rt.touchText.caretAt = time.Time{} }

// revealFocus scrolls the input with the focus into view, as the keyboard
// covers part of the window.
func (rt *engine) revealFocus() {
	if s := rt.states[rt.focused]; s != nil && s.textClient != nil {
		rt.reveal(rt.focused)
	}
}
