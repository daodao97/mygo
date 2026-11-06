package backend

import (
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// sameEvent compares two events by value. platform.SurfaceEvent holds a
// slice (Files), so it is not comparable with ==, and reflect is what is
// left.
func sameEvent(a, b platform.SurfaceEvent) bool { return reflect.DeepEqual(a, b) }

// These tests run anywhere, on any desktop, with cgo off: the mapping above
// is plain Go and that is the point of it (see input.go). Every table below
// is one of the kinds of input Android actually sends, and the numbers in
// it are the ones android.jar gives (T32.md), not guesses.

// evs is a short way to write the events of a result.
func evs(res TouchResult) []platform.SurfaceEvent { return res.Events }

func TestInputTouchDown(t *testing.T) {
	d := NewDecoder(160) // a DIP is a device pixel
	got := d.Touch(TouchInput{
		Action:      actDown,
		ActionIndex: 0,
		Pointers:    []Pointer{{ID: 7, X: 100, Y: 250}},
		EventTimeMs: 1000,
	})
	want := platform.SurfaceEvent{
		Kind: platform.PointerDown, PointerID: 7, Pointer: platform.PointerTouch, X: 100, Y: 250, Button: 0, Clicks: 1,
	}
	if len(got.Events) != 1 || !sameEvent(got.Events[0], want) {
		t.Fatalf("Touch = %+v, want one %+v", got.Events, want)
	}
	if len(got.Pointers) != 1 || got.Pointers[0].ID != 7 || got.Pointers[0].Action != PointerPressed {
		t.Fatalf("Pointers = %+v, want one pressed pointer with id 7", got.Pointers)
	}
}

func TestInputTouchUp(t *testing.T) {
	d := NewDecoder(160)
	d.Touch(TouchInput{Action: actDown, Pointers: []Pointer{{ID: 7, X: 10, Y: 20}}, EventTimeMs: 1000})
	got := d.Touch(TouchInput{Action: actUp, Pointers: []Pointer{{ID: 7, X: 12, Y: 24}}, EventTimeMs: 1050})
	want := platform.SurfaceEvent{
		Kind: platform.PointerUp, PointerID: 7, Pointer: platform.PointerTouch, X: 12, Y: 24, Button: 0, Clicks: 1,
	}
	if len(got.Events) != 1 || !sameEvent(got.Events[0], want) {
		t.Fatalf("Touch = %+v, want one %+v", got.Events, want)
	}
	if got.Pointers[0].Action != PointerReleased {
		t.Fatalf("Pointers = %+v, want the pointer released", got.Pointers)
	}
}

func TestInputTouchMove(t *testing.T) {
	d := NewDecoder(160)
	got := d.Touch(TouchInput{Action: actMove, ActionIndex: 0, Pointers: []Pointer{{ID: 1, X: 5, Y: 6}}, EventTimeMs: 10})
	want := platform.SurfaceEvent{Kind: platform.PointerMove, PointerID: 1, Pointer: platform.PointerTouch, X: 5, Y: 6}
	if len(got.Events) != 1 || !sameEvent(got.Events[0], want) {
		t.Fatalf("Touch = %+v, want one %+v", got.Events, want)
	}
}

// TestInputCoordinates covers the pixel-to-DIP conversion at the densities
// Android actually reports: 160 mdpi (1:1), 320 xhdpi (2:1) and 420 (the
// density of a 1080p phone).
func TestInputCoordinates(t *testing.T) {
	tests := []struct {
		name    string
		dpi     int
		px, py  float32
		wantX   float64
		wantY   float64
		wantDPI float64
	}{
		{"mdpi is one to one", 160, 100, 200, 100, 200, 1},
		{"xhdpi halves", 320, 100, 200, 50, 100, 2},
		{"420 dpi quarters", 420, 420, 840, 160, 320, 2.625},
		{"unknown dpi is one to one", 0, 30, 40, 30, 40, 1},
		{"negative dpi is one to one", -1, 30, 40, 30, 40, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(tt.dpi)
			got := d.Touch(TouchInput{
				Action:   actDown,
				Pointers: []Pointer{{ID: 0, X: tt.px, Y: tt.py}},
			})
			if len(got.Events) != 1 {
				t.Fatalf("Touch = %+v, want one event", got.Events)
			}
			ev := got.Events[0]
			if ev.X != tt.wantX || ev.Y != tt.wantY {
				t.Errorf("X,Y = %v,%v, want %v,%v", ev.X, ev.Y, tt.wantX, tt.wantY)
			}
			if got.Pointers[0].X != tt.wantX || got.Pointers[0].Y != tt.wantY {
				t.Errorf("Pointers = %+v, want the same DIPs as the event", got.Pointers[0])
			}
			if s := d.Scale(); s != tt.wantDPI {
				t.Errorf("Scale = %v, want %v", s, tt.wantDPI)
			}
		})
	}
}

