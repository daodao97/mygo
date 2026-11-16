package render

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestInterpolate(t *testing.T) {
	in, out := []float64{0, 10, 20}, []float64{0, 1, 0}
	for _, c := range []struct {
		x    float64
		o    Interpolation
		want float64
	}{
		{5, Interpolation{}, 0.5},
		{10, Interpolation{}, 1},
		{15, Interpolation{}, 0.5},
		{-5, Interpolation{}, 0},
		{25, Interpolation{}, 0},
		{-5, Interpolation{Left: Extend}, -0.5},
		{25, Interpolation{Right: Extend}, -0.5},
		{-5, Interpolation{Left: Identity}, -5},
		{5, Interpolation{Easing: func(t float32) float32 { return t * t }}, 0.25},
	} {
		if got := Interpolate(c.x, in, out, c.o); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("Interpolate(%v, %+v) = %v, want %v", c.x, c.o, got, c.want)
		}
	}
	for _, bad := range [][2][]float64{{{0}, {0}}, {{0, 1}, {0}}, {{1, 0}, {0, 1}}, {{0, 0}, {0, 1}}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Interpolate(%v, %v) did not panic", bad[0], bad[1])
				}
			}()
			Interpolate(0, bad[0], bad[1])
		}()
	}
	red, blue := ui.RGB(255, 0, 0), ui.RGB(0, 0, 255)
	if c := InterpolateColor(5, []float64{0, 10}, []ui.Color{red, blue}); c != red.Mix(blue, 0.5) {
		t.Errorf("InterpolateColor = %v", c)
	}
	if c := InterpolateColor(20, []float64{0, 10}, []ui.Color{red, blue}); c != blue {
		t.Errorf("InterpolateColor beyond the range = %v", c)
	}
}

func TestFrame(t *testing.T) {
	f := Frame{Index: 45, Count: 90, FPS: 30}
	if f.Time() != 1500*time.Millisecond || f.Seconds() != 1.5 {
		t.Errorf("Time = %v, Seconds = %v", f.Time(), f.Seconds())
	}
	if f.Frames(2*time.Second) != 60 {
		t.Errorf("Frames(2s) = %d", f.Frames(2*time.Second))
	}
	if g, ok := f.Shift(30, 30); !ok || g.Index != 15 || g.Count != 30 {
		t.Errorf("Shift(30, 30) = %+v, %v", g, ok)
	}
	if _, ok := f.Shift(60, 10); ok {
		t.Error("a sequence that starts later shows")
	}
	if _, ok := f.Shift(0, 45); ok {
		t.Error("a sequence that ended shows")
	}
	if g, ok := f.Shift(40, 0); !ok || g.Index != 5 || g.Count != 50 {
		t.Errorf("Shift(40, 0) = %+v, %v: lasts to the end", g, ok)
	}
	if g := f.Loop(20); g.Index != 5 || g.Count != 20 {
		t.Errorf("Loop(20) = %+v", g)
	}
	if p := (Frame{Index: 89, Count: 90}).Progress(); p != 1 {
		t.Errorf("Progress of the last frame = %v", p)
	}
	if n, d := (&Composition{FPS: 29.97}).rate(); n != 30000 || d != 1001 {
		t.Errorf("29.97 fps = %d/%d", n, d)
	}
	if n, d := (&Composition{}).rate(); n != 30 || d != 1 {
		t.Errorf("the default rate = %d/%d", n, d)
	}
}

func TestSpring(t *testing.T) {
	o := ui.Snappy.Spring
	f := Frame{FPS: 60}
	last := -1.0
	for i := range 20 {
		f.Index = i
		v := f.Spring(o)
		if v < last {
			t.Fatalf("the spring went back at frame %d: %v after %v", i, v, last)
		}
		last = v
	}
	settle := f.Frames(o.SettleTime())
	f.Index = settle
	if v := f.Spring(o); math.Abs(v-1) > 0.002 {
		t.Errorf("at rest the spring is at %v", v)
	}
	f.Index = 30
	if v := f.SpringIn(o, 30); math.Abs(v-1) > 0.002 {
		t.Errorf("SpringIn(30) at frame 30 = %v", v)
	}
	f.Index = 0
	if v := f.Spring(o); v != 0 {
		t.Errorf("the spring starts at %v", v)
	}
}

