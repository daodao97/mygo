// ios-native exercises MyGo's native Go UI on iPhone and iPad.
package main

import (
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type savedDemo struct {
	Count    int
	Message  string
	Selected int
	Scroll   ui.ScrollState
}
type demo struct {
	savedDemo
	app                 appState
	status              string
	lifecycle           string
	firstFrame          bool
	opened              string
	appearance          string
	mobile              bool
	inputKind           string
	inputValue          string
	editingValue        string
	editingPassword     string
	editingResult       string
	compositionValue    string
	biometricResult     string
	biometricPending    bool
	biometricCancel     func()
	submissions         int
	systemResult        string
	secrets             *mygo.SecureStore
	window              *mygo.Window
	media               *mediaState
	pickerResult        string
	pickerPaths         []string
	notificationResult  string
	notificationVersion uint64
	notificationRoute   string
	hapticResult        string
	gestureScale        float32
	gestureRotation     float32
	gestureUpdates      int
	gestureValue        string
	integrationResult   string
	statusHidden        bool
	lastDocument        string
}

func (s *demo) view(c *ui.Context) {
	if !s.app.Diagnostics {
		s.appView(c)
		return
	}
	s.diagnosticsView(c)
}

func (s *demo) diagnosticsView(c *ui.Context) {
	if s.mobile {
		s.mobileView(c)
		return
	}
	switch s.appearance {
	case "dark":
		t := ui.DarkTheme()
		t.Background = ui.Hex("#14213d")
		c.SetTheme(t)
	case "light":
		t := ui.LightTheme()
		t.Background = ui.Hex("#eef4f8")
		c.SetTheme(t)
	}
	ui.Scroll(c).HideScrollbars().TrackScroll(&s.Scroll).Fill().Padding(20).Gap(16).Children(func() {
		ui.Text(c, "MyGo on iOS").FontSize(28).Bold()
		ui.Text(c, "Native Go UI · UIKit · Metal").TextColor(c.Theme().TextMuted)
		ui.Textf(c, "Count: %d", s.Count).FontSize(36).Bold()
		ui.Row(c).Gap(12).Children(func() {
			if ui.Button(c, "−").Label("Decrement").Height(48).Width(64).Clicked() {
				s.Count--
			}
			if ui.PrimaryButton(c, "+").Label("Increment").Height(48).Width(64).Clicked() {
				s.Count++
			}
		})
		ui.Text(c, "Message / text or emoji")
		ui.TextInput(c, &s.Message).Label("Message").Height(48).FillWidth()
		ui.Row(c).Gap(12).Children(func() {
			if ui.Button(c, "Save").Height(48).Clicked() {
				if err := mygo.App.SaveState(); err != nil {
					s.status = err.Error()
				} else {
					s.status = "Saved"
				}
			}
		})
		ui.Textf(c, "Message: %s", s.Message)
		ui.Text(c, s.status)
		ui.Textf(c, "Selected: %d", s.Selected)
		ui.Text(c, "Lifecycle: "+s.lifecycle)
		if s.firstFrame {
			ui.Text(c, "First frame presented")
		}
		if s.opened != "" {
			ui.Text(c, "Opened: "+s.opened)
		}
		if ui.Button(c, "Try close").Height(48).Clicked() {
			if err := s.window.TryClose(); err != nil {
				s.status = "Close unsupported"
			}
		}
		if ui.Button(c, "Try quit").Height(48).Clicked() {
			if err := mygo.App.TryQuit(); err != nil {
				s.status = "Quit unsupported"
			}
		}
		for i := 1; i <= 40; i++ {
			if ui.Button(c, fmt.Sprint("Row ", i)).Height(48).FillWidth().Clicked() {
				s.Selected = i
			}
		}
	})
}

func main() {
	s := &demo{savedDemo: savedDemo{Message: "Hello, MyGo 👋"}, inputKind: "email"}
	mygo.App.SetName("MyGo iOS")
	mygo.App.OnLifecycleChanged(func(state mygo.LifecycleState) {
		s.lifecycle = string(state)
		log.Printf("lifecycle: %s", state)
		if s.window != nil {
			s.window.Invalidate()
		}
	})
	mygo.App.OnStateSaveError(func(err error) { log.Printf("checkpoint failed: %v", err) })
	mygo.App.OnOpenURL(func(rawURL string) {
		s.opened = rawURL
		// A deterministic reset is useful for repeated UI tests, and exercises
		// cold-launch URL delivery after readiness and state restoration.
		if strings.HasSuffix(rawURL, "/reset") && !strings.Contains(rawURL, "/app/") {
			s.app.Diagnostics = true
			s.mobile = false
			s.savedDemo = savedDemo{Message: "Hello, MyGo 👋"}
			s.appearance = ""
		}
		if u, err := url.Parse(rawURL); err == nil {
			if u.Path == "/diagnostics" {
				s.app.Diagnostics, s.mobile = true, false
			}
			if u.Path == "/app/reset" {
				s.app = appState{}
				s.media = nil
				s.mobile = false
				s.appearance = ""
				s.inputKind = "email"
				s.inputValue, s.systemResult, s.submissions = "", "", 0
				s.initNavigation()
			}
			if u.Path == "/app/feature" {
				for _, f := range features {
					if f.path == u.Query().Get("name") {
						s.app.Diagnostics = false
						s.mobile = false
						s.initNavigation()
						s.app.Tab = 1
						s.app.Routers[1].Reset("/features")
						s.app.Routers[1].Push("/features/" + f.path)
						s.app.PageScroll = nil
						break
					}
				}
			}
			if u.Path == "/app/home" {
				s.app.Diagnostics, s.mobile = false, false
				s.app.Tab = 0
				s.initNavigation()
				s.app.Routers[0].Reset("/home")
				s.app.Scroll[0] = ui.ScrollState{}
			}
			if strings.HasPrefix(u.Path, "/mobile") {
				s.app.Diagnostics = true
				s.mobile = true
				s.inputKind = strings.TrimPrefix(strings.TrimPrefix(u.Path, "/mobile/"), "input/")
				s.inputValue, s.systemResult, s.submissions = "", "", 0
			}
			switch u.Path {
			case "/appearance/dark":
				s.appearance = "dark"
			case "/appearance/light":
				s.appearance = "light"
			case "/appearance/system":
				s.appearance = ""
			}
		}
		if s.window != nil {
			s.window.Invalidate()
		}
		log.Printf("opened URL: %s", rawURL)
	})
	mygo.App.OnNotification(func(e mygo.NotificationEvent) {
		s.notificationRoute = e.Data["route"]
		s.notificationResult = "Delivered: " + e.ID
		if e.Clicked {
			s.notificationResult = "Opened notification: " + e.ID
			s.app.Diagnostics = false
			s.mobile = false
			s.initNavigation()
			s.app.Tab = 1
			s.app.Routers[1].Reset("/features")
			s.app.Routers[1].Push("/features/notifications")
		}
		if s.window != nil {
			s.window.Invalidate()
		}
	})
	mygo.App.OnPushToken(func(token string) {
		s.notificationResult = "APNs token: " + token
		if s.window != nil {
			s.window.Invalidate()
		}
	})
	mygo.App.OnPushRegistrationError(func(err error) {
		s.notificationResult = "APNs registration: " + err.Error()
		if s.window != nil {
			s.window.Invalidate()
		}
	})
	mygo.App.OnOpenFile(func(path string) {
		s.lastDocument = path
		s.app.Diagnostics = false
		s.mobile = false
		s.initNavigation()
		s.app.Tab = 1
		s.app.Routers[1].Reset("/features")
		s.app.Routers[1].Push("/features/integration")
		if s.window != nil {
			s.window.Invalidate()
		}
	})
	mygo.App.WhenReady(func() {
		var err error
		s.secrets, err = mygo.NewSecureStore("mobile-tests", mygo.SecureStoreOptions{})
		if err != nil {
			log.Fatal(err)
		}
		if _, err := mygo.PersistState("demo", &s.savedDemo); err != nil {
			log.Fatal(err)
		}
		if _, err := mygo.PersistState("navigation", &s.app); err != nil {
			log.Fatal(err)
		}
		s.initNavigation()
		s.window = mygo.NewWindow(mygo.WindowOptions{Title: "MyGo iOS", Width: 400, Height: 780, Content: ui.View(s.view)})
		s.window.OnFirstFrame(func() { s.firstFrame = true; log.Printf("first frame presented"); s.window.Invalidate() })
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