// TestInputMultiTouch is the multi-pointer case the contract cannot carry.
// It pins down the projection: the second finger is in Pointers with its
// own id and DIPs, and the flat stream is unchanged, so a one-pointer
// consumer is not told that its press moved.
func TestInputMultiTouch(t *testing.T) {
	t.Run("second finger down adds no flat event", func(t *testing.T) {
		d := NewDecoder(160)
		d.Touch(TouchInput{
			Action:      actDown,
			ActionIndex: 0,
			Pointers:    []Pointer{{ID: 11, X: 10, Y: 10}},
			EventTimeMs: 1,
		})
		got := d.Touch(TouchInput{
			Action:      actPointerDown,
			ActionIndex: 1,
			Pointers: []Pointer{
				{ID: 11, X: 10, Y: 10},
				{ID: 12, X: 300, Y: 400},
			},
			EventTimeMs: 2,
		})
		if len(got.Events) != 0 {
			t.Fatalf("Events = %+v, want none: the contract cannot say 'not the primary pointer'", got.Events)
		}
		if len(got.Pointers) != 2 {
			t.Fatalf("Pointers = %+v, want both fingers", got.Pointers)
		}
		if got.Pointers[0] != (PointerState{ID: 11, X: 10, Y: 10, Action: PointerNone}) {
			t.Errorf("Pointers[0] = %+v, want the first finger untouched", got.Pointers[0])
		}
		if got.Pointers[1] != (PointerState{ID: 12, X: 300, Y: 400, Action: PointerPressed}) {
			t.Errorf("Pointers[1] = %+v, want the second finger pressed with its own id", got.Pointers[1])
		}
	})

	t.Run("second finger up adds no flat event", func(t *testing.T) {
		d := NewDecoder(160)
		got := d.Touch(TouchInput{
			Action:      actPointerUp,
			ActionIndex: 1,
			Pointers: []Pointer{
				{ID: 11, X: 10, Y: 10},
				{ID: 12, X: 300, Y: 400},
			},
			EventTimeMs: 3,
		})
		if len(got.Events) != 0 {
			t.Fatalf("Events = %+v, want none", got.Events)
		}
		if got.Pointers[1].Action != PointerReleased || got.Pointers[0].Action != PointerNone {
			t.Fatalf("Pointers = %+v, want only the second finger released", got.Pointers)
		}
	})

	t.Run("a two finger move reports every pointer", func(t *testing.T) {
		d := NewDecoder(320) // 2 device pixels per DIP
		got := d.Touch(TouchInput{
			Action:      actMove,
			ActionIndex: 0,
			Pointers: []Pointer{
				{ID: 11, X: 20, Y: 40},
				{ID: 12, X: 100, Y: 200},
			},
			EventTimeMs: 4,
		})
		if len(got.Events) != 1 || got.Events[0].Kind != platform.PointerMove {
			t.Fatalf("Events = %+v, want one PointerMove", got.Events)
		}
		if got.Events[0].X != 10 || got.Events[0].Y != 20 {
			t.Errorf("flat event = %v,%v, want the primary pointer's DIPs 10,20", got.Events[0].X, got.Events[0].Y)
		}
		if got.Pointers[0].X != 10 || got.Pointers[1].X != 50 {
			t.Errorf("Pointers = %+v, want both in DIPs", got.Pointers)
		}
	})

	t.Run("cancel marks every pointer", func(t *testing.T) {
		d := NewDecoder(160)
		got := d.Touch(TouchInput{
			Action:      actCancel,
			Pointers:    []Pointer{{ID: 1, X: 1, Y: 1}, {ID: 2, X: 2, Y: 2}},
			EventTimeMs: 5,
		})
		if len(got.Events) != 0 {
			t.Errorf("Events = %+v, want none: platform has no PointerCancel kind", got.Events)
		}
		for i, p := range got.Pointers {
			if p.Action != PointerCancelled {
				t.Errorf("Pointers[%d] = %+v, want cancelled", i, p)
			}
		}
	})
}

