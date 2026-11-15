package ui

import (
	"encoding/binary"
	"image"
	"log"

	"github.com/egoist/mygo/internal/raster"
	"github.com/egoist/mygo/internal/scene"
)

// Paths the mask atlas has no room for. The atlas holds at most
// scene.MaxAtlasSize² pixels of masks a frame, and the masks of many large
// paths that change every frame, as the links of a node editor under a
// zoom that changes, can take many times that: a mask, or its tiles,
// covers a path's bounds, mostly empty for a thin curve. Making room and
// painting the frame again cannot help them, and they used to be left
// out of the frame, whole or tile by tile (#1841).
//
// Such a path spills: its masks are drawn on the CPU (raster.Layer) into
// a layer of the frame, a bitmap that the scene shows where the path
// would have drawn (an OpImage). The paths after it draw into the layer
// too, as long as the scene adds nothing else after it, so that they keep
// their order; the masks go through an atlas of the layer's own, drawn
// into the layer and reused each time it fills, so that a layer holds any
// number of paths. Its bitmap is premultiplied and composited over what
// the scene drew under it, which draws what drawing each path there
// would, up to the rounding of the layer's 8 bits a channel: each path
// with its paint, clipped as the scene clips the layer, transformed as
// the path was (in the layer's ops). Wide-gamut colors draw as their
// nearest sRGB ones there, as the CPU renderer draws them.
//
// A path spills where the atlas, at its largest, has no room for a mask of
// it: in the frame's last paint (engine.makeRoom), which no paint makes
// room for again, and in the paints after one that spilled
// (pathCrowdedPaints), without asking for room the frame would paint
// again for (System.TryMask). The fast path, a mask in the atlas drawn by
// the GPU, is unchanged for the paths that fit; the first spill is
// logged once, and so is a path that still cannot draw (pathLost).

// pathSpills is the engine's layers of spilled paths, and when paths
// spill.
type pathSpills struct {
	layers []*pathSpill
	// epoch counts the paints of the window, whole or in parts, and run
	// changes within one where a layer must end, as a part of it starts
	// or ends (Painter.draw).
	epoch, run uint64
	// last tells that the paint is the frame's last (engine.makeRoom), and
	// until is the last paint (engine.paints) in which masks spill at
	// once, without asking for room.
	last  bool
	until uint64
	// opened counts the layers opened in the window's scene, and inParts
	// those its parts painted again by themselves opened (paintPart),
	// which the rest of the scene must open none beside.
	opened, inParts uint64
	// logged and lost tell that spilling and a path not drawn were
	// logged.
	logged, lost bool
}

// pathSpill is a layer of spilled paths in a scene.
type pathSpill struct {
	// s is the scene it is in, op the index of its OpImage in s.Ops, or -1
	// until the path that opened it adds it, and img the bitmap it shows.
	s   *scene.Scene
	op  int
	img *scene.Image
	// epoch and run are those it was opened in, and open tells that the
	// paint has not finished it (finishSpills).
	epoch, run uint64
	open       bool
	// dst is what it drew, the scene's size, with draw, and mini the masks
	// yet to draw into it, of atlas, within pend; dirty is what of dst it
	// drew into.
	dst         raster.Image
	draw        raster.Layer
	mini        scene.Scene
	atlas       *scene.Atlas
	pend, dirty image.Rectangle
}

// spillPath is what the path being drawn spills into: layer, from glyph
// start of its masks there, which the path draws into whole if into, in
// the paint of op and with the color of tint.
type spillPath struct {
	layer *pathSpill
	into  bool
	start int32
	op    scene.Op
	tint  scene.Glyph
}

// spillEveryPath has every path spill, for tests that compare spilled
// paths with paths drawn from the atlas.
var spillEveryPath = false

// spillAtlas is the side of a layer's atlas, as first made: it grows,
// once it filled, to the largest.
const spillAtlas = 1024

// spills reports whether a mask the mask atlas has no room for spills,
// and whether it does without asking for room first (quiet): in the
// frame's last paint, and, with the atlas at its largest, which room
// made would not grow, in the paints after one that spilled.
func (p *Painter) spills() (spill, quiet bool) {
	sp := &p.rt.paths.spill
	if a := p.rt.text.MaskAtlas; a.W < scene.MaxAtlasSize || a.H < scene.MaxAtlasSize {
		return sp.last, false
	}
	quiet = sp.until >= p.rt.paints
	return quiet || sp.last, quiet
}

