package mobile

import (
	"math"
	"runtime"
	"strconv"

	"github.com/egoist/mygo/internal/mobile/wire"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/transfer"
)

// surface is the drawing area of a window of native UI: a CAMetalLayer the
// Metal renderer draws into on iOS, an ANativeWindow an EGL context draws
// into on Android, which the host made (CreateWindow, SurfaceChanged).
// Frames come at the display's refreshes the content asks for
// (RequestFrame, Frame). A finger is the pointer while it touches the
// screen, and there is no pointer once it lifts (SurfaceEvent.Touch).
type surface struct {
	w      *window
	handle uintptr
	// refresh is how many times a second the display refreshes.
	refresh float64

	pending bool // a frame asked of the host
	gone    bool
	// deferred is a frame the app in the background did not draw.
	deferred bool
	cursor   platform.Cursor
	// mouseButton is the button of a mouse that went down.
	mouseButton int

	// The fingers on the screen, by the host's numbers: the first is the
	// pointer, two pinch.
	primary   int
	touching  bool
	fingers   map[int][2]float64
	pinchDist float64
	// pinchAngle is the line between two fingers, once pinchTurning.
	pinchAngle   float64
	pinchTurning bool
	// lastTap is where and when the last tap ended, for double taps.
	lastTapX, lastTapY, lastTapTime float64
	// two is the fork's two-finger gestures (twoStart, twoMove, twoEnd).
	two  twoFingers
	taps int

	text     platform.TextInputState
	textSent wire.SetTextInput
	// textSerial counts the inputs that took the keyboard.
	textSerial int
	marked     string
	access     *platform.AccessTree
	accessOn   bool
}

func newSurface(w *window) *surface {
	return &surface{w: w, refresh: 60, fingers: map[int][2]float64{}}
}

func (s *surface) send(ev platform.SurfaceEvent) bool {
	if s.gone {
		return false
	}
	return s.w.h.SurfaceEvent(ev)
}

// resized tells the content the window's new size, or that its insets
// changed, which moves what it lays out inside them.
func (s *surface) resized(size, insets bool) {
	if size {
		s.send(platform.SurfaceEvent{Kind: platform.SurfaceResize})
	}
	if size || insets {
		s.RequestFrame()
	}
}

func (s *surface) closed() {
	s.gone = true
}

func (s *surface) themeChanged() { s.RequestFrame() }

// lowMemory has the content let go of what it can make again.
func (s *surface) lowMemory() { s.send(platform.SurfaceEvent{Kind: platform.SurfaceLowMemory}) }

// back hands Android's back to the content: Escape, which closes a menu
// or a dialog, then the Back key, as a mouse's back button, which a
// Router takes to go back. It reports whether the content took either.
func (s *surface) back() bool {
	for _, k := range []platform.Key{platform.KeyEscape, platform.KeyBack} {
		taken := s.send(platform.SurfaceEvent{Kind: platform.KeyPressed, Key: k})
		s.send(platform.SurfaceEvent{Kind: platform.KeyReleased, Key: k})
		if taken {
			return true
		}
	}
	return false
}

// Native is the CAMetalLayer of iOS, the ANativeWindow of Android.
func (s *surface) Native() platform.SurfaceNative {
	if mapsSchemes { // Android
		return platform.SurfaceNative{Window: s.handle}
	}
	return platform.SurfaceNative{Layer: s.handle}
}

func (s *surface) Size() (float64, float64, float64) { return s.w.width, s.w.height, s.w.scale }

// Insets are those of the system's bars, the cutout and the keyboard: the
// content lays out above the keyboard while it shows.
func (s *surface) Insets() platform.Insets {
	in := s.w.insets
	in.Bottom = max(in.Bottom, s.w.keyboard)
	return in
}

func (s *surface) RequestFrame() {
	if s.pending || s.gone {
		return
	}
	s.pending = true
	s.w.b.call(&wire.RequestFrame{ID: s.w.id}, nil)
}

func (s *surface) RefreshRate() float64 { return s.refresh }