// TestInputScroll covers ACTION_SCROLL, a wheel on a mouse or trackpad. The
// vertical sign flips (Android is positive scrolling up) and the horizontal
// one does not.
func TestInputScroll(t *testing.T) {
	d := NewDecoder(160)
	got := d.Touch(TouchInput{
		Action:      actScroll,
		Pointers:    []Pointer{{ID: 0, X: 50, Y: 60}},
		ScrollX:     3,
		ScrollY:     4,
		EventTimeMs: 6,
	})
	want := platform.SurfaceEvent{
		Kind: platform.PointerScroll, X: 50, Y: 60, DX: 3, DY: -4, Precise: true,
	}
	if len(got.Events) != 1 || !sameEvent(got.Events[0], want) {
		t.Fatalf("Touch = %+v, want one %+v", got.Events, want)
	}
}

// TestInputPointerType pins the device each flat event names. A contact — a
// press, a release and a move between them — is a finger, so it carries
// PointerTouch and enters the content's touch layer. Hover, a wheel scroll
// and a release outside come from a mouse (or a pen), so they stay
// PointerMouse and do not.
func TestInputPointerType(t *testing.T) {
	tests := []struct {
		name string
		in   TouchInput
		want platform.PointerKind
	}{
		{"a press is a finger", TouchInput{Action: actDown, Pointers: []Pointer{{ID: 0, X: 4, Y: 6}}}, platform.PointerTouch},
		{"a release is a finger", TouchInput{Action: actUp, Pointers: []Pointer{{ID: 0, X: 4, Y: 6}}}, platform.PointerTouch},
		{"a move between them is a finger", TouchInput{Action: actMove, Pointers: []Pointer{{ID: 0, X: 4, Y: 6}}}, platform.PointerTouch},
		{"hover is a mouse", TouchInput{Action: actHoverMove, Pointers: []Pointer{{ID: 0, X: 4, Y: 6}}}, platform.PointerMouse},
		{"a wheel is a mouse", TouchInput{Action: actScroll, Pointers: []Pointer{{ID: 0, X: 4, Y: 6}}, ScrollY: 3}, platform.PointerMouse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(160)
			got := d.Touch(tt.in)
			if len(got.Events) != 1 {
				t.Fatalf("Events = %+v, want one", got.Events)
			}
			if got.Events[0].Pointer != tt.want {
				t.Errorf("Pointer = %v, want %v", got.Events[0].Pointer, tt.want)
			}
		})
	}
}

