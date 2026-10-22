package platform

import (
	"github.com/egoist/mygo/transfer"
	"image"
)

// Surface is the drawing area of a window created with
// WindowOptions.Surface, which shows content MyGo draws itself (package
// ui) instead of a webview. Its methods run on the main thread.
type Surface interface {
	// Native returns the native objects a GPU renderer draws into, which it
	// calls before making one: a surface that shows frames drawn in memory
	// through those objects (Windows' Composed) lets go of them then.
	Native() SurfaceNative
	// Size returns the size of the drawing area in DIPs and how many
	// device pixels a DIP is.
	Size() (width, height, scale float64)
	// RequestFrame asks for a SurfaceFrame event at the next opportunity
	// to draw: the next display refresh, WM_PAINT or GTK draw. Requests
	// made before it comes coalesce.
	RequestFrame()
	// RefreshRate returns how many times a second the display showing the
	// surface refreshes, at most, 0 when unknown.
	RefreshRate() float64
	// PresentPixels shows a frame drawn in memory: premultiplied BGRA rows
	// of stride bytes, width×height device pixels. It is called while
	// handling a SurfaceFrame event.
	PresentPixels(pix []byte, stride, width, height int)
	// SetCursor sets the pointer's shape over the surface.
	SetCursor(c Cursor)
	// SetTextInput configures the focused editor and its surrounding text,
	// or turns input off. ReadOnly activates system selection without edits
	// or a software keyboard on surfaces that support native selection.
	SetTextInput(t TextInputState)
	// UpdateAccessibility gives assistive technology the content's
	// elements. The content calls it after every frame once the surface
	// sent AccessibilityOn.
	UpdateAccessibility(tree *AccessTree)
	// StartDataDrag starts a native transfer; CancelDataDrag cancels the
	// source owned by this surface. Both run on the main thread.
	StartDataDrag(DragRequest)
	CancelDataDrag()
	// SetDropFormats registers the formats the content can receive.
	SetDropFormats([]transfer.Format)
	// NewWebView creates a web view under the surface, hidden until
	// PlaceWebViews shows it. Of opts, the options of a webview apply
	// (UserScripts, Schemes, DevTools, UserAgent, Zoom, BackgroundColor,
	// Transparent); h hears of its page.
	NewWebView(opts *WindowOptions, h WindowHandler) (WebView, error)
	// PlaceWebViews shows the web views of the surface where the content's
	// last frame shows them, in the order it painted them, and hides the
	// others, which keep their pages. A press on one, which takes it,
	// comes as WebViewPress.
	PlaceWebViews(views []WebViewPlacement)
}

// DamageSurface is a Surface that shows a frame drawn in memory by what
// changed since the last one: Linux's, whose toolkit then repaints, and
// the compositor takes, only that.
type DamageSurface interface {
	// PresentDamage is PresentPixels for a frame that differs from the
	// last one presented only within damage, in device pixels.
	PresentDamage(pix []byte, stride, width, height int, damage []image.Rectangle)
}

// OccludableSurface is a Surface that tells when nothing of it shows, as
// when other windows cover its window, while the system may still give it
// frames: the content then draws nothing that moves until it shows again.
type OccludableSurface interface {
	// Occluded reports whether nothing of the surface shows on screen. The
	// surface sends SurfaceShown once some of it shows again.
	Occluded() bool
}

// WideGamutSurface is a Surface that tells whether its window's screen
// shows colors outside the sRGB gamut (macOS's).
type WideGamutSurface interface {
	// WideGamut reports whether the screen showing most of the window
	// shows colors outside the sRGB gamut, as Display P3 screens do.
	WideGamut() bool
}

// MaterialSurface is a Surface whose window can show a material
// (vibrancy) behind the content, wherever its frames are transparent.
type MaterialSurface interface {
	// ShowsMaterial reports whether the window shows a material behind the
	// content now: what its frames leave transparent shows it, rather than
	// nothing.
	ShowsMaterial() bool
}

// PacedSurface is a Surface whose frames following each other come at the
// display's refreshes, marked Paced: a renderer that waits for the display
// lets such a frame draw once the display can take it, with FrameNow.
type PacedSurface interface {
	// FrameNow sends a SurfaceFrame at once, as a refresh would, for a
	// frame a Paced one left to draw.
	FrameNow()
}

