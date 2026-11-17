package network

import (
	"context"
	"errors"
	"testing"

	"github.com/egoist/mygo"
)

func TestClassify(t *testing.T) {
	for _, kind := range []string{"offline", "timeout", "canceled", "failed"} {
		var e *Error
		if !errors.As(classify(&mygo.NativeError{Code: kind}), &e) || e.Kind != kind {
			t.Errorf("%s: %v", kind, e)
		}
	}
	other := &mygo.NativeError{Code: "NoPlugin"}
	if got := classify(other); got != error(other) {
		t.Errorf("another code changed: %v", got)
	}
}

func TestPrepareNeedsHTTPS(t *testing.T) {
	for _, raw := range []string{"http://example.com", "https://", "::"} {
		if Prepare(context.Background(), raw) == nil {
			t.Errorf("%q was taken", raw)
		}
	}
}

func TestInterfacesUnsupportedOnDesktops(t *testing.T) {
	if _, err := Interfaces(context.Background()); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("err = %v", err)
	}
}
