package render

import (
	"runtime"
	"sync"
)

// accumulator sums the pixels of a frame's motion blur samples, on all
// cores, and returns their mean.
type accumulator struct {
	sum []uint16
	out []byte
}

// parts runs fn on n bytes in a band per core, of whole pixels.
func parts(n int, fn func(from, to int)) {
	workers := max(1, min(runtime.GOMAXPROCS(0), n/(1<<16)))
	var wg sync.WaitGroup
	pixels := n / 4
	for k := range workers {
		from, to := 4*(pixels*k/workers), 4*(pixels*(k+1)/workers)
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn(from, to)
		}()
	}
	wg.Wait()
}

// add adds a sample's pixels; first starts a new sum.
func (a *accumulator) add(pix []byte, first bool) {
	if len(a.sum) != len(pix) {
		a.sum = make([]uint16, len(pix))
		first = true
	}
	parts(len(pix), func(from, to int) {
		sum, src := a.sum[from:to], pix[from:to]
		if first {
			for i, v := range src {
				sum[i] = uint16(v)
			}
			return
		}
		for i, v := range src {
			sum[i] += uint16(v)
		}
	})
}

// mean returns the mean of the n samples added, rounded: premultiplied
// pixels average as the light of the shutter's time does.
func (a *accumulator) mean(n int) []byte {
	if len(a.out) != len(a.sum) {
		a.out = make([]byte, len(a.sum))
	}
	half := uint32(n / 2)
	parts(len(a.sum), func(from, to int) {
		for i, v := range a.sum[from:to] {
			a.out[from+i] = byte((uint32(v) + half) / uint32(n))
		}
	})
	return a.out
}
