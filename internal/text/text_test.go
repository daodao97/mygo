package text

import (
	"image"
	"math"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf16"

	"github.com/egoist/mygo/internal/scene"
	"github.com/egoist/mygo/internal/wait"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/gomediumitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

func TestLayoutWraps(t *testing.T) {
	s := Shared()
	start := time.Now()
	s.Preload()
	t.Logf("system fonts loaded in %v", time.Since(start))

	one := s.Layout(Params{Text: "Hello, world", Style: Style{Size: 16}})
	if len(one.Lines) != 1 || one.Width <= 50 || one.Height <= 16 {
		t.Fatalf("single line: %d lines, %vx%v", len(one.Lines), one.Width, one.Height)
	}
	long := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 6)
	wrapped := s.Layout(Params{Text: long, Style: Style{Size: 16}, Width: 200})
	if len(wrapped.Lines) < 4 {
		t.Fatalf("wrapped to %d lines", len(wrapped.Lines))
	}
	for i, line := range wrapped.Lines {
		if line.Width > 200.5 {
			t.Errorf("line %d is %v wide", i, line.Width)
		}
		if i > 0 && line.Start != wrapped.Lines[i-1].End {
			t.Errorf("line %d starts at %d, the previous ended at %d", i, line.Start, wrapped.Lines[i-1].End)
		}
	}
	if last := wrapped.Lines[len(wrapped.Lines)-1]; last.End != len(wrapped.Runes) {
		t.Errorf("last line ends at %d of %d", last.End, len(wrapped.Runes))
	}
}

func TestLayoutNewlinesAndEmpty(t *testing.T) {
	s := Shared()
	l := s.Layout(Params{Text: "a\n\nb", Style: Style{Size: 14}})
	if len(l.Lines) != 3 {
		t.Fatalf("%d lines", len(l.Lines))
	}
	if l.Lines[1].Start != 2 || l.Lines[1].End != 2 || l.Lines[2].Start != 3 {
		t.Errorf("lines %+v %+v", l.Lines[1], l.Lines[2])
	}
	empty := s.Layout(Params{Style: Style{Size: 14}})
	if len(empty.Lines) != 1 || empty.Height <= 0 {
		t.Errorf("empty text: %d lines, height %v", len(empty.Lines), empty.Height)
	}
}

// TestLayoutTrailingLineSeparator verifies that a paragraph ending with
// U+2028 lays out a trailing empty line matching trailing newline semantics,
// with caret placement and hit-testing on that line.
func TestLayoutTrailingLineSeparator(t *testing.T) {
	s := Shared()
	l := s.Layout(Params{Text: "alpha\u2028", Style: Style{Size: 14}})
	if len(l.Lines) != 2 {
		t.Fatalf("%d lines, want 2", len(l.Lines))
	}
	last := l.Lines[1]
	if last.Start != len(l.Runes) || last.End != len(l.Runes) {
		t.Errorf("empty line is %d:%d, want %d:%d", last.Start, last.End, len(l.Runes), len(l.Runes))
	}
	if last.Height <= 0 || last.Y < l.Lines[0].Height {
		t.Errorf("empty line at y=%v height=%v", last.Y, last.Height)
	}
	if want := l.Lines[0].Height + last.Height; math.Abs(float64(l.Height-want)) > 0.5 {
		t.Errorf("height %v, want %v", l.Height, want)
	}
	x, y, h := l.Caret(len(l.Runes))
	if y != last.Y || h != last.Height {
		t.Errorf("caret after the break at y=%v h=%v, want y=%v h=%v", y, h, last.Y, last.Height)
	}
	if got := l.IndexAt(x, last.Y+last.Height/2); got != len(l.Runes) {
		t.Errorf("IndexAt on the empty line = %d, want %d", got, len(l.Runes))
	}
	// Wrapping reports the empty line itself: it is not added twice.
	wide := s.Layout(Params{Text: "alpha\u2028", Style: Style{Size: 14}, Width: 1000})
	if len(wide.Lines) != 2 || wide.Lines[1].Start != 6 || wide.Lines[1].End != 6 {
		t.Errorf("trailing break with wrapping lines %+v", wide.Lines)
	}
	// A break inside the paragraph keeps two lines of text and adds none.
	inner := s.Layout(Params{Text: "alpha\u2028beta", Style: Style{Size: 14}, Width: 1000})
	if len(inner.Lines) != 2 || inner.Lines[1].Start != 6 || inner.Lines[1].End != 10 {
		t.Errorf("inner break lines %+v", inner.Lines)
	}
}

// TestLayoutTrailingLineSeparatorMaxLines verifies that MaxLines truncation
// accounts for the trailing empty line after U+2028 just like a trailing \n.
func TestLayoutTrailingLineSeparatorMaxLines(t *testing.T) {
	s := Shared()
	style := Style{Size: 14}
	cases := []struct {
		name      string
		text      string
		newline   string
		maxLines  int
		wantLines int
		wantTrunc bool
	}{
		{
			name:      "u2028_before_newline_max_2",
			text:      "alpha\u2028\nbeta",
			newline:   "alpha\n\nbeta",
			maxLines:  2,
			wantLines: 2,
			wantTrunc: true,
		},
		{
			name:      "trailing_u2028_max_1",
			text:      "alpha\u2028",
			newline:   "alpha\n",
			maxLines:  1,
			wantLines: 1,
			wantTrunc: true,
		},
		{
			name:      "trailing_u2028_max_2",
			text:      "alpha\u2028",
			newline:   "alpha\n",
			maxLines:  2,
			wantLines: 2,
			wantTrunc: false,
		},
	}
	for _, tc := range cases {
		for _, width := range []float32{0, 1000} {
			got := s.Layout(Params{Text: tc.text, Style: style, Width: width, MaxLines: tc.maxLines})
			if len(got.Lines) != tc.wantLines || got.Truncated != tc.wantTrunc {
				t.Errorf("%s (width=%v): got %d lines, truncated=%v; want %d lines, truncated=%v",
					tc.name, width, len(got.Lines), got.Truncated, tc.wantLines, tc.wantTrunc)
			}
			nl := s.Layout(Params{Text: tc.newline, Style: style, Width: width, MaxLines: tc.maxLines})
			if len(got.Lines) != len(nl.Lines) || got.Truncated != nl.Truncated {
				t.Errorf("%s (width=%v): got (%d lines, truncated=%v) != newline (%d lines, truncated=%v)",
					tc.name, width, len(got.Lines), got.Truncated, len(nl.Lines), nl.Truncated)
			}
		}
	}
}

func TestCarets(t *testing.T) {
	s := Shared()
	l := s.Layout(Params{Text: "office files", Style: Style{Size: 20}})
	prev := float32(-1)
	for i := 0; i <= len(l.Runes); i++ {
		x, _, h := l.Caret(i)
		if x < prev || h <= 0 {
			t.Fatalf("caret %d at %v after %v (height %v)", i, x, prev, h)
		}
		prev = x
		if got := l.IndexAt(x+0.1, 5); got != i {
			t.Errorf("IndexAt(caret %d) = %d", i, got)
		}
	}
	if rects := l.Selection(0, 6); len(rects) != 1 || rects[0].W <= 0 {
		t.Errorf("selection %+v", rects)
	}
}

func TestTruncation(t *testing.T) {
	s := Shared()
	l := s.Layout(Params{Text: strings.Repeat("word ", 50), Style: Style{Size: 14}, Width: 120, MaxLines: 2})
	if len(l.Lines) != 2 || !l.Truncated {
		t.Fatalf("%d lines, truncated %v", len(l.Lines), l.Truncated)
	}
}

// TestFitsItsWidth lays a line out again at the width it measured, as an
// element sized to its text gives it, and at a rounding under it, as summing
// the same advances in another order comes out: the line fits, uncut.
func TestFitsItsWidth(t *testing.T) {
	s := Shared()
	for _, text := range []string{"停止翻译", "Stop translating", "翻訳を開始"} {
		style := Style{Size: 16, Weight: 700}
		w := s.Layout(Params{Text: text, Style: style}).Width
		for _, at := range []float32{w, w * (1 - 1e-6)} {
			if l := s.Layout(Params{Text: text, Style: style, Width: at, MaxLines: 1}); l.Truncated || len(l.Lines) != 1 {
				t.Errorf("%q at %v of its %v: %d lines, truncated %v", text, at, w, len(l.Lines), l.Truncated)
			}
		}
	}
}

func TestGlyphRaster(t *testing.T) {
	s := Shared()
	l := s.Layout(Params{Text: "Ag", Style: Style{Size: 32}})
	n := 0
	for _, g := range l.Lines[0].Glyphs {
		img := s.Glyph(g.Font, g.ID, 1, 0, 0, false)
		if !img.OK || img.W == 0 || img.H == 0 || img.Top >= 0 {
			t.Fatalf("glyph %v: %+v", g.ID, img)
		}
		var ink int
		for y := 0; y < int(img.H); y++ {
			for x := 0; x < int(img.W); x++ {
				if s.MaskAtlas.Pix[(int(img.Y)+y)*s.MaskAtlas.W+int(img.X)+x] > 128 {
					ink++
				}
			}
		}
		if ink < 20 {
			t.Errorf("glyph %v has %d inked pixels", g.ID, ink)
		}
		n++
	}
	if n != 2 {
		t.Errorf("%d glyphs", n)
	}
}

// TestShadeOf checks shades against the steps of Core Text's smoothing,
// measured on macOS 27.
func TestShadeOf(t *testing.T) {
	for _, c := range []struct {
		r, g, b uint8
		want    Shade
	}{
		{0, 0, 0, 0}, {0x4d, 0x4d, 0x4d, 0}, {0, 0, 255, 0},
		{0x73, 0x73, 0x73, 1}, {0x99, 0x99, 0x99, 1}, {255, 0, 0, 1}, {0x25, 0x63, 0xeb, 1},
		{0xb3, 0xb3, 0xb3, 2},
		{0xd9, 0xd9, 0xd9, 3}, {0, 255, 0, 3}, {255, 255, 255, 3},
	} {
		if got := ShadeOf(c.r, c.g, c.b); got != c.want {
			t.Errorf("ShadeOf(%#x, %#x, %#x) = %d, want %d", c.r, c.g, c.b, got, c.want)
		}
	}
}

// TestGlyphShades rasterizes lighter text bolder where the engine does, as
// AppKit draws it, and color glyphs once.
func TestGlyphShades(t *testing.T) {
	s := newSystem()
	l := s.Layout(Params{Text: "O", Style: Style{Size: 13}})
	g := l.Lines[0].Glyphs[0]
	if !g.Font.shaded {
		t.Skip("the engine draws glyphs the same in any color")
	}
	var ink [Shades]int
	for shade := range Shade(Shades) {
		img := s.Glyph(g.Font, g.ID, 2, 0, shade, false)
		for y := range int(img.H) {
			for x := range int(img.W) {
				ink[shade] += int(s.MaskAtlas.Pix[(int(img.Y)+y)*s.MaskAtlas.W+int(img.X)+x])
			}
		}
	}
	if ink[Shades-1] == ink[0] {
		t.Skip("font smoothing is off")
	}
	for i := 1; i < Shades; i++ {
		// Browsers' canvases embolden in fewer steps than Core Text.
		if ink[i] < ink[i-1] || ink[i] == ink[i-1] && runtime.GOOS != "js" {
			t.Errorf("ink of the shades %v", ink)
		}
	}
	l = s.Layout(Params{Text: "🎉", Style: Style{Size: 32}})
	if g := l.Lines[0].Glyphs[0]; g.Font.shaded {
		t.Errorf("the emoji font is shaded")
	}
}

// TestThick rasterizes Thick text as bold as the lightest shade where the
// engine thickens glyphs (Core Graphics' smoothing has four strengths),
// whether or not the user smooths fonts, and as other text elsewhere.
func TestThick(t *testing.T) {
	s := newSystem()
	l := s.Layout(Params{Text: "O", Style: Style{Size: 13}})
	g := l.Lines[0].Glyphs[0]
	ink := func(f *Font, shade Shade) (n int) {
		img := s.Glyph(f, g.ID, 2, 0, shade, false)
		for y := range int(img.H) {
			for x := range int(img.W) {
				n += int(s.MaskAtlas.Pix[(int(img.Y)+y)*s.MaskAtlas.W+int(img.X)+x])
			}
		}
		return n
	}
	dark, light, thick := ink(g.Font, 0), ink(g.Font, Shades-1), ink(g.Font, Thick)
	if !g.Font.thickens {
		if thick != dark {
			t.Errorf("thick text has ink %d, other text %d", thick, dark)
		}
		return
	}
	if thick < light || thick <= dark {
		t.Errorf("thick text has ink %d, the lightest shade %d, the darkest %d", thick, light, dark)
	}
	// Without smoothing, as when the user turns it off.
	unsmoothed := *g.Font
	unsmoothed.shaded = false
	if plain, thick := ink(&unsmoothed, Shades-1), ink(&unsmoothed, Thick); thick <= plain {
		t.Errorf("thick text has ink %d unsmoothed, other text %d", thick, plain)
	}
}

