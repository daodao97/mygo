//go:build ios && cgo

package ios

/*
#include "native.h"
void mygo_ios_experience(uint64_t token, const char *kind, const char *json);
void mygo_ios_experience_cancel(uint64_t token, const char *kind);
*/
import "C"

// These methods are invoked by the public API's main-thread dispatcher.
func MobileExperience(kind, value string, done func(string, error)) func() {
	id := systemRequest(nil, func(id C.uint64_t, _ *C.char) {
		cString(kind, func(k *C.char) { cString(value, func(v *C.char) { C.mygo_ios_experience(id, k, v) }) })
	}, done)
	return func() { cString(kind, func(k *C.char) { C.mygo_ios_experience_cancel(C.uint64_t(id), k) }) }
}

var activityListeners = map[uint64]func(string){}
var activityListenerID uint64

func OnLiveActivityToken(fn func(string)) func() {
	activityListenerID++
	id := activityListenerID
	activityListeners[id] = fn
	return func() { delete(activityListeners, id) }
}

//export goIOSLiveActivityEvent
func goIOSLiveActivityEvent(data *C.char) {
	value := C.GoString(data)
	for _, fn := range activityListeners {
		fn(value)
	}
}
