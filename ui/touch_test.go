package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func touch(tt *Tester, kind platform.SurfaceEventKind, id uint64, x, y float32) {
	tt.send(platform.SurfaceEvent{Kind: kind, PointerType: platform.PointerTouch, PointerID: id, X: float64(x), Y: float64(y), Clicks: 1})
}

func TestTouchScrollCustomWidgetDefersKeyboardAndSelection(t *testing.T) {
	var events []InputEvent
	tt := NewTester(func(c *Context) {
		e := Box(c).Fill().Focusable().TouchScroll().TextCaret(Rect{W: 1, H: 20})
		e.PointerPosition() // Pointer tracking must not steal finger scrolling.
		e.HandleInput(func(ev InputEvent) bool { events = append(events, ev); return true })
	}, 320, 240)
	touch(tt, platform.PointerDown, 1, 120, 180)
	touch(tt, platform.PointerMove, 1, 120, 177)
	if len(events) != 0 || tt.h.ime.Active {
		t.Fatal("a reserved contact selected text or opened the keyboard")
	}
	touch(tt, platform.PointerMove, 1, 120, 140)
	touch(tt, platform.PointerMove, 1, 120, 100)
	touch(tt, platform.PointerUp, 1, 120, 100)
	var dy float32
	for _, ev := range events {
		if ev.Kind != InputScroll || !ev.Precise {
			t.Fatalf("swipe delivered pointer selection: %+v", ev)
		}
		dy += ev.DY
	}
	if dy != 80 || tt.h.ime.Active || tt.rt.pressed != nil {
		t.Fatalf("swipe offset=%v keyboard=%v pressed=%v", dy, tt.h.ime.Active, tt.rt.pressed)
	}
	events = nil
	touch(tt, platform.PointerDown, 2, 120, 100)
	touch(tt, platform.PointerMove, 2, 120, 102)
	touch(tt, platform.PointerUp, 2, 120, 102)
	downs, ups := 0, 0
	for _, ev := range events {
		if ev.Kind == InputPointerDown {
			downs++
		}
		if ev.Kind == InputPointerUp {
			ups++
		}
	}
	if downs != 1 || ups != 1 || !tt.h.ime.Active {
		t.Fatalf("tap did not focus and deliver a complete click: %+v", events)
	}
	events = nil
	tt.send(platform.SurfaceEvent{Kind: platform.PointerDown, X: 120, Y: 100})
	tt.send(platform.SurfaceEvent{Kind: platform.PointerMove, X: 120, Y: 60})
	tt.send(platform.SurfaceEvent{Kind: platform.PointerUp, X: 120, Y: 60})
	for _, ev := range events {
		if ev.Kind == InputScroll {
			t.Fatal("mouse selection became a touch scroll")
		}
	}
}

func TestTouchBlurReleasesContact(t *testing.T) {
	for _, scrolling := range []bool{false, true} {
		t.Run(fmt.Sprint("scrolling=", scrolling), func(t *testing.T) {
			clicks := 0
			var scroll ScrollState
			tt := snapshotTester(func(c *Context) {
				Scroll(c).Fill().TrackScroll(&scroll).Children(func() {
					for i := range 20 {
						if Button(c, fmt.Sprint("Row ", i)).Height(48).Clicked() {
							clicks++
						}
					}
				})
			}, 320, 240)
			b, _ := tt.Find("Row 2")
			x, y := b.X+b.W/2, b.Y+b.H/2
			touch(tt, platform.PointerDown, 1, x, y)
			if scrolling {
				touch(tt, platform.PointerMove, 1, x, y-30)
			}
			offset := scroll.Y
			tt.send(platform.SurfaceEvent{Kind: platform.SurfaceBlur})
			if tt.rt.touch.active || tt.rt.touch.scrolling || tt.rt.pointerIn || tt.rt.pressed != nil || clicks != 0 {
				t.Fatal("blur retained the contact or activated its button")
			}
			tt.send(platform.SurfaceEvent{Kind: platform.SurfaceFocus})
			// Late input from the abandoned contact must be ignored too.
			touch(tt, platform.PointerUp, 1, x, y)
			b, _ = tt.Find("Row 2")
			x, y = b.X+b.W/2, b.Y+b.H/2
			touch(tt, platform.PointerDown, 2, x, y)
			touch(tt, platform.PointerUp, 2, x, y)
			if clicks != 1 || scroll.Y != offset {
				t.Fatalf("next touch: clicks=%d scroll=%v want=%v", clicks, scroll.Y, offset)
			}
		})
	}
}

