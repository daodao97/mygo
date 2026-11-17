// Package maps shows a map in a window's native UI: MapKit on iOS, and on Android
// osmdroid, with OpenStreetMap's tiles by default, which needs no account or key.
//
//	m, err := maps.New(win, maps.Options{Center: maps.LatLng{Lat: 48.86, Lng: 2.35}, Zoom: 12})
//	...
//	ui.WebView(c, m).Fill() // in the view; what it draws after shows over the map
//	m.OnTap(func(at maps.LatLng) { ... })
//	m.AddMarker(ctx, maps.Marker{ID: "home", At: at, Title: "Home"})
//
// A finger pans, pinches and taps it. OpenStreetMap's tile servers are for light
// use: an app with many users gives Options.TileURL a server of its own (Android;
// iOS shows Apple's maps). On desktops and in browsers New reports
// errors.ErrUnsupported.
package maps

import (
	"context"
	"encoding/json"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/surface"
)

const pluginID = "maps"

// LatLng is a place, in degrees.
type LatLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// Options say what a map shows first.
type Options struct {
	Center LatLng
	// Zoom is as on web maps: 0 shows the whole world, 18 a street; 2 by default.
	Zoom float64
	// TileURL is where Android's map gets its tiles, with {z}, {x} and {y}; by
	// default OpenStreetMap's.
	TileURL string
}

// Marker is a pin on a map.
type Marker struct {
	// ID tells it apart: OnMarkerTap reports it, and adding it again moves it.
	ID string `json:"id"`
	At LatLng `json:"at"`
	// Title shows as it's tapped.
	Title string `json:"title,omitempty"`
}

// Map is a map in a window. Show it with ui.WebView.
type Map struct {
	view *mygo.PluginView
	offs []func()
}

// New makes a map in a window showing native UI.
func New(w *mygo.Window, o Options) (*Map, error) {
	zoom := o.Zoom
	if zoom == 0 {
		zoom = 2
	}
	v, err := mygo.NewPluginView(w, "map", map[string]any{"lat": o.Center.Lat, "lng": o.Center.Lng, "zoom": zoom, "tileURL": o.TileURL})
	if err != nil {
		return nil, err
	}
	return &Map{view: v}, nil
}

// SurfaceWebView gives package ui the view to show.
func (m *Map) SurfaceWebView(conn *surface.Conn) platform.WebView {
	return m.view.SurfaceWebView(conn)
}

func (m *Map) call(ctx context.Context, method string, args map[string]any, reply any) error {
	args["id"] = m.view.ID
	return mygo.Native(pluginID).Call(ctx, method, args, reply)
}

// SetCamera moves the map to a place, gliding there if animated.
func (m *Map) SetCamera(ctx context.Context, center LatLng, zoom float64, animated bool) error {
	return m.call(ctx, "setCamera", map[string]any{"lat": center.Lat, "lng": center.Lng, "zoom": zoom, "animated": animated}, nil)
}

// Camera returns what the map shows now, as the user panned it.
func (m *Map) Camera(ctx context.Context) (center LatLng, zoom float64, err error) {
	var r struct {
		Lat, Lng, Zoom float64
	}
	err = m.call(ctx, "camera", map[string]any{}, &r)
	return LatLng{r.Lat, r.Lng}, r.Zoom, err
}

// AddMarker puts a pin on the map, in place of one of its ID.
func (m *Map) AddMarker(ctx context.Context, marker Marker) error {
	return m.call(ctx, "addMarker", map[string]any{"marker": marker.ID, "lat": marker.At.Lat, "lng": marker.At.Lng, "title": marker.Title}, nil)
}

// RemoveMarker takes a pin off the map.
func (m *Map) RemoveMarker(ctx context.Context, id string) error {
	return m.call(ctx, "removeMarker", map[string]any{"marker": id}, nil)
}

// OnTap calls fn on the main thread as a finger taps the map, not one of its pins.
func (m *Map) OnTap(fn func(at LatLng)) {
	id := m.view.ID
	m.offs = append(m.offs, mygo.Native(pluginID).On("tap", func(data []byte) {
		var e struct {
			ID       int
			Lat, Lng float64
		}
		if json.Unmarshal(data, &e) == nil && e.ID == id {
			fn(LatLng{e.Lat, e.Lng})
		}
	}))
}

// OnMarkerTap calls fn on the main thread as a finger taps a pin.
func (m *Map) OnMarkerTap(fn func(id string)) {
	view := m.view.ID
	m.offs = append(m.offs, mygo.Native(pluginID).On("markerTap", func(data []byte) {
		var e struct {
			ID     int
			Marker string
		}
		if json.Unmarshal(data, &e) == nil && e.ID == view {
			fn(e.Marker)
		}
	}))
}

// Destroy takes the map away, which closing its window doesn't: call it before.
func (m *Map) Destroy() {
	for _, off := range m.offs {
		off()
	}
	m.offs = nil
	go mygo.Native(pluginID).Call(context.Background(), "dispose", map[string]any{"id": m.view.ID}, nil)
	m.view.Destroy()
}
