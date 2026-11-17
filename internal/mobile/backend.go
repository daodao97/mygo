// Package mobile is the backend of phones and tablets: iOS, where the
// host is Swift and UIKit (ios/), and Android, where it is Kotlin and
// Android's views (android/). Both run Go the same way, so one backend
// serves them, and the host's language does what only it can: the views,
// the web view and its delegates, the keyboard, accessibility and the
// system's dialogs.
//
// The host owns the process's UI thread. Go runs on a thread of its own,
// which the core takes for the main thread: the backend's event loop
// (Run) handles there the events the host sends, and the host runs Go's
// calls on the UI thread (see transport), the messages of package wire.
// So Go can wait for an answer (await pumping Step) while the UI thread
// goes on, as the system needs it to; the browser backend works the same
// way. The Go code links into the host as a library, through the cgo of
// glue_ios.go and glue_android.go: the only cgo of MyGo, which phones'
// build modes need.
//
// Phones have one screen for the app: the window shown last fills it,
// above those shown before, and what desktops have that phones don't
// (menu bars, trays, the Dock, global shortcuts) does nothing, as in
// internal/unsupported.
package mobile

import (
	"bytes"
	"errors"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo/internal/mobile/wire"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/unsupported"
)

var errUnsupported = errors.New("mygo: " + runtime.GOOS + " does not support this")

// Backend is the backend of phones.
type Backend struct {
	*unsupported.Backend

	t      *transport
	h      platform.AppHandler
	mainID uint64

	// notify wakes the event loop: for events, Signal or Wake.
	notify   chan struct{}
	signaled atomic.Bool
	woken    atomic.Bool
	quit     atomic.Bool

	mu      sync.Mutex
	events  []event
	stopped bool

	// Main thread only.
	windows map[int]*window
	nextID  int
	reqs    map[int]func(wire.Reader)
	nextReq int
	schemes map[int]*schemeRequest
	tabs    map[int]*tabBar
	notes   map[string]bool
	// shown lists the windows on screen, the one in front last.
	shown []*window
	dark  bool
	// pluginEvents hears the events of plugins' native code.
	pluginEvents func(plugin, name string, data []byte)
	// ready is set once the core heard Ready, background while the app is
	// in the background.
	ready, background bool
	// startedHidden says it started out of sight, until it first shows.
	startedHidden bool

	infoMu sync.Mutex
	infoOK bool
	info   wire.AppInfoReply
}

type event struct {
	msg  []byte
	done chan []byte // nil for an event the host does not wait for
}

// theHost is the host the glue started Go with.
var theHost host

// New returns the backend of the host the app runs in.
func New() *Backend {
	b := &Backend{
		Backend: unsupported.New(),
		notify:  make(chan struct{}, 1),
		windows: map[int]*window{},
		reqs:    map[int]func(wire.Reader){},
		schemes: map[int]*schemeRequest{},
		tabs:    map[int]*tabBar{},
		notes:   map[string]bool{},
	}
	if theHost != nil {
		b.t = newTransport(theHost)
	}
	early.Lock()
	b.events = early.events
	early.events = nil
	current.Store(b)
	early.Unlock()
	return b
}

// current is the backend events go to.
var current atomic.Pointer[Backend]

// early holds the events the host sent before the app made its backend,
// which takes them: a link the app was opened with is sent as it launches,
// while its main function may yet be setting up.
var early struct {
	sync.Mutex
	events []event
}

func goroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	b := bytes.TrimPrefix(buf[:n], []byte("goroutine "))
	b = b[:max(bytes.IndexByte(b, ' '), 0)]
	id, _ := strconv.ParseUint(string(b), 10, 64)
	return id
}

func (b *Backend) Name() string { return runtime.GOOS + "/mobile" }

func (b *Backend) call(m message, reply interface{ Decode(*wire.Reader) }) {
	if b.t == nil {
		return // no host: tests of the core, or a build that never started
	}
	b.t.call(m, reply)
}

func (b *Backend) Init(h platform.AppHandler, _ platform.AppOptions) error {
	if b.t == nil {
		return errors.New("mygo: the app runs outside its host; build it with mygo build -platform " + runtime.GOOS)
	}
	b.h = h
	b.mainID = goroutineID()
	var t wire.ThemeReply
	b.call(&wire.Theme{}, &t)
	b.dark = t.Dark
	// Started for a background task: no frames until the user opens it.
	b.background = b.appInfo().Background
	b.startedHidden = b.background
	return nil
}

