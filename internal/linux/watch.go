//go:build linux && !android && (amd64 || arm64)

package linux

import (
	"github.com/egoist/mygo/internal/inotify"
	"github.com/egoist/mygo/internal/platform"
)

func (*Backend) NewFileWatch(notify func(platform.FileWatchNotice)) (platform.FileWatch, error) {
	return inotify.New(notify)
}