// frame draws the frame the content asked for, at a refresh. Frames
// following each other come at the display's refreshes (Paced), as macOS's
// display link's do: one the GPU renderer cannot draw yet, as the drawable
// it fetches for the last is not free until the display lets one go, waits
// for it off the main thread and draws once it is (FrameNow), rather than
// block Go's main thread most of a refresh, while the fingers that move
// meanwhile wait for it.
func (s *surface) frame() {
	s.pending = false
	s.draw(true)
}

// FrameNow draws a frame at once, for one a refresh left to draw until the
// renderer had a drawable (platform.PacedSurface).
func (s *surface) FrameNow() { s.draw(false) }

var _ platform.PacedSurface = (*surface)(nil)

// draw sends the content a frame, paced or not, unless there is nothing
// to draw on.
func (s *surface) draw(paced bool) {
	if s.gone || s.handle == 0 || s.w.width == 0 {
		return
	}
	// The GPU is not ours in the background, and iOS ends apps that use it
	// there. The content draws again as the app comes back (surfaces asks).
	if s.w.b.background {
		s.deferred = true
		return
	}
	s.send(platform.SurfaceEvent{Kind: platform.SurfaceFrame, Paced: paced})
	if s.text.Active {
		s.syncText()
	}
}

// foreground draws the frame the background held back.
func (s *surface) foreground() {
	if s.deferred {
		s.deferred = false
		s.RequestFrame()
	}
}

// SetFrameRate has the host's display link tick fps times a second while
// frames follow each other, or at the display's rate for 0
// (platform.FrameRateSurface): on a ProMotion display, the display then
// refreshes as often, and a drawing asking for fewer frames
// (Painter.AnimationFrameRate) wakes the app no more often than it draws.
// Android's Choreographer ticks at the display's rate whatever it is told.
func (s *surface) SetFrameRate(fps float64) {
	if s.gone {
		return
	}
	s.w.b.call(&wire.SetFrameRate{ID: s.w.id, FPS: fps}, nil)
}

var _ platform.FrameRateSurface = (*surface)(nil)

// PresentPixels has nothing to show frames drawn in memory on: phones
// draw with their GPUs.
func (s *surface) PresentPixels([]byte, int, int, int) {}

// SetCursor shows a mouse's or a trackpad's pointer, on an iPad or a Chromebook.
func (s *surface) SetCursor(c platform.Cursor) {
	if c == s.cursor {
		return
	}
	s.cursor = c
	name := "default"
	switch c {
	case platform.CursorPointer:
		name = "pointer"
	case platform.CursorText, platform.CursorVerticalText:
		name = "text"
	case platform.CursorMove:
		name = "move"
	case platform.CursorResizeEW, platform.CursorResizeE, platform.CursorResizeW, platform.CursorResizeColumn:
		name = "resizeX"
	case platform.CursorResizeNS, platform.CursorResizeN, platform.CursorResizeS, platform.CursorResizeRow:
		name = "resizeY"
	case platform.CursorGrab:
		name = "grab"
	case platform.CursorGrabbing:
		name = "grabbing"
	case platform.CursorCrosshair:
		name = "crosshair"
	case platform.CursorNotAllowed:
		name = "notAllowed"
	case platform.CursorNone:
		name = "none"
	}
	s.w.b.call(&wire.SetCursor{ID: s.w.id, Cursor: name}, nil)
}

// SetBackground tells the host the color the content's frames fill the
// window with, which the system's bars around it take on Android (its
// toolbar, its bottom navigation), for the screen to be of one color.
func (s *surface) SetBackground(c platform.Color) {
	if c == s.w.bg {
		return
	}
	s.w.bg = c
	s.w.b.call(&wire.SetBackground{ID: s.w.id, Color: rgba(c)}, nil)
}

func (s *surface) UpdateAccessibility(tree *platform.AccessTree) {
	old := s.access
	s.access = tree
	if !s.accessOn {
		return
	}
	changed := old == nil || len(old.Nodes) != len(tree.Nodes) || old.Focus != tree.Focus
	if !changed {
		for i := range tree.Nodes {
			a, b := &old.Nodes[i], &tree.Nodes[i]
			if a.ID != b.ID || a.Label != b.Label || a.Value != b.Value || a.Bounds != b.Bounds || a.States != b.States ||
				a.Now != b.Now || a.Min != b.Min || a.Max != b.Max || a.Actions != b.Actions || a.Description != b.Description {
				changed = true
				break
			}
		}
	}
	if changed {
		s.w.b.call(&wire.AccessibilityChanged{ID: s.w.id, Focus: int(tree.Focus)}, nil)
	}
	for _, a := range tree.Announcements {
		s.w.b.call(&wire.Announce{Text: a}, nil)
	}
}

