package ui

import (
	"slices"
	"testing"
)

// The toolbar's overflow button is chrome: when every control fits, the
// button is hidden and must not widen the toolbar. Regression test for
// https://github.com/egoist/mygo/issues/491
func TestToolbarEndsAtLastControl(t *testing.T) {
	tt := NewTester(func(c *Context) {
		Row(c).Width(400).Children(func() {
			Text(c, "Changes")
			Spacer(c)
			Toolbar(c, func() {
				Button(c, "One")
				Button(c, "Two").Label("Last")
			}).Padding(0).Label("Tools")
		})
	}, 400, 100)
	bar, _ := tt.Find("Tools")
	last, _ := tt.Find("Last")
	if end := last.X + last.W; end != bar.X+bar.W {
		t.Fatalf("last button ends at %v, toolbar ends at %v", end, bar.X+bar.W)
	}
}

// The overflow menu has none of the controls nobody sees: those hidden or
// invisible, in a group or beside it.
func TestToolbarOverflowLeavesOutHidden(t *testing.T) {
	on := false
	for _, tc := range []struct {
		name  string
		leave func(Element)
	}{
		{"hidden", func(e Element) { e.Hidden(true) }},
		{"invisible", func(e Element) { e.Invisible() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tt := NewTester(func(c *Context) {
				Toolbar(c, func() {
					Button(c, "Alpha")
					Button(c, "Bravo")
					ToggleGroup(c, func() {
						Toggle(c, &on, "Bold")
						tc.leave(Toggle(c, &on, "Italic"))
						Toggle(c, &on, "Underline")
					})
					tc.leave(Button(c, "Share"))
					Button(c, "Print")
				}).Fill()
			}, 120, 300)
			tt.Click("More")
			menu := tt.Menu()
			if i := slices.Index(menu, "Bold"); i < 0 || !slices.Equal(menu[i:], []string{"Bold", "Underline", "-", "Print"}) {
				t.Errorf("the overflow menu is %q", menu)
			}
		})
	}
}
