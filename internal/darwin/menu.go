//go:build darwin

package darwin

import (
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/ebitengine/purego/objc"

	"github.com/egoist/mygo/internal/accelerator"
	"github.com/egoist/mygo/internal/platform"
)

// Native menu items are tracked per owner (the app menu, a tray, a popup)
// so they can be updated in place and released when the owner rebuilds.
type nativeItem struct {
	owner int
	item  id
}

const ownerAppMenu = 0

var (
	menuOwners = 1
	itemsByID  = map[int][]nativeItem{}
	// menuHighlightDelegate is kept here: NSMenu holds its delegate weakly.
	menuHighlightDelegate id
	nativeRoleSel         = map[string]string{
		"undo":               "undo:",
		"redo":               "redo:",
		"cut":                "cut:",
		"copy":               "copy:",
		"paste":              "paste:",
		"pasteAndMatchStyle": "pasteAsPlainText:",
		"delete":             "delete:",
		"selectAll":          "selectAll:",
		"minimize":           "performMiniaturize:",
		"zoom":               "performZoom:",
		"close":              "performClose:",
		"front":              "arrangeInFront:",
		"togglefullscreen":   "toggleFullScreen:",
		"about":              "orderFrontStandardAboutPanel:",
		"hide":               "hide:",
		"hideOthers":         "hideOtherApplications:",
		"unhide":             "unhideAllApplications:",
		"quit":               "terminate:",
		"startSpeaking":      "startSpeaking:",
		"stopSpeaking":       "stopSpeaking:",
	}
)

// validatesItem reports whether the first responder enables it as it can
// act on its role, so that its menu has AppKit validate its items.
func validatesItem(it *platform.MenuItem) bool {
	return platform.EditCommands[it.Role] || it.Role == "startSpeaking" || it.Role == "stopSpeaking"
}

func newOwner() int {
	menuOwners++
	return menuOwners
}

func track(owner, itemID int, item id) {
	itemsByID[itemID] = append(itemsByID[itemID], nativeItem{owner, retain(item)})
}

func dropOwner(owner int) {
	for itemID, list := range itemsByID {
		kept := list[:0]
		for _, n := range list {
			if n.owner == owner {
				if view := send(n.item, "view"); view != 0 {
					delete(keepOpenBadges, view)
				}
				delete(menuBadges, n.item)
				release(n.item)
			} else {
				kept = append(kept, n)
			}
		}
		if len(kept) == 0 {
			delete(itemsByID, itemID)
		} else {
			itemsByID[itemID] = kept
		}
	}
}

func registerMenuTarget() {
	if hasClass("MyGoMenuTarget") {
		return
	}
	classDef("MyGoMenuTarget", "NSObject", nil, []objc.MethodDef{
		method("mygoMenuItemClicked:", func(self id, _ objc.SEL, item id) {
			theBackend.h.MenuItemClicked(sendInt(item, "tag"))
		}),
		method("mygoContextItemClicked:", contextItemClicked),
		// The app's items keep the state it gave them in a menu that
		// validates its items.
		method("validateMenuItem:", func(self id, _ objc.SEL, item id) bool { return sendBool(item, "isEnabled") }),
	})
	// Items that keep their menu open show a view MyGo draws: the view
	// handles the click itself, so NSMenu does not end its tracking and
	// the item's menu and the one above it stay up. It answers the keyboard
	// and VoiceOver as the items NSMenu draws do.
	classDef("MyGoMenuKeepOpenView", "NSView", nil, []objc.MethodDef{
		method("mouseUp:", func(self id, _ objc.SEL, _ id) {
			keepOpenClick(self)
		}),
		method("mouseEntered:", func(self id, _ objc.SEL, _ id) {
			setMenuViewHighlighted(self, true)
		}),
		method("mouseExited:", func(self id, _ objc.SEL, _ id) {
			setMenuViewHighlighted(self, false)
		}),
		method("acceptsFirstResponder", func(self id, _ objc.SEL) bool { return true }),
		method("canBecomeKeyView", func(self id, _ objc.SEL) bool { return true }),
		method("keyDown:", func(self id, _ objc.SEL, event id) {
			keepOpenKeyDown(self, event)
		}),
		method("isAccessibilityElement", func(self id, _ objc.SEL) bool { return true }),
		method("accessibilityRole", func(self id, _ objc.SEL) id {
			return nsString("AXMenuItem")
		}),
		method("accessibilityLabel", func(self id, _ objc.SEL) id {
			label, _, _ := keepOpenTitle(self)
			return nsString(label)
		}),
		method("accessibilityValue", func(self id, _ objc.SEL) id {
			_, badge, _ := keepOpenTitle(self)
			if badge == "" {
				return 0
			}
			return nsString(badge)
		}),
		method("accessibilityEnabled", func(self id, _ objc.SEL) bool {
			item := send(self, "enclosingMenuItem")
			return item != 0 && sendBool(item, "isEnabled")
		}),
		// The mark a menu item shows when it is chosen, which VoiceOver
		// reads as its state.
		method("accessibilityMenuItemMarkChar", func(self id, _ objc.SEL) id {
			item := send(self, "enclosingMenuItem")
			if item != 0 && sendInt(item, "state") != 0 {
				return nsString("✓")
			}
			return 0
		}),
		method("accessibilityActionNames", func(self id, _ objc.SEL) id {
			return nsArray(nsString("AXPress"))
		}),
		method("accessibilityPerformPress", func(self id, _ objc.SEL) bool {
			return keepOpenClick(self)
		}),
	})
	// A pill draws behind its text in the view, and the view handles the
	// mouse: the badge must not take the click.
	classDef("MyGoMenuBadge", "NSTextField", nil, []objc.MethodDef{
		method("hitTest:", func(self id, _ objc.SEL, _ NSPoint) id { return 0 }),
	})
	// NSMenu keeps key events in its tracking loop: it highlights the item
	// itself, so the view follows the highlight from the menu's delegate
	// whether the mouse or the keyboard moved it.
	classDef("MyGoMenuHighlight", "NSObject", nil, []objc.MethodDef{
		method("menu:willHighlightItem:", func(self id, _ objc.SEL, menu, item id) {
			highlightKeepOpenViews(menu, item)
		}),
		method("menuDidClose:", func(self id, _ objc.SEL, menu id) {
			highlightKeepOpenViews(menu, 0)
		}),
	})
	menuHighlightDelegate = send(send(class("MyGoMenuHighlight"), "alloc"), "init")
}

// highlightKeepOpenViews shows the selection of a menu in the views of its
// KeepOpen items.
func highlightKeepOpenViews(menu, item id) {
	for _, x := range arrayItems(send(menu, "itemArray")) {
		if view := send(x, "view"); view != 0 {
			setMenuViewHighlighted(view, x == item)
		}
	}
}

// keepOpenClick chooses the item of a KeepOpen view.
func keepOpenClick(view id) bool {
	item := send(view, "enclosingMenuItem")
	if item == 0 || !sendBool(item, "isEnabled") {
		return false
	}
	theBackend.h.MenuItemClicked(sendInt(send(view, "identifier"), "integerValue"))
	return true
}

// keepOpenKeyDown moves the highlight of the menu or chooses its item from
// the keyboard, as NSMenu does for the items it draws.
func keepOpenKeyDown(view id, event id) {
	item := send(view, "enclosingMenuItem")
	if item == 0 {
		return
	}
	menu := send(item, "menu")
	if menu == 0 {
		return
	}
	switch sendInt(event, "keyCode") {
	case 36, 76, 49: // Return, keypad Enter, Space
		keepOpenPress(menu)
	case 125: // Down
		highlightMenuItem(menu, 1)
	case 126: // Up
		highlightMenuItem(menu, -1)
	case 53: // Escape
		send(menu, "cancelTracking")
	}
}

// keepOpenPress chooses the item a menu highlights, keeping the menu up for
// a KeepOpen item.
func keepOpenPress(menu id) {
	item := send(menu, "highlightedItem")
	if item == 0 || !sendBool(item, "isEnabled") {
		return
	}
	if view := send(item, "view"); view != 0 {
		keepOpenClick(view)
		return
	}
	if i := sendInt(menu, "indexOfItem:", uintptr(item)); i >= 0 {
		send(menu, "performActionForItemAtIndex:", uintptr(i))
	}
}

