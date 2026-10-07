package unsupported

import "github.com/egoist/mygo/internal/platform"

func (*Backend) Sharing() platform.Sharing { return platform.UnsupportedSharing{} }
