// Package generationcache atomically publishes bounded sets of immutable files.
// Readers and writers cooperate through a process lock; corrupt current sets
// fall back to the preceding verified generation. It has no UI dependency.
package generationcache

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/egoist/mygo/sharedstore"
	"io"
	"os"
	"strings"
	"time"
)

var ErrBusy = sharedstore.ErrBusy
var ErrCorrupt = errors.New("generationcache: incomplete or corrupt generation")

type Options struct {
	MaxFileBytes, MaxTotalBytes int64
	MaxFiles                    int
}

// Manifest preserves the pointer format used by early adopters of this cache.
type Manifest struct {
	Directory string
	UpdatedAt int64
	Revision  string
	Files     map[string]string
}
type Cache struct {
	root    *os.Root
	store   *sharedstore.Store
	options Options
}

func Open(directory string, o Options) (*Cache, error) {
	if o.MaxFileBytes == 0 {
		o.MaxFileBytes = 16 << 20
	}
	if o.MaxTotalBytes == 0 {
		o.MaxTotalBytes = 64 << 20
	}
	if o.MaxFiles == 0 {
		o.MaxFiles = 64
	}
	if o.MaxFileBytes < 1 || o.MaxFileBytes > 64<<20 || o.MaxTotalBytes < o.MaxFileBytes || o.MaxTotalBytes > 256<<20 || o.MaxFiles < 1 || o.MaxFiles > 1024 {
		return nil, fmt.Errorf("generationcache: invalid limits")
	}
	s, err := sharedstore.Open(directory, sharedstore.Options{MaxBytes: 1 << 20})
	if err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(directory)
	if err != nil {
		s.Close()
		return nil, err
	}
	return &Cache{r, s, o}, nil
}

// Close only after all operations finish.
func (c *Cache) Close() error { a := c.root.Close(); b := c.store.Close(); return errors.Join(a, b) }
func validName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\\\x00")
}
func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func (c *Cache) read(pointer string) (Manifest, map[string][]byte, error) {
	var m Manifest
	b, e := c.store.ReadBytes(pointer)
	if e != nil {
		return m, nil, e
	}
	if json.Unmarshal(b, &m) != nil || !strings.HasPrefix(m.Directory, "generation-") || !validName(m.Directory) || len(m.Files) == 0 || len(m.Files) > c.options.MaxFiles {
		return m, nil, ErrCorrupt
	}
	r, e := c.root.OpenRoot(m.Directory)
	if e != nil {
		return m, nil, e
	}
	defer r.Close()
	out := make(map[string][]byte, len(m.Files))
	var total int64
	for name, hash := range m.Files {
		if !validName(name) || len(hash) != 64 {
			return m, nil, ErrCorrupt
		}
		f, e := r.Open(name)
		if e != nil {
			return m, nil, e
		}
		info, e := f.Stat()
		if e != nil || !info.Mode().IsRegular() {
			f.Close()
			return m, nil, ErrCorrupt
		}
		data, e := io.ReadAll(io.LimitReader(f, c.options.MaxFileBytes+1))
		f.Close()
		total += int64(len(data))
		if e != nil {
			return m, nil, e
		}
		if int64(len(data)) > c.options.MaxFileBytes || total > c.options.MaxTotalBytes || digest(data) != hash {
			return m, nil, ErrCorrupt
		}
		out[name] = data
	}
	return m, out, nil
}

// Load never returns a mix of generations. ErrBusy is nonblocking; the caller
// chooses retry policy. Fallback is used only if the complete current set fails.
func (c *Cache) Load() (Manifest, map[string][]byte, error) {
	release, e := c.store.TryLockFile(".generation.lock")
	if e != nil {
		return Manifest{}, nil, e
	}
	defer release()
	m, b, e := c.read("current.json")
	if e == nil {
		return m, b, nil
	}
	p, pb, pe := c.read("previous.json")
	if pe == nil {
		return p, pb, nil
	}
	return Manifest{}, nil, errors.Join(e, pe)
}

// Commit validates a private copy before changing the pointer. Validate is an
// optional application validator. Cancellation before publication leaves the
// prior set usable; cancellation after publication does not undo a commit.
func (c *Cache) Commit(ctx context.Context, revision string, now time.Time, files map[string][]byte, validate func(map[string][]byte) error) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if len(files) == 0 || len(files) > c.options.MaxFiles {
		return ErrCorrupt
	}
	copyFiles := make(map[string][]byte, len(files))
	var total int64
	for name, data := range files {
		total += int64(len(data))
		if !validName(name) || int64(len(data)) > c.options.MaxFileBytes || total > c.options.MaxTotalBytes {
			return ErrCorrupt
		}
		copyFiles[name] = append([]byte(nil), data...)
	}
	if validate != nil {
		if e := validate(copyFiles); e != nil {
			return e
		}
	}
	// Recheck limits after the validator, which must not introduce unbounded data.
	total = 0
	for name, data := range copyFiles {
		total += int64(len(data))
		if !validName(name) || int64(len(data)) > c.options.MaxFileBytes || total > c.options.MaxTotalBytes {
			return ErrCorrupt
		}
	}
	if len(copyFiles) == 0 || len(copyFiles) > c.options.MaxFiles {
		return ErrCorrupt
	}
	release, e := c.store.TryLockFile(".generation.lock")
	if e != nil {
		return e
	}
	defer release()
	previous, _, pe := c.read("current.json")
	if pe != nil {
		previous, _, pe = c.read("previous.json")
	}
	var id [16]byte
	if _, e = rand.Read(id[:]); e != nil {
		return e
	}
	dir := "generation-" + hex.EncodeToString(id[:])
	if e = c.root.Mkdir(dir, 0700); e != nil {
		return e
	}
	committed := false
	defer func() {
		if !committed {
			_ = c.root.RemoveAll(dir)
		}
	}()
	r, e := c.root.OpenRoot(dir)
	if e != nil {
		return e
	}
	defer r.Close()
	m := Manifest{dir, now.Unix(), revision, make(map[string]string, len(copyFiles))}
	for name, data := range copyFiles {
		if e = ctx.Err(); e != nil {
			return e
		}
		f, e := r.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, we := f.Write(data)
		se := f.Sync()
		ce := f.Close()
		if e = errors.Join(we, se, ce); e != nil {
			return e
		}
		m.Files[name] = digest(data)
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if pe == nil {
		b, e := json.Marshal(previous)
		if e != nil {
			return e
		}
		if e = c.store.WriteBytes("previous.json", b); e != nil {
			return e
		}
	}
	b, e := json.Marshal(m)
	if e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if e = c.store.WriteBytes("current.json", b); e != nil {
		return e
	}
	committed = true
	// Only framework-owned generations are collected. The lock covers readers too.
	base, e := c.root.Open(".")
	if e == nil {
		entries, _ := base.ReadDir(-1)
		base.Close()
		for _, entry := range entries {
			n := entry.Name()
			if entry.IsDir() && strings.HasPrefix(n, "generation-") && n != dir && (pe != nil || n != previous.Directory) {
				_ = c.root.RemoveAll(n)
			}
		}
	}
	return nil
}
