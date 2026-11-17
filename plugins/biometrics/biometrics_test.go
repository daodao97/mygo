package biometrics

import (
	"errors"
	"testing"

	"github.com/egoist/mygo"
)

func TestClassify(t *testing.T) {
	for code, want := range codes {
		native := &mygo.NativeError{Code: code}
		got := classify(native)
		if !errors.Is(got, want) {
			t.Errorf("%s: %v is not %v", code, got, want)
		}
		if want != ErrCanceled && want != ErrFailed {
			var n *mygo.NativeError
			if !errors.As(got, &n) || n.Code != code {
				t.Errorf("%s: lost the native error: %v", code, got)
			}
		}
	}
	other := &mygo.NativeError{Code: "Failed"}
	if got := classify(other); got != error(other) {
		t.Errorf("an unknown code changed: %v", got)
	}
	if got := classify(mygo.ErrUnsupported); !errors.Is(got, mygo.ErrUnsupported) {
		t.Errorf("unsupported: %v", got)
	}
}
