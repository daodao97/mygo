# Render

The render plugin makes videos of native UI: a composition is a view of
package `ui` that builds each frame of a video from the frame's number,
and the plugin draws the frames on the GPU, as a window's are, or on the
CPU where there is no GPU, and encodes them with the system's encoders,
in hardware where it has them. Launch videos, changelog clips, charts
that animate their data, captions for a recording: what a page made with
Remotion makes in Chrome, made with the views an app already has. No
browser runs: a frame costs what a frame of a window costs, a millisecond
or two of the CPU, and the GPU draws it. It is all Go: no JavaScript
package, nothing to `mygo.Use`, and no window or `mygo.App` needed.

[examples/render](../../examples/render) makes a six second promo with
sound, and plays it in a window with `-preview`.

```go
import "github.com/egoist/mygo/plugins/render"

comp := render.Composition{
	Width: 1920, Height: 1080, FPS: 30, Frames: 90, Scale: 2,
	View: func(c *ui.Context, f render.Frame) {
		ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Text(c, "Hello").FontSize(64).
				Opacity(float32(f.Interpolate([]float64{0, 20}, []float64{0, 1})))
		})
	},
}
err := render.Video(ctx, comp, "hello.mp4", render.Options{})
```

## Compositions

A `Composition` is the video: `Width` and `Height` in pixels, `FPS`
(30 by default; 29.97 is 30000/1001 to encoders), its length in
`Frames`, and the `View` that builds a frame. `Scale` is how many pixels a
DIP is, as a window's scale: a 1920×1080 video at scale 2 lays its view
out in 960×540 DIPs, as a window of a Retina display, with text and
widgets at their usual sizes. `Dark`, `Preferences` and `Locale` build
the view as the desktop's appearance, settings and language would.

`Background` fills the frames behind the view, the theme's background by
default. A `Transparent` composition leaves what the view does not paint
transparent instead, for ProRes 4444 and VP9, which keep alpha, and for
PNGs; other codecs show it black.

## Frames and time

The view gets a `Frame`: its `Index` from 0, the `Count` of frames and
the `FPS`, with `Time`, `Seconds` and `Progress` (0 at the first frame, 1
at the last). What the view shows follows from the frame:

- `f.Interpolate(in, out)` maps the frame's index from the frames `in` to
  the values `out`, along an `Interpolation`'s `Easing` (one of `ui`'s,
  linear by default), and clamped outside the range unless `Left` or
  `Right` says `Extend` or `Identity`. `render.Interpolate` maps any
  number; `render.InterpolateColor` maps to colors.
- `f.Spring(ui.Bouncy.Spring)` is where a spring going from 0 to 1 is at
  the frame, past 1 where it overshoots: `ui`'s springs (`ui.Smooth`,
  `ui.Snappy`, `ui.Bouncy`, or any `ui.SpringOptions`, perceptual or
  physical) on the trajectory `ui`'s animations take. `f.SpringIn(o, 30)`
  stretches it to come to rest in 30 frames.
- `f.Sequence(c, from, count, view)` shows `view` during `count` frames
  from `from`, in a layer filling the composition over those before it,
  as a video editor stacks clips; its frame counts from 0 at `from`.
  `f.Shift(from, count)` gives that frame without a layer, `f.Loop(n)`
  one going round every `n` frames, and `render.Layer(c)` a layer of
  one's own.

```go
View: func(c *ui.Context, f render.Frame) {
	f.Sequence(c, 0, 60, func(f render.Frame) { title(c, f) })
	f.Sequence(c, 45, 0, func(f render.Frame) { // to the end
		s := f.Spring(ui.Snappy.Spring)
		card(c).Translate(0, float32(80*(1-s)))
	})
},
```

Each frame is built anew at its own time, which the view's clock reads,
so `ui`'s own animations move with the video too: a `Transition`, a
switch turning on, `AnimateTo` and `Spring` on an element, `Context.After`.
They are where they would be on screen at that time, however long the
frame takes to make. As they move from what the frames before showed, a
render of some frames (`Options.From`) builds the frames before it,
without drawing them.

A view must build the same frame the same way each time: `Video` builds
the frames once to mix the sound before it draws them, and the preview
builds them again as it seeks. Load what the view shows, pictures,
fonts, data, before rendering, and read the time from the frame, not
from `time.Now`.

## Cores

A render draws several frames at once, each on a worker of its own:
`Options.Workers` of them, by default half the cores, up to 4. Worker
*k* of *n* draws frames *k*, *k*+*n*, *k*+2*n*…, and the frames go to
the encoder in order, into one file. Each worker builds every frame before
its last with a `ui.Recorder` of its own, drawing only its own, so what
`ui` animates is where a render on one thread has it, and the frames are
the same whatever the number of workers. Building a frame costs a
fraction of drawing it, reading it back and encoding it, which the
workers do at once: twice as many frames a second with motion blur, or
handed to `Frames`, on an Apple M5. A video without motion blur goes as
fast as the system's encoder takes frames.

The view then runs on several goroutines at once. What it reads, a
`Sound`, a picture, data loaded before rendering, it must not change;
`Workers: 1` builds and draws every frame on one thread, for a view that
keeps state of its own across frames. With more than one worker, Metal
no longer draws into the encoder's buffers itself: the frames are read
back, as Direct3D's and OpenGL's are.

## Motion blur

`MotionBlur` blurs what moves, as a camera's shutter does, as Remotion's
`CameraMotionBlur` does: each frame is the mean of `Samples` frames, built
at times spread over the time the shutter is open before it, `Shutter`
degrees of the frame's time (180, half of it, by default).

