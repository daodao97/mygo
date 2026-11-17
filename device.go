package mygo

import (
	"errors"

	"github.com/egoist/mygo/internal/platform"
)

// What a device held in the hand has that a computer does not. On desktops
// and in browsers these do nothing, or report errors.ErrUnsupported.

func device() (platform.DeviceBackend, bool) {
	d, ok := backend().(platform.DeviceBackend)
	return d, ok
}

// HapticsModule plays the feel of what happens on a phone's screen. Use the
// Haptics singleton.
type HapticsModule struct{}

// Haptics plays the feel of what happens on a phone's screen: UIKit's
// feedback generators on iOS, and on Android the system's haptic constants,
// as the user's settings allow. Desktops have none.
var Haptics = &HapticsModule{}

func (*HapticsModule) play(kind string) {
	App.WhenReady(func() {
		if d, ok := device(); ok {
			d.Haptic(kind)
		}
	})
}

// Selection plays the tick of a choice changing, as a picker's wheel.
func (h *HapticsModule) Selection() { h.play("selection") }

// Light, Medium and Heavy play the impact of a thing of that weight, as a
// button or something that lands.
func (h *HapticsModule) Light()  { h.play("light") }
func (h *HapticsModule) Medium() { h.play("medium") }
func (h *HapticsModule) Heavy()  { h.play("heavy") }

// Success, Warning and Error play how a task ended.
func (h *HapticsModule) Success() { h.play("success") }
func (h *HapticsModule) Warning() { h.play("warning") }
func (h *HapticsModule) Error()   { h.play("error") }

// BarStyle is the color of the icons of the system's bars.
type BarStyle string

const (
	// BarStyleAuto follows the appearance: dark icons in light mode.
	BarStyleAuto BarStyle = "auto"
	// BarStyleLight draws light icons, for dark content under them.
	BarStyleLight BarStyle = "light"
	// BarStyleDark draws dark icons, for light content under them.
	BarStyleDark BarStyle = "dark"
)

// SystemBarsModule styles and hides the bars the system draws over a phone's
// screen. Use the SystemBars singleton.
type SystemBarsModule struct {
	style  BarStyle
	hidden bool
}

// SystemBars styles and hides the bars the system draws over a phone's
// screen: the status bar, and Android's navigation bar. Desktops have none.
var SystemBars = &SystemBarsModule{style: BarStyleAuto}

func (s *SystemBarsModule) apply() {
	if d, ok := device(); ok {
		d.SetSystemBars(string(s.style), s.hidden)
	}
}

// SetStyle sets the color of the bars' icons, for what the app draws under
// them.
func (s *SystemBarsModule) SetStyle(style BarStyle) {
	App.WhenReady(func() {
		s.style = style
		s.apply()
	})
}

// SetHidden hides the bars, as a video or a game fills the screen: a swipe
// from the edge brings them back for a moment.
func (s *SystemBarsModule) SetHidden(hidden bool) {
	App.WhenReady(func() {
		s.hidden = hidden
		s.apply()
	})
}

// Orientation is a way the screen may be held, or several combined.
type Orientation uint32

// The orientations of a screen.
const (
	OrientationPortrait           = Orientation(platform.OrientationPortrait)
	OrientationPortraitUpsideDown = Orientation(platform.OrientationPortraitUpsideDown)
	OrientationLandscapeLeft      = Orientation(platform.OrientationLandscapeLeft)
	OrientationLandscapeRight     = Orientation(platform.OrientationLandscapeRight)
	// OrientationLandscape is either way on its side.
	OrientationLandscape = OrientationLandscapeLeft | OrientationLandscapeRight
	// OrientationAny lets the system turn the screen as it likes.
	OrientationAny Orientation = 0
)

// SetOrientations limits how a phone's screen may turn, as a game holds it on
// its side: from now on, whatever the app's configuration says. Desktops
// ignore it.
func SetOrientations(o Orientation) {
	App.WhenReady(func() {
		if d, ok := device(); ok {
			d.SetOrientations(uint32(o))
		}
	})
}

// ShareOptions is what Share shares: text, a link and files, any of them.
type ShareOptions struct {
	Text string
	URL  string
	// Files are paths the app can read.
	Files []string
	// X and Y are where in the window in front the sheet comes from, as what
	// was tapped: an iPad's sheet points at it.
	X, Y float64
}

// Share shows the system's share sheet: UIActivityViewController on iOS, and
// the chooser of ACTION_SEND on Android. It returns once the sheet shows;
// desktops report errors.ErrUnsupported.
func Share(o ShareOptions) error {
	needsApp("Share")
	ch := make(chan error, 1)
	if !postMain(func() {
		d, ok := device()
		if !ok {
			deliver(ch, error(errors.ErrUnsupported))
			return
		}
		d.Share(platform.ShareRequest{Text: o.Text, URL: o.URL, Paths: o.Files, X: o.X, Y: o.Y}, func(err error) { deliver(ch, err) })
	}) {
		return errLoopStopped
	}
	return await(ch)
}

// BatteryStatus is the state of a device's battery.
type BatteryStatus struct {
	// Level is from 0 to 1, or negative where it is not known, as on
	// computers.
	Level float64
	// Charging is true while it is plugged in.
	Charging bool
	// LowPower is true in the system's low power mode: do less in the
	// background, and animate less.
	LowPower bool
}

// Battery tells the state of the battery, on phones and tablets.
func (p *PowerModule) Battery() BatteryStatus {
	needsApp("Power.Battery")
	return onMainValue(func() BatteryStatus {
		if d, ok := device(); ok {
			b := d.Battery()
			return BatteryStatus{Level: b.Level, Charging: b.Charging, LowPower: b.LowPower}
		}
		return BatteryStatus{Level: -1}
	})
}

// BarItem is a button at the end of a window's navigation bar (Window.Push),
// the system's own.
type BarItem struct {
	// Title names it, which shows where the system has no icon, and to
	// assistive technology.
	Title string
	// Symbol names an SF Symbol, which iOS draws.
	Symbol string
	// Icon is a PNG drawn as a template: Android's icon, and iOS's without a
	// Symbol.
	Icon []byte
	// OnClick runs on the main thread when it is tapped.
	OnClick func()
}

// SetBarItems sets the buttons at the end of w's navigation bar, on phones.
// Desktops have no such bar and ignore them.
func SetBarItems(w *Window, items ...BarItem) {
	onMain(func() {
		d, ok := device()
		if !ok || w.native == nil {
			return
		}
		out := make([]platform.BarItem, len(items))
		for i, it := range items {
			out[i] = platform.BarItem{Title: it.Title, Symbol: it.Symbol, Icon: it.Icon, OnClick: it.OnClick}
		}
		d.SetBarItems(w.native, out)
	})
}
