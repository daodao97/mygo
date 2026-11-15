//go:build linux && (amd64 || arm64)

package render

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// On Linux, a GStreamer pipeline writes the video: appsrc elements take
// the frames, as BGRA, and the sound track, videoconvert makes YUV of the
// frames, and the first encoder of the codec that encodes a test frame
// on this system encodes them: VA-API's (vah264enc, vaapih264enc) or
// NVENC's in hardware where there are, x264 or OpenH264 on the CPU
// otherwise.

var (
	gstOnce sync.Once
	gstErr  error

	gstInit                func(argc, argv uintptr)
	gstParseLaunch         func(desc string, err *uintptr) uintptr
	gstBinGetByName        func(bin uintptr, name string) uintptr
	gstElementSetState     func(el uintptr, state int32) int32
	gstElementGetBus       func(el uintptr) uintptr
	gstBusTimedPopFiltered func(bus uintptr, timeout uint64, types uint32) uintptr
	gstMessageParseError   func(msg uintptr, gerr, debug *uintptr)
	gstMiniObjectUnref     func(obj uintptr)
	gstObjectUnref         func(obj uintptr)
	gstBufferNewAllocate   func(alloc uintptr, size uint, params uintptr) uintptr
	gstBufferFill          func(buf uintptr, offset uint, src unsafe.Pointer, size uint) uint
	gstElementFactoryFind  func(name string) uintptr
	gstAppSrcPushBuffer    func(src, buf uintptr) int32
	gstAppSrcEndOfStream   func(src uintptr) int32
	gErrorFree             func(err uintptr)
	gFree                  func(p uintptr)
)

const (
	gstStateNull    = 1
	gstStatePlaying = 4
	gstMessageEOS   = 1 << 0
	gstMessageError = 1 << 1
	gstClockNone    = ^uint64(0)
	// The offsets of a GstBuffer's PTS and duration, after its
	// GstMiniObject and pool (GST_BUFFER_PTS, GST_BUFFER_DURATION).
	gstBufferPTSOffset      = 72
	gstBufferDurationOffset = 88
)

func loadGStreamer() error {
	gstOnce.Do(func() {
		var libs [4]uintptr
		for i, name := range []string{"libgstreamer-1.0.so.0", "libgstapp-1.0.so.0", "libgobject-2.0.so.0", "libglib-2.0.so.0"} {
			h, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
			if err != nil {
				gstErr = fmt.Errorf("render: %w: no GStreamer (%v): install it, or encode with Options{Encoder: render.FFmpeg}", errors.ErrUnsupported, err)
				return
			}
			libs[i] = h
		}
		defer func() {
			if v := recover(); v != nil {
				gstErr = fmt.Errorf("render: %w: GStreamer: %v", errors.ErrUnsupported, v)
			}
		}()
		gst, app, glib := libs[0], libs[1], libs[3]
		purego.RegisterLibFunc(&gstInit, gst, "gst_init")
		purego.RegisterLibFunc(&gstParseLaunch, gst, "gst_parse_launch")
		purego.RegisterLibFunc(&gstBinGetByName, gst, "gst_bin_get_by_name")
		purego.RegisterLibFunc(&gstElementSetState, gst, "gst_element_set_state")
		purego.RegisterLibFunc(&gstElementGetBus, gst, "gst_element_get_bus")
		purego.RegisterLibFunc(&gstBusTimedPopFiltered, gst, "gst_bus_timed_pop_filtered")
		purego.RegisterLibFunc(&gstMessageParseError, gst, "gst_message_parse_error")
		purego.RegisterLibFunc(&gstMiniObjectUnref, gst, "gst_mini_object_unref")
		purego.RegisterLibFunc(&gstObjectUnref, gst, "gst_object_unref")
		purego.RegisterLibFunc(&gstBufferNewAllocate, gst, "gst_buffer_new_allocate")
		purego.RegisterLibFunc(&gstBufferFill, gst, "gst_buffer_fill")
		purego.RegisterLibFunc(&gstElementFactoryFind, gst, "gst_element_factory_find")
		purego.RegisterLibFunc(&gstAppSrcPushBuffer, app, "gst_app_src_push_buffer")
		purego.RegisterLibFunc(&gstAppSrcEndOfStream, app, "gst_app_src_end_of_stream")
		purego.RegisterLibFunc(&gErrorFree, glib, "g_error_free")
		purego.RegisterLibFunc(&gFree, glib, "g_free")
		gstInit(0, 0)
	})
	return gstErr
}

