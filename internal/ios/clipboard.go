//go:build ios && cgo

package ios

/*
#include "native.h"
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"runtime"
	"slices"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/transfer"
)

func (clipboard) ReadImage() []byte {
	var size C.size_t
	p := C.mygo_ios_clipboard_png(&size)
	if p == nil {
		return nil
	}
	defer C.free(p)
	return C.GoBytes(p, C.int(size))
}

// UIKit copies all supplied representations before returning. No Go provider
// or pointer remains retained by the pasteboard.
func (c clipboard) WriteData(data transfer.Data, released func()) error {
	var text *C.char
	var png []byte
	for _, f := range data.Formats() {
		switch f {
		case transfer.Text:
			value, err := data.Read(f)
			if err != nil {
				return err
			}
			text = C.CString(string(value))
			defer C.free(unsafe.Pointer(text))
		case transfer.PNG:
			var err error
			png, err = data.Read(f)
			if err != nil {
				return err
			}
			if len(png) == 0 {
				return errors.New("mygo: empty clipboard image")
			}
		default:
			return platform.ErrUnsupported
		}
	}
	var image unsafe.Pointer
	if len(png) > 0 {
		image = unsafe.Pointer(&png[0])
	}
	ok := bool(C.mygo_ios_set_clipboard_data(text, image, C.size_t(len(png))))
	runtime.KeepAlive(png)
	if !ok {
		return errors.New("mygo: invalid clipboard image")
	}
	if released != nil {
		released()
	}
	return nil
}

func (c clipboard) ReadData(formats []transfer.Format) (transfer.Data, error) {
	before := C.mygo_ios_clipboard_change()
	var reps []transfer.Representation
	for _, f := range c.Formats() {
		if formats != nil && !slices.Contains(formats, f) {
			continue
		}
		switch f {
		case transfer.PNG:
			if png := c.ReadImage(); len(png) > 0 {
				reps = append(reps, transfer.Bytes(f, png))
			}
		case transfer.Text:
			reps = append(reps, transfer.Bytes(f, []byte(c.ReadText())))
		}
	}
	if before != C.mygo_ios_clipboard_change() {
		return transfer.Data{}, platform.ErrClipboardChanged
	}
	if len(reps) == 0 {
		if formats != nil {
			return transfer.Data{}, transfer.ErrFormat
		}
		return transfer.Data{}, nil
	}
	return transfer.New(transfer.NewItem(reps...)), nil
}
func (clipboard) Formats() []transfer.Format {
	flags := uint32(C.mygo_ios_clipboard_formats())
	var formats []transfer.Format
	if flags&2 != 0 {
		formats = append(formats, transfer.PNG)
	}
	if flags&1 != 0 {
		formats = append(formats, transfer.Text)
	}
	return formats
}
func (clipboard) Flush() error { return nil }
func (clipboard) Close()       {}