// boxes is a composition of a blue box on red, moving right one pixel a
// frame, of w×h pixels.
func boxes(w, h, frames int) Composition {
	return Composition{
		Width: w, Height: h, FPS: 30, Frames: frames, Background: ui.RGB(255, 0, 0),
		View: func(c *ui.Context, f Frame) {
			ui.Box(c).Size(20, 20).Background(ui.RGB(0, 0, 255)).Translate(float32(f.Index), 0)
		},
	}
}

func at(img *image.RGBA, x, y int) [4]uint8 {
	i := img.PixOffset(x, y)
	return [4]uint8(img.Pix[i : i+4])
}

func TestStill(t *testing.T) {
	img, err := Still(boxes(64, 48, 30), 10, Options{Renderer: CPU})
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 64 || b.Dy() != 48 {
		t.Fatalf("a still of %v", b)
	}
	if p := at(img, 15, 10); p != [4]uint8{0, 0, 255, 255} {
		t.Errorf("in the box at frame 10: %v", p)
	}
	if p := at(img, 5, 10); p != [4]uint8{255, 0, 0, 255} {
		t.Errorf("left of the box at frame 10: %v", p)
	}
	if _, err := Still(boxes(64, 48, 30), 30, Options{}); err == nil {
		t.Error("a still after the end")
	}
}

func TestScale(t *testing.T) {
	// 90×60 at scale 1.5 is a view of 60×40 DIPs, the box 30 pixels wide.
	comp := boxes(90, 60, 1)
	comp.Scale = 1.5
	img, err := Still(comp, 0, Options{Renderer: CPU})
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 90 || b.Dy() != 60 {
		t.Fatalf("a still of %v", b)
	}
	if at(img, 28, 5)[2] != 255 || at(img, 31, 5)[0] != 255 {
		t.Errorf("the box is not 30 pixels wide: %v %v", at(img, 28, 5), at(img, 31, 5))
	}
}

func TestTransparent(t *testing.T) {
	comp := boxes(40, 30, 1)
	comp.Transparent = true
	img, err := Still(comp, 0, Options{Renderer: CPU})
	if err != nil {
		t.Fatal(err)
	}
	if p := at(img, 30, 25); p[3] != 0 {
		t.Errorf("outside the box: %v, want transparent", p)
	}
	if p := at(img, 5, 5); p != [4]uint8{0, 0, 255, 255} {
		t.Errorf("in the box: %v", p)
	}
}

