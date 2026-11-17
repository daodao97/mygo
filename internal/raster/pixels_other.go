//go:build !(darwin && !ios) && !(linux && !android) && !windows

package raster

func allocPixels(n int) *pixels { return &pixels{b: make([]byte, n)} }

func (p *pixels) free() {
	if p != nil {
		p.b = nil
	}
}
