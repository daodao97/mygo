//go:build linux && !android && (amd64 || arm64) && !android

package mygo

import (
	"github.com/egoist/mygo/internal/linux"
	"github.com/egoist/mygo/internal/platform"
)

func newBackend() platform.Backend { return linux.New() }

func linkContentRules() { linux.LinkContentRules() }

func linkNowPlaying() { linux.LinkNowPlaying() }

func linkNotificationImages() { linux.LinkNotificationImages() }

func linkMenuIcons() { linux.LinkMenuIcons() }

func linkContextMenus() { linux.LinkContextMenus() }

func linkDMABUFProbe() { linux.LinkDMABUFProbe() }
