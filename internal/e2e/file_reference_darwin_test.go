//go:build darwin

package e2e

import (
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/darwin"
	"github.com/egoist/mygo/ui"
)

func TestContentWindowFileReferenceDrop(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{filepath.Join(dir, "a file.txt"), filepath.Join(dir, "中文's file.txt")}
	for _, path := range paths {
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var frames atomic.Int32
	var zone []string
	w := newWindow(t, mygo.WindowOptions{Title: "File reference drop fixture", Width: 400, Height: 300, Content: ui.View(func(c *ui.Context) {
		frames.Add(1)
		if files := ui.Box(c).Size(200, 100).DroppedFiles(); files != nil {
			zone = files
		}
	})})
	eventually(t, "a frame", func() bool { return frames.Load() > 0 })
	drop := func(x, y float64) (over, dropped bool) {
		mygo.RunOnMain(func() { over, dropped = darwin.TestDropFileReferences(w.NativeHandle(), x, y, paths) })
		return
	}
	if over, dropped := drop(50, 50); !over || !dropped {
		t.Fatalf("file reference drop rejected: over %v, dropped %v", over, dropped)
	}
	var got []string
	eventually(t, "paths in the drop zone", func() bool {
		mygo.RunOnMain(func() { got = slices.Clone(zone) })
		return len(got) > 0
	})
	if !slices.Equal(got, paths) {
		t.Fatalf("file references did not resolve in DroppedFiles: got %q, want %q", got, paths)
	}
	var events atomic.Pointer[mygo.FileDropEvent]
	w.OnFileDrop(func(e *mygo.FileDropEvent) { events.Store(e) })
	if over, dropped := drop(300, 250); !over || !dropped {
		t.Fatalf("file reference listener drop rejected: over %v, dropped %v", over, dropped)
	}
	if e := events.Load(); e == nil || !slices.Equal(e.Paths, paths) {
		t.Fatalf("file references did not resolve in OnFileDrop: %+v", e)
	}
}
