package mobile

import "github.com/egoist/mygo/ui"

// RowOptions are what a ListRow shows.
type RowOptions struct {
	Title, Subtitle string
	// Leading and Trailing build what goes at its ends.
	Leading, Trailing func()
	// Disclosure shows that it goes somewhere: a chevron on Cupertino.
	Disclosure bool
}

// ListRow shows a row of a list, of a height a finger can tap: UITableViewCell's
// on Cupertino and Material's list item on Material, with its look of being
// pressed. Clicked reports a tap.
func ListRow(c *ui.Context, o RowOptions) ui.Element {
	t := c.Theme()
	minH := float32(44)
	if Current() == Material {
		minH = 56
	}
	label := o.Title
	if o.Subtitle != "" {
		label += ", " + o.Subtitle // screen readers read the row as one
	}
	row := ui.Row(c).FillWidth().MinHeight(minH).AlignItems(ui.Center).Padding(10, 16).Gap(12).Role(ui.RoleButton).Label(label)
	press(row, t)
	row.Children(func() {
		if o.Leading != nil {
			o.Leading()
		}
		ui.Column(c).Grow(1).Basis(0).Gap(2).Children(func() {
			ui.Text(c, o.Title)
			if o.Subtitle != "" {
				ui.Text(c, o.Subtitle).FontSize(14).TextColor(t.TextMuted)
			}
		})
		if o.Trailing != nil {
			o.Trailing()
		}
		if o.Disclosure && Current() == Cupertino {
			ui.Text(c, "›").FontSize(20).TextColor(t.TextMuted)
		}
	})
	return row
}

// Section shows rows together under a header: UITableView's inset grouped style
// on Cupertino, with a line between them; on Material, a heading over the list.
func Section(c *ui.Context, header string, rows func()) ui.Element {
	t := c.Theme()
	col := ui.Column(c).FillWidth().Gap(6)
	if Current() == Cupertino {
		col.PaddingX(16)
	}
	col.Children(func() {
		if header != "" {
			h := ui.Text(c, header).FontSize(13).TextColor(t.TextMuted)
			if Current() == Material {
				h.PaddingX(16)
			}
		}
		body := ui.Column(c).FillWidth().Dividers(0.5, t.Border)
		if Current() == Cupertino {
			body.Background(t.Surface).Radius(10).Clip()
		}
		body.Children(rows)
	})
	return col
}
