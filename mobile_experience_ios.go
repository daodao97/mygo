//go:build ios && cgo

package mygo

import (
	"encoding/json"
	"github.com/egoist/mygo/internal/ios"
)

func hostMobileExperience(kind, value string, done func(string, error)) func() {
	return ios.MobileExperience(kind, value, done)
}
func hostLiveActivityToken(fn func(LiveActivityInfo)) func() {
	var remove func()
	onMain(func() {
		remove = ios.OnLiveActivityToken(func(data string) {
			var info LiveActivityInfo
			if json.Unmarshal([]byte(data), &info) == nil {
				fn(info)
			}
		})
	})
	return func() { onMain(remove) }
}
