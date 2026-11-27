//go:build windows && (amd64 || arm64)

package video

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/egoist/mygo/internal/gpu/d3d11/device"
	"github.com/egoist/mygo/internal/media"
	"github.com/egoist/mygo/internal/scene"
)

// On Windows, Media Foundation plays the video: an IMFMediaEngine in
// frame-server mode, which decodes on a Direct3D 11 device of the
// engine's own (in hardware, through DXVA, where the GPU has it), plays
// the sound and keeps the clock. As each frame of the window builds, the
// engine converts the picture due into a texture in BGRA of a ring of
// three (TransferVideoFrame, a copy on the GPU), each shared with the
// window's device by an NT handle and ordered by a keyed mutex (key 0),
// which the renderer opens once and reads where it is. The engine's
// events reach Go through one COM object, made once, that wakes the
// window.

const ringSize = 3

var (
	mfOnce sync.Once
	mfErr  error

	mfplat   = syscall.NewLazyDLL(device.SystemDir() + `\mfplat.dll`)
	ole32    = syscall.NewLazyDLL(device.SystemDir() + `\ole32.dll`)
	oleaut32 = syscall.NewLazyDLL(device.SystemDir() + `\oleaut32.dll`)
	shlwapi  = syscall.NewLazyDLL(device.SystemDir() + `\shlwapi.dll`)

	procMFStartup                    = mfplat.NewProc("MFStartup")
	procMFCreateDXGIDeviceManager    = mfplat.NewProc("MFCreateDXGIDeviceManager")
	procMFCreateAttributes           = mfplat.NewProc("MFCreateAttributes")
	procMFCreateMFByteStreamOnStream = mfplat.NewProc("MFCreateMFByteStreamOnStream")
	procCoInitializeEx               = ole32.NewProc("CoInitializeEx")
	procCoCreateInstance             = ole32.NewProc("CoCreateInstance")
	procCoTaskMemAlloc               = ole32.NewProc("CoTaskMemAlloc")
	procCoTaskMemFree                = ole32.NewProc("CoTaskMemFree")
	procSysAllocString               = oleaut32.NewProc("SysAllocString")
	procSysFreeString                = oleaut32.NewProc("SysFreeString")
	procSHCreateMemStream            = shlwapi.NewProc("SHCreateMemStream")

	// notifyVtbl is the vtable of the engines' IMFMediaEngineNotify
	// objects, and engines the engines by the number each object holds.
	notifyVtbl [4]uintptr
	engines    sync.Map // uint64 → *mfEngine
	lastEngine atomic.Uint64
	lastShared atomic.Uint64
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	clsidMFMediaEngineClassFactory = guid{0xb44392da, 0x499b, 0x446b, [8]byte{0xa4, 0xcb, 0x00, 0x5f, 0xea, 0xd0, 0xe6, 0xd5}}
	iidIMFMediaEngineClassFactory  = guid{0x4d645ace, 0x26aa, 0x4688, [8]byte{0x9b, 0xe1, 0xdf, 0x35, 0x16, 0x99, 0x0b, 0x93}}
	iidIMFMediaEngineNotify        = guid{0xfee7c112, 0xe776, 0x42b5, [8]byte{0x9b, 0xbf, 0x00, 0x48, 0x52, 0x4e, 0x2b, 0xd5}}
	iidIMFMediaEngineEx            = guid{0x83015ead, 0xb1e6, 0x40d0, [8]byte{0xa9, 0x8a, 0x37, 0x14, 0x5f, 0xfe, 0x1a, 0xd1}}
	iidIUnknown                    = guid{0, 0, 0, [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidID3D10Multithread           = guid{0x9b7e4e00, 0x342c, 0x4106, [8]byte{0xa1, 0x9f, 0x4f, 0x27, 0x04, 0xf6, 0x89, 0xf0}}
	iidIDXGIKeyedMutex             = guid{0x9d8e1289, 0xd7b3, 0x465f, [8]byte{0x81, 0x26, 0x25, 0x0e, 0x34, 0x9a, 0xf8, 0x5d}}
	iidIDXGIResource1              = guid{0x30961379, 0x4609, 0x4a41, [8]byte{0x99, 0x8e, 0x54, 0xfe, 0x56, 0x7e, 0xe0, 0xc1}}
	mfMediaEngineCallback          = guid{0xc60381b8, 0x83a4, 0x41f8, [8]byte{0xa3, 0xd0, 0xde, 0x05, 0x07, 0x68, 0x49, 0xa9}}
	mfMediaEngineDXGIManager       = guid{0x065702da, 0x1094, 0x486d, [8]byte{0x86, 0x17, 0xee, 0x7c, 0xc4, 0xee, 0x46, 0x48}}
	mfMediaEngineVideoOutputFormat = guid{0x5066893c, 0x8cf9, 0x42bc, [8]byte{0x8b, 0x8a, 0x47, 0x22, 0x12, 0xe5, 0x27, 0x26}}
)

// The methods of the interfaces, by their places in the vtables.
const (
	comQueryInterface = 0
	comRelease        = 2

	attrSetUINT32  = 21
	attrSetUnknown = 27

	managerResetDevice = 7

	factoryCreateInstance = 3

	engineGetError            = 3
	engineSetSource           = 6
	engineIsSeeking           = 15
	engineGetCurrentTime      = 16
	engineSetCurrentTime      = 17
	engineGetDuration         = 19
	engineIsPaused            = 20
	engineSetDefaultRate      = 22
	engineSetPlaybackRate     = 24
	engineIsEnded             = 27
	engineSetLoop             = 31
	enginePlay                = 32
	enginePause               = 33
	engineSetMuted            = 35
	engineSetVolume           = 37
	engineGetNativeVideoSize  = 40
	engineShutdown            = 42
	engineTransferVideoFrame  = 43
	engineOnVideoStreamTick   = 44
	engineExSetSourceFromByte = 45
	engineExFrameStep         = 51

	errorGetErrorCode    = 3
	errorGetExtendedCode = 4

	multithreadSetProtected = 5

	devCreateTexture2D     = 5
	devGetImmediateContext = 40
	ctxMap                 = 14
	ctxUnmap               = 15
	ctxCopyResource        = 47

	keyedAcquireSync = 8
	keyedReleaseSync = 9

	resourceCreateSharedHandle = 13
)

// Media engine events (MF_MEDIA_ENGINE_EVENT).
const (
	eventError          = 5
	eventLoadedMetadata = 10
	eventLoadedData     = 11
	eventWaiting        = 12
	eventPlaying        = 13
	eventCanPlay        = 14
	eventSeeked         = 17
	eventEnded          = 19
	eventDurationChange = 21

	eventFrameStepCompleted = 1007
)

type texture2DDesc struct {
	Width, Height, MipLevels, ArraySize, Format, SampleCount, SampleQuality, Usage, BindFlags, CPUAccessFlags, MiscFlags uint32
}

func failed(hr uintptr) bool { return int32(uint32(hr)) < 0 }

// cptr is the pointer of an address of native memory.
func cptr(a uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&a)) }

