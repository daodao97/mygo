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
