package mygo

import (
	"errors"
	"runtime"
	"testing"
)

func TestAppGroupValidationAndUnsupported(t *testing.T) {
	for _, id := range []string{"", "group.", "private.app", "group.a/b", "group.a\x00b"} {
		if _, err := AppGroupDirectory(id); err == nil {
			t.Fatalf("accepted %q", id)
		}
	}
	if runtime.GOOS != "ios" {
		if _, err := AppGroupDirectory("group.org.example.app"); !errors.Is(err, ErrUnsupported) {
			t.Fatal(err)
		}
	}
}
