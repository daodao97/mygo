# 样式与主题

`Background`、`Border`、`Radius`、`Shadow` 和 `Opacity` 设置元素盒子的样式，`Cursor` 设置指针悬停在它上面时的形状：

```go
ui.Column(c).Padding(18).Gap(12).Radius(10).
	Background(t.Background).Border(1, t.Border).
	Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.06))
```

- **边框。** `Border(1, c)` 在每条边的内侧画一条边框；`BorderWidth` 按 CSS 的方式分别设置各边，配合 `BorderColor`，例如用 `BorderWidth(0, 0, 1, 0)` 在标题下画一条线，`BorderStyle(ui.BorderDashed)` 把它变成虚线。与 CSS 的 `box-sizing: border-box` 一样，边框占用元素尺寸内的空间，内边距和子元素都在它里面。
- **阴影。** `Shadow(x, y, blur, spread, c)` 投射盒子阴影，与 CSS 的 `box-shadow` 一样：多个阴影会叠加，每个都只在盒子外部显示，因此半透明背景永远不会透出自己的阴影。`InsetShadow(x, y, blur, spread, c)` 绘制内阴影，如 `box-shadow: inset`：位于盒子的内边距边缘之内，受其边框和圆角限制，在背景之上、子元素之下；偏移量移动它，正的扩展让它向内扩大。内阴影也会叠加，并可与外阴影一起使用，例如深色模式下为输入框描边的一条细线：

  ```go
  field.Radius(12).Background(t.Surface).
  	Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.3)).
  	InsetShadow(0, 0, 1, 0, ui.RGBA(255, 255, 255, 0.2))
  ```

  `NoShadow()` 会移除之前添加的所有阴影，如 `box-shadow: none`，可用于去掉辅助函数返回的卡片自带的阴影；之后仍可重新添加阴影。[过渡](transitions.md#移动的是什么)会在阴影变化时以淡入淡出衔接。
- **渐变和条纹。** `Gradient(from, to, angle)` 用线性渐变填充盒子；`LinearGradient` 还可用 `Start`、`End` 指定颜色在线上的位置，选择在 Oklab 而非 sRGB 中混色以保持亮度，或通过 `Stops` 指定任意多个色标，每个色标为 `GradientStop{Color, At}`，位置范围为 0 到 1。`ConicGradient` 让色标围绕一个点分布，如 CSS 的 `conic-gradient()`（SwiftUI 的 `AngularGradient`）：以 `Center` 为中心（默认是盒子中心），从 `Angle` 开始，角度从正上方顺时针计算，以度为单位。`RadialGradient` 让色标从 `Center` 向外呈圆形扩散，如 CSS 的 `radial-gradient()`（SwiftUI 和 Flutter 的 `RadialGradient`）：中心处为 0，距中心 `Radius` DIP 处为 1，未指定半径时取到盒子最远角的距离，如 CSS 的 `farthest-corner`；中心之前使用首个颜色，半径之外使用末尾颜色，可用于辉光、聚光和暗角：

  ```go
  ui.Box(c).Size(240, 140).Radius(16).RadialGradient(ui.RadialGradient{
  	Center: ui.Point{X: 0.3, Y: 0.2}, Oklab: true,
  	Stops:  []ui.GradientStop{{ui.Hex("#a5b4fc"), 0}, {ui.Hex("#312e81"), 1}},
  })
  ```

  `BorderLinearGradient` 和 `BorderConicGradient` 用渐变代替颜色来绘制边框，如 SwiftUI 带渐变的 `strokeBorder`：

  ```go
  rainbow := []ui.GradientStop{{red, 0}, {yellow, 0.33}, {blue, 0.66}, {red, 1}}
  ui.Box(c).Size(120, 80).Radius(16).Border(4, ui.Color{}).
  	BorderConicGradient(ui.ConicGradient{Stops: rainbow, Oklab: true})
  ```

  `Stripes(c, width, gap, angle)` 在背景上绘制条纹，比如用在不可用的内容上。绘制器用同样的渐变填充矩形和路径（`FillGradient`、`FillConicGradient`、`FillRadialGradient`、`FillPathGradient`、`FillPathConicGradient`、`StrokePathConicGradient`）。
- **不透明度。** `Opacity(o)` 把元素及其子元素作为一个组淡化，如 CSS 的 `opacity`：它们绘制的内容先合在一起，再淡化，因此卡片边缘上的徽章，或图表中重叠的两根柱子，显示为一个整体，而不是彼此透叠。

  ```go
  ui.Row(c).Gap(8).Children(func() { /* … */ }).Opacity(0.5) // a toolbar, faded
  ```

  不与其他内容重叠的子元素会各自调整透明度，视觉效果相同，也没有额外开销；发生重叠时，则和模糊一样先渲染到 GPU 位图中，其中文本使用灰度抗锯齿。Web 视图和显示背后内容的材质无法合成到位图中，因此会单独调整透明度；`Disabled` 的变暗效果也是分别应用的。
- **模糊。** `Blur(sigma)` 模糊元素及其子元素，如 CSS 的 `filter: blur()`：它们绘制的内容在 GPU 上向四周扩散约 3 sigma DIP，超出部分透明。它们会先渲染到位图中，如[效果](shaders.md#对元素的效果)那样，绘制的每一帧都如此；点击、焦点和辅助技术仍按元素所在的位置处理。辉光是模糊后的边框，一个清晰的边框和若干模糊的边框叠加：

  ```go
  for _, l := range []struct{ width, blur float32 }{{6, 0}, {9, 4}, {15, 15}} {
  	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().
  		Radius(16).Border(l.width, ui.Color{}).BorderConicGradient(glow).Blur(l.blur)
  }
  ```
- **裁剪到内缩区域。** `ClipInset(top, right, bottom, left, radius)` 按指定的 DIP 距离内缩盒子，并将元素及其全部内容（背景、阴影、边框、子元素）裁剪到该区域，如 CSS 的 `clip-path: inset()`。`ClipInsetPercent` 改用盒子尺寸的百分比。负的内缩量会向外扩展对应边，因此 `ClipInset(0, -40, 0, -40, 0)` 会裁剪上下两侧，同时保留左右两侧的阴影。内缩区域直接用场景裁剪实现，不生成图层，因此可以逐帧改变。为某一侧的内缩量添加动画，就能让元素以擦除式过渡逐渐显现：

  ```go
  hidden := float32(100)
  if app.open {
  	hidden = 0
  }
  card.ClipInsetPercent(0, card.AnimateTo("wipe", hidden, ui.Smooth), 0, 0, 12)
  ```

  内缩区域遮住的部分不接收点击。
- **渐变遮罩。** `MaskLinearGradient`、`MaskRadialGradient` 和 `MaskConicGradient` 用一个横跨元素盒子的渐变遮罩元素及其内部内容，如 CSS 的 `mask-image`。每个点显示的程度等于渐变在该处的 alpha；颜色无关紧要。盒子之外（或使用 `ClipInset` 时内缩区域之外）什么也不显示。让一个滚动列表的行在边缘淡出：

  ```go
  black := ui.RGB(0, 0, 0)
  fade := []ui.GradientStop{{ui.Transparent, 0}, {black, 0.08}, {black, 0.92}, {ui.Transparent, 1}}
  list.MaskLinearGradient(ui.LinearGradient{Angle: 180, Stops: fade})
  ```

  被截断文本的末尾以同样的方式沿其宽度淡出：`MaskLinearGradient(ui.LinearGradient{Angle: 90, Stops: …})`。与 `Opacity` 不同，遮罩分别作用于绘制的每个对象；元素不会先绘制到图层中。因此，当一行的背景被淡化时，该行下方的内容也会透过它显示。点击不受遮罩影响。
- **多层遮罩。** `Masks` 用多个图层遮罩元素，如 CSS 的 `mask-image` 配合 `mask-composite`：每个图层都是一个渐变（`ui.Mask` 的 `Image`，一个 `LinearGradient`、`RadialGradient` 或 `ConicGradient`），横跨边框盒，或横跨内边距盒或内容盒（其 `Box`，如同 `mask-origin` 和 `mask-clip`），第一个在最上面。每个图层与列表中它下面的图层按它的 `Composite` 组合：默认的 `MaskAdd` 显示任一图层显示的内容，`MaskSubtract` 显示该层中下面图层不显示的部分，`MaskIntersect` 显示两者都显示的部分，`MaskExclude` 显示只有其中一个显示的部分。一个 1.5 DIP 宽的渐变圆环，从边框盒中切出内容盒，如 CSS 的渐变边框技巧：

  ```go
  black := ui.RGB(0, 0, 0)
  solid := ui.LinearGradient{From: black, To: black}
  ui.Box(c).Size(120, 40).Radius(20).Padding(1.5).Gradient(pink, violet, 90).Masks(
  	ui.Mask{Image: solid, Box: ui.MaskContentBox, Composite: ui.MaskExclude},
  	ui.Mask{Image: solid},
  )
  ```

  以及一弯新月，一个圆盘减去旁边另一个圆盘：

  ```go
  disc := func(x, y float32) ui.RadialGradient {
  	return ui.RadialGradient{Center: ui.Point{X: x, Y: y}, Radius: 40,
  		Stops: []ui.GradientStop{{black, 0.98}, {ui.Transparent, 1}}}
  }
  moon := ui.Box(c).Size(80, 80).Background(t.Text)
  moon.Masks(ui.Mask{Image: disc(0.5, 0.5), Composite: ui.MaskSubtract}, ui.Mask{Image: disc(0.7, 0.35)})
  ```

  `Masks` 会替换上面的渐变遮罩，上面的渐变遮罩也会替换它。只有一个横跨边框盒的图层时，遮罩效果和它们一样；图层更多时，会在 CPU 上按它们的 alpha 生成遮罩，元素每个显示尺寸生成一次，各渲染器再把它画成路径裁剪。与渐变遮罩一样，它分别作用于绘制的每个对象。
- **缓存。** `Cache()` 把元素及其子元素绘制的内容保存在 GPU 位图中，如 Flutter 的 `RepaintBoundary` 和 SwiftUI 的 `drawingGroup()`，仅在内容、尺寸、窗口缩放比例或主题发生变化时重新绘制。例如，动态绘图周围的静态卡片即使包含渐变、模糊图像和大阴影，每帧重绘时也只需绘制一张图像；用变换移动或旋转元素时，也只需变换位图，无需重绘其中的内容。被变换缩放时，位图按其在屏幕上覆盖的像素绘制，使其中的文本保持清晰：静止时按缩放比例本身，动画缩放期间按约 9% 的步长。位图包含盒子、阴影，以及超出盒子但未被裁剪的子元素；如果子树没有为文本提供不透明背景，文本就和模糊效果中的文本一样使用灰度抗锯齿。使用 `Blur` 或效果时，会直接缓存它们生成的位图。

  ```go
  ui.Box(c).Size(900, 212).Radius(16).Gradient(from, to, 30).
  	Shadow(0, 10, 40, 0, shade).Cache().Children(func() { /* … */ })
  ```
- **材质。** `Material(m)` 用包提供的材质填充盒子，代替背景，例如 [glass 插件](../plugins/glass.md) 的 Liquid Glass（透出下方内容，带磨砂效果和边缘折射）、滚动边缘效果，以及可沿渐变淡出的背景模糊。
- **指针。** `Cursor` 使用各平台的指针形状：`CursorPointer`、`CursorText`、`CursorMove`、`CursorGrab` 和 `CursorGrabbing`、调整大小的光标（双向的，以及朝一侧的 `CursorResizeE`，还有列和行的）、用于拖放的 `CursorCopy` 和 `CursorAlias`、`CursorContextMenu`、`CursorVerticalText`、`CursorNotAllowed`、`CursorCrosshair`、`CursorNone`（隐藏指针）、CSS 的 `progress` 和 `wait`（`CursorProgress` 用于忙碌但仍接受输入的控件，如带有 `aria-busy` 的控件；`CursorWait` 用于什么也做不了的时候）、`CursorHelp`、`CursorCell`、`CursorZoomIn` 和 `CursorZoomOut`、`CursorAllScroll` 和 `CursorNoDrop`。Windows 没有缩放和单元格光标，MyGo 会自行绘制，就像浏览器自带这些光标一样；macOS 的 `CursorWait`、`CursorHelp`、缩放光标和 `CursorAllScroll` 属于 AppKit 的私有光标，WebKit 就是这样使用的；某个版本没有它们时，会显示最接近的公开光标（`CursorCell` 用十字准线）。元素的指针也会在其子元素上显示，但接收点击的子元素（按钮，或任何带有 `Clicked` 或 `OnClick` 的元素）除外：它们显示自己的指针，除非设置了其他指针，否则是默认箭头，因此带有 `CursorText` 的文本框中的按钮不会显示 I 形光标。

`ui.Oklch(l, c, h)` 是 CSS 的 `oklch()`；`.Alpha(a)` 是它的 `/ a`。它返回一个和别的颜色一样的 `ui.Color`，因此填充、边框、渐变、条纹、阴影、分隔线、文本、装饰、文本背景、玻璃的色调和主题（`t.Accent = ui.Oklch(0.6, 0.2, 240)`）都接受它。它可以表示 sRGB 之外的颜色，比大多数屏幕显示的更鲜艳。这样的颜色同时保留两者：它的真实值，在窗口位于能显示更宽色域的屏幕（近期 Mac 上的 Display P3）上时绘制；以及最接近的 sRGB 颜色，按 CSS 的方式选择（饱和度更低，亮度和色相相同）。该回退值就是它的 `R`、`G` 和 `B`，也是 sRGB 屏幕、Linux、Windows 和 SVG 图片绘制的内容；`SRGB()` 返回它而不含真实值。`Mix` 和过渡混合真实值：混合回 sRGB 内的结果是普通颜色。不要给这种颜色的 `R`、`G` 或 `B` 赋值，因为真实值将不再匹配：请创建一个新颜色。

可用 `ui.RGB`、`ui.RGBA` 和 `ui.Hex("#2563eb")` 创建颜色；`Mix` 混合两种颜色，`MixOklab` 则与渐变的 `Oklab` 选项一样在 Oklab 中混色（黄色与蓝色混合的中间色偏蓝，而不是 sRGB 中的灰色），`Alpha` 用于调整颜色的透明度。

## 状态，而非选择器

没有样式表，也没有状态选择器：视图就是代码，因此元素的外观跟随视图自身的状态，如 `if row.Hovered() { row.Background(t.SurfaceHover) }`，一个元素的状态可以给另一个元素设置样式，就像 CSS 中一个组的悬停那样。

控件会返回它的元素，因此在其后继续链式调用，就能单独给它设置不同的样式：`ui.Button(c, "Save").Padding(10, 20).Radius(999)`。想做出自己的外观，就基于各控件的基础类型来构建，它们本身无外观：参见[自定义控件](custom-widgets.md)。

## 主题

控件从主题 `c.Theme()` 获取颜色和尺寸参数：默认采用浅色或深色主题，随系统外观变化，并遵循系统控件所用的桌面设置。自定义元素也使用主题颜色，就能随之变化。要修改主题，请设置一个副本：

```go
// The default, which follows the system, in an accent of the app's,
// with its hover, pressed, selection and focus shades.
t := *c.Theme().WithAccent(ui.Hex("#7c3aed"))
t.Radius = 8
t.Spacing = 3 // compact
// A square bar, as VS Code's editor.
t.ScrollbarStyle = ui.ScrollbarStyle{
	TrackWidth: 14, ThumbWidth: 14, Square: true, Flush: true, MinLength: 20,
}
c.SetTheme(&t)
```

`Spacing` 是控件留出空间的单位：它们的内边距和间隙，以及复选框、开关、滑块和表格与树的行的大小，都是它的倍数。默认是 4；3 让所有控件紧凑，5 则更宽松。`FontSize` 决定它们文本的大小，`Radius` 让它们的角变圆，`Scrollbar` 和 `ScrollbarWidth` 设置滚动条滑块的颜色和大小，`ScrollbarStyle` 进一步设置样式：轨道、悬停和拖动颜色、宽度、圆角和滑块的最短长度。在 macOS 上，无论是控件的还是你自己元素的圆角，都像 AppKit 和 SwiftUI 那样连续弯曲，在其他地方则是四分之一圆，如 Windows 和 GTK 所绘制。用主题来设置你自己的元素尺寸，它们也会跟随：`t.Space(3)` 是三个间距单位，`t.Rem(2)` 是其字体大小的两倍，如 CSS 的 rem。

工具提示和 toast 使用反色主题，以便与下方内容区分：背景用 `Inverse`，文字用 `InverseText`；两者为零值时，分别采用主题的 `Text` 和 `Background`。深色应用如果希望它们也保持深色，可以同时设置这两个值：

```go
t.Inverse, t.InverseText = ui.Hex("#3f3f46"), ui.Hex("#fafafa")
```

`TooltipDelay` 是指针在元素上停留多久后显示其工具提示，留零时为 0.6 秒（参见[工具提示](tooltip.md#延迟)）。

在不是接收用户输入的窗口中，控件会把强调色淡化为灰色，如 AppKit 淡化非活动窗口那样（参见[活动和非活动窗口](windows.md#活动与非活动窗口)）：选中的行、勾选的控件、滑块和进度条用 `InactiveAccent` 填充，留零时是一种能让 `AccentText` 醒目的灰色，主按钮看起来则与其他按钮一样。一个希望在每个窗口中都保留强调色的主题，把它设置为强调色：

```go
t.InactiveAccent = t.Accent
```

## 焦点环

通过键盘获得焦点的元素会显示一个环，由主题绘制：宽度为 `FocusRingWidth` DIP（默认 2），其内边缘在元素边缘之外 `FocusRingOffset` DIP（浅色和深色主题中为 1；0 把它放在边缘上，负偏移则放在里面），颜色为 `Focus`。它的角跟随元素的角，如 CSS 的轮廓。一个环更粗、位置更靠外的主题：

```go
t.FocusRingWidth, t.FocusRingOffset = 3, 2
```

元素可以用 `FocusRingOffset` 设置自己的偏移，用 `FocusRingColor` 设置颜色，比如某个控件的环必须落在它内部，因为包着它的元素会裁掉超出边缘的部分，手风琴盒子里的标题就是如此：

```go
header.FocusRingOffset(-4) // inside, its outer edge 2 DIPs in
```

只给自身一部分画环的控件（例如复选框给它的方框画环）接受相同的设置。`FocusRing(false)` 为这样的控件关闭环：它自己用 `Painter.FocusRing`（主题围绕任意矩形的环）在 `FocusVisible` 时绘制。

## 窗口一部分的主题

`SetTheme` 为整个窗口设置主题，包括根元素、根元素的背景和文本，以及调用之后构建的控件。`ui.Themed(c, t)` 则创建一个列容器，让其中的子元素使用 `t` 构建，只改变窗口局部的主题，例如在主题选择器中并排预览浅色和深色，或在深色界面中嵌入浅色预览窗格：

```go
ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
	for _, t := range []*ui.Theme{ui.LightTheme(), ui.DarkTheme()} {
		ui.Themed(c, t).Grow(1).Padding(16).Gap(8).Children(func() {
			ui.Text(c, "Preview")
			ui.Button(c, "OK")
		})
	}
})
```

该列内部的行为与整个窗口使用 `t` 时相同：构建子元素时 `c.Theme()` 返回该主题，绘制时 `Painter.Theme()` 也返回该主题，因此自定义控件也能采用它；控件使用其中的颜色和尺寸参数（`Spacing`、`Radius`、`FontSize`、`ScrollbarWidth`、`ScrollbarStyle`，以及 `t.Space` 和 `t.Rem`）；从内部打开的浮层、菜单、工具提示和对话框即使显示在窗口上层，也使用该主题。该列以 `t` 的 `Background` 绘制背景，文本使用 `t` 的 `Text` 颜色、`FontSize` 和 `Font`，与根元素使用窗口主题的方式相同；也可以像根元素一样，用 `.Background(ui.Transparent)` 透出背后内容。根元素和该列之后构建的内容仍使用窗口主题；toast 属于窗口，因此也不受局部主题影响。主题可以嵌套，传入 nil 主题则得到普通列。在内部调用 `SetTheme` 仍会改变整个窗口的主题，包括根元素和该列之后的内容，但该列的子元素仍使用 `t`。

## 桌面的偏好设置

`c.Preferences()` 返回桌面的设置，默认主题遵循它，每一帧也会跟随它的变化：

- **`Accent`。** 用户选择的强调色（macOS、Windows，以及通过桌面门户提供强调色的桌面环境，如 GNOME 47 和 KDE），用于主按钮、选中状态和焦点环，并配以对比鲜明的文字颜色。
- **`HighContrast`。** macOS 的增强对比度、Windows 的对比度主题、桌面门户的更高对比度：边框和次要文本更深（深色模式下更浅），焦点环不透明。
- **`TextScale`。** Windows 和 GNOME 的文本大小：`FontSize` 增大相应倍数。
- **`ReduceMotion`。** macOS 的减弱动态效果、Windows 的动画效果和 GNOME 的动画被关闭，或 KDE 的动画速度设为“即时”：`Animate` 立即到达目标。
- **`Pointer`、`AnyPointer`、`Hover()` 和 `AnyHover()`。** 描述指向设备的能力，对应 Media Queries 4 的 `pointer`、`any-pointer`、`hover` 和 `any-hover`：`Pointer` 表示主指向设备的精度，`ui.PointerFine` 表示鼠标、触摸板或笔，`ui.PointerCoarse` 表示触摸屏，`ui.PointerNone` 表示无指向设备，`AnyPointer` 表示所有设备中的最高精度；`NoHover` 和 `NoAnyHover` 分别表示主设备不支持悬停、所有设备都不支持悬停（`Hover()` 和 `AnyHover()` 返回相反结果）。触摸优先的布局应为低精度指针提供更大的点击目标，并且仅在设备支持悬停时，才将某些内容设为悬停后显示：

  ```go
  p := c.Preferences()
  size := float32(28)
  if p.Pointer == ui.PointerCoarse {
  	size = 44
  }
  del := ui.Button(c, "×").Size(size, size).Label("Delete")
  if p.Hover() {
  	del.Tooltip("Delete")
  }
  ```

  Linux 读取 seat（输入设备组）中的设备（来自 GDK），并跟随它们的出现和消失；Windows 读取是否有鼠标、笔或触摸屏，在平板模式下触摸屏是主设备；浏览器回答媒体查询。macOS 的以及未知的设备按鼠标处理：精细，且可悬停。

自定义主题（`SetTheme`）不会自动遵循这些设置，而是由你控制。要让它像默认主题一样遵循桌面设置（例如允许用户手动选择浅色或深色、不随系统外观切换），可将设置传给 `WithPreferences`，得到采用桌面强调色、对比度和文本大小的主题副本：

```go
t := ui.DarkTheme()
t.Background = ui.Hex("#18181b")
c.SetTheme(t.WithPreferences(c.Preferences()))
```

已经遵循它们的主题（如 `c.Theme()` 和 `WithPreferences` 的返回值）会改为遵循这次传入的设置：先撤销之前跟随所做的更改，因此文本不会被放大两次，用户关闭的设置会被移除，而你此后设置的颜色会保留。`WithAccent` 只接收一个强调色，可以是桌面的或你自己的，并带上默认主题为它生成的色调：`AccentHover`、`AccentPressed`、`Selection` 和 `Focus` 成为该强调色的色调，在深色模式下更浅，`AccentText` 为白色，在像黄色这样浅的强调色上则接近黑色。`A` 为 0 的强调色（例如桌面没有强调色时）保留主题自己的强调色：

```go
t := ui.LightTheme().WithAccent(ui.Hex("#7c3aed")) // a brand color
```
