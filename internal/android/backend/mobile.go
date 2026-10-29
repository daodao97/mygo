//go:build android && cgo

package backend

// The phone capabilities Android has: scanning the desktop's QR code, the
// haptic, the keyboard. The rest — badge, status bar, push — are unsupported
// and come from the embedded unsupported backend.
//
// Scanning is the system's camera app, not a preview of ours: ACTION_PICK an
// image with ACTION_IMAGE_CAPTURE, or the photo the person already has. That
// needs no CAMERA permission of this app (the camera app has it), no preview
// surface to manage, and no decoder compiled in — the Java side decodes with
// the platform's own in MygoPlatform.scanImage.

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

func (b *Backend) Mobile() platform.Mobile { return androidMobile{b: b} }

type androidMobile struct{ b *Backend }

// Haptic is the system's own feedback, and there is no JNI method for it yet:
// reporting it unsupported is honest, and the ui package falls back to nothing.
func (androidMobile) Haptic(string) error { return platform.ErrUnsupported }

// SetBadge answers at once: App.SetNotificationBadge awaits done, so a stub
// that never calls it hangs the caller for good.
func (androidMobile) SetBadge(_ int, done func(error)) { done(platform.ErrUnsupported) }
func (androidMobile) SetStatusBar(string, bool) error  { return platform.ErrUnsupported }
func (androidMobile) RegisterPush() error              { return platform.ErrUnsupported }

// Device names this phone to desktops. There is no JNI method for the model
// string yet, so it is the host's answer — right on the OS, generic on the
// model, which is enough to tell two phones apart by the app's own names.
func (androidMobile) Device() platform.DeviceInfo { return platform.HostDevice() }

// DismissKeyboard ends editing, which is what lowers the keyboard: the Ime
// tells the host it is inactive, and the host hides the soft input.
func (androidMobile) DismissKeyboard() {
	TheIme().SetTextInput(platform.TextInputState{})
}
func (androidMobile) PrepareNetwork(_ string, _ time.Duration, done func(error)) func() {
	done(nil)
	return func() {}
}

// ScanCode opens the system's image picker and decodes what comes back.
//
// The prompt and the cancel label are the Java side's to show, in its own
// dialog; they are passed and not used by the picker itself, which has its own
// chrome — passing them keeps the platform call one shape on both phones.
func (m androidMobile) ScanCode(o platform.ScanOptions, done func(string, error)) func() {
	if platformBridge() == nil {
		done("", platform.ErrUnsupported)
		return func() {}
	}
	token := nextToken()
	scanTokens.Store(token, done)
	log.Printf("mygo/android: scan: ScanCode token=%d bridge=%v", token, platformBridge() != nil)
	if err := platformBridge().ScanCode(token, o.Prompt); err != nil {
		scanTokens.Delete(token)
		done("", err)
	}
	return func() {
		if _, ok := scanTokens.LoadAndDelete(token); ok {
			_ = platformBridge().ScanCancel(token)
		}
	}
}

// errScanFailed is a decode that ran and found nothing: a picture with no QR
// code in it is the common case, and not something the person needs to read a
// stack for.
var errScanFailed = errors.New("mygo: no QR code was found in the picture")

var (
	scanMu     sync.Mutex
	scanToken  uint64
	scanTokens sync.Map // token → func(string, error)
)

func nextToken() uint64 {
	scanMu.Lock()
	defer scanMu.Unlock()
	scanToken++
	return scanToken
}

// ScanResult is what the Java side reports, on the pump: the decoded text, or
// the reason there is none. code is platform's own vocabulary over JNI — 0 for
// a value, 5 canceled, 6 denied, 7 no camera, 1 unsupported, 2 anything else.
func ScanResult(token uint64, text string, code int32) {
	v, ok := scanTokens.LoadAndDelete(token)
	if !ok {
		// Canceled by the caller while the picker was up: the answer is dropped,
		// which is what cancel means.
		return
	}
	done := v.(func(string, error))
	log.Printf("mygo/android: ScanResult code=%d len=%d head=%q", code, len(text), truncate(text, 40))
	switch code {
	case 0:
		if text == "" {
			done("", platform.ErrScanCanceled)
			return
		}
		done(text, nil)
	case 5:
		done("", platform.ErrScanCanceled)
	case 6:
		done("", platform.ErrCameraDenied)
	case 7:
		done("", platform.ErrCameraUnavailable)
	case 1:
		done("", platform.ErrUnsupported)
	default:
		done("", errScanFailed)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
