// Package browser shows a web page in the system's browser without leaving
// the app: SFSafariViewController on iOS, a Custom Tab on Android. Elsewhere
// Open reports errors.ErrUnsupported.
//
// It is the fork's (FORK.md): #940 has no counterpart.
package browser

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/egoist/mygo"
)

const pluginID = "browser"

// ErrBusy is what Open fails with as another page shows.
var ErrBusy = errors.New("browser: another page shows")

var ids atomic.Int64

// Open shows rawURL, an http or https URL without credentials, and returns
// as the user closes it or ctx is canceled, which closes it on iOS. Call it
// off the main goroutine.
func Open(ctx context.Context, rawURL string) error {
	if ctx == nil {
		return errors.New("browser: Open needs a context")
	}
	if err := check(rawURL); err != nil {
		return err
	}
	p := mygo.Native(pluginID)
	if !p.Available() {
		return mygo.ErrUnsupported
	}
	id := ids.Add(1)
	err := p.Call(ctx, "open", map[string]any{"id": id, "url": rawURL}, nil)
	if ctx.Err() != nil {
		go p.Call(context.Background(), "close", map[string]any{"id": id}, nil)
		return ctx.Err()
	}
	var n *mygo.NativeError
	if errors.As(err, &n) && n.Code == "Busy" {
		return ErrBusy
	}
	return err
}

func check(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || u.User != nil || !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return errors.New("browser: Open needs an http or https URL without credentials")
	}
	return nil
}
