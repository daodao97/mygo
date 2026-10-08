package ui

import (
	"github.com/egoist/mygo/internal/platform"
	"testing"
)

func TestCustomTextContextNativeEditing(t *testing.T) {
	text, caret := "ab你好😀cd", 7
	var events []InputEvent
	var preedit string
	var mods Modifiers
	consumed := 0
	tt := NewTester(func(c *Context) {
		Box(c).Fill().AutoFocus().TextCaret(Rect{0, 0, 1, 20}).TextContext(func() (string, int) { return text, caret }).InputModifiers(mods, func() { consumed++; mods = 0 }).HandleInput(func(ev InputEvent) bool {
			events = append(events, ev)
			switch ev.Kind {
			case InputTextReplace:
				r := []rune(text)
				text = string(r[:ev.From]) + ev.Text + string(r[ev.To:])
				caret = ev.Caret
				preedit = ""
			case InputSelection:
				caret = ev.Caret
			case InputCompose:
				preedit = ev.Text
			}
			return true
		})
	}, 320, 200)
	defer tt.rt.close()
	snapshot := func(s string, pos int) {
		tt.send(platform.SurfaceEvent{Kind: platform.TextInput, Snapshot: true, Replace: true, Text: s, To: len([]rune(tt.h.ime.Text)), Caret: pos})
	}
	if tt.h.ime.Text != text || tt.h.ime.Start != 7 {
		t.Fatal("context not exposed", tt.h.ime)
	}
	tt.send(platform.SurfaceEvent{Kind: platform.TextSelectionChanged, From: 2, To: 2})
	if caret != 2 {
		t.Fatal("native caret lost", caret)
	}
	snapshot("ab新你好😀cd", 3)
	last := events[len(events)-1]
	if last.Kind != InputTextReplace || last.From != 2 || last.To != 2 || last.Text != "新" {
		t.Fatal("unchanged text re-sent", last)
	}
	snapshot("ab新你好😀cd", 3)
	if events[len(events)-1].Kind != InputCompose {
		t.Fatal("unchanged snapshot created another edit")
	}
	tt.send(platform.SurfaceEvent{Kind: platform.TextComposition, Snapshot: true, Text: "ab新ni你好😀cd", MarkedStart: 3, MarkedEnd: 5, Caret: 5})
	if text != "ab新你好😀cd" || preedit != "ni" {
		t.Fatal("committed context became preedit", text, preedit)
	}
	snapshot("ab新你你好😀cd", 4)
	if text != "ab新你你好😀cd" || preedit != "" {
		t.Fatal("composition did not commit once", text, preedit)
	}
	// A pending candidate does not consume an armed modifier.
	mods = Ctrl
	tt.Frame()
	tt.send(platform.SurfaceEvent{Kind: platform.TextComposition, Snapshot: true, Text: "ab新你hao你好😀cd", MarkedStart: 4, MarkedEnd: 7, Caret: 7})
	snapshot("ab新你好你好😀cd", 5)
	if consumed != 0 || mods != Ctrl {
		t.Fatal("candidate consumed Ctrl")
	}
	snapshot("ab新你好c你好😀cd", 6)
	last = events[len(events)-1]
	if last.Mods != Ctrl || !last.Software || consumed != 1 {
		t.Fatal("Ctrl lost on contextual ASCII", last, consumed)
	}
}

func TestCustomTextContextModifiedDelete(t *testing.T) {
	var events []InputEvent
	mods := Ctrl
	tt := NewTester(func(c *Context) {
		Box(c).AutoFocus().TextCaret(Rect{0, 0, 1, 20}).TextContext(func() (string, int) { return "abc", 3 }).InputModifiers(mods, func() { mods = 0 }).HandleInput(func(ev InputEvent) bool { events = append(events, ev); return true })
	}, 320, 200)
	defer tt.rt.close()
	tt.send(platform.SurfaceEvent{Kind: platform.TextInput, Snapshot: true, Replace: true, Text: "ab", To: 3, Caret: 2})
	if len(events) != 2 || events[0].Kind != InputKeyDown || events[0].Key != KeyBackspace || events[0].Mods != Ctrl || events[1].Kind != InputKeyUp || events[1].Mods != Ctrl {
		t.Fatal(events)
	}
}

func TestCustomTextContextIncrementalCandidate(t *testing.T) {
	text, caret := "", 0
	var preedit string
	mods, consumed := Ctrl, 0
	tt := NewTester(func(c *Context) {
		Box(c).AutoFocus().TextCaret(Rect{0, 0, 1, 20}).TextContext(func() (string, int) { return text, caret }).InputModifiers(mods, func() { consumed++; mods = 0 }).HandleInput(func(ev InputEvent) bool {
			switch ev.Kind {
			case InputTextReplace:
				r := []rune(text)
				text = string(r[:ev.From]) + ev.Text + string(r[ev.To:])
				caret = ev.Caret
			case InputCompose:
				preedit = ev.Text
			}
			return true
		})
	}, 320, 200)
	defer tt.rt.close()
	tt.send(platform.SurfaceEvent{Kind: platform.TextComposition, Snapshot: true, Text: "你好ni", MarkedStart: 2, MarkedEnd: 4, Caret: 4})
	if text != "你好" || preedit != "ni" || consumed != 0 {
		t.Fatal(text, preedit, consumed)
	}
	tt.send(platform.SurfaceEvent{Kind: platform.TextComposition, Snapshot: true, Text: "你好你hao", MarkedStart: 3, MarkedEnd: 6, Caret: 6})
	if text != "你好你" || preedit != "hao" || consumed != 0 {
		t.Fatal(text, preedit, consumed)
	}
	tt.send(platform.SurfaceEvent{Kind: platform.TextInput, Snapshot: true, Text: "你好你好", Replace: true, To: 3, Caret: 4})
	if text != "你好你好" || mods != Ctrl || consumed != 0 {
		t.Fatal("incremental candidates duplicated", text, mods, consumed)
	}
}
