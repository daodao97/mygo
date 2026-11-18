package render

import (
	"context"
	"runtime"
	"sync"
	"time"
)

// A render on several cores (Options.Workers) splits the frames among
// workers in turn: worker k of n draws frames from+k, from+k+n, and so
// on, while the goroutine of the render hands them to the sink in order.
// Each worker has a session and a drawer of its own, on a thread of its
// own (OpenGL's context is current on it), and builds every frame before
// its last, drawing only its own: what ui animates from frame to frame
// is then where a render on one thread has it, and the frames are the
// same, while no scene, which the next frame builds into, crosses
// threads. Building is a fraction of drawing a frame, reading its pixels
// back and encoding them, which the workers do at once.
//
// The frames in turn keep what waits for the sink small: a worker draws
// its next frame once the sink took its last, whose pixels are its
// drawer's (or its motion blur's) until then, while it builds the frames
// between.

// maxAutoWorkers bounds the workers Options.Workers zero chooses: each
// holds a frame's pixels, a drawer, and ui's text system of its own, and
// the GPU and the encoder they share draw and encode one frame at a time.
const maxAutoWorkers = 4

// workers returns how many workers draw frames frames of comp.
func (o Options) workers(comp Composition, frames int) int {
	n := o.Workers
	if n <= 0 {
		n = min(runtime.GOMAXPROCS(0)/2, maxAutoWorkers)
	}
	return max(min(n, frames), 1)
}

// A worker draws its frames of a render on several cores.
type worker struct {
	// done takes each frame the worker drew, and free the turn to draw
	// the next: the sink took the last one.
	done chan drawn
	free chan struct{}
}

// drawn is a frame a worker drew: its pixels, valid until it is given
// its next turn, or held, for a frame that shows what the frame before
// showed, which the sink holds (holder); and the name of what drew it; or
// the error that stopped the worker.
type drawn struct {
	pix  []byte
	held bool
	name string
	err  error
}

// runWorkers renders frames from to before to of comp into the sink open
// returns, with n workers.
func runWorkers(ctx context.Context, comp Composition, o Options, from, to, n int, open func(*session, drawer) (sink, error)) (err error) {
	// The sink draws no scene itself: the frames come from many drawers.
	out, err := open(nil, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			out.abort()
		}
	}()
	hold := o.holder(out)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	ws := make([]*worker, n)
	for k := range ws {
		w := &worker{done: make(chan drawn, 1), free: make(chan struct{}, 1)}
		w.free <- struct{}{}
		ws[k] = w
		wg.Go(func() { w.run(comp, o.Renderer, from, from+k, to, n, hold != nil, stop) })
	}
	// The workers stop and release their drawers before the sink is
	// given up.
	defer func() {
		close(stop)
		wg.Wait()
	}()
	start := time.Now()
	for i := from; i < to; i++ {
		w := ws[(i-from)%n]
		var f drawn
		select {
		case f = <-w.done:
		case <-ctx.Done():
			return ctx.Err()
		}
		if f.err != nil {
			return f.err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if f.held {
			err = hold.hold(i)
		} else {
			err = out.frame(i, f.pix)
		}
		if err != nil {
			return err
		}
		w.free <- struct{}{}
		if o.Progress != nil {
			o.Progress(Progress{Frame: i, Done: i - from + 1, Frames: to - from, Elapsed: time.Since(start), Renderer: f.name})
		}
	}
	return out.close()
}

// run draws frames first, first+step, ... before to, and builds the
// others before its last as a render on one thread does: those before
// from at their times, those from from at the times of their motion
// blur's samples too. A frame that shows what the frame before showed is
// held, and not drawn, when holds: the sink holds it (holder).
func (w *worker) run(comp Composition, r Renderer, from, first, to, step int, holds bool, stop <-chan struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	send := func(f drawn) bool {
		select {
		case w.done <- f:
			return true
		case <-stop:
			return false
		}
	}
	s := newSession(comp)
	s.track = true
	defer s.rec.Close()
	raw, err := newDrawer(r, comp.Width, comp.Height)
	if err != nil {
		send(drawn{err: err})
		return
	}
	defer raw.release()
	d := &kept{drawer: raw}
	samples, shutter := comp.MotionBlur.shutter()
	last := first + (to-1-first)/step*step
	for i := 0; i <= last; i++ {
		select {
		case <-stop:
			return
		default:
		}
		if i < first || (i-first)%step != 0 {
			if i < from || samples <= 1 {
				s.build(i)
				continue
			}
			for k := samples - 1; k >= 0; k-- {
				s.buildAt(i, -shutter*float64(k)/float64(samples))
			}
			continue
		}
		// The sink took the frame before, whose pixels this one's drawing
		// replaces.
		select {
		case <-w.free:
		case <-stop:
			return
		}
		var pix []byte
		held := false
		if samples > 1 {
			pix, err = s.blurred(i, d)
			held = holds && i > from && s.held(i)
		} else {
			sc := s.build(i)
			// A frame showing what the drawer holds is not drawn again.
			if held = holds && i > from && s.held(i); !held {
				if err = s.drawn(d, sc); err == nil {
					pix, err = d.pixels()
				}
			}
		}
		if !send(drawn{pix: pix, held: held, name: d.name(), err: err}) || err != nil {
			return
		}
	}
	// The drawer's pixels, as the CPU renderer's mapped memory, are the
	// sink's until it took the last frame.
	select {
	case <-w.free:
	case <-stop:
	}
}
