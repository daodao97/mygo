package unsupported

import "github.com/egoist/mygo/internal/platform"

func (*Backend) Permissions() platform.Permissions { return platform.UnsupportedPermissions{} }
