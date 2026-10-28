//go:build darwin

package darwin

import "testing"

// TestSecondaryClick tells a status item's right-clicks from its clicks: the
// right button's, and the primary button's with Control held, whatever else is.
func TestSecondaryClick(t *testing.T) {
	const (
		leftUp, rightUp                 = 2, 4
		shift, control, option, command = 1 << 17, 1 << 18, 1 << 19, 1 << 20
	)
	for _, c := range []struct {
		typ, flags uint
		want       bool
	}{
		{leftUp, 0, false},
		{leftUp, shift, false},
		{leftUp, option, false},
		{leftUp, command, false},
		{leftUp, control, true},
		{leftUp, control | shift, true},
		{rightUp, 0, true},
		{rightUp, control, true},
	} {
		if got := secondaryClick(c.typ, c.flags); got != c.want {
			t.Errorf("type %d, flags %#x: %v, want %v", c.typ, c.flags, got, c.want)
		}
	}
}
