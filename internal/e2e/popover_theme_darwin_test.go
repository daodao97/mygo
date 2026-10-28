//go:build darwin

package e2e

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/darwin"
	"github.com/egoist/mygo/ui"
)

func TestPopoverAppearanceFollowsAppInsteadOfAnchor(t *testing.T) {
	defer mygo.Theme.SetSource(mygo.Theme.Source())
	mygo.Theme.SetSource(mygo.ThemeLight)
	parent := newWindow(t, mygo.WindowOptions{Width: 300, Height: 200})
	eventually(t, "anchor window visible", parent.IsVisible)
	fl := mygo.NewFlyout(mygo.FlyoutOptions{Parent: parent, Anchor: mygo.Rectangle{X: 100, Y: 20, Width: 80, Height: 24}, Popover: true, Width: 220, Height: 120, Content: ui.View(func(c *ui.Context) { ui.Text(c, "Popover theme test") })})
	defer fl.Destroy()
	eventually(t, "popover window adopted", func() bool { return fl.NativeHandle() != 0 })
	appearance := func() (chrome, window, content string) {
		mygo.RunOnMain(func() { chrome, window, content = darwin.TestPopoverAppearance(fl.NativeHandle()) })
		return
	}
	if chrome, window, content := appearance(); chrome == "" {
		t.Fatalf("native popover inherits the status-item appearance instead of following the app (handle=%x visible=%v window=%s content=%s)", fl.NativeHandle(), fl.IsVisible(), window, content)
	}
	mygo.RunOnMain(func() { darwin.TestSetPopoverAnchorAppearance(fl.NativeHandle(), false) })
	check := func() {
		want := "NSAppearanceNameAqua"
		if mygo.Theme.IsDark() {
			want = "NSAppearanceNameDarkAqua"
		}
		eventually(t, "popover chrome, window and content match "+want, func() bool {
			chrome, window, content := appearance()
			return chrome == want && window == want && content == want
		})
	}
	for _, source := range []mygo.ThemeSource{mygo.ThemeDark, mygo.ThemeLight, mygo.ThemeDark, mygo.ThemeSystem} {
		mygo.Theme.SetSource(source)
		check()
	}
	fl.Hide()
	mygo.Theme.SetSource(mygo.ThemeDark)
	fl.Show()
	check()
}
