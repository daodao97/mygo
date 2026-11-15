package ui

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/media"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/scene"
	"github.com/egoist/mygo/internal/wait"
)

// TestVideoSeek checks that a decoder's video reports its length and the
// time it shows, that a seek shows the frame at the time it asks for and
// plays on from there, in the frame as far as it was asked, that a seek
// past the end ends the video at its last frame, and that a paused video
// stays paused.
func TestVideoSeek(t *testing.T) {
	p := newVideo(t, videoRed, videoGreen, videoBlue)
	tt, shows := videoTester(p, nil)
	if d := p.Duration(); d != 300*time.Millisecond {
		t.Fatalf("duration %v, want 300ms", d)
	}
	decodedAhead(t, p, 2)
	tt.Advance(50 * time.Millisecond)
	if pos := p.Position(); pos != 50*time.Millisecond {
		t.Fatalf("position %v, want 50ms", pos)
	}

	p.Seek(250 * time.Millisecond)
	tt.Frame()
	if got, pos := shows(), p.Position(); got != videoBlue || pos != 250*time.Millisecond {
		t.Fatalf("sought to 250ms: %v at %v", got, pos)
	}
	decodedAhead(t, p, 1) // the end
	tt.Advance(49 * time.Millisecond)
	if p.Ended() {
		t.Fatal("ended before the end")
	}
	tt.Advance(time.Millisecond)
	if got := shows(); got != videoBlue || !p.Ended() || p.Playing() {
		t.Fatalf("at the end: %v, ended %v, playing %v", got, p.Ended(), p.Playing())
	}

	// Sought back, an ended video plays on from there.
	p.Seek(120 * time.Millisecond)
	tt.Frame()
	if got, pos := shows(), p.Position(); got != videoGreen || pos != 120*time.Millisecond || p.Ended() || !p.Playing() {
		t.Fatalf("sought to 120ms after the end: %v at %v, ended %v, playing %v", got, pos, p.Ended(), p.Playing())
	}
	decodedAhead(t, p, 1)
	tt.Advance(80 * time.Millisecond)
	if got := shows(); got != videoBlue {
		t.Fatalf("80ms after 120ms: %v", got)
	}

	// Paused, a seek shows the frame and stays paused.
	p.Pause()
	p.Seek(0)
	tt.Frame()
	tt.Advance(time.Second)
	if got, pos := shows(), p.Position(); got != videoRed || pos != 0 || p.Playing() {
		t.Fatalf("paused, sought to 0: %v at %v, playing %v", got, pos, p.Playing())
	}

	// Past the end: the last frame, ended.
	p.Play()
	p.Seek(time.Hour)
	tt.Frame()
	if got, pos := shows(), p.Position(); got != videoBlue || pos != 300*time.Millisecond || !p.Ended() {
		t.Fatalf("sought past the end: %v at %v, ended %v", got, pos, p.Ended())
	}

	// Looping, past the end goes on from the start.
	p = newVideo(t, videoRed, videoGreen).SetLoop(true)
	tt, shows = videoTester(p, nil)
	p.Seek(time.Hour)
	if p.Ended() || p.cur == nil || p.cur.RGBAAt(0, 0) != videoGreen {
		t.Fatalf("looping, sought past the end: ended %v", p.Ended())
	}
	tt.Frame()
	wait.Until(t, "the first frame decoded again", func() bool { return p.frames == nil || len(p.frames) >= 1 || shows() == videoRed })
	tt.Frame() // as the frame decoded wakes the window
	if got := shows(); got != videoRed {
		t.Fatalf("looping, after the end: %v", got)
	}
}

// TestVideoRate checks that a video plays as fast as its rate says,
// asking for frames as soon.
func TestVideoRate(t *testing.T) {
	p := newVideo(t, videoRed, videoGreen, videoBlue).SetRate(2)
	tt, shows := videoTester(p, nil)
	if !tt.h.afterAt.Equal(tt.Now().Add(50 * time.Millisecond)) {
		t.Fatalf("After due %v, want 50ms", tt.h.afterAt.Sub(tt.Now()))
	}
	decodedAhead(t, p, 2)
	tt.Advance(50 * time.Millisecond)
	if got, pos := shows(), p.Position(); got != videoGreen || pos != 100*time.Millisecond {
		t.Fatalf("at double speed after 50ms: %v at %v", got, pos)
	}
}

