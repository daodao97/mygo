package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Routers own navigation and serialize their history directly. The JSON
// field name History also accepts checkpoints from the earlier demo.
type appState struct {
	Tab             int
	Routers         [3]*ui.Router `json:"History"`
	Scroll          [3]ui.ScrollState
	PageScroll      map[string]*ui.ScrollState
	NavigationValue string
	SavedValue      string
	// Full-screen diagnostic entry is transient; ordinary launches restore
	// the feature app and its bottom tabs, even after device regression tests.
	Diagnostics bool `json:"-"`
}

type feature struct{ path, title, summary string }

var features = []feature{
	{"text", "Text & Scrolling", "Chinese text, emoji, long lists and scroll position"},
	{"lifecycle", "Lifecycle & Launch", "Launch, background restoration and close behavior"},
	{"input", "Keyboard & Input", "Keyboard types, submission and text input"},
	{"dialogs", "Dialogs & Sharing", "Native alerts, action sheets, sharing and clipboard"},
	{"permissions", "Permissions & Settings", "Check permissions, request access and open Settings"},
	{"secrets", "Keychain Storage", "Binary storage, updates, deletion and cold restoration"},
	{"biometrics", "Biometric Authentication", "Face ID, Touch ID, availability, cancellation and passcode fallback"},
	{"appearance", "Appearance & Accessibility", "Text size, Reduce Motion and system contrast"},
	{"navigation", "Navigation & State", "Nested pages, swipe back, drafts and history restoration"},
	{"images", "Images", "Formats, image fitting, transparency and thumbnails"},
	{"links", "Links & Rich Text", "Inline links, route navigation and styled paragraphs"},
	{"rendering", "Rendering", "Gradients, shadows, opacity, clipping and drawing"},
	{"editing", "Text Editing", "System selection handles, cut, copy, paste and password protection"},
	{"notifications", "Notifications", "Schedule, cancel, badges, foreground delivery and notification launch"},
	{"haptics", "Haptic Feedback", "Selection, impacts and success, warning or error feedback"},
	{"gestures", "Input & Gestures", "Interactive keyboard dismissal, read-only selection, pinch and rotation"},
	{"integration", "System Integration", "Status bar, document opening and deep-link entry"},
	{"files", "Files & Photos", "Import files and photos, export documents and clean up copies"},
}

func appIcon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	homeIcon     = appIcon(`<path d="m3 10 9-7 9 7v10H3z"/><path d="M9 20v-7h6v7"/>`)
	discoverIcon = appIcon(`<circle cx="12" cy="12" r="9"/><path d="m16 8-3 5-5 3 3-5z"/>`)
	personIcon   = appIcon(`<path d="M4 18v-5M10 18V6M16 18V9M22 18V3"/>`)
	backIcon     = appIcon(`<path d="m14 5-7 7 7 7"/>`)
	forwardIcon  = appIcon(`<path d="m9 5 7 7-7 7"/>`)
)
var tabNames = [3]string{"Overview", "Features", "Status"}
var tabRoots = [3]string{"/home", "/features", "/status"}

func (s *demo) initNavigation() {
	if s.app.Tab < 0 || s.app.Tab > 2 {
		s.app.Tab = 0
	}
	for i, root := range tabRoots {
		if s.app.Routers[i] == nil {
			s.app.Routers[i] = ui.NewRouter(root)
		}
	}
}
func (s *demo) pageScroll(path string) *ui.ScrollState {
	if s.app.PageScroll == nil {
		s.app.PageScroll = make(map[string]*ui.ScrollState)
	}
	if s.app.PageScroll[path] == nil {
		s.app.PageScroll[path] = &ui.ScrollState{}
	}
	return s.app.PageScroll[path]
}
func (s *demo) pageTitle(p string) string {
	if strings.HasSuffix(p, "/images/grid") {
		return "Thumbnail Grid"
	}
	if strings.HasSuffix(p, "/links/detail") {
		return "Linked Detail"
	}
	if strings.HasSuffix(p, "/navigation/detail") {
		return "Detail"
	}
	if strings.HasSuffix(p, "/lab") {
		return "Interaction Demo"
	}
	for _, f := range features {
		if strings.HasSuffix(p, "/"+f.path) {
			return f.title
		}
	}
	return tabNames[s.app.Tab]
}

