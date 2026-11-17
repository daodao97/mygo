// The fork's device services (FORK.md, "第二阶段"): Device and DismissKeyboard.
// #940's phone backend has none: Device reports the host, and DismissKeyboard
// does nothing there. Scanner and Network.Prepare are plugins/scanner and
// plugins/network now.

package mygo

import (
	"github.com/egoist/mygo/internal/platform"
)

// DeviceInfo describes the device for people, e.g. to name a paired phone.
// On iOS, Name is the user-assigned device name only with the
// com.apple.developer.device-information.user-assigned-device-name
// entitlement; otherwise iOS reports a generic model name. Desktops report
// the host name and their operating system.
type DeviceInfo struct {
	Name    string
	System  string // "iOS", "iPadOS", "macOS", "Linux" or "Windows"
	Version string // the system version where known, e.g. "18.2"
	Model   string // e.g. "iPhone"; empty on desktops
}

// Device returns the current device. It is safe from any goroutine and
// does not need a running application.
func (a *Application) Device() DeviceInfo {
	if !a.isReady() {
		return DeviceInfo(platform.HostDevice())
	}
	return DeviceInfo(onMainValue(func() platform.DeviceInfo { return mobileBackend().Device() }))
}

// isReady reports, from any goroutine, whether the application finished
// launching. Device services work without an application before that.
func (a *Application) isReady() bool {
	select {
	case <-a.readyCh:
		return true
	default:
		return false
	}
}

// DismissKeyboard ends text editing in the application, hiding the iOS
// software keyboard even for native text fields MyGo does not draw. Go
// focus is a separate state: blur the focused element (ui.Context.Blur) as
// well. Desktop backends ignore it.
func (a *Application) DismissKeyboard() {
	if !a.isReady() {
		return
	}
	postMain(func() { mobileBackend().DismissKeyboard() })
}
