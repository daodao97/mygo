//go:build windows && (amd64 || arm64)

package windows

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// accessTree returns a tree of a list of rows, each with a text, as the
// content describes a frame: rows are the IDs of the rows in order.
func accessTree(rows ...uint64) *platform.AccessTree {
	t := &platform.AccessTree{Nodes: []platform.AccessNode{{ID: 1, Parent: -1, Role: platform.RoleList}}}
	for _, id := range rows {
		t.Nodes = append(t.Nodes, platform.AccessNode{ID: id, Parent: 0, Role: platform.RoleListItem, Label: "row"})
		t.Nodes = append(t.Nodes, platform.AccessNode{ID: id << 8, Parent: len(t.Nodes) - 1, Role: platform.RoleText, Label: "text"})
	}
	return t
}

// newTestTree returns a tree as accessRoot makes it, without a window.
func newTestTree(t testing.TB, clients bool) *uiaTree {
	uiaOnce.Do(initUIA)
	saved := listening
	listening = func() bool { return clients }
	t.Cleanup(func() { listening = saved })
	tree := &uiaTree{nodes: map[uint64]*uiaElement{}}
	tree.root = newUIAElement(tree)
	tree.root.root = true
	return tree
}

func ids(es []*uiaElement) []uint64 {
	var out []uint64
	for _, e := range es {
		out = append(out, e.n.ID)
	}
	return out
}

// The elements of the tree follow the content's trees as it reuses the
// buffers of the last ones: rows coming, going and moving.
func TestUIATreeUpdate(t *testing.T) {
	tree := newTestTree(t, false)
	frames := [][]uint64{{2, 3, 4}, {2, 3, 4}, {3, 4, 5, 6}, {6, 3}, {6, 3}, {}, {7, 2}}
	var list *uiaElement
	for i, rows := range frames {
		tree.update(accessTree(rows...))
		if got := ids(tree.root.children); !slices.Equal(got, []uint64{1}) {
			t.Fatalf("frame %d: the root's children %v", i, got)
		}
		l := tree.nodes[1]
		if i > 0 && l != list {
			t.Fatalf("frame %d: the list is a new element", i)
		}
		list = l
		if got := ids(l.children); !slices.Equal(got, rows) {
			t.Fatalf("frame %d: the list's children %v, want %v", i, got, rows)
		}
		for _, row := range l.children {
			if row.parent != l || len(row.children) != 1 || row.children[0].n.ID != row.n.ID<<8 || row.children[0].parent != row {
				t.Fatalf("frame %d: row %d's links", i, row.n.ID)
			}
		}
		if len(tree.nodes) != 1+2*len(rows) || len(tree.order) != len(tree.nodes) {
			t.Fatalf("frame %d: %d nodes, %d in order", i, len(tree.nodes), len(tree.order))
		}
	}
	// Rows that left were let go of: none of the spare buffers holds them.
	for _, e := range tree.spareOrder[:cap(tree.spareOrder)] {
		if e != nil {
			t.Fatal("the spare order keeps an element")
		}
	}
	if len(tree.spareNodes) != 0 {
		t.Fatal("the spare map keeps elements")
	}
}

// The nodes with the announcer are let go of once the elements have them,
// as the strings of a tree that shrank would be kept past its length.
func TestUIATreeAnnouncer(t *testing.T) {
	tree := newTestTree(t, false)
	frame := accessTree(2, 3, 4)
	frame.Announcements = []string{"saved"}
	tree.update(frame)
	tree.update(accessTree(2))
	if e := tree.nodes[announcerID]; e == nil || e.n.Label != "saved" {
		t.Fatal("the announcer left the tree")
	}
	if got := ids(tree.root.children); !slices.Equal(got, []uint64{1, announcerID}) {
		t.Fatalf("the root's children %v", got)
	}
	// Alerts go into an assertive region of their own, after it.
	frame = accessTree(2)
	frame.Alerts = []string{"Name is required", "Too short"}
	tree.update(frame)
	tree.update(accessTree(2))
	e := tree.nodes[alerterID]
	if e == nil || e.n.Label != "Name is required Too short" {
		t.Fatal("the alerts' region left the tree")
	}
	var live variant
	if e.property(uiaLiveSettingProperty, &live); live.Val != 2 {
		t.Errorf("the alerts' live setting %d, want assertive", live.Val)
	}
	if got := ids(tree.root.children); !slices.Equal(got, []uint64{1, announcerID, alerterID}) {
		t.Fatalf("the root's children %v", got)
	}
	for _, n := range tree.withAnnouncer[:cap(tree.withAnnouncer)] {
		if n.Label != "" {
			t.Fatalf("the nodes with the announcer keep %q", n.Label)
		}
	}
	if tree.announcerTree.Nodes != nil || tree.announcerTree.Announcements != nil || tree.announcerTree.Alerts != nil {
		t.Fatal("the tree with the announcer is kept")
	}
}

