//go:build ios && cgo

package mygo

import iosbackend "github.com/egoist/mygo/internal/ios"

func scheduleBackgroundRefresh(id string) error { return iosbackend.ScheduleBackgroundRefresh(id) }
func lastBackgroundRefreshError(id string) (string, error) {
	return iosbackend.LastBackgroundRefreshError(id)
}
