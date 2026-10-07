//go:build windows && (amd64 || arm64)

package windows

import "github.com/egoist/mygo/internal/platform"

func (*Backend) Permissions() platform.Permissions { return platform.UnsupportedPermissions{} }
