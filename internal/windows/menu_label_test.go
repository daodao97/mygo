//go:build windows && (amd64 || arm64)

package windows

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// TestMenuLabel keeps the mnemonic of the label, shows the subtitle's and
// badge's ampersands as they are, and puts both after the label.
func TestMenuLabel(t *testing.T) {
	for _, tt := range []struct{ label, subtitle, badge, want string }{
		{"&Open", "", "", "&Open"},
		{"&Open", "Bed & Bath", "", "&Open — Bed && Bath"},
		{"&Open", "", "12 & 3 ms", "&Open  12 && 3 ms"},
		{"&Open", "Bed & Bath", "12 & 3 ms", "&Open — Bed && Bath  12 && 3 ms"},
	} {
		if got := menuLabel(&platform.MenuItem{Label: tt.label, Subtitle: tt.subtitle, Badge: tt.badge}); got != tt.want {
			t.Errorf("menuLabel(%q, %q, %q) = %q, want %q", tt.label, tt.subtitle, tt.badge, got, tt.want)
		}
	}
}
