package raster

import (
	"github.com/egoist/mygo/internal/scene"
	"math"
)

// Contains tests the interior of a rounded clip, using the same continuous
// corner geometry as rendering. Hit testing uses the shape's edge, without
// the pixel coverage used to smooth that edge on screen.
func Contains(r scene.Rect, radii [4]float32, continuous bool, x, y float32) bool {
	if !r.Contains(x, y) {
		return false
	}
	s := newShape(r, scene.Corners(r, radii, continuous))
	if s.continuous {
		d, _ := s.contDist(x, y)
		return d <= 0
	}
	return scene.SDRoundRect(r, s.radii, x, y) <= 0
}

// transformedCoverage integrates a shape that m transforms (inv its
// inverse) over a device pixel. Interior pixels take one distance test;
// edges use a grid in device space. What a projective map puts behind the
// viewer covers nothing.
func transformedCoverage(s *shape, m, inv scene.Affine, x, y float32) float32 {
	if !inv.Set {
		return coverage(s, x, y)
	}
	lx, ly := inv.Point(x, y)
	if m.Perspective() && m.W(lx, ly) <= 0 {
		return 0
	}
	d := scene.SDRoundRect(s.r, s.radii, lx, ly)
	if s.continuous {
		d, _ = s.contDist(lx, ly)
	}
	rx, ry := inv.VectorAt(x, y, 0.5, 0.5)
	sx, sy := inv.VectorAt(x, y, 0.5, -0.5)
	reach := max(abs(rx)+abs(ry), abs(sx)+abs(sy)) * 2
	if d < -reach {
		return 1
	}
	if d > reach {
		return 0
	}
	const samples = 4
	n := 0
	for j := range samples {
		for i := range samples {
			px, py := inv.Point(x+(float32(i)+0.5)/samples-0.5, y+(float32(j)+0.5)/samples-0.5)
			v := scene.SDRoundRect(s.r, s.radii, px, py)
			if s.continuous {
				v, _ = s.contDist(px, py)
			}
			if v <= 0 {
				n++
			}
		}
	}
	return float32(n) / (samples * samples)
}

// generalTransforms turns off what transformedCover and shadowRows work
// out once, for tests that check the pixels they draw are those of the
// general per-pixel path.
var generalTransforms bool

// transformedCover is transformedCoverage of one shape under one map, with
// what its pixels share worked out once. Under an affine map a pixel
// reaches as far into the shape's coordinates wherever it is, so the
// pixels whose centers fall in inner, further inside the shape than that
// reach and past its corners' curves, are covered whole without a distance
// test: a shape scaled or moved, as a transition leaves it, draws its
// interior at the cost of mapping a point.
type transformedCover struct {
	s      *shape
	m, inv scene.Affine
	inner  scene.Rect
}

func newTransformedCover(s *shape, m, inv scene.Affine) transformedCover {
	c := transformedCover{s: s, m: m, inv: inv}
	if generalTransforms || !inv.Set || m.Perspective() || inv.Perspective() {
		return c
	}
	rx, ry := inv.VectorAt(0, 0, 0.5, 0.5)
	sx, sy := inv.VectorAt(0, 0, 0.5, -0.5)
	reach := max(abs(rx)+abs(ry), abs(sx)+abs(sy)) * 2
	var corner float32
	for i, rad := range s.radii {
		if s.continuous {
			rad = s.corners[i].e // where its curve starts
		}
		corner = max(corner, rad)
	}
	// A whole unit more than the reach, so that float32's rounding of the
	// distance cannot bring a pixel of inner within it.
	in := corner + reach + 1
	inner := scene.Rect{X: s.r.X + in, Y: s.r.Y + in, W: s.r.W - 2*in, H: s.r.H - 2*in}
	if inner.W > 0 && inner.H > 0 && !math.IsInf(float64(in), 0) {
		c.inner = inner
	}
	return c
}

// at returns transformedCoverage(c.s, c.m, c.inv, x, y), given lx and ly,
// what c.inv maps (x, y) to.
func (c *transformedCover) at(x, y, lx, ly float32) float32 {
	if lx > c.inner.X && lx < c.inner.X+c.inner.W && ly > c.inner.Y && ly < c.inner.Y+c.inner.H {
		return 1
	}
	return transformedCoverage(c.s, c.m, c.inv, x, y)
}

