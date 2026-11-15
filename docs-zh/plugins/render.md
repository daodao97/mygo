# Render

render 插件把原生 UI 做成视频：合成（composition）是 `ui` 包的一个视图，它根据帧号构建视频的每一帧；插件像绘制窗口的帧一样在 GPU 上绘制这些帧，没有 GPU 时在 CPU 上绘制，再用系统的编码器编码，有硬件编码器时用硬件。发布视频、更新日志短片、让数据动起来的图表、录屏的字幕：Remotion 用 Chrome 把网页做成的东西，这里用应用已有的视图来做。不运行任何浏览器：一帧的开销就是窗口一帧的开销，一两毫秒的 CPU 时间，由 GPU 绘制。全部用 Go 实现：没有 JavaScript 包，不需要调用 `mygo.Use`，也不需要窗口或 `mygo.App`。

[examples/render](../../examples/render) 制作一段带声音的六秒宣传视频，加上 `-preview` 时在窗口中播放它。

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

## 合成

`Composition` 就是视频：`Width` 和 `Height` 以像素为单位，`FPS`（默认 30；29.97 对编码器而言是 30000/1001），以 `Frames` 计的长度，以及构建一帧的 `View`。`Scale` 是一个 DIP 对应多少像素，就像窗口的缩放比例：缩放比例为 2 的 1920×1080 视频，其视图按 960×540 DIP 布局，如同 Retina 显示屏上的窗口，文字和控件都是平常的大小。`Dark`、`Preferences` 和 `Locale` 让视图按桌面的外观、设置和语言构建。

`Background` 填充视图后面的帧，默认是主题的背景。`Transparent` 的合成则让视图没有绘制的部分保持透明，用于保留 alpha 的 ProRes 4444 和 VP9，以及 PNG；其他编解码器显示为黑色。

## 帧与时间

视图会拿到一个 `Frame`：从 0 开始的 `Index`、帧数 `Count` 和 `FPS`，以及 `Time`、`Seconds` 和 `Progress`（第一帧为 0，最后一帧为 1）。视图显示的内容由帧决定：

- `f.Interpolate(in, out)` 把帧的序号从 `in` 中的帧映射到 `out` 中的值，沿着 `Interpolation` 的 `Easing`（`ui` 的缓动之一，默认线性），超出范围时取端点值，除非 `Left` 或 `Right` 指定 `Extend` 或 `Identity`。`render.Interpolate` 映射任意数，`render.InterpolateColor` 映射到颜色。
- `f.Spring(ui.Bouncy.Spring)` 是一个从 0 到 1 的弹簧在这一帧所处的位置，过冲时超过 1：`ui` 的弹簧（`ui.Smooth`、`ui.Snappy`、`ui.Bouncy`，或任意 `ui.SpringOptions`，感知参数或物理参数皆可），轨迹与 `ui` 的动画相同。`f.SpringIn(o, 30)` 把它拉伸为在 30 帧内静止。
- `f.Sequence(c, from, count, view)` 在从 `from` 开始的 `count` 帧内显示 `view`，放在一个铺满合成的图层里，叠在之前的图层之上，就像视频编辑器叠放片段；它的帧从 `from` 处的 0 开始计数。`f.Shift(from, count)` 给出同样的帧而不加图层，`f.Loop(n)` 给出每 `n` 帧循环一次的帧，`render.Layer(c)` 是一个单独的图层。

```go
View: func(c *ui.Context, f render.Frame) {
	f.Sequence(c, 0, 60, func(f render.Frame) { title(c, f) })
	f.Sequence(c, 45, 0, func(f render.Frame) { // to the end
		s := f.Spring(ui.Snappy.Spring)
		card(c).Translate(0, float32(80*(1-s)))
	})
},
```

每一帧都在它自己的时间重新构建，视图的时钟读到的就是这个时间，所以 `ui` 自己的动画也随视频移动：`Transition`、开关的打开、元素上的 `AnimateTo` 和 `Spring`、`Context.After`。无论一帧需要多长时间制作，它们都处在屏幕上那一时刻应在的位置。由于它们从之前的帧显示的状态继续移动，只渲染部分帧（`Options.From`）时，会先构建之前的帧，但不绘制。

视图每次构建同一帧时必须得到相同的结果：`Video` 在绘制之前会先把所有帧构建一遍来混音，预览在跳转时也会再次构建。渲染之前先加载视图显示的内容，图片、字体、数据；从帧读取时间，而不是用 `time.Now`。

## 运动模糊

`MotionBlur` 像相机快门一样模糊运动的内容，和 Remotion 的 `CameraMotionBlur` 相同：每一帧是 `Samples` 个帧的平均，它们构建在该帧之前快门打开的时间内，快门时长为帧时长的 `Shutter` 度（默认 180，即一半）。

```go
comp.MotionBlur = render.MotionBlur{Samples: 6}
```

采样的时间落在帧与帧之间，`f.Exact()` 给出带小数的帧位置：随 `Interpolate`、`Spring`、`Seconds` 或 `Exact` 运动的内容会被模糊，跟随时钟的 `ui` 自带动画也会；随整数 `f.Index` 运动的内容则不会。每个采样都要构建并绘制，所以一帧的开销是 `Samples` 倍；采样在内存中用所有核心求平均。

## 声音

`f.Audio(sound)` 在这一帧播放一段声音，从它在合成（或播放它的序列）的这一帧所处的位置开始：在 `Sequence` 中播放的声音随序列一起开始。视图在每个听得到声音的帧里播放它，就像在每个看得到元素的帧里显示它：

