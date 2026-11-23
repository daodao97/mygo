//go:build darwin && !ios

package darwin

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

type stubTextClient struct {
	sel      platform.TextSelection
	inserted []string
}

func (c *stubTextClient) TextForRange(r platform.TextRange) (string, platform.TextRange) {
	return "", r
}
func (c *stubTextClient) Selection() platform.TextSelection       { return c.sel }
func (c *stubTextClient) MarkedRange() (platform.TextRange, bool) { return platform.TextRange{}, false }
func (c *stubTextClient) ReplaceText(_ *platform.TextRange, s string) {
	c.inserted = append(c.inserted, s)
}
func (c *stubTextClient) SetMarkedText(_ *platform.TextRange, _ string, _ platform.TextRange) {
}
func (c *stubTextClient) UnmarkText() {}
func (c *stubTextClient) BoundsForRange(r platform.TextRange) (platform.RectF, platform.TextRange, bool) {
	return platform.RectF{X: 10, Y: 20, W: 1, H: 16}, r, true
}
func (c *stubTextClient) IndexForPoint(_, _ float64) (int, bool) { return 0, false }

func TestSurfaceInsertTextWhileBlurred(t *testing.T) {
	client := &stubTextClient{sel: platform.TextSelection{Range: platform.TextRange{Start: 3, End: 5}}}
	s := &surface{
		blurred: platform.TextInputState{Active: true, Client: client},
	}

	if got := s.selectedRange(); got != (nsRange{Location: 3, Length: 2}) {
		t.Fatalf("selectedRange while blurred = %+v, want {3 2}", got)
	}

	s.insertText("😀", nsRange{Location: nsNotFound})
	if len(client.inserted) != 1 || client.inserted[0] != "😀" {
		t.Fatalf("insertText while blurred delivered %q, want [😀]", client.inserted)
	}

	// Clearing blurred (e.g. on resignFirstResponder) disables fallback.
	s.blurred = platform.TextInputState{}
	if got := s.selectedRange(); got.Location != nsNotFound {
		t.Fatalf("selectedRange after clearing blurred = %+v, want nsNotFound", got)
	}
}
