//go:build windows && (amd64 || arm64)

package windows

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"unicode/utf16"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

// A window's thumbnail toolbar (ThumbBarWindow): the row of small buttons
// under the taskbar button's live preview. ThumbBarAddButtons fixes the
// buttons' count and order once and for all, and later changes go through
// ThumbBarUpdateButtons, so the buttons and their PNGs are kept on the
// window: Explorer sends TaskbarButtonCreated again when it restarts, and
// the toolbar is added again from the PNGs kept here, at the window's DPI.
// Until the taskbar button exists nothing reaches the shell; the buttons
// wait for TaskbarButtonCreated.

// thumbSet, thumbApply and thumbClicked are the toolbar's code, linked when
// the app first sets one (LinkThumbBars): SetThumbBar, applyTaskbar and the
// window proc run in every app, and only call them.
var (
	thumbSet     func(*window, []platform.ThumbButton) error
	thumbApply   func(*window)
	thumbClicked func(*window, uint16)
)

// LinkThumbBars links the code of the thumbnail toolbar, when the app
// first sets one.
func LinkThumbBars() {
	thumbSet = (*window).setThumbBar
	thumbApply = (*window).applyThumbBar
	thumbClicked = (*window).thumbButtonClicked
}

// More ITaskbarList3 vtable indices and a thumbnail toolbar button's
// masks, flags and notification (ShObjIdl_core.h).
const (
	tbThumbBarAddButtons    = 15
	tbThumbBarUpdateButtons = 16

	thbIcon    = 0x2
	thbToolTip = 0x4
	thbFlags   = 0x8

	thbfDisabled = 0x1
	thbfHidden   = 0x8

	// thbnClicked arrives in the high word of WM_COMMAND's wParam, with
	// the button's id (its index) in its low word.
	thbnClicked = 0x1800
)

// thumbButton is THUMBBUTTON (ShObjIdl_core.h): Go places the fields where
// C does on 64-bit Windows, with the icon 8-aligned after the bitmap.
type thumbButton struct {
	mask   uint32
	id     uint32
	bitmap uint32
	_      uint32
	icon   uintptr
	tip    [260]uint16
	flags  uint32
	_      uint32
}

// setTip copies a tooltip into szTip, which holds 259 runes and a zero,
// replacing what was there.
func (b *thumbButton) setTip(tip string) {
	s := utf16.Encode([]rune(tip))
	if len(s) > 259 {
		s = s[:259]
	}
	clear(b.tip[:])
	copy(b.tip[:], s)
}

// setFlags puts the state of a button of the core in dwFlags.
func (b *thumbButton) setFlags(disabled, hidden bool) {
	b.flags = 0
	if disabled {
		b.flags = thbfDisabled
	}
	if hidden {
		b.flags |= thbfHidden
	}
}

// SetThumbBar sets the window's thumbnail toolbar; its body is thumbSet,
// so apps without one don't carry it.
func (w *window) SetThumbBar(buttons []platform.ThumbButton) error {
	return thumbSet(w, buttons)
}

// setThumbBar keeps the buttons and, when the shell has the taskbar button,
// sends them: the first time with ThumbBarAddButtons, then with
// ThumbBarUpdateButtons, which only takes the icons that changed. The shell's
// refusal is returned and the previous buttons stay.
func (w *window) setThumbBar(buttons []platform.ThumbButton) error {
	prev := w.thumb.buttons
	if len(prev) != 0 && len(buttons) != len(prev) {
		return errors.New("mygo: the buttons of a thumbnail toolbar cannot change once they are set")
	}
	var shell []platform.ThumbButton // the buttons the shell has
	if w.thumb.live {
		shell = prev
	}
	// Build the icons even when nothing reaches the shell yet, so a bad
	// PNG fails here and not on TaskbarButtonCreated.
	icons, err := w.makeThumbIcons(buttons, shell)
	if err != nil {
		return err
	}
	defer destroyIcons(icons)
	w.thumb.buttons = slices.Clone(buttons)
	tl := w.shellThumbBar()
	if tl == 0 {
		return nil
	}
	which := tbThumbBarAddButtons
	if w.thumb.live {
		which = tbThumbBarUpdateButtons
	}
	if err := w.sendThumbButtons(tl, which, icons); err != nil {
		w.thumb.buttons = prev
		return err
	}
	w.thumb.live = true
	return nil
}

