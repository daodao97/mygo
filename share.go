package mygo

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/internal/platform"
)

// ShareModule presents the system's sharing services.
type ShareModule struct{}

var Share ShareModule

// ShareOptions supplies text, a URL and/or existing files to share. At least
// one item is required. Files are copied by the selected system activity as
// needed; keep them alive until Show returns. Parent anchors the iPad popover.
type ShareOptions struct {
	Parent *Window
	Text   string
	URL    string
	Files  []string
}

// ShareResult reports cancellation or completion of a system activity.
// Activity is the system's identifier, e.g. com.apple.UIKit.activity.CopyToPasteboard.
type ShareResult struct {
	Completed bool
	Activity  string
}

// Show waits for completion or cancellation, pumping native events when
// called on the main thread. Currently implemented on iOS; other backends
// return ErrUnsupported.
func (ShareModule) Show(o ShareOptions) (ShareResult, error) {
	needsApp("Share.Show")
	if o.Text == "" && o.URL == "" && len(o.Files) == 0 {
		return ShareResult{}, fmt.Errorf("mygo: sharing requires at least one item")
	}
	if o.URL != "" {
		u, err := url.Parse(o.URL)
		if err != nil || u.Scheme == "" || u.Scheme == "file" {
			return ShareResult{}, fmt.Errorf("mygo: sharing URL must have a non-file scheme")
		}
	}
	files := make([]string, len(o.Files))
	for i, path := range o.Files {
		abs, err := filepath.Abs(path)
		if err != nil {
			return ShareResult{}, err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return ShareResult{}, err
		}
		if !info.Mode().IsRegular() {
			return ShareResult{}, fmt.Errorf("mygo: sharing requires a regular file: %s", path)
		}
		files[i] = abs
	}
	type result struct {
		value ShareResult
		err   error
	}
	ch := make(chan result, 1)
	if !postMain(func() {
		backend().Sharing().Show(o.Parent.nativeOrNil(), platform.ShareOptions{Text: o.Text, URL: o.URL, Files: files},
			func(r platform.ShareResult, err error) { deliver(ch, result{ShareResult(r), err}) })
	}) {
		return ShareResult{}, errLoopStopped
	}
	r := await(ch)
	return r.value, r.err
}
