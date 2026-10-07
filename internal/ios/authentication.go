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

type authenticator struct{}

func (*Backend) Authenticator() platform.Authenticator { return authenticator{} }
func (authenticator) Query(done func(platform.BiometricStatus, error)) {
	systemRequest(nil, func(id C.uint64_t, _ *C.char) { C.mygo_ios_auth_query(id) }, func(data string, err error) {
		var s platform.BiometricStatus
		if err == nil {
			err = json.Unmarshal([]byte(data), &s)
		}
		done(s, err)
	})
}
func (authenticator) Authenticate(o platform.AuthenticationOptions, done func(error)) func() {
	id := systemRequest(o, func(id C.uint64_t, p *C.char) { C.mygo_ios_authenticate(id, p) }, func(data string, err error) {
		if err == nil {
			var result struct {
				Code    platform.AuthenticationErrorCode
				Message string
			}
			err = json.Unmarshal([]byte(data), &result)
			if err == nil && result.Code != "" {
				err = &platform.AuthenticationError{Code: result.Code, Message: result.Message}
			}
		}
		done(err)
	})
	return func() { C.mygo_ios_auth_cancel(C.uint64_t(id)) }
}