// call calls method i of a COM object, returning its HRESULT, and what
// the call left in the first floating point register on amd64. Pointers
// converted to uintptr in the call stay valid until it returns
// (go:uintptrescapes).
//
//go:uintptrescapes
func call(obj uintptr, i int, args ...uintptr) uintptr {
	r, _ := callRaw(obj, i, args...)
	return r
}

func callRaw(obj uintptr, i int, args ...uintptr) (uintptr, uintptr) {
	fn := method(obj, i)
	a := make([]uintptr, 0, len(args)+1)
	a = append(a, obj)
	a = append(a, args...)
	r1, r2, _ := syscall.SyscallN(fn, a...)
	return r1, r2
}

func method(obj uintptr, i int) uintptr {
	vtbl := *(*uintptr)(cptr(obj))
	return *(*uintptr)(cptr(vtbl + uintptr(i)*unsafe.Sizeof(uintptr(0))))
}

func free(p *uintptr) {
	if *p != 0 {
		call(*p, comRelease)
		*p = 0
	}
}

func loadMediaFoundation() error {
	mfOnce.Do(func() {
		for _, p := range []*syscall.LazyProc{procMFStartup, procMFCreateDXGIDeviceManager, procMFCreateAttributes, procMFCreateMFByteStreamOnStream, procCoCreateInstance, procSysAllocString, procSHCreateMemStream} {
			if err := p.Find(); err != nil {
				// Windows Server without its Media Foundation feature, or
				// an N edition without the Media Feature Pack.
				mfErr = fmt.Errorf("%w: %v", ErrUnsupported, err)
				return
			}
		}
		const mfVersion, mfStartupFull = 0x00020070, 0
		if hr, _, _ := procMFStartup.Call(mfVersion, mfStartupFull); failed(hr) {
			mfErr = fmt.Errorf("%w: MFStartup: %#x", ErrUnsupported, uint32(hr))
			return
		}
		if err := loadFloatCalls(); err != nil {
			mfErr = err
			return
		}
		notifyVtbl = [4]uintptr{
			syscall.NewCallback(notifyQueryInterface),
			syscall.NewCallback(notifyAddRef),
			syscall.NewCallback(notifyAddRef), // Release: the engine frees it
			syscall.NewCallback(notifyEvent),
		}
	})
	return mfErr
}

