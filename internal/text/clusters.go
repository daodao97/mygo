//go:build js || android

package text

import (
	"math"
	"slices"
	"unicode"

	"github.com/go-text/typesetting/bidi"
	"github.com/go-text/typesetting/segmenter"
)

// clusterEngine lays out text with a text stack that measures and draws
// text, rather than giving glyphs: a browser's canvas, Android's Paint.
// The engine's glyphs are grapheme clusters, measured within their words,
// which keeps the fonts' kerning, and drawn alone; a word of a script
// that joins or reorders its letters (Arabic, the scripts of India, …) is
// one glyph, drawn whole. go-text's segmenter finds the clusters and
// where lines may break, and its bidi package the direction of runs. The
// text stack's engine embeds it, and gives it its fonts and measures.
type clusterEngine struct {
	shapeScratch
	host clusterHost
	// emoji are the twins of fonts that draw emoji, unshaded: emoji
	// keep their colors whatever the text's.
	emoji map[*Font]*Font
	// glyphs are the texts of the glyphs of the Fonts made since the
	// System last let go of them.
	glyphs *glyphTexts
	// words caches the pen positions of the clusters of words.
	words map[wordKey][]float32
	seg   segmenter.Segmenter
	para  bidi.Paragraph
}

// clusterHost is the text stack of a clusterEngine.
type clusterHost interface {
	// font returns the font of a style at its size.
	font(style Style) *Font
	// measure returns how wide text is in f, kerned unless noKern.
	measure(f *Font, text string, noKern bool) float32
	// pens returns the pen positions of the boundaries of the clusters
	// of a word, bounds into text, from 0 at its start, as the stack lays
	// the word out, or nil for the engine to measure them.
	pens(f *Font, text []rune, bounds []int, noKern bool) []float32
}

// glyphTexts are the texts of glyphs, by ID: the engine's glyphs are
// texts drawn whole. A Font keeps the table of its time, so that a layout
// kept after the System let go of it still draws.
type glyphTexts struct {
	ids   map[string]uint32
	texts []string
}

func newGlyphTexts() *glyphTexts {
	return &glyphTexts{ids: map[string]uint32{"": 0}, texts: []string{""}}
}

type wordKey struct {
	font   *Font
	word   string
	noKern bool
}

func (e *clusterEngine) init(host clusterHost) {
	e.host = host
	e.glyphs = newGlyphTexts()
	e.words = map[wordKey][]float32{}
}

// forgetGlyphs lets go of the glyphs and the measurements, which Fonts
// that layouts keep still hold.
func (e *clusterEngine) forgetGlyphs() {
	clear(e.emoji)
	clear(e.words)
	e.glyphs = newGlyphTexts()
}

// glyphID returns the ID of a glyph drawing text.
func (e *clusterEngine) glyphID(text string) uint32 {
	g := e.glyphs
	if id, ok := g.ids[text]; ok {
		return id
	}
	id := uint32(len(g.texts))
	g.texts = append(g.texts, text)
	g.ids[text] = id
	return id
}

// noKern reports whether a style's OpenType features turn kerning off,
// the one feature canvases have (fontKerning): the others they leave as
// the font has them.
func noKern(style Style) bool {
	for _, f := range features(style.Features) {
		if f.tag == [4]byte{'k', 'e', 'r', 'n'} {
			return f.value == 0
		}
	}
	return false
}

// cluster is a glyph being laid out: a grapheme cluster, or a word of a
// script whose letters join, of runes [start, end).
type cluster struct {
	start, end int
	font       *Font
	x, advance float32
	level      bidi.Level
	text       string
}

// complexScript reports whether a rune is of a script whose letters
// change with their neighbors, or reorder: its words are drawn whole.
func complexScript(r rune) bool {
	return unicode.In(r, unicode.Arabic, unicode.Syriac, unicode.Nko, unicode.Mongolian, unicode.Devanagari,
		unicode.Bengali, unicode.Gurmukhi, unicode.Gujarati, unicode.Oriya, unicode.Tamil, unicode.Telugu,
		unicode.Kannada, unicode.Malayalam, unicode.Sinhala, unicode.Thai, unicode.Lao, unicode.Tibetan,
		unicode.Myanmar, unicode.Khmer, unicode.Javanese, unicode.Balinese)
}

