package mygo

import "github.com/egoist/mygo/internal/platform"

// NetworkPathStatus describes the system-selected path, not a reachability
// probe or a guarantee that a server answers. VPN paths can obscure the physical
// interface. Known is false until the first native path arrives. Currently iOS.
type NetworkPathStatus = platform.NetworkPathStatus

// PathStatus returns detailed path state. Other backends return ErrUnsupported
// while their existing Network.Status and OnChanged continue to work.
func (n *NetworkModule) PathStatus() (NetworkPathStatus, error) {
	needsApp("Network.PathStatus")
	type result struct {
		status NetworkPathStatus
		err    error
	}
	r := onMainValue(func() result {
		p, ok := backend().Network().(platform.NetworkPathSource)
		if !ok {
			return result{err: ErrUnsupported}
		}
		return result{status: p.PathStatus()}
	})
	return r.status, r.err
}

// OnPathChanged receives known path changes on the main thread, including
// interface/cost changes that leave Online unchanged. off cancels subscription.
func (n *NetworkModule) OnPathChanged(fn func(NetworkPathStatus)) (off func(), err error) {
	needsApp("Network.OnPathChanged")
	if fn == nil {
		return func() {}, nil
	}
	onMain(func() {
		p, ok := backend().Network().(platform.NetworkPathSource)
		if !ok {
			err = ErrUnsupported
			return
		}
		cancel := p.WatchPath(fn)
		off = func() { onMain(cancel) }
	})
	return
}