func notifyQueryInterface(this, riid, out uintptr) uintptr {
	g := *(*guid)(cptr(riid))
	if g == iidIUnknown || g == iidIMFMediaEngineNotify {
		*(*uintptr)(cptr(out)) = this
		return 0
	}
	*(*uintptr)(cptr(out)) = 0
	return 0x80004002 // E_NOINTERFACE
}

func notifyAddRef(this uintptr) uintptr { return 1 }

func notifyEvent(this, event, param1, param2 uintptr) uintptr {
	id := *(*uint64)(cptr(this + unsafe.Sizeof(uintptr(0))))
	if v, ok := engines.Load(id); ok {
		v.(*mfEngine).event(uint32(event), param1, uint32(param2))
	}
	return 0
}

// mfEngine plays a video with an IMFMediaEngine.
type mfEngine struct {
	id                    uint64
	engine, ex            uintptr // IMFMediaEngine, IMFMediaEngineEx
	notify                uintptr // the IMFMediaEngineNotify, CoTaskMemAlloc'd
	dev, ctx, manager, mt uintptr
	staging               uintptr // for reading a picture back (RGBA)
	stagingW, stagingH    int

	// ring are the shared textures the engine converts pictures into:
	// shown the one Frame last returned, -1 for none.
	ring  [ringSize]sharedFrame
	shown int
	w, h  int
	// pictured tells a new picture came since the last seek; reseek
	// that Frame seeks again, to what Seek asked for, once the step it
	// took is done, and resought how often it did since Seek.
	pictured, reseek bool
	resought         int

	mu        sync.Mutex
	wake      func()
	state     media.State
	err       error
	loop      bool
	seekTo    time.Duration
	seekSince time.Time // a seek not landed yet, until eventSeeked
	// landed tells a seek landed (eventSeeked) with no picture since
	// it, for Frame to step to the picture there if the engine has none.
	landed bool
	// stepped tells a frame step completed since Frame took one.
	stepped bool
	closed  bool
}

// sharedFrame is a texture of the ring, and its surface.
type sharedFrame struct {
	tex, mutex uintptr
	surface    *scene.Surface
	w, h       int
}