func (e *clusterEngine) shape(text []rune, style Style, spans []Span, width float32, rtl, wholeWords bool) []shapedLine {
	if len(text) == 0 {
		return nil
	}
	// The style of each rune: spans', then the text's.
	styles := make([]Style, 0, len(spans)+1)
	fontAt := make([]uint8, len(text))
	{
		start := 0
		for _, sp := range spans {
			end := min(sp.End, len(text))
			if end <= start {
				continue
			}
			for i := start; i < end; i++ {
				fontAt[i] = uint8(len(styles))
			}
			styles = append(styles, sp.style(style))
			start = end
			if len(styles) == 255 {
				break
			}
		}
		for i := start; i < len(text); i++ {
			fontAt[i] = uint8(len(styles))
		}
		styles = append(styles, style)
	}
	fonts := make([]*Font, len(styles))
	for i, s := range styles {
		fonts[i] = e.host.font(s)
	}

	// The direction of each rune.
	levels := make([]bidi.Level, len(text))
	mixed := rtl
	for _, r := range text {
		if r >= 0x0590 {
			mixed = true
			break
		}
	}
	if mixed {
		dir := bidi.LeftToRight
		if rtl {
			dir = bidi.RightToLeft
		}
		runs := e.para.Segment(text, dir)
		for i := range runs.NumRuns() {
			r := runs.Run(i)
			for k := r.Start; k < min(r.End, len(text)); k++ {
				levels[k] = r.Level
			}
		}
	}

	// Where lines may break, and the clusters between.
	e.seg.Init(text)
	var breaks []int
	for it := e.seg.LineIterator(); it.Next(); {
		l := it.Line()
		breaks = append(breaks, l.Offset+len(l.Text))
	}
	if width > 0 && keepsAnyWords(style.Lang, spans) {
		// Korean breaks between words (keepWordsWhole).
		at := make([]bool, len(text)+1)
		for _, b := range breaks {
			at[b] = true
		}
		if keepWordsWhole(at, text, style.Lang, spans) {
			breaks = breaks[:0]
			for i, ok := range at {
				if ok {
					breaks = append(breaks, i)
				}
			}
		}
	}
	var bounds []int
	for it := e.seg.GraphemeIterator(); it.Next(); {
		g := it.Grapheme()
		bounds = append(bounds, g.Offset)
	}
	bounds = append(bounds, len(text))

	// The clusters of each piece of text between breaks, of one style and
	// direction, measured as a word.
	var clusters []cluster
	b := 0 // the cluster boundary
	start := 0
	for _, end := range breaks {
		for start < end {
			// The piece: same style, level and script complexity.
			pe := start + 1
			for pe < end && fontAt[pe] == fontAt[start] && levels[pe] == levels[start] {
				pe++
			}
			// Whole clusters only.
			for b < len(bounds)-1 && bounds[b] < start {
				b++
			}
			ce := b
			for ce < len(bounds)-1 && bounds[ce+1] <= pe {
				ce++
			}
			if ce == b {
				ce = b + 1
			}
			pe = bounds[ce]
			st := styles[fontAt[start]]
			f := fonts[fontAt[start]]
			clusters = e.measureWord(clusters, text, bounds[b:ce+1], f, st.LetterSpacing, noKern(st), levels[start])
			b, start = ce, pe
		}
	}

	// Inline objects take their width, whatever the canvas measured of
	// their character.
	{
		start := 0
		for _, sp := range spans {
			end := min(sp.End, len(text))
			if sp.ObjectWidth > 0 {
				for k := range clusters {
					if c := &clusters[k]; c.start >= start && c.start < end {
						c.advance = sp.ObjectWidth
					}
				}
			}
			start = max(start, end)
		}
	}

	// Tabs advance to the next of the stops of the style, from the start
	// of their line, where it sets them; the text stack measures them as
	// spaces otherwise.
	tabbed := style.tabStop > 0 && slices.Contains(text, '\t')
	tabs := func(cs []cluster, x float32) {
		for k := range cs {
			if c := &cs[k]; c.end-c.start == 1 && text[c.start] == '\t' {
				c.advance = tabAdvance(x, style.tabStop)
			}
			x += cs[k].advance
		}
	}

	// Lines: whole pieces between breaks that fit, or clusters of those
	// longer than a line unless wholeWords.
	var lines [][2]int // clusters [i, j)
	if width <= 0 {
		if tabbed {
			tabs(clusters, 0)
		}
		lines = append(lines, [2]int{0, len(clusters)})
	} else {
		lineStart, lineW := 0, float32(0)
		ci := 0
		for _, end := range breaks {
			cj := ci
			for cj < len(clusters) && clusters[cj].start < end {
				cj++
			}
			// The piece's width, less the whitespace ending it.
			measure := func() (w, trail float32) {
				if tabbed {
					tabs(clusters[ci:cj], lineW)
				}
				for k := ci; k < cj; k++ {
					w += clusters[k].advance
				}
				for k := cj - 1; k >= ci && isSpace(text, clusters[k]); k-- {
					trail += clusters[k].advance
				}
				return w, trail
			}
			w, trail := measure()
			if lineW+w-trail > width+1e-3 && ci > lineStart {
				lines = append(lines, [2]int{lineStart, ci})
				lineStart, lineW = ci, 0
				if tabbed {
					w, trail = measure()
				}
			}
			if w-trail > width+1e-3 && !wholeWords {
				// Break the piece within itself.
				for k := ci; k < cj; k++ {
					a := clusters[k].advance
					if lineW+a > width+1e-3 && k > lineStart && !isSpace(text, clusters[k]) {
						lines = append(lines, [2]int{lineStart, k})
						lineStart, lineW = k, 0
					}
					lineW += a
				}
			} else {
				lineW += w
			}
			ci = cj
		}
		lines = append(lines, [2]int{lineStart, len(clusters)})
	}

	out := make([]shapedLine, 0, len(lines))
	for _, ln := range lines {
		cs := clusters[ln[0]:ln[1]]
		if len(cs) == 0 {
			continue
		}
		sl := shapedLine{start: cs[0].start, end: cs[len(cs)-1].end}
		visual := reorder(cs, rtl)
		x := float32(0)
		for k := 0; k < len(visual); {
			c := visual[k]
			r := shapedRun{font: c.font, start: c.start, end: c.end}
			for k < len(visual) && visual[k].font == c.font && visual[k].level%2 == c.level%2 {
				v := visual[k]
				r.start, r.end = min(r.start, v.start), max(r.end, v.end)
				r.glyphs = append(r.glyphs, Glyph{Font: v.font, ID: e.glyphID(v.text), X: x, Advance: v.advance, Cluster: v.start, RTL: v.level%2 == 1})
				x += v.advance
				k++
			}
			sl.runs = append(sl.runs, r)
		}
		out = append(out, sl)
	}
	return out
}

