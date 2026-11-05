//go:build ios && cgo

package ios

/*
#include <stdlib.h>
#include "native.h"
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

// The operations of mygo_ios_keychain, and the status of a missing item.
const (
	keychainSet        = 1
	keychainGet        = 2
	keychainDelete     = 3
	errSecItemNotFound = -25300
)

type secrets struct{}

// Secrets keeps secrets in the Keychain, as generic passwords of the service
// and account, on this device only and readable while it is unlocked.
func (*Backend) Secrets() platform.Secrets { return secrets{} }

func (secrets) SetSecret(service, account string, secret []byte) error {
	_, err := keychain(keychainSet, service, account, secret)
	return err
}

func (secrets) Secret(service, account string) ([]byte, error) {
	return keychain(keychainGet, service, account, nil)
}

func (secrets) DeleteSecret(service, account string) error {
	_, err := keychain(keychainDelete, service, account, nil)
	return err
}

// keychain runs one Keychain operation and waits for it. Secrets calls it off
// the main thread; it does not need the main thread either.
func keychain(op int, service, account string, value []byte) ([]byte, error) {
	cs, ca := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(cs))
	defer C.free(unsafe.Pointer(ca))
	var in unsafe.Pointer
	if len(value) > 0 {
		in = C.CBytes(value)
		defer C.free(in)
	}
	var out unsafe.Pointer
	var n C.int
	var msg *C.char
	status := C.mygo_ios_keychain(C.int(op), cs, ca, in, C.int(len(value)), &out, &n, &msg)
	if out != nil {
		defer C.free(out)
	}
	if msg != nil {
		defer C.free(unsafe.Pointer(msg))
	}
	switch status {
	case 0:
		if op == keychainGet {
			return C.GoBytes(out, n), nil
		}
		return nil, nil
	case errSecItemNotFound:
		return nil, platform.ErrSecretNotFound
	}
	detail := "unknown error"
	if msg != nil {
		detail = C.GoString(msg)
	}
	return nil, fmt.Errorf("mygo: keychain error %d: %s", int(status), detail)
}
