//go:build linux && !android && (amd64 || arm64)

package gl

import (
	"runtime"
	"testing"

	"github.com/egoist/mygo/internal/gpu/gputest"
	"github.com/egoist/mygo/internal/scene"
)

// BenchmarkRenderSprites draws frames of a 2D game at 1280×720 (gputest's
// SpriteScene, tiles 2 times as large: about 1,800 sprites of one sheet)
// and reads each back, which waits for it. Where no GPU runs it, Mesa's
// llvmpipe draws on the CPU, so the time is mostly its own.
func BenchmarkRenderSprites(b *testing.B) {
	benchRender(b, gputest.SpriteScene(1280, 720, 2, 3, 300))
}

// BenchmarkRenderEmpty draws frames of nothing at 1280×720: what the
// sprites' frames spend besides them.
func BenchmarkRenderEmpty(b *testing.B) {
	benchRender(b, &scene.Scene{Width: 1280, Height: 720, Clear: scene.Color{A: 255}})
}

func benchRender(b *testing.B, s *scene.Scene) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	o, err := NewOffscreen(false, s.Width, s.Height)
	if err != nil {
		b.Skip(err)
	}
	defer o.Release()
	for b.Loop() {
		if _, err := o.Render(s); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVideoFrameRGBA draws a new 1920×1080 frame of video each
// frame the way video drew before frames had surfaces: converted to RGBA
// on the CPU (as a decoder in Go would, or the CPU renderer does) and
// uploaded, 8 MB a frame.
func BenchmarkVideoFrameRGBA(b *testing.B) {
	img := gputest.NV12Frame(1920, 1080, scene.BT709)
	rgb := scene.NewImageRGBA(1920, 1080, nil)
	s := videoBenchScene(rgb)
	benchVideo(b, s, func() {
		rgb.Pix = img.Surface.RGBA(rgb.Pix, 1920, 1080)
		rgb.Changed()
	})
}

// BenchmarkVideoFrameNV12 draws a new 1920×1080 frame of video each frame
// from its NV12 planes, as GStreamer leaves them: 3 MB uploaded, and
// converted in the shader.
func BenchmarkVideoFrameNV12(b *testing.B) {
	img := gputest.NV12Frame(1920, 1080, scene.BT709)
	s := videoBenchScene(img)
	benchVideo(b, s, img.Changed)
}

func videoBenchScene(img *scene.Image) *scene.Scene {
	return &scene.Scene{Width: 1280, Height: 720, Clear: scene.Color{A: 255}, Ops: []scene.Op{{
		Kind: scene.OpImage, Rect: scene.Rect{W: 1280, H: 720}, Image: img, Src: scene.Rect{W: 1920, H: 1080},
	}}}
}

func benchVideo(b *testing.B, s *scene.Scene, next func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	o, err := NewOffscreen(false, s.Width, s.Height)
	if err != nil {
		b.Skip(err)
	}
	defer o.Release()
	b.SetBytes(1920 * 1080 * 4)
	for b.Loop() {
		next()
		if _, err := o.Render(s); err != nil {
			b.Fatal(err)
		}
	}
}
