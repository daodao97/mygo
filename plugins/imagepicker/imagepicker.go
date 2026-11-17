// Package imagepicker lets the user choose photos and videos from their
// library, or take one with the camera, on phones and tablets: the system's
// own pickers (PHPickerViewController and UIImagePickerController on iOS, the
// Photo Picker and the camera app on Android), which ask the user for no
// permission to read the library. Elsewhere its functions report
// errors.ErrUnsupported.
package imagepicker

import (
	"context"

	"github.com/egoist/mygo"
)

const pluginID = "imagepicker"

// Kind is what a picker shows.
type Kind string

const (
	Images Kind = "images"
	Videos Kind = "videos"
	// Any shows both.
	Any Kind = "any"
)

// Options are what Pick lets the user choose.
type Options struct {
	// Kind is what it shows: Images by default.
	Kind Kind
	// Limit is how many the user may choose: 1 by default, 0 meaning 1, and
	// negative as many as they like.
	Limit int
}

// Pick shows the system's picker of the user's library, and returns the paths
// of copies of what they chose in the app's temporary directory: none when
// they went back. Call it off the main goroutine, as it waits for the user.
func Pick(ctx context.Context, o Options) ([]string, error) {
	if o.Kind == "" {
		o.Kind = Images
	}
	if o.Limit == 0 {
		o.Limit = 1
	}
	var r struct {
		Paths []string `json:"paths"`
	}
	err := mygo.Native(pluginID).Call(ctx, "pick", map[string]any{"kind": o.Kind, "limit": o.Limit}, &r)
	return r.Paths, err
}

// Capture shows the camera, for a photo, and returns the path of a JPEG of it
// in the app's temporary directory, "" when the user went back.
func Capture(ctx context.Context) (string, error) {
	var r struct {
		Path string `json:"path"`
	}
	err := mygo.Native(pluginID).Call(ctx, "capture", nil, &r)
	return r.Path, err
}