// tabAdvance returns the advance of a tab x DIPs from the start of its
// line to the next stop of those every stop DIPs.
func tabAdvance(x, stop float32) float32 {
	next := (float32(math.Floor(float64(x/stop+1e-4))) + 1) * stop
	return next - x
}

func isSpace(text []rune, c cluster) bool {
	return c.end-c.start == 1 && hangs(text[c.start])
}

// measureWord appends the clusters of a word of runes [bounds[0],
// bounds[len-1]) in f, at level, their boundaries bounds, positioned as
// the browser lays the word out. Clusters of scripts whose letters join
// make one, which the browser shapes whole.
func (e *clusterEngine) measureWord(out []cluster, text []rune, bounds []int, f *Font, spacing float32, noKern bool, level bidi.Level) []cluster {
	for k := 0; k < len(bounds)-1; {
		j := k + 1
		joins := complexScript(text[bounds[k]])
		for j < len(bounds)-1 && complexScript(text[bounds[j]]) == joins {
			j++
		}
		if joins {
			word := string(text[bounds[k]:bounds[j]])
			w := e.host.measure(f, word, noKern)
			out = append(out, cluster{start: bounds[k], end: bounds[j], font: f, advance: w + spacing*float32(j-k), level: level, text: word})
		} else {
			out = e.measureClusters(out, text, bounds[k:j+1], f, spacing, noKern, level)
		}
		k = j
	}
	return out
}

// measureClusters appends clusters of runes [bounds[0], bounds[len-1]),
// each drawn alone, at the positions the browser gives them in the whole.
func (e *clusterEngine) measureClusters(out []cluster, text []rune, bounds []int, f *Font, spacing float32, noKern bool, level bidi.Level) []cluster {
	start, end := bounds[0], bounds[len(bounds)-1]
	key := wordKey{f, string(text[start:end]), noKern}
	pens, ok := e.words[key]
	if !ok {
		pens = e.host.pens(f, text[start:end], rebase(bounds, start), noKern)
	}
	if !ok && pens == nil {
		pens = make([]float32, len(bounds))
		if len(bounds) > 33 {
			// Long words: each cluster alone.
			for k := 1; k < len(bounds); k++ {
				pens[k] = pens[k-1] + e.host.measure(f, string(text[bounds[k-1]:bounds[k]]), noKern)
			}
		} else {
			// A cluster ends where the text through it ends, and starts
			// its own width before: kerning with the cluster before it
			// moves it, as the browser draws the word, not the next one.
			n := len(bounds) - 1
			pens[n] = e.host.measure(f, string(text[start:end]), noKern)
			for k := 1; k < n; k++ {
				through := e.host.measure(f, string(text[start:bounds[k+1]]), noKern)
				alone := e.host.measure(f, string(text[bounds[k]:bounds[k+1]]), noKern)
				pens[k] = max(through-alone, pens[k-1])
			}
		}
	}
	if !ok {
		if len(e.words) > 1<<14 {
			clear(e.words)
		}
		e.words[key] = pens
	}
	for k := 1; k < len(bounds); k++ {
		c := cluster{
			start: bounds[k-1], end: bounds[k], font: f,
			advance: max(pens[k]-pens[k-1], 0) + spacing, level: level,
			text: string(text[bounds[k-1]:bounds[k]]),
		}
		if isEmoji(text[bounds[k-1]:bounds[k]]) {
			c.font = e.emojiFont(f)
		}
		out = append(out, c)
	}
	return out
}

