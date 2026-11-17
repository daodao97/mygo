//go:build js

package text

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/egoist/mygo/internal/scene"
)

// newEngine returns the canvas engine, or where there is no canvas to
// measure and draw text with (Node, workers of browsers without
// OffscreenCanvas), the stub engine.
func newEngine() engine {
	if js.Global().Get("document").IsUndefined() && js.Global().Get("OffscreenCanvas").IsUndefined() {
		return &stubEngine{}
	}
	return newCanvas()
}

// canvasEngine lays out and draws text with a canvas's 2D context, in
// browsers: the browser finds the fonts, falls back for what one lacks and
// shapes, as for its pages. Canvases tell how wide text is and draw it,
// not its glyphs: the engine's glyphs are grapheme clusters, measured
// within their words, which keeps the fonts' kerning, and drawn alone; a
// word of a script that joins or reorders its letters (Arabic, the scripts
// of India, …) is one glyph, drawn whole. go-text's segmenter finds the
// clusters and where lines may break, and its bidi package the direction
// of runs.
type canvasEngine struct {
	clusterEngine
	canvas, ctx js.Value
	// current is the context's font now, kerning its fontKerning and fill
	// its fillStyle.
	current, kerning, fill string
	fonts                  map[string]*Font
	// names are the families the app registered fonts under.
	names []string
	// image is the canvas's pixels read back, reused.
	image []byte
	w, h  int
}

// fontCSS is a CSS font without its size: the style and weight before it,
// the families after.
type fontCSS struct{ pre, family string }

// canvasFont is what the engine keeps of a Font (Font.data): its CSS font,
// and the texts of its glyphs, which a layout kept after the System let go
// of the Font still draws.
type canvasFont struct {
	css    fontCSS
	glyphs *glyphTexts
}

func newCanvas() *canvasEngine {
	e := &canvasEngine{fonts: map[string]*Font{}}
	e.clusterEngine.init(e)
	doc := js.Global().Get("document")
	if doc.IsUndefined() {
		e.canvas = js.Global().Get("OffscreenCanvas").New(64, 64)
	} else {
		e.canvas = doc.Call("createElement", "canvas")
		e.canvas.Set("width", 64)
		e.canvas.Set("height", 64)
	}
	e.w, e.h = 64, 64
	e.ctx = e.canvas.Call("getContext", "2d", map[string]any{"willReadFrequently": true})
	e.ctx.Set("textBaseline", "alphabetic")
	e.ctx.Set("textAlign", "left")
	e.ctx.Set("direction", "ltr")
	e.ctx.Set("fillStyle", "#fff")
	return e
}

// cssFamily turns a list of families into CSS's, with the generic ones
// the system's own.
func cssFamily(list string) string {
	var out []string
	for _, f := range familyList(list) {
		switch generic(f) {
		case "system-ui":
			out = append(out, `system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, "Noto Sans", sans-serif, "Apple Color Emoji", "Segoe UI Emoji"`)
		case "sans-serif":
			out = append(out, "sans-serif")
		case "serif":
			out = append(out, "serif")
		case "monospace":
			out = append(out, `ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace`)
		default:
			out = append(out, strconv.Quote(f))
		}
	}
	if len(out) == 0 {
		return cssFamily("system-ui")
	}
	return strings.Join(out, ", ")
}

// cssOf returns the CSS font of a style.
func cssOf(style Style) fontCSS {
	pre := strconv.Itoa(style.weight()) + " "
	if style.Italic {
		pre = "italic " + pre
	}
	return fontCSS{pre, cssFamily(style.Family)}
}

// at returns the CSS font at size px.
func (c fontCSS) at(size float32) string {
	return c.pre + strconv.FormatFloat(float64(size), 'f', -1, 32) + "px " + c.family
}

func (e *canvasEngine) setFont(css string) {
	if css != e.current {
		e.current = css
		e.ctx.Set("font", css)
	}
}

