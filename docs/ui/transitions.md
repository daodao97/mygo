# Transitions

<!-- preview transitions 230 -->

`Transition` makes an element move smoothly where the layout changes it:
from where the last frame showed it to where it is now, in its parent, at
its size, and in its background and border colors. The view says only where
things are; the transition animates the way there.

```go
var rowMotion = ui.ElementTransition{
	Enter: &ui.Motion{Collapse: true},
	Exit:  &ui.Motion{Collapse: true},
}

ui.Column(c).Children(func() {
	removed := -1
	for _, it := range app.items {
		ui.Row(c).Key(it.ID).Padding(8).Transition(rowMotion).Children(func() {
			ui.Text(c, it.Title).Grow(1)
			if ui.Button(c, "Remove").Clicked() {
				removed = it.ID // once the loop is done (see Views)
			}
		})
	}
	if removed >= 0 {
		app.remove(removed)
	}
})
```

Rows added grow in, rows removed collapse, and the others slide to their
places, whether the items were added, removed, sorted or shuffled.

## What moves

An `ElementTransition` animates over its `Duration` (200 ms when zero),
along its `Ease` (`ui.EaseOut` when nil, or any [easing](drawing.md#animation)),
or as its `Animation` says, when set: with a spring, such as `ui.Snappy`,
an element sent elsewhere on its way turns smoothly, keeping its velocity
(see [Motion](motion.md)):

- **Position**: where the element is in its parent. Moving with its parent,
  as when a scroll view scrolls, is not a change.
- **Size**: its width and height. The content lays out at each size on the
  way, as a panel opening wraps its text anew.
- **Colors**: its background and border colors, as a hover fades in and
  out.

All three animate unless `Position`, `Size` or `Colors` choose some:

```go
tile := ui.Box(c).Size(64, 48).Radius(8).Background(t.Surface)
if tile.Hovered() {
	tile.Background(t.SurfaceHover)
}
tile.Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
```

The element needs the same ID from frame to frame: a `Key`, or the same
place among its siblings. A transition animates the element alone: the
elements that a change moves too need a transition of their own to move
smoothly, the same one to move in step, as the rows above, and the
column holding them if its size changes:

```go
open := app.sidebar
w := float32(56)
if open {
	w = 240
}
ui.Row(c).Fill().Children(func() {
	ui.Column(c).Width(w).ClipX().Transition(ui.ElementTransition{Size: true})
	ui.Column(c).Grow(1).Transition(ui.ElementTransition{})
})
```

A `Delay` in the `Animation` holds changes for a while: delays growing
with the index stagger the rows of a list:

```go
for i, it := range app.items {
	a := ui.Snappy
	a.Delay = time.Duration(i) * 25 * time.Millisecond
	ui.Row(c.Key(it.ID)).Transition(ui.ElementTransition{Animation: &a, Enter: &ui.Motion{Y: 8}})
}
```

`Transitioning()` reports whether the element's transition was moving it
in the frame before.

## Coming and going

`Enter` is where an element comes from as it appears, and `Exit` where it
goes as it leaves, from its place in the layout: a `Motion` moved by `X`
and `Y`, at an `Opacity` (0, transparent, when not set), scaled by `Scale`
(0 keeps it at its size) and turned by `Rotate` degrees around its
`TransformOrigin`, and with `Collapse`, without height in a column or width
in a row, its content clipped.

```go
toast := ui.ElementTransition{
	Enter: &ui.Motion{Y: 12},
	Exit:  &ui.Motion{Opacity: 1, X: 300},
}
popup := ui.ElementTransition{
	Animation: &ui.Snappy,
	Enter:     &ui.Motion{Scale: 0.95},
	Exit:      &ui.Motion{Scale: 0.95},
}
```

An element appears with its `Enter` only in a parent that was there in the
frame before, so that a page or a dialog showing all at once does not
animate every element in it. While it comes in, the layout moving it, as
rows landing above it, moves it at once where its transition does not
animate (`Position`, `Size`), and resizing it resizes its `Collapse` in
proportion: only its `Enter` plays. When an element is no longer built
while its parent still is, a copy of it, as the last frame showed it, goes
to its `Exit` under its siblings: it takes no room, so the siblings move
into its place at once, smoothly if they have transitions, and it takes
neither the pointer nor the focus. Built again before its exit ends, the
element comes back from where its copy was.

## Shared elements

`Match` gives an element an identity it shares with another, as a photo's
thumbnail in a grid and the photo in its detail view. An element with a
`Match` appearing where one with the same `Match` showed in the frame
before comes from there, at that one's size and in its colors, even from
another parent, as SwiftUI's `matchedGeometryEffect` and Motion's
`layoutId` do:

```go
type photoMatch struct{ id int }

if app.open == nil {
	for _, p := range app.photos {
		thumb := ui.Image(c.Key(p.ID), p.Thumb).Size(64, 64).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Match: photoMatch{p.ID}})
		if thumb.Clicked() {
			app.open = p
		}
	}
} else {
	ui.Image(c.Key("hero"), app.open.Full).Size(320, 240).
		Transition(ui.ElementTransition{Animation: &ui.Snappy, Match: photoMatch{app.open.ID}})
}
```

Use a type of your own for the identity, so that it is no other's. The
element appearing goes from the other's box to its own, its content laid
out at each size on the way, as `Size` lays it out, so nothing in it
stretches, its rounded corners included. Give one element a `Match` at a
time; the one gone goes as its own `Exit` says, if it has one.

### Flying

`Fly` flies the element appearing from the other, as Keynote's Magic Move
and Flutter's Hero do:

```go
tr := ui.ElementTransition{Animation: &ui.Snappy, Match: photoMatch{p.ID}, Fly: true}
```

On its way, the element appearing:

- goes from the other's box to its own, its content laid out at each size
  (or scaled, with `ScaleContent`, below);
- goes from the other's look to its own: its background and border
  colors, its rounded corners, its opacity, the color of its text, and its
  transform, turning the short way round, in 3D and in perspective too,
  as CSS interpolates transforms, and from the transforms of the
  elements around the other, as a container scaled, to those around it;
- paints over the rest of the window, or of the overlay it is in, as a
  dialog, under the overlays after it, and takes the pointer where it
  paints: out of the clips of the scroll views and containers around it,
  over the elements after it, as opaque as it is itself, whatever the
  opacity of the elements around it, and in the theme of the `Themed`
  around it;
- moves its content as one, smoothly, rather than each line of its text a
  pixel at a time, as text at rest once there;
- fades in over the element it came from, if that one has `Fly` too,
  which goes with it, laid out at each box too, fading out: the one coming
  is opaque by half the way, and the other fades over the second half, as
  Motion crossfades. Gone from the view, that element goes as a copy of
  itself, rather than as its own `Exit` says; still built, as in a page
  going away, it goes itself.

Removed on its way with the elements around it, as its view closes with
an `Exit`, the element flies on in their copy: from where it shows to
where that copy has it, fading as it fades.

### Everything else

Keynote's Magic Move fades what the slides do not share as what they share
moves. Do it between two views by giving each a key, in a parent that
stays, with an `Enter` and an `Exit` that fade: what the views share flies
from one to the other, and the rest fades out and in. The copy of the view
going away shows nothing of the elements that fly with the other's.

```go
fade := ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{}, Exit: &ui.Motion{}}
ui.Box(c.Key("stage")).Grow(1).Children(func() {
	if app.open == nil {
		ui.Column(c.Key("grid")).Absolute().Fill().Transition(fade).Children(app.grid)
	} else {
		ui.Column(c.Key("detail")).Absolute().Fill().Transition(fade).Children(app.detail)
	}
})
```

A [`Router`](navigation.md#transitions) does it for its pages with
`TransitionMatch`.

### Scaled content

`ScaleContent` shows the element's size changing by scaling it from its
own size, laid out once, rather than laying out its content at each size:
its text keeps its lines, growing or shrinking with it, as a title into a
heading, and a picture stretches with its box. It scales the element of a
`Match` on its way, and changes of `Size`.

### Text that morphs

A text flying with a `Match` and `ContentTransition{Morph: true}` morphs
from the text of the element of its `Match`: the characters both have
move from where they were to where they are, growing or shrinking to
their size and going to their color, the other characters fading out,
going with the text, and in, as Keynote's Magic Move by characters. Its
box flies as any element's of a `Match`, and its text, laid out once at
its own size, moves into it. Taken over before it got there, by the
element it came from or another of its `Match`, it morphs on from where
its characters are. Rich text morphs too: its characters go to the colors
of their spans, and its underlines, strikethroughs and highlights fade
out and in:

```go
type titleMatch struct{ id int }
title := ui.ElementTransition{Animation: &ui.Snappy, Match: titleMatch{p.ID}, Fly: true}
morph := ui.ContentTransition{Morph: true}

// In the grid.
ui.Text(c, p.Title).FontSize(12).Transition(title).ContentTransition(morph)
// In the detail view.
ui.Text(c, p.Title+" in "+p.Place).FontSize(24).Bold().Transition(title).ContentTransition(morph)
```

`Morph` morphs a text changing in place too (see
[Motion](motion.md#content-that-changes)).

## When nothing moves

Changes that resizing the window makes do not animate. While the desktop
asks for less motion (`c.Preferences().ReduceMotion`), nothing moves:
elements show where the layout puts them at once, and shared elements do
not fly. Colors still change smoothly, and an `Enter` or `Exit` that fades
without collapsing fades in place, as Apple's guidelines suggest; one that
collapses, which moves its siblings, is at once. Frames are drawn only
while something moves.

Rows of a [`List`](list.md) are placed by the list, which builds only
those in view: their content can have transitions, but the rows do not
slide, come or go with one.
