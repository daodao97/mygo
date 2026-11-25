package ui_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/ui"
)

// accessOf returns what assistive technology sees of the element of role
// r named name, failing the test without one.
func accessOf(t *testing.T, tt *ui.Tester, r ui.Role, name string) ui.AccessInfo {
	t.Helper()
	a, ok := tt.AccessByRole(r, name)
	if !ok {
		t.Fatalf("no element of role %d named %q", r, name)
	}
	return a
}

// TestTesterAccess checks what Tester.Access and AccessByRole read of the
// last frame's accessibility tree: roles given and found from what
// elements do, names, values and states.
func TestTesterAccess(t *testing.T) {
	var (
		streaming = true
		agree     bool
		tab       = 1
		volume    = 30.0
		name      = "Ada"
		open      bool
	)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Children(func() {
			ui.Box(c).Role(ui.RoleStatus).Label("Reply").Busy(streaming)
			ui.Checkbox(c, &agree, "I agree")
			ui.CheckboxBase(c, &agree).Label("Select all").Mixed()
			ui.Tabs(c, &tab, "One", "Two")
			ui.Slider(c, &volume, 0, 100).Label("Volume")
			ui.TextInput(c, &name).Label("Name").Placeholder("Your name")
			ui.Text(c, "Settings").Role(ui.RoleHeading).Level(2)
			ui.Row(c).Role(ui.RoleNavigation).Label("Main").Children(func() {
				ui.Link(c, "Home", "/")
				ui.Link(c, "Docs", "/docs").Current(ui.CurrentPage)
			})
			ui.Row(c).Role(ui.RoleSearch).Label("Site search").Children(func() {
				ui.Button(c, "Go")
			})
			ui.Column(c).Role(ui.RoleArticle).Label("Post by Ann").Children(func() {
				ui.Text(c, "Shipped the beta")
			})
			ui.Collapsible(c, "More", &open, func() { ui.Text(c, "Hidden") })
			ui.Button(c, "Save").Disabled(true)
			ui.Box(c).Size(24, 24).Label("Error").Description("Disk full")
		})
	}, 400, 600)

	reply := accessOf(t, tt, ui.RoleStatus, "Reply")
	if !reply.Busy || reply.Role != ui.RoleStatus {
		t.Errorf("Reply: %+v", reply)
	}
	if a, ok := tt.Access("Reply"); !ok || !reflect.DeepEqual(a, reply) {
		t.Errorf("Access(Reply) = %+v, %v; AccessByRole found %+v", a, ok, reply)
	}
	streaming = false
	tt.Frame()
	if accessOf(t, tt, ui.RoleStatus, "Reply").Busy {
		t.Error("Reply is still busy")
	}
	if _, ok := tt.AccessByRole(ui.RoleButton, "Reply"); ok {
		t.Error("found Reply as a button")
	}
	if _, ok := tt.Access("Nothing"); ok {
		t.Error("found an element named Nothing")
	}

	if box := accessOf(t, tt, ui.RoleCheckBox, "I agree"); box.Checked || box.Mixed || !box.Focusable {
		t.Errorf("I agree: %+v", box)
	}
	if err := tt.Click("I agree"); err != nil {
		t.Fatal(err)
	}
	if box := accessOf(t, tt, ui.RoleCheckBox, "I agree"); !box.Checked || !box.Focused {
		t.Errorf("I agree, clicked: %+v", box)
	}
	if all := accessOf(t, tt, ui.RoleCheckBox, "Select all"); !all.Mixed || all.Checked {
		t.Errorf("Select all: %+v", all)
	}

	// A tab chosen is selected, as aria-selected says, not checked.
	if two := accessOf(t, tt, ui.RoleTab, "Two"); !two.Selected || two.Checked {
		t.Errorf("tab Two: %+v", two)
	}
	if one := accessOf(t, tt, ui.RoleTab, "One"); one.Selected {
		t.Errorf("tab One: %+v", one)
	}

	if v := accessOf(t, tt, ui.RoleSlider, "Volume"); v.Min != 0 || v.Max != 100 || v.Now != 30 {
		t.Errorf("Volume: %+v", v)
	}
	field := accessOf(t, tt, ui.RoleTextField, "Name")
	if field.Value != "Ada" || field.Placeholder != "Your name" || field.Bounds.W <= 0 {
		t.Errorf("Name: %+v", field)
	}
	if r, _ := tt.Find("Name"); r != field.Bounds {
		t.Errorf("Name at %v, Find says %v", field.Bounds, r)
	}
	if h := accessOf(t, tt, ui.RoleHeading, "Settings"); h.Level != 2 {
		t.Errorf("Settings: %+v", h)
	}
	accessOf(t, tt, ui.RoleNavigation, "Main")
	if a, ok := tt.Access("Site search"); !ok || a.Role != ui.RoleSearch {
		t.Errorf("Site search: %+v, %v", a, ok)
	}
	accessOf(t, tt, ui.RoleArticle, "Post by Ann")
	if docs := accessOf(t, tt, ui.RoleLink, "Docs"); docs.Current != ui.CurrentPage {
		t.Errorf("Docs: %+v", docs)
	}
	if home := accessOf(t, tt, ui.RoleLink, "Home"); home.Current != ui.CurrentNone {
		t.Errorf("Home: %+v", home)
	}
	if first := accessOf(t, tt, ui.RoleLink, ""); first.Name != "Home" {
		t.Errorf("the first link is %q", first.Name)
	}
	if more := accessOf(t, tt, ui.RoleDisclosure, "More"); more.Expanded {
		t.Errorf("More: %+v", more)
	}
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if more := accessOf(t, tt, ui.RoleDisclosure, "More"); !more.Expanded {
		t.Errorf("More, opened: %+v", more)
	}
	if save := accessOf(t, tt, ui.RoleButton, "Save"); !save.Disabled || save.Focusable {
		t.Errorf("Save: %+v", save)
	}
	// A box with a Label and nothing else is a group.
	if e := accessOf(t, tt, ui.RoleGroup, "Error"); e.Description != "Disk full" {
		t.Errorf("Error: %+v", e)
	}
}