func (s *surface) Haptic(kind string) { s.w.b.Haptic(kind) }

func (s *surface) ShowEditMenu(x, y float64, actions []string) {
	s.w.b.call(&wire.ShowMenu{ID: s.w.id, X: x, Y: y, Actions: actions}, nil)
}

func (s *surface) SetBackAvailable(on bool) {
	s.w.b.call(&wire.SetBackAvailable{ID: s.w.id, On: on}, nil)
}

// CanStack: a window of its own scene, or one a surface holds, is in no stack.
func (s *surface) CanStack() bool { return !s.w.opts.OwnScene && s.w.parent == nil }

// StackMoves: UIKit's navigation controller moves its screens; Android's apps
// move their pages themselves.
func (s *surface) StackMoves() bool { return runtime.GOOS == "ios" }

func (s *surface) SetStack(screens []platform.StackScreen) {
	c := wire.SetStack{ID: s.w.id, Titles: make([]string, len(screens)), Keys: make([]string, len(screens))}
	for i, sc := range screens {
		c.Titles[i], c.Keys[i] = sc.Title, strconv.FormatUint(sc.Key, 10)
	}
	s.w.b.call(&c, &wire.SetStackReply{})
}

func (s *surface) StackShown() { s.w.b.call(&wire.StackShown{ID: s.w.id}, nil) }

func (s *surface) ShowMagnifier(x, y float64) {
	s.w.b.call(&wire.ShowMagnifier{ID: s.w.id, X: x, Y: y}, nil)
}

func (s *surface) HideMagnifier() { s.w.b.call(&wire.HideMagnifier{ID: s.w.id}, nil) }

func (s *surface) StartDataDrag(platform.DragRequest) {}
func (s *surface) CancelDataDrag()                    {}
func (s *surface) SetDropFormats([]transfer.Format)   {}

// NewWebView makes a web view the host holds under the surface, hidden until
// PlaceWebViews shows it.
func (s *surface) NewWebView(o *platform.WindowOptions, h platform.WindowHandler) (platform.WebView, error) {
	return s.w.b.makeWindow(o, h, s.w, "", "")
}

// NewPluginView makes a view of a plugin's native code, under the surface as a
// web view is (platform.PluginViewSurface).
func (s *surface) NewPluginView(viewType string, args []byte, h platform.WindowHandler) (platform.WebView, error) {
	o := &platform.WindowOptions{}
	return s.w.b.makeWindow(o, h, s.w, viewType, string(args))
}

// EmbedView shows a view of a native module's under the surface, as a
// plugin's view is (platform.ViewSurface): the host finds it by its
// handle, among the objects of the modules' Swift or Kotlin. It has no
// page to tell of: its handler is its host's, which hears nothing from it.
func (s *surface) EmbedView(view uintptr) (platform.WebView, error) {
	return s.w.b.makeWindow(&platform.WindowOptions{}, s.w.h, s.w, nativeViewType, strconv.FormatUint(uint64(view), 10))
}

// nativeViewType is the type of plugin view of a native module's view.
const nativeViewType = "mygo.native"

// PlaceWebViews shows the views the surface holds where the content shows
// them; the frames are transparent there, and the host routes fingers to them
// but where the content painted over them.
func (s *surface) PlaceWebViews(views []platform.WebViewPlacement) {
	var w wire.Writer
	for _, p := range views {
		v, ok := p.WebView.(*window)
		if !ok || v.closed {
			continue
		}
		w.Int(v.id)
		for _, r := range []platform.RectF{p.Frame, p.Clip} {
			w.Float(r.X)
			w.Float(r.Y)
			w.Float(r.W)
			w.Float(r.H)
		}
		w.Int(len(p.Covers))
		for _, r := range p.Covers {
			w.Float(r.X)
			w.Float(r.Y)
			w.Float(r.W)
			w.Float(r.H)
		}
	}
	s.w.b.call(&wire.PlaceViews{ID: s.w.id, Places: w.Buf}, nil)
}

