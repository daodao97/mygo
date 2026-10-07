//go:build ios && cgo

package ios

/*
#include "native.h"
*/
import "C"
import (
	"encoding/json"
	"errors"
	"github.com/egoist/mygo/internal/platform"
)

type mobile struct{}

func (*Backend) Mobile() platform.Mobile { return mobile{} }
func (mobile) Haptic(kind string) error {
	cString(kind, func(p *C.char) { C.mygo_ios_haptic(p) })
	return nil
}
func (mobile) SetBadge(n int, done func(error)) {
	systemRequest(nil, func(id C.uint64_t, _ *C.char) { C.mygo_ios_badge(id, C.int(n)) }, func(_ string, err error) { done(err) })
}
func (mobile) SetStatusBar(style string, hidden bool) error {
	cString(style, func(p *C.char) { C.mygo_ios_status_bar(p, C.bool(hidden)) })
	return nil
}
func (mobile) RegisterPush() error            { C.mygo_ios_register_push(); return nil }
func (*Backend) NotificationsSupported() bool { return true }
func (*Backend) ShowNotification(n *platform.Notification, done func(error)) {
	systemRequest(n, func(id C.uint64_t, p *C.char) { C.mygo_ios_notification(id, p) }, func(_ string, err error) { done(err) })
}
func (*Backend) RemoveNotification(id string) {
	cString(id, func(p *C.char) { C.mygo_ios_remove_notification(p) })
}
func (*Backend) RemoveAllNotifications() { C.mygo_ios_clear_notifications() }

//export goIOSNotification
func goIOSNotification(id *C.char, data *C.char, clicked C.bool) {
	b := current.Load()
	if b == nil || b.h == nil {
		return
	}
	var values map[string]string
	if json.Unmarshal([]byte(C.GoString(data)), &values) != nil {
		return
	}
	if h, ok := b.h.(platform.MobileHandler); ok {
		h.NotificationReceived(C.GoString(id), values, bool(clicked))
	}
}

//export goIOSPush
func goIOSPush(token *C.char, message *C.char) {
	b := current.Load()
	if b == nil || b.h == nil {
		return
	}
	var err error
	if s := C.GoString(message); s != "" {
		err = errors.New(s)
	}
	if h, ok := b.h.(platform.MobileHandler); ok {
		h.PushRegistered(C.GoString(token), err)
	}
}
