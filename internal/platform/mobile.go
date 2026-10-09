package platform

import (
	"errors"
	"os"
	"runtime"
	"time"
)

// Mobile translates device services. Methods and callbacks run on the UI thread.
type Mobile interface {
	Haptic(kind string) error
	SetBadge(count int, done func(error))
	SetStatusBar(style string, hidden bool) error
	RegisterPush() error
	Device() DeviceInfo
	// DismissKeyboard ends text editing in the application's windows.
	DismissKeyboard()
	// ScanCode presents a camera scanner for a QR code. done runs exactly once;
	// cancel dismisses the scanner, completing with ErrScanCanceled.
	ScanCode(o ScanOptions, done func(string, error)) (cancel func())
	// PrepareNetwork makes one request to url, letting a system that gates
	// network access ask for it before raw sockets are opened. done runs
	// exactly once; cancel stops the request.
	PrepareNetwork(url string, timeout time.Duration, done func(error)) (cancel func())
}

// MobileHandler receives system events, including events from an earlier run.
type MobileHandler interface {
	// NotificationReceived returns the foreground presentation bitmask.
	NotificationReceived(NotificationEvent) uint32
	PushRegistered(token string, err error)
}

type NotificationEvent struct {
	ID, Source, Action string
	Data               map[string]string
	Clicked            bool
}

type UnsupportedMobile struct{}

func (UnsupportedMobile) Haptic(string) error              { return ErrUnsupported }
func (UnsupportedMobile) SetBadge(_ int, done func(error)) { done(ErrUnsupported) }
func (UnsupportedMobile) SetStatusBar(string, bool) error  { return ErrUnsupported }
func (UnsupportedMobile) RegisterPush() error              { return ErrUnsupported }
func (UnsupportedMobile) Device() DeviceInfo               { return HostDevice() }
func (UnsupportedMobile) DismissKeyboard()                 {}
func (UnsupportedMobile) ScanCode(_ ScanOptions, done func(string, error)) func() {
	done("", ErrUnsupported)
	return func() {}
}
func (UnsupportedMobile) PrepareNetwork(_ string, _ time.Duration, done func(error)) func() {
	done(nil)
	return func() {}
}

// DeviceInfo identifies the device to people, e.g. to name a paired phone.
type DeviceInfo struct{ Name, System, Version, Model string }

// HostDevice describes a desktop from its host name and operating system.
func HostDevice() DeviceInfo {
	name, _ := os.Hostname()
	system := map[string]string{"darwin": "macOS", "linux": "Linux", "windows": "Windows"}[runtime.GOOS]
	if system == "" {
		system = runtime.GOOS
	}
	return DeviceInfo{Name: name, System: system}
}

// ScanOptions labels the camera scanner. Empty strings use system wording.
type ScanOptions struct{ Prompt, CancelLabel string }

var (
	ErrScanCanceled      = errors.New("mygo: scanning was canceled")
	ErrCameraDenied      = errors.New("mygo: the user does not allow camera access")
	ErrCameraUnavailable = errors.New("mygo: no camera is available")
)

// NetworkError is a failed network preparation. Kind is "offline" without a
// usable connection (or with cellular data disabled for the app), "timeout"
// when the request did not finish in time, and "failed" otherwise.
type NetworkError struct{ Kind string }

func (e *NetworkError) Error() string { return "mygo: network preparation " + e.Kind }