// FirstFrame tells the host that there's something to show: Android holds its
// splash until then, rather than showing a surface with nothing on it.
func (s *surface) FirstFrame() { s.w.b.call(&wire.FirstFrame{ID: s.w.id}, nil) }

// touch turns a finger, a pen or a mouse into pointer events: the first
// finger is the pointer, a second pinches, and a tap soon after another
// near it counts as a double tap.
func (s *surface) touch(e *wire.TouchEvent) {
	if e.Kind != "touch" {
		s.mouse(e)
		return
	}
	switch e.Phase {
	case "down":
		s.fingers[e.Pointer] = [2]float64{e.X, e.Y}
		if len(s.fingers) == 4 && s.w.devTools {
			// Four fingers open the inspector, as F12 does: a phone has no such
			// key, and the system and games keep fewer fingers to themselves.
			s.send(platform.SurfaceEvent{Kind: platform.KeyPressed, Key: platform.KeyF12})
			s.send(platform.SurfaceEvent{Kind: platform.KeyReleased, Key: platform.KeyF12})
			return
		}
		if len(s.fingers) == 2 {
			s.pinchDist = s.spread()
			if s.touching {
				// Pinching, not pressing: the press ends without a click.
				s.cancel()
			}
			s.twoStart(e.Time)
			return
		}
		if len(s.fingers) > 1 {
			return
		}
		s.primary, s.touching = e.Pointer, true
		if e.Time-s.lastTapTime < 0.3 && math.Hypot(e.X-s.lastTapX, e.Y-s.lastTapY) < 24 {
			s.taps++
		} else {
			s.taps = 1
		}
		s.send(platform.SurfaceEvent{Kind: platform.PointerMove, X: e.X, Y: e.Y, Touch: true})
		s.send(platform.SurfaceEvent{Kind: platform.PointerDown, X: e.X, Y: e.Y, Clicks: s.taps, Touch: true})
	case "move":
		if _, ok := s.fingers[e.Pointer]; !ok {
			return
		}
		s.fingers[e.Pointer] = [2]float64{e.X, e.Y}
		if len(s.fingers) == 2 && s.two.on {
			s.twoMove()
			return
		}
		if len(s.fingers) == 2 {
			d := s.spread()
			if s.pinchDist > 0 && d > 0 {
				cx, cy := s.center()
				s.send(platform.SurfaceEvent{Kind: platform.PointerMove, X: cx, Y: cy, Touch: true})
				s.send(platform.SurfaceEvent{Kind: platform.PointerZoom, X: cx, Y: cy, Scale: d / s.pinchDist, Touch: true})
			}
			s.pinchDist = d
			return
		}
		if s.touching && e.Pointer == s.primary {
			s.send(platform.SurfaceEvent{Kind: platform.PointerMove, X: e.X, Y: e.Y, Touch: true})
		}
	case "up", "cancel":
		if _, ok := s.fingers[e.Pointer]; ok && len(s.fingers) == 2 && s.two.on {
			s.twoEnd(e.Phase == "cancel", e.Time)
		}
		delete(s.fingers, e.Pointer)
		if len(s.fingers) < 2 {
			s.pinchDist = 0
		} else if len(s.fingers) == 2 {
			s.pinchDist = s.spread() // from where the two left are now
		}
		if !s.touching || e.Pointer != s.primary {
			return
		}
		s.touching = false
		if e.Phase == "cancel" {
			s.cancel()
			return
		}
		s.send(platform.SurfaceEvent{Kind: platform.PointerUp, X: e.X, Y: e.Y, Clicks: s.taps, Touch: true})
		s.send(platform.SurfaceEvent{Kind: platform.PointerLeave, Touch: true})
		s.lastTapX, s.lastTapY, s.lastTapTime = e.X, e.Y, e.Time
	}
}

// cancel ends the finger's press without a click: it lets go far outside
// what it pressed.
func (s *surface) cancel() {
	s.touching = false
	s.send(platform.SurfaceEvent{Kind: platform.PointerUp, X: -1e6, Y: -1e6, Touch: true, Canceled: true})
	s.send(platform.SurfaceEvent{Kind: platform.PointerLeave, Touch: true})
	s.taps = 0
}

