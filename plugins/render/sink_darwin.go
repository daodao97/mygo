//go:build darwin

package render

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/egoist/mygo/internal/scene"
)

// On macOS, an AVAssetWriter writes the video: VideoToolbox encodes H.264
// and HEVC, in hardware, and ProRes, from CVPixelBuffers in BGRA backed by
// IOSurfaces, which the pool of its pixel buffer adaptor gives. Metal
// draws each frame straight into one (metal.Frames.DrawSurface): no frame
// is copied through memory. The sound goes to an input of its own,
// encoded in AAC.

var (
	avOnce sync.Once
	avErr  error

	msgAppendPixels func(obj objc.ID, s objc.SEL, pb uintptr, t cmTime) bool
	msgStartSession func(obj objc.ID, s objc.SEL, t cmTime)
	msgBool         func(obj objc.ID, s objc.SEL) bool
	msgSetBool      func(obj objc.ID, s objc.SEL, v bool)
	msgInt          func(obj objc.ID, s objc.SEL) int

	cvPoolCreatePixelBuffer   func(alloc, pool uintptr, out *uintptr) int32
	cvPixelBufferRelease      func(pb uintptr)
	cvPixelBufferGetIOSurface func(pb uintptr) uintptr
	cvLock                    func(pb uintptr, flags uint64) int32
	cvUnlock                  func(pb uintptr, flags uint64) int32
	cvBase                    func(pb uintptr) unsafe.Pointer
	cvStride                  func(pb uintptr) uint
	cvSetAttachment           func(buf, key, value uintptr, mode uint32)
	cmAudioFormatCreate       func(alloc uintptr, asbd *audioFormat, layoutSize uint, layout uintptr, cookieSize uint, cookie uintptr, ext uintptr, out *uintptr) int32
	cmBlockCreate             func(alloc, block uintptr, length uint, blockAlloc, custom uintptr, offset, dataLength uint, flags uint32, out *uintptr) int32
	cmBlockReplace            func(src unsafe.Pointer, dest uintptr, offset, length uint) int32
	cmAudioSampleCreate       func(alloc, data, format uintptr, n int, pts cmTime, descs uintptr, out *uintptr) int32
	cfRelease                 func(obj uintptr)
	cfRetain                  func(obj uintptr) uintptr
	poolPush                  func() uintptr
	poolPop                   func(pool uintptr)

	// avKeys are the constants of AVFoundation and CoreVideo, by name.
	avKeys = map[string]uintptr{}
)

// cmTime is a CMTime.
type cmTime struct {
	Value     int64
	Timescale int32
	Flags     uint32
	Epoch     int64
}

// audioFormat is an AudioStreamBasicDescription.
type audioFormat struct {
	SampleRate                               float64
	FormatID, FormatFlags, BytesPerPacket    uint32
	FramesPerPacket, BytesPerFrame, Channels uint32
	BitsPerChannel, Reserved                 uint32
}

const (
	cmTimeValid         = 1
	pixelFormatBGRA     = 0x42475241 // kCVPixelFormatType_32BGRA
	formatLinearPCM     = 0x6c70636d // 'lpcm'
	formatAAC           = 0x61616320 // 'aac '
	formatIntPacked     = 4 | 8      // kAudioFormatFlagIsSignedInteger | IsPacked
	writerStatusDone    = 2
	writerStatusFailed  = 3
	attachmentPropagate = 1
)