// menuItemEnabled reports whether a menu may choose item.
func menuItemEnabled(item id) bool {
	return !sendBool(item, "isHidden") && !sendBool(item, "isSeparatorItem") && sendBool(item, "isEnabled")
}

// highlightMenuItem moves the highlight of a menu by delta, skipping the
// items it cannot choose, and follows it in the views of its KeepOpen
// items.
func highlightMenuItem(menu id, delta int) {
	items := arrayItems(send(menu, "itemArray"))
	if len(items) == 0 {
		return
	}
	start := 0
	if item := send(menu, "highlightedItem"); item != 0 {
		for i, x := range items {
			if x == item {
				start = i
				break
			}
		}
	}
	for step := 1; step <= len(items); step++ {
		i := ((start+delta*step)%len(items) + len(items)) % len(items)
		if menuItemEnabled(items[i]) {
			send(menu, "highlightItem:", uintptr(items[i]))
			break
		}
	}
	for _, x := range items {
		if view := send(x, "view"); view != 0 {
			setMenuViewHighlighted(view, sendBool(x, "isHighlighted"))
		}
	}
}

func (b *Backend) SetApplicationMenu(m *platform.Menu) {
	withPool(func() {
		dropOwner(ownerAppMenu)
		if m == nil {
			send(b.app, "setMainMenu:", 0)
			return
		}
		send(b.app, "setMainMenu:", uintptr(b.buildMenu(m, "", ownerAppMenu)))
	})
}

// menuMetrics are the columns a menu's items share: the x badges end at
// and the width of the views of its KeepOpen items.
type menuMetrics struct {
	badgeStart     float64
	badgeWidth     float64
	keepOpenStart  float64
	keepOpenWidth  float64
	keepOpenView   float64
	keepOpenLabelX float64
}

// buildMenu creates an autoreleased NSMenu.
func (b *Backend) buildMenu(m *platform.Menu, title string, owner int) id {
	menu := autorelease(send(send(class("NSMenu"), "alloc"), "initWithTitle:", uintptr(nsString(title))))
	send(menu, "setAutoenablesItems:", boolArg(slices.ContainsFunc(m.Items, validatesItem)))
	badgeStart, badgeWidth := badgeColumn(m)
	keepOpenStart, keepOpenWidth, keepOpenView, keepOpenLabelX := keepOpenMetrics(m)
	metrics := menuMetrics{
		badgeStart: badgeStart, badgeWidth: badgeWidth,
		keepOpenStart: keepOpenStart, keepOpenWidth: keepOpenWidth,
		keepOpenView: keepOpenView, keepOpenLabelX: keepOpenLabelX,
	}
	if keepOpenView > 0 {
		// NSMenu sizes itself from its titles, which a view replaces: without
		// this, it cuts the right of the views off.
		msgSetFloat(menu, sel("setMinimumWidth:"), keepOpenView)
		if menuHighlightDelegate != 0 {
			send(menu, "setDelegate:", uintptr(menuHighlightDelegate))
		}
	}
	for _, it := range m.Items {
		if it.Type == platform.MenuItemSeparator {
			item := send(class("NSMenuItem"), "separatorItem")
			send(item, "setHidden:", boolArg(!it.Visible))
			send(menu, "addItem:", uintptr(item))
			track(owner, it.ID, item)
			continue
		}
		item := b.buildItem(it, owner, metrics)
		send(menu, "addItem:", uintptr(item))
		if it.Role == "zoomIn" {
			// Also accept Cmd+= (no shift) like other macOS apps.
			alt := b.buildItem(&platform.MenuItem{ID: it.ID, Label: it.Label, Role: it.Role, Accelerator: "CmdOrCtrl+=", Enabled: it.Enabled}, owner, menuMetrics{})
			send(alt, "setHidden:", 1)
			if respondsTo(alt, "setAllowsKeyEquivalentWhenHidden:") {
				send(alt, "setAllowsKeyEquivalentWhenHidden:", 1)
			}
			send(menu, "addItem:", uintptr(alt))
		}
	}
	return menu
}

func (b *Backend) buildItem(it *platform.MenuItem, owner int, metrics menuMetrics) id {
	action := sel("mygoMenuItemClicked:")
	target := b.menuTarget
	if s, ok := nativeRoleSel[it.Role]; ok {
		action, target = sel(s), 0
	}
	if it.Submenu != nil {
		action, target = 0, 0
	}
	key, mods := keyEquivalent(it.Accelerator)
	item := autorelease(send(send(class("NSMenuItem"), "alloc"), "initWithTitle:action:keyEquivalent:",
		uintptr(nsString(it.Label)), uintptr(action), uintptr(nsString(key))))
	if key != "" {
		send(item, "setKeyEquivalentModifierMask:", uintptr(mods))
	}
	send(item, "setTarget:", uintptr(target))
	send(item, "setTag:", uintptr(it.ID))
	applyItemState(item, it)
	if it.KeepOpen && it.Submenu == nil {
		tab := badgeTabFor(it.BadgeAlign, metrics.keepOpenStart, metrics.keepOpenWidth)
		view := buildKeepOpenView(it, metrics)
		send(item, "setView:", uintptr(view))
		// The view replaces the title, but NSMenu sizes the menu by it: it
		// holds the same label, tab stop and badge, so the menu is wide
		// enough for the view.
		setItemTitle(item, keepOpenItemTitle(it), itemBadge(it), tab, it.BadgeAlign)
		updateKeepOpenView(view, it, tab)
	} else {
		tab := badgeTabFor(it.BadgeAlign, metrics.badgeStart, metrics.badgeWidth)
		st := setMenuBadgeStyle(item, it)
		setItemTitleStyled(item, itemTitle(it), itemBadge(it), tab, it.BadgeAlign, st)
	}
	if it.Submenu != nil {
		sub := b.buildMenu(it.Submenu, it.Label, owner)
		send(item, "setSubmenu:", uintptr(sub))
		switch it.Role {
		case "services":
			send(b.app, "setServicesMenu:", uintptr(sub))
		case "window", "windowMenu":
			send(b.app, "setWindowsMenu:", uintptr(sub))
		case "help":
			send(b.app, "setHelpMenu:", uintptr(sub))
		}
	}
	track(owner, it.ID, item)
	return item
}

func applyItemState(item id, it *platform.MenuItem) {
	if it.Type != platform.MenuItemSeparator {
		applyItemSubtitle(item, it)
	}
	if it.Type != platform.MenuItemSeparator && menuImage != nil {
		var img id
		if it.Icon != nil {
			img = menuImage(it.Icon)
		}
		if send(item, "image") != img {
			send(item, "setImage:", uintptr(img))
		}
	}
	send(item, "setEnabled:", boolArg(it.Enabled))
	if _, ok := nativeRoleSel[it.Role]; ok {
		// In a menu validating its items, the first responder would enable
		// a role's item: disabled, it targets an object without its action,
		// which its menu disables.
		var target id
		if !it.Enabled {
			target = theBackend.menuTarget
		}
		send(item, "setTarget:", uintptr(target))
	}
	send(item, "setHidden:", boolArg(!it.Visible))
	state := uintptr(0)
	if it.Checked && (it.Type == platform.MenuItemCheckbox || it.Type == platform.MenuItemRadio) {
		state = 1
	}
	send(item, "setState:", state)
	if it.ToolTip != "" {
		send(item, "setToolTip:", uintptr(nsString(it.ToolTip)))
	}
}

// applyItemSubtitle gives the item its subtitle, which NSMenuItem shows from
// macOS 14, and after the title before.
func applyItemSubtitle(item id, it *platform.MenuItem) {
	if !respondsTo(item, "setSubtitle:") {
		if it.Subtitle != "" {
			send(item, "setTitle:", uintptr(nsString(it.LabelWithSubtitle())))
		}
		return
	}
	if it.Subtitle != "" {
		send(item, "setSubtitle:", uintptr(nsString(it.Subtitle)))
	} else if send(item, "subtitle") != 0 {
		send(item, "setSubtitle:", 0)
	}
}

// subtitleOnLine reports whether NSMenuItem shows a subtitle on a line of
// its own, as macOS 14 does, rather than after the title.
func subtitleOnLine() bool {
	return sendBool(class("NSMenuItem"), "instancesRespondToSelector:", uintptr(sel("setSubtitle:")))
}

