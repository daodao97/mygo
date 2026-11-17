// Package camera shows what a phone's camera sees in a window's native UI, and
// takes photos: AVCaptureSession on iOS, CameraX on Android. For a photo in the
// system's own camera screen, use package imagepicker: it needs no permission.
//
//	cam, err := camera.New(win, camera.Options{})
//	...
//	ui.WebView(c, cam).Fill() // in the view; what it draws after shows over the preview
//	path, err := cam.TakePhoto(ctx)
//
// The preview shows under the content, as a web view does. On desktops and in
// browsers New reports errors.ErrUnsupported.
package camera

import (
	"context"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/surface"
)

const pluginID = "camera"

// Options are which camera shows.
type Options struct {
	// Front chooses the camera on the screen's side.
	Front bool
}

// Camera is a preview of a camera, which takes photos. Show it with ui.WebView.
type Camera struct{ view *mygo.PluginView }

// New asks the user for the camera, the first time, and makes its preview in a
// window showing native UI.
func New(w *mygo.Window, o Options) (*Camera, error) {
	v, err := mygo.NewPluginView(w, "camera", map[string]any{"front": o.Front})
	if err != nil {
		return nil, err
	}
	return &Camera{v}, nil
}

// SurfaceWebView gives package ui the view to show.
func (c *Camera) SurfaceWebView(conn *surface.Conn) platform.WebView {
	return c.view.SurfaceWebView(conn)
}

// TakePhoto takes a photo and returns the path of its JPEG in the app's
// temporary directory.
func (c *Camera) TakePhoto(ctx context.Context) (string, error) {
	var r struct {
		Path string `json:"path"`
	}
	err := mygo.Native(pluginID).Call(ctx, "takePhoto", map[string]any{"id": c.view.ID}, &r)
	return r.Path, err
}

// Destroy stops the camera and takes its preview away.
func (c *Camera) Destroy() {
	go mygo.Native(pluginID).Call(context.Background(), "dispose", map[string]any{"id": c.view.ID}, nil)
	c.view.Destroy()
}