// TestFlat rasterizes Flat text unsmoothed, thinner than the darkest shade
// where the engine smooths fonts, and as other text elsewhere.
func TestFlat(t *testing.T) {
	s := newSystem()
	l := s.Layout(Params{Text: "O", Style: Style{Size: 13}})
	g := l.Lines[0].Glyphs[0]
	ink := func(f *Font, shade Shade) (n int) {
		img := s.Glyph(f, g.ID, 2, 0, shade, false)
		for y := range int(img.H) {
			for x := range int(img.W) {
				n += int(s.MaskAtlas.Pix[(int(img.Y)+y)*s.MaskAtlas.W+int(img.X)+x])
			}
		}
		return n
	}
	dark, flat := ink(g.Font, 0), ink(g.Font, Flat)
	unsmoothed := *g.Font
	unsmoothed.shaded = false
	if plain := ink(&unsmoothed, 0); flat != plain {
		t.Errorf("flat text has ink %d, unsmoothed text %d", flat, plain)
	}
	// Browsers' canvases smooth text as they do, and draw Flat text as the
	// darkest shade.
	if g.Font.shaded && flat >= dark && runtime.GOOS != "js" {
		t.Errorf("flat text has ink %d, the darkest shade %d", flat, dark)
	}
}

// square draws a w×w mask filled with v.
func square(w int, v byte, calls *int) func() (int, int, []byte) {
	return func() (int, int, []byte) {
		*calls++
		pix := make([]byte, w*w)
		for i := range pix {
			pix[i] = v
		}
		return w, w, pix
	}
}

func (s *System) maskPixel(g GlyphImage) byte {
	return s.MaskAtlas.Pix[int(g.Y)*s.MaskAtlas.W+int(g.X)]
}

func TestMaskLastsOnceDrawnAgain(t *testing.T) {
	s := newSystem()
	width := s.MaskAtlas.W
	var calls int
	s.BeginFrame()
	first := s.Mask(1, square(8, 7, &calls))
	again := s.Mask(1, square(8, 7, &calls))
	s.EndFrame()
	if !first.OK || again != first || calls != 1 {
		t.Fatalf("first frame: %+v %+v, %d draws", first, again, calls)
	}
	if int(first.Y) < s.MaskAtlas.H/2 {
		t.Errorf("a new mask went to row %d, not to the transient rows", first.Y)
	}
	s.BeginFrame()
	second := s.Mask(1, square(8, 7, &calls))
	s.EndFrame()
	if !second.OK || calls != 2 || second.Y != 0 || s.maskPixel(second) != 7 {
		t.Fatalf("second frame: %+v, %d draws", second, calls)
	}
	s.BeginFrame()
	third := s.Mask(1, square(8, 7, &calls))
	s.EndFrame()
	if third != second || calls != 2 {
		t.Errorf("third frame: %+v, %d draws", third, calls)
	}
	// Masks drawn by one frame each, as in an animation, take the same
	// transient rows again and again.
	var y uint16
	for i := range 100 {
		s.BeginFrame()
		g := s.Mask(uint64(100+i), square(30, 9, &calls))
		s.EndFrame()
		if i == 0 {
			y = g.Y
		}
		if !g.OK || s.maskPixel(g) != 9 || g.Y != y || int(y) < s.MaskAtlas.H/2 {
			t.Fatalf("frame %d: %+v", i, g)
		}
	}
	if s.MaskAtlas.W != width {
		t.Errorf("the atlas grew to %d", s.MaskAtlas.W)
	}
}

func TestMakeRoom(t *testing.T) {
	s := newSystem()
	// Room for several old masks, so the test exercises eviction and
	// repacking rather than the small atlas's first growth.
	s.MaskAtlas = scene.NewAtlas(1, 1024, 1024)
	var calls int
	// Fill the lasting rows with masks of earlier frames.
	var old []uint64
	for i := 0; ; i++ {
		key := uint64(i + 1)
		for range 2 { // the second frame makes it last
			s.BeginFrame()
			s.Mask(key, square(200, byte(i+1), &calls))
			s.EndFrame()
		}
		if s.Full() {
			break
		}
		old = append(old, key)
	}
	// A frame drawing one old mask and new ones runs out of room...
	s.BeginFrame()
	kept := s.Mask(old[2], square(200, 3, &calls))
	var left int
	for i := range 3 {
		key := uint64(1000 + i)
		s.recent[key] = s.frame - 1 // drawn by the previous frame: lasting
		if !s.Mask(key, square(300, 50, &calls)).OK {
			left++
		}
	}
	if !s.Full() || left == 0 {
		t.Fatalf("full %v, %d left out", s.Full(), left)
	}
	// ...and has it when painted again.
	s.MakeRoom()
	if s.Full() {
		t.Fatal("still full")
	}
	if g := s.Mask(old[2], square(200, 3, &calls)); g == kept || s.maskPixel(g) != 3 {
		t.Errorf("the kept mask is at %d,%d with %d", g.X, g.Y, s.maskPixel(g))
	}
	for i := range 3 {
		if g := s.Mask(uint64(1000+i), square(300, 50, &calls)); !g.OK || s.maskPixel(g) != 50 {
			t.Errorf("mask %d: %+v", i, g)
		}
	}
	if s.Full() {
		t.Error("full again")
	}
	if _, ok := s.masks[old[0]]; ok {
		t.Error("kept a mask the frame did not draw")
	}
	s.EndFrame()
}

// Masks a frame drew and let go of (Unkeep) are not kept as it makes room,
// unless it draws them again before.
func TestUnkeep(t *testing.T) {
	s := newSystem()
	s.MaskAtlas = scene.NewAtlas(1, 1024, 1024)
	var calls int
	for range 2 { // the second frame makes them last
		s.BeginFrame()
		for key := range uint64(3) {
			s.Mask(key+1, square(200, byte(key+1), &calls))
		}
		s.EndFrame()
	}
	s.BeginFrame()
	for key := range uint64(3) {
		s.Mask(key+1, square(200, byte(key+1), &calls))
	}
	s.Unkeep([]uint64{1, 2, 9})
	s.Mask(2, square(200, 2, &calls))
	s.leftOut(false, 900, 900)
	s.MakeRoom()
	for key, want := range map[uint64]bool{1: false, 2: true, 3: true} {
		if _, ok := s.masks[key]; ok != want {
			t.Errorf("mask %d kept: %v", key, ok)
		}
	}
	s.EndFrame()
}

// What a frame draws many times counts once in the room it needs: an atlas
// that grew by every use of a glyph took 2048×2048 pixels, 16 MB of memory and
// as much of the GPU's, for 24 paragraphs that fill 3% of it.
func TestLeftOutCountsOnce(t *testing.T) {
	s := newSystem()
	s.BeginFrame()
	draws := 0
	draw := func(int) func() (int, int, []byte) {
		return func() (int, int, []byte) {
			draws++
			return 200, 200, make([]byte, 200*200)
		}
	}
	if !s.Mask(1, draw(1)).OK {
		t.Fatal("the first mask does not fit")
	}
	draws = 0
	for range 100 {
		if s.Mask(2, draw(2)).OK {
			t.Fatal("the second mask fit")
		}
	}
	if draws != 1 {
		t.Fatalf("a mask left out was drawn %d times", draws)
	}
	if want := 201 * 201; s.want[0] != want {
		t.Fatalf("the frame wants %d pixels, not %d", s.want[0], want)
	}
	s.MakeRoom()
	if s.MaskAtlas.W != 512 {
		t.Fatalf("the atlas grew to %d for two masks of 200", s.MaskAtlas.W)
	}
	if !s.Mask(2, draw(2)).OK {
		t.Fatal("the mask does not fit after making room")
	}
	s.EndFrame()

	// So does a glyph, in a frame after one that left it out too.
	s = newSystem()
	l := s.Layout(Params{Text: "Hi", Style: Style{Size: 40}})
	g := l.Lines[0].Glyphs[0]
	s.BeginFrame()
	s.MaskAtlas = scene.NewAtlas(1, 4, 4)
	s.MaskAtlas.Put(0, 0, 1, 1, []byte{1}, 1)
	if s.Glyph(g.Font, g.ID, 1, 0, 0, false).OK || !s.Full() {
		t.Skip("the glyph takes no room in the mask atlas")
	}
	want := s.want[0]
	for range 50 {
		s.Glyph(g.Font, g.ID, 1, 0, 0, false)
	}
	if s.want[0] != want {
		t.Fatalf("a glyph left out grew the room wanted from %d to %d", want, s.want[0])
	}
	s.MakeRoom()
	if !s.Glyph(g.Font, g.ID, 1, 0, 0, false).OK {
		t.Fatal("the glyph does not fit after making room")
	}
	s.EndFrame()
	s.BeginFrame()
	if len(s.outGlyphs) != 0 {
		t.Fatal("a new frame remembers what was left out")
	}
}

// Thin masks can need a much wider or taller atlas without taking much
// area: one MakeRoom must accommodate them, whatever the starting size.
func TestMakeRoomForLongMasks(t *testing.T) {
	for _, size := range []image.Point{{X: 2000, Y: 3}, {X: 3, Y: 2000}} {
		s := newSystem()
		s.BeginFrame()
		draw := func() (int, int, []byte) {
			pix := make([]byte, size.X*size.Y)
			for i := range pix {
				pix[i] = 123
			}
			return size.X, size.Y, pix
		}
		if s.Mask(1, draw).OK || !s.Full() {
			t.Fatal("the long mask fit the initial atlas")
		}
		s.MakeRoom()
		if g := s.Mask(1, draw); !g.OK || s.maskPixel(g) != 123 || s.Full() {
			t.Fatalf("mask %v still does not fit after making room: %+v", size, g)
		}
		s.EndFrame()
	}
}

// TestTryMaskAsksForNoRoom draws a mask too long for the initial atlas
// with TryMask, which a caller that draws it another way uses: it misses
// without leaving it out of the frame, which then needs no room made,
// and finds it once room was made for it otherwise.
func TestTryMaskAsksForNoRoom(t *testing.T) {
	s := newSystem()
	s.BeginFrame()
	draw := func() (int, int, []byte) {
		pix := make([]byte, 2000*3)
		for i := range pix {
			pix[i] = 123
		}
		return 2000, 3, pix
	}
	for range 2 {
		if s.TryMask(1, draw).OK || s.Full() {
			t.Fatal("the long mask fit the initial atlas, or asked for room")
		}
	}
	if s.Mask(1, draw).OK || !s.Full() {
		t.Fatal("Mask did not ask for room for the mask TryMask missed")
	}
	s.MakeRoom()
	if g := s.TryMask(1, draw); !g.OK || s.maskPixel(g) != 123 || s.Full() {
		t.Fatalf("the mask does not fit after making room: %+v", g)
	}
	s.EndFrame()
}

func TestMaskCanDeferRasterizationUntilRoom(t *testing.T) {
	for _, size := range []image.Point{{X: 256, Y: 3}, {X: 3, Y: 256}} {
		s := newSystem()
		s.BeginFrame()
		calls, rasterized := 0, 0
		draw := func() (int, int, []byte) {
			calls++
			if a := s.MaskAtlas; size.X >= a.W || size.Y >= a.H {
				return size.X, size.Y, nil
			}
			rasterized++
			pix := make([]byte, size.X*size.Y)
			pix[0] = 123
			return size.X, size.Y, pix
		}
		// The padding pixel makes even a 256-pixel mask need growth.
		for range 2 {
			if s.Mask(1, draw).OK || !s.MaskFull() {
				t.Fatal("the mask fit before its dimensions had room")
			}
		}
		if calls != 1 || rasterized != 0 {
			t.Fatalf("before growth: %d requests, %d rasterizations", calls, rasterized)
		}
		s.MakeRoom()
		if g := s.Mask(1, draw); !g.OK || s.maskPixel(g) != 123 || s.Full() || rasterized != 1 {
			t.Fatalf("after growth: mask %+v, rasterizations %d, full %v", g, rasterized, s.Full())
		}
		s.EndFrame()
	}
}

// Stale masks can fill a lasting shelf while an animation leaves a slot
// in a transient one. A repeated mask must still request recovery before
// using that slot, so it gets cached instead of rasterized every frame.
func TestRepeatedMaskReclaimsStaleLastingRoom(t *testing.T) {
	s := newSystem()
	ignored, calls := 0, 0
	for range 2 {
		s.BeginFrame()
		for key := uint64(1); key <= 2; key++ {
			if !s.Mask(key, square(127, byte(key), &ignored)).OK {
				t.Fatal("the masks did not fit while warming the lasting shelf")
			}
		}
		s.EndFrame()
	}
	draw := square(127, 123, &calls)
	s.BeginFrame()
	if !s.Mask(3, draw).OK {
		t.Fatal("the new mask did not fit in transient room")
	}
	s.EndFrame()
	s.BeginFrame()
	if !s.Mask(4, square(127, 4, &ignored)).OK {
		t.Fatal("the animation did not fit in transient room")
	}
	if s.Mask(3, draw).OK || !s.MaskFull() {
		t.Fatal("the repeated mask used transient room instead of requesting recovery")
	}
	s.MakeRoom()
	if !s.Mask(4, square(127, 4, &ignored)).OK {
		t.Fatal("the animation did not fit after recovery")
	}
	uses := s.TransientUses()
	if g := s.Mask(3, draw); !g.OK || s.maskPixel(g) != 123 || s.Full() || s.TransientUses() != uses {
		t.Fatalf("the repeated mask was not cached after recovery: %+v", g)
	}
	s.EndFrame()
	for range 3 {
		s.BeginFrame()
		before := calls
		if g := s.Mask(3, draw); !g.OK || s.maskPixel(g) != 123 || calls != before || s.TransientUses() != uses {
			t.Fatalf("the stable mask was rasterized again or remained transient: %+v", g)
		}
		s.EndFrame()
	}
}