// spread is how far apart two fingers are. Map order would swap them, so
// they go by their numbers.
func (s *surface) spread() float64 {
	a, b, ok := s.twoFingers()
	if !ok {
		return 0
	}
	return math.Hypot(a[0]-b[0], a[1]-b[1])
}

func (s *surface) twoFingers() (a, b [2]float64, ok bool) {
	lo, hi := -1, -1
	for id := range s.fingers {
		switch {
		case lo < 0 || id < lo:
			lo, hi = id, lo
		case hi < 0 || id < hi:
			hi = id
		}
	}
	if lo < 0 || hi < 0 {
		return a, b, false
	}
	return s.fingers[lo], s.fingers[hi], true
}

// turned is how far two fingers turned since it was last asked, in radians
// clockwise, the way the shorter way round.
func (s *surface) turned() float64 {
	a, b, ok := s.twoFingers()
	if !ok {
		return 0
	}
	angle := math.Atan2(b[1]-a[1], b[0]-a[0])
	d := angle - s.pinchAngle
	first := !s.pinchTurning
	s.pinchAngle, s.pinchTurning = angle, true
	if first {
		return 0
	}
	for d > math.Pi {
		d -= 2 * math.Pi
	}
	for d < -math.Pi {
		d += 2 * math.Pi
	}
	return d
}

func (s *surface) center() (float64, float64) {
	var x, y float64
	for _, f := range s.fingers {
		x += f[0]
		y += f[1]
	}
	n := float64(len(s.fingers))
	return x / n, y / n
}

// mouse handles a mouse's or a trackpad's pointer, as on an iPad with one,
// which hovers.
func (s *surface) mouse(e *wire.TouchEvent) {
	button := 0
	if e.Buttons&2 != 0 {
		button = 1
	}
	switch e.Phase {
	case "down":
		s.mouseButton = button
		s.send(platform.SurfaceEvent{Kind: platform.PointerDown, X: e.X, Y: e.Y, Button: button, Clicks: 1})
	case "up":
		// The one that went down: a release tells of none held any more.
		s.send(platform.SurfaceEvent{Kind: platform.PointerUp, X: e.X, Y: e.Y, Button: s.mouseButton, Clicks: 1})
	case "cancel":
		s.send(platform.SurfaceEvent{Kind: platform.PointerLeave})
	default:
		s.send(platform.SurfaceEvent{Kind: platform.PointerMove, X: e.X, Y: e.Y})
	}
}

// keys maps the names the hosts give a hardware keyboard's keys.
var keys = map[string]platform.Key{
	"Enter": platform.KeyEnter, "Escape": platform.KeyEscape, "Backspace": platform.KeyBackspace, "Tab": platform.KeyTab,
	"Space": platform.KeySpace, "Delete": platform.KeyDelete, "Home": platform.KeyHome, "End": platform.KeyEnd,
	"PageUp": platform.KeyPageUp, "PageDown": platform.KeyPageDown, "ArrowLeft": platform.KeyLeft,
	"ArrowRight": platform.KeyRight, "ArrowUp": platform.KeyUp, "ArrowDown": platform.KeyDown,
}

func keyOf(name string) platform.Key {
	if k, ok := keys[name]; ok {
		return k
	}
	if len(name) == 1 {
		c := name[0] | 0x20
		switch {
		case c >= 'a' && c <= 'z':
			return platform.KeyA + platform.Key(c-'a')
		case name[0] >= '0' && name[0] <= '9':
			return platform.Key0 + platform.Key(name[0]-'0')
		}
	}
	return platform.KeyUnknown
}

// accessTree encodes the content's accessible elements for the host,
// asking the content for them first if it was not giving them yet.
func (s *surface) accessTree() []byte {
	if !s.accessOn {
		s.accessOn = true
		// The content gives its tree at once (UpdateAccessibility).
		s.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	}
	var w wire.Writer
	if s.access == nil {
		return w.Buf
	}
	for i := range s.access.Nodes {
		n := &s.access.Nodes[i]
		w.Int(int(n.ID))
		parent := 0
		if n.Parent >= 0 && n.Parent < len(s.access.Nodes) {
			parent = int(s.access.Nodes[n.Parent].ID)
		}
		w.Int(parent)
		w.Int(int(n.Role))
		w.String(n.Label)
		value := n.Value
		if n.Role.Ranged() && value == "" && n.Max > n.Min {
			value = strconv.Itoa(int(math.Round(100*(n.Now-n.Min)/(n.Max-n.Min)))) + "%"
		}
		w.String(value)
		w.String(n.Description)
		w.Float(n.Bounds.X)
		w.Float(n.Bounds.Y)
		w.Float(n.Bounds.W)
		w.Float(n.Bounds.H)
		w.Int(int(n.States))
		w.Int(int(n.Actions))
		w.Int(n.Level)
	}
	return w.Buf
}

