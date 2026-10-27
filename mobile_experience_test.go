package mygo

import (
	"context"
	"errors"
	"math"
	"testing"
)

func TestMobileExperienceValidationAndUnsupportedBackend(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "javascript:alert(1)", "https://user:secret@example.com", "localhost:3000"} {
		if err := OpenInAppBrowser(context.Background(), raw); err == nil || errors.Is(err, ErrUnsupported) {
			t.Fatalf("invalid URL reached native browser: %q", raw)
		}
	}
	if err := OpenInAppBrowser(context.Background(), "http://127.0.0.1:3000/preview?q=1"); !errors.Is(err, ErrUnsupported) {
		t.Fatal("unsupported backend lost sentinel", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := OpenInAppBrowser(ctx, "https://example.com"); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled presentation started", err)
	}
	for _, progress := range []float64{-1, 2, math.NaN(), math.Inf(1)} {
		if _, err := UpdateLiveActivity(context.Background(), "fixture", LiveActivityState{Progress: &progress}); err == nil || errors.Is(err, ErrUnsupported) {
			t.Fatal("invalid progress reached native activity")
		}
	}
	if _, err := UpdateLiveActivity(context.Background(), "fixture", LiveActivityState{Title: "Retty", Status: "running"}); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
}
