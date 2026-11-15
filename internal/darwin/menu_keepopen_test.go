//go:build darwin

package darwin

import (
	"encoding/base64"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// TestMenuKeepOpenView checks the view MyGo draws for an item that keeps
// its menu open: its label, badge, check mark and the update path.
func TestMenuKeepOpenView(t *testing.T) {
	load()
	registerMenuTarget()
	withPool(func() {
		b := &Backend{}
		item := &platform.MenuItem{
			ID: 42, Label: "Node", Badge: "45 ms", KeepOpen: true,
			Visible: true, Enabled: true, Type: platform.MenuItemCheckbox, Checked: true,
		}
		m := &platform.Menu{Items: []*platform.MenuItem{
			item,
			{Label: "Other", Visible: true, Enabled: true, KeepOpen: true},
		}}
		menu := b.buildMenu(m, "", 1)
		defer dropOwner(1)

		native := send(menu, "itemAtIndex:", 0)
		view := send(native, "view")
		if view == 0 {
			t.Fatal("KeepOpen item has no view")
		}
		if got := goString(send(view, "identifier")); got != "42" {
			t.Errorf("view identifier = %q", got)
		}
		// The menu is at least as wide as its views, whose right the titles
		// do not account for.
		viewWidth := msgRect(view, sel("frame")).Size.Width
		if got := msgFloat(menu, sel("minimumWidth")); got < viewWidth {
			t.Errorf("menu minimum width = %v, view width = %v", got, viewWidth)
		}
		check := send(view, "viewWithTag:", menuViewCheck)
		if check == 0 || sendBool(check, "isHidden") {
			t.Fatal("checked item has no check mark")
		}
		label := send(view, "viewWithTag:", menuViewLabel)
		if label == 0 {
			t.Fatal("KeepOpen view has no label")
		}
		if got := goString(send(label, "stringValue")); got != "Node\t45 ms" {
			t.Errorf("label = %q", got)
		}
		backdrop := menuBackdrop(view)
		if backdrop == 0 || !sendBool(backdrop, "isHidden") {
			t.Fatal("KeepOpen view should start without a highlight")
		}
		// NSMenu highlights items itself, keyboard included: its delegate
		// follows the highlight in the views.
		delegate := send(menu, "delegate")
		if delegate == 0 {
			t.Fatal("menu with KeepOpen views has no delegate")
		}
		send(delegate, "menu:willHighlightItem:", uintptr(menu), uintptr(native))
		if sendBool(backdrop, "isHidden") {
			t.Error("delegate did not highlight the view")
		}
		send(delegate, "menu:willHighlightItem:", uintptr(menu), 0)
		if !sendBool(backdrop, "isHidden") {
			t.Error("delegate did not clear the highlight")
		}
		setMenuViewHighlighted(view, true)
		if sendBool(backdrop, "isHidden") {
			t.Error("highlight not shown")
		}
		setMenuViewHighlighted(view, false)
		if !sendBool(backdrop, "isHidden") {
			t.Error("highlight not hidden")
		}

		// The view answers the keyboard and VoiceOver as an item does.
		if !sendBool(view, "acceptsFirstResponder") || !sendBool(view, "canBecomeKeyView") {
			t.Error("view cannot take the keyboard")
		}
		if !sendBool(view, "isAccessibilityElement") {
			t.Error("view is not an accessibility element")
		}
		if got := goString(send(view, "accessibilityRole")); got != "AXMenuItem" {
			t.Errorf("accessibility role = %q", got)
		}
		if got := goString(send(view, "accessibilityLabel")); got != "Node" {
			t.Errorf("accessibility label = %q", got)
		}
		if got := goString(send(view, "accessibilityValue")); got != "45 ms" {
			t.Errorf("accessibility value = %q", got)
		}
		if !sendBool(view, "accessibilityEnabled") {
			t.Error("view is not enabled")
		}
		if got := goString(send(view, "accessibilityMenuItemMarkChar")); got != "✓" {
			t.Errorf("accessibility mark of a checked item = %q", got)
		}
		if names := arrayItems(send(view, "accessibilityActionNames")); len(names) != 1 || goString(names[0]) != "AXPress" {
			t.Errorf("accessibility actions = %v", names)
		}
		for _, method := range []string{"accessibilityPerformPress", "keyDown:", "mouseUp:"} {
			if !respondsTo(view, method) {
				t.Errorf("view does not answer %s", method)
			}
		}
		// Arrow keys are taken by the view while the menu is up.
		down := msgKeyEvent(class("NSEvent"), sel("keyEventWithType:location:modifierFlags:timestamp:windowNumber:context:characters:charactersIgnoringModifiers:isARepeat:keyCode:"),
			10 /* keyDown */, NSPoint{}, 0, 0, 0, 0, nsString(""), nsString(""), false, 125)
		send(view, "keyDown:", uintptr(down))

		// A wider label or badge of any of the menu's view items lays its
		// views out again.
		width := msgRect(view, sel("frame")).Size.Width
		tab := attributedTab(send(label, "attributedStringValue"))
		b.UpdateMenuItem(&platform.MenuItem{ID: 0, Label: "A node name much longer than the one before, wide enough to widen the views", Visible: true, Enabled: true, KeepOpen: true})
		if got := msgRect(view, sel("frame")).Size.Width; got <= width {
			t.Errorf("view width after a wider label = %v, was %v", got, width)
		}
		if got := msgFloat(menu, sel("minimumWidth")); got < msgRect(view, sel("frame")).Size.Width {
			t.Errorf("menu minimum width after a wider label = %v, view width = %v", got, msgRect(view, sel("frame")).Size.Width)
		}
		if got := attributedTab(send(label, "attributedStringValue")); got <= tab {
			t.Errorf("badge tab after a wider label = %v, was %v", got, tab)
		}

		item.Label, item.Badge, item.Checked, item.Enabled = "Node 2", "123456 ms", false, false
		b.UpdateMenuItem(item)
		if !sendBool(check, "isHidden") {
			t.Error("unchecked item still shows the check mark")
		}
		if got := goString(send(label, "stringValue")); got != "Node 2\t123456 ms" {
			t.Errorf("label after update = %q", got)
		}
		if got := msgFloat(view, sel("alphaValue")); got != 0.45 {
			t.Errorf("disabled view alpha = %v", got)
		}
		if got := goString(send(view, "accessibilityValue")); got != "123456 ms" {
			t.Errorf("accessibility value after update = %q", got)
		}
		if sendBool(view, "accessibilityEnabled") {
			t.Error("disabled item's view is enabled")
		}
		if mark := send(view, "accessibilityMenuItemMarkChar"); mark != 0 {
			t.Errorf("unchecked item's mark = %q", goString(mark))
		}
	})
}

// TestMenuKeepOpenSubtitleAndIcon checks that a KeepOpen item shows its
// subtitle after the label, since its view has one line, and that an icon
// takes a column of its own in the views of the menu.
func TestMenuKeepOpenSubtitleAndIcon(t *testing.T) {
	load()
	registerMenuTarget()
	LinkMenuIcons()
	defer func() { menuImage = nil }()
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	withPool(func() {
		b := &Backend{}
		m := &platform.Menu{Items: []*platform.MenuItem{
			{ID: 7, Label: "Node", Subtitle: "Hong Kong", Badge: "45 ms", KeepOpen: true,
				Visible: true, Enabled: true, Icon: &platform.MenuIcon{PNG: png, Template: true}},
			{ID: 8, Label: "Other", KeepOpen: true, Visible: true, Enabled: true},
		}}
		owner := newOwner()
		menu := b.buildMenu(m, "", owner)
		defer dropOwner(owner)

		view := send(send(menu, "itemAtIndex:", 0), "view")
		label := send(view, "viewWithTag:", menuViewLabel)
		if got := goString(send(label, "stringValue")); got != "Node — Hong Kong\t45 ms" {
			t.Errorf("label = %q", got)
		}
		if got := goString(send(view, "accessibilityLabel")); got != "Node — Hong Kong" {
			t.Errorf("accessibility label = %q", got)
		}
		icon := send(view, "viewWithTag:", menuViewIcon)
		if icon == 0 || sendBool(icon, "isHidden") {
			t.Fatal("item with an icon shows none")
		}
		if got := msgFloat(menu, sel("minimumWidth")); got < msgRect(view, sel("frame")).Size.Width {
			t.Errorf("menu minimum width = %v, view width = %v", got, msgRect(view, sel("frame")).Size.Width)
		}
		// The views of the menu share the icon column, whether or not
		// their own item has an icon.
		for i, want := range []float64{menuKeepOpenIconLabelX, menuKeepOpenIconLabelX} {
			itemView := send(send(menu, "itemAtIndex:", uintptr(i)), "view")
			field := send(itemView, "viewWithTag:", menuViewLabel)
			if got := msgRect(field, sel("frame")).Origin.X; got != want {
				t.Errorf("item %d label x = %v, want %v", i, got, want)
			}
		}
	})
}
