package unsupported

import "github.com/egoist/mygo/internal/platform"

func (*Backend) Mobile() platform.Mobile { return platform.UnsupportedMobile{} }
