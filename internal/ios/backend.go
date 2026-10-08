//go:build ios && cgo

// Package ios hosts MyGo's Go UI in UIKit. UIKit owns the OS main thread;
// the application entry runs on a background thread and dispatches UI work.
package ios

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework UIKit -framework Foundation -framework QuartzCore -framework CoreGraphics -framework CoreText -framework Metal -framework IOSurface -framework AVFoundation -framework Photos -framework PhotosUI -framework UniformTypeIdentifiers -framework CoreLocation -framework UserNotifications -framework Security -framework LocalAuthentication
#include "native.h"
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/unsupported"
)

type Backend struct {
	*unsupported.Backend
	h      platform.AppHandler // UI thread only
	window *window
	nextID uint64
	done   chan struct{}
	quit   sync.Once
	active bool
}

var current atomic.Pointer[Backend]

func New() *Backend {
	b := &Backend{Backend: unsupported.New(), done: make(chan struct{})}
	current.Store(b)
	return b
}
func (*Backend) Name() string                { return "ios/uikit" }
func (*Backend) SystemManagedLifetime() bool { return true }
func (b *Backend) Init(h platform.AppHandler, _ platform.AppOptions) error {
	// Go's iOS fallback is "/" when the launcher did not supply HOME.
	// Resolve the sandbox with Foundation before application callbacks.
	if err := os.Setenv("HOME", ownedString(C.mygo_ios_home())); err != nil {
		return err
	}
	b.h = h
	return nil
}
func (*Backend) IsMainThread() bool { return bool(C.mygo_ios_is_main()) }
func (b *Backend) RunApplication(start func() error, finish func()) error {
	if b.IsMainThread() {
		return errors.New("mygo: iOS App.Run must run in the hosted background entry")
	}
	var err error
	uiSync(func() {
		err = start()
		if err == nil {
			b.h.Ready()
			C.mygo_ios_ready()
		}
	})
	if err != nil {
		return err
	}
	<-b.done
	uiSync(finish)
	return nil
}
func (*Backend) Run() error { return errors.New("mygo: iOS uses the UIKit application host") }
func (b *Backend) Quit()    { b.quit.Do(func() { close(b.done) }) }
func (b *Backend) Signal() {
	dispatch(func() {
		if b.h != nil {
			b.h.Dispatch()
		}
	})
}
func (*Backend) Step() { C.mygo_ios_step() }
func (*Backend) Wake() { C.mygo_ios_wake() }
func (b *Backend) NewWindow(o *platform.WindowOptions, h platform.WindowHandler) (platform.Window, error) {
	if !o.Surface {
		return nil, fmt.Errorf("mygo: iOS currently requires WindowOptions.Content: %w", platform.ErrUnsupported)
	}
	if b.window != nil {
		return nil, errors.New("mygo: iOS currently supports one content window")
	}
	b.nextID++
	w := &window{b: b, h: h, id: b.nextID, title: o.Title}
	w.view = uintptr(C.mygo_ios_create(C.uint64_t(w.id)))
	if w.view == 0 {
		return nil, errors.New("mygo: UIKit could not create the content surface")
	}
	b.window = w
	if o.BackgroundColor != nil {
		w.SetBackgroundColor(*o.BackgroundColor)
	}
	return w, nil
}

var callbacks struct {
	sync.Mutex
	next uint64
	fns  map[uint64]func()
}

func dispatch(fn func()) {
	callbacks.Lock()
	if callbacks.fns == nil {
		callbacks.fns = map[uint64]func(){}
	}
	callbacks.next++
	id := callbacks.next
	callbacks.fns[id] = fn
	callbacks.Unlock()
	C.mygo_ios_dispatch(C.uint64_t(id))
}
func uiSync(fn func()) {
	done := make(chan struct{})
	dispatch(func() { defer close(done); fn() })
	<-done
}