func newEngine(s source) (media.Engine, error) {
	if err := loadMediaFoundation(); err != nil {
		return nil, err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// COM, for CoCreateInstance on this thread: the app's thread may
	// have it already, in its own mode, which serves.
	procCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED

	e := &mfEngine{id: lastEngine.Add(1), shown: -1}
	ok := false
	defer func() {
		if !ok {
			e.release()
		}
	}()
	var err error
	if e.dev, e.ctx, _, err = device.NewForVideo(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	// The engine decodes on the device from threads of its own.
	if !failed(call(e.dev, comQueryInterface, uintptr(unsafe.Pointer(&iidID3D10Multithread)), uintptr(unsafe.Pointer(&e.mt)))) {
		call(e.mt, multithreadSetProtected, 1)
	}
	var token uint32
	if hr, _, _ := procMFCreateDXGIDeviceManager.Call(uintptr(unsafe.Pointer(&token)), uintptr(unsafe.Pointer(&e.manager))); failed(hr) {
		return nil, fmt.Errorf("video: MFCreateDXGIDeviceManager: %#x", uint32(hr))
	}
	if hr := call(e.manager, managerResetDevice, e.dev, uintptr(token)); failed(hr) {
		return nil, fmt.Errorf("video: IMFDXGIDeviceManager.ResetDevice: %#x", uint32(hr))
	}
	n, _, _ := procCoTaskMemAlloc.Call(2 * unsafe.Sizeof(uintptr(0)))
	if n == 0 {
		return nil, errors.New("video: out of memory")
	}
	e.notify = n
	*(*uintptr)(cptr(n)) = uintptr(unsafe.Pointer(&notifyVtbl))
	*(*uint64)(cptr(n + unsafe.Sizeof(uintptr(0)))) = e.id
	engines.Store(e.id, e)

	var attrs, factory uintptr
	if hr, _, _ := procMFCreateAttributes.Call(uintptr(unsafe.Pointer(&attrs)), 3); failed(hr) {
		return nil, fmt.Errorf("video: MFCreateAttributes: %#x", uint32(hr))
	}
	defer free(&attrs)
	call(attrs, attrSetUnknown, uintptr(unsafe.Pointer(&mfMediaEngineCallback)), e.notify)
	call(attrs, attrSetUnknown, uintptr(unsafe.Pointer(&mfMediaEngineDXGIManager)), e.manager)
	const formatB8G8R8A8Unorm = 87
	call(attrs, attrSetUINT32, uintptr(unsafe.Pointer(&mfMediaEngineVideoOutputFormat)), formatB8G8R8A8Unorm)
	const clsctxInprocServer = 1
	if hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidMFMediaEngineClassFactory)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidIMFMediaEngineClassFactory)), uintptr(unsafe.Pointer(&factory))); failed(hr) {
		return nil, fmt.Errorf("%w: no media engine: %#x", ErrUnsupported, uint32(hr))
	}
	defer free(&factory)
	if hr := call(factory, factoryCreateInstance, 0, attrs, uintptr(unsafe.Pointer(&e.engine))); failed(hr) {
		return nil, fmt.Errorf("video: IMFMediaEngineClassFactory.CreateInstance: %#x", uint32(hr))
	}
	call(e.engine, comQueryInterface, uintptr(unsafe.Pointer(&iidIMFMediaEngineEx)), uintptr(unsafe.Pointer(&e.ex)))

	if s.data != nil {
		if err := e.setData(s.data); err != nil {
			return nil, err
		}
	} else {
		// Media Foundation reads the percent-escapes of a file: URL as
		// bytes of the system's code page, not UTF-8, so a file whose path
		// is not ASCII goes by its path, which SetSource takes as well.
		src := s.url
		if s.path != "" {
			src = s.path
		}
		url := bstr(src)
		hr := call(e.engine, engineSetSource, url)
		procSysFreeString.Call(url)
		if failed(hr) {
			return nil, fmt.Errorf("video: cannot open %s: %#x", s.url, uint32(hr))
		}
	}
	ok = true
	return e, nil
}

// setData has the engine read the video from bytes in memory, through a
// stream in memory: no file.
func (e *mfEngine) setData(data []byte) error {
	if e.ex == 0 {
		return fmt.Errorf("%w: no IMFMediaEngineEx", ErrUnsupported)
	}
	stream, _, _ := procSHCreateMemStream.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	if stream == 0 {
		return errors.New("video: out of memory")
	}
	defer free(&stream)
	var bs uintptr
	if hr, _, _ := procMFCreateMFByteStreamOnStream.Call(stream, uintptr(unsafe.Pointer(&bs))); failed(hr) {
		return fmt.Errorf("video: MFCreateMFByteStreamOnStream: %#x", uint32(hr))
	}
	defer free(&bs)
	// The URL names only the type, for the engine to choose its source.
	name := "data.mp4"
	if len(data) >= 12 && string(data[4:8]) == "ftyp" && string(data[8:12]) == "qt  " {
		name = "data.mov"
	} else if len(data) >= 4 && string(data[:4]) == "\x1a\x45\xdf\xa3" {
		name = "data.webm"
	}
	url := bstr(name)
	defer procSysFreeString.Call(url)
	if hr := call(e.ex, engineExSetSourceFromByte, bs, url); failed(hr) {
		return fmt.Errorf("video: cannot open the video: %#x", uint32(hr))
	}
	return nil
}

