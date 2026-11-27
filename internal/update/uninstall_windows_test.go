package update

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

var (
	advapi32Test        = syscall.NewLazyDLL("advapi32.dll")
	procRegCreateKeyExW = advapi32Test.NewProc("RegCreateKeyExW")
	procRegDeleteKeyW   = advapi32Test.NewProc("RegDeleteKeyW")
)

// testUninstallEntry registers an entry with values in Settings > Apps
// for the test, under an identifier of its own, which it returns, and
// removes it when the test ends.
func testUninstallEntry(t *testing.T, values map[string]string) string {
	t.Helper()
	identifier := fmt.Sprintf("com.mygo.test.uninstall-entry-%d-%s", os.Getpid(), strings.ReplaceAll(t.Name(), "/", "-"))
	path := uninstallKeys + identifier
	var key syscall.Handle
	if r, _, _ := procRegCreateKeyExW.Call(uintptr(syscall.HKEY_CURRENT_USER), uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(path))), 0, 0, 0, syscall.KEY_SET_VALUE, 0, uintptr(unsafe.Pointer(&key)), 0); r != 0 {
		t.Fatalf("creating %s: %v", path, syscall.Errno(r))
	}
	t.Cleanup(func() { deleteUninstallEntry(identifier) })
	defer syscall.RegCloseKey(key)
	for name, value := range values {
		if err := regSetString(key, name, value); err != nil {
			t.Fatal(err)
		}
	}
	return identifier
}

func deleteUninstallEntry(identifier string) {
	procRegDeleteKeyW.Call(uintptr(syscall.HKEY_CURRENT_USER), uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(uninstallKeys+identifier))))
}

// uninstallValue reads a value of the test's entry, "" when it has none.
func uninstallValue(t *testing.T, identifier, name string) string {
	t.Helper()
	var key syscall.Handle
	if err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, syscall.StringToUTF16Ptr(uninstallKeys+identifier), 0, syscall.KEY_QUERY_VALUE, &key); err != nil {
		t.Fatalf("opening the entry: %v", err)
	}
	defer syscall.RegCloseKey(key)
	v, _ := regString(key, name)
	return v
}

func TestRefreshUninstallEntry(t *testing.T) {
	dir := t.TempDir()
	for _, location := range []string{
		dir,
		strings.ToUpper(dir) + `\`,
		strings.ToLower(strings.ReplaceAll(dir, `\`, `/`)),
		`"` + dir + `"`,
		dir + `\sub\..`,
	} {
		t.Run("", func(t *testing.T) {
			id := testUninstallEntry(t, map[string]string{
				"DisplayName":     "My App",
				"DisplayVersion":  "1.0.0",
				"InstallLocation": location,
			})
			if err := RefreshUninstallEntry(dir, id, "1.1.0"); err != nil {
				t.Fatal(err)
			}
			if v := uninstallValue(t, id, "DisplayVersion"); v != "1.1.0" {
				t.Errorf("InstallLocation %q: DisplayVersion is %q, want 1.1.0", location, v)
			}
			if v := uninstallValue(t, id, "DisplayName"); v != "My App" {
				t.Errorf("DisplayName is %q, want it as it was", v)
			}
		})
	}

	// Another install of the app: the user's entry is not this folder's.
	other := t.TempDir()
	id := testUninstallEntry(t, map[string]string{"DisplayVersion": "1.0.0", "InstallLocation": other})
	if err := RefreshUninstallEntry(dir, id, "1.1.0"); err != nil {
		t.Fatal(err)
	}
	if v := uninstallValue(t, id, "DisplayVersion"); v != "1.0.0" {
		t.Errorf("the entry of another folder has DisplayVersion %q, want 1.0.0", v)
	}

	// An entry without InstallLocation.
	t.Run("no location", func(t *testing.T) {
		id := testUninstallEntry(t, map[string]string{"DisplayVersion": "1.0.0"})
		if err := RefreshUninstallEntry(dir, id, "1.1.0"); err != nil {
			t.Fatal(err)
		}
		if v := uninstallValue(t, id, "DisplayVersion"); v != "1.0.0" {
			t.Errorf("an entry without InstallLocation has DisplayVersion %q, want 1.0.0", v)
		}
	})

	// No entry, as for a folder unzipped: none is made.
	missing := fmt.Sprintf("com.mygo.test.uninstall-entry-%d-missing", os.Getpid())
	deleteUninstallEntry(missing)
	if err := RefreshUninstallEntry(dir, missing, "1.1.0"); err != nil {
		t.Fatal(err)
	}
	var key syscall.Handle
	if err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, syscall.StringToUTF16Ptr(uninstallKeys+missing), 0, syscall.KEY_QUERY_VALUE, &key); err == nil {
		syscall.RegCloseKey(key)
		deleteUninstallEntry(missing)
		t.Error("RefreshUninstallEntry made an entry where there was none")
	}

	// Without an identifier, it names no entry.
	if err := RefreshUninstallEntry(dir, "", "1.1.0"); err != nil {
		t.Fatal(err)
	}
}

func TestSameDir(t *testing.T) {
	dir := t.TempDir()
	for _, tt := range []struct {
		location, dir string
		want          bool
	}{
		{`C:\Users\a\AppData\Local\Programs\My App`, `c:\users\A\appdata\local\programs\my app`, true},
		{`C:\Users\a\AppData\Local\Programs\My App\`, `C:\Users\a\AppData\Local\Programs\My App`, true},
		{`C:/Users/a/AppData/Local/Programs/My App`, `C:\Users\a\AppData\Local\Programs\My App`, true},
		{`"C:\Program Files\My App"`, `C:\Program Files\My App`, true},
		{`D:\`, `d:\`, true},
		{`C:\Users\a\AppData\Local\Programs\My App`, `C:\Users\a\AppData\Local\Programs\My App 2`, false},
		{`C:\Users\a\AppData\Local\Programs`, `C:\Users\a\AppData\Local\Programs\My App`, false},
		{``, `C:\Users\a\AppData\Local\Programs\My App`, false},
		{`My App`, `My App`, false},
		{dir, dir, true},
		{dir, filepath.Join(dir, "sub"), false},
	} {
		if got := sameDir(tt.location, tt.dir); got != tt.want {
			t.Errorf("sameDir(%q, %q) = %v, want %v", tt.location, tt.dir, got, tt.want)
		}
	}
}
