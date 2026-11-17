//go:build !android

package pty

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
	"unsafe"
)

func openMaster() (*os.File, string, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", fmt.Errorf("pty: %w", err)
	}
	var unlock int32
	if err := ioctl(master, syscall.TIOCSPTLCK, unsafe.Pointer(&unlock)); err != nil {
		master.Close()
		return nil, "", fmt.Errorf("pty: unlocking: %w", err)
	}
	var n uint32
	if err := ioctl(master, syscall.TIOCGPTN, unsafe.Pointer(&n)); err != nil {
		master.Close()
		return nil, "", fmt.Errorf("pty: %w", err)
	}
	return master, "/dev/pts/" + strconv.Itoa(int(n)), nil
}

// waitReadable waits until the master has output, for a read that came back
// with EAGAIN because the os package could not make the descriptor one its
// poller waits on.
func (p *PTY) waitReadable() error {
	fd := int(p.master.Fd())
	var r syscall.FdSet
	r.Bits[fd/64] |= 1 << (uint(fd) % 64)
	for {
		if _, err := syscall.Select(fd+1, &r, nil, nil, nil); err != syscall.EINTR {
			return err
		}
	}
}
