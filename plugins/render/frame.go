package render

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// A Frame is the frame of a composition a view builds: its Index, counted
// from 0 at the start of the composition, or of the sequence showing the
// view (Sequence, Shift), of Count frames at FPS frames a second.
//
// What a view shows follows from the frame alone, as a function of
// Index: Interpolate and Spring turn it into positions, opacities and
// colors, and Sequence shows a part of the video during some of its
// frames. ui's own animations (Animate, Spring, Transition) move with the
// video's clock too, but from what the frames before showed.
type Frame struct {
	// Index is the frame's, from 0, and Count the frames of the
	// composition or sequence.
	Index, Count int
	// FPS is the frames a second.
	FPS float64

	// s is the render the frame is of, nil for a Frame made by hand; at is
	// the index in the composition of this frame.
	s  *session
	at int
	// sub is how far, in frames, the time the view builds is from Index's:
	// before it, below 0, while motion blur samples the shutter's time.
	sub float64
}

// Exact returns where the frame is, in frames: Index, or a fraction
// before it while motion blur samples the time the shutter is open
// before the frame (Composition.MotionBlur). What moves with Exact,
// Seconds, Time, Interpolate and Spring blurs; what moves with Index,
// a whole number, does not.
func (f Frame) Exact() float64 { return float64(f.Index) + f.sub }

// Time returns how long after the start of the composition, or sequence,
// the frame shows.
func (f Frame) Time() time.Duration {
	return time.Duration(math.Round(f.Exact() * float64(time.Second) / f.fps()))
}

// Seconds returns Time in seconds.
func (f Frame) Seconds() float64 { return f.Exact() / f.fps() }

// Progress returns how far into its Count frames the frame is: 0 for the
// first, 1 for the last.
func (f Frame) Progress() float64 {
	if f.Count <= 1 {
		return 1
	}
	return min(max(f.Exact()/float64(f.Count-1), 0), 1)
}

func (f Frame) fps() float64 {
	if f.FPS > 0 {
		return f.FPS
	}
	return defaultFPS
}

// Frames returns how many frames d lasts at the frame's FPS, rounded.
func (f Frame) Frames(d time.Duration) int {
	return int(math.Round(d.Seconds() * f.fps()))
}

// Shift returns the frame as a sequence of count frames starting at from
// sees it, and whether the sequence shows then: Index is f.Index-from,
// within 0 and count. A count of 0 or less lasts to the end of f's.
func (f Frame) Shift(from, count int) (Frame, bool) {
	if count <= 0 {
		count = f.Count - from
	}
	g := f
	g.Index, g.Count = f.Index-from, count
	return g, g.Index >= 0 && g.Index < count
}

// Loop returns the frame as a sequence of count frames playing again and
// again sees it: Index is f.Index modulo count.
func (f Frame) Loop(count int) Frame {
	if count <= 0 {
		panic("render: Loop takes a positive count")
	}
	g := f
	g.Index, g.Count = ((f.Index%count)+count)%count, count
	if g.Index == 0 && f.sub < 0 && f.Index != 0 {
		// A sample just before the loop starts again is at its end.
		g.Index = count
	}
	return g
}

// Sequence shows view during count frames from from, given the frame as
// the sequence sees it (Shift), in a layer that fills the parent, above
// the layers before it, as video editors stack clips. A count of 0 or
// less lasts to the end of f's. The layer stays in the layout while the
// sequence does not show, empty, so that the elements after it keep their
// identities.
func (f Frame) Sequence(c *ui.Context, from, count int, view func(f Frame)) {
	g, ok := f.Shift(from, count)
	Layer(c).Children(func() {
		if ok {
			view(g)
		}
	})
}

// Layer returns a box filling its parent, laid out apart from its
// siblings, over those before it: for what overlaps, as the layers of a
// video.
func Layer(c *ui.Context) ui.Element {
	return ui.Box(c).Absolute().Top(0).Right(0).Bottom(0).Left(0)
}

