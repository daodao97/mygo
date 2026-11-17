//go:build linux && !android && (amd64 || arm64)

package linux

import "github.com/egoist/mygo/internal/platform"

// The core reaches a window's input region through the interface alone.
var _ platform.InputRegionWindow = (*window)(nil)
