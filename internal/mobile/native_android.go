//go:build android

package mobile

/*
#include <stdint.h>
#include <stdlib.h>

char *mygo_native_init(const char *name, const char *cls, uint64_t base);
int mygo_native_invoke(const char *name, const char *sym, uint64_t h, const uint8_t *args, size_t n, uint64_t call, uint8_t **reply, size_t *reply_len);
uint64_t mygo_native_direct(const char *name, const char *sym, uint64_t h, const uint64_t *args, size_t n);
void mygo_native_op(const char *name, const char *op, uint64_t a, int64_t status, const uint8_t *data, size_t n);
*/
import "C"

import (
	"encoding/binary"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo/internal/nativehost"
)

// Native modules' Kotlin, through JNI (jni_android.c): Go calls it on the
// calling thread, which joins the JVM the first time.
func init() {
	nativehost.Kotlin = &nativehost.KotlinBridge{
		Init: func(name, class string, idBase uint64) string {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			msg := C.mygo_native_init(cstr(name), cstr(class), C.uint64_t(idBase))
			if msg == nil {
				return ""
			}
			defer C.free(unsafe.Pointer(msg))
			return C.GoString(msg)
		},
		Invoke: func(name, sym string, h uint64, args []byte, call uint64) []byte {
			var reply *C.uint8_t
			var n C.size_t
			ok := C.mygo_native_invoke(cstr(name), cstr(sym), C.uint64_t(h), bytePtr(args), C.size_t(len(args)), C.uint64_t(call), &reply, &n)
			if ok == 0 || reply == nil {
				return nil
			}
			defer C.free(unsafe.Pointer(reply))
			return C.GoBytes(unsafe.Pointer(reply), C.int(n))
		},
		Direct: func(name, sym string, h uint64, args []uint64) uint64 {
			var p *C.uint64_t
			if len(args) > 0 {
				p = (*C.uint64_t)(unsafe.Pointer(&args[0]))
			}
			return uint64(C.mygo_native_direct(cstr(name), cstr(sym), C.uint64_t(h), p, C.size_t(len(args))))
		},
		Op: func(name, op string, a uint64, status int64, data []byte) {
			C.mygo_native_op(cstr(name), cstr(op), C.uint64_t(a), C.int64_t(status), bytePtr(data), C.size_t(len(data)))
		},
		Malloc: func(n int) uintptr { return uintptr(C.malloc(C.size_t(n))) },
		Free:   func(p uintptr) { C.free(unsafe.Add(nil, p)) },
	}
}

// cstrs are the C strings of the names of modules and their functions,
// made once each.
var cstrs sync.Map // string → *C.char

func cstr(s string) *C.char {
	if p, ok := cstrs.Load(s); ok {
		return p.(*C.char)
	}
	p, _ := cstrs.LoadOrStore(s, C.CString(s))
	return p.(*C.char)
}

func bytePtr(b []byte) *C.uint8_t {
	if len(b) == 0 {
		return nil
	}
	return (*C.uint8_t)(unsafe.Pointer(unsafe.SliceData(b)))
}

// mygo_native_handle is what Kotlin tells Go (dev.mygo.Native.send). The
// answer of a question, which the caller frees, is its status and then
// its data.
//
//export mygo_native_handle
func mygo_native_handle(kind, id C.uint64_t, status C.int64_t, data *C.uint8_t, n C.size_t, reply **C.uint8_t, replyLen *C.size_t) C.int {
	*reply, *replyLen = nil, 0
	h := nativehost.Handle
	if h == nil {
		return 0
	}
	var b []byte
	if data != nil {
		b = C.GoBytes(unsafe.Pointer(data), C.int(n))
	}
	st, out, ok := h(uint64(kind), uint64(id), int64(status), b)
	if !ok {
		return 0
	}
	p := C.malloc(C.size_t(8 + len(out)))
	r := unsafe.Slice((*byte)(p), 8+len(out))
	binary.LittleEndian.PutUint64(r, uint64(st))
	copy(r[8:], out)
	*reply, *replyLen = (*C.uint8_t)(p), C.size_t(len(r))
	return 1
}

// onUIThread reports whether this is the UI thread, the process's main
// thread on Android.
func onUIThread() bool { return syscall.Gettid() == syscall.Getpid() }
