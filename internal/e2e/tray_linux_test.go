//go:build linux

package e2e

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/linux"
	"github.com/egoist/mygo/internal/wait"
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
	wait.For(t, "OnClick after the icon was activated", clicked)

	off()
	mygo.RunOnMain(func() { activated = linux.TestActivateTray() })
	if activated {
		t.Fatal("an icon whose last click listener left answered activation")
	}
}
