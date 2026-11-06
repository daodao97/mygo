// Android's touch and key events, mapped onto mygo's platform contract.
//
// This file is the whole of the Android backend's input mapping and it is
// deliberately free of cgo, JNI and Android types: it takes plain Go values
// and returns platform.SurfaceEvent, so it can be read, reviewed and — the
// point — tested on a desktop with
//
//	CGO_ENABLED=0 go test ./internal/android/backend/
//
// The other two halves do nothing this file cannot do itself:
//
//	android/jni/jni_input.go  decodes Java's arrays into the types
//	                               below and hands them to the Backend;
//	android/.../MygoInput.kt       reads a MotionEvent's pointers and a
//	                               KeyEvent's fields into those arrays.
//
// # Android's constants come from Java, not from memory
//
// The action and key codes here are only *recognisers*: the Kotlin side
// passes the values it has (event.getActionMasked(), keyEvent.getKeyCode()),
// so what crosses the boundary is Android's own number. Every number below
// was read out of the platform's android.jar rather than written from
// memory —
//
//	javap -constants -classpath <sdk>/platforms/android-36/android.jar \
//	    android.view.MotionEvent android.view.KeyEvent
//
// whose output is reproduced in T32.md — and they are kept in this one
// block so a wrong one is a one-line fix in a one-line review.
//
// # What the contract cannot carry
//
// platform.SurfaceEvent carries one pointer at a time — its Pointer and
// its PointerID — but has no pointer list (see internal/platform/surface.go),
// so a MotionEvent's whole pointer set cannot be delivered in the flat
// stream. This file therefore produces two things from every touch:
//
//   - Events, the projection onto what the contract holds today, which a
//     single-pointer consumer can use unchanged; and
//   - Pointers, every pointer of the event in DIPs with what the action did
//     to it — exactly the payload the extended contract needs.
//
// The projection is lossy and the loss is known, not accidental: a second
// finger going down or up adds a PointerPressed/Released to Pointers but no
// flat event, because a flat PointerDown at a second finger's coordinates
// would tell a one-pointer consumer that the primary press moved; and a
// cancelled gesture leaves Pointers marked PointerCancelled and no flat
// event, because platform.SurfaceEventKind has no PointerCancel. Both gaps
// and the shape that closes them are in T32.md.
package backend

import (
	"math"
	"strconv"
	"unicode"

	"github.com/egoist/mygo/internal/platform"
)

