package mobile

import "github.com/egoist/mygo/ui"

// ZoomState is how a Zoomable shows its content, kept in the app's state.
type ZoomState struct {
	// Scale is how large, 1 by default; X and Y how far the content moved, in
	// DIPs from its place; Rotation how far it turned, in radians, if
	// ZoomOptions.Rotate.
	Scale, X, Y, Rotation float32
}

// Reset shows the content as it was.
func (z *ZoomState) Reset() { *z = ZoomState{Scale: 1} }

// ZoomOptions are how far a Zoomable goes.
type ZoomOptions struct {
	// Min and Max are the scales it goes from and to: 1 and 4 by default.
	Min, Max float32
	// DoubleTap is the scale a double tap zooms to from the smallest, and
	// back; 2 by default.
	DoubleTap float32
	// Rotate lets two fingers turn the content too.
	Rotate bool
}

// Zoomable shows content that two fingers pinch to zoom and one drags, and a
// double tap zooms, as UIScrollView's zooming and Flutter's InteractiveViewer,
// for a photo or a map: on what pinches the system tells, around the point
// between the fingers. It clips the content to its box. Size it: Fill, Height.
func Zoomable(c *ui.Context, z *ZoomState, o ZoomOptions, content func()) ui.Element {
	if o.Min <= 0 {
		o.Min = 1
	}
	if o.Max <= 0 {
		o.Max = 4
	}
	if o.DoubleTap <= 0 {
		o.DoubleTap = 2
	}
	if z.Scale == 0 {
		z.Scale = 1
	}
	box := ui.Box(c).Clip().Role(ui.RoleImage)
	bounds := box.Bounds()
	clamp := func() {
		z.Scale = max(o.Min, min(z.Scale, o.Max))
		mx := bounds.W * (z.Scale - 1) / 2
		my := bounds.H * (z.Scale - 1) / 2
		z.X = max(-mx, min(z.X, mx))
		z.Y = max(-my, min(z.Y, my))
	}
	box.HandleInput(func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputZoom:
			// Around the point between the fingers.
			k := max(o.Min, min(z.Scale*ev.Scale, o.Max)) / z.Scale
			cx, cy := ev.X-bounds.W/2, ev.Y-bounds.H/2
			z.X = cx - (cx-z.X)*k
			z.Y = cy - (cy-z.Y)*k
			z.Scale *= k
			if o.Rotate {
				z.Rotation += ev.Rotation
			}
			clamp()
			return true
		case ui.InputScroll:
			if z.Scale <= 1 {
				return false
			}
			z.X -= ev.DX
			z.Y -= ev.DY
			clamp()
			return true
		}
		return false
	})
	if box.DoubleClicked() {
		if z.Scale > o.Min {
			z.Scale, z.X, z.Y = o.Min, 0, 0
		} else {
			z.Scale = o.DoubleTap
			x, y, _ := box.PointerPosition()
			z.X = (bounds.W/2 - x) * (z.Scale - 1)
			z.Y = (bounds.H/2 - y) * (z.Scale - 1)
			clamp()
		}
	}
	scale := box.AnimateTo("scale", z.Scale, ui.Snappy)
	x := box.AnimateTo("x", z.X, ui.Snappy)
	y := box.AnimateTo("y", z.Y, ui.Snappy)
	box.Children(func() {
		ui.Box(c).Fill().Translate(x, y).Scale(scale, scale).Rotate(z.Rotation).Children(content)
	})
	return box
}
