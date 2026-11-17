//go:build !(linux && !android && (amd64 || arm64)) && !darwin && !(windows && (amd64 || arm64)) && !js

package video

import "github.com/egoist/mygo/internal/media"

func newEngine(source) (media.Engine, error) { return nil, ErrUnsupported }
