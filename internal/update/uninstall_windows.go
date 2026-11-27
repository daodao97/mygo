package update

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// Windows apps that the installer of `mygo build` installed have their
// entry in Settings > Apps registered for the user, under
// HKCU\<uninstallKeys><identifier>, with the version it installed
// (DisplayVersion) and the folder it installed it in (InstallLocation).
// RefreshUninstallEntry registers the version again after an update.

const uninstallKeys = `Software\Microsoft\Windows\CurrentVersion\Uninstall\`

// RefreshUninstallEntry sets the version of the entry of the app whose
// identifier is identifier in Settings > Apps to version, when the
// installer registered it: when the user's entry has the app installed in
// dir. Other installs, such as a folder unzipped or an entry of the
// machine's, are left as they are.
func RefreshUninstallEntry(dir, identifier, version string) error {
	if identifier == "" || strings.ContainsAny(identifier, `\/`) || version == "" {
		return nil
	}
	path, err := syscall.UTF16PtrFromString(uninstallKeys + identifier)
	if err != nil {
		return nil
	}
	var key syscall.Handle
	if err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, path, 0, syscall.KEY_QUERY_VALUE|syscall.KEY_SET_VALUE, &key); err != nil {
		if err == syscall.ERROR_FILE_NOT_FOUND {
			return nil
		}
		return err
	}
	defer syscall.RegCloseKey(key)
	if location, ok := regString(key, "InstallLocation"); !ok || !sameDir(location, dir) {
		return nil
	}
	if old, ok := regString(key, "DisplayVersion"); ok && old == version {
		return nil
	}
	return regSetString(key, "DisplayVersion", version)
}

// sameDir reports whether location, an InstallLocation, is the folder dir:
// Windows compares paths without case, and either may go another way to
// the folder, such as a short name or a junction.
func sameDir(location, dir string) bool {
	location = strings.Trim(strings.TrimSpace(location), `"`)
	if !filepath.IsAbs(location) || !filepath.IsAbs(dir) {
		return false
	}
	location, dir = filepath.Clean(location), filepath.Clean(dir)
	return strings.EqualFold(location, dir) || sameFile(location, dir)
}

// regString reads a string value of key, reporting whether it is one.
func regString(key syscall.Handle, name string) (string, bool) {
	n, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return "", false
	}
	var typ, size uint32
	if syscall.RegQueryValueEx(key, n, nil, &typ, nil, &size) != nil || typ != syscall.REG_SZ {
		return "", false
	}
	if size < 2 {
		return "", true
	}
	buf := make([]uint16, (size+1)/2)
	if syscall.RegQueryValueEx(key, n, nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &size) != nil || typ != syscall.REG_SZ {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

// regSetString writes a string value of key.
func regSetString(key syscall.Handle, name, value string) error {
	n, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	v, err := syscall.UTF16FromString(value)
	if err != nil {
		return err
	}
	// Looked up here, not in a package variable: an app that never updates
	// has no init for it.
	set := syscall.NewLazyDLL("advapi32.dll").NewProc("RegSetValueExW") // a KnownDLL
	if r, _, _ := set.Call(uintptr(key), uintptr(unsafe.Pointer(n)), 0, syscall.REG_SZ, uintptr(unsafe.Pointer(&v[0])), uintptr(len(v)*2)); r != 0 {
		return syscall.Errno(r)
	}
	return nil
}