func (e *canvasEngine) font(style Style) *Font {
	size := style.FontSize()
	fc := cssOf(style)
	css := fc.at(size)
	if f, ok := e.fonts[css]; ok {
		return f
	}
	e.setFont(css)
	m := e.ctx.Call("measureText", "Hg")
	// Shaded: browsers draw text of each color with its own coverage, as
	// Core Text's font smoothing emboldens lighter text more, and Skia
	// corrects coverage by the text's luminance. Thick text is drawn in
	// white, the boldest.
	f := &Font{Size: size, data: &canvasFont{fc, e.glyphs}, shaded: true, thickens: true}
	if a := m.Get("fontBoundingBoxAscent"); a.Type() == js.TypeNumber {
		f.Ascent, f.Descent = float32(a.Float()), float32(m.Get("fontBoundingBoxDescent").Float())
	} else {
		f.Ascent, f.Descent = size*0.9, size*0.25
	}
	// Underlines a tenth of an em below the baseline, strikethroughs
	// through the middle of an x.
	thick := max(size/14, 1)
	f.underlineTop, f.underlineThick = -size/10, thick
	x := e.ctx.Call("measureText", "x").Get("actualBoundingBoxAscent")
	if x.Type() == js.TypeNumber {
		f.strikeTop = float32(x.Float())/2 + thick/2
	} else {
		f.strikeTop = size * 0.3
	}
	f.strikeThick = thick
	e.fonts[css] = f
	return f
}

// measure returns how wide text is in f, kerned unless noKern.
func (e *canvasEngine) measure(f *Font, text string, noKern bool) float32 {
	e.setFont(e.sized(f, f.Size))
	kerning := "normal"
	if noKern {
		kerning = "none"
	}
	if kerning != e.kerning {
		e.kerning = kerning
		e.ctx.Set("fontKerning", kerning)
	}
	return float32(e.ctx.Call("measureText", text).Get("width").Float())
}

// sized returns the CSS font of f at size px.
func (e *canvasEngine) sized(f *Font, size float32) string {
	return f.data.(*canvasFont).css.at(size)
}

// shadeGray is the gray glyphs of text of each shade are drawn in: of
// relative luminance 0, 1/4, 1/2 and 3/4 (ShadeOf), and white for Thick.
// Canvases draw no text unsmoothed: Flat is drawn as the darkest shade.
var shadeGray = [...]uint8{0, 137, 188, 225, 255, 0}

// glyph draws a glyph's text in the gray of its shade and reads its
// coverage back: gray, or color for color fonts (emoji).
func (e *canvasEngine) glyph(f *Font, id uint32, scale, dx float32, shade Shade, _ bool) bitmap {
	// The texts of the glyphs of f's time: the engine's may be newer.
	texts := f.data.(*canvasFont).glyphs.texts
	if int(id) >= len(texts) || id == 0 {
		return bitmap{}
	}
	text := texts[id]
	if strings.TrimSpace(text) == "" {
		return bitmap{}
	}
	// The font at the size text was measured at, the canvas scaled: at
	// the pixels' size, the system's font may take another optical size
	// (SF Pro Display rather than Text), whose glyphs the advances measured
	// do not fit, as pages draw at the CSS size, scaled.
	e.setFont(e.sized(f, f.Size))
	m := e.ctx.Call("measureText", text)
	s := float64(scale)
	l, r := s*m.Get("actualBoundingBoxLeft").Float(), s*m.Get("actualBoundingBoxRight").Float()
	a, d := s*m.Get("actualBoundingBoxAscent").Float(), s*m.Get("actualBoundingBoxDescent").Float()
	const pad = 1
	left := int(math.Floor(-l+float64(dx))) - pad
	right := int(math.Ceil(r+float64(dx))) + pad
	top := int(math.Floor(-a)) - pad
	bottom := int(math.Ceil(d)) + pad
	w, h := right-left, bottom-top
	if w <= 2*pad || h <= 2*pad || w > 2048 || h > 2048 {
		return bitmap{}
	}
	if w > e.w || h > e.h {
		e.w, e.h = max(w, e.w), max(h, e.h)
		e.canvas.Set("width", e.w)
		e.canvas.Set("height", e.h)
		// A canvas resized forgets its state.
		e.current = ""
		e.setFont(e.sized(f, f.Size))
		e.ctx.Set("textBaseline", "alphabetic")
		e.ctx.Set("textAlign", "left")
		e.fill = ""
	}
	gray := shadeGray[0]
	if int(shade) < len(shadeGray) {
		gray = shadeGray[shade]
	}
	if fill := "rgb(" + strconv.Itoa(int(gray)) + "," + strconv.Itoa(int(gray)) + "," + strconv.Itoa(int(gray)) + ")"; fill != e.fill {
		e.fill = fill
		e.ctx.Set("fillStyle", fill)
	}
	e.ctx.Call("clearRect", 0, 0, w, h)
	e.ctx.Call("setTransform", s, 0, 0, s, 0, 0)
	e.ctx.Call("fillText", text, (float64(dx)-float64(left))/s, -float64(top)/s)
	e.ctx.Call("setTransform", 1, 0, 0, 1, 0, 0)
	data := e.ctx.Call("getImageData", 0, 0, w, h).Get("data")
	n := 4 * w * h
	if cap(e.image) < n {
		e.image = make([]byte, n)
	}
	px := e.image[:n]
	js.CopyBytesToGo(px, data)
	// The ink's box: measurements leave room around it.
	x0, y0, x1, y1 := w, h, 0, 0
	for y := range h {
		for x := range w {
			if px[4*(y*w+x)+3] != 0 {
				x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x+1), max(y1, y+1)
			}
		}
	}
	if x0 >= x1 {
		return bitmap{}
	}
	if x0 > 0 || y0 > 0 || x1 < w || y1 < h {
		cw, ch := x1-x0, y1-y0
		for y := range ch {
			copy(px[4*y*cw:4*(y+1)*cw], px[4*((y+y0)*w+x0):4*((y+y0)*w+x1)])
		}
		left, top, w, h = left+x0, top+y0, cw, ch
		n = 4 * w * h
		px = px[:n]
	}
	// Gray text is its gray where it covers: the alpha is the coverage.
	// Others are colored.
	color := false
	far := func(c uint8) bool { return int(c) > int(gray)+32 || int(gray) > int(c)+32 }
	for i := 0; i < n; i += 4 {
		if px[i+3] > 32 && (far(px[i]) || far(px[i+1]) || far(px[i+2])) {
			color = true
			break
		}
	}
	bm := bitmap{left: left, top: top, w: w, h: h, color: color}
	if color {
		bm.pix = make([]byte, n)
		for i := 0; i < n; i += 4 {
			al := uint16(px[i+3])
			bm.pix[i] = uint8((uint16(px[i])*al + 127) / 255)
			bm.pix[i+1] = uint8((uint16(px[i+1])*al + 127) / 255)
			bm.pix[i+2] = uint8((uint16(px[i+2])*al + 127) / 255)
			bm.pix[i+3] = px[i+3]
		}
		return bm
	}
	bm.pix = make([]byte, w*h)
	for i := range w * h {
		bm.pix[i] = px[4*i+3]
	}
	return bm
}