func bstr(s string) uintptr {
	p, _ := syscall.UTF16PtrFromString(s)
	b, _, _ := procSysAllocString.Call(uintptr(unsafe.Pointer(p)))
	return b
}

// event takes an event of the engine, on a thread of Media Foundation's.
func (e *mfEngine) event(event uint32, param1 uintptr, param2 uint32) {
	e.mu.Lock()
	switch event {
	case eventError:
		e.state = media.Failed
		e.err = fmt.Errorf("video: Media Foundation could not play the video (error %d, %#x)", param1, param2)
	case eventEnded:
		if !e.loop {
			e.state = media.Ended
		}
	case eventWaiting:
		if e.state == media.Ready {
			e.state = media.Buffering
		}
	case eventPlaying, eventCanPlay:
		if e.state == media.Buffering {
			e.state = media.Ready
		}
	case eventSeeked:
		e.seekSince = time.Time{}
		e.landed = true
	case eventFrameStepCompleted:
		e.stepped = true
	case eventLoadedMetadata, eventLoadedData, eventDurationChange:
	default:
		e.mu.Unlock()
		return
	}
	wake := e.wake
	e.mu.Unlock()
	if wake != nil {
		wake()
	}
}

func (e *mfEngine) SetWake(wake func()) {
	e.mu.Lock()
	e.wake = wake
	e.mu.Unlock()
}

func (e *mfEngine) Frame(time.Time) (media.Picture, bool, bool) {
	changed := false
	var pts int64
	fresh := call(e.engine, engineOnVideoStreamTick, uintptr(unsafe.Pointer(&pts))) == 0 // S_OK: a new picture
	e.mu.Lock()
	landed, to, stepped := e.landed, e.seekTo, e.stepped
	e.mu.Unlock()
	if landed && (fresh || call(e.engine, engineIsSeeking) == 0) {
		e.mu.Lock()
		e.landed = false
		e.mu.Unlock()
		if !fresh && !e.pictured && e.ex != 0 && call(e.engine, engineIsPaused) != 0 {
			// A seek of a paused video landed with no picture, which then
			// never comes (Media Foundation on Windows on Arm, about one
			// seek in three): a step forward shows the one there, and
			// keeps the position. A seek just after a pause, as the video
			// starts playing, may also land where the video was (about one
			// in ten), and seeking again at once lands there too: once the
			// step is done, Frame seeks again, up to three times, and
			// shows none of the pictures from where the video was.
			if at := callGetDouble(e.engine, engineGetCurrentTime); e.resought < 3 && math.Abs(at-to.Seconds()) > 0.001 {
				e.reseek = true
				e.resought++
			}
			e.mu.Lock()
			e.stepped, stepped = false, false
			e.mu.Unlock()
			call(e.ex, engineExFrameStep, 1)
		}
	}
	if e.reseek {
		if stepped {
			e.reseek = false
			e.seek(to)
		}
	} else if fresh {
		e.pictured = true
		var w, h uint32
		call(e.engine, engineGetNativeVideoSize, uintptr(unsafe.Pointer(&w)), uintptr(unsafe.Pointer(&h)))
		if w > 0 && h > 0 {
			i := (e.shown + 1) % ringSize
			if e.transfer(i, int(w), int(h)) {
				e.shown, changed = i, true
				e.w, e.h = int(w), int(h)
				e.mu.Lock()
				if e.state == media.Loading {
					e.state = media.Ready
				}
				e.mu.Unlock()
			}
		}
	}
	if e.shown < 0 {
		return media.Picture{}, false, false
	}
	f := &e.ring[e.shown]
	i := e.shown
	return media.Picture{Surface: f.surface, W: f.w, H: f.h, RGBA: func(dst []byte) []byte { return e.readBack(i, dst) }}, changed, true
}

