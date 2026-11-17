// Mobile shows what phones and tablets ask of native UI: gestures, sheets,
// wheels, swiped pages and rows, the keyboards of text fields, and what the
// system offers apps. Package ui/mobile builds most of it.
//
//	go run ./cmd/mygo dev -platform ios examples/mobile
//	go run ./cmd/mygo dev -platform android examples/mobile
package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/plugins/background"
	"github.com/egoist/mygo/plugins/biometrics"
	"github.com/egoist/mygo/plugins/camera"
	"github.com/egoist/mygo/plugins/imagepicker"
	"github.com/egoist/mygo/plugins/location"
	"github.com/egoist/mygo/plugins/maps"
	"github.com/egoist/mygo/plugins/permissionflow"
	"github.com/egoist/mygo/plugins/preferences"
	"github.com/egoist/mygo/plugins/sensors"
	"github.com/egoist/mygo/plugins/sqlite"
	"github.com/egoist/mygo/plugins/videoplayer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/mobile"
	"github.com/egoist/mygo/ui/netimage"
)

type app struct {
	win *mygo.Window
	tab int

	// Widgets.
	sheet, actions, drawer, picker, datePicker bool
	dark, wifi                                 bool
	size                                       int
	when                                       time.Time
	picked                                     string

	// Lists.
	refresh ui.RefreshState
	rows    []string
	nextRow int

	// Pages.
	video    *videoplayer.Player
	mapOn    bool
	world    *maps.Map
	videoNow videoplayer.State
	web      *mygo.WebView
	overPage int
	pages    mobile.PageState
	zoom     mobile.ZoomState

	// Keyboards.
	email, password, code, phone, url, amount, search, notes string
	searched                                                 string

	// System.
	link    string
	bars    bool
	status  string
	results map[string]string
	cam     *camera.Camera
	camOn   bool
}

func heading(c *ui.Context, s string) {
	ui.Text(c, s).FontSize(13).TextColor(c.Theme().TextMuted).PaddingX(16).Padding(16, 16, 4, 16)
}

func (a *app) widgets(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).Padding(8, 0, 24, 0).Gap(16).Children(func() {
			mobile.Section(c, "Presented", func() {
				if mobile.ListRow(c.Key(1), mobile.RowOptions{Title: "Sheet", Subtitle: "Two detents; drag it", Disclosure: true}).Clicked() {
					a.sheet = true
				}
				if mobile.ListRow(c.Key(2), mobile.RowOptions{Title: "Action sheet", Disclosure: true}).Clicked() {
					a.actions = true
				}
				if mobile.ListRow(c.Key(3), mobile.RowOptions{Title: "Drawer", Disclosure: true}).Clicked() {
					a.drawer = true
				}
				if mobile.ListRow(c.Key(4), mobile.RowOptions{Title: "Wheel", Subtitle: a.pickedSize(), Disclosure: true}).Clicked() {
					a.picker = true
				}
				if mobile.ListRow(c.Key(5), mobile.RowOptions{Title: "Date", Subtitle: a.when.Format("2 Jan 2006, 15:04"), Disclosure: true}).Clicked() {
					a.datePicker = true
				}
			})
			mobile.Section(c, "Controls", func() {
				mobile.ListRow(c.Key(6), mobile.RowOptions{Title: "Wi-Fi", Trailing: func() { mobile.Switch(c, &a.wifi) }})
				mobile.ListRow(c.Key(7), mobile.RowOptions{Title: "Dark", Trailing: func() { mobile.Switch(c, &a.dark) }})
			})
			if a.picked != "" {
				ui.Textf(c, "Chose %s", a.picked).TextColor(t.TextMuted).PaddingX(16)
			}
		})
	})
	mobile.Sheet(c, &a.sheet, mobile.SheetOptions{Detents: []float32{0.45, 0.9}, Label: "Sheet"}, func() {
		ui.Scroll(c).Grow(1).Basis(0).Children(func() {
			ui.Column(c).Padding(8, 16).Gap(12).Children(func() {
				ui.Text(c, "A sheet").FontSize(22).Bold()
				ui.Text(c, "Drag the grabber between its two heights, or down and away. What it holds scrolls.").TextColor(t.TextMuted)
				for i := range 30 {
					ui.Textf(c.Key(i), "Row %d", i+1)
				}
			})
		})
	})
	choose := func(s string) func() { return func() { a.picked = s; c.Root() } }
	mobile.ActionSheet(c, &a.actions, mobile.ActionSheetOptions{Title: "Photo", Message: "What should happen to it?"},
		mobile.Action{Label: "Share", OnTap: choose("Share")},
		mobile.Action{Label: "Duplicate", OnTap: choose("Duplicate")},
		mobile.Action{Label: "Delete", Destructive: true, OnTap: choose("Delete")},
	)
	mobile.Drawer(c, &a.drawer, mobile.DrawerOptions{}, func() {
		ui.Column(c).Padding(16).Gap(12).Children(func() {
			ui.Text(c, "Navigation").FontSize(22).Bold()
			for _, s := range []string{"Inbox", "Sent", "Drafts", "Trash"} {
				mobile.ListRow(c.Key(s), mobile.RowOptions{Title: s})
			}
		})
	})
	mobile.Sheet(c, &a.picker, mobile.SheetOptions{Detents: []float32{0.38}, Label: "Size"}, func() {
		ui.Column(c).Padding(8, 16).Children(func() { mobile.Wheel(c, &a.size, sizes) })
	})
	mobile.Sheet(c, &a.datePicker, mobile.SheetOptions{Detents: []float32{0.6}, Label: "Date"}, func() {
		ui.Column(c).Padding(8, 16).Gap(8).Children(func() {
			mobile.DatePicker(c, &a.when)
			mobile.TimePicker(c, &a.when)
		})
	})
}

