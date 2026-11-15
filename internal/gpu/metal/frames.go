//go:build darwin

package metal

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/egoist/mygo/internal/scene"
)

// Frames draws frames one after another without a layer, for videos
// (plugins/render): into a texture of its own, whose pixels Read copies,
// or into IOSurfaces of the caller's, as the CVPixelBuffers an encoder
// takes, without a copy. A frame is committed without waiting for the
// GPU, which draws it while the caller builds the next one; Wait waits
// for it.
//
// Frames is not safe for concurrent use.
type Frames struct {
	r *Renderer
	// tex is the texture Draw draws into, of w×h pixels.
	tex  id
	w, h int
	// last is the command buffer of the last frame, retained until Wait
	// sees it done, and surface the texture wrapping the IOSurface it drew
	// into, released then too.
	last, surface id
}

// NewFrames returns a Frames on the system's default GPU.
func NewFrames() (*Frames, error) {
	r, err := newRenderer()
	if err != nil {
		return nil, err
	}
	f := &Frames{r: r}
	pool(func() { err = r.initGPU() })
	if err != nil {
		r.Release()
		return nil, err
	}
	return f, nil
}

// Name returns the name of the GPU, as "Apple M2".
func (f *Frames) Name() (name string) {
	pool(func() { name = goString(send(f.r.device, "name")) })
	return name
}

// Draw draws s into the Frames' own texture, after waiting for the frame
// before, whose pixels the caller read. Read copies its pixels once Wait
// returned.
func (f *Frames) Draw(s *scene.Scene) (err error) {
	pool(func() {
		if err = f.wait(); err != nil {
			return
		}
		if f.tex == 0 || f.w != s.Width || f.h != s.Height {
			release(&f.tex)
			f.r.cur = &f.r.formats[0]
			if f.tex = f.r.newTexture(s.Width, s.Height, pixelFormatBGRA8Unorm, usageRenderTarget|usageShaderRead, nil, 0); f.tex == 0 {
				err = errors.New("metal: cannot create a target")
				return
			}
			f.w, f.h = s.Width, s.Height
		}
		err = f.commit(s, f.tex, true)
	})
	return err
}

// DrawSurface draws s into surface, an IOSurfaceRef of s's size in BGRA,
// as a CVPixelBuffer's from a pool, after waiting for the frame before.
// The surface holds the frame once Wait returned.
func (f *Frames) DrawSurface(s *scene.Scene, surface uintptr) (err error) {
	pool(func() {
		if err = f.wait(); err != nil {
			return
		}
		desc := send(class("MTLTextureDescriptor"), "texture2DDescriptorWithPixelFormat:width:height:mipmapped:",
			pixelFormatBGRA8Unorm, uintptr(s.Width), uintptr(s.Height), 0)
		send(desc, "setUsage:", usageRenderTarget|usageShaderRead)
		tex := send(f.r.device, "newTextureWithDescriptor:iosurface:plane:", desc, surface, 0)
		if tex == 0 {
			err = errors.New("metal: cannot draw into the IOSurface")
			return
		}
		f.surface = tex
		f.r.cur = &f.r.formats[0]
		if err = f.commit(s, tex, false); err != nil {
			release(&f.surface)
		}
	})
	return err
}

// commit encodes s into target and commits it, keeping its command
// buffer for Wait; read synchronizes a managed texture for the CPU.
func (f *Frames) commit(s *scene.Scene, target id, read bool) error {
	cb, err := f.r.encode(s, target)
	if err != nil {
		return err
	}
	if read {
		blit := send(cb, "blitCommandEncoder")
		send(blit, "synchronizeResource:", target)
		send(blit, "endEncoding")
	}
	send(cb, "commit")
	f.r.track(cb)
	f.last = send(cb, "retain")
	f.r.sweepIfDue()
	return nil
}

// Wait waits for the GPU to finish the last frame drawn.
func (f *Frames) Wait() (err error) {
	pool(func() { err = f.wait() })
	return err
}

func (f *Frames) wait() error {
	if f.last == 0 {
		return nil
	}
	send(f.last, "waitUntilCompleted")
	var err error
	if int(send(f.last, "status")) == statusError {
		err = fmt.Errorf("metal: the frame failed: %s", describe(send(f.last, "error")))
	}
	release(&f.last)
	release(&f.surface)
	return err
}

// Read copies the pixels of the frame Draw drew last into dst, once Wait
// returned: premultiplied BGRA rows from the top, 4×width bytes each.
func (f *Frames) Read(dst []byte) error {
	if f.tex == 0 || len(dst) < f.w*f.h*4 {
		return errors.New("metal: no frame to read, or too small a buffer")
	}
	pool(func() {
		msgGetBytes(f.tex, sel("getBytes:bytesPerRow:fromRegion:mipmapLevel:"), unsafe.Pointer(&dst[0]), uint(f.w*4),
			mtlRegion{W: uint(f.w), H: uint(f.h), D: 1}, 0)
	})
	return nil
}

// Release waits for the GPU, and frees the texture and the renderer.
func (f *Frames) Release() {
	pool(func() {
		f.wait()
		release(&f.surface)
		release(&f.tex)
	})
	f.r.Release()
}

// goString returns the text of an NSString.
func goString(str id) string {
	if str == 0 {
		return ""
	}
	p := send(str, "UTF8String")
	if p == 0 {
		return ""
	}
	n := 0
	for *(*byte)(ptr(p + uintptr(n))) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(ptr(p)), n))
}
