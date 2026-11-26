# Video

<!-- preview video 240 -->

`ui.Video` shows a video that plays: a trailer with sound and controls, a
clip looping behind the content, as a launcher's library or a music
player's hero shows one, or a preview in a grid. It shows a
`ui.VideoPlayer`, and is laid out, fitted and rounded as an
[Image](image.md) is.

A player comes from one of two places:

- [`ui/video`](#system-video) opens video files and streams (MP4 and MOV
  with H.264 and AAC everywhere, more where the system has the codecs, HLS
  streams on macOS, Windows and browsers) with the system's own media
  framework, which decodes them in hardware and plays their sound. Its
  pictures reach the window's renderer where the decoder left them, on the
  GPU.
- A decoder in Go: `ui.NewGIFDecoder` and `ui.NewAPNGDecoder` play animated
  GIFs and PNGs on every system, without sound, as backgrounds that should
  never depend on the system; a `ui.VideoDecoder` of your own plays another
  format.

```go
import "github.com/egoist/mygo/ui/video"

//go:embed trailer.mp4
var trailerMP4 []byte

app.trailer, err = video.OpenData(trailerMP4) // or video.Open(path), video.OpenURL(url)
if err != nil {
	log.Fatal(err)
}

// In the view:
ui.Video(c, app.trailer).Fill().Fit(ui.Contain)
```

```go
//go:embed waves.gif
var wavesGIF []byte

d, err := ui.NewGIFDecoder(wavesGIF)
if err != nil {
	log.Fatal(err)
}
app.waves, err = ui.NewVideoPlayer(d)
if err != nil {
	log.Fatal(err)
}
app.waves.SetLoop(true).SetMuted(true)

// In the view:
ui.Video(c, app.waves).Fill().Fit(ui.Cover)
```

The [video example](../../examples/video) plays a GIF behind a window's
content and a trailer with controls over it.

## Players

Make players once, not in the view, as bitmaps. A new player plays as soon
as a `Video` shows it. `Pause` holds it at the frame it shows, and `Play`
goes on from there, or plays it again from its start once it ended;
`Playing` tells which, and `Ended` whether it played to its end, where it
shows its last frame. `SetLoop` plays it again each time it ends, without
a gap. `Seek` shows the frame at a time and plays on from there, unless
paused; `Position` and `Duration` tell where it plays and how long it is
(0 for a live stream). `SetRate` plays it faster or slower, its sound at
the same pitch; `SetVolume` and `SetMuted` set its sound. `Err` tells what
stopped it before its end, as a file that does not decode or a stream that
broke off. `Close` stops it, and its sound. Call a player's methods on the
main thread, in the view or an input handler, and show it in one `Video`
at a time.

```go
label := "Pause"
if !p.Playing() {
	label = "Play"
}
if ui.Button(c.Key("play"), label).Clicked() {
	if p.Playing() {
		p.Pause()
	} else {
		p.Play()
	}
}
pos := p.Position().Seconds()
if ui.Slider(c.Key("seek"), &pos, 0, p.Duration().Seconds()).Label("Position").Changed() {
	p.Seek(time.Duration(pos * float64(time.Second)))
}
```

`ui.NewVideoPlayer` decodes the first frame at once, which `Video` shows
from its first frame and which reports a video that does not decode. A
system's video opens at once and loads in the background: `Ready` is false
and `Size` 0×0 until its first picture came, and `Buffering` is true while
a stream waits for data, for the view to show a spinner. Each change of
these builds a frame of the window, so the view never polls.

## Playing

