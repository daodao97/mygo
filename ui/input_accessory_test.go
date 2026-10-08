package ui

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func TestInputAccessoryPreservesCompositionAndRejectsStaleOwner(t *testing.T) {
	value, enabled, chosen := "", true, ""
	tt := NewTester(func(c *Context) {
		e := TextInput(c, &value).Label("Input").Width(300).AutoFocus()
		if enabled {
			e.InputAccessory([]InputAction{{ID: "escape", Label: "Esc"}, {ID: "more", Label: "More", Items: []InputAction{{ID: "paste", Label: "Paste"}}}}, func(id string) { chosen = id })
		}
		Button(c, "Tool").KeepFocus()
	}, 320, 180)
	var actions []InputAction
	if err := json.Unmarshal([]byte(tt.h.ime.Accessory), &actions); err != nil || len(actions) != 2 || actions[1].Items[0].ID != "paste" {
		t.Fatalf("accessory model lost: %s", tt.h.ime.Accessory)
	}
	owner := tt.h.ime.AccessoryID
	tt.Compose("ni", 2)
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceInputAction, ID: owner, Text: "paste"})
	if chosen != "paste" || !tt.Focused("Input") || tt.rt.states[owner].editor.compose != "ni" {
		t.Fatal("accessory action changed focus or committed the IME")
	}
	tt.Click("Tool")
	if !tt.Focused("Input") || tt.rt.states[owner].editor.compose != "ni" {
		t.Fatal("toolbar press changed focus or cancelled the IME")
	}
	tt.Type("你")
	if value != "你" {
		t.Fatalf("candidate commit after accessory interaction: %q", value)
	}
	chosen = ""
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceInputAction, ID: owner + 1, Text: "paste"})
	if chosen != "" {
		t.Fatal("stale owner received an action")
	}
	enabled = false
	tt.Frame()
	if tt.h.ime.Accessory != "" {
		t.Fatal("omitted accessory remained attached")
	}
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceInputAction, ID: owner, Text: "paste"})
	if chosen != "" {
		t.Fatal("removed accessory still received events")
	}
}

func TestCustomTextCaretAccessoryHasAnOwner(t *testing.T) {
	chosen := ""
	tt := NewTester(func(c *Context) {
		Box(c).Label("Terminal").Size(300, 100).AutoFocus().TextCaret(Rect{X: 4, Y: 4, W: 1, H: 20}).HandleInput(func(InputEvent) bool { return true }).InputAccessory([]InputAction{{ID: "tab", Label: "Tab"}}, func(id string) { chosen = id })
	}, 320, 180)
	if tt.h.ime.ID != 0 || tt.h.ime.AccessoryID == 0 {
		t.Fatal("custom text input accessory lost its independent owner")
	}
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceInputAction, ID: tt.h.ime.AccessoryID, Text: "tab"})
	if chosen != "tab" || !tt.Focused("Terminal") {
		t.Fatal("custom input action was not delivered without losing focus")
	}
}

func TestSoftwareInputModifiersRespectCompositionAndSingleKeyConsumption(t *testing.T) {
	mods, consumed := Ctrl, 0
	var events []InputEvent
	tt := NewTester(func(c *Context) {
		Box(c).Fill().AutoFocus().TextCaret(Rect{W: 1, H: 20}).InputModifiers(mods, func() { consumed++; mods = 0 }).HandleInput(func(ev InputEvent) bool { events = append(events, ev); return true })
	}, 320, 180)
	tt.Type("c")
	if events[len(events)-1].Mods != Ctrl || consumed != 1 {
		t.Fatal("software Ctrl did not reach next letter", events, consumed)
	}
	tt.Type("d")
	if events[len(events)-1].Mods != 0 {
		t.Fatal("one-shot Ctrl leaked")
	}
	mods = Alt
	tt.Frame()
	tt.Compose("zhong", 5)
	tt.Type("中")
	if events[len(events)-1].Mods != 0 || consumed != 1 || mods != Alt {
		t.Fatal("candidate commit consumed the modifier")
	}
	tt.Type("b")
	if events[len(events)-1].Mods != Alt || consumed != 2 {
		t.Fatal("modifier not available after candidate commit")
	}
	mods = Super
	tt.Frame()
	tt.Type("pasted text")
	if events[len(events)-1].Mods != 0 || consumed != 2 {
		t.Fatal("multi-character text turned into shortcuts")
	}
}

func TestAccessoryLongPressAndStateSurviveJSONWithoutMutatingDefinitions(t *testing.T) {
	actions := []InputAction{{ID: "ctrl", Label: "Ctrl", LongPressID: "lock:ctrl", Selected: true, Locked: true}}
	var chosen string
	tt := NewTester(func(c *Context) {
		Box(c).Fill().AutoFocus().TextCaret(Rect{W: 1, H: 20}).HandleInput(func(InputEvent) bool { return true }).InputAccessory(actions, func(id string) { chosen = id })
	}, 320, 180)
	var got []InputAction
	if err := json.Unmarshal([]byte(tt.h.ime.Accessory), &got); err != nil || !got[0].Locked || got[0].LongPressID != "lock:ctrl" {
		t.Fatal(got, err)
	}
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceInputAction, ID: tt.h.ime.AccessoryID, Text: "lock:ctrl"})
	if chosen != "lock:ctrl" {
		t.Fatal("long-press action lost")
	}
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceInputAction, ID: tt.h.ime.AccessoryID + 1, Text: "ctrl"})
	if chosen != "lock:ctrl" {
		t.Fatal("old owner changed modifier state")
	}
}

func TestSoftwareControlKeyReleaseKeepsConsumedModifiers(t *testing.T) {
	mods := Alt
	var events []InputEvent
	tt := NewTester(func(c *Context) {
		Box(c).Fill().AutoFocus().TextCaret(Rect{W: 1, H: 20}).InputModifiers(mods, func() { mods = 0 }).HandleInput(func(ev InputEvent) bool { events = append(events, ev); return true })
	}, 320, 180)
	tt.Key(0, KeyBackspace)
	n := len(events)
	if n < 2 || events[n-2].Mods != Alt || events[n-1].Mods != Alt || !events[n-1].Software || mods != 0 {
		t.Fatal("release lost one-shot modifiers", events)
	}
	tt.Key(0, KeyBackspace)
	if events[len(events)-1].Mods != 0 {
		t.Fatal("modifier leaked into following key")
	}
}
