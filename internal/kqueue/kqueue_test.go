//go:build darwin

package kqueue

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/wait"
)

func TestKqueueWatch(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	file := filepath.Join(dir, "file")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	notices := make(chan platform.FileWatchNotice, 16)
	w, err := New(func(n platform.FileWatchNotice) { notices <- n })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	add := func(id uint64, path string, subtree bool) error {
		t.Helper()
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		return w.Add(platform.FileWatchTarget{ID: id, Path: path, Directory: info.IsDir(), Info: info, Subtree: subtree})
	}
	if err := add(1, dir, true); err != platform.ErrUnsupported {
		t.Fatalf("subtree: %v", err)
	}
	// A pipe is skipped rather than opened, which would wait for a writer.
	for id, path := range map[uint64]string{1: dir, 2: sub, 3: file, 4: filepath.Join(dir, "pipe")} {
		if err := add(id, path, false); err != nil {
			t.Fatal(err)
		}
	}
	next := func(want platform.FileWatchNotice) {
		t.Helper()
		for {
			n := wait.For(t, "notice", notices)
			if n == want {
				return
			}
			if n.Err != nil || n.Kind == platform.FileWatchLost {
				t.Fatalf("notice %+v, want %+v", n, want)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(sub, "new"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	next(platform.FileWatchNotice{Target: 2, Kind: platform.FileWatchDirty})
	if err := os.WriteFile(file, []byte("written"), 0600); err != nil {
		t.Fatal(err)
	}
	next(platform.FileWatchNotice{Target: 3, Kind: platform.FileWatchWrite})
	// A file replaced: its watch is lost, and its directory changed.
	if err := os.Rename(filepath.Join(sub, "new"), file); err != nil {
		t.Fatal(err)
	}
	next(platform.FileWatchNotice{Target: 3, Kind: platform.FileWatchLost})
	// The lost target is gone: its ID can be enrolled again.
	if err := add(3, file, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("again"), 0600); err != nil {
		t.Fatal(err)
	}
	next(platform.FileWatchNotice{Target: 3, Kind: platform.FileWatchWrite})
	// A target removed reports nothing more: the Add after it waits
	// until the driver has taken it off.
	w.Remove(2)
	if err := add(6, dir, false); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(sub); err != nil {
		t.Fatal(err)
	}
	next(platform.FileWatchNotice{Target: 1, Kind: platform.FileWatchDirty})
	// A replacement racing the enrollment is not watched in its place.
	info, err := os.Lstat(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := w.Add(platform.FileWatchTarget{ID: 5, Path: file, Info: info}); err == nil {
		t.Fatal("watched a replacement")
	}
}
