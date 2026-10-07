package mygo

import (
	"fmt"
	"github.com/egoist/mygo/internal/platform"
	"strings"
)

// ErrSecretNotFound reports an absent secure-store key. Delete is idempotent
// and does not return this error for an already absent key.
var ErrSecretNotFound = platform.ErrSecretNotFound

// SecretAccessibility controls when iOS can read an encrypted item. Both
// policies are device-local and do not migrate items to a different device.
type SecretAccessibility string

const (
	SecretWhenUnlocked     SecretAccessibility = ""
	SecretAfterFirstUnlock SecretAccessibility = "after-first-unlock"
)

type SecureStoreOptions struct{ Accessibility SecretAccessibility }

// SecureStore stores small secrets in the OS keychain. Namespaces are scoped
// to the application's bundle identifier. A store may be created before Run;
// its operations need a running application and are safe from any goroutine.
// Currently implemented on iOS, with ErrUnsupported on other backends.
type SecureStore struct {
	namespace string
	access    SecretAccessibility
}

// NewSecureStore creates a namespace using the specified access policy.
// The default permits reads only while the device is unlocked. This API
// neither synchronizes with iCloud nor implements biometric access control.
func NewSecureStore(namespace string, options SecureStoreOptions) (*SecureStore, error) {
	if strings.TrimSpace(namespace) == "" || strings.ContainsRune(namespace, 0) {
		return nil, fmt.Errorf("mygo: secure store requires a nonempty namespace without NUL")
	}
	if options.Accessibility != SecretWhenUnlocked && options.Accessibility != SecretAfterFirstUnlock {
		return nil, fmt.Errorf("mygo: invalid secret accessibility %q", options.Accessibility)
	}
	return &SecureStore{namespace: namespace, access: options.Accessibility}, nil
}

func (s *SecureStore) validate(key string) error {
	if s == nil || s.namespace == "" {
		return fmt.Errorf("mygo: secure store must be created with NewSecureStore")
	}
	if key == "" || strings.ContainsRune(key, 0) {
		return fmt.Errorf("mygo: secure store key must be nonempty and contain no NUL")
	}
	return nil
}

// Set atomically adds or replaces a key. Empty values are supported. The
// backend copies data and never writes a plaintext fallback file.
func (s *SecureStore) Set(key string, value []byte) error {
	needsApp("SecureStore.Set")
	if err := s.validate(key); err != nil {
		return err
	}
	data := make([]byte, len(value))
	copy(data, value)
	ch := make(chan error, 1)
	if !postMain(func() {
		backend().SecureStorage().Set(s.namespace, key, data, string(s.access), func(err error) { deliver(ch, err) })
	}) {
		return errLoopStopped
	}
	return await(ch)
}

// Get returns an independently owned value, or ErrSecretNotFound. Locked
// device and keychain failures are errors, not absent keys.
func (s *SecureStore) Get(key string) ([]byte, error) {
	needsApp("SecureStore.Get")
	if err := s.validate(key); err != nil {
		return nil, err
	}
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	if !postMain(func() {
		backend().SecureStorage().Get(s.namespace, key, func(data []byte, err error) { deliver(ch, result{data, err}) })
	}) {
		return nil, errLoopStopped
	}
	r := await(ch)
	return r.data, r.err
}

// Delete removes this key, succeeding when it is already absent.
func (s *SecureStore) Delete(key string) error {
	needsApp("SecureStore.Delete")
	if err := s.validate(key); err != nil {
		return err
	}
	ch := make(chan error, 1)
	if !postMain(func() { backend().SecureStorage().Delete(s.namespace, key, func(err error) { deliver(ch, err) }) }) {
		return errLoopStopped
	}
	return await(ch)
}
