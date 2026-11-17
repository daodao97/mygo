//go:build linux && !android && (amd64 || arm64)

package video

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/egoist/mygo/internal/media"
	"github.com/egoist/mygo/internal/scene"
)

// On Linux, GStreamer plays the video: a playbin, which demuxes, decodes
// (in hardware through VA-API where its plugins are installed), plays the
// sound and keeps the clock, with an appsink as its video sink taking
// NV12. The appsink hands each frame over as its time comes, on the clock
// the sound plays by; a goroutine of the engine's takes it, copies its
// planes into memory of its own, 1.5 bytes a pixel, and wakes the window,
// whose renderer uploads them as they are and converts them in its
// shader. Another goroutine reads the pipeline's messages. No GLib
// callback is made, so nothing of Go runs on GStreamer's threads.
//
// libgstreamer, libgstapp and libgstvideo are loaded with the first video
// opened.

var (
	gstOnce sync.Once
	gstErr  error

	gstInit                  func(argc, argv uintptr)
	gstElementFactoryMake    func(factory, name string) uintptr
	gstElementSetState       func(el uintptr, state int32) int32
	gstElementGetBus         func(el uintptr) uintptr
	gstBusTimedPopFiltered   func(bus uintptr, timeout uint64, types uint32) uintptr
	gstMessageParseError     func(msg uintptr, err, debug *uintptr)
	gstMessageParseBuffering func(msg uintptr, percent *int32)
	gstMiniObjectUnref       func(obj uintptr)
	gstObjectUnref           func(obj uintptr)
	gstElementSeek           func(el uintptr, rate float64, format, flags, startType int32, start int64, stopType int32, stop int64) bool
	gstElementQueryPosition  func(el uintptr, format int32, cur *int64) bool
	gstElementQueryDuration  func(el uintptr, format int32, dur *int64) bool
	gstCapsFromString        func(s string) uintptr
	gstSampleGetBuffer       func(sample uintptr) uintptr
	gstSampleGetCaps         func(sample uintptr) uintptr
	gstBufferMap             func(buf uintptr, info *gstMapInfo, flags int32) bool
	gstBufferUnmap           func(buf uintptr, info *gstMapInfo)
	gstAppSinkSetCaps        func(sink, caps uintptr)
	gstAppSinkSetMaxBuffers  func(sink uintptr, n uint32)
	gstAppSinkSetDrop        func(sink uintptr, drop bool)
	gstAppSinkTryPullSample  func(sink uintptr, timeout uint64) uintptr
	gstSampleGetType         func() uintptr
	gstAppSinkIsEOS          func(sink uintptr) bool
	gObjectGetProperty       func(obj uintptr, name string, v *gValue)
	gValueDupBoxed           func(v *gValue) uintptr
	gstBufferGetVideoMeta    func(buf uintptr) uintptr
	gstVideoInfoNew          func() uintptr
	gstVideoInfoFromCaps     func(info, caps uintptr) bool
	gstVideoInfoFree         func(info uintptr)
	gObjectSetProperty       func(obj uintptr, name string, v *gValue)
	gValueInit               func(v *gValue, t uintptr) *gValue
	gValueUnset              func(v *gValue)
	gValueSetString          func(v *gValue, s string)
	gValueSetObject          func(v *gValue, obj uintptr)
	gValueSetDouble          func(v *gValue, d float64)
	gValueSetBoolean         func(v *gValue, b bool)
	gErrorFree               func(err uintptr)
	gFree                    func(p uintptr)
)

const (
	gstStateNull    = 1
	gstStatePaused  = 3
	gstStatePlaying = 4

	gstFormatTime = 3

	gstSeekFlagFlush    = 1
	gstSeekFlagAccurate = 2
	gstSeekTypeNone     = 0
	gstSeekTypeSet      = 1

	gstMessageEOS             = 1 << 0
	gstMessageError           = 1 << 1
	gstMessageBuffering       = 1 << 5
	gstMessageDurationChanged = 1 << 18
	gstMessageAsyncDone       = 1 << 21

	gTypeBoolean = 5 << 2
	gTypeDouble  = 15 << 2
	gTypeString  = 16 << 2
	gTypeObject  = 20 << 2

	gstMapRead = 1

	// gstMessageTypeOffset is where a GstMessage keeps its type, after
	// its GstMiniObject (GST_MESSAGE_TYPE).
	gstMessageTypeOffset = 64
	// gstBufferPTSOffset is where a GstBuffer keeps its PTS, after its
	// GstMiniObject and pool (GST_BUFFER_PTS).
	gstBufferPTSOffset = 72
)

