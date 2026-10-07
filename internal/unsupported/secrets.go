package unsupported

import "github.com/egoist/mygo/internal/platform"

func (*Backend) SecureStorage() platform.SecureStorage { return platform.UnsupportedSecureStorage{} }