// Android's MotionEvent and KeyEvent constants, verified against
// android-36/android.jar (see the file comment for the command). They are
// the fallback for a Java side that spells them out; the live values come
// from Java at run time.
const (
	// MotionEvent actions: event.getActionMasked(). The low 8 bits are the
	// action and the bits above them the pointer index
	// (ACTION_POINTER_INDEX_SHIFT is 8), which is why the Java side sends
	// the masked value.
	actDown          = 0  // MotionEvent.ACTION_DOWN
	actUp            = 1  // ACTION_UP
	actMove          = 2  // ACTION_MOVE
	actCancel        = 3  // ACTION_CANCEL
	actOutside       = 4  // ACTION_OUTSIDE
	actPointerDown   = 5  // ACTION_POINTER_DOWN
	actPointerUp     = 6  // ACTION_POINTER_UP
	actHoverMove     = 7  // ACTION_HOVER_MOVE
	actScroll        = 8  // ACTION_SCROLL
	actHoverEnter    = 9  // ACTION_HOVER_ENTER
	actHoverExit     = 10 // ACTION_HOVER_EXIT
	actButtonPress   = 11 // ACTION_BUTTON_PRESS
	actButtonRelease = 12 // ACTION_BUTTON_RELEASE

	// MotionEvent button bits: event.getButtonState().
	buttonPrimary   = 1 // MotionEvent.BUTTON_PRIMARY
	buttonSecondary = 2 // BUTTON_SECONDARY
	buttonTertiary  = 4 // BUTTON_TERTIARY (the middle button)

	// KeyEvent actions: KeyEvent.getAction().
	keyActionDown     = 0 // KeyEvent.ACTION_DOWN
	keyActionUp       = 1 // ACTION_UP
	keyActionMultiple = 2 // ACTION_MULTIPLE

	// KeyEvent meta bits: KeyEvent.getMetaState().
	metaShiftOn = 1     // KeyEvent.META_SHIFT_ON
	metaAltOn   = 2     // META_ALT_ON
	metaCtrlOn  = 4096  // META_CTRL_ON
	metaMetaOn  = 65536 // META_META_ON (the Command/Windows key)

	// Key codes: KeyEvent.getKeyCode(). Ranges are named by their first
	// member and their length, and the table below fills them in a loop
	// rather than by 48 transcribed literals.
	kcDigit0  = 7   // KEYCODE_0; KEYCODE_1..KEYCODE_9 are 8..16, contiguous
	kcLetterA = 29  // KEYCODE_A; KEYCODE_Z is 54, contiguous
	kcF1      = 131 // KEYCODE_F1; KEYCODE_F12 is 142, contiguous

	kcBack         = 4   // KEYCODE_BACK (Android's Back; platform.KeyBack is "back in a history")
	kcDpadUp       = 19  // KEYCODE_DPAD_UP
	kcDpadDown     = 20  // KEYCODE_DPAD_DOWN
	kcDpadLeft     = 21  // KEYCODE_DPAD_LEFT
	kcDpadRight    = 22  // KEYCODE_DPAD_RIGHT
	kcComma        = 55  // KEYCODE_COMMA
	kcPeriod       = 56  // KEYCODE_PERIOD
	kcAltLeft      = 57  // KEYCODE_ALT_LEFT
	kcAltRight     = 58  // KEYCODE_ALT_RIGHT
	kcShiftLeft    = 59  // KEYCODE_SHIFT_LEFT
	kcShiftRight   = 60  // KEYCODE_SHIFT_RIGHT
	kcTab          = 61  // KEYCODE_TAB
	kcSpace        = 62  // KEYCODE_SPACE
	kcEnter        = 66  // KEYCODE_ENTER
	kcDel          = 67  // KEYCODE_DEL (Backspace)
	kcGrave        = 68  // KEYCODE_GRAVE
	kcMinus        = 69  // KEYCODE_MINUS
	kcEquals       = 70  // KEYCODE_EQUALS
	kcLeftBracket  = 71  // KEYCODE_LEFT_BRACKET
	kcRightBracket = 72  // KEYCODE_RIGHT_BRACKET
	kcBackslash    = 73  // KEYCODE_BACKSLASH
	kcSemicolon    = 74  // KEYCODE_SEMICOLON
	kcApostrophe   = 75  // KEYCODE_APOSTROPHE
	kcSlash        = 76  // KEYCODE_SLASH
	kcMenu         = 82  // KEYCODE_MENU (the Context menu key on a keyboard)
	kcPageUp       = 92  // KEYCODE_PAGE_UP
	kcPageDown     = 93  // KEYCODE_PAGE_DOWN
	kcEscape       = 111 // KEYCODE_ESCAPE
	kcForwardDel   = 112 // KEYCODE_FORWARD_DEL
	kcCtrlLeft     = 113 // KEYCODE_CTRL_LEFT
	kcCtrlRight    = 114 // KEYCODE_CTRL_RIGHT
	kcMetaLeft     = 117 // KEYCODE_META_LEFT
	kcMetaRight    = 118 // KEYCODE_META_RIGHT
	kcMoveHome     = 122 // KEYCODE_MOVE_HOME
	kcMoveEnd      = 123 // KEYCODE_MOVE_END
	kcInsert       = 124 // KEYCODE_INSERT
	kcForward      = 125 // KEYCODE_FORWARD (platform.KeyForward)
)

// clickWindowMs is how soon a second press of the primary button counts as
// the second click; a MotionEvent is a raw DOWN/UP stream (Android's own
// GestureDetector is not installed), so the count is made here, as
// internal/browser's mouseButton makes it for a page.
const clickWindowMs = 400

// keyTable maps an Android key code to the platform.Key it means. It is
// filled by init because letters, digits and function keys are contiguous
// ranges on the Android side; the singles are in the const block above.
var keyTable = map[int]platform.Key{}

