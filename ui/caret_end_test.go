package ui_test

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// A caret that the text's alignment puts against the right of the box
// shows inside the box, as it does where the text scrolls: drawn on the
// edge, the input's clip would cut it away. A text area's too, where a
// paragraph laid out right to left starts at the right.
func TestRightAlignedCaretShows(t *testing.T) {
	for _, test := range []struct {
		name, value string
		align       ui.Align
		dir         ui.Direction
		area        bool
		key         ui.Key
	}{
		{"end empty", "", ui.End, ui.LeftToRight, false, ui.KeyEnd},
		{"end after text", "abc", ui.End, ui.LeftToRight, false, ui.KeyEnd},
		{"start right to left", "", ui.Start, ui.RightToLeft, false, ui.KeyEnd},
		{"area end empty", "", ui.End, ui.LeftToRight, true, ui.KeyEnd},
		{"area start right to left", "", ui.Start, ui.RightToLeft, true, ui.KeyEnd},
		{"area after left to right text, right to left", "abc", ui.Start, ui.RightToLeft, true, ui.KeyEnd},
		{"area before right to left text", "שלום", ui.Start, ui.LeftToRight, true, ui.KeyHome},
	} {
		t.Run(test.name, func(t *testing.T) {
			v := test.value
			field := ui.TextInputBase
			if test.area {
				field = ui.TextAreaBase
			}
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Padding(20).Children(func() {
					field(c, &v).Width(200).TextAlign(test.align).Direction(test.dir).AutoFocus().Label("field")
				})
			}, 260, 120)
			tt.Key(0, test.key)
			box, ok := tt.Find("field")
			if !ok {
				t.Fatal("no field")
			}
			on := tt.Image()
			pix := append([]uint8(nil), on.Pix...)
			tt.Advance(530 * time.Millisecond)
			off := tt.Image()
			blinked := -1
			for i := range pix {
				if pix[i] != off.Pix[i] {
					blinked = max(blinked, (i%on.Stride)/4)
				}
			}
			if blinked < 0 {
				t.Fatalf("no caret blinks in %v", box)
			}
			if right := int(box.X + box.W); blinked >= right || blinked < right-3 {
				t.Errorf("the caret blinks at column %d, not at the right of %v", blinked, box)
			}
		})
	}
}