// appInfo is what the host said of the app and the system, asked once.
func (b *Backend) appInfo() wire.AppInfoReply {
	b.infoMu.Lock()
	defer b.infoMu.Unlock()
	if !b.infoOK {
		b.call(&wire.AppInfo{}, &b.info)
		b.infoOK = b.t != nil
	}
	return b.info
}

// forgetInfo makes appInfo ask again, as the system's settings changed.
func (b *Backend) forgetInfo() {
	b.infoMu.Lock()
	b.infoOK = false
	b.infoMu.Unlock()
}

func (b *Backend) Run() error {
	b.postReady()
	for !b.quit.Load() {
		<-b.notify
		b.handleAll()
	}
	b.stopEvents()
	return nil
}

// stopEvents answers what the host waits for, and all it will send: with the
// core's loop gone, no one would, and the UI thread would hang in it.
func (b *Backend) stopEvents() {
	b.mu.Lock()
	b.stopped = true
	events := b.events
	b.events = nil
	b.mu.Unlock()
	for _, e := range events {
		if e.done != nil {
			e.done <- nil
		}
	}
}

// postReady tells the core the app is ready once the loop runs, as the
// host started Go once it had launched.
func (b *Backend) postReady() {
	b.mu.Lock()
	// Before what the host sent while the app started.
	b.events = append([]event{{msg: readyMsg}}, b.events...)
	b.mu.Unlock()
	b.wakeLoop()
}

var readyMsg = func() []byte {
	var w wire.Writer
	w.Uvarint(wire.EventReady)
	return w.Buf
}()

// handleAll runs Dispatch if signaled, and the events that came.
func (b *Backend) handleAll() bool {
	did := false
	if b.signaled.Swap(false) {
		b.h.Dispatch()
		did = true
	}
	for {
		b.mu.Lock()
		if len(b.events) == 0 {
			b.mu.Unlock()
			break
		}
		e := b.events[0]
		b.events = b.events[1:]
		b.mu.Unlock()
		reply := b.handle(e.msg)
		if e.done != nil {
			e.done <- reply
		}
		did = true
		if b.signaled.Swap(false) {
			b.h.Dispatch()
		}
	}
	return did
}

func (b *Backend) wakeLoop() {
	select {
	case b.notify <- struct{}{}:
	default:
	}
}

func (b *Backend) Quit() {
	// Phones don't quit apps: the system ends them. The loop stops for the
	// core, which then finishes; the process stays until the system ends
	// it.
	b.quit.Store(true)
	b.wakeLoop()
}

func (b *Backend) IsMainThread() bool { return b.mainID == 0 || goroutineID() == b.mainID }

func (b *Backend) Signal() {
	b.signaled.Store(true)
	b.wakeLoop()
}

func (b *Backend) Step() {
	for {
		if b.handleAll() || b.woken.Swap(false) || b.quit.Load() {
			return
		}
		<-b.notify
	}
}

func (b *Backend) Wake() {
	b.woken.Store(true)
	b.wakeLoop()
}

// deliver hands an event of the host to the event loop, and waits for its
// answer when the host waits for one. onUI tells whether the host sends
// it from the UI thread, which then runs Go's calls while it waits.
func deliver(msg []byte, onUI bool) []byte {
	if len(msg) == 0 {
		return nil
	}
	op := (&wire.Reader{Buf: msg}).Uvarint()
	msg = bytes.Clone(msg)
	b := current.Load()
	if b == nil {
		early.Lock()
		b = current.Load()
		if b == nil {
			// What waits has no one to answer it yet.
			if !wire.EventWaits(op) {
				early.events = append(early.events, event{msg: msg})
			}
			early.Unlock()
			return nil
		}
		early.Unlock()
	}
	if !wire.EventWaits(op) {
		b.mu.Lock()
		b.events = append(b.events, event{msg: msg})
		b.mu.Unlock()
		b.wakeLoop()
		return nil
	}
	done := make(chan []byte, 1)
	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		return nil
	}
	b.events = append(b.events, event{msg: msg, done: done})
	b.mu.Unlock()
	b.wakeLoop()
	if onUI && b.t != nil {
		return b.t.wait(done)
	}
	return <-done
}

// request numbers an asynchronous answer, which an event brings with the
// number: fn decodes the rest of it. Main thread only.
func (b *Backend) request(fn func(r wire.Reader)) int {
	b.nextReq++
	b.reqs[b.nextReq] = fn
	return b.nextReq
}

