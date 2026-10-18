//go:build ios && cgo

package ios

/*
#cgo LDFLAGS: -framework Network
#include <stdint.h>
uint32_t mygo_ios_network_status(void);
void mygo_ios_network_watch(void);
*/
import "C"
import "github.com/egoist/mygo/internal/platform"

type network struct{ b *Backend }

func (b *Backend) Network() platform.Network { return network{b} }
func (n network) Status() platform.NetworkStatus {
	return basicNetwork(uint32(C.mygo_ios_network_status()))
}
func (n network) Watch() { n.b.networkWatching = true; C.mygo_ios_network_watch() }
func (n network) PathStatus() platform.NetworkPathStatus {
	return detailedNetwork(uint32(C.mygo_ios_network_status()))
}
func (n network) WatchPath(fn func(platform.NetworkPathStatus)) func() {
	b := n.b
	b.networkToken++
	id := b.networkToken
	if b.networkObservers == nil {
		b.networkObservers = map[uint64]func(platform.NetworkPathStatus){}
	}
	b.networkObservers[id] = fn
	C.mygo_ios_network_watch()
	// First answer is native/asynchronous. Already known state is also delivered.
	if s := n.PathStatus(); s.Known {
		dispatch(func() {
			if f := b.networkObservers[id]; f != nil {
				f(s)
			}
		})
	}
	return func() { delete(b.networkObservers, id) }
}

//export mygo_ios_network_changed
func mygo_ios_network_changed(raw C.uint32_t) {
	b := current.Load()
	if b == nil || b.h == nil {
		return
	}
	select {
	case <-b.done:
		return
	default:
	}
	mask := uint32(raw)
	if b.networkWatching {
		b.h.NetworkChanged(basicNetwork(mask))
	}
	if b.networkLast == mask {
		return
	}
	b.networkLast = mask
	callbacks := make([]uint64, 0, len(b.networkObservers))
	for id := range b.networkObservers {
		callbacks = append(callbacks, id)
	}
	for _, id := range callbacks {
		if fn := b.networkObservers[id]; fn != nil {
			fn(detailedNetwork(mask))
		}
	}
}