// TestGPU draws the same frames on the GPU as on the CPU.
func TestGPU(t *testing.T) {
	comp := Composition{
		Width: 160, Height: 90, Frames: 20, Background: ui.RGB(250, 250, 250),
		View: func(c *ui.Context, f Frame) {
			ui.Column(c).Fill().Padding(10).Gap(6).Children(func() {
				ui.Box(c).Size(40, 20).Background(ui.Hex("#2563eb")).Radius(6).Opacity(float32(f.Progress()))
				ui.Text(c, "MyGo").FontSize(20)
			})
		},
	}
	var cpu []*image.RGBA
	if err := Frames(context.Background(), comp, Options{Renderer: CPU}, func(_ int, img *image.RGBA) error {
		cpu = append(cpu, img)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	name := ""
	i := 0
	err := Frames(context.Background(), comp, Options{Renderer: GPU, Progress: func(p Progress) { name = p.Renderer }}, func(_ int, img *image.RGBA) error {
		if d := maxDiff(img, cpu[i]); d > 64 {
			t.Errorf("frame %d differs by %d from the CPU's", i, d)
		}
		i++
		return nil
	})
	if err != nil {
		t.Skip("no GPU renderer:", err)
	}
	if i != len(cpu) {
		t.Errorf("%d frames on the GPU, %d on the CPU", i, len(cpu))
	}
	t.Log(name)
}

func maxDiff(a, b *image.RGBA) int {
	d := 0
	for i := range a.Pix {
		d = max(d, int(a.Pix[i])-int(b.Pix[i]), int(b.Pix[i])-int(a.Pix[i]))
	}
	return d
}

// TestDeterministic renders the same frames each time, and a range the
// same frames a whole render does, ui's animations included, which move
// with the composition's clock.
func TestDeterministic(t *testing.T) {
	var values []float32
	comp := Composition{
		Width: 80, Height: 40, FPS: 60, Frames: 60,
		View: func(c *ui.Context, f Frame) {
			show := f.Index >= 10
			b := ui.Box(c).Size(40, 40).Background(ui.RGB(0, 128, 0))
			target := float32(0)
			if show {
				target = 1
			}
			o := b.AnimateWith("o", target, 500*time.Millisecond, ui.Linear)
			b.Opacity(o)
			values = append(values, o)
		},
	}
	render := func(o Options) []*image.RGBA {
		values = values[:0]
		var out []*image.RGBA
		if err := Frames(context.Background(), comp, o, func(_ int, img *image.RGBA) error {
			out = append(out, img)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	a := render(Options{Renderer: CPU})
	// The value moves from frame 10 for half a second, 30 frames.
	if v := values[len(values)-60+25]; math.Abs(float64(v)-0.5) > 0.02 {
		t.Errorf("15 frames into a half second animation at 60 fps: %v", v)
	}
	b := render(Options{Renderer: CPU})
	for i := range a {
		if maxDiff(a[i], b[i]) != 0 {
			t.Fatalf("frame %d differs between renders", i)
		}
	}
	part := render(Options{Renderer: CPU, From: 20, To: 30})
	if len(part) != 10 {
		t.Fatalf("%d frames from 20 to 30", len(part))
	}
	for i, img := range part {
		if maxDiff(img, a[20+i]) != 0 {
			t.Errorf("frame %d of a range differs from the whole render's", 20+i)
		}
	}
}

func TestCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	err := Frames(ctx, boxes(32, 32, 100), Options{Renderer: CPU}, func(int, *image.RGBA) error {
		n++
		if n == 5 {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) || n > 6 {
		t.Errorf("canceled after 5 frames: %v, %d frames", err, n)
	}
	stop := errors.New("stop")
	if err := Frames(context.Background(), boxes(32, 32, 10), Options{}, func(int, *image.RGBA) error { return stop }); err != stop {
		t.Errorf("fn's error: %v", err)
	}
}

func TestImages(t *testing.T) {
	dir := t.TempDir()
	comp := boxes(40, 30, 6)
	comp.Transparent = true
	if err := Images(context.Background(), comp, filepath.Join(dir, "frames", "f-%03d.png"), Options{Renderer: CPU}); err != nil {
		t.Fatal(err)
	}
	for i := range 6 {
		f, err := os.Open(filepath.Join(dir, "frames", fmt.Sprintf("f-%03d.png", i)))
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		r, g, b, a := img.At(i+5, 5).RGBA()
		if b>>8 != 255 || a>>8 != 255 || r != 0 || g != 0 {
			t.Errorf("frame %d: the box is %v %v %v %v", i, r>>8, g>>8, b>>8, a>>8)
		}
		if _, _, _, a := img.At(35, 25).RGBA(); a != 0 {
			t.Errorf("frame %d: transparent outside the box, alpha %d", i, a>>8)
		}
	}
	if err := Images(context.Background(), comp, filepath.Join(dir, "x.png"), Options{}); err == nil {
		t.Error("a pattern without a verb")
	}
	if err := Images(context.Background(), comp, filepath.Join(dir, "%d.jpg"), Options{Renderer: CPU, Quality: 80}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "5.jpg")); err != nil {
		t.Error(err)
	}
}

// tone returns a second of a sine at hz, mono, at rate.
func tone(hz float64, rate int) *Sound {
	pcm := make([]float32, rate)
	for i := range pcm {
		pcm[i] = 0.5 * float32(math.Sin(2*math.Pi*hz*float64(i)/float64(rate)))
	}
	return SoundOf(rate, 1, pcm)
}

// beeping is a composition playing a beep in its frames 10 to 30.
func beeping(w, h, frames int, beep *Sound) Composition {
	comp := boxes(w, h, frames)
	view := comp.View
	comp.View = func(c *ui.Context, f Frame) {
		view(c, f)
		f.Sequence(c, 10, 20, func(f Frame) { f.Audio(beep) })
	}
	return comp
}

func TestSoundtrack(t *testing.T) {
	beep := tone(440, sampleRate)
	track, err := soundtrack(context.Background(), beeping(32, 32, 60, beep), 0, 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(track) != 2*2*sampleRate {
		t.Fatalf("%d samples for 2 seconds", len(track))
	}
	// Frames 10 to 30 at 30 fps: from a third of a second for two thirds.
	start, end := sampleRate/3, sampleRate
	for i := 0; i < len(track)/2; i++ {
		l, r := track[2*i], track[2*i+1]
		if l != r {
			t.Fatalf("a mono sound differs between channels at %d", i)
		}
		in := i >= start && i < end
		if !in && l != 0 {
			t.Fatalf("sound at %d, outside the sequence", i)
		}
		if in {
			want := math.Round(0.5 * math.Sin(2*math.Pi*440*float64(i-start)/sampleRate) * 32767)
			if math.Abs(float64(l)-want) > 2 {
				t.Fatalf("at %d: %d, want %v", i, l, want)
			}
		}
	}
	if track, _ := soundtrack(context.Background(), boxes(32, 32, 10), 0, 10); track != nil {
		t.Error("a sound track for a silent composition")
	}
	// A range: the track starts at its first frame.
	part, _ := soundtrack(context.Background(), beeping(32, 32, 60, beep), 20, 40)
	if len(part) != 2*2*sampleRate/3 || part[0] == 0 && part[2] == 0 {
		t.Errorf("a range's track: %d samples, starting %v", len(part), part[:4])
	}
}

func TestVolume(t *testing.T) {
	beep := tone(440, sampleRate)
	comp := Composition{Width: 16, Height: 16, Frames: 30, View: func(c *ui.Context, f Frame) {
		f.Audio(beep, AudioOptions{Volume: f.Interpolate([]float64{0, 29}, []float64{0.001, 1})})
	}}
	track, err := soundtrack(context.Background(), comp, 0, 30)
	if err != nil {
		t.Fatal(err)
	}
	peak := func(from, to int) (p int16) {
		for _, x := range track[2*from : 2*to] {
			p = max(p, x)
		}
		return p
	}
	if a, b := peak(0, 1600), peak(len(track)/2-1600, len(track)/2); a >= b/4 {
		t.Errorf("a fade in: %d at the start, %d at the end", a, b)
	}
}

func TestResample(t *testing.T) {
	// A sine at 44.1 kHz, resampled, is the same sine at 48 kHz.
	s := tone(1000, 44100)
	out := make([]float32, 2*4800)
	s.mix(out, 0.25, 4800, 1, 1, false, 0)
	for k := 0; k < 4800; k += 7 {
		want := 0.5 * math.Sin(2*math.Pi*1000*(0.25+float64(k)/sampleRate))
		if d := math.Abs(float64(out[2*k]) - want); d > 0.01 {
			t.Fatalf("at %d: %v, want %v", k, out[2*k], want)
		}
	}
}

// mp4Tracks returns the handler type and sample count of each track of
// an MP4 or QuickTime file.
func mp4Tracks(t *testing.T, path string) map[string]int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tracks := map[string]int{}
	var walk func(b []byte, handler *string)
	walk = func(b []byte, handler *string) {
		for len(b) >= 8 {
			size := int(binary.BigEndian.Uint32(b))
			kind := string(b[4:8])
			head := 8
			if size == 1 && len(b) >= 16 {
				size, head = int(binary.BigEndian.Uint64(b[8:])), 16
			} else if size == 0 {
				size = len(b)
			}
			if size < head || size > len(b) {
				return
			}
			body := b[head:size]
			switch kind {
			case "moov", "mdia", "minf", "stbl":
				walk(body, handler)
			case "trak":
				var h string
				walk(body, &h)
			case "hdlr":
				// The media's; QuickTime's minf has one of its data too.
				if len(body) >= 12 && handler != nil && *handler == "" {
					*handler = string(body[8:12])
				}
			case "stsz":
				if len(body) >= 12 && handler != nil {
					tracks[*handler] = int(binary.BigEndian.Uint32(body[8:]))
				}
			}
			b = b[size:]
		}
	}
	walk(data, nil)
	return tracks
}

func TestVideo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "beep.mp4")
	comp := beeping(320, 180, 45, tone(440, sampleRate))
	err := Video(context.Background(), comp, path, Options{})
	if errors.Is(err, errors.ErrUnsupported) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if out := os.Getenv("MYGO_RENDER_OUT"); out != "" {
		// For a look at what the system's encoder made.
		if data, err := os.ReadFile(path); err == nil {
			os.WriteFile(filepath.Join(out, "beep.mp4"), data, 0o644)
		}
	}
	tracks := mp4Tracks(t, path)
	if tracks["vide"] != 45 {
		t.Errorf("%d video frames, want 45: %v", tracks["vide"], tracks)
	}
	if tracks["soun"] == 0 {
		t.Errorf("no sound: %v", tracks)
	}
	// No file but the video is left behind.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("files left: %v", entries)
	}

	// Without sound, no sound track; a range, its frames.
	silent := filepath.Join(dir, "silent.mp4")
	if err := Video(context.Background(), boxes(320, 180, 45), silent, Options{From: 15, To: 30, Renderer: CPU}); err != nil {
		t.Fatal(err)
	}
	if tracks := mp4Tracks(t, silent); tracks["vide"] != 15 || tracks["soun"] != 0 {
		t.Errorf("a silent range: %v", tracks)
	}

	if err := Video(context.Background(), boxes(321, 180, 10), filepath.Join(dir, "odd.mp4"), Options{}); err == nil {
		t.Error("H.264 of an odd width")
	}
	if err := Video(context.Background(), boxes(320, 180, 10), filepath.Join(dir, "x.avi"), Options{}); err == nil {
		t.Error("an .avi")
	}
}

// mp4FrameTimes returns the time scale of the video track of an MP4 or a
// QuickTime movie, and how long its frames last in it: the runs of its
// stts, each a count of frames and their duration.
func mp4FrameTimes(t *testing.T, path string) (scale uint32, runs [][2]uint32) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	type track struct {
		handler string
		scale   uint32
		runs    [][2]uint32
	}
	var walk func(b []byte, tr *track)
	walk = func(b []byte, tr *track) {
		for len(b) >= 8 {
			size := int(binary.BigEndian.Uint32(b))
			kind := string(b[4:8])
			head := 8
			if size == 1 && len(b) >= 16 {
				size, head = int(binary.BigEndian.Uint64(b[8:])), 16
			} else if size == 0 {
				size = len(b)
			}
			if size < head || size > len(b) {
				return
			}
			body := b[head:size]
			switch {
			case kind == "moov", kind == "mdia", kind == "minf", kind == "stbl":
				walk(body, tr)
			case kind == "trak":
				var found track
				walk(body, &found)
				if found.handler == "vide" {
					scale, runs = found.scale, found.runs
				}
			case tr == nil:
			case kind == "hdlr" && len(body) >= 12 && tr.handler == "":
				tr.handler = string(body[8:12])
			case kind == "mdhd" && len(body) >= 24:
				// After the times of its making, of 8 bytes each in version 1.
				at := 12
				if body[0] == 1 {
					at = 20
				}
				tr.scale = binary.BigEndian.Uint32(body[at:])
			case kind == "stts" && len(body) >= 8:
				for b := body[8:]; len(b) >= 8; b = b[8:] {
					tr.runs = append(tr.runs, [2]uint32{binary.BigEndian.Uint32(b), binary.BigEndian.Uint32(b[4:])})
				}
			}
			b = b[size:]
		}
	}
	walk(data, nil)
	return scale, runs
}

// TestVideoFrameTimes checks that every frame of a video lasts what its
// rate says: 1001/30000 s at 29.97 frames a second, which a track counted
// in the 1/600 s of AVFoundation cannot say.
func TestVideoFrameTimes(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Media Foundation takes times in 100 ns, which 1001/30000 s is no
		// whole number of either: what it writes is not known here.
		t.Skip("not checked with Media Foundation")
	}
	dir := t.TempDir()
	const frames = 30
	for _, fps := range []float64{30, 29.97, 23.976, 59.94, 48, 240} {
		for _, name := range []string{"frames.mp4", "frames.mov"} {
			comp := boxes(320, 180, frames)
			comp.FPS = fps
			path := filepath.Join(dir, name)
			err := Video(context.Background(), comp, path, Options{Renderer: CPU})
			if errors.Is(err, errors.ErrUnsupported) {
				t.Skip(err)
			}
			if err != nil {
				t.Fatal(err)
			}
			num, den := comp.rate()
			scale, runs := mp4FrameTimes(t, path)
			if len(runs) != 1 || runs[0][0] != frames || int64(runs[0][1])*num != den*int64(scale) {
				t.Errorf("%v frames a second in %s: frames last %v (count, duration) of 1/%d s, want %d of %d/%d s", fps, name, runs, scale, frames, den, num)
			}
		}
	}
}

