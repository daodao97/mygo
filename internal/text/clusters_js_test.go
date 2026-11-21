package text

import (
	"testing"
)

// runRecorder is a clusterHost that lays pieces out within their words
// (runHost), as Android's, every rune 10 DIPs wide; it keeps the runs it
// measured.
type runRecorder struct {
	runs []recordedRun
}

type recordedRun struct {
	text string
	run  textRun
}

func (h *runRecorder) font(style Style) *Font { return &Font{Size: style.FontSize()} }

func (h *runRecorder) measure(f *Font, text string, noKern bool) float32 {
	return 10 * float32(len([]rune(text)))
}

func (h *runRecorder) pens(*Font, []rune, []int, bool) []float32 { return nil }

func (h *runRecorder) measureRun(f *Font, text []rune, run textRun, noKern bool) float32 {
	h.runs = append(h.runs, recordedRun{string(text), run})
	return 10 * float32(run.end-run.start)
}

// TestClusterRunHost checks that a host laying pieces of words out within
// their words gets each piece of an Arabic word whose style changes with
// the rest of the word around it, and every such word with its run's
// direction, measured and drawn alike: Android's joined letters broke
// apart at a tajweed color, and its Arabic words were measured left to
// right (#2171).
func TestClusterRunHost(t *testing.T) {
	h := &runRecorder{}
	var e clusterEngine
	e.init(h)

	word := []rune("ٱلرَّحْمَٰنِ")
	text := append(append([]rune{}, word...), []rune(" لِلَّهِ")...)
	// Spans end after the first letter, and within the word.
	spans := []Span{{End: 1}, {End: 6}}
	lines := e.shape(text, Style{Size: 16}, spans, 0, true, false)
	if len(lines) != 1 {
		t.Fatalf("%d lines, want 1", len(lines))
	}

	// The measured pieces of the first word: each within the whole word,
	// right to left, side by side.
	var pieces []recordedRun
	for _, r := range h.runs {
		if r.text == string(word) {
			pieces = append(pieces, r)
		}
	}
	if len(pieces) < 2 {
		t.Fatalf("measured %+v, want the pieces of %q within it", h.runs, string(word))
	}
	at := 0
	for _, p := range pieces {
		if !p.run.rtl || p.run.start != at || p.run.end <= p.run.start {
			t.Errorf("piece %+v, want one right to left from rune %d", p.run, at)
		}
		at = p.run.end
	}
	if at != len(word) {
		t.Errorf("pieces end at rune %d of %d", at, len(word))
	}

	// The second word, of one style, laid out right to left alone: the
	// space and the word before it are no context of it.
	found := false
	for _, r := range h.runs {
		if r.text == "لِلَّهِ" {
			found = true
			if !r.run.rtl || r.run.start != 0 || r.run.end != len([]rune(r.text)) {
				t.Errorf("second word measured as %+v", r.run)
			}
		}
	}
	if !found {
		t.Errorf("measured %+v, want لِلَّهِ alone", h.runs)
	}

	// The glyphs draw what was measured: the pieces within the word.
	drawn := map[textRun]bool{}
	for _, ln := range lines {
		for _, r := range ln.runs {
			for _, g := range r.glyphs {
				if e.glyphs.texts[g.ID] == string(word) {
					drawn[e.glyphs.run(g.ID)] = true
				}
			}
		}
	}
	for _, p := range pieces {
		if !drawn[p.run] {
			t.Errorf("piece %+v measured but not drawn; drew %v", p.run, drawn)
		}
	}

	// A word of a left-to-right paragraph is still laid out right to
	// left.
	h.runs = nil
	e.shape([]rune("hi لِلَّهِ"), Style{Size: 16}, nil, 0, false, false)
	if len(h.runs) != 1 || !h.runs[0].run.rtl {
		t.Errorf("measured %+v, want لِلَّهِ right to left", h.runs)
	}
}
