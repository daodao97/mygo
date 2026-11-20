//go:build windows && (amd64 || arm64)

package windows

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

func TestMain(m *testing.M) {
	if path := os.Getenv("MYGO_TEST_TRASH_ITEM"); path != "" {
		// Helper process for the tests of the recycle bin's size: the
		// shell reads its settings once per process, so TrashItem runs in
		// one started after the test has written them.
		runtime.LockOSThread()
		if r, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded); failed(r) {
			fmt.Fprintln(os.Stderr, hresultError("CoInitializeEx", r))
			os.Exit(1)
		}
		paths := strings.Split(path, string(os.PathListSeparator))
		resize := os.Getenv("MYGO_TEST_TRASH_RESIZE")
		var resizeKey string
		var resizeCapacity uint32
		if len(paths) > 1 && resize != "" {
			capacity, err := strconv.ParseUint(resize, 10, 32)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			key, ok := recycleBinKey(paths[0])
			if !ok {
				fmt.Fprintln(os.Stderr, "cannot find the recycle bin key for", paths[0])
				os.Exit(1)
			}
			resizeKey, resizeCapacity = key, uint32(capacity)
		}
		for i, path := range paths {
			fmt.Println((shell{}).TrashItem(path))
			if i == 0 && resizeKey != "" {
				if err := setRegistryDWORD(resizeKey, "MaxCapacity", resizeCapacity); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
			}
		}
		procCoUninitialize.Call()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// saveRecycleBinSettings registers restoration before the caller writes
// anything. Preserve the types and raw data too, even for non-DWORD values.
func saveRecycleBinSettings(t *testing.T, path string) error {
	t.Helper()
	var key uintptr
	r, _, _ := procRegOpenKeyExW.Call(hkeyCurrentUser, uintptr(unsafe.Pointer(u16(path))), 0, keyQueryValue, uintptr(unsafe.Pointer(&key)))
	missing := syscall.Errno(r) == syscall.ERROR_FILE_NOT_FOUND || syscall.Errno(r) == syscall.ERROR_PATH_NOT_FOUND
	if r != 0 && !missing {
		return fmt.Errorf("RegOpenKeyEx(%s): %w", path, syscall.Errno(r))
	}
	if !missing {
		defer procRegCloseKey.Call(key)
	}
	values := []struct {
		name string
		kind uint32
		data []byte
	}{{name: "NukeOnDelete"}, {name: "MaxCapacity"}}
	if !missing {
		for i := range values {
			v := &values[i]
			var size uint32
			r, _, _ := procRegQueryValueExW.Call(key, uintptr(unsafe.Pointer(u16(v.name))), 0, uintptr(unsafe.Pointer(&v.kind)), 0, uintptr(unsafe.Pointer(&size)))
			if syscall.Errno(r) == syscall.ERROR_FILE_NOT_FOUND {
				continue
			}
			if r != 0 {
				return fmt.Errorf("RegQueryValueEx(%s): %w", v.name, syscall.Errno(r))
			}
			v.data = make([]byte, size)
			if size > 0 {
				r, _, _ = procRegQueryValueExW.Call(key, uintptr(unsafe.Pointer(u16(v.name))), 0, uintptr(unsafe.Pointer(&v.kind)), uintptr(unsafe.Pointer(&v.data[0])), uintptr(unsafe.Pointer(&size)))
				if r != 0 {
					return fmt.Errorf("RegQueryValueEx(%s): %w", v.name, syscall.Errno(r))
				}
				v.data = v.data[:size]
			}
		}
	}
	t.Cleanup(func() {
		if missing {
			if err := regDeleteTree(hkeyCurrentUser, path); err != nil {
				t.Errorf("restore missing recycle bin key: %v", err)
			}
			return
		}
		for _, v := range values {
			var err error
			if v.data != nil {
				err = writeRegistryValue(path, v.name, v.kind, v.data)
			} else {
				err = regDeleteValue(hkeyCurrentUser, path, v.name)
			}
			if err != nil {
				t.Errorf("restore recycle bin value %s: %v", v.name, err)
			}
		}
	})
	return nil
}

func writeRegistryValue(path, name string, kind uint32, data []byte) error {
	var key uintptr
	if r, _, _ := procRegCreateKeyExW.Call(hkeyCurrentUser, uintptr(unsafe.Pointer(u16(path))), 0, 0, 0, keySetValue, 0, uintptr(unsafe.Pointer(&key)), 0); r != 0 {
		return fmt.Errorf("RegCreateKeyEx(%s): %w", path, syscall.Errno(r))
	}
	defer procRegCloseKey.Call(key)
	var p *byte
	if len(data) > 0 {
		p = &data[0]
	}
	if r, _, _ := procRegSetValueExW.Call(key, uintptr(unsafe.Pointer(u16(name))), 0, uintptr(kind), uintptr(unsafe.Pointer(p)), uintptr(len(data))); r != 0 {
		return fmt.Errorf("RegSetValueEx(%s): %w", name, syscall.Errno(r))
	}
	return nil
}

func setRegistryDWORD(path, name string, value uint32) error {
	var data [4]byte
	binary.LittleEndian.PutUint32(data[:], value)
	return writeRegistryValue(path, name, 4 /* REG_DWORD */, data[:])
}

// setRecycleBinCapacity sizes the recycle bin of path's volume, in MB, until
// the test ends.
func setRecycleBinCapacity(t *testing.T, path string, capacity uint32) {
	t.Helper()
	key, ok := recycleBinKey(path)
	if !ok {
		t.Fatalf("cannot find the recycle bin key for %s", path)
	}
	if err := saveRecycleBinSettings(t, key); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]uint32{"NukeOnDelete": 0, "MaxCapacity": capacity} {
		if err := setRegistryDWORD(key, name, value); err != nil {
			t.Fatal(err)
		}
	}
}