//export goIOSDispatch
func goIOSDispatch(token C.uint64_t) {
	callbacks.Lock()
	fn := callbacks.fns[uint64(token)]
	delete(callbacks.fns, uint64(token))
	callbacks.Unlock()
	if fn != nil {
		fn()
	}
}
func find(id uint64) *window {
	b := current.Load()
	if b != nil && b.window != nil && b.window.id == id {
		return b.window
	}
	return nil
}

//export goIOSFrame
func goIOSFrame(id C.uint64_t) {
	if w := find(uint64(id)); w != nil {
		w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceFrame})
	}
}

//export goIOSPresented
func goIOSPresented(id C.uint64_t) {
	if w := find(uint64(id)); w != nil {
		w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfacePresented})
	}
}

//export goIOSResize
func goIOSResize(id C.uint64_t) {
	if w := find(uint64(id)); w != nil {
		w.h.Resized()
		w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceResize})
	}
}

//export goIOSTouch
func goIOSTouch(id, pointer C.uint64_t, kind C.int, x, y C.double) C.bool {
	w := find(uint64(id))
	if w == nil {
		return false
	}
	kinds := []platform.SurfaceEventKind{platform.PointerDown, platform.PointerMove, platform.PointerUp, platform.PointerCancel}
	if int(kind) < 0 || int(kind) >= len(kinds) {
		return false
	}
	return C.bool(w.h.SurfaceEvent(platform.SurfaceEvent{Kind: kinds[int(kind)], X: float64(x), Y: float64(y), Clicks: 1, PointerID: uint64(pointer), PointerType: platform.PointerTouch}))
}

//export goIOSScroll
func goIOSScroll(id C.uint64_t, x, y, dx, dy C.double) {
	if w := find(uint64(id)); w != nil {
		w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.PointerScroll, PointerType: platform.PointerTouch, X: float64(x), Y: float64(y), DX: float64(dx), DY: float64(dy), Precise: true})
	}
}

//export goIOSText
func goIOSText(id C.uint64_t, kind C.int, text *C.char, from, to, caret C.int) {
	if w := find(uint64(id)); w != nil {
		k := platform.TextInput
		if kind == 1 {
			k = platform.TextComposition
		}
		if kind == 2 {
			k = platform.TextSelectionChanged
		}
		event := platform.SurfaceEvent{Kind: k, Text: C.GoString(text), Replace: kind < 3, Snapshot: kind == 0, From: int(from), To: int(to), Caret: int(caret)}
		if kind == 4 || kind == 5 {
			event.Kind = platform.KeyPressed
			event.Key = platform.KeyBackspace
			if kind == 5 {
				event.Key = platform.KeyEnter
			}
		}
		w.h.SurfaceEvent(event)
		// UITextView may send several edits before the next display tick.
		// Apply each snapshot before accepting another replacement, while
		// the delegate's sending guard prevents echoing text into UIKit.
		w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceFrame})
	}
}

//export goIOSInputSync
func goIOSInputSync(id C.uint64_t) {
	if w := find(uint64(id)); w != nil {
		w.SetTextInput(w.input)
	}
}

//export goIOSInputAction
func goIOSInputAction(id, owner C.uint64_t, action *C.char) {
	if w := find(uint64(id)); w != nil {
		w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceInputAction, ID: uint64(owner), Text: C.GoString(action)})
		w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceFrame})
	}
}

//export goIOSComposition
func goIOSComposition(id C.uint64_t, text *C.char, start, end, caret C.int) {
	if w := find(uint64(id)); w != nil {
		w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.TextComposition, Text: C.GoString(text),
			Replace: true, Snapshot: true, To: len([]rune(w.input.Text)),
			MarkedStart: int(start), MarkedEnd: int(end), Caret: int(caret)})
		w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceFrame})
	}
}

//export goIOSTextGeometry
func goIOSTextGeometry(id C.uint64_t, field C.uint64_t, kind *C.char, start, end C.int, x, y C.double) *C.char {
	var result platform.TextGeometry
	if w := find(uint64(id)); w != nil {
		if h, ok := w.h.(platform.TextGeometryHandler); ok {
			result = h.TextGeometry(platform.TextGeometryQuery{ID: uint64(field), Kind: C.GoString(kind),
				Start: int(start), End: int(end), X: float64(x), Y: float64(y)})
		}
	}
	data, _ := json.Marshal(result)
	return C.CString(string(data)) // UIKit owns and frees this result.
}

