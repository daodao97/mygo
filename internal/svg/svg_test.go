package svg

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
	"strings"
	"testing"
)

var red = color.NRGBA{255, 0, 0, 255}

func draw(t *testing.T, src string, w, h int) *image.RGBA {
	t.Helper()
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	d.Draw(img, Options{Current: red})
	return img
}

func at(img *image.RGBA, x, y int) color.RGBA { return img.RGBAAt(x, y) }

// alphaSum is the total coverage of an image, in pixels.
func alphaSum(img *image.RGBA) float64 {
	var s float64
	for i := 3; i < len(img.Pix); i += 4 {
		s += float64(img.Pix[i]) / 255
	}
	return s
}

func TestFill(t *testing.T) {
	img := draw(t, `<svg viewBox="0 0 10 10"><rect x="2" y="2" width="6" height="6" fill="#00f"/></svg>`, 20, 20)
	if c := at(img, 10, 10); c != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("inside: %v", c)
	}
	if c := at(img, 4, 4); c.A != 255 {
		t.Errorf("the first pixel inside: %v", c)
	}
	if c := at(img, 3, 3); c.A != 0 {
		t.Errorf("outside: %v", c)
	}
	if got := alphaSum(img); math.Abs(got-144) > 0.5 {
		t.Errorf("coverage %v, want 144", got)
	}
}

func TestPathSyntax(t *testing.T) {
	// The same square, written in the ways path data allows.
	want := draw(t, `<svg viewBox="0 0 10 10"><path d="M1 1 L9 1 L9 9 L1 9 Z"/></svg>`, 10, 10)
	for _, d := range []string{
		"M1,1L9,1L9,9L1,9Z",
		"M1 1h8v8h-8z",
		"m1 1 8 0 0 8-8 0z",
		"M1 1 9 1 9 9 1 9z",
		"M+1.0e0,1.L9,1,9,9,1,9Z",
		"M1 1H9V9H1Z M20 20", // a lone move draws nothing
	} {
		got := draw(t, `<svg viewBox="0 0 10 10"><path d="`+d+`"/></svg>`, 10, 10)
		if string(got.Pix) != string(want.Pix) {
			t.Errorf("%q draws another shape", d)
		}
	}
	// Path data in error draws up to the error.
	part := draw(t, `<svg viewBox="0 0 10 10"><path d="M1 1H9V9H1Z M0 0 L5 x"/></svg>`, 10, 10)
	if string(part.Pix) != string(want.Pix) {
		t.Error("path data in error does not draw what comes before the error")
	}
	if img := draw(t, `<svg viewBox="0 0 10 10"><path d="L1 1 9 9 1 9Z"/></svg>`, 10, 10); alphaSum(img) != 0 {
		t.Error("path data that does not start with a move draws")
	}
}

func TestArcs(t *testing.T) {
	circle := draw(t, `<svg viewBox="0 0 20 20"><circle cx="10" cy="10" r="8"/></svg>`, 40, 40)
	arcs := draw(t, `<svg viewBox="0 0 20 20"><path d="M2 10a8 8 0 1 1 16 0A8 8 0 1 1 2 10z"/></svg>`, 40, 40)
	want := math.Pi * 16 * 16
	for name, img := range map[string]*image.RGBA{"circle": circle, "arcs": arcs} {
		if got := alphaSum(img); math.Abs(got-want) > want*0.01 {
			t.Errorf("%s covers %v pixels, want %v", name, got, want)
		}
	}
	// Flags packed without separators, and radii too small to reach.
	packed := draw(t, `<svg viewBox="0 0 20 20"><path d="M2 10a1 1 0 1116 0a1,1,0,1,1-16,0z"/></svg>`, 40, 40)
	if got := alphaSum(packed); math.Abs(got-want) > want*0.01 {
		t.Errorf("packed arcs cover %v pixels, want %v", got, want)
	}
}

func TestFillRule(t *testing.T) {
	const rings = `M0 0H10V10H0Z M2 2H8V8H2Z`
	nz := draw(t, `<svg viewBox="0 0 10 10"><path d="`+rings+`"/></svg>`, 10, 10)
	eo := draw(t, `<svg viewBox="0 0 10 10"><path fill-rule="evenodd" d="`+rings+`"/></svg>`, 10, 10)
	if at(nz, 5, 5).A != 255 {
		t.Error("the non-zero rule does not fill contours that turn the same way")
	}
	if at(eo, 5, 5).A != 0 || at(eo, 1, 1).A != 255 {
		t.Errorf("the even-odd rule: hole %v, ring %v", at(eo, 5, 5), at(eo, 1, 1))
	}
}

func TestStrokeCaps(t *testing.T) {
	line := func(cap string) *image.RGBA {
		return draw(t, `<svg viewBox="0 0 20 10"><path d="M5 5H15" stroke="#000" stroke-width="4" stroke-linecap="`+cap+`"/></svg>`, 20, 10)
	}
	butt, square, round := line("butt"), line("square"), line("round")
	if got := alphaSum(butt); math.Abs(got-40) > 0.5 {
		t.Errorf("butt caps cover %v, want 40", got)
	}
	if got := alphaSum(square); math.Abs(got-56) > 0.5 {
		t.Errorf("square caps cover %v, want 56", got)
	}
	if got, want := alphaSum(round), 40+4*math.Pi; math.Abs(got-want) > 0.5 {
		t.Errorf("round caps cover %v, want %v", got, want)
	}
	if at(butt, 4, 5).A != 0 || at(square, 4, 5).A != 255 {
		t.Error("caps do not end where they should")
	}
	// A subpath of no length is a dot with round caps.
	dot := draw(t, `<svg viewBox="0 0 10 10"><path d="M5 5z" stroke="#000" stroke-width="4" stroke-linecap="round"/></svg>`, 10, 10)
	if got, want := alphaSum(dot), 4*math.Pi; math.Abs(got-want) > 0.3 {
		t.Errorf("a dot covers %v, want %v", got, want)
	}
}

