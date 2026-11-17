//go:build ios || android

package permissionflow

import (
	"context"
	"errors"

	"github.com/egoist/mygo"
)

// Phones keep them through the plugin's native code. It uses the system's
// frameworks by name on iOS, not to make every app that imports it declare why
// it would use the camera: an app declares what it asks for.

const ungated = false

func title(_ Permission, t string) string { return t }

func call(ctx context.Context, method string, p Permission) (Status, error) {
	var r struct {
		Status string `json:"status"`
	}
	err := mygo.Native("permissionflow").Call(ctx, method, map[string]string{"permission": string(p)}, &r)
	var n *mygo.NativeError
	if errors.As(err, &n) {
		switch n.Code {
		case "Unsupported":
			return Unknown, errors.ErrUnsupported
		case "NoUsageDescription":
			return Unknown, ErrUsageDescription
		}
		return Unknown, err
	}
	if err != nil {
		return Unknown, err
	}
	switch r.Status {
	case "granted":
		return Granted, nil
	case "denied":
		return Denied, nil
	case "notDetermined":
		return NotDetermined, nil
	}
	return Unknown, nil
}

func check(p Permission) (Status, error) { return call(context.Background(), "check", p) }

func request(ctx context.Context, p Permission) (Status, error) { return call(ctx, "request", p) }

func openSettings(p Permission) error {
	return mygo.Native("permissionflow").Call(context.Background(), "openSettings", map[string]string{"permission": string(p)}, nil)
}

func guide(ctx context.Context, p Permission, _ GuideOptions) (Status, error) {
	if err := openSettings(p); err != nil {
		return Unknown, err
	}
	return Wait(ctx, p)
}
