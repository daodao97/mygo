package mygo

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/internal/platform"
)

// ExportDialogOptions supplies existing regular files to copy to a location
// chosen by the user. Keep the files alive until Export returns.
type ExportDialogOptions struct {
	Parent *Window
	Title  string
	Files  []string
}

// PhotoDialogOptions selects images using the system photo picker. The zero
// value selects one image; Multiple allows any number of images.
type PhotoDialogOptions struct {
	Parent   *Window
	Multiple bool
}

// Export presents a document export picker and waits for it to finish. It
// returns false, nil on cancellation. Unlike Save, it exports existing files
// rather than returning a future writable path. Currently supported on iOS;
// other backends return ErrUnsupported.
func (DialogModule) Export(o ExportDialogOptions) (bool, error) {
	needsApp("Dialog.Export")
	if len(o.Files) == 0 {
		return false, fmt.Errorf("mygo: export requires at least one file")
	}
	files := make([]string, len(o.Files))
	for i, path := range o.Files {
		abs, err := filepath.Abs(path)
		if err != nil {
			return false, err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return false, err
		}
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf("mygo: export requires a regular file: %s", path)
		}
		files[i] = abs
	}
	type result struct {
		completed bool
		err       error
	}
	ch := make(chan result, 1)
	if !postMain(func() {
		backend().Dialogs().ShowExportDialog(o.Parent.nativeOrNil(), &platform.ExportDialogOptions{Title: o.Title, Files: files},
			func(completed bool, err error) { deliver(ch, result{completed, err}) })
	}) {
		return false, errLoopStopped
	}
	r := await(ch)
	return r.completed, r.err
}

// Photos waits for image selection or cancellation (nil, nil). On iOS it
// uses PHPicker without requesting full photo-library access. Paths refer to
// private cache copies, preserving the selected representation (e.g. HEIC).
// The caller may read them after dismissal and should remove them with
// os.Remove when finished. Copy them to user data for persistent storage;
// iOS can purge caches. Other backends return ErrUnsupported.
func (DialogModule) Photos(o PhotoDialogOptions) ([]string, error) {
	needsApp("Dialog.Photos")
	type result struct {
		paths []string
		err   error
	}
	ch := make(chan result, 1)
	if !postMain(func() {
		backend().Dialogs().ShowPhotoDialog(o.Parent.nativeOrNil(), &platform.PhotoDialogOptions{Multiple: o.Multiple},
			func(paths []string, err error) { deliver(ch, result{paths, err}) })
	}) {
		return nil, errLoopStopped
	}
	r := await(ch)
	return r.paths, r.err
}
