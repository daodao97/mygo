package gpu

import (
	"math"

	"github.com/egoist/mygo/internal/scene"
)

// Continuous corners (negative radii) cost Metal's shader far more than
// circular ones: a fill's coverage near them is their value and its two
// derivatives, and a blurred shadow finds, for each of its four rows of
// samples, where the curve is by regula falsi. A batch holding any of them
// draws with a pipeline holding that code, which every fragment of the batch
// pays for, curved or not. Yet the curves shape only the pixels near the
// corners: elsewhere the shader computes exactly what it computes for a
// rectangle with square corners, by the same code (see cornerLines). So a
// large shape with continuous corners is drawn as a nine-patch, as
// compositors draw rounded rectangles: its corners as the shape, each
// piece with a scissor rectangle of its own, and the rest, a cross, as its
// twin with square corners, in batches of a pipeline without the curves.
// The pixels are identical.

// contExtent is how far from its corner the curve of a continuous corner of
// radius 1 leaves the edges (the shaders' contExtent).
const contExtent = 1.528665

// cornerSplitMin is the area, in pixels, of a shape's twin below which the
// shape is not drawn in pieces: the draw calls of the pieces cost more than
// what shading those fragments with the curves' code costs. A shadow's
// fragments cost far more than a fill's (its curves are found for each of
// four rows of samples), so its twin pays at a quarter of the area.
const (
	cornerSplitMin       = 16384
	shadowCornerSplitMin = 4096
)

// SplitCorners makes the builder draw large shapes with continuous corners
// as nine-patches (see addCorners), for a renderer whose shader draws them
// (Metal's). Only through it do apps link that code, which the other
// renderers' apps never run.
func (b *Builder) SplitCorners() { b.cutCorners = (*Builder).addCorners }

// addCut appends in, a fill, a shadow or an image, within the current
// clip's or split's scissor rectangle: as a nine-patch where the builder
// splits corners (addCorners), else whole.
func (b *Builder) addCut(in Instance, image uintptr, m scene.Affine) {
	if b.cutCorners != nil && !b.NoCornerSplit {
		b.cutCorners(b, in, image, m)
		return
	}
	b.add(in, image, m)
}

