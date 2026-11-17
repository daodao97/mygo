//go:build ios

package mobile

/*
#include <pthread.h>
#include <stdlib.h>
#include "ios/MyGo.h"

static mygo_host host;

static void mygo_set_host(mygo_host *h) { host = *h; }
static void mygo_host_call(const uint8_t *msg, size_t len, uint8_t **reply, size_t *reply_len) {
	host.call(msg, len, reply, reply_len);
}
static void mygo_host_post(void) { host.post(); }
static void mygo_host_log(int error, const uint8_t *line, size_t len) { host.log(error, line, len); }
*/
import "C"

import "unsafe"

// cHost is the Swift host, through the functions it gave mygo_start.
type cHost struct{}

func (cHost) call(msg []byte) []byte {
	var reply *C.uint8_t
	var n C.size_t
	C.mygo_host_call((*C.uint8_t)(unsafe.Pointer(unsafe.SliceData(msg))), C.size_t(len(msg)), &reply, &n)
	if reply == nil {
		return nil
	}
	b := C.GoBytes(unsafe.Pointer(reply), C.int(n))
	C.free(unsafe.Pointer(reply))
	return b
}

// callHere calls the host on this thread, for what needs no UI thread.
func (h cHost) callHere(msg []byte) []byte { return h.call(msg) }

func (cHost) post() { C.mygo_host_post() }

func hostLog(isErr bool, line []byte) {
	e := C.int(0)
	if isErr {
		e = 1
	}
	C.mygo_host_log(e, (*C.uint8_t)(unsafe.Pointer(unsafe.SliceData(line))), C.size_t(len(line)))
}

//export mygo_start
func mygo_start(h *C.mygo_host) {
	C.mygo_set_host(h)
	theHost = cHost{}
	start()
}

//export mygo_event
func mygo_event(msg *C.uint8_t, n C.size_t, onMain C.int, reply **C.uint8_t, replyLen *C.size_t) {
	r := deliver(unsafe.Slice((*byte)(unsafe.Pointer(msg)), int(n)), onMain != 0)
	*reply, *replyLen = nil, 0
	if r != nil {
		p := C.malloc(C.size_t(max(len(r), 1)))
		copy(unsafe.Slice((*byte)(p), len(r)), r)
		*reply, *replyLen = (*C.uint8_t)(p), C.size_t(len(r))
	}
}

//export mygo_drain
func mygo_drain() { drain() }

//export mygo_free
func mygo_free(p *C.uint8_t) { C.free(unsafe.Pointer(p)) }

// onUIThread reports whether this is the UI thread, the process's main
// thread on iOS.
func onUIThread() bool { return C.pthread_main_np() != 0 }