// spillOnTop returns the layer of spilled paths the painter's scene draws
// last, after all else, if any: the path drawn next goes into it.
func (p *Painter) spillOnTop() *pathSpill {
	sp := &p.rt.paths.spill
	n := len(p.s.Ops)
	if len(sp.layers) == 0 || n == 0 || p.s.Ops[n-1].Kind != scene.OpImage {
		return nil
	}
	for _, l := range sp.layers {
		if l.open && l.s == p.s && l.run == sp.run && l.op == n-1 && p.s.Ops[n-1].Image == l.img {
			return l
		}
	}
	return nil
}

// spillAll has the path being drawn draw into a layer of spilled paths
// whole, opening one, and reports whether it can.
func (p *Painter) spillAll() bool {
	cur := &p.rt.paths.cur
	if cur.layer == nil {
		if cur.layer = p.openSpill(); cur.layer == nil {
			return false
		}
	}
	cur.into = true
	return true
}

// openSpill opens a layer of spilled paths in the painter's scene, which
// the path being drawn adds once drawn (endSpill), or returns nil for a
// scene of no pixels.
func (p *Painter) openSpill() *pathSpill {
	rt := p.rt
	sp := &rt.paths.spill
	s := p.s
	if s.Width <= 0 || s.Height <= 0 {
		return nil
	}
	var l *pathSpill
	for _, c := range sp.layers {
		// A layer of an earlier paint of the window is free: the window's
		// scene was painted anew, or in parts, which the rest of the scene
		// opened no layer beside (engine.paint). One of a canvas or an
		// effect's layer is, once that was drawn anew without it.
		if c.epoch != sp.epoch && (c.s == &rt.scene || !c.shown()) {
			l = c
			break
		}
	}
	if l == nil {
		l = &pathSpill{img: scene.NewImageRGBA(1, 1, make([]byte, 4)), atlas: scene.NewAtlas(1, spillAtlas, spillAtlas)}
		sp.layers = append(sp.layers, l)
	}
	l.reset(s)
	l.op, l.epoch, l.run, l.open = -1, sp.epoch, sp.run, true
	if s == &rt.scene {
		sp.opened++
	}
	if sp.last {
		sp.until = rt.paints + pathCrowdedPaints
	}
	if !sp.logged {
		sp.logged = true
		msg := "mygo: drawing paths on the CPU: the mask atlas has no room for their masks, as for many large paths that change every frame; " +
			"opaque StrokePolyline lines draw without masks on a GPU"
		log.Print(msg)
		rt.warn(msg)
	}
	return l
}

// shown reports whether the layer's scene still shows it.
func (l *pathSpill) shown() bool {
	return l.s != nil && l.op >= 0 && l.op < len(l.s.Ops) && l.s.Ops[l.op].Image == l.img
}

// reset empties the layer, for scene s.
func (l *pathSpill) reset(s *scene.Scene) {
	l.s = s
	if w, h := s.Width, s.Height; l.dst.W != w || l.dst.H != h {
		l.dst.Resize(w, h)
		clear(l.dst.Pix)
	} else if !l.dirty.Empty() {
		for y := l.dirty.Min.Y; y < l.dirty.Max.Y; y++ {
			clear(l.dst.Pix[y*l.dst.Stride+4*l.dirty.Min.X : y*l.dst.Stride+4*l.dirty.Max.X])
		}
	}
	l.pend, l.dirty = image.Rectangle{}, image.Rectangle{}
	l.mini.Reset(s.Width, s.Height, scene.Color{})
	l.mini.Scale, l.mini.Zoom, l.mini.Text, l.mini.MaskAtlas = s.Scale, s.Zoom, s.Text, l.atlas
	l.atlas.ResetTransient()
}

