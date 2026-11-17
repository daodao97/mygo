//go:build !ios

package pty

import (
	"bytes"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// The ioctls of grantpt, unlockpt and ptsname on macOS (sys/ttycom.h).
const (
	tiocptygrant = 0x20007454 // _IO('t', 84)
	tiocptyunlk  = 0x20007452 // _IO('t', 82)
	tiocptygname = 0x40807453 // _IOC(IOC_OUT, 't', 83, 128)
)

func openMaster() (*os.File, string, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", fmt.Errorf("pty: %w", err)
	}
	var name [128]byte
	for _, step := range []struct {
		req uintptr
		arg unsafe.Pointer
	}{{tiocptygrant, nil}, {tiocptyunlk, nil}, {tiocptygname, unsafe.Pointer(&name[0])}} {
		if err := ioctl(master, step.req, step.arg); err != nil {
			master.Close()
			return nil, "", fmt.Errorf("pty: %w", err)
		}
	}
	n := bytes.IndexByte(name[:], 0)
	if n < 0 {
		n = len(name)
	}
	return master, string(name[:n]), nil
}

// waitReadable waits until the master has output, for a read that came back
// with EAGAIN because the os package could not make the descriptor one its
// poller waits on.
func (p *PTY) waitReadable() error {
	fd := int(p.master.Fd())
	var r syscall.FdSet
	r.Bits[fd/32] |= 1 << (uint(fd) % 32)
	for {
		if err := syscall.Select(fd+1, &r, nil, nil, nil); err != syscall.EINTR {
			return err
		}
	}
}
