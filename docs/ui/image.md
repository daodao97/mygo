# Image

<!-- preview image 240 -->

`ui.Image` shows a bitmap, or an SVG in its own colors, by default at its
size in DIPs, scaled to fit when given another size.

```go
//go:embed logo.png
var logoPNG []byte

var logo, _ = ui.DecodeBitmap(logoPNG)

ui.Image(c, logo).Size(64, 64).Fit(ui.Contain).Radius(12)
```

## Bitmaps

`ui.NewBitmap` makes a bitmap of an `image.Image`, and `ui.DecodeBitmap` of
PNG, JPEG, GIF (its first frame), WebP or BMP data, turning photos upright
as their camera's EXIF orientation says. Make bitmaps once, not in the
view: MyGo keeps a bitmap on the GPU as long as you use it. A bitmap shown
much smaller than its pixels, as a photo in a thumbnail, is drawn from a
copy halved as many times as that keeps it no smaller, made once, so that
it shows every pixel's part rather than shimmering.

A bitmap keeps its pixels on the CPU as well as on the GPU, which
applications full of photos notice. Once a bitmap has shown,
`ReleasePixels` drops its CPU copies, of the picture and of the halved
ones: it draws as before from the GPU, and the pixels are made again when
something needs them, as a GPU that lost its textures or let one go after
frames without it, the CPU renderer, or a halved copy for drawing it
smaller, and a GPU's upload drops them again. A bitmap `DecodeBitmap` made
decodes them again from the data it took, which costs a fraction of the
pixels. One `NewBitmap` made converts again the `image.Image` it was made
of, which it keeps from `ReleasePixels` on, without a copy: for an image
the app keeps anyway, as a screenshot it works on at full size, the
pixels are then in memory once rather than twice. Either must stay as it
was. A bitmap `NewBitmap` made of an image the app no longer keeps (or of
an image type other than package `image`'s), a canvas's, or one `Update`
changed has nothing else to make its pixels from, and keeps them. Call
`ReleasePixels` on the main thread, as drawing does:

```go
cover, _ := ui.DecodeBitmap(fileData)
// … once the cover has shown:
cover.ReleasePixels()

shot := ui.NewBitmap(capture) // the app keeps capture
// … once the shot has shown:
shot.ReleasePixels()
```

A window that closes lets go of what it showed: its view, the elements and
the scene of its last frame, and the bitmaps they drew. A bitmap then stays
only as long as the app keeps it.

`Update` shows another picture in a bitmap, for one that changes as video
does. Of the same size, it keeps the bitmap's texture on the GPU, which
uploads the new pixels, where a new bitmap each frame would make a texture
each. An `*image.RGBA` made by `image.NewRGBA` is taken without a copy:
leave its pixels as they are until the next `Update`. Frames decoded on a
goroutine reach the bitmap through `Window.Update`, which redraws the
window:

```go
go func() {
	for frame := range frames { // *image.RGBA, a new one each time
		win.Update(func() { app.video.Update(frame) })
	}
}()
```

[`ui.Video`](video.md) plays a video's frames so, decoded ahead and each
shown as it comes due, with animated GIFs and PNGs out of the box; video
files and streams play with the system's media engine (`ui/video`), their
frames staying on the GPU.

`UpdateYUV` shows a frame of video in Y′CbCr, as decoders give it, without
converting it to RGBA. A `ui.YUVImage` holds its planes, each in rows of
its stride: NV12's luma, then its chroma with Cb and Cr interleaved, as
hardware decoders leave them, or I420's luma, Cb and Cr, as software
decoders do; with the matrix that converts them (`ui.BT601`, `ui.BT709` or
`ui.BT2020`) and whether they take the full range. The GPU uploads the
planes, 1.5 bytes a pixel against RGBA's 4, and converts them as it
samples them, which leaves the CPU nothing to convert, most of what
showing a frame otherwise costs it. The planes are taken without a copy,
but for I420's chroma, which the bitmap interleaves into a plane of its
own: leave them as they are until the next update.

```go
win.Update(func() {
	app.video.UpdateYUV(&ui.YUVImage{
		Format: ui.NV12, Width: 1920, Height: 1080,
		Planes:  [3][]byte{luma, chroma},
		Strides: [3]int{lumaStride, chromaStride},
		Matrix:  ui.BT709,
	})
})
```

Such a frame shows at its size, which the GPU scales as it samples it,
without the halved copies a bitmap shown much smaller than its size gets:
have the decoder give the size the frame shows at, where it can. The CPU
renderer, and captures, convert the frame as they draw it.

`Pixelated` draws a bitmap shown larger than its pixels as squares of
their colors, as pixel art and game sprites are drawn, where it would
blend neighbors smoothly. At whole multiples of its size each pixel is a
block of whole screen pixels. At other sizes, such as a zoom of 2.5, the
squares keep one size, their edges blending across a single screen pixel
rather than some squares coming out a pixel wider than others, and they
move smoothly rather than in jumps, as Ebitengine's `FilterPixelated`
draws. Shown smaller, it is as smooth as any.

```go
tiles, err := ui.DecodeBitmap(tilesPNG)
if err != nil {
	log.Fatal(err)
}
tiles.Pixelated()
ui.Image(c, tiles).Size(64, 64) // 16×16 pixels, each 4×4
```

## Fitting

`Fit` says how an image fills its box: `Contain` (the default) fits it
inside, `Cover` covers the box and crops the rest, `FillBox` stretches it,
`ScaleDown` shows it at its own size unless that does not fit, and
`NaturalSize` at its own size, cropped. `Grayscale` draws it in shades of
gray, as for what is disabled. `Radius` rounds its corners, and
`AspectRatio` keeps its proportions in a box that grows.

```go
ui.Image(c, photo).AspectRatio(16.0 / 9).Fit(ui.Cover).Radius(8)
```

`ObjectPosition(x, y)` places the picture where `Fit` crops it or leaves
room around it, as CSS's `object-position`: fractions of that room across
and down, 0 by the left or top edge, 1 by the right or bottom one, and
0.5, the default, in the middle. Its edge keywords are 0 and 1: `left
top` is `ObjectPosition(0, 0)`. An avatar whose face, near the top of the
photo, stays in view, as `object-position: 50% 15%`:

```go
ui.Image(c, photo).Size(40, 40).Radius(20).Fit(ui.Cover).ObjectPosition(0.5, 0.15)
```

## SVGs

`ui.ParseSVG` parses an SVG, and `ui.MustParseSVG` one that is part of the
program, such as an embedded file, panicking when it is in error. `Image`
shows an SVG in its own colors, with the text color for its
`currentColor`, at the size the SVG gives (its `width` and `height`, or
those of its `viewBox`) unless given another:

```go
ui.Image(c, illustration).Width(240)
```

To show an SVG in the color of the text, as an icon, use [Icon](icon.md).

MyGo draws an SVG's shapes into the GPU's atlas once for each size it shows
at, the way it draws text, and the GPU draws them from there: moving an
image, scrolling it or changing its color draws nothing again. MyGo draws:

- paths and the basic shapes, groups, nested `svg` elements, `use` and
  `symbol` elements, and transforms;
- fills and strokes (with their joins, caps and dashes) of colors,
  `currentColor` and linear and radial gradients, opacity, clip paths and
  masks;
- markers (`marker-start`, `marker-mid`, `marker-end` and `marker`) on
  paths, lines, polylines and polygons, with `markerWidth`,
  `markerHeight`, `refX`, `refY`, `viewBox`, `markerUnits` and `orient`
  (an angle, `auto` or `auto-start-reverse`), clipped to their viewport
  unless their `overflow` is visible, in `context-fill` and
  `context-stroke` too;
- text: `text`, `tspan` and `a` elements in the system's fonts, with
  `font-family`, `font-size`, `font-weight`, `font-style`, the `font`
  shorthand, `letter-spacing`, `text-anchor`, `dominant-baseline` and
  `alignment-baseline`, `x`, `y`, `dx` and `dy` (lists of them too, one
  per character), white space as CSS and `xml:space` collapse it, and
  fills and strokes of any paint;
- style sheets and `style` attributes, with selectors of types, classes,
  ids and `:root` joined by descendant and child combinators, custom
  properties (`var(--name)` and `var(--name, fallback)`, inherited as in
  CSS) and `@media` queries of `prefers-color-scheme`.

Text is laid out and rasterized by the same text stack as the rest of the
window, a line at a time, in its place among the shapes: what is drawn
after a label covers it. Its glyphs are drawn at the size they show at,
so text stays as sharp as other text, and rotated or skewed text is
sampled from upright glyphs. Its stroke follows the edges of the glyphs'
pixels, as text's strokes do in MyGo (`TextStroke`), so a stroke wider
than a few pixels is rounder at corners than a browser draws it.

An SVG made for a page that may be light or dark, as the diagrams of
README files are, draws in the colors of its `prefers-color-scheme: dark`
media queries where the theme is dark (`Theme.Dark`), as a browser follows
the system's appearance, and redraws as the theme changes. `WithDark`
returns the SVG fixed to one scheme, for an app whose own setting, or the
background behind the picture, says otherwise:

```go
diagram := ui.MustParseSVG(data)
ui.Image(c, diagram)                // light or dark, as the theme is
ui.Image(c, diagram.WithDark(true)) // always the dark colors
```

`SVG.Image`, which draws pixels outside a window, draws the light scheme
unless the SVG was made with `WithDark`.

MyGo leaves out embedded images, patterns, filters, blend modes, text on
a path, SMIL animation and `@font-face`, and style sheet rules whose
selectors have other combinators, attribute selectors or pseudo-classes.
`Unsupported` lists what an SVG has that MyGo leaves out, empty when it
draws all of it, so that an app showing SVGs it did not make, as in a
diff, can show a note, or another view, instead of a picture that is not
quite right:

```go
s, err := ui.ParseSVG(data)
if err != nil {
	return err
}
if missing := s.Unsupported(); len(missing) > 0 {
	// e.g. ["<image>", "@font-face", "filter", "selector .a + .b"]
	note = "Not shown: " + strings.Join(missing, ", ")
}
```

In `Draw` callbacks, `Painter.Image` draws bitmaps and SVGs too, in the
Painter's theme.

## Pictures of any format

`ui.DecodeImage` decodes a picture whose format isn't known up front, as
one downloaded from a link: a bitmap when `DecodeBitmap` knows its format,
else an SVG of a size. `ui.ImageSize` gives the size `Image` shows a
bitmap or an SVG at by default, in DIPs, to size it to its aspect ratio:

```go
pic, err := ui.DecodeImage(data) // once, as it arrives, not in the view
if err != nil {
	return err
}
// … in the view:
if w, h := ui.ImageSize(pic); w > 0 && h > 0 {
	ui.Image(c, pic).MaxWidthPercent(100).AspectRatio(w / h)
}
```

## Accessibility

Images are decorations that assistive technology does not see, unless
`Label` names them, as a photo that matters.

## Pictures from the network

`ui/netimage` downloads, caches and shows them, with a placeholder until they
come:

```go
netimage.Image(c, url, netimage.Options{MaxSide: 240}).Size(80, 80).Radius(8)
```

They stay in memory while there is room and on disk across runs, as their
servers' caching headers say. `MaxSide` bounds the bitmap kept: give about what
it shows at, so that a list of thumbnails doesn't hold its photos. `netimage.Load`
returns the picture to draw it some other way, `netimage.Preload` downloads what's
about to show, and `netimage.Forget` and `netimage.Clear` let go of them.
