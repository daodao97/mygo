# Accessibility

Assistive technology, such as VoiceOver on macOS, Orca on Linux and
Narrator or NVDA on Windows, reads a window's native UI as it reads other
apps: the widgets with their roles, names, values and states, the texts,
and the elements that take the focus, as the content changes. It acts on
them as the keyboard and the pointer would: it presses buttons, checks
boxes, moves sliders, edits text inputs and moves the focus.

## Names

Widgets describe themselves, and the text inside an element names it, as a
button's does. Elements without text need a `Label`, which also finds them
in [tests](testing.md):

```go
ui.Slider(c, &app.volume, 0, 100).Label("Volume")
ui.TextInput(c, &app.query).Placeholder("Search").Label("Search")
ui.Box(c).Size(24, 24).Draw(drawIcon).Label("Unread messages")
```

A [field](form.md) names its control with its label.

## Roles

Other elements get a role from what they do: one that is clickable and
takes the focus is a button, a scroll container a scroll area, and one with
a `Label`, or that takes the focus, a group. `Role` sets it for an element
drawn as a widget it is not built from, and `ui.RoleNone` leaves an element
out but not its children ([`AccessHidden`](#out-of-assistive-technologys-way)
leaves them out too):

```go
toggle := ui.Box(c).Size(36, 20).Focusable().Role(ui.RoleSwitch).Label("Wi-Fi")
```

Besides the roles of MyGo's widgets, `ui.RoleHeading` is the title of a
section, ranked with `Level`, `ui.RoleNavigation` a landmark of links
that move around the app or a document, as a breadcrumb trail or a
navigation bar, which screen readers list and jump to (as
[`Breadcrumbs`](breadcrumbs.md) is), and `ui.RoleMenu`, `ui.RoleMenuBar`,
`ui.RoleMenuItem`, `ui.RoleMenuItemCheckBox` and `ui.RoleMenuItemRadio`
make menus drawn in the window, as a drop-down menu or a menu bar of your
own (an element's menu drawn in the window,
[`PopoverMenu`](context-menu.md#drawn-in-the-window) or
[`PopoverMenuButton`](menu-button.md#drawn-in-the-window), has them, as
do [`MenuBase`](menu-button.md#without-a-look) and `ContextMenuBase`,
menus without a look that keep their keys and focus):

```go
ui.Text(c, "Appearance").Bold().Role(ui.RoleHeading).Level(2)
ui.Row(c).Role(ui.RoleNavigation).Label("Sections").Children(sectionLinks)
```

## States

The bases tell assistive technology what their widgets show. A widget of
your own tells it with these, as ARIA's states do on the web:

| Method | What it tells |
|---|---|
| `Checked(on)`, `Mixed()` | whether a check box, switch, radio button, toggle or menu item is on, or partly on |
| `Expanded(open)` | whether what the element opens shows, as a popup or a section |
| `Value(s)` | its value, as the choice a button opening a popup shows |
| `Range(lo, hi, value)`, `Step(step)` | the range and value of a slider, progress bar, meter or stepper, and how far the keys move it |
| `Level(n)` | the rank of a heading, or how deep an item of a tree is |
| `ActiveDescendant(e)` | the option that has the focus while the element keeps it, as the one the arrows are on in a menu or a list |
| `Busy(on)` | that the element's content is about to change, as a region loading: screen readers wait for it before reading it |
| `Current(kind)` | that the element is the current one of the items around it: `ui.CurrentPage` (the page the user is on), `CurrentStep`, `CurrentLocation`, `CurrentDate`, `CurrentTime` or `CurrentItem` |

```go
all := ui.CheckboxBase(c, &app.all).Label("Select all")
if app.some {
	all.Mixed()
}

date := ui.ButtonBase(c).Role(ui.RolePopUpButton).Label("Due").Value(app.due.Format("Jan 2")).Expanded(app.picking)
```

A region loading says so while a placeholder stands in for it, and the
link to the page shown in a navigation bar says it is the current page:

```go
ui.Column(c).Label("Messages").Busy(app.loading).Children(func() {
	if app.loading {
		ui.Spinner(c).Label("Loading")
		return
	}
	messageList(c, app.messages)
})

ui.Row(c).Role(ui.RoleNavigation).Label("Main").Children(func() {
	for _, p := range pages {
		l := ui.Link(c, p.Title, p.Path)
		if p.Path == app.path {
			l.Current(ui.CurrentPage)
		}
	}
})
```

An element with no role of its own and no `Label` is still seen once
`Busy` or `Current` marks it, as a group, as ARIA's states make a
generic element seen on the web. `Busy` is AppKit's `AXElementBusy`,
ATK's busy state and UI Automation's `AriaProperties` (`busy=true`);
`Current` is AppKit's `AXARIACurrent`, ATK's `current` attribute with the
active state, and `AriaProperties` (`current=page`).

A [test](testing.md#what-assistive-technology-sees) reads them as
assistive technology gets them, with `Tester.AccessByRole`:

```go
if a, _ := tt.AccessByRole(ui.RoleGroup, "Messages"); !a.Busy {
	t.Error("Messages is not busy while loading")
}
```

A check box that is mixed as often as not is better built on
[`CheckboxStateBase`](checkbox.md#without-a-look), which tells it itself
from its `ui.CheckState`, and checks it as it is pressed.

## Descriptions and errors

`Description` tells more than the name, as help text read after it, as a
[tooltip](tooltip.md) does, and `Error` marks a value as invalid, for a
message read with it; a [field](form.md) gives its control the texts below
it.

## Keyboard shortcuts

`KeyShortcuts(mods, key)` tells screen readers a key that presses or
focuses the element, as ARIA's `aria-keyshortcuts`: a toolbar button
whose command the window's shortcut runs, or the button opening a command
palette, is read with its key after its name. It only tells the key,
which the view handles itself, as with
[`c.Shortcut`](input.md#shortcuts); call it once for each key that does:

```go
if c.Shortcut(ui.Cmd, ui.KeyK) {
	app.palette = true
}
ui.Toolbar(c, func() {
	if ui.Button(c, "Search").KeyShortcuts(ui.Cmd, ui.KeyK).Clicked() {
		app.palette = true
	}
	ui.Button(c, "Redo").KeyShortcuts(ui.Cmd|ui.Shift, ui.KeyZ).KeyShortcuts(ui.Cmd, ui.KeyY)
})
```

The keys are written as the platform's menus write them, separated by
spaces: `⌘K` and `⇧⌘Z ⌘Y` on macOS, in AppKit's `AXKeyShortcutsValue` as
WebKit gives it; `Ctrl+K` and `Ctrl+Shift+Z Ctrl+Y` on Linux, in ATK's
`keyshortcuts` object attribute, and on Windows, in UI Automation's
`AcceleratorKey`, where Super is `Win`. A
[test](testing.md#what-assistive-technology-sees) reads them in the
`KeyShortcuts` of `Tester.AccessByRole`'s answer, and the
[automation plugin](../plugins/automation.md) in a node's `keyShortcuts`.

## Formats of text

The [rich text editor](rich-text-editor.md) tells screen readers the formats
of its runs as they read them: bold, italic, underlined, struck through,
links, fonts, colors and headings. VoiceOver reads them through
`AXAttributedStringForRange`, Orca through ATK's run attributes, and
Narrator through UI Automation's Text pattern, which every text field has,
by characters, formats, words and lines. Screen readers move the selection
of text fields too.

## Lists and tables

A [list](list.md)'s rows are list items, and a [table](table.md)'s rows,
named by the text inside them, and each says which of all the rows it is,
"5 of 10,000", although the list builds only those in view. A list choosing
its rows gives assistive technology the focus on the row chosen: Up and
Down are read as they move the choice, and focusing a row chooses it. A
screen reader moving out of view asks the list to scroll there, which
builds the rows it reaches. `Label` names a list or a table.

## Out of the pointer's way

`IgnoreInput(true)` leaves an element, and those inside it, to assistive
technology alone: the pointer goes through it to what is under it, Tab
and clicks give it no focus, but assistive technology still finds it, with
its role and name, and presses it, which `Clicked` reports. A button that
shows while its row is hovered can so stay built at rest, at `Opacity(0)`,
where a screen reader finds it and a click on the row is the row's:

```go
row := ui.Row(c).Padding(8)
hovered := row.Hovered()
if row.Clicked() {
	app.open(item)
}
row.Children(func() {
	ui.Text(c, item.Title).Grow(1)
	del := ui.Button(c, "Delete").IgnoreInput(!hovered)
	if !hovered {
		del.Opacity(0)
	}
	if del.Clicked() {
		app.remove(item)
	}
})
```

Assistive technology can neither focus it nor move a slider inside it, as
both go through the keyboard focus; it can set a text input's value.
Unlike HTML's `inert`, which hides an element from assistive technology
too, it stays in the tree: `Invisible` and `Hidden` take it out of it,
`AccessHidden` too while it still shows, and `Disabled` keeps it there
without its press.

## Out of assistive technology's way

`AccessHidden(true)` leaves an element, and those inside it, out of what
assistive technology reads, while they still paint, as HTML's
`aria-hidden="true"` does: none of them is in its tree, their text does
not name the element around them, and it can neither press, focus nor edit
them. It is for what shows but is not there to read, or is read already:

- a page or a panel sliding out beside the one replacing it, while both
  show;
- the cards peeking from behind the front one of a stack, the slides
  beside the current one of a carousel;
- a copy of a text drawn twice, as a label fading into its next one.

With `IgnoreInput(true)`, it is HTML's `inert`: the pointer goes through
them, Tab passes them by, they lose the focus and their text cannot be
selected, as for the copy of an
[element leaving](transitions.md#coming-and-going) or a
[page going away](navigation.md#transitions):

```go
ui.Box(c).Fill().Children(func() {
	if app.leaving >= 0 { // the tab that was selected, sliding out
		page(c, app.leaving).Absolute().Translate(-x, 0).IgnoreInput(true).AccessHidden(true)
	}
	page(c, app.tab).Translate(width-x, 0)
})
```

Alone, it leaves them to the pointer and the keyboard, and their text to
selection, as `aria-hidden` does: keep the keyboard focus out of them, as
assistive technology cannot follow it there. `RoleNone` leaves out an
element alone, which suits a leaf, as a decorative icon.

## Announcements

News that does not move the focus, as a search done or a file saved,
reaches screen readers with `c.Announce(text)`, which they read once, after
what they are reading: VoiceOver's and Orca's announcements, and a live
region that Narrator and NVDA read on Windows. [Toasts](toast.md) announce
their text, and a [router](navigation.md) the title of a page shown while
the focus stays outside it.

```go
if ui.Button(c, "Search").Clicked() {
	app.results = search(app.query)
	c.Announce(fmt.Sprintf("%d results", len(app.results)))
}
```

MyGo describes frames only once assistive technology asked, so apps pay
nothing for it otherwise.

Each component's page says what assistive technology sees of it.
