//go:build ios

package mobile

import (
	"github.com/egoist/mygo/internal/kqueue"
	"github.com/egoist/mygo/internal/platform"
)

func (*Backend) NewFileWatch(notify func(platform.FileWatchNotice)) (platform.FileWatch, error) {
	return kqueue.New(notify)
}
