// Package render makes videos of native UI: a composition is a view of
// package ui that draws each frame of a video from the frame's number,
// and Video draws its frames on the GPU, or on the CPU where there is
// none, and encodes them with the system's own encoders, in hardware
// where it has them, into an MP4 or a QuickTime movie, with sound:
//
//	comp := render.Composition{
//		Width: 1920, Height: 1080, FPS: 30, Frames: 90,
//		View: func(c *ui.Context, f render.Frame) {
//			ui.Text(c, "Hello").FontSize(96).
//				Opacity(float32(f.Interpolate([]float64{0, 20}, []float64{0, 1})))
//		},
//	}
//	err := render.Video(ctx, comp, "hello.mp4", render.Options{})
//
// Frames are built, laid out and painted by package ui, as a window's
// are, then drawn by its renderers: Metal on macOS, Direct3D 11 on
// Windows, OpenGL on Linux (through EGL, without a display), or the CPU
// renderer, which draws the same pixels, where the GPU's cannot run. No
// browser is involved: a frame costs what a frame of a window costs.
//
// Images writes the frames as PNG or JPEG files, Still one frame, and
// Frames hands each to a function. Nothing opens a window, and none of it
// needs mygo.App.
package render

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"runtime"
	"time"

	"github.com/egoist/mygo/internal/scene"
	"github.com/egoist/mygo/ui"
)

// defaultFPS is the frames a second of a Composition that gives none.
const defaultFPS = 30

// A Composition is a video: its size, its frames, and the view that
// builds each of them.
type Composition struct {
	// Width and Height are the size of the video in pixels, even for
	// H.264, HEVC and VP9, whose chroma has half the size.
	Width, Height int
	// FPS is how many frames a second the video shows, 30 when zero.
	// Rates of NTSC, as 29.97, are 30000/1001 to encoders.
	FPS float64
	// Frames is the length of the video in frames.
	Frames int
	// Scale is how many pixels a DIP is, 1 when zero: the view lays out
	// in Width/Scale × Height/Scale DIPs, as a window at that scale, so
	// that a 1920×1080 video at scale 2 is laid out as a 960×540 window
	// of a Retina display.
	Scale float32
	// View builds the frame f. It runs on several goroutines at once, one
	// a worker (Options.Workers), each building every frame: it must
	// build the same frame the same way each time, and read what it
	// shares, never change it.
	View func(c *ui.Context, f Frame)

	// Background fills the frames behind the view; zero is the theme's
	// background. Transparent leaves what the view does not paint
	// transparent instead, for the codecs with alpha (ProRes4444) and
	// PNGs; other codecs show it black.
	Background  ui.Color
	Transparent bool
	// Dark builds the view in the dark appearance, Preferences with the
	// desktop's settings that controls follow, and Locale in a language
	// (Context.Locale), "" for the system's default.
	Dark        bool
	Preferences ui.Preferences
	Locale      string

	// MotionBlur blurs what moves, as a camera's shutter does, when its
	// Samples are more than 1.
	MotionBlur MotionBlur
}

// MotionBlur is a camera's motion blur, as Remotion's CameraMotionBlur
// makes it: each frame is the mean of Samples frames built at times
// spread over the time the shutter is open before it, which Shutter
// gives in degrees of the frame's time, 180 (half of it) when zero, up to
// 360. Each sample is built and drawn, so a frame costs Samples times
// as much; 5 to 10 samples blur smoothly.
//
// The samples' times are fractions of a frame before it: what moves with
// time blurs, the view's Interpolate, Spring, Exact and Seconds, and ui's
// own animations, which follow the clock; what moves with the whole
// Frame.Index does not.
type MotionBlur struct {
	Samples int
	Shutter float64
}

// shutter returns the samples and the fraction of a frame they spread
// over, 1 sample for none.
func (m MotionBlur) shutter() (int, float64) {
	if m.Samples <= 1 {
		return 1, 0
	}
	angle := m.Shutter
	if angle == 0 {
		angle = 180
	}
	return m.Samples, angle / 360
}

// rate returns the frames a second as a fraction num/den.
func (c *Composition) rate() (num, den int64) {
	fps := c.FPS
	if fps <= 0 {
		fps = defaultFPS
	}
	if fps == math.Trunc(fps) {
		return int64(fps), 1
	}
	if n := math.Round(fps * 1001); math.Abs(n/1001-fps) < 1e-3 && math.Mod(n, 1000) == 0 {
		return int64(n), 1001
	}
	return int64(math.Round(fps * 1000)), 1000
}

