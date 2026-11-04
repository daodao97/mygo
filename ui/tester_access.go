package ui

import "github.com/egoist/mygo/internal/platform"

// AccessInfo is what assistive technology sees of an element of the last
// frame (Tester.Access): its role, name, value and states, as the
// accessibility tree gives them to screen readers.
type AccessInfo struct {
	// Role is the element's role, as Role set it or as its widget or what
	// it does gave it: never RoleAuto or RoleNone.
	Role Role
	// Name is the accessible name: the Label, or the text inside the
	// element. Value is its value, as a text field's text, the choice a
	// pop-up button shows, or what a slider reads (aria-valuetext).
	// Description is read after the name, as a tooltip or a field's
	// error, and Placeholder is what an empty text field shows.
	Name, Value, Description, Placeholder string
	// Bounds is the element's visible box, in the window's DIPs as Find's.
	Bounds Rect
	// Checked is set on a check box, switch, radio button or menu item
	// that is on, and on a toggle button pressed (aria-checked,
	// aria-pressed); Mixed in its place on one partly on.
	Checked, Mixed bool
	// Expanded is set while what the element opens shows; Expandable on
	// an item of a tree with children.
	Expanded, Expandable bool
	// Selected is set on the tab chosen, and on the items of a tree and
	// the rows of a list or a table chosen (aria-selected), which are
	// Selectable where their list chooses them; Multiselectable is set on
	// a list or a table choosing several.
	Selected, Selectable, Multiselectable bool
	// Busy is set while the element's content is about to change
	// (aria-busy).
	Busy bool
	// Disabled is set on an element that takes no input, Invalid on one
	// whose value is not valid, as a field with an error.
	Disabled, Invalid bool
	// Focusable is set on an element that takes the keyboard focus, and
	// Focused on the one assistive technology has the focus on: the
	// element with it, or the option the arrows are on in a list or a
	// combobox that keeps it (ActiveDescendant).
	Focusable, Focused bool
	// ReadOnly, Multiline, Password and Search tell of a text field.
	ReadOnly, Multiline, Password, Search bool
	// SortAscending and SortDescending are set on the header of the
	// column a table's rows are sorted by.
	SortAscending, SortDescending bool
	// Vertical is set on a slider going up, and Segment on a segment of
	// a segmented control or a group of toggles.
	Vertical, Segment bool
	// Offscreen is set on an element built out of view, as the rows a
	// list builds beyond its edges.
	Offscreen bool
	// Current is the item the element is the current one of
	// (aria-current), CurrentNone for none.
	Current Current
	// Level is the rank of a heading, or how deep an item of a tree is,
	// from 1; 0 for other elements.
	Level int
	// PosInSet is the place of an item in its set, from 1, and SetSize
	// the size of the set, as a list's rows and the list have them; 0
	// when not given.
	PosInSet, SetSize int
	// Min, Max and Now are the range and the value of a slider, progress
	// bar, meter or stepper (aria-valuemin, -valuemax and -valuenow), Now
	// below Min for progress of unknown length; Step is how far the keys
	// move the value, 0 for none.
	Min, Max, Now, Step float64
}

// Access returns what assistive technology sees of the first element of
// the last frame, in the order screen readers read them, whose accessible
// name is name: its Label, or the text it shows. The tree is the one
// screen readers would get, whether or not one reads the window, and
// reading it changes nothing.
func (t *Tester) Access(name string) (AccessInfo, bool) {
	return t.access(func(_ Role, n *platform.AccessNode) bool { return n.Label == name })
}

// AccessByRole returns what assistive technology sees of the first
// element of the last frame with role r and the accessible name name, as
// Testing Library's getByRole(role, {name}) finds it; "" for name finds
// the first element of the role whatever its name.
func (t *Tester) AccessByRole(r Role, name string) (AccessInfo, bool) {
	return t.access(func(role Role, n *platform.AccessNode) bool {
		return role == r && (name == "" || n.Label == name)
	})
}

// access describes the last frame as it would for assistive technology,
// and returns the first node that match takes, given its role.
func (t *Tester) access(match func(r Role, n *platform.AccessNode) bool) (AccessInfo, bool) {
	// Reading the window's focus would have its next change rebuild the
	// frame.
	defer func(read bool) { t.rt.focusRead = read }(t.rt.focusRead)
	tree := t.rt.windowAccess(t.rt.accessTree())
	// The roles elements were given, by their IDs: several may share a
	// node's role, as a list box and a list do.
	given := map[uint64]Role{}
	var walk func(e *node)
	walk = func(e *node) {
		if e.role > RoleNone {
			given[e.id] = e.role
		}
		for ch := e.first; ch != nil; ch = ch.next {
			walk(ch)
		}
	}
	if root := t.rt.c.root; root != nil {
		walk(root)
	}
	for i := range tree.Nodes {
		n := &tree.Nodes[i]
		r, ok := given[n.ID]
		if !ok {
			// One the role of what it does: a text, a button, a group…,
			// which follow RoleGroup in the order of the node's roles
			// (accessRole).
			r = Role(n.Role) + RoleGroup
		}
		if match(r, n) {
			return accessInfoOf(r, n, tree.Focus), true
		}
	}
	return AccessInfo{}, false
}

// accessInfoOf returns what n tells of an element of role r, in the tree
// whose focus is focus.
func accessInfoOf(r Role, n *platform.AccessNode, focus uint64) AccessInfo {
	s, b := n.States, n.Bounds
	// The choice among tabs, a tree's items and the rows of a list is
	// AccessChecked, which screen readers read as selected.
	chosen := false
	switch n.Role {
	case platform.RoleTab, platform.RoleTreeItem, platform.RoleRow, platform.RoleListItem:
		chosen = s&platform.AccessChecked != 0
	}
	return AccessInfo{
		Role:            r,
		Name:            n.Label,
		Value:           n.Value,
		Description:     n.Description,
		Placeholder:     n.Placeholder,
		Bounds:          Rect{float32(b.X), float32(b.Y), float32(b.W), float32(b.H)},
		Checked:         s&platform.AccessChecked != 0 && !chosen,
		Mixed:           s&platform.AccessMixed != 0,
		Expanded:        s&platform.AccessExpanded != 0,
		Expandable:      s&platform.AccessExpandable != 0,
		Selected:        s&platform.AccessSelected != 0 || chosen,
		Selectable:      s&platform.AccessSelectable != 0,
		Multiselectable: s&platform.AccessMultiselectable != 0,
		Busy:            s&platform.AccessBusy != 0,
		Disabled:        s&platform.AccessDisabled != 0,
		Invalid:         s&platform.AccessInvalid != 0,
		Focusable:       s&platform.AccessFocusable != 0,
		Focused:         focus != 0 && n.ID == focus,
		ReadOnly:        s&platform.AccessReadOnly != 0,
		Multiline:       s&platform.AccessMultiline != 0,
		Password:        s&platform.AccessPassword != 0,
		Search:          s&platform.AccessSearch != 0,
		SortAscending:   s&platform.AccessSortAscending != 0,
		SortDescending:  s&platform.AccessSortDescending != 0,
		Vertical:        s&platform.AccessVertical != 0,
		Segment:         s&platform.AccessSegment != 0,
		Offscreen:       s&platform.AccessOffscreen != 0,
		Current:         Current(n.Current),
		Level:           n.Level,
		PosInSet:        n.PosInSet,
		SetSize:         n.SetSize,
		Min:             n.Min,
		Max:             n.Max,
		Now:             n.Now,
		Step:            n.Step,
	}
}
