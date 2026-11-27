package ui

import (
	"github.com/egoist/mygo/internal/scene"
	"github.com/egoist/mygo/internal/text"
	"github.com/egoist/mygo/transfer"
)

// Elements drag values to others within a window: Drag makes an element
// the source of a value, which a press moving a few DIPs starts dragging,
// with a copy of the element following the pointer; Drop and DragOver make
// an element take values of a type, the innermost under the pointer taking
// them. Escape gives up a drag. Lists and grids reorder their rows by
// dragging them (ListState.Reorder, GridState.Reorder), and lists drag
// them out for other elements to take (ListState.DragRows).

// dragStart is how far the pointer moves pressing a source before it
// drags it.
const dragStart = 4

// valueDrag is a value being dragged: the element dragging it, the value,
// where the pointer pressed the element, the element taking it under the
// pointer, and the source's element in the frame, for its copy. ghost
// is a copy of the source as it first showed, which follows the pointer
// once the frame builds the source no more, as a view it was in went.
type valueDrag struct {
	src      uint64
	value    any
	offX     float32
	offY     float32
	over     uint64
	elem     *node
	ghost    *node
	canceled bool
	native   bool
	// count is how many things the value stands for, as rows dragged
	// (countedDrag), for the badge beside the pointer.
	count int
	// label, when set (ListState.DragLabel), is what follows the pointer
	// in place of a copy of the element.
	label string
}

// Drag makes the element the source of value, dragged within the window
// once the pointer pressing the element moves a few DIPs: a copy of the
// element follows the pointer, and the element taking the value under it
// gets it when the pointer is released (Drop). A press dragging is no
// click. Dragging reports the drag.
func (e *node) Drag(value any) *node {
	e.flags |= flagDraggable | flagHover
	e.st.dragValue = value
	if d := e.c.rt.drag; d != nil && d.src == e.id {
		d.elem = e
		// The value as the frame has it, as of rows that moved.
		if !d.native {
			d.value = value
		}
	}
	return e
}

// dragFrom makes the element the source of a value that value returns as
// the drag starts, as the rows chosen in a list.
func (e *node) dragFrom(value func() any) *node {
	e.flags |= flagDraggable | flagHover
	e.st.dragFn = value
	if d := e.c.rt.drag; d != nil && d.src == e.id {
		d.elem = e
	}
	return e
}

// DragMissed reports, in the frame after, that the value the element
// drags (Drag) was let go over no element taking it, and where, in the
// window's coordinates: outside the window when it was let go past its
// edge, as to tear a tab out into a window of its own. A drag given up,
// with Escape, is not missed.
func (e *node) DragMissed() (Point, bool) {
	if !e.hasState() || !e.st.dragMissed {
		return Point{}, false
	}
	e.c.rt.consumed = true
	return Point{e.st.missX, e.st.missY}, true
}

// Dragging reports whether the element is dragging its value (Drag), or
// the pointer is dragging the element (Draggable).
func (e *node) Dragging() bool {
	if !e.hasState() {
		return false
	}
	if d := e.c.rt.drag; d != nil && d.src == e.id && !d.canceled {
		return true
	}
	return e.st.pressed && !e.disabled() && e.st.dragRec.dragged(e.c.rt, e.st)
}

// Drop makes e take values of type T dragged within the window (Element.Drag)
// and returns the value dropped on it since the last frame.
//
//	if f, ok := ui.Drop[*File](folder); ok {
//		app.move(f, dir)
//	}
func coreDrop[T any](e *node) (T, bool) {
	var zero T
	e.takes(func(v any) bool { _, ok := v.(T); return ok })
	if v, ok := e.st.droppedValue.(T); ok && e.st.hasDropped {
		e.c.rt.consumed = true
		return v, true
	}
	return zero, false
}

// DragOver makes e take values of type T dragged within the window, as
// Drop does, and returns the value dragged over it, for showing it would
// take it.
func coreDragOver[T any](e *node) (T, bool) {
	var zero T
	e.takes(func(v any) bool { _, ok := v.(T); return ok })
	if in := e.c.rt.incoming; in != nil && e.c.rt.dataOver == e.id {
		v, ok := in.Local.(T)
		return v, ok
	}
	d := e.c.rt.drag
	if d == nil || d.canceled || d.over != e.id {
		return zero, false
	}
	v, ok := d.value.(T)
	return v, ok
}

// takes makes e take the values accepts takes, as well as those it took
// already this pass: one element takes several types, as a folder takes
// files and a favorite moved among others.
func (e *node) takes(accepts func(any) bool) {
	e.flags |= flagValueDrop
	if prev := e.st.accepts; prev != nil {
		e.st.accepts = func(v any) bool { return prev(v) || accepts(v) }
		return
	}
	e.st.accepts = accepts
}

