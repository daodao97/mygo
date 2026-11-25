# Split view

<!-- preview split 270 -->

`ui.Split` lays out two panes side by side, with a divider between them that
the user drags to resize them, or moves with the arrows once it has the
keyboard focus: a `*float32` is the width of the first, which the divider
keeps 40 DIPs from either edge. When the window leaves less room than
that, the first pane gives way so the second keeps its 40 DIPs, and the
first takes its size back when there is room again; the `*float32` keeps
the size the user chose. `Changed` reports a move.

```go
ui.Split(c, &app.sidebarWidth, func() { app.files(c) }, func() { app.editor(c) }).Fill()
```

`ui.SplitVertical` puts the first above the second, the `*float32` its
height:

```go
ui.SplitVertical(c, &app.consoleHeight, func() { app.editor(c) }, func() { app.console(c) }).Grow(1)
```

`ui.SplitPercent` and `ui.SplitVerticalPercent` take the percentage of
the room the first pane takes instead, from 0 to 100: both panes then grow
and shrink with the window, in proportion, as a results panel under an
editor does.

```go
ui.SplitVerticalPercent(c, &app.editorPercent, func() { app.editor(c) }, func() { app.results(c) }).Grow(1)
```

`SplitSecond` makes the `*float32` the size of the second pane instead, for
a panel at the end of the window that keeps its size while the first pane
takes the rest of the room: a chat's list of members on the right, an
inspector, or a terminal under an editor. The second pane is on the right
of `Split` (on the left right to left) and at the bottom of
`SplitVertical`. The divider moves as before, the arrows the way they
point, 40 DIPs from either edge; when the window shrinks, the first pane
gives way down to its 40 DIPs, then the second, which takes its size back
when there is room again. Of the percent variants, the percentage is the
second pane's, and `SplitReset` puts the second pane at its value.

```go
ui.Split(c, &app.membersWidth, func() { app.chat(c) }, func() { app.members(c) }).SplitSecond().Fill()
ui.SplitVertical(c, &app.terminalHeight, func() { app.editor(c) }, func() { app.terminal(c) }).SplitSecond().Grow(1)
```

Keep the size in the app's state, and save it to restore the layout when
the app starts again. The pointer shows a resize cursor over the divider.

`SplitReset` makes a double click on the divider put the first pane back at
the size the app chooses, in the split's own unit: DIPs for `Split` and
`SplitVertical`, a percentage for the percent variants. A split without it
keeps its size on a double click. The size is kept even when the window is
too small to show it, so the pane takes it back when there is room again,
as a press on the divider does.

```go
ui.Split(c, &app.sidebarWidth, func() { app.files(c) }, func() { app.editor(c) }).SplitReset(220).Fill()
```

`SplitGap` spaces the two panes apart by a number of DIPs without drawing
the 1 DIP line between them, as two cards side by side or panes that draw
their own edges: `SplitGap(0)` puts them flush with no line, and a
positive gap leaves that room between them, where the resize handle sits.

```go
ui.Split(c, &app.sidebarWidth, func() { app.files(c) }, func() { app.editor(c) }).SplitGap(8).Fill()
```

## Limits

`SplitLimits` gives each pane a minimum and a maximum size, in DIPs for
all four kinds of split (the width side by side, the height one above the
other). A zero field sets no limit, and a minimum under the 40 DIPs every
pane keeps changes nothing. The divider stops where a pane would leave its
limits, as it stops 40 DIPs from either edge: dragged past one, it waits
there until the pointer comes back over it, and the arrows go no further.
As the window changes, both panes stay within their limits: the pane
`*size` measures takes the room the other can't, at its maximum, and gives
way to the other's minimum. The limits name the panes by place, first and
second, also with `SplitSecond`. A three-column window keeps its chat
column at least 360 DIPs wide and its side panel between 280 and 480:

```go
ui.Split(c, &app.listWidth, func() { app.list(c) }, func() {
	ui.Split(c, &app.chatWidth, func() { app.chat(c) }, func() { app.panel(c) }).SplitLimits(ui.SplitLimits{
		FirstMin: 360, SecondMin: 280, SecondMax: 480,
	}).Fill()
}).SplitLimits(ui.SplitLimits{SecondMin: 360 + 1 + 280}).Fill()
```

The outer split's second pane holds the inner split, so its minimum is the
inner one's two minimums and its divider.

Where the limits can't all hold, or don't fill the room:

- a pane's minimum wins over its maximum;
- when the room is smaller than both minimums together, both panes keep
  them, and the second overflows the end of the split, where the window
  cuts it: give the window a minimum size (`WindowOptions.MinWidth`) that
  holds them;
- when the room is larger than both maximums together, both panes keep
  them, and the rest of the room, after the second, stays empty.

A size outside the limits is kept, and shows within them, until the user
moves the divider; `SplitReset`'s size is put within them too. As with
`SplitReset`, the input applies the limits of the build before, and the
layout those of the build that sets them.

## Accessibility

Assistive technology sees the divider as a splitter named "Divider",
which takes the focus and moves with the arrows. Its value is where it
is from the start of the split, also with `SplitSecond`, so that raising
it moves it toward the end.
