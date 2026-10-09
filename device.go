package mygo

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

// DeviceInfo describes the device for people, e.g. to name a paired phone.
// On iOS, Name is the user-assigned device name only with the
// com.apple.developer.device-information.user-assigned-device-name
// entitlement; otherwise iOS reports a generic model name. Desktops report
// the host name and their operating system.
type DeviceInfo struct {
	Name    string
	System  string // "iOS", "iPadOS", "macOS", "Linux" or "Windows"
	Version string // the system version where known, e.g. "18.2"
	Model   string // e.g. "iPhone"; empty on desktops
}

// Device returns the current device. It is safe from any goroutine and
// does not need a running application.
func (a *Application) Device() DeviceInfo {
	if !a.isReady() {
		return DeviceInfo(platform.HostDevice())
	}
	return DeviceInfo(onMainValue(func() platform.DeviceInfo { return backend().Mobile().Device() }))
}

// isReady reports, from any goroutine, whether the application finished
// launching. Device services work without an application before that.
func (a *Application) isReady() bool {
	select {
	case <-a.readyCh:
		return true
	default:
		return false
	}
}

// DismissKeyboard ends text editing in the application, hiding the iOS
// software keyboard even for native text fields MyGo does not draw. Go
// focus is a separate state: blur the focused element (ui.Context.Blur) as
// well. Desktop backends ignore it.
func (a *Application) DismissKeyboard() {
	if !a.isReady() {
		return
	}
	postMain(func() { backend().Mobile().DismissKeyboard() })
}

var (
	// ErrScanCanceled is ScanCode's error when the person closed the
	// scanner, or the context ended it.
	ErrScanCanceled = platform.ErrScanCanceled
	// ErrCameraDenied is ScanCode's error without camera authorization.
	ErrCameraDenied = platform.ErrCameraDenied
	// ErrCameraUnavailable is ScanCode's error without a usable camera.
	ErrCameraUnavailable = platform.ErrCameraUnavailable
)

// ScannerModule reads codes with the device camera.
type ScannerModule struct{}

var Scanner ScannerModule

// ScanOptions customizes the scanner's text; empty fields use a system
// cancel title and no prompt.
type ScanOptions struct {
	Prompt      string
	CancelLabel string
}

// ScanCode presents a full-screen camera scanner and returns the first QR
// code it reads. It requests camera access as needed; iOS requires
// NSCameraUsageDescription in ios.infoPlist. Canceling ctx dismisses the
// scanner and returns ctx.Err(). Only one system presentation can be active.
// Currently implemented on iOS; other backends return ErrUnsupported.
func (ScannerModule) ScanCode(ctx context.Context, o ScanOptions) (string, error) {
	needsApp("Scanner.ScanCode")
	if ctx == nil {
		return "", fmt.Errorf("mygo: scanning requires a context")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	type result struct {
		value string
		err   error
	}
	ch := make(chan result, 1)
	finished := make(chan struct{})
	if !postMain(func() {
		cancel := backend().Mobile().ScanCode(platform.ScanOptions(o), func(value string, err error) {
			if ctx.Err() != nil {
				value, err = "", ctx.Err()
			}
			deliver(ch, result{value, err})
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
		return "", errLoopStopped
	}
	r := await(ch)
	return r.value, r.err
}

// NetworkError reports why Network.Prepare failed: Kind is "offline" without
// a usable connection (or with cellular data off for the app), "timeout",
// "canceled" or "failed". Use errors.As.
type NetworkError = platform.NetworkError

// NetworkModule prepares the system network for Go's own sockets.
type NetworkModule struct {
	mu       sync.Mutex
	prepared bool
}

var Network = &NetworkModule{}

// Prepare makes one HEAD request to an https URL through the system network
// stack, before an app opens raw sockets with package net. On iOS this
// triggers the cellular-data and local-network prompts, and brings up VPN
// and DNS64 routes, which BSD sockets alone do not. Any HTTP response counts
// as success. After a success later calls return nil at once. Desktops have
// nothing to prepare and return nil. Canceling ctx stops the request.
func (n *NetworkModule) Prepare(ctx context.Context, rawURL string) error {
	if ctx == nil {
		return fmt.Errorf("mygo: network preparation requires a context")
	}
	u, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Host == "" {
		return fmt.Errorf("mygo: network preparation requires an https URL")
	}
	n.mu.Lock()
	prepared := n.prepared
	n.mu.Unlock()
	if prepared || !App.isReady() {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	timeout := 10 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		timeout = min(timeout, time.Until(deadline))
	}
	ch := make(chan error, 1)
	finished := make(chan struct{})
	if !postMain(func() {
		cancel := backend().Mobile().PrepareNetwork(rawURL, timeout, func(err error) {
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
	err = await(ch)
	if ctx.Err() != nil {
		var ne *NetworkError
		if errors.As(err, &ne) && ne.Kind == "canceled" {
			return ctx.Err()
		}
	}
	if err == nil {
		n.mu.Lock()
		n.prepared = true
		n.mu.Unlock()
	}
	return err
}
