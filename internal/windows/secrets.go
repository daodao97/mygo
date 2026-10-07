//go:build windows && (amd64 || arm64)

package windows

import "github.com/egoist/mygo/internal/platform"

func (*Backend) SecureStorage() platform.SecureStorage { return platform.UnsupportedSecureStorage{} }
