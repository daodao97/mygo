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

type secureStorage struct{}

func (*Backend) SecureStorage() platform.SecureStorage { return secureStorage{} }

type secretRequest struct {
	Operation, Namespace, Key, Accessibility string
	Value                                    []byte
}

func secret(o secretRequest, done func([]byte, error)) {
	systemRequest(o, func(id C.uint64_t, json *C.char) { C.mygo_ios_secret(id, json) }, func(data string, err error) {
		var value []byte
		if err == nil {
			err = json.Unmarshal([]byte(data), &value)
		}
		done(value, err)
	})
}
func (secureStorage) Set(namespace, key string, value []byte, accessibility string, done func(error)) {
	secret(secretRequest{Operation: "set", Namespace: namespace, Key: key, Accessibility: accessibility, Value: value}, func(_ []byte, err error) { done(err) })
}
func (secureStorage) Get(namespace, key string, done func([]byte, error)) {
	secret(secretRequest{Operation: "get", Namespace: namespace, Key: key}, done)
}
func (secureStorage) Delete(namespace, key string, done func(error)) {
	secret(secretRequest{Operation: "delete", Namespace: namespace, Key: key}, func(_ []byte, err error) { done(err) })
}
