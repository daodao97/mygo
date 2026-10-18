//go:build !ios || !cgo

package mygo

func scheduleBackgroundRefresh(string) error            { return ErrUnsupported }
func lastBackgroundRefreshError(string) (string, error) { return "", ErrUnsupported }
