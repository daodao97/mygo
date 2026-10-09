package ui

import "encoding/json"

// InputAction is a software-keyboard accessory button. Symbol optionally
// names a system symbol; Label remains its accessible name. Items makes a
// button expand a panel of secondary actions without dismissing the keyboard.
type InputAction struct {
	ID     string
	Label  string
	Symbol string `json:",omitempty"`
	// LongPressID is sent instead of ID after a long press. Selected and
	// Locked describe app-owned modifier state; locking never generates a tap.
	LongPressID string        `json:",omitempty"`
	Selected    bool          `json:",omitempty"`
	Locked      bool          `json:",omitempty"`
	Items       []InputAction `json:",omitempty"`
}

// InputAccessory attaches actions above the iOS system keyboard while this
// input has focus. Other platforms ignore the native controls. The handler
// runs on the UI thread without changing text focus or committing marked text.
// Omit the call on later frames to remove the accessory.
func (e *node) InputAccessory(actions []InputAction, handler func(string)) *node {
	if len(actions) > 0 {
		data, _ := json.Marshal(actions)
		e.inputAccessory = string(data)
		e.inputAction = handler
	}
	return e
}

// ModifierLatch is the state of software modifier keys on a keyboard
// accessory, as on a terminal's Ctrl key: a tap arms a modifier for the next
// key, a long press locks it until tapped again. Pass Active to
// InputModifiers with Consume as its callback, and Clear when the input
// session ends. The zero value has nothing armed.
type ModifierLatch struct{ armed, locked Modifiers }

// Active returns the modifiers for the next key, locked ones included.
func (l *ModifierLatch) Active() Modifiers { return l.armed }

// Locked returns the modifiers that stay after a key.
func (l *ModifierLatch) Locked() Modifiers { return l.locked }

// Tap arms m for the next key, or releases it when armed or locked.
func (l *ModifierLatch) Tap(m Modifiers) {
	if l.armed&m != 0 {
		l.armed &^= m
		l.locked &^= m
	} else {
		l.armed |= m
	}
}

// Lock keeps m for every key until it is tapped.
func (l *ModifierLatch) Lock(m Modifiers) { l.armed |= m; l.locked |= m }

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
func (l *ModifierLatch) Decorate(actions []InputAction, modifier func(id string) Modifiers) []InputAction {
	out := append([]InputAction(nil), actions...)
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

// inputAccessoryColumns matches the iOS accessory's expanded rows.
const inputAccessoryColumns = 6

// InputAccessoryBar draws actions as Go buttons, the way iOS shows an
// InputAccessory above its keyboard, for platforms and tests without that
// native view. It keeps the input's focus. Actions with Items open their
// panel above the row while *expanded; a long press sends LongPressID. Draw
// it below the focused input while it has the focus.
func coreInputAccessoryBar(c *context, actions []InputAction, expanded *bool, handler func(string)) *node {
	t := c.theme
	button := func(a InputAction) {
		b := coreButtonBase(c).Label(a.Label).Role(RoleButton).KeepFocus().Height(44).MinWidth(44).Grow(1).Shrink(1).Radius(8)
		if b.Pressed() || a.Selected || len(a.Items) > 0 && *expanded {
			b.Background(t.SurfacePressed)
		}
		b.Children(func() { coreText(c, a.Label).FontSize(13) })
		if a.LongPressID != "" {
			id := a.LongPressID
			b.HandleInput(func(ev InputEvent) bool {
				if ev.Kind == InputLongPress {
					handler(id)
					return true
				}
				return false
			})
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
	row := func(actions []InputAction) {
		coreRow(c).FillWidth().Gap(2).Children(func() {
			for _, a := range actions {
				button(a)
			}
			// Short rows keep the full rows' key widths, as on iOS.
			for range max(0, inputAccessoryColumns-len(actions)) {
				coreBox(c).Grow(1).Shrink(1).MinWidth(44)
			}
		})
	}
	return coreColumn(c).FillWidth().Padding(0, 8).Background(t.Surface).Children(func() {
		if *expanded {
			for _, a := range actions {
				for i := 0; i < len(a.Items); i += inputAccessoryColumns {
					row(a.Items[i:min(i+inputAccessoryColumns, len(a.Items))])
				}
			}
		}
		coreRow(c).FillWidth().Gap(2).Children(func() {
			for _, a := range actions {
				button(a)
			}
		})
	})
}
