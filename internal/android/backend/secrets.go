//go:build android && cgo

package backend

import "github.com/egoist/mygo/internal/platform"

// Secrets keeps secrets in the Keystore-backed store of the platform bridge.
// Secrets calls it off the main thread, so it calls the bridge directly.
func (b *Backend) Secrets() platform.Secrets { return androidSecrets{} }

type androidSecrets struct{}

func (androidSecrets) SetSecret(service, account string, secret []byte) error {
	bridge := platformBridge()
	if bridge == nil {
		return ErrPlatformBridge
	}
	return bridge.SecretSet(service, account, secret, "")
}

func (androidSecrets) Secret(service, account string) ([]byte, error) {
	bridge := platformBridge()
	if bridge == nil {
		return nil, ErrPlatformBridge
	}
	return bridge.SecretGet(service, account)
}

func (androidSecrets) DeleteSecret(service, account string) error {
	bridge := platformBridge()
	if bridge == nil {
		return ErrPlatformBridge
	}
	return bridge.SecretDelete(service, account)
}