func loadAV() error {
	avOnce.Do(func() {
		defer func() {
			if v := recover(); v != nil {
				avErr = fmt.Errorf("render: AVFoundation: %v", v)
			}
		}()
		open := func(path string) uintptr {
			h, err := purego.Dlopen(path, purego.RTLD_GLOBAL|purego.RTLD_NOW)
			if err != nil {
				panic(err)
			}
			return h
		}
		objcLib := open("/usr/lib/libobjc.A.dylib")
		open("/System/Library/Frameworks/Foundation.framework/Foundation")
		av := open("/System/Library/Frameworks/AVFoundation.framework/AVFoundation")
		cv := open("/System/Library/Frameworks/CoreVideo.framework/CoreVideo")
		cm := open("/System/Library/Frameworks/CoreMedia.framework/CoreMedia")
		cf := open("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
		msgSend, err := purego.Dlsym(objcLib, "objc_msgSend")
		if err != nil {
			panic(err)
		}
		purego.RegisterFunc(&msgAppendPixels, msgSend)
		purego.RegisterFunc(&msgStartSession, msgSend)
		purego.RegisterFunc(&msgBool, msgSend)
		purego.RegisterFunc(&msgSetBool, msgSend)
		purego.RegisterFunc(&msgInt, msgSend)
		purego.RegisterLibFunc(&poolPush, objcLib, "objc_autoreleasePoolPush")
		purego.RegisterLibFunc(&poolPop, objcLib, "objc_autoreleasePoolPop")
		purego.RegisterLibFunc(&cvPoolCreatePixelBuffer, cv, "CVPixelBufferPoolCreatePixelBuffer")
		purego.RegisterLibFunc(&cvPixelBufferRelease, cv, "CVPixelBufferRelease")
		purego.RegisterLibFunc(&cvPixelBufferGetIOSurface, cv, "CVPixelBufferGetIOSurface")
		purego.RegisterLibFunc(&cvLock, cv, "CVPixelBufferLockBaseAddress")
		purego.RegisterLibFunc(&cvUnlock, cv, "CVPixelBufferUnlockBaseAddress")
		purego.RegisterLibFunc(&cvBase, cv, "CVPixelBufferGetBaseAddress")
		purego.RegisterLibFunc(&cvStride, cv, "CVPixelBufferGetBytesPerRow")
		purego.RegisterLibFunc(&cvSetAttachment, cv, "CVBufferSetAttachment")
		purego.RegisterLibFunc(&cmAudioFormatCreate, cm, "CMAudioFormatDescriptionCreate")
		purego.RegisterLibFunc(&cmBlockCreate, cm, "CMBlockBufferCreateWithMemoryBlock")
		purego.RegisterLibFunc(&cmBlockReplace, cm, "CMBlockBufferReplaceDataBytes")
		purego.RegisterLibFunc(&cmAudioSampleCreate, cm, "CMAudioSampleBufferCreateReadyWithPacketDescriptions")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
		purego.RegisterLibFunc(&cfRetain, cf, "CFRetain")
		for lib, names := range map[uintptr][]string{
			av: {"AVFileTypeMPEG4", "AVFileTypeQuickTimeMovie", "AVMediaTypeVideo", "AVMediaTypeAudio",
				"AVVideoCodecKey", "AVVideoCodecTypeH264", "AVVideoCodecTypeHEVC", "AVVideoCodecTypeAppleProRes422HQ",
				"AVVideoCodecTypeAppleProRes4444", "AVVideoWidthKey", "AVVideoHeightKey", "AVVideoCompressionPropertiesKey",
				"AVVideoAverageBitRateKey", "AVVideoExpectedSourceFrameRateKey", "AVVideoMaxKeyFrameIntervalKey",
				"AVVideoProfileLevelKey", "AVVideoProfileLevelH264HighAutoLevel", "AVVideoColorPropertiesKey",
				"AVVideoColorPrimariesKey", "AVVideoColorPrimaries_ITU_R_709_2", "AVVideoTransferFunctionKey",
				"AVVideoTransferFunction_ITU_R_709_2", "AVVideoYCbCrMatrixKey", "AVVideoYCbCrMatrix_ITU_R_709_2",
				"AVFormatIDKey", "AVSampleRateKey", "AVNumberOfChannelsKey", "AVEncoderBitRateKey"},
			cv: {"kCVPixelBufferPixelFormatTypeKey", "kCVPixelBufferWidthKey", "kCVPixelBufferHeightKey",
				"kCVPixelBufferIOSurfacePropertiesKey", "kCVPixelBufferMetalCompatibilityKey"},
		} {
			for _, name := range names {
				p, err := purego.Dlsym(lib, name)
				if err != nil || p == 0 {
					panic("no " + name)
				}
				avKeys[name] = *(*uintptr)(cptr(p))
			}
		}
		// The alpha mode of ProRes 4444's pixel buffers, from macOS 12.
		for _, name := range []string{"kCVImageBufferAlphaChannelModeKey", "kCVImageBufferAlphaChannelMode_PremultipliedAlpha"} {
			if p, err := purego.Dlsym(cv, name); err == nil && p != 0 {
				avKeys[name] = *(*uintptr)(cptr(p))
			}
		}
		if class("AVAssetWriter") == 0 {
			panic("no AVAssetWriter")
		}
	})
	return avErr
}

var sels sync.Map

func sel(name string) objc.SEL {
	if s, ok := sels.Load(name); ok {
		return s.(objc.SEL)
	}
	s := objc.RegisterName(name)
	sels.Store(name, s)
	return s
}

func send(obj objc.ID, name string, args ...any) objc.ID { return obj.Send(sel(name), args...) }

func class(name string) objc.ID { return objc.ID(objc.GetClass(name)) }

func key(name string) objc.ID { return objc.ID(avKeys[name]) }

func nsString(s string) objc.ID { return send(class("NSString"), "stringWithUTF8String:", s) }

func nsInt(n int64) objc.ID { return send(class("NSNumber"), "numberWithLongLong:", n) }

// nsDict returns an autoreleased dictionary of the keys and values.
func nsDict(kv ...objc.ID) objc.ID {
	n := len(kv) / 2
	keys, values := make([]objc.ID, n), make([]objc.ID, n)
	for i := range n {
		keys[i], values[i] = kv[2*i], kv[2*i+1]
	}
	return send(class("NSDictionary"), "dictionaryWithObjects:forKeys:count:", unsafe.Pointer(&values[0]), unsafe.Pointer(&keys[0]), uint(n))
}

func goString(str objc.ID) string {
	if str == 0 {
		return ""
	}
	p := uintptr(send(str, "UTF8String"))
	if p == 0 {
		return ""
	}
	n := 0
	for *(*byte)(cptr(p + uintptr(n))) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(cptr(p)), n))
}

