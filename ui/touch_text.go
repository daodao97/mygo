package ui

import (
	"github.com/egoist/mygo/internal/platform"
	"time"
)

type textTouchSelection struct {
	rt       *engine
	target   uint64
	selected bool
}

func (p *textTouchSelection) reset()        { p.target = 0; p.selected = false }
func (p *textTouchSelection) pending() bool { return p.target != 0 }
func (p *textTouchSelection) started() bool { return p.selected }
func (p *textTouchSelection) reserve(x, y float32) bool {
	s := p.hit(x, y)
	if s == nil {
		return false
	}
	p.target = s.id
	p.rt.touch.holdUntil = p.rt.now().Add(500 * time.Millisecond)
	p.rt.requestFrame()
	return true
}
func (p *textTouchSelection) finish(kind platform.SurfaceEventKind) bool {
	if p.target == 0 {
		return false
	}
	p.target = 0
	if !p.rt.touch.scrolling && kind == platform.PointerUp {
		p.rt.selection = textSelection{}
		p.rt.focused = 0
		p.rt.requestFrame()
	}
	return true
}

// touchTextAt reserves ordinary read-only text, including a selectable
// container's padding. Interactive children and custom input handlers keep
// their tap behavior (links, buttons, editable fields and custom editors).
func (p *textTouchSelection) hit(x, y float32) *state {
	rt := p.rt
	chain := rt.hitChain(x, y)
	if rt.handler(chain) != nil {
		return nil
	}
	for _, id := range chain {
		s := rt.states[id]
		if s == nil {
			continue
		}
		if s.flags&(flagDisabled|flagInert|flagUnselectable) != 0 || s.input != nil {
			return nil
		}
		if s.flags&interactive == 0 {
			continue
		}
		if s.flags&flagSelectable != 0 && s.flags&(flagEditable|flagClickable|flagFocusable|flagDraggable|flagTrackPointer) == 0 && (s.editor == nil || s.editor.readOnly) {
			return s
		}
		return nil
	}
	return nil
}

func (p *textTouchSelection) begin() {
	rt := p.rt
	t := &rt.touch
	if t.selecting || p.target == 0 || t.holdUntil.IsZero() {
		return
	}
	s := p.hit(t.x, t.y)
	if s == nil || s.id != p.target {
		return
	}
	rt.pointerMove(t.x, t.y)
	rt.pointerDown(t.x, t.y, 0, 0, 1)
	if focus := rt.states[rt.focused]; focus != nil && focus.editor != nil && focus.editor.readOnly {
		// A long press is a word selection, independent of the backend's
		// multi-click counter (which restarts when the target changes).
		if !rt.pressTextSelection(focus, t.x, t.y, 0, 2) {
			focus.editor.press(focus.pressX, focus.pressY, 2, 0)
		}
		p.selected = true
		t.selecting = true
		p.target = 0
		t.holdUntil = time.Time{}
		rt.requestFrame()
	}
}