// itemTitle is the text of the title of it, which carries the subtitle
// where the item cannot show one on a line of its own.
func itemTitle(it *platform.MenuItem) string {
	label := it.Label
	if !subtitleOnLine() && it.Subtitle != "" {
		label = it.LabelWithSubtitle()
	}
	return strings.ReplaceAll(label, "\t", " ")
}

// itemBadge is the text of the badge of it.
func itemBadge(it *platform.MenuItem) string {
	return strings.ReplaceAll(it.Badge, "\t", " ")
}

// keepOpenItem reports whether MyGo draws the item in a view of its own,
// which shows its badge.
func keepOpenItem(it *platform.MenuItem) bool {
	return it.KeepOpen && it.Submenu == nil
}

// menuBadgeGap is the least space between the longest title of a menu and
// its badges' left edge.
const menuBadgeGap = 24

// badgeColumn returns where the badges of m start and how wide their column
// is: past the menu's longest title, and as wide as its widest badge. Zero
// when no item has one.
func badgeColumn(m *platform.Menu) (start, width float64) {
	found := false
	for _, it := range m.Items {
		if it.Visible && it.Badge != "" && it.Type != platform.MenuItemSeparator && !keepOpenItem(it) {
			found = true
			break
		}
	}
	if !found {
		return 0, 0
	}
	labelFont := menuFont()
	var maxLabel, maxBadge float64
	for _, it := range m.Items {
		if !it.Visible || it.Type == platform.MenuItemSeparator || keepOpenItem(it) {
			continue
		}
		maxLabel = max(maxLabel, textWidth(itemTitle(it), labelFont))
		maxBadge = max(maxBadge, menuBadgeBoxWidth(itemBadge(it), it.BadgeShape))
	}
	return maxLabel + menuBadgeGap, maxBadge
}

// badgeTabFor returns the tab stop location of a badge of align in a column
// that starts at start and is width wide.
func badgeTabFor(align platform.BadgeAlignment, start, width float64) float64 {
	switch align {
	case platform.BadgeAlignLeft:
		return start
	case platform.BadgeAlignCenter:
		return start + width/2
	default:
		return start + width
	}
}

// NSTextAlignment values. Center and right differ by architecture: arm64
// uses UIKit's (center 1, right 2), x86_64 AppKit's original ones (right 1,
// center 2).
const nsTextAlignmentLeft = 0

var nsTextAlignmentCenter, nsTextAlignmentRight = textAlignments(runtime.GOARCH)

func textAlignments(arch string) (center, right int) {
	if arch == "amd64" {
		return 2, 1
	}
	return 1, 2
}

// tabAlignment is the NSTextTab alignment of a badge.
func tabAlignment(align platform.BadgeAlignment) uint {
	switch align {
	case platform.BadgeAlignLeft:
		return nsTextAlignmentLeft
	case platform.BadgeAlignCenter:
		return uint(nsTextAlignmentCenter)
	default:
		return uint(nsTextAlignmentRight)
	}
}

// menuFont is the font AppKit draws menu item titles in, autoreleased.
func menuFont() id {
	return msgFloatID(class("NSFont"), sel("menuFontOfSize:"), 0)
}

// menuBadgeFont is the monospaced-digit font of a badge, so that latencies
// and counts keep their width as their digits change, autoreleased.
func menuBadgeFont() id {
	return msgFloat2ID(class("NSFont"), sel("monospacedDigitSystemFontOfSize:weight:"), 12, 0)
}

// textWidth measures s in font, in points.
func textWidth(s string, font id) float64 {
	if s == "" || font == 0 {
		return 0
	}
	attrs := send(class("NSMutableDictionary"), "dictionary")
	send(attrs, "setObject:forKey:", uintptr(font), uintptr(nsString("NSFont")))
	str := autorelease(send(send(class("NSAttributedString"), "alloc"), "initWithString:attributes:",
		uintptr(nsString(s)), uintptr(attrs)))
	return msgSize(str, sel("size")).Width
}

// menuTitle returns the title of a label and, at the tab stop tab aligned
// as align says, its badge in a monospaced-digit font, autoreleased.
func menuTitle(label, badge string, tab float64, align platform.BadgeAlignment) id {
	return menuTitleStyled(label, badge, tab, align, menuBadgeStyle{})
}

// menuTitleStyled is menuTitle with the badge's colors and, for a shape,
// a square background behind it. System items cannot round it.
func menuTitleStyled(label, badge string, tab float64, align platform.BadgeAlignment, st menuBadgeStyle) id {
	attrs := send(class("NSMutableDictionary"), "dictionary")
	if font := menuFont(); font != 0 {
		send(attrs, "setObject:forKey:", uintptr(font), uintptr(nsString("NSFont")))
	}
	if badge == "" || tab <= 0 {
		return autorelease(send(send(class("NSAttributedString"), "alloc"), "initWithString:attributes:",
			uintptr(nsString(label)), uintptr(attrs)))
	}
	style := autorelease(send(send(class("NSMutableParagraphStyle"), "alloc"), "init"))
	stop := autorelease(msgInitTextTab(send(class("NSTextTab"), "alloc"),
		sel("initWithTextAlignment:location:options:"), tabAlignment(align), tab, 0))
	send(style, "setTabStops:", uintptr(nsArray(stop)))
	send(attrs, "setObject:forKey:", uintptr(style), uintptr(nsString("NSParagraphStyle")))
	shown := badge
	if st.shape != "" {
		shown = menuBadgePad + badge + menuBadgePad
	}
	title := autorelease(send(send(class("NSMutableAttributedString"), "alloc"), "initWithString:attributes:",
		uintptr(nsString(label+"\t"+shown)), uintptr(attrs)))
	start := units([]rune(label + "\t"))
	length := units([]rune(shown))
	if font := menuBadgeFont(); font != 0 {
		send(title, "addAttribute:value:range:",
			uintptr(nsString("NSFont")), uintptr(font), uintptr(start), uintptr(length))
	}
	if st.shape != "" {
		background := send(class("NSColor"), "quaternaryLabelColor")
		if st.hasBG {
			background = nsColor(st.bg)
		}
		send(title, "addAttribute:value:range:",
			uintptr(nsString("NSBackgroundColor")), uintptr(background), uintptr(start), uintptr(length))
	}
	if color := menuBadgeTextColor(st); color != 0 {
		send(title, "addAttribute:value:range:",
			uintptr(nsString("NSColor")), uintptr(color), uintptr(start), uintptr(length))
	}
	return title
}

// menuBadgeTextColor is the foreground of a styled system badge, or 0 for
// the menu's own color.
func menuBadgeTextColor(st menuBadgeStyle) id {
	if st.hasFG {
		return nsColor(st.fg)
	}
	if st.shape != "" && st.hasBG && st.bg.A >= 128 {
		if keepOpenColorDark(st.bg) {
			return send(class("NSColor"), "whiteColor")
		}
		return send(class("NSColor"), "blackColor")
	}
	return 0
}

// setItemTitle shows label and, at the tab stop tab aligned as align says,
// badge in a monospaced-digit font.
func setItemTitle(item id, label, badge string, tab float64, align platform.BadgeAlignment) {
	setItemTitleStyled(item, label, badge, tab, align, menuBadgeStyle{})
}

func setItemTitleStyled(item id, label, badge string, tab float64, align platform.BadgeAlignment, st menuBadgeStyle) {
	if badge == "" || tab <= 0 {
		send(item, "setAttributedTitle:", 0)
		send(item, "setTitle:", uintptr(nsString(label)))
		return
	}
	send(item, "setAttributedTitle:", uintptr(menuTitleStyled(label, badge, tab, align, st)))
}

// titleAlignment returns the alignment of the badge tab stop of a title.
func titleAlignment(title id) platform.BadgeAlignment {
	if title == 0 {
		return platform.BadgeAlignRight
	}
	style := send(title, "attribute:atIndex:effectiveRange:", uintptr(nsString("NSParagraphStyle")), 0, 0)
	if style == 0 {
		return platform.BadgeAlignRight
	}
	tabs := arrayItems(send(style, "tabStops"))
	if len(tabs) == 0 {
		return platform.BadgeAlignRight
	}
	switch sendInt(tabs[0], "alignment") {
	case nsTextAlignmentLeft:
		return platform.BadgeAlignLeft
	case nsTextAlignmentCenter:
		return platform.BadgeAlignCenter
	default:
		return platform.BadgeAlignRight
	}
}