// apngFrameSpec is a frame of a test APNG: a solid w×h rectangle at x, y.
type apngFrameSpec struct {
	c                color.NRGBA
	x, y, w, h       int
	dispose, blend   byte
	delayNum, delDen uint16
}

// testAPNG builds an animated PNG of w×h of the frames given, the first
// the default image unless hidden, which adds a default image that is no
// frame.
func testAPNG(t *testing.T, w, h int, hidden bool, frames ...apngFrameSpec) []byte {
	t.Helper()
	encode := func(c color.NRGBA, fw, fh int) (ihdr, idat []byte) {
		img := image.NewNRGBA(image.Rect(0, 0, fw, fh))
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
		}
		var b bytes.Buffer
		if err := (&png.Encoder{CompressionLevel: png.NoCompression}).Encode(&b, translucent{img}); err != nil {
			t.Fatal(err)
		}
		data := b.Bytes()[8:]
		for len(data) >= 12 {
			n := binary.BigEndian.Uint32(data)
			kind, chunk := string(data[4:8]), data[8:8+n]
			switch kind {
			case "IHDR":
				ihdr = chunk
			case "IDAT":
				idat = append(idat, chunk...)
			}
			data = data[12+n:]
		}
		return ihdr, idat
	}
	var b bytes.Buffer
	b.Write(pngSignature)
	ihdr, _ := encode(color.NRGBA{}, w, h)
	ihdr = append([]byte(nil), ihdr...)
	writeChunk(&b, "IHDR", ihdr)
	actl := make([]byte, 8)
	binary.BigEndian.PutUint32(actl, uint32(len(frames)))
	writeChunk(&b, "acTL", actl)
	seq := uint32(0)
	if hidden {
		_, idat := encode(color.NRGBA{255, 255, 255, 255}, w, h)
		writeChunk(&b, "IDAT", idat)
	}
	for i, f := range frames {
		fc := make([]byte, 26)
		binary.BigEndian.PutUint32(fc, seq)
		seq++
		binary.BigEndian.PutUint32(fc[4:], uint32(f.w))
		binary.BigEndian.PutUint32(fc[8:], uint32(f.h))
		binary.BigEndian.PutUint32(fc[12:], uint32(f.x))
		binary.BigEndian.PutUint32(fc[16:], uint32(f.y))
		binary.BigEndian.PutUint16(fc[20:], f.delayNum)
		binary.BigEndian.PutUint16(fc[22:], f.delDen)
		fc[24], fc[25] = f.dispose, f.blend
		writeChunk(&b, "fcTL", fc)
		_, idat := encode(f.c, f.w, f.h)
		if i == 0 && !hidden {
			writeChunk(&b, "IDAT", idat)
			continue
		}
		fd := make([]byte, 4, 4+len(idat))
		binary.BigEndian.PutUint32(fd, seq)
		seq++
		writeChunk(&b, "fdAT", append(fd, idat...))
	}
	writeChunk(&b, "IEND", nil)
	return b.Bytes()
}

// translucent has the PNG encoder keep alpha, as every frame of an APNG
// is of the same color type.
type translucent struct{ *image.NRGBA }

func (translucent) Opaque() bool { return false }