func TestTouchBlurCancelsInputHandler(t *testing.T) {
	cancels, releases := 0, 0
	tt := snapshotTester(func(c *Context) {
		Box(c).Size(100, 100).HandleInput(func(ev InputEvent) bool {
			switch ev.Kind {
			case InputPointerCancel:
				cancels++
			case InputPointerUp:
				releases++
			}
			return true
		})
	}, 320, 240)
	touch(tt, platform.PointerDown, 1, 50, 50)
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceBlur})
	if cancels != 1 || releases != 0 {
		t.Fatalf("interrupted handler got cancel=%d release=%d", cancels, releases)
	}
}

func TestTouchScrollCancelsButton(t *testing.T) {
	clicks := 0
	var scroll ScrollState
	tt := snapshotTester(func(c *Context) {
		Scroll(c).Fill().TrackScroll(&scroll).Children(func() {
			for i := range 20 {
				if Button(c, fmt.Sprint("Row ", i)).Height(48).Clicked() {
					clicks++
				}
			}
		})
	}, 320, 240)
	r, _ := tt.Find("Row 2")
	x, y := r.X+r.W/2, r.Y+r.H/2
	touch(tt, platform.PointerDown, 1, x, y)
	touch(tt, platform.PointerMove, 1, x, y-2)
	if scroll.Y != 0 {
		t.Fatal("a small movement must remain a tap")
	}
	touch(tt, platform.PointerUp, 1, x, y-2)
	if clicks != 1 {
		t.Fatalf("tap produced %d clicks", clicks)
	}
	touch(tt, platform.PointerDown, 2, x, y)
	touch(tt, platform.PointerMove, 2, x, y-40)
	touch(tt, platform.PointerUp, 2, x, y-40)
	if scroll.Y < 30 || clicks != 1 {
		t.Fatalf("swipe: scroll=%v clicks=%d", scroll.Y, clicks)
	}
	if tt.rt.pressed != nil || tt.rt.touch.active {
		t.Fatal("swipe left a pointer pressed")
	}
}

func TestTouchCancelAndExtraContact(t *testing.T) {
	clicks := 0
	tt := snapshotTester(func(c *Context) {
		if Button(c, "Tap").Clicked() {
			clicks++
		}
	}, 320, 240)
	r, _ := tt.Find("Tap")
	x, y := r.X+r.W/2, r.Y+r.H/2
	touch(tt, platform.PointerDown, 1, x, y)
	touch(tt, platform.PointerDown, 2, x, y)
	touch(tt, platform.PointerUp, 2, x, y)
	if tt.rt.pressed == nil {
		t.Fatal("an extra contact released the primary touch")
	}
	touch(tt, platform.PointerCancel, 1, x, y)
	touch(tt, platform.PointerUp, 1, x, y)
	if clicks != 0 || tt.rt.pressed != nil {
		t.Fatal("cancelled contact produced a click or remained pressed")
	}
	touch(tt, platform.PointerDown, 3, x, y)
	touch(tt, platform.PointerUp, 3, x, y)
	if clicks != 1 {
		t.Fatal("cancelled contact prevented the next tap")
	}
}

