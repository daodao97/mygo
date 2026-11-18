//go:build windows

package d3d11

import (
	"bytes"
	"cmp"
	"fmt"
	"runtime"
	"sync"
	"testing"

	"github.com/egoist/mygo/internal/gpu/gputest"
)

// TestFramesOnThreads draws the frames of videos with Frames on several
// threads at once, as plugins/render's workers do, and requires the bytes
// one Frames draws alone: each has a device of its own, whose immediate
// context no other thread draws with.
func TestFramesOnThreads(t *testing.T) {
	const threads, steps = 4, 12
	draw := func(seed uint64, each func(step int, pix []byte)) error {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		m := gputest.NewChanging(seed)
		s := m.Scene()
		f, err := NewFrames(s.Width, s.Height)
		if err != nil {
			return err
		}
		defer f.Release()
		pix := make([]byte, 4*s.Width*s.Height)
		for step := range steps {
			if step > 0 {
				m.Change(s)
			}
			if err := f.Draw(s); err != nil {
				return err
			}
			if err := f.Read(pix, s.Width, s.Height); err != nil {
				return err
			}
			each(step, pix)
		}
		return nil
	}
	want := make([][][]byte, threads)
	for k := range threads {
		if err := draw(uint64(k), func(_ int, pix []byte) { want[k] = append(want[k], bytes.Clone(pix)) }); err != nil {
			t.Skip("no Direct3D 11:", err)
		}
	}
	var wg sync.WaitGroup
	errs := make([]error, threads)
	for k := range threads {
		wg.Go(func() {
			var differs error
			err := draw(uint64(k), func(step int, pix []byte) {
				if differs == nil && !bytes.Equal(pix, want[k][step]) {
					differs = fmt.Errorf("thread %d, step %d: %d bytes differ from the frame drawn alone", k, step, differing(pix, want[k][step]))
				}
			})
			errs[k] = cmp.Or(err, differs)
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}
