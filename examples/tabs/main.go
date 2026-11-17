// Tabs shows the navigation of phones: the system's own bar of tabs at the
// bottom of the screen (UIKit's tab bar, Material's bottom navigation), a
// window of native UI or a web page in each, and a stack of windows pushed
// over a tab's, under the system's navigation bar with its back button.
// On desktops, the tabs' windows are windows.
//
//	go run ./cmd/mygo dev -platform ios examples/tabs
//	go run ./cmd/mygo dev -platform android examples/tabs
package main

import (
	"fmt"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// app is the state the windows show.
type app struct {
	tabs     *mygo.Tabs
	home     *mygo.Window
	liked    map[int]bool
	settings struct{ notify, dark, wifi bool }
}

var fruits = []string{"Apple", "Banana", "Cherry", "Dragon fruit", "Elderberry", "Fig", "Grape", "Honeydew",
	"Kiwi", "Lemon", "Mango", "Nectarine", "Orange", "Papaya", "Quince", "Raspberry", "Strawberry", "Tangerine"}

// homeView lists the fruits; choosing one pushes its window.
func (a *app) homeView(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).Padding(16).Gap(8).Children(func() {
			ui.Text(c, "Choose a fruit: its page slides in over this one, with a back button.").TextColor(t.TextMuted)
			for i, name := range fruits {
				row := ui.Row(c.Key(i)).Padding(14, 16).Gap(12).AlignItems(ui.Center).
					Background(t.Surface).Radius(12).Label(name)
				row.Children(func() {
					ui.Text(c, name).Grow(1)
					if a.liked[i] {
						ui.Text(c, "♥").TextColor(t.Accent)
					}
					ui.Text(c, "›").TextColor(t.TextMuted).FontSize(20)
				})
				if row.Clicked() {
					a.open(i)
				}
			}
		})
	})
}

// open pushes the window of fruit i over the home tab's.
func (a *app) open(i int) {
	detail := mygo.NewWindow(mygo.WindowOptions{
		Title:  fruits[i],
		Hidden: true,
		Content: ui.View(func(c *ui.Context) {
			t := c.Theme()
			// The navigation bar shows the window's title.
			ui.Column(c).Fill().Padding(24).Gap(16).Children(func() {
				ui.Textf(c, "Fruit number %d of %d.", i+1, len(fruits)).TextColor(t.TextMuted)
				liked := a.liked[i]
				label := "Like"
				if liked {
					label = "Unlike"
				}
				if ui.PrimaryButton(c, label).Clicked() {
					a.liked[i] = !liked
					a.home.Update(func() {}) // the list shows the heart
					a.updateBadge()
				}
			})
		}),
	})
	a.home.Push(detail)
}

// updateBadge counts the fruits liked on the home tab.
func (a *app) updateBadge() {
	n := 0
	for _, v := range a.liked {
		if v {
			n++
		}
	}
	badge := ""
	if n > 0 {
		badge = fmt.Sprint(n)
	}
	a.tabs.SetBadge(0, badge)
}

func (a *app) settingsView(c *ui.Context) {
	t := c.Theme()
	setting := func(key, label string, on *bool) {
		ui.Row(c.Key(key)).Padding(12, 16).AlignItems(ui.Center).Background(t.Surface).Radius(12).Children(func() {
			ui.Text(c, label).Grow(1)
			ui.Switch(c, on).Label(label)
		})
	}
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).Padding(16).Gap(8).Children(func() {
			setting("notify", "Notifications", &a.settings.notify)
			setting("wifi", "Wi-Fi", &a.settings.wifi)
			setting("dark", "Dark appearance", &a.settings.dark)
			if a.settings.dark {
				mygo.Theme.SetSource(mygo.ThemeDark)
			} else {
				mygo.Theme.SetSource(mygo.ThemeSystem)
			}
		})
	})
}

// Greeter is callable from the web tab's page.
type Greeter struct{}

func (Greeter) Greet(name string) string { return "Hello, " + name + ", from Go!" }

const page = `<!doctype html>
<html>
<head>
  <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
  <style>
    :root { color-scheme: light dark; font-family: system-ui, sans-serif; }
    body { margin: 0; padding: 24px; padding-top: max(24px, env(safe-area-inset-top)); }
    button { font: inherit; padding: 10px 16px; border-radius: 10px; border: 0; background: #0a84ff; color: white; }
  </style>
</head>
<body>
  <h1>A web page</h1>
  <p>This tab shows a page in the system's web view; it calls Go:</p>
  <button onclick="mygo.call('Greeter.Greet', 'tabs').then(t => document.getElementById('out').textContent = t)">Greet</button>
  <p id="out"></p>
</body>
</html>`

func main() {
	a := &app{liked: map[int]bool{}}
	mygo.Bind(Greeter{})
	mygo.App.WhenReady(func() {
		a.home = mygo.NewWindow(mygo.WindowOptions{Title: "Fruits", Hidden: true, Content: ui.View(a.homeView)})
		web := mygo.NewWindow(mygo.WindowOptions{Title: "Web", Hidden: true})
		web.Page().LoadHTML(page, "")
		settings := mygo.NewWindow(mygo.WindowOptions{Title: "Settings", Hidden: true, Content: ui.View(a.settingsView)})
		var err error
		a.tabs, err = mygo.NewTabs(
			mygo.TabItem{Title: "Fruits", Symbol: "leaf", Icon: homeIcon, Window: a.home},
			mygo.TabItem{Title: "Web", Symbol: "globe", Icon: globeIcon, Window: web},
			mygo.TabItem{Title: "Settings", Symbol: "gearshape", Icon: gearIcon, Window: settings},
		)
		if err != nil {
			log.Fatal(err)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