// answer runs the function waiting for the answer req of an event.
func (b *Backend) answer(req int, r wire.Reader) {
	if fn := b.reqs[req]; fn != nil {
		delete(b.reqs, req)
		fn(r)
	}
}

func (b *Backend) NewWindow(opts *platform.WindowOptions, h platform.WindowHandler) (platform.Window, error) {
	return b.newWindow(opts, h)
}

// PopupMenu shows the system's menu at pos in w, and waits for the user's
// choice, handling events meanwhile, as menus do on desktops. What it
// chooses reaches the item's handlers through MenuItemClicked.
func (b *Backend) PopupMenu(m *platform.Menu, pw platform.Window, pos *platform.Point) {
	w, _ := pw.(*window)
	if w == nil {
		w = b.front()
	}
	if w == nil || m == nil {
		return
	}
	var enc wire.Writer
	encodeMenu(&enc, m, "")
	var x, y float64
	if pos != nil {
		x, y = float64(pos.X), float64(pos.Y)
	}
	picked, done := 0, false
	req := b.request(func(r wire.Reader) {
		var e wire.MenuPickedEvent
		e.Decode(&r)
		picked, done = e.Item, true
		b.Wake()
	})
	b.call(&wire.PopupMenu{ID: w.id, Req: req, X: x, Y: y, Items: enc.Buf}, nil)
	for !done && !b.quit.Load() {
		b.Step()
	}
	if picked != 0 {
		b.h.MenuItemClicked(picked)
	}
}

// encodeMenu writes the items a phone's menu shows, in order, as the hosts
// read them: ID (0 for a separator), label, enabled, checked, destructive
// role. Phones' menus have one level: a submenu's items follow, under their
// labels.
func encodeMenu(w *wire.Writer, m *platform.Menu, prefix string) {
	for _, it := range m.Items {
		if !it.Visible {
			continue
		}
		if it.Submenu != nil {
			encodeMenu(w, it.Submenu, prefix+it.Label+" › ")
			continue
		}
		if it.Type == platform.MenuItemSeparator {
			w.Int(0)
			w.String("")
			w.Bool(false)
			w.Bool(false)
			continue
		}
		w.Int(it.ID)
		w.String(prefix + it.Label)
		w.Bool(it.Enabled)
		w.Bool(it.Checked)
	}
}

func (b *Backend) Network() platform.Network             { return network{b: b} }
func (b *Backend) App() platform.AppController           { return appController{b: b} }
func (b *Backend) Dialogs() platform.Dialogs             { return dialogs{b: b} }
func (b *Backend) Clipboard() platform.Clipboard         { return clipboard{b: b} }
func (b *Backend) Shell() platform.Shell                 { return shell{b: b} }
func (b *Backend) Screen() platform.Screen               { return screen{b: b} }
func (b *Backend) Theme() platform.Theme                 { return theme{b: b} }
func (b *Backend) Power() platform.Power                 { return power{b: b} }
func (b *Backend) Secrets() platform.Secrets             { return secrets{b: b} }
func (b *Backend) NotificationsSupported() bool          { return true }
func (b *Backend) WebViewAvailable() bool                { return true }
func (b *Backend) ReplaceFile(target, repl string) error { return os.Rename(repl, target) }

func (b *Backend) ShowNotification(n *platform.Notification, done func(error)) {
	req := b.request(func(r wire.Reader) {
		var e wire.NotifyDoneEvent
		e.Decode(&r)
		done(errorOf(e.Error))
	})
	b.call(&wire.Notify{Req: req, NotificationID: n.ID, Title: n.Title, Subtitle: n.Subtitle, Body: n.Body,
		Group: n.Group, Silent: n.Silent}, nil)
}

func (b *Backend) RemoveNotification(id string) {
	b.call(&wire.RemoveNotification{NotificationID: id}, nil)
}

func (b *Backend) RemoveAllNotifications() { b.call(&wire.RemoveAllNotifications{}, nil) }

// errorOf makes a host's message of an error an error, nil for none.
func errorOf(msg string) error {
	if msg == "" {
		return nil
	}
	return errors.New(msg)
}

// rgba packs a color as the host reads it: 0xRRGGBBAA.
func rgba(c platform.Color) int {
	return int(c.R)<<24 | int(c.G)<<16 | int(c.B)<<8 | int(c.A)
}

func colorOf(v int) platform.Color {
	return platform.Color{R: uint8(v >> 24), G: uint8(v >> 16), B: uint8(v >> 8), A: uint8(v)}
}
