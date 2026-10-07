package main

import (
	"errors"
	"fmt"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (s *demo) mobileView(c *ui.Context) {
	if s.inputKind == "secrets" {
		s.secretView(c)
		return
	}
	ui.Scroll(c).HideScrollbars().Key("mobile-services").Fill().Padding(20).Gap(12).Children(func() {
		ui.Text(c, "Mobile services").FontSize(26).Bold()
		p := c.Preferences()
		ui.Textf(c, "Text scale: %.2f", p.TextScale)
		ui.Textf(c, "Reduce motion: %v · Contrast: %v", p.ReduceMotion, p.HighContrast)
		s.inputControls(c)
		s.dialogControls(c)
		s.permissionControls(c)
	})
}

func (s *demo) inputControls(c *ui.Context) {
	options := ui.InputOptions{Return: ui.ReturnDone, Correction: ui.CorrectionOff, Capitalization: ui.CapitalizeNone}
	switch s.inputKind {
	case "number":
		options.Keyboard = ui.KeyboardNumber
	case "decimal":
		options.Keyboard = ui.KeyboardDecimal
	case "phone":
		options.Keyboard = ui.KeyboardPhone
	case "url":
		options.Keyboard = ui.KeyboardURL
	case "code":
		options.Keyboard, options.Content = ui.KeyboardNumber, ui.ContentOneTimeCode
	default:
		options.Keyboard, options.Content, options.Return = ui.KeyboardEmail, ui.ContentEmail, ui.ReturnSend
	}
	ui.Text(c, "Keyboard: "+s.inputKind)
	field := ui.TextInput(c, &s.inputValue).InputOptions(options).Label("Mobile field").Height(48).FillWidth()
	if field.Submitted() {
		s.submissions++
	}
	ui.Text(c, "Input value: "+s.inputValue)
	ui.Textf(c, "Submissions: %d", s.submissions)
}

func (s *demo) dialogControls(c *ui.Context) {
	if ui.Button(c, "Native alert").Height(48).Clicked() {
		s.showMessage(mygo.MessageAlert)
	}
	if ui.Button(c, "Native actions").Height(48).Clicked() {
		s.showMessage(mygo.MessageActionSheet)
	}
	if ui.Button(c, "Share text").Height(48).Clicked() {
		go func() {
			r, err := mygo.Share.Show(mygo.ShareOptions{Parent: s.window, Text: "MyGo mobile sharing"})
			s.window.Update(func() {
				if err != nil {
					s.systemResult = err.Error()
				} else {
					s.systemResult = fmt.Sprintf("Share completed: %v", r.Completed)
				}
			})
		}()
	}
	ui.Text(c, s.systemResult)
	if ui.Button(c, "Read clipboard").Height(48).Clicked() {
		s.systemResult = "Clipboard: " + mygo.Clipboard.ReadText()
	}
}

func (s *demo) permissionControls(c *ui.Context) {
	if ui.Button(c, "Check camera permission").Height(48).Clicked() {
		s.checkPermission(mygo.PermissionCamera, false)
	}
	if ui.Button(c, "Request camera permission").Height(48).Clicked() {
		s.checkPermission(mygo.PermissionCamera, true)
	}
	if ui.Button(c, "Request microphone permission").Height(48).Clicked() {
		s.checkPermission(mygo.PermissionMicrophone, true)
	}
	if ui.Button(c, "Open app settings").Height(48).Clicked() {
		go func() {
			err := mygo.Permissions.OpenSettings()
			s.window.Update(func() {
				if err != nil {
					s.systemResult = err.Error()
				} else {
					s.systemResult = "Settings opened"
				}
			})
		}()
	}
}

func (s *demo) secretView(c *ui.Context) {
	ui.Column(c).Key("secure-storage").Fill().Padding(20).Gap(12).Children(func() {
		ui.Text(c, "Secure storage").FontSize(26).Bold()
		if ui.Button(c, "Set secret").Height(48).Clicked() {
			s.secretOperation(func() string {
				err := s.secrets.Set("e2e-binary", []byte{0, 255, 1, 2, 3})
				if err != nil {
					return err.Error()
				}
				return "Secret saved"
			})
		}
		if ui.Button(c, "Update secret").Height(48).Clicked() {
			s.secretOperation(func() string {
				err := s.secrets.Set("e2e-binary", []byte("更新 👋"))
				if err != nil {
					return err.Error()
				}
				return "Secret updated"
			})
		}
		if ui.Button(c, "Read secret").Height(48).Clicked() {
			s.secretOperation(func() string {
				v, err := s.secrets.Get("e2e-binary")
				if errors.Is(err, mygo.ErrSecretNotFound) {
					return "Secret absent"
				}
				if err != nil {
					return err.Error()
				}
				return fmt.Sprintf("Secret: %x", v)
			})
		}
		if ui.Button(c, "Empty secret").Height(48).Clicked() {
			s.secretOperation(func() string {
				err := s.secrets.Set("e2e-binary", nil)
				if err != nil {
					return err.Error()
				}
				return "Empty secret saved"
			})
		}
		if ui.Button(c, "Delete secret").Height(48).Clicked() {
			s.secretOperation(func() string {
				err := s.secrets.Delete("e2e-binary")
				if err != nil {
					return err.Error()
				}
				return "Secret deleted"
			})
		}
		ui.Text(c, s.systemResult)
	})
}

func (s *demo) secretOperation(operation func() string) {
	go func() { result := operation(); s.window.Update(func() { s.systemResult = result }) }()
}

func (s *demo) checkPermission(kind mygo.Permission, request bool) {
	go func() {
		var status mygo.PermissionStatus
		var err error
		if request {
			status, err = mygo.Permissions.Request(kind)
		} else {
			status, err = mygo.Permissions.Query(kind)
		}
		s.window.Update(func() {
			if err != nil {
				s.systemResult = err.Error()
			} else {
				s.systemResult = fmt.Sprintf("Permission %s: %s", kind, status)
			}
		})
	}()
}

func (s *demo) showMessage(style mygo.MessageStyle) {
	go func() {
		r, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: s.window, Title: "MyGo native dialog", Message: "Choose a result",
			Detail: "Go remains responsive", Buttons: []string{"Confirm", "Cancel", "Delete"}, CancelButton: 1, Style: style, DestructiveButtons: []int{2}})
		s.window.Update(func() {
			if err != nil {
				s.systemResult = err.Error()
			} else {
				s.systemResult = fmt.Sprintf("Dialog result: %d", r.Button)
			}
		})
	}()
}
