//go:build windows

package render

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo/internal/gpu/d3d11/device"
)

// On Windows, a Media Foundation sink writer writes the video, into an
// MP4: it takes the frames in RGB32 (BGRA), converts them with the video
// processor and encodes them with the system's H.264 or HEVC encoder, the
// GPU's where it has one (MF_READWRITE_ENABLE_HARDWARE_TRANSFORMS), and
// the sound track in PCM, which its AAC encoder encodes.

var (
	mfOnce sync.Once
	mfErr  error

	mfplat      = syscall.NewLazyDLL(device.SystemDir() + `\mfplat.dll`)
	mfreadwrite = syscall.NewLazyDLL(device.SystemDir() + `\mfreadwrite.dll`)
	ole32       = syscall.NewLazyDLL(device.SystemDir() + `\ole32.dll`)

	procMFStartup                 = mfplat.NewProc("MFStartup")
	procMFCreateAttributes        = mfplat.NewProc("MFCreateAttributes")
	procMFCreateMediaType         = mfplat.NewProc("MFCreateMediaType")
	procMFCreateSample            = mfplat.NewProc("MFCreateSample")
	procMFCreateMemoryBuffer      = mfplat.NewProc("MFCreateMemoryBuffer")
	procMFCreateSinkWriterFromURL = mfreadwrite.NewProc("MFCreateSinkWriterFromURL")
	procCoInitializeEx            = ole32.NewProc("CoInitializeEx")
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// mfGUID returns the GUID of a Media Foundation format, from its FOURCC
// or D3DFORMAT: {xxxxxxxx-0000-0010-8000-00AA00389B71}.
func mfGUID(fourcc uint32) guid {
	return guid{fourcc, 0, 0x10, [8]byte{0x80, 0, 0, 0xaa, 0, 0x38, 0x9b, 0x71}}
}

var (
	mfMTMajorType          = guid{0x48eba18e, 0xf8c9, 0x4687, [8]byte{0xbf, 0x11, 0x0a, 0x74, 0xc9, 0xf9, 0x6a, 0x8f}}
	mfMTSubtype            = guid{0xf7e34c9a, 0x42e8, 0x4714, [8]byte{0xb7, 0x4b, 0xcb, 0x29, 0xd7, 0x2c, 0x35, 0xe5}}
	mfMTAvgBitrate         = guid{0x20332624, 0xfb0d, 0x4d9e, [8]byte{0xbd, 0x0d, 0xcb, 0xf6, 0x78, 0x6c, 0x10, 0x2e}}
	mfMTInterlaceMode      = guid{0xe2724bb8, 0xe676, 0x4806, [8]byte{0xb4, 0xb2, 0xa8, 0xd6, 0xef, 0xb4, 0x4c, 0xcd}}
	mfMTFrameSize          = guid{0x1652c33d, 0xd6b2, 0x4012, [8]byte{0xb8, 0x34, 0x72, 0x03, 0x08, 0x49, 0xa3, 0x7d}}
	mfMTFrameRate          = guid{0xc459a2e8, 0x3d2c, 0x4e44, [8]byte{0xb1, 0x32, 0xfe, 0xe5, 0x15, 0x6c, 0x7b, 0xb0}}
	mfMTPixelAspectRatio   = guid{0xc6376a1e, 0x8d0a, 0x4027, [8]byte{0xbe, 0x45, 0x6d, 0x9a, 0x0a, 0xd3, 0x9b, 0xb6}}
	mfMTDefaultStride      = guid{0x644b4e48, 0x1e02, 0x4516, [8]byte{0xb0, 0xeb, 0xc0, 0x1c, 0xa9, 0xd4, 0x9a, 0xc6}}
	mfMTMPEG2Profile       = guid{0xad76a80b, 0x2d5c, 0x4e0b, [8]byte{0xb3, 0x75, 0x64, 0xe5, 0x20, 0x13, 0x70, 0x36}}
	mfMTVideoPrimaries     = guid{0xdbfbe4d7, 0x0740, 0x4ee0, [8]byte{0x81, 0x92, 0x85, 0x0a, 0xb0, 0xe2, 0x19, 0x35}}
	mfMTTransferFunction   = guid{0x5fb0fce9, 0xbe5c, 0x4935, [8]byte{0xa8, 0x11, 0xec, 0x83, 0x8f, 0x8e, 0xed, 0x93}}
	mfMTYUVMatrix          = guid{0x3e23d450, 0x2c75, 0x4d25, [8]byte{0xa0, 0x0e, 0xb9, 0x16, 0x70, 0xd1, 0x23, 0x27}}
	mfMTNominalRange       = guid{0xc21b8ee5, 0xb956, 0x4071, [8]byte{0x8d, 0xaf, 0x32, 0x5e, 0xdf, 0x5c, 0xab, 0x11}}
	mfMTAudioBitsPerSample = guid{0xf2deb57f, 0x40fa, 0x4764, [8]byte{0xaa, 0x33, 0xed, 0x4f, 0x2d, 0x1f, 0xf6, 0x69}}
	mfMTAudioSampleRate    = guid{0x5faeeae7, 0x0290, 0x4c31, [8]byte{0x9e, 0x8a, 0xc5, 0x34, 0xf6, 0x8d, 0x9d, 0xba}}
	mfMTAudioChannels      = guid{0x37e48bf5, 0x645e, 0x4c5b, [8]byte{0x89, 0xde, 0xad, 0xa9, 0xe2, 0x9b, 0x69, 0x6a}}
	mfMTAudioBytesPerSec   = guid{0x1aab75c8, 0xcfef, 0x451c, [8]byte{0xab, 0x95, 0xac, 0x03, 0x4b, 0x8e, 0x17, 0x31}}
	mfMTAudioBlockAlign    = guid{0x322de230, 0x9eeb, 0x43bd, [8]byte{0xab, 0x7a, 0xff, 0x41, 0x22, 0x51, 0x54, 0x1d}}
	mfReadwriteHardware    = guid{0xa634a91c, 0x822b, 0x41b9, [8]byte{0xa4, 0x94, 0x4d, 0xe4, 0x64, 0x36, 0x12, 0xb0}}
	mfTranscodeContainer   = guid{0x150ff23f, 0x4abc, 0x478b, [8]byte{0xac, 0x4f, 0xe1, 0x91, 0x6f, 0xba, 0x1c, 0xca}}
	mfContainerMPEG4       = guid{0xdc6cd05d, 0xb9d0, 0x40ef, [8]byte{0xbd, 0x35, 0xfa, 0x62, 0x2c, 0x1a, 0xb2, 0x8a}}

	mfMediaTypeVideo  = mfGUID(0x73646976) // 'vids'
	mfMediaTypeAudio  = mfGUID(0x73647561) // 'auds'
	mfVideoFormatH264 = mfGUID(0x34363248) // 'H264'
	mfVideoFormatHEVC = mfGUID(0x43564548) // 'HEVC'
	mfVideoFormatNV12 = mfGUID(0x3231564e) // 'NV12'
	mfAudioFormatAAC  = mfGUID(0x1610)     // WAVE_FORMAT_MPEG_HEAAC
	mfAudioFormatPCM  = mfGUID(1)
)

// The methods of the interfaces, by their places in the vtables.
const (
	comRelease = 2

	attrSetUINT32 = 21
	attrSetUINT64 = 22
	attrSetGUID   = 24

	writerAddStream         = 3
	writerSetInputMediaType = 4
	writerBeginWriting      = 5
	writerWriteSample       = 6
	writerFinalize          = 11

	sampleSetTime     = 36
	sampleSetDuration = 38
	sampleAddBuffer   = 42

	bufferLock             = 3
	bufferUnlock           = 4
	bufferSetCurrentLength = 6
)

// mfCodecNotFound is MF_E_TOPO_CODEC_NOT_FOUND: no encoder of the format.
const mfCodecNotFound = 0xc00d5212

func failed(hr uintptr) bool { return int32(uint32(hr)) < 0 }

func cptr(a uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&a)) }