var sizes = []string{"Extra small", "Small", "Medium", "Large", "Extra large", "Huge"}

func (a *app) pickedSize() string { return sizes[a.size] }

func (a *app) lists(c *ui.Context) {
	t := c.Theme()
	l := ui.Scroll(c).Fill()
	if l.PullToRefresh(&a.refresh) {
		go func() {
			time.Sleep(1200 * time.Millisecond)
			a.win.Update(func() {
				a.nextRow++
				a.rows = append([]string{fmt.Sprintf("New row %d", a.nextRow)}, a.rows...)
				a.refresh.Refreshing = false
			})
		}()
	}
	l.Children(func() {
		ui.Column(c).Children(func() {
			ui.Text(c, "Pull down to refresh. Swipe a row aside. Hold one for its menu.").TextColor(t.TextMuted).Padding(12, 16)
			ui.ScrollHorizontal(c).FillWidth().Gap(8).PaddingX(16).Children(func() {
				for i := range 24 {
					url := fmt.Sprintf("https://picsum.photos/seed/mygo%d/240", i)
					netimage.Image(c.Key(i), url, netimage.Options{MaxSide: 240}).Size(80, 80).Radius(8).Shrink(0)
				}
			})
			for i, r := range a.rows {
				i, r := i, r
				mobile.SwipeRow(c.Key(r), nil, mobile.SwipeOptions{
					Leading: []mobile.SwipeAction{{Label: "Pin"}},
					Trailing: []mobile.SwipeAction{{Label: "Delete", Destructive: true, OnTap: func() {
						a.rows = append(a.rows[:i:i], a.rows[i+1:]...)
					}}},
				}, func() {
					row := mobile.ListRow(c, mobile.RowOptions{Title: r, Subtitle: "Swipe me"})
					row.ContextMenu(func(m *ui.Menu) {
						m.Item("Copy")
						if m.Item("Delete").Chosen() {
							a.rows = append(a.rows[:i:i], a.rows[i+1:]...)
						}
					})
				})
				ui.Divider(c)
			}
		})
	})
}