// shellThumbBar returns the ITaskbarList3 to send the toolbar to, or 0 when
// the window has no taskbar button now: it is skipped, or Explorer has not
// sent TaskbarButtonCreated yet.
func (w *window) shellThumbBar() uintptr {
	if !w.taskbarButton || w.skipTaskbar {
		return 0
	}
	return w.b.taskbar()
}

// applyThumbBar adds the toolbar to the window's taskbar button, at the
// window's DPI. Called whenever Explorer creates the button
// (TaskbarButtonCreated), which has no toolbar yet.
func (w *window) applyThumbBar() {
	w.thumb.live = false
	tl := w.shellThumbBar()
	if len(w.thumb.buttons) == 0 || tl == 0 {
		return
	}
	icons, err := w.makeThumbIcons(w.thumb.buttons, nil)
	if err != nil {
		return
	}
	defer destroyIcons(icons)
	w.thumb.live = w.sendThumbButtons(tl, tbThumbBarAddButtons, icons) == nil
}

// sendThumbButtons builds the THUMBBUTTON array from w.thumb.buttons and
// passes it to which, the ITaskbarList3 method AddButtons or UpdateButtons,
// on tl, and returns the shell's refusal. Entries without an HICON leave
// thbIcon out so updates keep the existing icon.
func (w *window) sendThumbButtons(tl uintptr, which int, icons []uintptr) error {
	bs := make([]thumbButton, len(w.thumb.buttons))
	for i, b := range w.thumb.buttons {
		bs[i] = thumbButton{mask: thbToolTip | thbFlags, id: uint32(i)}
		if icons != nil && icons[i] != 0 {
			bs[i].mask |= thbIcon
			bs[i].icon = icons[i]
		}
		bs[i].setTip(b.ToolTip)
		bs[i].setFlags(b.Disabled, b.Hidden)
	}
	if hr := comCall(tl, which, w.hwnd, uintptr(len(bs)), uintptr(unsafe.Pointer(&bs[0]))); failed(hr) {
		return fmt.Errorf("mygo: the shell refused the thumbnail toolbar: HRESULT %#08x", uint32(hr))
	}
	return nil
}

// makeThumbIcons builds button HICONs at the window's DPI from button PNGs,
// leaving a zero slot for each button whose PNG is the one the shell has
// (shell[i]); with no shell buttons it builds every icon. If it builds none
// it returns nil. ThumbBarAddButtons and ThumbBarUpdateButtons copy each
// HICON, so the caller destroys the returned handles afterwards.
func (w *window) makeThumbIcons(buttons, shell []platform.ThumbButton) ([]uintptr, error) {
	var icons []uintptr
	var size int
	for i, b := range buttons {
		if i < len(shell) && bytes.Equal(b.Icon, shell[i].Icon) {
			continue
		}
		if icons == nil {
			icons = make([]uintptr, len(buttons))
			size = iconSize(smCxIcon, 32, dpiOf(w.hwnd))
		}
		icon, err := iconFromPNG(b.Icon, size)
		if err != nil {
			destroyIcons(icons)
			return nil, err
		}
		icons[i] = icon
	}
	return icons, nil
}

// destroyIcons releases the HICONs the shell copied from.
func destroyIcons(icons []uintptr) {
	for _, icon := range icons {
		if icon != 0 {
			procDestroyIcon.Call(icon)
		}
	}
}

// thumbButtonClicked reports a click of the thumbnail toolbar button whose
// id the shell sent.
func (w *window) thumbButtonClicked(id uint16) {
	if h, ok := w.h.(platform.ThumbBarHandler); ok {
		h.ThumbButtonClicked(int(id))
	}
}
