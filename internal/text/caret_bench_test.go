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
