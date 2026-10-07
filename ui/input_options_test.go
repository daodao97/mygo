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
			var e *Element
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