// TestInputPointerID pins the id a contact's flat event carries: the value
// Java read from MotionEvent.getPointerId, which names that finger for the
// whole gesture. It is the id and not the index — a finger at index 0 with
// id 7 reports 7 — because the index shifts as pointers come and go. A mouse
// has one pointer, so hover and a wheel carry 0, the contract's value for a
// device with no multi-touch.
func TestInputPointerID(t *testing.T) {
	tests := []struct {
		name string
		in   TouchInput
		want uint64
	}{
		{"a press carries the finger's id", TouchInput{Action: actDown, Pointers: []Pointer{{ID: 7, X: 4, Y: 6}}}, 7},
		{"a move carries it too", TouchInput{Action: actMove, Pointers: []Pointer{{ID: 7, X: 4, Y: 6}}}, 7},
		{"a release carries it", TouchInput{Action: actUp, Pointers: []Pointer{{ID: 7, X: 4, Y: 6}}}, 7},
		{"a move naming the second pointer carries its id", TouchInput{
			Action: actMove, ActionIndex: 1,
			Pointers: []Pointer{{ID: 7, X: 4, Y: 6}, {ID: 9, X: 40, Y: 60}},
		}, 9},
		{"hover is a mouse and has no second pointer", TouchInput{Action: actHoverMove, Pointers: []Pointer{{ID: 3, X: 4, Y: 6}}}, 0},
		{"a wheel is a mouse", TouchInput{Action: actScroll, Pointers: []Pointer{{ID: 3, X: 4, Y: 6}}, ScrollY: 3}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(160)
			got := d.Touch(tt.in)
			if len(got.Events) != 1 {
				t.Fatalf("Events = %+v, want one", got.Events)
			}
			if got.Events[0].PointerID != tt.want {
				t.Errorf("PointerID = %d, want %d", got.Events[0].PointerID, tt.want)
			}
		})
	}
}

// TestInputButton covers a mouse's buttons, which Android reports in
// buttonState: the middle button is platform's Button 2, and the secondary
// button survives the release even though ACTION_UP's buttonState may be
// empty by then.
func TestInputButton(t *testing.T) {
	t.Run("secondary press and release", func(t *testing.T) {
		d := NewDecoder(160)
		down := d.Touch(TouchInput{
			Action:      actDown,
			Pointers:    []Pointer{{ID: 0, X: 1, Y: 1}},
			ButtonState: buttonSecondary,
			EventTimeMs: 1,
		})
		if len(down.Events) != 1 || down.Events[0].Button != 1 {
			t.Fatalf("down = %+v, want Button 1", down.Events)
		}
		up := d.Touch(TouchInput{
			Action:      actUp,
			Pointers:    []Pointer{{ID: 0, X: 1, Y: 1}},
			ButtonState: 0,
			EventTimeMs: 2,
		})
		if len(up.Events) != 1 || up.Events[0].Button != 1 {
			t.Fatalf("up = %+v, want the button remembered as 1", up.Events)
		}
	})

	t.Run("middle press", func(t *testing.T) {
		d := NewDecoder(160)
		got := d.Touch(TouchInput{
			Action:      actDown,
			Pointers:    []Pointer{{ID: 0, X: 1, Y: 1}},
			ButtonState: buttonTertiary,
			EventTimeMs: 1,
		})
		if len(got.Events) != 1 || got.Events[0].Button != 2 {
			t.Fatalf("down = %+v, want Button 2", got.Events)
		}
	})

	t.Run("button press and release add no flat event", func(t *testing.T) {
		d := NewDecoder(160)
		pat := []Pointer{{ID: 0, X: 1, Y: 1}}
		for _, action := range []int{actButtonPress, actButtonRelease} {
			got := d.Touch(TouchInput{Action: action, Pointers: pat, ButtonState: buttonSecondary, EventTimeMs: 1})
			if len(got.Events) != 0 {
				t.Errorf("action %d: Events = %+v, want none: the DOWN/UP pair already carried the button", action, got.Events)
			}
			if got.Pointers[0].Action == PointerNone {
				t.Errorf("action %d: Pointers = %+v, want the pointer's state recorded", action, got.Pointers[0])
			}
		}
	})
}

