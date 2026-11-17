package main

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"

	"golang.org/x/image/vector"
)

// Android draws tabs' icons from PNGs (iOS draws SF Symbols): these are
// drawn as templates, their alpha in the bar's colors, 72 pixels square,
// sharp at every phone's scale, on a grid of 24 like Material's icons.

const iconSize = 72

func iconPNG(draw func(r *vector.Rasterizer, u float32)) []byte {
	r := vector.NewRasterizer(iconSize, iconSize)
	draw(r, iconSize/24.0)
	img := image.NewAlpha(image.Rect(0, 0, iconSize, iconSize))
	r.Draw(img, img.Bounds(), image.NewUniform(color.Alpha{A: 255}), image.Point{})
	var b bytes.Buffer
	png.Encode(&b, toNRGBA(img))
	return b.Bytes()
}

func toNRGBA(a *image.Alpha) *image.NRGBA {
	out := image.NewNRGBA(a.Bounds())
	draw.Draw(out, out.Bounds(), &image.Uniform{color.Black}, image.Point{}, draw.Src)
	for i := range a.Pix {
		out.Pix[4*i+3] = a.Pix[i]
	}
	return out
}

// polygon fills the points, in units of the grid.
func polygon(r *vector.Rasterizer, u float32, pts ...float32) {
	r.MoveTo(pts[0]*u, pts[1]*u)
	for i := 2; i+1 < len(pts); i += 2 {
		r.LineTo(pts[i]*u, pts[i+1]*u)
	}
	r.ClosePath()
}

// ring fills the ring between two circles about (cx, cy).
func ring(r *vector.Rasterizer, u, cx, cy, outer, inner float32) {
	circle(r, u, cx, cy, outer, false)
	if inner > 0 {
		circle(r, u, cx, cy, inner, true)
	}
}

func circle(r *vector.Rasterizer, u, cx, cy, rad float32, reverse bool) {
	const n = 48
	for i := 0; i <= n; i++ {
		a := 2 * math.Pi * float64(i) / n
		if reverse {
			a = -a
		}
		x, y := (cx+rad*float32(math.Cos(a)))*u, (cy+rad*float32(math.Sin(a)))*u
		if i == 0 {
			r.MoveTo(x, y)
		} else {
			r.LineTo(x, y)
		}
	}
	r.ClosePath()
}

var (
	homeIcon = iconPNG(func(r *vector.Rasterizer, u float32) {
		polygon(r, u, 12, 3, 21.5, 11, 19, 11, 19, 21, 14.5, 21, 14.5, 15, 9.5, 15, 9.5, 21, 5, 21, 5, 11, 2.5, 11)
	})
	globeIcon = iconPNG(func(r *vector.Rasterizer, u float32) {
		ring(r, u, 12, 12, 9.5, 7.8)
		// The meridian and the equator.
		polygon(r, u, 11.15, 2.5, 12.85, 2.5, 12.85, 21.5, 11.15, 21.5)
		polygon(r, u, 2.5, 11.15, 21.5, 11.15, 21.5, 12.85, 2.5, 12.85)
	})
	gearIcon = iconPNG(func(r *vector.Rasterizer, u float32) {
		for k := 0; k < 8; k++ {
			a := float64(k) * math.Pi / 4
			c, s := float32(math.Cos(a)), float32(math.Sin(a))
			// A tooth: a box along the ray at angle a.
			pt := func(along, across float32) (float32, float32) {
				return 12 + along*c - across*s, 12 + along*s + across*c
			}
			x0, y0 := pt(6, -1.6)
			x1, y1 := pt(10, -1.6)
			x2, y2 := pt(10, 1.6)
			x3, y3 := pt(6, 1.6)
			polygon(r, u, x0, y0, x1, y1, x2, y2, x3, y3)
		}
		ring(r, u, 12, 12, 7.5, 3.2)
	})
)
