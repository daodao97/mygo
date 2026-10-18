package sharedstore

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSnapshotExpiryMigrationAndRollback(t *testing.T) {
	s, err := Open(t.TempDir(), Options{MaxBytes: 512})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.UnixMilli(1800000000000)
	expiry := now.Add(time.Minute).UnixMilli()
	if err := Write(s, "status.json", Snapshot[map[string]int]{Version: 1, UpdatedAt: now.UnixMilli(), ExpiresAt: &expiry, Value: map[string]int{"count": 4}}); err != nil {
		t.Fatal(err)
	}
	read, err := Read[map[string]int](s, "status.json", 1, now)
	if err != nil || read.Value["count"] != 4 {
		t.Fatal(read, err)
	}
	if _, err := Read[map[string]int](s, "status.json", 1, now.Add(time.Minute)); !errors.Is(err, ErrExpired) {
		t.Fatal(err)
	}
	before, _ := s.ReadBytes("status.json")
	sentinel := errors.New("migration failed")
	if err := Migrate(s, "status.json", 1, 2, func(json.RawMessage) (json.RawMessage, error) { return nil, sentinel }); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	after, _ := s.ReadBytes("status.json")
	if !bytes.Equal(before, after) {
		t.Fatal("failed migration changed file")
	}
	if err := Migrate(s, "status.json", 1, 2, func(old json.RawMessage) (json.RawMessage, error) { return json.RawMessage(`{"total":4}`), nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := Read[map[string]int](s, "status.json", 1, now); !errors.Is(err, ErrIncompatible) {
		t.Fatal(err)
	}
	updated, err := Read[map[string]int](s, "status.json", 2, now)
	if err != nil || updated.Value["total"] != 4 || updated.UpdatedAt != now.UnixMilli() {
		t.Fatal(updated, err)
	}
	if err := Update(s, "status.json", 2, now, func(p Snapshot[map[string]int], exists bool) (Snapshot[map[string]int], error) {
		p.Value["total"]++
		return p, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestBoundedFilesAndPathConfinement(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, Options{MaxBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, name := range []string{"", ".", "..", "../outside", "a/b", "a\\b", ".sharedstore-swap"} {
		if err := s.WriteBytes(name, []byte("x")); err == nil {
			t.Fatalf("accepted name %q", name)
		}
	}
	if err := s.WriteBytes("data", bytes.Repeat([]byte("x"), 65)); !errors.Is(err, ErrTooLarge) {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data"), bytes.Repeat([]byte("x"), 65), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadBytes("data"); !errors.Is(err, ErrTooLarge) {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if _, err := s.ReadBytes("link"); err == nil {
		t.Fatal("read escaped root")
	}
	if err := s.WriteBytes("link", []byte("safe")); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(outside)
	if string(raw) != "private" {
		t.Fatal("write followed symlink")
	}
	info, _ := os.Stat(filepath.Join(dir, "link"))
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
}

func TestAtomicGenerations(t *testing.T) {
	s, err := Open(t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := bytes.Repeat([]byte("a"), 16384)
	b := bytes.Repeat([]byte("b"), 16384)
	if err := s.WriteBytes("sample", a); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 30 {
			if err := s.WriteBytes("sample", b); err != nil {
				errs <- err
				return
			}
			if err := s.WriteBytes("sample", a); err != nil {
				errs <- err
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range 120 {
			data, err := s.ReadBytes("sample")
			if err != nil {
				errs <- err
				return
			}
			if !bytes.Equal(data, a) && !bytes.Equal(data, b) {
				errs <- errors.New("partial generation")
				return
			}
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestLockAcrossProcesses(t *testing.T) {
	if dir := os.Getenv("MYGO_SHAREDSTORE_CHILD"); dir != "" {
		s, err := Open(dir, Options{})
		if err != nil {
			os.Exit(2)
		}
		defer s.Close()
		release, err := s.TryLock("transaction")
		if release != nil {
			release()
		}
		if errors.Is(err, ErrBusy) {
			return
		}
		os.Exit(3)
	}
	dir := t.TempDir()
	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	release, err := s.TryLock("transaction")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestLockAcrossProcesses$")
	cmd.Env = append(os.Environ(), "MYGO_SHAREDSTORE_CHILD="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		release()
		t.Fatalf("child lock contention: %v %s", err, out)
	}
	release()
	release()
	other, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	next, err := other.TryLock("transaction")
	if err != nil {
		t.Fatal(err)
	}
	next()
}

func TestSwiftSnapshotWireFixture(t *testing.T) {
	s, err := Open(t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	wire, err := os.ReadFile("../native/ios/tests/snapshot-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.WriteBytes("wire.json", wire); err != nil {
		t.Fatal(err)
	}
	got, err := Read[map[string]int](s, "wire.json", 1, time.UnixMilli(1800000000001))
	if err != nil || got.Value["count"] != 4 || got.ExpiresAt == nil || *got.ExpiresAt != 1800000060000 {
		t.Fatal(got, err)
	}
}
