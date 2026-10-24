package text

import (
	"fmt"
	"testing"
)

// BenchmarkCaretCold measures the first caret query after an edit invalidates
// its geometry. Fixed advances isolate navigation from native text shaping.
func BenchmarkCaretCold(b *testing.B) {
	for _, n := range []int{64, 1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			l := Layout{Runes: make([]rune, n), Lines: []Line{{End: n, Width: float32(n * 8), Height: 20, Glyphs: make([]Glyph, n)}}}
			for i := range l.Runes {
				l.Runes[i] = 'a'
				l.Lines[0].Glyphs[i] = Glyph{X: float32(i * 8), Advance: 8, Cluster: i, Runes: 1}
			}
			line := l.Lines[0]
			b.ReportAllocs()
			for b.Loop() {
				l.Lines[0] = line
				l.boundaries = nil
				x, _, _ := l.CaretAt(CaretPosition{Index: n})
				if x != float32(n*8) {
					b.Fatal(x)
				}
			}
		})
	}
}

// BenchmarkVisualStopsLongLine computes the visual caret stops of one very
// long line, as opening a minified or generated file does. Fixed advances
// keep the fixture independent of installed fonts; shaping and setup are not
// timed, and the stops cache is dropped every iteration, as an edit does.
func BenchmarkVisualStopsLongLine(b *testing.B) {
	const runes = 200_000
	l := &Layout{Runes: make([]rune, runes), Lines: []Line{{Start: 0, End: runes, Width: runes * 8, Height: 20}}}
	for i := range l.Runes {
		l.Runes[i] = 'a'
	}
	line := &l.Lines[0]
	line.Glyphs = make([]Glyph, runes)
	for i := range line.Glyphs {
		line.Glyphs[i] = Glyph{X: float32(i) * 8, Advance: 8, Cluster: i, Runes: 1}
	}
	b.ReportAllocs()
	for b.Loop() {
		line.stops = nil
		stops := l.visualStops(0)
		if len(stops) != 2*runes {
			b.Fatalf("got %d stops, want %d", len(stops), 2*runes)
		}
	}
}
