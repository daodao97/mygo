package scene

import (
	"bytes"
	"testing"
)

// ownedBitmap returns the w×h bitmap a glyph of s shows, from its atlas.
func ownedBitmap(s *Scene, g Glyph) []byte {
	a := s.MaskAtlas
	if g.Colored || g.Subpixel {
		a = s.ColorAtlas
	}
	var pix []byte
	for y := range int(g.VH) {
		i := ((int(g.V)+y)*a.W + int(g.U)) * a.BPP
		pix = append(pix, a.Pix[i:][:int(g.UW)*a.BPP]...)
	}
	return pix
}

// TestOwn checks that a scene given atlases of its own draws the bitmaps
// its glyphs showed, though the atlases it was painted with then hold
// others in their places, from atlases holding nothing else.
func TestOwn(t *testing.T) {
	mask, color := NewAtlas(1, 64, 64), NewAtlas(4, 64, 64)
	put := func(a *Atlas, w, h int, fill byte) (uint16, uint16) {
		x, y, ok := a.Alloc(w, h)
		if !ok {
			t.Fatal("no room")
		}
		a.Put(x, y, w, h, bytes.Repeat([]byte{fill}, w*h*a.BPP), w*a.BPP)
		return uint16(x), uint16(y)
	}
	// Others first, so that the scene's are not at the atlases' corners.
	put(mask, 20, 20, 1)
	put(color, 20, 20, 2)
	au, av := put(mask, 9, 11, 30)
	bu, bv := put(mask, 30, 5, 60)
	cu, cv := put(color, 7, 7, 90)
	s := &Scene{MaskAtlas: mask, ColorAtlas: color}
	s.Glyphs = []Glyph{
		{U: au, V: av, UW: 9, VH: 11},
		{U: bu, V: bv, UW: 30, VH: 5},
		{U: cu, V: cv, UW: 7, VH: 7, Colored: true},
		{U: au, V: av, UW: 9, VH: 11}, // the first again
		{},                            // a glyph without pixels
	}
	var want [][]byte
	for _, g := range s.Glyphs {
		want = append(want, ownedBitmap(s, g))
	}
	var o Owned
	if !s.Own(&o) {
		t.Fatal("the bitmaps did not fit atlases of their own")
	}
	if s.MaskAtlas != o.Mask || s.ColorAtlas != o.Color || o.Mask == mask || o.Color == color {
		t.Fatal("the scene draws from the atlases it was painted with")
	}
	// As another frame making room in them leaves them.
	mask.Reset()
	color.Reset()
	for i, g := range s.Glyphs {
		if got := ownedBitmap(s, g); !bytes.Equal(got, want[i]) {
			t.Errorf("glyph %d shows another bitmap from the scene's own atlas", i)
		}
	}
	if s.Glyphs[0].U != s.Glyphs[3].U || s.Glyphs[0].V != s.Glyphs[3].V {
		t.Error("a bitmap two glyphs show was copied twice")
	}
	// No larger than they need: 31×6 and 10×12 padded, in 32 columns.
	if o.Mask.W != 32 || o.Mask.H > 24 || o.Color.W != 16 || o.Color.H > 16 {
		t.Errorf("atlases of %d×%d and %d×%d", o.Mask.W, o.Mask.H, o.Color.W, o.Color.H)
	}

	// Painted again with other glyphs of those sizes, the same atlas holds
	// those, as another generation of it.
	gen := o.Mask.Generation()
	s.MaskAtlas, s.ColorAtlas = mask, color
	du, dv := put(mask, 9, 11, 120)
	eu, ev := put(mask, 30, 5, 150)
	s.Glyphs = []Glyph{{U: du, V: dv, UW: 9, VH: 11}, {U: eu, V: ev, UW: 30, VH: 5}}
	kept := o.Mask
	if !s.Own(&o) || s.MaskAtlas != kept || kept.Generation() == gen {
		t.Fatalf("drawn again: the atlas reused %v, at generation %d after %d", s.MaskAtlas == kept, kept.Generation(), gen)
	}
	if s.ColorAtlas != color {
		t.Error("a scene without color glyphs took a color atlas of its own")
	}
	if got := ownedBitmap(s, s.Glyphs[1]); !bytes.Equal(got, bytes.Repeat([]byte{150}, 150)) {
		t.Error("drawn again, it shows another bitmap")
	}
}

