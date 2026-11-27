package ui

import (
	"strings"
	"testing"
)

// TestTruncated checks that Truncated reports the previous frame's cut of
// an element's text: false on the first frame, true the frame after
// MaxLines or SingleLine cut it, and false again once it fits.
func TestTruncated(t *testing.T) {
	long := strings.Repeat("A long name that does not fit ", 4)
	for _, tc := range []struct {
		name string
		cut  func(e Element) Element
	}{
		{"MaxLines", func(e Element) Element { return e.MaxLines(1) }},
		{"SingleLine", func(e Element) Element { return e.SingleLine() }},
		{"MaxLines(2)", func(e Element) Element { return e.MaxLines(2) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			width := float32(120)
			var cut, fits, plain, empty bool
			tt := NewTester(func(c *Context) {
				Column(c).Children(func() {
					cut = tc.cut(Text(c.Key("cut"), long)).Width(width).Truncated()
					fits = tc.cut(Text(c.Key("fits"), "Short")).Width(width).Truncated()
					plain = Text(c.Key("plain"), long).Width(width).Truncated()
					empty = Column(c.Key("box")).Width(width).Truncated()
				})
			}, 400, 300)
			if cut || fits || plain || empty {
				t.Fatalf("first frame: cut %v, fits %v, plain %v, empty %v; want all false", cut, fits, plain, empty)
			}
			tt.Frame()
			if !cut {
				t.Fatal("text cut in the previous frame is not Truncated")
			}
			if fits || plain || empty {
				t.Fatalf("fits %v, plain %v, empty %v; want false", fits, plain, empty)
			}
			// It stays as it was while nothing changes.
			for range 2 {
				tt.Frame()
				if !cut || fits {
					t.Fatalf("rebuilt: cut %v, fits %v", cut, fits)
				}
			}
			width = 2000
			tt.Frame() // lays out at the new width
			tt.Frame()
			if cut {
				t.Fatal("text that fits at its new width is still Truncated")
			}
			width = 120
			tt.Frame()
			tt.Frame()
			if !cut {
				t.Fatal("text narrowed again is not Truncated")
			}
		})
	}
}

// TestTruncatedInside checks that an element without text of its own
// reports text cut inside it, as a Button's label, and not the text of a
// sibling or of hidden text.
func TestTruncatedInside(t *testing.T) {
	label := "Export everything to a folder"
	var narrow, wide, row, sibling, hidden bool
	tt := NewTester(func(c *Context) {
		Column(c).Gap(8).Children(func() {
			b := Button(c.Key("narrow"), label).Width(60)
			if narrow = b.Truncated(); narrow {
				b.Tooltip(label)
			}
			wide = Button(c.Key("wide"), label).Truncated()
			row = Row(c.Key("row")).Children(func() {
				Row(c.Key("inner")).Width(50).Children(func() {
					Text(c, label).SingleLine().MinWidth(0)
				})
			}).Truncated()
			sibling = Text(c.Key("sibling"), "OK").Truncated()
			hidden = Row(c.Key("hidden")).Width(50).Children(func() {
				Text(c, label).SingleLine().MinWidth(0).Hidden(true)
			}).Truncated()
		})
	}, 400, 300)
	tt.Frame()
	if !narrow {
		t.Fatal("a button whose label is cut is not Truncated")
	}
	if wide || sibling || hidden {
		t.Fatalf("wide %v, sibling %v, hidden %v; want false", wide, sibling, hidden)
	}
	if !row {
		t.Fatal("text cut two levels down does not make the row Truncated")
	}
}