// TestInputClicks counts quick successive presses, and the releases that
// belong to them.
func TestInputClicks(t *testing.T) {
	d := NewDecoder(160)
	pat := []Pointer{{ID: 0, X: 1, Y: 1}}
	cases := []struct {
		name   string
		action int
		at     int64
		want   int
		events int
	}{
		{"first press is click one", actDown, 1000, 1, 1},
		{"first release carries it", actUp, 1010, 1, 1},
		{"a second press inside the window is click two", actDown, 1200, 2, 1},
		{"its release carries it", actUp, 1210, 2, 1},
		{"a third press too late starts again", actDown, 2000, 1, 1},
		{"a press with no time at all is still one", actUp, 2005, 1, 1},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := d.Touch(TouchInput{Action: tt.action, Pointers: pat, EventTimeMs: tt.at})
			if len(got.Events) != tt.events {
				t.Fatalf("Events = %+v, want %d", got.Events, tt.events)
			}
			if got.Events[0].Clicks != tt.want {
				t.Errorf("Clicks = %d, want %d", got.Events[0].Clicks, tt.want)
			}
		})
	}

	t.Run("a backwards clock does not read as a double click", func(t *testing.T) {
		d := NewDecoder(160)
		d.Touch(TouchInput{Action: actDown, Pointers: pat, EventTimeMs: 5000})
		got := d.Touch(TouchInput{Action: actDown, Pointers: pat, EventTimeMs: 4000})
		if got.Events[0].Clicks != 1 {
			t.Errorf("Clicks = %d, want 1", got.Events[0].Clicks)
		}
	})
}

// TestInputEdgeCases is the malformed end of the wire: an event with no
// pointer, an action nothing here knows, an index past the end of the list,
// and coordinates that are not numbers.
func TestInputEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		in        TouchInput
		wantEvent int
		wantPtrs  int
	}{
		{"no pointers at all", TouchInput{Action: actDown, EventTimeMs: 1}, 0, 0},
		{"an action nothing knows", TouchInput{Action: 999, Pointers: []Pointer{{ID: 1}}, EventTimeMs: 1}, 0, 1},
		{"an index past the end falls back to the first pointer", TouchInput{
			Action: actDown, ActionIndex: 9, Pointers: []Pointer{{ID: 5, X: 3, Y: 4}}, EventTimeMs: 1,
		}, 1, 1},
		{"a negative index falls back to the first pointer", TouchInput{
			Action: actDown, ActionIndex: -1, Pointers: []Pointer{{ID: 5, X: 3, Y: 4}}, EventTimeMs: 1,
		}, 1, 1},
		{"an empty pointer list with an index set", TouchInput{Action: actMove, ActionIndex: 3, EventTimeMs: 1}, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(160)
			got := d.Touch(tt.in)
			if len(got.Events) != tt.wantEvent {
				t.Errorf("Events = %+v, want %d", got.Events, tt.wantEvent)
			}
			if len(got.Pointers) != tt.wantPtrs {
				t.Errorf("Pointers = %+v, want %d", got.Pointers, tt.wantPtrs)
			}
		})
	}

	t.Run("an index past the end really names the first pointer", func(t *testing.T) {
		d := NewDecoder(160)
		got := d.Touch(TouchInput{
			Action: actDown, ActionIndex: 9,
			Pointers:    []Pointer{{ID: 5, X: 3, Y: 4}, {ID: 6, X: 30, Y: 40}},
			EventTimeMs: 1,
		})
		if got.Events[0].X != 3 || got.Pointers[0].Action != PointerPressed {
			t.Fatalf("Events = %+v Pointers = %+v, want the first pointer", got.Events, got.Pointers)
		}
	})

	// NaN and infinities: a non-finite coordinate would poison every layout
	// computed from it, so that coordinate becomes 0 rather than travelling
	// on. A finite one beside it is untouched.
	for _, tc := range []struct {
		name         string
		x, y         float32
		wantX, wantY float64
	}{
		{"NaN x", float32(math.NaN()), 5, 0, 5},
		{"positive infinity x", float32(math.Inf(1)), 5, 0, 5},
		{"negative infinity y", 5, float32(math.Inf(-1)), 5, 0},
	} {
		t.Run("non-finite "+tc.name, func(t *testing.T) {
			d := NewDecoder(160)
			got := d.Touch(TouchInput{
				Action:   actDown,
				Pointers: []Pointer{{ID: 0, X: tc.x, Y: tc.y}},
			})
			if len(got.Events) != 1 {
				t.Fatalf("Events = %+v, want one", got.Events)
			}
			if got.Events[0].X != tc.wantX || got.Events[0].Y != tc.wantY {
				t.Errorf("X,Y = %v,%v, want %v,%v", got.Events[0].X, got.Events[0].Y, tc.wantX, tc.wantY)
			}
			if got.Pointers[0].X != tc.wantX || got.Pointers[0].Y != tc.wantY {
				t.Errorf("Pointers = %+v, want %v,%v too", got.Pointers[0], tc.wantX, tc.wantY)
			}
		})
	}
}

