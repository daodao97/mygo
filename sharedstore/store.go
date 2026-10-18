// Package sharedstore coordinates bounded files and versioned JSON snapshots
// shared by an application and its extensions. It has no UI or platform runtime
// dependency. Open an App Group directory using mygo.AppGroupDirectory on iOS.
package sharedstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	ErrLockUnsupported = errors.New("sharedstore: cross-process locking is unavailable on this platform")
	ErrBusy            = errors.New("sharedstore: another writer holds the lock")
	ErrTooLarge        = errors.New("sharedstore: file exceeds size limit")
	ErrIncompatible    = errors.New("sharedstore: incompatible snapshot version")
	ErrExpired         = errors.New("sharedstore: snapshot expired")
)

// Store confines names to one directory. All participants share trust; its
// locks coordinate cooperating processes and are not an authorization system.
// Use a stable lock name for a logical transaction. Never delete a lock file.
// Close only after outstanding operations and lock leases have finished.
type Store struct {
	root     *os.Root
	maxBytes int64
}
type Options struct{ MaxBytes int64 }

func Open(directory string, options Options) (*Store, error) {
	limit := options.MaxBytes
	if limit == 0 {
		limit = 1024 * 1024
	}
	if limit < 1 || limit > 64*1024*1024 {
		return nil, fmt.Errorf("sharedstore: MaxBytes must be between 1 and 64 MiB")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	return &Store{root: root, maxBytes: limit}, nil
}
func (s *Store) Close() error { return s.root.Close() }
func validName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") || strings.HasPrefix(name, ".sharedstore-") {
		return fmt.Errorf("sharedstore: invalid file name")
	}
	return nil
}

// ReadBytes reads a complete bounded generation. Atomic replacement means
// readers do not need to take a writer lock. Missing files return os.ErrNotExist.
func (s *Store) ReadBytes(name string) ([]byte, error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	f, err := s.root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("sharedstore: expected a regular file")
	}
	b, err := io.ReadAll(io.LimitReader(f, s.maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > s.maxBytes {
		return nil, ErrTooLarge
	}
	return b, nil
}

// WriteBytes atomically replaces a complete file (0600). Writers to the same
// name must use TryLock for read/modify/write; simple replacements are last-wins.
// The directory's iOS file-protection policy is inherited by the new file.
func (s *Store) WriteBytes(name string, data []byte) error {
	if err := validName(name); err != nil {
		return err
	}
	if int64(len(data)) > s.maxBytes {
		return ErrTooLarge
	}
	// Root-scoped creation and rename prevent path/symlink escape. Random names
	// avoid collisions between independently running host/extension processes.
	var f *os.File
	var tmp string
	for attempt := 0; attempt < 10; attempt++ {
		tmp = ".sharedstore-" + randomName()
		var err error
		f, err = s.root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return err
		}
		break
	}
	if f == nil {
		return fmt.Errorf("sharedstore: cannot create temporary file")
	}
	defer s.root.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return s.root.Rename(tmp, name)
}

// TryLock is a nonblocking process/file lock. The returned release is idempotent.
// It returns ErrBusy on contention; callers choose retry/backoff policy.
func (s *Store) TryLock(name string) (func(), error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	return s.lockFile(".sharedstore-lock-" + name)
}

// TryLockFile coordinates an existing application's lock-file convention.
// The name must be reserved exclusively for locking and never replaced/deleted.
func (s *Store) TryLockFile(name string) (func(), error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	return s.lockFile(name)
}

func (s *Store) lockFile(name string) (func(), error) {
	f, err := s.root.OpenFile(name, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(func() { unlockFile(f); f.Close() }) }, nil
}

// Snapshot is wire-compatible with MyGoSharedSnapshot<Value> in Swift. Times
// are Unix milliseconds. Version belongs to the application schema, not MyGo.
type Snapshot[T any] struct {
	Version   int    `json:"version"`
	UpdatedAt int64  `json:"updatedAt"`
	ExpiresAt *int64 `json:"expiresAt,omitempty"`
	Value     T      `json:"value"`
}

func Read[T any](s *Store, name string, version int, now time.Time) (Snapshot[T], error) {
	var zero Snapshot[T]
	data, err := s.ReadBytes(name)
	if err != nil {
		return zero, err
	}
	var value Snapshot[T]
	if err := json.Unmarshal(data, &value); err != nil {
		return zero, err
	}
	if version < 1 || value.Version != version || value.UpdatedAt <= 0 {
		return zero, ErrIncompatible
	}
	if value.ExpiresAt != nil && *value.ExpiresAt <= now.UnixMilli() {
		return zero, ErrExpired
	}
	return value, nil
}
func Write[T any](s *Store, name string, value Snapshot[T]) error {
	if value.Version < 1 || value.UpdatedAt <= 0 || value.ExpiresAt != nil && *value.ExpiresAt < value.UpdatedAt {
		return ErrIncompatible
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.WriteBytes(name, data)
}

// Migrate atomically changes a versioned snapshot under the same per-file lock
// used by Update. It retains timestamps/expiry. Migration errors leave the old
// generation untouched; an unexpected source version returns ErrIncompatible.
func Migrate(s *Store, name string, from, to int, transform func(json.RawMessage) (json.RawMessage, error)) error {
	if from < 1 || to <= from || transform == nil {
		return ErrIncompatible
	}
	release, err := s.TryLock(name)
	if err != nil {
		return err
	}
	defer release()
	data, err := s.ReadBytes(name)
	if err != nil {
		return err
	}
	var snap Snapshot[json.RawMessage]
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	if snap.Version != from {
		return ErrIncompatible
	}
	snap.Value, err = transform(snap.Value)
	if err != nil {
		return err
	}
	snap.Version = to
	return Write(s, name, snap)
}

// Update serializes read/modify/write across processes. A missing file is passed
// as exists=false; incompatible/corrupt/expired snapshots return their error.
func Update[T any](s *Store, name string, version int, now time.Time, change func(previous Snapshot[T], exists bool) (Snapshot[T], error)) error {
	if change == nil {
		return fmt.Errorf("sharedstore: missing update callback")
	}
	release, err := s.TryLock(name)
	if err != nil {
		return err
	}
	defer release()
	previous, err := Read[T](s, name, version, now)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	next, err := change(previous, exists)
	if err != nil {
		return err
	}
	if next.Version != version {
		return ErrIncompatible
	}
	return Write(s, name, next)
}
