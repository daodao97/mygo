package main

import (
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const editingSample = "Hello MyGo 👋\nSelect text, then cut, copy or paste.\n中文与 emoji stay intact."

func (s *demo) editingControls(c *ui.Context) {
	ui.Text(c, "Long-press text to select it. Drag the system handles to adjust the selection.").FontSize(15).LineHeight(1.4)
	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, "Reset sample").Height(40).Clicked() {
			s.editingValue, s.editingResult = editingSample, ""
		}
		if ui.Button(c, "Read editing clipboard").Height(40).Clicked() {
			s.editingResult = "Copied: " + mygo.Clipboard.ReadText()
		}
	})
	area := ui.TextArea(c, &s.editingValue).HideScrollbars().Label("Editing sample").InputOptions(ui.InputOptions{
		Keyboard: ui.KeyboardText, Correction: ui.CorrectionOff, Capitalization: ui.CapitalizeNone,
	}).Height(120).FillWidth()
	revealEditingField(c, area)
	if s.editingValue == "" {
		ui.Text(c, "Editor is empty")
	}
	if s.editingResult != "" {
		ui.Text(c, s.editingResult).FontSize(14)
	}
	ui.Text(c, "Password fields accept paste but never expose surrounding text or offer copy and cut.").FontSize(15).LineHeight(1.4)
	password := ui.TextInput(c, &s.editingPassword).Password().Label("Editing password").Height(48).FillWidth()
	revealEditingField(c, password)
	ui.Textf(c, "Password length: %d", utf8.RuneCountInString(s.editingPassword))
	ui.Text(c, "Input Method Composition").FontSize(20).Bold()
	ui.Text(c, "Use a Chinese or Japanese keyboard. The committed value excludes marked text until a candidate is accepted.").FontSize(14).LineHeight(1.4)
	if ui.Button(c, "Reset composition").Height(44).Clicked() {
		s.compositionValue = ""
	}
	ui.Text(c, "Committed input: "+s.compositionValue).FontSize(14)
	input := ui.TextInput(c, &s.compositionValue).Label("Composition input").Height(48).FillWidth().InputOptions(ui.InputOptions{Keyboard: ui.KeyboardText, Correction: ui.CorrectionOff, Capitalization: ui.CapitalizeNone})
	revealEditingField(c, input)
}

func revealEditingField(c *ui.Context, field *ui.Element) {
	viewport := ui.Local(field, "editing-viewport", func() inputViewport { return inputViewport{} })
	w, h := c.Size()
	focused := field.Focused()
	if focused && (!viewport.focused || viewport.width != w || viewport.height != h) {
		field.ScrollIntoView()
	}
	viewport.focused, viewport.width, viewport.height = focused, w, h
}
