# Scroll view

<!-- preview scroll 280 -->

`ui.Scroll` and `ui.ScrollHorizontal` scroll their children with the wheel,
the touchpad, their scroll bar and the keyboard: the arrow keys, Page Up and
Page Down, Space, Home and End scroll the container around the focus, or
under the pointer, unless the focused element takes those keys, as a text
input does. `ui.ScrollBoth` scrolls both ways, as a canvas or a wide table
does. A notch of a mouse wheel glides over a short ease out, as browsers
scroll it, and notches in a row add up, until the app scrolls the
container (a `ScrollState`, a `ListState`'s `ScrollTo`, `ScrollIntoView`
and `ScrollToEnd`, or `ScrollIntoView` of an element in it), which takes
over; a touchpad follows the fingers, as a finger dragged on a
touchscreen does on Linux and Windows, and while the desktop asks for less
motion, a notch scrolls at once.

```go
ui.Scroll(c).Grow(1).Padding(16).Gap(12).Children(func() {
	for _, p := range app.posts {
		post(c, p)
	}
})
```

Give them a size, or grow them in their parent. For the rows of a
collection, use a [list](list.md), which builds only those in view.

Laid out [right to left](right-to-left.md), a container scrolling sideways
shows the start of its content, at the right, first, and `ScrollState.X`
counts from there: 0 is the start and `MaxX` the end, as Flutter's offsets
are.

## Keeping the offset

A scroll container keeps its offset as it keeps its other state, by its
place in the view or its `Key`. To read the offset, set it or keep it in
the app's state, give the container a `ui.ScrollState` with `TrackScroll`:
the container shows its content from the state's `X` and `Y`, writes them
as the user scrolls, and sets `MaxX` and `MaxY` to how far they go. `Y = 0`
scrolls to the top and `math.MaxFloat32` to the end, and a `ScrollState`
for each page a container shows keeps each page's place. A log that follows
its end, unless the user scrolled up from it:

```go
if app.log.Y >= app.log.MaxY {
	app.log.Y = math.MaxFloat32
}
ui.Scroll(c).TrackScroll(&app.log).Grow(1).Children(app.lines)
```

`ScrollTo(x, y, animation)` scrolls there smoothly, as a spring or a curve
(see [Motion](motion.md)), turning toward a new place if called again on
the way; the user scrolling stops it, and `Scrolling()` reports whether it
is on its way. Where the desktop asks for less motion, it scrolls at once:

```go
if ui.Button(c, "Back to top").Clicked() {
	app.feed.ScrollTo(0, 0, ui.Smooth)
}
```

## Sticky elements

`Sticky` keeps an element in the flow, where it takes its room, and holds
it inside the scroll container around it while its parent is in view, as
CSS's `position: sticky`: `Top`, `Right`, `Bottom` and `Left` (or `Start`
and `End`, and their percentages of the container's size) say how far in
from the container's edges, inside its border, it stays. It goes no
further than its parent's content box, so as its parent scrolls away it
takes the element along, and the next parent's comes up in its place. A
day's heading stays at the top of a timeline until the next day pushes it
out:

```go
ui.Scroll(c).Fill().Children(func() {
	for _, day := range app.days {
		ui.Column(c).Children(func() {
			ui.Text(c, day.Title).Bold().Padding(8, 16).Background(t.Surface).Sticky().Top(0)
			for _, ev := range day.Events {
				eventRow(c, ev)
			}
		})
	}
})
```

Each axis follows the nearest container scrolling that way (or a header
following another sideways with `ScrollWith`), or else the window. So the
check box choosing all the rows of a table scrolling sideways stays at its
start, while the header row it is in sticks to the top of the page that
scrolls down, which CSS would not do, as a container scrolling one way
there scrolls both:

```go
ui.Scroll(c).Fill().Children(func() {
	ui.ScrollHorizontal(c).Shrink(0).Children(func() {
		ui.Column(c).Width(app.tableWidth()).Children(func() {
			ui.Row(c).Background(t.Surface).Sticky().Top(0).Children(func() {
				ui.Checkbox(c, &app.all, "").Background(t.Surface).Sticky().Start(0)
				headerCells(c)
			})
			rows(c)
		})
	})
})
```

Where both insets of an axis are set and the container is too small for
both, `Top` wins, and `Left`, or `Right` [right to left](right-to-left.md).
A sticky element paints above its siblings, as absolute ones do, so what
scrolls under it needs no clip, but give it a background; the pointer,
assistive technology, what is attached to it (`AttachTo`) and
`ScrollIntoView` find it where it shows. The focus and assistive technology
still take it in its place among its siblings. It does nothing to an
`Absolute` element or to one inside a text. A [list](list.md) builds only
the rows in view, so give its section headers with `ListState.Header`,
which pins them, rather than making a row sticky.

## Scrolling into view

`ScrollIntoView` scrolls the containers around an element as little as
shows it, once the frame is laid out, so that it works in the frame that
adds the element: a new message, or the item the keys chose. Call it in that
frame only, or the user could not scroll the element away. Rows a `List`
has not built scroll into view with its `ListState`.

