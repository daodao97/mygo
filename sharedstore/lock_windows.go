package sharedstore

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	lockKernel   = syscall.NewLazyDLL("kernel32.dll")
	lockFileEx   = lockKernel.NewProc("LockFileEx")
	unlockFileEx = lockKernel.NewProc("UnlockFileEx")
)

func lockFile(f *os.File) error {
	var overlapped syscall.Overlapped
	ok, _, err := lockFileEx.Call(f.Fd(), 3, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	if ok != 0 {
		return nil
	}
	if err == syscall.Errno(33) {
		return ErrBusy
	}
	return err
}
func unlockFile(f *os.File) {
	var overlapped syscall.Overlapped
	unlockFileEx.Call(f.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
}
