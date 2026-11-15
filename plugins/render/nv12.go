package render

import (
	"runtime"
	"sync"
)

// bgraToNV12 converts premultiplied BGRA pixels, w×h with even sizes, to
// NV12 of BT.709 in the limited range, as H.264 and HEVC carry them: a
// plane of luma, then one of the chroma of each 2×2 block, interleaved
// Cb, Cr. What alpha leaves out is black, as premultiplied values are.
// The rows are converted on all cores.
func bgraToNV12(dst, src []byte, w, h int) {
	workers := min(max(runtime.GOMAXPROCS(0), 1), h/2)
	var wg sync.WaitGroup
	pairs := h / 2
	for k := range workers {
		from, to := pairs*k/workers, pairs*(k+1)/workers
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := from; p < to; p++ {
				nv12Rows(dst, src, w, h, p)
			}
		}()
	}
	wg.Wait()
}

// The BT.709 coefficients in the limited range, scaled by 2^16, for
// values of 0 to 255: Y = 16 + 219/255 (Kr R + Kg G + Kb B), Cb and Cr
// 128 + 224/255 of the color differences (B - Y) / 1.8556 and
// (R - Y) / 1.5748.
const (
	yR, yG, yB    = 11966, 40254, 4064
	cbR, cbG, cbB = -6596, -22189, 28784
	crR, crG, crB = 28784, -26145, -2639
)

// nv12Rows converts the pair of rows p.
func nv12Rows(dst, src []byte, w, h, p int) {
	stride := 4 * w
	y0 := 2 * p
	r0, r1 := src[y0*stride:(y0+1)*stride], src[(y0+1)*stride:(y0+2)*stride]
	l0, l1 := dst[y0*w:(y0+1)*w], dst[(y0+1)*w:(y0+2)*w]
	c := dst[w*h+p*w : w*h+(p+1)*w]
	luma := func(px []byte) byte {
		b, g, r := int32(px[0]), int32(px[1]), int32(px[2])
		return byte((16<<16 + yR*r + yG*g + yB*b + 1<<15) >> 16)
	}
	for x := 0; x < w; x += 2 {
		i := 4 * x
		a, b, d, e := r0[i:i+4], r0[i+4:i+8], r1[i:i+4], r1[i+4:i+8]
		l0[x], l0[x+1], l1[x], l1[x+1] = luma(a), luma(b), luma(d), luma(e)
		// The block's mean color, of four: its chroma.
		sb := int32(a[0]) + int32(b[0]) + int32(d[0]) + int32(e[0])
		sg := int32(a[1]) + int32(b[1]) + int32(d[1]) + int32(e[1])
		sr := int32(a[2]) + int32(b[2]) + int32(d[2]) + int32(e[2])
		c[x] = byte(clamp8((128<<18 + cbR*sr + cbG*sg + cbB*sb + 1<<17) >> 18))
		c[x+1] = byte(clamp8((128<<18 + crR*sr + crG*sg + crB*sb + 1<<17) >> 18))
	}
}

func clamp8(v int32) int32 { return min(max(v, 0), 255) }
