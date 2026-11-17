package mobile

import (
	"encoding/base64"

	"github.com/egoist/mygo/internal/mobile/wire"
	"github.com/egoist/mygo/internal/platform"
)

// What phones and tablets have that desktops don't (platform.DeviceBackend).

func (b *Backend) Haptic(kind string) { b.call(&wire.Haptic{Kind: kind}, nil) }

func (b *Backend) SetSystemBars(style string, hidden bool) {
	b.call(&wire.SetSystemBars{Style: style, Hidden: hidden}, nil)
}

func (b *Backend) SetOrientations(mask uint32) { b.call(&wire.SetOrientations{Mask: int(mask)}, nil) }

func (b *Backend) Share(s platform.ShareRequest, done func(error)) {
	req := b.request(func(r wire.Reader) {
		var e wire.ShareResultEvent
		e.Decode(&r)
		done(errorOf(e.Error))
	})
	b.call(&wire.Share{Req: req, Text: s.Text, URL: s.URL, Paths: s.Paths, X: s.X, Y: s.Y}, nil)
}

func (b *Backend) SaveFile(path, name string, done func(string, error)) {
	req := b.request(func(r wire.Reader) {
		var e wire.FilesResultEvent
		e.Decode(&r)
		saved := ""
		if len(e.Paths) > 0 {
			saved = e.Paths[0]
		}
		done(saved, errorOf(e.Error))
	})
	b.call(&wire.SaveFile{Req: req, Path: path, Name: name}, nil)
}

func (b *Backend) Battery() platform.BatteryStatus {
	var r wire.BatteryReply
	b.call(&wire.Battery{}, &r)
	return platform.BatteryStatus{Level: r.Level, Charging: r.Charging, LowPower: r.LowPower}
}

func (b *Backend) SetBarItems(pw platform.Window, items []platform.BarItem) {
	w, ok := pw.(*window)
	if !ok {
		return
	}
	w.barItems = items
	m := wire.SetBarItems{ID: w.id}
	for _, it := range items {
		m.Titles = append(m.Titles, it.Title)
		m.Symbols = append(m.Symbols, it.Symbol)
		m.Icons = append(m.Icons, base64.StdEncoding.EncodeToString(it.Icon))
	}
	b.call(&m, nil)
}

func (b *Backend) PluginCall(plugin, method string, args []byte, done func([]byte, error)) {
	req := b.request(func(r wire.Reader) {
		var e wire.PluginResultEvent
		e.Decode(&r)
		if e.Error != "" || e.Code != "" {
			done(nil, &platform.PluginError{Code: e.Code, Message: e.Error})
			return
		}
		done([]byte(e.Result), nil)
	})
	b.call(&wire.PluginCall{Req: req, Plugin: plugin, Method: method, Args: string(args)}, nil)
}

func (b *Backend) OnPluginEvent(fn func(plugin, name string, data []byte)) { b.pluginEvents = fn }

// StartedHidden reports whether the system started the app for something in the
// background, and it hasn't shown yet.
func (b *Backend) StartedHidden() bool { return b.startedHidden }

// WasEndedBySystem reports whether this launch follows the system ending the
// app, rather than the user.
func (b *Backend) WasEndedBySystem() bool { return b.appInfo().Restoring }