// TestOwnTooLarge checks that a scene whose bitmaps fit no atlas keeps the
// atlases it was painted with, its glyphs where they were.
func TestOwnTooLarge(t *testing.T) {
	mask := &Atlas{BPP: 1, W: 2 * MaxAtlasSize, H: 4, Pix: make([]byte, 2*MaxAtlasSize*4)}
	s := &Scene{MaskAtlas: mask, Glyphs: []Glyph{{U: 3, V: 1, UW: MaxAtlasSize, VH: 2}}}
	var o Owned
	if s.Own(&o) || s.MaskAtlas != mask || s.Glyphs[0].U != 3 || s.Glyphs[0].V != 1 {
		t.Errorf("a bitmap wider than the largest atlas: owned, glyph at %d,%d", s.Glyphs[0].U, s.Glyphs[0].V)
	}
}

// TestOwnStrayGlyph checks that a glyph beyond the atlas it names shows
// nothing, while the others keep their bitmaps.
func TestOwnStrayGlyph(t *testing.T) {
	mask := NewAtlas(1, 16, 16)
	mask.Put(2, 3, 4, 4, bytes.Repeat([]byte{7}, 16), 4)
	s := &Scene{MaskAtlas: mask, Glyphs: []Glyph{{U: 10, V: 10, UW: 8, VH: 8}, {U: 2, V: 3, UW: 4, VH: 4}, {U: 1, V: 1, UW: 2, VH: 2, Colored: true}}}
	var o Owned
	if !s.Own(&o) || s.MaskAtlas != o.Mask {
		t.Fatal("a glyph beyond its atlas kept the others from atlases of their own")
	}
	if g := s.Glyphs[0]; g.UW != 0 || g.VH != 0 {
		t.Errorf("the glyph beyond its atlas shows %d×%d pixels", g.UW, g.VH)
	}
	if got := ownedBitmap(s, s.Glyphs[1]); !bytes.Equal(got, bytes.Repeat([]byte{7}, 16)) {
		t.Error("the glyph within its atlas shows another bitmap")
	}
	if g := s.Glyphs[2]; g.UW != 0 || s.ColorAtlas != nil {
		t.Error("a glyph of an atlas the scene has not shows pixels")
	}
}

// TestOwnAgain checks that a scene painted again and again keeps its atlas
// while its bitmaps fit it and take a quarter of it or more, and takes
// another, with room to grow, when they do not.
func TestOwnAgain(t *testing.T) {
	mask := NewAtlas(1, 256, 256)
	var all []Glyph
	for i := range 40 {
		x, y, ok := mask.Alloc(10, 10)
		if !ok {
			t.Fatal("no room")
		}
		mask.Put(x, y, 10, 10, bytes.Repeat([]byte{byte(i + 1)}, 100), 10)
		all = append(all, Glyph{U: uint16(x), V: uint16(y), UW: 10, VH: 10})
	}
	s := &Scene{}
	var o Owned
	own := func(n int) *Atlas {
		t.Helper()
		s.MaskAtlas, s.Glyphs = mask, append(s.Glyphs[:0], all[:n]...)
		if !s.Own(&o) {
			t.Fatalf("%d bitmaps did not fit an atlas", n)
		}
		for i, g := range s.Glyphs {
			if got := ownedBitmap(s, g); !bytes.Equal(got, bytes.Repeat([]byte{byte(i + 1)}, 100)) {
				t.Fatalf("of %d bitmaps, glyph %d shows another", n, i)
			}
		}
		return s.MaskAtlas
	}
	first := own(10)
	if first.H&(first.H-1) == 0 {
		t.Errorf("painted once, an atlas %d high: with room it may never use", first.H)
	}
	if a := own(8); a != first {
		t.Error("fewer bitmaps, which fit the atlas, took another")
	}
	grown := own(40)
	if grown == first || grown.H&(grown.H-1) != 0 {
		t.Errorf("more bitmaps than fit: the same atlas %v, now %d×%d", grown == first, grown.W, grown.H)
	}
	if a := own(30); a != grown {
		t.Error("fewer bitmaps, most of the atlas, took another")
	}
	gen := grown.Generation()
	if a := own(30); a != grown || a.Generation() == gen {
		t.Error("painted again, the atlas is not another generation of the same")
	}
	if a := own(2); a == grown || a.W*a.H >= grown.W*grown.H {
		t.Errorf("two bitmaps keep an atlas of %d×%d", a.W, a.H)
	}
}