func TestNativeTextSnapshotAndSelection(t *testing.T) {
	value := "你好 👋 world"
	tt := snapshotTester(func(c *Context) { TextInput(c, &value).Label("Message").Width(300).AutoFocus() }, 320, 240)
	tt.send(platform.SurfaceEvent{Kind: platform.TextInput, Text: "你好 🌏 world", Replace: true, From: 0, To: 10, Snapshot: true, Caret: 4})
	if value != "你好 🌏 world" || tt.h.ime.Start != 4 || tt.h.ime.End != 4 {
		t.Fatalf("snapshot: text=%q selection=%d:%d", value, tt.h.ime.Start, tt.h.ime.End)
	}
	tt.send(platform.SurfaceEvent{Kind: platform.TextSelectionChanged, From: 3, To: 4})
	tt.Type("😀")
	if value != "你好 😀 world" {
		t.Fatalf("rune selection split an emoji: %q", value)
	}
}

func TestNativeCompositionSnapshot(t *testing.T) {
	value := "Hello "
	tt := snapshotTester(func(c *Context) { TextInput(c, &value).Width(300).AutoFocus() }, 320, 240)
	tt.send(platform.SurfaceEvent{Kind: platform.TextComposition, Text: "Hello zhong", Replace: true, From: 0, To: 6, Caret: 11})
	tt.send(platform.SurfaceEvent{Kind: platform.TextComposition, Text: "Hello 中", Replace: true, From: 0, To: len([]rune(tt.h.ime.Text)), Caret: 7})
	tt.send(platform.SurfaceEvent{Kind: platform.TextInput, Text: "Hello 中文👋", Replace: true, From: 0, To: len([]rune(tt.h.ime.Text)), Snapshot: true, Caret: 9})
	if value != "Hello 中文👋" {
		t.Fatalf("composition committed %q", value)
	}
}

func TestNativeMarkedTextKeepsCommittedContext(t *testing.T) {
	value := "Before 👋 after"
	tt := snapshotTester(func(c *Context) { TextArea(c, &value).Width(300).AutoFocus() }, 320, 240)
	snapshot := func(text string, caret int) {
		tt.send(platform.SurfaceEvent{Kind: platform.TextInput, Text: text, Replace: true,
			To: len([]rune(tt.h.ime.Text)), Snapshot: true, Caret: caret})
	}
	compose := func(text string, at, end int) {
		tt.send(platform.SurfaceEvent{Kind: platform.TextComposition, Text: text, Replace: true, Snapshot: true,
			To: len([]rune(tt.h.ime.Text)), MarkedStart: at, MarkedEnd: end, Caret: end})
	}
	for _, marked := range []string{"zhong", "中", "中文"} {
		compose("Before 👋"+marked+" after", 8, 8+len([]rune(marked)))
		if value != "Before 👋 after" {
			t.Fatalf("provisional %q changed committed text: %q", marked, value)
		}
	}
	// A candidate commits while the next syllable remains marked.
	compose("Before 👋中文shu after", 10, 13)
	if value != "Before 👋中文 after" {
		t.Fatalf("incremental candidate lost surrounding text: %q", value)
	}
	// Canceling that syllable must retain the earlier committed candidate.
	snapshot("Before 👋中文 after", 10)
	if value != "Before 👋中文 after" {
		t.Fatalf("cancel changed committed text: %q", value)
	}
	for _, s := range tt.rt.states {
		if ed := s.editor; ed != nil && ed.compose != "" {
			t.Fatal("cancel retained marked text")
		}
	}
	// Repeated unchanged snapshots must not add undo steps.
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "undo"})
	if value != "Before 👋 after" {
		t.Fatalf("undo did not remove only the committed candidate: %q", value)
	}
}

