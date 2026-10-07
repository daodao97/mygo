package mygo

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/internal/platform"
)

type AuthenticationErrorCode = platform.AuthenticationErrorCode

const (
	AuthenticationCanceled       = platform.AuthenticationCanceled
	AuthenticationFailed         = platform.AuthenticationFailed
	AuthenticationUnavailable    = platform.AuthenticationUnavailable
	AuthenticationNotEnrolled    = platform.AuthenticationNotEnrolled
	AuthenticationLockedOut      = platform.AuthenticationLockedOut
	AuthenticationPasscodeNotSet = platform.AuthenticationPasscodeNotSet
	AuthenticationFallback       = platform.AuthenticationFallback
	AuthenticationBusy           = platform.AuthenticationBusy
	AuthenticationInactive       = platform.AuthenticationInactive
	AuthenticationMissingPurpose = platform.AuthenticationMissingPurpose
	AuthenticationUnknown        = platform.AuthenticationUnknown
)

// AuthenticationError distinguishes user/system cancellation, denial,
// lockout, missing enrollment and other native failures. Use errors.As.
type AuthenticationError = platform.AuthenticationError

type BiometricKind string

const (
	BiometricNone    BiometricKind = "none"
	BiometricTouchID BiometricKind = "touch-id"
	BiometricFaceID  BiometricKind = "face-id"
)

// BiometricStatus is a fresh capability check, not a persisted permission.
// Available refers to biometrics alone. DeviceAuthenticationAvailable also
// permits the OS device-passcode fallback, including during biometric lockout.
type BiometricStatus struct {
	Kind                          BiometricKind
	Available                     bool
	UnavailableReason             AuthenticationErrorCode
	DeviceAuthenticationAvailable bool
}

type AuthenticationOptions struct {
	// Reason explains why authentication is needed; it must be nonempty.
	Reason string
	// AllowDevicePasscode opts into the OS's passcode fallback. The default
	// requires biometrics. MyGo never receives the passcode or biometric data.
	AllowDevicePasscode bool
}

type BiometricModule struct{}

var Biometrics BiometricModule

// Query checks availability without presenting authentication UI. Currently
// implemented on iOS; other backends return ErrUnsupported.
func (BiometricModule) Query() (BiometricStatus, error) {
	needsApp("Biometrics.Query")
	type result struct {
		status BiometricStatus
		err    error
	}
	ch := make(chan result, 1)
	if !postMain(func() {
		backend().Authenticator().Query(func(s platform.BiometricStatus, err error) {
			deliver(ch, result{BiometricStatus{BiometricKind(s.Kind), s.Available, s.UnavailableReason, s.DeviceAuthenticationAvailable}, err})
		})
	}) {
		return BiometricStatus{}, errLoopStopped
	}
	r := await(ch)
	return r.status, r.err
}

// Authenticate waits for the native prompt while continuing to process UI
// events. ctx cancellation dismisses the prompt and returns ctx.Err(). iOS
// requires an active Scene and NSFaceIDUsageDescription for Face ID. Background
// entry or Scene disconnection cancels pending authentication. Success is a
// local device-owner check; it does not protect a Keychain item or log in to a
// remote service. Each call uses a fresh context, without cached-auth reuse.
func (BiometricModule) Authenticate(ctx context.Context, o AuthenticationOptions) error {
	needsApp("Biometrics.Authenticate")
	if ctx == nil {
		return fmt.Errorf("mygo: authentication requires a context")
	}
	if strings.TrimSpace(o.Reason) == "" || strings.ContainsRune(o.Reason, 0) {
		return fmt.Errorf("mygo: authentication requires a nonempty reason without NUL")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ch := make(chan error, 1)
	finished := make(chan struct{})
	if !postMain(func() {
		if err := ctx.Err(); err != nil {
			deliver(ch, err)
			close(finished)
			return
		}
		cancel := backend().Authenticator().Authenticate(platform.AuthenticationOptions{Reason: o.Reason, AllowDevicePasscode: o.AllowDevicePasscode}, func(err error) {
			if ctx.Err() != nil {
				err = ctx.Err()
			}
			deliver(ch, err)
			close(finished)
		})
		go func() {
			select {
			case <-finished:
			case <-ctx.Done():
				postMain(cancel)
			}
		}()
	}) {
		return errLoopStopped
	}
	return await(ch)
}
