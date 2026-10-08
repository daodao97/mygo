package ui

import "github.com/egoist/mygo/internal/platform"

// KeyboardType selects a software keyboard. Desktop input is unaffected.
type KeyboardType string

const (
	KeyboardDefault KeyboardType = ""
	KeyboardText    KeyboardType = "text"
	KeyboardASCII   KeyboardType = "ascii"
	KeyboardNumber  KeyboardType = "number"
	KeyboardDecimal KeyboardType = "decimal"
	KeyboardPhone   KeyboardType = "phone"
	KeyboardEmail   KeyboardType = "email"
	KeyboardURL     KeyboardType = "url"
)

// ReturnKey selects the caption of a software keyboard's return button.
// Single-line fields report Submitted when it is pressed. Text areas keep
// their newline behavior; changing the caption does not change editing policy.
type ReturnKey string

const (
	ReturnDefault  ReturnKey = ""
	ReturnDone     ReturnKey = "done"
	ReturnGo       ReturnKey = "go"
	ReturnNext     ReturnKey = "next"
	ReturnSearch   ReturnKey = "search"
	ReturnSend     ReturnKey = "send"
	ReturnContinue ReturnKey = "continue"
)

// TextContent describes the semantic value for system autofill. Password()
// is still needed to conceal a password field. Suggestions depend on the
// user's settings and available credentials; this is not a storage API.
type TextContent string

const (
	ContentNone        TextContent = ""
	ContentUsername    TextContent = "username"
	ContentPassword    TextContent = "password"
	ContentNewPassword TextContent = "newPassword"
	ContentOneTimeCode TextContent = "oneTimeCode"
	ContentEmail       TextContent = "emailAddress"
	ContentPhone       TextContent = "telephoneNumber"
	ContentName        TextContent = "name"
	ContentAddress     TextContent = "fullStreetAddress"
	ContentPostalCode  TextContent = "postalCode"
)

// InputCorrection selects native autocorrection.
type InputCorrection string

const (
	CorrectionDefault InputCorrection = ""
	CorrectionOn      InputCorrection = "on"
	CorrectionOff     InputCorrection = "off"
)

// Capitalization selects automatic capitalization by a software keyboard.
type Capitalization string

const (
	CapitalizeDefault   Capitalization = ""
	CapitalizeNone      Capitalization = "none"
	CapitalizeWords     Capitalization = "words"
	CapitalizeSentences Capitalization = "sentences"
	CapitalizeAll       Capitalization = "all"
)

// InputOptions configures native keyboard traits for TextInput and TextArea.
// Zero values follow system defaults. Currently iOS consumes these hints;
// other backends keep accepting ordinary text and ignore keyboard hints.
// KeyboardDismissMode chooses how scrolling dismisses the software keyboard.
// Interactive is currently implemented by iOS; other platforms treat it as none.
type KeyboardDismissMode string

const (
	KeyboardDismissNone        KeyboardDismissMode = ""
	KeyboardDismissOnDrag      KeyboardDismissMode = "on-drag"
	KeyboardDismissInteractive KeyboardDismissMode = "interactive"
)

type InputOptions struct {
	Dismiss        KeyboardDismissMode
	Keyboard       KeyboardType
	Return         ReturnKey
	Content        TextContent
	Correction     InputCorrection
	Capitalization Capitalization
}

// InputOptions supplies this frame's keyboard traits for an editable field or
// a custom HandleInput/TextCaret element, such as a terminal. Omitting the call
// on a subsequent frame restores the default traits.
func (e *Element) InputOptions(o InputOptions) *Element {
	e.inputOptions = o
	if ed := e.st.editor; ed != nil && e.flags&flagEditable != 0 {
		ed.inputOptions = o.nativeOptions()
	}
	return e
}

func (o InputOptions) nativeOptions() platform.TextInputOptions {
	return platform.TextInputOptions{
		Keyboard: string(o.Keyboard), Return: string(o.Return), Content: string(o.Content),
		Correction: string(o.Correction), Capitalization: string(o.Capitalization), Dismiss: string(o.Dismiss),
	}
}
