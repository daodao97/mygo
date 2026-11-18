# 绘制与动画

`Draw` 在元素背景之后绘制，`DrawOver` 在子元素之后绘制，均使用以窗口 DIP 为单位的 `*ui.Painter`：`Fill`、`FillGradient`、`FillConicGradient` 和 `FillRadialGradient` 用于填充矩形，`Stroke` 和 `StrokeDashed` 用于描边；还有 `Shadow`、`Line`（水平或垂直）、`Image`、`Clip` 和 `ClipPath`。`FillPath` 和 `StrokePath` 用于填充和描画直线、曲线组成的路径，渐变版本为 `FillPathGradient` 和 `StrokePathGradient`；文本可用 `Text` 绘制，或用 `RichText` 绘制不同样式的文本段，先用 `MeasureText` 测量即可居中或对齐：

```go
ui.Box(c).Height(120).Draw(func(p *ui.Painter, r ui.Rect) {
	var wave ui.Path
	for i := 0; i <= 100; i++ {
		x := r.X + r.W*float32(i)/100
		y := r.Y + r.H/2 + 40*float32(math.Sin(float64(i)/8))
		if i == 0 {
			wave.MoveTo(x, y)
		} else {
			wave.LineTo(x, y)
		}
	}
	p.StrokePath(&wave, 2, c.Theme().Accent)
})
```

```go
ui.Box(c).Size(200, 40).Draw(func(p *ui.Painter, r ui.Rect) {
	label := ui.Span{Text: "42%", Weight: 600, Color: c.Theme().TextMuted}
	w, h := p.MeasureText(0, label)
	p.RichText(r.X+(r.W-w)/2, r.Y+(r.H-h)/2, 0, label)
})
```