```go
comp.MotionBlur = render.MotionBlur{Samples: 6}
```

The samples' times fall between frames, where `f.Exact()` has the
fraction: what moves with `Interpolate`, `Spring`, `Seconds` or `Exact`,
and `ui`'s own animations, which follow the clock, blur; what moves with
the whole `f.Index` does not. Each sample is built and drawn, so a frame
costs `Samples` times as much; the samples are averaged in memory, on
all cores.

## Sound

`f.Audio(sound)` plays a sound during the frame, from where it is at
this frame of the composition, or of the sequence playing it: a sound
played inside a `Sequence` starts with the sequence. A view plays a
sound in each frame it is heard, as it shows an element in each frame it
is seen:

```go
music, err := render.OpenSound("music.mp3") // with _ "github.com/egoist/mygo/audio/mp3"
...
f.Audio(music, render.AudioOptions{Volume: f.Interpolate([]float64{0, 30}, []float64{0, 1})})
```

`AudioOptions` set its `Volume`, which may change from frame to frame and
moves smoothly across each, the `Start` it plays from, and `Loop`.
`OpenSound` and `DecodeSound` decode the formats package `audio`
registers (WAV, and MP3, Ogg Vorbis, FLAC and AAC with their packages
imported) into memory; `SoundOf` takes samples an app makes; `NewSound`
decodes an `audio.Decoder`. Sounds of any rate are resampled to 48 kHz
stereo, mixed, and encoded into the video in AAC, or Opus in WebM. A
video whose frames play no sound has no sound track.

## Rendering

- `Video(ctx, comp, path, options)` renders into a video file: an MP4
  (`.mp4`, `.m4v`), a QuickTime movie (`.mov`) or a WebM (`.webm`). The
  file appears when the video is whole; until then the frames go into a
  file beside it, which an error or `ctx`'s end removes.
- `Images(ctx, comp, "frames/%04d.png", options)` writes each frame into
  a PNG, with the alpha of a transparent composition, or a JPEG (`.jpg`,
  of `Options.Quality`), encoded on all cores while the next frames draw.
- `Still(comp, frame, options)` returns one frame as an `*image.RGBA`,
  and `Frames(ctx, comp, options, fn)` hands each frame to `fn`, for an
  encoder of one's own.

`Options` choose the frames, `From` up to before `To`; the `Renderer`:
`Auto`, the GPU, or the CPU where no GPU renderer starts, `GPU`, or `CPU`;
how many `Workers` draw frames at once ([Cores](#cores));
the `Codec`, `Bitrate` and `AudioBitrate`; and `Progress`, called after
each frame with how far the render is and what draws it, as
`Metal (Apple M2)`. Canceling `ctx` stops the render.

## Codecs and platforms

The frames are drawn by Metal on macOS, Direct3D 11 on Windows, and
OpenGL on Linux, through EGL without a display (Mesa's surfaceless
platform). On macOS, Metal draws each frame into the pixel buffer the
encoder takes, an IOSurface: no frame is copied through memory. A
system with no GPU but a software rasterizer, llvmpipe on Linux or WARP
on Windows, draws with MyGo's own CPU renderer under `Auto`, which
redraws only what changed from frame to frame and is faster there.

| Codec | macOS (AVFoundation) | Windows (Media Foundation) | Linux (GStreamer) |
| --- | --- | --- | --- |
| `H264`, the default | MP4, MOV | MP4 | MP4, MOV |
| `HEVC` | MP4, MOV | MP4, with the HEVC Video Extensions | MP4, MOV |
| `ProRes`, `ProRes4444` | MOV | – | – |
| `VP9` | – | – | WebM |

On Linux, the first encoder of the codec that encodes a test frame
encodes the video: VA-API's or NVENC's in hardware, else x264, x265 or
OpenH264 (`gstreamer1.0-plugins-ugly`), with `gstreamer1.0-plugins-bad`'s
parsers and `gstreamer1.0-libav`'s AAC encoder. What a system cannot
encode fails with an error wrapping `errors.ErrUnsupported`, which names
what to install.

`Options{Encoder: render.FFmpeg}` encodes with ffmpeg instead
(`Options.FFmpeg` names it, `ffmpeg` from `PATH` by default): every codec
into every container. The
frames reach it through a pipe; MyGo still draws them. H.264 is libx264's
CRF 18 unless `Bitrate` is set.

## Previewing

`render.NewPlayer(comp)` returns a `ui.VideoPlayer` of the composition,
for `ui.Video` to show in a window while the video is made: its frames
are the video's, drawn on the CPU as they come due, and a seek builds
the frames before the one it goes to without drawing them. It plays no
sound.

```go
player, err := render.NewPlayer(comp)
...
ui.Video(c, player).FillWidth()
```

`go run ./examples/render -preview` shows it with controls to pause and
seek.

## Speed

A dashboard of 24 cards with shadows, gradients and text, at 1920×1080
and 60 frames a second, renders on an Apple M5 at 242 frames a second
with Metal and the hardware encoder, using 1.7 ms of the CPU a frame,
building included: 0.4 of a core. The CPU renderer makes the same video
at 167 frames a second on all cores, 34 ms of the CPU a frame.

What the GPU renderers draw on the CPU costs every frame of a video:
the coverage of clips with a transform and of masks, which they keep
until the clip or mask moves. A clip under a 3D perspective, as a window
tilting in, covers the whole frame; a mask on an element that moves
every frame, as a scrolling grid's, is made anew each frame. Round the
corners of what is inside such a window rather than clip it, and put a
mask on a parent that stands still, with what moves inside it: a promo
of 34 seconds that did both went from 106 seconds of the CPU to 7.
