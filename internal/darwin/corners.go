//go:build darwin && !ios

package darwin

import (
	"runtime"

	"github.com/ebitengine/purego/objc"
)

// The corners of a titled window given a radius of its own
// (WindowOptions.CornerRadius). AppKit draws a titled window's corners
// itself, in its theme frame, at the system's radius, and widens them near
// the corners of a display that has rounded ones, as a window comes close
// to them. Cutting the content to a radius, as a frameless window is cut,
// or setting the window's radius, would keep it fixed there. So the
// window's class gives AppKit a subclass of the theme frame it would use
// (frameViewClassForStyleMask:), which answers the window's radius where
// AppKit asks for the corners (_getCachedWindowCornerRadii), passed through
// AppKit's own adjustment for the display (_adjustedCornerRadii:), and its
// default radius (_getCachedDefaultWindowCornerRadius). After the approach
// of UnevenRoundedWindow (https://github.com/mrkai77/UnevenRoundedWindow,
// MIT). A frame AppKit does not ask so, as a borderless window's, is used
// as it is.

const (
	selCornerRadii  = "_getCachedWindowCornerRadii"
	selCornerRadius = "_getCachedDefaultWindowCornerRadius"
)

// cornerFrames are the theme frame classes made for windows of a radius of
// their own, by the frame class AppKit would use, which they subclass; a
// frame that cannot be is itself. Main thread only.
var cornerFrames = map[id]id{}

// cgFloatRet is a CGFloat returned from a method of Go: purego returns no
// float from a callback, but a struct of one double, which the ABI returns
// in the same register.
type cgFloatRet struct{ V float64 }

// msgSuperFloat is objc_msgSendSuper for a method that returns a CGFloat.
var msgSuperFloat func(sup uintptr, sel objc.SEL) float64

// frameViewClassFor is the class method frameViewClassForStyleMask: of the
// window class whose AppKit class is base ("NSWindow", "NSPanel"): AppKit's
// theme frame for the style, subclassed for corners of a window's own.
func frameViewClassFor(base string) func(id, objc.SEL, uint) id {
	return func(_ id, _ objc.SEL, mask uint) id {
		return cornerFrame(send(class(base), "frameViewClassForStyleMask:", uintptr(mask)))
	}
}

// cornerFrame is the subclass of the theme frame class frame that gives a
// window its own corners, made once.
func cornerFrame(frame id) id {
	if frame == 0 {
		return frame
	}
	if c, ok := cornerFrames[frame]; ok {
		return c
	}
	cornerFrames[frame] = frame
	if !sendBool(frame, "instancesRespondToSelector:", uintptr(sel(selCornerRadii))) ||
		!sendBool(frame, "instancesRespondToSelector:", uintptr(sel(selCornerRadius))) {
		return frame
	}
	super := goString(send(frame, "description"))
	name := "MyGoCorners" + super
	sub := objc.Class(classDef(name, super, nil, nil))
	sub.AddMethod(sel(selCornerRadii), objc.NewIMP(func(self id, cmd objc.SEL) id {
		w := frameWindow(self)
		if w == nil || w.frameRadius <= 0 {
			return sendSuper(self, name, cmd)
		}
		if radii := cornerRadii(self, float64(w.frameRadius)); radii != 0 {
			return radii
		}
		return sendSuper(self, name, cmd)
	}), "@16@0:8")
	sub.AddMethod(sel(selCornerRadius), objc.NewIMP(func(self id, cmd objc.SEL) cgFloatRet {
		w := frameWindow(self)
		if w == nil || w.frameRadius <= 0 {
			var pin runtime.Pinner
			defer pin.Unpin()
			return cgFloatRet{msgSuperFloat(superOf(self, name, &pin), cmd)}
		}
		return cgFloatRet{float64(w.frameRadius)}
	}), "d16@0:8")
	cornerFrames[frame] = id(sub)
	return id(sub)
}

// frameWindow is the window whose theme frame is frame, when it is one of
// the backend's.
func frameWindow(frame id) *window {
	if theBackend == nil {
		return nil
	}
	return theBackend.byNSWindow[send(frame, "window")]
}

// cornerRadii are the corners of radius r for the theme frame frame, as
// AppKit has them (NSViewCornerRadii): square on an edge the frame does not
// round, as in full screen, then adjusted by AppKit for the display's
// corners. 0 where the system has no such class.
func cornerRadii(frame id, r float64) id {
	cls := id(objc.GetClass("NSViewCornerRadii"))
	if cls == 0 {
		return 0
	}
	top, bottom := r, r
	if respondsTo(frame, "topCornerRounded") && !sendBool(frame, "topCornerRounded") {
		top = 0
	}
	if respondsTo(frame, "bottomCornerRounded") && !sendBool(frame, "bottomCornerRounded") {
		bottom = 0
	}
	radii := send(send(send(cls, "alloc"), "init"), "autorelease")
	for _, c := range []struct {
		key string
		v   float64
	}{{"topLeft", top}, {"topRight", top}, {"bottomLeft", bottom}, {"bottomRight", bottom}} {
		send(radii, "setValue:forKey:", uintptr(msgFloatID(class("NSNumber"), sel("numberWithDouble:"), c.v)), uintptr(nsString(c.key)))
	}
	if respondsTo(frame, "_adjustedCornerRadii:") {
		if adjusted := send(frame, "_adjustedCornerRadii:", uintptr(radii)); adjusted != 0 {
			return adjusted
		}
	}
	return radii
}

// invalidateCorners has AppKit ask the theme frame for the window's corners
// again.
func (w *window) invalidateCorners() {
	if respondsTo(w.win, "_cornerMaskChanged") {
		send(w.win, "_cornerMaskChanged")
	}
}
