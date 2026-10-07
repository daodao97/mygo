package mygo

import (
	"errors"
	"github.com/egoist/mygo/internal/platform"
	"testing"
	"time"
)

func TestMobileNotificationScheduleAndLaunch(t *testing.T) {
	data := map[string]string{"route": "/features/notifications"}
	badge := 2
	n := NewNotification(NotificationOptions{ID: "mobile-schedule", Title: "Test", Data: data, Badge: &badge})
	data["route"] = "changed"
	badge = 9
	if err := n.Schedule(0); err == nil {
		t.Fatal("accepted zero delay")
	}
	if err := n.Schedule(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	onMain(func() {
		r := fb.Notification(n.ID())
		if r.DelaySeconds != 5 || r.Data["route"] != "/features/notifications" || *r.Badge != 2 {
			t.Errorf("request: %+v", r)
		}
	})
	clicked := 0
	off := n.OnClick(func() { clicked++ })
	defer off()
	var events []NotificationEvent
	offEvent := App.OnNotification(func(e NotificationEvent) { events = append(events, e) })
	defer offEvent()
	offClick := App.OnNotificationClick(func(id string) {
		if id == "previous-run" {
			clicked++
		}
	})
	defer offClick()
	onMain(func() {
		fb.ReceiveNotification(n.ID(), map[string]string{"route": "/features/notifications"}, false)
		fb.ReceiveNotification(n.ID(), map[string]string{"route": "/features/notifications"}, true)
		fb.ReceiveNotification("previous-run", map[string]string{"route": "cold"}, true)
	})
	if clicked != 2 || len(events) != 3 || events[0].Clicked || !events[2].Clicked || events[2].Data["route"] != "cold" {
		t.Fatalf("clicks %d events %+v", clicked, events)
	}
	n.Close()
	onMain(func() {
		if fb.Notification(n.ID()) != nil {
			t.Fatal("scheduled request remained after Close")
		}
	})
	onMain(func() { fb.NotificationError = platform.ErrNotificationsDenied })
	defer onMain(func() { fb.NotificationError = nil })
	if err := n.Schedule(time.Second); !errors.Is(err, ErrNotificationsDenied) {
		t.Fatalf("denied: %v", err)
	}
}
func TestMobileFeedbackStatusAndPush(t *testing.T) {
	onMain(func() { fb.HapticCalls = nil; fb.MobileError = nil; fb.PushRegistrations = 0 })
	if err := Haptics.Play("invalid"); err == nil {
		t.Fatal("invalid feedback")
	}
	for _, kind := range []HapticFeedback{HapticSelection, HapticLight, HapticMedium, HapticHeavy, HapticSoft, HapticRigid, HapticSuccess, HapticWarning, HapticError} {
		if err := Haptics.Play(kind); err != nil {
			t.Fatal(err)
		}
	}
	if err := App.SetNotificationBadge(-1); err == nil {
		t.Fatal("negative badge")
	}
	onMain(func() {
		if err := App.SetNotificationBadge(3); err != nil {
			t.Error(err)
		}
		if fb.BadgeCount != 3 || len(fb.HapticCalls) != 9 {
			t.Error("device services not forwarded")
		}
	})
	if err := App.SetStatusBar(StatusBarLight, true); err != nil {
		t.Fatal(err)
	}
	onMain(func() {
		if fb.StatusBarStyle != "light" || !fb.StatusBarHidden {
			t.Error("status bar not forwarded")
		}
	})
	if err := App.SetStatusBar("invalid", false); err == nil {
		t.Fatal("invalid style")
	}
	token := ""
	var registrationError error
	off := App.OnPushToken(func(value string) { token = value })
	defer off()
	offError := App.OnPushRegistrationError(func(err error) { registrationError = err })
	defer offError()
	if err := App.RegisterPushNotifications(); err != nil {
		t.Fatal(err)
	}
	onMain(func() { fb.RegisterPushResult("abcdef", nil); fb.RegisterPushResult("", platform.ErrUnsupported) })
	if token != "abcdef" || !errors.Is(registrationError, ErrUnsupported) {
		t.Fatal("push results lost")
	}
	onMain(func() { fb.MobileError = platform.ErrUnsupported })
	defer onMain(func() { fb.MobileError = nil; fb.HapticCalls = nil })
	if !errors.Is(Haptics.Play(HapticSuccess), ErrUnsupported) || !errors.Is(App.SetNotificationBadge(0), ErrUnsupported) || !errors.Is(App.SetStatusBar(StatusBarAuto, false), ErrUnsupported) {
		t.Fatal("device errors not propagated")
	}
}

func TestFailedNotificationReplacementRetainsClickListener(t *testing.T) {
	old := NewNotification(NotificationOptions{ID: "replacement"})
	called := false
	off := old.OnClick(func() { called = true })
	defer off()
	defer old.Close()
	if err := old.Show(); err != nil {
		t.Fatal(err)
	}
	onMain(func() { fb.NotificationError = platform.ErrNotificationsDenied })
	replacement := NewNotification(NotificationOptions{ID: old.ID()})
	err := replacement.Schedule(time.Second)
	onMain(func() { fb.NotificationError = nil; fb.ReceiveNotification(old.ID(), nil, true) })
	if !errors.Is(err, ErrNotificationsDenied) || !called {
		t.Fatal("failed replacement discarded delivered notification listener")
	}
}