func TestRepeatedMissingMaskUsesLastingRoom(t *testing.T) {
	s := newSystem()
	draw := func(w int) func() (int, int, []byte) {
		return func() (int, int, []byte) {
			pix := make([]byte, w*200)
			pix[0] = 123
			return w, 200, pix
		}
	}
	s.BeginFrame()
	s.Mask(1, draw(8))
	s.EndFrame()
	s.BeginFrame()
	s.Mask(1, draw(8)) // a lasting shelf with room beside this mask
	if s.Mask(2, draw(100)).OK || !s.MaskFull() {
		t.Fatal("the new mask fit in transient room below the lasting shelf")
	}
	s.EndFrame()
	s.BeginFrame()
	s.Mask(1, draw(8))
	if g := s.Mask(2, draw(100)); !g.OK || s.maskPixel(g) != 123 || s.Full() {
		t.Fatalf("the repeated request did not use spare lasting room: %+v", g)
	}
	if a := s.MaskAtlas; a.W != 256 || a.H != 256 {
		t.Fatalf("the atlas grew instead of sharing its lasting shelf: %dx%d", a.W, a.H)
	}
	s.EndFrame()
}

// A repeated miss prefers lasting storage, but cannot start a lasting
// shelf when transient shelves occupy every row. It must still use an
// empty slot after recovery and forget that bitmap at the next frame.
func TestRepeatedMissingMaskUsesTransientRoom(t *testing.T) {
	s := newSystem()
	// At the size limit, recovery must reuse the four slots rather than
	// grow room for another lasting shelf.
	s.MaskAtlas = scene.NewAtlas(1, scene.MaxAtlasSize, scene.MaxAtlasSize)
	const size = scene.MaxAtlasSize/2 - 1
	pix := make([]byte, size*size)
	calls := 0
	draw := func() (int, int, []byte) {
		calls++
		pix[0] = byte(calls)
		return size, size, pix
	}
	s.BeginFrame()
	for key := uint64(1); key <= 4; key++ {
		if !s.Mask(key, draw).OK {
			t.Fatal("the four transient masks did not fit")
		}
	}
	if s.Mask(5, draw).OK || !s.MaskFull() {
		t.Fatal("the fifth mask fit the full atlas")
	}
	s.EndFrame()
	s.BeginFrame()
	for key := uint64(6); key <= 8; key++ {
		if !s.Mask(key, draw).OK {
			t.Fatal("the new transient masks did not fit")
		}
	}
	if s.Mask(5, draw).OK || !s.MaskFull() {
		t.Fatal("the repeated request did not ask for recovery before falling back")
	}
	s.MakeRoom()
	for key := uint64(6); key <= 8; key++ {
		if !s.Mask(key, draw).OK {
			t.Fatal("the transient masks did not fit after recovery")
		}
	}
	if g := s.Mask(5, draw); !g.OK || s.maskPixel(g) != byte(calls) || s.Full() {
		t.Fatalf("the repeated request did not use spare transient room: %+v", g)
	}
	s.EndFrame()
	s.BeginFrame()
	before := calls
	if g := s.Mask(5, draw); !g.OK || calls != before+1 || s.maskPixel(g) != byte(calls) {
		t.Fatalf("the transient mask survived into the next frame: %+v, calls %d", g, calls-before)
	}
	s.EndFrame()
	s.BeginFrame()
	before = calls
	if g := s.Mask(5, draw); !g.OK || calls != before {
		t.Fatalf("the repeated mask was not cached once lasting room was free: %+v", g)
	}
	s.EndFrame()
}

func TestRightToLeft(t *testing.T) {
	s := Shared()
	l := s.Layout(Params{Text: "שלום עולם", Style: Style{Size: 16}})
	if len(l.Lines) != 1 || !l.Lines[0].RTL || len(l.Lines[0].Glyphs) == 0 {
		t.Fatalf("%+v", l.Lines)
	}
	// The caret before the first rune is on the right.
	first, _, _ := l.Caret(0)
	last, _, _ := l.Caret(len(l.Runes))
	if first <= last {
		t.Errorf("carets at %v and %v", first, last)
	}
	// Start aligns right-to-left lines to the right.
	wide := s.Layout(Params{Text: "שלום", Style: Style{Size: 16}, Width: 300})
	if line := wide.Lines[0]; line.X+line.Width < 299 {
		t.Errorf("line at %v, %v wide", line.X, line.Width)
	}
}

// TestGlyphsInVisualOrder checks that every engine gives the glyphs of a
// line in visual order: X ascends along the line, and through a
// right-to-left run the clusters descend, the glyphs marked RTL.
func TestGlyphsInVisualOrder(t *testing.T) {
	s := Shared()
	for _, p := range []Params{
		{Text: "中文 hello שלום，مرحبا world。", NoWrap: true, KeepSpaces: true},
		{Text: "שלום abc עולם 123 مرحبا", NoWrap: true},
		{Text: "hello שלום עולם, مرحبا بالعالم and the rest of it", Width: 120},
		{Text: "שלום עולם, hello world and مرحبا بالعالم again", Width: 120},
		// Broken at soft hyphens, which show a hyphen at the left.
		{Text: "مرح\u00adبا بال\u00adعالم", Width: 30},
		{Text: "ab مرح\u00adبا بال\u00adعالم", Width: 30},
	} {
		p.Style = Style{Size: 17}
		l := s.Layout(p)
		rtl := 0
		for li, line := range l.Lines {
			// The leftmost glyph of the cluster before: within one, as of
			// an Arabic letter and its marks, Core Text may place a glyph
			// left of the one before it.
			var start, at float32
			for i, g := range line.Glyphs {
				if g.Cluster < 0 || g.Cluster >= len(l.Runes) {
					continue
				}
				r := l.Runes[g.Cluster]
				if strong := unicode.In(r, unicode.Hebrew, unicode.Arabic); strong != g.RTL && (strong || unicode.IsLetter(r)) {
					t.Errorf("%q: line %d glyph %d of %q has RTL %v", p.Text, li, i, r, g.RTL)
				}
				if g.RTL {
					rtl++
				}
				if i == 0 {
					at = g.X
					continue
				}
				prev := line.Glyphs[i-1]
				if g.Cluster != prev.Cluster {
					start, at = at, g.X
					if g.X < start-0.01 {
						t.Errorf("%q: line %d glyph %d (cluster %d) at %v, left of the cluster before it (%d) at %v", p.Text, li, i, g.Cluster, g.X, prev.Cluster, start)
					}
				} else {
					at = min(at, g.X)
				}
				if g.RTL && prev.RTL && g.Cluster > prev.Cluster {
					t.Errorf("%q: line %d right to left, cluster %d follows cluster %d", p.Text, li, g.Cluster, prev.Cluster)
				}
				if !g.RTL && !prev.RTL && g.Cluster < prev.Cluster && prev.Cluster >= 0 {
					t.Errorf("%q: line %d left to right, cluster %d follows cluster %d", p.Text, li, g.Cluster, prev.Cluster)
				}
			}
		}
		if rtl == 0 {
			t.Errorf("%q: no glyph is right to left", p.Text)
		}
	}
}

func TestKeepSpaces(t *testing.T) {
	s := Shared()
	p := Params{Text: "aaaa bbbb", Style: Style{Size: 16}}
	p.Width = s.Layout(p).Width - 1
	trimmed := s.Layout(p)
	p.KeepSpaces = true
	kept := s.Layout(p)
	if len(trimmed.Lines) != 2 || len(kept.Lines) != 2 {
		t.Fatalf("%d and %d lines", len(trimmed.Lines), len(kept.Lines))
	}
	if trimmed.Lines[0].End != 5 || kept.Lines[0].End != 5 || trimmed.Lines[1].Start != 5 {
		t.Errorf("lines %d-%d and %d-%d", trimmed.Lines[0].Start, trimmed.Lines[0].End, trimmed.Lines[1].Start, trimmed.Lines[1].End)
	}
	if kept.Lines[0].Width <= trimmed.Lines[0].Width {
		t.Errorf("with the space %v wide, without %v", kept.Lines[0].Width, trimmed.Lines[0].Width)
	}
}

// A no-break space ending a line keeps its room, as in CSS, where the
// other whitespace hangs.
func TestNoBreakSpaceKept(t *testing.T) {
	s := Shared()
	width := func(text string) float32 { return s.Layout(Params{Text: text, Style: Style{Size: 16}}).Width }
	x := width("x")
	if w := width("x "); math.Abs(float64(w-x)) > 0.05 {
		t.Errorf("x and a space are %v wide, x %v", w, x)
	}
	if w := width("x\u00a0"); w <= x {
		t.Errorf("x and a no-break space are %v wide, x %v", w, x)
	}
}

func TestNoBreakWords(t *testing.T) {
	s := Shared()
	p := Params{Text: "Supercalifragilistic word", Style: Style{Size: 16}, Width: 40}
	if l := s.Layout(p); len(l.Lines) < 3 {
		t.Errorf("a long word took %d lines", len(l.Lines))
	}
	p.NoBreakWords = true
	if l := s.Layout(p); len(l.Lines) != 2 || l.Lines[0].Width <= 40 || l.Lines[0].End != 21 {
		t.Errorf("%d lines, the first %v wide, ending at %d", len(l.Lines), l.Lines[0].Width, l.Lines[0].End)
	}
}

func TestEllipsis(t *testing.T) {
	s := Shared()
	l := s.Layout(Params{Text: "The quick brown fox jumps", Style: Style{Size: 16}, Width: 100, MaxLines: 1})
	line := l.Lines[0]
	if !l.Truncated || len(line.Glyphs) == 0 || line.Width > 100 || line.End >= len(l.Runes) {
		t.Fatalf("truncated %v, %d glyphs, %v wide, ending at %d", l.Truncated, len(line.Glyphs), line.Width, line.End)
	}
	if g := line.Glyphs[len(line.Glyphs)-1]; g.Runes != 0 || g.Cluster != line.End {
		t.Errorf("the last glyph is %+v", g)
	}
}

// checkMiddleCut returns the runes a line that TruncateMiddle cut keeps
// before its ellipsis and from where it keeps them after it, checking
// that its glyphs hold no hidden rune and that one ellipsis stands
// between them.
func checkMiddleCut(t *testing.T, l *Layout) (cut, resume int) {
	t.Helper()
	if !l.Truncated || len(l.Lines) != 1 {
		t.Fatalf("truncated %v, %d lines", l.Truncated, len(l.Lines))
	}
	line := l.Lines[0]
	cut, resume = -1, len(l.Runes)
	for _, g := range line.Glyphs {
		if g.Runes == 0 {
			if cut >= 0 && g.Cluster != cut {
				t.Fatalf("ellipsis glyphs at %d and %d", cut, g.Cluster)
			}
			cut = g.Cluster
		}
	}
	if cut < 0 {
		t.Fatalf("no ellipsis in %+v", line.Glyphs)
	}
	for _, g := range line.Glyphs {
		if g.Runes > 0 && g.Cluster >= cut {
			resume = min(resume, g.Cluster)
		}
	}
	for _, g := range line.Glyphs {
		if g.Runes > 0 && g.Cluster < cut && g.Cluster+g.Runes > cut {
			t.Errorf("glyph %+v holds runes the ellipsis hides from %d", g, cut)
		}
	}
	if line.Start != 0 || line.End != len(l.Runes) || resume <= cut {
		t.Fatalf("line %d to %d, cut at %d, resuming at %d", line.Start, line.End, cut, resume)
	}
	return cut, resume
}

func TestTruncateMiddle(t *testing.T) {
	s := newSystem()
	s.UseTruncateMiddle()
	path := "src/components/settings/appearance/theme/picker.go"
	p := Params{Text: path, Style: Style{Size: 14}, Width: 160, MaxLines: 1, TruncateMiddle: true}
	l := s.Layout(p)
	cut, resume := checkMiddleCut(t, l)
	if w := l.Lines[0].Width; w > p.Width {
		t.Errorf("the line is %v wide in %v", w, p.Width)
	}
	if !strings.HasPrefix(path, "src/") || cut < 4 || resume > len(path)-len("picker.go") {
		t.Errorf("kept %q…%q", path[:cut], path[resume:])
	}
	// Both sides are about as wide: they differ by less than a grapheme.
	width := func(text string) float32 { return s.Layout(Params{Text: text, Style: p.Style}).Width }
	head, tail, m := width(path[:cut]), width(path[resume:]), width("m")
	if head-tail > m || tail-head > m {
		t.Errorf("%q is %v wide, %q %v", path[:cut], head, path[resume:], tail)
	}
	// The runes it hides sit where the text resumes, after the ellipsis.
	at, _, _ := l.Caret(resume)
	for i := cut + 1; i < resume; i++ {
		if x, _, _ := l.Caret(i); x != at {
			t.Errorf("the caret before hidden rune %d is at %v, not %v", i, x, at)
		}
	}
	prev := float32(-1)
	for i := 0; i <= len(path); i++ {
		x, _, _ := l.Caret(i)
		if x < prev {
			t.Errorf("the caret before rune %d is at %v, left of %v", i, x, prev)
		}
		prev = x
	}
	if i := l.IndexAt(l.Lines[0].Width+10, 5); i != len(path) {
		t.Errorf("past the end is rune %d", i)
	}
}