// TestAPNGDecoderFrames checks that an animated PNG's frames draw over
// what the ones before left as their disposal and blending say, for as
// long as their delays say, that a default image before the first frame
// control is not a frame, and that a PNG not animated is a frame.
func TestAPNGDecoderFrames(t *testing.T) {
	red, green := color.NRGBA{255, 0, 0, 255}, color.NRGBA{0, 255, 0, 255}
	half := color.NRGBA{0, 0, 255, 128}
	data := testAPNG(t, 4, 4, false,
		apngFrameSpec{c: red, w: 4, h: 4, delayNum: 1, delDen: 5},
		// Over the red, then cleared.
		apngFrameSpec{c: green, x: 1, y: 1, w: 2, h: 2, delayNum: 30, dispose: apngDisposeBackground},
		// Blended over what the clearing left.
		apngFrameSpec{c: half, x: 0, y: 0, w: 2, h: 2, blend: apngBlendOver, dispose: apngDisposePrevious},
		// Over what the frame before had under it, replacing it.
		apngFrameSpec{c: half, x: 3, y: 3, w: 1, h: 1, delayNum: 0},
	)
	d, err := NewAPNGDecoder(data)
	if err != nil {
		t.Fatal(err)
	}
	at := func(f VideoFrame, x, y int) color.RGBA { return f.Image.(*image.RGBA).RGBAAt(x, y) }
	type check struct {
		x, y int
		want color.RGBA
	}
	blue := color.RGBA{0, 0, 128, 128}
	steps := []struct {
		delay  time.Duration
		checks []check
	}{
		{200 * time.Millisecond, []check{{0, 0, videoRed}, {3, 3, videoRed}}},
		{300 * time.Millisecond, []check{{1, 1, videoGreen}, {0, 0, videoRed}}},
		{100 * time.Millisecond, []check{{0, 0, blend(videoRed, blue)}, {1, 1, blue}, {2, 2, videoRed}}},
		{100 * time.Millisecond, []check{{0, 0, videoRed}, {1, 1, color.RGBA{}}, {3, 3, blue}}},
	}
	for range 2 {
		for i, s := range steps {
			f, err := d.Next()
			if err != nil {
				t.Fatalf("frame %d: %v", i, err)
			}
			if f.Duration != s.delay {
				t.Errorf("frame %d shows for %v, want %v", i, f.Duration, s.delay)
			}
			for _, c := range s.checks {
				if got := at(f, c.x, c.y); !nearRGBA(got, c.want) {
					t.Errorf("frame %d at %d,%d: %v, want %v", i, c.x, c.y, got, c.want)
				}
			}
		}
		if _, err := d.Next(); err == nil {
			t.Fatal("a frame after the last")
		}
		if err := d.Rewind(); err != nil {
			t.Fatal(err)
		}
	}
	if got := d.(interface{ Duration() time.Duration }).Duration(); got != 700*time.Millisecond {
		t.Errorf("duration %v, want 700ms", got)
	}

	// A default image before the first frame control is no frame.
	d, err = NewAPNGDecoder(testAPNG(t, 2, 2, true, apngFrameSpec{c: green, w: 2, h: 2}))
	if err != nil {
		t.Fatal(err)
	}
	if f, err := d.Next(); err != nil || at(f, 0, 0) != videoGreen {
		t.Fatalf("first frame after a default image: %v", err)
	}
	if _, err := d.Next(); err == nil {
		t.Fatal("the default image was a frame")
	}

	// A PNG not animated.
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3, 2)))
	d, err = NewAPNGDecoder(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if f, err := d.Next(); err != nil || f.Image.Bounds().Dx() != 3 {
		t.Fatalf("a PNG not animated: %v", err)
	}
	if _, err := NewAPNGDecoder([]byte("GIF89a")); err == nil {
		t.Error("a GIF decoded as a PNG")
	}
}

// TestAPNGDecoderDimensions checks that a PNG whose picture is empty or
// too large for its pixel buffer returns an error instead of panicking.
func TestAPNGDecoderDimensions(t *testing.T) {
	var plain bytes.Buffer
	if err := png.Encode(&plain, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	for _, src := range []struct {
		name string
		data []byte
	}{
		{"plain", plain.Bytes()},
		{"animated", testAPNG(t, 1, 1, false, apngFrameSpec{w: 1, h: 1})},
	} {
		for _, size := range []struct {
			name string
			w, h uint32
		}{
			{"zero width", 0, 1},
			{"zero height", 1, 0},
			{"pixel buffer overflow", 0x7fffffff, 0x7fffffff},
			{"largest dimensions", 0xffffffff, 0xffffffff},
		} {
			t.Run(src.name+"/"+size.name, func(t *testing.T) {
				// Replace only IHDR, including its checksum; keep the frames.
				hdr := append([]byte(nil), src.data[16:29]...)
				binary.BigEndian.PutUint32(hdr, size.w)
				binary.BigEndian.PutUint32(hdr[4:], size.h)
				var b bytes.Buffer
				b.Write(pngSignature)
				writeChunk(&b, "IHDR", hdr)
				b.Write(src.data[33:])
				if _, err := NewAPNGDecoder(b.Bytes()); err == nil {
					t.Fatal("a PNG of an invalid size decoded")
				}
			})
		}
	}
}

// blend is src, premultiplied, over dst.
func blend(dst, src color.RGBA) color.RGBA {
	k := 255 - uint32(src.A)
	f := func(d, s uint8) uint8 { return uint8(uint32(s) + (uint32(d)*k+127)/255) }
	return color.RGBA{f(dst.R, src.R), f(dst.G, src.G), f(dst.B, src.B), f(dst.A, src.A)}
}

func nearRGBA(a, b color.RGBA) bool {
	d := func(x, y uint8) bool { return x-y <= 1 || y-x <= 1 }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B) && d(a.A, b.A)
}