// transformed inverse maps pixels into the operation's coordinates.
// Identity operations retain the existing span renderer.
func (r *renderer) transformed(op *scene.Op, px scene.EffectPixels, b *scene.BackdropImage) {
	inv, ok := op.Transform.Inverse()
	inset := op.Kind == scene.OpShadow && op.Inset
	if !ok || !inset && op.Rect.Empty() || inset && op.Cast.Empty() {
		return
	}
	// An image whose owner dropped its pixels (Bitmap.ReleasePixels) has
	// them made again here, once for the whole operation, as the op reads
	// them directly; without them it draws nothing.
	var image *scene.Image
	if op.Kind == scene.OpImage {
		if op.Image == nil || op.Image.W == 0 || op.Image.H == 0 || unflipped(op.Src).Empty() || op.Image.Pixels() == nil {
			return
		}
		image = op.Image
	}
	radii := scene.Corners(op.Rect, op.Radii, op.Continuous)
	outer := newShape(op.Rect, radii)
	inner := outer
	bw := op.Border
	if op.BorderColor.A == 0 {
		bw = [4]float32{}
	}
	hasBorder := scene.HasBorder(bw)
	if hasBorder {
		inner = newShape(scene.InnerRadii(op.Rect, radii, bw))
	}
	outerCover := newTransformedCover(&outer, op.Transform, inv)
	innerCover := newTransformedCover(&inner, op.Transform, inv)
	var caster shape
	var casterCover transformedCover
	if op.Kind == scene.OpShadow && !op.Cast.Empty() {
		caster = newShape(op.Cast, scene.Corners(op.Cast, op.CastRadii, op.Continuous))
		casterCover = newTransformedCover(&caster, op.Transform, inv)
	}
	var rows shadowRows
	if op.Kind == scene.OpShadow {
		rows = newShadowRows(op, &outer)
	}
	// How far a pixel of the frame reaches in the image's, as the GPU's
	// fwidth: the same for every pixel under an affine map.
	affine := !op.Transform.Perspective()
	var sx, sy, du, dv float32
	if image != nil {
		sx, sy = op.Src.W/op.Rect.W, op.Src.H/op.Rect.H
		if affine {
			ax, ay := inv.VectorAt(0, 0, 1, 0)
			bx, by := inv.VectorAt(0, 0, 0, 1)
			du, dv = (abs32(ax)+abs32(bx))*abs32(sx), (abs32(ay)+abs32(by))*abs32(sy)
		}
	}
	pt, bp := borderPainter(op, opacityOf(op.Opacity), r.s.Stops)
	border := op.BorderColor.Premul(opacityOf(op.Opacity))
	box := op.Rect
	if inset {
		box = op.Cast
	} else if op.Kind == scene.OpShadow {
		e := 1.5*op.Blur + 1
		box = scene.Rect{X: box.X - e, Y: box.Y - e, W: box.W + 2*e, H: box.H + 2*e}
	}
	box = op.Transform.Bounds(box)
	x0, y0, x1, y1 := r.pixelBounds(box)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			clip := r.clipCoverage(x, y)
			if clip <= 0 {
				continue
			}
			wx, wy := float32(x)+0.5, float32(y)+0.5
			lx, ly := inv.Point(wx, wy)
			if op.Transform.Perspective() && op.Transform.W(lx, ly) <= 0 {
				continue // behind the viewer
			}
			p := r.dst.Pix[y*r.dst.Stride+4*x:][:4]
			if inset {
				// Inside the box, what the hole, blurred, leaves.
				cov := casterCover.at(wx, wy, lx, ly)
				if cov <= 0 {
					continue
				}
				if !op.Rect.Empty() {
					var hole float32
					if op.Blur < 1 {
						hole = outerCover.at(wx, wy, lx, ly)
					} else {
						hole = rows.at(lx, ly)
					}
					cov *= 1 - hole
				}
				blend(p, op.Color.Premul(opacityOf(op.Opacity)), cov*clip)
				continue
			}
			if op.Kind == scene.OpShadow {
				// Outside the box casting it: what that box hides whole
				// takes none of it.
				hidden := float32(0)
				if !op.Cast.Empty() {
					if hidden = casterCover.at(wx, wy, lx, ly); hidden >= 1 {
						continue
					}
				}
				var cov float32
				if op.Blur < 1 {
					cov = outerCover.at(wx, wy, lx, ly)
				} else {
					cov = rows.at(lx, ly)
				}
				if !op.Cast.Empty() {
					cov *= 1 - hidden
				}
				blend(p, op.Color.Premul(opacityOf(op.Opacity)), cov*clip)
				continue
			}
			cov := outerCover.at(wx, wy, lx, ly)
			if cov <= 0 {
				continue
			}
			switch op.Kind {
			case scene.OpFill:
				var fc [4]float32
				if pt.visible() {
					fc = pt.at(lx, ly)
				}
				if !hasBorder {
					blend(p, fc, cov*clip)
					continue
				}
				ic := float32(0)
				if !inner.r.Empty() {
					ic = innerCover.at(wx, wy, lx, ly)
				}
				bc := max(cov-ic, 0)
				if op.Dashed {
					bc *= dash(op.Rect, bw, lx, ly)
				}
				blendFilled(p, fc, cov, borderAt(bp, border, op.BorderColor, lx, ly), bc, clip)
			case scene.OpImage:
				if image == nil {
					continue
				}
				du, dv := du, dv
				if !affine {
					ax, ay := inv.VectorAt(wx, wy, 1, 0)
					bx, by := inv.VectorAt(wx, wy, 0, 1)
					du, dv = (abs32(ax)+abs32(bx))*abs32(sx), (abs32(ay)+abs32(by))*abs32(sy)
				}
				c := sampleOp(op, op.Src.X+(lx-op.Rect.X)*sx, op.Src.Y+(ly-op.Rect.Y)*sy, du, dv)
				blend(p, c, cov*clip*opacityOf(op.Opacity))
			case scene.OpEffect:
				if px != nil {
					blend(p, px.Color(lx, ly, b), cov*clip*opacityOf(op.Opacity))
				}
			case scene.OpHole:
				erase(p, cov*clip*opacityOf(op.Opacity))
			}
		}
	}
}

