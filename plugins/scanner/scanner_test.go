package scanner

import (
	"errors"
	"testing"

	"github.com/egoist/mygo"
)

func TestClassify(t *testing.T) {
	for code, want := range codes {
		if got := classify(&mygo.NativeError{Code: code}); !errors.Is(got, want) {
			t.Errorf("%s: %v", code, got)
		}
	}
	other := &mygo.NativeError{Code: "Failed"}
	if got := classify(other); got != error(other) {
		t.Errorf("another code changed: %v", got)
	}
}
