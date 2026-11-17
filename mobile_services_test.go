//go:build !js

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
	if clicked != 2 {
		t.Fatalf("clicks %d", clicked)
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
