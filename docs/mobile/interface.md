# Interfaces on phones

## Tabs and stacks

Phones move through an app with the system's bar of tabs at the bottom
and stacks of screens, each with a bar at the top holding its title and a
back button. `mygo.NewTabs` shows a bar of tabs (UIKit's on iOS, Material's
bottom navigation on Android), and `Window.Push` shows a window over
another, on its tab's stack:

```go
mygo.App.WhenReady(func() {
	list := mygo.NewWindow(mygo.WindowOptions{Title: "Fruits", Content: ui.View(app.list), Hidden: true})
	web := mygo.NewWindow(mygo.WindowOptions{Title: "Web", URL: "/", Hidden: true})
	tabs, _ := mygo.NewTabs(
		mygo.TabItem{Title: "Fruits", Symbol: "leaf", Icon: leafPNG, Window: list},
		mygo.TabItem{Title: "Web", Symbol: "globe", Icon: globePNG, Window: web},
	)
	tabs.SetBadge(1, "3")
	app.open = func(fruit string) {
		list.Push(mygo.NewWindow(mygo.WindowOptions{Title: fruit, Content: ui.View(app.detail(fruit)), Hidden: true}))
	}
})
```

A tab names an SF Symbol for iOS and gives a PNG for Android (and for iOS
without a symbol), drawn as a template in the bar's colors. The back
button, iOS's swipe from the left edge and Android's back pop a pushed
window, which closes it; `Window.Pop` does too. On iOS the bars float
over native UI, whose lists scroll on beneath the tab bar's glass; on
Android the bars take the color of the window's background. `Tabs.SetTitle`
changes a tab's title after the fact, for an app whose language changes
without restarting (`App.SetLocale`); on desktops it does nothing. On
desktops the windows are windows: `Tabs.Select` shows one, and a pushed
window shows as one. `examples/tabs` shows both, with a page in one of its
tabs.

A `ui.Router` can show its pages as the system's stack instead, in one
window: with `SystemStack` set, the history up to the page shown is the
stack, under the system's navigation bar with the title each page gives
`Route.Title` and a back button. On iOS UIKit's navigation controller moves
the pages, with its swipe from the left edge; on Android the bar is
Material's toolbar, with its back arrow, and the router slides the pages as
Android's apps do. Show no bar of your own while `InSystemStack` reports it:

```go
func (app *notes) view(c *ui.Context) {
	app.router.SystemStack = true // on desktops and in browsers, nothing changes
	ui.Column(c).Fill().Children(func() {
		if !app.router.InSystemStack(c) {
			ui.Row(c).Children(func() {
				ui.BackButton(c, app.router)
				ui.Text(c, app.router.Title()).Bold()
			})
		}
		app.router.View(c, func(r *ui.Route) {
			switch {
			case r.Match("/"):
				r.Title("Notes")
				app.list(c)
			case r.Match("/notes/{id}"):
				r.Title("Note")
				app.note(c, r.Param("id"))
			}
		})
	})
}
```

The router's View fills the window. On iOS the window keeps one surface,
which goes with the page shown: the screens beneath hold a snapshot of
their page as it last showed, which a finger going back uncovers, and
the page shows live again once back. `examples/gallery` shows its
pages so on phones.

On iPad a window can be a window of the system's own, beside the app's others, with
`"ios": {"multipleWindows": true}` in `mygo.json`:

```go
mygo.NewWindow(mygo.WindowOptions{Title: "Draft", OwnScene: true, Content: ui.View(draft)}).Show()
```

Elsewhere it's a window as any: on a phone, over the one before. Android's tablets
split the screen with other apps by themselves.

## Fingers

Native UI takes fingers as the systems' own toolkits do:

- A tap is a click. A finger reaches what it nearly touches, by 14 DIPs: a
  slider's thumb is narrower than a fingertip.
- A finger that moves locks to the way it went, and to the innermost container
  that scrolls that way: a row of photos in a list down the screen takes what
  goes across, and leaves what goes down to the list. It flings the content on
  as it lifts, and stops a fling as it lands, tapping nothing.
- Past the content's ends, the content follows less and less far and springs
  back, a long way on iOS and less on Android. A scroll indicator shows as the
  content moves.
- What drags along a way takes what goes that way: a sideways `Draggable` in a
  list, even with buttons inside it, leaves what goes down to the list.