func TestVideoCanceled(t *testing.T) {
	// Canceled while the sound is mixed, and while the frames draw.
	for _, mixing := range []bool{true, false} {
		dir := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		comp := beeping(320, 180, 300, tone(440, sampleRate))
		o := Options{Renderer: CPU}
		if mixing {
			view := comp.View
			comp.View = func(c *ui.Context, f Frame) {
				view(c, f)
				if f.Index == 20 {
					cancel()
				}
			}
		} else {
			o.Progress = func(p Progress) {
				if p.Frame == 20 {
					cancel()
				}
			}
		}
		err := Video(ctx, comp, filepath.Join(dir, "canceled.mp4"), o)
		if errors.Is(err, errors.ErrUnsupported) {
			t.Skip(err)
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("canceled: %v", err)
		}
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Errorf("files left: %v", entries)
		}
	}
}

// TestCodecs writes each codec the system has into its container.
func TestCodecs(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name  string
		codec Codec
	}{{"hevc.mp4", HEVC}, {"hevc.mov", HEVC}, {"h264.mov", H264}, {"prores.mov", ProRes}, {"alpha.mov", ProRes4444}} {
		comp := beeping(160, 90, 20, tone(440, sampleRate))
		comp.Transparent = c.codec == ProRes4444
		path := filepath.Join(dir, c.name)
		err := Video(context.Background(), comp, path, Options{Codec: c.codec})
		if errors.Is(err, errors.ErrUnsupported) {
			t.Logf("%s: %v", c.name, err)
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if tracks := mp4Tracks(t, path); tracks["vide"] != 20 || tracks["soun"] == 0 {
			t.Errorf("%s: %v", c.name, tracks)
		}
	}
	for _, bad := range []struct {
		name  string
		codec Codec
	}{{"prores.mp4", ProRes}, {"vp9.mp4", VP9}, {"h264.webm", H264}} {
		if err := Video(context.Background(), boxes(160, 90, 2), filepath.Join(dir, bad.name), Options{Codec: bad.codec}); err == nil || errors.Is(err, errors.ErrUnsupported) {
			t.Errorf("%s: %v, want an error of the format", bad.name, err)
		}
	}
}

func TestFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	dir := t.TempDir()
	for _, name := range []string{"beep.mp4", "beep.webm"} {
		path := filepath.Join(dir, name)
		if err := Video(context.Background(), beeping(160, 90, 40, tone(440, sampleRate)), path, Options{Encoder: FFmpeg, Renderer: CPU}); err != nil {
			t.Fatal(err)
		}
		if filepath.Ext(name) == ".mp4" {
			if tracks := mp4Tracks(t, path); tracks["vide"] != 40 || tracks["soun"] == 0 {
				t.Errorf("%s: %v", name, tracks)
			}
		} else if fi, err := os.Stat(path); err != nil || fi.Size() == 0 {
			t.Errorf("%s: %v", name, err)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 2 {
		t.Errorf("files left: %v", entries)
	}
}

// TestPlayer shows a composition's frames in a window, as the video has
// them, and seeks without drawing the frames before.
func TestPlayer(t *testing.T) {
	comp := Composition{Width: 40, Height: 40, FPS: 10, Frames: 30, View: func(c *ui.Context, f Frame) {
		ui.Box(c).Fill().Background(InterpolateColor(float64(f.Index), []float64{0, 29}, []ui.Color{ui.RGB(255, 0, 0), ui.RGB(0, 0, 255)}))
	}}
	p, err := NewPlayer(comp)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if p.Duration() != 3*time.Second {
		t.Errorf("a duration of %v", p.Duration())
	}
	tt := ui.NewTester(func(c *ui.Context) { ui.Video(c, p).Size(40, 40) }, 40, 40)
	p.Pause()
	for _, c := range []struct {
		at    time.Duration
		frame int
	}{{2050 * time.Millisecond, 20}, {500 * time.Millisecond, 5}, {time.Hour, 29}} {
		p.Seek(c.at)
		tt.Frame()
		want := InterpolateColor(float64(c.frame), []float64{0, 29}, []ui.Color{ui.RGB(255, 0, 0), ui.RGB(0, 0, 255)})
		got := tt.Image().RGBAAt(20, 20)
		if d := max(absDiff(got.R, want.R), absDiff(got.B, want.B)); d > 2 {
			t.Errorf("sought to %v: %v, want frame %d's %v", c.at, got, c.frame, want)
		}
	}
}

func absDiff(a, b uint8) int { return max(int(a)-int(b), int(b)-int(a)) }

// TestNV12 converts colors as BT.709 in the limited range has them.
func TestNV12(t *testing.T) {
	for _, c := range []struct {
		bgra      [4]byte
		y, cb, cr byte
	}{
		{[4]byte{0, 0, 0, 255}, 16, 128, 128},
		{[4]byte{255, 255, 255, 255}, 235, 128, 128},
		{[4]byte{0, 0, 255, 255}, 63, 102, 240}, // red
		{[4]byte{0, 255, 0, 255}, 173, 42, 26},  // green
		{[4]byte{255, 0, 0, 255}, 32, 240, 118}, // blue
		{[4]byte{0, 0, 0, 0}, 16, 128, 128},     // transparent: black
	} {
		src := make([]byte, 4*4*2)
		for i := 0; i < len(src); i += 4 {
			copy(src[i:], c.bgra[:])
		}
		dst := make([]byte, 4*2*3/2)
		bgraToNV12(dst, src, 4, 2)
		if dst[0] != c.y || dst[7] != c.y || dst[8] != c.cb || dst[9] != c.cr {
			t.Errorf("%v: Y %d, Cb %d, Cr %d, want %d %d %d", c.bgra, dst[0], dst[8], dst[9], c.y, c.cb, c.cr)
		}
	}
}

// TestPlayerSeeksToFrameStarts seeks to the time a frame starts at, as
// stepping frame by frame does, without ending the video.
func TestPlayerSeeksToFrameStarts(t *testing.T) {
	comp := boxes(32, 32, 90)
	p, err := NewPlayer(comp)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	tt := ui.NewTester(func(c *ui.Context) { ui.Video(c, p).Size(32, 32) }, 32, 32)
	p.Pause()
	s := newSession(comp)
	defer s.rec.Close()
	for _, i := range []int{1, 2, 45, 88} {
		p.Seek(s.at(i))
		tt.Frame()
		if p.Ended() || p.Position() != s.at(i) {
			t.Errorf("sought to frame %d's start: ended %v at %v", i, p.Ended(), p.Position())
		}
	}
}

// TestSoundAcrossFrames plays a sound without a jump where frames meet,
// at a rate whose frames do not hold whole samples.
func TestSoundAcrossFrames(t *testing.T) {
	beep := tone(1000, sampleRate)
	comp := Composition{Width: 16, Height: 16, FPS: 29.97, Frames: 20, View: func(c *ui.Context, f Frame) {
		f.Sequence(c, 3, 0, func(f Frame) { f.Audio(beep) })
	}}
	track, err := soundtrack(context.Background(), comp, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	m := newMixer(30000, 1001)
	start := m.sampleAt(3)
	// From past the kernel's reach of the sound's onset.
	for k := int(start) + reach; k < len(track)/2; k++ {
		// The sound started at frame 3's time, between two samples.
		at := float64(k)/sampleRate - 3*1001.0/30000
		if at >= 1 {
			break
		}
		want := 0.0
		if at >= 0 {
			want = 32767 * 0.5 * math.Sin(2*math.Pi*1000*at)
		}
		if d := math.Abs(float64(track[2*k]) - want); d > 60 {
			t.Fatalf("sample %d: %d, want %.0f", k, track[2*k], want)
		}
	}
}

// TestMotionBlur makes each frame the mean of samples over the shutter's
// time: a box moving 4 pixels a frame covers, at frame 5 with 4 samples
// over the whole frame, the pixels from 17 to 28, the more the later.
func TestMotionBlur(t *testing.T) {
	comp := Composition{
		Width: 48, Height: 8, Frames: 10, Background: ui.RGB(255, 255, 255),
		MotionBlur: MotionBlur{Samples: 4, Shutter: 360},
		View: func(c *ui.Context, f Frame) {
			ui.Box(c).Size(8, 8).Background(ui.RGB(0, 0, 0)).Translate(float32(4*f.Exact()), 0)
			ui.Box(c).Absolute().Left(40).Top(0).Size(8, 8).Background(ui.RGB(255, 0, 0)) // still
		},
	}
	for _, r := range []Renderer{CPU, GPU} {
		img, err := Still(comp, 5, Options{Renderer: r})
		if r == GPU && err != nil {
			t.Log("no GPU:", err)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		// Samples at frames 4.25, 4.5, 4.75 and 5: the box at 17, 18, 19, 20.
		for x, covered := range map[int]int{16: 0, 17: 1, 18: 2, 19: 3, 20: 4, 24: 4, 25: 3, 26: 2, 27: 1, 28: 0} {
			want := 255 - (255*covered+2)/4
			if got := int(at(img, x, 4)[0]); absDiff(uint8(got), uint8(want)) > 2 {
				t.Errorf("%v: x %d: %d, want %d (%d of 4 samples)", r, x, got, want, covered)
			}
		}
		if p := at(img, 44, 4); p != [4]uint8{255, 0, 0, 255} {
			t.Errorf("%v: what stands still blurs: %v", r, p)
		}
	}
	comp.MotionBlur = MotionBlur{Samples: 100}
	if _, err := Still(comp, 0, Options{}); err == nil {
		t.Error("100 samples")
	}
}

// TestMotionBlurOfUI blurs ui's own animations, which follow the clock,
// and writes a video of it.
func TestMotionBlurOfUI(t *testing.T) {
	comp := Composition{
		Width: 64, Height: 16, Frames: 12, MotionBlur: MotionBlur{Samples: 5},
		View: func(c *ui.Context, f Frame) {
			b := ui.Box(c).Size(16, 16).Background(ui.RGB(0, 0, 255))
			x := b.AnimateWith("x", float32(min(f.Index, 1)*48), 300*time.Millisecond, ui.Linear)
			b.Translate(x, 0)
		},
	}
	img, err := Still(comp, 4, Options{Renderer: CPU})
	if err != nil {
		t.Fatal(err)
	}
	partial := 0
	for x := range 64 {
		// Red: none in the box, all in the light background.
		if r := at(img, x, 8)[0]; r > 10 && r < 235 {
			partial++
		}
	}
	if partial < 2 {
		t.Errorf("a box moving with ui's animation shows no blur: %d pixels between", partial)
	}
	dir := t.TempDir()
	// As large as Media Foundation's encoder takes.
	comp.Width, comp.Height = 160, 96
	err = Video(context.Background(), comp, filepath.Join(dir, "blur.mp4"), Options{})
	if errors.Is(err, errors.ErrUnsupported) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if tracks := mp4Tracks(t, filepath.Join(dir, "blur.mp4")); tracks["vide"] != 12 {
		t.Errorf("a video with motion blur: %v", tracks)
	}
}
