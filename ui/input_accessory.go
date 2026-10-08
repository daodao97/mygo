package ui

import "encoding/json"

// InputAction is a software-keyboard accessory button. Symbol optionally
// names a system symbol; Label remains its accessible name. Items makes a
// button expand a panel of secondary actions without dismissing the keyboard.
type InputAction struct {
	ID     string
	Label  string
	Symbol string        `json:",omitempty"`
	Items  []InputAction `json:",omitempty"`
}

// InputAccessory attaches actions above the iOS system keyboard while this
// input has focus. Other platforms ignore the native controls. The handler
// runs on the UI thread without changing text focus or committing marked text.
// Omit the call on later frames to remove the accessory.
func (e *Element) InputAccessory(actions []InputAction, handler func(string)) *Element {
	if len(actions) > 0 {
		data, _ := json.Marshal(actions)
		e.inputAccessory = string(data)
		e.inputAction = handler
	}
	return e
}