// TestInputKeyMapping is the Android-to-platform key table, including the
// keys it does not have: an unmapped code must come back reported, not as
// KeyUnknown.
func TestInputKeyMapping(t *testing.T) {
	tests := []struct {
		name    string
		keyCode int
		want    platform.Key
	}{
		{"a letter", kcLetterA + ('k' - 'a'), platform.KeyK},
		{"the last letter", kcLetterA + 25, platform.KeyZ},
		{"a digit", kcDigit0 + 4, platform.Key4},
		{"function twelve", kcF1 + 11, platform.KeyF12},
		{"enter", kcEnter, platform.KeyEnter},
		{"escape", kcEscape, platform.KeyEscape},
		{"Android's DEL is backspace", kcDel, platform.KeyBackspace},
		{"forward delete", kcForwardDel, platform.KeyDelete},
		{"tab", kcTab, platform.KeyTab},
		{"space", kcSpace, platform.KeySpace},
		{"insert", kcInsert, platform.KeyInsert},
		{"home", kcMoveHome, platform.KeyHome},
		{"end", kcMoveEnd, platform.KeyEnd},
		{"page up", kcPageUp, platform.KeyPageUp},
		{"page down", kcPageDown, platform.KeyPageDown},
		{"the up arrow", kcDpadUp, platform.KeyUp},
		{"the down arrow", kcDpadDown, platform.KeyDown},
		{"the left arrow", kcDpadLeft, platform.KeyLeft},
		{"the right arrow", kcDpadRight, platform.KeyRight},
		{"minus", kcMinus, platform.KeyMinus},
		{"equals", kcEquals, platform.KeyEqual},
		{"comma", kcComma, platform.KeyComma},
		{"period", kcPeriod, platform.KeyPeriod},
		{"slash", kcSlash, platform.KeySlash},
		{"semicolon", kcSemicolon, platform.KeySemicolon},
		{"apostrophe is the quote key", kcApostrophe, platform.KeyQuote},
		{"left bracket", kcLeftBracket, platform.KeyBracketLeft},
		{"right bracket", kcRightBracket, platform.KeyBracketRight},
		{"backslash", kcBackslash, platform.KeyBackslash},
		{"grave is the backquote key", kcGrave, platform.KeyBackquote},
		{"menu is the context menu key", kcMenu, platform.KeyContextMenu},
		{"Android's Back is platform's back", kcBack, platform.KeyBack},
		{"the forward key", kcForward, platform.KeyForward},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(160)
			got, outcome := d.Key(KeyEventInput{KeyCode: tt.keyCode, Action: keyActionDown})
			if outcome != KeyMapped {
				t.Fatalf("outcome = %v, want KeyMapped", outcome)
			}
			if len(got) != 1 || got[0].Kind != platform.KeyPressed || got[0].Key != tt.want {
				t.Fatalf("Key = %+v, want one KeyPressed with %v", got, tt.want)
			}
			if name := KeyCodeName(tt.keyCode); name == "KEYCODE_"+strconv.Itoa(tt.keyCode) {
				t.Errorf("KeyCodeName(%d) = %q, want a real Android name", tt.keyCode, name)
			}
		})
	}
}

