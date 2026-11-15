//go:build darwin

package darwin

import (
	"encoding/base64"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// titleTab returns the x the badge of item starts at, zero when it has none.
func titleTab(item id) float64 {
	return attributedTab(send(item, "attributedTitle"))
}

// attributedTab returns the right-aligned tab stop of a title, zero when it
// has none.
func attributedTab(title id) float64 {
	if title == 0 {
		return 0
	}
	style := send(title, "attribute:atIndex:effectiveRange:", uintptr(nsString("NSParagraphStyle")), 0, 0)
	if style == 0 {
		return 0
	}
	tabs := arrayItems(send(style, "tabStops"))
	if len(tabs) == 0 {
		return 0
	}
	return msgFloat(tabs[0], sel("location"))
}

// TestMenuBadgeTitle checks that a badge takes an attributed title whose
// tab stop sits in the column the menu's badges share, aligned as the item
// says, and that clearing the badge restores the plain title.
func TestMenuBadgeTitle(t *testing.T) {
	load()
	withPool(func() {
		longest := "A longer node name without a badge"
		start, width := badgeColumn(&platform.Menu{Items: []*platform.MenuItem{
			{Label: "Node", Badge: "120 ms", Visible: true},
			{Label: "A longer node name", Badge: "1.2s", Visible: true},
			{Label: longest, Visible: true},
		}})
		if start <= textWidth(longest, menuFont()) {
			t.Fatalf("column start %v does not clear the longest label", start)
		}

		item := autorelease(send(send(class("NSMenuItem"), "alloc"), "initWithTitle:action:keyEquivalent:",
			uintptr(nsString("Node")), 0, uintptr(nsString(""))))
		for _, a := range []struct {
			align platform.BadgeAlignment
			tab   float64
			want  int
		}{
			{platform.BadgeAlignLeft, start, nsTextAlignmentLeft},
			{platform.BadgeAlignCenter, start + width/2, nsTextAlignmentCenter},
			{platform.BadgeAlignRight, start + width, nsTextAlignmentRight},
		} {
			setItemTitle(item, "Node", "120 ms", a.tab, a.align)
			title := send(item, "attributedTitle")
			if title == 0 || goString(send(title, "string")) != "Node\t120 ms" {
				t.Fatalf("%s attributed title missing: %v", a.align, title)
			}
			if got := titleTab(item); got != a.tab {
				t.Errorf("%s tab stop at %v, want %v", a.align, got, a.tab)
			}
			style := send(title, "attribute:atIndex:effectiveRange:", uintptr(nsString("NSParagraphStyle")), 0, 0)
			if tabs := arrayItems(send(style, "tabStops")); len(tabs) != 1 || sendInt(tabs[0], "alignment") != a.want {
				t.Errorf("%s tab stop has alignment %d, want %d", a.align, sendInt(tabs[0], "alignment"), a.want)
			}
		}

		setItemTitle(item, "Node", "", 0, platform.BadgeAlignRight)
		if got := goString(send(item, "title")); got != "Node" {
			t.Errorf("title after clearing = %q", got)
		}
		if at := send(item, "attributedTitle"); at != 0 && goString(send(at, "string")) != "Node" {
			t.Errorf("attributed title after clearing = %q", goString(send(at, "string")))
		}
	})
}

// TestMenuBadgeIconColumn checks that an item with an icon puts its badge
// in the same column as the items without one.
func TestMenuBadgeIconColumn(t *testing.T) {
	load()
	LinkMenuIcons()
	defer func() { menuImage = nil }()
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	withPool(func() {
		b := &Backend{}
		m := &platform.Menu{Items: []*platform.MenuItem{
			{ID: 1, Label: "Node", Badge: "45 ms", Visible: true, Enabled: true},
			{ID: 2, Label: "Node", Badge: "45 ms", Visible: true, Enabled: true, Icon: &platform.MenuIcon{PNG: png}},
		}}
		owner := newOwner()
		menu := b.buildMenu(m, "", owner)
		defer dropOwner(owner)
		plain := send(menu, "itemAtIndex:", 0)
		icon := send(menu, "itemAtIndex:", 1)
		if got, want := titleTab(icon), titleTab(plain); got != want {
			t.Errorf("icon item tab = %v, want %v", got, want)
		}
	})
}

// TestMenuBadgeRetab checks that a badge set, a label made wider, or an
// item hidden after the menu was built moves the tab stop the menu's badges
// start at: the menu no longer keeps the column it was built with.
func TestMenuBadgeRetab(t *testing.T) {
	load()
	registerMenuTarget()
	withPool(func() {
		b := &Backend{}
		m := &platform.Menu{Items: []*platform.MenuItem{
			{ID: 1, Label: "Node", Visible: true, Enabled: true},
			{ID: 2, Label: "A longer node name", Visible: true, Enabled: true},
		}}
		owner := newOwner()
		menu := b.buildMenu(m, "", owner)
		defer dropOwner(owner)
		node := send(menu, "itemAtIndex:", 0)
		long := send(menu, "itemAtIndex:", 1)

		if tab := titleTab(node); tab != 0 {
			t.Fatalf("badgeless item has a tab stop at %v", tab)
		}

		// A badge given after the menu was built gets the column now.
		b.UpdateMenuItem(&platform.MenuItem{ID: 1, Label: "Node", Badge: "120 ms", Visible: true, Enabled: true})
		tab := titleTab(node)
		if tab <= textWidth("Node", menuFont()) {
			t.Fatalf("badge tab %v does not clear the label", tab)
		}
		if got := goString(send(send(node, "attributedTitle"), "string")); got != "Node\t120 ms" {
			t.Fatalf("node title = %q", got)
		}

		// A label made wider moves the badges of the menu along.
		wider := "A node name much longer than the one before"
		b.UpdateMenuItem(&platform.MenuItem{ID: 2, Label: wider, Visible: true, Enabled: true})
		if got := titleTab(node); got <= tab || got <= textWidth(wider, menuFont()) {
			t.Errorf("badge tab after a wider label = %v, was %v", got, tab)
		}
		tab = titleTab(node)

		// Hiding the longest item brings them back.
		b.UpdateMenuItem(&platform.MenuItem{ID: 2, Label: wider, Visible: false, Enabled: true})
		if got := titleTab(node); got >= tab {
			t.Errorf("badge tab after hiding the longest item = %v, was %v", got, tab)
		}
		if !sendBool(long, "isHidden") {
			t.Error("hidden item is shown")
		}

		// Clearing the last badge restores the plain titles.
		b.UpdateMenuItem(&platform.MenuItem{ID: 1, Label: "Node", Visible: true, Enabled: true})
		if at := send(node, "attributedTitle"); at != 0 && goString(send(at, "string")) != "Node" {
			t.Errorf("attributed title after clearing = %q", goString(send(at, "string")))
		}
		if got := goString(send(node, "title")); got != "Node" {
			t.Errorf("title after clearing = %q", got)
		}
	})
}

// TestMenuBadgeSubtitle checks that the tab stop clears the subtitle where
// the item shows it after the title, and the title alone where the subtitle
// has a line of its own.
func TestMenuBadgeSubtitle(t *testing.T) {
	load()
	withPool(func() {
		itString := (&platform.MenuItem{Label: "Node", Subtitle: "Hong Kong", Badge: "120 ms"}).LabelWithSubtitle()
		start, _ := badgeColumn(&platform.Menu{Items: []*platform.MenuItem{
			{Label: "Node", Subtitle: "Hong Kong", Badge: "120 ms", Visible: true},
		}})
		want := "Node"
		if !subtitleOnLine() {
			want = itString
		}
		if base := textWidth(want, menuFont()); start <= base {
			t.Errorf("column start %v does not clear %q", start, want)
		}
	})
}

func TestTextAlignments(t *testing.T) {
	for _, c := range []struct {
		arch          string
		center, right int
	}{
		{"arm64", 1, 2},
		{"amd64", 2, 1},
	} {
		if center, right := textAlignments(c.arch); center != c.center || right != c.right {
			t.Errorf("%s: center %d, right %d, want %d, %d", c.arch, center, right, c.center, c.right)
		}
	}
}