// call calls method i of COM object obj. Pointers converted to uintptr in
// the call stay valid until it returns (go:uintptrescapes).
//
//go:uintptrescapes
func call(obj uintptr, i int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(cptr(obj))
	fn := *(*uintptr)(cptr(vtbl + uintptr(i)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
	return r
}

func free(p *uintptr) {
	if *p != 0 {
		call(*p, comRelease)
		*p = 0
	}
}

func loadMF() error {
	mfOnce.Do(func() {
		for _, p := range []*syscall.LazyProc{procMFStartup, procMFCreateAttributes, procMFCreateMediaType, procMFCreateSample,
			procMFCreateMemoryBuffer, procMFCreateSinkWriterFromURL, procCoInitializeEx} {
			if err := p.Find(); err != nil {
				// Windows Server without its Media Foundation feature, or
				// an N edition without the Media Feature Pack.
				mfErr = fmt.Errorf("render: %w: no Media Foundation (%v): encode with Options{Encoder: render.FFmpeg}", errors.ErrUnsupported, err)
				return
			}
		}
		const mfVersion, mfStartupFull = 0x00020070, 0
		if hr, _, _ := procMFStartup.Call(mfVersion, mfStartupFull); failed(hr) {
			mfErr = fmt.Errorf("render: %w: MFStartup: %#x", errors.ErrUnsupported, uint32(hr))
		}
	})
	return mfErr
}

// attrs sets attributes of an IMFAttributes: GUID values, uint32s and
// uint64s, by their keys.
type attr struct {
	key   *guid
	guid  *guid
	value uint64
	wide  bool
}

func setAttrs(obj uintptr, as ...attr) error {
	for _, a := range as {
		var hr uintptr
		switch {
		case a.guid != nil:
			hr = call(obj, attrSetGUID, uintptr(unsafe.Pointer(a.key)), uintptr(unsafe.Pointer(a.guid)))
		case a.wide:
			hr = call(obj, attrSetUINT64, uintptr(unsafe.Pointer(a.key)), uintptr(a.value))
		default:
			hr = call(obj, attrSetUINT32, uintptr(unsafe.Pointer(a.key)), uintptr(uint32(a.value)))
		}
		if failed(hr) {
			return fmt.Errorf("render: Media Foundation: an attribute: %#x", uint32(hr))
		}
	}
	return nil
}

func mediaType(as ...attr) (uintptr, error) {
	var t uintptr
	if hr, _, _ := procMFCreateMediaType.Call(uintptr(unsafe.Pointer(&t))); failed(hr) {
		return 0, fmt.Errorf("render: Media Foundation: no media type: %#x", uint32(hr))
	}
	if err := setAttrs(t, as...); err != nil {
		free(&t)
		return 0, err
	}
	return t, nil
}

// mfSink writes a video with a Media Foundation sink writer.
type mfSink struct {
	spec         videoSpec
	writer       uintptr
	video, audio uint32
	// n is the number of the next frame from the first, counting those
	// held (hold); samples the sound's frames written.
	n, samples int64
	// pending is the sample of the last frame, written once the next that
	// shows something else comes, or the end, with the sound up to then;
	// buf is its buffer, and at its number. A sample lasts a frame: the
	// writer stamps one lasting longer at its end, less a frame, and holds
	// the frame before it to the next one's time itself.
	pending, buf uintptr
	at           int64
	hasSound     bool
}

func newVideoSink(spec videoSpec, _ drawer) (sink, error) {
	if spec.container != mp4 {
		return nil, fmt.Errorf("render: %w: Media Foundation writes MP4 alone: encode a .mov or .webm with Options{Encoder: render.FFmpeg}", errors.ErrUnsupported)
	}
	if spec.codec != H264 && spec.codec != HEVC {
		return nil, fmt.Errorf("render: %w: Media Foundation has no %v encoder: encode it with Options{Encoder: render.FFmpeg}", errors.ErrUnsupported, spec.codec)
	}
	if err := loadMF(); err != nil {
		return nil, err
	}
	const coinitMultithreaded = 0
	procCoInitializeEx.Call(0, coinitMultithreaded)
	s := &mfSink{spec: spec, hasSound: spec.sound != nil}
	if err := s.open(); err != nil {
		s.abort()
		return nil, err
	}
	return s, nil
}

func (s *mfSink) open() error {
	sp := s.spec
	var attrs uintptr
	if hr, _, _ := procMFCreateAttributes.Call(uintptr(unsafe.Pointer(&attrs)), 2); failed(hr) {
		return fmt.Errorf("render: Media Foundation: no attributes: %#x", uint32(hr))
	}
	defer free(&attrs)
	if err := setAttrs(attrs, attr{key: &mfReadwriteHardware, value: 1}, attr{key: &mfTranscodeContainer, guid: &mfContainerMPEG4}); err != nil {
		return err
	}
	path, err := syscall.UTF16PtrFromString(sp.path)
	if err != nil {
		return err
	}
	if hr, _, _ := procMFCreateSinkWriterFromURL.Call(uintptr(unsafe.Pointer(path)), 0, attrs, uintptr(unsafe.Pointer(&s.writer))); failed(hr) {
		return fmt.Errorf("render: Media Foundation cannot write %s: %#x", sp.path, uint32(hr))
	}
	const progressive, bt709Primaries, bt709Transfer, bt709Matrix, range16to235, h264High, hevcMain = 2, 2, 5, 1, 2, 100, 1
	format, profile := &mfVideoFormatH264, uint64(h264High)
	if sp.codec == HEVC {
		format, profile = &mfVideoFormatHEVC, hevcMain
	}
	size := uint64(sp.w)<<32 | uint64(sp.h)
	rate := uint64(sp.num)<<32 | uint64(sp.den)
	common := []attr{
		{key: &mfMTMajorType, guid: &mfMediaTypeVideo},
		{key: &mfMTInterlaceMode, value: progressive},
		{key: &mfMTFrameSize, value: size, wide: true},
		{key: &mfMTFrameRate, value: rate, wide: true},
		{key: &mfMTPixelAspectRatio, value: 1<<32 | 1, wide: true},
	}
	out, err := mediaType(append(common,
		attr{key: &mfMTSubtype, guid: format},
		attr{key: &mfMTAvgBitrate, value: uint64(sp.bitrate)},
		attr{key: &mfMTMPEG2Profile, value: profile},
		attr{key: &mfMTVideoPrimaries, value: bt709Primaries},
		attr{key: &mfMTTransferFunction, value: bt709Transfer},
		attr{key: &mfMTYUVMatrix, value: bt709Matrix},
		attr{key: &mfMTNominalRange, value: range16to235})...)
	if err != nil {
		return err
	}
	defer free(&out)
	if hr := call(s.writer, writerAddStream, out, uintptr(unsafe.Pointer(&s.video))); failed(hr) {
		if uint32(hr) == mfCodecNotFound {
			return fmt.Errorf("render: %w: Windows has no %v encoder: encode it with Options{Encoder: render.FFmpeg}", errors.ErrUnsupported, sp.codec)
		}
		return fmt.Errorf("render: Media Foundation has no %v encoder of %d×%d: %#x", sp.codec, sp.w, sp.h, uint32(hr))
	}
	// The frames come in NV12 of BT.709, converted in Go (bgraToNV12), as
	// the encoder takes them: the sink writer's own conversion of RGB32
	// takes BT.601's matrix.
	in, err := mediaType(append(common,
		attr{key: &mfMTSubtype, guid: &mfVideoFormatNV12},
		attr{key: &mfMTDefaultStride, value: uint64(sp.w)},
		attr{key: &mfMTVideoPrimaries, value: bt709Primaries},
		attr{key: &mfMTTransferFunction, value: bt709Transfer},
		attr{key: &mfMTYUVMatrix, value: bt709Matrix},
		attr{key: &mfMTNominalRange, value: range16to235})...)
	if err != nil {
		return err
	}
	defer free(&in)
	if hr := call(s.writer, writerSetInputMediaType, uintptr(s.video), in, 0); failed(hr) {
		if uint32(hr) == mfCodecNotFound {
			return fmt.Errorf("render: %w: Windows has no %v encoder (for HEVC, the HEVC Video Extensions): encode it with Options{Encoder: render.FFmpeg}", errors.ErrUnsupported, sp.codec)
		}
		return fmt.Errorf("render: Media Foundation's %v encoder takes no frames of %d×%d: %#x", sp.codec, sp.w, sp.h, uint32(hr))
	}
	if s.hasSound {
		// The AAC encoder takes 96, 128, 160 or 192 kb/s.
		bytes := uint64(24000)
		for _, b := range []uint64{12000, 16000, 20000} {
			if uint64(sp.audioBitrate)/8 <= b {
				bytes = b
				break
			}
		}
		out, err := mediaType(
			attr{key: &mfMTMajorType, guid: &mfMediaTypeAudio}, attr{key: &mfMTSubtype, guid: &mfAudioFormatAAC},
			attr{key: &mfMTAudioBitsPerSample, value: 16}, attr{key: &mfMTAudioSampleRate, value: sampleRate},
			attr{key: &mfMTAudioChannels, value: 2}, attr{key: &mfMTAudioBytesPerSec, value: bytes})
		if err != nil {
			return err
		}
		defer free(&out)
		if hr := call(s.writer, writerAddStream, out, uintptr(unsafe.Pointer(&s.audio))); failed(hr) {
			return fmt.Errorf("render: Media Foundation has no AAC encoder: %#x", uint32(hr))
		}
		in, err := mediaType(
			attr{key: &mfMTMajorType, guid: &mfMediaTypeAudio}, attr{key: &mfMTSubtype, guid: &mfAudioFormatPCM},
			attr{key: &mfMTAudioBitsPerSample, value: 16}, attr{key: &mfMTAudioSampleRate, value: sampleRate},
			attr{key: &mfMTAudioChannels, value: 2}, attr{key: &mfMTAudioBlockAlign, value: 4},
			attr{key: &mfMTAudioBytesPerSec, value: 4 * sampleRate})
		if err != nil {
			return err
		}
		defer free(&in)
		if hr := call(s.writer, writerSetInputMediaType, uintptr(s.audio), in, 0); failed(hr) {
			return fmt.Errorf("render: Media Foundation cannot encode the sound: %#x", uint32(hr))
		}
	}
	if hr := call(s.writer, writerBeginWriting); failed(hr) {
		return fmt.Errorf("render: Media Foundation cannot start writing: %#x", uint32(hr))
	}
	return nil
}

// write gives stream a sample of size bytes, which fill writes, at time t
// for duration d, in 100 ns.
func (s *mfSink) write(stream uint32, size int, fill func([]byte), t, d int64) error {
	buf, err := newBuffer(size, fill)
	if err != nil {
		return err
	}
	defer free(&buf)
	sample, err := newSample(buf, t, d)
	if err != nil {
		return err
	}
	defer free(&sample)
	return s.writeSample(stream, sample)
}

// writeSample writes sample into stream.
func (s *mfSink) writeSample(stream uint32, sample uintptr) error {
	if hr := call(s.writer, writerWriteSample, uintptr(stream), sample); failed(hr) {
		return fmt.Errorf("render: Media Foundation cannot write a sample: %#x", uint32(hr))
	}
	return nil
}

// newBuffer returns a buffer of size bytes, which fill fills; the caller
// frees it.
func newBuffer(size int, fill func([]byte)) (uintptr, error) {
	var buf uintptr
	if hr, _, _ := procMFCreateMemoryBuffer.Call(uintptr(size), uintptr(unsafe.Pointer(&buf))); failed(hr) {
		return 0, fmt.Errorf("render: Media Foundation: no buffer: %#x", uint32(hr))
	}
	var p uintptr
	if hr := call(buf, bufferLock, uintptr(unsafe.Pointer(&p)), 0, 0); failed(hr) {
		free(&buf)
		return 0, fmt.Errorf("render: Media Foundation: cannot lock a buffer: %#x", uint32(hr))
	}
	fill(unsafe.Slice((*byte)(cptr(p)), size))
	call(buf, bufferUnlock)
	call(buf, bufferSetCurrentLength, uintptr(size))
	return buf, nil
}

// newSample returns a sample of buf at t for d, in 100 ns; the caller
// frees it.
func newSample(buf uintptr, t, d int64) (uintptr, error) {
	var sample uintptr
	if hr, _, _ := procMFCreateSample.Call(uintptr(unsafe.Pointer(&sample))); failed(hr) {
		return 0, fmt.Errorf("render: Media Foundation: no sample: %#x", uint32(hr))
	}
	call(sample, sampleAddBuffer, buf)
	call(sample, sampleSetTime, uintptr(t))
	call(sample, sampleSetDuration, uintptr(d))
	return sample, nil
}

// soundTo writes the sound track up to frame `to` of it, in 0.1 s
// samples: the writer interleaves it with the video.
func (s *mfSink) soundTo(to int64) error {
	track := s.spec.sound
	to = min(to, int64(len(track)/2))
	for s.samples < to {
		n := min(to-s.samples, sampleRate/10)
		pcm := track[2*s.samples : 2*(s.samples+n)]
		t := s.samples * 10_000_000 / sampleRate
		fill := func(b []byte) {
			for i, v := range pcm {
				b[2*i], b[2*i+1] = byte(v), byte(uint16(v)>>8)
			}
		}
		if err := s.write(s.audio, len(pcm)*2, fill, t, n*10_000_000/sampleRate); err != nil {
			return err
		}
		s.samples += n
	}
	return nil
}

// time returns the time of frame n from the first, in 100 ns.
func (s *mfSink) time(n int64) int64 { return n * s.spec.den * 10_000_000 / s.spec.num }

func (s *mfSink) frame(_ int, pix []byte) error {
	sp := s.spec
	buf, err := newBuffer(sp.w*sp.h*3/2, func(b []byte) { bgraToNV12(b, pix, sp.w, sp.h) })
	if err != nil {
		return err
	}
	sample, err := newSample(buf, s.time(s.n), s.time(s.n+1)-s.time(s.n))
	if err == nil {
		err = s.flush(s.n)
	}
	if err != nil {
		free(&sample)
		free(&buf)
		return err
	}
	s.pending, s.buf, s.at = sample, buf, s.n
	s.n++
	return nil
}

// hold shows the frame before for the time of one more.
func (s *mfSink) hold(int) error {
	s.n++
	return nil
}

// flush writes the pending frame, shown until frame to, with the sound up
// to half a second ahead of that.
func (s *mfSink) flush(to int64) error {
	if s.pending == 0 {
		return nil
	}
	defer free(&s.pending)
	defer free(&s.buf)
	if s.hasSound {
		if err := s.soundTo(s.time(to)*sampleRate/10_000_000 + sampleRate/2); err != nil {
			return err
		}
	}
	return s.writeSample(s.video, s.pending)
}

func (s *mfSink) close() error {
	if s.pending != 0 && s.at < s.n-1 {
		// A video ending with frames held ends with the last frame again,
		// at the time of the last: players read a video's length off its
		// frames.
		last, err := newSample(s.buf, s.time(s.n-1), s.time(s.n)-s.time(s.n-1))
		if err != nil {
			return err
		}
		// The frame before lasts to it; the sample holds the buffer.
		if err := s.flush(s.n - 1); err != nil {
			free(&last)
			return err
		}
		s.pending, s.at = last, s.n-1
	}
	if err := s.flush(s.n); err != nil {
		return err
	}
	if s.hasSound {
		if err := s.soundTo(int64(len(s.spec.sound) / 2)); err != nil {
			return err
		}
	}
	hr := call(s.writer, writerFinalize)
	free(&s.writer)
	if failed(hr) {
		return fmt.Errorf("render: Media Foundation cannot finish the file: %#x", uint32(hr))
	}
	return nil
}

func (s *mfSink) abort() {
	free(&s.pending)
	free(&s.buf)
	free(&s.writer)
}