// itemTitleParts returns the title and badge of a native item, as its title
// holds them, and the alignment of its badge.
func itemTitleParts(item id) (label, badge string, align platform.BadgeAlignment) {
	align = platform.BadgeAlignRight
	var title id
	if title = send(item, "attributedTitle"); title != 0 {
		label = goString(send(title, "string"))
		align = titleAlignment(title)
	} else {
		label = goString(send(item, "title"))
	}
	if i := strings.IndexByte(label, '\t'); i >= 0 {
		return label[:i], strings.Trim(label[i+1:], menuBadgePad), align
	}
	return label, "", align
}

// retabMenu aligns the badges of the menu that item is in with each other
// and with its titles: the titles of the items since a label, a badge or
// the visibility of an item changed. label and badge are those of item,
// which the menu's items show, in place of its title's.
func retabMenu(item id, label, badge string, align platform.BadgeAlignment) {
	menu := send(item, "menu")
	if menu == 0 {
		return
	}
	type row struct {
		item  id
		label string
		badge string
		align platform.BadgeAlignment
		style menuBadgeStyle
	}
	var rows []row
	labelFont := menuFont()
	var maxLabel, maxBadge float64
	for _, x := range arrayItems(send(menu, "itemArray")) {
		if x == 0 || sendBool(x, "isHidden") || sendBool(x, "isSeparatorItem") || send(x, "view") != 0 {
			continue
		}
		l, b, a := label, badge, align
		style := menuBadges[x]
		if x != item {
			l, b, a = itemTitleParts(x)
		}
		if l == "" && b == "" {
			continue
		}
		rows = append(rows, row{x, l, b, a, style})
		maxLabel = max(maxLabel, textWidth(l, labelFont))
		maxBadge = max(maxBadge, menuBadgeBoxWidth(b, style.shape))
	}
	start := maxLabel + menuBadgeGap
	for _, r := range rows {
		tab := badgeTabFor(r.align, start, maxBadge)
		setItemTitleStyled(r.item, r.label, r.badge, tab, r.align, r.style)
	}
}

// The layout of the view of an item that keeps its menu open: a check
// mark column, an icon column, the label and its badge, and room after
// them.
const (
	menuKeepOpenHeight     = 22
	menuKeepOpenCheckX     = 6
	menuKeepOpenIconX      = 24
	menuKeepOpenIconSize   = 16
	menuKeepOpenLabelX     = 26
	menuKeepOpenIconLabelX = menuKeepOpenLabelX + 20
	menuKeepOpenRight      = 8
	menuKeepOpenMinRoom    = 120
	menuViewBackdrop       = 1
	menuViewCheck          = 2
	menuViewLabel          = 3
	menuViewIcon           = 4
	menuViewBadge          = 5
	menuBadgePadX          = 6
	menuBadgeHeight        = 16
)

// menuBadgePad is the space a system badge's background adds on each side.
const menuBadgePad = "\u2002"

// menuBadgeStyle is the badge of a system-drawn item: its shape and colors.
type menuBadgeStyle struct {
	shape platform.BadgeShape
	bg    platform.Color
	fg    platform.Color
	hasBG bool
	hasFG bool
}

// menuBadges holds the badge style of each system-drawn item.
var menuBadges = map[id]menuBadgeStyle{}

// setMenuBadgeStyle records the badge style of it for item.
func setMenuBadgeStyle(item id, it *platform.MenuItem) menuBadgeStyle {
	st := menuBadgeStyle{shape: it.BadgeShape}
	if it.BadgeColor != nil {
		st.bg, st.hasBG = *it.BadgeColor, true
	}
	if it.BadgeTextColor != nil {
		st.fg, st.hasFG = *it.BadgeTextColor, true
	}
	menuBadges[item] = st
	return st
}

// menuBadgeBoxWidth is the width a system badge takes: a background keeps
// room for its padding.
func menuBadgeBoxWidth(badge string, shape platform.BadgeShape) float64 {
	w := textWidth(badge, menuBadgeFont())
	if shape != "" && badge != "" {
		w += 2 * textWidth(menuBadgePad, menuBadgeFont())
	}
	return w
}

// keepOpenBadgeStyle is the badge of a KeepOpen view: its alignment and,
// for a pill, its colors.
type keepOpenBadgeStyle struct {
	shape platform.BadgeShape
	align platform.BadgeAlignment
	bg    platform.Color
	fg    platform.Color
	hasBG bool
	hasFG bool
}

// keepOpenBadges holds the badge style of each KeepOpen view. The menu's
// views last as long as the menu, which clears their entries when it closes.
var keepOpenBadges = map[id]keepOpenBadgeStyle{}

// setKeepOpenBadge records the badge style of it for view.
func setKeepOpenBadge(view id, it *platform.MenuItem) keepOpenBadgeStyle {
	s := keepOpenBadgeStyle{shape: it.BadgeShape, align: it.BadgeAlign}
	if it.BadgeColor != nil {
		s.bg, s.hasBG = *it.BadgeColor, true
	}
	if it.BadgeTextColor != nil {
		s.fg, s.hasFG = *it.BadgeTextColor, true
	}
	keepOpenBadges[view] = s
	return s
}

// keepOpenBadgeWidth is the width a badge takes in its column: a pill
// keeps room for its text padding.
func keepOpenBadgeWidth(badge string, shape platform.BadgeShape) float64 {
	w := textWidth(badge, menuBadgeFont())
	if shape != "" && badge != "" {
		w += 2 * menuBadgePadX
	}
	return w
}

// keepOpenBadgeRadius is the corner radius of a badge background.
func keepOpenBadgeRadius(shape platform.BadgeShape) float64 {
	switch shape {
	case platform.BadgeShapeSquare:
		return 0
	case platform.BadgeShapeRounded:
		return 4
	case platform.BadgeShapePill:
		return menuBadgeHeight / 2
	default:
		return 0
	}
}

// keepOpenItemTitle is the text of the view of it. The view has one line,
// so the subtitle follows the label.
func keepOpenItemTitle(it *platform.MenuItem) string {
	return strings.ReplaceAll(it.LabelWithSubtitle(), "\t", " ")
}

// keepOpenMetrics returns the x badges start at in the views of m's KeepOpen
// items, the width those views share, and the x their labels start at: past
// the icon column when one of them has an icon. Zero when m has no such
// item.
func keepOpenMetrics(m *platform.Menu) (start, badgeWidth, viewWidth, labelX float64) {
	found := false
	icon := false
	for _, it := range m.Items {
		if it.Visible && it.KeepOpen && it.Submenu == nil && it.Type != platform.MenuItemSeparator {
			found = true
			icon = icon || it.Icon != nil
		}
	}
	if !found {
		return 0, 0, 0, 0
	}
	labelX = menuKeepOpenLabelX
	if icon {
		labelX = menuKeepOpenIconLabelX
	}
	labelFont := menuFont()
	hasBadge := false
	var maxLabel, maxBadge float64
	for _, it := range m.Items {
		if !it.Visible || !it.KeepOpen || it.Submenu != nil || it.Type == platform.MenuItemSeparator {
			continue
		}
		maxLabel = max(maxLabel, textWidth(keepOpenItemTitle(it), labelFont))
		if it.Badge != "" {
			hasBadge = true
			maxBadge = max(maxBadge, keepOpenBadgeWidth(itemBadge(it), it.BadgeShape))
		}
	}
	start = maxLabel
	if hasBadge {
		start = maxLabel + menuBadgeGap
	}
	viewWidth = keepOpenViewWidth(labelX, start, maxBadge)
	return start, maxBadge, viewWidth, labelX
}

// keepOpenViewWidth is the width of the views of a menu's KeepOpen items:
// the label column, the badge column, and the space after the badges. The
// menu sizes itself from it (setMinimumWidth) and the badges are anchored
// to the view's right edge, so this space is all that separates a badge
// from the row's end: 8pt is the room a menu row keeps at its trailing
// edge, and the badge box carries its own padding on top of it.
func keepOpenViewWidth(labelX, start, badgeWidth float64) float64 {
	return max(labelX+menuKeepOpenMinRoom, labelX+start+badgeWidth+menuKeepOpenRight)
}

