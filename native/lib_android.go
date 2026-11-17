//go:build android

package native

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/egoist/mygo/internal/nativehost"
)

// On Android a module is implemented in Kotlin, which Go calls through
// JNI (nativehost.Kotlin, which the host's Go side sets): the generated
// Kotlin object of the module decodes the arguments, calls the
// implementation and encodes its answer, as a Swift library's C functions
// do elsewhere. Kotlin tells Go what happens later through one native
// method, which reaches handle as purego's callback does on desktops.

// lastModule numbers the modules as they load.
var lastModule atomic.Uint64

// library is a module's Kotlin, by the name it registered under.
type library struct{ name string }

func init() {
	nativehost.Handle = func(kind, id uint64, status int64, data []byte) (int64, []byte, bool) {
		st, out, ok := handle(kind, id, status, data)
		if !ok {
			return 0, nil, false
		}
		b := append([]byte(nil), out.bytes()...)
		out.release()
		return st, b, true
	}
}

func (m *Module) load() (*library, error) {
	m.once.Do(func() { m.lib, m.err = m.open() })
	return m.lib, m.err
}

func (m *Module) open() (*library, error) {
	k := nativehost.Kotlin
	if !m.supported() {
		return nil, errors.ErrUnsupported
	}
	if k == nil {
		return nil, errors.New("native: the app's host has no native modules: build it with mygo build -platform android")
	}
	for _, name := range m.Deps {
		dep := registered(name)
		if dep == nil {
			return nil, fmt.Errorf("native: %s uses the native module %s, which the program does not link", m.Name, name)
		}
		if _, err := dep.load(); err != nil {
			return nil, err
		}
	}
	idx := lastModule.Add(1)
	modules.Store(idx, m)
	if msg := k.Init(m.Name, m.Kotlin, idx<<contBits); msg != "" {
		modules.Delete(idx)
		return nil, fmt.Errorf("native: %s", msg)
	}
	return &library{name: m.Name}, nil
}

// isMainThread reports whether this is Go's main thread, which on phones
// is not the UI thread: main-thread methods run on the UI thread (onMain).
func isMainThread() bool { return false }

// LibraryName is the name of a module's implementation on Android: its
// Gradle library in the app.
func LibraryName(module string) string { return module }

func (l *library) sym(m *Module, name string) (uintptr, error) { return 1, nil }

func (l *library) invoke(m *Module, name string, h uint64, args []byte, call uint64) reply {
	b := nativehost.Kotlin.Invoke(l.name, name, h, args, call)
	if len(b) < 8 {
		return reply{status: statusOK}
	}
	return reply{status: int64(binary.LittleEndian.Uint64(b)), data: b[8:]}
}

// direct calls a method of numbers by its name.
func (l *library) directSym(m *Module, name string, h uint64, args []uintptr) uintptr {
	a := make([]uint64, len(args))
	for i, v := range args {
		a[i] = uint64(v)
	}
	return uintptr(nativehost.Kotlin.Direct(l.name, name, h, a))
}

func (l *library) release(m *Module, h uint64) { nativehost.Kotlin.Op(l.name, "release", h, 0, nil) }
func (l *library) cancel(m *Module, call uint64) {
	nativehost.Kotlin.Op(l.name, "cancel", call, 0, nil)
}
func (l *library) next(m *Module, call uint64) { nativehost.Kotlin.Op(l.name, "next", call, 0, nil) }
func (l *library) resume(m *Module, cont uint64, status int64, data []byte) {
	nativehost.Kotlin.Op(l.name, "resume", cont, status, data)
}

// The replies of Kotlin are Go's memory.
func freeBlock(uintptr) {}

func allocate(n int) uintptr {
	if k := nativehost.Kotlin; k != nil {
		return k.Malloc(max(n, 1))
	}
	return 0
}

func deallocate(p uintptr) {
	if k := nativehost.Kotlin; k != nil {
		k.Free(p)
	}
}

// releaseSurface has no surfaces to release: Kotlin implementations take
// no textures.
func releaseSurface(uintptr) {}