func (s *demo) appView(c *ui.Context) {
	if s.app.Routers[0] == nil {
		s.initNavigation()
	}
	t := ui.LightTheme()
	t.Background, t.Accent = ui.Hex("#f6f7f9"), ui.Hex("#315cbd")
	t.Text, t.TextMuted = ui.Hex("#172238"), ui.Hex("#758096")
	if s.appearance == "dark" {
		t = ui.DarkTheme()
		t.Background = ui.Hex("#14213d")
	}
	c.SetTheme(t)
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		r := s.app.Routers[s.app.Tab]
		ui.Row(c).Height(54).Shrink(0).Padding(0, 12).AlignItems(ui.Center).Background(t.Background).Children(func() {
			if r.CanGoBack() {
				b := ui.ButtonBase(c).Label("Back").Width(64).Height(44).Gap(3).TextColor(t.Accent)
				b.Children(func() { ui.Icon(c, backIcon).FontSize(18); ui.Text(c, "Back").FontSize(14) })
				if b.Clicked() {
					s.app.Routers[s.app.Tab].Pop()
				}
			} else {
				ui.Box(c).Width(64).Height(44)
			}
			ui.Text(c, s.pageTitle(r.Path())).Grow(1).TextAlign(ui.Center).FontSize(17).Bold()
			ui.Box(c).Width(64).Height(44)
		})
		// The keyed container keeps each tab's element IDs independent.
		ui.Column(c.Key(tabRoots[s.app.Tab])).Grow(1).MinHeight(0).Children(func() {
			r.View(c, func(page *ui.Route) {
				p := page.Path()
				page.Title(s.pageTitle(p))
				switch p {
				case "/home":
					s.homeView(c)
				case "/features":
					s.featuresView(c)
				case "/status":
					s.statusView(c)
				default:
					s.featureView(c, p)
				}

			})
		})
		tabs := ui.TabsBase(c, &s.app.Tab, 3)
		tabs.List.Height(66).FillWidth().Background(t.Background).BorderWidth(1, 0, 0, 0).BorderColor(ui.Hex("#e6e9ee")).Children(func() {
			for i, icon := range []*ui.SVG{homeIcon, discoverIcon, personIcon} {
				color := t.TextMuted
				if i == s.app.Tab {
					color = t.Accent
				}
				tab := tabs.Tab(i).Label(tabNames[i]).Grow(1).Height(66).Center().TextColor(color)
				tab.Children(func() {
					ui.Column(c).Center().Gap(4).Children(func() { ui.Icon(c, icon).FontSize(23); ui.Text(c, tabNames[i]).FontSize(11) })
				})
			}
		})
	})
}
func appCard(c *ui.Context, fn func()) {
	ui.Column(c).FillWidth().Padding(20).Gap(14).Radius(20).Background(c.Theme().Surface).Children(fn)
}
func (s *demo) featureLink(c *ui.Context, f feature) {
	b := ui.ButtonBase(c).Label(f.title).FillWidth().Padding(18).Radius(18).Background(c.Theme().Surface).Gap(12)
	b.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(7).Children(func() {
			ui.Text(c, f.title).FontSize(16).Bold()
			ui.Text(c, f.summary).FontSize(12).LineHeight(1.4).TextColor(c.Theme().TextMuted)
		})
		ui.Icon(c, forwardIcon).FontSize(16).TextColor(c.Theme().TextMuted)
	})
	if b.Clicked() {
		s.app.Routers[s.app.Tab].Push(tabRoots[s.app.Tab] + "/" + f.path)
	}
}
func (s *demo) homeView(c *ui.Context) {
	ui.Scroll(c).HideScrollbars().TrackScroll(&s.app.Scroll[0]).Fill().Padding(20).Gap(18).Children(func() {
		ui.Text(c, "MyGo Mobile").FontSize(28).Bold()
		ui.Text(c, "Explore a feature and try its native integration.").FontSize(14).LineHeight(1.5).TextColor(c.Theme().TextMuted)
		appCard(c, func() {
			ui.Text(c, "Runtime").FontSize(18).Bold()
			ui.Text(c, "Scene: "+s.lifecycle)
			ui.Textf(c, "First frame presented: %v", s.firstFrame)
			ui.Text(c, "Native Go UI · UIKit · Metal").FontSize(13).TextColor(c.Theme().TextMuted)
		})
		ui.Text(c, "Quick Start").FontSize(18).Bold()
		for _, path := range []string{"navigation", "input", "lifecycle"} {
			for _, f := range features {
				if f.path == path {
					s.featureLink(c, f)
					break
				}
			}
		}
		if ui.PrimaryButton(c, "View All Features").FillWidth().Height(46).Clicked() {
			// This is a catalog destination, while the bottom tab resumes
			// its remembered page. Start the catalog at the top every time.
			s.app.Routers[1].Reset(tabRoots[1])
			s.app.Scroll[1] = ui.ScrollState{}
			s.app.Tab = 1
		}
	})
}
func (s *demo) featuresView(c *ui.Context) {
	ui.Scroll(c).HideScrollbars().TrackScroll(&s.app.Scroll[1]).Fill().Padding(20).Gap(14).Children(func() {
		ui.Text(c, "Feature Demos").FontSize(24).Bold()
		ui.Text(c, "Try each integration and see the result here.").FontSize(14).TextColor(c.Theme().TextMuted)
		for _, f := range features {
			s.featureLink(c, f)
		}
	})
}
func (s *demo) featureView(c *ui.Context, p string) {
	switch {
	case strings.HasSuffix(p, "/images/grid"):
		s.thumbnailView(c)
	case strings.HasSuffix(p, "/images"):
		s.imagesView(c, p)
	case strings.HasSuffix(p, "/links/detail"):
		s.linkDetailView(c, p)
	case strings.HasSuffix(p, "/links"):
		s.linksView(c, p)
	case strings.HasSuffix(p, "/rendering"):
		s.renderingView(c, p)
	case strings.HasSuffix(p, "/secrets"):
		s.secretView(c)
	case strings.HasSuffix(p, "/lab"):
		s.diagnosticsView(c)
	case strings.HasSuffix(p, "/navigation/detail"):
		s.navigationDetail(c, p)
	default:
		ui.Scroll(c.Key(p)).HideScrollbars().TrackScroll(s.pageScroll(p)).Fill().Padding(20).Gap(16).Children(func() {
			switch {
			case strings.HasSuffix(p, "/text"):
				ui.Text(c, "Chinese Text").FontSize(24).Bold()
				ui.Text(c, "Scroll the list, navigate away and back, then check the restored position.").FontSize(16).LineHeight(1.6)
				ui.Text(c, "你好，MyGo 👋。中文排版与长列表。中英文混排：Go + UIKit + Metal。").FontSize(16).LineHeight(1.6)
				for i := 1; i <= 40; i++ {
					if ui.Button(c, fmt.Sprintf("中文测试项 %02d · 输入与滚动 👋", i)).FillWidth().Height(48).Clicked() {
						s.Selected = i
					}
				}
				ui.Textf(c, "Selected row: %d", s.Selected)
			case strings.HasSuffix(p, "/lifecycle"):
				ui.Text(c, "Background or relaunch the app to check saving and restoration.").FontSize(16).LineHeight(1.5)
				s.lifecycleControls(c)
			case strings.HasSuffix(p, "/input"):
				ui.Text(c, "Choose a keyboard, then type and submit.").FontSize(15).LineHeight(1.5)
				ui.Row(c).Wrap().Gap(8).Children(func() {
					for _, kind := range []string{"email", "number", "decimal", "phone", "url", "code"} {
						if ui.Button(c, kind).Height(38).Clicked() {
							s.inputKind = kind
						}
					}
				})
				s.inputControls(c)
			case strings.HasSuffix(p, "/biometrics"):
				s.biometricControls(c)
			case strings.HasSuffix(p, "/notifications"):
				s.notificationControls(c)
			case strings.HasSuffix(p, "/haptics"):
				s.hapticControls(c)
			case strings.HasSuffix(p, "/gestures"):
				s.gestureControls(c)
			case strings.HasSuffix(p, "/integration"):
				s.integrationControls(c)
			case strings.HasSuffix(p, "/files"):
				s.pickerControls(c)
			case strings.HasSuffix(p, "/editing"):
				s.editingControls(c)
			case strings.HasSuffix(p, "/dialogs"):
				s.dialogControls(c)
			case strings.HasSuffix(p, "/permissions"):
				s.permissionControls(c)
				ui.Text(c, s.systemResult)
			case strings.HasSuffix(p, "/appearance"):
				ui.Row(c).Gap(12).Children(func() {
					if ui.Button(c, "Light").Height(44).Clicked() {
						s.appearance = "light"
					}
					if ui.Button(c, "Dark").Height(44).Clicked() {
						s.appearance = "dark"
					}
				})
				ui.Text(c, "Change text size, Reduce Motion or contrast in Settings, then return to see the preferences.").FontSize(16).LineHeight(1.6)
				prefs := c.Preferences()
				ui.Textf(c, "Text scale: %.2f", prefs.TextScale)
				ui.Textf(c, "Reduce Motion: %v", prefs.ReduceMotion)
				ui.Textf(c, "High contrast: %v", prefs.HighContrast)
				ui.Text(c, "Text size preview 👋").FontSize(20)
			case strings.HasSuffix(p, "/navigation"):
				ui.Text(c, "Router Demo").FontSize(24).Bold()
				ui.Text(c, "Edit text on the detail page. Tabs retain their pages. Swipe from the left edge to go back; a short swipe cancels.").FontSize(16).LineHeight(1.6)
				if ui.PrimaryButton(c, "Open Detail").FillWidth().Height(48).Clicked() {
					s.app.Routers[s.app.Tab].Push(p + "/detail")
				}
				ui.Text(c, "Saved State").FontSize(18).Bold()
				if s.app.SavedValue == "" {
					ui.Text(c, "Nothing saved yet")
				} else {
					ui.Text(c, s.app.SavedValue)
				}
			}
		})
	}
}
func (s *demo) navigationDetail(c *ui.Context, p string) {
	ui.Scroll(c).HideScrollbars().TrackScroll(s.pageScroll(p)).Fill().Padding(20).Gap(18).Children(func() {
		ui.Text(c, "State Editor").FontSize(22).Bold()
		ui.Text(c, "Drafts survive navigation. Cancel a back swipe to keep focus. Background the app to save.").FontSize(15).LineHeight(1.5)
		ui.TextArea(c, &s.app.NavigationValue).HideScrollbars().Label("State input").InputOptions(ui.InputOptions{Return: ui.ReturnDefault}).MinHeight(150).FillWidth()
		if ui.PrimaryButton(c, "Save State").FillWidth().Height(48).Clicked() {
			s.app.SavedValue = s.app.NavigationValue
			s.app.Routers[s.app.Tab].Pop()
			if err := mygo.App.SaveState(); err != nil {
				s.status = err.Error()
			} else {
				s.status = "Saved"
			}
		}
	})
}
func (s *demo) lifecycleControls(c *ui.Context) {
	ui.Text(c, "Scene: "+s.lifecycle)
	ui.Textf(c, "First frame presented: %v", s.firstFrame)
	ui.Textf(c, "Count: %d", s.Count).FontSize(20)
	ui.Row(c).Gap(10).Children(func() {
		if ui.Button(c, "Decrement").Height(44).Clicked() {
			s.Count--
		}
		if ui.PrimaryButton(c, "Increment").Height(44).Clicked() {
			s.Count++
		}
	})
	ui.TextInput(c, &s.Message).Label("Restoration input").Height(48).FillWidth()
	if ui.Button(c, "Save Checkpoint").Height(44).Clicked() {
		if err := mygo.App.SaveState(); err != nil {
			s.status = err.Error()
		} else {
			s.status = "Saved"
		}
	}
	ui.Text(c, s.status)
	ui.Text(c, "Opened: "+s.opened).FontSize(12).TextColor(c.Theme().TextMuted)
	ui.Row(c).Gap(10).Children(func() {
		if ui.Button(c, "Try close").Height(44).Clicked() {
			if err := s.window.TryClose(); err != nil {
				s.status = err.Error()
			}
		}
		if ui.Button(c, "Try quit").Height(44).Clicked() {
			if err := mygo.App.TryQuit(); err != nil {
				s.status = err.Error()
			}
		}
	})
}
func (s *demo) statusView(c *ui.Context) {
	ui.Scroll(c).HideScrollbars().TrackScroll(&s.app.Scroll[2]).Fill().Padding(20).Gap(18).Children(func() {
		ui.Text(c, "Runtime Status").FontSize(24).Bold()
		s.lifecycleControls(c)
		ui.Text(c, "Interactions").FontSize(18).Bold()
		if ui.Button(c, "Open Interaction Demo").FillWidth().Height(46).Clicked() {
			s.mobile = false
			s.app.Routers[s.app.Tab].Push("/status/lab")
		}
		ui.Text(c, "The interaction demo covers counters, input, scrolling and restoration. Find system integrations in Features.").FontSize(14).LineHeight(1.5).TextColor(c.Theme().TextMuted)
	})
}
