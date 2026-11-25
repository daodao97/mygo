//go:build linux && !android && (amd64 || arm64)

package linux

import (
	"os"
	"path/filepath"
	"testing"
)

// The AppImage a program runs from is the one whose mount it is in: one
// that another AppImage's runtime named in the environment it inherited is
// not taken for its own.
func TestAppImageOf(t *testing.T) {
	tmp := t.TempDir()
	img := filepath.Join(tmp, "App.AppImage")
	os.WriteFile(img, []byte("appimage"), 0o755)
	mount := filepath.Join(tmp, ".mount_App")
	os.MkdirAll(filepath.Join(mount, "usr", "bin"), 0o755)
	inside := filepath.Join(mount, "usr", "bin", "app")
	other := filepath.Join(tmp, "elsewhere", "app")
	for _, c := range []struct {
		name          string
		img, dir, exe string
		want          string
	}{
		{"its own", img, mount, inside, img},
		{"inherited from another", img, mount, other, ""},
		{"the mount itself", img, mount, mount, ""},
		{"none", "", "", inside, ""},
		{"relative", "App.AppImage", mount, inside, ""},
		{"not a file", tmp, mount, inside, ""},
		{"a mount that is gone", img, filepath.Join(tmp, "gone"), inside, ""},
	} {
		if got := appImageOf(c.img, c.dir, c.exe); got != c.want {
			t.Errorf("%s: appImageOf(%q, %q, %q) = %q, want %q", c.name, c.img, c.dir, c.exe, got, c.want)
		}
	}
}
