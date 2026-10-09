package fake

import (
	"time"

	"github.com/egoist/mygo/internal/platform"
)

type mobile struct{ b *Backend }

func (b *Backend) Mobile() platform.Mobile { return mobile{b} }
func (m mobile) Haptic(kind string) error {
	m.b.HapticCalls = append(m.b.HapticCalls, kind)
	return m.b.MobileError
}
func (m mobile) SetBadge(n int, done func(error)) { m.b.BadgeCount = n; done(m.b.MobileError) }
func (m mobile) SetStatusBar(s string, h bool) error {
	m.b.StatusBarStyle, m.b.StatusBarHidden = s, h
	return m.b.MobileError
}
func (m mobile) RegisterPush() error { m.b.PushRegistrations++; return m.b.MobileError }
func (m mobile) Device() platform.DeviceInfo {
	if m.b.DeviceInfo != (platform.DeviceInfo{}) {
		return m.b.DeviceInfo
	}
	return platform.HostDevice()
}
func (m mobile) DismissKeyboard() { m.b.KeyboardDismissals++ }
func (m mobile) ScanCode(o platform.ScanOptions, done func(string, error)) func() {
	m.b.LastScan = o
	if !m.b.ScanPending {
		done(m.b.ScanResult, m.b.ScanError)
		return func() {}
	}
	m.b.scanDone = done
	return func() { m.b.FinishScan("", platform.ErrScanCanceled) }
}

// FinishScan completes a pending ScanCode, as a recognized code or the
// scanner's cancel button would.
func (b *Backend) FinishScan(value string, err error) {
	if done := b.scanDone; done != nil {
		b.scanDone = nil
		done(value, err)
	}
}
func (m mobile) PrepareNetwork(url string, _ time.Duration, done func(error)) func() {
	m.b.NetworkURLs = append(m.b.NetworkURLs, url)
	if !m.b.NetworkPending {
		done(m.b.NetworkError)
		return func() {}
	}
	m.b.networkDone = done
	return func() { m.b.FinishNetwork(errNetworkCanceled) }
}

var errNetworkCanceled = &platform.NetworkError{Kind: "canceled"}

// FinishNetwork completes a pending PrepareNetwork request.
func (b *Backend) FinishNetwork(err error) {
	if done := b.networkDone; done != nil {
		b.networkDone = nil
		done(err)
	}
}
func (b *Backend) ReceiveNotification(id string, data map[string]string, clicked bool) {
	b.DeliverNotification(platform.NotificationEvent{ID: id, Data: data, Clicked: clicked, Source: "local"})
}

// DeliverNotification also models APNs delivery and custom action responses.
func (b *Backend) DeliverNotification(event platform.NotificationEvent) uint32 {
	if h, ok := b.h.(platform.MobileHandler); ok {
		return h.NotificationReceived(event)
	}
	return 0
}
func (b *Backend) RegisterPushResult(token string, err error) {
	if h, ok := b.h.(platform.MobileHandler); ok {
		h.PushRegistered(token, err)
	}
}

// ScanOpen and NetworkOpen report pending requests (main thread).
func (b *Backend) ScanOpen() bool    { return b.scanDone != nil }
func (b *Backend) NetworkOpen() bool { return b.networkDone != nil }