// keyNames holds the Android name of every code keyTable recognises, so a
// log line can say KEYCODE_LEFT_BRACKET instead of 71.
var keyNames = map[int]string{}

func init() {
	for i := 0; i < 10; i++ {
		keyTable[kcDigit0+i] = platform.Key0 + platform.Key(i)
		keyNames[kcDigit0+i] = "KEYCODE_" + strconv.Itoa(i)
	}
	for i := 0; i < 26; i++ {
		keyTable[kcLetterA+i] = platform.KeyA + platform.Key(i)
		keyNames[kcLetterA+i] = "KEYCODE_" + string(rune('A'+i))
	}
	for i := 0; i < 12; i++ {
		keyTable[kcF1+i] = platform.KeyF1 + platform.Key(i)
		keyNames[kcF1+i] = "KEYCODE_F" + strconv.Itoa(i+1)
	}
	for code, entry := range map[int]struct {
		key  platform.Key
		name string
	}{
		kcEnter:        {platform.KeyEnter, "KEYCODE_ENTER"},
		kcEscape:       {platform.KeyEscape, "KEYCODE_ESCAPE"},
		kcDel:          {platform.KeyBackspace, "KEYCODE_DEL"},
		kcForwardDel:   {platform.KeyDelete, "KEYCODE_FORWARD_DEL"},
		kcTab:          {platform.KeyTab, "KEYCODE_TAB"},
		kcSpace:        {platform.KeySpace, "KEYCODE_SPACE"},
		kcInsert:       {platform.KeyInsert, "KEYCODE_INSERT"},
		kcMoveHome:     {platform.KeyHome, "KEYCODE_MOVE_HOME"},
		kcMoveEnd:      {platform.KeyEnd, "KEYCODE_MOVE_END"},
		kcPageUp:       {platform.KeyPageUp, "KEYCODE_PAGE_UP"},
		kcPageDown:     {platform.KeyPageDown, "KEYCODE_PAGE_DOWN"},
		kcDpadUp:       {platform.KeyUp, "KEYCODE_DPAD_UP"},
		kcDpadDown:     {platform.KeyDown, "KEYCODE_DPAD_DOWN"},
		kcDpadLeft:     {platform.KeyLeft, "KEYCODE_DPAD_LEFT"},
		kcDpadRight:    {platform.KeyRight, "KEYCODE_DPAD_RIGHT"},
		kcMinus:        {platform.KeyMinus, "KEYCODE_MINUS"},
		kcEquals:       {platform.KeyEqual, "KEYCODE_EQUALS"},
		kcComma:        {platform.KeyComma, "KEYCODE_COMMA"},
		kcPeriod:       {platform.KeyPeriod, "KEYCODE_PERIOD"},
		kcSlash:        {platform.KeySlash, "KEYCODE_SLASH"},
		kcSemicolon:    {platform.KeySemicolon, "KEYCODE_SEMICOLON"},
		kcApostrophe:   {platform.KeyQuote, "KEYCODE_APOSTROPHE"},
		kcLeftBracket:  {platform.KeyBracketLeft, "KEYCODE_LEFT_BRACKET"},
		kcRightBracket: {platform.KeyBracketRight, "KEYCODE_RIGHT_BRACKET"},
		kcBackslash:    {platform.KeyBackslash, "KEYCODE_BACKSLASH"},
		kcGrave:        {platform.KeyBackquote, "KEYCODE_GRAVE"},
		kcMenu:         {platform.KeyContextMenu, "KEYCODE_MENU"},
		kcBack:         {platform.KeyBack, "KEYCODE_BACK"},
		kcForward:      {platform.KeyForward, "KEYCODE_FORWARD"},
	} {
		keyTable[code] = entry.key
		keyNames[code] = entry.name
	}
}

// Pointer is one touch point of a MotionEvent, in the view's own pixels:
// what MotionEvent.getX(i), getY(i) and getPointerId(i) give.
type Pointer struct {
	// ID is MotionEvent.getPointerId(i): stable for the life of the
	// gesture, unlike the index, which shifts as pointers come and go.
	ID int
	// X and Y are view-local pixels, not screen pixels: a press at the
	// bottom of a window in the corner of the screen must land at the
	// bottom of the surface.
	X, Y float32
}