```go
music, err := render.OpenSound("music.mp3") // with _ "github.com/egoist/mygo/audio/mp3"
...
f.Audio(music, render.AudioOptions{Volume: f.Interpolate([]float64{0, 30}, []float64{0, 1})})
```

`AudioOptions` 设置它的 `Volume`（可以逐帧变化，并在每一帧内平滑过渡）、开始播放的位置 `Start`，以及 `Loop`。`OpenSound` 和 `DecodeSound` 把 `audio` 包注册的格式（WAV；导入相应的包后还有 MP3、Ogg Vorbis、FLAC 和 AAC）解码到内存；`SoundOf` 接受应用自己生成的采样；`NewSound` 解码一个 `audio.Decoder`。任意采样率的声音都会重采样为 48 kHz 立体声，混音后以 AAC 编码进视频，WebM 中则用 Opus。帧中没有播放声音的视频没有音轨。

## 渲染

- `Video(ctx, comp, path, options)` 渲染为视频文件：MP4（`.mp4`、`.m4v`）、QuickTime 影片（`.mov`）或 WebM（`.webm`）。视频完整后文件才会出现；在此之前，帧写入旁边的一个文件，出错或 `ctx` 结束时会删除它。
- `Images(ctx, comp, "frames/%04d.png", options)` 把每一帧写成 PNG（透明合成会保留 alpha）或 JPEG（`.jpg`，质量为 `Options.Quality`），在后续帧绘制的同时用所有核心编码。
- `Still(comp, frame, options)` 以 `*image.RGBA` 返回一帧，`Frames(ctx, comp, options, fn)` 把每一帧交给 `fn`，用于自己的编码器。

`Options` 选择帧，从 `From` 到 `To` 之前；`Renderer`：`Auto`（GPU，没有 GPU 渲染器可用时用 CPU）、`GPU` 或 `CPU`；`Codec`、`Bitrate` 和 `AudioBitrate`；以及 `Progress`，每帧之后调用，告知渲染进度和绘制者，例如 `Metal (Apple M2)`。取消 `ctx` 会停止渲染。

## 编解码器与平台

帧在 macOS 上由 Metal 绘制，在 Windows 上由 Direct3D 11 绘制，在 Linux 上由 OpenGL 通过不需要显示器的 EGL（Mesa 的 surfaceless 平台）绘制。在 macOS 上，Metal 把每一帧直接画进编码器接收的像素缓冲区，也就是一个 IOSurface：没有任何帧经过内存复制。没有 GPU、只有软件光栅化器的系统（Linux 上的 llvmpipe、Windows 上的 WARP），在 `Auto` 下用 MyGo 自己的 CPU 渲染器绘制，它只重绘帧与帧之间变化的部分，在这种情况下更快。

| 编解码器 | macOS（AVFoundation） | Windows（Media Foundation） | Linux（GStreamer） |
| --- | --- | --- | --- |
| `H264`，默认 | MP4、MOV | MP4 | MP4、MOV |
| `HEVC` | MP4、MOV | MP4，需要 HEVC 视频扩展 | MP4、MOV |
| `ProRes`、`ProRes4444` | MOV | – | – |
| `VP9` | – | – | WebM |

在 Linux 上，由该编解码器的编码器中第一个能编码测试帧的来编码视频：硬件的 VA-API 或 NVENC，否则是 x264、x265 或 OpenH264（`gstreamer1.0-plugins-ugly`），以及 `gstreamer1.0-plugins-bad` 的解析器和 `gstreamer1.0-libav` 的 AAC 编码器。系统无法编码的内容会返回一个包装了 `errors.ErrUnsupported` 的错误，其中写明需要安装什么。

`Options{Encoder: render.FFmpeg}` 改用 ffmpeg 编码（`Options.FFmpeg` 指定它，默认是 `PATH` 中的 `ffmpeg`）：任意编解码器写入任意容器。帧通过管道传给它；绘制仍由 MyGo 完成。H.264 在未设置 `Bitrate` 时使用 libx264 的 CRF 18。

## 预览

`render.NewPlayer(comp)` 返回合成的 `ui.VideoPlayer`，用 `ui.Video` 在窗口中显示，便于在制作视频时预览：它的帧就是视频的帧，到期时在 CPU 上绘制；跳转时，会构建目标帧之前的帧而不绘制它们。它不播放声音。

```go
player, err := render.NewPlayer(comp)
...
ui.Video(c, player).FillWidth()
```

`go run ./examples/render -preview` 显示它，并提供暂停和跳转的控件。

## 速度

一个包含 24 张卡片、带阴影、渐变和文字的仪表盘，1920×1080、每秒 60 帧，在 Apple M5 上用 Metal 和硬件编码器以每秒 242 帧的速度渲染，每帧使用 1.7 毫秒 CPU 时间（包括构建）：0.4 个核心。CPU 渲染器用全部核心以每秒 167 帧制作同一个视频，每帧 34 毫秒 CPU 时间。

GPU 渲染器在 CPU 上计算的部分，视频的每一帧都要付出代价：带变换的裁剪和遮罩的覆盖率，渲染器会保留它们，直到裁剪或遮罩移动。3D 透视下的裁剪（例如倾斜进入的窗口）覆盖整个画面；在每帧都移动的元素上的遮罩（例如滚动的网格）每帧都要重新计算。对这种窗口，让其中的内容自己画圆角，而不是裁剪；把遮罩放在静止的父元素上，让运动的内容在它里面移动：一段 34 秒的宣传视频这样改了两处之后，CPU 时间从 106 秒降到 7 秒。
