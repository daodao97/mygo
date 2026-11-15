package render

import (
	"image"
	"io"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// NewPlayer returns a player of the composition, for ui.Video to show in
// a window, as a preview of the video while it is made: its frames are
// the video's, built at their times and drawn on the CPU on a goroutine
// of the player's, a few ahead as they come due, and a seek builds the
// frames before the one it goes to without drawing them. Play, Pause,
// Seek and SetLoop control it as any video's; it plays no sound.
//
//	player, err := render.NewPlayer(comp)
//	...
//	ui.Video(c, player).FillWidth()
func NewPlayer(comp Composition) (*ui.VideoPlayer, error) {
	if err := comp.validate(); err != nil {
		return nil, err
	}
	d := &compDecoder{comp: comp}
	d.reset()
	return ui.NewVideoPlayer(d)
}

// compDecoder decodes a composition's frames, for a ui.VideoPlayer.
type compDecoder struct {
	comp Composition
	s    *session
	r    cpuDrawer
	img  *image.RGBA
	// next is the frame Next draws, which the session builds next.
	next int
}

// reset starts again from the first frame, with a session of its own:
// what ui animates goes on from the frames built before.
func (d *compDecoder) reset() {
	if d.s != nil {
		d.s.rec.Close()
	}
	d.s, d.next = newSession(d.comp), 0
}

func (d *compDecoder) Next() (ui.VideoFrame, error) {
	if d.next >= d.comp.Frames {
		return ui.VideoFrame{}, io.EOF
	}
	var pix []byte
	if n, _ := d.comp.MotionBlur.shutter(); n > 1 {
		var err error
		if pix, err = d.s.blurred(d.next, &d.r); err != nil {
			return ui.VideoFrame{}, err
		}
	} else {
		d.r.submit(d.s.build(d.next))
		pix, _ = d.r.pixels()
	}
	if d.img == nil {
		d.img = image.NewRGBA(image.Rect(0, 0, d.comp.Width, d.comp.Height))
	}
	bgraToRGBA(d.img.Pix, pix)
	dur := d.s.at(d.next+1) - d.s.at(d.next)
	d.next++
	return ui.VideoFrame{Image: d.img, Duration: dur}, nil
}

func (d *compDecoder) Rewind() error {
	d.reset()
	return nil
}

// Seek goes to the frame showing at t, building those before it.
func (d *compDecoder) Seek(t time.Duration) (time.Duration, error) {
	target := min(int(int64(t)*d.s.num/(d.s.den*int64(time.Second))), d.comp.Frames-1)
	target = max(target, 0)
	// Frames start at times rounded to the nanosecond: one starting at t
	// is the frame at t.
	if target+1 < d.comp.Frames && d.s.at(target+1) <= t {
		target++
	}
	if target < d.next {
		d.reset()
	}
	for ; d.next < target; d.next++ {
		d.s.build(d.next)
	}
	return d.s.at(target), nil
}

// Duration returns the length of the composition.
func (d *compDecoder) Duration() time.Duration {
	num, den := d.comp.rate()
	return time.Duration(math.Round(float64(d.comp.Frames) * float64(den) * float64(time.Second) / float64(num)))
}

// Close drops the session and the frames' memory, as the player closes.
func (d *compDecoder) Close() error {
	if d.s != nil {
		d.s.rec.Close()
		d.s = nil
	}
	d.r.release()
	return nil
}
