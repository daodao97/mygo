package mygo

import (
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/pushhub"
	"maps"
)

// Push notifications reach apps through plugins/push, in upstream #940's
// API (FORK.md, "向 #940 收敛"), and taps of the app's own notifications
// through App.OnNotificationClick; the backends tell them here.

// RegisterPushNotifications starts the system's registration for pushes,
// whose token or error plugins/push tells.
//
// Deprecated: apps call push.Register (plugins/push), in #940's API, which
// calls this; it stays here, linked only into apps that use the plugin,
// until the fork takes #940's backends.
func (a *Application) RegisterPushNotifications() error {
	needsApp("push.Register")
	return onMainValue(func() error { return mobileBackend().RegisterPush() })
}

// presentAll is iOS's banner, list, sound and badge of a notification that
// arrives in front of the app.
const presentAll = 1 | 2 | 4 | 8

func (appHandler) NotificationReceived(input platform.NotificationEvent) uint32 {
	if input.Clicked {
		notificationClicked(input.ID)
	}
	if pushhub.Received == nil {
		return presentAll
	}
	ev := pushhub.Event{ID: input.ID, Data: maps.Clone(input.Data), Clicked: input.Clicked, Remote: input.Source == "remote", Action: input.Action}
	if !pushhub.Received(ev) && !ev.Clicked {
		return 0
	}
	return presentAll
}
func (appHandler) PushRegistered(token string, err error) {
	if pushhub.Registered != nil {
		pushhub.Registered(token, err)
	}
	if err == nil {
		fire1(&pushSubscriptions, PushSubscription{Service: PushAPNs, Token: token})
	}
}