// retabKeepOpenMenu lays the views of the KeepOpen items of the menu that
// item is in out again, as the labels and badges of its items are now.
func retabKeepOpenMenu(item id, label, badge string, align platform.BadgeAlignment) {
	menu := send(item, "menu")
	if menu == 0 {
		return
	}
	type row struct {
		view  id
		label string
		badge string
		align platform.BadgeAlignment
		style keepOpenBadgeStyle
	}
	var rows []row
	labelFont := menuFont()
	labelX := float64(menuKeepOpenLabelX)
	var maxLabel, maxBadge float64
	hasBadge := false
	for _, x := range arrayItems(send(menu, "itemArray")) {
		view := send(x, "view")
		if view == 0 || sendBool(x, "isHidden") {
			continue
		}
		if icon := send(view, "viewWithTag:", menuViewIcon); icon != 0 && !sendBool(icon, "isHidden") {
			labelX = menuKeepOpenIconLabelX
		}
		style := keepOpenBadges[view]
		l, b, a := label, badge, align
		if x != item {
			l, b, _ = keepOpenTitle(view)
			a = style.align
		}
		if a == "" {
			a = style.align
		}
		if l == "" && b == "" {
			continue
		}
		rows = append(rows, row{view, l, b, a, style})
		maxLabel = max(maxLabel, textWidth(l, labelFont))
		if b != "" {
			hasBadge = true
			maxBadge = max(maxBadge, keepOpenBadgeWidth(b, style.shape))
		}
	}
	start := maxLabel
	if hasBadge {
		start = maxLabel + menuBadgeGap
	}
	width := keepOpenViewWidth(labelX, start, maxBadge)
	msgSetFloat(menu, sel("setMinimumWidth:"), width)
	for _, r := range rows {
		layoutKeepOpenView(r.view, r.label, r.badge, start, maxBadge, width, labelX, r.align, r.style)
	}
}

// buildKeepOpenView returns the view of an item that keeps its menu open:
// MyGo draws the check mark, label and badge, and handles the mouse itself
// so NSMenu does not end its tracking when the item is clicked.
func buildKeepOpenView(it *platform.MenuItem, metrics menuMetrics) id {
	width := metrics.keepOpenView
	view := autorelease(msgInitRect(send(class("MyGoMenuKeepOpenView"), "alloc"),
		sel("initWithFrame:"), NSRect{Size: NSSize{width, menuKeepOpenHeight}}))
	send(view, "setIdentifier:", uintptr(nsString(strconv.Itoa(it.ID))))
	send(view, "setWantsLayer:", 1)
	send(view, "setAutoresizingMask:", 2) // NSViewWidthSizable
	layer := send(view, "layer")
	if layer != 0 {
		backdrop := autorelease(send(send(class("CALayer"), "alloc"), "init"))
		msgSetRect(backdrop, sel("setFrame:"), NSRect{
			Origin: NSPoint{X: 4, Y: 1},
			Size:   NSSize{Width: width - 8, Height: menuKeepOpenHeight - 2},
		})
		msgSetFloat(backdrop, sel("setCornerRadius:"), 4)
		send(backdrop, "setAutoresizingMask:", 2) // kCALayerWidthSizable
		send(backdrop, "setHidden:", 1)
		send(layer, "addSublayer:", uintptr(backdrop))
	}

	check := autorelease(msgInitRect(send(class("NSImageView"), "alloc"), sel("initWithFrame:"),
		NSRect{Origin: NSPoint{X: menuKeepOpenCheckX, Y: (menuKeepOpenHeight - 14) / 2}, Size: NSSize{Width: 14, Height: 14}}))
	send(check, "setImage:", uintptr(send(class("NSImage"), "imageNamed:", uintptr(nsString("NSMenuOnStateTemplate")))))
	send(check, "setTag:", menuViewCheck)
	send(view, "addSubview:", uintptr(check))

	icon := autorelease(msgInitRect(send(class("NSImageView"), "alloc"), sel("initWithFrame:"),
		NSRect{Origin: NSPoint{X: menuKeepOpenIconX, Y: (menuKeepOpenHeight - menuKeepOpenIconSize) / 2},
			Size: NSSize{Width: menuKeepOpenIconSize, Height: menuKeepOpenIconSize}}))
	send(icon, "setImageScaling:", 2) // NSImageScaleProportionallyDown
	send(icon, "setTag:", menuViewIcon)
	send(icon, "setHidden:", 1)
	send(view, "addSubview:", uintptr(icon))

	label := autorelease(msgInitRect(send(class("NSTextField"), "alloc"), sel("initWithFrame:"),
		NSRect{Origin: NSPoint{X: metrics.keepOpenLabelX, Y: 2},
			Size: NSSize{Width: width - metrics.keepOpenLabelX - 4, Height: menuKeepOpenHeight - 4}}))
	send(label, "setBezeled:", 0)
	send(label, "setEditable:", 0)
	send(label, "setSelectable:", 0)
	send(label, "setDrawsBackground:", 0)
	send(label, "setAlignment:", 0)
	send(label, "setFont:", uintptr(menuFont()))
	send(label, "setTag:", menuViewLabel)
	// Not width-sizable: the badge column sits to its right, and a wider
	// menu must not run the label under it.
	send(label, "setAutoresizingMask:", 0)
	send(view, "addSubview:", uintptr(label))

	badge := autorelease(msgInitRect(send(class("MyGoMenuBadge"), "alloc"), sel("initWithFrame:"), NSRect{}))
	send(badge, "setBezeled:", 0)
	send(badge, "setEditable:", 0)
	send(badge, "setSelectable:", 0)
	send(badge, "setDrawsBackground:", 0)
	send(badge, "setAlignment:", uintptr(nsTextAlignmentCenter))
	send(badge, "setFont:", uintptr(menuBadgeFont()))
	send(badge, "setTag:", menuViewBadge)
	send(badge, "setRefusesFirstResponder:", 1)
	send(badge, "setAutoresizingMask:", 1) // NSViewMinXMargin: keep to the right edge
	send(badge, "setAccessibilityElement:", 0)
	send(badge, "setWantsLayer:", 1)
	if layer := send(badge, "layer"); layer != 0 {
		send(layer, "setMasksToBounds:", 1)
	}
	send(badge, "setHidden:", 1)
	send(view, "addSubview:", uintptr(badge))

	const options = 0x01 | 0x80 | 0x200 // entered/exited, active always, visible rect
	tracking := msgInitMenuTrack(send(class("NSTrackingArea"), "alloc"),
		sel("initWithRect:options:owner:userInfo:"), NSRect{}, options, view, 0)
	send(view, "addTrackingArea:", uintptr(tracking))
	release(tracking)
	style := setKeepOpenBadge(view, it)
	layoutKeepOpenView(view, keepOpenItemTitle(it), itemBadge(it),
		metrics.keepOpenStart, metrics.keepOpenWidth, width, metrics.keepOpenLabelX, it.BadgeAlign, style)
	return view
}

// updateKeepOpenView follows the state of it in the view of a KeepOpen item.
func updateKeepOpenView(view id, it *platform.MenuItem, tab float64) {
	send(view, "setIdentifier:", uintptr(nsString(strconv.Itoa(it.ID))))
	style := setKeepOpenBadge(view, it)
	if label := send(view, "viewWithTag:", menuViewLabel); label != 0 {
		if style.shape != "" && itemBadge(it) != "" {
			send(label, "setAttributedStringValue:", uintptr(menuTitle(keepOpenItemTitle(it), "", 0, it.BadgeAlign)))
		} else {
			text := menuTitleStyled(keepOpenItemTitle(it), itemBadge(it), tab, it.BadgeAlign,
				menuBadgeStyle{fg: style.fg, hasFG: style.hasFG})
			send(label, "setAttributedStringValue:", uintptr(text))
		}
	}
	if icon := send(view, "viewWithTag:", menuViewIcon); icon != 0 {
		var image id
		if it.Icon != nil && menuImage != nil {
			image = menuImage(it.Icon)
		}
		send(icon, "setImage:", uintptr(image))
		send(icon, "setHidden:", boolArg(image == 0))
	}
	if check := send(view, "viewWithTag:", menuViewCheck); check != 0 {
		checked := it.Checked && (it.Type == platform.MenuItemCheckbox || it.Type == platform.MenuItemRadio)
		send(check, "setHidden:", boolArg(!checked))
	}
	alpha := 1.0
	if !it.Enabled {
		alpha = 0.45
	}
	msgSetFloat(view, sel("setAlphaValue:"), alpha)
	setMenuViewColors(view)
}

