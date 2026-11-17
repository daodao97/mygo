package ui_test

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

// A popup of SelectBase's, a panel with a scroll container that shrinks
// inside it, is no taller than the room the window has, as ui.Select's:
// its options scroll inside the window, and End shows the last (#1956).
// One that fits keeps its height.
func TestAttachToFitsRoom(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int
		want  float32 // the panel's height
	}{
		// 140 DIP above the trigger, 148 below it: the panel takes the
		// room below, less its top margin and the window's edge.
		{"long", 30, 320 - (140 + 32) - 4 - 4},
		// Three options and the panel's padding fit below the trigger.
		{"short", 3, 3*32 + 2*4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := make([]string, tc.count)
			for i := range options {
				options[i] = fmt.Sprintf("Option %02d", i)
			}
			last := options[len(options)-1]
			selected := options[0]
			var panel ui.Rect
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Padding(140, 20).Children(func() {
					sel := ui.SelectBase(c, &selected)
					sel.Trigger.Height(32).Padding(0, 10).Label("Choice").Children(func() {
						ui.Text(c, selected)
					})
					p := sel.Popup(func(panel ui.Element) {
						panel.Margin(4, 0, 0, 0).Padding(4).MaxHeight(256)
						ui.Scroll(c).Shrink(1).Children(func() {
							for _, opt := range options {
								sel.Item(opt).Height(32).Padding(0, 10).Children(func() {
									ui.Text(c, opt)
								})
							}
						})
					})
					panel = ui.Rect{}
					if p.Valid() {
						panel = p.Bounds()
					}
				})
			}, 400, 320)
			if err := tt.Click("Choice"); err != nil {
				t.Fatal(err)
			}
			tt.Key(0, ui.KeyEnd)
			tt.Frame()
			if panel.Y != 140+32+4 || panel.H != tc.want {
				t.Errorf("the list box is %+v, want at y %v, %v high", panel, 140+32+4, tc.want)
			}
			if bottom := panel.Y + panel.H; bottom > 320-4 {
				t.Errorf("the list box ends at %v, past the window's edge", bottom)
			}
			r, ok := tt.Find(last)
			if !ok || r.Y < panel.Y || r.Y+r.H > panel.Y+panel.H || r.Y+r.H > 320-4 {
				t.Errorf("%s after End is at %+v (found %v), outside the list box %+v or the window", last, r, ok, panel)
			}
		})
	}
}
