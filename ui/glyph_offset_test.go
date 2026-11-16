package ui

import (
	"image"
	"math"
	"testing"

	"github.com/egoist/mygo/internal/text"
)

// markAt is a glyph a text drew: its pen, and the baseline it sits on,
// in DIPs from the window's top-left corner, and how far below its line's
// baseline that is.
type markAt struct {
	font   *text.Font
	id     uint32
	pen, y float32
	dy     float32
}

// drawMarked draws s in black on white, 64 DIPs large, right to left, as
// way does, and returns the picture, the text system, and the glyphs of
// no advance it drew (the marks).
func drawMarked(t *testing.T, s, way string) (*image.RGBA, *text.System, []markAt) {
	t.Helper()
	const size, x, y = 64, 10, 90 // the baseline at y for the painters
	var e *node
	var marks []markAt
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Fill().Padding(10).AlignItems(Start).Background(RGB(255, 255, 255)).Children(func() {
			switch way {
			case "element":
				e = coreText(c, s).FontSize(size).TextColor(RGB(0, 0, 0)).Direction(RightToLeft)
			case "Glyphs":
				coreBox(c).Size(380, 120).Draw(func(p *Painter, r Rect) {
					gs := Shape(s, Font{Size: size})
					marks = marks[:0]
					for _, g := range gs {
						if g.Advance == 0 && g.font != nil {
							marks = append(marks, markAt{g.font, g.id, x + g.X, y + g.y, g.y})
						}
					}
					p.Glyphs(gs, x, y, RGB(0, 0, 0))
				})
			case "TextLine":
				coreBox(c).Size(380, 120).Draw(func(p *Painter, r Rect) {
					l := p.rt.text.Shape(text.Params{Text: s, Style: text.Style{Size: size}, NoWrap: true, RTL: true})
					line := &l.Lines[0]
					marks = marks[:0]
					for _, g := range line.Glyphs {
						if g.Advance == 0 && g.Font != nil {
							marks = append(marks, markAt{g.Font, g.ID, x + g.X, y + g.Y - line.Baseline, g.Y - line.Baseline})
						}
					}
					p.TextLine(l, x, y, func(int) Color { return RGB(0, 0, 0) })
				})
			}
		})
	}, 400, 140)
	if e != nil {
		if e.tl == nil || len(e.tl.Lines) == 0 {
			t.Fatalf("%q: no text laid out", s)
		}
		ox, oy := e.x+e.contentX(), e.y+e.contentY()
		line := &e.tl.Lines[0]
		for _, g := range line.Glyphs {
			if g.Advance == 0 && g.Font != nil {
				marks = append(marks, markAt{g.Font, g.ID, ox + g.X, oy + g.Y, g.Y - line.Baseline})
			}
		}
	}
	return tt.Image(), tt.h.textSystem(), marks
}

// differs reports whether the pixel at (x, y) of a and b differs, as
// where ink of one is not the other's.
func differs(a, b *image.RGBA, x, y int) bool {
	ca, cb := a.RGBAAt(x, y), b.RGBAAt(x, y)
	return math.Abs(float64(ca.R)-float64(cb.R)) > 48 || math.Abs(float64(ca.G)-float64(cb.G)) > 48 || math.Abs(float64(ca.B)-float64(cb.B)) > 48
}

// TestMarksDrawWhereShaped draws Arabic marks where shaping placed them
// off the baseline (OpenType's mark positioning): a fatha above a joined
// kaf, a kasra below a joined reh, not on the baseline of their line,
// where the fatha fell on the kaf's top stroke and the kasra into the
// reh's tail (#1925). Each mark's ink, what the text with marks has that
// the text without them has not, in the mark's columns, reaches as far up
// (a mark above) or down (a mark below) as its glyph's bitmap where the
// layout places it: in an element's text, and in text Painter.Glyphs and
// Painter.TextLine draw.
func TestMarksDrawWhereShaped(t *testing.T) {
	for _, way := range []string{"element", "Glyphs", "TextLine"} {
		img, sys, marks := drawMarked(t, "کَسرِه", way)
		bare, _, _ := drawMarked(t, "کسره", way)
		off := 0
		for _, m := range marks {
			if math.Abs(float64(m.dy)) < 3 {
				continue // no farther off the baseline than the tolerance below
			}
			off++
			gi := sys.Glyph(m.font, m.id, 1, m.pen, text.ShadeOf(0, 0, 0), true)
			if !gi.OK {
				t.Fatalf("%s: mark %d has no bitmap", way, m.id)
			}
			x0 := int(math.Floor(float64(m.pen))) + int(gi.Left)
			y0 := int(sys.Baseline(m.y)) + int(gi.Top)
			y1 := y0 + int(gi.H)
			top, bottom := -1, -1
			for y := range img.Bounds().Dy() {
				for x := x0; x < x0+int(gi.W); x++ {
					if differs(img, bare, x, y) {
						if top < 0 {
							top = y
						}
						bottom = y + 1
						break
					}
				}
			}
			switch {
			case top < 0:
				t.Errorf("%s: mark %d, %v off the baseline: no ink in columns %d to %d", way, m.id, m.dy, x0, x0+int(gi.W))
			case m.dy < 0 && (top < y0-1 || top > y0+2):
				t.Errorf("%s: mark %d, %v above the baseline: ink from row %d, want its bitmap's top, row %d", way, m.id, -m.dy, top, y0)
			case m.dy > 0 && (bottom > y1+1 || bottom < y1-2):
				t.Errorf("%s: mark %d, %v below the baseline: ink down to row %d, want its bitmap's bottom, row %d", way, m.id, m.dy, bottom, y1)
			}
		}
		if off == 0 {
			t.Logf("%s: the system's Arabic font puts no mark of %q off the baseline", way, "کَسرِه")
		}
	}
}

// TestBaselineShiftDrawsGlyphs raises the glyphs of a span with a
// BaselineShift, as the line has room for them, not only its objects
// and decorations.
func TestBaselineShiftDrawsGlyphs(t *testing.T) {
	const shift = 8
	var e *node
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Fill().Padding(10).AlignItems(Start).Background(RGB(255, 255, 255)).Children(func() {
			e = coreRichText(c, Span{Text: "l"}, Span{Text: "  "}, Span{Text: "l", BaselineShift: shift}).FontSize(32).TextColor(RGB(0, 0, 0))
		})
	}, 200, 100)
	if e == nil || e.tl == nil || len(e.tl.Lines) == 0 {
		t.Fatal("no text laid out")
	}
	img := tt.Image()
	ox := e.x + e.contentX()
	// top returns the first row with ink in the advance of the glyph of
	// rune i.
	top := func(i int) int {
		for _, g := range e.tl.Lines[0].Glyphs {
			if g.Cluster != i {
				continue
			}
			x0, x1 := int(math.Floor(float64(ox+g.X))), int(math.Ceil(float64(ox+g.X+g.Advance)))
			for y := range img.Bounds().Dy() {
				for x := x0; x < x1; x++ {
					if c := img.RGBAAt(x, y); c.R < 128 {
						return y
					}
				}
			}
		}
		return -1
	}
	plain, raised := top(0), top(3)
	if plain < 0 || raised < 0 {
		t.Fatalf("no ink: %d and %d", plain, raised)
	}
	if d := plain - raised; d < shift-1 || d > shift+1 {
		t.Errorf("the shifted l's ink starts %d rows above the other's, want %d", d, shift)
	}
}