```go
if app.added {
	ui.Text(c, msg.Text).ScrollIntoView()
}
```

`ScrollIntoViewOptions`, as the web's `scrollIntoView` options, line the
element up elsewhere, or glide there. `Block` lines it up down the
containers and `Inline` across them: `ui.ScrollNearest`, the default,
scrolls as little as shows it, and `ui.ScrollStart`, `ui.ScrollCenter` and
`ui.ScrollEnd` line its start, center or end up with each container's,
inside its border; [right to left](right-to-left.md), the inline start is
at the right. `Animation`, when set, glides every container around it
there as the animation says, as the web's `behavior: "smooth"`, rather
than at once: the user scrolling, or the app, stops it, and where the
desktop asks for less motion it scrolls at once. The current match of a
search, centered:

```go
if app.matchMoved {
	hit.ScrollIntoView(ui.ScrollIntoViewOptions{Block: ui.ScrollCenter, Animation: &ui.Smooth})
}
```

The zero options are those of `ScrollIntoView()` without them.

The keyboard focus scrolls into view as it moves. An element with a
`TextInputClient` of the app's ([text input clients](text-input-client.md))
scrolls its caret into view, where `BoundsForRange` places it, rather than
its whole box: a code editor as high as its lines shows the line being
edited as it takes the focus, not its first.

## Snapping

`ScrollSnap` makes a container come to rest at the places the elements in
it mark with `SnapAlign`, as CSS's `scroll-snap-type` and
`scroll-snap-align` do, on the axes it scrolls: a strip of cards that
always rests showing a card from its start, a carousel's pages, or the
sections of a long page.

```go
ui.ScrollHorizontal(c).ScrollSnap(ui.SnapMandatory).Padding(16).Gap(12).Children(func() {
	for _, card := range app.cards {
		cardView(c, card).Width(240).Shrink(0).SnapAlign(ui.Start)
	}
})
```

`SnapAlign(ui.Start)`, `ui.Center` or `ui.End` lines the element's start,
center or end up with the container's content box, inside its padding, so
the first card of the strip rests where it starts; right to left, the
start is at the right. `SnapAlignX` and `SnapAlignY` mark one axis only, as
a `ScrollBoth` that snaps its columns and scrolls freely down them. The
elements are any inside the container, not only its children, short of
another scroll container, whose own they are; a `List`'s rows mark theirs
as they are built.

The container snaps as the user's scrolling settles, as browsers snap:

- a touchpad, the momentum of a fling after it, a finger on a touchscreen,
  and a scroll bar's thumb once it is let go glide on to the nearest place;
- a notch of a mouse wheel, the arrow keys, Page Up and Page Down, Home and
  End and a press on a scroll bar's track go to the next place that way,
  past where it was: the nearest to where they would have scrolled. At the
  last place, the wheel scrolls the container around it.

`ui.SnapMandatory` always rests at a place: it lines one up as it first
shows, and the wheel and the keys go from place to place.
`ui.SnapProximity` rests at one only where the scrolling stops within a
third of the view of it, and wherever it stops in between. An element
larger than the view, as a tall section, rests anywhere it fills the
view, so the wheel scrolls through it before going on to the next.

Resting at an element, the container stays at it as the layout moves it,
as when the window resizes or rows above change height, and an element
that comes into view (`ScrollIntoView`, or the focus moving) lines up
the place of the element marked around it. Where the app scrolls the
container, with its `ScrollState`, `ScrollTo` or a `ListState`, it stays
where the app puts it. While the desktop asks for less motion, it goes to
rest at once.

## Scroll bars

A scroll container whose content overflows it shows the thumbs of its
scroll bars over its content while the pointer is over it, which the user
drags; they take no room. Right to left, the vertical bar is on the left.
The theme's `ScrollbarWidth` sets their width and its `Scrollbar` their
color.

`ScrollbarStyle` styles them further: the color of the track behind the
thumb, the thumb's hover and drag colors, the widths of the bar and its
thumb, the radius of its corners, and the shortest thumb. It overrides
`Scrollbar` and `ScrollbarWidth` where it sets a field; its own zero fields
follow the theme, so a theme that sets neither draws as it always has.
The defaults are MyGo's own: a 6 DIP rounded thumb in a 12 DIP bar, 2 DIPs
clear of its ends. Set a style in a theme for every scroll bar of the app,
on a container for its own, or on an editor's `State.Scrollbar`:

```go
t := *c.Theme()
// As VS Code's editor: a 14 DIP square thumb in a 14 DIP bar, flush
// with its ends.
t.ScrollbarStyle = ui.ScrollbarStyle{
	TrackWidth: 14, ThumbWidth: 14, Square: true, Flush: true, MinLength: 20,
	Thumb: ui.RGBA(121, 121, 121, 0.4),
	Hover: ui.RGBA(100, 100, 100, 0.7),
	Active: ui.RGBA(191, 191, 191, 0.4),
}
c.SetTheme(&t)
```