func TestTruncateMiddleGraphemes(t *testing.T) {
	s := newSystem()
	s.UseTruncateMiddle()
	// Each grapheme is two runes: an e and its accent, and a family of
	// four joined into one emoji is seven.
	for _, text := range []string{strings.Repeat("é", 40), strings.Repeat("👩‍👩‍👧‍👦", 12)} {
		l := s.Layout(Params{Text: text, Style: Style{Size: 14}, Width: 90, MaxLines: 1, TruncateMiddle: true})
		cut, resume := checkMiddleCut(t, l)
		var b Boundaries
		b.Reset(l.Runes)
		for _, i := range []int{cut, resume} {
			if _, ok := slices.BinarySearch(b.graphemes, i); !ok {
				t.Errorf("%q is cut at rune %d, inside a grapheme", text, i)
			}
		}
	}
}

func TestTruncateMiddleRTL(t *testing.T) {
	s := newSystem()
	s.UseTruncateMiddle()
	for _, text := range []string{
		"שלום עולם זהו טקסט ארוך מאוד שצריך לקצר באמצע",
		"مرحبا بالعالم هذا نص طويل جدا يجب اختصاره في المنتصف",
		"docs/מדריכים/התחלה/מהירה/readme.md",
	} {
		p := Params{Text: text, Style: Style{Size: 14}, Width: 140, MaxLines: 1, TruncateMiddle: true}
		l := s.Layout(p)
		cut, resume := checkMiddleCut(t, l)
		line := l.Lines[0]
		// Arabic's letters join, and beside the ellipsis take other forms
		// than those measured, as at the end of a line cut there.
		joins := strings.ContainsFunc(text, func(r rune) bool { return unicode.Is(unicode.Arabic, r) })
		if line.Width > p.Width && !joins || cut == 0 || resume == len(l.Runes) {
			t.Errorf("%q: %v wide, cut at %d, resuming at %d", text, line.Width, cut, resume)
		}
		if want := isRTL(l.Runes, false); line.RTL != want {
			t.Errorf("%q: RTL %v", text, line.RTL)
		}
	}
}

func TestTruncateMiddleSpans(t *testing.T) {
	s := newSystem()
	s.UseTruncateMiddle()
	// A folder in the style of the text, then a file's name larger.
	text := "src/components/settings/theme/picker.go"
	name := strings.Index(text, "picker")
	spans := EncodeSpans([]Span{{End: name}, {End: len(text), Size: 20}})
	l := s.Layout(Params{Text: text, Style: Style{Size: 12}, Spans: spans, Width: 180, MaxLines: 1, TruncateMiddle: true})
	cut, resume := checkMiddleCut(t, l)
	if resume > name {
		t.Fatalf("resumes at %d, after the name at %d", resume, name)
	}
	for _, g := range l.Lines[0].Glyphs {
		want := float32(12)
		if g.Runes > 0 && g.Cluster >= name {
			want = 20
		}
		if g.Size != want {
			t.Errorf("glyph %+v of rune %d (cut at %d) is %v large", g, g.Cluster, cut, g.Size)
		}
	}
}

func TestTruncateMiddleEnds(t *testing.T) {
	s := newSystem()
	text := "src/components/settings/appearance/theme/picker.go"
	p := Params{Text: text, Style: Style{Size: 14}, Width: 160, MaxLines: 1, TruncateMiddle: true}
	endsWithEllipsis := func(l *Layout) bool {
		gs := l.Lines[0].Glyphs
		return l.Truncated && gs[len(gs)-1].Runes == 0 && l.Lines[0].End < len(l.Runes)
	}
	// Before UseTruncateMiddle the ellipsis ends the line.
	if l := s.Layout(p); !endsWithEllipsis(l) {
		t.Errorf("without UseTruncateMiddle: %+v", l.Lines[0])
	}
	s.UseTruncateMiddle()
	// So it does where the paragraph fits and only the next one is cut,
	// and without a width.
	for _, q := range []Params{
		{Text: "src/a.go\nsrc/b.go", Style: p.Style, Width: 160, MaxLines: 1, TruncateMiddle: true},
		{Text: text + "\n" + text, Style: p.Style, MaxLines: 1, TruncateMiddle: true},
	} {
		if l := s.Layout(q); !endsWithEllipsis(l) {
			t.Errorf("%q in %v: %+v", q.Text, q.Width, l.Lines[0])
		}
	}
	// A text that fits is not cut.
	p.Width = 1000
	if l := s.Layout(p); l.Truncated {
		t.Errorf("a text that fits is cut: %+v", l.Lines[0])
	}
	// The last of several lines is cut in its middle, keeping the end of
	// its paragraph.
	long := "The quick brown fox jumps over the lazy dog and keeps running to the end.txt"
	l := s.Layout(Params{Text: long, Style: p.Style, Width: 160, MaxLines: 2, TruncateMiddle: true})
	if len(l.Lines) != 2 || !l.Truncated {
		t.Fatalf("%d lines, truncated %v", len(l.Lines), l.Truncated)
	}
	last := l.Lines[1]
	if g := last.Glyphs[len(last.Glyphs)-1]; last.End != len(l.Runes) || g.Runes == 0 || g.Cluster+g.Runes != len(l.Runes) || last.Width > 160 {
		t.Errorf("the last line ends at %d with %+v, %v wide", last.End, g, last.Width)
	}
}

// checkStartCut returns from where a line that TruncateStart cut keeps
// the runes of its paragraph, checking it as checkMiddleCut does and
// that the ellipsis, before them, keeps none of its start.
func checkStartCut(t *testing.T, l *Layout) (resume int) {
	t.Helper()
	cut, resume := checkMiddleCut(t, l)
	if cut != 0 {
		t.Fatalf("keeps %d runes before the ellipsis", cut)
	}
	return resume
}

func TestTruncateStart(t *testing.T) {
	s := newSystem()
	s.UseTruncateStart()
	path := "src/components/settings/appearance/theme/picker.go"
	p := Params{Text: path, Style: Style{Size: 14}, Width: 160, MaxLines: 1, TruncateStart: true}
	l := s.Layout(p)
	resume := checkStartCut(t, l)
	line := l.Lines[0]
	if line.Width > p.Width {
		t.Errorf("the line is %v wide in %v", line.Width, p.Width)
	}
	if g := line.Glyphs[0]; g.Runes != 0 {
		t.Errorf("the line starts with %+v, not the ellipsis", g)
	}
	if resume > len(path)-len("theme/picker.go") {
		t.Errorf("kept …%q", path[resume:])
	}
	// It keeps as much of the end as fits: one grapheme more does not.
	width := func(text string) float32 { return s.Layout(Params{Text: text, Style: p.Style}).Width }
	if w := width("…" + path[resume-1:]); w <= p.Width {
		t.Errorf("…%q is %v wide, fitting %v", path[resume-1:], w, p.Width)
	}
	// The runes it hides, its first too, sit where the text resumes,
	// after the ellipsis, as those an end cut hides sit before it.
	at, _, _ := l.Caret(resume)
	for i := 0; i < resume; i++ {
		if x, _, _ := l.Caret(i); x != at {
			t.Errorf("the caret before hidden rune %d is at %v, not %v", i, x, at)
		}
	}
	prev := float32(-1)
	for i := 0; i <= len(path); i++ {
		x, _, _ := l.Caret(i)
		if x < prev {
			t.Errorf("the caret before rune %d is at %v, left of %v", i, x, prev)
		}
		prev = x
	}
	if i := l.IndexAt(-10, 5); i != resume {
		t.Errorf("before the start is rune %d, not %d", i, resume)
	}
	if i := l.IndexAt(line.Width+10, 5); i != len(path) {
		t.Errorf("past the end is rune %d", i)
	}
	// Asked for both, the start is cut.
	p.TruncateMiddle = true
	if r := checkStartCut(t, s.Layout(p)); r != resume {
		t.Errorf("with TruncateMiddle too, resumes at %d, not %d", r, resume)
	}
}

func TestTruncateStartGraphemes(t *testing.T) {
	s := newSystem()
	s.UseTruncateStart()
	for _, text := range []string{strings.Repeat("é", 40), strings.Repeat("👩‍👩‍👧‍👦", 12)} {
		l := s.Layout(Params{Text: text, Style: Style{Size: 14}, Width: 90, MaxLines: 1, TruncateStart: true})
		resume := checkStartCut(t, l)
		var b Boundaries
		b.Reset(l.Runes)
		if _, ok := slices.BinarySearch(b.graphemes, resume); !ok {
			t.Errorf("%q resumes at rune %d, inside a grapheme", text, resume)
		}
	}
}

func TestTruncateStartRTL(t *testing.T) {
	s := newSystem()
	s.UseTruncateStart()
	for _, text := range []string{
		"שלום עולם זהו טקסט ארוך מאוד שצריך לקצר בהתחלה",
		"مرحبا بالعالم هذا نص طويل جدا يجب اختصاره في البداية",
		"docs/מדריכים/התחלה/מהירה/readme.md",
	} {
		p := Params{Text: text, Style: Style{Size: 14}, Width: 140, MaxLines: 1, TruncateStart: true}
		l := s.Layout(p)
		resume := checkStartCut(t, l)
		line := l.Lines[0]
		joins := strings.ContainsFunc(text, func(r rune) bool { return unicode.Is(unicode.Arabic, r) })
		if line.Width > p.Width && !joins || resume == len(l.Runes) {
			t.Errorf("%q: %v wide, resuming at %d", text, line.Width, resume)
		}
		if want := isRTL(l.Runes, false); line.RTL != want {
			t.Errorf("%q: RTL %v", text, line.RTL)
		}
	}
}

func TestTruncateStartSpans(t *testing.T) {
	s := newSystem()
	s.UseTruncateStart()
	// A folder in the style of the text, then a file's name larger.
	text := "src/components/settings/theme/picker.go"
	name := strings.Index(text, "picker")
	spans := EncodeSpans([]Span{{End: name}, {End: len(text), Size: 20}})
	l := s.Layout(Params{Text: text, Style: Style{Size: 12}, Spans: spans, Width: 180, MaxLines: 1, TruncateStart: true})
	resume := checkStartCut(t, l)
	if resume > name {
		t.Fatalf("resumes at %d, after the name at %d", resume, name)
	}
	for _, g := range l.Lines[0].Glyphs {
		want := float32(12)
		if g.Runes > 0 && g.Cluster >= name {
			want = 20
		}
		if g.Size != want {
			t.Errorf("glyph %+v of rune %d (resuming at %d) is %v large", g, g.Cluster, resume, g.Size)
		}
	}
	// Where only the name shows, the ellipsis takes its style, as it
	// takes that of the text it ends at the end of a line.
	l = s.Layout(Params{Text: text, Style: Style{Size: 12}, Spans: spans, Width: 80, MaxLines: 1, TruncateStart: true})
	if resume := checkStartCut(t, l); resume < name {
		t.Fatalf("resumes at %d, before the name at %d", resume, name)
	}
	for _, g := range l.Lines[0].Glyphs {
		if g.Size != 20 {
			t.Errorf("glyph %+v is %v large", g, g.Size)
		}
	}
}

func TestTruncateStartEnds(t *testing.T) {
	s := newSystem()
	text := "src/components/settings/appearance/theme/picker.go"
	p := Params{Text: text, Style: Style{Size: 14}, Width: 160, MaxLines: 1, TruncateStart: true}
	endsWithEllipsis := func(l *Layout) bool {
		gs := l.Lines[0].Glyphs
		return l.Truncated && gs[len(gs)-1].Runes == 0 && l.Lines[0].End < len(l.Runes)
	}
	// Before UseTruncateStart the ellipsis ends the line.
	if l := s.Layout(p); !endsWithEllipsis(l) {
		t.Errorf("without UseTruncateStart: %+v", l.Lines[0])
	}
	s.UseTruncateStart()
	// So it does where the paragraph fits and only the next one is cut,
	// and without a width.
	for _, q := range []Params{
		{Text: "src/a.go\nsrc/b.go", Style: p.Style, Width: 160, MaxLines: 1, TruncateStart: true},
		{Text: text + "\n" + text, Style: p.Style, MaxLines: 1, TruncateStart: true},
	} {
		if l := s.Layout(q); !endsWithEllipsis(l) {
			t.Errorf("%q in %v: %+v", q.Text, q.Width, l.Lines[0])
		}
	}
	// A text that fits is not cut.
	p.Width = 1000
	if l := s.Layout(p); l.Truncated {
		t.Errorf("a text that fits is cut: %+v", l.Lines[0])
	}
	// The last of several lines is cut at its start, keeping the end of
	// its paragraph, and the lines before it are whole.
	long := "The quick brown fox jumps over the lazy dog and keeps running to the end.txt"
	whole := s.Layout(Params{Text: long, Style: p.Style, Width: 160})
	l := s.Layout(Params{Text: long, Style: p.Style, Width: 160, MaxLines: 2, TruncateStart: true})
	if len(l.Lines) != 2 || !l.Truncated || len(whole.Lines) < 3 {
		t.Fatalf("%d lines, truncated %v, of %d", len(l.Lines), l.Truncated, len(whole.Lines))
	}
	if first := l.Lines[0]; first.Start != 0 || first.End != whole.Lines[0].End {
		t.Errorf("the first line holds runes %d to %d, not to %d", first.Start, first.End, whole.Lines[0].End)
	}
	last := l.Lines[1]
	head, tail := last.Glyphs[0], last.Glyphs[len(last.Glyphs)-1]
	if head.Runes != 0 || head.Cluster != last.Start || last.End != len(l.Runes) || tail.Cluster+tail.Runes != len(l.Runes) || last.Width > 160 {
		t.Errorf("the last line, runes %d to %d, starts with %+v and ends with %+v, %v wide", last.Start, last.End, head, tail, last.Width)
	}
}