// fakeEngine is a system engine playing frames of solid colors, a tenth
// of a second each, as its clock says: it starts on Play.
type fakeEngine struct {
	mu      sync.Mutex
	colors  []color.RGBA
	playing bool
	started time.Time
	base    time.Duration
	shown   int
	state   media.State
	loop    bool
	muted   bool
	closed  bool
	wake    func()
	plays   int
	seeks   []time.Duration
	// intervals counts the calls of Interval.
	intervals int
	// readyOnFrame makes the engine ready by its first picture, in
	// Frame, without a wake, as AVFoundation's does.
	readyOnFrame bool
}

func (e *fakeEngine) SetWake(w func()) { e.mu.Lock(); e.wake = w; e.mu.Unlock() }

func (e *fakeEngine) pos(t time.Time) time.Duration {
	p := e.base
	if e.playing && !e.started.IsZero() {
		p += t.Sub(e.started)
	}
	return p
}

func (e *fakeEngine) Frame(t time.Time) (media.Picture, bool, bool) {
	if e.state == media.Loading {
		if !e.readyOnFrame {
			return media.Picture{}, false, false
		}
		e.state = media.Ready
	}
	if e.playing && e.started.IsZero() {
		e.started = t
	}
	i := int(e.pos(t) / (100 * time.Millisecond))
	if i >= len(e.colors) {
		i = len(e.colors) - 1
		e.state = media.Ended
	}
	changed := i != e.shown || e.shown < 0
	e.shown = i
	c := e.colors[i]
	pic := media.Picture{Surface: &scene.Surface{Format: scene.SurfaceBGRA, Planes: [2]scene.Plane{{Pix: bytes.Repeat([]byte{c.B, c.G, c.R, c.A}, 16), Stride: 16}}}, W: 4, H: 4}
	pic.RGBA = func(dst []byte) []byte { return pic.Surface.RGBA(dst, 4, 4) }
	return pic, changed, true
}

func (e *fakeEngine) Interval() time.Duration     { e.intervals++; return 100 * time.Millisecond }
func (e *fakeEngine) State() (media.State, error) { return e.state, nil }
func (e *fakeEngine) Size() (int, int)            { return 4, 4 }
func (e *fakeEngine) Duration() time.Duration {
	return time.Duration(len(e.colors)) * 100 * time.Millisecond
}
func (e *fakeEngine) Position() time.Duration { return e.base }
func (e *fakeEngine) Play()                   { e.playing = true; e.plays++ }
func (e *fakeEngine) Pause()                  { e.playing = false }
func (e *fakeEngine) Seek(d time.Duration) {
	e.seeks = append(e.seeks, d)
	e.base = d
	e.started = time.Time{}
	e.state = media.Ready
}
func (e *fakeEngine) SetLoop(l bool)         { e.loop = l }
func (e *fakeEngine) SetRate(float64)        {}
func (e *fakeEngine) SetVolume(float64)      {}
func (e *fakeEngine) SetMuted(m bool)        { e.muted = m }
func (e *fakeEngine) Close() error           { e.closed = true; return nil }
func (e *fakeEngine) wakeNow()               { e.mu.Lock(); w := e.wake; e.mu.Unlock(); w() }
func (e *fakeEngine) setState(s media.State) { e.state = s }