// isEmoji reports whether a cluster is an emoji: a pictograph, a flag, or
// a character with the emoji presentation selector.
func isEmoji(cluster []rune) bool {
	r := cluster[0]
	if r >= 0x1F000 && r <= 0x1FAFF || r >= 0x2600 && r <= 0x27BF {
		return true
	}
	for _, r := range cluster[1:] {
		if r == 0xFE0F {
			return true
		}
	}
	return false
}

// emojiFont returns f's twin for emoji.
func (e *clusterEngine) emojiFont(f *Font) *Font {
	if t, ok := e.emoji[f]; ok {
		return t
	}
	if e.emoji == nil {
		e.emoji = map[*Font]*Font{}
	}
	t := *f
	t.shaded, t.thickens = false, false
	e.emoji[f] = &t
	return &t
}

// reorder returns the clusters of a line in visual order (UAX #9's L2):
// from the highest level down to the lowest odd one, runs at that level or
// higher reverse.
func reorder(cs []cluster, rtl bool) []cluster {
	var hi bidi.Level
	lo := bidi.Level(127)
	for _, c := range cs {
		hi = max(hi, c.level)
		if c.level%2 == 1 {
			lo = min(lo, c.level)
		}
	}
	if hi == 0 {
		return cs
	}
	v := append([]cluster(nil), cs...)
	// Whitespace ending the line takes the paragraph's level (L1).
	base := bidi.Level(0)
	if rtl {
		base = 1
	}
	for k := len(v) - 1; k >= 0 && v[k].end-v[k].start == 1 && v[k].text == " "; k-- {
		v[k].level = base
	}
	for l := hi; l >= lo && l > 0; l-- {
		for i := 0; i < len(v); {
			if v[i].level < l {
				i++
				continue
			}
			j := i
			for j < len(v) && v[j].level >= l {
				j++
			}
			for a, b := i, j-1; a < b; a, b = a+1, b-1 {
				v[a], v[b] = v[b], v[a]
			}
			i = j
		}
	}
	return v
}

// positions are a quarter of a pixel apart, the nearest to the pen, as
// Skia, which draws the text of Chrome's canvases, places glyphs.
func (e *clusterEngine) positions(*Font, float32, bool) (int, bool) { return 4, true }

func (e *clusterEngine) baseline(y float32) float32 { return float32(math.Round(float64(y))) }

// join: browsers' canvases blend each glyph of a run in turn (Skia).
func (e *clusterEngine) join() bool { return false }

// decorate places underlines and strikethroughs along the advances of the
// runs, at the positions of their fonts, whole pixels from the baseline.
func (e *clusterEngine) decorate(r decoRange) []Stroke {
	s := r.scale
	base := e.baseline(r.baseline() * s)
	var out []Stroke
	r.runs(func(f *Font, i, j int) {
		x0, x1 := r.span(i, j)
		top, thick := f.strikeTop, f.strikeThick
		if r.d == Underline {
			top, thick = f.underlineTop, f.underlineThick
		}
		t := float32(math.Round(float64(base - top*s)))
		st := Stroke{X0: (r.x + x0) * s, X1: (r.x + x1) * s, Top: t, Bottom: t + max(float32(math.Round(float64(thick*s))), 1)}
		if n := len(out); n > 0 && out[n-1].Top == st.Top && out[n-1].Bottom == st.Bottom {
			out[n-1].X1 = max(out[n-1].X1, st.X1)
			return
		}
		out = append(out, st)
	})
	return out
}

// rebase returns bounds less start.
func rebase(bounds []int, start int) []int {
	out := make([]int, len(bounds))
	for i, b := range bounds {
		out[i] = b - start
	}
	return out
}