// PointerAction is what one event did to one pointer.
type PointerAction uint8

const (
	// PointerNone: the pointer was already down (or only moved): its
	// position is current and its state did not change.
	PointerNone PointerAction = iota
	PointerPressed
	PointerReleased
	// PointerCancelled: the gesture was cancelled, as when the system
	// takes the touch for a gesture of its own.
	PointerCancelled
)

// PointerState is one pointer of an event, in DIPs, with what happened to
// it. It is the payload an extended platform.SurfaceEvent would carry:
// see T32.md for the patch and for how this type becomes
// platform.PointerState.
type PointerState struct {
	ID     int
	X, Y   float64 // DIPs relative to the surface
	Action PointerAction
}

// TouchInput is one MotionEvent, decoded into plain Go values.
type TouchInput struct {
	// Action is event.getActionMasked().
	Action int
	// ActionIndex is event.getActionIndex(): which pointer this action is
	// about. Android promises it is valid for DOWN, POINTER_DOWN, UP and
	// POINTER_UP, and 0 for MOVE.
	ActionIndex int
	// ButtonState is event.getButtonState(): which mouse buttons are held.
	// A finger carries no button bits.
	ButtonState int
	// MetaState is event.getMetaState(): the modifier keys held.
	MetaState int
	// ScrollX and ScrollY are event.getAxisValue(AXIS_HSCROLL/AXIS_VSCROLL)
	// in the view's pixels, for ACTION_SCROLL (a wheel on a mouse or a
	// trackpad); both are 0 for a touch.
	ScrollX, ScrollY float32
	// EventTimeMs is event.getEventTime(): Android's own call
	// milliseconds, which is what makes the double-click count above
	// reproducible.
	EventTimeMs int64
	// Pointers is every pointer of the event, in view pixels, in
	// MotionEvent's own order.
	Pointers []Pointer
}

// TouchResult is what one MotionEvent means: the events the contract can
// carry today, and the pointers it cannot yet.
type TouchResult struct {
	// Events are the platform.SurfaceEvents to deliver, in order. It may
	// be empty, as it is for a second finger going down (see the file
	// comment and T32.md).
	Events []platform.SurfaceEvent
	// Pointers is every pointer of the event in DIPs, or nil when the
	// event carried none.
	Pointers []PointerState
}

// KeyEventInput is one KeyEvent, decoded into plain Go values.
type KeyEventInput struct {
	// KeyCode is KeyEvent.getKeyCode().
	KeyCode int
	// Action is KeyEvent.getAction(): ACTION_DOWN, ACTION_UP or
	// ACTION_MULTIPLE.
	Action int
	// MetaState is KeyEvent.getMetaState(): the modifier keys held.
	MetaState int
	// UnicodeChar is KeyEvent.getUnicodeChar(): the character the key
	// types, with the keyboard layout and the shift already applied. It is
	// 0 when the key types nothing (a modifier, an arrow), which is how a
	// text event is told from a bare key event without a layout table
	// here.
	UnicodeChar int
	// RepeatCount is KeyEvent.getRepeatCount(): above 0 marks an
	// auto-repeat.
	RepeatCount int
	// EventTimeMs is KeyEvent.getEventTime().
	EventTimeMs int64
}

// KeyOutcome says what Key made of an Android key code.
type KeyOutcome uint8

const (
	// KeyMapped: the events are ready. There may be two — the key event
	// and the character it types.
	KeyMapped KeyOutcome = iota
	// KeyUnmapped: Android defines this code but platform.Key has no
	// equivalent, so there is nothing to deliver. The caller records it:
	// a key that silently became KeyUnknown would be a bug nobody can see.
	KeyUnmapped
	// KeyModifierOnly: the code is a modifier key, and a modifier going
	// down or up on its own is platform.ModifiersChanged rather than a
	// key event (internal/linux/surface.go reports it the same way).
	KeyModifierOnly
	// KeyIgnored: ACTION_MULTIPLE and anything unrecognised. Nothing to
	// deliver and nothing worth reporting.
	KeyIgnored
)

