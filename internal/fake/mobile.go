package fake

import "github.com/egoist/mygo/internal/platform"

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