// layoutKeepOpenView places the label and badge of view in their columns
// and makes room for width, the width the menu's views share.
func layoutKeepOpenView(view id, label, badge string, start, badgeWidth, width, labelX float64, align platform.BadgeAlignment, style keepOpenBadgeStyle) {
	// A menu can be wider than its widest KeepOpen row, as a long item NSMenu
	// draws itself sets the width; AppKit then widens the view to fill it.
	// Lay out to the view's own width when that is wider, so the backdrop,
	// the label and the badge follow instead of stopping halfway.
	if f := msgRect(view, sel("frame")); f.Size.Width > width {
		width = f.Size.Width
	}
	if align == platform.BadgeAlignRight && badgeWidth > 0 {
		// Anchor the column to the view's right edge rather than to the
		// longest label: the room after the badge is then exactly
		// menuKeepOpenRight, and if the view turns out wider than measured
		// the badge moves with the edge instead of leaving a gap.
		start = max(start, width-menuKeepOpenRight-badgeWidth-labelX)
	}
	background := style.shape != "" && badge != ""
	if field := send(view, "viewWithTag:", menuViewLabel); field != 0 {
		fieldWidth := width - labelX - 4
		if background {
			send(field, "setAttributedStringValue:", uintptr(menuTitle(label, "", 0, align)))
			fieldWidth = max(start-4, 0)
		} else {
			tab := badgeTabFor(align, start, badgeWidth)
			text := menuTitleStyled(label, badge, tab, align, menuBadgeStyle{fg: style.fg, hasFG: style.hasFG})
			send(field, "setAttributedStringValue:", uintptr(text))
		}
		msgSetRect(field, sel("setFrame:"), NSRect{
			Origin: NSPoint{X: labelX, Y: 2},
			Size:   NSSize{Width: fieldWidth, Height: menuKeepOpenHeight - 4},
		})
	}
	if field := send(view, "viewWithTag:", menuViewBadge); field != 0 {
		if background {
			boxWidth := keepOpenBadgeWidth(badge, style.shape)
			msgSetRect(field, sel("setFrame:"), NSRect{
				Origin: NSPoint{X: labelX + start + keepOpenBadgeOffset(align, badgeWidth, boxWidth),
					Y: (menuKeepOpenHeight - menuBadgeHeight) / 2},
				Size: NSSize{Width: boxWidth, Height: menuBadgeHeight},
			})
			send(field, "setStringValue:", uintptr(nsString(badge)))
			send(field, "setTextColor:", uintptr(keepOpenBadgeTextColor(style)))
			if layer := send(field, "layer"); layer != 0 {
				send(layer, "setBackgroundColor:", uintptr(send(keepOpenBadgeColor(style), "CGColor")))
				msgSetFloat(layer, sel("setCornerRadius:"), keepOpenBadgeRadius(style.shape))
			}
			send(field, "setHidden:", 0)
		} else {
			send(field, "setHidden:", 1)
		}
	}
	// Only grow: AppKit sizes the view to the menu when it opens. Shrinking
	// it while the menu stays open would cut the backdrop and pull the
	// badges in as latencies change; the next open computes a fresh width.
	frame := msgRect(view, sel("frame"))
	if frame.Size.Width < width {
		frame.Size.Width = width
		msgSetRect(view, sel("setFrame:"), frame)
	}
	if backdrop := menuBackdrop(view); backdrop != 0 {
		msgSetRect(backdrop, sel("setFrame:"), NSRect{
			Origin: NSPoint{X: 4, Y: 1},
			Size:   NSSize{Width: width - 8, Height: menuKeepOpenHeight - 2},
		})
	}
}

// keepOpenBadgeOffset is where a badge box of box sits in a column of
// column, for its alignment.
func keepOpenBadgeOffset(align platform.BadgeAlignment, column, box float64) float64 {
	switch align {
	case platform.BadgeAlignLeft:
		return 0
	case platform.BadgeAlignCenter:
		return (column - box) / 2
	default:
		return column - box
	}
}

// keepOpenBadgeColor is the background of a pill.
func keepOpenBadgeColor(style keepOpenBadgeStyle) id {
	if style.hasBG {
		return nsColor(style.bg)
	}
	return send(class("NSColor"), "quaternaryLabelColor")
}

// keepOpenBadgeTextColor is the text color of a pill: the one asked for,
// or white or black against its background.
func keepOpenBadgeTextColor(style keepOpenBadgeStyle) id {
	if style.hasFG {
		return nsColor(style.fg)
	}
	if style.hasBG && style.bg.A >= 128 {
		name := "blackColor"
		if keepOpenColorDark(style.bg) {
			name = "whiteColor"
		}
		return send(class("NSColor"), name)
	}
	return send(class("NSColor"), "labelColor")
}

func keepOpenColorDark(c platform.Color) bool {
	l := (0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)) / 255
	return l < 0.5
}

// keepOpenTitle returns the label, badge and badge alignment a KeepOpen
// view shows.
func keepOpenTitle(view id) (label, badge string, align platform.BadgeAlignment) {
	field := send(view, "viewWithTag:", menuViewLabel)
	if field == 0 {
		return "", "", platform.BadgeAlignRight
	}
	align = platform.BadgeAlignRight
	var text string
	if value := send(field, "attributedStringValue"); value != 0 {
		text = goString(send(value, "string"))
		align = titleAlignment(value)
	} else {
		text = goString(send(field, "stringValue"))
	}
	if badgeField := send(view, "viewWithTag:", menuViewBadge); badgeField != 0 && !sendBool(badgeField, "isHidden") {
		if style := keepOpenBadges[view]; style.align != "" {
			align = style.align
		}
		return text, goString(send(badgeField, "stringValue")), align
	}
	if i := strings.IndexByte(text, '\t'); i >= 0 {
		return text[:i], text[i+1:], align
	}
	return text, "", align
}

// menuBackdrop returns the selection layer of a KeepOpen view.
func menuBackdrop(view id) id {
	sublayers := send(send(view, "layer"), "sublayers")
	if sublayers == 0 {
		return 0
	}
	layers := arrayItems(sublayers)
	if len(layers) == 0 {
		return 0
	}
	return layers[0]
}

// setMenuViewHighlighted shows the selection of a KeepOpen view under the
// mouse or the keyboard, as NSMenu does for the items it draws itself.
func setMenuViewHighlighted(view id, on bool) {
	if backdrop := menuBackdrop(view); backdrop != 0 {
		if on {
			color := send(class("NSColor"), "selectedContentBackgroundColor")
			send(backdrop, "setBackgroundColor:", uintptr(send(color, "CGColor")))
		}
		send(backdrop, "setHidden:", boolArg(!on))
	}
	setMenuViewColors(view)
}

// setMenuViewColors colors the label of a KeepOpen view for its state.
func setMenuViewColors(view id) {
	label := send(view, "viewWithTag:", menuViewLabel)
	if label == 0 {
		return
	}
	name := "labelColor"
	if backdrop := menuBackdrop(view); backdrop != 0 && !sendBool(backdrop, "isHidden") {
		name = "alternateSelectedControlTextColor"
	} else if item := send(view, "enclosingMenuItem"); item != 0 && !sendBool(item, "isEnabled") {
		name = "disabledControlTextColor"
	}
	send(label, "setTextColor:", uintptr(send(class("NSColor"), name)))
	setMenuBadgeColors(view)
}

// setMenuBadgeColors follows the appearance in the badge of a KeepOpen
// view: its default background and text color are dynamic.
func setMenuBadgeColors(view id) {
	field := send(view, "viewWithTag:", menuViewBadge)
	if field == 0 || sendBool(field, "isHidden") {
		return
	}
	style := keepOpenBadges[view]
	if layer := send(field, "layer"); layer != 0 && !style.hasBG {
		color := send(send(class("NSColor"), "quaternaryLabelColor"), "CGColor")
		send(layer, "setBackgroundColor:", uintptr(color))
	}
	send(field, "setTextColor:", uintptr(keepOpenBadgeTextColor(style)))
}

