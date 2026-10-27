//go:build !ios || !cgo

package mygo

func hostMobileExperience(_, _ string, done func(string, error)) func() {
	done("", ErrUnsupported)
	return func() {}
}
func hostLiveActivityToken(func(LiveActivityInfo)) func() { return func() {} }
