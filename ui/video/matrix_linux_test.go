//go:build linux && !android && (amd64 || arm64)

package video

import "github.com/ebitengine/purego"

// matrixMissing is the elements of names that GStreamer has not, as one
// of a plugin set not installed (gst-plugins-bad's tsdemux, gst-libav's
// avdec_h264).
func matrixMissing(names []string) []string {
	if loadGStreamer() != nil {
		return names
	}
	h, err := purego.Dlopen("libgstreamer-1.0.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return names
	}
	var find func(name string) uintptr
	purego.RegisterLibFunc(&find, h, "gst_element_factory_find")
	var missing []string
	for _, n := range names {
		f := find(n)
		if f == 0 {
			missing = append(missing, n)
			continue
		}
		gstObjectUnref(f)
	}
	return missing
}