// gValue is a GValue: its type, and two words of value.
type gValue struct {
	typ  uintptr
	data [2]uint64
}

// gstMapInfo is a GstMapInfo.
type gstMapInfo struct {
	memory   uintptr
	flags    int32
	_        int32
	data     unsafe.Pointer
	size     uintptr
	maxsize  uintptr
	user     [4]uintptr
	reserved [4]uintptr
}

func loadGStreamer() error {
	gstOnce.Do(func() {
		var libs [5]uintptr
		for i, name := range []string{"libgstreamer-1.0.so.0", "libgstapp-1.0.so.0", "libgstvideo-1.0.so.0", "libgobject-2.0.so.0", "libglib-2.0.so.0"} {
			h, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
			if err != nil {
				gstErr = fmt.Errorf("%w: %v", ErrUnsupported, err)
				return
			}
			libs[i] = h
		}
		defer func() {
			if v := recover(); v != nil {
				gstErr = fmt.Errorf("%w: %v", ErrUnsupported, v)
			}
		}()
		gst, app, video, gobject, glib := libs[0], libs[1], libs[2], libs[3], libs[4]
		purego.RegisterLibFunc(&gstInit, gst, "gst_init")
		purego.RegisterLibFunc(&gstElementFactoryMake, gst, "gst_element_factory_make")
		purego.RegisterLibFunc(&gstElementSetState, gst, "gst_element_set_state")
		purego.RegisterLibFunc(&gstElementGetBus, gst, "gst_element_get_bus")
		purego.RegisterLibFunc(&gstBusTimedPopFiltered, gst, "gst_bus_timed_pop_filtered")
		purego.RegisterLibFunc(&gstMessageParseError, gst, "gst_message_parse_error")
		purego.RegisterLibFunc(&gstMessageParseBuffering, gst, "gst_message_parse_buffering")
		purego.RegisterLibFunc(&gstMiniObjectUnref, gst, "gst_mini_object_unref")
		purego.RegisterLibFunc(&gstObjectUnref, gst, "gst_object_unref")
		purego.RegisterLibFunc(&gstElementSeek, gst, "gst_element_seek")
		purego.RegisterLibFunc(&gstElementQueryPosition, gst, "gst_element_query_position")
		purego.RegisterLibFunc(&gstElementQueryDuration, gst, "gst_element_query_duration")
		purego.RegisterLibFunc(&gstCapsFromString, gst, "gst_caps_from_string")
		purego.RegisterLibFunc(&gstSampleGetBuffer, gst, "gst_sample_get_buffer")
		purego.RegisterLibFunc(&gstSampleGetCaps, gst, "gst_sample_get_caps")
		purego.RegisterLibFunc(&gstBufferMap, gst, "gst_buffer_map")
		purego.RegisterLibFunc(&gstBufferUnmap, gst, "gst_buffer_unmap")
		purego.RegisterLibFunc(&gstAppSinkSetCaps, app, "gst_app_sink_set_caps")
		purego.RegisterLibFunc(&gstAppSinkSetMaxBuffers, app, "gst_app_sink_set_max_buffers")
		purego.RegisterLibFunc(&gstAppSinkSetDrop, app, "gst_app_sink_set_drop")
		purego.RegisterLibFunc(&gstAppSinkTryPullSample, app, "gst_app_sink_try_pull_sample")
		purego.RegisterLibFunc(&gstSampleGetType, gst, "gst_sample_get_type")
		purego.RegisterLibFunc(&gstAppSinkIsEOS, app, "gst_app_sink_is_eos")
		purego.RegisterLibFunc(&gObjectGetProperty, gobject, "g_object_get_property")
		purego.RegisterLibFunc(&gValueDupBoxed, gobject, "g_value_dup_boxed")
		purego.RegisterLibFunc(&gstBufferGetVideoMeta, video, "gst_buffer_get_video_meta")
		purego.RegisterLibFunc(&gstVideoInfoNew, video, "gst_video_info_new")
		purego.RegisterLibFunc(&gstVideoInfoFromCaps, video, "gst_video_info_from_caps")
		purego.RegisterLibFunc(&gstVideoInfoFree, video, "gst_video_info_free")
		purego.RegisterLibFunc(&gObjectSetProperty, gobject, "g_object_set_property")
		purego.RegisterLibFunc(&gValueInit, gobject, "g_value_init")
		purego.RegisterLibFunc(&gValueUnset, gobject, "g_value_unset")
		purego.RegisterLibFunc(&gValueSetString, gobject, "g_value_set_string")
		purego.RegisterLibFunc(&gValueSetObject, gobject, "g_value_set_object")
		purego.RegisterLibFunc(&gValueSetDouble, gobject, "g_value_set_double")
		purego.RegisterLibFunc(&gValueSetBoolean, gobject, "g_value_set_boolean")
		purego.RegisterLibFunc(&gErrorFree, glib, "g_error_free")
		purego.RegisterLibFunc(&gFree, glib, "g_free")
		gstInit(0, 0)
	})
	return gstErr
}