func TestColorEmoji(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("emoji fonts vary")
	}
	s := Shared()
	l := s.Layout(Params{Text: "🎉", Style: Style{Size: 32}})
	g := l.Lines[0].Glyphs[0]
	if img := s.Glyph(g.Font, g.ID, 1, 0, 0, false); !img.OK || !img.Colored || img.W < 16 {
		t.Errorf("%+v", img)
	}
}

// TestTextPresentation checks that symbols with text presentation, such as
// ⏺, draw in the text's color though only a color font has them, and that
// emoji and those with the emoji presentation selector draw in color.
func TestTextPresentation(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("emoji fonts vary")
	}
	for _, tc := range []struct {
		text  string
		color bool
	}{
		{"\u23fa", false}, {"\u23fa\ufe0f", true}, {"\u2b50", true}, {"\u2b50\ufe0e", false}, {"\U0001f389", true},
	} {
		s := newSystem() // each glyph in atlases of its own
		l := s.Layout(Params{Text: "a" + tc.text + "a", Style: Style{Size: 32}})
		g := l.Lines[0].Glyphs[1]
		if g.Font.color != tc.color {
			t.Errorf("%+q: color %v, want %v", tc.text, g.Font.color, tc.color)
		}
		if img := s.Glyph(g.Font, g.ID, 1, 0, 0, false); !img.OK || img.Colored != tc.color || img.W < 8 {
			t.Errorf("%+q: %+v", tc.text, img)
		}
	}
}

func TestRegisterFont(t *testing.T) {
	s := newSystem()
	if err := s.RegisterFont(goregular.TTF, "MyGo Test Sans"); err != nil {
		t.Fatal(err)
	}
	font := func(family string) *Font {
		return s.Layout(Params{Text: "Hello", Style: Style{Size: 20, Family: family}}).Lines[0].Glyphs[0].Font
	}
	system := font("")
	if font("MyGo Test Sans") == system || font("Go") == system {
		t.Error("text does not use the registered font")
	}
}

// An app's fonts (RegisterAppFont) reach Shared and every System New
// makes, as a Tester's window has: one made before them takes them at its
// next layout, one made after before its first. Go Mono is monospaced, so
// "iiiiiiii" is as wide as "mmmmmmmm" only in it.
func TestRegisterAppFontReachesSystemsNewMakes(t *testing.T) {
	const family = "MyGo Test App Mono"
	monospaced := func(s *System) bool {
		width := func(text string) float32 {
			return s.Layout(Params{Text: text, Style: Style{Size: 20, Family: family}}).Lines[0].Width
		}
		i, m := width("iiiiiiii"), width("mmmmmmmm")
		return i > m-0.5 && i < m+0.5
	}
	before := New()
	if monospaced(before) {
		t.Fatal("text uses the app's font before it is registered")
	}
	if err := RegisterAppFont(gomono.TTF, family); err != nil {
		t.Fatal(err)
	}
	after := New()
	for name, s := range map[string]*System{"Shared": Shared(), "made before": before, "made after": after} {
		if !monospaced(s) {
			t.Errorf("%s: text does not use the app's font", name)
		}
	}
}

// TestRegisterFontUnderGivenFamilies tells apart faces of one family
// registered under families the app gives, as two that declare the same
// weight need: each family the app gives holds only the files registered
// under it, matched by weight and style among them, and the font's own
// family, which the app gives none, holds them all (#1224).
func TestRegisterFontUnderGivenFamilies(t *testing.T) {
	s := newSystem()
	for _, f := range []struct {
		data   []byte
		family string
	}{
		{goregular.TTF, "MyGo Test Given Upright"},
		{goitalic.TTF, "MyGo Test Given Other"},
		{gobold.TTF, "MyGo Test Given Other"},
	} {
		if err := s.RegisterFont(f.data, f.family); err != nil {
			t.Fatal(err)
		}
	}
	width := func(family string, weight int, italic bool) float32 {
		return s.Layout(Params{Text: "The quick brown fox jumps over the lazy dog 0123456789", Style: Style{Size: 40, Family: family, Weight: weight, Italic: italic}}).Width
	}
	// The faces, in their own family Go.
	regular, italic, bold := width("Go", 400, false), width("Go", 400, true), width("Go", 700, false)
	if regular == italic || regular == bold || italic == bold {
		t.Fatalf("Go's faces are alike: regular %v, italic %v, bold %v", regular, italic, bold)
	}
	for _, c := range []struct {
		family string
		weight int
		italic bool
		want   float32
	}{
		{"MyGo Test Given Upright", 400, false, regular},
		{"MyGo Test Given Other", 400, true, italic},
		{"MyGo Test Given Other", 700, false, bold},
	} {
		if got := width(c.family, c.weight, c.italic); got != c.want {
			t.Errorf("%s at %d, italic %v: width %v, want %v (regular %v, italic %v, bold %v)", c.family, c.weight, c.italic, got, c.want, regular, italic, bold)
		}
	}
	// Regular is Go's face at 400, but not one of this family's.
	if got := width("MyGo Test Given Other", 400, false); got != italic && got != bold {
		t.Errorf("MyGo Test Given Other at 400: width %v, not one of its faces (regular %v, italic %v, bold %v)", got, regular, italic, bold)
	}
}

// TestRegisterFontUnderOwnFamily gives a font's own family name to some
// of its faces, after another face of the family was registered under a
// family of its own: the own name then holds only the faces registered
// under it, whichever came first, as Selawik's Light and Semilight need
// (#1224).
func TestRegisterFontUnderOwnFamily(t *testing.T) {
	s := newSystem()
	if err := s.RegisterFont(gomediumitalic.TTF, "MyGo Test Given Medium Italic"); err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterFont(gomedium.TTF, "Go Medium"); err != nil {
		t.Fatal(err)
	}
	width := func(family string, italic bool) float32 {
		return s.Layout(Params{Text: "The quick brown fox jumps over the lazy dog 0123456789", Style: Style{Size: 40, Family: family, Weight: 500, Italic: italic}}).Width
	}
	upright, italic := width("Go Medium", false), width("MyGo Test Given Medium Italic", true)
	if upright == italic {
		t.Fatalf("Go Medium's faces are alike: %v", upright)
	}
	if got := width("Go Medium", true); got == italic {
		t.Errorf("Go Medium, italic: width %v, the face registered under another family (upright %v)", got, upright)
	}
}

// TestFontFamilies lists the app's families first, each under the name
// it registered it with, or its own, then the system's, telling the
// monospaced ones, and lists the system's again after RegisterFont.
func TestFontFamilies(t *testing.T) {
	s := newSystem()
	if err := s.RegisterFont(goregular.TTF, "MyGo Test Families"); err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterFont(gomono.TTF, ""); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	all := s.FontFamilies()
	t.Logf("%d families in %v", len(all), time.Since(start))
	app := []FontFamily{{Name: "Go Mono", Monospaced: true, Registered: true}, {Name: "MyGo Test Families", Registered: true}}
	if len(all) < len(app) || !slices.Equal(all[:len(app)], app) {
		t.Fatalf("the registered families are not first: %+v", all[:min(4, len(all))])
	}
	name := func(f FontFamily) string { return f.Name }
	names := func(fs []FontFamily) []string {
		out := make([]string, len(fs))
		for i, f := range fs {
			out[i] = name(f)
		}
		return out
	}
	if slices.Contains(names(all), "Go") {
		t.Error("a font registered under a name the app gave is listed under its own too")
	}
	system := all[len(app):]
	if slices.ContainsFunc(system, func(f FontFamily) bool { return f.Registered }) ||
		!slices.IsSortedFunc(system, func(a, b FontFamily) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) }) {
		t.Errorf("the system's families are not sorted without regard to case: %q", names(system)[:min(8, len(system))])
	}
	start = time.Now()
	if again := s.FontFamilies(); !slices.Equal(again, all) {
		t.Error("listing again changed the families")
	}
	t.Logf("again in %v", time.Since(start))
	if err := s.RegisterFont(gobold.TTF, ""); err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(s.FontFamilies(), func(f FontFamily) bool { return f.Name == "Go" }); i != 0 {
		t.Errorf("a font registered after listing is at %d", i)
	}

	// A monospaced and a proportional family every such system has.
	known, ok := map[string][2]string{
		"windows": {"Consolas", "Segoe UI"},
		"darwin":  {"Menlo", "Helvetica"},
	}[runtime.GOOS]
	if !ok {
		return // the families of Linux desktops vary
	}
	for i, want := range known {
		j := slices.IndexFunc(system, func(f FontFamily) bool { return f.Name == want })
		if j < 0 || system[j].Monospaced != (i == 0) {
			t.Errorf("%s: at %d of the system's families, %+v", want, j, system[max(j, 0)])
		}
	}
}

// TestUIFamily gives system-ui the desktop's interface font where the
// engine takes it, as Pango does: the family before fontconfig's default.
func TestUIFamily(t *testing.T) {
	s := newSystem()
	if err := s.RegisterFont(goregular.TTF, "MyGo Test UI"); err != nil {
		t.Fatal(err)
	}
	font := func(family string) *Font {
		return s.Layout(Params{Text: "Hello", Style: Style{Size: 20, Family: family}}).Lines[0].Glyphs[0].Font
	}
	before := font("")
	registered := font("MyGo Test UI")
	s.SetUIFamily("MyGo Test UI")
	_, takes := s.engine().(uiFamilySetter)
	for _, family := range []string{"", "system-ui"} {
		if got := font(family); takes && got != registered || !takes && got != before {
			t.Errorf("%q after SetUIFamily: the font of the interface family %v, of the system %v", family, got == registered, got == before)
		}
	}
	s.SetUIFamily("")
	if font("") != before {
		t.Error("system-ui keeps the interface family once it is unset")
	}
}

func TestLetterSpacing(t *testing.T) {
	s := newSystem()
	width := func(spacing float32) float32 {
		return s.Layout(Params{Text: "Hello", Style: Style{Size: 20, LetterSpacing: spacing}}).Width
	}
	plain, spaced, tight := width(0), width(4), width(-1)
	// Four DIPs after each of the five letters, or of the four between
	// them, as engines differ.
	if d := spaced - plain; d < 4*4-0.5 || d > 4*5+0.5 {
		t.Errorf("letter spacing of 4 widens Hello from %v to %v", plain, spaced)
	}
	if tight >= plain {
		t.Errorf("negative letter spacing makes Hello %v wide, not less than %v", tight, plain)
	}
}

func TestFontFeatures(t *testing.T) {
	s := newSystem()
	width := func(features string) float32 {
		return s.Layout(Params{Text: "AVAVAVAVAV", Style: Style{Size: 40, Features: features}}).Width
	}
	// The system's interface font kerns A and V together.
	kerned, plain := width(""), width("kern=0")
	if plain <= kerned {
		t.Errorf("AVAVAVAVAV is %v wide without kerning, %v with it", plain, kerned)
	}
	if w := width("kern=0, bogus, liga"); w != plain {
		t.Errorf("a list with an invalid tag is %v wide, not %v", w, plain)
	}
}

func TestParseFeatures(t *testing.T) {
	got := features(" tnum, liga=0 ,salt=2,toolong, bad=x, ss01")
	want := []feature{{[4]byte{'t', 'n', 'u', 'm'}, 1}, {[4]byte{'l', 'i', 'g', 'a'}, 0}, {[4]byte{'s', 'a', 'l', 't'}, 2}, {[4]byte{'s', 's', '0', '1'}, 1}}
	if !slices.Equal(got, want) {
		t.Errorf("features = %v, want %v", got, want)
	}
}

