//go:build linux && !android && (amd64 || arm64)

package linux

import (
	"bytes"
	"image"
	"image/png"
	"slices"
	"testing"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

// indicatorCalls stands in for libayatana and GTK's menus, recording the
// calls a tray makes, and gives the tests a fresh watcher state.
func indicatorCalls(t *testing.T) *[]string {
	t.Helper()
	var calls []string
	str := func(p *byte) string { return goStr(ptr(unsafe.Pointer(p))) }
	oldIcon, oldLabel, oldTitle, oldTip, oldMenu := appIndicatorSetIconFull, appIndicatorSetLabel, appIndicatorSetTitle, appIndicatorSetTooltipFull, appIndicatorSetMenu
	oldMenuNew, oldShowAll, oldDrop, oldWatcher := gtkMenuNew, gtkWidgetShowAll, dropFallbackTooltips, trayWatcher
	t.Cleanup(func() {
		appIndicatorSetIconFull, appIndicatorSetLabel, appIndicatorSetTitle, appIndicatorSetTooltipFull, appIndicatorSetMenu = oldIcon, oldLabel, oldTitle, oldTip, oldMenu
		gtkMenuNew, gtkWidgetShowAll, dropFallbackTooltips, trayWatcher = oldMenuNew, oldShowAll, oldDrop, oldWatcher
	})
	appIndicatorSetIconFull = func(_ ptr, file, _ *byte) { calls = append(calls, "icon "+str(file)) }
	appIndicatorSetLabel = func(_ ptr, label, _ *byte) { calls = append(calls, "label "+str(label)) }
	appIndicatorSetTitle = func(_ ptr, title *byte) { calls = append(calls, "title "+str(title)) }
	appIndicatorSetTooltipFull = func(_ ptr, _, title, _ *byte) { calls = append(calls, "tooltip "+str(title)) }
	appIndicatorSetMenu = func(_, _ ptr) { calls = append(calls, "menu") }
	gtkMenuNew = func() ptr { return 1 }
	gtkWidgetShowAll = func(ptr) {}
	dropFallbackTooltips = func(ptr) { calls = append(calls, "drop tooltips") }
	trayWatcher = struct{ subscribed, seen, away bool }{subscribed: true}
	return &calls
}

func testTray(t *testing.T) (*Backend, *tray) {
	b := &Backend{trays: map[int]*tray{}}
	tr := &tray{b: b, id: 1, ind: 1, iconDir: t.TempDir()}
	b.trays[tr.id] = tr
	return b, tr
}

func pngImage(t *testing.T) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestTrayWithoutWatcher checks that a session where no watcher was ever
// seen, whose only icon is libayatana's fallback, calls the indicator
// as before, even when told the watcher has no owner.
func TestTrayWithoutWatcher(t *testing.T) {
	calls := indicatorCalls(t)
	b, tr := testTray(t)
	b.watcherChanged(false)
	tr.SetToolTip("tip")
	tr.SetTitle("title")
	if want := []string{"title tip", "tooltip tip", "label title"}; !slices.Equal(*calls, want) || tr.pending {
		t.Errorf("calls %q, pending %v; want %q, not pending", *calls, tr.pending, want)
	}
}

// TestTrayHoldsWhileWatcherAway checks that once a watcher seen goes
// away, the tray keeps what the app sets, empty strings too, without
// calling the indicator, and gives it all once when a watcher returns,
// the image under a new file name.
func TestTrayHoldsWhileWatcherAway(t *testing.T) {
	calls := indicatorCalls(t)
	b, tr := testTray(t)
	b.watcherChanged(true)
	img := pngImage(t)
	if err := tr.SetImage(img, false); err != nil {
		t.Fatal(err)
	}
	tr.SetTitle("before")
	*calls = nil

	b.watcherChanged(false)
	menu := &platform.Menu{}
	if err := tr.SetImage(img, false); err != nil {
		t.Fatal(err)
	}
	tr.SetTitle("")
	tr.SetToolTip("away")
	tr.SetMenu(menu)
	if len(*calls) != 0 {
		t.Fatalf("called the indicator while the watcher was away: %q", *calls)
	}
	if !tr.pending || !bytes.Equal(tr.image, img) || tr.title != "" || tr.tip != "away" || tr.spec != menu {
		t.Fatalf("state not kept: pending %v, title %q, tip %q, menu kept %v", tr.pending, tr.title, tr.tip, tr.spec == menu)
	}

	b.watcherChanged(true)
	want := []string{"drop tooltips", "icon " + tr.iconDir + "/icon-2.png", "label ", "title away", "tooltip away", "menu"}
	if !slices.Equal(*calls, want) || tr.pending {
		t.Errorf("on return: calls %q, pending %v; want %q, not pending", *calls, tr.pending, want)
	}
	*calls = nil
	b.watcherChanged(true)
	if want := []string{"drop tooltips"}; !slices.Equal(*calls, want) {
		t.Errorf("a second owner reapplied the state: calls %q, want %q", *calls, want)
	}
}

// TestTrayDestroyedWhileHeld checks that a tray destroyed while it held
// calls gives the indicator nothing when the watcher returns.
func TestTrayDestroyedWhileHeld(t *testing.T) {
	calls := indicatorCalls(t)
	b, tr := testTray(t)
	b.watcherChanged(true)
	b.watcherChanged(false)
	tr.SetToolTip("away")
	oldStatus, oldUnref := appIndicatorSetStatus, gObjectUnref
	t.Cleanup(func() { appIndicatorSetStatus, gObjectUnref = oldStatus, oldUnref })
	appIndicatorSetStatus = func(ptr, int32) {}
	gObjectUnref = func(ptr) {}
	tr.Destroy()
	*calls = nil
	b.watcherChanged(true)
	tr.resume()
	if len(*calls) != 0 {
		t.Errorf("a destroyed tray called the indicator: %q", *calls)
	}
}