func opacityOf(v float32) float32 {
	if v == 0 {
		return 1
	}
	return v
}

// shadowRows is how much of a point of its own coordinates a shadow
// covers, as the renderer's shadow works it out in the frame's: the box blurred along y at four samples, of
// the box blurred exactly along x, narrowed by its corners. What a row of
// those coordinates shares, the samples' weights and widths, it works out
// once for the row last asked, which a map without rotation keeps from
// pixel to pixel, and two samples as wide share their blur along x.
type shadowRows struct {
	op               *scene.Op
	box              *shape
	sigma, hx, hy, k float32
	corner           float32
	cc               contCorner
	row              uint32 // the bits of the row worked out, when have
	have, general    bool
	weight, half     [4]float32
}

func newShadowRows(op *scene.Op, box *shape) shadowRows {
	sigma := op.Blur / 2
	corner := max(box.radii[0], box.radii[1], box.radii[2], box.radii[3])
	return shadowRows{
		op: op, box: box, sigma: sigma,
		hx: op.Rect.W / 2, hy: op.Rect.H / 2,
		k:       float32(math.Sqrt(0.5)) / sigma,
		corner:  corner,
		cc:      newContCorner(corner, corner, corner, op.Rect.W, op.Rect.H),
		general: generalTransforms,
	}
}

// at returns the shadow's coverage of the point (x, y) of its coordinates.
func (s *shadowRows) at(x, y float32) float32 {
	if s.sigma < 0.5 {
		return coverage(s.box, x, y)
	}
	if bits := math.Float32bits(y); !s.have || bits != s.row || s.general {
		s.setRow(y)
		s.row, s.have = bits, true
	}
	x = x - s.op.Rect.X - s.hx
	var v, e float32
	for i := range 4 {
		if i == 0 || s.half[i] != s.half[i-1] {
			e = erf((x+s.half[i])*s.k) - erf((x-s.half[i])*s.k)
		}
		v += s.weight[i] * e
	}
	return v
}

