//go:build windows && (amd64 || arm64)

package windows

import "github.com/egoist/mygo/internal/platform"

func (*Backend) Sharing() platform.Sharing { return platform.UnsupportedSharing{} }