// cptr is the pointer of an address of native memory.
func cptr(a uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&a)) }

// pooled runs fn in an autorelease pool.
func pooled(fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := poolPush()
	defer poolPop(p)
	return fn()
}

// avSink writes a video with an AVAssetWriter.
type avSink struct {
	spec                        videoSpec
	writer, video, adaptor, snd objc.ID
	pool, format                uintptr
	// frames are the frames the video input did not take yet, in order.
	frames  []avFrame
	written int64 // video frames queued
	samples int64 // frames of the sound track given to the writer
	// soundDone is set once the sound input is finished.
	soundDone bool
	alpha     bool
}

type avFrame struct {
	pb uintptr
	n  int64 // frames from the first
}

// avSceneSink is an avSink whose frames Metal draws into its pixel buffers.
type avSceneSink struct {
	*avSink
	d *metalDrawer
	// pending is the pixel buffer of the frame the GPU draws, 0 for none.
	pending avFrame
}

func newVideoSink(spec videoSpec, d drawer) (sink, error) {
	if spec.container == webm {
		return nil, fmt.Errorf("render: %w: macOS has no WebM encoder: encode it with Options{Encoder: render.FFmpeg}", errors.ErrUnsupported)
	}
	if err := loadAV(); err != nil {
		return nil, err
	}
	s := &avSink{spec: spec, alpha: spec.alpha && avKeys["kCVImageBufferAlphaChannelModeKey"] != 0}
	if err := pooled(s.open); err != nil {
		s.abort()
		return nil, err
	}
	if md, ok := d.(*metalDrawer); ok {
		return &avSceneSink{avSink: s, d: md}, nil
	}
	return s, nil
}

// mediaTimeScale is the time scale of the video's track, and of the movie,
// for frames at num/den a second: the smallest multiple of num from
// AVFoundation's own 600 on, in which each frame lasts a whole number of
// units. In 1/600 s a frame at 29.97 a second, 1001/30000 s, does not.
func mediaTimeScale(num int64) int32 {
	const usual = 600
	return int32(num * ((usual + num - 1) / num))
}

