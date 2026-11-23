package platform

import (
	"errors"
	"fmt"
	"testing"
)

// An app tells a call it can live without by checking its error against
// ErrUnsupported, so an error a backend says in its own words still answers
// that check.
func TestUnsupportedErrors(t *testing.T) {
	browsers := Unsupported("mygo: browsers do not support this")
	tests := []struct {
		name string
		err  error
		is   bool
		want string
	}{
		{"the error itself", ErrUnsupported, true, "not supported on this platform"},
		{"a backend's error", browsers, true, "mygo: browsers do not support this"},
		{"a wrapped backend's error", fmt.Errorf("opening a tray: %w", browsers), true, "opening a tray: mygo: browsers do not support this"},
		{"an error of another kind", errors.New("mygo: no such file"), false, "mygo: no such file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errors.Is(tt.err, ErrUnsupported); got != tt.is {
				t.Errorf("errors.Is(err, ErrUnsupported) = %v, want %v", got, tt.is)
			}
			if got := errors.Is(tt.err, errors.ErrUnsupported); got != tt.is {
				t.Errorf("errors.Is(err, errors.ErrUnsupported) = %v, want %v", got, tt.is)
			}
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}