视图自己排版的段落，例如阅读器中的书页，会用 `ui.ShapeRichTextWith` 塑形一次，需要时两端对齐并缩进，再用 `Painter.TextLayout` 绘制；它们的 `Lines` 说明每一行在哪里，用于在行之间分页（见[保留的文本几何](text-input-client.md#保留的文本几何信息)）。

绘制函数只负责绘制：MyGo 可能在一帧内多次调用它们。`Painter.Icon` 和 `Painter.Image` 绘制 SVG 和位图，`Painter.ImageRegion` 绘制位图的局部区域（见[位图的部分](#位图的部分)）；自行绘制的控件可用 `Painter.FocusRing` 按主题样式绘制键盘焦点环（见[焦点环](styling.md#焦点环)）。

`Path.Trim(from, to)` 保留路径长度中从 `from` 到 `to` 的一段，这两个值各占总长度的比例；这样在 `to` 从 0 动画到 1 时，路径就会逐渐画出，如 SwiftUI 的 `trim` 和 Motion 的 `pathLength`：

```go
drawn := float32(0)
if app.done {
	drawn = 1
}
drawn = box.AnimateTo("check", drawn, ui.Smooth)
box.Draw(func(p *ui.Painter, r ui.Rect) {
	check := (&ui.Path{}).MoveTo(r.X+4, r.Y+12).LineTo(r.X+10, r.Y+18).LineTo(r.X+20, r.Y+6)
	p.StrokePath(check.Trim(0, drawn), 2, c.Theme().Accent)
})
```

描边有圆形的端点和拐角，除非它的路径另行设置，如 SVG 的 `stroke-linecap`、`stroke-linejoin` 和 `stroke-miterlimit`：`Path.Cap` 把它平头结束（`ui.ButtCap`）或方形结束（`ui.SquareCap`），`Path.Join` 把拐角做成尖角（`ui.MiterJoin`，超过 `Path.MiterLimit` 时改为斜切）或削平（`ui.BevelJoin`）。`Path.Dash` 用虚线描边，如 `stroke-dasharray` 和 `stroke-dashoffset`；随着 `Trim` 把路径画出来，虚线仍留在原位：

```go
box.Draw(func(p *ui.Painter, r ui.Rect) {
	edge := (&ui.Path{}).MoveTo(r.X, r.Y+r.H/2).LineTo(r.X+r.W, r.Y+r.H/2)
	p.StrokePath(edge.Cap(ui.ButtCap).Dash(0, 6, 4), 1.5, c.Theme().TextMuted)
})
```

## 填充规则与命中

路径填充和裁剪的范围，是它的子路径所环绕的点（SVG 的 `nonzero`），所以一个子路径在另一个里面时，只有反向绕行才会留下洞。`Path.FillRule(ui.EvenOdd)` 则填充子路径环绕奇数次的点，如 SVG 的 `fill-rule` 和 `clip-rule: evenodd`，这样用两个圆就能做成环或甜甜圈，也能做成带挖空的形状，无论它们的路径数据怎么画：

```go
box.Draw(func(p *ui.Painter, r ui.Rect) {
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	donut := (&ui.Path{}).Circle(cx, cy, r.W/2).Circle(cx, cy, r.W/4).FillRule(ui.EvenOdd)
	p.FillPath(donut, c.Theme().Accent)
})
```

`Path.Contains(x, y)` 根据路径的填充规则判断点是否位于填充区域内，类似 Web 的 `isPointInPath`：视图处理器可据此判断按下位置属于饼图的哪一块。`HitTest` 根据函数返回值决定元素在哪些位置接收指针事件，传入的指针坐标与 `Draw` 使用的坐标一致。因此，饼图分块、地图区域或圆环只在实际绘制的区域响应按下、悬停并设置鼠标指针样式，类似 SVG 的 `pointer-events: visiblePainted`；其他位置的事件会传给下方内容：

```go
seg := func(r ui.Rect) *ui.Path { return slice(r, from, to) } // a segment's outline
ui.Box(c).Absolute().Fill().
	Draw(func(p *ui.Painter, r ui.Rect) { p.FillPath(seg(r), color) }).
	HitTest(func(x, y float32, r ui.Rect) bool { return seg(r).Contains(x, y) }).
	OnClick(func() { app.pick(i) })
```

## 从 SVG 数据得到的路径

`ui.ParsePath(d)` 把 SVG `<path d="…">` 的路径数据（它的所有命令，绝对和相对，还有弧线）读入一个 `*ui.Path`，这样从 SVG 复制来的图标轮廓，就能像任何路径一样填充、描边、修剪或裁剪。它返回的错误会给出第一处无法读取的内容的偏移；`ui.MustParsePath` 则会 panic，用于写进程序的数据：

```go
var heart = ui.MustParsePath("M12 21 L10.6 19.7 C5.4 15 2 11.9 2 8.1 C2 5 4.4 2.6 7.5 2.6 " +
	"C9.2 2.6 10.9 3.4 12 4.7 C13.1 3.4 14.8 2.6 16.5 2.6 C19.6 2.6 22 5 22 8.1 " +
	"C22 11.9 18.6 15 13.4 19.7 Z")

box.Draw(func(p *ui.Painter, r ui.Rect) {
	p.Transform(ui.Scaling(r.W/24, r.H/24).Then(ui.Translation(r.X, r.Y)), 0, 0, func() {
		p.StrokePath(heart.Trim(0, drawn), 1.5, c.Theme().Accent)
	})
})
```

路径使用数据自己的单位；一个变换把它缩放到盒子。

## 裁剪到路径

`Painter.Clip(r, radius, fn)` 把 `fn` 绘制的内容裁剪到圆角矩形。`Painter.ClipPath(path, fn)` 把它裁剪到任意路径的内部（按其 `FillRule`），边缘平滑，如 SVG 和 CSS 的 `clip-path`：画进多边形的图表，或被切成星形的图片。

```go
star := ui.MustParsePath("M50 0 L61 35 L98 35 L68 57 L79 91 L50 70 L21 91 L32 57 L2 35 L39 35 Z")
ui.Box(c).Size(100, 100).Draw(func(p *ui.Painter, r ui.Rect) {
	p.Transform(ui.Translation(r.X, r.Y), 0, 0, func() {
		p.ClipPath(star, func() {
			p.FillGradient(ui.Rect{W: 100, H: 100}, ui.LinearGradient{From: gold, To: orange, Angle: 135}, 0)
		})
	})
})
```

路径裁剪可以相互嵌套，也可以与 `Clip` 嵌套，并随变换一起旋转。所有渲染器都通过采样路径覆盖率遮罩完成裁剪；每种绘制形状和尺寸只生成一次遮罩，并在后续帧继续使用，因此静止路径无需重复生成遮罩。路径移动或变化时，每帧会按路径的设备像素尺寸生成新遮罩（每边最多 8192 像素）。

要裁剪元素及其子元素而不是绘制内容，见 `ClipInset` 以及[样式](styling.md)中的渐变遮罩。

## 位图的部分

一张位图可以包含多幅图像，例如图标集、动画帧或缩略图索引。`Painter.ImageRegion` 将其中一部分（`src`，以位图像素为单位）绘制到矩形内；无论采用平滑还是像素化绘制，都不会显示该区域周围的位图内容。`ImageOptions` 可镜像这一区域（`FlipX`、`FlipY`）或着色（`Tint` 与其颜色和 alpha 相乘）：

```go
icons, _ := ui.DecodeBitmap(iconsPNG) // 16×16 icons in a row
icon := func(i int) ui.Rect { return ui.Rect{X: float32(i * 16), W: 16, H: 16} }

box.Draw(func(p *ui.Painter, r ui.Rect) {
	p.ImageRegion(icons, icon(3), ui.Rect{X: r.X, Y: r.Y, W: 24, H: 24},
		ui.ImageOptions{Tint: c.Theme().TextMuted})
})
```

绘制器会跳过显示范围之外的区域，因此即使视图包含大量图像，也可逐一调用绘制，无需自行剔除不可见内容，例如游戏中的精灵和图块。

## 画布

`ui.Canvas` 是绘制器可以在上面绘制的位图；窗口用 GPU 绘制时，它也在 GPU 上：渲染到纹理，如 Skia 的 `SkSurface` 和 LÖVE 的 `Canvas`。`Draw` 会在一帧中重新绘制它，使用以它的像素为单位的绘制器；它保留其中的内容，并通过 `Bitmap` 在任何接受位图的地方显示：

```go
grid := ui.NewCanvas(800, 400)
drawn := false

box.Draw(func(p *ui.Painter, r ui.Rect) {
	if !drawn { // once, then every frame for the price of an image
		drawn = true
		grid.Draw(p, func(cp *ui.Painter) { drawGrid(cp, 800, 400) })
	}
	p.Image(grid.Bitmap(), r, ui.FillBox)
	drawSeries(p, r)
})
```

用着色或不透明度绘制的画布，会一次给整个图层着色或让它淡出。画布可以显示其他画布，也可以显示文本。画布的像素就是它的单位：按窗口的缩放绘制它（`Painter.Scale`），就能得到和窗口一样清晰的画布；或者画得小一些，并让它的位图 `Pixelated`，把像素画清晰地放大（[游戏插件](../plugins/game.md#像素级精确缩放) 以整数倍显示）。[着色器](shaders.md)把画布当作图像读取，并画进它们。

## 变换

`Painter.Transform(t, ox, oy, fn)` 用 `t` 变换 `fn` 绘制的内容，`t` 是 `ui.Transform`，例如 `Translation`、`Scaling` 或 `Rotation`，围绕点 `(ox, oy)`，该点留在原处。变换可以嵌套，并在绘制器自己的变换之后生效，因此一个会平移和缩放的视图（比如地图或图表）可以通过一个变换绘制内容，并在其中旋转一部分：

```go
ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
	view := ui.Translation(-doc.panX, -doc.panY).Then(ui.Scaling(doc.zoom, doc.zoom))
	p.Transform(view, r.X, r.Y, func() {
		for _, n := range doc.nodes {
			p.Fill(n.box, n.color, 6) // culled where the view does not look
		}
		cx, cy := doc.dial.X, doc.dial.Y
		p.Transform(ui.Rotation(doc.dial.angle), cx, cy, func() {
			p.Line(cx, cy, cx, cy-40, 2, c.Theme().Accent)
		})
	})
})
```

在其中创建的裁剪会随被裁剪的内容一起旋转。输入不做变换：指针以元素的坐标传入，应用用视图的逆变换把它映射到自己的内容。在变换下，图像和它们的部分会放在它们所在的位置，而不是对齐到整像素，这样在缩放的视图中移动时就会平滑滑动。

## 排版文本

控件自行排版文本时，例如绘制网格单元格，可用 `ui.Shape` 进行一次字形塑形；系统文本引擎会返回沿一行排列的字形，处理连字、字距调整和字体回退，并记录每个字形对应的 rune。所有平台都按从左到右的视觉顺序返回字形：希伯来文或阿拉伯文的 `Cluster` 递减，并带有 `RTL` 标记，因此可用 `Cluster` 和 `Runes` 查找某段 rune 对应的字形。调整字形的 `X` 后，用 `Painter.Glyphs` 绘制；`Font.Metrics` 返回字体的上伸高度、下伸高度和行间距，`Painter.Scale` 返回每 DIP 对应的设备像素数，便于对齐屏幕像素。与元素文本不同，`Shape` 不缓存结果：跨帧重复绘制的文本应保留其字形。`Font.Features` 启用或禁用 OpenType 特性；`Font.Thicken` 在 macOS 上加粗文本笔画，类似 Ghostty 的 `font-thicken`；`Font.Antialiased` 则关闭字体平滑，使笔画更细，与浏览器使用 `-webkit-font-smoothing: antialiased` 绘制文本的效果一致。

```go
font := ui.Font{Family: "monospace", Size: 13}
glyphs := ui.Shape("grid", font)
for i := range glyphs {
	glyphs[i].X = float32(glyphs[i].Cluster) * cellWidth // one per cell
}
ui.Box(c).Height(20).Draw(func(p *ui.Painter, r ui.Rect) {
	p.Glyphs(glyphs, r.X, r.Y+font.Metrics().Ascent, c.Theme().Text)
})
```

## 动画

`Element.Animate` 返回逐渐缓动到目标的值，并持续请求绘制，直到到达目标：

```go
panel := ui.Column(c).Clip()
width := float32(0)
if app.sidebar {
	width = 280
}
panel.Width(panel.Animate("width", width, 200*time.Millisecond))
```

`AnimateWith` 接受缓动：`ui.Linear`、`ui.EaseIn`、`ui.EaseOut`（同 `Animate`）、`ui.EaseInOut`、`ui.CubicBezier(x1, y1, x2, y2)`、任意 `func(t float32) float32`，或 `ui.Bounce(e)`，它沿 `e` 走再折返。`AnimateTo` 也接受弹簧（见 [Motion](motion.md)）。`Loop` 返回每周期重新开始的动画的进度，用于加载指示器和脉冲；`Rotate` 旋转元素，例如图标：

```go
spin := ui.Icon(c, loader)
spin.Rotate(spin.Loop("spin", time.Second, ui.Linear) * 360)

skeleton := ui.Box(c).Height(14).Radius(7).Background(t.Border)
skeleton.Opacity(0.4 + 0.6*skeleton.Loop("pulse", 1600*time.Millisecond, ui.Bounce(ui.EaseInOut)))
```

要以其他方式做动画，从 `c.Now()` 计算，并在每个移动的帧中调用 `c.AnimationFrame()`：MyGo 在显示器能显示时绘制下一帧，没有变化时什么也不画。

如果布局不变，只有绘制内容在动，例如加载指示器或滚动的图表，应在 `Draw` 中驱动动画：根据 `p.Now()` 计算状态，运动期间调用 `p.AnimationFrame()`；如果按步骤变化，则调用 `p.After(d)`。没有其他变化时，这些帧只重绘上一帧的元素，不运行视图，因此只有绘制开销；视口外的元素不执行绘制，也就不会请求新帧。窗口不可见时（隐藏、最小化或被其他窗口完全遮挡），MyGo 不绘制任何帧，无论动画由 `Draw` 还是视图驱动，直到窗口部分重新可见。像下例一样根据时间计算动画状态，恢复显示时就能呈现当前时刻对应的状态（见[动效](motion.md#不在视线内)）：

```go
ui.Box(c).Height(4).Draw(func(p *ui.Painter, r ui.Rect) {
	p.AnimationFrame()
	x := r.X + (r.W-20)*float32(p.Now().UnixMilli()%1000)/1000
	p.Fill(ui.Rect{X: x, Y: r.Y, W: 20, H: r.H}, c.Theme().Accent, 2)
})
```

在较少帧数下看起来仍足够平滑的绘制，例如转动的唱片、缓慢的波形或电平表，可以用 `p.AnimationFrameRate(fps)` 代替 `p.AnimationFrame()` 来请求帧：窗口会按那个频率重新绘制它，而不是在每次显示器刷新时（ProMotion 显示器上每秒 120 次）；在 macOS 和 iOS 上，这还会让显示器以这个频率刷新，从而省电。频率会向上取整到显示器能均匀显示的频率，即每帧对应整数次刷新：120 Hz 显示器上每秒 60、40 或 30 帧，60 Hz 上 60 或 30 帧。窗口会以其中任何东西请求的最快频率绘制，所以请求 `AnimationFrame` 的绘制，或会做动画的视图，仍以显示器的频率绘制。改用 `p.After(time.Second / 60)` 控制节奏反而会绘制不均：比刷新更晚请求的帧要等下一次刷新。

这类 `Draw` 使用的是视图上次运行时的状态；如果它读取的数据通过其他途径发生变化，需要调用 `Window.Update`，让视图重新构建。

当桌面要求更少的动效（`c.Preferences().ReduceMotion`）时，`Animate` 和 `AnimateWith` 会立刻到达目标。`Loop` 继续，如系统的加载指示器那样：它表明有事情正在进行。你自己计算的动效也应该读取这个偏好。

[动效](motion.md)介绍保留速度的弹簧、延迟、跟随指针的值、关键帧，以及如何判断动画结束；[变换](transforms.md)介绍如何移动、缩放和旋转任意元素。