// valueTarget returns the innermost element at (x, y) taking value.
func (rt *engine) valueTarget(x, y float32, value any) uint64 {
	for _, id := range rt.hitChain(x, y) {
		if s := rt.states[id]; s != nil && s.flags&flagValueDrop != 0 && s.flags&flagDisabled == 0 && s.accepts != nil && s.accepts(value) {
			return id
		}
	}
	return 0
}

// dragMove starts dragging the source pressed once the pointer moved
// far enough, and follows the element under it.
func (rt *engine) dragMove(x, y float32) {
	s := rt.pressed
	if s == nil || rt.pressButton != 0 {
		return
	}
	var v countedDrag
	handed := false
	if !s.drags() {
		if s, v = rt.handPress(x, y); s == nil {
			return
		}
		handed = true
	}
	d := rt.drag
	if d == nil {
		px, py := s.world.Point(s.x+s.pressX, s.y+s.pressY)
		dx, dy := x-px, y-py
		if dx*dx+dy*dy < dragStart*dragStart {
			return
		}
		if !handed {
			v = s.valueToDrag()
		}
		if v.value == nil && s.dataSource == nil {
			return
		}
		d = &valueDrag{src: s.id, value: v.value, offX: s.pressX, offY: s.pressY, count: v.n, label: v.label}
		rt.drag = d
		if src := s.dataSource; src != nil {
			d.native = true
			options := src.options
			if options.Preview == nil {
				options.Preview, options.Hotspot = rt.dataPreview(s)
			}
			done := options.Done
			options.Done = func(result transfer.Result) {
				rt.drag = nil
				if p := rt.pressed; p != nil {
					p.pressed = false
					rt.pressed = nil
				}
				rt.clearDataOver()
				if !rt.closed {
					rt.requestFrame()
				}
				if done != nil {
					done(result)
				}
			}
			if err := rt.host.startDataDrag(src.data(), v.value, options, x*rt.zoomFactor(), y*rt.zoomFactor()); err != nil {
				options.Done(transfer.Result{Err: err})
			}
			return
		}
	}
	if d.native {
		return
	}
	if d.canceled {
		return
	}
	d.over = rt.valueTarget(x, y, d.value)
	rt.requestFrame()
}

// valueToDrag is the value a press on the element drags, with how many
// things it stands for and its label: a nil value for none, as for a row
// DragRows leaves out.
func (s *state) valueToDrag() countedDrag {
	value := s.dragValue
	if s.dragFn != nil {
		value = s.dragFn()
	}
	if c, ok := value.(countedDrag); ok {
		return c
	}
	return countedDrag{value: value}
}

// countedDrag is a value a source drags with how many things it stands
// for, as a list's rows or a grid's items, and what names them
// (ListState.DragLabel): the drag carries the value alone, and shows the
// number and the label.
type countedDrag struct {
	value any
	n     int
	label string
}

// drags reports whether the element drags a value (Drag, a list's rows,
// DragData).
func (s *state) drags() bool {
	return s.dragValue != nil || s.dragFn != nil || s.dataSource != nil
}

// pressDragger returns the element around target, the one pressed, that
// drags a value, as a list's row around a button, when target only takes
// clicks: one taking the pointer's moves (a text, a slider, a value of its
// own to drag) keeps them.
func pressDragger(rt *engine, chain []uint64, target *state) uint64 {
	if target.drags() || target.flags&(flagEditable|flagSelectable|flagDraggable|flagTrackPointer) != 0 {
		return 0
	}
	inside := false
	for _, id := range chain {
		s := rt.states[id]
		switch {
		case s == nil:
		case s == target:
			inside = true
		case inside && s.flags&flagDisabled == 0 && s.drags():
			return id
		}
	}
	return 0
}

// handPress gives the press to the element around the one pressed that
// drags a value, once the pointer moves as far as starts a drag: the
// element pressed has no click then. It returns that element with the
// value it drags, or nil. An element with nothing to drag from there, as
// a row DragRows leaves out, leaves the element pressed its press, and
// its click.
func (rt *engine) handPress(x, y float32) (*state, countedDrag) {
	s, a := rt.pressed, rt.states[rt.pressDragger]
	if a == nil || rt.drag != nil || !a.drags() {
		return nil, countedDrag{}
	}
	px, py := s.world.Point(s.x+s.pressX, s.y+s.pressY)
	if dx, dy := x-px, y-py; dx*dx+dy*dy < dragStart*dragStart {
		return nil, countedDrag{}
	}
	v := a.valueToDrag()
	if v.value == nil && a.dataSource == nil {
		rt.pressDragger = 0
		return nil, countedDrag{}
	}
	rt.pressed, rt.pressDragger = a, 0
	s.pressed = false
	a.pressed, a.pressButton, a.pressMods, a.pressPending = true, s.pressButton, s.pressMods, true
	a.pressX, a.pressY = a.local(px, py)
	a.drag.reset(rt.now(), px, py)
	return a, v
}

