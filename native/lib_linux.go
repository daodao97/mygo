//go:build linux && !android

package native

import (
	"os"

	"github.com/ebitengine/purego"
)

// Swift's main actor is the main thread, the process's first. Linux runs a
// module's Swift under go test and go run, and its Rust or Zig anywhere.
const (
	libcPath     = "libc.so.6"
	isMainSymbol = "gettid"
)

func mainThread() bool {
	r, _, _ := purego.SyscallN(libc.isMain)
	return int(r) == os.Getpid()
}

// olderMacOS is false off macOS: a module's minimum is macOS's.
func olderMacOS(string) bool { return false }

// releaseSurface has no surfaces to release off macOS.
func releaseSurface(uintptr) {}