// MaximizeButtonSurface is a Surface whose window makes the maximize
// button its content draws (in a window without its own, as
// WindowOptions.HideWindowButtons) the system's for the pointer: Windows
// shows the snap layouts over it. Package ui reaches it only once a view
// marks such a button (ui.Element.MaximizeButton), for apps that mark none
// not to link it.
type MaximizeButtonSurface interface {
	// SetMaximizeButton tells where the content shows the window's
	// maximize button, in DIPs from the top-left corner of the content.
	// The zero RectF is nowhere.
	SetMaximizeButton(r RectF)
}

// IdleSurface is a Surface that can give back memory once frames stop.
type IdleSurface interface {
	// Idle tells that no frame came for a while: the surface may give
	// back memory that drawing frames took and freed.
	Idle()
}

// SwapIntervalSurface is a Surface whose present blocks until a vsync, and
// which can be told not to (Android's EGL). Optional because only EGL works
// this way: GTK's area and a Windows swap chain present on their own
// schedule and have no interval to set.
//
// It exists for measurement: with the interval at its default the present
// span reads the frame's waiting, which is the frame's *pacing* and not its
// work — a frame that lands just after a vsync waits most of an interval,
// one in a steady stream waits almost none. Setting it to 0 separates the
// two, and the pair of readings is what says whether a frame is slow or
// merely early.
type SwapIntervalSurface interface {
	// SetSwapInterval is how many vsyncs the present waits for: 1 is the
	// default and 0 returns as soon as the buffer is handed over.
	SetSwapInterval(n int)
}

// FramePresenter is a surface whose frames drawn on the GPU need showing
// when the renderer is done with them. EGL is such a renderer: unlike GTK's
// GtkGLArea (which swaps for its area) and a Windows swap chain (which
// presents on its own), it leaves the swap to the code that drew, so a
// window drawing on the GPU has to say when.
type FramePresenter interface {
	// SetPresented tells the surface a finished frame is in the renderer's
	// back buffer, so it can be put on the screen.
	SetPresented()
}

// TextInputState is the state of the text input that has the keyboard,
// for input methods.
type TextInputState struct {
	// Active is true while an editable element has the keyboard.
	Active bool
	// ID identifies a focused built-in editor (zero for a custom TextCaret).
	// Bounds is its visible rectangle in surface DIPs.
	ID     uint64
	Bounds RectF
	// Password keeps native input methods from retaining surrounding text.
	ReadOnly  bool
	Password  bool
	Multiline bool
	// Shared editor history for native undo managers. Custom text handlers
	// leave these false and retain ownership of their own history.
	CanUndo, CanRedo bool
	Options          TextInputOptions
	// Accessory is JSON describing application-defined software-keyboard
	// actions. AccessoryID identifies their owner, including custom inputs.
	// Desktop backends ignore the accessory while retaining text input.
	Accessory   string
	AccessoryID uint64
	// Caret is the caret's rectangle, in DIPs relative to the surface.
	Caret RectF
	// Text is the input's text around the caret, and Start and End are the
	// selection in it, in runes: both the caret when nothing is selected.
	// Input methods read it to edit what was typed before, as the accents
	// of macOS's press and hold replace the letter they decorate.
	Text       string
	Start, End int
	// Client supplies full text and geometry for a custom text element. The
	// Text/Start/End snapshot remains the plain-widget and TextCaret fallback.
	Client TextInputClient
	// NoInputMethod keeps input methods out while Active: keys type the
	// characters of the keyboard's layout as TextInput, and nothing is
	// composed, as winit's set_ime_allowed(false). Browsers cannot turn
	// input methods off, and ignore it.
	NoInputMethod bool
}

// NativeTextSelectionSurface uses system selection visuals and gestures for
// editable and read-only selectable text. Passwords and custom TextCaret
// handlers still draw in Go.
type NativeTextSelectionSurface interface{ NativeTextSelection() bool }

// TextGeometryHandler optionally answers UIKit-style text geometry queries.
// Indices refer to the last TextInputState.Text, in runes. Queries run on the
// UI thread and never change focus or build a frame.
type TextGeometryHandler interface {
	TextGeometry(TextGeometryQuery) TextGeometry
}

type TextGeometryQuery struct {
	ID         uint64
	Kind       string // "caret", "selection", or "hit"
	Start, End int
	X, Y       float64
}

type TextGeometry struct {
	Valid bool
	Caret RectF
	Rects []TextSelectionRect
	Index int
}

type TextSelectionRect struct {
	RectF
	RTL bool
}