// Interpolate maps f.Index from the frames in to the values out, as the
// package's Interpolate does.
func (f Frame) Interpolate(in, out []float64, o ...Interpolation) float64 {
	return Interpolate(f.Exact(), in, out, o...)
}

// Spring returns where a spring of o going from 0 to 1, starting at the
// first frame, is at this one: 0 at the start, past 1 where it
// overshoots, 1 once it rests (ui.SpringOptions.Progress). ui.Smooth,
// ui.Snappy and ui.Bouncy have the springs of ui's animations.
func (f Frame) Spring(o ui.SpringOptions) float64 {
	return o.Progress(f.Time())
}

// SpringIn returns where a spring of o is at this frame, as Spring does,
// its time stretched or shrunk so that it comes to rest in frames frames
// (ui.SpringOptions.SettleTime).
func (f Frame) SpringIn(o ui.SpringOptions, frames int) float64 {
	if frames <= 0 {
		return 1
	}
	settle := o.SettleTime()
	t := time.Duration(float64(settle) * f.Exact() / float64(frames))
	return o.Progress(t)
}

// Extrapolate says what Interpolate returns outside the range of its
// input.
type Extrapolate uint8

const (
	// Clamp keeps the value at the end of the output range.
	Clamp Extrapolate = iota
	// Extend goes on as the nearest segment goes.
	Extend
	// Identity returns the input.
	Identity
)

// Interpolation describes how Interpolate goes from value to value: along
// Easing, linear when nil, between each two, and Left and Right before
// and after the input's range, Clamp by default.
type Interpolation struct {
	Easing      ui.Easing
	Left, Right Extrapolate
}

// Interpolate maps x from the increasing values in to those of out, of
// the same length, two at least: between in[i] and in[i+1] it goes from
// out[i] to out[i+1] along o's Easing. A view fades in over its first
// half second with
//
//	opacity := f.Interpolate([]float64{0, 15}, []float64{0, 1})
func Interpolate(x float64, in, out []float64, o ...Interpolation) float64 {
	if len(in) < 2 || len(in) != len(out) {
		panic("render: Interpolate takes as many inputs as outputs, two at least")
	}
	for i := 1; i < len(in); i++ {
		if !(in[i] > in[i-1]) {
			panic("render: Interpolate's inputs must increase")
		}
	}
	var opt Interpolation
	if len(o) > 0 {
		opt = o[0]
	}
	if x < in[0] {
		switch opt.Left {
		case Clamp:
			return out[0]
		case Identity:
			return x
		}
	}
	last := len(in) - 1
	if x > in[last] {
		switch opt.Right {
		case Clamp:
			return out[last]
		case Identity:
			return x
		}
	}
	i := 0
	for i < last-1 && x > in[i+1] {
		i++
	}
	t := (x - in[i]) / (in[i+1] - in[i])
	if opt.Easing != nil && t >= 0 && t <= 1 {
		t = float64(opt.Easing(float32(t)))
	}
	return out[i] + t*(out[i+1]-out[i])
}

// InterpolateColor maps x from in to the colors out as Interpolate maps
// it to numbers, mixing them in sRGB, outside in's range clamped.
func InterpolateColor(x float64, in []float64, out []ui.Color, o ...Interpolation) ui.Color {
	if len(in) < 2 || len(in) != len(out) {
		panic("render: InterpolateColor takes as many inputs as outputs, two at least")
	}
	idx := make([]float64, len(in))
	for i := range idx {
		idx[i] = float64(i)
	}
	var opt Interpolation
	if len(o) > 0 {
		opt = o[0]
	}
	// Where x is, in segments: the easing applies within one.
	p := Interpolate(x, in, idx, Interpolation{Easing: opt.Easing})
	i := min(int(p), len(out)-2)
	return out[i].Mix(out[i+1], float32(p-float64(i)))
}
