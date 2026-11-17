//go:build darwin && !ios

package metal

import (
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/egoist/mygo/internal/gpu/gputest"
	"github.com/egoist/mygo/internal/scene"
)

// TestDrawsVideoAsTheCPURenderer draws frames of video in NV12 from their
// planes in memory, then from CVPixelBuffers backed by IOSurfaces, which
// the renderer samples where they are, as the shader converts them as
// the CPU renderer does.
func TestDrawsVideoAsTheCPURenderer(t *testing.T) {
	r, err := newRenderer()
	if err != nil {
		unavailable(t, err)
	}
	defer r.Release()
	s := gputest.VideoScene()
	for range 2 {
		for _, op := range s.Ops {
			op.Image.Changed()
		}
		pix, err := r.renderOffscreen(s)
		if err != nil {
			t.Fatal(err)
		}
		gputest.Compare(t, "metal-video", pix, s.Width*4, s)
	}
	for _, op := range s.Ops {
		if tex := r.images[op.Image.ID()]; tex == nil || tex.chroma == 0 {
			t.Fatal("a frame of video drew from RGBA, not its planes")
		}
	}

	// The same frames in CVPixelBuffers.
	if err := loadCoreVideo(); err != nil {
		t.Fatal(err)
	}
	var bufs []uintptr
	defer func() {
		for _, b := range bufs {
			purego.SyscallN(cfRelease, b)
		}
	}()
	done := map[*scene.Image]bool{}
	for _, op := range s.Ops {
		img := op.Image
		if done[img] {
			continue
		}
		done[img] = true
		b := pixelBuffer(t, img)
		bufs = append(bufs, b)
		img.Surface = &scene.Surface{Format: img.Surface.Format, Matrix: img.Surface.Matrix, Native: b, Planes: img.Surface.Planes}
		img.Changed()
	}
	for range 3 {
		pix, err := r.renderOffscreen(s)
		if err != nil {
			t.Fatal(err)
		}
		gputest.Compare(t, "metal-video-iosurface", pix, s.Width*4, s)
	}
	for _, op := range s.Ops {
		if tex := r.images[op.Image.ID()]; tex == nil || tex.cv[1] == 0 {
			t.Fatal("a frame in a CVPixelBuffer was not wrapped as textures")
		}
	}
}

// pixelBuffer returns a CVPixelBuffer backed by an IOSurface holding the
// planes of img's surface, in NV12 of the video range ('420v'), as
// AVFoundation's decoders give them.
func pixelBuffer(t *testing.T, img *scene.Image) uintptr {
	cv, err := purego.Dlopen("/System/Library/Frameworks/CoreVideo.framework/CoreVideo", purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		t.Fatal(err)
	}
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		t.Fatal(err)
	}
	var (
		create     func(alloc uintptr, w, h uint, format uint32, attrs uintptr, out *uintptr) int32
		lock       func(buf uintptr, flags uint64) int32
		unlock     func(buf uintptr, flags uint64) int32
		base       func(buf uintptr, plane uint) unsafe.Pointer
		stride     func(buf uintptr, plane uint) uint
		dictCreate func(alloc uintptr, keys, values *uintptr, n int, kcb, vcb uintptr) uintptr
	)
	purego.RegisterLibFunc(&create, cv, "CVPixelBufferCreate")
	purego.RegisterLibFunc(&lock, cv, "CVPixelBufferLockBaseAddress")
	purego.RegisterLibFunc(&unlock, cv, "CVPixelBufferUnlockBaseAddress")
	purego.RegisterLibFunc(&base, cv, "CVPixelBufferGetBaseAddressOfPlane")
	purego.RegisterLibFunc(&stride, cv, "CVPixelBufferGetBytesPerRowOfPlane")
	purego.RegisterLibFunc(&dictCreate, cf, "CFDictionaryCreate")
	sym := func(h uintptr, name string) uintptr {
		p, err := purego.Dlsym(h, name)
		if err != nil {
			t.Fatal(err)
		}
		return *(*uintptr)(ptr(p))
	}
	keyCB, _ := purego.Dlsym(cf, "kCFTypeDictionaryKeyCallBacks")
	valueCB, _ := purego.Dlsym(cf, "kCFTypeDictionaryValueCallBacks")
	empty := dictCreate(0, nil, nil, 0, keyCB, valueCB)
	defer purego.SyscallN(cfRelease, empty)
	keys := []uintptr{sym(cv, "kCVPixelBufferIOSurfacePropertiesKey"), sym(cv, "kCVPixelBufferMetalCompatibilityKey")}
	values := []uintptr{empty, sym(cf, "kCFBooleanTrue")}
	attrs := dictCreate(0, &keys[0], &values[0], len(keys), keyCB, valueCB)
	defer purego.SyscallN(cfRelease, attrs)
	var buf uintptr
	const format420v = 0x34323076
	if create(0, uint(img.W), uint(img.H), format420v, attrs, &buf) != 0 {
		t.Fatal("cannot create a CVPixelBuffer")
	}
	lock(buf, 0)
	s := img.Surface
	rows := [2]int{img.H, (img.H + 1) / 2}
	widths := [2]int{img.W, 2 * ((img.W + 1) / 2)}
	for p := range 2 {
		dst := unsafe.Slice((*byte)(base(buf, uint(p))), int(stride(buf, uint(p)))*rows[p])
		for y := range rows[p] {
			copy(dst[y*int(stride(buf, uint(p))):][:widths[p]], s.Planes[p].Pix[y*s.Planes[p].Stride:][:widths[p]])
		}
	}
	unlock(buf, 0)
	return buf
}