const webPage = `<!doctype html><meta name=viewport content="width=device-width,initial-scale=1">
<style>body{font:16px system-ui;margin:12px;background:#fef3c7}button{font-size:16px;padding:8px 12px}</style>
<h3 style="margin:0 0 8px">A web page</h3><button onclick="this.textContent='Tapped '+(++n)">Tap the page</button>
<input placeholder="Type here" style="font-size:16px;margin-left:8px;width:110px"><script>var n=0</script>`

var pageColors = []ui.Color{ui.Hex("#2563eb"), ui.Hex("#16a34a"), ui.Hex("#d97706"), ui.Hex("#db2777")}

func (a *app) pagesView(c *ui.Context) {
	ui.Column(c).Fill().Children(func() {
		if a.web == nil {
			v, err := a.win.NewWebView(mygo.WebViewOptions{})
			if err == nil {
				v.Page().LoadHTML(webPage, "")
				a.web = v
			}
		}
		if a.web != nil {
			// A web page among native UI, and native UI over it.
			ui.Box(c).Height(130).Shrink(0).FillWidth().Children(func() {
				ui.WebView(c, a.web).Fill()
				b := ui.Text(c, fmt.Sprintf("Native over the page: %d", a.overPage)).Absolute().Right(8).Bottom(8).Padding(6, 10).Radius(8).
					Background(c.Theme().Accent).TextColor(c.Theme().AccentText).FontSize(13)
				if b.Clicked() {
					a.overPage++
				}
			})
		}
		mobile.PageView(c, &a.pages, len(pageColors), func(i int) {
			ui.Box(c).Fill().Background(pageColors[i]).Center().Children(func() {
				ui.Textf(c, "Page %d", i+1).FontSize(32).Bold().TextColor(ui.RGB(255, 255, 255))
			})
		}).Height(110).Shrink(0)
		mobile.PageDots(c, a.pages.Page, len(pageColors))
		a.videoView(c)
		heading(c, "Pinch to zoom, double tap, drag")
		mobile.Zoomable(c, &a.zoom, mobile.ZoomOptions{}, func() {
			ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
				const n = 8
				cw, ch := r.W/n, r.H/n
				for y := range n {
					for x := range n {
						col := ui.Hex("#dbeafe")
						if (x+y)%2 == 0 {
							col = ui.Hex("#1e3a8a")
						}
						p.Fill(ui.Rect{X: r.X + float32(x)*cw, Y: r.Y + float32(y)*ch, W: cw, H: ch}, col, 0)
					}
				}
			})
		}).FillWidth().Grow(1).Basis(0)
	})
}

const videoURL = "https://devstreaming-cdn.apple.com/videos/streaming/examples/bipbop_4x3/bipbop_4x3_variant.m3u8"

// videoView plays a video in the view, with native UI's controls over it.
func (a *app) videoView(c *ui.Context) {
	if a.video == nil {
		p, err := videoplayer.New(a.win, videoURL, videoplayer.Options{AutoPlay: true, Muted: true})
		if err != nil {
			ui.Text(c, err.Error()).PaddingX(16)
			return
		}
		p.OnState(func(s videoplayer.State) { a.win.Update(func() { a.videoNow = s }) })
		a.video = p
	}
	s := a.videoNow
	ui.Box(c).Height(120).Shrink(0).FillWidth().Background(ui.RGB(0, 0, 0)).Children(func() {
		ui.WebView(c, a.video).Fill()
		label := "Pause"
		if !s.Playing {
			label = "Play"
		}
		b := ui.Text(c, fmt.Sprintf("%s %v / %v", label, s.Position.Truncate(time.Second), s.Duration.Truncate(time.Second))).Absolute().Left(8).Bottom(8).Padding(4, 8).Radius(6).
			Background(ui.RGBA(0, 0, 0, 0.6)).TextColor(ui.RGB(255, 255, 255)).FontSize(13)
		if b.Clicked() {
			if s.Playing {
				a.video.Pause()
			} else {
				a.video.Play()
			}
		}
	})
}