// TextInputOptions are semantic keyboard hints. Backends without software
// keyboard traits ignore them; they never restrict accepted text.
type TextInputOptions struct {
	Keyboard, Return, Content, Correction, Capitalization, Dismiss string
}

// SurfaceNative holds the native objects of a Surface.
type SurfaceNative struct {
	// HWND is the surface's child window (Windows).
	HWND uintptr
	// Composed tells that the window has no redirection bitmap, which
	// would hide its material (Windows): a GPU renderer presents through
	// DirectComposition, with the frames' alpha, which the material shows
	// through.
	Composed bool
	// View is the surface's NSView (macOS) or UIView (iOS), and Layer its layer.
	View, Layer uintptr
	// Widget is the surface's GtkGLArea or GtkDrawingArea, and GLArea the
	// GtkGLArea while its render signal draws a frame with OpenGL, in the
	// context it made current (Linux).
	Widget, GLArea uintptr
	// Canvas is the number of the canvas's WebGL 2 context (package gl's
	// Context.ID), current while SurfaceFrame events are handled
	// (browsers).
	Canvas uintptr
	// Present is the surface itself, when a frame drawn on the GPU needs
	// showing by the drawing code (Android, where EGL leaves the swap to
	// it — see FramePresenter). It is the one field here that is not a
	// native pointer, because the thing the renderer has to reach is the
	// surface, and it has no handle the renderer could call through.
	Present Surface
}

// RectF is a rectangle in DIPs with fractional coordinates.
type RectF struct{ X, Y, W, H float64 }

// Contains reports whether the point is inside r.
func (r RectF) Contains(x, y float64) bool {
	return x >= r.X && y >= r.Y && x < r.X+r.W && y < r.Y+r.H
}

// SurfaceEventKind is the kind of a SurfaceEvent.
type SurfaceEventKind uint8

const (
	// SurfaceFrame asks the content to draw a frame now.
	SurfaceFrame SurfaceEventKind = iota
	// SurfaceResize reports a new Size; a frame follows.
	SurfaceResize
	// PointerMove reports the pointer at X, Y.
	PointerMove
	// PointerDown and PointerUp report Button pressed or released at X,
	// Y; Clicks counts quick successive presses (2 for a double click),
	// by the system's double-click settings. A release may leave it 0, for
	// its press's.
	PointerDown
	PointerUp
	// PointerLeave reports the pointer left the surface.
	PointerLeave
	// PointerScroll scrolls by DX, DY DIPs at X, Y: positive DY moves the
	// view down the content, as dragging a scroll bar's thumb down does.
	PointerScroll
	// PointerZoom zooms by Scale at X, Y, as pinching a touchpad does: 2
	// shows the content twice as large as it was.
	PointerZoom
	// KeyPressed and KeyReleased report Key with Mods; Repeat marks
	// auto-repeat.
	KeyPressed
	KeyReleased
	// ModifiersChanged reports the modifier keys held, Mods, as one of
	// them goes down or up on its own.
	ModifiersChanged
	// TextInput inserts Text, typed or committed by an input method.
	TextInput
	// TextComposition shows Text as the input method's composition, its
	// caret at rune Caret; an empty Text ends the composition.
	TextComposition
	// SurfaceFocus and SurfaceBlur report the surface gaining or losing
	// the keyboard.
	SurfaceFocus
	SurfaceBlur
	// SurfaceCommand performs the edit command Text: "copy", "cut",
	// "paste", "pasteAndMatchStyle", "selectAll", "undo", "redo" or
	// "delete" (Edit menu roles), or a Format menu role's: "bold",
	// "italic", "underline", "strikethrough", "alignLeft", "alignCenter"
	// or "alignRight".
	SurfaceCommand
	// FileDragOver reports files of another app dragged over X, Y, and
	// FileDragLeave that they left. FileDrop drops Files at X, Y.
	// WindowHandler.SurfaceEvent returns whether the content takes them
	// there.
	FileDragOver
	FileDragLeave
	FileDrop
	// AccessibilityOn reports that assistive technology asked about the
	// surface: the content calls UpdateAccessibility at once and after
	// every frame from then on.
	AccessibilityOn
	// AccessAction performs Action on the element ID of the accessibility
	// tree, with Text the value of AccessSetValue.
	AccessAction
	// SurfaceShown reports that some of an OccludableSurface shows again
	// after none did.
	SurfaceShown
	// DataDragOver queries a destination; DataDrop commits a transfer.
	// DataDragLeave clears its hover state. Drag carries the query/result.
	DataDragOver
	DataDragLeave
	DataDrop
	// PointerCancel aborts a press without activating the element.
	PointerCancel
	// TextSelectionChanged changes the selection in the last TextInputState.Text.
	TextSelectionChanged
	// SurfaceMemoryPressure asks the renderer to release disposable buffers.
	SurfaceMemoryPressure
	// SurfacePresented confirms the first native presentation completed.
	SurfacePresented
	SurfaceKeyboardDismiss
	SurfaceGesture
	// SurfaceInputAction invokes Text on the input accessory owned by ID.
	SurfaceInputAction
	// SurfaceRenew reports that the native objects a GPU renderer draws
	// into changed (Surface.Native), as a window of Windows composes its
	// frames once it has web views: the content makes its renderer again.
	SurfaceRenew
	// WebViewPress reports that Button went down at X, Y on a web view
	// the content shows there (Surface.PlaceWebViews), which takes the
	// press and what follows: the content notes it, as popovers close for
	// a press outside them, but handles none of it.
	WebViewPress
	// WebViewTabOut reports that Tab moved the keyboard out of WebView
	// past its page's last element, or Shift+Tab (Mods) past its first:
	// the surface has the keyboard again, and the content moves the focus
	// on from the web view's element.
	WebViewTabOut
)

