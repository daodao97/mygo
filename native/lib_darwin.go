package native

import (
	"strconv"
	"strings"
	"syscall"

	"github.com/ebitengine/purego"
)

const (
	libcPath     = "/usr/lib/libSystem.B.dylib"
	isMainSymbol = "pthread_main_np"
)

func mainThread() bool {
	r, _, _ := purego.SyscallN(libc.isMain)
	return r != 0
}

// olderMacOS reports whether the system, macOS or iOS, is older than
// version, as "12" or "13.3".
func olderMacOS(version string) bool {
	have, err := syscall.Sysctl("kern.osproductversion")
	if err != nil {
		return false
	}
	return compareVersions(have, version) < 0
}

func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(as), len(bs)) {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// releaseSurface lets go of a surface presented to a texture that is gone.
func releaseSurface(s uintptr) {
	if s == 0 {
		return
	}
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return
	}
	if release, err := purego.Dlsym(cf, "CFRelease"); err == nil {
		purego.SyscallN(release, s)
	}
}