// Decoder turns Android input into platform events. Its zero value works
// and means a scale of 1 (a DIP is a device pixel).
//
// It holds the little state the mapping needs across events: the density
// last reported, the primary button of the press in progress, and the click
// count. That state is per process and is touched only from the backend's
// pump goroutine (see OnTouchBatch and friends in jni_input.go), never from
// the thread Java called on.
type Decoder struct {
	// scale is device pixels per DIP: dpi/160, Android's own definition
	// of a density-independent pixel (platform.Surface.Size reports the
	// same ratio).
	scale float64
	// downButton is the button of the press in progress, remembered
	// because ACTION_UP's buttonState may already have been cleared.
	downButton int
	// haveDown, downMs and clicks count quick successive presses of the
	// primary button.
	haveDown bool
	downMs   int64
	clicks   int
}

// NewDecoder returns a Decoder whose DIPs are the ones the display of dpi
// dots per inch gives: a DIP is dpi/160 device pixels. A dpi of 0 or less
// means unknown, and a DIP is then a device pixel.
func NewDecoder(dpi int) Decoder {
	if dpi <= 0 {
		dpi = 160
	}
	return Decoder{scale: float64(dpi) / 160}
}

// SetDPI reports the display's density in dots per inch.
func (d *Decoder) SetDPI(dpi int) {
	if dpi <= 0 {
		return
	}
	d.scale = float64(dpi) / 160
}

// Scale returns how many device pixels a DIP is.
func (d Decoder) Scale() float64 {
	if d.scale <= 0 {
		return 1
	}
	return d.scale
}