//export goIOSEditCommand
func goIOSEditCommand(id C.uint64_t, command *C.char) {
	if w := find(uint64(id)); w != nil {
		w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: C.GoString(command)})
		w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceFrame})
	}
}

//export goIOSHardwareCommand
func goIOSHardwareCommand(id C.uint64_t, name *C.char) {
	w := find(uint64(id))
	if w == nil {
		return
	}
	command := C.GoString(name)
	switch command {
	case "tab", "backtab", "enter", "space":
		var mods platform.Modifiers
		key := platform.KeyTab
		if command == "enter" {
			key = platform.KeyEnter
		}
		if command == "space" {
			key = platform.KeySpace
		}
		if command == "backtab" {
			mods = platform.ModShift
		}
		w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.KeyPressed, Key: key, Mods: mods})
		w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.KeyReleased, Key: key, Mods: mods})
	case "escape":
		if w.input.Active {
			w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceKeyboardDismiss})
		} else {
			w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.KeyPressed, Key: platform.KeyEscape})
			w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.KeyReleased, Key: platform.KeyEscape})
		}
	default:
		w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: command})
	}
	w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceFrame})
	w.SetTextInput(w.input)
}

//export goIOSLifecycle
func goIOSLifecycle(kind C.int) {
	b := current.Load()
	if b == nil || b.h == nil {
		return
	}
	switch int(kind) {
	case 0:
		b.active = false
		b.h.DidResignActive()
		if b.window != nil {
			b.window.h.Blurred()
			b.window.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceBlur})
		}
	case 1:
		b.active = true
		b.h.DidBecomeActive()
		if b.window != nil {
			b.window.h.Focused()
			b.window.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceFocus})
			b.window.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceShown})
			b.window.RequestFrame()
		}
	case 2:
		b.h.ThemeChanged()
	case 3:
		if b.window != nil {
			b.window.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceMemoryPressure})
			C.mygo_ios_memory(C.uintptr_t(b.window.view))
		}
	case 4:
		b.h.Terminating()
		b.Quit()
	case 5:
		b.active = false
		b.h.DidEnterBackground()
	case 6:
		b.h.WillEnterForeground()
	case 7:
		if b.window != nil {
			C.mygo_ios_attach(C.uintptr_t(b.window.view))
		}
		b.h.Activated(b.window != nil)
	case 8:
		if b.active {
			b.h.DidResignActive()
			if b.window != nil {
				b.window.h.Blurred()
				b.window.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceBlur})
			}
		}
		b.active = false
		b.h.DidEnterBackground()
		if b.window != nil {
			b.window.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceMemoryPressure})
		}
	}
}

//export goIOSFile
func goIOSFile(path *C.char) {
	if b := current.Load(); b != nil && b.h != nil {
		b.h.OpenFiles([]string{C.GoString(path)})
	}
}

//export goIOSURL
func goIOSURL(url *C.char) {
	if b := current.Load(); b != nil && b.h != nil {
		b.h.OpenURLs([]string{C.GoString(url)})
	}
}

//export goIOSAccess
func goIOSAccess(id, node C.uint64_t, action C.int) C.bool {
	if w := find(uint64(id)); w != nil {
		w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.AccessAction, ID: uint64(node), Action: platform.AccessActionKind(action)})
		return true
	}
	return false
}

//export goIOSAccessibilityOn
func goIOSAccessibilityOn(id C.uint64_t) {
	if w := find(uint64(id)); w != nil {
		w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	}
}

func cString(s string, fn func(*C.char)) { p := C.CString(s); defer C.free(unsafe.Pointer(p)); fn(p) }
func ownedString(p *C.char) string {
	if p == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p)
}

