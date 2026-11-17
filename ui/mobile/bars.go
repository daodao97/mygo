package mobile

import "github.com/egoist/mygo/ui"

// TopBarOptions are what a TopBar shows.
type TopBarOptions struct {
	Title string
	// Leading and Trailing build what goes at its ends, as IconButton's.
	Leading, Trailing func()
	// Large shows the title large, under the bar, as UINavigationBar's large
	// titles.
	Large bool
	// Background is its color, the theme's Background by default.
	Background *ui.Color
}

// TopBar shows a bar across the top of a screen, drawn by the app rather than
// the system's (Window.Push): with its title centered on Cupertino and from
// the start on Material, for screens of one window that route within it
// (ui.Router).
func TopBar(c *ui.Context, o TopBarOptions) ui.Element {
	t := c.Theme()
	bg := t.Background
	if o.Background != nil {
		bg = *o.Background
	}
	height := float32(44)
	if Current() == Material {
		height = 56
	}
	bar := ui.Column(c).FillWidth().Background(bg).Shrink(0).Role(ui.RoleGroup)
	bar.Children(func() {
		ui.Row(c).Height(height).AlignItems(ui.Center).PaddingX(4).Gap(4).Children(func() {
			ui.Row(c).AlignItems(ui.Center).MinWidth(48).Children(func() {
				if o.Leading != nil {
					o.Leading()
				}
			})
			title := ui.Row(c).Grow(1).Basis(0).AlignItems(ui.Center)
			if Current() == Cupertino {
				title.Justify(ui.Center)
			}
			title.Children(func() {
				if !o.Large {
					size := float32(17)
					if Current() == Material {
						size = 22
					}
					ui.Text(c, o.Title).FontSize(size).FontWeight(600).MaxLines(1).Role(ui.RoleHeading).Level(1)
				}
			})
			ui.Row(c).AlignItems(ui.Center).Justify(ui.End).MinWidth(48).Children(func() {
				if o.Trailing != nil {
					o.Trailing()
				}
			})
		})
		if o.Large {
			ui.Text(c, o.Title).FontSize(34).Bold().PaddingX(16).Padding(0, 16, 8, 16).Role(ui.RoleHeading).Level(1)
		}
	})
	return bar
}

// keyboardAtLeast tells a keyboard from the system's bars in the window's
// bottom inset: they are far less tall.
const keyboardAtLeast = 120

// BarButton is a destination of a BottomBar.
type BarButton struct {
	Label string
	// Icon draws it, in color, in r: lines and shapes (ui.Painter).
	Icon func(p *ui.Painter, r ui.Rect, color ui.Color)
	// Badge shows a count or a word on it.
	Badge string
}

// BottomBar shows a bar of destinations across the bottom of a screen, choosing
// *selected: UITabBar's on Cupertino, Material's navigation bar on Material,
// drawn by the app rather than the system's (mygo.NewTabs), keeping above the
// system's bars. Changed reports a new choice.
func BottomBar(c *ui.Context, selected *int, buttons ...BarButton) ui.Element {
	t := c.Theme()
	_, _, bottom, _ := c.SafeArea()
	// Down to the screen's edge, under the system's bar: the window's root
	// keeps its content above it.
	bar := ui.Row(c).FillWidth().Background(t.Surface).Padding(0, 0, bottom, 0).Margin(0, 0, -bottom, 0).Shrink(0).Role(ui.RoleTabList)
	if bottom > keyboardAtLeast {
		// The keyboard covers it, as it does the system's.
		return bar.Hidden(true)
	}
	height := float32(49)
	if Current() == Material {
		height = 80
	}
	bar.Children(func() {
		for i, b := range buttons {
			on := i == *selected
			color := t.TextMuted
			if on {
				color = t.Accent
			}
			item := ui.Column(c.Key(i)).Grow(1).Basis(0).Height(height).AlignItems(ui.Center).Justify(ui.Center).Gap(2).
				Role(ui.RoleTab).Label(b.Label).Checked(on)
			item.Children(func() {
				icon := ui.Box(c).Size(28, 24)
				if Current() == Material && on {
					icon.Size(56, 32).Radius(16).Background(t.Accent.Alpha(0.18))
				}
				if b.Icon != nil {
					icon.Draw(func(p *ui.Painter, r ui.Rect) {
						s := float32(24)
						b.Icon(p, ui.Rect{X: r.X + (r.W-s)/2, Y: r.Y + (r.H-s)/2, W: s, H: s}, color)
					})
				}
				if b.Badge != "" {
					icon.Children(func() {
						ui.Text(c, b.Badge).Absolute().Top(-2).Right(-6).FontSize(10).PaddingX(4).Radius(8).
							Background(t.Danger).TextColor(ui.RGB(255, 255, 255))
					})
				}
				size := float32(10)
				if Current() == Material {
					size = 12
				}
				ui.Text(c, b.Label).FontSize(size).TextColor(color).NoWrap()
			})
			if item.Clicked() && !on {
				*selected = i
				bar.SetChanged()
			}
		}
	})
	return bar
}