func TestNativeCompositionPinsLongSurroundingContext(t *testing.T) {
	value := strings.Repeat("前", 1200) + "👋"
	original := value
	tt := snapshotTester(func(c *Context) { TextArea(c, &value).Width(300).AutoFocus() }, 320, 240)
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "selectAll"})
	tt.send(platform.SurfaceEvent{Kind: platform.TextSelectionChanged, From: len([]rune(tt.h.ime.Text)), To: len([]rune(tt.h.ime.Text))})
	base, context := tt.rt.ime.base, tt.h.ime.Text
	if base == 0 {
		t.Fatal("test did not reach a bounded surrounding context")
	}
	start := len([]rune(context))
	for _, candidate := range []string{"中", "中文", "中文书"} {
		at := start + len([]rune(candidate))
		tt.send(platform.SurfaceEvent{Kind: platform.TextComposition, Text: context + candidate + "pinyin", Replace: true, Snapshot: true,
			To: len([]rune(tt.h.ime.Text)), MarkedStart: at, MarkedEnd: at + 6, Caret: at + 6})
		if value != original+candidate || tt.rt.ime.base != base || tt.h.ime.Text != context+candidate {
			t.Fatalf("candidate %q rebased/lost context: base=%d want=%d text=%q", candidate, tt.rt.ime.base, base, value)
		}
	}
	tt.send(platform.SurfaceEvent{Kind: platform.TextInput, Text: context + "中文书👋", Replace: true, Snapshot: true,
		To: len([]rune(tt.h.ime.Text)), Caret: start + 4})
	if value != original+"中文书👋" || tt.rt.ime.base <= base {
		t.Fatal("commit did not release the pinned context or split Unicode text")
	}
}

func TestNativeEditorHistoryAvailability(t *testing.T) {
	value := "Hello 👋"
	tt := snapshotTester(func(c *Context) { TextInput(c, &value).AutoFocus() }, 320, 240)
	if tt.h.ime.CanUndo || tt.h.ime.CanRedo {
		t.Fatal("new editor advertised history")
	}
	tt.send(platform.SurfaceEvent{Kind: platform.TextInput, Text: "中文 👋", Replace: true, Snapshot: true,
		To: len([]rune(tt.h.ime.Text)), Caret: 4})
	if !tt.h.ime.CanUndo || tt.h.ime.CanRedo {
		t.Fatal("native undo availability did not follow the edit")
	}
	tt.Command("undo")
	if value != "Hello 👋" || tt.h.ime.CanUndo || !tt.h.ime.CanRedo {
		t.Fatal("native history did not follow undo")
	}
	tt.Command("redo")
	if value != "中文 👋" || !tt.h.ime.CanUndo || tt.h.ime.CanRedo {
		t.Fatal("native history did not follow redo")
	}
}

func TestTouchKeyboardDismissDoesNotMovePressedControl(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint("cancelled=", cancelled), func(t *testing.T) {
			value, clicks := "", 0
			tt := snapshotTester(func(c *Context) {
				Column(c).Fill().Children(func() {
					TextInput(c, &value).Label("Message").Height(44).AutoFocus()
					Box(c).Grow(1)
					if Button(c, "Navigate").Height(48).Clicked() {
						clicks++
					}
				})
			}, 320, 240) // keyboard-adjusted viewport
			if !tt.h.ime.Active {
				t.Fatal("editor did not activate")
			}
			r, _ := tt.Find("Navigate")
			x, y := r.X+r.W/2, r.Y+r.H/2
			touch(tt, platform.PointerDown, 1, x, y)
			// Model a host which restores full height as soon as the input
			// method deactivates: this previously moved the button before Up.
			if !tt.h.ime.Active {
				tt.SetSize(320, 480)
			}
			if cancelled {
				touch(tt, platform.PointerCancel, 1, x, y)
			} else {
				touch(tt, platform.PointerUp, 1, x, y)
			}
			want := 1
			if cancelled {
				want = 0
			}
			if clicks != want {
				t.Fatalf("touch produced %d clicks, want %d", clicks, want)
			}
			if tt.h.ime.Active {
				t.Fatal("keyboard remained active after the touch ended")
			}
		})
	}
}
