//go:build linux && !android && (amd64 || arm64)

package gl

import (
	"fmt"
	"runtime"
	"sync"
	"testing"

	"github.com/egoist/mygo/internal/scene"
)

// TestNewOffscreenFailsWithItsError checks that a setup failing after EGL
// loaded returns its error, rather than panicking as it undoes what it
// did: a framebuffer of 0×0 is incomplete, and without libepoxy loading
// fails, both after the display and the context.
func TestNewOffscreenFailsWithItsError(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	o, err := NewOffscreen(false, 0, 0)
	if err == nil {
		o.Release()
		t.Skip("a framebuffer of 0×0 is complete here")
	}
	if o != nil {
		t.Errorf("NewOffscreen returned %v with the error %v", o, err)
	}
	t.Log(err)
}

// TestOffscreensAtOnce makes, draws with and releases offscreen contexts
// on several threads at once, as plugins/render's workers do: they share
// the process's one surfaceless EGL display, which one context's release
// must not terminate under the others (it crashed Mesa).
func TestOffscreensAtOnce(t *testing.T) {
	const w, h = 16, 16
	s := &scene.Scene{Width: w, Height: h, Clear: scene.Color{R: 255, A: 255}, Ops: []scene.Op{
		{Kind: scene.OpFill, Rect: scene.Rect{X: 4, Y: 4, W: 8, H: 8}, Color: scene.Color{B: 255, A: 255}},
	}}
	// check draws s with o and checks a pixel of each color.
	check := func(o *Offscreen) error {
		pix, err := o.Render(s)
		if err != nil {
			return err
		}
		at := func(x, y int) [4]byte { return [4]byte(pix[4*(y*w+x):]) }
		if got, want := at(0, 0), [4]byte{0, 0, 255, 255}; got != want {
			return fmt.Errorf("the background is %v, not %v", got, want)
		}
		if got, want := at(8, 8), [4]byte{255, 0, 0, 255}; got != want {
			return fmt.Errorf("the box is %v, not %v", got, want)
		}
		return nil
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// A context held throughout, which the others' releases leave working.
	held, err := NewOffscreen(false, w, h)
	if err != nil {
		t.Skip(err)
	}
	defer held.Release()
	if err := check(held); err != nil {
		t.Fatal(err)
	}
	const threads, rounds = 8, 10
	errs := make(chan error, threads*rounds)
	var wg sync.WaitGroup
	for k := range threads {
		wg.Go(func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			for i := range rounds {
				o, err := NewOffscreen(k%2 == 1, w, h)
				if err != nil {
					errs <- fmt.Errorf("thread %d, round %d: %w", k, i, err)
					continue
				}
				// Every other one is released at once, as a render's
				// workers release a context drawing with llvmpipe.
				if (k+i)%2 == 0 {
					if err := check(o); err != nil {
						errs <- fmt.Errorf("thread %d, round %d: %w", k, i, err)
					}
				}
				o.Release()
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if err := check(held); err != nil {
		t.Errorf("the context held throughout: %v", err)
	}
}
