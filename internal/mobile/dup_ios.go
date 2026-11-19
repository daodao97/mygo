package mobile

import (
	"os"
	"runtime/debug"
	"syscall"
)

// dupOutput makes the descriptor of old (standard output or error) write
// to w, for the runtime's own writes (panics) to go there too.
func dupOutput(w, old *os.File) error {
	return syscall.Dup2(int(w.Fd()), int(old.Fd()))
}

// keepCrashOutput has the runtime write the report of a crash to the
// standard error the app started with too, which mygo dev, simctl and
// Xcode show, before redirectOutput takes it.
func keepCrashOutput() {
	fd, err := syscall.Dup(2)
	if err != nil {
		return
	}
	f := os.NewFile(uintptr(fd), "stderr")
	_ = debug.SetCrashOutput(f, debug.CrashOptions{})
	f.Close()
}
