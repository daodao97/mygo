// Package scanner reads QR codes with the device's camera: in a full-screen
// scanner of AVFoundation on iOS, and on Android from a photo the camera
// takes or the library holds, decoded with ZXing. It asks for the camera as
// needed. Elsewhere Scan reports errors.ErrUnsupported.
//
// It is the fork's (FORK.md): #940 has no counterpart.
package scanner

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/egoist/mygo"
)

const pluginID = "scanner"

var (
	// ErrCanceled is what Scan fails with as the user closes the scanner.
	ErrCanceled = errors.New("scanner: canceled")
	// ErrCameraDenied is what Scan fails with without the camera's permission.
	ErrCameraDenied = errors.New("scanner: camera access is not allowed")
	// ErrCameraUnavailable is what Scan fails with without a usable camera.
	ErrCameraUnavailable = errors.New("scanner: camera unavailable")
	// ErrNotFound is what Scan fails with as a picture holds no QR code
	// (Android).
	ErrNotFound = errors.New("scanner: no QR code was found in the picture")
	// ErrBusy is what Scan fails with as another scan runs.
	ErrBusy = errors.New("scanner: another scan runs")
)

// Options are the scanner's text; empty fields take the system's cancel
// title and no prompt.
type Options struct {
	Prompt      string
	CancelLabel string
}

var ids atomic.Int64

// Scan shows the scanner and returns the first QR code it reads. Canceling
// ctx closes it and returns ctx.Err(). Call it off the main goroutine, as
// it waits for the user.
func Scan(ctx context.Context, o Options) (string, error) {
	if ctx == nil {
		return "", errors.New("scanner: Scan needs a context")
	}
	p := mygo.Native(pluginID)
	if !p.Available() {
		return "", mygo.ErrUnsupported
	}
	id := ids.Add(1)
	var r struct {
		Value string `json:"value"`
	}
	err := p.Call(ctx, "scan", map[string]any{"id": id, "prompt": o.Prompt, "cancelLabel": o.CancelLabel}, &r)
	if ctx.Err() != nil {
		go p.Call(context.Background(), "cancel", map[string]any{"id": id}, nil)
		return "", ctx.Err()
	}
	if err != nil {
		return "", classify(err)
	}
	if r.Value == "" {
		return "", ErrCanceled
	}
	return r.Value, nil
}

var codes = map[string]error{
	"Canceled":    ErrCanceled,
	"Denied":      ErrCameraDenied,
	"Unavailable": ErrCameraUnavailable,
	"NotFound":    ErrNotFound,
	"Busy":        ErrBusy,
}

func classify(err error) error {
	var n *mygo.NativeError
	if errors.As(err, &n) {
		if e, ok := codes[n.Code]; ok {
			return e
		}
	}
	return err
}
