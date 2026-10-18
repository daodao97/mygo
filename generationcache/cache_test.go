package generationcache

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/egoist/mygo/sharedstore"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestCommitFallbackAndGC(t *testing.T) {
	home := t.TempDir()
	c, e := Open(home, Options{})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	for _, v := range []string{"one", "two", "three"} {
		if e = c.Commit(t.Context(), v, time.Unix(100, 0), map[string][]byte{"a": []byte(v), "b": []byte(v)}, nil); e != nil {
			t.Fatal(e)
		}
	}
	m, b, e := c.Load()
	if e != nil || m.Revision != "three" || string(b["a"]) != "three" {
		t.Fatal(m, b, e)
	}
	entries, _ := os.ReadDir(home)
	n := 0
	for _, entry := range entries {
		if entry.IsDir() {
			n++
		}
	}
	if n != 2 {
		t.Fatal("old generations leaked", n)
	}
	if e = os.WriteFile(filepath.Join(home, m.Directory, "a"), []byte("corrupt"), 0600); e != nil {
		t.Fatal(e)
	}
	m, b, e = c.Load()
	if e != nil || m.Revision != "two" || string(b["b"]) != "two" {
		t.Fatal("fallback failed", m, b, e)
	}
	// A failed validator and canceled operation never replace the good fallback.
	sentinel := errors.New("rejected")
	if e = c.Commit(t.Context(), "bad", time.Now(), map[string][]byte{"a": []byte("bad")}, func(map[string][]byte) error { return sentinel }); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if e = c.Commit(ctx, "bad", time.Now(), map[string][]byte{"a": {}}, nil); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	m, _, e = c.Load()
	if e != nil || m.Revision != "two" {
		t.Fatal(m, e)
	}
}
func TestLegacyManifestLimitsAndConfinement(t *testing.T) {
	home := t.TempDir()
	c, e := Open(home, Options{MaxFileBytes: 8, MaxTotalBytes: 16, MaxFiles: 2})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	// Existing uppercase-field manifests remain readable without a migration.
	dir := "generation-legacy"
	_ = os.Mkdir(filepath.Join(home, dir), 0700)
	_ = os.WriteFile(filepath.Join(home, dir, "a"), []byte("legacy"), 0600)
	meta := Manifest{dir, 123, "old", map[string]string{"a": digest([]byte("legacy"))}}
	data, _ := json.Marshal(meta)
	_ = os.WriteFile(filepath.Join(home, "current.json"), data, 0600)
	if m, b, e := c.Load(); e != nil || m.Revision != "old" || string(b["a"]) != "legacy" {
		t.Fatal(m, b, e)
	}
	for _, files := range []map[string][]byte{{"../escape": {}}, {"a": make([]byte, 9)}, {"a": {}, "b": {}, "c": {}}} {
		if e = c.Commit(t.Context(), "bad", time.Now(), files, nil); e == nil {
			t.Fatal("invalid set accepted")
		}
	}
	meta.Directory = ".."
	data, _ = json.Marshal(meta)
	_ = os.WriteFile(filepath.Join(home, "current.json"), data, 0600)
	if _, _, e = c.Load(); e == nil {
		t.Fatal("path traversal accepted")
	}
}
func TestLocksAndConcurrentReadersSeeCompleteSets(t *testing.T) {
	home := t.TempDir()
	c, e := Open(home, Options{})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	s, _ := sharedstore.Open(home, sharedstore.Options{})
	defer s.Close()
	release, e := s.TryLockFile(".generation.lock")
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = c.Load(); !errors.Is(e, ErrBusy) {
		t.Fatal(e)
	}
	if e = c.Commit(t.Context(), "x", time.Now(), map[string][]byte{"a": {}}, nil); !errors.Is(e, ErrBusy) {
		t.Fatal(e)
	}
	release()
	_ = c.Commit(t.Context(), "v", time.Now(), map[string][]byte{"a": []byte("v"), "b": []byte("v")}, nil)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				m, b, e := c.Load()
				if errors.Is(e, ErrBusy) {
					continue
				}
				if e != nil || string(b["a"]) != m.Revision || string(b["b"]) != m.Revision {
					t.Error("mixed generation", m, e)
				}
			}
		}()
	}
	for i := 0; i < 25; i++ {
		v := string(rune('A' + i))
		e = c.Commit(t.Context(), v, time.Now(), map[string][]byte{"a": []byte(v), "b": []byte(v)}, nil)
		if e != nil && !errors.Is(e, ErrBusy) {
			t.Fatal(e)
		}
	}
	wg.Wait()
}