func (c *Composition) validate() error {
	switch {
	case c.Width <= 0 || c.Height <= 0:
		return errors.New("render: a composition needs a Width and a Height")
	case c.Width > 16384 || c.Height > 16384:
		return errors.New("render: a composition is at most 16384 pixels wide and high")
	case c.Frames <= 0:
		return errors.New("render: a composition needs Frames")
	case c.FPS != 0 && !(c.FPS >= 0.01 && c.FPS <= 1000):
		return errors.New("render: FPS must be between 0.01 and 1000")
	case c.Scale < 0:
		return errors.New("render: Scale must not be negative")
	case c.View == nil:
		return errors.New("render: a composition needs a View")
	case c.MotionBlur.Samples < 0 || c.MotionBlur.Samples > 64:
		return errors.New("render: MotionBlur takes 0 to 64 Samples")
	case c.MotionBlur.Shutter < 0 || c.MotionBlur.Shutter > 360 || math.IsNaN(c.MotionBlur.Shutter):
		return errors.New("render: MotionBlur's Shutter is 0 to 360 degrees")
	}
	return nil
}

// Renderer chooses what draws the frames.
type Renderer uint8

const (
	// Auto draws on the GPU, or on the CPU where no GPU renderer starts,
	// as on a server without one.
	Auto Renderer = iota
	// GPU draws on the GPU, and fails where none starts.
	GPU
	// CPU draws on the CPU, with the renderer windows fall back to.
	CPU
)

// Options are how Video, Images, Frames and Still make a composition's
// frames.
type Options struct {
	// From and To are the frames rendered: those from From to before To,
	// all when To is zero. The frames before From are built, not drawn,
	// so that what ui animates from frame to frame is where it would be.
	From, To int
	// Renderer chooses what draws the frames.
	Renderer Renderer
	// Workers is how many frames are drawn at once, each by a worker of
	// its own on a thread of its own: 1 draws them one after another, and
	// zero chooses. See Composition.View for what a view does then.
	Workers int
	// Progress is called after each frame written, on the goroutine
	// rendering.
	Progress func(Progress)

	// Codec encodes the video, by default H264, or VP9 into WebM.
	// Bitrate is its bits a second, chosen for the size and rate when
	// zero; AudioBitrate the sound's, 192 kb/s when zero. ProRes takes
	// neither.
	Codec        Codec
	Bitrate      int
	AudioBitrate int
	// Encoder is what encodes Video's frames: the system's media
	// framework, or ffmpeg (FFmpeg), for formats the system has no
	// encoder of.
	Encoder Encoder
	// FFmpeg is the path of ffmpeg for the FFmpeg Encoder, "ffmpeg" from
	// PATH when empty.
	FFmpeg string
	// Quality is that of the JPEGs of Images, from 1 to 100, 90 when
	// zero.
	Quality int
}

// Progress tells how a render goes.
type Progress struct {
	// Frame is the frame written last, Done how many were written, of
	// Frames.
	Frame, Done, Frames int
	// Elapsed is the time since the render started.
	Elapsed time.Duration
	// Renderer names what draws the frames, as "Metal (Apple M2)" or
	// "CPU".
	Renderer string
}

// Still draws frame of the composition, after building the frames before
// it, and returns its pixels.
func Still(comp Composition, frame int, o Options) (*image.RGBA, error) {
	var img *image.RGBA
	o.From, o.To = frame, frame+1
	err := Frames(context.Background(), comp, o, func(_ int, m *image.RGBA) error {
		img = m
		return nil
	})
	return img, err
}

// Frames draws the frames of the composition Options choose and calls fn
// with each, in order, on the goroutine rendering: its pixels, with
// premultiplied alpha, are fn's to keep. An error from fn stops the
// render, and Frames returns it.
func Frames(ctx context.Context, comp Composition, o Options, fn func(frame int, img *image.RGBA) error) error {
	return run(ctx, comp, o, func(*session, drawer) (sink, error) {
		return &funcSink{w: comp.Width, h: comp.Height, fn: fn}, nil
	})
}

// funcSink hands the frames to a function.
type funcSink struct {
	w, h int
	fn   func(int, *image.RGBA) error
}

