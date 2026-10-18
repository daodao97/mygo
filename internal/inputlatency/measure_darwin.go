package main

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type nsPoint struct{ X, Y float64 }

type nsRect struct{ X, Y, W, H float64 }

var (
	mediaTime    func() float64
	getMethod    func(cls, sel uintptr) uintptr
	setImp       func(m, imp uintptr) uintptr
	keyEvent     func(cls, sel uintptr, typ uint, loc nsPoint, flags uint, ts float64, window int, ctx, chars, unmodified uintptr, repeat bool, code uint16) uintptr
	mouseEvent   func(cls, sel uintptr, typ uint, loc nsPoint, flags uint, ts float64, window int, ctx uintptr, number, clicks int, pressure float32) uintptr
	convertPoint func(obj, sel uintptr, p nsPoint, view uintptr) nsPoint
	scrollEvent  func(source uintptr, units, count uint32, dy, dx, dz int32) uintptr
	setField     func(event uintptr, field uint32, v int64)
	setLocation  func(event uintptr, p nsPoint)
	cfRelease    func(uintptr)
)

func load() error {
	libs := map[string]uintptr{}
	for _, path := range []string{
		"/usr/lib/libobjc.A.dylib",
		"/System/Library/Frameworks/QuartzCore.framework/QuartzCore",
		"/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics",
		"/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation",
	} {
		h, err := purego.Dlopen(path, purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			return err
		}
		libs[path[strings.LastIndex(path, "/")+1:]] = h
	}
	objcLib, cg := libs["libobjc.A.dylib"], libs["CoreGraphics"]
	purego.RegisterLibFunc(&mediaTime, libs["QuartzCore"], "CACurrentMediaTime")
	purego.RegisterLibFunc(&getMethod, objcLib, "class_getInstanceMethod")
	purego.RegisterLibFunc(&setImp, objcLib, "method_setImplementation")
	purego.RegisterLibFunc(&scrollEvent, cg, "CGEventCreateScrollWheelEvent2")
	purego.RegisterLibFunc(&setField, cg, "CGEventSetIntegerValueField")
	purego.RegisterLibFunc(&setLocation, cg, "CGEventSetLocation")
	purego.RegisterLibFunc(&cfRelease, libs["CoreFoundation"], "CFRelease")
	send, err := purego.Dlsym(objcLib, "objc_msgSend")
	if err != nil {
		return err
	}
	purego.RegisterFunc(&keyEvent, send)
	purego.RegisterFunc(&mouseEvent, send)
	purego.RegisterFunc(&convertPoint, send)
	return nil
}

func sel(name string) objc.SEL { return objc.RegisterName(name) }

// An event: when it arrived, and when the main thread began and finished
// handling it.
type event struct{ arrived, handling, handled float64 }

// A frame: when it started (the tick or display that drew it), when it
// asked for a drawable and got one, and the events it shows.
type frame struct {
	start, asked, got float64
	events            []event
}

// A measured event, its frame, and when that reached the screen.
type sample struct {
	event
	frame *frame
	shown float64
}

var (
	mu        sync.Mutex
	measuring bool
	pending   []event               // handled since the last drawable
	frames    = map[uint64]*frame{} // by drawableID, until presented
	samples   []sample
	drawn     int // frames while measuring
	notShown  int // of them
	// On the main thread: when the frame being drawn started.
	frameStart float64
	presented  objc.Block

	origNext, origTick, origUpdate uintptr
)

// swizzle wraps -[CAMetalLayer nextDrawable] and the surface view's frame
// methods.
func swizzle() error {
	wrap := func(class, name string, fn any) (uintptr, error) {
		m := getMethod(uintptr(objc.GetClass(class)), uintptr(sel(name)))
		if m == 0 {
			return 0, fmt.Errorf("no -[%s %s]", class, name)
		}
		return setImp(m, purego.NewCallback(fn)), nil
	}
	var err error
	origNext, err = wrap("CAMetalLayer", "nextDrawable", func(self, cmd uintptr) uintptr {
		asked := mediaTime()
		d, _, _ := purego.SyscallN(origNext, self, cmd)
		if d == 0 {
			return 0
		}
		f := &frame{start: frameStart, asked: asked, got: mediaTime()}
		mu.Lock()
		f.events, pending = pending, nil
		frames[objc.Send[uint64](objc.ID(d), sel("drawableID"))] = f
		if measuring {
			drawn++
		}
		mu.Unlock()
		objc.ID(d).Send(sel("addPresentedHandler:"), presented)
		return d
	})
	if err != nil {
		return err
	}
	origTick, err = wrap("MyGoSurfaceView", "mygoTick:", func(self, cmd, link uintptr) {
		frameStart = mediaTime()
		purego.SyscallN(origTick, self, cmd, link)
	})
	if err != nil {
		return err
	}
	origUpdate, err = wrap("MyGoSurfaceView", "updateLayer", func(self, cmd uintptr) {
		frameStart = mediaTime()
		purego.SyscallN(origUpdate, self, cmd)
	})
	return err
}

