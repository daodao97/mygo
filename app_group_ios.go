//go:build ios && cgo

package mygo

import "github.com/egoist/mygo/internal/ios"

func hostAppGroupDirectory(identifier string) (string, error) {
	return ios.AppGroupDirectory(identifier)
}
