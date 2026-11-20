package markdown

import (
	"image"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
)

var (
	asideRed  = ui.RGB(230, 0, 0)
	themeBlue = ui.RGB(0, 0, 230)
)

// asides colors the links to destinations ending in "aside" red, while
// *on.
func asides(on *bool) func(dest string) (Color, bool) {
	return func(dest string) (Color, bool) {
		return asideRed, *on && strings.HasSuffix(dest, "aside")
	}
}

// spanColors returns the colors of the spans of block i, by text, and
// those of the links they are in, as the view gives the link elements.
func spanColors(st *viewState, i int) (spans, links map[string]ui.Color) {
	spans, links = map[string]ui.Color{}, map[string]ui.Color{}
	for _, pc := range st.paras[i].p.pieces {
		for _, s := range pc.spans {
			spans[s.Text], links[s.Text] = s.Color, pc.color
		}
	}
	return spans, links
}

// TestLinkColor colors the text of the links LinkColor picks, a
// mention's, a reference's and code's included, and their links alike,
// and leaves the others Theme.Link's.
func TestLinkColor(t *testing.T) {
	on := true
	src := "[aside](x:aside) [plain](x:plain) @octocat #12 `fn`\n"
	o := &Options{
		LinkColor: asides(&on),
		Resolve: func(in parser.Inline) (string, bool) {
			if in.Kind == parser.Mention {
				return "/users/aside", true
			}
			return "x:ref-aside", true
		},
		CodeLink: func(code string) (string, bool) { return "x:code-aside", true },
	}
	_, get := rvView(&src, o)
	spans, links := spanColors(get(), 1)
	for text, want := range map[string]ui.Color{"aside": asideRed, "@octocat": asideRed, "#12": asideRed, "fn": asideRed, "plain": {}} {
		if spans[text] != want || links[text] != want {
			t.Errorf("%q colored %v in a link of %v, want %v", text, spans[text], links[text], want)
		}
	}
}

// TestLinkColorNone leaves Theme.Link to the links given a color of
// alpha 0, code links as the others, and to a link still being written,
// whose destination is not yet whole.
func TestLinkColorNone(t *testing.T) {
	src := "[aside](x:aside) `fn` [writ](x:other"
	o := &Options{
		LinkColor: func(dest string) (Color, bool) {
			if dest == "x:aside" {
				return ui.Color{R: 230}, true
			}
			return asideRed, true
		},
		CodeLink: func(code string) (string, bool) { return "x:aside", true },
	}
	var st *viewState
	tt := ui.NewTester(func(c *ui.Context) { st = *ui.Local(Stream(c, src, o), stateKey{}, newState) }, 700, 400)
	// Revealed, not fading in.
	tt.Advance(10 * time.Second)
	spans, links := spanColors(st, 1)
	for _, text := range []string{"aside", "fn", "writ"} {
		if spans[text] != (Color{}) || links[text] != (Color{}) {
			t.Errorf("%q colored %v in a link of %v, want Theme.Link's", text, spans[text], links[text])
		}
	}
}

// colorPixels counts the pixels of img near c in r.
func colorPixels(img *image.RGBA, r ui.Rect, c ui.Color) int {
	n := 0
	for y := int(r.Y); y < int(r.Y+r.H); y++ {
		for x := int(r.X); x < int(r.X+r.W); x++ {
			p := img.RGBAAt(x, y)
			if absDiff(p.R, c.R) < 40 && absDiff(p.G, c.G) < 40 && absDiff(p.B, c.B) < 40 {
				n++
			}
		}
	}
	return n
}

