//go:build ios && cgo

package ios

/*
#include <stdlib.h>
char *mygo_ios_app_group(const char *identifier, char **error);
*/
import "C"
import (
	"errors"
	"unsafe"
)

// AppGroupDirectory uses Foundation's thread-safe container lookup. It does
// not need UIApplication or dispatch onto the main thread.
func AppGroupDirectory(identifier string) (string, error) {
	id := C.CString(identifier)
	defer C.free(unsafe.Pointer(id))
	var message *C.char
	path := C.mygo_ios_app_group(id, &message)
	if message != nil {
		defer C.free(unsafe.Pointer(message))
		return "", errors.New(C.GoString(message))
	}
	if path == nil {
		return "", errors.New("mygo: App Group container is unavailable")
	}
	defer C.free(unsafe.Pointer(path))
	return C.GoString(path), nil
}