// menuImage is iconImage once LinkMenuIcons links it, when the app first
// gives a menu item an icon.
var menuImage func(ic *platform.MenuIcon) id

// LinkMenuIcons links the code of the icons of menu items.
func LinkMenuIcons() { menuImage = iconImage }

// iconImage returns the NSImage of an icon, made once and released once
// no snapshot holds the icon: AppKit draws it at 16 points, keeping its
// aspect, from the representation fit for the display (an image of 32
// pixels for Retina), and tints a template image as the menu's text, white
// on the highlight. Its menu items show their state (the check) beside it.
func iconImage(ic *platform.MenuIcon) id {
	img, _ := ic.Native(func() any {
		img := send(send(class("NSImage"), "alloc"), "initWithData:", uintptr(nsData(ic.PNG)))
		if img == 0 {
			return nil
		}
		size := msgSize(img, sel("size"))
		if box := float64(platform.MenuIconSize); size.Width > 0 && size.Height > 0 {
			scale := box / max(size.Width, size.Height)
			msgSetSize(img, sel("setSize:"), NSSize{size.Width * scale, size.Height * scale})
		}
		send(img, "setTemplate:", boolArg(ic.Template))
		// NSImage may be released on any thread.
		runtime.AddCleanup(ic, func(img id) { release(img) }, img)
		return img
	}).(id)
	return img
}

func (b *Backend) UpdateMenuItem(it *platform.MenuItem) {
	withPool(func() {
		key, mods := keyEquivalent(it.Accelerator)
		for _, n := range itemsByID[it.ID] {
			// The hidden Cmd+= twin of "Zoom In" only follows the enabled state.
			if it.Role == "zoomIn" && sendBool(n.item, "isHidden") && respondsTo(n.item, "allowsKeyEquivalentWhenHidden") &&
				sendBool(n.item, "allowsKeyEquivalentWhenHidden") {
				send(n.item, "setEnabled:", boolArg(it.Enabled))
				continue
			}
			var view id
			if it.Type != platform.MenuItemSeparator {
				if sub := send(n.item, "submenu"); sub != 0 {
					send(sub, "setTitle:", uintptr(nsString(it.Label)))
				}
				send(n.item, "setKeyEquivalent:", uintptr(nsString(key)))
				send(n.item, "setKeyEquivalentModifierMask:", uintptr(mods))
				view = send(n.item, "view")
			}
			applyItemState(n.item, it)
			if it.Type != platform.MenuItemSeparator {
				setMenuBadgeStyle(n.item, it)
			}
			switch {
			case it.Type == platform.MenuItemSeparator:
			case view != 0:
				updateKeepOpenView(view, it, 0)
				retabKeepOpenMenu(n.item, keepOpenItemTitle(it), itemBadge(it), it.BadgeAlign)
			default:
				// The menu's widest title and badge may be this item's now.
				retabMenu(n.item, itemTitle(it), itemBadge(it), it.BadgeAlign)
			}
		}
	})
}

func (b *Backend) PopupMenu(m *platform.Menu, pw platform.Window, pos *platform.Point) {
	withPool(func() {
		owner := newOwner()
		defer dropOwner(owner)
		menu := b.buildMenu(m, "", owner)
		var view id
		var loc NSPoint
		var content *surface
		if w, ok := pw.(*window); ok && w != nil && !w.closed {
			content = w.surface
			view = w.web
			if w.surface != nil {
				view = w.surface.view
			}
			if pos != nil {
				loc = NSPoint{float64(pos.X), float64(pos.Y)}
				if !sendBool(view, "isFlipped") {
					loc.Y = msgRect(view, sel("bounds")).Size.Height - loc.Y
				}
			} else {
				p := msgPoint(w.win, sel("mouseLocationOutsideOfEventStream"))
				loc = msgPointFromView(view, sel("convertPoint:fromView:"), p, 0)
			}
		} else {
			loc = msgPoint(class("NSEvent"), sel("mouseLocation"))
		}
		if m.RightToLeft {
			// NSUserInterfaceLayoutDirectionRightToLeft lays the items out
			// from the right and opens submenus to the left. A menu popped
			// up in a view takes the view's direction, which the surface
			// has only while the menu shows.
			send(menu, "setUserInterfaceLayoutDirection:", 1)
			if view != 0 {
				was := sendInt(view, "userInterfaceLayoutDirection")
				send(view, "setUserInterfaceLayoutDirection:", 1)
				defer send(view, "setUserInterfaceLayoutDirection:", uintptr(was))
			}
		}
		// A menu shown while a button is down takes its release, in its
		// own loop: the content's press ends with none.
		held := content != nil && send(class("NSEvent"), "pressedMouseButtons") != 0
		msgPopUpMenu(menu, sel("popUpMenuPositioningItem:atLocation:inView:"), 0, loc, view)
		if held && b.surfaceOf(content.view) == content {
			content.send(platform.SurfaceEvent{Kind: platform.PointerCancel})
		}
	})
}

// keyEquivalent converts an accelerator to an NSMenuItem key equivalent and
// modifier mask.
func keyEquivalent(acc string) (string, uint) {
	if acc == "" {
		return "", 0
	}
	a, err := accelerator.Parse(acc, "darwin")
	if err != nil {
		return "", 0
	}
	var mods uint
	if a.Has(accelerator.Super) {
		mods |= 1 << 20
	}
	if a.Has(accelerator.Ctrl) {
		mods |= 1 << 18
	}
	if a.Has(accelerator.Alt) {
		mods |= 1 << 19
	}
	if a.Has(accelerator.Shift) {
		mods |= 1 << 17
	}
	if len(a.Key) == 1 {
		return a.Key, mods
	}
	named := map[string]rune{
		"Enter": '\r', "Tab": '\t', "Space": ' ', "Backspace": 0x08, "Delete": 0xF728, "Escape": 0x1b,
		"Up": 0xF700, "Down": 0xF701, "Left": 0xF702, "Right": 0xF703, "Home": 0xF729, "End": 0xF72B,
		"PageUp": 0xF72C, "PageDown": 0xF72D, "Insert": 0xF727, "PrintScreen": 0xF72E,
	}
	if r, ok := named[a.Key]; ok {
		return string(r), mods
	}
	var n int
	if len(a.Key) > 1 && a.Key[0] == 'F' {
		for _, c := range a.Key[1:] {
			n = n*10 + int(c-'0')
		}
		return string(rune(0xF704 + n - 1)), mods
	}
	return "", 0
}

// Tray icons (NSStatusItem).

type tray struct {
	b      *Backend
	h      platform.TrayHandler
	item   id
	button id
	owner  int
	// dark is the menu bar's appearance, as last told.
	dark bool
	// mono draws the title in the monospaced font (SetTitleMonospaced).
	mono bool
	// clicked is where on screen the copy of the item that the user last
	// clicked is, and of the frame the item's window had then. macOS 26
	// shows a menu bar item on each display from one window, which is on
	// one of them, or out of sight on a display whose menu bar has no room
	// for it; the copies share its size. Zero until a click.
	clicked, of NSRect
}

// secondaryClick reports whether a click on a status item, ending with an
// event of type typ and modifier flags, is a right-click: the right button's,
// or the primary button's with Control held, as a Control-click is a
// secondary click across macOS, and in windows of native UI.
func secondaryClick(typ, flags uint) bool {
	const (
		leftMouseUp  = 2       // NSEventTypeLeftMouseUp
		rightMouseUp = 4       // NSEventTypeRightMouseUp
		control      = 1 << 18 // NSEventModifierFlagControl
	)
	return typ == rightMouseUp || typ == leftMouseUp && flags&control != 0
}