func init() {
	presented = objc.NewBlock(func(_ objc.Block, d objc.ID) {
		shown := objc.Send[float64](d, sel("presentedTime"))
		id := objc.Send[uint64](d, sel("drawableID"))
		mu.Lock()
		defer mu.Unlock()
		f := frames[id]
		delete(frames, id)
		if f == nil || !measuring {
			return
		}
		if shown == 0 {
			// Another frame replaced it before the screen showed it,
			// showing its events too: they are not counted.
			notShown++
			return
		}
		for _, e := range f.events {
			samples = append(samples, sample{e, f, shown})
		}
	})
}

// The window's state.
var (
	text      = strings.Repeat("The quick brown fox jumps over the lazy dog. ", 40)
	list      ui.ListState
	offset    ui.Point
	mode      string
	window    *mygo.Window
	nsWindow  uintptr
	surface   uintptr
	errResult error
)

func view(c *ui.Context) {
	switch mode {
	case "type":
		ui.Column(c).Fill().Padding(16).Children(func() {
			ui.TextArea(c, &text).AutoFocus().Grow(1)
		})
	case "scroll":
		ui.List(c, &list, 1_000_000, func(i int) {
			ui.Row(c).Height(float32(28 + i%3*8)).Gap(8).Padding(4).Children(func() {
				ui.Textf(c, "Row %d", i).Grow(1)
				ui.Text(c, "detail").TextColor(c.Theme().TextMuted)
			})
		}).Grow(1)
	case "drag":
		ui.Box(c).Fill().Children(func() {
			ui.Column(c).Fill().Children(func() {
				for i := range 30 {
					ui.Textf(c, "Row %d of the content under the box", i)
				}
			})
			ui.Box(c).Absolute().Width(80).Height(80).Background(c.Theme().Accent).Draggable(&offset, ui.DraggableOptions{})
		})
	}
}

func measure(m string, events, warmup, x, y int) error {
	switch m {
	case "type", "scroll", "drag":
	default:
		return fmt.Errorf("unknown mode %q", m)
	}
	mode = m
	if err := load(); err != nil {
		return err
	}
	// Out of the user's way: the app is not activated, and the window
	// neither takes the keyboard nor comes to the front of the others.
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	mygo.App.WhenReady(func() {
		window = mygo.NewWindow(mygo.WindowOptions{Title: "inputlatency", Width: 800, Height: 600, X: x, Y: y, Hidden: true, Content: ui.View(view)})
		nsWindow = window.NativeHandle()
		// As the app the user types into, which is active: App Nap would
		// delay the inactive app's wakeups by milliseconds.
		const userInitiated, latencyCritical = 0x00EFFFFF, 0xFF00000000
		reason := objc.ID(objc.GetClass("NSString")).Send(sel("stringWithUTF8String:"), "measuring input latency\x00")
		objc.ID(objc.GetClass("NSProcessInfo")).Send(sel("processInfo")).Send(sel("beginActivityWithOptions:reason:"), uint64(userInitiated|latencyCritical), reason).Send(sel("retain"))
		objc.ID(nsWindow).Send(sel("orderFrontRegardless"))
		go func() {
			errResult = run(events, warmup)
			mygo.App.Quit()
		}()
	})
	if err := mygo.App.Run(); err != nil {
		return err
	}
	return errResult
}

func findSurface(v objc.ID) objc.ID {
	if objc.Send[bool](v, sel("isKindOfClass:"), objc.GetClass("MyGoSurfaceView")) {
		return v
	}
	subviews := v.Send(sel("subviews"))
	for i := range objc.Send[int](subviews, sel("count")) {
		if s := findSurface(subviews.Send(sel("objectAtIndex:"), i)); s != 0 {
			return s
		}
	}
	return 0
}

// inject has the main thread handle an event arriving now.
func inject(handle func()) {
	e := event{arrived: mediaTime()}
	mygo.RunOnMain(func() {
		e.handling = mediaTime()
		handle()
		e.handled = mediaTime()
		mu.Lock()
		pending = append(pending, e)
		mu.Unlock()
	})
}

func windowPoint(x, y float64) nsPoint {
	return convertPoint(surface, uintptr(sel("convertPoint:toView:")), nsPoint{x, y}, 0)
}

func sendKey() {
	number := objc.Send[int](objc.ID(nsWindow), sel("windowNumber"))
	s := objc.ID(objc.GetClass("NSString")).Send(sel("stringWithUTF8String:"), "a\x00")
	for _, typ := range []uint{10, 11} { // NSEventTypeKeyDown, KeyUp
		ev := keyEvent(uintptr(objc.GetClass("NSEvent")), uintptr(sel("keyEventWithType:location:modifierFlags:timestamp:windowNumber:context:characters:charactersIgnoringModifiers:isARepeat:keyCode:")),
			typ, nsPoint{}, 0, mediaTime(), number, 0, uintptr(s), uintptr(s), false, 0)
		objc.ID(nsWindow).Send(sel("sendEvent:"), ev)
	}
}

