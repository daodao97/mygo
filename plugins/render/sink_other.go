//go:build !darwin && !windows && !(linux && (amd64 || arm64))

package render

import (
	"errors"
	"fmt"
)

// newVideoSink returns none: the system has no media framework MyGo
// encodes video with.
func newVideoSink(spec videoSpec, d drawer) (sink, error) {
	return nil, fmt.Errorf("render: %w: no video encoder on this system: encode with Options{Encoder: render.FFmpeg}", errors.ErrUnsupported)
}
