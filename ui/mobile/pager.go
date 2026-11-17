package mobile

import "github.com/egoist/mygo/ui"

// PageState is what page of a PageView shows, kept in the app's state.
type PageState struct {
	// Page is the page it rests at, or goes to. Set it to go there.
	Page int

	offset ui.Point
	width  float32
	last   int
}

// PageView shows one of count pages at a time, which a finger swipes between,
// as UIPageViewController and ViewPager do, the page it lands on following the
// throw. Only the pages next to what shows are built. It fills its parent:
// size it with Height, Grow or Fill.
//
//	mobile.PageView(c, &app.pages, 3, func(i int) { pages[i](c) }).Fill()
func PageView(c *ui.Context, s *PageState, count int, page func(i int)) ui.Element {
	box := ui.Box(c).Clip().Role(ui.RoleGroup)
	width := box.Bounds().W
	if width <= 0 {
		// Laid out for the first time: its width shows next frame.
		c.AnimationFrame()
		return box
	}
	if count <= 0 {
		return box
	}
	s.Page = max(0, min(s.Page, count-1))
	box.Children(func() {
		points := make([]ui.Point, count)
		for i := range points {
			points[i] = ui.Point{X: -float32(i) * width}
		}
		track := ui.Row(c).Absolute().Left(0).Top(0).Bottom(0).Width(width * float32(count))
		if s.width != width {
			// A new width, as the device turned: the page stays.
			s.width = width
			s.offset = points[s.Page]
		}
		if s.last != s.Page {
			s.last = s.Page
			s.offset = points[s.Page]
		}
		track.Draggable(&s.offset, ui.DraggableOptions{
			Axis:   ui.Horizontal,
			Bounds: &ui.Rect{X: -float32(count-1) * width, W: float32(count-1) * width},
			// One page a throw, as ViewPager's: from the page it left.
			Snap: func(p ui.Point) ui.Point {
				to := int(-p.X/width + 0.5)
				return points[max(0, min(to, min(s.Page+1, count-1)), max(0, s.Page-1))]
			},
		})
		shown := track.DragOffset().X
		at := int(-shown/width + 0.5)
		lo, hi := max(0, at-1), min(count-1, at+1)
		track.Children(func() {
			for i := range count {
				cell := ui.Box(c.Key(i)).Width(width).Shrink(0).FillHeight()
				if i >= lo && i <= hi {
					cell.Children(func() { page(i) })
				}
			}
		})
		if !track.Transitioning() && !track.Dragging() {
			if p := int(-s.offset.X/width + 0.5); p != s.Page && p >= 0 && p < count {
				s.Page, s.last = p, p
				c.AnimationFrame()
			}
		}
	})
	return box
}

// PageDots shows which of count pages is chosen, as UIPageControl does.
func PageDots(c *ui.Context, page, count int) ui.Element {
	t := c.Theme()
	return ui.Row(c).Gap(8).Justify(ui.Center).AlignItems(ui.Center).Padding(8).Children(func() {
		for i := range count {
			color := t.TextMuted.Alpha(0.35)
			if i == page {
				color = t.Text
			}
			ui.Box(c.Key(i)).Size(7, 7).Radius(3.5).Background(color)
		}
	})
}