// setRow works out the samples of row y.
func (s *shadowRows) setRow(y float32) {
	sigma, hx, hy, corner := s.sigma, s.hx, s.hy, s.corner
	y = y - s.op.Rect.Y - hy
	ext := 3 * sigma
	start, end := min(max(-ext, y-hy), y+hy), min(max(ext, y-hy), y+hy)
	step := (end - start) / 4
	for i := range 4 {
		yy := start + (float32(i)+0.5)*step
		half := hx
		if s.box.continuous {
			at, _ := s.cc.inset(hy - abs(y-yy))
			half -= at
		} else if delta := min(hy-corner-abs(y-yy), 0); delta < 0 {
			half = hx - corner + float32(math.Sqrt(float64(max(0, corner*corner-delta*delta))))
		}
		s.half[i] = half
		s.weight[i] = gaussian(yy, sigma) * step * 0.5
	}
}

func (r *renderer) transformedGlyphs(op *scene.Op) {
	var pt *painter
	if op.Paint == scene.PaintLinear || op.Paint == scene.PaintOklab || op.Paint.HasStops() {
		p := newPainter(op, opacityOf(op.Opacity), r.s.Stops)
		pt = &p
	}
	for _, g := range r.s.Glyphs[op.Start:op.End] {
		m := op.Transform.Mul(g.Transform)
		inv, ok := m.Inverse()
		if !ok {
			continue
		}
		atlas := r.s.MaskAtlas
		if g.Colored || g.Subpixel {
			atlas = r.s.ColorAtlas
		}
		if atlas == nil {
			continue
		}
		box := scene.Rect{X: float32(math.Round(float64(g.X))), Y: float32(math.Round(float64(g.Y))), W: float32(g.UW), H: float32(g.VH)}
		shape := newShape(box, [4]float32{})
		x0, y0, x1, y1 := r.pixelBounds(m.Bounds(box))
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				wx, wy := float32(x)+0.5, float32(y)+0.5
				cov := r.clipCoverage(x, y) * transformedCoverage(&shape, m, inv, wx, wy)
				if cov <= 0 {
					continue
				}
				lx, ly := inv.Point(wx, wy)
				a := atlasSample(atlas, float32(g.U)+lx-box.X, float32(g.V)+ly-box.Y, int(g.U), int(g.V), int(g.UW), int(g.VH))
				tint := g.Color.Premul(1)
				if pt != nil {
					tint = pt.at(lx, ly)
				}
				p := r.dst.Pix[y*r.dst.Stride+4*x:][:4]
				if g.Colored {
					for i := range a {
						a[i] *= float32(g.Color.A) / 255
					}
					blend(p, a, cov)
					continue
				}
				contrast, boost := r.s.Text.Contrast, float32(0)
				if g.Thin {
					boost = scene.ThinBoost
				}
				if g.Subpixel {
					a[0] = (a[0] + a[1] + a[2]) / 3
					contrast = r.s.Text.SubpixelContrast
				}
				coverage := scene.TextCoverage(a[0], unpremul(tint), contrast, boost, r.s.Text.GammaRatios)
				blend(p, tint, cov*coverage)
			}
		}
	}
}

func atlasSample(a *scene.Atlas, u, v float32, ax, ay, w, h int) [4]float32 {
	u, v = u-0.5, v-0.5
	x, y := int(math.Floor(float64(u))), int(math.Floor(float64(v)))
	tx, ty := u-float32(x), v-float32(y)
	var c [4]float32
	for j := range 2 {
		for i := range 2 {
			sx, sy := min(max(x+i, ax), ax+w-1), min(max(y+j, ay), ay+h-1)
			weight := (1 - tx) * (1 - ty)
			if i == 1 {
				weight = tx * (1 - ty)
			}
			if j == 1 {
				weight = (1 - tx) * ty
				if i == 1 {
					weight = tx * ty
				}
			}
			for k := range a.BPP {
				c[k] += float32(a.Pix[(sy*a.W+sx)*a.BPP+k]) / 255 * weight
			}
		}
	}
	return c
}