// transfer converts the picture due into texture i of the ring, which it
// makes, or makes again at another size, under its keyed mutex.
func (e *mfEngine) transfer(i, w, h int) bool {
	f := &e.ring[i]
	if f.tex == 0 || f.w != w || f.h != h {
		e.freeFrame(f)
		if !e.newShared(f, w, h) {
			return false
		}
	}
	const acquireTimeout = 100 // ms: the renderer reads the texture meanwhile
	if hr := call(f.mutex, keyedAcquireSync, 0, acquireTimeout); failed(hr) || hr == 0x102 {
		return false
	}
	rect := [4]int32{0, 0, int32(w), int32(h)}
	border := uint32(0xff000000) // opaque black, MFARGB
	hr := call(e.engine, engineTransferVideoFrame, f.tex, 0, uintptr(unsafe.Pointer(&rect)), uintptr(unsafe.Pointer(&border)))
	call(f.mutex, keyedReleaseSync, 0)
	return !failed(hr)
}

// newShared makes f a w×h texture in BGRA that the engine converts into
// and the window's device opens, by an NT handle, with a keyed mutex.
func (e *mfEngine) newShared(f *sharedFrame, w, h int) bool {
	const (
		formatB8G8R8A8Unorm  = 87
		usageDefault         = 0
		bindShaderResource   = 0x8
		bindRenderTarget     = 0x20
		miscSharedKeyedMutex = 0x100
		miscSharedNTHandle   = 0x800
		sharedRead           = 0x80000000
		sharedWrite          = 1
	)
	desc := texture2DDesc{Width: uint32(w), Height: uint32(h), MipLevels: 1, ArraySize: 1, Format: formatB8G8R8A8Unorm, SampleCount: 1,
		Usage: usageDefault, BindFlags: bindShaderResource | bindRenderTarget, MiscFlags: miscSharedKeyedMutex | miscSharedNTHandle}
	if failed(call(e.dev, devCreateTexture2D, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&f.tex)))) {
		f.tex = 0
		return false
	}
	var res, handle uintptr
	if failed(call(f.tex, comQueryInterface, uintptr(unsafe.Pointer(&iidIDXGIKeyedMutex)), uintptr(unsafe.Pointer(&f.mutex)))) ||
		failed(call(f.tex, comQueryInterface, uintptr(unsafe.Pointer(&iidIDXGIResource1)), uintptr(unsafe.Pointer(&res)))) {
		e.freeFrame(f)
		return false
	}
	hr := call(res, resourceCreateSharedHandle, 0, sharedRead|sharedWrite, 0, uintptr(unsafe.Pointer(&handle)))
	free(&res)
	if failed(hr) {
		e.freeFrame(f)
		return false
	}
	f.w, f.h = w, h
	f.surface = &scene.Surface{Format: scene.SurfaceBGRA, Native: handle, NativeID: lastShared.Add(1)}
	// The handle stays open while anything shows the surface: a renderer
	// opens it as it first draws it.
	runtime.AddCleanup(f.surface, func(h uintptr) { syscall.CloseHandle(syscall.Handle(h)) }, handle)
	return true
}

func (e *mfEngine) freeFrame(f *sharedFrame) {
	free(&f.mutex)
	free(&f.tex)
	f.surface = nil
}

