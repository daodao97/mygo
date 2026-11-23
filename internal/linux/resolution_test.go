//go:build linux && !android && (amd64 || arm64)

package linux

import "testing"

func TestSaneResolution(t *testing.T) {
	for _, c := range []struct {
		in, want float64
		changed  bool
	}{
		{-1, 96, true}, // unset
		{0, 96, true},
		{1, 96, true},
		{2, 96, true},
		{47.9, 96, true},
		{48, 48, false}, // text scaling factor 0.5
		{96, 96, false},
		{144, 144, false},
		{288, 288, false}, // X11 at 3x
	} {
		if got, changed := saneResolution(c.in); got != c.want || changed != c.changed {
			t.Errorf("saneResolution(%v) = %v, %v; want %v, %v", c.in, got, changed, c.want, c.changed)
		}
	}
}