func TestSpans(t *testing.T) {
	s := newSystem()
	if err := s.RegisterFont(goregular.TTF, "MyGo Test Span"); err != nil {
		t.Fatal(err)
	}
	const text = "Small LARGE small"
	lay := func(spans ...Span) *Layout {
		return s.Layout(Params{Text: text, Style: Style{Size: 14}, Spans: EncodeSpans(spans)})
	}
	glyph := func(l *Layout, r int) Glyph {
		for _, g := range l.Lines[0].Glyphs {
			if g.Cluster == r {
				return g
			}
		}
		t.Fatalf("no glyph of rune %d", r)
		return Glyph{}
	}
	plain := lay()
	big := lay(Span{End: 6}, Span{End: 11, Size: 28, Weight: 700})
	if glyph(big, 0).Size != 14 || glyph(big, 7).Size != 28 || glyph(big, 13).Size != 14 {
		t.Errorf("glyph sizes %v, %v, %v", glyph(big, 0).Size, glyph(big, 7).Size, glyph(big, 13).Size)
	}
	if big.Height < plain.Height*1.5 || big.Width <= plain.Width {
		t.Errorf("a span twice the size leaves the text %v×%v, from %v×%v", big.Width, big.Height, plain.Width, plain.Height)
	}
	reg := lay(Span{End: 6}, Span{End: 11, Family: "MyGo Test Span"})
	want := s.Layout(Params{Text: "L", Style: Style{Size: 14, Family: "MyGo Test Span"}}).Lines[0].Glyphs[0].Font
	if glyph(reg, 7).Font != want || glyph(reg, 0).Font == want {
		t.Error("a span's family does not draw its runes, or draws the others")
	}
	// Truncated, the last line keeps its spans' styles.
	cut := s.Layout(Params{Text: text, Style: Style{Size: 14}, Width: glyph(big, 9).X, MaxLines: 1, Spans: EncodeSpans([]Span{{End: 6}, {End: 11, Size: 28}})})
	if !cut.Truncated || glyph(cut, 7).Size != 28 {
		t.Errorf("truncated: %v, the large run's size %v", cut.Truncated, glyph(cut, 7).Size)
	}
}

func TestEncodeSpans(t *testing.T) {
	spans := []Span{{End: 3, Family: "A, B", Size: 1.5, Weight: 600, Italic: true, LetterSpacing: -0.5, Features: "tnum", Lang: "ja"}, {End: 5, BaselineShift: -2.5}, {End: 7, SpacingSet: true, PaddingX: 3}, {End: 8, PaddingStart: 16}, {End: 9}}
	if got := DecodeSpans(EncodeSpans(spans)); !slices.Equal(got, spans) {
		t.Errorf("decoded %+v, want %+v", got, spans)
	}
	if got := DecodeSpans(EncodeSpans(spans)[:5]); len(got) != 0 {
		t.Errorf("a cut encoding decodes to %+v", got)
	}
	// The spans of a paragraph, relative to it.
	in := spansIn([]Span{{End: 3, Weight: 1}, {End: 8, Weight: 2}, {End: 12, Weight: 3}}, 5, 10)
	if want := []Span{{End: 3, Weight: 2}, {End: 5, Weight: 3}}; !slices.Equal(in, want) {
		t.Errorf("spansIn = %+v, want %+v", in, want)
	}
}

// TestSpanZeroLetterSpacing checks that a span's letter spacing of 0,
// once set, is its own over the text's, as CSS's letter-spacing: 0 on an
// inline box, and that a zero not set is the text's.
func TestSpanZeroLetterSpacing(t *testing.T) {
	s := newSystem()
	width := func(spacing float32, spans ...Span) float32 {
		return s.Layout(Params{Text: "LABEL 42", Style: Style{Size: 20, LetterSpacing: spacing}, Spans: EncodeSpans(spans)}).Width
	}
	plain, tracked := width(0), width(4)
	if tracked <= plain+20 {
		t.Fatalf("letter spacing of 4 leaves LABEL 42 %v wide, from %v", tracked, plain)
	}
	if w := width(4, Span{End: 6}, Span{End: 8}); w != tracked {
		t.Errorf("spans without a spacing are %v wide, the tracked text %v", w, tracked)
	}
	// The number keeps normal spacing: two letters' worth less.
	untracked := width(4, Span{End: 6}, Span{End: 8, SpacingSet: true})
	if d := tracked - untracked; d < 4*1-0.5 || d > 4*2+0.5 {
		t.Errorf("a span set to no spacing makes the text %v wide, %v tracked", untracked, tracked)
	}
	if w := width(0, Span{End: 6}, Span{End: 8, SpacingSet: true}); w != plain {
		t.Errorf("no spacing over none is %v wide, not %v", w, plain)
	}
}

// TestSpanPaddingStart checks a span's padding at its start alone: in
// left-to-right text its glyphs move in by it, and the text after it by it
// too, not twice; in right-to-left text the room is at the span's right,
// which is where it starts.
func TestSpanPaddingStart(t *testing.T) {
	s := newSystem()
	const pad = 16
	lay := func(text string, spans ...Span) *Layout {
		return s.Layout(Params{Text: text, Style: Style{Size: 14}, Spans: EncodeSpans(spans)})
	}
	x := func(l *Layout, r int) float32 {
		for _, g := range l.Lines[0].Glyphs {
			if g.Cluster == r {
				return g.X
			}
		}
		t.Fatalf("no glyph of rune %d", r)
		return 0
	}
	check := func(name, text string, spans []Span, moves [][2]int) {
		t.Helper()
		plain, padded := lay(text), lay(text, spans...)
		if d := padded.Width - plain.Width; math.Abs(float64(d-pad)) > 0.01 {
			t.Errorf("%s: a span padded at its start widens the text by %v, not %v", name, d, pad)
		}
		for _, c := range moves {
			if d := x(padded, c[0]) - x(plain, c[0]); math.Abs(float64(d-float32(c[1]))) > 0.01 {
				t.Errorf("%s: rune %d moves %v, not %d", name, c[0], d, c[1])
			}
		}
	}
	// "Ctrl K" padded: the room is at its left, before rune 4
	check("left to right", "See Ctrl K now", []Span{{End: 4}, {End: 10, PaddingStart: pad}, {End: 14}},
		[][2]int{{3, 0}, {4, pad}, {9, pad}, {10, pad}, {13, pad}})
	// "עולם" (runes 5-8) padded, laid out right to left: the room is at its
	// right, between it and the space and word before it, which are on its
	// right and move; it and what follows it, on its left, stay
	check("right to left", "שלום עולם זה", []Span{{End: 5}, {End: 9, PaddingStart: pad}, {End: 12}},
		[][2]int{{11, 0}, {8, 0}, {5, 0}, {4, pad}, {0, pad}})
}

// TestSpanPadding checks the horizontal padding of spans: each line's
// piece of a span takes it at both ends, moving its glyphs in and the
// text after it on, and lines break to fit it.
func TestSpanPadding(t *testing.T) {
	s := newSystem()
	const text = "See Ctrl K now"
	const pad = 6
	lay := func(width float32, maxLines int, spans ...Span) *Layout {
		return s.Layout(Params{Text: text, Style: Style{Size: 14}, Width: width, MaxLines: maxLines, Spans: EncodeSpans(spans)})
	}
	chip := []Span{{End: 4}, {End: 10, PaddingX: pad}, {End: 14}}
	x := func(l *Layout, r int) float32 {
		for _, line := range l.Lines {
			for _, g := range line.Glyphs {
				if g.Cluster == r {
					return g.X
				}
			}
		}
		t.Fatalf("no glyph of rune %d", r)
		return 0
	}
	plain, padded := lay(0, 0), lay(0, 0, chip...)
	if d := padded.Width - plain.Width; math.Abs(float64(d-2*pad)) > 0.01 {
		t.Errorf("a padded span widens the text by %v, not %v", d, 2*pad)
	}
	for _, c := range []struct{ r, by int }{{0, 0}, {3, 0}, {4, pad}, {9, pad}, {10, 2 * pad}, {13, 2 * pad}} {
		if d := x(padded, c.r) - x(plain, c.r); math.Abs(float64(d-float32(c.by))) > 0.01 {
			t.Errorf("rune %d moves %v, not %d", c.r, d, c.by)
		}
	}
	// Wide enough for the text but not its padding, the line breaks
	// before what does not fit.
	fit := x(plain, 10) + 1
	if l := lay(fit, 0); len(l.Lines) != 2 || l.Lines[0].End != 11 {
		t.Fatalf("unpadded at %v: %d lines, the first to %d", fit, len(l.Lines), l.Lines[0].End)
	}
	wrapped := lay(fit, 0, chip...)
	if len(wrapped.Lines) < 2 {
		t.Fatalf("padded at %v: %d lines", fit, len(wrapped.Lines))
	}
	for i, line := range wrapped.Lines {
		if line.Width > fit+0.01 {
			t.Errorf("line %d is %v wide, past %v", i, line.Width, fit)
		}
	}
	// A span that wraps pads its piece of each line: the clone of
	// box-decoration-break.
	long := []Span{{End: 4}, {End: 14, PaddingX: pad}}
	at := x(plain, 9) + 1
	unpadded, split := lay(at, 0), lay(at+2*pad, 0, long...)
	if len(unpadded.Lines) != 2 || len(split.Lines) != 2 || split.Lines[0].End != unpadded.Lines[0].End {
		t.Fatalf("lines %d and %d, the first to %d and %d", len(unpadded.Lines), len(split.Lines), unpadded.Lines[0].End, split.Lines[0].End)
	}
	for i := range split.Lines {
		if d := split.Lines[i].Width - unpadded.Lines[i].Width; math.Abs(float64(d-2*pad)) > 0.01 {
			t.Errorf("line %d is %v wider padded, not %v", i, d, 2*pad)
		}
	}
	second := split.Lines[1].Start
	if d := x(split, second) - x(unpadded, second); math.Abs(float64(d-pad)) > 0.01 {
		t.Errorf("the second line's first glyph moves %v, not %d", d, pad)
	}
	// Truncated, the ellipsis and the padding fit the width.
	if cut := lay(fit, 1, chip...); !cut.Truncated || cut.Width > fit+0.01 {
		t.Errorf("truncated at %v: %v, %v wide", fit, cut.Truncated, cut.Width)
	}
	// Justified, padded lines stretch to the width, not past it.
	s.UseJustify()
	j := s.Layout(Params{Text: text, Style: Style{Size: 14}, Width: fit, Justify: true, Spans: EncodeSpans(chip)})
	if w := j.Lines[0].Width; len(j.Lines) > 1 && math.Abs(float64(w-fit)) > 0.01 {
		t.Errorf("a justified padded line is %v wide, not %v", w, fit)
	}
}

func TestSpansAcrossParagraphs(t *testing.T) {
	s := newSystem()
	l := s.Layout(Params{Text: "ab\ncd", Style: Style{Size: 14}, Spans: EncodeSpans([]Span{{End: 1}, {End: 4, Size: 30}})})
	sizes := map[int]float32{}
	for _, line := range l.Lines {
		for _, g := range line.Glyphs {
			sizes[g.Cluster] = g.Size
		}
	}
	if sizes[0] != 14 || sizes[1] != 30 || sizes[3] != 30 || sizes[4] != 14 {
		t.Errorf("sizes by rune %v", sizes)
	}
}

// TestDigitFeatures picks proportional or tabular figures of the system's
// font, where it has both.
func TestDigitFeatures(t *testing.T) {
	s := newSystem()
	width := func(features string) float32 {
		return s.Layout(Params{Text: "1111111111", Style: Style{Size: 40, Features: features}}).Width
	}
	tabular, proportional := width("tnum"), width("pnum")
	if tabular == proportional {
		t.Skip("the system's font has digits of one kind")
	}
	if proportional >= tabular {
		t.Errorf("proportional ones are %v wide, tabular ones %v", proportional, tabular)
	}
}

// TestFontListFallback draws what the first family of a list lacks with
// the next family that has it, before the system's choice.
func TestFontListFallback(t *testing.T) {
	second := map[string]string{"windows": "MS Gothic", "darwin": "Hiragino Mincho ProN", "linux": "Noto Serif CJK JP"}[runtime.GOOS]
	if second == "" {
		// Browsers fall back within a list themselves (package text's
		// canvas engine), and their fonts vary.
		t.Skip("no font of another family to fall back to on " + runtime.GOOS)
	}
	s := newSystem()
	if err := s.RegisterFont(goregular.TTF, "MyGo Latin"); err != nil {
		t.Fatal(err)
	}
	// drawn tells fonts apart by what they draw, as engines may make a
	// Font of one face for each family list.
	drawn := func(text, family string, rune int) [4]float32 {
		for _, g := range s.Layout(Params{Text: text, Style: Style{Size: 20, Family: family}}).Lines[0].Glyphs {
			if g.Cluster == rune {
				return [4]float32{float32(g.ID), g.Advance, g.Font.Ascent, g.Font.Descent}
			}
		}
		return [4]float32{}
	}
	want := drawn("日", second, 0)
	// Without a font that has it, engines draw a missing glyph: 0, or one
	// of Pango's PANGO_GLYPH_UNKNOWN_FLAG.
	if id := uint32(want[0]); id == 0 || id&0x10000000 != 0 || want == drawn("日", "MyGo Latin", 0) {
		t.Skipf("%s is missing, or what the system falls back to", second)
	}
	list := "MyGo Latin, " + second
	if got := drawn("A日", list, 1); got != want {
		t.Errorf("日 in %q is not drawn with %s", list, second)
	}
	if drawn("A日", list, 0) != drawn("A", "MyGo Latin", 0) {
		t.Errorf("A in %q is not drawn with MyGo Latin", list)
	}
}