// TestTesterAccessReadOnly checks that reading the tree changes nothing:
// it builds no frame, and asks for none.
func TestTesterAccessReadOnly(t *testing.T) {
	frames := 0
	tt := ui.NewTester(func(c *ui.Context) {
		frames++
		ui.Button(c, "Save")
	}, 200, 100)
	before := frames
	if _, ok := tt.AccessByRole(ui.RoleButton, "Save"); !ok {
		t.Fatal("no button Save")
	}
	tt.Access("Save")
	if frames != before || tt.FramePending() {
		t.Errorf("reading the tree built %d frames, and asks for more: %v", frames-before, tt.FramePending())
	}
}

// TestTesterAccessRows checks a list's rows: where they are in the list,
// and the one chosen.
func TestTesterAccessRows(t *testing.T) {
	fruit := []string{"Apples", "Pears", "Plums"}
	selected := 0
	list := ui.ListState{Selected: &selected}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.List(c, &list, len(fruit), func(i int) {
			ui.Text(c, fruit[i]).Padding(6, 12)
		}).Grow(1).Label("Fruit")
	}, 300, 300)
	if err := tt.Click("Pears"); err != nil {
		t.Fatal(err)
	}
	if list := accessOf(t, tt, ui.RoleList, "Fruit"); list.SetSize != 3 || !list.Selectable {
		t.Errorf("Fruit: %+v", list)
	}
	pears := accessOf(t, tt, ui.RoleListItem, "Pears")
	if pears.PosInSet != 2 || pears.SetSize != 3 || !pears.Selected || pears.Checked {
		t.Errorf("Pears: %+v", pears)
	}
	if plums := accessOf(t, tt, ui.RoleListItem, "Plums"); plums.Selected {
		t.Errorf("Plums: %+v", plums)
	}
}

