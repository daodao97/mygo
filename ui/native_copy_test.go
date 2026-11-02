package ui

import (
	"github.com/egoist/mygo/internal/platform"
	"testing"
)

// Match the native menu's order: flush Copy before collapsing its document
// range and releasing the read-only responder. A shared range spans paragraphs.
func TestNativeCopyThenDismissReadOnlySelection(t *testing.T) {
	for _, shared := range []bool{false, true} {
		tt := snapshotTester(func(c *Context) {
			if shared {
				Column(c).Selectable().Children(func() {
					Text(c, "Hello 你好👋").Label("message")
					Text(c, "Second paragraph").Label("second")
				})
			} else {
				Text(c, "Hello 你好👋").Selectable().Label("message")
			}
		}, 320, 200)
		tt.rt.host = &selectionTestHost{tt.h}
		tt.Click("message")
		end := 9
		want := "你好👋"
		if shared {
			end += 1 + 6
			want += "\nSecond"
		}
		tt.send(platform.SurfaceEvent{Kind: platform.TextSelectionChanged, From: 6, To: end})
		field := tt.h.ime
		tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "copy"})
		if tt.Clipboard() != want {
			t.Fatalf("shared=%v: copy lost content: %q", shared, tt.Clipboard())
		}
		tt.send(platform.SurfaceEvent{Kind: platform.TextSelectionChanged, From: field.End, To: field.End})
		tt.send(platform.SurfaceEvent{Kind: platform.SurfaceKeyboardDismiss})
		if tt.h.ime.Active || tt.rt.focused != 0 {
			t.Fatal("copy retained native responder/handles")
		}
		if _, _, selected := tt.rt.textSelectionBounds(); selected {
			t.Fatal("copy retained shared selection")
		}
		for _, s := range tt.rt.texts {
			if s.editor.anchor != s.editor.caret {
				t.Fatal("copy retained a paragraph highlight")
			}
		}
		if tt.Clipboard() != want {
			t.Fatal("selection cleanup changed the clipboard")
		}
		tt.Click("message")
		tt.send(platform.SurfaceEvent{Kind: platform.TextSelectionChanged, From: 0, To: 5})
		tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "copy"})
		if tt.Clipboard() != "Hello" {
			t.Fatal("a later selection could not be copied")
		}
	}
}

func TestNativeEditableCopyKeepsSelection(t *testing.T) {
	value := "Hello 你好👋"
	tt := snapshotTester(func(c *Context) { TextInput(c, &value).Label("input").Width(280) }, 320, 120)
	tt.Click("input")
	tt.send(platform.SurfaceEvent{Kind: platform.TextSelectionChanged, From: 6, To: 9})
	before := tt.h.ime
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "copy"})
	if tt.Clipboard() != "你好👋" || !tt.h.ime.Active || tt.h.ime.Start != before.Start || tt.h.ime.End != before.End || value != "Hello 你好👋" {
		t.Fatal("editable copy changed text, focus, or selection")
	}
}