// tintedPixels counts the pixels of img in r that are c drawn over white
// with at least a quarter of its coverage: an underline a fraction of a
// pixel thick, as Pango's metrics are before 1.52 (Ubuntu 22.04, Debian
// 12), draws faint, as GTK draws it.
func tintedPixels(img *image.RGBA, r ui.Rect, c ui.Color) int {
	n := 0
	for y := int(r.Y); y < int(r.Y+r.H); y++ {
		for x := int(r.X); x < int(r.X+r.W); x++ {
			p := img.RGBAAt(x, y)
			ch := [3][2]float64{{float64(p.R), float64(c.R)}, {float64(p.G), float64(c.G)}, {float64(p.B), float64(c.B)}}
			a := 0.0
			for _, v := range ch {
				if 255-v[1] > 64 {
					a = max(a, (255-v[0])/(255-v[1]))
				}
			}
			ok := a >= 0.25
			for _, v := range ch {
				ok = ok && math.Abs(v[0]-(255-a*(255-v[1]))) <= 24
			}
			if ok {
				n++
			}
		}
	}
	return n
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

// TestLinkColorDraws draws a colored link, its underline on hover
// included, in its color, and the others in Theme.Link, as the theme
// and LinkColor's answers change.
func TestLinkColorDraws(t *testing.T) {
	on := true
	o := &Options{Theme: &Theme{Link: themeBlue}, LinkColor: asides(&on)}
	tt := ui.NewTester(func(c *ui.Context) { View(c, "[aside](x:aside) [plain](x:plain)", o) }, 300, 80)
	aside, ok := tt.Find("aside")
	plain, ok2 := tt.Find("plain")
	if !ok || !ok2 {
		t.Fatalf("no links in %q", tt.Texts())
	}
	// Below the text, where the underline goes.
	under := ui.Rect{X: aside.X, Y: aside.Y + aside.H*0.75, W: aside.W, H: aside.H * 0.5}
	check := func(when string, asideIn, plainIn ui.Color) {
		t.Helper()
		img := tt.Image()
		if colorPixels(img, aside, asideIn) == 0 || colorPixels(img, plain, plainIn) == 0 {
			t.Errorf("%s: aside has %d pixels in %v, plain %d in %v", when, colorPixels(img, aside, asideIn), asideIn, colorPixels(img, plain, plainIn), plainIn)
		}
	}
	check("at first", asideRed, themeBlue)
	before := tintedPixels(tt.Image(), under, asideRed)
	tt.Move(aside.X+aside.W/2, aside.Y+aside.H/2)
	if after := tintedPixels(tt.Image(), under, asideRed); after <= before {
		t.Errorf("hovered, the aside's underline has %d pixels in its color, %d before", after, before)
	}
	tt.Move(0, 79)
	green := ui.RGB(0, 160, 0)
	o.Theme = &Theme{Link: green}
	tt.Frame()
	check("once the theme changes", asideRed, green)
	on = false
	tt.Frame()
	check("once LinkColor leaves it", green, green)
	if n := colorPixels(tt.Image(), aside, asideRed); n > 0 {
		t.Errorf("once LinkColor leaves it, the aside keeps %d pixels in its color", n)
	}
}

// TestLinkColorFollows colors a paragraph's links anew when LinkColor
// answers otherwise, or is set or cleared, while the source stays.
func TestLinkColorFollows(t *testing.T) {
	on := false
	src := "[aside](x:aside) @octocat\n"
	o := &Options{Resolve: func(in parser.Inline) (string, bool) { return "/users/aside", true }}
	tt, get := rvView(&src, o)
	for _, c := range []struct {
		when string
		set  func()
		want ui.Color
	}{
		{"without LinkColor", func() {}, Color{}},
		{"once set, leaving the links", func() { o.LinkColor = asides(&on) }, Color{}},
		{"once it colors them", func() { on = true }, asideRed},
		{"once it leaves them", func() { on = false }, Color{}},
		{"once it colors them again", func() { on = true }, asideRed},
		{"once cleared", func() { o.LinkColor = nil }, Color{}},
	} {
		c.set()
		tt.Frame()
		spans, links := spanColors(get(), 1)
		for _, text := range []string{"aside", "@octocat"} {
			if spans[text] != c.want || links[text] != c.want {
				t.Errorf("%s: %q colored %v in a link of %v, want %v", c.when, text, spans[text], links[text], c.want)
			}
		}
	}
}