// TestEngineVideo checks that a player of a system's engine plays once a
// Video shows it, shows the engine's pictures as they come due, asking
// for frames at the video's rate, builds a frame when the engine wakes
// it, plays again from the start once ended, holds a muted video that
// goes out of the view and lets one with sound play on, and closes the
// engine.
func TestEngineVideo(t *testing.T) {
	e := &fakeEngine{colors: []color.RGBA{videoRed, videoGreen, videoBlue}, shown: -1, state: media.Loading}
	p := NewEngineVideoPlayer(e).SetMuted(true)
	if e.plays != 0 || p.Ready() {
		t.Fatal("played or ready before a Video showed it")
	}
	visible := true
	tt, shows := videoTester(p, &visible)
	if e.plays != 1 || p.Ready() {
		t.Fatalf("shown, loading: %d plays, ready %v", e.plays, p.Ready())
	}
	// AVFoundation would load the asset's tracks for it, on the main
	// thread (#1743).
	tt.Frame()
	if e.intervals != 0 {
		t.Fatalf("Interval asked %d times while loading", e.intervals)
	}
	e.setState(media.Ready)
	builds := tt.rt.frame
	if tt.FramePending() {
		t.Fatal("loading: a frame asked for")
	}
	e.wakeNow()
	if !tt.FramePending() {
		t.Fatal("the engine's wake asked for no frame")
	}
	tt.Frame()
	if got := shows(); got != videoRed || !p.Ready() || tt.rt.frame == builds {
		t.Fatalf("loaded: %v, ready %v", got, p.Ready())
	}
	if w, h := p.Size(); w != 4 || h != 4 {
		t.Fatalf("size %d×%d", w, h)
	}
	if !tt.h.afterAt.Equal(tt.Now().Add(100 * time.Millisecond)) {
		t.Fatalf("After due %v, want 100ms", tt.h.afterAt.Sub(tt.Now()))
	}
	tt.Advance(100 * time.Millisecond)
	if got := shows(); got != videoGreen {
		t.Fatalf("100ms on: %v", got)
	}

	// Muted, out of the view: held, then played again once shown.
	visible = false
	tt.Frame()
	if e.playing {
		t.Fatal("a muted video out of the view plays")
	}
	visible = true
	tt.Frame()
	if !e.playing || !p.Playing() {
		t.Fatal("a muted video back in the view does not play")
	}
	// With sound, it plays on, unseen.
	p.SetMuted(false)
	visible = false
	tt.Frame()
	if !e.playing {
		t.Fatal("a video with sound out of the view stopped")
	}
	visible = true
	tt.Frame()

	// Ended, played again from the start.
	tt.Advance(time.Second)
	tt.Advance(100 * time.Millisecond)
	if got := shows(); got != videoBlue || !p.Ended() || p.Playing() {
		t.Fatalf("at the end: %v, ended %v, playing %v", got, p.Ended(), p.Playing())
	}
	p.Play()
	tt.Frame()
	if len(e.seeks) == 0 || e.seeks[len(e.seeks)-1] != 0 || !p.Playing() {
		t.Fatalf("played again: seeks %v, playing %v", e.seeks, p.Playing())
	}
	if got := shows(); got != videoRed {
		t.Fatalf("played again: %v", got)
	}

	p.Close()
	if !e.closed {
		t.Fatal("the engine did not close")
	}
	tt.Frame()
}

// TestEngineVideoReadyByFrame checks that a video whose engine became
// ready by the picture the frame took, with no wake, asks for the next.
func TestEngineVideoReadyByFrame(t *testing.T) {
	e := &fakeEngine{colors: []color.RGBA{videoRed, videoGreen}, shown: -1, state: media.Loading, readyOnFrame: true}
	p := NewEngineVideoPlayer(e).SetMuted(true)
	visible := true
	tt, shows := videoTester(p, &visible)
	if got := shows(); got != videoRed || !p.Ready() {
		t.Fatalf("first picture: %v, ready %v", got, p.Ready())
	}
	if !tt.h.afterAt.Equal(tt.Now().Add(100 * time.Millisecond)) {
		t.Fatalf("After due %v, want 100ms", tt.h.afterAt.Sub(tt.Now()))
	}
	tt.Advance(100 * time.Millisecond)
	if got := shows(); got != videoGreen {
		t.Fatalf("100ms on: %v", got)
	}
}