// setProperty sets a property of a GObject from a GValue of type t that
// set fills.
func setProperty(obj uintptr, name string, t uintptr, set func(*gValue)) {
	var v gValue
	gValueInit(&v, t)
	set(&v)
	gObjectSetProperty(obj, name, &v)
	gValueUnset(&v)
}

// gstEngine plays a video with a playbin.
type gstEngine struct {
	pipe, sink, bus uintptr
	// memfd holds the bytes of OpenData, which the playbin reads as a
	// file of /proc/self/fd; -1 for none.
	memfd int
	quit  chan struct{}
	done  sync.WaitGroup

	mu sync.Mutex
	// ring are the frames copied out of the samples: shown is the one
	// Frame last returned, latest the newest, which the next Frame
	// returns; the goroutine copies into neither. changed tells latest
	// is not shown yet.
	ring           [3]gstFrame
	shown, latest  int
	changed        bool
	w, h           int
	interval       time.Duration
	state          media.State
	err            error
	wake           func()
	wantPlay, loop bool
	// percent is how full a stream's buffer is, below 100 while it
	// waits for data.
	buffering bool
	// pending are a seek, and a rate, asked for before the pipeline
	// could take them, once it prerolled (prerolled).
	prerolled  bool
	seekTo     time.Duration
	seekSet    bool
	rate       float64
	closed     bool
	lastPosNs  int64
	durationNs int64
}

// gstFrame is a frame of the ring: NV12 planes in Go memory.
type gstFrame struct {
	surface scene.Surface
	w, h    int
}

func newEngine(s source) (media.Engine, error) {
	if err := loadGStreamer(); err != nil {
		return nil, err
	}
	e := &gstEngine{memfd: -1, shown: -1, latest: -1, quit: make(chan struct{}), rate: 1}
	uri := s.url
	if s.data != nil {
		fd, err := memfd(s.data)
		if err != nil {
			return nil, err
		}
		e.memfd = fd
		uri = fmt.Sprintf("file:///proc/%d/fd/%d", os.Getpid(), fd)
	}
	e.pipe = gstElementFactoryMake("playbin", "")
	e.sink = gstElementFactoryMake("appsink", "")
	if e.pipe == 0 || e.sink == 0 {
		e.free()
		return nil, fmt.Errorf("%w: GStreamer has no playbin or appsink (gst-plugins-base)", ErrUnsupported)
	}
	caps := gstCapsFromString("video/x-raw,format=NV12")
	gstAppSinkSetCaps(e.sink, caps)
	gstMiniObjectUnref(caps)
	gstAppSinkSetMaxBuffers(e.sink, 1)
	gstAppSinkSetDrop(e.sink, true)
	setProperty(e.pipe, "video-sink", gTypeObject, func(v *gValue) { gValueSetObject(v, e.sink) })
	if st := gstElementFactoryMake("scaletempo", ""); st != 0 {
		// The sound at another rate keeps its pitch.
		setProperty(e.pipe, "audio-filter", gTypeObject, func(v *gValue) { gValueSetObject(v, st) })
	}
	setProperty(e.pipe, "uri", gTypeString, func(v *gValue) { gValueSetString(v, uri) })
	e.bus = gstElementGetBus(e.pipe)
	gstElementSetState(e.pipe, gstStatePaused)
	e.done.Add(2)
	go e.pull()
	go e.messages()
	return e, nil
}