func field(c *ui.Context, label string, value *string, style func(ui.Element) ui.Element) ui.Element {
	t := c.Theme()
	var in ui.Element
	ui.Column(c).Gap(4).Children(func() {
		ui.Text(c, label).FontSize(13).TextColor(t.TextMuted)
		in = style(ui.TextInput(c, value).Label(label))
	})
	return in
}

func (a *app) keyboards(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).Padding(16).Gap(12).Children(func() {
			field(c, "Email", &a.email, func(e ui.Element) ui.Element {
				return e.Keyboard(ui.KeyboardEmail).Autofill(ui.AutofillEmail).ReturnKey(ui.ReturnNext).Capitalize(ui.CapitalizeNone).Autocorrect(false)
			})
			field(c, "Password", &a.password, func(e ui.Element) ui.Element {
				return e.Password().Autofill(ui.AutofillPassword).ReturnKey(ui.ReturnNext)
			})
			field(c, "One-time code", &a.code, func(e ui.Element) ui.Element {
				return e.Keyboard(ui.KeyboardNumber).Autofill(ui.AutofillOneTimeCode)
			})
			field(c, "Phone", &a.phone, func(e ui.Element) ui.Element { return e.Keyboard(ui.KeyboardPhone).Autofill(ui.AutofillPhone) })
			field(c, "Website", &a.url, func(e ui.Element) ui.Element {
				return e.Keyboard(ui.KeyboardURL).ReturnKey(ui.ReturnGo).Autocorrect(false).Capitalize(ui.CapitalizeNone)
			})
			field(c, "Amount", &a.amount, func(e ui.Element) ui.Element { return e.Keyboard(ui.KeyboardDecimal) })
			s := field(c, "Search", &a.search, func(e ui.Element) ui.Element { return e.Keyboard(ui.KeyboardSearch).ReturnKey(ui.ReturnSearch) })
			if s.Submitted() {
				a.searched = a.search
			}
			if a.searched != "" {
				ui.Textf(c, "Searched for %s", a.searched).TextColor(t.TextMuted)
			}
			ui.Text(c, "Notes").FontSize(13).TextColor(t.TextMuted)
			ui.TextArea(c, &a.notes).Label("Notes").Capitalize(ui.CapitalizeSentences).Height(120)
		})
	})
}

