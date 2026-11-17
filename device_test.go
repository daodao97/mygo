//go:build !js

package mygo

import (
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

func TestDeviceAndKeyboard(t *testing.T) {
	onMain(func() {
		fb.DeviceInfo = platform.DeviceInfo{Name: "Test Phone", System: "iOS", Version: "18.0", Model: "iPhone"}
		fb.KeyboardDismissals = 0
	})
	defer onMain(func() { fb.DeviceInfo = platform.DeviceInfo{} })
	if d := App.Device(); d.Name != "Test Phone" || d.System != "iOS" || d.Version != "18.0" || d.Model != "iPhone" {
		t.Fatalf("device %+v", d)
	}
	App.DismissKeyboard()
	onMain(func() {
		if fb.KeyboardDismissals != 1 {
			t.Error("keyboard dismissal not forwarded")
		}
	})
}

// waitMain polls cond on the main thread until it holds.
func waitMain(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !onMainValue(cond) {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
