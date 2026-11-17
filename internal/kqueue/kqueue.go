//go:build darwin

// Package kqueue watches files with kqueue's vnode events, for iOS, which
// has no FSEvents: a descriptor for every file and directory watched,
// opened for events only, as fsnotify does. A directory's event says
// that its entries changed, not which, so the core lists it again; a
// file's says that it was written.
package kqueue

import (
	"errors"
	"syscall"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/watchdriver"
)

// vnodeEvents are what a target's descriptor reports.
const vnodeEvents = syscall.NOTE_DELETE | syscall.NOTE_WRITE | syscall.NOTE_EXTEND | syscall.NOTE_ATTRIB | syscall.NOTE_LINK | syscall.NOTE_RENAME | syscall.NOTE_REVOKE

// watch is a watch's driver: a kqueue of its own, with a user event that
// wakes Wait, and a descriptor for each target.
type watch struct {
	kq      int
	targets map[uint64]int // descriptors, by ID
	ids     map[int]uint64 // IDs, by descriptor
	dirs    map[int]bool
}

// New starts a watch with a kqueue of its own.
func New(notify func(platform.FileWatchNotice)) (platform.FileWatch, error) {
	kq, err := syscall.Kqueue()
	if err != nil {
		return nil, err
	}
	syscall.CloseOnExec(kq)
	wake := syscall.Kevent_t{Ident: 0, Filter: syscall.EVFILT_USER, Flags: syscall.EV_ADD | syscall.EV_CLEAR}
	if _, err := syscall.Kevent(kq, []syscall.Kevent_t{wake}, nil, nil); err != nil {
		syscall.Close(kq)
		return nil, err
	}
	d := &watch{kq: kq, targets: make(map[uint64]int), ids: make(map[int]uint64), dirs: make(map[int]bool)}
	return watchdriver.New(d, notify), nil
}

func (d *watch) Add(t platform.FileWatchTarget) error {
	if t.Subtree {
		return platform.ErrUnsupported
	}
	if m := t.Info.Mode(); !m.IsDir() && !m.IsRegular() {
		// Links, pipes and devices: their directory names what happens
		// to them, and opening a pipe would wait for its writer.
		return nil
	}
	fd, err := syscall.Open(t.Path, syscall.O_EVTONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil {
		syscall.Close(fd)
		return err
	}
	if seen, ok := t.Info.Sys().(*syscall.Stat_t); !ok || seen.Dev != st.Dev || seen.Ino != st.Ino {
		syscall.Close(fd)
		return errors.New("watch target identity changed")
	}
	ev := syscall.Kevent_t{Ident: uint64(fd), Filter: syscall.EVFILT_VNODE, Flags: syscall.EV_ADD | syscall.EV_CLEAR, Fflags: vnodeEvents}
	if _, err := syscall.Kevent(d.kq, []syscall.Kevent_t{ev}, nil, nil); err != nil {
		syscall.Close(fd)
		return err
	}
	d.targets[t.ID] = fd
	d.ids[fd] = t.ID
	d.dirs[fd] = t.Directory
	return nil
}

func (d *watch) Remove(id uint64) {
	if fd, ok := d.targets[id]; ok {
		d.forget(fd)
	}
}

// forget closes a target's descriptor, which takes its events, queued
// ones included, off the kqueue.
func (d *watch) forget(fd int) {
	delete(d.targets, d.ids[fd])
	delete(d.ids, fd)
	delete(d.dirs, fd)
	syscall.Close(fd)
}

func (d *watch) Wait() ([]platform.FileWatchNotice, error) {
	var events [64]syscall.Kevent_t
	n, err := syscall.Kevent(d.kq, nil, events[:], nil)
	if err == syscall.EINTR {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var notices []platform.FileWatchNotice
	for _, ev := range events[:n] {
		if ev.Filter != syscall.EVFILT_VNODE {
			continue // Wake
		}
		fd := int(ev.Ident)
		id, ok := d.ids[fd]
		if !ok {
			continue
		}
		var kind platform.FileWatchKind
		switch f := ev.Fflags; {
		case f&(syscall.NOTE_DELETE|syscall.NOTE_RENAME|syscall.NOTE_REVOKE) != 0:
			// The descriptor follows the file wherever it went: the core
			// enrolls what is at the path now.
			kind = platform.FileWatchLost
			d.forget(fd)
		case !d.dirs[fd] && f&(syscall.NOTE_WRITE|syscall.NOTE_EXTEND) != 0:
			kind = platform.FileWatchWrite
		}
		// A directory written to gained or lost entries, which listing it
		// again finds; NOTE_ATTRIB is dirty, as metadata alone is not a
		// write.
		notices = append(notices, platform.FileWatchNotice{Target: id, Kind: kind})
	}
	return notices, nil
}

func (d *watch) Wake() {
	ev := syscall.Kevent_t{Ident: 0, Filter: syscall.EVFILT_USER, Fflags: syscall.NOTE_TRIGGER}
	syscall.Kevent(d.kq, []syscall.Kevent_t{ev}, nil, nil)
}

func (d *watch) Close() {
	for fd := range d.ids {
		d.forget(fd)
	}
	syscall.Close(d.kq)
}
