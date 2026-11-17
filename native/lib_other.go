//go:build !darwin && !linux && !windows

package native

import "errors"

// library is the loaded implementation of a module, which this system
// cannot load.
type library struct{}

func (m *Module) load() (*library, error) { return nil, errors.ErrUnsupported }

func isMainThread() bool { return false }

func (l *library) invoke(m *Module, name string, h uint64, args []byte, call uint64) reply {
	return reply{status: statusError, data: errorBlock(errors.ErrUnsupported)}
}

func (l *library) sym(m *Module, name string) (uintptr, error) { return 0, errors.ErrUnsupported }

func (l *library) direct(fn uintptr, h uint64, args []uintptr) uintptr { return 0 }

func (l *library) directSym(m *Module, name string, h uint64, args []uintptr) uintptr { return 0 }

func freeBlock(uintptr) {}

func (l *library) release(m *Module, h uint64)                              {}
func (l *library) cancel(m *Module, call uint64)                            {}
func (l *library) next(m *Module, call uint64)                              {}
func (l *library) resume(m *Module, cont uint64, status int64, data []byte) {}

// LibraryName is the file of a module's implementation on Linux.
func LibraryName(module string) string { return "lib" + module + ".so" }

func allocate(n int) uintptr { return 0 }
func deallocate(p uintptr)   {}

func releaseSurface(uintptr) {}
