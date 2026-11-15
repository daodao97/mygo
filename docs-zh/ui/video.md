# 视频

<!-- preview video 240 -->

`ui.Video` 显示一个可播放的视频：带声音和控件的预告片、在内容背后循环播放的片段（例如启动器的资料库或音乐播放器的主视觉），或网格中的预览。它显示一个 `ui.VideoPlayer`，布局、适配和圆角方式与[图像](image.md)相同。

播放器有两种来源：

- [`ui/video`](#系统视频) 使用系统媒体框架打开视频文件和流：各平台都支持采用 H.264 和 AAC 的 MP4 和 MOV，其他格式取决于系统安装的编解码器；macOS、Windows 和浏览器还支持 HLS 流。框架负责硬件解码和音频播放，窗口渲染器直接使用解码器输出的 GPU 画面。
- Go 中的解码器：`ui.NewGIFDecoder` 和 `ui.NewAPNGDecoder` 在每个系统上播放动画 GIF 和 PNG，没有声音，适合作为需要在各系统上保持一致的背景动画；你自己实现的 `ui.VideoDecoder` 可以播放其他格式。

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

[视频示例](../../examples/video)在窗口内容背后播放 GIF，并在其上播放带控件的预告片。

## 播放器

与位图一样，播放器只创建一次，不要在视图中创建。新的播放器在 `Video` 显示它时立即开始播放。`Pause` 将它停在当前显示的帧，`Play` 从那里继续，或在它结束后从头再播放；`Playing` 说明当前是哪种状态，`Ended` 说明它是否播放到了结尾，此时它显示最后一帧。`SetLoop` 让它在每次结束时重新播放，没有间隙。`Seek` 显示指定时间的帧，并从那里继续播放，已暂停时则停在那一帧；`Position` 和 `Duration` 说明它播放到哪里以及有多长（直播流为 0）。`SetRate` 让它播放得更快或更慢，声音保持同样的音高；`SetVolume` 和 `SetMuted` 设置它的声音。`Err` 说明是什么让它在结束前停止，例如无法解码的文件或中断的流。`Close` 停止它及其声音。在视图或输入处理器中，于主线程上调用播放器的方法；并且同一时间只在一个 `Video` 中显示它。

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

`ui.NewVideoPlayer` 会立即解码第一帧，`Video` 从第一帧起就显示它；无法解码的视频也会通过它报告出来。系统视频会立即打开并在后台加载：在第一幅画面到来之前，`Ready` 为 false，`Size` 为 0×0；流在等待数据时 `Buffering` 为 true，供视图显示旋转指示器。这些状态每次变化都会构建窗口的一帧，因此视图无需轮询。

## 播放

播放器在独立的 goroutine 中提前解码几帧，复用帧缓冲区；到显示时间后，通过[位图的 `Update`](image.md#位图) 将画面送到屏幕。`After` 会为每个视频帧请求一次窗口绘制，两帧之间不绘制。解码跟不上时，窗口暂停绘制、等待解码完成；等待期间错过的帧只显示最后一帧，使播放进度与时钟同步，而不累积延迟。

系统视频以音频播放时钟为准。每次构建窗口帧时，播放器会取得该帧显示时应播放的画面，直接使用解码器的输出；到下一幅画面的显示时间，窗口才再次绘制。因此在 120 Hz 显示器上播放 24 fps 视频，每秒也只绘制 24 帧。

静音视频属于视觉效果，例如背景动画。只有当视图构建它、且窗口有一部分可见时，它才会继续播放：离开视图的视频（例如位于另一个标签页），或位于最小化、隐藏或被其他窗口遮挡的窗口中的视频（在 macOS 和浏览器中），会在再次显示时从原处继续，期间不解码任何内容。系统要求减少动效（`Preferences.ReduceMotion`）时，它会静止不动：使用解码器的视频会停在第一帧。有声音的视频在不可见时仍继续播放，如同浏览器标签页，在减少动效下也继续播放，因为用户明确要求了它；使用解码器的视频没有声音，总是前一种情况。

## 解码器

`ui.NewGIFDecoder` 用 Go 解码动画 GIF，显示规则与浏览器一致：按各帧指定的帧处置规则，在前面各帧留下的画面上绘制；时长不超过 10 毫秒的帧会显示 100 毫秒。GIF 一次性解码到内存中，每帧每像素占一个字节，播放时再逐帧绘制。是否循环由 `SetLoop` 决定，不采用 GIF 中指定的循环次数。

你自己实现的 `ui.VideoDecoder` 可以播放其他格式：`Next` 返回下一帧，一幅任意图像类型的画面，以及它显示多长时间，或最后一帧之后的 `io.EOF`；`Rewind` 回到第一帧之前。播放器会复制每一幅画面，所以解码器可以把每一帧绘制到同一幅中，并且同一时间只从一个 goroutine 调用解码器。应用自己解码的直播流，例如用 FFmpeg，改为通过位图的 [`UpdateYUV`](image.md#位图) 到达屏幕，它的帧是解码器给出的 Y′CbCr 格式，由 GPU 转换。

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

`ui.NewAPNGDecoder` 以同样的方式解码动画 PNG，每一帧按其处置方式和混合方式绘制，并让每一帧保持压缩，直到播放时才解压。不是动画的 PNG 是只有一帧的视频。

如果解码器还提供 `Duration() time.Duration` 方法，它的视频长度会通过 `Duration` 报告；GIF 和 APNG 解码器就有这个方法。对解码器视频调用 `Seek` 时，会从开头解码到那一帧；但如果解码器是 `ui.VideoSeeker`，它的 `Seek` 会直接跳到那一帧，播放器只解码这一帧，[render 插件的播放器](../plugins/render.md#预览)就是这样。

## 系统视频

`ui/video` 包用系统的媒体框架播放。不导入它的应用不会链接其中任何代码。

| 系统   | 框架                                                     | 画面到达渲染器的方式                                                                                         |
| -------- | ------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| macOS    | AVFoundation（`AVPlayer`），用 VideoToolbox 解码          | 由 IOSurface 支持的 NV12 `CVPixelBuffer`，Metal 将其包装为纹理：不复制                                  |
| Windows  | Media Foundation（`IMFMediaEngine`），用 DXVA 解码        | 引擎在自己的 Direct3D 设备上转换、并与窗口的设备共享的 BGRA 纹理：在 GPU 上复制一次   |
| Linux    | GStreamer 1.x（`playbin`），在安装了 VA-API 的地方用它解码 | NV12 平面，每像素 1.5 字节，OpenGL 按原样上传，由着色器转换                               |
| 浏览器 | video 元素                                             | 该元素，由 WebGL 在 GPU 之间上传                                                                            |

`video.Open` 打开文件，`video.OpenURL` 打开 `file:`、`http:` 或 `https:` URL 或 HLS 播放列表，`video.OpenData` 打开内存中的字节，例如嵌入的文件，而无需写入文件。`video.ErrUnsupported` 报告系统没有可播放视频的框架，例如没有 GStreamer 及其基础插件的 Linux，或没有 Media Foundation 功能的 Windows Server，以便应用改为显示 GIF。能播放哪些格式取决于系统：MP4 中的 H.264 和 AAC 在所有这些系统上都能播放。Linux 需要 `gstreamer1.0-plugins-base`，H.264 还需要 `gstreamer1.0-libav`（或其他 H.264 解码器）；缺少解码器时会出现 `Err`。

Windows on Arm 的 MPEG-4 读取器会忽略轨道的编辑列表。带 B 帧的 H.264 视频（x264 的默认设置）通过编辑列表补偿帧重排延迟；该列表被忽略后，画面会相应延后，最多比声音落后几帧。如果需要避免这一问题，可编码为不带 B 帧的视频（`-x264-params bframes=0`），示例中的预告片就是如此。

在浏览器中，有声音的视频可能无法在用户与页面交互之前播放：此时它会静音播放，如浏览器所允许。

## 辅助功能

视频是辅助技术看不到的装饰，除非用 `Label` 为它们命名，与图像一样。