func (a *app) system(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).Padding(8, 0, 24, 0).Gap(16).Children(func() {
			mobile.Section(c, "Feel", func() {
				for _, h := range []struct {
					name string
					play func()
				}{{"Selection", mygo.Haptics.Selection}, {"Light", mygo.Haptics.Light}, {"Heavy", mygo.Haptics.Heavy}, {"Success", mygo.Haptics.Success}, {"Error", mygo.Haptics.Error}} {
					if mobile.ListRow(c.Key(h.name), mobile.RowOptions{Title: h.name}).Clicked() {
						h.play()
					}
				}
			})
			mobile.Section(c, "The system", func() {
				if mobile.ListRow(c.Key("share"), mobile.RowOptions{Title: "Share", Subtitle: "Text and a link", Disclosure: true}).Clicked() {
					go func() {
						if err := mygo.Share(mygo.ShareOptions{Text: "Made with MyGo", URL: "https://github.com/egoist/mygo"}); err != nil {
							log.Print(err)
						}
					}()
				}
				row := mobile.ListRow(c.Key("bars"), mobile.RowOptions{Title: "Hide the system's bars", Trailing: func() { mobile.Switch(c.Key("bs"), &a.bars) }})
				_ = row
				if mobile.ListRow(c.Key("window"), mobile.RowOptions{Title: "Another window", Subtitle: "Over this one", Disclosure: true}).Clicked() {
					var w *mygo.Window
					w = mygo.NewWindow(mygo.WindowOptions{Title: "Another window", OwnScene: true, Content: ui.View(func(c *ui.Context) {
						ui.Column(c).Fill().Background(c.Theme().Accent).Padding(80, 24, 24, 24).Gap(16).Children(func() {
							ui.Text(c, "A window of its own").FontSize(24).TextColor(ui.RGB(255, 255, 255))
							if ui.Button(c, "Close").Clicked() {
								w.Close()
							}
						})
					})})
					w.Show()
				}
				if mobile.ListRow(c.Key("land"), mobile.RowOptions{Title: "Hold on its side"}).Clicked() {
					mygo.SetOrientations(mygo.OrientationLandscape)
				}
				if mobile.ListRow(c.Key("any"), mobile.RowOptions{Title: "Turn freely"}).Clicked() {
					mygo.SetOrientations(mygo.OrientationAny)
				}
				if a.link != "" {
					mobile.ListRow(c.Key("link"), mobile.RowOptions{Title: "Opened with", Subtitle: a.link})
				}
				if mobile.ListRow(c.Key("status"), mobile.RowOptions{Title: "Network and battery", Subtitle: a.status}).Clicked() {
					go func() {
						n, b := mygo.Network.Status(), mygo.Power.Battery()
						a.win.Update(func() {
							a.status = fmt.Sprintf("online %v, metered %v; battery %.0f%%, charging %v", n.Online, n.Metered, b.Level*100, b.Charging)
						})
					}()
				}
			})
			mobile.Section(c, "Plugins with native code", func() {
				row := func(title string, run func(ctx context.Context) string) {
					if mobile.ListRow(c.Key(title), mobile.RowOptions{Title: title, Subtitle: a.results[title]}).Clicked() {
						go func() {
							s := run(context.Background())
							a.win.Update(func() { a.results[title] = s })
						}()
					}
				}
				row("Where am I", func(ctx context.Context) string {
					f, err := location.Current(ctx)
					if err != nil {
						return err.Error()
					}
					return fmt.Sprintf("%.4f, %.4f ±%.0f m", f.Latitude, f.Longitude, f.Accuracy)
				})
				row("Is it me", func(ctx context.Context) string {
					kind, err := biometrics.Available(ctx)
					if err != nil {
						return err.Error()
					}
					if kind == biometrics.None {
						return "this device has no way to tell"
					}
					if err := biometrics.Authenticate(ctx, "Confirm it's you"); err != nil {
						return err.Error()
					}
					return "It's you, by " + string(kind)
				})
				row("Choose a photo", func(ctx context.Context) string {
					paths, err := imagepicker.Pick(ctx, imagepicker.Options{})
					if err != nil {
						return err.Error()
					}
					return fmt.Sprint(paths)
				})
				row("Read the accelerometer", func(ctx context.Context) string {
					done := make(chan sensors.Reading, 1)
					stop, err := sensors.Watch(ctx, sensors.Accelerometer, 100*time.Millisecond, func(r sensors.Reading) {
						select {
						case done <- r:
						default:
						}
					})
					if err != nil {
						return err.Error()
					}
					defer stop()
					select {
					case r := <-done:
						return fmt.Sprintf("%.2f, %.2f, %.2f m/s²", r.X, r.Y, r.Z)
					case <-time.After(2 * time.Second):
						return "no reading"
					}
				})
				if mobile.ListRow(c.Key("Camera"), mobile.RowOptions{Title: "Camera", Trailing: func() { mobile.Switch(c.Key("cs"), &a.camOn) }}).Clicked() {
				}
				if a.camOn {
					if a.cam == nil {
						cam, err := camera.New(a.win, camera.Options{})
						if err != nil {
							a.results["camera"] = err.Error()
							a.camOn = false
						} else {
							a.cam = cam
						}
					}
					if a.cam != nil {
						ui.Box(c).Height(240).Shrink(0).FillWidth().Children(func() {
							ui.WebView(c, a.cam).Fill()
							b := ui.Text(c, "Take a photo").Absolute().Bottom(8).Left(8).Padding(6, 10).Radius(8).
								Background(ui.RGBA(0, 0, 0, 0.6)).TextColor(ui.RGB(255, 255, 255))
							if b.Clicked() {
								go func() {
									p, err := a.cam.TakePhoto(context.Background())
									a.win.Update(func() {
										if err != nil {
											a.results["camera"] = err.Error()
										} else {
											a.results["camera"] = p
										}
									})
								}()
							}
						})
					}
				}
				mobile.ListRow(c.Key("mapOn"), mobile.RowOptions{Title: "Map", Trailing: func() { mobile.Switch(c.Key("mapSwitch"), &a.mapOn) }})
				if a.mapOn {
					if a.world == nil {
						paris := maps.LatLng{Lat: 48.8584, Lng: 2.2945}
						m, err := maps.New(a.win, maps.Options{Center: paris, Zoom: 13})
						if err != nil {
							a.results["map"] = err.Error()
							a.mapOn = false
						} else {
							a.world = m
							go m.AddMarker(context.Background(), maps.Marker{ID: "tower", At: paris, Title: "Eiffel Tower"})
							m.OnTap(func(at maps.LatLng) {
								a.results["map"] = fmt.Sprintf("tapped %.3f, %.3f", at.Lat, at.Lng)
								a.win.Update(func() {})
							})
							m.OnMarkerTap(func(id string) {
								a.results["map"] = "tapped the pin " + id
								a.win.Update(func() {})
							})
						}
					}
					if a.world != nil {
						ui.Box(c).Height(240).Shrink(0).FillWidth().Children(func() { ui.WebView(c, a.world).Fill() })
					}
				}
				if s := a.results["map"]; s != "" {
					ui.Text(c, s).FontSize(13).Padding(4, 16).TextColor(t.TextMuted)
				}
				if s := a.results["camera"]; s != "" {
					ui.Text(c, s).FontSize(13).Padding(4, 16).TextColor(t.TextMuted)
				}
				row("Play a tone", func(ctx context.Context) string {
					samples := make([]float32, 48000)
					for i := range samples {
						samples[i] = 0.2 * float32(math.Sin(2*math.Pi*440*float64(i)/48000))
					}
					p := audio.NewSound(48000, 1, samples).NewPlayer()
					p.Play()
					time.Sleep(500 * time.Millisecond)
					return fmt.Sprintf("at %v of a second", p.Position())
				})
				row("May it use the camera", func(ctx context.Context) string {
					st, err := permissionflow.Check(permissionflow.Camera)
					if err != nil {
						return err.Error()
					}
					if st == permissionflow.NotDetermined {
						if st, err = permissionflow.Request(ctx, permissionflow.Camera); err != nil {
							return err.Error()
						}
					}
					return st.String()
				})
				row("Run a task in the background", func(ctx context.Context) string {
					if err := background.Schedule(ctx, "count", background.Options{Delay: 30 * time.Second}); err != nil {
						return err.Error()
					}
					n := 0
					preferences.Get("background runs", &n)
					return fmt.Sprintf("due in 30 seconds, leave the app; it ran %d times", n)
				})
				row("What time is it", func(ctx context.Context) string {
					return time.Now().Format("15:04 MST, Monday")
				})
				row("Keep a note in SQLite", func(ctx context.Context) string {
					dir, err := mygo.App.Path(mygo.PathUserData)
					if err != nil {
						return err.Error()
					}
					db, err := sqlite.Open(ctx, filepath.Join(dir, "notes.db"), sqlite.OpenOptions{})
					if err != nil {
						return err.Error()
					}
					defer db.Close()
					// Full-text search, on a table that outlives the app.
					if _, err := db.Execute(ctx, "CREATE VIRTUAL TABLE IF NOT EXISTS notes USING fts5(body)"); err != nil {
						return err.Error()
					}
					if _, err := db.Execute(ctx, "INSERT INTO notes VALUES (?)", "made with mygo"); err != nil {
						return err.Error()
					}
					r, err := db.Query(ctx, "SELECT count(*), sqlite_version() FROM notes WHERE notes MATCH 'mygo'")
					if err != nil {
						return err.Error()
					}
					return fmt.Sprintf("%v notes found, SQLite %v", r.Values[0][0], r.Values[0][1])
				})
				row("Count launches", func(ctx context.Context) string {
					n := 0
					preferences.Get("launches", &n)
					n++
					if err := preferences.Set("launches", n); err != nil {
						return err.Error()
					}
					return fmt.Sprintf("%d, kept between runs", n)
				})
			})
			ui.Text(c, "Native UI, drawn by MyGo.").FontSize(13).TextColor(t.TextMuted).PaddingX(16)
		})
	})
	mygo.SystemBars.SetHidden(a.bars)
}