// Landmarks are groups whose LandmarkType says which, and whose AriaRole
// names them: the navigation and search landmarks have types of their
// own, a region the custom one.
func TestUIALandmarks(t *testing.T) {
	for role, want := range map[platform.AccessRole]uint64{
		platform.RoleNavigation: 80003, // UIA_NavigationLandmarkTypeId
		platform.RoleSearch:     80004, // UIA_SearchLandmarkTypeId
		platform.RoleRegion:     80000, // UIA_CustomLandmarkTypeId
	} {
		e := &uiaElement{n: platform.AccessNode{Role: role, Label: "Landmark"}}
		var v variant
		if e.property(uiaLandmarkTypeProperty, &v); v.VT != vtI4 || v.Val != want {
			t.Errorf("role %d: landmark type %d, want %d", role, v.Val, want)
		}
		if uiaControlTypes[role] != 50026 || ariaRoles[role] == "" {
			t.Errorf("role %d: control type %d, ARIA role %q", role, uiaControlTypes[role], ariaRoles[role])
		}
	}
}

// An article is a group that says it is one, but no landmark.
func TestUIAArticle(t *testing.T) {
	e := &uiaElement{n: platform.AccessNode{Role: platform.RoleArticle, Label: "Post by Ann"}}
	var v variant
	if e.property(uiaControlTypeProperty, &v); v.Val != 50026 {
		t.Errorf("control type %d, want Group", v.Val)
	}
	for _, id := range []int{uiaLocalizedControlProperty, uiaAriaRoleProperty} {
		v = variant{}
		if e.property(id, &v); v.VT != vtBSTR || bstrString(uintptr(v.Val)) != "article" {
			t.Errorf("property %d %+v, want \"article\"", id, v)
		}
	}
	v = variant{}
	if e.property(uiaLandmarkTypeProperty, &v); v.VT != vtEmpty {
		t.Errorf("an article's landmark type %+v, want none", v)
	}
}

func TestChildChanges(t *testing.T) {
	e := func(id uint64) *uiaElement { return &uiaElement{n: platform.AccessNode{ID: id}} }
	a, b, c, d := e(1), e(2), e(3), e(4)
	added, removed := childChanges([]*uiaElement{a, b, c}, []*uiaElement{b, d, a}, nil, nil)
	if !slices.Equal(ids(added), []uint64{4}) || !slices.Equal(ids(removed), []uint64{3}) {
		t.Errorf("added %v, removed %v", ids(added), ids(removed))
	}
	added, removed = childChanges([]*uiaElement{a}, []*uiaElement{a}, added, removed)
	if len(added) != 0 || len(removed) != 0 {
		t.Errorf("unchanged children: added %v, removed %v", ids(added), ids(removed))
	}
}

// A frame like the last allocates nothing, whether clients listen or not:
// the content describes every frame once one asked for the window.
func TestUIATreeUpdateAllocations(t *testing.T) {
	for _, clients := range []bool{false, true} {
		tree := newTestTree(t, clients)
		frame := accessTree(2, 3, 4, 5, 6, 7, 8, 9)
		// The first tree, as a client asks for it, raises no events: they
		// would reach UI Automation, which has no window here.
		tree.quiet = true
		tree.update(frame)
		tree.quiet = false
		tree.update(frame)
		if n := testing.AllocsPerRun(20, func() { tree.update(frame) }); n != 0 {
			t.Errorf("clients listening %v: %v allocations a frame", clients, n)
		}
	}
}

