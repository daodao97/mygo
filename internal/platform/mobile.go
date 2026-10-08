package platform

// Mobile translates device services. Methods and callbacks run on the UI thread.
type Mobile interface {
	Haptic(kind string) error
	SetBadge(count int, done func(error))
	SetStatusBar(style string, hidden bool) error
	RegisterPush() error
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