func TestStrokeJoins(t *testing.T) {
	corner := func(join string) *image.RGBA {
		return draw(t, `<svg viewBox="0 0 20 20"><path d="M2 10H10V18" fill="none" stroke="#000" stroke-width="4" stroke-linejoin="`+join+`"/></svg>`, 20, 20)
	}
	miter, bevel, round := corner("miter"), corner("bevel"), corner("round")
	// The outer corner of the turn is at (12, 8): the miter reaches it,
	// the bevel cuts it off, the round join rounds it.
	if at(miter, 11, 8).A != 255 {
		t.Errorf("the miter does not fill the corner: %v", at(miter, 11, 8))
	}
	if at(bevel, 11, 8).A > 128 {
		t.Errorf("the bevel fills the corner: %v", at(bevel, 11, 8))
	}
	m, b, r := alphaSum(miter), alphaSum(bevel), alphaSum(round)
	if !(m > r && r > b) {
		t.Errorf("coverage of miter %v, round %v, bevel %v", m, r, b)
	}
	// A sharp turn beyond the miter limit is beveled.
	sharp := func(limit string) float64 {
		return alphaSum(draw(t, `<svg viewBox="0 0 40 40"><path d="M2 20L38 22L2 24" fill="none" stroke="#000" stroke-width="2" stroke-miterlimit="`+limit+`"/></svg>`, 40, 40))
	}
	if sharp("4") >= sharp("100") {
		t.Error("the miter limit does not bevel a sharp turn")
	}
}

func TestDashes(t *testing.T) {
	img := draw(t, `<svg viewBox="0 0 20 4"><path d="M0 2H20" stroke="#000" stroke-width="2" stroke-dasharray="2 3"/></svg>`, 20, 4)
	for x, want := range []uint8{255, 255, 0, 0, 0, 255, 255, 0, 0, 0} {
		if got := at(img, x, 2).A; got != want {
			t.Errorf("pixel %d: %d, want %d", x, got, want)
		}
	}
	shifted := draw(t, `<svg viewBox="0 0 20 4"><path d="M0 2H20" stroke="#000" stroke-width="2" stroke-dasharray="2 3" stroke-dashoffset="1"/></svg>`, 20, 4)
	if at(shifted, 0, 2).A != 255 || at(shifted, 1, 2).A != 0 || at(shifted, 4, 2).A != 255 {
		t.Error("the dash offset does not shift the dashes")
	}
}

func TestTransforms(t *testing.T) {
	plain := draw(t, `<svg viewBox="0 0 20 20"><rect x="5" y="0" width="5" height="5"/></svg>`, 20, 20)
	for _, rect := range []string{
		`<rect width="5" height="5" transform="translate(5)"/>`,
		`<rect width="5" height="5" transform="translate(5 0)"/>`,
		`<rect width="5" height="5" transform=" matrix(1,0,0,1,5,0) "/>`,
		`<rect width="10" height="10" transform="translate(5 0) scale(0.5)"/>`,
		`<rect width="5" height="5" transform="rotate(90 5 5)"/>`,
		`<g transform="translate(5)"><rect width="10" height="5" transform="scale(.5 1)"/></g>`,
	} {
		got := draw(t, `<svg viewBox="0 0 20 20">`+rect+`</svg>`, 20, 20)
		if alphaSum(got) == 0 || string(got.Pix) != string(plain.Pix) {
			t.Errorf("%s moves the rectangle elsewhere", rect)
		}
	}
	// A transform in error transforms nothing.
	got := draw(t, `<svg viewBox="0 0 20 20"><rect x="5" width="5" height="5" transform="translate(5 0"/></svg>`, 20, 20)
	if string(got.Pix) != string(plain.Pix) {
		t.Error("a transform in error moves the rectangle")
	}
}

func TestCurrentColor(t *testing.T) {
	img := draw(t, `<svg viewBox="0 0 2 1"><rect width="1" height="1" fill="currentColor"/><g color="#0f0"><rect x="1" width="1" height="1" fill="currentColor"/></g></svg>`, 2, 1)
	if c := at(img, 0, 0); c != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("currentColor is %v, not the color it is drawn with", c)
	}
	if c := at(img, 1, 0); c != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("currentColor is %v, not the color property", c)
	}
}

func TestColors(t *testing.T) {
	for v, want := range map[string]color.RGBA{
		"#f00":                     {255, 0, 0, 255},
		"#FF0000":                  {255, 0, 0, 255},
		"rgb(0, 0, 255)":           {0, 0, 255, 255},
		"rgb(0 0 100%)":            {0, 0, 255, 255},
		"rgba(0,0,255,0.5)":        {0, 0, 128, 128},
		"rgb(0 0 255 / 50%)":       {0, 0, 128, 128},
		"#0000ff80":                {0, 0, 128, 128},
		"hsl(120, 100%, 50%)":      {0, 255, 0, 255},
		"hsl(240deg 100% 50%)":     {0, 0, 255, 255},
		"DarkOrange":               {255, 140, 0, 255},
		"transparent":              {},
		"#ff0000 icc-color(x, 1)":  {255, 0, 0, 255},
		"none":                     {},
		"url(#nothing) #00f":       {0, 0, 255, 255},
		"url(#nothing)":            {},
		"not a color":              {0, 0, 0, 255}, // ignored: black, the initial fill
		"rgb(0,0,0":                {0, 0, 0, 255},
		"#12345":                   {0, 0, 0, 255},
		"hsl(120, 100, 50)":        {0, 0, 0, 255},
		"rgba(255, 255, 255, 0.0)": {},
	} {
		img := draw(t, `<svg viewBox="0 0 1 1"><rect width="1" height="1" fill="`+v+`"/></svg>`, 1, 1)
		if got := at(img, 0, 0); got != want {
			t.Errorf("fill %q: %v, want %v", v, got, want)
		}
	}
}