func BenchmarkUIATreeUpdate(b *testing.B) {
	rows := make([]uint64, 400)
	for i := range rows {
		rows[i] = uint64(i + 2)
	}
	frame := accessTree(rows...)
	for _, clients := range []bool{false, true} {
		name := "quiet"
		if clients {
			name = "listening"
		}
		b.Run(name, func(b *testing.B) {
			tree := newTestTree(b, clients)
			tree.quiet = true
			tree.update(frame)
			tree.quiet = false
			b.ReportAllocs()
			for b.Loop() {
				tree.update(frame)
			}
		})
	}
}

// A range with a text for its value (aria-valuetext) gives it through the
// Value pattern beside RangeValue; one without has RangeValue alone.
func TestUIARangeValueText(t *testing.T) {
	for _, c := range []struct {
		value string
		want  bool
	}{{"30 percent", true}, {"", false}} {
		e := &uiaElement{n: platform.AccessNode{Role: platform.RoleSlider, Max: 100, Now: 30, Value: c.value}}
		if !e.supports(ifaceRangeValue) {
			t.Errorf("value %q: no RangeValue", c.value)
		}
		if got := e.supports(ifaceValue); got != c.want {
			t.Errorf("value %q: Value pattern %v, want %v", c.value, got, c.want)
		}
	}
}

// The ARIA states of #1181 as UI Automation reads them: a role
// description in LocalizedControlType, a popup in ExpandCollapse and
// AriaProperties, a sort in ItemStatus, what a button controls in
// ControllerFor, and a cell chosen in its table's Selection.
func TestUIAStates(t *testing.T) {
	tree := newTestTree(t, false)
	tree.update(&platform.AccessTree{Nodes: []platform.AccessNode{
		{ID: 1, Parent: -1, Role: platform.RoleGroup, Label: "Highlights", RoleDescription: "carousel"},
		{ID: 2, Parent: -1, Role: platform.RoleButton, Label: "Fruit", HasPopup: platform.PopupListBox, Controls: []uint64{3, 99}},
		{ID: 3, Parent: -1, Role: platform.RoleTable, Label: "Days", States: platform.AccessSelectable},
		{ID: 4, Parent: 2, Role: platform.RoleRow},
		{ID: 5, Parent: 3, Role: platform.RoleColumnHeader, Label: "Day", States: platform.AccessSortOther},
		{ID: 6, Parent: 3, Role: platform.RoleCell, Label: "2", States: platform.AccessSelectable | platform.AccessChecked},
	}})
	text := func(id uint64, property int) string {
		var v variant
		tree.nodes[id].property(property, &v)
		if v.VT != vtBSTR {
			return ""
		}
		return bstrString(uintptr(v.Val))
	}
	if got := text(1, uiaLocalizedControlProperty); got != "carousel" {
		t.Errorf("the carousel's control type %q", got)
	}
	if got := text(2, uiaAriaPropertiesProperty); got != "haspopup=listbox" {
		t.Errorf("the button's ARIA properties %q", got)
	}
	if !tree.nodes[2].supports(ifaceExpandCollapse) {
		t.Error("the button opening a list box does not expand")
	}
	if got := text(5, uiaItemStatusProperty); got != "other" {
		t.Errorf("the header's item status %q", got)
	}
	var v variant
	if tree.nodes[2].property(uiaControllerForProperty, &v); v.VT != vtArray|vtUnknown || v.Val == 0 {
		t.Errorf("the button controls %+v", v)
	}
	if day := tree.nodes[6]; !day.supports(ifaceSelectionItem) || day.container() != tree.nodes[3] {
		t.Error("the day is not chosen in its table")
	}
	if chosen := tree.nodes[3].chosen(); len(chosen) != 1 || chosen[0].n.ID != 6 {
		t.Errorf("the table chose %v", ids(chosen))
	}
}
