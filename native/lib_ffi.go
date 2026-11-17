//go:build (darwin || linux || windows) && !android

package native

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ffi"
	"github.com/egoist/mygo/internal/nativelib"
)

// lastModule numbers the modules as they load.
var lastModule atomic.Uint64

// library is the loaded implementation of a module: a native library,
// found and loaded as package ffi finds and loads any (see ffi.Library.Path).
type library struct {
	lib  *ffi.Library
	syms sync.Map // name → uintptr
}

var libc struct {
	once     sync.Once
	malloc   uintptr
	free     uintptr
	isMain   uintptr
	callback uintptr
	err      error
}

func loadLibc() error {
	libc.once.Do(func() {
		if libc.malloc, libc.free, libc.isMain, libc.err = openLibc(); libc.err != nil {
			return
		}
		// One callback for every module, created once: purego's
		// callbacks are never freed.
		libc.callback = purego.NewCallback(func(kind, id, status, data, n uintptr) uintptr {
			var in []byte
			if n > 0 {
				in = append([]byte(nil), unsafe.Slice(bytesAt(data), n)...)
			}
			st, out, ok := handle(uint64(kind), uint64(id), int64(status), in)
			if !ok {
				return 0
			}
			p := newBlock(st, out.bytes())
			out.release()
			return p
		})
	})
	return libc.err
}

func isMainThread() bool {
	if loadLibc() != nil {
		return false
	}
	return mainThread()
}

// newBlock copies a result into memory the platform frees: its status, its
// length, then its bytes.
func newBlock(status int64, data []byte) uintptr {
	p, _, _ := purego.SyscallN(libc.malloc, uintptr(16+len(data)))
	if p == 0 {
		return 0
	}
	b := unsafe.Slice(bytesAt(p), 16+len(data))
	binary.LittleEndian.PutUint64(b, uint64(status))
	binary.LittleEndian.PutUint64(b[8:], uint64(len(data)))
	copy(b[16:], data)
	return p
}

// readBlock reads a block the platform made, which the reply frees.
func readBlock(p uintptr) reply {
	if p == 0 {
		return reply{}
	}
	head := unsafe.Slice(bytesAt(p), 16)
	n := binary.LittleEndian.Uint64(head[8:])
	return reply{status: int64(binary.LittleEndian.Uint64(head)), data: bytesOf(p+16, int(n)), block: p}
}

func freeBlock(p uintptr) { purego.SyscallN(libc.free, p) }

func (m *Module) load() (*library, error) {
	m.once.Do(func() { m.lib, m.err = m.open() })
	return m.lib, m.err
}

func (m *Module) open() (*library, error) {
	min := m.MinMacOS
	if runtime.GOOS == "ios" {
		min = m.MinIOS
	}
	if !m.supported() || min != "" && olderMacOS(min) {
		return nil, errors.ErrUnsupported
	}
	if err := loadLibc(); err != nil {
		return nil, err
	}
	// The modules it uses first, whose libraries it links to: dyld then
	// finds them loaded, by their install names, wherever they are.
	for _, name := range m.Deps {
		dep := registered(name)
		if dep == nil {
			return nil, fmt.Errorf("native: %s uses the native module %s, which the program does not link", m.Name, name)
		}
		if _, err := dep.load(); err != nil {
			return nil, err
		}
	}
	fl := ffi.New(m.Name, m.published()).From(m.Dir)
	if err := m.checkFresh(fl); err != nil {
		return nil, err
	}
	if err := fl.Load(); err != nil {
		return nil, fmt.Errorf("native: %w", err)
	}
	lib := &library{lib: fl}
	init, err := lib.sym(m, "init")
	if err != nil {
		return nil, err
	}
	// The module's index makes its continuations' ids unique.
	idx := lastModule.Add(1)
	modules.Store(idx, m)
	// The implementation allocates the blocks it gives Go, and frees those
	// Go gives it, with the C library's malloc and free, as Go does.
	purego.SyscallN(init, libc.callback, uintptr(idx<<contBits), libc.malloc, libc.free)
	return lib, nil
}

// LibraryName is the file of a module's implementation on this system.
func LibraryName(module string) string { return nativelib.FileName(module, runtime.GOOS) }

// published returns the manifest of a module that a dependency of the
// program declares, in the module cache, where the files it publishes are
// built from the sources there: go run and go test then download its
// library, as the CLI does. A module of the program's own is built from its
// sources instead, which may have changed since they were published.
func (m *Module) published() []byte {
	if m.Dir == "" || mygo.App.IsPackaged() || !inModuleCache(m.Dir) {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(m.Dir, nativelib.ManifestFile))
	if err != nil {
		return nil
	}
	if man, err := nativelib.Parse(data); err != nil {
		return nil
	} else if _, ok := man.Lookup(m.Name); !ok {
		return nil
	}
	return data
}

