//go:build ios && cgo

package ios

/*
#include "native.h"
*/
import "C"

import (
	"encoding/json"
	"errors"
	"github.com/egoist/mygo/internal/platform"
)

// Main-thread-only requests, removed before invoking their callbacks.
var systemRequests = map[uint64]func(string, error){}
var nextSystemRequest uint64

func systemRequest(value any, call func(C.uint64_t, *C.char), done func(string, error)) uint64 {
	data, err := json.Marshal(value)
	if err != nil {
		done("", err)
		return 0
	}
	nextSystemRequest++
	id := nextSystemRequest
	systemRequests[id] = done
	cString(string(data), func(p *C.char) { call(C.uint64_t(id), p) })
	return id
}

//export goIOSSystemResult
func goIOSSystemResult(token C.uint64_t, data *C.char, code C.int, message *C.char) {
	done := systemRequests[uint64(token)]
	delete(systemRequests, uint64(token))
	if done == nil {
		return
	}
	var err error
	switch code {
	case 1:
		err = platform.ErrUnsupported
	case 4:
		err = platform.ErrNotificationsDenied
	case 3:
		err = platform.ErrSecretNotFound
	case 2:
		err = errors.New(C.GoString(message))
	case 5:
		err = platform.ErrScanCanceled
	case 6:
		err = platform.ErrCameraDenied
	case 7:
		err = platform.ErrCameraUnavailable
	case 8:
		err = &platform.NetworkError{Kind: C.GoString(message)}
	}
	done(C.GoString(data), err)
}

type dialogs struct{ platform.Dialogs }

func (b *Backend) Dialogs() platform.Dialogs { return dialogs{b.Backend.Dialogs()} }

func (dialogs) ShowMessageBox(_ platform.Window, o *platform.MessageBoxOptions, done func(platform.MessageBoxResult, error)) {
	systemRequest(o, func(id C.uint64_t, json *C.char) { C.mygo_ios_message(id, json) }, func(data string, err error) {
		var r platform.MessageBoxResult
		if err == nil {
			err = json.Unmarshal([]byte(data), &r)
		}
		done(r, err)
	})
}

type sharing struct{}

func (*Backend) Sharing() platform.Sharing { return sharing{} }

func (sharing) Show(_ platform.Window, o platform.ShareOptions, done func(platform.ShareResult, error)) {
	systemRequest(o, func(id C.uint64_t, json *C.char) { C.mygo_ios_share(id, json) }, func(data string, err error) {
		var r platform.ShareResult
		if err == nil {
			err = json.Unmarshal([]byte(data), &r)
		}
		done(r, err)
	})
}

func (dialogs) ShowOpenDialog(_ platform.Window, o *platform.OpenDialogOptions, done func([]string, error)) {
	systemRequest(o, func(id C.uint64_t, json *C.char) { C.mygo_ios_open_files(id, json) }, importedResult(done))
}
func (dialogs) ShowExportDialog(_ platform.Window, o *platform.ExportDialogOptions, done func(bool, error)) {
	systemRequest(o, func(id C.uint64_t, json *C.char) { C.mygo_ios_export_files(id, json) }, func(data string, err error) {
		var r struct{ Completed bool }
		if err == nil {
			err = json.Unmarshal([]byte(data), &r)
		}
		done(r.Completed, err)
	})
}
func (dialogs) ShowPhotoDialog(_ platform.Window, o *platform.PhotoDialogOptions, done func([]string, error)) {
	systemRequest(o, func(id C.uint64_t, json *C.char) { C.mygo_ios_photos(id, json) }, importedResult(done))
}
func importedResult(done func([]string, error)) func(string, error) {
	return func(data string, err error) {
		var r struct{ Paths []string }
		if err == nil {
			err = json.Unmarshal([]byte(data), &r)
		}
		if len(r.Paths) == 0 {
			r.Paths = nil
		}
		done(r.Paths, err)
	}
}
