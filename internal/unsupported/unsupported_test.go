package unsupported

import (
	"errors"
	"runtime"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// The backend of a platform that MyGo doesn't draw on says so, and answers the
// check an app makes to find out whether a call it can live without is worth
// asking for.
func TestErrorsAreUnsupported(t *testing.T) {
	b := New()
	errs := map[string]error{
		"Init":      b.Init(nil, platform.AppOptions{}),
		"Run":       b.Run(),
		"NewWindow": newWindow(b),
		"NewTray":   newTray(b),
	}
	for name, err := range errs {
		if err == nil {
			t.Errorf("%s returned no error", name)
			continue
		}
		if !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("%s: errors.Is(err, platform.ErrUnsupported) = false, for %q", name, err)
		}
		if !errors.Is(err, errors.ErrUnsupported) {
			t.Errorf("%s: errors.Is(err, errors.ErrUnsupported) = false, for %q", name, err)
		}
		want := "mygo: " + runtime.GOOS + "/" + runtime.GOARCH + " is not supported yet"
		if err.Error() != want {
			t.Errorf("%s: Error() = %q, want %q", name, err.Error(), want)
		}
	}
}

func newWindow(b *Backend) error {
	_, err := b.NewWindow(&platform.WindowOptions{}, nil)
	return err
}

func newTray(b *Backend) error {
	_, err := b.NewTray("MyGo", nil)
	return err
}