// TestTesterAccessStates checks the states built on the others: a role
// description, a popup, what an element controls, a column's sort and a
// grid's cell chosen without painting it.
func TestTesterAccessStates(t *testing.T) {
	open := true
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Children(func() {
			ui.Column(c).Role(ui.RoleGroup).Label("Highlights").RoleDescription("carousel")
			ui.Button(c, "Fruit").HasPopup(ui.PopupListBox)
			toggle := ui.Button(c, "Details").Expanded(open)
			if open {
				toggle.Controls(ui.Column(c).Role(ui.RoleRegion).Label("More details").Children(func() {
					ui.Text(c, "Hidden")
				}))
			}
			ui.Column(c).Role(ui.RoleTable).Label("November").Children(func() {
				ui.Row(c).Role(ui.RoleRow).Children(func() {
					ui.Text(c, "Day").Role(ui.RoleColumnHeader).Sort(ui.SortDescending)
					ui.Text(c, "Note").Role(ui.RoleColumnHeader).Sort(ui.SortOther)
				})
				ui.Row(c).Role(ui.RoleRow).Children(func() {
					ui.Text(c, "1").Role(ui.RoleCell).Checked(false)
					ui.Text(c, "2").Role(ui.RoleCell).Checked(true)
				})
			})
		})
	}, 400, 400)

	if h := accessOf(t, tt, ui.RoleGroup, "Highlights"); h.RoleDescription != "carousel" {
		t.Errorf("Highlights: %+v", h)
	}
	if f := accessOf(t, tt, ui.RoleButton, "Fruit"); f.HasPopup != ui.PopupListBox || f.RoleDescription != "" {
		t.Errorf("Fruit: %+v", f)
	}
	if d := accessOf(t, tt, ui.RoleButton, "Details"); !slices.Equal(d.Controls, []string{"More details"}) {
		t.Errorf("Details controls %q", d.Controls)
	}
	if day := accessOf(t, tt, ui.RoleColumnHeader, "Day"); day.Sort != ui.SortDescending {
		t.Errorf("Day: %+v", day)
	}
	if note := accessOf(t, tt, ui.RoleColumnHeader, "Note"); note.Sort != ui.SortOther {
		t.Errorf("Note: %+v", note)
	}
	if two := accessOf(t, tt, ui.RoleCell, "2"); !two.Selected || !two.Selectable || two.Checked {
		t.Errorf("cell 2: %+v", two)
	}
	if one := accessOf(t, tt, ui.RoleCell, "1"); one.Selected || !one.Selectable {
		t.Errorf("cell 1: %+v", one)
	}
	if nov := accessOf(t, tt, ui.RoleTable, "November"); !nov.Selectable {
		t.Errorf("November: %+v", nov)
	}

	open = false
	tt.Frame()
	if d := accessOf(t, tt, ui.RoleButton, "Details"); d.Controls != nil {
		t.Errorf("Details, closed, controls %q", d.Controls)
	}
}