func TestLinearGradient(t *testing.T) {
	img := draw(t, `<svg viewBox="0 0 100 10">
		<defs><linearGradient id="g"><stop offset="0" stop-color="#f00"/><stop offset="100%" stop-color="#00f"/></linearGradient></defs>
		<rect width="100" height="10" fill="url(#g)"/></svg>`, 100, 10)
	left, mid, right := at(img, 0, 5), at(img, 50, 5), at(img, 99, 5)
	if left.R < 250 || left.B > 5 || right.B < 250 || right.R > 5 {
		t.Errorf("ends: %v, %v", left, right)
	}
	if mid.R < 115 || mid.R > 140 || mid.B < 115 || mid.B > 140 {
		t.Errorf("middle: %v", mid)
	}
	// Stops and coordinates inherited through href, in user space, and
	// spread.
	img = draw(t, `<svg viewBox="0 0 100 10">
		<linearGradient id="a"><stop offset="0" stop-color="#000"/><stop offset="1" stop-color="#fff"/></linearGradient>
		<linearGradient id="b" href="#a" gradientUnits="userSpaceOnUse" x1="0" x2="50" spreadMethod="reflect"/>
		<rect width="100" height="10" fill="url(#b)"/></svg>`, 100, 10)
	if c := at(img, 49, 5); c.R < 245 {
		t.Errorf("at the end of the gradient: %v", c)
	}
	if c := at(img, 99, 5); c.R > 10 {
		t.Errorf("reflected: %v", c)
	}
}

func TestRadialGradient(t *testing.T) {
	img := draw(t, `<svg viewBox="0 0 20 20">
		<radialGradient id="g"><stop offset="0" stop-color="#fff"/><stop offset="1" stop-color="#000"/></radialGradient>
		<rect width="20" height="20" fill="url(#g)"/></svg>`, 20, 20)
	center, edge, corner := at(img, 10, 10), at(img, 10, 0), at(img, 0, 0)
	if center.R < 230 || edge.R > 30 || corner != (color.RGBA{0, 0, 0, 255}) {
		t.Errorf("center %v, edge %v, corner %v", center, edge, corner)
	}
	// A focal point off center.
	img = draw(t, `<svg viewBox="0 0 20 20">
		<radialGradient id="g" fx="0.25"><stop offset="0" stop-color="#fff"/><stop offset="1" stop-color="#000"/></radialGradient>
		<rect width="20" height="20" fill="url(#g)"/></svg>`, 20, 20)
	if at(img, 5, 10).R < 230 || at(img, 15, 10).R > at(img, 5, 10).R {
		t.Errorf("focal point: %v, %v", at(img, 5, 10), at(img, 15, 10))
	}
}

func TestGroupOpacity(t *testing.T) {
	const rects = `<rect width="6" height="10"/><rect x="4" width="6" height="10"/>`
	group := draw(t, `<svg viewBox="0 0 10 10"><g opacity="0.5">`+rects+`</g></svg>`, 10, 10)
	each := draw(t, `<svg viewBox="0 0 10 10"><g fill-opacity="0.5">`+rects+`</g></svg>`, 10, 10)
	if a, b := at(group, 5, 5).A, at(group, 1, 5).A; a != b || a < 126 || a > 129 {
		t.Errorf("group opacity: overlap %d, alone %d", a, b)
	}
	if at(each, 5, 5).A <= at(each, 1, 5).A {
		t.Error("shapes with their own opacity do not add up where they overlap")
	}
	// Fill and stroke of one shape with opacity.
	img := draw(t, `<svg viewBox="0 0 10 10"><rect x="2" y="2" width="6" height="6" stroke="#000" stroke-width="2" opacity="0.5"/></svg>`, 10, 10)
	if a, b := at(img, 2, 2).A, at(img, 5, 5).A; a != b {
		t.Errorf("an element's opacity: stroke over fill %d, fill %d", a, b)
	}
}

func TestClipPath(t *testing.T) {
	img := draw(t, `<svg viewBox="0 0 20 20">
		<clipPath id="c"><circle cx="10" cy="10" r="8"/></clipPath>
		<rect width="20" height="20" clip-path="url(#c)"/></svg>`, 20, 20)
	if at(img, 10, 10).A != 255 || at(img, 0, 0).A != 0 {
		t.Errorf("clipped: center %v, corner %v", at(img, 10, 10), at(img, 0, 0))
	}
	if got, want := alphaSum(img), math.Pi*64; math.Abs(got-want) > want*0.02 {
		t.Errorf("clipped coverage %v, want %v", got, want)
	}
	// objectBoundingBox units, with an even-odd clip rule.
	img = draw(t, `<svg viewBox="0 0 20 20">
		<clipPath id="c" clipPathUnits="objectBoundingBox"><path clip-rule="evenodd" d="M0 0H1V1H0Z M.25 .25H.75V.75H.25Z"/></clipPath>
		<g clip-path="url(#c)"><rect x="4" y="4" width="12" height="12"/></g></svg>`, 20, 20)
	if at(img, 10, 10).A != 0 || at(img, 5, 5).A != 255 || at(img, 2, 2).A != 0 {
		t.Errorf("box clip: hole %v, ring %v, outside %v", at(img, 10, 10), at(img, 5, 5), at(img, 2, 2))
	}
}

func TestMask(t *testing.T) {
	img := draw(t, `<svg viewBox="0 0 20 10">
		<mask id="m" maskUnits="userSpaceOnUse" x="0" y="0" width="20" height="10">
			<rect width="10" height="10" fill="#fff"/><rect x="10" width="10" height="10" fill="#000"/>
		</mask>
		<rect width="20" height="10" fill="#00f" mask="url(#m)"/></svg>`, 20, 10)
	if at(img, 5, 5).A != 255 || at(img, 15, 5).A != 0 {
		t.Errorf("masked: white %v, black %v", at(img, 5, 5), at(img, 15, 5))
	}
}

