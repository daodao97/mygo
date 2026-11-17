// Package biometrics confirms it is the device's owner with what the device
// has: Face ID or Touch ID on iOS (LocalAuthentication), and fingerprints or
// faces on Android 9 and later (BiometricPrompt), falling back to the device's
// passcode. Elsewhere its functions report errors.ErrUnsupported.
package biometrics

import (
	"context"
	"errors"

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
	err := mygo.Native(pluginID).Call(ctx, "authenticate", map[string]string{"reason": reason}, nil)
	var n *mygo.NativeError
	if errors.As(err, &n) {
		switch n.Code {
		case "Canceled":
			return ErrCanceled
		case "NotRecognized":
			return ErrFailed
		}
	}
	return err
}