// DrawerOptions are how a Drawer shows.
type DrawerOptions struct {
	// Width is how wide it is, 300 by default.
	Width float32
	// End puts it at the end of the screen, rather than the start.
	End bool
}

type drawerState struct {
	offset        ui.Point
	shown, opened bool
}

// Drawer shows a panel sliding in from the side of the window while *open, over
// what is under it, dimmed, as Material's navigation drawer: a finger drags it
// away, a tap outside and back close it, which sets *open to false.
func Drawer(c *ui.Context, open *bool, o DrawerOptions, content func()) {
	st := ui.Local(c.Root(), drawerKey{open}, func() drawerState { return drawerState{} })
	if !*open && !st.shown {
		return
	}
	width := o.Width
	if width <= 0 {
		width = 300
	}
	gone := -width
	if o.End {
		gone = width
	}
	born := !st.shown
	if born {
		st.shown, st.opened = true, false
		st.offset = ui.Point{X: gone}
	}
	t := c.Theme()
	top, _, bottom, _ := c.SafeArea()
	ui.Overlay(c, func() {
		back := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Modal()
		if back.Clicked() || back.OverlayShortcut(0, ui.KeyEscape) {
			*open = false
		}
		var panel ui.Element
		back.Children(func() {
			panel = ui.Column(c).Absolute().Top(0).Bottom(0).Width(width).Background(t.Background).Padding(top, 0, bottom, 0).
				Shadow(0, 0, 16, 0, ui.RGBA(0, 0, 0, 0.2)).Role(ui.RoleDialog)
			if o.End {
				panel.Right(0)
			} else {
				panel.Left(0)
			}
			bounds := ui.Rect{X: min(gone, 0), W: width}
			panel.Draggable(&st.offset, ui.DraggableOptions{Axis: ui.Horizontal, Bounds: &bounds, Rigid: true,
				Snap: ui.Nearest(ui.Point{}, ui.Point{X: gone})})
			panel.Children(content)
		})
		switch {
		case born:
			c.AnimationFrame()
		case !*open:
			st.offset = ui.Point{X: gone}
			st.opened = false
		case !st.opened:
			st.opened = true
			st.offset = ui.Point{}
		case st.offset.X == gone && !panel.Dragging():
			*open = false // dragged away
			st.opened = false
		}
		shown := panel.DragOffset().X
		frac := 1 - abs(shown)/width
		back.Background(scrim(c, frac))
		if !*open && !born && abs(shown) >= width-0.5 {
			st.shown = false
		}
	})
}

type drawerKey struct{ open *bool }

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// FloatingButton shows Material's floating action button, or a round button on
// Cupertino: place it with Absolute.
func FloatingButton(c *ui.Context, label string, icon func(p *ui.Painter, r ui.Rect, color ui.Color)) ui.Element {
	t := c.Theme()
	e := ui.Box(c).Size(56, 56).Radius(16).Background(t.Accent).Shadow(0, 3, 8, 0, ui.RGBA(0, 0, 0, 0.3)).Role(ui.RoleButton).Label(label)
	if Current() == Cupertino {
		e.Radius(28)
	}
	press(e, t)
	if icon != nil {
		e.Draw(func(p *ui.Painter, r ui.Rect) {
			icon(p, ui.Rect{X: r.X + 16, Y: r.Y + 16, W: 24, H: 24}, t.AccentText)
		})
	}
	return e
}
