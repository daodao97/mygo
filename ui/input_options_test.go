package ui

import (
	"github.com/egoist/mygo/internal/platform"
	"testing"
)

func TestInputOptionsFollowFieldAndReset(t *testing.T) {
	value, options := "", InputOptions{Keyboard: KeyboardEmail, Return: ReturnNext, Content: ContentEmail,
		Correction: CorrectionOff, Capitalization: CapitalizeNone}
	enabled := true
	tt := NewTester(func(c *Context) {
		e := TextInput(c, &value).Label("Email").Width(300).AutoFocus()
		if enabled {
			e.InputOptions(options)
		}
	}, 320, 200)
	if got := tt.h.ime.Options; got.Keyboard != "email" || got.Return != "next" || got.Content != "emailAddress" || got.Correction != "off" || got.Capitalization != "none" {
		t.Fatalf("keyboard options not forwarded: %+v", got)
	}
	tt.Type("中文👋@example.com")
	if value != "中文👋@example.com" {
		t.Fatalf("keyboard hint filtered text: %q", value)
	}
	options.Keyboard, options.Return = KeyboardNumber, ReturnDone
	tt.Frame()
	if tt.h.ime.Options.Keyboard != "number" || tt.h.ime.Options.Return != "done" {
		t.Fatal("changing traits was not propagated")
	}
	enabled = false
	tt.Frame()
	if tt.h.ime.Options != (platform.TextInputOptions{}) {
		t.Fatal("omitted traits leaked from a previous frame")
	}
}

func TestInputReturnPreservesSubmitAndMultiline(t *testing.T) {
	for _, multiline := range []bool{false, true} {
		value, submitted := "", false
		tt := NewTester(func(c *Context) {
			var e Element
			if multiline {
				e = TextArea(c, &value)
			} else {
				e = TextInput(c, &value)
			}
			e.InputOptions(InputOptions{Return: ReturnSend}).Width(300).AutoFocus()
			if e.Submitted() {
				submitted = true
			}
		}, 320, 200)
		tt.Type("hello")
		tt.Key(0, KeyEnter)
		if multiline && (value != "hello\n" || submitted) {
			t.Fatalf("text area submitted or lost newline: %q %v", value, submitted)
		}
		if !multiline && (value != "hello" || !submitted) {
			t.Fatalf("single line did not submit: %q %v", value, submitted)
		}
	}
}

func TestCustomTextCaretKeyboardOptions(t *testing.T) {
	enabled := true
	var received string
	tt := NewTester(func(c *Context) {
		e := Box(c).Size(300, 100).AutoFocus().TextCaret(Rect{X: 8, Y: 8, W: 1, H: 18}).HandleInput(func(ev InputEvent) bool {
			if ev.Kind == InputText {
				received += ev.Text
			}
			return true
		})
		if enabled {
			e.InputOptions(InputOptions{Keyboard: KeyboardASCII, Correction: CorrectionOff, Capitalization: CapitalizeNone})
		}
	}, 320, 200)
	if got := tt.h.ime.Options; !tt.h.ime.Active || got.Keyboard != "ascii" || got.Correction != "off" || got.Capitalization != "none" {
		t.Fatalf("custom input did not receive keyboard options: %+v", tt.h.ime)
	}
	tt.Type("echo 中文")
	if received != "echo 中文" {
		t.Fatalf("custom input changed committed text: %q", received)
	}
	enabled = false
	tt.Frame()
	if tt.h.ime.Options != (platform.TextInputOptions{}) {
		t.Fatal("custom input retained omitted keyboard options")
	}
}
