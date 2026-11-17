//go:build ios || android

package mygo

import (
	"github.com/egoist/mygo/internal/mobile"
	"github.com/egoist/mygo/internal/platform"
)

func newBackend() platform.Backend { return mobile.New() }
