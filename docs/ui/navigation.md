# Pages and navigation

A `ui.Router` keeps the history of the pages of a window, or of a part of
one, as a browser does for a tab. A page is a path, as `/notes/42`, which
`View` matches to build the page shown, a `switch` over patterns whose
`{name}` takes one part of the path and `{name...}` the rest.

The router is part of your app's state: make it once, with the page to
show first, and keep it in a field. The view, which runs for every frame,
builds the page it shows:

```go
// notesApp is the app's state.
type notesApp struct {
	router *ui.Router
	// … the notes and the files
}

func main() {
	app := &notesApp{router: ui.NewRouter("/notes")}
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{Title: "Notes", Content: ui.View(app.view)})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (app *notesApp) view(c *ui.Context) {
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		app.sidebar(c) // stays as the pages change, see below
		app.router.View(c, func(r *ui.Route) {
			switch {
			case r.Match("/notes"):
				r.Title("Notes")
				app.notes(c)
			case r.Match("/notes/{id}"):
				r.Title("Note")
				app.note(c, r.Param("id"))
			case r.Match("/files/{path...}"):
				app.files(c, r.Param("path"))
			default:
				ui.Text(c, "Not found")
			}
		})
	})
}
```

`app.notes`, `app.note` and `app.files` are methods of `notesApp` that
build those pages, as `view` builds the window.

## Going to pages

`Push` goes to a page, after the page shown, `Replace` shows one in its
place, and `Pop`/`Back`, `Forward` and `Go` move through the history; paths
relative to the page shown resolve as links in a web page do, so
`Push("?tab=info")` changes the query and `Push("edit")` goes to a sibling.
A [link](link.md) to a path in a page goes there in its router, and
choosing an item of a [sidebar](sidebar.md) pushes its page, as the sidebar
of the app above does:

```go
func (app *notesApp) sidebar(c *ui.Context) {
	page := app.router.Path()
	if ui.Sidebar(c, &page, func() {
		ui.SidebarItem(c, "/notes", nil, "Notes")
		ui.SidebarItem(c, "/files", nil, "Files")
	}).Width(220).Changed() {
		app.router.Push(page)
	}
}

// In a page:
ui.Link(c, "Open note", "/notes/42")
```

`Path`, `Query` and `Location` read the page shown; `Location` with its
query, which `NewRouter` takes to show it again when the app starts. A deep
link (`mygo.App.OnOpenURL`) pushes its page.

## Back and forward

Users go back and forward as in browsers and Finder: Cmd+[ and Cmd+] on
macOS, Alt+Left and Alt+Right on Linux and Windows, the back and forward
buttons of a mouse and the keys of keyboards that have them, and the
[back and forward buttons](history-buttons.md), which a toolbar holds, and
whose right click lists the pages to go back or forward to, by their
titles. The keys go to the router holding the keyboard focus, else to the
first of the window.

`Pop()` is the same as `Back()` / `Go(-1)`: it returns to the previous
entry and retains forward history. A new `Push` after returning replaces the
forward branch. `Replace` changes only the current entry. `Reset("/home")`
clears both directions and shows that location as the only entry. History
ownership stays in the router; page code calls these methods directly.
This follows the traversal model of [Vue Router](https://router.vuejs.org/guide/essentials/navigation.html)
and the [history library](https://github.com/remix-run/history/blob/main/docs/api-reference.md).

On iOS, `ui.NewRouter` enables `InteractiveBack`: a touch within 24 DIPs of
the view's left edge can drag the current page right, revealing the previous
page. Release beyond halfway, or flick right, to finish returning; a short
or reversed drag cancels. Cancellation retains the current page's editing,
keyboard focus and scroll position. Shared layouts remain fixed and the
root page cannot return further. Vertical edge gestures still scroll.
Set `router.InteractiveBack = false` to disable it, or `true` to opt in on
another platform with touch input. Entries differing only by query share
one page and use the ordinary Back controls rather than a drag preview.

## Layouts

Anything built around `View`, as a sidebar or a toolbar, stays as pages
change. A page can also be a layout around the pages of the rest of its
path, as a page of settings with a list of its sections beside the section
shown: `r.View` on its route builds them, matching what the pattern's
`{name...}` took. The layout stays as they change, keeping its state, and
they slide or fade in its place; `Param` reads the layout's wildcards too.
A page of settings is another case of the `switch` in the view above:

```go
case r.Match("/settings/{section...}"):
	ui.Row(c).Grow(1).Children(func() {
		app.sectionList(c) // pushes "/settings/general", "/settings/fonts"
		r.View(c, func(r *ui.Route) {
			switch {
			case r.Match("/general"):
				r.Title("General")
				app.general(c)
			case r.Match("/fonts"):
				r.Title("Fonts")
				app.fonts(c)
			}
		})
	})
```

A second `Router` inside a page, in a field of its own, is a history of its
own, as the detail of a split view going deeper while the list beside it
stays.

## Pages keep their state

A page keeps the state of its elements while it is in the history, ten
pages either way: going back to one finds it scrolled where it was, with
its text and the keyboard focus where they were. A path with another query
is the same page, in another entry of the history, keeping its state, as a
page whose tabs or search are in its query.

A Router serializes its complete history and current index itself. Keep it
directly in state registered with `mygo.PersistState`, rather than copying
locations or replaying Push calls in the application:

```go
type State struct {
    Router *ui.Router
    Draft  string
    Scroll ui.ScrollState
}

state := State{}
mygo.App.WhenReady(func() {
    if _, err := mygo.PersistState("main", &state); err != nil {
        log.Fatal(err)
    }
    if state.Router == nil {
        state.Router = ui.NewRouter("/home")
    }
    // Build content with state.Router.View(c, ...).
})

// Inside page handlers:
state.Router.Push("/settings")
state.Router.Pop()
```

Automatic background checkpoints include navigation from methods, links,
keyboard shortcuts and completed edge gestures. Back and forward entries
and the selected index survive relaunch; a cancelled gesture does not change
the checkpoint. Invalid navigation JSON returns an error without changing
the live router. A fresh restored router uses `NewRouter`'s platform defaults;
restoring into an initialized router retains its configuration. Options,
widget caches, focus and animations are not serialized.

`History()` remains a read-only copy of locations through the current entry,
excluding forward history. Legacy JSON location arrays are also accepted on
restore, with their last entry selected.

On the first restored build, `Route.View` reconstructs shared layouts across
adjacent history entries. Back, Forward and edge gestures then retain the
layout's running element state, including edits made after restoration.
Nested query variants share their leaf state; a separate history branch
does not reuse another branch's layout. Existing checkpoint formats need
no migration.

Element state belongs to the running UI tree; persist editor values and
tracked [scroll positions](scroll.md) separately. For a tabbed app, keep
one router per tab. See the [iOS feature demo](../../examples/ios-native/README.md).

## The focus and screen readers

Going to a page moves the keyboard focus into it, to the element that had
it there, else to the page itself, which Tab goes into; screen readers read
the page's `Title`. With the focus outside the page that changes, as in a
sidebar or a layout's list choosing the pages, it stays there, and screen
readers hear the title of the page.

## Transitions

A page deeper in the paths, as `/notes/42` after `/notes`, slides in from
the right over the page it leaves, and back up from the left; others fade
in. With less motion asked of the desktop, all fade, and
`router.Transition = ui.TransitionNone` shows them at once. The page going
away takes neither the pointer nor the keyboard as it slides.
