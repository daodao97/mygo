package ui

import "testing"

func TestCustomTouchSelectionKeepsPointerPolicyInSharedUI(t *testing.T) {
	custom := true
	tt := NewTester(func(c *Context) {
		input := Box(c).Fill().Focusable().Label("input").TextCaret(Rect{W: 1, H: 20}).HandleInput(func(InputEvent) bool { return true })
		if custom {
			input.TouchSelection()
		}
	}, 320, 200)
	if err := tt.Click("input"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.h.ime.Active || !tt.h.ime.Options.PointerInput {
		t.Fatal("custom text client did not retain pointer ownership")
	}
	custom = false
	tt.Frame()
	if tt.h.ime.Options.PointerInput {
		t.Fatal("default pointer policy was not restored")
	}
}