func (f *funcSink) frame(i int, pix []byte) error {
	img := image.NewRGBA(image.Rect(0, 0, f.w, f.h))
	bgraToRGBA(img.Pix, pix)
	return f.fn(i, img)
}
func (f *funcSink) close() error { return nil }
func (f *funcSink) abort()       {}

// bgraToRGBA copies BGRA pixels into RGBA ones.
func bgraToRGBA(dst, src []byte) {
	for i := 0; i+3 < len(src) && i+3 < len(dst); i += 4 {
		dst[i], dst[i+1], dst[i+2], dst[i+3] = src[i+2], src[i+1], src[i], src[i+3]
	}
}

// A sink takes the frames drawn, in order.
type sink interface {
	// frame takes frame i's pixels: premultiplied BGRA rows from the top,
	// 4×Width bytes each, which change once it returns.
	frame(i int, pix []byte) error
	// close finishes what the sink writes, abort gives it up.
	close() error
	abort()
}

// A sceneSink draws frames itself into the buffers its encoder takes, on
// the GPU of the drawer it was opened with: without copying them through
// memory. A sink is one only when it can.
type sceneSink interface {
	sink
	// scene draws frame i, and writes the frame before it, which the GPU
	// drew meanwhile.
	scene(i int, s *scene.Scene) error
	// flush writes the last frame.
	flush() error
}

// A drawer draws the frames on the GPU or the CPU.
type drawer interface {
	// submit starts drawing s; pixels waits for it and returns its
	// pixels: premultiplied BGRA rows from the top, valid until the next
	// submit.
	submit(s *scene.Scene) error
	pixels() ([]byte, error)
	// name names the renderer, as "Metal (Apple M2)".
	name() string
	release()
}

// newDrawer returns the drawer o asks for, of frames of w×h pixels.
func newDrawer(r Renderer, w, h int) (drawer, error) {
	if r != CPU {
		d, err := newGPUDrawer(w, h, r == GPU)
		if err == nil && d != nil {
			return d, nil
		}
		if r == GPU {
			if err == nil {
				err = errors.New("render: no GPU renderer on this system")
			}
			return nil, err
		}
	}
	return &cpuDrawer{}, nil
}

// session is a render: the composition's recorder, and what it heard.
type session struct {
	comp     Composition
	num, den int64 // frames a second, num/den
	rec      *ui.Recorder
	// frame is the frame the view builds.
	frame Frame
	// plays are the sounds the view played in the frame it built last.
	plays []play
	mix   *mixer
	// acc sums the samples of motion blur.
	acc accumulator
}

func newSession(comp Composition) *session {
	s := &session{comp: comp}
	s.num, s.den = comp.rate()
	s.mix = newMixer(s.num, s.den)
	view := comp.View
	bg, transparent := comp.Background, comp.Transparent
	s.rec = ui.NewRecorder(func(c *ui.Context) {
		// Each pass of a frame plays what it builds: the last one's count.
		s.plays = s.plays[:0]
		if transparent || bg != (ui.Color{}) {
			t := *c.Theme()
			t.Background = bg
			if transparent {
				t.Background = ui.Transparent
			}
			c.SetTheme(&t)
		}
		view(c, s.frame)
	}, ui.RecorderOptions{
		Width: comp.Width, Height: comp.Height, Scale: comp.Scale,
		Dark: comp.Dark, Preferences: comp.Preferences, Locale: comp.Locale,
		FrameRate: float64(s.num) / float64(s.den),
	})
	return s
}

// at returns the time of frame i.
func (s *session) at(i int) time.Duration {
	return time.Duration(math.Round(float64(i) * float64(s.den) * float64(time.Second) / float64(s.num)))
}

// build builds frame i and returns its scene.
func (s *session) build(i int) *scene.Scene { return s.buildAt(i, 0) }

// buildAt builds frame i at sub frames from its time, as a sample of
// motion blur does.
func (s *session) buildAt(i int, sub float64) *scene.Scene {
	s.frame = Frame{Index: i, Count: s.comp.Frames, FPS: float64(s.num) / float64(s.den), s: s, at: i, sub: sub}
	at := s.at(i)
	if sub != 0 {
		at = time.Duration(math.Round((float64(i) + sub) * float64(s.den) * float64(time.Second) / float64(s.num)))
	}
	return s.rec.Frame(at)
}

