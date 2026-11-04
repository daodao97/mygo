package ui_test

import (
	"testing"

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
			ui.Collapsible(c, "More", &open, func() { ui.Text(c, "Hidden") })
			ui.Button(c, "Save").Disabled(true)
			ui.Box(c).Size(24, 24).Label("Error").Description("Disk full")
		})
	}, 400, 600)

	reply := accessOf(t, tt, ui.RoleStatus, "Reply")
	if !reply.Busy || reply.Role != ui.RoleStatus {
		t.Errorf("Reply: %+v", reply)
	}
	if a, ok := tt.Access("Reply"); !ok || a != reply {
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