func registerTrayTarget() {
	classDef("MyGoTrayTarget", "NSObject", nil, []objc.MethodDef{
		method("mygoTrayClicked:", func(self id, _ objc.SEL, sender id) {
			t := theBackend.trays[sender]
			if t == nil {
				return
			}
			ev := send(theBackend.app, "currentEvent")
			t.noteClick(ev)
			if ev != 0 && secondaryClick(uint(sendInt(ev, "type")), uint(send(ev, "modifierFlags"))) {
				t.h.RightClicked()
			} else {
				t.h.Clicked()
			}
		}),
		// The menu bar under the button turned dark or light: with the
		// system's appearance, or the wallpaper under a translucent menu
		// bar.
		method("observeValueForKeyPath:ofObject:change:context:", func(self id, _ objc.SEL, keyPath, object, change id, ctx uintptr) {
			t := theBackend.trays[object]
			if t == nil {
				return
			}
			if dark := t.Dark(); dark != t.dark {
				t.dark = dark
				t.h.AppearanceChanged()
			}
		}),
	})
}

var trayTarget id

func (b *Backend) NewTray(_ string, h platform.TrayHandler) (platform.Tray, error) {
	if trayTarget == 0 {
		trayTarget = alloc("MyGoTrayTarget")
	}
	t := &tray{b: b, h: h, owner: newOwner()}
	withPool(func() {
		bar := send(class("NSStatusBar"), "systemStatusBar")
		t.item = retain(msgFloatID(bar, sel("statusItemWithLength:"), -1)) // NSVariableStatusItemLength
		t.button = send(t.item, "button")
		send(t.button, "setTarget:", uintptr(trayTarget))
		send(t.button, "setAction:", uintptr(sel("mygoTrayClicked:")))
		send(t.button, "sendActionOn:", 1<<2|1<<4) // left and right mouse up
		t.setTitleFont()
		send(t.button, "addObserver:forKeyPath:options:context:", uintptr(trayTarget), uintptr(nsString("effectiveAppearance")), 0, 0)
	})
	b.trays[t.button] = t
	t.dark = t.Dark()
	return t, nil
}

// Dark reports whether the menu bar under the button is dark, as AppKit
// draws its own status items: by the button's appearance, which follows
// the menu bar's.
func (t *tray) Dark() bool {
	if t.item == 0 {
		return false
	}
	dark := false
	withPool(func() {
		names := nsArray(nsString("NSAppearanceNameAqua"), nsString("NSAppearanceNameDarkAqua"))
		match := send(send(t.button, "effectiveAppearance"), "bestMatchFromAppearancesWithNames:", uintptr(names))
		dark = match != 0 && goString(match) == "NSAppearanceNameDarkAqua"
	})
	return dark
}

// SetID gives the status item an autosave name: macOS keeps under it where
// the user dragged the item with ⌘, and puts it there as it is made again.
func (t *tray) SetID(id string) {
	withPool(func() { send(t.item, "setAutosaveName:", uintptr(nsString(id))) })
}

func (t *tray) SetImage(png []byte, template bool) error {
	var err error
	withPool(func() {
		img := autorelease(send(send(class("NSImage"), "alloc"), "initWithData:", uintptr(nsData(png))))
		if img == 0 {
			err = errInvalidImage
			return
		}
		// Menu bar icons are 18 points tall.
		size := msgSize(img, sel("size"))
		if size.Height > 0 {
			msgSetSize(img, sel("setSize:"), NSSize{size.Width * 18 / size.Height, 18})
		}
		send(img, "setTemplate:", boolArg(template))
		send(t.button, "setImage:", uintptr(img))
	})
	return err
}

func (t *tray) SetTitle(title string) {
	// The monospaced font has no figure space: AppKit would draw one of
	// another font, narrower than the digits it stands for.
	if t.mono {
		title = strings.ReplaceAll(title, "\u2007", " ")
	}
	withPool(func() { send(t.button, "setTitle:", uintptr(nsString(title))) })
}

// SetTitleMonospaced draws the titles set from here on in the system's
// monospaced font, or in its font again.
func (t *tray) SetTitleMonospaced(on bool) {
	t.mono = on
	withPool(t.setTitleFont)
}

// setTitleFont gives the button the font of its title, at the menu bar
// font's size and weight: the system's with its digits at one width, so a
// changing number (a rate, a timer) doesn't resize the item and shift the
// icon and the items beside it, or the system's monospaced one, where
// letters and punctuation have that width too. Set on the button rather
// than in an attributed title, it keeps AppKit's colors for light and dark
// bars and the highlight.
func (t *tray) setTitleFont() {
	name := "monospacedDigitSystemFontOfSize:weight:"
	if t.mono {
		name = "monospacedSystemFontOfSize:weight:"
	}
	size := msgFloat(msgFloatID(class("NSFont"), sel("menuBarFontOfSize:"), 0), sel("pointSize"))
	if font := msgFloat2ID(class("NSFont"), sel(name), size, 0); font != 0 { // NSFontWeightRegular
		send(t.button, "setFont:", uintptr(font))
	}
}

func (t *tray) SetToolTip(tip string) {
	withPool(func() { send(t.button, "setToolTip:", uintptr(nsString(tip))) })
}

func (t *tray) SetMenu(m *platform.Menu) {
	withPool(func() {
		dropOwner(t.owner)
		if m == nil {
			send(t.item, "setMenu:", 0)
			return
		}
		send(t.item, "setMenu:", uintptr(t.b.buildMenu(m, "", t.owner)))
	})
}

// SetActivatable does nothing: AppKit reports its clicks itself.
func (t *tray) SetActivatable(bool) {}

func (t *tray) PopUpMenu(m *platform.Menu) {
	if m == nil || t.item == 0 {
		return
	}
	withPool(func() {
		owner := newOwner()
		defer dropOwner(owner)
		menu := t.b.buildMenu(m, "", owner)
		// Let the status item place and track its menu, including the
		// button's highlight. Keep its previous menu alive while swapped.
		previous := retain(send(t.item, "menu"))
		defer release(previous)
		send(t.item, "setMenu:", uintptr(menu))
		send(t.button, "performClick:", 0)
		// Menu actions run in a nested event loop: they may destroy the
		// tray or install another menu, which we must leave intact.
		if t.item != 0 && send(t.item, "menu") == menu {
			send(t.item, "setMenu:", uintptr(previous))
		}
	})
}

func (t *tray) Bounds() platform.Rect {
	if send(t.button, "window") == 0 {
		return platform.Rect{}
	}
	return rectFromMac(t.frame())
}

// noteClick keeps where on screen the copy of the item that the click ev was
// on is: the pointer's place, less the click's place in the item's window,
// which the copies share. An event in another window, which an action that no
// click sent leaves current, changes nothing.
func (t *tray) noteClick(ev id) {
	win := send(t.button, "window")
	if ev == 0 || win == 0 || send(ev, "window") != win {
		return
	}
	of := msgRect(win, sel("frame"))
	t.clicked = clickedCopy(msgPoint(class("NSEvent"), sel("mouseLocation")), msgPoint(ev, sel("locationInWindow")), of.Size)
	t.of = of
}

// clickedCopy is the frame on screen of the copy of a menu bar item of size
// that a click hit at at, a point on screen, and at in, its place in the
// item's window.
func clickedCopy(at, in NSPoint, size NSSize) NSRect {
	return NSRect{Origin: NSPoint{X: at.X - in.X, Y: at.Y - in.Y}, Size: size}
}

// itemFrame is where a menu bar item shows, given its window's frame: the copy
// the user last clicked, while the window keeps the frame it had then, or else
// the window's frame. A window that moved, as the menu bar was laid out again,
// may have moved its copies too.
func itemFrame(window, clicked, of NSRect) NSRect {
	if clicked.Size.Width > 0 && window == of {
		return clicked
	}
	return window
}

// frame is where the item shows on screen (itemFrame), in Cocoa's
// coordinates; zero without a window.
func (t *tray) frame() NSRect {
	win := send(t.button, "window")
	if win == 0 {
		return NSRect{}
	}
	return itemFrame(msgRect(win, sel("frame")), t.clicked, t.of)
}

func (t *tray) Destroy() {
	if t.item == 0 {
		return
	}
	delete(t.b.trays, t.button)
	send(t.button, "removeObserver:forKeyPath:", uintptr(trayTarget), uintptr(nsString("effectiveAppearance")))
	dropOwner(t.owner)
	send(send(class("NSStatusBar"), "systemStatusBar"), "removeStatusItem:", uintptr(t.item))
	release(t.item)
	t.item = 0
}
