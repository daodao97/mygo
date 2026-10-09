package mygo

import (
	"context"
	"errors"
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

func TestScanCode(t *testing.T) {
	onMain(func() { fb.ScanResult, fb.ScanError, fb.ScanPending = "tailcat://code", nil, false })
	defer onMain(func() { fb.ScanResult, fb.ScanError, fb.ScanPending = "", nil, false })
	value, err := Scanner.ScanCode(context.Background(), ScanOptions{Prompt: "Scan", CancelLabel: "Close"})
	if err != nil || value != "tailcat://code" {
		t.Fatalf("scan %q %v", value, err)
	}
	onMain(func() {
		if fb.LastScan.Prompt != "Scan" || fb.LastScan.CancelLabel != "Close" {
			t.Errorf("options %+v", fb.LastScan)
		}
	})
	onMain(func() { fb.ScanResult, fb.ScanError = "", platform.ErrCameraDenied })
	if _, err := Scanner.ScanCode(context.Background(), ScanOptions{}); !errors.Is(err, ErrCameraDenied) {
		t.Fatalf("denied: %v", err)
	}
	// Canceling the context dismisses a scanner that is still open.
	onMain(func() { fb.ScanPending = true })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := Scanner.ScanCode(ctx, ScanOptions{}); done <- err }()
	waitMain(t, func() bool { return fb.ScanOpen() })
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled scan: %v", err)
	}
	if _, err := Scanner.ScanCode(ctx, ScanOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ended context: %v", err)
	}
}

func TestNetworkPrepare(t *testing.T) {
	n := &NetworkModule{}
	if err := n.Prepare(context.Background(), "http://example.com"); err == nil {
		t.Fatal("accepted a plain http URL")
	}
	onMain(func() {
		fb.NetworkURLs, fb.NetworkError, fb.NetworkPending = nil, &platform.NetworkError{Kind: "offline"}, false
	})
	defer onMain(func() { fb.NetworkURLs, fb.NetworkError, fb.NetworkPending = nil, nil, false })
	var ne *NetworkError
	if err := n.Prepare(context.Background(), "https://derp.example.com/"); !errors.As(err, &ne) || ne.Kind != "offline" {
		t.Fatalf("offline: %v", err)
	}
	onMain(func() { fb.NetworkError = nil })
	for range 2 {
		if err := n.Prepare(context.Background(), "https://derp.example.com/"); err != nil {
			t.Fatal(err)
		}
	}
	onMain(func() {
		if len(fb.NetworkURLs) != 2 {
			t.Errorf("a prepared network was requested again: %v", fb.NetworkURLs)
		}
	})
	n = &NetworkModule{}
	onMain(func() { fb.NetworkPending = true })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- n.Prepare(ctx, "https://derp.example.com/") }()
	waitMain(t, func() bool { return fb.NetworkOpen() })
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled preparation: %v", err)
	}
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