// TestInputKeyUnmapped: Android has many more keys than platform.Key does,
// and one of them must not quietly become KeyUnknown.
func TestInputKeyUnmapped(t *testing.T) {
	for _, tc := range []struct {
		name    string
		keyCode int
	}{
		{"the camera key", 27},    // KEYCODE_CAMERA
		{"the volume up key", 24}, // KEYCODE_VOLUME_UP
		{"something no Android has either", 9999},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDecoder(160)
			got, outcome := d.Key(KeyEventInput{KeyCode: tc.keyCode, Action: keyActionDown})
			if outcome != KeyUnmapped {
				t.Errorf("outcome = %v, want KeyUnmapped", outcome)
			}
			if len(got) != 0 {
				t.Errorf("Key = %+v, want nothing delivered", got)
			}
			if name := KeyCodeName(tc.keyCode); name != "KEYCODE_"+strconv.Itoa(tc.keyCode) && name == "" {
				t.Errorf("KeyCodeName(%d) = %q, want the code when the name is unknown", tc.keyCode, name)
			}
		})
	}
}

// TestInputKeyModifiers: a modifier on its own is ModifiersChanged, and the
// key's own bit goes in on the press and out on the release.
func TestInputKeyModifiers(t *testing.T) {
	tests := []struct {
		name    string
		keyCode int
		action  int
		meta    int
		want    platform.Modifiers
	}{
		{"shift down", kcShiftLeft, keyActionDown, 0, platform.ModShift},
		{"shift down with the bit already set", kcShiftRight, keyActionDown, metaShiftOn, platform.ModShift},
		{"shift up clears it", kcShiftLeft, keyActionUp, metaShiftOn, 0},
		{"ctrl down keeps alt", kcCtrlLeft, keyActionDown, metaAltOn, platform.ModAlt | platform.ModCtrl},
		{"alt up clears alt keeps shift", kcAltRight, keyActionUp, metaAltOn | metaShiftOn, platform.ModShift},
		{"the Command key is Super", kcMetaLeft, keyActionDown, 0, platform.ModSuper},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(160)
			got, outcome := d.Key(KeyEventInput{KeyCode: tt.keyCode, Action: tt.action, MetaState: tt.meta})
			if outcome != KeyModifierOnly {
				t.Fatalf("outcome = %v, want KeyModifierOnly", outcome)
			}
			if len(got) != 1 || got[0].Kind != platform.ModifiersChanged || got[0].Mods != tt.want {
				t.Fatalf("Key = %+v, want one ModifiersChanged with %v", got, tt.want)
			}
		})
	}
}

// TestInputKeyText: the character a key types comes with the key event, from
// Android's own getUnicodeChar, and a shortcut does not type anything.
func TestInputKeyText(t *testing.T) {
	tests := []struct {
		name      string
		in        KeyEventInput
		wantEvent int
		wantText  string
	}{
		{
			name:      "a letter types itself",
			in:        KeyEventInput{KeyCode: kcLetterA, Action: keyActionDown, UnicodeChar: 'a'},
			wantEvent: 2, wantText: "a",
		},
		{
			name:      "a shifted letter types the capital",
			in:        KeyEventInput{KeyCode: kcLetterA, Action: keyActionDown, MetaState: metaShiftOn, UnicodeChar: 'A'},
			wantEvent: 2, wantText: "A",
		},
		{
			name:      "a shortcut types nothing",
			in:        KeyEventInput{KeyCode: kcLetterA, Action: keyActionDown, MetaState: metaCtrlOn, UnicodeChar: 1},
			wantEvent: 1,
		},
		{
			name:      "a key that types nothing emits only the key",
			in:        KeyEventInput{KeyCode: kcDpadLeft, Action: keyActionDown, UnicodeChar: 0},
			wantEvent: 1,
		},
		{
			name:      "a release types nothing",
			in:        KeyEventInput{KeyCode: kcLetterA, Action: keyActionUp, UnicodeChar: 'a'},
			wantEvent: 1,
		},
		{
			name:      "an auto-repeat is marked",
			in:        KeyEventInput{KeyCode: kcLetterA, Action: keyActionDown, RepeatCount: 3, UnicodeChar: 'a'},
			wantEvent: 2, wantText: "a",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(160)
			got, outcome := d.Key(tt.in)
			if outcome != KeyMapped {
				t.Fatalf("outcome = %v, want KeyMapped", outcome)
			}
			if len(got) != tt.wantEvent {
				t.Fatalf("Key = %+v, want %d events", got, tt.wantEvent)
			}
			if tt.wantEvent == 2 {
				if got[1].Kind != platform.TextInput || got[1].Text != tt.wantText {
					t.Errorf("second event = %+v, want TextInput %q", got[1], tt.wantText)
				}
			}
			if tt.in.RepeatCount > 0 && !got[0].Repeat {
				t.Errorf("Repeat = false, want true for repeat count %d", tt.in.RepeatCount)
			}
		})
	}

	t.Run("a control character is not text", func(t *testing.T) {
		d := NewDecoder(160)
		got, _ := d.Key(KeyEventInput{KeyCode: kcEnter, Action: keyActionDown, UnicodeChar: '\n'})
		if len(got) != 1 {
			t.Fatalf("Key = %+v, want only the key event for a newline", got)
		}
	})
}

