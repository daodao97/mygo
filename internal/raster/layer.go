package raster

import (
	"image"

	"github.com/egoist/mygo/internal/scene"
)

// Layer draws scenes over what an image holds already, one after another,
// where Render draws a scene over its clear color: a layer that takes
// what it draws in parts, as the paths a frame's mask atlas has no room
// for, each part a scene of the masks of an atlas that the next one
// reuses. It keeps the memory it draws with from one part to the next.
type Layer struct {
	d    drawer
	diff scene.Differ
}

// Over draws s within area over what dst, s.Width×s.Height, holds.
func (l *Layer) Over(dst *Image, s *scene.Scene, area image.Rectangle) {
	l.diff.Reach = 1
	l.diff.Diff(s)
	l.d.over = true
	l.d.draw(dst, s, area, l.diff.Bounds())
}