// accessAction performs an assistive technology's action on an element.
func (s *surface) accessAction(node int, action string) bool {
	kinds := map[string]platform.AccessActionKind{
		"press": platform.AccessPress, "focus": platform.AccessFocus, "increment": platform.AccessIncrement,
		"decrement": platform.AccessDecrement, "scrollIntoView": platform.AccessScrollIntoView,
		"expand": platform.AccessExpand, "collapse": platform.AccessCollapse,
	}
	k, ok := kinds[action]
	if !ok {
		return false
	}
	s.send(platform.SurfaceEvent{Kind: platform.AccessAction, ID: uint64(node), Action: k})
	return true
}

// handleSurface handles the events of surfaces, and reports whether op
// was one.
func (b *Backend) handleSurface(op uint64, r *wire.Reader, w *wire.Writer) bool {
	sf := func(id int) *surface {
		if win := b.windows[id]; win != nil {
			return win.s
		}
		return nil
	}
	switch op {
	case wire.EventFrame:
		var e wire.FrameEvent
		e.Decode(r)
		if s := sf(e.ID); s != nil {
			s.frame()
		}
	case wire.EventSurfaceChanged:
		var e wire.SurfaceChangedEvent
		e.Decode(r)
		if s := sf(e.ID); s != nil {
			old := s.handle
			s.handle = uintptr(e.Handle)
			if e.Refresh > 0 {
				s.refresh = e.Refresh
			}
			if old != 0 && s.handle != old {
				s.send(platform.SurfaceEvent{Kind: platform.SurfaceRenew})
			}
			if s.handle != 0 {
				s.RequestFrame()
			}
		}
	case wire.EventSurfaceDestroyed:
		var e wire.SurfaceDestroyedEvent
		e.Decode(r)
		if s := sf(e.ID); s != nil && s.handle != 0 {
			s.handle = 0
			// The content lets go of its renderer at once.
			s.send(platform.SurfaceEvent{Kind: platform.SurfaceRenew})
		}
	case wire.EventTouch:
		var e wire.TouchEvent
		e.Decode(r)
		if s := sf(e.ID); s != nil {
			s.touch(&e)
		}
	case wire.EventScroll:
		var e wire.ScrollEvent
		e.Decode(r)
		if s := sf(e.ID); s != nil {
			s.send(platform.SurfaceEvent{Kind: platform.PointerScroll, X: e.X, Y: e.Y, DX: e.DX, DY: e.DY,
				Precise: e.Precise, Momentum: e.Phase == "momentum"})
		}
	case wire.EventKey:
		var e wire.KeyEvent
		e.Decode(r)
		reply := wire.KeyReply{}
		if s := sf(e.ID); s != nil {
			kind := platform.KeyPressed
			if !e.Down {
				kind = platform.KeyReleased
			}
			k := keyOf(e.Key)
			reply.Handled = s.send(platform.SurfaceEvent{Kind: kind, Key: k, PhysicalKey: k, Mods: platform.Modifiers(e.Mods), Repeat: e.Repeated})
		}
		reply.Encode(w)
	case wire.EventBackGesture:
		var e wire.BackGestureEvent
		e.Decode(r)
		if s := sf(e.ID); s != nil {
			s.send(platform.SurfaceEvent{Kind: platform.SurfaceBackGesture, Text: e.Phase, Scale: e.Progress})
		}
	case wire.EventStackPopped:
		var e wire.StackPoppedEvent
		e.Decode(r)
		if s := sf(e.ID); s != nil {
			s.send(platform.SurfaceEvent{Kind: platform.SurfaceStackPopped, To: e.Depth})
		}
	case wire.EventStatusBarTapped:
		if w := b.front(); w != nil && w.s != nil {
			w.s.send(platform.SurfaceEvent{Kind: platform.SurfaceScrollToTop})
		}
	case wire.EventViewPressed:
		var e wire.ViewPressedEvent
		e.Decode(r)
		if s := sf(e.ID); s != nil {
			s.send(platform.SurfaceEvent{Kind: platform.WebViewPress, X: e.X, Y: e.Y})
		}
	case wire.EventTextReplace, wire.EventTextMark, wire.EventTextUnmark, wire.EventTextDelete, wire.EventTextSelect,
		wire.EventTextCommand, wire.EventTextState, wire.EventTextRange, wire.EventTextBounds, wire.EventTextIndexAt, wire.EventTextInputEnded:
		b.textEvent(op, r, w)
	case wire.EventAccessibilityTree:
		var e wire.AccessibilityTreeEvent
		e.Decode(r)
		reply := wire.AccessibilityTreeReply{}
		if s := sf(e.ID); s != nil {
			reply.Nodes = s.accessTree()
		}
		reply.Encode(w)
	case wire.EventAccessibilityAction:
		var e wire.AccessibilityActionEvent
		e.Decode(r)
		reply := wire.AccessibilityActionReply{}
		if s := sf(e.ID); s != nil {
			reply.Handled = s.accessAction(e.Node, e.Action)
		}
		reply.Encode(w)
	default:
		return false
	}
	return true
}

