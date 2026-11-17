package mobile

import (
	"github.com/egoist/mygo/ui"
)

// SheetOptions are how a Sheet rests.
type SheetOptions struct {
	// Detents are the heights it rests at, as fractions of the window's, in
	// any order: UISheetPresentationController's medium and large are
	// {0.5, 0.92}. It opens at the first, and one {0.5} when none.
	Detents []float32
	// NoGrabber hides the grabber at its top.
	NoGrabber bool
	// NoDismiss keeps it from going away by a drag, a tap outside or back.
	NoDismiss bool
	// Background is its color, the theme's Background by default.
	Background *ui.Color
	// Label names it for assistive technology.
	Label string
}

type sheetState struct {
	offset ui.Point
	// shown says it is on screen, opened that it went up from below it.
	shown, opened bool
	// height is what it was laid out for, which detent, the one it rests at,
	// takes it back to as the window changes.
	height float32
	detent int
}

// Sheet shows a panel coming up from the bottom of the window while *open,
// over what is under it, dimmed: a finger drags it among its detents, and
// down and away, which sets *open to false, as do a tap outside it and back
// (Android's, and Escape). What it holds that scrolls scrolls first.
//
//	mobile.Sheet(c, &app.filters, mobile.SheetOptions{Detents: []float32{0.5, 0.92}}, func() {
//		ui.Text(c, "Filters")
//	})
func Sheet(c *ui.Context, open *bool, o SheetOptions, content func()) {
	s := ui.Local(c.Root(), sheetKey{open}, func() sheetState { return sheetState{} })
	if !*open && !s.shown {
		return
	}
	_, winH := c.Size()
	detents := o.Detents
	if len(detents) == 0 {
		detents = []float32{0.5}
	}
	tall := float32(0)
	for _, d := range detents {
		tall = max(tall, d)
	}
	height := winH * tall
	// The offset's 0 is the tallest detent; closed it is below the window.
	rest := func(d float32) ui.Point { return ui.Point{Y: height - winH*d} }
	closedY := height
	born := !s.shown
	if born {
		s.shown, s.opened = true, false
		s.offset = ui.Point{Y: closedY}
	}
	if s.opened && s.height != height && s.height > 0 {
		// The window turned: the same detent, in its new height.
		s.offset = rest(detents[min(s.detent, len(detents)-1)])
	}
	s.height = height
	points := make([]ui.Point, 0, len(detents)+1)
	for i, d := range detents {
		points = append(points, rest(d))
		if s.opened && s.offset == points[i] {
			s.detent = i
		}
	}
	if !o.NoDismiss {
		points = append(points, ui.Point{Y: closedY})
	}
	t := c.Theme()
	_, _, bottom, _ := c.SafeArea()
	ui.Overlay(c, func() {
		back := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Modal()
		if !o.NoDismiss && (back.Clicked() || back.OverlayShortcut(0, ui.KeyEscape)) {
			*open = false
		}
		bg := t.Background
		if o.Background != nil {
			bg = *o.Background
		}
		var panel ui.Element
		back.Children(func() {
			panel = ui.Column(c).Absolute().Left(0).Right(0).Bottom(0).Height(height).
				Background(bg).Radius(overlayRadius(), overlayRadius(), 0, 0).
				Shadow(0, -2, 16, 0, ui.RGBA(0, 0, 0, 0.18)).Role(ui.RoleDialog).Label(o.Label)
			// Clicks in it stay in it.
			panel.Clicked()
			panel.Draggable(&s.offset, ui.DraggableOptions{
				Axis:   ui.Vertical,
				Bounds: &ui.Rect{H: closedY},
				Snap:   ui.Nearest(points...),
			})
			panel.Children(func() {
				if !o.NoGrabber {
					ui.Box(c).Width(36).Height(5).Radius(2.5).Background(t.TextMuted.Alpha(0.35)).AlignSelf(ui.Center).Margin(8, 0, 4, 0)
				}
				ui.Column(c).Grow(1).Basis(0).Padding(0, 0, bottom, 0).Children(content)
			})
		})
		switch {
		case born:
			c.AnimationFrame() // shows below the window, to go up from there
		case !*open:
			s.offset = ui.Point{Y: closedY}
			s.opened = false
		case !s.opened:
			s.opened, s.detent = true, 0
			s.offset = rest(detents[0])
		case !o.NoDismiss && s.offset.Y >= closedY && !panel.Dragging():
			*open = false // dragged away
			s.opened = false
		}
		shown := panel.DragOffset()
		back.Background(scrim(c, 1-shown.Y/max(closedY, 1)))
		if !*open && !born && shown.Y >= closedY-0.5 {
			s.shown = false
		}
	})
}

