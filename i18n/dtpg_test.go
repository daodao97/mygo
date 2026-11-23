package i18n

import "testing"

// TestBestPatternKeepsTextForNumber asks for a numeric field that the
// nearest pattern writes as text: the text stays, as ICU 78 keeps it
// (ICU-23114, whose test this is, with English patterns of CLDR 47).
func TestBestPatternKeepsTextForNumber(t *testing.T) {
	g := &patternGenerator{}
	g.add("y G", "Gy", true)
	g.add("M/d/y G", "GyMd", true)
	g.add("MMM y G", "GyMMM", true)
	g.add("MMM d, y G", "GyMMMd", true)
	g.add("EEE, MMM d, y G", "GyMMMEd", true)
	if got, want := g.bestPattern("GyMEd"), "EEE, MMM d, y G"; got != want {
		t.Errorf("bestPattern(GyMEd) = %q, want %q", got, want)
	}
}