// retarget makes the element taking the value dragged under a pointer
// that did not move, as one the frame moved there or built anew in a view
// that just showed, the one the value is over, and builds the frame anew
// for it to show it.
func (rt *engine) retarget() {
	d := rt.drag
	if d == nil || d.native || d.canceled || !rt.pointerIn {
		return
	}
	if t := rt.valueTarget(rt.pointerX, rt.pointerY, d.value); t != d.over {
		d.over = t
		rt.askBuild()
	}
}

// dragEnd drops the value dragged on the element taking it under the
// pointer, and reports whether a drag ended, which is no click.
func (rt *engine) dragEnd() bool {
	d := rt.drag
	if d == nil {
		return false
	}
	if d.native {
		return true
	}
	rt.drag = nil
	if !d.canceled {
		if t := rt.states[rt.valueTarget(rt.pointerX, rt.pointerY, d.value)]; t != nil {
			t.droppedValue, t.hasDropped = d.value, true
			t.dropX, t.dropY = rt.pointerX, rt.pointerY
		} else if s := rt.states[d.src]; s != nil {
			s.dragMissed, s.missX, s.missY = true, rt.pointerX, rt.pointerY
		}
	}
	rt.requestFrame()
	return true
}

// dragCancel gives up the drag, as Escape does.
func (rt *engine) dragCancel() bool {
	if d := rt.drag; d != nil && !d.canceled {
		if d.native {
			rt.host.cancelDataDrag()
			return true
		}
		d.canceled, d.over = true, 0
		rt.requestFrame()
		return true
	}
	return false
}

// dragScroll scrolls the scroll container under the pointer while a value
// or native transfer is dragged near one of its edges, faster nearer to it.
func (rt *engine) dragScroll() {
	d := rt.drag
	// A native source may still be alive after leaving this window. Only
	// its destination events (incoming) tell us it is over this window.
	if rt.incoming == nil && (d == nil || d.canceled || d.native) {
		return
	}
	const edge = 32
	for _, id := range rt.hitChain(rt.pointerX, rt.pointerY) {
		s := rt.states[id]
		if s == nil || s.flags&flagScrollY == 0 {
			continue
		}
		var dy float32
		_, y := s.local(rt.pointerX, rt.pointerY)
		switch top, bottom := y, s.h-y; {
		case top < edge:
			dy = -(edge - top) / 2
		case bottom < edge:
			dy = (edge - bottom) / 2
		}
		if dy != 0 && scrollBy(s, 0, dy) {
			rt.animate()
			// The element under the pointer changes as the content moves.
			if d != nil && !d.native && !d.canceled {
				d.over = rt.valueTarget(rt.pointerX, rt.pointerY, d.value)
			}
		}
		return
	}
}

// paintDrag paints a copy of the element dragging its value under the
// pointer, above the rest, or the label of the rows a list drags
// (ListState.DragLabel) beside it.
func (rt *engine) paintDrag(p *Painter, w, h float32) {
	d := rt.drag
	if d == nil || d.canceled || d.native {
		return
	}
	e := d.elem
	if e == nil {
		e = d.ghost
	}
	if e == nil || e.c == nil {
		return
	}
	if d.label != "" {
		rt.paintDragLabel(p, e, d, w, h)
		return
	}
	px, py := e.world.Point(e.x+d.offX, e.y+d.offY)
	dx, dy := rt.pointerX-px, rt.pointerY-py
	affine := hasVisualTransform(e)
	if affine {
		shiftVisual(e, dx, dy)
	} else {
		shift(e, dx, dy)
	}
	saved, savedClip, savedTheme, dimmed, copying := p.opacity, p.clip, p.theme, e.opacitySet, p.copying
	// Painted alone, it takes the theme of the Themed around it. It is a
	// copy: the page of a web view in it stays where the element is, so the
	// hole the copy paints tells nothing of how opaque the page shows.
	p.opacity, p.clip, p.theme, e.opacitySet, p.copying = 0.75, Rect{0, 0, w, h}, e.theme(), false, true
	p.element(e)
	p.opacity = 1
	rt.paintDragCount(p, d, nil)
	p.opacity, p.clip, p.theme, e.opacitySet, p.copying = saved, savedClip, savedTheme, dimmed, copying
	if affine {
		restoreVisual(e)
	} else {
		shift(e, -dx, -dy)
	}
}