type sheetKey struct{ open *bool }

// Action is a choice of an ActionSheet.
type Action struct {
	Label string
	// Destructive shows that it destroys something, in the theme's Danger.
	Destructive bool
	// Disabled grays it, and it does nothing.
	Disabled bool
	// OnTap runs as it is chosen, on the main thread; the sheet then closes.
	OnTap func()
}

// ActionSheetOptions are what an ActionSheet says.
type ActionSheetOptions struct {
	Title, Message string
	// Cancel is what its button of going back says, "Cancel" by default.
	Cancel string
}

// ActionSheet shows a list of choices coming up from the bottom of the window
// while *open: iOS's action sheet on Cupertino, a bottom sheet's list on
// Material. Choosing one, or going back, sets *open to false.
func ActionSheet(c *ui.Context, open *bool, o ActionSheetOptions, actions ...Action) {
	if o.Cancel == "" {
		o.Cancel = "Cancel"
	}
	t := c.Theme()
	_, winH := c.Size()
	if Current() == Cupertino {
		bg := ui.RGBA(0, 0, 0, 0)
		rowH := float32(57)
		_, _, bottom, _ := c.SafeArea()
		need := rowH*float32(len(actions)+1) + 40 + bottom
		if o.Title != "" || o.Message != "" {
			need += 72
		}
		Sheet(c, open, SheetOptions{Detents: []float32{min(need/max(winH, 1), 0.9)}, NoGrabber: true, Background: &bg, Label: o.Title}, func() {
			ui.Column(c).Padding(0, 8, 8, 8).Gap(8).Grow(1).Justify(ui.End).Children(func() {
				ui.Column(c).Background(t.Surface).Radius(14).Clip().Children(func() {
					if o.Title != "" || o.Message != "" {
						ui.Column(c).Padding(14, 16).AlignItems(ui.Center).Gap(2).Children(func() {
							if o.Title != "" {
								ui.Text(c, o.Title).FontSize(13).Bold().TextColor(t.TextMuted)
							}
							if o.Message != "" {
								ui.Text(c, o.Message).FontSize(13).TextColor(t.TextMuted).TextAlign(ui.Center)
							}
						})
						ui.Divider(c)
					}
					for i, a := range actions {
						if i > 0 {
							ui.Divider(c)
						}
						actionRow(c, i, a, open, rowH, true)
					}
				})
				cancel := ui.Row(c).Height(rowH).AlignItems(ui.Center).Justify(ui.Center).Background(t.Surface).Radius(14).Role(ui.RoleButton).Label(o.Cancel)
				press(cancel, t)
				cancel.Children(func() { ui.Text(c, o.Cancel).Bold().TextColor(t.Accent).FontSize(20) })
				if cancel.Clicked() {
					*open = false
				}
			})
		})
		return
	}
	rowH := float32(52)
	_, _, bottom, _ := c.SafeArea()
	need := rowH*float32(len(actions)) + 48 + bottom
	if o.Title != "" {
		need += 48
	}
	Sheet(c, open, SheetOptions{Detents: []float32{min(need/max(winH, 1), 0.9)}, Label: o.Title}, func() {
		ui.Column(c).Grow(1).Children(func() {
			if o.Title != "" {
				ui.Text(c, o.Title).TextColor(t.TextMuted).Padding(12, 24)
			}
			for i, a := range actions {
				actionRow(c, i, a, open, rowH, false)
			}
		})
	})
}

func actionRow(c *ui.Context, i int, a Action, open *bool, height float32, centered bool) {
	t := c.Theme()
	color := t.Accent
	if !centered {
		color = t.Text
	}
	if a.Destructive {
		color = t.Danger
	}
	row := ui.Row(c.Key(i)).Height(height).AlignItems(ui.Center).PaddingX(24).Role(ui.RoleButton).Label(a.Label).Disabled(a.Disabled)
	if centered {
		row.Justify(ui.Center)
	}
	press(row, t)
	size := float32(16)
	if centered {
		size = 20
	}
	row.Children(func() { ui.Text(c, a.Label).TextColor(color).FontSize(size) })
	if row.Clicked() {
		*open = false
		if a.OnTap != nil {
			a.OnTap()
		}
	}
}

// press shows the look of a press: Material's ink, or Cupertino's highlight.
func press(e ui.Element, t *ui.Theme) {
	if Current() == Material {
		Ink(e)
		return
	}
	if e.Pressed() {
		e.Background(t.SurfacePressed)
	}
}