// memfd returns a file in memory holding data.
func memfd(data []byte) (int, error) {
	name := []byte("mygo-video\x00")
	fd, _, errno := syscall.Syscall(sysMemfdCreate, uintptr(unsafe.Pointer(&name[0])), 1 /* MFD_CLOEXEC */, 0)
	runtime.KeepAlive(name)
	if errno != 0 {
		return -1, fmt.Errorf("video: memfd_create: %w", errno)
	}
	f := os.NewFile(fd, "mygo-video")
	if _, err := f.Write(data); err != nil {
		f.Close()
		return -1, err
	}
	// The file stays open, as its descriptor; f must not close it when
	// collected.
	nfd, err := syscall.Dup(int(fd))
	f.Close()
	if err != nil {
		return -1, err
	}
	return nfd, nil
}

// pull takes the frames the appsink hands over as their times come and,
// while none comes, the frame the sink holds last, which is the one a
// paused pipeline prerolled after loading or a seek: taken once each.
func (e *gstEngine) pull() {
	defer e.done.Done()
	var lastBuf uintptr
	lastPTS := ^uint64(0)
	for {
		select {
		case <-e.quit:
			return
		default:
		}
		s := gstAppSinkTryPullSample(e.sink, uint64(20*time.Millisecond))
		if s == 0 && gstAppSinkIsEOS(e.sink) {
			// Which returns at once: nothing comes until a seek.
			select {
			case <-e.quit:
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
		if s == 0 {
			s = lastSample(e.sink)
		}
		if s == 0 {
			continue
		}
		if buf := gstSampleGetBuffer(s); buf != 0 {
			pts := *(*uint64)(unsafe.Add(cptr(buf), gstBufferPTSOffset))
			if buf != lastBuf || pts != lastPTS {
				lastBuf, lastPTS = buf, pts
				e.take(s)
			}
		}
		gstMiniObjectUnref(s)
	}
}

// lastSample returns a reference to the sample a sink holds last, or 0.
func lastSample(sink uintptr) uintptr {
	var v gValue
	gValueInit(&v, gstSampleGetType())
	gObjectGetProperty(sink, "last-sample", &v)
	s := gValueDupBoxed(&v)
	gValueUnset(&v)
	return s
}

// take copies a sample's planes into a frame of the ring, which becomes
// the latest, and wakes the window.
func (e *gstEngine) take(sample uintptr) {
	buf, caps := gstSampleGetBuffer(sample), gstSampleGetCaps(sample)
	if buf == 0 || caps == 0 {
		return
	}
	info := gstVideoInfoNew()
	defer gstVideoInfoFree(info)
	if !gstVideoInfoFromCaps(info, caps) {
		return
	}
	vi := unsafe.Slice((*byte)(cptr(info)), 152)
	w, h := int(int32(le32(vi[16:]))), int(int32(le32(vi[20:])))
	rng, matrix := le32(vi[40:]), le32(vi[44:])
	fpsN, fpsD := int32(le32(vi[64:])), int32(le32(vi[68:]))
	offset := [2]uintptr{uintptr(le64(vi[72:])), uintptr(le64(vi[80:]))}
	stride := [2]int{int(int32(le32(vi[104:]))), int(int32(le32(vi[108:])))}
	if meta := gstBufferGetVideoMeta(buf); meta != 0 {
		m := unsafe.Slice((*byte)(cptr(meta)), 96)
		offset = [2]uintptr{uintptr(le64(m[48:])), uintptr(le64(m[56:]))}
		stride = [2]int{int(int32(le32(m[80:]))), int(int32(le32(m[84:])))}
	}
	if w <= 0 || h <= 0 {
		return
	}
	var mi gstMapInfo
	if !gstBufferMap(buf, &mi, gstMapRead) {
		return
	}
	defer gstBufferUnmap(buf, &mi)
	cw, ch := (w+1)/2, (h+1)/2
	if offset[0]+uintptr(stride[0]*(h-1)+w) > mi.size || offset[1]+uintptr(stride[1]*(ch-1)+2*cw) > mi.size {
		return
	}
	data := unsafe.Slice((*byte)(mi.data), mi.size)

	e.mu.Lock()
	i := 0
	for i == e.shown || i == e.latest {
		i++
	}
	e.mu.Unlock()
	f := &e.ring[i]
	f.w, f.h = w, h
	s := &f.surface
	s.Format = scene.SurfaceNV12
	s.Matrix = gstMatrix(matrix, h)
	if rng == 1 { // GST_VIDEO_COLOR_RANGE_0_255
		s.Matrix += scene.FullRange
	}
	planes := [2]struct{ rowBytes, rows int }{{w, h}, {2 * cw, ch}}
	for p, pl := range planes {
		n := pl.rowBytes * pl.rows
		if cap(s.Planes[p].Pix) < n {
			s.Planes[p].Pix = make([]byte, n)
		}
		dst := s.Planes[p].Pix[:n]
		for y := range pl.rows {
			copy(dst[y*pl.rowBytes:][:pl.rowBytes], data[offset[p]+uintptr(y*stride[p]):][:pl.rowBytes])
		}
		s.Planes[p] = scene.Plane{Pix: dst, Stride: pl.rowBytes}
	}

	e.mu.Lock()
	e.latest, e.changed = i, true
	e.w, e.h = w, h
	if fpsN > 0 && fpsD > 0 {
		e.interval = time.Duration(int64(time.Second) * int64(fpsD) / int64(fpsN))
	}
	if e.state == media.Loading {
		e.state = media.Ready
	}
	wake := e.wake
	e.mu.Unlock()
	if wake != nil {
		wake()
	}
}

// gstMatrix is the YUV matrix of a GstVideoColorMatrix, or for an unknown
// one that of the video's size, as players guess it.
func gstMatrix(m uint32, h int) scene.YUVMatrix {
	switch m {
	case 3:
		return scene.BT709
	case 4:
		return scene.BT601
	case 6:
		return scene.BT2020
	}
	if h >= 720 {
		return scene.BT709
	}
	return scene.BT601
}

func le32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}
func le64(b []byte) uint64 { return uint64(le32(b)) | uint64(le32(b[4:]))<<32 }

// messages reads what the pipeline reports: that it prerolled, its end,
// an error, a stream's buffering.
func (e *gstEngine) messages() {
	defer e.done.Done()
	const types = gstMessageEOS | gstMessageError | gstMessageBuffering | gstMessageDurationChanged | gstMessageAsyncDone
	for {
		select {
		case <-e.quit:
			return
		default:
		}
		msg := gstBusTimedPopFiltered(e.bus, uint64(50*time.Millisecond), types)
		if msg == 0 {
			continue
		}
		kind := *(*uint32)(unsafe.Add(cptr(msg), gstMessageTypeOffset))
		e.handle(msg, kind)
		gstMiniObjectUnref(msg)
	}
}

func (e *gstEngine) handle(msg uintptr, kind uint32) {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch kind {
	case gstMessageAsyncDone:
		// Prerolled, after loading or a seek: a seek the pipeline did
		// not take yet goes now.
		first := !e.prerolled
		e.prerolled = true
		if e.seekSet || first && e.rate != 1 {
			e.seekLocked(e.seekTo)
		}
	case gstMessageEOS:
		if e.loop {
			e.seekLocked(0)
		} else {
			e.state = media.Ended
		}
	case gstMessageError:
		var gerr, debug uintptr
		gstMessageParseError(msg, &gerr, &debug)
		text := "GStreamer could not play the video"
		if gerr != 0 {
			if p := *(**byte)(unsafe.Add(cptr(gerr), 8)); p != nil {
				text = goString(p)
			}
			gErrorFree(gerr)
		}
		if debug != 0 {
			gFree(debug)
		}
		e.state, e.err = media.Failed, errors.New("video: "+text)
	case gstMessageBuffering:
		var percent int32
		gstMessageParseBuffering(msg, &percent)
		e.buffering = percent < 100
		switch {
		case e.buffering && e.state == media.Ready:
			e.state = media.Buffering
		case !e.buffering && e.state == media.Buffering:
			e.state = media.Ready
		}
		e.applyState()
	case gstMessageDurationChanged:
		e.durationNs = 0
	}
	if e.wake != nil {
		go e.wake()
	}
}

// cptr is the pointer of an address of C memory, which Go's collector
// does not move.
func cptr(a uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&a)) }

