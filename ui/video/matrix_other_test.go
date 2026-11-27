//go:build !(linux && !android && (amd64 || arm64))

package video

// matrixMissing is none: only GStreamer's elements vary by what is
// installed, where its engine builds.
func matrixMissing([]string) []string { return nil }