A player decodes its frames on a goroutine of its own, a couple ahead of
the one showing, into frames it uses again, and each frame reaches the
screen through a [bitmap's `Update`](image.md#bitmaps) as it comes due:
the window draws one frame for each of the video's, which `After` asks
for, and none between. When the decoder falls behind, the window waits
for it without drawing, and of the frames that came due meanwhile shows
only the last, so the video keeps to the clock rather than lag behind it.

A system's video keeps the clock its sound plays by. As each frame of the
window builds, the player takes the picture due as the frame shows, where
the decoder left it, and the window draws again as the next comes due: a
24 fps video on a 120 Hz display draws 24 frames a second.

A muted video is a picture, as a background is. It moves on only while
the view builds it and some of the window shows: one that went out of the
view, as on another tab, or in a window that is minimized, hidden or
covered by other windows (on macOS, and in browsers), goes on from where it
stood once it shows again, and decodes nothing meanwhile. While the desktop
asks for less motion (`Preferences.ReduceMotion`), it stands still: a
decoder's at its first frame. A video with sound plays on out of sight, as
a browser tab's does, and plays under less motion too, as the user asked
for it; a decoder's video has no sound, and is always the former kind.

## Decoders

`ui.NewGIFDecoder` decodes animated GIFs, in Go, as browsers show them:
each frame drawn over what the ones before it left, as its disposal says,
and a frame of 10 ms or less shown for 100 ms. The GIF decodes into memory
at once, a byte a pixel for each frame, and each frame is drawn as it
plays. It plays once or loops as `SetLoop` says, whatever loop count the
GIF gives.

A `ui.VideoDecoder` of your own plays another format: `Next` returns the
next frame, a picture of any image type and how long it shows, or `io.EOF`
after the last, and `Rewind` goes back to before the first. The player
copies each picture, so a decoder may draw every frame into the same one,
and calls the decoder from one goroutine at a time. A live stream that an
app decodes itself, as with FFmpeg, reaches the screen through a bitmap's
[`UpdateYUV`](image.md#bitmaps) instead, its frames in Y′CbCr as the
decoder gives them, which the GPU converts.

```go
type frames struct {
	pics []*image.RGBA
	next int
}

func (f *frames) Next() (ui.VideoFrame, error) {
	if f.next == len(f.pics) {
		return ui.VideoFrame{}, io.EOF
	}
	f.next++
	return ui.VideoFrame{Image: f.pics[f.next-1], Duration: time.Second / 30}, nil
}

func (f *frames) Rewind() error { f.next = 0; return nil }
```

`ui.NewAPNGDecoder` decodes animated PNGs the same way, each frame drawn
as its disposal and blending say, and keeps each frame compressed until it
plays. A PNG that is not animated is a video of one frame.

A decoder that also has a method `Duration() time.Duration` reports the
video's length through `Duration`; GIFs' and APNGs' do. `Seek` on a
decoder's video decodes from its start up to the frame, unless the decoder
is a `ui.VideoSeeker`, whose `Seek` goes to the frame at once: the player
then decodes that frame alone, as [the render plugin's
player](../plugins/render.md#previewing) does.

## System video

Package `ui/video` plays with the system's media framework. Apps that do
not import it link none of it.

| System   | Framework                                                     | Pictures reach the renderer as                                                                                         |
| -------- | ------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| macOS    | AVFoundation (`AVPlayer`), decoding with VideoToolbox          | `CVPixelBuffer`s in NV12 backed by IOSurfaces, which Metal wraps as textures: no copy                                  |
| Windows  | Media Foundation (`IMFMediaEngine`), decoding with DXVA        | textures in BGRA the engine converts into on its own Direct3D device and shares with the window's: a copy on the GPU   |
| Linux    | GStreamer 1.x (`playbin`), decoding with VA-API where installed | NV12 planes, 1.5 bytes a pixel, which OpenGL uploads as they are and the shader converts                               |
| Browsers | the video element                                             | the element, which WebGL uploads GPU to GPU                                                                            |

`video.Open` opens a file, `video.OpenURL` a `file:`, `http:` or `https:`
URL or an HLS playlist, and `video.OpenData` bytes in memory, as an
embedded file, without writing them to a file. `video.ErrUnsupported`
reports a system without a framework that plays video, as a Linux without
GStreamer and its base plugins or a Windows Server without its Media
Foundation feature, for the app to show a GIF instead. What formats play
is the system's: H.264 and AAC in MP4 play on all of them. Linux needs
`gstreamer1.0-plugins-base`, and `gstreamer1.0-libav` (or another H.264
decoder) for H.264; a missing decoder shows as `Err`. So does a video
whose picture the system cannot decode or read, as VP9, or HEVC in
MPEG-TS, on macOS, rather than play its sound alone; one that plays to
its end without a picture, as one of sound alone on Linux, fails then. On Linux a video over HTTP,
but HLS, is buffered in a file in the user's cache directory, at most
32 MB of it, which GStreamer removes as it stops: it seeks in it where it
asks, as in a file on disk.

Windows on Arm's MPEG-4 reader ignores a track's edit list. An H.264
video with B-frames (x264's default) has one that takes back its
reordering delay, so there each picture shows that much late, up to a few
frames behind its sound. Encode a clip without B-frames
(`-x264-params bframes=0`) where that matters, as the example's trailer
is.

On macOS, an AV1 video in MP4 whose `sdtp` box marks frames as ones no
other frame refers to, as ffmpeg marks SVT-AV1's, shows no picture after
a `Seek` until its next keyframe: AVFoundation, which can pass over such
frames as it decodes up to the time sought, gives none until then. Where
seeking matters, make such a file fragmented MP4
(`-movflags +frag_keyframe+empty_moov+default_base_moof`), which has no
`sdtp` box.

In browsers a video with sound may not play before the user interacted
with the page: it then plays muted, as the browser allows.

## Accessibility

Videos are decorations that assistive technology does not see, unless
`Label` names them, as images are.
