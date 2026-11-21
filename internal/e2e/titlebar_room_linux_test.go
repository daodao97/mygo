//go:build linux && !android && (amd64 || arm64)

package e2e

import (
	"sync"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/linux"
	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

// The room the window controls take over the page of a window with a
// hidden title bar is the buttons and the bar's padding. GTK's header bars
// keep room for a title even when they have none — its sizing box asks
// five characters (GTK's MIN_TITLE_CHARS) whatever the title — and a bar
// that held that phantom had the page keep a whole button's width clear of
// the buttons it cannot see, over a strip that took the title bar's clicks
// for its own. The bars carry an empty custom title, so the room, less the
// buttons, is only spacing and the theme's padding.
func TestTitleBarRoomFollowsButtons(t *testing.T) {
	var mu sync.Mutex
	var bar ui.TitleBar
	w := newWindow(t, mygo.WindowOptions{Width: 480, Height: 220,
		TitleBarStyle:  mygo.TitleBarHidden,
		TitleBarHeight: 50,
		Content: ui.View(func(c *ui.Context) {
			mu.Lock()
			bar = c.TitleBar()
			mu.Unlock()
			ui.Row(c).Height(bar.Height).Shrink(0).Padding(0, bar.Right+16, 0, bar.Left+14).
				AlignItems(ui.Center).DragWindow().Children(func() {
				ui.Text(c, "room")
			})
		}),
	})

	// The room and the buttons, read together on the main thread, once GTK
	// has laid the controls out.
	var right float32
	var buttons [][2]int
	read := func() bool {
		ok := false
		mygo.RunOnMain(func() {
			buttons = linux.TestTitleButtonSizes(w.NativeHandle())
			mu.Lock()
			right = bar.Right
			mu.Unlock()
			ok = len(buttons) > 0 && right > 0
		})
		return ok
	}
	if !wait.Poll(read) {
		t.Skip("this desktop's button layout names no window buttons")
	}

	total := 0
	for _, s := range buttons {
		total += s[0]
	}
	// Besides spacing between the bars' children and the theme's header-bar
	// padding and margins, a few dozen px, the room holds nothing. The
	// empty title kept ~69 px more, half a button's width of it nothing
	// draws in.
	if gap := int(right) - total; gap < 0 || gap > 55 {
		t.Errorf("TitleBar().Right is %v for %d px of buttons in %d: %d px of the bar is neither buttons, spacing nor padding",
			right, total, len(buttons), gap)
	}
}