// spillMask draws the mask of the path job into the layer the path being
// drawn spills into, opening one, from the pixels the job rasterized
// just now if have, and returns maskSpilled, or maskMissed if it cannot.
func (p *Painter) spillMask(have bool) int {
	rt := p.rt
	cur := &rt.paths.cur
	if cur.layer == nil {
		if cur.layer = p.openSpill(); cur.layer == nil {
			return maskMissed
		}
	}
	l, j := cur.layer, &rt.paths.job
	w, h, pix := j.w, j.h, rt.paths.pix
	if have && len(pix) >= w*h {
		pix = pix[:w*h]
	} else {
		w, h, pix = rt.rasterizeJob()
	}
	if j.full || p.transform.Set || w*h <= 4*spillCell*spillCell {
		// Under a transform, cells would show where they meet, as tiles
		// do.
		if !p.spillPut(l, pix, w, j.x0, j.y0, w, h) {
			return maskMissed
		}
		return maskSpilled
	}
	// The CPU draws each pixel of a mask, which, for a long thin stroke,
	// is mostly empty: it draws the cells of it that are not.
	for cy := 0; cy < h; cy += spillCell {
		ch := min(spillCell, h-cy)
		for cx := 0; cx < w; cx += spillCell {
			cw := min(spillCell, w-cx)
			empty := true
			for y := cy; y < cy+ch && empty; y++ {
				empty = zeros(pix[y*w+cx : y*w+cx+cw])
			}
			if !empty && !p.spillPut(l, pix[cy*w+cx:], w, j.x0+float32(cx), j.y0+float32(cy), cw, ch) {
				return maskMissed
			}
		}
	}
	return maskSpilled
}

