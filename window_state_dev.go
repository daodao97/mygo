//go:build !js

package mygo

import (
	"encoding/json"
	"os"

	"github.com/egoist/mygo/internal/platform"
)

// Under mygo dev, which ends the app and starts the new build as the code
// changes, a window without a StateKey comes back where the previous
// build's window was, as the user put it, maximized or full screen as it
// was (#2000): mygo dev names a file in MYGO_DEV_WINDOWS, in which the app
// keeps the state of each such window it creates, by the order it creates
// them in, with the frame its code asked for. A window whose code asks for
// another frame than the previous build's did gets what it asks for, so
// that changing the size in the code shows. mygo dev starts every session
// without the file. Production builds ignore it, as they do MYGO_DEV.

// devWindowsFile is the file MYGO_DEV_WINDOWS names, which the core takes
// out of its environment as MYGO_DEV.
var devWindowsFile = func() string {
	f := os.Getenv("MYGO_DEV_WINDOWS")
	os.Unsetenv("MYGO_DEV_WINDOWS")
	return f
}()

// devWindow is what the file keeps of a window.
type devWindow struct {
	Asked devWindowAsk `json:"asked"`
	State WindowState  `json:"state"`
}

// devWindowAsk is the frame the code asked a window for.
type devWindowAsk struct {
	X           int  `json:"x"`
	Y           int  `json:"y"`
	Width       int  `json:"width"`
	Height      int  `json:"height"`
	Center      bool `json:"center,omitempty"`
	ContentSize bool `json:"contentSize,omitempty"`
	Maximized   bool `json:"maximized,omitempty"`
	FullScreen  bool `json:"fullScreen,omitempty"`
}

// devWindows are the windows of the file, loaded with the first one the
// app creates, and how many the app created. Main thread only.
var devWindows struct {
	loaded  bool
	list    []devWindow
	created int
}

// restoreDevWindow gives a window that mygo dev's file keeps a state of
// the frame the previous build's had, and records what the code asked
// for. Only top-level windows without a StateKey take part: not child
// windows, panels, flyouts or windows the host made. Main thread only.
func (w *Window) restoreDevWindow(p *platform.WindowOptions, native uintptr) {
	if devWindowsFile == "" || w.stateKey != "" || w.parent != nil || w.panel || p.Flyout != nil || native != 0 || !launchedByDev() {
		return
	}
	if !devWindows.loaded {
		devWindows.loaded = true
		if data, err := os.ReadFile(devWindowsFile); err == nil {
			json.Unmarshal(data, &devWindows.list) // a damaged file is replaced
		}
	}
	i := devWindows.created
	devWindows.created++
	w.devSlot = i + 1
	asked := devWindowAsk{p.X, p.Y, p.Width, p.Height, p.Center, p.UseContentSize, p.Maximized, p.FullScreen}
	if i < len(devWindows.list) {
		if prev := devWindows.list[i]; prev.Asked == asked && prev.State.Width > 0 && prev.State.Height > 0 {
			restoreWindowStateFrom(prev.State, p, backend())
			return
		}
	}
	for len(devWindows.list) <= i {
		devWindows.list = append(devWindows.list, devWindow{})
	}
	devWindows.list[i] = devWindow{Asked: asked}
	if p.Maximized || p.FullScreen {
		// The window has not shown its normal bounds yet.
		devWindows.list[i].State = requestedState(p)
	}
}

// captureDevWindow records the window's state in mygo dev's file, which it
// writes when the state changed. Main thread only.
func (w *Window) captureDevWindow() {
	n, i := w.native, w.devSlot-1
	if n == nil || i < 0 || i >= len(devWindows.list) || devWindowsFile == "" {
		return
	}
	st := captureWindowState(devWindows.list[i].State, n)
	if st.Width <= 0 || st.Height <= 0 || st == devWindows.list[i].State {
		return
	}
	devWindows.list[i].State = st
	data, err := json.Marshal(devWindows.list)
	if err != nil {
		return
	}
	tmp := devWindowsFile + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		replaceAppFile(tmp, devWindowsFile)
	}
}
