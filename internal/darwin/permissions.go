//go:build darwin && !ios

package darwin

import "github.com/egoist/mygo/internal/platform"

func (*Backend) Permissions() platform.Permissions { return platform.UnsupportedPermissions{} }
