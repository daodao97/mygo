package mygo

import (
	"errors"
	"github.com/egoist/mygo/internal/platform"
	"testing"
)

func TestNativePermissionDecisions(t *testing.T) {
	onMain(func() {
		fb.PermissionStatuses = nil
		fb.PermissionError = nil
		fb.PermissionDecision = ""
		fb.SettingsOpened = 0
	})
	t.Cleanup(func() {
		onMain(func() {
			fb.PermissionStatuses = nil
			fb.PermissionError = nil
			fb.PermissionDecision = ""
			fb.SettingsOpened = 0
		})
	})
	if _, err := Permissions.Query("unknown"); err == nil {
		t.Fatal("accepted unknown permission")
	}
	if status, err := Permissions.Query(PermissionCamera); err != nil || status != PermissionNotDetermined {
		t.Fatalf("initial authorization: %q %v", status, err)
	}
	for _, decision := range []PermissionStatus{PermissionGranted, PermissionDenied, PermissionRestricted, PermissionLimited, PermissionProvisional} {
		onMain(func() { fb.PermissionDecision = string(decision) })
		status, err := Permissions.Request(PermissionCamera)
		if err != nil || status != decision {
			t.Fatalf("request: %q %v", status, err)
		}
		onMain(func() {
			status, err := Permissions.Query(PermissionCamera)
			if err != nil || status != decision {
				t.Errorf("query on UI thread: %q %v", status, err)
			}
		})
	}
	if err := Permissions.OpenSettings(); err != nil {
		t.Fatal(err)
	}
	onMain(func() {
		if fb.SettingsOpened != 1 {
			t.Error("did not open settings")
		}
		fb.PermissionError = platform.ErrUnsupported
	})
	if _, err := Permissions.Request(PermissionMicrophone); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported: %v", err)
	}
	if err := Permissions.OpenSettings(); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("settings unsupported: %v", err)
	}
}
