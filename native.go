package mygo

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/surface"
)

// NativePlugin is a plugin's native code on phones: the Swift and Kotlin it
// brings, which its mygo-plugin.json names and mygo build compiles into the
// app. Use Native to get one.
type NativePlugin struct{ id string }

// Native returns the native code of the plugin with an id, as its
// mygo-plugin.json's mobile.id says:
//
//	var fix struct{ Latitude, Longitude float64 }
//	err := mygo.Native("location").Call(ctx, "current", nil, &fix)
func Native(id string) *NativePlugin { return &NativePlugin{id} }

// Available reports whether the app runs where plugins have native code: on
// iOS and Android.
func (p *NativePlugin) Available() bool {
	_, ok := backend().(platform.PluginBackend)
	return ok
}

// NativeError is an error of a plugin's native code, with a code to tell
// errors apart.
type NativeError = platform.PluginError

// Call calls a method of the plugin's native code and waits for its answer:
// args and reply go as JSON, either may be nil. It reports
// errors.ErrUnsupported where plugins have no native code, and a *NativeError
// for what the native code fails with. Call it off the main goroutine, as it
// may wait for the user.
func (p *NativePlugin) Call(ctx context.Context, method string, args, reply any) error {
	needsApp("NativePlugin.Call")
	var raw []byte
	if args == nil {
		raw = []byte("{}")
	} else {
		b, err := json.Marshal(args)
		if err != nil {
			return err
		}
		raw = b
	}
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	if !postMain(func() {
		pb, ok := backend().(platform.PluginBackend)
		if !ok {
			deliver(ch, result{err: errors.ErrUnsupported})
			return
		}
		pb.PluginCall(p.id, method, raw, func(data []byte, err error) { deliver(ch, result{data, err}) })
	}) {
		return errLoopStopped
	}
	var r result
	if isMainThread() {
		r = await(ch)
	} else {
		select {
		case r = <-ch:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if r.err != nil {
		return r.err
	}
	if reply == nil || len(r.data) == 0 {
		return nil
	}
	return json.Unmarshal(r.data, reply)
}

// PluginView is a view of a plugin's native code, which a window's native UI
// shows with ui.WebView, under what it paints over it, as it does a web view:
// a map, a camera's preview, a video.
type PluginView struct {
	w    *Window
	view platform.WebView
	// ID names the view to the plugin's native code: pass it with the calls
	// about the view.
	ID int
}

// NewPluginView makes a view of a type a plugin's native code registered
// (PluginHost.registerView), with its arguments as JSON, in a window showing
// native UI. Phones only: elsewhere it reports errors.ErrUnsupported.
func NewPluginView(w *Window, viewType string, args any) (*PluginView, error) {
	needsApp("NewPluginView")
	raw := []byte("{}")
	if args != nil {
		b, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	var v *PluginView
	err := errNoContent
	onMain(func() {
		if w.native == nil || w.conn == nil {
			return
		}
		s, ok := w.conn.Surface.(platform.PluginViewSurface)
		if !ok {
			err = errors.ErrUnsupported
			return
		}
		nv, e := s.NewPluginView(viewType, raw, &windowHandler{w})
		if err = e; err != nil {
			return
		}
		id := 0
		if h, ok := nv.(interface{ ViewID() int }); ok {
			id = h.ViewID()
		}
		v = &PluginView{w: w, view: nv, ID: id}
	})
	return v, err
}

// Destroy closes the view.
func (v *PluginView) Destroy() { onMain(v.view.Close) }

// SurfaceWebView gives package ui the view to place, in its window's content
// only. Main thread only.
func (v *PluginView) SurfaceWebView(conn *surface.Conn) platform.WebView {
	if v == nil || conn.Window != v.w {
		return nil
	}
	return v.view
}

// nativeEvents holds who hears plugins' native code.
var nativeEvents struct {
	sync.Mutex
	on    map[string][]*nativeListener
	wired bool
}

type nativeListener struct{ fn func(data []byte) }

// On calls fn on the main thread as the plugin's native code sends the event
// name, with its JSON.
func (p *NativePlugin) On(name string, fn func(data []byte)) (off func()) {
	l := &nativeListener{fn}
	key := p.id + "\x00" + name
	nativeEvents.Lock()
	if nativeEvents.on == nil {
		nativeEvents.on = map[string][]*nativeListener{}
	}
	nativeEvents.on[key] = append(nativeEvents.on[key], l)
	wire := !nativeEvents.wired
	nativeEvents.wired = true
	nativeEvents.Unlock()
	if wire {
		App.WhenReady(func() {
			if pb, ok := backend().(platform.PluginBackend); ok {
				pb.OnPluginEvent(func(plugin, name string, data []byte) {
					nativeEvents.Lock()
					list := append([]*nativeListener(nil), nativeEvents.on[plugin+"\x00"+name]...)
					nativeEvents.Unlock()
					for _, l := range list {
						l.fn(data)
					}
				})
			}
		})
	}
	return func() {
		nativeEvents.Lock()
		defer nativeEvents.Unlock()
		list := nativeEvents.on[key]
		for i, x := range list {
			if x == l {
				nativeEvents.on[key] = append(list[:i:i], list[i+1:]...)
				return
			}
		}
	}
}
