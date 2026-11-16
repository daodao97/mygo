//go:build linux

package e2e

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/linux"
)

// TestTrayActivation checks that a panel's activation, the double click on
// GNOME's AppIndicator icons, reaches Tray.OnClick, and that an icon whose
// clicks have nowhere to go answers no activation, so that a panel opens
// its menu on the first click.
func TestTrayActivation(t *testing.T) {
	tray, err := mygo.NewTray(mygo.TrayOptions{Icon: squarePNG(), ToolTip: "activation"})
	if err != nil {
		t.Skip("no tray: ", err)
	}
	defer tray.Destroy()
	var activates bool
	mygo.RunOnMain(func() { activates = linux.TestTrayActivates() })
	if !activates {
		t.Skip("this libayatana-appindicator3 has no activate signal")
	}

	var activated bool
	mygo.RunOnMain(func() { activated = linux.TestActivateTray() })
	if activated {
		t.Fatal("an icon with no click listener answered activation")
	}

	clicked := make(chan struct{}, 1)
	off := tray.OnClick(func() { clicked <- struct{}{} })
	mygo.RunOnMain(func() { activated = linux.TestActivateTray() })
	if !activated {
		t.Fatal("an icon with a click listener answered no activation")
	}
	select {
	case <-clicked:
	case <-time.After(5 * time.Second):
		t.Fatal("OnClick did not run when the icon was activated")
	}

	off()
	mygo.RunOnMain(func() { activated = linux.TestActivateTray() })
	if activated {
		t.Fatal("an icon whose last click listener left answered activation")
	}
}

// TestTrayIconPath checks that the tray gives the panel its icon by the
// PNG's absolute path: a name, looked up in the panel's icon theme first,
// showed a theme's icon on KDE (#1207).
func TestTrayIconPath(t *testing.T) {
	tray, err := mygo.NewTray(mygo.TrayOptions{Icon: squarePNG(), ToolTip: "icon path"})
	if err != nil {
		t.Skip("no tray: ", err)
	}
	defer tray.Destroy()
	var icon string
	mygo.RunOnMain(func() { icon = linux.TestTrayIcon() })
	if !filepath.IsAbs(icon) {
		t.Fatalf("the icon is %q, want an absolute path", icon)
	}
	if b, err := os.ReadFile(icon); err != nil || len(b) == 0 {
		t.Fatalf("reading the icon %q: %v", icon, err)
	}
}

// TestTrayID checks that an icon of an ID is given the panel under it,
// after the program's name, so that it is the same icon across launches
// (#1835), and that an icon of no ID is named after the program and its
// number, not after the process.
func TestTrayID(t *testing.T) {
	tray, err := mygo.NewTray(mygo.TrayOptions{ID: "e2e.icon", Icon: squarePNG(), ToolTip: "id"})
	if err != nil {
		t.Skip("no tray: ", err)
	}
	var id string
	mygo.RunOnMain(func() { id = linux.TestTrayID() })
	tray.Destroy()
	if want := linux.TestProgramName() + "-e2e.icon"; id != want {
		t.Errorf("the icon of ID e2e.icon is %q, want %q", id, want)
	}

	tray, err = mygo.NewTray(mygo.TrayOptions{Icon: squarePNG(), ToolTip: "no id"})
	if err != nil {
		t.Fatal(err)
	}
	defer tray.Destroy()
	mygo.RunOnMain(func() { id = linux.TestTrayID() })
	prefix := linux.TestProgramName() + "-"
	if n, ok := strings.CutPrefix(id, prefix); !ok || !isNumber(n) {
		t.Errorf("the icon of no ID is %q, want %s<number>", id, prefix)
	}
}

// isNumber tells whether s is a decimal number as strconv writes one.
func isNumber(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && strconv.Itoa(n) == s
}