// cornerLines returns where the pixels lie that the continuous corners
// of radii (positive) of the rectangle r cannot reach: the columns from
// Left to Right and the rows from Top to Bottom, each as far in, from the
// sides, as the curves reach (contExtent times the radius) and, by reachX
// and reachY, beyond. A pixel in neither is in one of the corners' boxes,
// which reach out of the rectangle to the frame's edges. The shaders compute
// a corner's curve only at a pixel within its box (contValue tests u and v
// against e), and anywhere else the area of the pixel in the rectangle, as
// for square corners.
func cornerLines(r scene.Rect, radii [4]float32, reachX, reachY float32) Scissor {
	e := func(i, j int) float32 { return contExtent*max(abs32(radii[i]), abs32(radii[j])) + 1 }
	return Scissor{
		Left:   int32(math.Ceil(float64(r.X + e(0, 3) + reachX))),
		Top:    int32(math.Ceil(float64(r.Y + e(0, 1) + reachY))),
		Right:  int32(math.Floor(float64(r.X + r.W - e(1, 2) - reachX))),
		Bottom: int32(math.Floor(float64(r.Y + r.H - e(2, 3) - reachY))),
	}
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func continuous(r [4]float32) bool { return r[0] < 0 || r[1] < 0 || r[2] < 0 || r[3] < 0 }

// cornersOf returns, for a fill, an image or a shadow with continuous
// corners, not transformed, where it and its twin with square corners (no
// radii, on its border's inner edge or its caster neither) draw the same,
// as the lines that bound those pixels (see cornerLines), and the least
// area of the twin worth its draw calls; ok is false for any other
// instance.
//
// A fill's twin computes its border's inner edge the same way too, whose
// curves lie within the outer ones' boxes: each of its corners is a
// border's width or more in from the outer one, and its radius smaller by
// the larger of the two widths (scene.InnerRadii).
//
// A shadow's blur, of standard deviation sigma, takes all its corners as
// the largest (boxShadow), and the curve shapes the rows of its samples
// that lie within the curve's reach of the top or the bottom, the samples
// reaching 3 sigma from the pixel. At a pixel whose samples all lie outside
// those rows the shader finds the sides straight, as for square corners;
// at one in those rows but 9 sigma or more from where the curve can put the
// sides, the blurred edges are exactly 0 and 1 (erf2 rounds to ±1 from about
// 7.7 sigma), wherever the curve puts them. The box casting it, cut out of
// the shadow, is a fill's coverage.
func cornersOf(in *Instance, m scene.Affine) (free Scissor, least int64, ok bool) {
	shadow := in.Params[0] == 1
	if m.Set || !continuous(in.Radii) && !(shadow && continuous(in.Inner)) {
		return Scissor{}, 0, false
	}
	r := scene.Rect{X: in.Rect[0], Y: in.Rect[1], W: in.Rect[2], H: in.Rect[3]}
	if !shadow {
		return cornerLines(r, in.Radii, 0, 0), cornerSplitMin, true
	}
	sigma := in.Params[2]
	corner := max(abs32(in.Radii[0]), abs32(in.Radii[1]), abs32(in.Radii[2]), abs32(in.Radii[3]))
	free = cornerLines(r, [4]float32{corner, corner, corner, corner}, 9*sigma, 3*sigma)
	if in.UV[2] > 0 && in.UV[3] > 0 {
		free = free.intersect(cornerLines(scene.Rect{X: in.UV[0], Y: in.UV[1], W: in.UV[2], H: in.UV[3]}, in.Inner, 0, 0))
	}
	return free, shadowCornerSplitMin, true
}

// addCorners appends in as a nine-patch within the batches' scissor
// rectangle: its corners as it is and the rest as its twin. In each band of
// rows across a corner the twin draws between the corners unless that is
// small; the band of rows between the corners is all the twin's.
func (b *Builder) addCorners(in Instance, image uintptr, m scene.Affine) {
	f, least, ok := cornersOf(&in, m)
	if !ok {
		b.add(in, image, m)
		return
	}
	cur := &b.stack[len(b.stack)-1]
	region := b.batchScissor(cur)
	if f.Top > f.Bottom {
		// Every row is near a corner: either band can hold those between.
		f.Top = f.Bottom
	}
	top := Scissor{region.Left, region.Top, region.Right, f.Top}.intersect(region)
	mid := Scissor{region.Left, f.Top, region.Right, f.Bottom}.intersect(region)
	bottom := Scissor{region.Left, f.Bottom, region.Right, region.Bottom}.intersect(region)
	var pieces [7]Scissor // the shape's, then the twin's
	var twin [7]bool
	n := 0
	piece := func(s Scissor, t bool) {
		if !s.Empty() {
			pieces[n], twin[n] = s, t
			n++
		}
	}
	area := mid.area()
	var between [2]Scissor
	for i, band := range [2]Scissor{top, bottom} {
		c := Scissor{f.Left, band.Top, f.Right, band.Bottom}.intersect(band)
		if c.area() < least/4 {
			piece(band, false)
			continue
		}
		piece(Scissor{band.Left, band.Top, c.Left, band.Bottom}, false)
		piece(Scissor{c.Right, band.Top, band.Right, band.Bottom}, false)
		between[i] = c
		area += c.area()
	}
	if area < least {
		b.add(in, image, m)
		return
	}
	piece(mid, true)
	piece(between[0], true)
	piece(between[1], true)
	sq := in
	sq.Radii, sq.Inner = [4]float32{}, [4]float32{}
	split, splitScissor := b.split, b.splitScissor
	defer b.scissorBatches(splitScissor, split)
	for i, s := range pieces[:n] {
		b.scissorBatches(s, true)
		if twin[i] {
			b.add(sq, image, m)
		} else {
			b.add(in, image, m)
		}
	}
}
