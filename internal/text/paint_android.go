package text

import (
	"unicode/utf16"

	"github.com/egoist/mygo/internal/mobile"
	"github.com/egoist/mygo/internal/scene"
)

func newEngine() engine {
	e := &paintEngine{fonts: map[paintSpec]*Font{}}
	e.clusterEngine.init(e)
	return e
}

// paintEngine lays out and draws text with Android's Paint, through the
// host: Minikin finds the fonts, falls back for what one lacks and lays
// words out, Skia draws them, as for a TextView. Its glyphs are grapheme
// clusters (clusterEngine), measured within their words.
type paintEngine struct {
	clusterEngine
	fonts map[paintSpec]*Font
	names []string // the families the app registered fonts under
}

// paintFont is what the engine keeps of a Font (Font.data): the font as the
// host takes it, and the texts of its glyphs, which a layout kept after the
// System let go of the Font still draws.
type paintFont struct {
	spec   paintSpec
	glyphs *glyphTexts
}

// paintSpec is a font as the host takes it.
type paintSpec struct {
	family string
	size   float32
	weight int
	italic bool
}

func (e *paintEngine) font(style Style) *Font {
	key := paintSpec{style.Family, style.FontSize(), style.weight(), style.Italic}
	if f, ok := e.fonts[key]; ok {
		return f
	}
	f := &Font{Size: key.size, data: &paintFont{key, e.glyphs}}
	m, ok := mobile.TextMetrics(key.family, float64(key.size), key.weight, key.italic)
	if ok && m.Ascent > 0 {
		f.Ascent, f.Descent = float32(m.Ascent), float32(m.Descent)
		f.underlineTop, f.underlineThick = -float32(m.UnderlinePosition), max(float32(m.UnderlineThickness), 0.5)
		f.strikeTop, f.strikeThick = float32(m.StrikePosition), f.underlineThick
	} else {
		f.Ascent, f.Descent = key.size*0.93, key.size*0.24
		f.underlineTop, f.underlineThick = -key.size/10, max(key.size/14, 1)
		f.strikeTop, f.strikeThick = key.size*0.3, f.underlineThick
	}
	e.fonts[key] = f
	return f
}

func (e *paintEngine) spec(f *Font) paintSpec { return f.data.(*paintFont).spec }

func (e *paintEngine) measure(f *Font, text string, noKern bool) float32 {
	s := e.spec(f)
	if w := mobile.MeasureText(s.family, float64(s.size), s.weight, s.italic, noKern, []string{text}); len(w) == 1 {
		return float32(w[0])
	}
	return 0
}

// pens lays the word out whole and sums the advances of its clusters'
// UTF-16 units, as Minikin gives them: kerning between clusters goes to
// the one before.
func (e *paintEngine) pens(f *Font, text []rune, bounds []int, noKern bool) []float32 {
	s := e.spec(f)
	adv := mobile.TextAdvances(s.family, float64(s.size), s.weight, s.italic, noKern, string(text), false)
	if len(adv) == 0 {
		return nil
	}
	// The advance of each rune, of its one or two units.
	runeAdv := make([]float32, len(text))
	u := 0
	for i, r := range text {
		n := utf16.RuneLen(r)
		if n < 1 {
			n = 1
		}
		for k := 0; k < n && u < len(adv); k++ {
			runeAdv[i] += float32(adv[u])
			u++
		}
	}
	pens := make([]float32, len(bounds))
	r := 0
	var x float32
	for k := 1; k < len(bounds); k++ {
		for ; r < bounds[k] && r < len(runeAdv); r++ {
			x += runeAdv[r]
		}
		pens[k] = x
	}
	return pens
}

func (e *paintEngine) glyph(f *Font, id uint32, scale, dx float32, _ Shade, _ bool) bitmap {
	// The texts of the glyphs of f's time: the engine's may be newer.
	pf := f.data.(*paintFont)
	if int(id) >= len(pf.glyphs.texts) || id == 0 {
		return bitmap{}
	}
	s := pf.spec
	r := mobile.DrawText(s.family, float64(s.size), s.weight, s.italic, pf.glyphs.texts[id], float64(scale), float64(dx))
	if r.Width <= 0 || r.Height <= 0 {
		return bitmap{}
	}
	return bitmap{left: r.Left, top: r.Top, w: r.Width, h: r.Height, pix: r.Pixels, color: r.Color}
}

// textParams leaves coverage as Skia drew it, without subpixel
// antialiasing, which Android's text has none of.
func (e *paintEngine) textParams() (scene.TextParams, bool) { return scene.TextParams{}, false }

// maxTexts is how many glyphs, texts, the engine keeps before it has the
// System let go of its fonts with them.
const maxPaintTexts = 1 << 16

func (e *paintEngine) fontCount() int {
	if len(e.glyphs.texts) > maxPaintTexts {
		return maxFonts + 1
	}
	return len(e.fonts)
}

func (e *paintEngine) forgetFonts() {
	clear(e.fonts)
	e.forgetGlyphs()
}

// releaseFonts has nothing to release: the host keeps its fonts.
func (e *paintEngine) releaseFonts() {}

func (e *paintEngine) appFamilies() []FontFamily {
	out := make([]FontFamily, len(e.names))
	for i, n := range e.names {
		out[i] = FontFamily{Name: n}
	}
	return out
}

// systemFamilies lists none: Android tells no families by name, only the
// generic ones.
func (e *paintEngine) systemFamilies() func() []FontFamily { return nil }

func (e *paintEngine) register(data []byte, family string) error {
	name, err := mobile.RegisterFont(data, family)
	if err != nil {
		return err
	}
	e.names = append(e.names, name)
	clear(e.words)
	return nil
}
