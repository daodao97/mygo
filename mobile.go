package mygo

import (
	"fmt"
	"github.com/egoist/mygo/internal/platform"
	"maps"
)

// HapticFeedback is a short system feedback pattern.
type HapticFeedback string

const (
	HapticSelection HapticFeedback = "selection"
	HapticLight     HapticFeedback = "light"
	HapticMedium    HapticFeedback = "medium"
	HapticHeavy     HapticFeedback = "heavy"
	HapticSoft      HapticFeedback = "soft"
	HapticRigid     HapticFeedback = "rigid"
	HapticSuccess   HapticFeedback = "success"
	HapticWarning   HapticFeedback = "warning"
	HapticError     HapticFeedback = "error"
)

// Haptics provides device feedback. A successful call does not imply the
// hardware emitted a vibration (simulators and system settings may suppress it).
var Haptics hapticsModule

type hapticsModule struct{}

func (hapticsModule) Play(kind HapticFeedback) error {
	switch kind {
	case HapticSelection, HapticLight, HapticMedium, HapticHeavy, HapticSoft, HapticRigid, HapticSuccess, HapticWarning, HapticError:
	default:
		return fmt.Errorf("mygo: unknown haptic %q", kind)
	}
	needsApp("Haptics.Play")
	return onMainValue(func() error { return backend().Mobile().Haptic(string(kind)) })
}

// StatusBarStyle controls system text; auto follows the window background.
type StatusBarStyle string

const (
	StatusBarAuto  StatusBarStyle = ""
	StatusBarLight StatusBarStyle = "light"
	StatusBarDark  StatusBarStyle = "dark"
)

// SetStatusBar sets iOS status bar text and visibility.
func (a *Application) SetStatusBar(style StatusBarStyle, hidden bool) error {
	if style != StatusBarAuto && style != StatusBarLight && style != StatusBarDark {
		return fmt.Errorf("mygo: unknown status bar style %q", style)
	}
	needsApp("App.SetStatusBar")
	return onMainValue(func() error { return backend().Mobile().SetStatusBar(string(style), hidden) })
}

// SetNotificationBadge sets the iOS app icon badge (zero clears it). Requires badge
// authorization; failure is returned without altering the shared Dock API.
func (a *Application) SetNotificationBadge(count int) error {
	if count < 0 {
		return fmt.Errorf("mygo: badge count cannot be negative")
	}
	needsApp("App.SetNotificationBadge")
	ch := make(chan error, 1)
	onMain(func() { backend().Mobile().SetBadge(count, func(err error) { deliver(ch, err) }) })
	return await(ch)
}

// NotificationEvent contains custom string data retained by iOS. Clicked is
// false for foreground delivery and true for a system notification response.
type NotificationEvent struct {
	ID      string
	Data    map[string]string
	Clicked bool
	// Source identifies local scheduling or remote push delivery.
	Source NotificationSource
	// Action is a custom system action identifier; empty means ordinary delivery
	// or opening the notification. Clicked remains true for an ordinary open.
	Action string
}

type NotificationSource string

const (
	NotificationLocal  NotificationSource = "local"
	NotificationRemote NotificationSource = "remote"
)

// NotificationPresentation controls iOS foreground presentation. Zero suppresses
// system UI while still delivering OnNotification. Background presentation is
// controlled by the push payload and the user's system settings.
type NotificationPresentation uint32

const (
	PresentNotificationBanner NotificationPresentation = 1 << iota
	PresentNotificationList
	PresentNotificationSound
	PresentNotificationBadge
	PresentNotificationDefault = PresentNotificationBanner | PresentNotificationList | PresentNotificationSound | PresentNotificationBadge
)

var mobileEvents struct {
	notifications listeners[func(NotificationEvent)]
	tokens        listeners[func(string)]
	errors        listeners[func(error)]
	presentation  func(NotificationEvent) NotificationPresentation // main thread
}

// SetNotificationPresentationHandler chooses iOS foreground presentation per
// notification. Register before Run; nil restores the default banner, list,
// sound and badge. The callback runs synchronously on the UI thread before
// OnNotification and must return promptly. It does not request authorization.
// Other platforms retain their native presentation behavior.
func (a *Application) SetNotificationPresentationHandler(fn func(NotificationEvent) NotificationPresentation) {
	onMain(func() { mobileEvents.presentation = fn })
}

// OnNotification receives foreground delivery and notification clicks, also
// after cold launch. Register before App.Run; callbacks run on the UI thread.
func (a *Application) OnNotification(fn func(NotificationEvent)) func() {
	return mobileEvents.notifications.add(fn, false)
}

// RegisterPushNotifications starts APNs registration. Configure aps-environment
// entitlements and a matching provisioning profile; it does not request alert
// permission. Tokens and errors arrive through the listeners below.
func (a *Application) RegisterPushNotifications() error {
	needsApp("App.RegisterPushNotifications")
	return onMainValue(func() error { return backend().Mobile().RegisterPush() })
}
func (a *Application) OnPushToken(fn func(string)) func() { return mobileEvents.tokens.add(fn, false) }
func (a *Application) OnPushRegistrationError(fn func(error)) func() {
	return mobileEvents.errors.add(fn, false)
}
func (appHandler) NotificationReceived(input platform.NotificationEvent) uint32 {
	event := NotificationEvent{ID: input.ID, Data: maps.Clone(input.Data), Clicked: input.Clicked, Source: NotificationSource(input.Source), Action: input.Action}
	presentation := PresentNotificationDefault
	if !event.Clicked && mobileEvents.presentation != nil {
		presentation = mobileEvents.presentation(event) & PresentNotificationDefault
	}
	if event.Clicked {
		notificationClicked(event.ID)
	}
	event.Data = maps.Clone(input.Data)
	fire1(&mobileEvents.notifications, event)
	return uint32(presentation)
}
func (appHandler) PushRegistered(token string, err error) {
	if err != nil {
		fire1(&mobileEvents.errors, err)
	} else {
		fire1(&mobileEvents.tokens, token)
	}
}