// TestTesterAccessRelations checks, through the public API, what a field
// is described by, its error read while invalid, an orientation given, a
// search field of the app's own, and content deleted and inserted.
func TestTesterAccessRelations(t *testing.T) {
	card, query := "", ""
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Children(func() {
			hint := ui.Text(c, "16 digits")
			msg := ui.Text(c, "Required")
			ui.TextInput(c, &card).Label("Card").DescribedBy(hint).Invalid(card == "").ErrorMessage(msg)
			ui.TextInput(c, &query).Label("Search mail").Search()
			ui.Column(c).Role(ui.RoleToolbar).Label("Tools").Orientation(ui.Vertical)
			ui.Row(c).Children(func() {
				ui.Text(c, "$40").Strikethrough().Role(ui.RoleDeletion)
				ui.Text(c, "$30").Role(ui.RoleInsertion)
			})
		})
	}, 400, 400)
	f := accessOf(t, tt, ui.RoleTextField, "Card")
	if !f.Invalid || f.Description != "Required\n16 digits" || !slices.Equal(f.DescribedBy, []string{"16 digits"}) || !slices.Equal(f.ErrorMessage, []string{"Required"}) {
		t.Errorf("Card: %+v", f)
	}
	if s := accessOf(t, tt, ui.RoleTextField, "Search mail"); !s.Search {
		t.Errorf("Search mail: %+v", s)
	}
	if b := accessOf(t, tt, ui.RoleToolbar, "Tools"); !b.Vertical || b.Horizontal {
		t.Errorf("Tools: %+v", b)
	}
	accessOf(t, tt, ui.RoleDeletion, "$40")
	accessOf(t, tt, ui.RoleInsertion, "$30")
}

