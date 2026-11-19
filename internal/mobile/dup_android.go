package mobile

import (
	"os"
	"runtime/debug"
	"syscall"
)

// dupOutput makes the descriptor of old (standard output or error) write
// to w, for the runtime's own writes (panics) to go there too.
func dupOutput(w, old *os.File) error {
	return syscall.Dup3(int(w.Fd()), int(old.Fd()), 0)
}

// keepCrashOutput has the runtime report a crash with the goroutine's
// stack, which it writes to the system's log itself, tagged Go. Android
// starts apps in secure mode (AT_SECURE, from zygote), for which the
// runtime sets GOTRACEBACK=none: the report would be the panic's message
// alone. SetTraceback raises it back to Go's default.
func keepCrashOutput() { debug.SetTraceback("single") }
