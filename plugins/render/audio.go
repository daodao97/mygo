package render

import (
	"bytes"
	"errors"
	"io"
	"math"
	"os"
	"sync"
	"time"

	"github.com/egoist/mygo/audio"
)

// sampleRate is the rate of the sound videos carry: 48 kHz, stereo.
const sampleRate = 48000

// A Sound is audio a composition plays (Frame.Audio), decoded into memory
// once, so that any part of it plays at any frame. WAV decodes out of
// the box; import audio/mp3, audio/vorbis, audio/flac or audio/aac for
// theirs, as package audio's formats register.
type Sound struct {
	// samples are interleaved frames of channels samples, at rate.
	samples  []float32
	channels int
	rate     int
	frames   int
}

// OpenSound decodes the audio file at path.
func OpenSound(path string) (*Sound, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	d, _, err := audio.NewDecoder(f)
	if err != nil {
		return nil, err
	}
	return NewSound(d)
}

// DecodeSound decodes audio in memory, as a file embedded in the program.
func DecodeSound(data []byte) (*Sound, error) {
	d, _, err := audio.NewDecoder(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return NewSound(d)
}

// NewSound decodes all d has, as sound an app makes itself.
func NewSound(d audio.Decoder) (*Sound, error) {
	ch, rate := d.Channels(), d.SampleRate()
	if ch < 1 || rate <= 0 {
		return nil, errors.New("render: a sound of no channels or sample rate")
	}
	keep := min(ch, 2)
	var samples []float32
	if n := d.Len(); n > 0 && n < 1<<26 {
		samples = make([]float32, 0, n*int64(keep))
	}
	buf := make([]float32, 4096/ch*ch)
	for {
		n, err := d.Read(buf)
		n -= n % ch
		for i := 0; i < n; i += ch {
			samples = append(samples, buf[i:i+keep]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, io.ErrNoProgress
		}
	}
	return &Sound{samples: samples, channels: keep, rate: rate, frames: len(samples) / keep}, nil
}

// SoundOf returns a sound of samples, interleaved frames of channels (1
// or 2) samples in [-1, 1] at rate frames a second, as an app makes them.
func SoundOf(rate, channels int, samples []float32) *Sound {
	if channels != 1 && channels != 2 {
		panic("render: SoundOf takes 1 or 2 channels")
	}
	if rate <= 0 {
		panic("render: SoundOf takes a positive rate")
	}
	samples = samples[:len(samples)/channels*channels]
	return &Sound{samples: samples, channels: channels, rate: rate, frames: len(samples) / channels}
}

// Duration returns how long the sound plays.
func (s *Sound) Duration() time.Duration {
	return time.Duration(float64(s.frames) * float64(time.Second) / float64(s.rate))
}

// AudioOptions are how Frame.Audio plays a sound.
type AudioOptions struct {
	// Volume scales the sound, 1 when zero; it may change from frame to
	// frame, as a fade, and moves smoothly across each.
	Volume float64
	// Start is where in the sound it starts playing, cutting what comes
	// before.
	Start time.Duration
	// Loop plays the sound again from Start each time it ends, rather
	// than fall silent.
	Loop bool
}

// Audio plays s during the frame, from where it is at this frame of the
// composition or the sequence playing it: the sound starts at its frame
// 0, so that a sound played inside a Sequence starts with the sequence.
// A view plays a sound in each frame it is heard, as it shows an element
// in each frame it is seen:
//
//	f.Audio(music, render.AudioOptions{Volume: f.Interpolate([]float64{0, 30}, []float64{0, 1})})
//
// Sounds played in the frames of Images, Frames and Still are not
// written anywhere.
func (f Frame) Audio(s *Sound, o ...AudioOptions) {
	if f.s == nil || s == nil || f.Index < 0 {
		return
	}
	var opt AudioOptions
	if len(o) > 0 {
		opt = o[0]
	}
	vol := opt.Volume
	if vol == 0 {
		vol = 1
	}
	if vol < 0 || math.IsNaN(vol) || math.IsInf(vol, 0) {
		panic("render: Volume must be a finite number, 0 or more")
	}
	// The frame of the composition its frame 0 is: the sound played from
	// one start is one voice, whose volume moves smoothly.
	origin := f.at - f.Index
	f.s.plays = append(f.s.plays, play{
		sound: s,
		// When, in the composition, the sound is at 0: each sample's place
		// in the sound follows from its own time, so that frames of rates
		// whose samples do not divide evenly join without a jump.
		zero:   float64(int64(origin)*f.s.den)/float64(f.s.num) - opt.Start.Seconds(),
		origin: origin,
		start:  opt.Start,
		volume: vol,
		loop:   opt.Loop,
	})
}

// play is a sound a frame plays, at 0 zero seconds into the composition.
type play struct {
	sound  *Sound
	zero   float64
	origin int
	start  time.Duration
	volume float64
	loop   bool
}

// voice is a sound played from a start: what moves smoothly from frame to
// frame.
type voice struct {
	sound  *Sound
	origin int
	start  time.Duration
}

// mixer mixes the sounds each frame plays into stereo at 48 kHz.
type mixer struct {
	num, den int64
	// start is the first frame written: the sound starts there.
	start int
	// volumes are the voices' volumes at the end of the frame mixed last.
	volumes, next map[voice]float64
	out           []float32
}

func newMixer(num, den int64) *mixer {
	return &mixer{num: num, den: den, volumes: map[voice]float64{}, next: map[voice]float64{}}
}

// sampleAt returns the first sample of frame i of the video.
func (m *mixer) sampleAt(i int) int64 { return int64(i) * sampleRate * m.den / m.num }

// frame returns the sound of frame i: interleaved stereo samples, until
// the next call.
func (m *mixer) frame(i int, plays []play) []float32 {
	n := int(m.sampleAt(i+1) - m.sampleAt(i))
	m.out = append(m.out[:0], make([]float32, 2*n)...)
	out := m.out
	clear(m.next)
	for _, p := range plays {
		v := voice{p.sound, p.origin, p.start}
		from, ok := m.volumes[v]
		if !ok {
			from = p.volume
		}
		if _, dup := m.next[v]; dup {
			// The same sound played twice from one start: once.
			continue
		}
		m.next[v] = p.volume
		at := float64(m.sampleAt(i))/sampleRate - p.zero
		p.sound.mix(out, at, n, from, p.volume, p.loop, p.start.Seconds())
	}
	m.volumes, m.next = m.next, m.volumes
	for j, x := range out {
		out[j] = max(-1, min(x, 1))
	}
	return out
}

// mix adds n frames of the sound from at seconds into it to out, stereo
// at 48 kHz, its volume going from v0 to v1; looped, it goes on from loopAt
// seconds at its end.
func (s *Sound) mix(out []float32, at float64, n int, v0, v1 float64, loop bool, loopAt float64) {
	if s.frames == 0 {
		return
	}
	length := float64(s.frames) / float64(s.rate)
	step := float64(s.rate) / sampleRate
	pos := at * float64(s.rate)
	whole := s.rate == sampleRate && math.Abs(pos-math.Round(pos)) < 1e-6
	for k := range n {
		t := at + float64(k)/sampleRate
		if t >= length {
			if !loop || length <= loopAt {
				break
			}
			t = loopAt + math.Mod(t-loopAt, length-loopAt)
		}
		if t < 0 {
			continue
		}
		vol := float32(v0 + (v1-v0)*float64(k+1)/float64(n))
		var l, r float32
		if whole && !loop {
			i := int(math.Round(pos)) + k
			l, r = s.frame(i)
		} else {
			l, r = s.resample(t*float64(s.rate), step)
		}
		out[2*k] += l * vol
		out[2*k+1] += r * vol
	}
}

// frame returns the samples of frame i of the sound, left and right.
func (s *Sound) frame(i int) (l, r float32) {
	if i < 0 || i >= s.frames {
		return 0, 0
	}
	if s.channels == 1 {
		v := s.samples[i]
		return v, v
	}
	return s.samples[2*i], s.samples[2*i+1]
}

// The kernel resampling sounds of other rates: a sinc windowed by Kaiser
// over reach frames of the output either side, its cutoff a little below
// the lower of the two rates' Nyquist frequencies.
const (
	reach     = 16
	kaiserB   = 8.0
	kernelRes = 512 // entries of the kernel a frame
)

var (
	kernelOnce sync.Once
	kernel     []float32 // the window at distances 0 to reach, kernelRes a frame
)

func windowAt(d float64) float32 {
	kernelOnce.Do(func() {
		kernel = make([]float32, reach*kernelRes+2)
		for i := range kernel {
			x := float64(i) / kernelRes / reach
			if x > 1 {
				continue
			}
			kernel[i] = float32(bessel0(kaiserB*math.Sqrt(1-x*x)) / bessel0(kaiserB))
		}
	})
	d = math.Abs(d) * kernelRes
	i := int(d)
	if i+1 >= len(kernel) {
		return 0
	}
	f := float32(d - float64(i))
	return kernel[i]*(1-f) + kernel[i+1]*f
}

func bessel0(x float64) float64 {
	sum, term := 1.0, 1.0
	for k := 1; k < 50; k++ {
		term *= (x / (2 * float64(k))) * (x / (2 * float64(k)))
		sum += term
		if term < 1e-12*sum {
			break
		}
	}
	return sum
}

// resample returns the sound at frame position p, between its frames, as
// a band-limited interpolation does: step is the sound's frames an output
// frame, above 1 for sounds of higher rates, whose kernel widens and
// cuts off lower.
func (s *Sound) resample(p, step float64) (l, r float32) {
	scale := max(step, 1)
	cutoff := 0.95 / scale
	width := reach * scale
	c := int(math.Floor(p))
	var sl, sr float64
	for i := c - int(width) + 1; i <= c+int(width); i++ {
		d := p - float64(i)
		w := float64(windowAt(d/width*reach)) * sincf(d*cutoff) * cutoff
		if w == 0 {
			continue
		}
		a, b := s.frame(i)
		sl += float64(a) * w
		sr += float64(b) * w
	}
	return float32(sl), float32(sr)
}

func sincf(x float64) float64 {
	if x == 0 {
		return 1
	}
	x *= math.Pi
	return math.Sin(x) / x
}
