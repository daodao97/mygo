//go:build (darwin && !ios) || (linux && !android && (amd64 || arm64)) || (windows && (amd64 || arm64))

package vt

import "github.com/ebitengine/purego"

// Supported reports whether the platform can load the library.
const Supported = true

// call calls a C function whose arguments are integers or pointers. Go
// pointers converted in the call, as in uintptr(unsafe.Pointer(&x)), are
// moved to the heap and kept alive until it returns (go:uintptrescapes):
// otherwise stack growth could move x while the library uses it.
//
//go:uintptrescapes
func call(fn uintptr, args ...uintptr) uintptr {
	r, _, _ := purego.SyscallN(fn, args...)
	return r
}

// callv calls fn with args, which the caller keeps and fills: a call of
// discrete arguments builds its variadic slice anew at every call, which
// a read made for every row of every frame need not. As with call, what
// the pointers among args name must outlive it.
func callv(fn uintptr, args []uintptr) uintptr {
	return call(fn, args...)
}

func registerFunc(fptr any, addr uintptr) { purego.RegisterFunc(fptr, addr) }

func newCallback(fn any) uintptr { return purego.NewCallback(fn) }
