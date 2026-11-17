package network

import (
	"context"
	"errors"

	"github.com/egoist/mygo"
)

// Interface is a network interface as the system tells it. Addresses are in
// CIDR notation, as "192.168.1.20/24".
type Interface struct {
	Name      string   `json:"name"`
	Index     int      `json:"index"`
	MTU       int      `json:"mtu"`
	Up        bool     `json:"up"`
	Loopback  bool     `json:"loopback"`
	Addresses []string `json:"addresses"`
}

// Interfaces returns the device's network interfaces from the system, on
// Android, whose apps may not read them through netlink, as Go's
// net.Interfaces does, since Android 11. Elsewhere it reports
// errors.ErrUnsupported: use net.Interfaces. It waits for the app to be
// ready; call it off the main goroutine.
func Interfaces(ctx context.Context) ([]Interface, error) {
	if ctx == nil {
		return nil, errors.New("network: Interfaces needs a context")
	}
	p := mygo.Native(pluginID)
	if !p.Available() {
		return nil, errors.ErrUnsupported
	}
	ready := make(chan struct{})
	mygo.App.WhenReady(func() { close(ready) })
	select {
	case <-ready:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	var r struct {
		Interfaces []Interface `json:"interfaces"`
	}
	if err := p.Call(ctx, "interfaces", nil, &r); err != nil {
		var n *mygo.NativeError
		if errors.As(err, &n) && (n.Code == "Unsupported" || n.Code == "NoMethod") {
			return nil, errors.ErrUnsupported
		}
		return nil, err
	}
	return r.Interfaces, nil
}