type clipboard struct{ platform.Clipboard }

func (b *Backend) Clipboard() platform.Clipboard { return clipboard{b.Backend.Clipboard()} }
func (clipboard) ReadText() string               { return ownedString(C.mygo_ios_clipboard()) }
func (clipboard) WriteText(s string)             { cString(s, func(p *C.char) { C.mygo_ios_set_clipboard(p) }) }
func (c clipboard) Clear()                       { c.WriteText("") }
func (c clipboard) AvailableFormats() []string {
	var formats []string
	for _, f := range c.Formats() {
		formats = append(formats, string(f))
	}
	return formats
}

type theme struct{ platform.Theme }

func (b *Backend) Theme() platform.Theme { return theme{b.Backend.Theme()} }
func (theme) IsDark() bool               { return bool(C.mygo_ios_dark()) }
func (theme) SetSource(s string) {
	mode := 0
	if s == "light" {
		mode = 1
	}
	if s == "dark" {
		mode = 2
	}
	C.mygo_ios_theme(C.int(mode))
}
func (theme) FontRendering() platform.FontRendering { return platform.FontRendering{Antialias: "gray"} }
func (theme) Preferences() platform.Preferences {
	var motion, contrast C.bool
	var scale C.double
	C.mygo_ios_preferences(&motion, &contrast, &scale)
	return platform.Preferences{ReduceMotion: bool(motion), HighContrast: bool(contrast), TextScale: float64(scale)}
}

type app struct{ platform.AppController }

func (app) SetBadge(label string) {
	n, _ := strconv.Atoi(label)
	if n < 0 {
		n = 0
	}
	mobile{}.SetBadge(n, func(error) {})
}
func (app) Badge() string {
	n := int(C.mygo_ios_badge_count())
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func (b *Backend) App() platform.AppController { return app{b.Backend.App()} }
func (app) Locale() string                     { return ownedString(C.mygo_ios_locale()) }

type shell struct{ platform.Shell }

func (b *Backend) Shell() platform.Shell { return shell{b.Backend.Shell()} }
func (shell) OpenExternal(url string) error {
	var ok bool
	cString(url, func(p *C.char) { ok = bool(C.mygo_ios_open(p)) })
	if !ok {
		return platform.ErrUnsupported
	}
	return nil
}

// IDs are strings so JSON bridges never round a 64-bit element identity.
type accessNode struct {
	ID                        string
	Role                      platform.AccessRole
	Label, Value, Description string
	Bounds                    platform.RectF
	States                    platform.AccessStates
	Actions                   platform.AccessActions
}

func (w *window) UpdateAccessibility(t *platform.AccessTree) {
	nodes := make([]accessNode, 0, len(t.Nodes))
	for _, n := range t.Nodes {
		nodes = append(nodes, accessNode{fmt.Sprint(n.ID), n.Role, n.Label, n.Value, n.Description, n.Bounds, n.States, n.Actions})
	}
	data, _ := json.Marshal(struct {
		Nodes         []accessNode
		Focus         string
		Announcements []string
	}{nodes, fmt.Sprint(t.Focus), t.Announcements})
	cString(string(data), func(p *C.char) { C.mygo_ios_access(C.uintptr_t(w.view), p) })
}

var _ platform.Backend = (*Backend)(nil)
var _ platform.ApplicationHost = (*Backend)(nil)

//export goIOSGesture
func goIOSGesture(id C.uint64_t, kind, phase *C.char, x, y, scale, rotation C.double) C.bool {
	if w := find(uint64(id)); w != nil {
		return C.bool(w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceGesture, Gesture: C.GoString(kind), Phase: C.GoString(phase), X: float64(x), Y: float64(y), Scale: float64(scale), Rotation: float64(rotation)}))
	}
	return false
}

//export goIOSDismissKeyboard
func goIOSDismissKeyboard(id C.uint64_t) {
	if w := find(uint64(id)); w != nil {
		w.h.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfaceKeyboardDismiss})
	}
}
