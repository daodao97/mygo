package platform

import "errors"

var ErrSecretNotFound = errors.New("mygo: secret not found")

// SecureStorage stores small binary values in the system's encrypted store.
// Native work may run off-thread, but every callback runs on the main thread
// exactly once. Implementations do not substitute unencrypted files.
type SecureStorage interface {
	Set(namespace, key string, value []byte, accessibility string, done func(error))
	Get(namespace, key string, done func([]byte, error))
	Delete(namespace, key string, done func(error))
}

type UnsupportedSecureStorage struct{}

func (UnsupportedSecureStorage) Set(_, _ string, _ []byte, _ string, done func(error)) {
	done(ErrUnsupported)
}
func (UnsupportedSecureStorage) Get(_, _ string, done func([]byte, error)) { done(nil, ErrUnsupported) }
func (UnsupportedSecureStorage) Delete(_, _ string, done func(error))      { done(ErrUnsupported) }
