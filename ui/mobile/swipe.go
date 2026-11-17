package mobile

import "github.com/egoist/mygo/ui"

// SwipeAction is a button a SwipeRow reveals.
type SwipeAction struct {
	Label string
	// Color is its background, the theme's Danger for one that is
	// Destructive, or else its Accent.
	Color       *ui.Color
	Destructive bool
	OnTap       func()
}

// SwipeState is how a SwipeRow rests, kept by the app per row where rows may
// go away; a nil state keeps it in the element.
type SwipeState struct {
	offset ui.Point
	open   bool
}

// Close puts the row back, hiding its actions.
func (s *SwipeState) Close() { s.offset, s.open = ui.Point{}, false }

const swipeActionWidth = 80

// SwipeRow shows content that a finger swipes aside to reveal actions behind
// it, as UITableView's swipe actions: Trailing as it goes left, Leading as it
// goes right. A long swipe chooses the first action of that side. A finger
// going down the list scrolls the list instead.
//
//	mobile.SwipeRow(c.Key(i), nil, mobile.SwipeOptions{
//		Trailing: []mobile.SwipeAction{{Label: "Delete", Destructive: true, OnTap: func() { app.remove(i) }}},
//	}, func() { ui.Text(c, row.Title).Padding(14) })
func SwipeRow(c *ui.Context, s *SwipeState, o SwipeOptions, content func()) ui.Element {
	t := c.Theme()
	// One cell, which holds the actions and the content over them.
	box := ui.Grid(c).Columns(1).Clip().FillWidth()
	if s == nil {
		s = ui.Local(box, swipeKey{}, func() SwipeState { return SwipeState{} })
	}
	box.Children(func() {
		trailing := float32(len(o.Trailing)) * swipeActionWidth
		leading := float32(len(o.Leading)) * swipeActionWidth
		// Behind the content.
		actions := func(list []SwipeAction, end bool) {
			if len(list) == 0 {
				return
			}
			row := ui.Row(c).ColumnStart(1).RowStart(1).AlignSelf(ui.Stretch).AlignItems(ui.Stretch)
			if end {
				row.JustifySelf(ui.End)
			} else {
				row.JustifySelf(ui.Start)
			}
			row.Children(func() {
				for i, a := range list {
					bg := t.Accent
					if a.Destructive {
						bg = t.Danger
					}
					if a.Color != nil {
						bg = *a.Color
					}
					b := ui.Row(c.Key(i)).Width(swipeActionWidth).AlignItems(ui.Center).Justify(ui.Center).
						Background(bg).Role(ui.RoleButton).Label(a.Label).Children(func() {
						ui.Text(c, a.Label).TextColor(ui.RGB(255, 255, 255)).FontSize(14).TextAlign(ui.Center)
					})
					if b.Clicked() {
						s.Close()
						if a.OnTap != nil {
							a.OnTap()
						}
					}
				}
			})
		}
		actions(o.Leading, false)
		actions(o.Trailing, true)
		// The content, over them.
		var points []ui.Point
		points = append(points, ui.Point{})
		if trailing > 0 {
			points = append(points, ui.Point{X: -trailing})
		}
		if leading > 0 {
			points = append(points, ui.Point{X: leading})
		}
		face := ui.Box(c).ColumnStart(1).RowStart(1).Background(t.Background).Draggable(&s.offset, ui.DraggableOptions{
			Axis:   ui.Horizontal,
			Bounds: &ui.Rect{X: -trailing, W: trailing + leading},
			Snap:   ui.Nearest(points...),
		})
		face.Children(content)
		if !face.Transitioning() && !face.Dragging() {
			s.open = s.offset.X != 0
		}
	})
	return box
}

type swipeKey struct{}

// SwipeOptions are the actions a SwipeRow reveals.
type SwipeOptions struct {
	Leading, Trailing []SwipeAction
}