// trashItemInHelper runs TrashItem in a helper process (see TestMain) and
// returns what it printed: its error, or <nil>.
func trashItemInHelper(t *testing.T, path string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "MYGO_TEST_TRASH_ITEM="+path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("TrashItem helper for %s: %v\n%s", path, err, out)
	}
	return strings.TrimSpace(string(out))
}

// inRecycleBin reports whether the recycle bin of path's drive holds the
// item deleted from path, by the $I file Windows keeps for each item, which
// names where it was deleted from. Counting the items would not tell: a
// recycle bin fuller than its size drops its oldest items, at a moment of
// its own, while an item is deleted or long after.
func inRecycleBin(t *testing.T, path string) bool {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	// The $I files name the item by its long path, which a temporary
	// directory's (RUNNER~1) may not be.
	dir, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, filepath.Base(path))
	infos, err := filepath.Glob(filepath.Join(filepath.VolumeName(path)+`\`, "$Recycle.Bin", u.Uid, "$I*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, info := range infos {
		// Version 2: the version, the item's size, when it was deleted,
		// and the length of the path, NUL included, before the path.
		b, err := os.ReadFile(info)
		if err != nil || len(b) < 28 || binary.LittleEndian.Uint64(b) != 2 {
			continue
		}
		n := int(binary.LittleEndian.Uint32(b[24:]))
		if len(b) < 28+2*n {
			continue
		}
		name := make([]uint16, n)
		for i := range name {
			name[i] = binary.LittleEndian.Uint16(b[28+2*i:])
		}
		if strings.EqualFold(syscall.UTF16ToString(name), want) {
			return true
		}
	}
	return false
}

func TestTrashItem(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procCoInitializeEx.Call(0, coinitApartmentThreaded)
	dir := t.TempDir()

	file := filepath.Join(dir, "file.txt")
	folder := filepath.Join(dir, "folder")
	os.WriteFile(file, []byte("trash me"), 0o644)
	os.MkdirAll(filepath.Join(folder, "inner"), 0o755)
	os.WriteFile(filepath.Join(folder, "inner", "a.txt"), []byte("a"), 0o644)
	for _, path := range []string{file, folder} {
		if err := (shell{}).TrashItem(path); err != nil {
			t.Fatalf("TrashItem(%s): %v", path, err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s is still there: %v", path, err)
		}
		if !inRecycleBin(t, path) {
			t.Errorf("TrashItem(%s): not in the recycle bin", path)
		}
	}

	if err := (shell{}).TrashItem(filepath.Join(dir, "missing")); err == nil {
		t.Error("TrashItem of a missing file did not fail")
	}

	// A share has no recycle bin: Windows would delete the file for good,
	// so TrashItem fails and leaves it.
	kept := filepath.Join(dir, "kept.txt")
	os.WriteFile(kept, []byte("keep me"), 0o644)
	vol := filepath.VolumeName(kept)
	unc := `\\localhost\` + strings.TrimSuffix(vol, ":") + `$` + strings.TrimPrefix(kept, vol)
	if _, err := os.Stat(unc); err != nil {
		t.Skipf("no administrative share for %s: %v", vol, err)
	}
	if err := (shell{}).TrashItem(unc); err == nil {
		t.Errorf("TrashItem(%s) did not fail", unc)
	} else {
		t.Log(err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("TrashItem(%s) deleted the file: %v", unc, err)
	}
}

func TestTrashItemKeepsAnItemTooBigForTheRecycleBin(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if r, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded); failed(r) {
		t.Fatal(hresultError("CoInitializeEx", r))
	}
	defer procCoUninitialize.Call()
	dir := t.TempDir()
	setRecycleBinCapacity(t, dir, 1)

	file := filepath.Join(dir, "file.bin")
	if err := os.WriteFile(file, make([]byte, 4<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(dir, "folder")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		if err := os.WriteFile(filepath.Join(folder, fmt.Sprintf("%d.bin", i)), make([]byte, 2<<20), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{file, folder} {
		if result := trashItemInHelper(t, path); result == "" || result == "<nil>" {
			t.Errorf("TrashItem(%s) reported %q, want an error for an item bigger than the 1 MiB recycle bin", path, result)
		} else {
			t.Logf("TrashItem(%s): %s", path, result)
		}
		var size int64
		err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				size += info.Size()
			}
			return nil
		})
		if err != nil {
			t.Errorf("TrashItem(%s) did not keep the item at its path: %v", path, err)
		} else if size != 4<<20 {
			t.Errorf("TrashItem(%s) left %d bytes, want %d", path, size, 4<<20)
		}
		if inRecycleBin(t, path) {
			t.Errorf("TrashItem(%s): in the recycle bin, want it kept at its path", path)
		}
	}
}

func TestTrashItemRecyclesAFileAsBigAsTheRecycleBin(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if r, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded); failed(r) {
		t.Fatal(hresultError("CoInitializeEx", r))
	}
	defer procCoUninitialize.Call()
	dir := t.TempDir()
	setRecycleBinCapacity(t, dir, 1)
	path := filepath.Join(dir, "file.bin")
	if err := os.WriteFile(path, make([]byte, 1<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	if result := trashItemInHelper(t, path); result != "<nil>" {
		t.Errorf("TrashItem(%s) reported %q, want <nil> for an item that fits the recycle bin", path, result)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("TrashItem(%s) did not remove the item: %v", path, err)
	}
	if !inRecycleBin(t, path) {
		t.Errorf("TrashItem(%s): not in the recycle bin", path)
	}
}

func TestDefaultRecycleBinCapacity(t *testing.T) {
	for _, tc := range []struct {
		total uint64
		want  uint64 // MiB
	}{
		{0, 0},
		{10 << 30, 1024},
		{40 << 30, 4096},
		{160939618304, 9722},
		{274284392448, 15126},
	} {
		if got := defaultRecycleBinCapacity(tc.total); got != tc.want {
			t.Errorf("defaultRecycleBinCapacity(%d) = %d MiB, want %d", tc.total, got, tc.want)
		}
	}
}

func TestTrashItemFailsWhenWindowsDeletesAFileAllTheSame(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if r, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded); failed(r) {
		t.Fatal(hresultError("CoInitializeEx", r))
	}
	defer procCoUninitialize.Call()
	dir := t.TempDir()
	setRecycleBinCapacity(t, dir, 1)
	folder := filepath.Join(dir, "folder")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		if err := os.WriteFile(filepath.Join(folder, fmt.Sprintf("%d.bin", i)), make([]byte, 2<<20), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(dir, "file.bin")
	if err := os.WriteFile(file, make([]byte, 4<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	// The helper's shell reads the size of 1 MB refusing the folder, which
	// puts nothing in the recycle bin: Windows may empty a recycle bin that
	// holds more than its size. The size then grows behind the shell's back,
	// so the file passes TrashItem's check and the shell deletes it.
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(),
		"MYGO_TEST_TRASH_ITEM="+folder+string(os.PathListSeparator)+file,
		"MYGO_TEST_TRASH_RESIZE=1048576")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("TrashItem helper: %v\n%s", err, out)
	}
	results := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(results) != 2 {
		t.Fatalf("TrashItem helper reported %q, want two results", out)
	}
	if !strings.Contains(results[0], "would be deleted permanently") {
		t.Errorf("TrashItem(%s) reported %q, want a refusal", folder, results[0])
	}
	var size int64
	if err := filepath.Walk(folder, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return err
	}); err != nil || size != 4<<20 {
		t.Errorf("TrashItem(%s) left %d bytes (%v), want %d", folder, size, err, 4<<20)
	}
	if !strings.Contains(results[1], "Windows deleted it permanently") {
		t.Errorf("TrashItem(%s) reported %q, want an error saying Windows deleted it", file, results[1])
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("TrashItem(%s) kept the file (%v), so the shell did not delete it", file, err)
	}
}

func TestTrashSinkNotesOnlyPermanentDeletes(t *testing.T) {
	trashSinkOnce.Do(initTrashSink)
	for _, tc := range []struct {
		name     string
		hr, item uintptr // hrDelete, and whether there is a new item
		deleted  bool
	}{
		{"recycled", 0x00270008, 1, false},
		{"deleted for good", 0x00270008, 0, true},
		{"a folder too big, before its items", copyEngineSNotHandled, 0, false},
		{"refused", eAbort, 0, false},
	} {
		sink := &trashSink{vtbl: &trashSinkVtbl, refs: 1}
		p := uintptr(unsafe.Pointer(sink))
		trashSinks[p] = sink
		// PostDeleteItem(flags, item, hrDelete, newlyCreated), which only
		// compares newlyCreated with nil.
		syscall.SyscallN(trashSinkVtbl[12], p, 0, 0, tc.hr, tc.item)
		delete(trashSinks, p)
		if sink.deleted != tc.deleted {
			t.Errorf("%s: deleted = %t, want %t", tc.name, sink.deleted, tc.deleted)
		}
	}
}