// DrawableSurface observes native drawable presentation (Metal on iOS).
// Renderers call WatchDrawable before presenting; the surface reports
// SurfacePresented on the UI thread once, also for direct CPU presentation.
type DrawableSurface interface{ WatchDrawable(drawable uintptr) }

// PointerType identifies the device producing a pointer event.
type PointerType uint8

const (
	PointerMouse PointerType = iota
	PointerTouch
	PointerPen
)

// SurfaceEvent is input on a Surface, or a change of it.
type SurfaceEvent struct {
	Gesture, Phase string
	Rotation       float64
	Kind           SurfaceEventKind
	PointerID      uint64
	PointerType    PointerType
	// X and Y locate the pointer in DIPs relative to the surface.
	X, Y float64
	// Button is 0 for the primary button, 1 the secondary, 2 the middle.
	Button int
	Clicks int
	// DX and DY are scroll distances in DIPs; Precise marks touchpads and
	// other devices that scroll by pixels rather than by lines.
	DX, DY  float64
	Precise bool
	// Scale is PointerZoom's factor or SurfaceGesture's cumulative scale.
	Scale float64
	Key   Key
	// PhysicalKey is the key by its position on an ANSI US keyboard,
	// whatever the active layout types; KeyUnknown where the backend does
	// not report it. Key and PhysicalKey agree on a US layout.
	PhysicalKey Key
	Mods        Modifiers
	Repeat      bool
	Text        string
	Caret       int
	// Snapshot places the caret at From + Caret after a TextInput replacement.
	// UIKit sends snapshots of its surrounding-text proxy this way.
	Snapshot bool
	// A TextComposition snapshot contains the entire native text proxy.
	// MarkedStart/End delimit its provisional runes; Caret is in that proxy.
	// The remaining prefix/suffix are committed without losing surrounding text.
	MarkedStart, MarkedEnd int
	// Replace makes TextInput replace, and TextComposition compose over,
	// the runes From to To of the last TextInputState.Text, instead of the
	// selection.
	Replace  bool
	From, To int
	// Files are the paths of FileDrop's files, or original native paths
	// for DataDrop's legacy file-listener fallback.
	Files []string
	Drag  *DataDragEvent
	// ID and Action are AccessAction's.
	ID     uint64
	Action AccessActionKind
	// WebView is WebViewTabOut's.
	WebView WebView
	// Paced marks a SurfaceFrame of frames following each other at the
	// display's refreshes (PacedSurface): the content may draw it later,
	// once the display can take it, which it tells by returning true from
	// SurfaceEvent; the refreshes go on meanwhile.
	Paced bool
}

// Modifiers are the modifier keys held during an event.
type Modifiers uint8

const (
	ModShift Modifiers = 1 << iota
	ModCtrl
	ModAlt
	// ModSuper is Command on macOS and the Windows key elsewhere.
	ModSuper
)

// Key identifies a key. Letter and digit keys follow the keyboard layout:
// KeyA is the key that types an a.
type Key uint16

