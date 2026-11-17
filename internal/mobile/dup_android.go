package mobile

import (
	"os"
	"syscall"
)

// dupOutput makes the descriptor of old (standard output or error) write
// to w, for the runtime's own writes (panics) to go there too.
func dupOutput(w, old *os.File) error {
	return syscall.Dup3(int(w.Fd()), int(old.Fd()), 0)
}