func icon(path func(p *ui.Path, r ui.Rect)) func(p *ui.Painter, r ui.Rect, color ui.Color) {
	return func(p *ui.Painter, r ui.Rect, color ui.Color) {
		var l ui.Path
		path(&l, r)
		p.StrokePath(&l, 2, color)
	}
}

var tabs = []mobile.BarButton{
	{Label: "Widgets", Icon: icon(func(p *ui.Path, r ui.Rect) {
		p.MoveTo(r.X+3, r.Y+5).LineTo(r.X+21, r.Y+5).MoveTo(r.X+3, r.Y+12).LineTo(r.X+21, r.Y+12).MoveTo(r.X+3, r.Y+19).LineTo(r.X+21, r.Y+19)
	})},
	{Label: "Lists", Icon: icon(func(p *ui.Path, r ui.Rect) {
		p.MoveTo(r.X+4, r.Y+4).LineTo(r.X+20, r.Y+4).LineTo(r.X+20, r.Y+20).LineTo(r.X+4, r.Y+20).Close()
	})},
	{Label: "Pages", Icon: icon(func(p *ui.Path, r ui.Rect) {
		p.Circle(r.X+12, r.Y+12, 8)
	})},
	{Label: "Keyboards", Icon: icon(func(p *ui.Path, r ui.Rect) {
		p.MoveTo(r.X+2, r.Y+7).LineTo(r.X+22, r.Y+7).LineTo(r.X+22, r.Y+18).LineTo(r.X+2, r.Y+18).Close()
	})},
	{Label: "System", Icon: icon(func(p *ui.Path, r ui.Rect) {
		p.MoveTo(r.X+12, r.Y+3).LineTo(r.X+21, r.Y+21).LineTo(r.X+3, r.Y+21).Close()
	})},
}

