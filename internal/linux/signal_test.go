//go:build linux && !android && (amd64 || arm64)

package linux

import "testing"

// A Signal before Init is done, as from a goroutine posting while the app
// starts, adds no idle source and leaves the backend unsignaled, so that
// the Signal at the end of Init and those after it dispatch (#1770).
func TestSignalBeforeInit(t *testing.T) {
	saved := gIdleAddFull
	defer func() { gIdleAddFull = saved }()
	var added []ptr
	gIdleAddFull = func(_ int32, fn, _, _ ptr) uint32 {
		added = append(added, fn)
		return 1
	}

	b := &Backend{}
	b.Signal()
	if len(added) != 0 || b.signaled.Load() {
		t.Fatalf("before Init: added %v, signaled %v", added, b.signaled.Load())
	}

	// As Init ends.
	b.signalable.Store(true)
	b.Signal()
	if len(added) != 1 || !b.signaled.Load() {
		t.Fatalf("after Init: added %v, signaled %v", added, b.signaled.Load())
	}
	b.Signal()
	if len(added) != 1 {
		t.Fatalf("signaled twice before dispatching: added %v", added)
	}
}
