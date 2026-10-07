//go:build linux && (amd64 || arm64)

package linux

import "github.com/egoist/mygo/internal/platform"

func (*Backend) SecureStorage() platform.SecureStorage { return platform.UnsupportedSecureStorage{} }
