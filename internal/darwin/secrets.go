//go:build darwin && !ios

package darwin

import "github.com/egoist/mygo/internal/platform"

func (*Backend) SecureStorage() platform.SecureStorage { return platform.UnsupportedSecureStorage{} }