func (s *avSink) open() error {
	sp := s.spec
	url := send(class("NSURL"), "fileURLWithPath:", nsString(sp.path))
	fileType := key("AVFileTypeMPEG4")
	if sp.container == mov {
		fileType = key("AVFileTypeQuickTimeMovie")
	}
	var nserr objc.ID
	s.writer = send(send(class("AVAssetWriter"), "alloc"), "initWithURL:fileType:error:", url, fileType, unsafe.Pointer(&nserr))
	if s.writer == 0 {
		return fmt.Errorf("render: cannot write %s: %s", sp.path, describe(nserr))
	}
	if sp.container == mp4 {
		msgSetBool(s.writer, sel("setShouldOptimizeForNetworkUse:"), true)
	}
	send(s.writer, "setMovieTimeScale:", mediaTimeScale(sp.num))
	codec := map[Codec]string{H264: "AVVideoCodecTypeH264", HEVC: "AVVideoCodecTypeHEVC",
		ProRes: "AVVideoCodecTypeAppleProRes422HQ", ProRes4444: "AVVideoCodecTypeAppleProRes4444"}[sp.codec]
	settings := []objc.ID{
		key("AVVideoCodecKey"), key(codec),
		key("AVVideoWidthKey"), nsInt(int64(sp.w)),
		key("AVVideoHeightKey"), nsInt(int64(sp.h)),
		key("AVVideoColorPropertiesKey"), nsDict(
			key("AVVideoColorPrimariesKey"), key("AVVideoColorPrimaries_ITU_R_709_2"),
			key("AVVideoTransferFunctionKey"), key("AVVideoTransferFunction_ITU_R_709_2"),
			key("AVVideoYCbCrMatrixKey"), key("AVVideoYCbCrMatrix_ITU_R_709_2")),
	}
	if sp.codec == H264 || sp.codec == HEVC {
		fps := max((sp.num+sp.den/2)/sp.den, 1)
		props := []objc.ID{
			key("AVVideoAverageBitRateKey"), nsInt(int64(sp.bitrate)),
			key("AVVideoExpectedSourceFrameRateKey"), nsInt(fps),
			key("AVVideoMaxKeyFrameIntervalKey"), nsInt(2 * fps),
		}
		if sp.codec == H264 {
			props = append(props, key("AVVideoProfileLevelKey"), key("AVVideoProfileLevelH264HighAutoLevel"))
		}
		settings = append(settings, key("AVVideoCompressionPropertiesKey"), nsDict(props...))
	}
	s.video = send(send(class("AVAssetWriterInput"), "assetWriterInputWithMediaType:outputSettings:", key("AVMediaTypeVideo"), nsDict(settings...)), "retain")
	if s.video == 0 {
		return errors.New("render: AVFoundation takes no input of these settings")
	}
	msgSetBool(s.video, sel("setExpectsMediaDataInRealTime:"), false)
	send(s.video, "setMediaTimeScale:", mediaTimeScale(sp.num))
	attrs := nsDict(
		key("kCVPixelBufferPixelFormatTypeKey"), nsInt(pixelFormatBGRA),
		key("kCVPixelBufferWidthKey"), nsInt(int64(sp.w)),
		key("kCVPixelBufferHeightKey"), nsInt(int64(sp.h)),
		key("kCVPixelBufferIOSurfacePropertiesKey"), send(class("NSDictionary"), "dictionary"),
		key("kCVPixelBufferMetalCompatibilityKey"), send(class("NSNumber"), "numberWithBool:", true),
	)
	s.adaptor = send(send(class("AVAssetWriterInputPixelBufferAdaptor"), "assetWriterInputPixelBufferAdaptorWithAssetWriterInput:sourcePixelBufferAttributes:", s.video, attrs), "retain")
	if byte(send(s.writer, "canAddInput:", s.video)) == 0 {
		return fmt.Errorf("render: AVFoundation cannot write %v into this file", sp.codec)
	}
	send(s.writer, "addInput:", s.video)
	if sp.sound != nil {
		settings := nsDict(
			key("AVFormatIDKey"), nsInt(formatAAC),
			key("AVSampleRateKey"), nsInt(sampleRate),
			key("AVNumberOfChannelsKey"), nsInt(2),
			key("AVEncoderBitRateKey"), nsInt(int64(sp.audioBitrate)),
		)
		s.snd = send(send(class("AVAssetWriterInput"), "assetWriterInputWithMediaType:outputSettings:", key("AVMediaTypeAudio"), settings), "retain")
		msgSetBool(s.snd, sel("setExpectsMediaDataInRealTime:"), false)
		if s.snd == 0 || byte(send(s.writer, "canAddInput:", s.snd)) == 0 {
			return errors.New("render: AVFoundation cannot write the sound into this file")
		}
		send(s.writer, "addInput:", s.snd)
		af := audioFormat{SampleRate: sampleRate, FormatID: formatLinearPCM, FormatFlags: formatIntPacked,
			BytesPerPacket: 4, FramesPerPacket: 1, BytesPerFrame: 4, Channels: 2, BitsPerChannel: 16}
		if st := cmAudioFormatCreate(0, &af, 0, 0, 0, 0, 0, &s.format); st != 0 {
			return fmt.Errorf("render: no audio format: %d", st)
		}
	}
	if !msgBool(s.writer, sel("startWriting")) {
		return s.failure("cannot start writing")
	}
	msgStartSession(s.writer, sel("startSessionAtSourceTime:"), cmTime{Timescale: 1, Flags: cmTimeValid})
	// The adaptor's pool, retained: the adaptor returns it autoreleased.
	s.pool = uintptr(send(s.adaptor, "pixelBufferPool"))
	if s.pool == 0 {
		return s.failure("no pixel buffers")
	}
	cfRetain(s.pool)
	return nil
}