// blurred draws frame i as the mean of its motion blur's samples, the
// earliest first, and returns its pixels, valid until the next call.
func (s *session) blurred(i int, d drawer) ([]byte, error) {
	n, shutter := s.comp.MotionBlur.shutter()
	for k := n - 1; k >= 0; k-- {
		if err := d.submit(s.buildAt(i, -shutter*float64(k)/float64(n))); err != nil {
			return nil, err
		}
		pix, err := d.pixels()
		if err != nil {
			return nil, err
		}
		s.acc.add(pix, k == n-1)
	}
	return s.acc.mean(n), nil
}

// run renders the frames o chooses of comp into the sink open returns.
func run(ctx context.Context, comp Composition, o Options, open func(*session, drawer) (sink, error)) (err error) {
	if err := comp.validate(); err != nil {
		return err
	}
	from, to, err := comp.span(o)
	if err != nil {
		return err
	}
	if n := o.workers(comp, to-from); n > 1 {
		return runWorkers(ctx, comp, o, from, to, n, open)
	}
	// One thread draws: OpenGL's context is current on it.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	s := newSession(comp)
	defer s.rec.Close()
	d, err := newDrawer(o.Renderer, comp.Width, comp.Height)
	if err != nil {
		return err
	}
	defer d.release()
	s.mix.start = from
	out, err := open(s, d)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			out.abort()
		}
	}()
	scenes, _ := out.(sceneSink)
	samples, _ := comp.MotionBlur.shutter()
	if samples > 1 {
		// The samples are averaged in memory.
		scenes = nil
	}
	start := time.Now()
	report := func(i int) {
		if o.Progress != nil {
			o.Progress(Progress{Frame: i, Done: i - from + 1, Frames: to - from, Elapsed: time.Since(start), Renderer: d.name()})
		}
	}
	// The frames before From, built for what ui animates.
	for i := range from {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.build(i)
	}
	prev := -1
	for i := from; i < to; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if samples > 1 {
			pix, err := s.blurred(i, d)
			if err != nil {
				return err
			}
			if err := out.frame(i, pix); err != nil {
				return err
			}
			report(i)
			continue
		}
		sc := s.build(i)
		if scenes != nil {
			if err := scenes.scene(i, sc); err != nil {
				return err
			}
			if prev >= 0 {
				report(prev)
			}
			prev = i
			continue
		}
		// The GPU draws the frame before while this one built: its pixels
		// first, then this one's drawing.
		if prev >= 0 {
			if err := writePixels(d, out, prev); err != nil {
				return err
			}
			report(prev)
		}
		if err := d.submit(sc); err != nil {
			return err
		}
		prev = i
	}
	switch {
	case samples > 1:
		// Each frame was written as it was drawn.
		return out.close()
	case scenes != nil:
		err = scenes.flush()
	default:
		err = writePixels(d, out, prev)
	}
	if err != nil {
		return err
	}
	report(prev)
	return out.close()
}

// span returns the frames o renders, from to before to.
func (c *Composition) span(o Options) (from, to int, err error) {
	from, to = o.From, o.To
	if to == 0 {
		to = c.Frames
	}
	if from < 0 || to > c.Frames || from >= to {
		return 0, 0, fmt.Errorf("render: frames %d to %d are not in the composition's %d", from, to, c.Frames)
	}
	return from, to, nil
}

// soundtrack builds the frames before to and mixes the sound those from
// from play (Frame.Audio): interleaved stereo samples at 48 kHz, of the
// frames from from to before to, or nil when they play none. The system's
// writers take their tracks before the first frame, and interleave the
// sound with the video ahead of it: the sound is mixed before the frames
// are drawn, from frames built without drawing them, which costs a
// fraction of drawing them.
func soundtrack(ctx context.Context, comp Composition, from, to int) ([]int16, error) {
	s := newSession(comp)
	defer s.rec.Close()
	var track []int16
	heard := false
	for i := range to {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s.build(i)
		if i < from {
			continue
		}
		heard = heard || len(s.plays) > 0
		for _, x := range s.mix.frame(i, s.plays) {
			track = append(track, int16(math.Round(float64(x)*32767)))
		}
	}
	if !heard {
		return nil, nil
	}
	return track, nil
}

func writePixels(d drawer, out sink, i int) error {
	pix, err := d.pixels()
	if err != nil {
		return err
	}
	return out.frame(i, pix)
}