func goString(p *byte) string {
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

// applyState plays the pipeline when the player plays it, unless a
// stream fills its buffer.
func (e *gstEngine) applyState() {
	if e.closed {
		return
	}

	if e.wantPlay && !e.buffering && e.state != media.Ended {
		gstElementSetState(e.pipe, gstStatePlaying)
	} else {
		gstElementSetState(e.pipe, gstStatePaused)
	}
}

func (e *gstEngine) SetWake(wake func()) {
	e.mu.Lock()
	e.wake = wake
	e.mu.Unlock()
}

func (e *gstEngine) Frame(time.Time) (media.Picture, bool, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.latest < 0 {
		return media.Picture{}, false, false
	}
	changed := e.changed
	e.shown, e.changed = e.latest, false
	f := &e.ring[e.shown]
	s := &f.surface
	w, h := f.w, f.h
	return media.Picture{Surface: s, W: w, H: h, RGBA: func(dst []byte) []byte { return s.RGBA(dst, w, h) }}, changed, true
}

func (e *gstEngine) Interval() time.Duration { return 0 } // the appsink wakes the window

func (e *gstEngine) State() (media.State, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state, e.err
}

func (e *gstEngine) Size() (int, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.w, e.h
}

func (e *gstEngine) Duration() time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.durationNs <= 0 && !e.closed {
		var d int64
		if gstElementQueryDuration(e.pipe, gstFormatTime, &d) && d > 0 {
			e.durationNs = d
		}
	}
	return time.Duration(max(e.durationNs, 0))
}

