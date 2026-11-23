package ui

import (
	"runtime"
	"testing"
)

// Copy with nothing selected in selectable text, after a click that leaves
// only a caret, goes to the shortcuts of its key, as a window's Cmd+C
// copying rows chosen otherwise, and the Edit menu enables Copy for it
// (#2244). A selection still copies.
func TestSelectableCopyNothingGoesToShortcuts(t *testing.T) {
	for _, alone := range []bool{false, true} {
		fired := 0
		tt := NewTester(func(c *Context) {
			if c.Shortcut(Cmd, KeyC) {
				fired++
			}
			if alone {
				Text(c, "alpha beta").Selectable()
				return
			}
			Column(c).Selectable().Children(func() {
				Text(c, "alpha beta")
				Text(c, "gamma")
			})
		}, 400, 100)
		tt.ClickAt(selectableAt(t, tt, "alpha beta", 2))
		tt.Frame()
		tt.SetClipboard("before")
		tt.Key(Cmd, KeyC)
		if fired != 1 || tt.Clipboard() != "before" {
			t.Fatalf("alone=%v: Cmd+C with nothing selected: shortcut fired %d times, clipboard %q", alone, fired, tt.Clipboard())
		}
		if runtime.GOOS == "darwin" {
			// The Edit menu takes the key before the window there.
			if !tt.rt.commandEnabled("copy") {
				t.Errorf("alone=%v: Edit › Copy disabled though a shortcut takes it", alone)
			}
			tt.Command("copy")
			if fired != 2 {
				t.Errorf("alone=%v: Edit › Copy with nothing selected: shortcut fired %d times, want 2", alone, fired)
			}
		}

		tt.Key(Cmd, KeyA)
		before := fired
		tt.Key(Cmd, KeyC)
		tt.SetClipboard("before")
		tt.Command("copy")
		if fired != before || tt.Clipboard() == "before" {
			t.Errorf("alone=%v: with a selection, Copy went to the shortcut (%d) or copied %q", alone, fired-before, tt.Clipboard())
		}
	}
}

// Without a shortcut for it, Copy with nothing selected stays disabled.
func TestSelectableCopyNothingDisabled(t *testing.T) {
	tt := NewTester(func(c *Context) {
		Column(c).Selectable().Children(func() { Text(c, "alpha beta") })
	}, 400, 100)
	tt.ClickAt(selectableAt(t, tt, "alpha beta", 2))
	tt.Frame()
	if tt.rt.commandEnabled("copy") {
		t.Error("Edit › Copy enabled with nothing selected and no shortcut")
	}
}
