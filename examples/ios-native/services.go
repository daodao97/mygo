package main

import (
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"math"
	"path/filepath"
	"time"
)

const demoNotificationID = "mygo-demo-local"

func (s *demo) notificationControls(c *ui.Context) {
	ui.Text(c, "Local Notifications").FontSize(24).Bold()
	ui.Text(c, "Schedule a notification, background or terminate the app, then tap it to return to this page.").FontSize(15).LineHeight(1.4)
	if ui.Button(c, "Request notification permission").Height(44).FillWidth().Clicked() {
		s.checkPermission(mygo.PermissionNotifications, true)
	}
	for _, delay := range []time.Duration{0, 5 * time.Second, 15 * time.Second} {
		label := "Show notification now"
		if delay > 0 {
			label = fmt.Sprintf("Schedule in %d seconds", int(delay.Seconds()))
		}
		if ui.Button(c, label).Height(44).FillWidth().Clicked() {
			n := mygo.NewNotification(mygo.NotificationOptions{ID: demoNotificationID, Title: "MyGo notification", Body: "Tap to open the Notifications feature.", Group: "demo", Data: map[string]string{"route": "/features/notifications"}})
			s.notificationVersion++
			version := s.notificationVersion
			s.notificationResult = "Submitting notification: " + demoNotificationID
			go func() {
				var err error
				if delay > 0 {
					err = n.Schedule(delay)
				} else {
					err = n.Show()
				}
				s.window.Update(func() {
					if version != s.notificationVersion {
						return
					}
					if err != nil {
						s.notificationResult = err.Error()
					} else if s.notificationResult == "Submitting notification: "+demoNotificationID {
						// An immediate foreground event can precede submission's
						// completion; retain its delivered/opened status.
						s.notificationResult = "Notification submitted: " + demoNotificationID
					}
				})
			}()
		}
	}
	if ui.Button(c, "Cancel demo notification").Height(44).FillWidth().Clicked() {
		s.notificationVersion++
		mygo.NewNotification(mygo.NotificationOptions{ID: demoNotificationID}).Close()
		s.notificationResult = "Notification cancelled"
	}
	ui.Row(c).Gap(8).Children(func() {
		for _, count := range []int{1, 0} {
			label := "Set badge to 1"
			if count == 0 {
				label = "Clear badge"
			}
			if ui.Button(c, label).Height(44).Clicked() {
				go func() {
					err := mygo.App.SetNotificationBadge(count)
					s.window.Update(func() {
						if err != nil {
							s.notificationResult = err.Error()
						} else {
							s.notificationResult = fmt.Sprintf("Badge count: %d", count)
						}
					})
				}()
			}
		}
	})
	ui.Text(c, s.notificationResult).FontSize(14).LineHeight(1.4)
	if s.notificationRoute != "" {
		ui.Text(c, "Notification route: "+s.notificationRoute).FontSize(13)
	}
	ui.Text(c, s.systemResult).FontSize(14)
	ui.Text(c, "APNs requires a matching push entitlement and your delivery service.").FontSize(13).TextColor(c.Theme().TextMuted)
	if ui.Button(c, "Register for push notifications").Height(44).FillWidth().Clicked() {
		if err := mygo.App.RegisterPushNotifications(); err != nil {
			s.notificationResult = err.Error()
		} else {
			s.notificationResult = "APNs registration requested"
		}
	}
}
func (s *demo) hapticControls(c *ui.Context) {
	ui.Text(c, "Haptic Feedback").FontSize(24).Bold()
	ui.Text(c, "Try each pattern on an iPhone. System settings and device hardware control the physical feedback.").FontSize(15).LineHeight(1.4)
	for _, kind := range []mygo.HapticFeedback{mygo.HapticSelection, mygo.HapticLight, mygo.HapticMedium, mygo.HapticHeavy, mygo.HapticSoft, mygo.HapticRigid, mygo.HapticSuccess, mygo.HapticWarning, mygo.HapticError} {
		if ui.Button(c, "Haptic "+string(kind)).Height(44).FillWidth().Clicked() {
			err := mygo.Haptics.Play(kind)
			if err != nil {
				s.hapticResult = err.Error()
			} else {
				s.hapticResult = "Haptic requested: " + string(kind)
			}
		}
	}
	ui.Text(c, s.hapticResult)
}
func (s *demo) gestureControls(c *ui.Context) {
	ui.Text(c, "Input & Gestures").FontSize(24).Bold()
	ui.Text(c, "Focus the field, then drag down outside it to track the keyboard. Drag upward again before lifting to cancel.").FontSize(15).LineHeight(1.4)
	field := ui.TextInput(c, &s.gestureValue).Label("Gesture input").Height(48).FillWidth().InputOptions(ui.InputOptions{Dismiss: ui.KeyboardDismissInteractive, Correction: ui.CorrectionOff, Capitalization: ui.CapitalizeNone})
	revealEditingField(c, field)
	ui.Text(c, "Input: "+s.gestureValue)
	ui.Text(c, "Read-only text: Hello MyGo 👋 中文").Selectable().Label("Read-only sample").FontSize(17).LineHeight(1.5)
	if ui.Button(c, "Read selection clipboard").Height(44).Clicked() {
		s.editingResult = "Selection clipboard: " + mygo.Clipboard.ReadText()
	}
	ui.Text(c, s.editingResult).FontSize(13)
	ui.Text(c, "Pinch and rotate the pad below. A gesture remains captured by its starting element.").FontSize(15).LineHeight(1.4)
	if s.gestureScale == 0 {
		s.gestureScale = 1
	}
	pad := ui.Box(c).Label("Gesture pad").Role(ui.RoleImage).Height(160).FillWidth().Radius(16).Background(ui.Hex("#dce7ff")).Center()
	for _, event := range pad.Gestures() {
		if event.Phase == "change" || event.Phase == "end" {
			s.gestureScale = max(0.5, min(2, s.gestureScale*event.Scale))
			s.gestureRotation += event.Rotation
			s.gestureUpdates++
		}
	}
	pad.Children(func() {
		ui.Icon(c, discoverIcon).FontSize(48 * s.gestureScale).Rotate(s.gestureRotation * 180 / math.Pi).TextColor(c.Theme().Accent)
	})
	ui.Textf(c, "Gesture scale: %.2f", s.gestureScale)
	ui.Textf(c, "Gesture rotation: %.2f", s.gestureRotation)
	ui.Textf(c, "Gesture updates: %d", s.gestureUpdates)
	if ui.Button(c, "Reset gestures").Height(44).Clicked() {
		s.gestureScale, s.gestureRotation, s.gestureUpdates = 1, 0, 0
	}
	for i := range 12 {
		ui.Textf(c, "Scroll target %02d", i+1).Height(40)
	}
}
func (s *demo) integrationControls(c *ui.Context) {
	ui.Text(c, "System Integration").FontSize(24).Bold()
	for _, style := range []mygo.StatusBarStyle{mygo.StatusBarAuto, mygo.StatusBarLight, mygo.StatusBarDark} {
		label := "Status bar auto"
		if style != "" {
			label = "Status bar " + string(style)
		}
		if ui.Button(c, label).Height(44).FillWidth().Clicked() {
			err := mygo.App.SetStatusBar(style, false)
			s.statusHidden = false
			if err != nil {
				s.integrationResult = err.Error()
			} else {
				s.integrationResult = label
			}
		}
	}
	if ui.Button(c, "Toggle status bar").Height(44).FillWidth().Clicked() {
		s.statusHidden = !s.statusHidden
		err := mygo.App.SetStatusBar(mygo.StatusBarAuto, s.statusHidden)
		if err != nil {
			s.integrationResult = err.Error()
		} else {
			s.integrationResult = fmt.Sprintf("Status bar hidden: %v", s.statusHidden)
		}
	}
	if ui.Button(c, "Open demo deep link").Height(44).FillWidth().Clicked() {
		go func() {
			err := mygo.Shell.OpenExternal("mygo-native://demo/app/feature?name=integration")
			s.window.Update(func() {
				if err != nil {
					s.integrationResult = err.Error()
				} else {
					s.integrationResult = "Deep link opened"
				}
			})
		}()
	}
	ui.Text(c, s.integrationResult)
	ui.Text(c, "Opened URL: "+s.opened).FontSize(12).LineHeight(1.4)
	document := "None"
	if s.lastDocument != "" {
		document = filepath.Base(s.lastDocument)
	}
	ui.Text(c, "Opened document: "+document).FontSize(14)
	ui.Text(c, "Universal Links use ios.associatedDomains, signed entitlements and a hosted apple-app-site-association file. File types come from fileAssociations.").FontSize(14).LineHeight(1.4)
}
