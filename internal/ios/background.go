//go:build ios && cgo

package ios

/*
#include <stdlib.h>
extern void MyGoScheduleBackgroundRefresh(const char *) __attribute__((weak_import));
extern char *MyGoBackgroundRefreshLastError(const char *) __attribute__((weak_import));
extern int MyGoBackgroundRefreshRegistered(const char *) __attribute__((weak_import));
static int mygo_background_available(const char *id) {
 if (!MyGoScheduleBackgroundRefresh || !MyGoBackgroundRefreshLastError || !MyGoBackgroundRefreshRegistered) return -1;
 return MyGoBackgroundRefreshRegistered(id);
}
static void mygo_background_schedule(const char *id) {MyGoScheduleBackgroundRefresh(id);}
static char *mygo_background_error(const char *id) {return MyGoBackgroundRefreshLastError(id);}
*/
import "C"
import (
	"fmt"
	"github.com/egoist/mygo/internal/platform"
	"unsafe"
)

func backgroundIdentifier(id string) (*C.char, error) {
	s := C.CString(id)
	available := int(C.mygo_background_available(s))
	if available < 1 {
		C.free(unsafe.Pointer(s))
		if available < 0 {
			return nil, platform.ErrUnsupported
		}
		return nil, fmt.Errorf("mygo: background refresh task is not registered")
	}
	return s, nil
}
func ScheduleBackgroundRefresh(id string) error {
	s, e := backgroundIdentifier(id)
	if e != nil {
		return e
	}
	defer C.free(unsafe.Pointer(s))
	C.mygo_background_schedule(s)
	return nil
}
func LastBackgroundRefreshError(id string) (string, error) {
	s, e := backgroundIdentifier(id)
	if e != nil {
		return "", e
	}
	defer C.free(unsafe.Pointer(s))
	message := C.mygo_background_error(s)
	defer C.free(unsafe.Pointer(message))
	return C.GoString(message), nil
}