// zeros reports whether b holds zeros alone.
func zeros(b []byte) bool {
	for len(b) >= 8 {
		if binary.LittleEndian.Uint64(b) != 0 {
			return false
		}
		b = b[8:]
	}
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

// spillTile is the side of the tiles of a path that draws into a layer
// whole (spillPath.into), in pixels, which pathTiles draws instead of one
// mask of all of it once it is larger: of the tiles that show, those
// that the path reaches.
var spillTile = 128

// spillCell is the side of the cells of a spilled mask, in pixels, that
// are drawn only if they are not empty.
const spillCell = 32

// spillPut puts the w×h mask pix, of rows of stride bytes, at (x, y) in
// device pixels, into layer l, for the path being drawn, and reports
// whether it could.
func (p *Painter) spillPut(l *pathSpill, pix []byte, stride int, x, y float32, w, h int) bool {
	a := l.atlas
	ax, ay, ok := a.AllocTransient(w, h)
	if !ok {
		// The layer's atlas is full: draw what it holds into the layer,
		// and reuse it, larger.
		p.flushSpill(l)
		if a.W < scene.MaxAtlasSize {
			a.Grow()
		}
		for (w+1 > a.W || h+1 > a.H) && a.Grow() {
		}
		if ax, ay, ok = a.AllocTransient(w, h); !ok {
			p.rt.pathLost("its mask is larger than the largest atlas")
			return false
		}
	}
	a.Put(ax, ay, w, h, pix, stride)
	g := p.rt.paths.cur.tint
	g.X, g.Y, g.W, g.H = x, y, float32(w), float32(h)
	g.U, g.V, g.UW, g.VH = uint16(ax), uint16(ay), uint16(w), uint16(h)
	l.mini.Glyphs = append(l.mini.Glyphs, g)
	return true
}

// closeSpillOp adds to the layer the path being drawn spills into the op
// of the masks it put there since the last, in the path's paint and
// transform.
func (p *Painter) closeSpillOp() {
	cur := &p.rt.paths.cur
	l := cur.layer
	if l == nil || int(cur.start) >= len(l.mini.Glyphs) {
		return
	}
	op := cur.op
	op.Start, op.End, op.Wide = cur.start, int32(len(l.mini.Glyphs)), 0
	if op.Stops[1] > op.Stops[0] {
		n := int32(len(l.mini.Stops))
		l.mini.Stops = append(l.mini.Stops, p.s.Stops[op.Stops[0]:op.Stops[1]]...)
		op.Stops = [2]int32{n, int32(len(l.mini.Stops))}
	}
	if p.transform.Set {
		op.Transform = p.transform.Pixels(p.scale).Mul(op.Transform)
	}
	l.mini.Ops = append(l.mini.Ops, op)
	for _, g := range l.mini.Glyphs[op.Start:op.End] {
		b := op.Transform.Bounds(scene.Rect{X: g.X, Y: g.Y, W: g.W, H: g.H})
		r := image.Rect(int(b.X)-2, int(b.Y)-2, int(b.X+b.W)+2, int(b.Y+b.H)+2)
		l.pend = l.pend.Union(r.Intersect(image.Rect(0, 0, l.dst.W, l.dst.H)))
	}
	cur.start = op.End
}

// flushSpill draws the masks layer l holds into it, with the path being
// drawn's so far, and empties its atlas.
func (p *Painter) flushSpill(l *pathSpill) {
	cur := &p.rt.paths.cur
	if cur.layer == l {
		p.closeSpillOp()
		cur.start = 0
	}
	l.flush()
}

// flush draws the masks the layer holds into it, and empties its atlas.
func (l *pathSpill) flush() {
	if len(l.mini.Ops) > 0 && !l.pend.Empty() {
		l.draw.Over(&l.dst, &l.mini, l.pend)
		l.dirty = l.dirty.Union(l.pend)
	}
	l.mini.Ops, l.mini.Glyphs, l.mini.Stops = l.mini.Ops[:0], l.mini.Glyphs[:0], l.mini.Stops[:0]
	l.pend = image.Rectangle{}
	l.atlas.ResetTransient()
}

// endSpill ends the path being drawn's spilling: its masks in a layer go
// in an op of their own there, and a layer it opened, after what it drew
// from the atlas, in the scene.
func (p *Painter) endSpill() {
	cur := &p.rt.paths.cur
	if l := cur.layer; l != nil {
		p.closeSpillOp()
		if l.op < 0 {
			l.op = len(p.s.Ops)
			p.s.Ops = append(p.s.Ops, scene.Op{Kind: scene.OpImage, Image: l.img})
		}
	}
	*cur = spillPath{}
}

// beginSpills starts a paint of the window, whole or in parts, in which
// paths spill into layers of their own.
func (sp *pathSpills) beginPaint() {
	sp.epoch++
	sp.run++
}

// finishSpills draws what the layers the paint opened hold into their
// bitmaps, which their ops then show, once the paint has painted all.
func (rt *engine) finishSpills() {
	sp := &rt.paths.spill
	keep := sp.layers[:0]
	for _, l := range sp.layers {
		if l.open {
			l.open = false
			l.flush()
			l.show()
		}
		// Drop the layers no paint opened for a while, unless a canvas
		// still shows one.
		if sp.epoch-l.epoch < 2*pathCrowdedPaints || l.s != &rt.scene && l.shown() {
			keep = append(keep, l)
		}
	}
	clear(sp.layers[len(keep):])
	sp.layers = keep
}

// show has the layer's bitmap hold what it drew, and its op draw it there.
func (l *pathSpill) show() {
	i := l.op
	if !l.shown() {
		// The window's parts painted again went in place of what they
		// painted before (engine.repaintParts).
		i = -1
		for k := range l.s.Ops {
			if l.s.Ops[k].Kind == scene.OpImage && l.s.Ops[k].Image == l.img {
				i = k
				break
			}
		}
		if i < 0 {
			return
		}
		l.op = i
	}
	op := &l.s.Ops[i]
	// The bitmap is what the layer drew into, in steps of 64 pixels, so
	// that its texture changes size less often as that changes.
	r := l.dirty
	r = image.Rect(r.Min.X&^63, r.Min.Y&^63, min((r.Max.X+63)&^63, l.dst.W), min((r.Max.Y+63)&^63, l.dst.H))
	if r.Empty() {
		op.Rect, op.Src = scene.Rect{}, scene.Rect{}
		return
	}
	l.img.Resize(r.Dx(), r.Dy())
	pix := l.img.Pix
	for y := r.Min.Y; y < r.Max.Y; y++ {
		src := l.dst.Pix[y*l.dst.Stride+4*r.Min.X : y*l.dst.Stride+4*r.Max.X]
		dst := pix[(y-r.Min.Y)*4*r.Dx():][:len(src)]
		for x := 0; x < len(src); x += 4 {
			dst[x], dst[x+1], dst[x+2], dst[x+3] = src[x+2], src[x+1], src[x], src[x+3]
		}
	}
	op.Rect = scene.Rect{X: float32(r.Min.X), Y: float32(r.Min.Y), W: float32(r.Dx()), H: float32(r.Dy())}
	op.Src = scene.Rect{W: float32(r.Dx()), H: float32(r.Dy())}
}

// pathLost logs, once, that a path could not be drawn, and why.
func (rt *engine) pathLost(why string) {
	sp := &rt.paths.spill
	if sp.lost {
		return
	}
	sp.lost = true
	msg := "mygo: a path was not drawn: " + why
	log.Print(msg)
	rt.warn(msg)
}