func describe(nserr objc.ID) string {
	if nserr == 0 {
		return "unknown error"
	}
	msg := goString(send(nserr, "localizedDescription"))
	if reason := goString(send(nserr, "localizedFailureReason")); reason != "" {
		msg += ": " + reason
	}
	return msg
}

// failure returns the writer's error, after what.
func (s *avSink) failure(what string) error {
	return fmt.Errorf("render: AVFoundation: %s: %s", what, describe(send(s.writer, "error")))
}

// buffer returns a pixel buffer from the pool, owned.
func (s *avSink) buffer() (uintptr, error) {
	var pb uintptr
	if st := cvPoolCreatePixelBuffer(0, s.pool, &pb); st != 0 || pb == 0 {
		return 0, fmt.Errorf("render: no pixel buffer: %d", st)
	}
	if s.alpha {
		cvSetAttachment(pb, avKeys["kCVImageBufferAlphaChannelModeKey"], avKeys["kCVImageBufferAlphaChannelMode_PremultipliedAlpha"], attachmentPropagate)
	}
	return pb, nil
}

func (s *avSink) frame(i int, pix []byte) error {
	return pooled(func() error {
		pb, err := s.buffer()
		if err != nil {
			return err
		}
		cvLock(pb, 0)
		base, stride := cvBase(pb), int(cvStride(pb))
		row := s.spec.w * 4
		for y := range s.spec.h {
			copy(unsafe.Slice((*byte)(unsafe.Add(base, y*stride)), row), pix[y*row:(y+1)*row])
		}
		cvUnlock(pb, 0)
		return s.queueFrame(pb)
	})
}

// queueFrame gives the writer the pixel buffer of the next frame, which
// it owns.
func (s *avSink) queueFrame(pb uintptr) error {
	s.frames = append(s.frames, avFrame{pb, s.written})
	s.written++
	return s.pump(3)
}

// soundBuffer returns a sample buffer of the sound track's next frames,
// up to n, owned, or 0 at its end.
func (s *avSink) soundBuffer(n int) (uintptr, error) {
	track := s.spec.sound
	at := int(s.samples) * 2
	if at >= len(track) {
		return 0, nil
	}
	pcm := track[at:min(at+2*n, len(track))]
	size := uint(len(pcm) * 2)
	var block, sb uintptr
	if st := cmBlockCreate(0, 0, size, 0, 0, 0, size, 1, &block); st != 0 {
		return 0, fmt.Errorf("render: no audio buffer: %d", st)
	}
	defer cfRelease(block)
	cmBlockReplace(unsafe.Pointer(&pcm[0]), block, 0, size)
	frames := len(pcm) / 2
	pts := cmTime{Value: s.samples, Timescale: sampleRate, Flags: cmTimeValid}
	if st := cmAudioSampleCreate(0, block, s.format, frames, pts, 0, &sb); st != 0 {
		return 0, fmt.Errorf("render: no audio sample buffer: %d", st)
	}
	s.samples += int64(frames)
	return sb, nil
}