// TestEngineVideoBuiltOutOfSight checks that a frame built while none of
// the window shows, as keys typed into a window that another covers build
// one, asks for the frame of the next picture, which waits for the window
// to show and builds once it does: the video goes on, muted or not,
// rather than stand still for good while its sound plays on.
func TestEngineVideoBuiltOutOfSight(t *testing.T) {
	for _, muted := range []bool{false, true} {
		e := &fakeEngine{colors: []color.RGBA{videoRed, videoGreen, videoBlue}, shown: -1, state: media.Loading, readyOnFrame: true}
		p := NewEngineVideoPlayer(e).SetMuted(muted)
		tt, shows := videoTester(p, nil)
		if got := shows(); got != videoRed || !p.Ready() {
			t.Fatalf("muted %v, first picture: %v, ready %v", muted, got, p.Ready())
		}
		tt.h.hidden = true
		tt.Frame()
		if tt.h.afterAt.IsZero() {
			t.Fatalf("muted %v, built out of sight: no frame asked for", muted)
		}
		tt.Advance(100 * time.Millisecond)
		if !tt.rt.held {
			t.Fatalf("muted %v, out of sight: the frame asked for is not held", muted)
		}
		tt.h.hidden = false
		tt.rt.event(platform.SurfaceEvent{Kind: platform.SurfaceShown})
		if !tt.FramePending() {
			t.Fatalf("muted %v, shown again: no frame asked for", muted)
		}
		tt.Frame()
		if !e.playing || shows() == videoRed {
			t.Fatalf("muted %v, shown again: playing %v, showing %v", muted, e.playing, shows())
		}
		p.Close()
	}
}

// seekingDecoder is a decoder of solid frames of 100 ms each that seeks
// to a frame at once, counting the frames it decodes.
type seekingDecoder struct {
	colors []color.RGBA
	next   int
	mu     sync.Mutex
	nexts  int
}

func (d *seekingDecoder) Next() (VideoFrame, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.next >= len(d.colors) {
		return VideoFrame{}, io.EOF
	}
	d.nexts++
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := 0; i < len(img.Pix); i += 4 {
		c := d.colors[d.next]
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	d.next++
	return VideoFrame{Image: img, Duration: 100 * time.Millisecond}, nil
}

func (d *seekingDecoder) Rewind() error { d.mu.Lock(); d.next = 0; d.mu.Unlock(); return nil }

func (d *seekingDecoder) Seek(at time.Duration) (time.Duration, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.next = min(int(at/(100*time.Millisecond)), len(d.colors)-1)
	return time.Duration(d.next) * 100 * time.Millisecond, nil
}

func (d *seekingDecoder) Duration() time.Duration {
	return time.Duration(len(d.colors)) * 100 * time.Millisecond
}

func (d *seekingDecoder) decoded() int { d.mu.Lock(); defer d.mu.Unlock(); return d.nexts }

// TestVideoSeeker checks that a player seeks a VideoSeeker at once,
// decoding the frame it goes to alone, rather than every frame before it.
func TestVideoSeeker(t *testing.T) {
	colors := make([]color.RGBA, 50)
	for i := range colors {
		colors[i] = videoRed
	}
	colors[42], colors[49] = videoGreen, videoBlue
	d := &seekingDecoder{colors: colors}
	p, err := NewVideoPlayer(d)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	tt, shows := videoTester(p, nil)
	decodedAhead(t, p, 2)
	p.Pause()
	before := d.decoded()
	p.Seek(4250 * time.Millisecond)
	tt.Frame()
	if got, pos := shows(), p.Position(); got != videoGreen || pos != 4250*time.Millisecond {
		t.Fatalf("sought to 4.25s: %v at %v", got, pos)
	}
	if n := d.decoded() - before; n > 1 {
		t.Errorf("seeking decoded %d frames", n)
	}
	p.Play()
	p.Seek(time.Hour)
	tt.Frame()
	if got, pos := shows(), p.Position(); got != videoBlue || pos != 5*time.Second || !p.Ended() {
		t.Fatalf("sought past the end: %v at %v, ended %v", got, pos, p.Ended())
	}
}