- Held still for half a second, a finger long-presses: `Element.LongPressed`,
  a context menu, a word of text, or what drags a value, as a list's rows that
  `Reorder` moves. The device gives a tick.
- Two fingers pinch and turn (`InputEvent`'s `InputZoom`, with `Rotation`).
- From the left edge on iOS, a finger takes a `ui.Router`'s page back, following
  it, as UIKit's stacks do. A tap on iOS's status bar sends what scrolls back to
  its top.

`Element.PullToRefresh` lets a finger pull a scroll container past its top:

```go
list := ui.Scroll(c).Fill()
if list.PullToRefresh(&app.refresh) {
	go func() {
		items := fetch()
		win.Update(func() { app.items, app.refresh.Refreshing = items, false })
	}()
}
```

## Text

The keyboard edits text through its input's `TextInputClient`, with the system's
suggestions and input methods. Elements say what they hold, which chooses the
keyboard:

```go
ui.TextInput(c, &app.email).
	Keyboard(ui.KeyboardEmail).ReturnKey(ui.ReturnNext).
	Capitalize(ui.CapitalizeNone).Autocorrect(false).Autofill(ui.AutofillEmail)
```

`ReturnNext` moves the focus on; `Password` is a secure entry. As the keyboard
comes up it shows the input in use. Holding a word chooses it, with the system's
menu of what to do with it (`UIEditMenuInteraction` and Android's floating
toolbar); the handles at its ends drag them, with the system's magnifier, drawn
as each system's. Tapping the caret shows the menu too.

## Widgets of phones

Package `ui/mobile` is what phones' interfaces are made of and desktops' aren't,
looking as each system's, on `ui`'s public API. Apps that don't import it don't
pay for it.

| | |
|---|---|
| `Sheet`, `ActionSheet` | Panels coming up from the bottom, with detents, which a finger drags away |
| `Drawer` | A panel sliding in from the side |
| `PageView`, `PageDots` | Pages a finger swipes between, built as they come near |
| `SwipeRow` | A row a finger swipes aside to reveal actions |
| `Wheel`, `DatePicker`, `TimePicker` | Wheels that spin, with a tick as the choice changes |
| `TopBar`, `BottomBar`, `FloatingButton` | Bars drawn by the app, for one window that routes within it |
| `ListRow`, `Section` | Rows and groups of them, with a press's look |
| `Ink` | Material's ripple |
| `Switch` | The system's switch |
| `Zoomable` | Content that fingers pinch, turn and drag, as a photo |

`mobile.SetLook` chooses a design whatever the system. `examples/mobile` shows
them all.

Pictures from the network take `ui/netimage`, in place of `ui.Image`:

```go
netimage.Image(c, user.AvatarURL, netimage.Options{MaxSide: 240}).Size(80, 80).Radius(40)
```

A placeholder holds their place, and they fade in once they come. They stay in
memory while there is room and on disk across runs, as their servers' caching
headers say, and show offline. What's on screen downloads before what a fling
left behind, which doesn't download at all if it's gone by its turn. Desktops
and browsers take it too.

## Testing

Unit tests run views with a `ui.Tester`, whose clock they advance, with
`FingerDown`, `FingerMove`, `FingerUp` and `SetInsets`. Package `ui/golden` compares
what it draws with images beside the tests:

```go
func TestCard(t *testing.T) {
	golden.Match(t, ui.NewTester(card, 320, 200), "testdata/card.png")
}
```

Run with `MYGO_UPDATE_GOLDEN=1` to write them, on the platform that runs the
tests: text differs from one to another.

`MYGO_FRAME_STATS=1` logs frames slower than 8 ms, with what took the time, also
on phones: `SIMCTL_CHILD_MYGO_FRAME_STATS=1` for a simulator, and on Android
`adb shell am start ... --es mygo.env MYGO_FRAME_STATS=1`.

End-to-end tests drive the running app from another process, on simulators and
devices alike, with [the automation plugin](../plugins/automation.md#on-phones): they
read the elements on screen, act on them, send fingers and text, capture the
screen and wait for it to settle.

Tap with four fingers to open the inspector of native UI in a development build:
along the bottom of a phone, with the elements, their styles, frame times and
issues, as F12 opens it on a desktop.