func (a *app) view(c *ui.Context) {
	ui.Column(c).Fill().Children(func() {
		mobile.TopBar(c, mobile.TopBarOptions{Title: tabs[a.tab].Label})
		// Keyed by its tab: each keeps its own scroll.
		ui.Box(c.Key(a.tab)).Grow(1).Basis(0).FillWidth().Children(func() {
			switch a.tab {
			case 0:
				a.widgets(c)
			case 1:
				a.lists(c)
			case 2:
				a.pagesView(c)
			case 3:
				a.keyboards(c)
			case 4:
				a.system(c)
			}
		})
		mobile.BottomBar(c, &a.tab, tabs...)
	})
}

func main() {
	// The system may start the app just for this, with nothing on screen.
	background.Handle("count", func(ctx context.Context) error {
		n := 0
		preferences.Get("background runs", &n)
		log.Printf("a task in the background, run %d", n+1)
		return preferences.Set("background runs", n+1)
	})
	a := &app{wifi: true, when: time.Now(), results: map[string]string{}}
	// Back as it was after each reload of mygo dev, and when the system ended it.
	mygo.KeepState(a)
	for i := range 30 {
		a.rows = append(a.rows, fmt.Sprintf("Row %d", i+1))
	}
	mygo.App.OnOpenURL(func(u string) {
		a.link, a.tab = u, 4
		if a.win != nil {
			a.win.Update(func() {})
		}
	})
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: "Mobile", Width: 420, Height: 800, Content: ui.View(a.view)})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
