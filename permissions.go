package mygo

import (
	"fmt"
	"github.com/egoist/mygo/internal/platform"
)

// Permission is something a page asks the user for.
type Permission string

// Permissions.
const (
	PermissionCamera        Permission = "camera"
	PermissionMicrophone    Permission = "microphone"
	PermissionGeolocation   Permission = "geolocation"
	PermissionNotifications Permission = "notifications"
	PermissionPhotos        Permission = "photos"
	PermissionPhotosAddOnly Permission = "photos-add-only"
)

// PermissionStatus is the operating system's authorization decision. Limited
// permits only user-selected photos; Provisional permits quiet notifications.
type PermissionStatus string

const (
	PermissionNotDetermined PermissionStatus = "not-determined"
	PermissionDenied        PermissionStatus = "denied"
	PermissionRestricted    PermissionStatus = "restricted"
	PermissionGranted       PermissionStatus = "granted"
	PermissionLimited       PermissionStatus = "limited"
	PermissionProvisional   PermissionStatus = "provisional"
)

// PermissionModule manages native application permissions. These decisions
// are separate from a web page's SetPermissionHandler policy.
type PermissionModule struct{}

var Permissions PermissionModule

// Query reads authorization without presenting a prompt. Currently supported
// on iOS. Unsupported backends return ErrUnsupported, rather than a denial.
func (PermissionModule) Query(kind Permission) (PermissionStatus, error) {
	return nativePermission(kind, false)
}

// Request asks the OS for authorization and returns its resulting status.
// It may wait for the user; native events continue on the UI thread. Denial
// is a status, not an error. iOS requires the corresponding non-empty purpose
// string in ios.infoPlist; a missing string returns an error before calling
// the native API. Geolocation requests authorization while the app is in use.
func (PermissionModule) Request(kind Permission) (PermissionStatus, error) {
	return nativePermission(kind, true)
}

func nativePermission(kind Permission, request bool) (PermissionStatus, error) {
	needsApp("Permissions")
	switch kind {
	case PermissionCamera, PermissionMicrophone, PermissionGeolocation, PermissionNotifications, PermissionPhotos, PermissionPhotosAddOnly:
	default:
		return "", fmt.Errorf("mygo: unknown permission %q", kind)
	}
	type result struct {
		status PermissionStatus
		err    error
	}
	ch := make(chan result, 1)
	if !postMain(func() {
		done := func(status string, err error) { deliver(ch, result{PermissionStatus(status), err}) }
		if request {
			backend().Permissions().Request(string(kind), done)
		} else {
			backend().Permissions().Query(string(kind), done)
		}
	}) {
		return "", errLoopStopped
	}
	r := await(ch)
	return r.status, r.err
}

// OpenSettings opens this application's system settings so the user can
// revise a denial. After returning to the foreground, Query again; opening
// settings itself does not grant permission.
func (PermissionModule) OpenSettings() error {
	needsApp("Permissions.OpenSettings")
	ch := make(chan error, 1)
	if !postMain(func() { backend().Permissions().OpenSettings(func(err error) { deliver(ch, err) }) }) {
		return errLoopStopped
	}
	return await(ch)
}

var _ platform.Permissions

// PermissionRequest is passed to the permission handler of a window.
type PermissionRequest struct {
	// Permissions asked for together, such as the camera and the
	// microphone of a video call.
	Permissions []Permission
	// Origin of the page asking, e.g. "https://example.com".
	Origin string
}

// SetPermissionHandler decides what the pages of the window may use, such
// as the camera: fn returns whether to grant a request, and runs on the main
// thread. nil restores the default: the app's own pages (see
// PageOptions.TrustedOrigins) get what they ask for, other pages do not.
//
// The operating system may still ask the user, like macOS does once per
// app for the camera and the microphone. That needs usage descriptions in
// macos.infoPlist of mygo.config.ts (NSCameraUsageDescription,
// NSMicrophoneUsageDescription), without which macOS ends the app.
func (p *Page) SetPermissionHandler(fn func(req PermissionRequest) bool) {
	p.w.mu.Lock()
	p.w.permissionHandler = fn
	p.w.mu.Unlock()
}

func (h *windowHandler) PermissionRequested(kinds []string, origin string) bool {
	h.w.mu.Lock()
	fn := h.w.permissionHandler
	h.w.mu.Unlock()
	if fn == nil {
		return h.w.isTrusted(origin)
	}
	req := PermissionRequest{Origin: origin}
	for _, k := range kinds {
		req.Permissions = append(req.Permissions, Permission(k))
	}
	return fn(req)
}
