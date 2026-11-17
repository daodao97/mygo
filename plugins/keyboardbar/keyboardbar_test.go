package keyboardbar

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestCapShowsAndLabelNames(t *testing.T) {
	var got string
	expanded := false
	tt := ui.NewTester(func(c *ui.Context) {
		Bar(c, []Action{{ID: "backspace", Label: "Backspace", Cap: "⌫"}}, &expanded, func(id string) { got = id })
	}, 400, 100)
	// HasText also reads accessible names, so only the cap's presence is
	// checked here; the label is checked by clicking it.
	if !tt.HasText("⌫") {
		t.Fatal("the key does not show its cap")
	}
	tt.Click("Backspace")
	if got != "backspace" {
		t.Fatalf("the key's accessible name does not reach it: %q", got)
	}
}
