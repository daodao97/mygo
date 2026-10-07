package fake

import "github.com/egoist/mygo/internal/platform"

func (b *Backend) Authenticator() platform.Authenticator { return authenticator{b} }

type authenticator struct{ b *Backend }

func (a authenticator) Query(done func(platform.BiometricStatus, error)) {
	done(a.b.BiometricStatus, a.b.AuthenticationError)
}
func (a authenticator) Authenticate(o platform.AuthenticationOptions, done func(error)) func() {
	if a.b.authenticationDone != nil {
		done(&platform.AuthenticationError{Code: platform.AuthenticationBusy})
		return func() {}
	}
	a.b.LastAuthentication = o
	if !a.b.AuthenticationPending {
		done(a.b.AuthenticationError)
		return func() {}
	}
	a.b.authenticationDone = done
	a.b.authenticationID++
	id := a.b.authenticationID
	return func() {
		if a.b.authenticationDone != nil && a.b.authenticationID == id {
			fn := a.b.authenticationDone
			a.b.authenticationDone = nil
			fn(&platform.AuthenticationError{Code: platform.AuthenticationCanceled})
		}
	}
}

// CompleteAuthentication simulates a native result on the main thread.
func (b *Backend) CompleteAuthentication(err error) {
	fn := b.authenticationDone
	b.authenticationDone = nil
	if fn != nil {
		fn(err)
	}
}