// pump gives the inputs what they take: the queued frames, until at
// most frames are left, and the sound, as far as a second ahead of the
// frames given, or to its end once all are (frames < 0). The writer
// interleaves the two: the video input takes nothing more until the
// sound caught up with it.
func (s *avSink) pump(frames int) error {
	// lead is how far ahead of the video the sound goes, a second, and
	// more while the video input waits for it.
	lead := int64(sampleRate)
	for wait := time.Duration(0); ; {
		moved := false
		if s.snd != 0 {
			ahead := int64(math.MaxInt64)
			if frames >= 0 {
				ahead = (s.written-int64(len(s.frames)))*s.spec.den*sampleRate/s.spec.num + lead
			}
			for !s.soundDone && s.samples < ahead && msgBool(s.snd, sel("isReadyForMoreMediaData")) {
				sb, err := s.soundBuffer(4800)
				if err != nil {
					return err
				}
				if sb == 0 {
					// The whole track: the writer waits for more sound to
					// interleave the video with until it is finished.
					if !s.soundDone {
						send(s.snd, "markAsFinished")
						s.soundDone = true
					}
					break
				}
				ok := byte(send(s.snd, "appendSampleBuffer:", sb)) != 0
				cfRelease(sb)
				if !ok {
					return s.failure("cannot write the sound")
				}
				moved = true
			}
		}
		for len(s.frames) > 0 && msgBool(s.video, sel("isReadyForMoreMediaData")) {
			f := s.frames[0]
			ok := msgAppendPixels(s.adaptor, sel("appendPixelBuffer:withPresentationTime:"), f.pb,
				cmTime{Value: f.n * s.spec.den, Timescale: int32(s.spec.num), Flags: cmTimeValid})
			cvPixelBufferRelease(f.pb)
			s.frames = s.frames[1:]
			if !ok {
				return s.failure("cannot write a frame")
			}
			moved = true
		}
		if msgInt(s.writer, sel("status")) == writerStatusFailed {
			return s.failure("writing failed")
		}
		soundDone := s.snd == 0 || frames >= 0 || s.soundDone
		if len(s.frames) <= max(frames, 0) && soundDone {
			return nil
		}
		if moved {
			wait = 0
			continue
		}
		lead += sampleRate / 2
		// The encoder works through what it took: a little while.
		wait = min(max(wait*2, 100*time.Microsecond), 5*time.Millisecond)
		time.Sleep(wait)
	}
}

func (s *avSink) close() error {
	return pooled(func() error {
		// The frames, then the rest of the sound, which the writer takes
		// only once the video is finished.
		if err := s.pump(0); err != nil {
			return err
		}
		send(s.video, "markAsFinished")
		if s.snd != 0 {
			if err := s.pump(-1); err != nil {
				return err
			}
		}
		done := make(chan struct{})
		block := objc.NewBlock(func(objc.Block) { close(done) })
		send(s.writer, "finishWritingWithCompletionHandler:", block)
		<-done
		block.Release()
		ok := msgInt(s.writer, sel("status")) == writerStatusDone
		var err error
		if !ok {
			err = s.failure("cannot finish the file")
		}
		s.release()
		return err
	})
}

func (s *avSink) abort() {
	pooled(func() error {
		if s.writer != 0 && msgInt(s.writer, sel("status")) == 1 {
			send(s.writer, "cancelWriting")
		}
		s.release()
		return nil
	})
}

// release frees what the sink holds.
func (s *avSink) release() {
	for _, f := range s.frames {
		cvPixelBufferRelease(f.pb)
	}
	s.frames = nil
	if s.format != 0 {
		cfRelease(s.format)
		s.format = 0
	}
	if s.pool != 0 {
		cfRelease(s.pool)
		s.pool = 0
	}
	for _, o := range []*objc.ID{&s.adaptor, &s.video, &s.snd, &s.writer} {
		if *o != 0 {
			send(*o, "release")
			*o = 0
		}
	}
}

func (s *avSceneSink) scene(i int, sc *scene.Scene) error {
	return pooled(func() error {
		pb, err := s.buffer()
		if err != nil {
			return err
		}
		// Drawing it waits for the GPU to finish the frame before, whose
		// buffer then holds it.
		if err := s.d.f.DrawSurface(sc, cvPixelBufferGetIOSurface(pb)); err != nil {
			cvPixelBufferRelease(pb)
			return err
		}
		prev := s.pending.pb
		s.pending.pb = pb
		if prev != 0 {
			return s.queueFrame(prev)
		}
		return nil
	})
}

func (s *avSceneSink) flush() error {
	if err := s.d.f.Wait(); err != nil {
		return err
	}
	pb := s.pending.pb
	s.pending.pb = 0
	if pb == 0 {
		return nil
	}
	return pooled(func() error { return s.queueFrame(pb) })
}

func (s *avSceneSink) abort() {
	s.d.f.Wait()
	if s.pending.pb != 0 {
		cvPixelBufferRelease(s.pending.pb)
		s.pending.pb = 0
	}
	s.avSink.abort()
}
