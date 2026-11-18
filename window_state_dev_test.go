//go:build !js

package mygo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/fake"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/wait"
)

func TestDevWindows(t *testing.T) {
	file := filepath.Join(t.TempDir(), "windows.json")
	delay := windowStateDelay
	windowStateDelay = 20 * time.Millisecond
	// A new build: the windows close, as the app quits, and what the file
	// keeps is read again.
	var build []*Window
	open := func(o WindowOptions) (*Window, *fake.Window) {
		w, fw := testWindow(t, o)
		build = append(build, w)
		return w, fw
	}
	relaunch := func() {
		for _, w := range build {
			w.Destroy()
		}
		build = nil
		onMain(func() { devWindows.loaded, devWindows.list, devWindows.created = false, nil, 0 })
	}
	onMain(func() { devLaunched, devWindowsFile = true, file })
	relaunch()
	t.Cleanup(func() {
		onMain(func() { devLaunched, devWindowsFile = false, "" })
		relaunch()
		windowStateDelay = delay
	})
	saved := func(want []WindowState) {
		t.Helper()
		var got []devWindow
		if !wait.Poll(func() bool {
			data, err := os.ReadFile(file)
			if err != nil || json.Unmarshal(data, &got) != nil || len(got) != len(want) {
				return false
			}
			for i := range want {
				if got[i].State != want[i] {
					return false
				}
			}
			return true
		}) {
			t.Fatalf("kept %+v, want the states %+v", got, want)
		}
	}
	opts := WindowOptions{X: 100, Y: 120, Width: 700, Height: 500}

	// The first build's windows get their options, and their frames are
	// kept as the user moves them. A window with a StateKey, a child
	// window and a panel take no part.
	w1, fw1 := open(opts)
	_, fw2 := open(WindowOptions{Width: 400, Height: 300}) // centered
	keyed, _ := open(WindowOptions{StateKey: "dev-test", Width: 300, Height: 200})
	child, _ := open(WindowOptions{Parent: w1, Width: 300, Height: 200})
	panel, _ := open(WindowOptions{Panel: true, Width: 300, Height: 200})
	for _, w := range []*Window{keyed, child, panel} {
		if slot := onMainValue(func() int { return w.devSlot }); slot != 0 {
			t.Errorf("a window that takes no part has the place %d", slot)
		}
	}
	if o := fw1.Opts; o.X != 100 || o.Y != 120 || o.Width != 700 || o.Height != 500 {
		t.Errorf("first build's window at %d,%d %dx%d", o.X, o.Y, o.Width, o.Height)
	}
	onMain(func() {
		fw1.SetBounds(platform.Rect{X: 600, Y: 80, Width: 900, Height: 640})
		fw2.SetBounds(platform.Rect{X: 300, Y: 200, Width: 400, Height: 300})
	})
	second := WindowState{X: 300, Y: 200, Width: 400, Height: 300}
	saved([]WindowState{{X: 600, Y: 80, Width: 900, Height: 640}, second})

	// The next build's come back there, by the order they are created in;
	// the second window, which this one does not create, stays kept.
	relaunch()
	_, fw1 = open(opts)
	if o := fw1.Opts; o.X != 600 || o.Y != 80 || o.Width != 900 || o.Height != 640 || o.Center {
		t.Errorf("relaunched window at %d,%d %dx%d (centered: %v), want the previous frame", o.X, o.Y, o.Width, o.Height, o.Center)
	}
	onMain(func() { fw1.Maximize() })
	saved([]WindowState{{X: 600, Y: 80, Width: 900, Height: 640, Maximized: true}, second})

	relaunch()
	_, fw1 = open(opts)
	if o := fw1.Opts; !o.Maximized || o.X != 600 || o.Width != 900 {
		t.Errorf("relaunched window at %d,%d %dx%d (maximized: %v), want the previous frame, maximized", o.X, o.Y, o.Width, o.Height, o.Maximized)
	}

	// A window whose code asks for another frame gets what it asks for.
	relaunch()
	changed := opts
	changed.Width = 720
	_, fw1 = open(changed)
	if o := fw1.Opts; o.X != 100 || o.Y != 120 || o.Width != 720 || o.Height != 500 || o.Maximized {
		t.Errorf("window whose options changed at %d,%d %dx%d (maximized: %v), want its options", o.X, o.Y, o.Width, o.Height, o.Maximized)
	}

	// Without mygo dev, nothing is kept.
	relaunch()
	onMain(func() { devLaunched = false })
	plain, _ := open(opts)
	if slot := onMainValue(func() int { return plain.devSlot }); slot != 0 {
		t.Errorf("a window of an app mygo dev did not launch has the place %d", slot)
	}
}
