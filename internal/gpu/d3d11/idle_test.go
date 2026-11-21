//go:build windows

package d3d11

import (
	"runtime"
	"testing"
	"weak"

	"github.com/egoist/mygo/internal/scene"
)

func TestIdleReleasesCollectedImages(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	r, err := New(hiddenWindow(t, 64, 64))
	if err != nil {
		t.Skip("no Direct3D 11:", err)
	}
	defer r.Release()
	live := scene.NewImageRGBA(16, 16, make([]byte, 16*16*4))
	stale := scene.NewImageRGBA(16, 16, make([]byte, 16*16*4))
	liveID, staleID := live.ID(), stale.ID()
	if r.imageView(live) == 0 || r.imageView(stale) == 0 {
		t.Fatal("could not upload both images")
	}
	liveTexture := r.images[liveID]
	staleSource := weak.Make(stale)
	stale = nil
	runtime.GC()
	if staleSource.Value() != nil {
		t.Fatal("texture cache retains the source image")
	}
	r.TrimIdleImages()
	if r.images[staleID] != nil {
		t.Error("idle renderer retains the collected image's texture")
	}
	if r.images[liveID] != liveTexture || r.imageView(live) != liveTexture.srv {
		t.Error("idle cleanup replaced the texture of a live image")
	}
	runtime.KeepAlive(live)
}
