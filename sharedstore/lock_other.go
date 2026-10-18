//go:build !darwin && !ios && !linux && !freebsd && !openbsd && !netbsd && !dragonfly && !windows

package sharedstore

import (
	"os"
)

// ErrLockUnsupported is returned rather than silently providing a process-only
// lock. Reads and atomic replacements remain available on these platforms.

func lockFile(*os.File) error { return ErrLockUnsupported }
func unlockFile(*os.File)     {}
