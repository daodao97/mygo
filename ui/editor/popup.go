package editor

import "github.com/egoist/mygo/ui"

// popup is what ShowPopup shows: the app's panel below the caret at an
// offset, which takes the keyboard while it is open.
type popup struct {
	shown bool
	at    int
	build func(c *ui.Context, panel ui.Element)
	// version is the text's as it opened: an edit closes it, as what it
	// listed, the usages of a word, are of the text before it.
	version uint64
	// focused reports the keyboard in the popup, which goes back to the
	// editor as the popup closes.
	focused bool
}

// ShowPopup shows a panel of the app's below the caret at offset, above
// the window's content, as the usages or the implementations of the word
// there a go to finds, to choose one from: build styles and fills the
// panel. The panel takes the keyboard as it opens, for its own keys, as
// the arrows moving through a list; Escape, a press outside it, an edit
// or ClosePopup closes it, and gives the keyboard back to the editor.
// Showing another replaces it. An offset outside the text is taken as its
// nearest end, as Rect does.
func (st *State) ShowPopup(offset int, build func(c *ui.Context, panel ui.Element)) {
	st.ensure()
	p := &st.ed.popup
	offset = max(0, min(offset, st.ed.view.text.Len()))
	p.shown, p.at, p.build, p.version = true, offset, build, st.ed.view.version
}

// ClosePopup closes what ShowPopup shows, as a choice in it is made.
func (st *State) ClosePopup() {
	st.ensure()
	st.ed.closePopup()
}

// PopupShown reports whether what ShowPopup shows is open.
func (st *State) PopupShown() bool { return st.ed.popup.shown }

func (ed *editor) closePopup() {
	if !ed.popup.shown {
		return
	}
	if ed.popup.focused {
		ed.refocus = true
	}
	ed.popup = popup{}
}

// buildPopup places the app's popup below the caret it opened at, above
// the window's content, and builds it.
func (ed *editor) buildPopup(c *ui.Context, anchor ui.Element) {
	p := &ed.popup
	ui.Overlay(c, func() {
		panel := ui.Box(c).Key("code popup").AttachTo(anchor, ui.AnchorBottomLeft, ui.AnchorTopLeft).
			Focusable().FocusRing(false).AutoFocus().Cursor(ui.CursorDefault).HandleInput(shield)
		p.focused = panel.FocusWithin()
		if panel.Shortcut(0, ui.KeyEscape) || panel.PressedOutside() {
			ed.closePopup()
			c.Invalidate()
			return
		}
		build := p.build
		build(c, panel)
	})
}