// inModuleCache reports whether dir is in Go's module cache, whose
// directories are named module@version.
func inModuleCache(dir string) bool {
	for _, e := range strings.Split(filepath.ToSlash(dir), "/") {
		if i := strings.LastIndex(e, "@v"); i > 0 && i+2 < len(e) && e[i+2] >= '0' && e[i+2] <= '9' {
			return true
		}
	}
	return false
}

// checkFresh refuses a build of the implementation that mygo dev or
// mygo native build -target host made for go test and go run, in the module's .mygo/native, once the
// Swift sources are newer: the build would not be what they say.
func (m *Module) checkFresh(l *ffi.Library) error {
	if m.Dir == "" || mygo.App.IsPackaged() {
		return nil
	}
	path, err := l.Path()
	if err != nil {
		return fmt.Errorf("native: the implementation of %s is not built: run mygo dev, or mygo native build -target host %s (%w)", m.Name, m.Dir, err)
	}
	if !strings.Contains(filepath.ToSlash(path), "/.mygo/native/") {
		return nil
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	var newer time.Time
	for _, lang := range m.Langs {
		if t := newestSource(filepath.Join(m.Dir, lang)); t.After(newer) {
			newer = t
		}
	}
	if newer.After(st.ModTime()) {
		return fmt.Errorf("native: the implementation of %s is older than its sources: run mygo dev, or mygo native build -target host %s", m.Name, m.Dir)
	}
	return nil
}

// newestSource returns when a source of the implementation in dir last
// changed: its Swift, Rust or Zig, or what builds it.
func newestSource(dir string) time.Time {
	var t time.Time
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if (strings.HasPrefix(d.Name(), ".") || d.Name() == "target" || d.Name() == "zig-out") && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if ext := filepath.Ext(path); ext == ".swift" || ext == ".rs" || ext == ".zig" || ext == ".toml" || ext == ".zon" {
			if info, err := d.Info(); err == nil && info.ModTime().After(t) {
				t = info.ModTime()
			}
		}
		return nil
	})
	return t
}

func (l *library) sym(m *Module, name string) (uintptr, error) {
	if p, ok := l.syms.Load(name); ok {
		return p.(uintptr), nil
	}
	full := "mygo_" + m.Symbol + "_" + name
	p, err := l.lib.Symbol(full)
	if err != nil {
		return 0, fmt.Errorf("native: %s has no %s: rebuild it: run mygo dev, or mygo native build -target host %s", LibraryName(m.Name), full, m.Dir)
	}
	l.syms.Store(name, p)
	return p, nil
}

func (l *library) invoke(m *Module, name string, h uint64, args []byte, call uint64) reply {
	fn, err := l.sym(m, name)
	if err != nil {
		return reply{status: statusError, data: errorBlock(err)}
	}
	p, _, _ := purego.SyscallN(fn, uintptr(h), bytesPointer(args), uintptr(len(args)), uintptr(call))
	runtime.KeepAlive(args)
	if p == 0 {
		return reply{status: statusOK}
	}
	return readBlock(p)
}

func (l *library) simple(m *Module, name string, args ...uintptr) {
	if fn, err := l.sym(m, name); err == nil {
		purego.SyscallN(fn, args...)
	}
}

func (l *library) release(m *Module, h uint64)   { l.simple(m, "release", uintptr(h)) }
func (l *library) cancel(m *Module, call uint64) { l.simple(m, "cancel", uintptr(call)) }
func (l *library) next(m *Module, call uint64)   { l.simple(m, "next", uintptr(call)) }

// resume answers a continuation with a block the platform frees.
func (l *library) resume(m *Module, cont uint64, status int64, data []byte) {
	l.simple(m, "resume", uintptr(cont), newBlock(status, data))
}

// directSym calls the C function of a method of numbers, by its name.
func (l *library) directSym(m *Module, name string, h uint64, args []uintptr) uintptr {
	fn, err := l.sym(m, name)
	if err != nil {
		return 0
	}
	return l.direct(fn, h, args)
}

// direct calls fn, a method of numbers, on the object h.
func (l *library) direct(fn uintptr, h uint64, args []uintptr) uintptr {
	// SyscallN's arguments escape, whatever their size: the fewer, the
	// less it allocates.
	a := make([]uintptr, 1+len(args))
	a[0] = uintptr(h)
	copy(a[1:], args)
	r, _, _ := purego.SyscallN(fn, a...)
	return r
}

func allocate(n int) uintptr {
	if loadLibc() != nil {
		return 0
	}
	p, _, _ := purego.SyscallN(libc.malloc, uintptr(max(n, 1)))
	return p
}

func deallocate(p uintptr) { purego.SyscallN(libc.free, p) }