// draggingFrom reports whether the element id is the source of a value
// dragged within the window.
func (rt *engine) draggingFrom(id uint64) bool {
	d := rt.drag
	return d != nil && !d.native && d.src == id
}

// dragGhost copies the element e dragging its value, with its children,
// for its copy to follow the pointer once the frame builds e no more. The
// copy keeps the text e laid out, as it is laid out no more.
func dragGhost(e *node) *node {
	g := ghostOf(e)
	keepTextLayouts(g, e)
	return g
}

// keepTextLayouts gives the copy g of e, and those of its children, the
// text e and its children laid out.
func keepTextLayouts(g, e *node) {
	g.tl = e.tl
	c := g.first
	for ec := e.first; ec != nil && c != nil; ec = ec.next {
		if ec.leaving == 0 {
			keepTextLayouts(c, ec)
			c = c.next
		}
	}
}

func hasVisualTransform(e *node) bool {
	if e.world.Set {
		return true
	}
	for ch := e.first; ch != nil; ch = ch.next {
		if hasVisualTransform(ch) {
			return true
		}
	}
	return false
}

func restoreVisual(e *node) {
	e.world = e.st.world
	for ch := e.first; ch != nil; ch = ch.next {
		restoreVisual(ch)
	}
}

func shiftVisual(e *node, dx, dy float32) {
	e.world = scene.Translation(dx, dy).Mul(e.world)
	for ch := e.first; ch != nil; ch = ch.next {
		shiftVisual(ch, dx, dy)
	}
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// shift moves an element and those inside it by (dx, dy).
func shift(e *node, dx, dy float32) {
	e.x, e.y = e.x+dx, e.y+dy
	for i := range e.frags {
		e.frags[i].X += dx
		e.frags[i].Y += dy
	}
	for ch := e.first; ch != nil; ch = ch.next {
		shift(ch, dx, dy)
	}
}

// paintDragCount paints how many go with what is dragged, as several
// rows: beside the pointer, as the copy may be wider than the window, or
// with a label (ListState.DragLabel) on its corner, at.
func (rt *engine) paintDragCount(p *Painter, d *valueDrag, at *Rect) {
	n := d.count
	if n <= 1 {
		return
	}
	t := p.Theme()
	label := Span{Text: itoa(n), Size: t.FontSize - 2, Weight: 600, Color: t.AccentText}
	tw, th := p.MeasureText(0, label)
	bw, bh := max(tw+10, th+4), th+4
	x, y := rt.pointerX+10, rt.pointerY-th-14
	if at != nil {
		x, y = max(at.X+at.W-bw/2-2, 0), max(at.Y-bh/2, 0)
	}
	p.Fill(Rect{x, y, bw, bh}, t.Accent, bh/2)
	p.RichText(x+(bw-tw)/2, y+2, 0, label)
}

// paintDragLabel paints the label of rows a list drags out
// (ListState.DragLabel) below and to the right of the pointer, clear of
// its arrow, so that it hides neither the row under the pointer nor its
// name: on one line, as wide as a tooltip at most. Near the window's
// right or bottom edge, w by h, it goes to the other side of the pointer,
// as a tooltip does, rather than over it. Several rows show their number
// on its corner.
func (rt *engine) paintDragLabel(p *Painter, e *node, d *valueDrag, w, h float32) {
	saved := p.theme
	p.theme = e.theme()
	t := p.Theme()
	padX, padY, radius := t.Space(2), t.Space(1.25), t.Space(1.25)
	// No wider than the window either, and aligned to the left of its
	// box, which is as wide as the text: right to left text would align
	// to the right of the width it wraps at, outside the box.
	m := rt.richSpans(t, max(min(t.Space(80), w-2*padX), t.Space(10)), []Span{{Text: d.label, Size: t.FontSize - 1}})
	params := m.p
	params.MaxLines, params.Align = 1, text.Left
	l := rt.text.Layout(params)
	r := Rect{rt.pointerX + 14, rt.pointerY + 10, l.Width + 2*padX, l.Height + 2*padY}
	if r.X+r.W > w {
		r.X = rt.pointerX - 14 - r.W
	}
	if r.Y+r.H > h {
		r.Y = rt.pointerY - 10 - r.H
	}
	r.X, r.Y = max(r.X, 0), max(r.Y, 0)
	p.Shadow(r, radius, 0, 2, 8, 0, RGBA(0, 0, 0, 0.2))
	p.Fill(r, t.Surface, radius)
	p.Stroke(r, t.Border, radius, 1)
	p.textLayout(l, r.X+padX, r.Y+padY, t.Text, textStyle{size: t.FontSize}, m.paint)
	rt.paintDragCount(p, d, &r)
	p.theme = saved
}