func TestStyleSheets(t *testing.T) {
	img := draw(t, `<svg viewBox="0 0 4 1">
		<style><![CDATA[
			/* Illustrator writes classes */
			.a { fill: #00f }
			rect.b, .c { fill: #0f0 !important }
			#d { fill: #f0f }
			@media print { .a { fill: #fff } }
			g > rect { fill: #fff }
		]]></style>
		<rect class="a" width="1" height="1"/>
		<rect class="a b" x="1" width="1" height="1"/>
		<rect class="a" id="d" x="2" width="1" height="1"/>
		<rect class="a" x="3" width="1" height="1" fill="#f00" style="fill: #ff0"/>
	</svg>`, 4, 1)
	for x, want := range []color.RGBA{{0, 0, 255, 255}, {0, 255, 0, 255}, {255, 0, 255, 255}, {255, 255, 0, 255}} {
		if got := at(img, x, 0); got != want {
			t.Errorf("rect %d: %v, want %v", x, got, want)
		}
	}
}

func TestUse(t *testing.T) {
	img := draw(t, `<svg viewBox="0 0 20 10">
		<defs><rect id="r" width="5" height="5"/>
		<symbol id="s" viewBox="0 0 1 1"><rect width="1" height="1" fill="#00f"/></symbol></defs>
		<use href="#r" x="5"/>
		<use xlink:href="#s" x="10" y="0" width="5" height="5" xmlns:xlink="http://www.w3.org/1999/xlink"/>
		<use href="#loop" id="loop"/>
	</svg>`, 20, 10)
	if at(img, 1, 1).A != 0 || at(img, 6, 1).A != 255 {
		t.Error("use does not draw its element where it says")
	}
	if c := at(img, 12, 2); c != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("a symbol: %v", c)
	}
	if at(img, 12, 7).A != 0 {
		t.Error("a symbol draws outside its viewport")
	}
}

func TestDisplayAndVisibility(t *testing.T) {
	img := draw(t, `<svg viewBox="0 0 3 1">
		<rect width="1" height="1" display="none"/>
		<g visibility="hidden"><rect x="1" width="1" height="1"/><rect x="2" width="1" height="1" visibility="visible"/></g>
	</svg>`, 3, 1)
	if at(img, 0, 0).A != 0 || at(img, 1, 0).A != 0 || at(img, 2, 0).A != 255 {
		t.Errorf("display none %v, hidden %v, visible %v", at(img, 0, 0), at(img, 1, 0), at(img, 2, 0))
	}
}

func TestSizeAndAspect(t *testing.T) {
	for src, want := range map[string][2]float64{
		`<svg width="24" height="24" viewBox="0 0 48 48"/>`:   {24, 24},
		`<svg viewBox="0 0 48 24"/>`:                          {48, 24},
		`<svg width="10" viewBox="0 0 48 24"/>`:               {10, 5},
		`<svg width="100%" height="100%" viewBox="0 0 8 4"/>`: {8, 4},
		`<svg width="1in" height="12pt"/>`:                    {96, 16},
		`<svg/>`:                                              {300, 150},
	} {
		d, err := Parse([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if w, h := d.Size(); w != want[0] || h != want[1] {
			t.Errorf("%s: %vx%v, want %vx%v", src, w, h, want[0], want[1])
		}
	}
	// A wide viewBox in a square: centered, unless aligned or stretched.
	const wide = `<rect width="10" height="5"/></svg>`
	mid := draw(t, `<svg viewBox="0 0 10 5">`+wide, 10, 10)
	if at(mid, 5, 1).A != 0 || at(mid, 5, 5).A != 255 || at(mid, 5, 8).A != 0 {
		t.Error("the viewBox is not centered")
	}
	top := draw(t, `<svg viewBox="0 0 10 5" preserveAspectRatio="xMinYMin meet">`+wide, 10, 10)
	if at(top, 5, 1).A != 255 || at(top, 5, 8).A != 0 {
		t.Error("the viewBox is not aligned to the top")
	}
	d, _ := Parse([]byte(`<svg viewBox="0 0 10 5">` + wide))
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	d.Draw(img, Options{Current: red, Stretch: true})
	if alphaSum(img) != 100 {
		t.Error("the viewBox is not stretched")
	}
}

func TestOldEditors(t *testing.T) {
	// Illustrator's declarations, entities, namespaces and switch.
	src := `<?xml version="1.0" encoding="iso-8859-1"?>
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd" [
	<!ENTITY ns_svg "http://www.w3.org/2000/svg">
]>
<svg xmlns="&ns_svg;" xmlns:i="&ns_ai;" viewBox="0 0 10 10">
<switch>
	<foreignObject requiredExtensions="&ns_ai;" x="0" y="0" width="1" height="1"/>
	<g i:extraneous="self"><rect width="10" height="10" fill="#00f"/></g>
</switch>
<!-- caf` + "\xe9" + ` -->
</svg>`
	img := draw(t, src, 10, 10)
	if c := at(img, 5, 5); c != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("drawn %v", c)
	}
}

func TestErrors(t *testing.T) {
	for _, src := range []string{``, `<html/>`, `<svg><rect`, `not xml`} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%q parses", src)
		}
	}
}

func TestIcons(t *testing.T) {
	// A check mark stroked in currentColor, as icon sets draw them.
	check := draw(t, checkIcon, 24, 24)
	if c := at(check, 9, 16); c != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("the check mark's corner: %v", c)
	}
	if at(check, 2, 2).A != 0 {
		t.Error("the check mark covers its corner")
	}
	// A house filled in a viewBox above the origin, with a door cut out.
	house := draw(t, `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 -960 960 960"><path fill-rule="evenodd" d="M160-120V-600L480-840L800-600V-120ZM400-120V-360H560V-120Z"/></svg>`, 24, 24)
	if at(house, 12, 6).A != 255 || at(house, 6, 15).A != 255 || at(house, 12, 18).A != 0 || at(house, 2, 2).A != 0 {
		t.Errorf("the house: roof %v, wall %v, door %v", at(house, 12, 6), at(house, 6, 15), at(house, 12, 18))
	}
}

