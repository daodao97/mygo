//go:build linux && !android && (amd64 || arm64)

package linux

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeTrashInfo puts an item named name in the trash dir, as GIO does,
// with orig as its info file's Path.
func writeTrashInfo(t *testing.T, dir, name, orig string, when time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	info := filepath.Join(dir, "info", name+".trashinfo")
	escaped := (&url.URL{Path: orig}).EscapedPath()
	body := "[Trash Info]\nPath=" + escaped + "\nDeletionDate=" + when.Format("2006-01-02T15:04:05") + "\n"
	if err := os.WriteFile(info, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(info, when, when); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "files", name), []byte("trashed"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindTrashed(t *testing.T) {
	dir := t.TempDir()
	home := trashDir{dir: filepath.Join(dir, "Trash")}
	orig := filepath.Join(dir, "report.pdf")
	now := time.Now()
	writeTrashInfo(t, home.dir, "report.pdf", orig, now)
	writeTrashInfo(t, home.dir, "other.txt", filepath.Join(dir, "other.txt"), now)

	got, ok := findTrashed(orig, orig, []trashDir{home})
	if want := filepath.Join(home.dir, "files", "report.pdf"); !ok || got != want {
		t.Errorf("findTrashed = %q, %v, want %q", got, ok, want)
	}
	if _, ok := findTrashed(filepath.Join(dir, "never-trashed"), filepath.Join(dir, "never-trashed"), []trashDir{home}); ok {
		t.Error("findTrashed found a path that was never trashed")
	}
	if _, ok := findTrashed(orig, orig, []trashDir{{dir: filepath.Join(dir, "no-trash")}}); ok {
		t.Error("findTrashed found something without a trash")
	}

	// The same path trashed twice: the newest info file wins.
	writeTrashInfo(t, home.dir, "report.2.pdf", orig, now.Add(time.Second))
	got, ok = findTrashed(orig, orig, []trashDir{home})
	if want := filepath.Join(home.dir, "files", "report.2.pdf"); !ok || got != want {
		t.Errorf("findTrashed after a second trash = %q, %v, want %q", got, ok, want)
	}

	// A name GIO did not pick (another trash implementation's, or GIO's
	// shortened one) is found by reading every info file.
	other := filepath.Join(dir, "a very long name.txt")
	writeTrashInfo(t, home.dir, "ry long name.txt", other, now)
	got, ok = findTrashed(other, other, []trashDir{home})
	if want := filepath.Join(home.dir, "files", "ry long name.txt"); !ok || got != want {
		t.Errorf("findTrashed of a renamed item = %q, %v, want %q", got, ok, want)
	}
}

// The trash of another volume names items relative to the volume's top
// directory, with symlinks expanded, as GIO writes them.
func TestFindTrashedOnAVolume(t *testing.T) {
	dir := t.TempDir()
	top := filepath.Join(dir, "media", "usb")
	vol := trashDir{dir: filepath.Join(top, ".Trash-1000"), top: top}
	home := trashDir{dir: filepath.Join(dir, "Trash")}
	link := filepath.Join(dir, "docs") // a symlink to top/docs
	path := filepath.Join(link, "plan b.txt")
	real := filepath.Join(top, "docs", "plan b.txt")
	now := time.Now()
	writeTrashInfo(t, vol.dir, "plan b.txt", "docs/plan b.txt", now)
	writeTrashInfo(t, vol.dir, "plan b.2.txt", "other/plan b.txt", now.Add(time.Second))

	got, ok := findTrashed(path, real, []trashDir{home, vol})
	if want := filepath.Join(vol.dir, "files", "plan b.txt"); !ok || got != want {
		t.Errorf("findTrashed = %q, %v, want %q", got, ok, want)
	}
	// A relative path means nothing in the home trash.
	writeTrashInfo(t, home.dir, "plan b.txt", "docs/plan b.txt", now.Add(2*time.Second))
	got, ok = findTrashed(path, real, []trashDir{home, vol})
	if want := filepath.Join(vol.dir, "files", "plan b.txt"); !ok || got != want {
		t.Errorf("findTrashed with a relative path in the home trash = %q, %v, want %q", got, ok, want)
	}
}

func TestTrashInfoPath(t *testing.T) {
	for _, c := range []struct {
		body string
		want string
		ok   bool
	}{
		{"[Trash Info]\nPath=/home/u/a b.txt\nDeletionDate=2026-11-21T10:00:00\n", "/home/u/a b.txt", true},
		{"[Trash Info]\r\nPath=/home/u/caf%C3%A9.txt\r\n", "/home/u/café.txt", true},
		{"[Trash Info]\nPath=/home/u/a%2Fb.txt\n", "/home/u/a/b.txt", true},
		{"[Trash Info]\nPath=docs/a%20b.txt\n", "docs/a b.txt", true},
		{"[Trash Info]\nDeletionDate=2026-11-21T10:00:00\n", "", false},
		{"[Trash Info]\nPath=%zz\n", "", false},
	} {
		got, ok := trashInfoPath([]byte(c.body))
		if got != c.want || ok != c.ok {
			t.Errorf("trashInfoPath(%q) = %q, %v, want %q, %v", c.body, got, ok, c.want, c.ok)
		}
	}
}

func TestTrashDirs(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/data/home")
	dir := t.TempDir()
	dirs := trashDirs(filepath.Join(dir, "file.txt"))
	if dirs[0] != (trashDir{dir: "/data/home/Trash"}) {
		t.Errorf("home trash = %+v, want /data/home/Trash", dirs[0])
	}
	top, ok := topDir(dir)
	if !ok || !strings.HasPrefix(dir+"/", strings.TrimSuffix(top, "/")+"/") {
		t.Fatalf("topDir(%s) = %q, %v", dir, top, ok)
	}
	if len(dirs) != 3 || dirs[1].top != top || dirs[2].top != top {
		t.Errorf("trashDirs = %+v, want the trashes of %s", dirs, top)
	}
}
