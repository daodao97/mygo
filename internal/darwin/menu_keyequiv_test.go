//go:build darwin && !ios

package darwin

import "testing"

// shortcutMatches reports whether a menu holding one item with the key
// equivalent of acc chooses it for a Command key press of chars, with
// Shift held or not. The characters of a shifted press are shifted, as
// AppKit gives them.
func shortcutMatches(acc, chars string, shift bool) bool {
	matched := false
	withPool(func() {
		send(class("NSApplication"), "sharedApplication")
		key, mods := keyEquivalent(acc)
		// The item's action empties the array.
		target := autorelease(send(send(class("NSMutableArray"), "alloc"), "init"))
		send(target, "addObject:", uintptr(nsString("x")))
		menu := autorelease(send(send(class("NSMenu"), "alloc"), "initWithTitle:", uintptr(nsString("Test"))))
		item := autorelease(send(send(class("NSMenuItem"), "alloc"), "initWithTitle:action:keyEquivalent:",
			uintptr(nsString("Item")), uintptr(sel("removeAllObjects")), uintptr(nsString(key))))
		send(item, "setKeyEquivalentModifierMask:", uintptr(mods))
		send(item, "setTarget:", uintptr(target))
		send(menu, "addItem:", uintptr(item))
		flags := uint(1 << 20) // NSEventModifierFlagCommand
		if shift {
			flags |= 1 << 17 // NSEventModifierFlagShift
		}
		ev := msgKeyEvent(class("NSEvent"), sel("keyEventWithType:location:modifierFlags:timestamp:windowNumber:context:characters:charactersIgnoringModifiers:isARepeat:keyCode:"),
			10 /* NSEventTypeKeyDown */, NSPoint{}, flags, 0, 0, 0, nsString(chars), nsString(chars), false, 0)
		sendBool(menu, "performKeyEquivalent:", uintptr(ev))
		matched = sendInt(target, "count") == 0
	})
	return matched
}

// TestShiftedKeyEquivalent checks that an item of a shortcut with Shift and
// a letter is chosen for the shifted press alone, not for the letter with
// the other modifiers (#2257), and that one without Shift is not chosen
// for the shifted press.
func TestShiftedKeyEquivalent(t *testing.T) {
	load()
	for _, c := range []struct {
		acc   string
		chars string
		shift bool
		want  bool
	}{
		{"CmdOrCtrl+Shift+W", "w", false, false},
		{"CmdOrCtrl+Shift+W", "W", true, true},
		{"CmdOrCtrl+W", "w", false, true},
		{"CmdOrCtrl+W", "W", true, false},
		{"Shift+CmdOrCtrl+Z", "z", false, false},
		{"Shift+CmdOrCtrl+Z", "Z", true, true},
	} {
		if got := shortcutMatches(c.acc, c.chars, c.shift); got != c.want {
			t.Errorf("%s pressed as %q (Shift %v): chosen %v, want %v", c.acc, c.chars, c.shift, got, c.want)
		}
	}
}