// TestFontsOfManySizes lets go of the fonts of sizes once they are many,
// as with an animated size, and draws on.
func TestFontsOfManySizes(t *testing.T) {
	s := newSystem()
	f, ok := s.engine().(fontForgetter)
	if !ok {
		t.Skip("the engine keeps no fonts")
	}
	for i := range 2 * maxFonts {
		l := s.Layout(Params{Text: "Hi", Style: Style{Size: 10 + float32(i)/10}})
		g := l.Lines[0].Glyphs[0]
		img := s.Glyph(g.Font, g.ID, 1, 0, 0, false)
		if !img.OK && s.Full() {
			s.MakeRoom() // as frames do once the atlas fills
			img = s.Glyph(g.Font, g.ID, 1, 0, 0, false)
		}
		if !img.OK {
			t.Fatalf("no glyph at size %v", g.Size)
		}
		s.EndFrame()
		if n := f.fontCount(); n > maxFonts {
			t.Fatalf("%d fonts after a frame", n)
		}
	}
}

// TestLayoutKeptAcrossFontFlush draws a layout kept while the System lets
// go of its fonts, as a cross-fade draws the text it fades out (#982): its
// fonts still draw it as before.
func TestLayoutKeptAcrossFontFlush(t *testing.T) {
	s := newSystem()
	f, ok := s.engine().(fontForgetter)
	if !ok {
		t.Skip("the engine keeps no fonts")
	}
	kept := s.Layout(Params{Text: "Saved", Style: Style{Size: 13}})
	before := layoutInk(t, s, kept)
	for i := 0; f.fontCount() <= maxFonts; i++ {
		s.Layout(Params{Text: "Hi", Style: Style{Size: 20 + float32(i)/10}})
	}
	s.EndFrame()
	if f.fontCount() != 0 {
		t.Fatalf("%d fonts after letting go of them", f.fontCount())
	}
	// The fonts nothing refers to are released.
	runtime.GC()
	s.EndFrame()
	if after := layoutInk(t, s, kept); after != before {
		t.Fatalf("the kept layout draws %d of ink, %d before", after, before)
	}
}

// layoutInk draws a layout's glyphs and returns their coverage.
func layoutInk(t *testing.T, s *System, l *Layout) int {
	t.Helper()
	ink := 0
	for _, g := range l.Lines[0].Glyphs {
		img := s.Glyph(g.Font, g.ID, 2, 0, 0, false)
		if !img.OK && s.Full() {
			s.MakeRoom()
			img = s.Glyph(g.Font, g.ID, 2, 0, 0, false)
		}
		if !img.OK || img.inColor() {
			t.Fatalf("glyph %v: %+v", g.ID, img)
		}
		for y := range int(img.H) {
			for x := range int(img.W) {
				ink += int(s.MaskAtlas.Pix[(int(img.Y)+y)*s.MaskAtlas.W+int(img.X)+x])
			}
		}
	}
	return ink
}

// releasingEngine gives its Fonts native fonts, numbered, and records
// which it released, as the system's engines release theirs.
type releasingEngine struct {
	stubEngine
	nativeFonts
	made     uintptr
	released map[uintptr]bool
}

func (e *releasingEngine) font(style Style) *Font {
	f := e.stubEngine.font(style)
	if f.native == 0 {
		e.made++
		f.native = e.made
		e.own(f)
	}
	return f
}

func (e *releasingEngine) shape(text []rune, style Style, spans []Span, width float32, rtl, wholeWords bool) []shapedLine {
	e.font(style) // the stub's shape finds it among its fonts
	return e.stubEngine.shape(text, style, spans, width, rtl, wholeWords)
}

func (e *releasingEngine) fontCount() int { return len(e.fonts) }

func (e *releasingEngine) forgetFonts() { clear(e.fonts) }

func (e *releasingEngine) releaseFonts() {
	e.releaseDead(func(native uintptr) { e.released[native] = true })
}

// TestFontOfKeptLayoutOutlivesFlush keeps a layout while the System lets
// go of its fonts (#982): the native font of the layout's Font stays for
// as long as the layout refers to it, and goes after, as the others do.
func TestFontOfKeptLayoutOutlivesFlush(t *testing.T) {
	s := newSystem()
	e := &releasingEngine{released: map[uintptr]bool{}}
	s.eng = e
	kept := s.Layout(Params{Text: "Saved", Style: Style{Size: 13}})
	native := kept.Lines[0].Glyphs[0].Font.native
	for i := 0; e.fontCount() <= maxFonts; i++ {
		s.Layout(Params{Text: "Hi", Style: Style{Size: 20 + float32(i)/10}})
	}
	s.EndFrame()
	if e.fontCount() != 0 || len(e.released) != 0 {
		t.Fatalf("%d fonts kept, %d released as the System let go of them", e.fontCount(), len(e.released))
	}
	wait.Until(t, "the fonts nothing refers to released", func() bool {
		runtime.GC()
		s.EndFrame()
		return len(e.released) == int(e.made)-1
	})
	if e.released[native] {
		t.Fatal("the font of the kept layout was released")
	}
	if got := kept.Lines[0].Glyphs[0].Font.native; got != native {
		t.Fatalf("the kept layout's font is %d, was %d", got, native)
	}
	runtime.KeepAlive(kept)
	wait.Until(t, "the kept layout's font released", func() bool {
		runtime.GC()
		s.EndFrame()
		return e.released[native]
	})
}

// TestPlacement checks that glyphs go to the positions within a pixel the
// system's text stack draws them at, and baselines to its whole pixels:
// Core Graphics' two positions a pixel at 26 pixels an em, left of the pen,
// and baselines down to the next pixel; GTK's whole pixels.
func TestPlacement(t *testing.T) {
	s := newSystem()
	l := s.Layout(Params{Text: "l", Style: Style{Size: 13}})
	g := l.Lines[0].Glyphs[0]
	at := func(x float32) GlyphImage { return s.Glyph(g.Font, g.ID, 2, x, 0, false) }
	same := func(a, b GlyphImage) bool { return a.X == b.X && a.Y == b.Y && a.Left == b.Left }
	switch runtime.GOOS {
	case "darwin":
		if !same(at(10), at(10.49)) || same(at(10), at(10.5)) {
			t.Error("not two positions a pixel")
		}
		if b := s.Baseline(20.01); b != 21 {
			t.Errorf("baseline 20.01 drawn at %v", b)
		}
	case "linux":
		// Pango rounds glyph positions to the nearest whole pixel, as GTK.
		if !same(at(10), at(10.49)) || same(at(10), at(10.5)) {
			t.Error("not two positions a pixel")
		}
		if b := s.Baseline(20.9); b != 20 {
			t.Errorf("baseline 20.9 drawn at %v", b)
		}
	case "windows":
		if b := s.Baseline(20.6); b != 21 {
			t.Errorf("baseline 20.6 drawn at %v", b)
		}
	}
}

// TestDecorate checks underlines and strikethroughs: below and above the
// baseline, at least a fraction of a pixel thick, from the text's start,
// on whole pixels with a thickness of the app's, and on macOS where AppKit draws them for the system font at 13 points,
// skipping the ink of descenders.
func TestDecorate(t *testing.T) {
	s := newSystem()
	l := s.Layout(Params{Text: "Hello", Style: Style{Size: 13}})
	under := s.Decorate(l, 0, 0, len(l.Lines[0].Glyphs), 0, 0, 1, Underline, LineStyle{})
	strike := s.Decorate(l, 0, 0, len(l.Lines[0].Glyphs), 0, 0, 1, Strikethrough, LineStyle{})
	if len(under) != 1 || len(strike) != 1 {
		t.Fatalf("%d underlines, %d strikethroughs", len(under), len(strike))
	}
	base := s.Baseline(l.Lines[0].Baseline)
	u, st := under[0], strike[0]
	if u.Top < base || u.Bottom <= u.Top || st.Bottom > base || st.Bottom <= st.Top || u.X0 > 1 || u.X1 < l.Width-1 {
		t.Errorf("underline %+v, strikethrough %+v, baseline %v, width %v", u, st, base, l.Width)
	}
	// Lines of a thickness of the app's cover whole pixels, wherever the
	// text is.
	for _, scale := range []float32{1, 1.5} {
		for _, d := range []Decoration{Underline, Strikethrough} {
			for _, st := range s.Decorate(l, 0, 0, len(l.Lines[0].Glyphs), 0, 0.3, scale, d, LineStyle{Thick: 3}) {
				if st.Bottom-st.Top != float32(math.Round(float64(3*scale))) || st.Top != float32(math.Floor(float64(st.Top))) {
					t.Errorf("a line 3 DIPs thick at scale %v: %+v", scale, st)
				}
			}
		}
	}
	if runtime.GOOS == "darwin" {
		if u.Top != base+1 || u.Bottom != base+2 || st.Top != base-4 || st.Bottom != base-3 {
			t.Errorf("underline %+v, strikethrough %+v from baseline %v", u, st, base)
		}
		l = s.Layout(Params{Text: "gypsy", Style: Style{Size: 24}})
		if n := len(s.Decorate(l, 0, 0, len(l.Lines[0].Glyphs), 0, 0, 2, Underline, LineStyle{})); n < 3 {
			t.Errorf("an underline of gypsy in %d pieces", n)
		}
	}
	// A solid underline stays whole through descenders, where the
	// underline is, on every system.
	l = s.Layout(Params{Text: "gypsy/url", Style: Style{Size: 24}})
	solid := s.Decorate(l, 0, 0, len(l.Lines[0].Glyphs), 0, 0, 2, SolidUnderline, LineStyle{})
	plain := s.Decorate(l, 0, 0, len(l.Lines[0].Glyphs), 0, 0, 2, Underline, LineStyle{})
	if len(solid) != 1 || len(plain) == 0 || solid[0].Top != plain[0].Top || solid[0].Bottom != plain[0].Bottom {
		t.Errorf("solid underline %+v, underline %+v", solid, plain)
	}
}

// TestUnderlineOffset checks text-underline-offset: the top of the
// underline that many DIPs below the baseline, on a whole pixel, as thick
// as without it, a strikethrough where it was, and an underline low
// enough to clear the descenders skipping none of them.
func TestUnderlineOffset(t *testing.T) {
	s := newSystem()
	l := s.Layout(Params{Text: "Hello", Style: Style{Size: 13}})
	n := len(l.Lines[0].Glyphs)
	// Solid underlines, which Core Text does not cut where they cross the
	// ink of the glyphs at their offset: where they are is the same.
	for _, scale := range []float32{1, 1.5, 2} {
		for _, y := range []float32{0, 0.3} {
			base := s.Baseline((y + l.Lines[0].Baseline) * scale)
			plain := s.Decorate(l, 0, 0, n, 0, y, scale, SolidUnderline, LineStyle{})
			for _, off := range []float32{0, 4, -3} {
				under := s.Decorate(l, 0, 0, n, 0, y, scale, SolidUnderline, LineStyle{Offset: off, HasOffset: true})
				if len(under) != len(plain) || len(under) == 0 {
					t.Fatalf("scale %v, offset %v: %d strokes, %d without", scale, off, len(under), len(plain))
				}
				want := base + float32(math.Round(float64(off*scale)))
				for k, st := range under {
					if st.Top != want || st.Bottom-st.Top != plain[k].Bottom-plain[k].Top || st.X0 != plain[k].X0 || st.X1 != plain[k].X1 {
						t.Errorf("scale %v, y %v, offset %v: %+v, want top %v as thick as %+v", scale, y, off, st, want, plain[k])
					}
				}
				thick := s.Decorate(l, 0, 0, n, 0, y, scale, SolidUnderline, LineStyle{Thick: 2, Offset: off, HasOffset: true})
				if thick[0].Top != want || thick[0].Bottom != want+float32(math.Round(float64(2*scale))) {
					t.Errorf("scale %v, offset %v, 2 thick: %+v, want top %v", scale, off, thick[0], want)
				}
			}
			strike := s.Decorate(l, 0, 0, n, 0, y, scale, Strikethrough, LineStyle{})
			moved := s.Decorate(l, 0, 0, n, 0, y, scale, Strikethrough, LineStyle{Offset: 4, HasOffset: true})
			if len(strike) != len(moved) || len(strike) > 0 && strike[0] != moved[0] {
				t.Errorf("an offset moved a strikethrough: %+v, was %+v", moved, strike)
			}
		}
	}
	if runtime.GOOS == "darwin" {
		// An underline skipping ink skips the glyphs where its offset puts
		// it: through the letters, it is cut.
		ls := LineStyle{Offset: -3, HasOffset: true}
		var cut, whole float32
		for _, st := range s.Decorate(l, 0, 0, n, 0, 0, 2, Underline, ls) {
			cut += st.X1 - st.X0
		}
		for _, st := range s.Decorate(l, 0, 0, n, 0, 0, 2, SolidUnderline, ls) {
			whole += st.X1 - st.X0
		}
		if cut >= whole {
			t.Errorf("an underline through Hello is %v wide, as wide as a solid one, %v", cut, whole)
		}
		// Below the descenders, an underline skipping ink is whole.
		l = s.Layout(Params{Text: "gypsy", Style: Style{Size: 24}})
		if n := len(s.Decorate(l, 0, 0, len(l.Lines[0].Glyphs), 0, 0, 2, Underline, LineStyle{Offset: 10, HasOffset: true})); n != 1 {
			t.Errorf("an underline of gypsy 10 points down in %d pieces", n)
		}
	}
}