// readBack copies texture i of the ring into premultiplied RGBA, for the
// CPU renderer, through a staging texture.
func (e *mfEngine) readBack(i int, dst []byte) []byte {
	f := &e.ring[i]
	if e.closed || f.tex == 0 {
		return nil
	}
	w, h := f.w, f.h
	if e.staging == 0 || e.stagingW != w || e.stagingH != h {
		free(&e.staging)
		const usageStaging, cpuAccessRead = 3, 0x20000
		desc := texture2DDesc{Width: uint32(w), Height: uint32(h), MipLevels: 1, ArraySize: 1, Format: 87, SampleCount: 1,
			Usage: usageStaging, CPUAccessFlags: cpuAccessRead}
		if failed(call(e.dev, devCreateTexture2D, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&e.staging)))) {
			e.staging = 0
			return nil
		}
		e.stagingW, e.stagingH = w, h
	}
	if hr := call(f.mutex, keyedAcquireSync, 0, 1000); failed(hr) || hr == 0x102 {
		return nil
	}
	call(e.ctx, ctxCopyResource, e.staging, f.tex)
	call(f.mutex, keyedReleaseSync, 0)
	var mapped struct {
		data               uintptr
		rowPitch, depthPit uint32
	}
	const mapRead = 1
	if failed(call(e.ctx, ctxMap, e.staging, 0, mapRead, 0, uintptr(unsafe.Pointer(&mapped)))) {
		return nil
	}
	defer call(e.ctx, ctxUnmap, e.staging, 0)
	n := 4 * w * h
	if cap(dst) < n {
		dst = make([]byte, n)
	}
	dst = dst[:n]
	src := unsafe.Slice((*byte)(cptr(mapped.data)), int(mapped.rowPitch)*h)
	for y := range h {
		row, out := src[y*int(mapped.rowPitch):][:4*w], dst[4*w*y:][:4*w]
		for x := 0; x < 4*w; x += 4 {
			out[x], out[x+1], out[x+2], out[x+3] = row[x+2], row[x+1], row[x], 255
		}
	}
	return dst
}

func (e *mfEngine) Interval() time.Duration { return time.Second / 60 }

func (e *mfEngine) State() (media.State, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state, e.err
}

func (e *mfEngine) Size() (int, int) { return e.w, e.h }

func (e *mfEngine) Duration() time.Duration {
	d := callGetDouble(e.engine, engineGetDuration)
	if math.IsNaN(d) || math.IsInf(d, 0) || d <= 0 {
		return 0
	}
	return time.Duration(d * float64(time.Second))
}

func (e *mfEngine) Position() time.Duration {
	e.mu.Lock()
	if !e.seekSince.IsZero() {
		d := e.seekTo
		e.mu.Unlock()
		return d
	}
	e.mu.Unlock()
	d := callGetDouble(e.engine, engineGetCurrentTime)
	if math.IsNaN(d) || d < 0 {
		return 0
	}
	return time.Duration(d * float64(time.Second))
}

func (e *mfEngine) Play() {
	e.mu.Lock()
	if e.state == media.Ended {
		e.state = media.Ready
	}
	e.mu.Unlock()
	call(e.engine, enginePlay)
}

func (e *mfEngine) Pause() { call(e.engine, enginePause) }

func (e *mfEngine) Seek(d time.Duration) {
	e.reseek, e.resought = false, 0
	e.seek(d)
}

func (e *mfEngine) seek(d time.Duration) {
	e.mu.Lock()
	if e.state == media.Ended {
		e.state = media.Ready
	}
	e.seekTo, e.seekSince, e.landed = d, time.Now(), false
	e.mu.Unlock()
	e.pictured = false
	callSetDouble(e.engine, engineSetCurrentTime, d.Seconds())
}

func (e *mfEngine) SetLoop(loop bool) {
	e.mu.Lock()
	e.loop = loop
	e.mu.Unlock()
	call(e.engine, engineSetLoop, boolArg(loop))
}

func (e *mfEngine) SetRate(r float64) {
	callSetDouble(e.engine, engineSetDefaultRate, r)
	callSetDouble(e.engine, engineSetPlaybackRate, r)
}

func (e *mfEngine) SetVolume(v float64) { callSetDouble(e.engine, engineSetVolume, v) }

func (e *mfEngine) SetMuted(m bool) { call(e.engine, engineSetMuted, boolArg(m)) }

func boolArg(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

// Close shuts the engine down and frees what it made. The handles of the
// shared textures close once nothing shows their surfaces.
func (e *mfEngine) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	e.wake = nil
	e.mu.Unlock()
	if e.engine != 0 {
		call(e.engine, engineShutdown)
	}
	e.release()
	return nil
}

func (e *mfEngine) release() {
	free(&e.ex)
	free(&e.engine)
	engines.Delete(e.id)
	if e.notify != 0 {
		procCoTaskMemFree.Call(e.notify)
		e.notify = 0
	}
	for i := range e.ring {
		e.freeFrame(&e.ring[i])
	}
	free(&e.staging)
	free(&e.manager)
	free(&e.mt)
	free(&e.ctx)
	free(&e.dev)
}
