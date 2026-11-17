// Package keyboardbar draws a row of keyboard actions, as a terminal's Tab,
// Esc and Ctrl keys, with native UI's buttons, for the bottom of a view
// above a phone's software keyboard. It keeps the focus of the input in use
// and the composition in it, so the keyboard stays up and an input method's
// marked text survives a tap. ModifierLatch keeps the state of software
// modifier keys for ui.Element.InputModifiers.
//
// Phones lay a window's content out above the keyboard while it shows, so
// a Bar drawn last in a column that fills the window sits on top of it.
package keyboardbar

import "github.com/egoist/mygo/ui"

// Action is a button of the bar. Label is its accessible name, and its text
// unless Cap is set: Cap is what the key shows instead, as ⌫ or ⏎.
// Items makes the button open a panel of further actions above the row
// instead of sending its ID. LongPressID is sent instead of ID after a long
// press. Selected and Locked show app-owned modifier state.
type Action struct {
	ID, Label   string
	Cap         string
	LongPressID string
	Selected    bool
	Locked      bool
	Items       []Action
}

// columns is how many keys a row of the panel holds: six fit even a
// 320-point phone at 44 points per tap target.
const columns = 6

// Bar draws actions as a row of buttons that keep the input's focus, and
// handler receives the ID of the one tapped, or its LongPressID. An action
// with Items toggles *expanded, which shows every such action's items above
// the row, six to a row. Draw it while the input has the focus.
func Bar(c *ui.Context, actions []Action, expanded *bool, handler func(string)) ui.Element {
	t := c.Theme()
	button := func(a Action) {
		b := ui.ButtonBase(c).Label(a.Label).Role(ui.RoleButton).KeepFocus().Height(44).MinWidth(44).Grow(1).Shrink(1).Radius(8)
		if b.Pressed() || a.Selected || len(a.Items) > 0 && *expanded {
			b.Background(t.SurfacePressed)
		}
		b.Children(func() {
			if a.Cap != "" {
				// A symbol reads at the size of the keyboard's own.
				ui.Text(c, a.Cap).FontSize(17)
			} else {
				ui.Text(c, a.Label).FontSize(13)
			}
		})
		if a.LongPressID != "" && b.LongPressed() {
			handler(a.LongPressID)
		}
		if b.Clicked() {
			if len(a.Items) > 0 {
				*expanded = !*expanded
			} else {
				handler(a.ID)
			}
			c.Invalidate()
		}
	}
	row := func(actions []Action, pad bool) {
		ui.Row(c).FillWidth().Gap(2).Children(func() {
			for _, a := range actions {
				button(a)
			}
			// A short row of the panel keeps the full rows' key widths.
			if pad {
				for range max(0, columns-len(actions)) {
					ui.Box(c).Grow(1).Shrink(1).MinWidth(44)
				}
			}
		})
	}
	return ui.Column(c).FillWidth().Padding(0, 8).Background(t.Surface).Children(func() {
		if *expanded {
			for _, a := range actions {
				for i := 0; i < len(a.Items); i += columns {
					row(a.Items[i:min(i+columns, len(a.Items))], true)
				}
			}
		}
		row(actions, false)
	})
}

// ModifierLatch is the state of software modifier keys, as on a terminal's
// Ctrl key: a tap arms a modifier for the next key, a long press locks it
// until tapped again. Pass Active to ui.Element.InputModifiers with Consume
// as its callback, and Clear when the input session ends. The zero value has
// nothing armed.
type ModifierLatch struct{ armed, locked ui.Modifiers }

// Active returns the modifiers for the next key, locked ones included.
func (l *ModifierLatch) Active() ui.Modifiers { return l.armed }

// Locked returns the modifiers that stay after a key.
func (l *ModifierLatch) Locked() ui.Modifiers { return l.locked }

// Tap arms m for the next key, or releases it when armed or locked.
func (l *ModifierLatch) Tap(m ui.Modifiers) {
	if l.armed&m != 0 {
		l.armed &^= m
		l.locked &^= m
	} else {
		l.armed |= m
	}
}

// Lock keeps m for every key until it is tapped.
func (l *ModifierLatch) Lock(m ui.Modifiers) { l.armed |= m; l.locked |= m }

// Consume releases the modifiers armed for one key, keeping locked ones.
func (l *ModifierLatch) Consume() { l.armed = l.locked }

// Clear releases every modifier, reporting whether any was armed.
func (l *ModifierLatch) Clear() bool {
	changed := l.armed != 0 || l.locked != 0
	l.armed, l.locked = 0, 0
	return changed
}

// Decorate returns a copy of actions, items included, whose Selected and
// Locked show the latch for the actions modifier maps to a modifier. The
// definitions passed in are left unchanged.
func (l *ModifierLatch) Decorate(actions []Action, modifier func(id string) ui.Modifiers) []Action {
	out := append([]Action(nil), actions...)
	for i := range out {
		if m := modifier(out[i].ID); m != 0 {
			out[i].Selected, out[i].Locked = l.armed&m != 0, l.locked&m != 0
		}
		if len(out[i].Items) > 0 {
			out[i].Items = l.Decorate(out[i].Items, modifier)
		}
	}
	return out
}
