//go:build darwin && !ios

package darwin

import "github.com/egoist/mygo/internal/platform"

func (*Backend) Sharing() platform.Sharing { return platform.UnsupportedSharing{} }