func cptr(a uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&a)) }

func cString(p uintptr) string {
	if p == 0 {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(cptr(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(cptr(p)), n))
}

// gerror returns the message of a GError, and frees it.
func gerror(gerr uintptr) string {
	if gerr == 0 {
		return ""
	}
	msg := cString(*(*uintptr)(unsafe.Add(cptr(gerr), 8)))
	gErrorFree(gerr)
	return msg
}

// gstEncoder is an encoder element: its description, with %k the bitrate
// in kb/s, %b in b/s and %g the frames between keyframes, the raw format
// it takes, and the parser after it.
type gstEncoder struct {
	name, desc, format, parser string
}

var gstEncoders = map[Codec][]gstEncoder{
	H264: {
		{"vah264enc", "vah264enc bitrate=%k key-int-max=%g", "NV12", "h264parse"},
		{"vaapih264enc", "vaapih264enc bitrate=%k keyframe-period=%g", "NV12", "h264parse"},
		{"nvh264enc", "nvh264enc bitrate=%k gop-size=%g", "NV12", "h264parse"},
		{"x264enc", "x264enc bitrate=%k key-int-max=%g speed-preset=medium", "I420", "h264parse"},
		{"openh264enc", "openh264enc bitrate=%b gop-size=%g", "I420", "h264parse"},
	},
	HEVC: {
		{"vah265enc", "vah265enc bitrate=%k key-int-max=%g", "NV12", "h265parse"},
		{"vaapih265enc", "vaapih265enc bitrate=%k keyframe-period=%g", "NV12", "h265parse"},
		{"nvh265enc", "nvh265enc bitrate=%k gop-size=%g", "NV12", "h265parse"},
		{"x265enc", "x265enc bitrate=%k key-int-max=%g speed-preset=medium", "I420", "h265parse"},
	},
	VP9: {
		{"vavp9enc", "vavp9enc bitrate=%k key-int-max=%g", "NV12", ""},
		{"vp9enc", "vp9enc target-bitrate=%b keyframe-max-dist=%g end-usage=vbr deadline=1 cpu-used=4 row-mt=true", "I420", ""},
	},
}

// The encoders of the sound, by container.
var gstAudioEncoders = map[container][]string{
	mp4:  {"fdkaacenc bitrate=%b", "avenc_aac bitrate=%b", "voaacenc bitrate=%b", "faac bitrate=%b"},
	mov:  {"fdkaacenc bitrate=%b", "avenc_aac bitrate=%b", "voaacenc bitrate=%b", "faac bitrate=%b"},
	webm: {"opusenc bitrate=%b"},
}

// expand fills an element's description with the bitrate and the frames
// between keyframes.
func expand(desc string, bitrate, gop int) string {
	return strings.NewReplacer("%k", strconv.Itoa(max(bitrate/1000, 1)), "%b", strconv.Itoa(bitrate), "%g", strconv.Itoa(gop)).Replace(desc)
}

func hasElement(name string) bool {
	f := gstElementFactoryFind(name)
	if f == 0 {
		return false
	}
	gstObjectUnref(f)
	return true
}

// runPipeline runs a pipeline that ends by itself to its end, and returns
// its error.
func runPipeline(desc string) error {
	var gerr uintptr
	p := gstParseLaunch(desc, &gerr)
	if p == 0 || gerr != 0 {
		msg := gerror(gerr)
		if p != 0 {
			gstObjectUnref(p)
		}
		return errors.New(msg)
	}
	defer gstObjectUnref(p)
	defer gstElementSetState(p, gstStateNull)
	bus := gstElementGetBus(p)
	defer gstObjectUnref(bus)
	if gstElementSetState(p, gstStatePlaying) == 0 {
		return errors.New("cannot play")
	}
	return busError(bus, gstClockNone)
}

// busError waits up to timeout for the end of the stream or an error, and
// returns the error; nil at the end, or when neither came.
func busError(bus uintptr, timeout uint64) error {
	msg := gstBusTimedPopFiltered(bus, timeout, gstMessageEOS|gstMessageError)
	if msg == 0 {
		return nil
	}
	defer gstMiniObjectUnref(msg)
	if *(*uint32)(unsafe.Add(cptr(msg), 64)) != gstMessageError {
		return nil
	}
	var gerr, debug uintptr
	gstMessageParseError(msg, &gerr, &debug)
	text := gerror(gerr)
	if debug != 0 {
		gFree(debug)
	}
	if text == "" {
		text = "the pipeline failed"
	}
	return errors.New(text)
}

var (
	probeMu sync.Mutex
	probed  = map[string]error{}
)

// probe reports whether an encoder encodes frames of w×h on this system:
// a hardware encoder's element is there without the hardware.
func probe(e gstEncoder, w, h int) error {
	if !hasElement(e.name) {
		return errors.New("no " + e.name)
	}
	key := fmt.Sprintf("%s %dx%d", e.name, w, h)
	probeMu.Lock()
	defer probeMu.Unlock()
	if err, ok := probed[key]; ok {
		return err
	}
	err := runPipeline(fmt.Sprintf("videotestsrc num-buffers=2 ! video/x-raw,format=%s,width=%d,height=%d,framerate=30/1 ! %s ! fakesink",
		e.format, w, h, expand(e.desc, 2_000_000, 30)))
	probed[key] = err
	return err
}

// gstSink writes a video with a GStreamer pipeline.
type gstSink struct {
	spec         videoSpec
	pipe, bus    uintptr
	video, audio uintptr
	sound        sync.WaitGroup
	soundErr     error
	n            int64
	stop         sync.Once
}

func newVideoSink(spec videoSpec, _ drawer) (sink, error) {
	if err := loadGStreamer(); err != nil {
		return nil, err
	}
	if spec.codec == ProRes || spec.codec == ProRes4444 {
		return nil, fmt.Errorf("render: %w: GStreamer has no %v encoder: encode it with Options{Encoder: render.FFmpeg}", errors.ErrUnsupported, spec.codec)
	}
	var enc *gstEncoder
	var tried []string
	for _, e := range gstEncoders[spec.codec] {
		if err := probe(e, spec.w, spec.h); err != nil {
			tried = append(tried, e.name+": "+err.Error())
			continue
		}
		enc = &e
		break
	}
	if enc == nil {
		return nil, fmt.Errorf("render: %w: no GStreamer encoder of %v works here (%s): install gstreamer1.0-plugins-ugly (x264, x265) or a VA-API driver, or encode with Options{Encoder: render.FFmpeg}",
			errors.ErrUnsupported, spec.codec, strings.Join(tried, "; "))
	}
	parser := ""
	if enc.parser != "" {
		if !hasElement(enc.parser) {
			return nil, fmt.Errorf("render: %w: GStreamer has no %s: install gstreamer1.0-plugins-bad", errors.ErrUnsupported, enc.parser)
		}
		parser = " ! " + enc.parser
	}
	mux := map[container]string{mp4: "mp4mux faststart=true", mov: "qtmux", webm: "webmmux"}[spec.container]
	if !hasElement(strings.Fields(mux)[0]) {
		return nil, fmt.Errorf("render: %w: GStreamer has no %s: install gstreamer1.0-plugins-good", errors.ErrUnsupported, strings.Fields(mux)[0])
	}
	gop := int(2 * (spec.num + spec.den/2) / spec.den)
	frame := spec.w * spec.h * 4
	desc := fmt.Sprintf("appsrc name=video format=time block=true max-bytes=%d caps=\"video/x-raw,format=BGRA,width=%d,height=%d,framerate=%d/%d,pixel-aspect-ratio=1/1\""+
		" ! videoconvert ! video/x-raw,format=%s,colorimetry=bt709 ! queue ! %s%s ! %s name=mux ! filesink location=%s",
		3*frame, spec.w, spec.h, spec.num, spec.den, enc.format, expand(enc.desc, spec.bitrate, gop), parser, mux, quote(spec.path))
	if spec.sound != nil {
		var aenc string
		for _, a := range gstAudioEncoders[spec.container] {
			if hasElement(strings.Fields(a)[0]) {
				aenc = expand(a, spec.audioBitrate, 0)
				break
			}
		}
		if aenc == "" {
			return nil, fmt.Errorf("render: %w: GStreamer has no encoder of the sound: install gstreamer1.0-libav, or encode with Options{Encoder: render.FFmpeg}", errors.ErrUnsupported)
		}
		desc += " appsrc name=audio format=time block=true max-bytes=192000 caps=\"audio/x-raw,format=S16LE,rate=48000,channels=2,layout=interleaved\"" +
			" ! audioconvert ! " + aenc + " ! queue ! mux."
	}
	s := &gstSink{spec: spec}
	var gerr uintptr
	s.pipe = gstParseLaunch(desc, &gerr)
	if s.pipe == 0 || gerr != 0 {
		msg := gerror(gerr)
		if s.pipe != 0 {
			gstObjectUnref(s.pipe)
		}
		return nil, errors.New("render: GStreamer: " + msg)
	}
	s.bus = gstElementGetBus(s.pipe)
	s.video = gstBinGetByName(s.pipe, "video")
	if spec.sound != nil {
		s.audio = gstBinGetByName(s.pipe, "audio")
	}
	if gstElementSetState(s.pipe, gstStatePlaying) == 0 {
		err := busError(s.bus, 0)
		s.abort()
		if err == nil {
			err = errors.New("cannot start the pipeline")
		}
		return nil, fmt.Errorf("render: GStreamer: %w", err)
	}
	if s.audio != 0 {
		s.sound.Add(1)
		go s.pushSound()
	}
	return s, nil
}

// quote quotes a value of gst_parse_launch's.
func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// push gives an appsrc a buffer of data at pts for duration, in ns.
func push(src uintptr, data unsafe.Pointer, size int, pts, duration int64) int32 {
	buf := gstBufferNewAllocate(0, uint(size), 0)
	gstBufferFill(buf, 0, data, uint(size))
	*(*int64)(unsafe.Add(cptr(buf), gstBufferPTSOffset)) = pts
	*(*int64)(unsafe.Add(cptr(buf), gstBufferDurationOffset)) = duration
	return gstAppSrcPushBuffer(src, buf) // takes the buffer
}

// pushSound gives the pipeline the sound track, on a goroutine of its own:
// the muxer interleaves it with the frames, and blocks either while it
// waits for the other.
func (s *gstSink) pushSound() {
	defer s.sound.Done()
	track := s.spec.sound
	const chunk = 4800 // frames
	for at := 0; at < len(track); at += 2 * chunk {
		pcm := track[at:min(at+2*chunk, len(track))]
		n := int64(len(pcm) / 2)
		pts := int64(at/2) * 1_000_000_000 / sampleRate
		if r := push(s.audio, unsafe.Pointer(&pcm[0]), len(pcm)*2, pts, n*1_000_000_000/sampleRate); r != 0 {
			if r != -2 { // GST_FLOW_FLUSHING: stopped
				s.soundErr = fmt.Errorf("render: GStreamer took no sound (%d)", r)
			}
			return
		}
	}
	gstAppSrcEndOfStream(s.audio)
}

func (s *gstSink) frame(_ int, pix []byte) error {
	pts := s.n * s.spec.den * 1_000_000_000 / s.spec.num
	next := (s.n + 1) * s.spec.den * 1_000_000_000 / s.spec.num
	s.n++
	if r := push(s.video, unsafe.Pointer(&pix[0]), len(pix), pts, next-pts); r != 0 {
		if err := busError(s.bus, 0); err != nil {
			return fmt.Errorf("render: GStreamer: %w", err)
		}
		return fmt.Errorf("render: GStreamer took no frame (%d)", r)
	}
	if err := busError(s.bus, 0); err != nil {
		return fmt.Errorf("render: GStreamer: %w", err)
	}
	return nil
}

func (s *gstSink) close() error {
	gstAppSrcEndOfStream(s.video)
	err := busError(s.bus, gstClockNone)
	if err != nil {
		// The muxer takes no more: stopping the pipeline flushes the
		// sound's appsrc, whose push may be blocked.
		s.free()
	}
	s.sound.Wait()
	if err == nil {
		err = s.soundErr
	}
	s.free()
	if err != nil {
		return fmt.Errorf("render: GStreamer: %w", err)
	}
	return nil
}

func (s *gstSink) abort() { s.free() }

func (s *gstSink) free() {
	s.stop.Do(func() {
		if s.pipe != 0 {
			// Stopping flushes the appsrcs: a push blocked returns.
			gstElementSetState(s.pipe, gstStateNull)
		}
		s.sound.Wait()
		for _, o := range []uintptr{s.video, s.audio, s.bus, s.pipe} {
			if o != 0 {
				gstObjectUnref(o)
			}
		}
	})
}
