// Package network prepares the app's network access through the system's
// own stack before the app opens raw sockets with package net: on iOS one
// request through Foundation brings up the cellular-data and local-network
// prompts and the VPN and DNS64 routes, which BSD sockets alone do not.
// Desktops and Android have nothing to prepare.
//
// It is the fork's (FORK.md): #940 has no counterpart.
package network

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
)

const pluginID = "network"

// Error reports why Prepare failed: Kind is "offline" without a usable
// connection (or with cellular data off for the app), "timeout",
// "canceled" or "failed". Use errors.As.
type Error struct{ Kind string }

func (e *Error) Error() string { return "network: preparation " + e.Kind }

var (
	prepared atomic.Bool
	ids      atomic.Int64
)

// Prepare makes one HEAD request to rawURL, an https URL, through the
// system's network stack. Any HTTP response is a success, after which
// later calls return nil at once. It returns nil at once where there is
// nothing to prepare. Canceling ctx stops the request.
func Prepare(ctx context.Context, rawURL string) error {
	if ctx == nil {
		return errors.New("network: Prepare needs a context")
	}
	u, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Host == "" {
		return errors.New("network: Prepare needs an https URL")
	}
	p := mygo.Native(pluginID)
	if prepared.Load() || !p.Available() {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	timeout := 10 * time.Second
	if d, ok := ctx.Deadline(); ok {
		timeout = min(timeout, time.Until(d))
	}
	id := ids.Add(1)
	err = p.Call(ctx, "prepare", map[string]any{"id": id, "url": rawURL, "timeout": timeout.Seconds()}, nil)
	if ctx.Err() != nil {
		go p.Call(context.Background(), "cancel", map[string]any{"id": id}, nil)
		return ctx.Err()
	}
	if err = classify(err); err == nil {
		prepared.Store(true)
	}
	return err
}

func classify(err error) error {
	var n *mygo.NativeError
	if errors.As(err, &n) {
		switch n.Code {
		case "offline", "timeout", "canceled", "failed":
			return &Error{Kind: n.Code}
		}
	}
	return err
}
