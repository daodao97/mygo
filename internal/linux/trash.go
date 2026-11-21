//go:build linux && !android && (amd64 || arm64)

package linux

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// TrashItemResult moves path to the trash and returns where it landed.
// g_file_trash does not report the destination, so it is found by the
// freedesktop.org trash specification, as GIO writes it: the info file
// whose Path names the original, newest first when the same path was
// trashed more than once.
func (shell) TrashItemResult(path string) (string, error) {
	// Before the item is gone: GIO names it in the trash of its volume
	// relative to the volume's top directory, both with their symlinks
	// expanded.
	real := path
	if dir, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		real = filepath.Join(dir, filepath.Base(path))
	}
	dirs := trashDirs(real)
	f := gFileNewForPath(cs(path))
	defer gObjectUnref(f)
	var gerr ptr
	if !gFileTrash(f, 0, &gerr) {
		return "", gErr(gerr)
	}
	if p, ok := findTrashed(path, real, dirs); ok {
		return p, nil
	}
	return "", fmt.Errorf("mygo: trashed %s but cannot find it in the trash", path)
}

// A trashDir is a trash directory, with "info" and "files" in it. top is
// the top directory of the volume it belongs to, which its info files'
// relative paths are relative to; the home trash has none.
type trashDir struct{ dir, top string }

// trashDirs returns the trash directories GIO may put real (a path whose
// parent has its symlinks expanded) in: the home trash, and the ones of
// its volume's top directory, .Trash/<uid> and .Trash-<uid>.
func trashDirs(real string) []trashDir {
	dirs := []trashDir{{dir: filepath.Join(xdgDir("XDG_DATA_HOME", ".local/share"), "Trash")}}
	if top, ok := topDir(filepath.Dir(real)); ok {
		uid := strconv.Itoa(os.Getuid())
		dirs = append(dirs,
			trashDir{filepath.Join(top, ".Trash", uid), top},
			trashDir{filepath.Join(top, ".Trash-"+uid), top})
	}
	return dirs
}

// topDir returns the top directory of dir's volume as GIO finds it: the
// last ancestor on the same device.
func topDir(dir string) (string, bool) {
	var st syscall.Stat_t
	if syscall.Stat(dir, &st) != nil {
		return "", false
	}
	for dir != "/" {
		parent := filepath.Dir(dir)
		var pst syscall.Stat_t
		if syscall.Stat(parent, &pst) != nil {
			return "", false
		}
		if pst.Dev != st.Dev {
			return dir, true
		}
		dir = parent
	}
	return dir, true
}

// findTrashed returns the file in the files directory of one of dirs
// whose info file names path as its original location: path itself when
// absolute, real relative to the volume's top directory otherwise. The
// info file written last wins, so a path trashed twice returns its newest
// item. It reads first the info files GIO names after path's base name,
// then, if none is it, all of them.
func findTrashed(path, real string, dirs []trashDir) (string, bool) {
	base := filepath.Base(path)
	stem, _, _ := strings.Cut(base, ".")
	gioName := func(name string) bool {
		return name == base || strings.HasPrefix(name, stem+".")
	}
	if p, ok := scanTrash(path, real, dirs, gioName); ok {
		return p, ok
	}
	return scanTrash(path, real, dirs, func(string) bool { return true })
}

func scanTrash(path, real string, dirs []trashDir, match func(name string) bool) (string, bool) {
	best := ""
	var bestTime int64
	for _, d := range dirs {
		infos, err := os.ReadDir(filepath.Join(d.dir, "info"))
		if err != nil {
			continue
		}
		for _, e := range infos {
			name, ok := strings.CutSuffix(e.Name(), ".trashinfo")
			if !ok || !match(name) {
				continue
			}
			b, err := os.ReadFile(filepath.Join(d.dir, "info", e.Name()))
			if err != nil {
				continue
			}
			orig, ok := trashInfoPath(b)
			if !ok {
				continue
			}
			if filepath.IsAbs(orig) {
				ok = orig == path
			} else {
				ok = d.top != "" && filepath.Join(d.top, orig) == real
			}
			if !ok {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if when := info.ModTime().UnixNano(); best == "" || when > bestTime {
				best = filepath.Join(d.dir, "files", name)
				bestTime = when
			}
		}
	}
	return best, best != ""
}

// trashInfoPath reads the Path of a .trashinfo file, which the
// specification percent-encodes. GIO writes it absolute in the home
// trash and relative to the volume's top directory in the others.
func trashInfoPath(b []byte) (string, bool) {
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "Path="); ok {
			p, err := url.PathUnescape(v)
			return p, err == nil
		}
	}
	return "", false
}