`TrackWidth` is the width of the bar's track and hit area, in DIPs, 0 for
`ThumbWidth` plus 6; `ThumbWidth` is the width of the thumb, 0 for the
theme's `ScrollbarWidth`; `Radius` rounds its corners, 0 for half its
width, and `Square` leaves them square, as VS Code's scroll bar does;
`Flush` leaves no gap between the thumb and the ends of its track, as VS
Code's, and otherwise 2 DIPs clear them; `MinLength` is the shortest it
gets, 0 for 24 DIPs. `Track` fills the bar behind the thumb, and zero
paints none. `Thumb` colors the thumb at rest, `Hover` while the pointer
is over the bar, and `Active` while it is dragged; zero `Thumb` takes the
theme's `Scrollbar`, and zero `Hover` and `Active` take `Thumb`. A kept
bar (`KeepScrollbars`) shows its `Hover` color, as it looks on hover.

A container's own style wins over the theme's:

```go
ui.Scroll(c).ScrollbarStyle(ui.ScrollbarStyle{TrackWidth: 14, ThumbWidth: 14, Square: true, Flush: true}).Children(func() { /* ... */ })
```

`HideScrollbars()` hides both thumbs and disables their pointer targets,
while retaining touch/wheel scrolling, keyboard navigation, accessibility
scrolling and saved offsets. Apply it to the container each frame; omit it
to show the bars again. It also works on `List`, `GridView` and `TextArea`.

```go
ui.Scroll(c).HideScrollbars().TrackScroll(&app.scroll).Fill().Children(func() { /* ... */ })
```

`ScrollbarInsets` moves the bars in from the container's edges, CSS style
as `Padding`: the vertical bar runs from the top inset to the bottom one,
the right inset in from the right, and the horizontal bar from the left
inset to the right one, the bottom inset up. So the bars keep clear of
what floats over the content, as a toolbar it scrolls under, as AppKit's
and UIKit's do:

```go
ui.Scroll(c).Fill().Padding(64, 16, 16).ScrollbarInsets(64, 0, 0).Children(func() { /* ... */ })
// The toolbar floats over the top 64 DIPs.
ui.Row(c).Absolute().Top(0).Left(0).Right(0).Height(64).Children(func() { /* ... */ })
```

`Scrollbars(false)` takes the bars away, and the presses on them, from a
container that draws its own chrome or shows where it is by itself, as a
carousel or a map: the wheel, `TrackScroll`, the keyboard and assistive
technology still scroll it.

`ScrollbarAxes(horizontal, vertical)` controls the two indicators and
their pointer hit targets independently. For a view with its own vertical
navigation, keep the horizontal bar and hide the vertical one:

```go
ui.ScrollBoth(c).Fill().ScrollbarAxes(true, false).Children(func() { /* ... */ })
```

Hidden axes still scroll by wheel, touchpad, `TrackScroll`, keyboard focus
and assistive technology. Hiding the axis being dragged cancels that drag
without clicking the content underneath. The remaining bar keeps its track
and leaves the shared corner clear.
`Scrollbars(false)` hides both regardless of call order; `Scrollbars(true)`
preserves the axis settings. Repeating `ScrollbarAxes` uses its last value,
and omitting it in the next build enables both axes again.

`KeepScrollbars(true)` keeps them showing while the content overflows,
with the pointer over the container or not, for a view whose place in its
content the user should see at a glance, as a long log or a diff filling
the window. They look as they do on hover, on the track if the style has
one, so nothing changes as the pointer comes over them, and take presses
as they do then; `Scrollbars(false)` and `ScrollbarAxes` still hide them.
All three apply to `Scroll`, `ScrollHorizontal`, `ScrollBoth`, `List` and
`Table`, are left at their defaults by leaving them out, and are not inherited:

```go
ui.List(c, &app.rows, len(app.lines), app.line).Fill().KeepScrollbars(true)
```

An element that scrolls itself rather than through a container, as the
code editor and the terminal do, draws its bars with `ui.Scrollbar`, so
that they look as the containers' do and follow the theme: `Painter.Scrollbar`
paints the track and thumb, and `Thumb` and `OffsetAt` map a drag of it to
an offset. A view that draws marks on its bar paints `ScrollbarTrack`,
then the marks, then `ScrollbarThumb`. Show it as a container does, while
the pointer is over the element or drags the thumb, or always where the
app keeps the bars, as the code editor's `KeepScrollbars` does:

```go
bar := ui.Scrollbar{Box: r, View: r.H, Content: contentH, Offset: scrollY}
if hovered || dragging {
	p.Scrollbar(bar, ui.ScrollbarState{Hovered: hovered, Dragging: dragging})
}
```

An element inside that zooms or pans with the wheel, the touchpad and
pinches, as a chart, takes them from the container: see
[Scrolling and pinching](input.md#scrolling-and-pinching).

## Accessibility

Assistive technology sees a scroll area, which it scrolls to show what it
reads.
