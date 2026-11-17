// Package biometrics confirms it is the device's owner with what the device
// has: Face ID or Touch ID on iOS (LocalAuthentication), and fingerprints or
// faces on Android 9 and later (BiometricPrompt), falling back to the device's
// passcode. Elsewhere its functions report errors.ErrUnsupported.
//
// The errors after ErrFailed are the fork's (FORK.md): #940 tells only a
// cancel and a failure apart.
package biometrics

import (
	"context"
	"errors"
	"fmt"

	"github.com/egoist/mygo"
)

const pluginID = "biometrics"

// Kind is what the device confirms its owner with.
type Kind string

const (
	None       Kind = ""
	FaceID     Kind = "face"
	TouchID    Kind = "fingerprint"
	Iris       Kind = "iris"
	DeviceCode Kind = "passcode"
	Biometric  Kind = "biometric"
)

// ErrCanceled is what Authenticate fails with as the user goes back.
var ErrCanceled = errors.New("biometrics: canceled")

// ErrFailed is what Authenticate fails with as the device didn't recognize
// the user.
var ErrFailed = errors.New("biometrics: not recognized")

// Why Authenticate could not ask at all. The error Authenticate returns wraps
// one of them and the native code's own (a *mygo.NativeError).
var (
	// ErrUnavailable: the device has nothing to confirm its owner with.
	ErrUnavailable = errors.New("biometrics: unavailable")
	// ErrNotEnrolled: biometrics are not set up.
	ErrNotEnrolled = errors.New("biometrics: not enrolled")
	// ErrLockedOut: too many failures; the system locked biometrics.
	ErrLockedOut = errors.New("biometrics: locked out")
	// ErrPasscodeNotSet: the device has no passcode to fall back to.
	ErrPasscodeNotSet = errors.New("biometrics: no device passcode")
	// ErrBusy: another prompt is showing.
	ErrBusy = errors.New("biometrics: busy")
	// ErrInactive: the app is not in front, where the prompt shows.
	ErrInactive = errors.New("biometrics: app inactive")
	// ErrMissingPurpose: the app's bundle lacks the usage description the
	// system asks for (NSFaceIDUsageDescription).
	ErrMissingPurpose = errors.New("biometrics: missing usage description")
)

// Available tells what the device can confirm its owner with now, None when
// nothing, as without a passcode set.
func Available(ctx context.Context) (Kind, error) {
	var r struct {
		Kind string `json:"kind"`
	}
	err := mygo.Native(pluginID).Call(ctx, "available", nil, &r)
	return Kind(r.Kind), err
}

// Authenticate asks the user to confirm it's them, saying reason. It returns
// nil once they did. Call it off the main goroutine, as it waits for the user.
func Authenticate(ctx context.Context, reason string) error {
	return classify(mygo.Native(pluginID).Call(ctx, "authenticate", map[string]string{"reason": reason}, nil))
}

var codes = map[string]error{
	"Canceled":       ErrCanceled,
	"NotRecognized":  ErrFailed,
	"Unavailable":    ErrUnavailable,
	"NotEnrolled":    ErrNotEnrolled,
	"LockedOut":      ErrLockedOut,
	"PasscodeNotSet": ErrPasscodeNotSet,
	"Busy":           ErrBusy,
	"Inactive":       ErrInactive,
	"MissingPurpose": ErrMissingPurpose,
}

// classify gives the native code's error the error of this package it is.
func classify(err error) error {
	var n *mygo.NativeError
	if !errors.As(err, &n) {
		return err
	}
	kind, ok := codes[n.Code]
	if !ok {
		return err
	}
	if kind == ErrCanceled || kind == ErrFailed {
		return kind // as #940 returns them
	}
	return fmt.Errorf("%w: %w", kind, err)
}
