package scene

import (
	"cmp"
	"image"
	"math/bits"
	"slices"
)

// Owned are the atlases of a scene that keeps the bitmaps of its glyphs
// (Scene.Own), and what making them again reuses.
type Owned struct {
	// Mask and Color are the scene's atlases, each nil until the scene
	// draws a glyph from one.
	Mask, Color *Atlas

	at      map[ownedRect]image.Point
	rects   [2][]ownedRect
	shelves []shelf
}

// ownedRect is a rectangle of an atlas a glyph shows: of the color atlas,
// or of the mask atlas.
type ownedRect struct {
	color      bool
	u, v, w, h uint16
}

// Own has s draw its glyphs from atlases of its own, o's, holding the
// bitmaps they show and no other, copied from the atlases it was painted
// with. Those are every window's: what a frame does not draw moves or
// goes as one makes room in them (text.System.MakeRoom), and the masks a
// frame drew for itself alone go as the next begins. A scene drawn after
// the frame that painted it, as a canvas's by a renderer that has not its
// texture, then draws what it was painted with.
//
// An atlas none of the glyphs shows stays the scene's as it is, and a
// glyph showing what its atlas does not hold shows nothing, rather than
// what the scene's own holds there. A scene painted again keeps the atlas
// it has while its bitmaps fit it and take a quarter of its height or
// more: one drawn every frame makes none in most.
//
// It reports false, leaving s as it is, when the bitmaps of an atlas do
// not fit one of the largest size.
func (s *Scene) Own(o *Owned) bool {
	if o.at == nil {
		o.at = map[ownedRect]image.Point{}
	}
	clear(o.at)
	src := [2]*Atlas{s.MaskAtlas, s.ColorAtlas}
	rects := [2][]ownedRect{o.rects[0][:0], o.rects[1][:0]}
	for i := range s.Glyphs {
		k, n := ownedOf(&s.Glyphs[i])
		if !ownedIn(k, src[n]) {
			continue
		}
		if _, ok := o.at[k]; ok {
			continue
		}
		o.at[k] = image.Point{}
		rects[n] = append(rects[n], k)
	}
	o.rects = rects
	own := [2]**Atlas{&o.Mask, &o.Color}
	var sizes [2]image.Point
	for n, rs := range rects {
		if len(rs) == 0 {
			continue
		}
		// Tallest first, as Repack: shelves waste the least.
		slices.SortFunc(rs, func(a, b ownedRect) int {
			return cmp.Or(cmp.Compare(b.h, a.h), cmp.Compare(b.w, a.w), cmp.Compare(a.v, b.v), cmp.Compare(a.u, b.u))
		})
		a := *own[n]
		if a != nil && a.BPP == src[n].BPP {
			if h, ok := o.height(rs, a.W); ok && h <= a.H && 4*h >= a.H {
				sizes[n] = image.Pt(a.W, a.H)
				continue
			}
		}
		size, ok := o.size(rs)
		if !ok {
			return false
		}
		if a != nil {
			// Painted again, it may be painted again: with room for a
			// little more than it draws now.
			size.Y = min(1<<bits.Len(uint(size.Y-1)), MaxAtlasSize)
		}
		sizes[n] = size
	}
	for n, rs := range rects {
		if len(rs) == 0 {
			continue
		}
		from, a := src[n], *own[n]
		if a == nil || a.BPP != from.BPP || a.W != sizes[n].X || a.H != sizes[n].Y {
			a = NewAtlas(from.BPP, sizes[n].X, sizes[n].Y)
			*own[n] = a
		} else {
			a.Reset()
		}
		for _, k := range rs {
			x, y, _ := a.Alloc(int(k.w), int(k.h))
			a.Put(x, y, int(k.w), int(k.h), from.Pix[(int(k.v)*from.W+int(k.u))*from.BPP:], from.W*from.BPP)
			o.at[k] = image.Pt(x, y)
		}
	}
	for i := range s.Glyphs {
		g := &s.Glyphs[i]
		k, _ := ownedOf(g)
		if at, ok := o.at[k]; ok {
			g.U, g.V = uint16(at.X), uint16(at.Y)
		} else {
			g.U, g.V, g.UW, g.VH = 0, 0, 0, 0
		}
	}
	if len(rects[0]) > 0 {
		s.MaskAtlas = o.Mask
	}
	if len(rects[1]) > 0 {
		s.ColorAtlas = o.Color
	}
	return true
}

// ownedOf returns the rectangle a glyph shows, and the index of its atlas:
// 0 for the mask atlas, 1 for the color atlas.
func ownedOf(g *Glyph) (ownedRect, int) {
	k := ownedRect{color: g.Colored || g.Subpixel, u: g.U, v: g.V, w: g.UW, h: g.VH}
	if k.color {
		return k, 1
	}
	return k, 0
}

// ownedIn reports whether k is a rectangle of a, with pixels.
func ownedIn(k ownedRect, a *Atlas) bool {
	return a != nil && k.w > 0 && k.h > 0 && int(k.u)+int(k.w) <= a.W && int(k.v)+int(k.h) <= a.H
}

// size returns the size of the smallest atlas, of a width that is a power
// of two, about as wide as high, that rects fit allocated in their order,
// or false when none does.
func (o *Owned) size(rects []ownedRect) (image.Point, bool) {
	area, widest := 0, 0
	for _, k := range rects {
		// As Alloc pads them.
		area += (int(k.w) + 1) * (int(k.h) + 1)
		widest = max(widest, int(k.w)+1)
	}
	w := 16
	for w < widest || w*w < area {
		w *= 2
	}
	for ; w <= MaxAtlasSize; w *= 2 {
		// Only as high as its shelves: a wide mask alone takes its own
		// size, not a square of its width.
		if h, ok := o.height(rects, w); ok {
			return image.Pt(w, h), true
		}
	}
	return image.Point{}, false
}

// height returns how high an atlas w wide is with rects allocated in
// their order, or false when they do not fit the highest. One that high
// or higher gives them the same places.
func (o *Owned) height(rects []ownedRect, w int) (int, bool) {
	plan := Atlas{W: w, H: MaxAtlasSize, top: MaxAtlasSize, shelves: o.shelves[:0]}
	defer func() { o.shelves = plan.shelves }()
	for _, k := range rects {
		if _, _, ok := plan.Alloc(int(k.w), int(k.h)); !ok {
			return 0, false
		}
	}
	return plan.bottom, true
}
