package ui

import (
	"github.com/egoist/mygo/internal/platform"
	"testing"
	"time"
)

func touchMessageFixture(shared bool) *Tester {
	tt := snapshotTester(func(c *Context) {
		content := func() {
			Text(c, "Hello").Label("message").Width(280).Height(50)
			Text(c, "第二段👋").Label("second").Width(280).Height(50)
		}
		if shared {
			Column(c).Selectable().Children(content)
		} else {
			Text(c, "Hello").Selectable().Label("message").Width(280).Height(50)
		}
	}, 320, 180)
	tt.rt.host = &selectionTestHost{tt.h}
	return tt
}

func TestTouchReadOnlyTextRequiresLongPress(t *testing.T) {
	for _, shared := range []bool{false, true} {
		tt := touchMessageFixture(shared)
		for id := uint64(1); id <= 2; id++ {
			touch(tt, platform.PointerDown, id, 30, 15)
			tt.Advance(100 * time.Millisecond)
			touch(tt, platform.PointerUp, id, 30, 15)
			if tt.h.ime.Active || tt.rt.focused != 0 {
				t.Fatal("short or repeated tap activated message selection")
			}
		}
		touch(tt, platform.PointerDown, 3, 30, 15)
		tt.Advance(499 * time.Millisecond)
		if tt.h.ime.Active {
			t.Fatal("read-only text focused before the hold threshold")
		}
		tt.Advance(time.Millisecond)
		if !tt.h.ime.Active || !tt.h.ime.ReadOnly || tt.h.ime.Start == tt.h.ime.End {
			t.Fatalf("shared=%v: long press did not select a word: %+v", shared, tt.h.ime)
		}
		// The native recognizer and scheduled frame can arrive in either order.
		tt.send(platform.SurfaceEvent{Kind: platform.PointerLongPress, PointerType: platform.PointerTouch, PointerID: 3, X: 30, Y: 15})
		touch(tt, platform.PointerUp, 3, 30, 15)
		tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "copy"})
		if tt.Clipboard() != "Hello" {
			t.Fatalf("long press copied %q", tt.Clipboard())
		}
		if shared {
			tt.send(platform.SurfaceEvent{Kind: platform.TextSelectionChanged, From: 0, To: 10})
			tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "copy"})
			if tt.Clipboard() != "Hello\n第二段👋" {
				t.Fatalf("native handles could not extend across paragraphs: %q", tt.Clipboard())
			}
		}
		touch(tt, platform.PointerDown, 4, 30, 15)
		touch(tt, platform.PointerUp, 4, 30, 15)
		if tt.h.ime.Active {
			t.Fatal("ordinary tap did not dismiss the existing selection")
		}
		if _, _, selected := tt.rt.textSelectionBounds(); selected {
			t.Fatal("ordinary tap retained highlight")
		}
	}
}

func TestTouchTextScrollingAndCancellationDoNotSelect(t *testing.T) {
	for _, kind := range []platform.SurfaceEventKind{platform.PointerMove, platform.PointerCancel, platform.SurfaceBlur, platform.SurfaceResize} {
		var scroll ScrollState
		tt := snapshotTester(func(c *Context) {
			Scroll(c).Fill().TrackScroll(&scroll).Children(func() { Text(c, "Hello").Selectable().Width(280).Height(70); Box(c).Height(1000).Shrink(0) })
		}, 320, 180)
		tt.rt.host = &selectionTestHost{tt.h}
		touch(tt, platform.PointerDown, 1, 50, 40)
		if kind == platform.SurfaceBlur || kind == platform.SurfaceResize {
			tt.send(platform.SurfaceEvent{Kind: kind})
		} else {
			touch(tt, kind, 1, 50, 10)
		}
		tt.Advance(time.Second)
		tt.send(platform.SurfaceEvent{Kind: platform.PointerLongPress, PointerType: platform.PointerTouch, PointerID: 1, X: 50, Y: 10})
		if tt.h.ime.Active {
			t.Fatalf("%v became accidental selection", kind)
		}
		if kind == platform.PointerMove && scroll.Y <= 0 {
			t.Fatal("a swipe over text did not scroll")
		}
		touch(tt, platform.PointerUp, 1, 50, 10)
	}
}

func TestNativeLongPressCanPrecedeScheduledFrame(t *testing.T) {
	tt := touchMessageFixture(true)
	touch(tt, platform.PointerDown, 7, 30, 15)
	// Advance the clock without delivering a display frame: UIKit can recognize
	// its hold before the Go deadline's frame gets to draw.
	tt.now = tt.now.Add(500 * time.Millisecond)
	accepted := tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerLongPress, PointerType: platform.PointerTouch, PointerID: 7, X: 30, Y: 15})
	tt.Frame()
	if !accepted || !tt.h.ime.Active || tt.h.ime.Start == tt.h.ime.End {
		t.Fatalf("native long press accepted=%v ime=%+v touch=%+v", accepted, tt.h.ime, tt.rt.touch)
	}
	touch(tt, platform.PointerCancel, 7, 30, 15)
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "copy"})
	if tt.Clipboard() != "Hello" {
		t.Fatal("native takeover cleared the selection")
	}
}

func TestTouchSelectionPreservesControlsAndMouse(t *testing.T) {
	value := "Editable"
	clicks := 0
	tt := snapshotTester(func(c *Context) {
		Column(c).Selectable().Children(func() {
			Text(c, "Hello").Label("message").Width(280).Height(40)
			Link(c, "Open link", "https://example.com").Height(44)
			if Button(c, "Action").Height(44).Clicked() {
				clicks++
			}
			TextInput(c, &value).Label("input").Width(280).Height(44)
		})
	}, 320, 240)
	tt.rt.host = &selectionTestHost{tt.h}
	for _, label := range []string{"Open link", "Action", "input"} {
		rect, _ := tt.Find(label)
		x, y := rect.X+rect.W/2, rect.Y+rect.H/2
		touch(tt, platform.PointerDown, 1, x, y)
		touch(tt, platform.PointerUp, 1, x, y)
	}
	if len(tt.OpenedURLs()) != 1 || clicks != 1 || !tt.h.ime.Active || tt.h.ime.ReadOnly {
		t.Fatal("long-press arbitration stole a link, button, or editable tap")
	}
	tt.ClickAt(30, 15)
	tt.ClickAt(30, 15)
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "copy"})
	if tt.Clipboard() != "Hello" {
		t.Fatalf("mouse copied %q ime=%+v", tt.Clipboard(), tt.h.ime)
	}
}

func TestNativeLongPressKeepsSelectionAcrossKeyboardResize(t *testing.T) {
	tt := touchMessageFixture(true)
	touch(tt, platform.PointerDown, 9, 30, 15)
	tt.Advance(500 * time.Millisecond)
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceResize})
	accepted := tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerLongPress, PointerType: platform.PointerTouch, PointerID: 9, X: 30, Y: 15})
	if !accepted {
		t.Fatal("keyboard resize lost the completed native long press")
	}
	tt.Frame()
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "copy"})
	if tt.Clipboard() != "Hello" {
		t.Fatal("keyboard resize cleared the copied word")
	}
	if tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerLongPress, PointerType: platform.PointerTouch, PointerID: 8, X: 30, Y: 15}) {
		t.Fatal("stale contact inherited another contact's selection")
	}
}