// TestInputKeyIgnored: an action with nothing to deliver.
func TestInputKeyIgnored(t *testing.T) {
	d := NewDecoder(160)
	got, outcome := d.Key(KeyEventInput{KeyCode: kcLetterA, Action: keyActionMultiple})
	if outcome != KeyIgnored {
		t.Errorf("outcome = %v, want KeyIgnored", outcome)
	}
	if len(got) != 0 {
		t.Errorf("Key = %+v, want nothing delivered", got)
	}
	if _, outcome := d.Key(KeyEventInput{KeyCode: kcShiftLeft, Action: keyActionMultiple}); outcome != KeyIgnored {
		t.Errorf("a modifier with ACTION_MULTIPLE: outcome = %v, want KeyIgnored", outcome)
	}
}

// TestInputModifiersFromMeta is the meta state on its own, which touch
// events do not carry but a key event's non-modifier keys do.
func TestInputModifiersFromMeta(t *testing.T) {
	for _, tt := range []struct {
		name string
		meta int
		want platform.Modifiers
	}{
		{"nothing", 0, 0},
		{"shift", metaShiftOn, platform.ModShift},
		{"all four", metaShiftOn | metaCtrlOn | metaAltOn | metaMetaOn,
			platform.ModShift | platform.ModCtrl | platform.ModAlt | platform.ModSuper},
		{"only the left-hand bits are not read", 16 | 8192 | 131072, 0},
		{"the aggregate bits beside the side ones", 16 | metaAltOn | 8192 | metaCtrlOn | 131072 | metaMetaOn,
			platform.ModAlt | platform.ModCtrl | platform.ModSuper},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ModifiersFromMeta(tt.meta); got != tt.want {
				t.Errorf("ModifiersFromMeta(%d) = %v, want %v", tt.meta, got, tt.want)
			}
		})
	}
}

// TestInputDispatchInput is the delivery path: every event notes that the
// person did something, and the sink gets it.
func TestInputDispatchInput(t *testing.T) {
	got := []platform.SurfaceEvent{}
	SurfaceSink = func(ev platform.SurfaceEvent) bool {
		got = append(got, ev)
		return true
	}
	defer func() { SurfaceSink = nil }()

	DispatchInput([]platform.SurfaceEvent{{Kind: platform.PointerDown}, {Kind: platform.PointerUp}})
	if len(got) != 2 {
		t.Fatalf("the sink saw %d events, want 2", len(got))
	}
	if !platform.LastInput().IsZero() {
		// LastInput is the clock's, so only that it was noted is checked.
		return
	}
	t.Error("DispatchInput did not note the input (platform.LastInput is zero)")
}
