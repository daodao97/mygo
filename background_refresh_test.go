package mygo

import (
	"errors"
	"runtime"
	"testing"
)

func TestBackgroundRefreshValidationAndUnsupported(t *testing.T) {
	for _, id := range []string{"", "onlyname", "org..task", "org.app/task", "org.app\x00task"} {
		if ScheduleBackgroundRefresh(id) == nil {
			t.Fatal("invalid accepted")
		}
	}
	if runtime.GOOS != "ios" {
		if e := ScheduleBackgroundRefresh("org.example.refresh"); !errors.Is(e, ErrUnsupported) {
			t.Fatal(e)
		}
		if _, e := LastBackgroundRefreshError("org.example.refresh"); !errors.Is(e, ErrUnsupported) {
			t.Fatal(e)
		}
	}
}
