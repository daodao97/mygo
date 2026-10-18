package mygo

import (
	"fmt"
	"strings"
)

func validBackgroundIdentifier(id string) bool {
	if !strings.Contains(id, ".") {
		return false
	}
	for _, part := range strings.Split(id, ".") {
		if part == "" {
			return false
		}
		for _, r := range part {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

// ScheduleBackgroundRefresh submits the configured identifier asynchronously.
// A negative nextDelay cancels pending requests. LastBackgroundRefreshError
// reports the latest native registration/submission error, if any. Currently iOS.
// This does not promise a wakeup time and does not require App.Run readiness.
func ScheduleBackgroundRefresh(identifier string) error {
	if !validBackgroundIdentifier(identifier) {
		return fmt.Errorf("mygo: invalid background task identifier")
	}
	return scheduleBackgroundRefresh(identifier)
}

// LastBackgroundRefreshError returns the latest native scheduling error text.
// Empty means no recorded error; an asynchronous submission may still be pending.
func LastBackgroundRefreshError(identifier string) (string, error) {
	if !validBackgroundIdentifier(identifier) {
		return "", fmt.Errorf("mygo: invalid background task identifier")
	}
	return lastBackgroundRefreshError(identifier)
}