// maxTexts is how many glyphs, texts, the engine keeps before it has the
// System let go of its fonts with them: a glyph is a cluster, or a whole
// word of some scripts, which pile up with what a long session shows.
const maxTexts = 1 << 16

func (e *canvasEngine) fontCount() int {
	if len(e.glyphs.texts) > maxTexts {
		return maxFonts + 1
	}
	return len(e.fonts)
}

// forgetFonts lets go of the fonts, and of the glyphs and measurements.
// A Font kept by a layout keeps its CSS font and its glyphs' texts.
func (e *canvasEngine) forgetFonts() {
	clear(e.fonts)
	e.forgetGlyphs()
}

// releaseFonts has nothing to release: the browser keeps its fonts.
func (e *canvasEngine) releaseFonts() {}

// pens leaves the browser's words to measure, cluster by cluster.
func (e *canvasEngine) pens(*Font, []rune, []int, bool) []float32 { return nil }

// textParams leaves coverage as the browser drew it, without subpixel
// antialiasing, which canvases do not read back.
func (e *canvasEngine) textParams() (scene.TextParams, bool) { return scene.TextParams{}, false }

// appFamilies lists the app's families. A family is monospaced when an
// "i" is as wide as a "W" in it, which tells only once its face has
// loaded.
func (e *canvasEngine) appFamilies() []FontFamily {
	out := make([]FontFamily, len(e.names))
	for i, name := range e.names {
		e.setFont("100px " + strconv.Quote(name))
		mono := e.ctx.Call("measureText", "i").Get("width").Float() == e.ctx.Call("measureText", "W").Get("width").Float()
		out[i] = FontFamily{Name: name, Monospaced: mono}
	}
	return out
}

// systemFamilies lists none: a browser does not tell what fonts it has.
func (e *canvasEngine) systemFamilies() func() []FontFamily { return nil }

// register adds a font file to the document's fonts, as a FontFace of
// family, or of its own family when family is "" (which the browser does
// not tell: the face needs a family). Browsers load faces in the
// background: text laid out before draws with others.
func (e *canvasEngine) register(data []byte, family string) error {
	if family == "" {
		return errNoFamily
	}
	buf := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(buf, data)
	face := js.Global().Get("FontFace").New(family, buf.Get("buffer"))
	fonts := js.Global().Get("document").Get("fonts")
	fonts.Call("add", face)
	face.Call("load").Call("catch", js.Global().Get("Function").New(""))
	e.names = append(e.names, family)
	clear(e.words)
	return nil
}

var errNoFamily = errors.New("mygo: browsers need the family of a font to add")