func (e *gstEngine) Position() time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return time.Duration(e.lastPosNs)
	}
	if !e.prerolled && e.seekSet {
		return e.seekTo
	}
	var p int64
	if gstElementQueryPosition(e.pipe, gstFormatTime, &p) && p >= 0 {
		e.lastPosNs = p
	}
	return time.Duration(e.lastPosNs)
}

func (e *gstEngine) Play() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.wantPlay = true
	if e.state == media.Ended {
		e.state = media.Ready
		e.seekLocked(0)
	}
	e.applyState()
}

func (e *gstEngine) Pause() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.wantPlay = false
	e.applyState()
}

func (e *gstEngine) Seek(d time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state == media.Ended {
		e.state = media.Ready
	}
	e.seekLocked(d)
	e.applyState()
}

// seekLocked seeks the pipeline to d at the rate, or keeps d for once it
// prerolled.
func (e *gstEngine) seekLocked(d time.Duration) {

	if e.closed {
		return
	}
	e.seekTo, e.seekSet = d, true
	e.lastPosNs = int64(d)
	if !e.prerolled {
		return
	}
	if gstElementSeek(e.pipe, e.rate, gstFormatTime, gstSeekFlagFlush|gstSeekFlagAccurate, gstSeekTypeSet, int64(d), gstSeekTypeNone, -1) {
		e.seekSet = false
	}
}

func (e *gstEngine) SetLoop(loop bool) {
	e.mu.Lock()
	e.loop = loop
	e.mu.Unlock()
}

func (e *gstEngine) SetRate(r float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r == e.rate {
		return
	}
	e.rate = r
	if e.prerolled {
		var p int64
		gstElementQueryPosition(e.pipe, gstFormatTime, &p)
		e.seekLocked(time.Duration(max(p, 0)))
	}
}

func (e *gstEngine) SetVolume(v float64) {
	setProperty(e.pipe, "volume", gTypeDouble, func(g *gValue) { gValueSetDouble(g, v) })
}

func (e *gstEngine) SetMuted(m bool) {
	setProperty(e.pipe, "mute", gTypeBoolean, func(g *gValue) { gValueSetBoolean(g, m) })
}

// Close stops the goroutines, then the pipeline, which may take a moment.
func (e *gstEngine) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	e.wake = nil
	e.mu.Unlock()
	close(e.quit)
	e.done.Wait()
	gstElementSetState(e.pipe, gstStateNull)
	e.free()
	return nil
}

func (e *gstEngine) free() {
	if e.bus != 0 {
		gstObjectUnref(e.bus)
		e.bus = 0
	}
	if e.pipe != 0 {
		gstObjectUnref(e.pipe) // and the sink it took
		e.pipe, e.sink = 0, 0
	} else if e.sink != 0 {
		gstObjectUnref(e.sink)
		e.sink = 0
	}
	if e.memfd >= 0 {
		syscall.Close(e.memfd)
		e.memfd = -1
	}
}
