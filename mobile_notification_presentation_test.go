package mygo

import (
	"github.com/egoist/mygo/internal/platform"
	"testing"
)

func TestNotificationPresentationAndRemoteColdLaunchData(t *testing.T) {
	defer App.SetNotificationPresentationHandler(nil)
	var received []NotificationEvent
	off := App.OnNotification(func(e NotificationEvent) { received = append(received, e) })
	defer off()
	calls := 0
	App.SetNotificationPresentationHandler(func(e NotificationEvent) NotificationPresentation {
		calls++
		if e.Source == NotificationRemote && e.Data["session"] == "active" {
			e.Data["session"] = "mutated"
			return 0
		}
		return PresentNotificationBanner | PresentNotificationList
	})
	onMain(func() {
		mask := fb.DeliverNotification(platform.NotificationEvent{ID: "push", Source: "remote", Data: map[string]string{"session": "active"}})
		if mask != 0 || received[0].Source != NotificationRemote || received[0].Data["session"] != "active" {
			t.Error("presentation mutated payload or wasn't suppressed")
		}
		mask = fb.DeliverNotification(platform.NotificationEvent{ID: "other", Source: "local"})
		if mask != uint32(PresentNotificationBanner|PresentNotificationList) {
			t.Error("presentation options lost")
		}
		fb.DeliverNotification(platform.NotificationEvent{ID: "cold-launch", Source: "remote", Clicked: true, Action: "open", Data: map[string]string{"session": "cold"}})
	})
	if calls != 2 || len(received) != 3 || !received[2].Clicked || received[2].Action != "open" || received[2].Data["session"] != "cold" {
		t.Fatal("action/cold-launch event lost")
	}
	App.SetNotificationPresentationHandler(nil)
	onMain(func() {
		if fb.DeliverNotification(platform.NotificationEvent{}) != uint32(PresentNotificationDefault) {
			t.Error("default presentation not restored")
		}
	})
}