// twoFingers is the fork's two-finger gestures (FORK.md, ui.Element.Gestures):
// where an element under the fingers takes them, they come as pinch, rotate,
// pan and tap, from where they started, rather than as PointerZoom.
type twoFingers struct {
	on           bool
	at           float64 // when the second finger landed
	x, y, spread float64 // their center and spread then
	turn         float64 // how far they turned since
	moved        bool
}

func (s *surface) gesture(kind, phase string, x, y, scale, rotation, dx, dy float64) bool {
	return s.send(platform.SurfaceEvent{Kind: platform.SurfaceGesture, Gesture: kind, Phase: phase, X: x, Y: y, Scale: scale, Rotation: rotation, DX: dx, DY: dy, Touch: true})
}

// twoStart asks whether what is under two fingers that landed takes them.
func (s *surface) twoStart(at float64) {
	cx, cy := s.center()
	s.two = twoFingers{at: at, x: cx, y: cy, spread: s.spread()}
	s.pinchTurning = false
	if !s.gesture("pinch", "probe", cx, cy, 1, 0, 0, 0) {
		return
	}
	s.two.on = true
	for _, kind := range []string{"pinch", "rotate", "pan"} {
		s.gesture(kind, "begin", cx, cy, 1, 0, 0, 0)
	}
}

func (s *surface) twoMove() {
	cx, cy := s.center()
	d := s.spread()
	if math.Hypot(cx-s.two.x, cy-s.two.y) > 10 || math.Abs(d-s.two.spread) > 10 {
		s.two.moved = true
	}
	s.two.turn += s.turned()
	scale := 1.0
	if s.two.spread > 0 && d > 0 {
		scale = d / s.two.spread
	}
	s.gesture("pinch", "change", cx, cy, scale, 0, 0, 0)
	s.gesture("rotate", "change", cx, cy, 1, s.two.turn, 0, 0)
	s.gesture("pan", "change", cx, cy, 1, 0, cx-s.two.x, cy-s.two.y)
	s.pinchDist = d
}

// twoEnd ends the gestures as one of the two fingers lifts: two that landed
// and lifted together, still, tap.
func (s *surface) twoEnd(canceled bool, at float64) {
	cx, cy := s.center()
	phase := "end"
	if canceled {
		phase = "cancel"
	}
	for _, kind := range []string{"pinch", "rotate", "pan"} {
		s.gesture(kind, phase, cx, cy, 1, 0, 0, 0)
	}
	if !canceled && !s.two.moved && at-s.two.at < 0.3 && s.gesture("tap", "begin", cx, cy, 1, 0, 0, 0) {
		s.gesture("tap", "end", cx, cy, 1, 0, 0, 0)
	}
	s.two = twoFingers{}
}