// TestTesterAccessAll checks that Tester.AccessAll lists every element of
// the last frame in reading order, as Access describes each, and finds a
// control without a name.
func TestTesterAccessAll(t *testing.T) {
	plus := ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path d="M8 3v10M3 8h10" stroke="currentColor"/></svg>`))
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Children(func() {
			ui.Button(c.Key("save"), "Save")
			ui.ButtonBase(c.Key("add")).Children(func() { ui.Icon(c, plus) })
			ui.ButtonBase(c.Key("close")).Label("Close").Children(func() { ui.Icon(c, plus) })
		})
	}, 300, 200)
	tt.Frame()
	all := tt.AccessAll()
	var buttons []string
	for _, a := range all {
		if a.Role == ui.RoleButton {
			buttons = append(buttons, a.Name)
		}
	}
	if !slices.Equal(buttons, []string{"Save", "", "Close"}) {
		t.Fatalf("buttons %q of %+v", buttons, all)
	}
	if save, _ := tt.AccessByRole(ui.RoleButton, "Save"); !slices.ContainsFunc(all, func(a ui.AccessInfo) bool { return reflect.DeepEqual(a, save) }) {
		t.Errorf("Save as Access reads it, %+v, missing", save)
	}
}

// TestTesterAccessTabPanel checks that the content of a tab is a tab
// panel, which its tab points to as the panel it controls.
func TestTesterAccessTabPanel(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Children(func() {
			tab := ui.ButtonBase(c.Key("tab")).Role(ui.RoleTab).Label("Settings").Children(func() { ui.Text(c, "Settings") })
			tab.Controls(ui.Column(c).Role(ui.RoleTabPanel).Label("Settings panel").Children(func() {
				ui.Text(c, "Volume")
			}))
		})
	}, 400, 400)
	if p := accessOf(t, tt, ui.RoleTabPanel, "Settings panel"); p.Role != ui.RoleTabPanel {
		t.Errorf("Settings panel: %+v", p)
	}
	if tab := accessOf(t, tt, ui.RoleTab, "Settings"); !slices.Equal(tab.Controls, []string{"Settings panel"}) {
		t.Errorf("Settings tab controls %q", tab.Controls)
	}
}

// TestTesterAccessLandmarks checks that the main content, the top bar,
// the footer, the sidebar and a form of a window are landmarks.
func TestTesterAccessLandmarks(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Children(func() {
			ui.Row(c).Role(ui.RoleBanner).Label("Mail").Children(func() { ui.Text(c, "Mail") })
			ui.Column(c).Role(ui.RoleComplementary).Label("Folders").Children(func() { ui.Text(c, "Inbox") })
			ui.Column(c).Role(ui.RoleMain).Label("Messages").Children(func() { ui.Text(c, "Hello") })
			ui.Row(c).Role(ui.RoleContentInfo).Label("Status").Children(func() { ui.Text(c, "3 unread") })
			ui.Column(c).Role(ui.RoleForm).Label("Sign in").Children(func() { ui.Text(c, "Password") })
		})
	}, 400, 400)
	for r, name := range map[ui.Role]string{ui.RoleBanner: "Mail", ui.RoleComplementary: "Folders", ui.RoleMain: "Messages", ui.RoleContentInfo: "Status", ui.RoleForm: "Sign in"} {
		if a := accessOf(t, tt, r, name); a.Role != r {
			t.Errorf("%s: %+v", name, a)
		}
	}
}

// TestTesterAccessBoundsClipped checks that AccessInfo.Bounds returns the
// clipped visible box, matching Find, also under content zoom.
func TestTesterAccessBoundsClipped(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Size(60, 25).Shrink(0).Clip().Children(func() {
			ui.ButtonBase(c).Label("Clipped").Size(120, 40)
		})
		ui.Scroll(c).Size(200, 80).Children(func() {
			ui.Box(c).Size(200, 200).Shrink(0)
			ui.ButtonBase(c).Label("Below").Size(100, 30)
		})
	}, 300, 200)

	clipped := accessOf(t, tt, ui.RoleButton, "Clipped")
	findClip, _ := tt.Find("Clipped")
	if clipped.Bounds != (ui.Rect{X: 0, Y: 0, W: 60, H: 25}) || clipped.Bounds != findClip {
		t.Errorf("Clipped Bounds = %v, Find = %v", clipped.Bounds, findClip)
	}

	below := accessOf(t, tt, ui.RoleButton, "Below")
	findBelow, _ := tt.Find("Below")
	if !below.Offscreen || below.Bounds.W != 0 || below.Bounds.H != 0 || below.Bounds != findBelow {
		t.Errorf("Below Bounds = %v, Offscreen = %v, Find = %v", below.Bounds, below.Offscreen, findBelow)
	}

	tt.SetZoom(2)
	zoomed := accessOf(t, tt, ui.RoleButton, "Clipped")
	findZoomed, _ := tt.Find("Clipped")
	if zoomed.Bounds != (ui.Rect{X: 0, Y: 0, W: 120, H: 50}) || zoomed.Bounds != findZoomed {
		t.Errorf("zoomed Bounds = %v, Find = %v", zoomed.Bounds, findZoomed)
	}
}

// TestTesterAccessBoundsClippedChildren checks that nodes without element
// state (AccessChildren virtual nodes) and inline links inside texts are
// clipped to their owning containers and scroll viewports, also under zoom.
func TestTesterAccessBoundsClippedChildren(t *testing.T) {
	t.Run("AccessChildrenClipAndBorder", func(t *testing.T) {
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Box(c).Size(60, 25).Shrink(0).Border(2, ui.RGB(0, 0, 0)).Clip().
				Role(ui.RoleGroup).Label("Host").
				AccessChildren(func(parent *platform.AccessNode, add func(platform.AccessNode)) {
					add(platform.AccessNode{
						ID:     parent.ID ^ 1,
						Role:   platform.RoleText,
						Label:  "VirtualClipped",
						Bounds: platform.RectF{X: parent.Bounds.X, Y: parent.Bounds.Y, W: 120, H: 40},
					})
				})
		}, 300, 200)

		got := accessOf(t, tt, ui.RoleText, "VirtualClipped")
		if got.Bounds != (ui.Rect{X: 2, Y: 2, W: 56, H: 21}) {
			t.Errorf("VirtualClipped Bounds = %v, want {2 2 56 21}", got.Bounds)
		}

		tt.SetZoom(2)
		zoomed := accessOf(t, tt, ui.RoleText, "VirtualClipped")
		if zoomed.Bounds != (ui.Rect{X: 4, Y: 4, W: 112, H: 42}) {
			t.Errorf("zoomed VirtualClipped Bounds = %v, want {4 4 112 42}", zoomed.Bounds)
		}
	})

	t.Run("AccessChildrenClipX", func(t *testing.T) {
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Box(c).Size(60, 25).Shrink(0).ClipX().
				Role(ui.RoleGroup).Label("Host").
				AccessChildren(func(parent *platform.AccessNode, add func(platform.AccessNode)) {
					add(platform.AccessNode{
						ID:     parent.ID ^ 1,
						Role:   platform.RoleText,
						Label:  "VirtualClipX",
						Bounds: platform.RectF{X: parent.Bounds.X, Y: parent.Bounds.Y, W: 120, H: 40},
					})
				})
		}, 300, 200)

		got := accessOf(t, tt, ui.RoleText, "VirtualClipX")
		if got.Bounds != (ui.Rect{X: 0, Y: 0, W: 60, H: 40}) {
			t.Errorf("VirtualClipX Bounds = %v, want {0 0 60 40}", got.Bounds)
		}
	})

	t.Run("AccessChildrenScroll", func(t *testing.T) {
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Scroll(c).Size(200, 80).Children(func() {
				ui.Box(c).Size(200, 240).Shrink(0).
					Role(ui.RoleGroup).Label("ScrolledHost").
					AccessChildren(func(parent *platform.AccessNode, add func(platform.AccessNode)) {
						add(platform.AccessNode{
							ID:     parent.ID ^ 1,
							Role:   platform.RoleText,
							Label:  "VirtualPart",
							Bounds: platform.RectF{X: parent.Bounds.X, Y: parent.Bounds.Y + 60, W: 100, H: 40},
						})
						add(platform.AccessNode{
							ID:     parent.ID ^ 2,
							Role:   platform.RoleText,
							Label:  "VirtualOffscreen",
							Bounds: platform.RectF{X: parent.Bounds.X, Y: parent.Bounds.Y + 180, W: 100, H: 30},
						})
					})
			})
		}, 300, 200)

		part := accessOf(t, tt, ui.RoleText, "VirtualPart")
		if part.Bounds != (ui.Rect{X: 0, Y: 60, W: 100, H: 20}) {
			t.Errorf("VirtualPart Bounds = %v, want {0 60 100 20}", part.Bounds)
		}
		off := accessOf(t, tt, ui.RoleText, "VirtualOffscreen")
		if off.Bounds.W != 0 || off.Bounds.H != 0 {
			t.Errorf("VirtualOffscreen Bounds = %v, want 0 size", off.Bounds)
		}

		tt.Scroll(100, 40, 0, 160)
		shown := accessOf(t, tt, ui.RoleText, "VirtualOffscreen")
		if shown.Bounds != (ui.Rect{X: 0, Y: 20, W: 100, H: 30}) {
			t.Errorf("scrolled VirtualOffscreen Bounds = %v, want {0 20 100 30}", shown.Bounds)
		}
	})

	t.Run("InlineLink", func(t *testing.T) {
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Box(c).Size(40, 25).Shrink(0).Clip().Children(func() {
				ui.RichText(c).NoWrap().Children(func() {
					ui.Link(c, "InlineClippedLink", "https://example.com")
				})
			})
			ui.Scroll(c).Size(200, 80).Children(func() {
				ui.Box(c).Size(200, 200).Shrink(0)
				ui.RichText(c).Children(func() {
					ui.Link(c, "InlineBelowLink", "https://example.com")
				})
			})
		}, 300, 200)

		clipped := accessOf(t, tt, ui.RoleLink, "InlineClippedLink")
		if clipped.Bounds.W != 40 || clipped.Bounds.H <= 0 || clipped.Bounds.H > 25 {
			t.Errorf("InlineClippedLink Bounds = %v, want W=40 and 0<H<=25", clipped.Bounds)
		}
		below := accessOf(t, tt, ui.RoleLink, "InlineBelowLink")
		if !below.Offscreen || below.Bounds.W != 0 || below.Bounds.H != 0 {
			t.Errorf("InlineBelowLink Bounds = %v, Offscreen = %v", below.Bounds, below.Offscreen)
		}
	})
}