const (
	KeyUnknown Key = iota
	KeyEnter
	KeyEscape
	KeyBackspace
	KeyTab
	KeySpace
	KeyDelete
	KeyInsert
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
	KeyMinus
	KeyEqual
	KeyComma
	KeyPeriod
	KeySlash
	KeySemicolon
	KeyQuote
	KeyBracketLeft
	KeyBracketRight
	KeyBackslash
	KeyBackquote
	KeyContextMenu
	Key0
	Key1
	Key2
	Key3
	Key4
	Key5
	Key6
	Key7
	Key8
	Key9
	KeyA
	KeyB
	KeyC
	KeyD
	KeyE
	KeyF
	KeyG
	KeyH
	KeyI
	KeyJ
	KeyK
	KeyL
	KeyM
	KeyN
	KeyO
	KeyP
	KeyQ
	KeyR
	KeyS
	KeyT
	KeyU
	KeyV
	KeyW
	KeyX
	KeyY
	KeyZ
	// KeyBack and KeyForward go back and forward in a history: the side
	// buttons of a mouse, and the keys of keyboards that have them.
	KeyBack
	KeyForward
)

// KeyForRune returns the key typing r unshifted, KeyUnknown for others.
func KeyForRune(r rune) Key {
	switch {
	case r >= 'a' && r <= 'z':
		return KeyA + Key(r-'a')
	case r >= 'A' && r <= 'Z':
		return KeyA + Key(r-'A')
	case r >= '0' && r <= '9':
		return Key0 + Key(r-'0')
	}
	switch r {
	case '-':
		return KeyMinus
	case '=':
		return KeyEqual
	case ',':
		return KeyComma
	case '.':
		return KeyPeriod
	case '/':
		return KeySlash
	case ';':
		return KeySemicolon
	case '\'':
		return KeyQuote
	case '[':
		return KeyBracketLeft
	case ']':
		return KeyBracketRight
	case '\\':
		return KeyBackslash
	case '`':
		return KeyBackquote
	case ' ':
		return KeySpace
	}
	return KeyUnknown
}

// scancodeKeys maps the scancodes of the letter, digit and punctuation keys
// of an ANSI US keyboard to the key at that position. The main block's
// codes are the same in Linux's evdev and in XT set 1, which Windows
// reports.
var scancodeKeys = map[uint16]Key{
	2: Key1, 3: Key2, 4: Key3, 5: Key4, 6: Key5, 7: Key6, 8: Key7, 9: Key8, 10: Key9, 11: Key0,
	12: KeyMinus, 13: KeyEqual,
	16: KeyQ, 17: KeyW, 18: KeyE, 19: KeyR, 20: KeyT, 21: KeyY, 22: KeyU, 23: KeyI, 24: KeyO, 25: KeyP,
	26: KeyBracketLeft, 27: KeyBracketRight,
	30: KeyA, 31: KeyS, 32: KeyD, 33: KeyF, 34: KeyG, 35: KeyH, 36: KeyJ, 37: KeyK, 38: KeyL,
	39: KeySemicolon, 40: KeyQuote, 41: KeyBackquote, 43: KeyBackslash,
	44: KeyZ, 45: KeyX, 46: KeyC, 47: KeyV, 48: KeyB, 49: KeyN, 50: KeyM,
	51: KeyComma, 52: KeyPeriod, 53: KeySlash,
}

// KeyForScancode returns the key at a scancode's position on an ANSI US
// keyboard, whatever the layout types (SurfaceEvent.PhysicalKey), or
// KeyUnknown for a key it does not map. A scancode is Linux's evdev code
// (the X keycode less 8) or Windows's XT set 1 code without the extended
// flag.
func KeyForScancode(code uint16) Key { return scancodeKeys[code] }

// KeyByPosition reports whether a key press whose layout key is key may
// stand for its physical key: only when the layout types no US key, and a
// shortcut modifier is held, so that typing stays on the layout.
func KeyByPosition(key, physical Key, mods Modifiers) bool {
	return key == KeyUnknown && physical != KeyUnknown && mods&(ModCtrl|ModAlt|ModSuper) != 0
}

// Cursor is a pointer shape.
type Cursor uint8

const (
	CursorDefault Cursor = iota
	CursorPointer
	CursorText
	CursorMove
	CursorResizeEW
	CursorResizeNS
	CursorResizeNWSE
	CursorResizeNESW
	CursorNotAllowed
	CursorCrosshair
	CursorGrab
	CursorGrabbing
	CursorResizeN
	CursorResizeE
	CursorResizeS
	CursorResizeW
	CursorResizeColumn
	CursorResizeRow
	CursorVerticalText
	CursorCopy
	CursorAlias
	CursorContextMenu
	CursorNone
)