// TestParseBasic draws icons as Parse does, and leaves out text, markers
// and style sheets.
func TestParseBasic(t *testing.T) {
	basic := func(src string) *image.RGBA {
		t.Helper()
		d, err := ParseBasic([]byte(src))
		if err != nil {
			t.Fatalf("ParseBasic: %v", err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 24, 24))
		d.Draw(img, Options{Current: red, Fonts: boxFonts{}})
		return img
	}
	for _, src := range []string{
		checkIcon,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 3"/></svg>`,
		`<svg viewBox="0 0 24 24"><g transform="rotate(20 12 12)" opacity=".5" style="fill: currentColor; stroke: blue"><rect x="4" y="4" width="16" height="16" rx="3"/></g></svg>`,
		`<svg viewBox="0 0 24 24"><linearGradient id="g"><stop offset="0" stop-color="red"/><stop offset="1" stop-color="blue"/></linearGradient><clipPath id="c"><circle cx="12" cy="12" r="10"/></clipPath><rect width="24" height="24" fill="url(#g)" clip-path="url(#c)"/></svg>`,
	} {
		want := draw(t, src, 24, 24)
		if got := basic(src); !slices.Equal(got.Pix, want.Pix) {
			t.Errorf("ParseBasic draws differently from Parse:\n%s", src)
		}
	}
	empty := basic(`<svg viewBox="0 0 24 24"><style>rect{fill:red}</style><rect width="24" height="24" fill="none"/><text y="20" font-size="20">a</text><marker id="m" viewBox="0 0 2 2"><rect width="2" height="2"/></marker><path d="M2 2H22" fill="none" marker-end="url(#m)"/></svg>`)
	if alphaSum(empty) != 0 {
		t.Errorf("ParseBasic drew text, a marker or a style sheet's rule: %v pixels", alphaSum(empty))
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{
		`<svg viewBox="0 0 10 10"><path d="M1 1a5 5 0 1 0 8 8z" stroke="red" stroke-dasharray="1 2"/></svg>`,
		`<svg><g opacity=".5" clip-path="url(#c)"><use href="#u"/></g><clipPath id="c"><use href="#r"/></clipPath><rect id="r" width="5" height="5"/></svg>`,
		`<svg><mask id="m"><rect width="1" height="1" mask="url(#m)"/></mask><circle r="5" mask="url(#m)"/></svg>`,
		`<svg><linearGradient id="a" href="#b"/><radialGradient id="b" href="#a" fr="2"/><rect width="9" height="9" fill="url(#a)"/></svg>`,
		`<svg><style>:root{--a:var(--b,red);--b:var(--a)} g > .c{fill:var(--a,#0f0)} @media not (prefers-color-scheme:dark){*{stroke:blue}}</style><g><rect class="c" width="9" height="9"/></g></svg>`,
		`<svg><marker id="m" orient="auto-start-reverse" viewBox="0 0 4 4" refX="2"><path d="M0 0L4 2L0 4z" fill="context-stroke" marker-end="url(#m)"/></marker><path d="M1 1a4 4 0 1 1 8 8Z" stroke="red" marker-start="url(#m)" marker-mid="url(#m)"/></svg>`,
		`<svg><text x="1 2" dx="3" y="9" text-anchor="middle" transform="rotate(30)" font="bold 8px serif" stroke="red">a <tspan dy="-2" xml:space="preserve"> b  </tspan>c</text></svg>`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := Parse(data)
		if err != nil {
			return
		}
		d.Draw(image.NewRGBA(image.Rect(0, 0, 16, 16)), Options{Current: red, Fonts: boxFonts{}})
		if d, err = ParseBasic(data); err != nil {
			t.Fatalf("ParseBasic fails where Parse does not: %v", err)
		}
		d.Draw(image.NewRGBA(image.Rect(0, 0, 16, 16)), Options{Current: red, Fonts: boxFonts{}})
	})
}

const checkIcon = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>`

// gear returns an icon of a gear: arcs and lines with round joins.
func gear(teeth int) string {
	var b strings.Builder
	b.WriteString(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="`)
	for i := range teeth * 2 {
		r := 10.0
		if i%2 == 1 {
			r = 7.5
		}
		a0, a1 := float64(i)*math.Pi/float64(teeth), float64(i+1)*math.Pi/float64(teeth)
		cmd := "L"
		if i == 0 {
			cmd = "M"
		}
		fmt.Fprintf(&b, "%s%.2f %.2fA%.1f %.1f 0 0 1 %.2f %.2f", cmd, 12+r*math.Cos(a0), 12+r*math.Sin(a0), r, r, 12+r*math.Cos(a1), 12+r*math.Sin(a1))
	}
	b.WriteString(`Z"/><circle cx="12" cy="12" r="3"/></svg>`)
	return b.String()
}

var gearIcon = gear(8)