func sendMouse(typ uint, x, y float64) {
	number := objc.Send[int](objc.ID(nsWindow), sel("windowNumber"))
	ev := mouseEvent(uintptr(objc.GetClass("NSEvent")), uintptr(sel("mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:")),
		typ, windowPoint(x, y), 0, mediaTime(), number, 0, 0, 1, 1)
	objc.ID(nsWindow).Send(sel("sendEvent:"), ev)
}

// sendScroll scrolls as a trackpad does, by dy points, in a phase of the
// gesture (kCGScrollPhase: 1 began, 2 changed, 4 ended).
func sendScroll(phase int64, dy int32) {
	ev := scrollEvent(0, 0, 1, dy, 0, 0) // in pixels
	setField(ev, 88, 1)                  // kCGScrollWheelEventIsContinuous
	setField(ev, 99, phase)              // kCGScrollWheelEventScrollPhase
	setField(ev, 96, int64(dy))          // kCGScrollWheelEventPointDeltaAxis1
	// A window-less event's location is on the screen, flipped by Core
	// Graphics: its locationInWindow is then this point.
	p := windowPoint(300, 300)
	screen := objc.ID(objc.GetClass("NSScreen")).Send(sel("screens")).Send(sel("objectAtIndex:"), 0)
	setLocation(ev, nsPoint{p.X, objc.Send[nsRect](screen, sel("frame")).H - p.Y})
	objc.ID(surface).Send(sel("scrollWheel:"), objc.ID(objc.GetClass("NSEvent")).Send(sel("eventWithCGEvent:"), ev))
	cfRelease(ev)
}

func run(events, warmup int) error {
	// The first frames, at full speed.
	time.Sleep(1500 * time.Millisecond)
	var err error
	var refresh float64
	mygo.RunOnMain(func() {
		surface = uintptr(findSurface(objc.ID(nsWindow).Send(sel("contentView"))))
		if surface == 0 {
			err = errors.New("no MyGoSurfaceView in the window")
			return
		}
		refresh = float64(objc.Send[int](objc.ID(nsWindow).Send(sel("screen")), sel("maximumFramesPerSecond")))
		err = swizzle()
	})
	if err != nil {
		return err
	}
	between := func(lo, hi float64) time.Duration {
		return time.Duration((lo + rand.Float64()*(hi-lo)) * float64(time.Millisecond))
	}
	n := 0
	next := func() bool {
		if n == warmup {
			mu.Lock()
			measuring = true
			mu.Unlock()
		}
		n++
		return n <= warmup+events
	}
	switch mode {
	case "type":
		for next() {
			inject(sendKey)
			time.Sleep(between(60, 140))
		}
	case "scroll":
		for n < warmup+events {
			mygo.RunOnMain(func() { sendScroll(1, -2) })
			for range 60 {
				if !next() {
					break
				}
				inject(func() { sendScroll(2, -3) })
				time.Sleep(between(4, 12))
			}
			mygo.RunOnMain(func() { sendScroll(4, 0) })
			time.Sleep(300 * time.Millisecond)
		}
	case "drag":
		for n < warmup+events {
			var x, y float64
			mygo.RunOnMain(func() {
				x, y = 40+float64(offset.X), 40+float64(offset.Y)
				sendMouse(1, x, y) // NSEventTypeLeftMouseDown
			})
			time.Sleep(50 * time.Millisecond)
			for i := range 60 {
				if !next() {
					break
				}
				if i < 30 {
					x += 3
				} else {
					x -= 3
				}
				px := x
				inject(func() { sendMouse(6, px, y) }) // NSEventTypeLeftMouseDragged
				time.Sleep(between(4, 12))
			}
			mygo.RunOnMain(func() { sendMouse(2, x, y) }) // NSEventTypeLeftMouseUp
			time.Sleep(300 * time.Millisecond)
		}
	}
	// The last frames reach the screen.
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	measuring = false
	ss, d, ns := samples, drawn, notShown
	mu.Unlock()
	if len(ss) == 0 {
		return errors.New("no event reached the screen")
	}
	var total []float64
	var f figures
	for _, s := range ss {
		start := max(s.frame.start, s.handled) // drawn in the pass that handled it
		total = append(total, (s.shown-s.arrived)*1000)
		f.queue += (s.handling - s.arrived) * 1000
		f.handle += (s.handled - s.handling) * 1000
		f.start += (start - s.handled) * 1000
		f.build += (s.frame.asked - start) * 1000
		f.drawable += (s.frame.got - s.frame.asked) * 1000
		f.toScreen += (s.shown - s.frame.got) * 1000
	}
	slices.Sort(total)
	k := float64(len(ss))
	f.refresh, f.events = refresh, k
	f.queue, f.handle, f.start, f.build, f.drawable, f.toScreen = f.queue/k, f.handle/k, f.start/k, f.build/k, f.drawable/k, f.toScreen/k
	for _, t := range total {
		f.mean += t / k
	}
	f.p50, f.p90, f.p99 = percentile(total, .5), percentile(total, .9), percentile(total, .99)
	if d > 0 {
		f.notShown = 100 * float64(ns) / float64(d)
	}
	fmt.Println(f.line())
	return nil
}