func TestLangRuns(t *testing.T) {
	for _, c := range []struct {
		text, base string
		spans      []Span
		want       []langRun
	}{
		{"abc 直", "", nil, nil},
		{"abc 直", "zh-Hans", nil, nil},
		{"abc", "en", nil, nil},
		// Han characters, and their punctuation, in a language that does
		// not write them keep the engine's own language.
		{"ab直骨。c", "en", nil, []langRun{{2, 5, ""}}},
		{"ab直c", "en", []Span{{End: 2}, {End: 3, Lang: "ja"}}, []langRun{{2, 3, "ja"}}},
		{"ab直c", "", []Span{{End: 1, Lang: "fr"}}, []langRun{{0, 1, "fr"}}},
		{"直直", "ja", []Span{{End: 1, Lang: "zh-Hans"}}, []langRun{{0, 1, "zh-Hans"}}},
		{"a直", "", []Span{{End: 2, Lang: "en"}}, []langRun{{0, 1, "en"}}},
	} {
		if got := langRuns([]rune(c.text), c.base, c.spans); !slices.Equal(got, c.want) {
			t.Errorf("langRuns(%q, %q, %+v) = %+v, want %+v", c.text, c.base, c.spans, got, c.want)
		}
	}
	// The Han characters langRuns leaves in the engine's own language.
	for _, c := range []struct {
		text, base string
		spans      []Span
		want       []langRun
	}{
		{"abc", "", nil, nil},
		{"直", "zh-Hans", nil, nil},
		{"ab直骨。c", "", nil, []langRun{{2, 5, ""}}},
		{"ab直骨。c", "en", nil, []langRun{{2, 5, ""}}},
		{"直 直", "en", nil, []langRun{{0, 1, ""}, {2, 3, ""}}},
		{"ab直c直", "en", []Span{{End: 2}, {End: 3, Lang: "ja"}}, []langRun{{4, 5, ""}}},
		{"直直", "ja", []Span{{End: 1, Lang: "fr"}}, []langRun{{0, 1, ""}}},
	} {
		if got := userHanRuns([]rune(c.text), c.base, c.spans); !slices.Equal(got, c.want) {
			t.Errorf("userHanRuns(%q, %q, %+v) = %+v, want %+v", c.text, c.base, c.spans, got, c.want)
		}
	}
	for _, c := range []struct {
		locale    string
		preferred []string
		want      string
	}{
		{"en-US", []string{"en-US", "zh-Hans-CN"}, "zh-Hans-CN"},
		{"en-US", []string{"en-GB", "ja-JP", "zh-Hans-CN"}, "ja-JP"},
		{"en-US", []string{"en-US"}, ""},
		{"en-US", nil, ""},
		{"zh-CN", []string{"en-US", "ja-JP"}, ""},
		{"en-US", []string{"en-US", "zh-Hant-TW", "zh-Hans-CN"}, "zh-Hant-TW"},
		{"en-GB", []string{"en-GB", "ko-KR", "ja-JP"}, "ko-KR"},
		{"de-DE", []string{"fr-FR", "ja", "zh-Hans"}, "ja"},
		{"ja-JP", []string{"en-US", "zh-Hans-CN"}, ""},
	} {
		if got := userHanLanguage(c.locale, c.preferred); got != c.want {
			t.Errorf("userHanLanguage(%q, %q) = %q, want %q", c.locale, c.preferred, got, c.want)
		}
	}
	for _, s := range [][]uint16{nil, {0}, {0, 0}} {
		if got := splitMultiString(s); got != nil {
			t.Errorf("splitMultiString(%v) = %q", s, got)
		}
	}
	for _, c := range []struct {
		list string
		want []string
	}{
		{"en-US\x00zh-Hans-CN\x00\x00", []string{"en-US", "zh-Hans-CN"}},
		{"en-US\x00\x00\x00\x00", []string{"en-US"}},
		{"ja-JP\x00ko", []string{"ja-JP", "ko"}},
	} {
		if got := splitMultiString(utf16.Encode([]rune(c.list))); !slices.Equal(got, c.want) {
			t.Errorf("splitMultiString(%q) = %q, want %q", c.list, got, c.want)
		}
	}
}

// Windows lists the user's display languages apart from the languages of
// their profile: English Windows with Simplified Chinese second among the
// languages of Settings, without its language pack, has en-US alone for
// the first and en-US and zh-Hans-CN for the second (#1479). Han
// characters take the first list's first language that writes them, else
// the second's, and a list is read only when needed.
func TestResolveUserHanLanguage(t *testing.T) {
	for _, c := range []struct {
		locale       string
		ui, profile  []string
		want         string
		readsUI      bool
		readsProfile bool
	}{
		// #1479: the display languages have no Chinese, the profile has.
		{"en-US", []string{"en-US"}, []string{"en-US", "zh-Hans-CN"}, "zh-Hans-CN", true, true},
		// #1454: the display languages have Chinese already.
		{"en-US", []string{"en-US", "zh-Hans-CN"}, []string{"en-US", "ja-JP", "zh-Hans-CN"}, "zh-Hans-CN", true, false},
		{"en-US", nil, []string{"en-GB", "zh-Hant-TW", "zh-Hans-CN"}, "zh-Hant-TW", true, true},
		{"en-US", []string{"en-US"}, []string{"en-US", "ja-JP", "zh-Hans-CN"}, "ja-JP", true, true},
		{"en-US", []string{"en-US"}, []string{"en-US", "ko-KR", "ja-JP"}, "ko-KR", true, true},
		// No list has a language writing Han characters, or none is read.
		{"en-US", []string{"en-US"}, []string{"en-US", "fr-FR"}, "", true, true},
		{"en-US", nil, nil, "", true, true},
		// A locale writing Han characters reads no list.
		{"zh-CN", []string{"en-US"}, []string{"en-US", "ja-JP"}, "", false, false},
		{"ja-JP", nil, []string{"en-US", "zh-Hans-CN"}, "", false, false},
	} {
		var readUI, readProfile bool
		got := resolveUserHanLanguage(c.locale,
			func() []string { readUI = true; return c.ui },
			func() []string { readProfile = true; return c.profile })
		if got != c.want || readUI != c.readsUI || readProfile != c.readsProfile {
			t.Errorf("resolveUserHanLanguage(%q, %q, %q) = %q, reading the display languages %v and the profile's %v; want %q, %v and %v",
				c.locale, c.ui, c.profile, got, readUI, readProfile, c.want, c.readsUI, c.readsProfile)
		}
	}
	if got := resolveUserHanLanguage("en-US"); got != "" {
		t.Errorf("resolveUserHanLanguage without lists = %q", got)
	}
	for lang, want := range map[string]bool{"zh-Hant": true, "ja_JP": true, "ko": true, "yue": true, "en": false, "vi": false, "": false} {
		if writesHan(lang) != want {
			t.Errorf("writesHan(%q) = %v", lang, !want)
		}
	}
}

// TestSetLanguage holds the system's language to a new generation of
// layouts, which the layouts of text without a language are made anew in.
func TestSetLanguage(t *testing.T) {
	s := newSystem()
	p := Params{Text: "abc", Style: Style{Size: 14}}
	before, gen := s.Layout(p), s.Generation()
	s.SetLanguage("ja")
	if s.Generation() == gen || s.Language() != "ja" {
		t.Fatal("SetLanguage kept the generation")
	}
	after := s.Layout(p)
	if after == before || after.Params != p {
		t.Errorf("the layout of text without a language was kept, or took other parameters: %+v", after.Params)
	}
	gen = s.Generation()
	s.SetLanguage("ja")
	if s.Generation() != gen {
		t.Error("the same language made a new generation")
	}
}

// A flex item gives text exactly the width of its unconstrained layout.
// That width sums advances in float32, which may round below the
// engine's own line width: on Core Text, 20pt semibold "Action required:
// Complete your Artifact Signing identity validation" measured 599.78
// and wrapped its last word at that width.
func TestLayoutAtMeasuredWidth(t *testing.T) {
	s := Shared()
	texts := []string{
		"Action required: Complete your Artifact Signing identity validation",
		"The quick brown fox jumps over the lazy dog",
		"Settings and Privacy",
		"Welcome back, your inbox is up to date",
	}
	for _, text := range texts {
		for _, weight := range []int{400, 500, 600, 700} {
			for size := float32(10); size <= 32; size += 0.5 {
				p := Params{Text: text, Style: Style{Size: size, Weight: weight}}
				p.Width = s.Layout(p).Width
				if l := s.Layout(p); len(l.Lines) != 1 {
					t.Errorf("%q, weight %d, size %g: at measured width %g, %d lines", text, weight, size, p.Width, len(l.Lines))
				}
			}
		}
	}
}

// TestLayoutUnwrappedAtWidths checks that a text laid out at widths its
// lines fit in is shaped once for every width, and lays out as it would at
// each, its lines where its alignment puts them.
func TestLayoutUnwrappedAtWidths(t *testing.T) {
	style := Style{Size: 14}
	for _, c := range []struct {
		name   string
		p      Params
		shares bool
	}{
		{"label", Params{Text: "Forest in the rain", Style: style}, true},
		{"lines", Params{Text: "Forest\nin the rain\n", Style: style}, true},
		{"indented", Params{Text: "Forest in the rain", Style: style, Indent: 12}, true},
		{"spans", Params{Text: "Forest in the rain", Style: style, Spans: EncodeSpans([]Span{{End: 6, Weight: 700}, {End: 18}})}, true},
		{"right to left", Params{Text: "שלום עולם", Style: style}, true},
		{"right to left at the end", Params{Text: "שלום עולם", Style: style, Align: End}, true},
		{"centered", Params{Text: "Forest in the rain", Style: style, Align: Center}, true},
		{"centered lines", Params{Text: "Forest\nin the rain", Style: style, Align: Center, Indent: 8}, true},
		{"at the right", Params{Text: "Forest in the rain", Style: style, Align: Right}, true},
		{"centered, not wrapped", Params{Text: "Forest in the rain, and the sea", Style: style, Align: Center, NoWrap: true}, true},
		{"no wrap", Params{Text: "Forest in the rain, and the sea", Style: style, NoWrap: true}, true},
		{"justified", Params{Text: "Forest in the rain", Style: style, Justify: true}, true},
		{"cut", Params{Text: "Forest in the rain", Style: style, MaxLines: 1}, false},
		{"wrapping", Params{Text: "Forest in the rain, and the sea at dusk", Style: style}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			// From the layout at the width before, as a box resizing lays
			// its text out, and from the text's unwrapped layout.
			for _, unwrapped := range []bool{false, true} {
				s := newSystem()
				if unwrapped {
					s.Layout(unwrappedParams(c.p))
				}
				var last *Layout
				var n uint64
				for _, w := range []float32{400, 300, 250.5, 200, 180} {
					p := c.p
					p.Width = w
					made := s.LayoutsMade()
					got := s.LayoutFrom(last, p)
					if unwrapped {
						got = s.Layout(p)
					}
					n += s.LayoutsMade() - made
					// Made anew by the same system, whose glyph IDs are the
					// same, as the browser's engine numbers glyphs as it meets
					// them.
					want := &s.layout(p).layout
					last = got
					if got.Params != p {
						t.Fatalf("at %v, the layout's parameters are %+v", w, got.Params)
					}
					if !sameLayout(got, want) {
						t.Fatalf("at %v, the layout differs from one made at that width:\n%+v\n%+v", w, got.Lines, want.Lines)
					}
					for i := range len(want.Runes) + 1 {
						gx, gy, _ := got.CaretAt(CaretPosition{Index: i})
						wx, wy, _ := want.CaretAt(CaretPosition{Index: i})
						if gx != wx || gy != wy {
							t.Fatalf("at %v, the caret at %d is at %v, %v, not %v, %v", w, i, gx, gy, wx, wy)
						}
					}
				}
				shaped := uint64(1)
				if unwrapped {
					shaped = 0
				}
				if c.shares && n != shaped {
					t.Errorf("unwrapped first %v, laid out %d times, not %d", unwrapped, n, shaped)
				}
			}
		})
	}
	// A text laid out at a width is shaped once, wrapping or not.
	s := newSystem()
	for _, p := range []Params{
		{Text: "Harbor lights at night", Style: style, Width: 40},
		{Text: "Lighthouse keepers", Style: style, Width: 400},
	} {
		made := s.LayoutsMade()
		s.Layout(p)
		if n := s.LayoutsMade() - made; n != 1 {
			t.Errorf("%q at %v laid out %d times", p.Text, p.Width, n)
		}
	}
}

// sameLayout reports whether a and b place the same glyphs on the same
// lines.
func sameLayout(a, b *Layout) bool {
	if a.Width != b.Width || a.Height != b.Height || a.Truncated != b.Truncated || !slices.Equal(a.Runes, b.Runes) || len(a.Lines) != len(b.Lines) {
		return false
	}
	for i := range a.Lines {
		x, y := &a.Lines[i], &b.Lines[i]
		if x.X != y.X || x.Y != y.Y || x.Width != y.Width || x.Height != y.Height || x.Baseline != y.Baseline ||
			x.Start != y.Start || x.End != y.End || x.RTL != y.RTL || len(x.Glyphs) != len(y.Glyphs) {
			return false
		}
		for j := range x.Glyphs {
			g, h := x.Glyphs[j], y.Glyphs[j]
			if g.ID != h.ID || g.X != h.X || g.Y != h.Y || g.Advance != h.Advance || g.Cluster != h.Cluster || g.Size != h.Size {
				return false
			}
		}
	}
	return true
}