// dip converts a view pixel to a DIP. A value that is not finite — a
// device, or a Java side, that sent NaN or an infinity — becomes 0: a
// non-finite coordinate would poison every layout computed from it, and 0
// is at least inside the surface.
func (d Decoder) dip(px float32) float64 {
	v := float64(px) / d.Scale()
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

// Touch maps one MotionEvent to the platform.SurfaceEvents it means, with
// the event's pointers beside them. See the file comment for why the second
// finger reaches Pointers and not Events.
//
// The flat event of a contact carries the contact's own PointerID — the
// value of MotionEvent.getPointerId, which names that finger for the whole
// gesture — so a consumer can tell one finger's contact from another's. It
// is the id and not the index on purpose: the index shifts as pointers come
// and go, which is why Java sends both. A mouse event (hover, a wheel) has
// one pointer and leaves PointerID 0, as the contract says.
func (d *Decoder) Touch(in TouchInput) TouchResult {
	res := TouchResult{}
	if n := len(in.Pointers); n > 0 {
		res.Pointers = make([]PointerState, n)
		for i, p := range in.Pointers {
			res.Pointers[i] = PointerState{ID: p.ID, X: d.dip(p.X), Y: d.dip(p.Y)}
		}
	}
	// The pointer this action is about, or -1 when the event carried none.
	at := in.pointerAt(len(in.Pointers))
	mods := ModifiersFromMeta(in.MetaState)

	switch in.Action {
	case actDown:
		if at < 0 {
			break
		}
		button := buttonFromState(in.ButtonState)
		if button < 0 {
			button = 0
		}
		d.downButton = button
		res.Pointers[at].Action = PointerPressed
		res.Events = []platform.SurfaceEvent{{
			Kind:      platform.PointerDown,
			PointerID: uint64(res.Pointers[at].ID),
			Pointer:   platform.PointerTouch,
			X:         res.Pointers[at].X,
			Y:         res.Pointers[at].Y,
			Button:    button,
			Clicks:    d.countClick(button, in.EventTimeMs, true),
			Mods:      mods,
		}}
	case actUp:
		if at < 0 {
			break
		}
		// ACTION_UP's buttonState may already be empty (the button went
		// up), so the press's own button is the one remembered.
		button := d.downButton
		res.Pointers[at].Action = PointerReleased
		res.Events = []platform.SurfaceEvent{{
			Kind:      platform.PointerUp,
			PointerID: uint64(res.Pointers[at].ID),
			Pointer:   platform.PointerTouch,
			X:         res.Pointers[at].X,
			Y:         res.Pointers[at].Y,
			Button:    button,
			Clicks:    d.countClick(button, in.EventTimeMs, false),
			Mods:      mods,
		}}
	case actPointerDown:
		// A second finger (or a third): the contract has nowhere to say
		// "another pointer, not the primary one", so it is recorded and
		// not forced into the flat stream.
		if at >= 0 {
			res.Pointers[at].Action = PointerPressed
		}
	case actPointerUp:
		if at >= 0 {
			res.Pointers[at].Action = PointerReleased
		}
	case actMove:
		// A change during a press gesture (ACTION_DOWN..ACTION_UP): on a
		// phone that is a finger, which is the contact the content's touch
		// layer runs for, so it is PointerTouch.
		if at < 0 {
			break
		}
		res.Events = []platform.SurfaceEvent{{
			Kind:      platform.PointerMove,
			PointerID: uint64(res.Pointers[at].ID),
			Pointer:   platform.PointerTouch,
			X:         res.Pointers[at].X,
			Y:         res.Pointers[at].Y,
			Mods:      mods,
		}}
	case actHoverMove, actHoverEnter:
		// A pointer that is over the surface but not touching it is a mouse
		// or a pen, never a finger, so it is PointerMouse and does not
		// enter the content's touch layer.
		if at < 0 {
			break
		}
		res.Events = []platform.SurfaceEvent{{
			Kind: platform.PointerMove,
			X:    res.Pointers[at].X,
			Y:    res.Pointers[at].Y,
			Mods: mods,
		}}
	case actHoverExit, actOutside:
		// The pointer is no longer over the surface. ACTION_OUTSIDE can
		// carry a position, but there is no position for it to be at on
		// this surface, which is what leaving means. Leaving names no
		// device, so the zero value (PointerMouse) stands.
		res.Events = []platform.SurfaceEvent{{Kind: platform.PointerLeave}}
	case actCancel:
		// The gesture was taken from us. platform.SurfaceEventKind has no
		// PointerCancel, so every pointer is marked cancelled and no flat
		// event is invented; T32.md carries this as a third gap.
		for i := range res.Pointers {
			res.Pointers[i].Action = PointerCancelled
		}
	case actScroll:
		if at < 0 {
			break
		}
		// AXIS_VSCROLL is positive scrolling up, and platform's DY is
		// positive moving the view down the content, so the vertical sign
		// flips and the horizontal one does not (AXIS_HSCROLL positive
		// scrolls right, DX positive moves right). Android's axes are
		// continuous, which is what Precise marks.
		// ACTION_SCROLL comes from a wheel, a trackpad or rotary input,
		// not from a finger on the screen, so it is not a touch and stays
		// PointerMouse, the zero value.
		res.Events = []platform.SurfaceEvent{{
			Kind:    platform.PointerScroll,
			X:       res.Pointers[at].X,
			Y:       res.Pointers[at].Y,
			DX:      d.dip(in.ScrollX),
			DY:      -d.dip(in.ScrollY),
			Precise: true,
			Mods:    mods,
		}}
	case actButtonPress, actButtonRelease:
		// A mouse's button press and release arrive around the
		// ACTION_DOWN/ACTION_UP pair that already carries the button in
		// buttonState, so they add no flat event; the pointer's own state
		// is the part of them that is new.
		if at >= 0 {
			if in.Action == actButtonPress {
				res.Pointers[at].Action = PointerPressed
			} else {
				res.Pointers[at].Action = PointerReleased
			}
		}
	}
	return res
}

// pointerAt returns the index of the pointer the action is about, clamped
// into the event's own list, or -1 when the event carried no pointer. A
// malformed event — or a Java side that sent an empty array — is clamped
// rather than trusted, so no slice is ever indexed out of range.
func (in TouchInput) pointerAt(n int) int {
	if n == 0 {
		return -1
	}
	if i := in.ActionIndex; i >= 0 && i < n {
		return i
	}
	return 0
}

// countClick returns the click count to carry on a press or release of
// button: the second press of the primary button within clickWindowMs of
// the first is the second click. Other buttons count as one, as
// internal/browser does. A release carries the press's count, so a consumer
// can tell a double click's second release from a single one.
func (d *Decoder) countClick(button int, eventTimeMs int64, pressed bool) int {
	if button != 0 {
		return 1
	}
	if pressed {
		// A device whose clock went backwards must not read as a double
		// click, so the window is a window and not "less than".
		if elapsed := eventTimeMs - d.downMs; d.haveDown && elapsed >= 0 && elapsed < clickWindowMs {
			d.clicks++
		} else {
			d.clicks = 1
		}
		d.haveDown, d.downMs = true, eventTimeMs
	}
	if d.clicks < 1 {
		return 1
	}
	return d.clicks
}

// buttonFromState maps a MotionEvent buttonState to platform.SurfaceEvent's
// Button — 0 the primary, 1 the secondary, 2 the middle — or -1 when no
// mapped button is held, which is what a finger's event says.
func buttonFromState(state int) int {
	switch {
	case state&buttonPrimary != 0:
		return 0
	case state&buttonSecondary != 0:
		return 1
	case state&buttonTertiary != 0:
		return 2
	}
	return -1
}

// Key maps one KeyEvent to the platform.SurfaceEvents it means and says
// what it made of the key code, so an unmapped key is recorded and not
// silently dropped.
func (d *Decoder) Key(in KeyEventInput) ([]platform.SurfaceEvent, KeyOutcome) {
	mods := ModifiersFromMeta(in.MetaState)

	if bit := modifierBit(in.KeyCode); bit != 0 {
		if in.Action != keyActionDown && in.Action != keyActionUp {
			return nil, KeyIgnored
		}
		// The state is the one before the event, so the key's own bit goes
		// in on a press and out on a release. Android's metaState may
		// already contain it either way, and this is right in both cases:
		// setting a set bit changes nothing, and on a release the key is
		// no longer held.
		state := mods
		if in.Action == keyActionDown {
			state |= bit
		} else {
			state &^= bit
		}
		return []platform.SurfaceEvent{{Kind: platform.ModifiersChanged, Mods: state}}, KeyModifierOnly
	}

	if in.Action != keyActionDown && in.Action != keyActionUp {
		// ACTION_MULTIPLE carries its text in a different way and reaches
		// content as TextInput once an input method is wired, not here.
		return nil, KeyIgnored
	}

	k, ok := keyTable[in.KeyCode]
	if !ok {
		return nil, KeyUnmapped
	}

	kind := platform.KeyPressed
	if in.Action == keyActionUp {
		kind = platform.KeyReleased
	}
	events := []platform.SurfaceEvent{{
		Kind:   kind,
		Key:    k,
		Mods:   mods,
		Repeat: in.RepeatCount > 0,
	}}
	// The character the key types, on a press only: Android's
	// getUnicodeChar already applied the keyboard layout and the shift,
	// and a key held as a shortcut types nothing. A control character
	// (including 0, which means "nothing") is not text.
	if in.Action == keyActionDown && in.UnicodeChar > 0 {
		if r := rune(in.UnicodeChar); !unicode.IsControl(r) && mods&(platform.ModCtrl|platform.ModAlt|platform.ModSuper) == 0 {
			events = append(events, platform.SurfaceEvent{Kind: platform.TextInput, Text: string(r), Mods: mods})
		}
	}
	return events, KeyMapped
}

// modifierBit returns the Modifiers bit an Android modifier key code stands
// for, or 0 when the code is not a modifier.
func modifierBit(keyCode int) platform.Modifiers {
	switch keyCode {
	case kcShiftLeft, kcShiftRight:
		return platform.ModShift
	case kcCtrlLeft, kcCtrlRight:
		return platform.ModCtrl
	case kcAltLeft, kcAltRight:
		return platform.ModAlt
	case kcMetaLeft, kcMetaRight:
		return platform.ModSuper
	}
	return 0
}

// KeyCodeName returns the Android name of a key code, so a log line about
// an unmapped key says which key it was: "KEYCODE_LEFT_BRACKET" rather than
// 71, and "KEYCODE_<n>" for one outside this file's table.
func KeyCodeName(keyCode int) string {
	if name, ok := keyNames[keyCode]; ok {
		return name
	}
	return "KEYCODE_" + strconv.Itoa(keyCode)
}

// ModifiersFromMeta maps Android's KeyEvent meta state to
// platform.Modifiers.
//
// Android's bits are per side (META_SHIFT_LEFT_ON and META_SHIFT_RIGHT_ON)
// with an aggregate bit (META_SHIFT_ON) set when either is; the aggregate
// is the one tested, which is what a consumer wants.
func ModifiersFromMeta(metaState int) platform.Modifiers {
	var m platform.Modifiers
	if metaState&metaShiftOn != 0 {
		m |= platform.ModShift
	}
	if metaState&metaCtrlOn != 0 {
		m |= platform.ModCtrl
	}
	if metaState&metaAltOn != 0 {
		m |= platform.ModAlt
	}
	if metaState&metaMetaOn != 0 {
		m |= platform.ModSuper
	}
	return m
}

// SurfaceSink receives every mapped event. It is nil until the Android
// backend has a WindowHandler.SurfaceEvent to deliver to, which it does not
// yet: package ui is not wired on Android, and app.go's touch and key are
// stubs. The mapping here is the part that was missing, so wiring this
// variable is the only step that is left.
var SurfaceSink func(platform.SurfaceEvent) bool

// DispatchInput hands mapped events to SurfaceSink, on the caller's
// goroutine: the backend calls it from its pump (see jni_input.go). Every
// event notes that the person did something — these are their events, not
// the framework's — and is dropped when nothing is listening, as app.go's
// stubs drop them today.
func DispatchInput(events []platform.SurfaceEvent) {
	for _, ev := range events {
		platform.NoteInput()
		if SurfaceSink != nil {
			SurfaceSink(ev)
		}
	}
}

// input is the process's decoder. It is package state because the density
// and the click count outlive a single event, and it is read and written
// only from the pump goroutine: the three functions below are what the JNI
// entry points queue (jni_input.go), so no Java thread ever touches it —
// the same rule app.go's On* methods follow for everything else.
var input Decoder

// DisplayInfo records the display's density in dots per inch, which is what
// turns the view pixels Java sends into the DIPs platform.SurfaceEvent
// carries. It runs on the pump.
func DisplayInfo(dpi int) { input.SetDPI(dpi) }

// displayRefreshRate is the rate the Activity asked the display for, in Hz,
// and 0 when nothing has said. It is read by Surface.RefreshRate.
//
// 🔴 Why it is set from Java instead of read from the window. The rate is a
// property of the display mode the Activity selected, and the mode is
// picked in Java (MygoView.requestMaxRefreshRate, through
// WindowManager.LayoutParams.preferredDisplayModeId) — ANativeWindow, which
// is all the native side holds, exposes no rate at all. Reading it anywhere
// but where it was chosen would be reading a different number: on a panel
// that drops to 60 Hz while still and rises to 120 Hz while touched, a rate
// read at the wrong moment is the rate of the wrong moment.
//
// Guarded like the rest of the input globals: written on the pump, read
// anywhere.
var displayRefreshRate float64

// SetDisplayRefreshRate records the rate the Activity's display mode runs
// at, in Hz, so the engine's pacing sees the display it is actually on. It
// runs on the pump.
func SetDisplayRefreshRate(hz float64) {
	if hz > 0 {
		displayRefreshRate = hz
	}
}

// DisplayRefreshRate is the rate SetDisplayRefreshRate last recorded, or 0
// when nothing has.
func DisplayRefreshRate() float64 { return displayRefreshRate }

// TouchBatch maps one MotionEvent — already decoded into plain Go values by
// the JNI layer — and delivers what it means. It runs on the pump.
func TouchBatch(in TouchInput) { DispatchInput(input.Touch(in).Events) }

// KeyBatch maps one KeyEvent and delivers what it means, returning what it
// made of the key code so the caller can record an unmapped key: Android has
// far more keys than platform.Key names, and one of them silently becoming
// KeyUnknown would be invisible. It runs on the pump.
func KeyBatch(in KeyEventInput) KeyOutcome {
	events, outcome := input.Key(in)
	DispatchInput(events)
	return outcome
}

// SetSurfaceSink points DispatchInput at a window's handler, which is what
// makes a mapped event reach the content. The backend sets it when it makes
// its window (window.go's NewWindow); nothing else should.
func SetSurfaceSink(fn func(platform.SurfaceEvent) bool) { SurfaceSink = fn }
