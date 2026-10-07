//go:build ios && cgo

package ios

/*
#include "native.h"
*/
import "C"

import (
	"encoding/json"
	"github.com/egoist/mygo/internal/platform"
)

type permissions struct{}

func (*Backend) Permissions() platform.Permissions { return permissions{} }

func permission(kind string, request bool, done func(string, error)) {
	systemRequest(kind, func(id C.uint64_t, _ *C.char) {
		cString(kind, func(p *C.char) { C.mygo_ios_permission(id, p, C.bool(request)) })
	}, func(data string, err error) {
		var status string
		if err == nil {
			err = json.Unmarshal([]byte(data), &status)
		}
		done(status, err)
	})
}
func (permissions) Query(kind string, done func(string, error))   { permission(kind, false, done) }
func (permissions) Request(kind string, done func(string, error)) { permission(kind, true, done) }
func (permissions) OpenSettings(done func(error)) {
	systemRequest(nil, func(id C.uint64_t, _ *C.char) { C.mygo_ios_settings(id) }, func(_ string, err error) { done(err) })
}
