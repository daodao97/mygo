package render

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Codec is how Video compresses the frames.
type Codec uint8

const (
	// DefaultCodec is H264, or VP9 for WebM.
	DefaultCodec Codec = iota
	// H264 is H.264 (AVC), which every player plays: in MP4 or QuickTime.
	H264
	// HEVC is H.265, about as good at two thirds of the bitrate, which
	// newer systems play: in MP4 or QuickTime. Windows encodes it with the
	// HEVC Video Extensions.
	HEVC
	// ProRes is Apple ProRes 422 HQ, for editing: in QuickTime, on macOS,
	// or with ffmpeg.
	ProRes
	// ProRes4444 is Apple ProRes 4444, with the alpha of a Transparent
	// composition: in QuickTime, on macOS, or with ffmpeg.
	ProRes4444
	// VP9 is VP9 in WebM, with the alpha of a Transparent composition
	// with ffmpeg: on Linux with GStreamer's vpx plugin, or with ffmpeg.
	VP9
)

func (c Codec) String() string {
	switch c {
	case H264:
		return "H.264"
	case HEVC:
		return "HEVC"
	case ProRes:
		return "ProRes 422 HQ"
	case ProRes4444:
		return "ProRes 4444"
	case VP9:
		return "VP9"
	}
	return "Codec(" + strconv.Itoa(int(c)) + ")"
}

// alpha reports whether the codec keeps alpha.
func (c Codec) alpha() bool { return c == ProRes4444 || c == VP9 }

// Encoder chooses what encodes a Video.
type Encoder uint8

const (
	// System encodes with the system's media framework, in hardware
	// where it can: AVFoundation (VideoToolbox) on macOS, Media
	// Foundation on Windows, GStreamer on Linux.
	System Encoder = iota
	// FFmpeg pipes the frames to ffmpeg (Options.FFmpeg), for what the
	// system has no encoder of, as WebM on macOS and Windows, or a Linux
	// without GStreamer's encoders.
	FFmpeg
)

// container is the format of a video file.
type container uint8

const (
	mp4 container = iota
	mov
	webm
)

// videoSpec is what a video sink writes.
type videoSpec struct {
	path      string
	container container
	codec     Codec
	// w and h are the size of the frames, num/den their rate.
	w, h     int
	num, den int64
	// bitrate and audioBitrate are bits a second; alpha keeps the frames'
	// alpha, premultiplied.
	bitrate, audioBitrate int
	alpha                 bool
	// sound is the sound track, interleaved stereo at 48 kHz, nil for
	// none.
	sound  []int16
	ffmpeg string
}

// Video renders the composition into a video file at path: an MP4 (.mp4,
// .m4v), a QuickTime movie (.mov), or a WebM (.webm), with the sound its
// frames play (Frame.Audio) in AAC, or Opus in WebM. The file appears
// when the video is whole: until then the frames go into a file beside
// it, which an error or ctx's end removes.
func Video(ctx context.Context, comp Composition, path string, o Options) error {
	if err := comp.validate(); err != nil {
		return err
	}
	spec := videoSpec{w: comp.Width, h: comp.Height, codec: o.Codec, bitrate: o.Bitrate, audioBitrate: o.AudioBitrate, ffmpeg: o.FFmpeg}
	spec.num, spec.den = comp.rate()
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".mp4", ".m4v":
		spec.container = mp4
	case ".mov":
		spec.container = mov
	case ".webm":
		spec.container = webm
	default:
		return fmt.Errorf("render: no video format for %q: name an .mp4, .mov or .webm", ext)
	}
	if spec.codec == DefaultCodec {
		spec.codec = H264
		if spec.container == webm {
			spec.codec = VP9
		}
	}
	switch {
	case spec.container == webm && spec.codec != VP9,
		spec.container != webm && spec.codec == VP9:
		return fmt.Errorf("render: %v goes into WebM alone", VP9)
	case spec.container == mp4 && (spec.codec == ProRes || spec.codec == ProRes4444):
		return fmt.Errorf("render: %v goes into QuickTime (.mov)", spec.codec)
	case spec.codec > VP9:
		return fmt.Errorf("render: unknown codec %v", spec.codec)
	}
	if spec.codec != ProRes && spec.codec != ProRes4444 && (spec.w%2 != 0 || spec.h%2 != 0) {
		// YUV 4:2:0 halves the chroma's size.
		return fmt.Errorf("render: %v takes an even Width and Height, not %d×%d", spec.codec, spec.w, spec.h)
	}
	spec.alpha = comp.Transparent && spec.codec.alpha()
	if spec.bitrate <= 0 {
		bpp := 0.2 // bits a pixel a frame: H.264 of motion graphics
		if spec.codec == HEVC || spec.codec == VP9 {
			bpp = 0.12
		}
		spec.bitrate = max(int(float64(spec.w*spec.h)*float64(spec.num)/float64(spec.den)*bpp), 500_000)
	}
	if spec.audioBitrate <= 0 {
		spec.audioBitrate = 192_000
	}
	from, to, err := comp.span(o)
	if err != nil {
		return err
	}
	if spec.sound, err = soundtrack(ctx, comp, from, to); err != nil {
		return err
	}
	dir, base := filepath.Split(path)
	ext := filepath.Ext(base)
	tmp := filepath.Join(dir, "."+strings.TrimSuffix(base, ext)+".render-"+strconv.Itoa(os.Getpid())+ext)
	os.Remove(tmp)
	spec.path = tmp
	err = run(ctx, comp, o, func(s *session, d drawer) (sink, error) {
		if o.Encoder == FFmpeg {
			return newFFmpegSink(ctx, spec)
		}
		return newVideoSink(spec, d)
	})
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