// BenchmarkParse parses an icon of a gear drawn with arcs.
func BenchmarkParse(b *testing.B) {
	for b.Loop() {
		if _, err := Parse([]byte(gearIcon)); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDrawIcon draws icons of a check and of the gear at 16 to 256
// pixels.
func BenchmarkDrawIcon(b *testing.B) {
	for _, bc := range []struct {
		name string
		src  string
		size int
	}{{"check16", checkIcon, 16}, {"gear16", gearIcon, 16}, {"gear32", gearIcon, 32}, {"gear256", gearIcon, 256}} {
		b.Run(bc.name, func(b *testing.B) {
			d, _ := Parse([]byte(bc.src))
			img := image.NewRGBA(image.Rect(0, 0, bc.size, bc.size))
			for b.Loop() {
				clear(img.Pix)
				d.Draw(img, Options{Current: red})
			}
		})
	}
}

func TestUsesCurrentColor(t *testing.T) {
	for src, want := range map[string]bool{
		checkIcon: true,
		`<svg><rect width="1" height="1" fill="#000"/></svg>`:                                                                             false,
		`<svg color="red"><rect width="1" height="1" fill="currentColor"/></svg>`:                                                         false,
		`<svg><linearGradient id="g"><stop stop-color="currentColor"/></linearGradient><rect width="1" height="1" fill="url(#g)"/></svg>`: true,
	} {
		d, err := Parse([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if got := d.UsesCurrentColor(); got != want {
			t.Errorf("%s: UsesCurrentColor %v", src, got)
		}
	}
}

// drawWith draws a document as o says.
func drawWith(t *testing.T, src string, w, h int, o Options) *image.RGBA {
	t.Helper()
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	d.Draw(img, o)
	return img
}

// TestCustomProperties draws colors of var(), as the diagrams of READMEs
// set them.
func TestCustomProperties(t *testing.T) {
	// The repro of #1691: white, not the black of a fill MyGo did not
	// understand.
	img := draw(t, `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100"><style>svg { --bg: #ffffff; } .bg { fill: var(--bg); }</style><rect class="bg" width="100" height="100"/></svg>`, 100, 100)
	if c := at(img, 50, 50); c != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("var(--bg) = #ffffff draws %v", c)
	}
	img = draw(t, `<svg viewBox="0 0 6 1">
		<style>
			:root { --a: #00f; --b: var(--a); --loop1: var(--loop2); --loop2: var(--loop1) }
			.a { fill: var(--a) } .b { fill: var(--b) }
			.fallback { fill: var(--missing, var(--also-missing, #0f0)) }
			.loop { fill: var(--loop1, #ff0) }
			.bad { fill: var(--missing) }
		</style>
		<rect class="a" width="1" height="1"/>
		<rect class="b" x="1" width="1" height="1"/>
		<rect class="fallback" x="2" width="1" height="1"/>
		<g style="--c: rgb(255, 0, 255)"><rect x="3" width="1" height="1" style="fill: var(--c)"/></g>
		<rect class="loop" x="4" width="1" height="1"/>
		<g fill="#0ff"><rect class="bad" x="5" width="1" height="1"/></g>
	</svg>`, 6, 1)
	for x, want := range []color.RGBA{
		{0, 0, 255, 255},   // var()
		{0, 0, 255, 255},   // a var() of a var()
		{0, 255, 0, 255},   // nested fallbacks
		{255, 0, 255, 255}, // inherited from a style attribute
		{255, 255, 0, 255}, // a cycle, which falls back
		{0, 255, 255, 255}, // undefined: unset, which inherits
	} {
		if got := at(img, x, 0); got != want {
			t.Errorf("rect %d: %v, want %v", x, got, want)
		}
	}
}

// TestColorScheme draws the colors of prefers-color-scheme media queries
// in the scheme Draw is asked for.
func TestColorScheme(t *testing.T) {
	const src = `<svg viewBox="0 0 2 1">
		<style>
			:root { --fg: #000; --bg: #fff }
			@media (prefers-color-scheme: dark) { :root { --fg: #fff; --bg: #000 } .only { fill: #f00 } }
			@media screen and (prefers-color-scheme: light) { .only { fill: #00f } }
			.bg { fill: var(--bg) } .only { fill: #0f0 }
		</style>
		<rect class="bg" width="1" height="1"/><rect class="only" x="1" width="1" height="1"/>
	</svg>`
	light := drawWith(t, src, 2, 1, Options{})
	dark := drawWith(t, src, 2, 1, Options{Dark: true})
	if c := at(light, 0, 0); c != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("light background %v", c)
	}
	if c := at(dark, 0, 0); c != (color.RGBA{0, 0, 0, 255}) {
		t.Errorf("dark background %v", c)
	}
	// The rule after the media queries wins either way, as it would in a
	// browser.
	if at(light, 1, 0) != at(dark, 1, 0) || at(light, 1, 0) != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("a later rule: light %v, dark %v", at(light, 1, 0), at(dark, 1, 0))
	}
	d, _ := Parse([]byte(src))
	if !d.UsesColorScheme() {
		t.Error("a document of prefers-color-scheme does not use the color scheme")
	}
	d, _ = Parse([]byte(checkIcon))
	if d.UsesColorScheme() {
		t.Error("an icon uses the color scheme")
	}
}

// TestSelectors matches descendant and child combinators, and :root.
func TestSelectors(t *testing.T) {
	img := draw(t, `<svg viewBox="0 0 4 1">
		<style>
			.diagram .node rect { fill: #00f }
			g > rect.direct { fill: #0f0 }
			:root > rect { fill: #f0f }
			.x + rect { fill: #fff }
		</style>
		<g class="diagram"><g class="node"><rect width="1" height="1"/></g></g>
		<g><rect class="direct" x="1" width="1" height="1"/></g>
		<rect x="2" width="1" height="1"/>
		<g class="node"><rect x="3" width="1" height="1"/></g>
	</svg>`, 4, 1)
	for x, want := range []color.RGBA{{0, 0, 255, 255}, {0, 255, 0, 255}, {255, 0, 255, 255}, {0, 0, 0, 255}} {
		if got := at(img, x, 0); got != want {
			t.Errorf("rect %d: %v, want %v", x, got, want)
		}
	}
}

// TestUnsupported lists what a document has that is not drawn, and
// nothing for one that is drawn whole.
func TestUnsupported(t *testing.T) {
	d, err := Parse([]byte(`<svg viewBox="0 0 10 10">
		<style>@font-face { font-family: X; src: url(x.woff) } .a + .b { fill: red } .c:hover { fill: blue }
		@media (min-width: 600px) { rect { fill: red } }</style>
		<pattern id="p" width="2" height="2"><rect width="1" height="1"/></pattern>
		<filter id="f"><feGaussianBlur stdDeviation="2"/></filter>
		<rect width="5" height="5" fill="url(#p)" style="filter: url(#f)"/>
		<image href="a.png" width="5" height="5"/>
		<text>a<textPath href="#q">b</textPath></text>
	</svg>`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"<image>", "<pattern>", "<textPath>", "@font-face", "@media (min-width: 600px)", "filter", "selector .a + .b"}
	if got := d.Unsupported(); !slices.Equal(got, want) {
		t.Errorf("Unsupported() = %q, want %q", got, want)
	}
	for _, src := range []string{checkIcon, gearIcon, diagram} {
		d, err := Parse([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if got := d.Unsupported(); len(got) != 0 {
			t.Errorf("Unsupported() = %q for a document drawn whole", got)
		}
	}
}

// diagram is a diagram as READMEs have them: colors of custom properties
// with a dark scheme, labels and an arrow.
const diagram = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 40">
<style>
	:root { --fg: #1f2328 } @media (prefers-color-scheme: dark) { :root { --fg: #e6edf3 } }
	text { fill: var(--fg); font: 600 10px sans-serif; text-anchor: middle; dominant-baseline: central }
	.edge { stroke: var(--fg); marker-end: url(#arrow) }
</style>
<defs><marker id="arrow" viewBox="0 0 10 10" refX="10" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path d="M0 0L10 5L0 10z" fill="context-stroke"/></marker></defs>
<text x="20" y="20">A</text><path class="edge" d="M30 20H70"/><text x="80" y="20">B</text>
</svg>`

// boxFonts measures every character half an em wide, 0.8 em high and 0.2
// em deep, and draws it as a box that wide and high.
type boxFonts struct{}

func (boxFonts) Measure(s string, f Font) (float64, float64, float64) {
	return float64(len([]rune(s))) * f.Size / 2, 0.8 * f.Size, 0.2 * f.Size
}

func (boxFonts) Rasterize(s string, f Font, x, stroke float64) Bitmap {
	w := int(math.Round(x + float64(len([]rune(s)))*f.Size/2))
	h := int(math.Round(0.8 * f.Size))
	pix := make([]byte, w*h)
	for i := range pix {
		pix[i] = 255
	}
	return Bitmap{Left: 0, Top: -h, W: w, H: h, Pix: pix}
}

// inkX returns the columns of row y an image covers, from the first to
// after the last.
func inkX(img *image.RGBA, y int) (x0, x1 int) {
	x0, x1 = -1, -1
	for x := range img.Bounds().Dx() {
		if at(img, x, y).A > 127 {
			if x0 < 0 {
				x0 = x
			}
			x1 = x + 1
		}
	}
	return x0, x1
}

// TestText lays out text with the Fonts of Options: anchored, placed and
// moved, and between the shapes around it.
func TestText(t *testing.T) {
	o := Options{Fonts: boxFonts{}}
	img := drawWith(t, `<svg viewBox="0 0 100 100">
		<text x="50" y="20" font-size="10" text-anchor="middle">abcd</text>
		<text x="50" y="40" font-size="10" text-anchor="end">ab</text>
		<text x="10" y="60" font-size="10">ab<tspan dx="10">c</tspan></text>
		<text x="10 30" y="80" font-size="10">a<tspan font-size="20">b</tspan></text>
	</svg>`, 100, 100, o)
	for _, c := range []struct {
		y, x0, x1 int
	}{
		{16, 40, 60}, // middle: centered on x
		{36, 40, 50}, // end: ending at x
		{56, 10, 35}, // a run, then one 10 further
	} {
		if x0, x1 := inkX(img, c.y); x0 != c.x0 || x1 != c.x1 {
			t.Errorf("row %d: ink from %d to %d, want %d to %d", c.y, x0, x1, c.x0, c.x1)
		}
	}
	// dx moves the run after it; a list of x places each character.
	if at(img, 22, 56).A != 0 || at(img, 32, 56).A != 255 {
		t.Errorf("dx: %v at 22, %v at 32", at(img, 22, 56), at(img, 32, 56))
	}
	if at(img, 20, 70).A != 0 || at(img, 35, 70).A != 255 || at(img, 12, 78).A != 255 {
		t.Errorf("x list: %v at 20, %v at 35", at(img, 20, 70), at(img, 35, 70))
	}
	// Central puts the middle of the em box on y.
	img = drawWith(t, `<svg viewBox="0 0 100 100"><text x="0" y="50" font-size="20" dominant-baseline="central">a</text></svg>`, 100, 100, o)
	// The box of 0.8 em above the baseline and 0.2 em below it is from 40
	// to 60: what is above the baseline, from 40 to 56, is drawn.
	if at(img, 5, 41).A != 255 || at(img, 5, 55).A != 255 || at(img, 5, 38).A != 0 || at(img, 5, 57).A != 0 {
		t.Errorf("central: %v %v %v %v", at(img, 5, 38), at(img, 5, 41), at(img, 5, 55), at(img, 5, 57))
	}
	// Text between shapes, in document order, scaled and rotated.
	img = drawWith(t, `<svg viewBox="0 0 50 50">
		<rect width="50" height="50" fill="#00f"/>
		<text x="5" y="20" font-size="10" fill="#0f0">abc</text>
		<rect x="0" y="30" width="50" height="20" fill="#f00"/>
		<text x="5" y="45" font-size="10" fill="#ff0" opacity=".5">abc</text>
		<text transform="rotate(90 40 5)" x="40" y="5" font-size="10" fill="#fff">abc</text>
	</svg>`, 100, 100, o)
	if c := at(img, 20, 30); c != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("text over a shape: %v", c)
	}
	if c := at(img, 20, 86); c.R != 255 || c.G < 120 || c.G > 135 {
		t.Errorf("translucent text over a shape: %v", c)
	}
	if c := at(img, 88, 30); c != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("rotated text: %v", c)
	}
	// Without Fonts, no text is drawn.
	if img := draw(t, `<svg viewBox="0 0 10 10"><text y="10" font-size="10">a</text></svg>`, 10, 10); alphaSum(img) != 0 {
		t.Error("text drawn without Fonts")
	}
}

// TestMarkers draws arrowheads along the direction of a path's end, and
// at its start reversed.
func TestMarkers(t *testing.T) {
	const defs = `<defs><marker id="a" viewBox="0 0 10 10" refX="10" refY="5" markerWidth="5" markerHeight="5" orient="auto-start-reverse"><path d="M0 0L10 5L0 10z" fill="#00f"/></marker></defs>`
	// Down the middle, then right: the arrowhead at the end points right,
	// its tip at the end; the one at the start points up.
	img := draw(t, `<svg viewBox="0 0 100 100">`+defs+`<path d="M20 30V70H80" fill="none" stroke="#000" stroke-width="4" marker-start="url(#a)" marker-end="url(#a)"/></svg>`, 100, 100)
	blue := func(x, y int) bool { c := at(img, x, y); return c.B > 200 && c.R < 50 }
	// The end: 20 wide (5 × the stroke's 4) and 20 high, from x 60 to 80.
	if !blue(62, 70) || !blue(62, 62) || !blue(76, 70) || blue(76, 64) || blue(82, 70) {
		t.Errorf("the end's arrowhead does not point right: %v %v %v %v", blue(62, 70), blue(62, 62), blue(76, 70), blue(76, 64))
	}
	// The start: reversed, pointing up, its tip at (20, 30).
	if !blue(20, 32) || !blue(12, 48) || blue(12, 33) || blue(20, 27) {
		t.Errorf("the start's arrowhead does not point up: %v %v %v %v", blue(20, 32), blue(12, 48), blue(12, 33), blue(20, 27))
	}
	// userSpaceOnUse sizes it in user units; a fixed orient turns it
	// whatever the path's direction.
	img = draw(t, `<svg viewBox="0 0 100 100"><defs><marker id="b" markerUnits="userSpaceOnUse" markerWidth="10" markerHeight="10" refX="5" refY="5" orient="90"><path d="M0 0H10L5 10z" fill="#00f"/></marker></defs>
		<line x1="10" y1="50" x2="50" y2="50" stroke="#000" stroke-width="10" marker-end="url(#b)"/></svg>`, 100, 100)
	// Turned a quarter: its point at the left.
	if !blue(53, 47) || !blue(53, 52) || blue(46, 46) {
		t.Errorf("a marker of a fixed orient: %v %v %v", blue(53, 47), blue(53, 52), blue(46, 46))
	}
	// A marker's overflow is hidden: what is past its viewport is cut.
	img = draw(t, `<svg viewBox="0 0 100 100"><defs><marker id="c" markerUnits="userSpaceOnUse" markerWidth="10" markerHeight="10"><rect width="20" height="20" fill="#00f"/></marker></defs>
		<line x1="10" y1="10" x2="50" y2="10" stroke="#000" marker-end="url(#c)"/></svg>`, 100, 100)
	if !blue(55, 15) || blue(65, 15) {
		t.Errorf("a marker's overflow: %v inside, %v outside", blue(55, 15), blue(65, 15))
	}
}

// TestMarkerVertices finds the vertices of paths: arcs split into curves
// are one segment, and closed subpaths turn at their start.
func TestMarkerVertices(t *testing.T) {
	pa := parsePath("M0 0A10 10 0 1 1 0 20L0 30")
	if vs := pa.vertices(); len(vs) != 3 {
		t.Errorf("an arc and a line: %d vertices, want 3", len(vs))
	}
	pa = parsePath("M0 0H10V10Z")
	vs := pa.vertices()
	if len(vs) != 4 {
		t.Fatalf("a closed triangle: %d vertices, want 4", len(vs))
	}
	// At its start, between the closing segment's direction (up and to
	// the left, -135 degrees) and the first's (right, 0).
	if a := vs[0].angle(); math.Abs(a+67.5) > 1e-9 {
		t.Errorf("the start of a closed subpath turns %v degrees, want -67.5", a)
	}
}

// spacingFonts notes the letter spacing of the runs it measures.
type spacingFonts struct {
	boxFonts
	got *float64
}

func (f spacingFonts) Measure(s string, ft Font) (float64, float64, float64) {
	*f.got = ft.LetterSpacing
	return f.boxFonts.Measure(s, ft)
}

// Letter spacing in ems is of the element's own font size, wherever that is
// declared: attributes come in no order.
func TestLetterSpacingEm(t *testing.T) {
	for _, c := range []struct {
		attrs string
		want  float64
	}{
		{`font-size="40" letter-spacing="0.1em"`, 4},
		{`style="letter-spacing:0.1em;font-size:40px"`, 4},
		{`style="letter-spacing:0.1em;font:40px sans-serif"`, 4},
		{`style="letter-spacing:10%;font-size:40px"`, 4},
		{`style="letter-spacing:3px;letter-spacing:wide"`, 3},
		{`style="letter-spacing:0.1em;letter-spacing:inherit;font-size:40px"`, 2},
		{`style="letter-spacing:0.1em;letter-spacing:normal"`, 0},
		{`style="font-size:40px"`, 2}, // inherited as a length
	} {
		for range 20 {
			var got float64
			drawWith(t, `<svg viewBox="0 0 100 100"><g font-size="20" style="letter-spacing:0.1em"><text x="0" y="50" `+c.attrs+`>a</text></g></svg>`, 100, 100, Options{Fonts: spacingFonts{got: &got}})
			if math.Abs(got-c.want) > 1e-9 {
				t.Errorf("%s: letter spacing %v, want %v", c.attrs, got, c.want)
				break
			}
		}
	}
}
