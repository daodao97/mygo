# 动效

<!-- preview motion 220 -->

元素可以自动对数值进行动画插值：给定目标值后，它会返回当前帧的值，并持续绘制，
直到到达目标。视图只需指定目标状态，动画负责平滑过渡：

```go
card := ui.Box(c).Size(120, 80).Radius(12).Background(t.Accent)
target := float32(0)
if app.open {
	target = 200
}
card.Translate(card.AnimateTo("x", target, ui.Snappy), 0)
```

键（`"x"`）用来区分同一个元素的多个动画值。元素必须在各帧中保持相同的 ID：
使用相同的 `Key`，或保持在同级元素中的相同位置。值的初始值就是第一个目标值，可用于
变换（见[变换](transforms.md)）、尺寸、不透明度、颜色混合，或
`Draw` 绘制的内容。

## 动画

`Animation` 指定值如何过渡到新目标：

- **弹簧**，即零值 `Animation` 以及预设 `ui.Smooth`（不回弹）、
  `ui.Snappy`（轻微回弹）和 `ui.Bouncy`（明显回弹）。弹簧在途中目标改变时
  保留当前速度：在移动途中再次点击，上面的卡片会平滑掉头，而不是骤然停住。
- **曲线**：一个 `Duration` 和一个 `Ease`（为 nil 时用 `ui.EaseOut`），如
  `ui.Animation{Duration: 150 * time.Millisecond, Ease: ui.EaseInOut}`。
  `Animate(key, target, d)` 和 `AnimateWith(key, target, d, ease)` 是它们的
  简写。

弹簧的 `SpringOptions` 用直观参数描述动效：`Duration` 是大致到达目标所需的时间
（零值表示 500 ms）；`Bounce` 控制回弹程度，从 0（不越过目标）到接近 1，
负值会让到达目标的过程更慢。SwiftUI 和 Motion 也采用这些参数。`Stiffness`、`Damping` 和 `Mass`
则使用物理参数描述弹簧：

```go
gentle := ui.Animation{Spring: ui.SpringOptions{Duration: 700 * time.Millisecond, Bounce: 0.1}}
stiff := ui.Animation{Spring: ui.SpringOptions{Stiffness: 400, Damping: 30}}
x := card.AnimateTo("x", target, gentle)
```

`Spring(key, target, options)` 是使用弹簧的 `AnimateTo` 的简写。弹簧采用精确
求解，而非逐步模拟，因此慢帧不会改变运动轨迹。动画类型也可在途中切换：
从曲线切换到弹簧时，会保留曲线当前的速度。

`Easing` 函数有 `ui.Linear`、`ui.EaseIn`、`ui.EaseOut`、`ui.EaseInOut`、
`ui.Bounce(e)`、CSS 的 `ui.CubicBezier(x1, y1, x2, y2)`，或任何
`func(t float32) float32`。

`AnimateColor(key, target, a)` 以同样的方式为颜色做动画，用预乘 alpha 混合，
这样从透明淡入的颜色能保持色相：

```go
row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
```

颜色在 sRGB 中混合，除非 `Animation` 有 `Oklab`：那就在 Oklab 中混合，如
渐变的 `Oklab` 混合那样，它在两种饱和颜色之间的路径保持它们的明度和饱和度，
而不是经过一个暗淡、更深的中间色（黄色到蓝色会经过蓝色系的中间色，而不是灰色）。
[过渡](transitions.md#移动的是什么)的颜色也用它来混合，`ContentTransition`
变形中的颜色也是如此，`Color.MixOklab` 则这样混合两种颜色：

```go
badge.Background(badge.AnimateColor("bg", level, ui.Animation{Spring: ui.SpringOptions{Duration: 300 * time.Millisecond}, Oklab: true}))
```

## 只绘制的姿态

上面的值需要由视图读取，因此动画的每一帧都会运行视图。`AnimatePose` 则直接为元素
添加平移、缩放、旋转和淡出，叠加在元素自身的变换之后。动画帧只重绘，不运行视图，
类似 Core Animation 和 Web Animations API 对变换和不透明度的处理。这种动效
开销最低，适合保持大型视图的流畅度：

```go
lift := ui.Pose{}
if card.Hovered() {
	lift = ui.Pose{Y: -4, Scale: 1.02}
}
card.AnimatePose(lift, ui.Snappy)
```

`Pose` 通过 `X` 和 `Y` 移动，通过 `XPercent` 和 `YPercent` 按元素自身宽高的
百分比移动（就像 [`TranslatePercent`](transforms.md) 那样，会随元素尺寸变化
而调整），通过 `Scale` 缩放（0 保持尺寸），按 `Rotate` 指定的角度旋转，通过
`Fade` 淡出，从 0（不透明）到 1（消失）。元素移动时，指针、文本光标和浮层都
会跟着它，`Transitioning()` 报告它的姿态是否在移动。

首次设置姿态时，元素会直接显示在该姿态，不播放动画。如果上一帧未设置姿态（例如元素
首次构建），也遵循这一规则；只有后续帧设置了不同姿态，才会播放过渡。要实现入场动画，
例如让浮出面板升起并淡入，先在一次构建中设置入场起始姿态，
用 `c.Invalidate()` 请求下一帧，再在下一帧设置目标姿态：

```go
panel := ui.Column(c)
entered := ui.Local(panel, "entered", func() bool { return false })
p := ui.Pose{}
if !*entered {
	p, *entered = ui.Pose{Y: 8, Fade: 1}, true
	c.Invalidate()
}
panel.AnimatePose(p, ui.Snappy)
```

没有第二次构建，元素会停留在第一次姿态放置它的地方。出现在上一帧已显示的
父元素中的元素，也会带着其[过渡](transitions.md#出现与离开)的 `Enter`
进入，如添加到列表中的一行。

姿态可以嵌套：每个姿态都在周围元素的姿态之后生效，所以一个元素内部的元素会
沿自己的曲线添加自己的动效。在[彩纸示例](../../examples/confetti)中，一团
彩纸落下并淡出，其中的每一片沿一条减速的曲线迸出，并在内部旋转：数百片飞舞
而无需再次构建视图。每个被赋予姿态的元素在绘制它的每一帧仍有开销，所以移动
方式相同的元素可以共享一个姿态（如整团的下落），而不是每个都用自己的。

## 延迟与错开

`Delay` 在目标改变后让值保持原来的走势一段时间。随索引增长的延迟会让列表
依次错开：

```go
for i, it := range app.items {
	row := ui.Row(c.Key(it.ID))
	a := ui.Snappy
	a.Delay = time.Duration(i) * 30 * time.Millisecond
	row.Opacity(row.AnimateTo("shown", 1, a))
}
```

## 手势

`Draggable` 让指针可以拖动并甩出元素，类似 motion.dev 的 `drag`：元素
先跟随指针，松开后以指针释放时的速度开始弹簧动画，直到停稳：

```go
card := ui.Box(c).Size(90, 60).Radius(10).Background(t.Accent)
card.Draggable(&app.card, ui.DraggableOptions{
	Axis:   ui.Horizontal,
	Bounds: &ui.Rect{W: 200},
	Snap:   ui.Nearest(ui.Point{}, ui.Point{X: 200}),
})
```

`app.card` 是一个 `ui.Point`，保存元素相对于布局位置的偏移。拖动期间，
偏移跟随指针；松开后，它表示元素最终停下的位置；应用也可以设置偏移来移动元素，
例如通过按钮翻页。偏移指针为 nil 时，由元素内部保存偏移。以下选项控制
移动方式：

- **`Axis`**，`ui.Horizontal` 或 `ui.Vertical`，是它移动的方向；为零值时两个
  方向都可以。
- **`Bounds`** 让偏移在静止时保持在一个矩形内，从 `X` 到 `X+W`、从 `Y` 到
  `Y+H`。指针把元素拉过边界时，拉得越远，能移动的距离越小，就像 iOS 对待
  被拉过边缘的内容那样；松开后它会弹回边界内。`Rigid` 让它在边界处停住。
- **`Snap`** 决定它在哪里停下，依据抛出会把它带到哪里（`ui.Project`，限制在
  边界内），这样一次轻扫会继续到下一页，或下一个高度：`ui.Nearest(points...)`
  选择最近的点。如果 `Snap` 把点原样返回，元素就会滑到抛出带它去的地方。
  没有 `Snap` 时，它停在指针松开的地方，如放在画布上的一项。
- **`Animation`** 是它如何停下，以及如何前往应用设置的偏移：为零时是
  `ui.Smooth`。弹簧从指针的速度开始。

指针移动几个 DIP 后才会开始拖动元素，那次按下不算点击；元素移动时按在它
上面，就能把它接住。元素在自身的变换和姿态之后移动，沿着父元素的轴，所以它
在缩放或旋转时仍留在指针下，并且会在只绘制的帧中停下，不构建视图，就像
`AnimatePose` 移动它那样。`Dragging()` 报告指针是否正在拖动它，以便在拖动时把它抬起：

```go
lift := ui.Pose{}
if card.Dragging() {
	lift = ui.Pose{Scale: 1.05}
}
card.AnimatePose(lift, ui.Snappy)
```

一旦它停下，`Transitioning()` 为 false，好让视图继续处理，比如移除被滑走的一行：

```go
for i := range app.rows {
	r := &app.rows[i]
	row := ui.Row(c).Key(r.ID)
	row.Draggable(&r.swipe, ui.DraggableOptions{
		Axis: ui.Horizontal,
		Snap: func(p ui.Point) ui.Point {
			if p.X < -160 {
				return ui.Point{X: -400} // away to the left
			}
			return ui.Point{}
		},
	})
	if r.swipe.X == -400 && !row.Transitioning() {
		app.remove(r.ID)
	}
}
```

`DragOffset()` 返回元素在当前帧实际显示的偏移，包括拖动中和松开后尚未停稳时的偏移，
用于联动效果，例如附着式对话框下滑时背景随之淡出。与 `AnimateTo` 一样，读取它会让元素
移动的每一帧都重新构建视图；应在 `Draggable` 之后调用：

```go
backdrop := ui.Box(c).Absolute().Fill().Background(ui.RGBA(0, 0, 0, 0.4))
sheet := ui.Column(c).Draggable(&app.sheet, ui.DraggableOptions{
	Axis:   ui.Vertical,
	Bounds: &ui.Rect{H: 400},
	Snap:   ui.Nearest(ui.Point{}, ui.Point{Y: 400}),
})
backdrop.Opacity(1 - sheet.DragOffset().Y/400)
```

当桌面要求减少动效时，元素跟随指针，并立即停下。

### 手动实现

对于拨盘角度等并非位置偏移的值，可以用底层功能组合手势。`Jump`
立即设置值以跟随指针，同时记录变化速度；松开后，弹簧以该速度
继续动画。`DragEnded` 报告指针释放时的速度，通过 `SpringOptions.Velocity` 传给
弹簧：

```go
dial := ui.Box(c).Size(80, 80)
var turn float32
if dx, _, ok := dial.Dragged(); ok {
	app.turn += dx
	turn = dial.Jump("turn", app.turn)
} else {
	a := ui.Snappy
	if vx, _, ok := dial.DragEnded(); ok {
		// Where the throw would carry it decides the notch it lands on.
		app.turn = float32(math.Round(float64(ui.Project(app.turn, vx)/45))) * 45
		a.Spring.Velocity = vx
	}
	turn = dial.AnimateTo("turn", app.turn, a)
}
dial.Transform(ui.Rotation(turn))
```

一次拖动的最后几次移动会在 `Dragged` 仍报告按下时到来，而 `DragEnded` 在
之后的一次构建中到来：拨盘先转到指针所到的位置，然后弹到一个刻度。

`DragVelocity` 是指针按下时的速度，以沿元素各轴每秒的 DIP 计，停留片刻后为
零。`Velocity(key)` 是正在做动画的值的速度。不移动的点击不会结束拖动。

`ui.Project(value, velocity)` 是快速抛出会把值带到的地方，按 iOS 滚动视图的
减速方式计算，用来选择它落在哪里，如分页器的一页。
`ui.Rubberband(value, lo, hi, dimension)` 让被拖过 lo 或 hi 的值保持弹性，如
iOS 对待被拉过边缘的内容：拖得越远，能移动的距离越小：

```go
if _, dy, ok := sheet.Dragged(); ok {
	app.y += dy
	y = sheet.Jump("y", ui.Rubberband(app.y, 0, 400, 600))
}
```

## 完成时

`Settled(key)` 报告值是否停在它的目标处，`Arrived(key)` 在它停下的那一帧为
true，好让视图继续处理，比如移除淡出的内容：

```go
toast.Opacity(toast.AnimateTo("fade", 0, ui.Animation{Duration: 200 * time.Millisecond}))
if toast.Arrived("fade") {
	app.dismiss(toast)
}
```

## 不使用动画

`c.DisableAnimations()` 让当前帧中的动画值、过渡、姿态和路由页面立即到达目标，类似
SwiftUI 的 `disablesAnimations`，适用于恢复窗口状态、跳转搜索结果等无需动效的变化。
应在构建变化内容之前调用。`Keyframes` 和时间线按序列自行播放，
不依赖目标值，因此不受影响；跳转时间线要用 `Seek`。`c.Instant()` 报告当前帧是否禁用了
动画，自行实现动画的视图也可据此在同一帧中直接到达目标。

## 关键帧

`Keyframes` 按指定时间播放一系列值，每一段使用终点关键帧的 `Ease` 插值，
播放完毕后保持最后一个值：

```go
tile := ui.Box(c).Key("tile").Size(80, 50)
x := tile.Keyframes("x", []ui.Keyframe{
	{Value: 0},
	{At: 200 * time.Millisecond, Value: 100, Ease: ui.EaseOut},
	{At: 700 * time.Millisecond, Value: 40},
	{At: time.Second, Value: 0, Ease: ui.EaseInOut},
}, ui.KeyframeOptions{Run: app.shakes})
tile.Translate(x, 0)
```

第一个时间必须为零，之后的时间必须递增。改变 `Run` 可以再次播放序列，或播放
改变后的序列：被中断后重新播放时，它从当前显示的值开始，而不是像第一次播放
那样跳到第一个值。`Repeat` 是它在第一次之后再次播放的次数，`RepeatForever` 表示只要
它被构建就一直播放，`Alternate` 让每隔一次反向播放；`Delay` 在播放前保持
第一个值一段时间，`RepeatDelay` 在两次播放之间保持它到达的值。

## 时间线

`Timeline` 用同一个播放头驱动多个补间动画，便于统一安排、控制和测试一组动效：
卡片移入时标题淡入、各行依次出现、卡片到位后轻微回弹。`AnimateTo`
将一个元素的一个值过渡到目标，`Keyframes` 按时间节点播放一个值；时间线
则用同一个时钟协调多个值，每个值由独立的补间控制。

元素像保存 `Keyframes` 一样，按键保存时间线。在视图中构建时间线，
并为每个补间指定当前帧显示值的接收位置：

```go
card := ui.Column(c)
tl := card.Timeline("intro", ui.TimelineOptions{Run: app.opened})
x := tl.Tween(ui.Tween{From: -200, To: 0, Duration: 400 * time.Millisecond})
fade := tl.Tween(ui.Tween{From: 0, To: 1, Duration: 300 * time.Millisecond},
	ui.AfterPrevious(-150*time.Millisecond))
card.Translate(x, 0).Opacity(fade)
```

补间默认缓出（`EaseOut`），除非它的 `Ease` 另有说明，和 `Animation` 一样。
没有指定放置方式的补间会在前一个结束后立即开始，所以这里第二个会在 400 ms 开始；
放置方式可以把它放到别处，就像 GSAP 的位置参数那样：

| 放置方式 | 补间开始于 |
| --- | --- |
| `AfterPrevious(50*time.Millisecond)` | 前一个结束后的 50 ms；负偏移与它重叠，如上面的 `-150ms` 让淡入在 250 ms 开始 |
| `WithPrevious(0)` | 与前一个补间同时开始；也可指定相对于该时刻的偏移 |
| `AtLabel("rows", 100*time.Millisecond)` | 某个标签之后的 100 ms |
| `AtTime(500*time.Millisecond)` | 时间线的某个时间 |

任何补间都不会早于时间线开始：早于开始的放置会落在开始处。

### 标签与错开

`Label` 在时间线中标记一个时间点，供补间从此开始；在标签的时间上加上偏移，
就能按索引把它们错开：

```go
tl := list.Timeline("rows", ui.TimelineOptions{Run: app.shown})
tl.Label("rows", ui.AtTime(300*time.Millisecond))
for i := range app.rows {
	app.rows[i] = tl.Tween(ui.Tween{From: 0, To: 1, Duration: 200 * time.Millisecond},
		ui.AtLabel("rows", time.Duration(i)*40*time.Millisecond))
}
for i, shown := range app.rows {
	rows[i].Opacity(shown).Translate(0, (1-shown)*12)
}
```

### 播放

`Duration` 是最后一个补间的结束时间，`Time` 是播放头的当前时间，`Progress` 是
0 到 1 的播放进度，可用于拖动进度条。`Paused` 将播放头固定在原处：
暂停时间不计入播放时间，因此无论暂停多久，恢复后都会从暂停处
继续播放。`Reverse` 从播放头当前位置反向播放到起点，例如
一个按打开时相反方向关闭的面板（`Reverse: !app.open`）；在反向状态下被重新
播放的时间线会从末尾开始。`Seek` 和 `Progress` 在给出期间把时间线保持在某个
时间或某个进度，不给它们时则从那里继续；最多给出其中一个。选项的内容在设置
它的那一帧就生效，所以暂停或再次播放时间线的按钮，在点击的那一帧就显示出
效果：

```go
o := ui.TimelineOptions{Run: app.run, Paused: app.paused, Reverse: app.reverse}
if app.scrubbing {
	o.Progress = &app.scrub
}
tl := card.Timeline("intro", o)
```

`Repeat` 是时间线在第一次之后再次播放的次数，和 `KeyframeOptions.Repeat`
一样，`RepeatForever` 表示只要视图构建它就播放，`Alternate` 让每隔一次反向
播放；`Delay` 在播放前把时间线保持在它的第一个值一段时间，`RepeatDelay` 在
两次播放之间保持它到达的值。`Settled` 报告它已停下，在它的末尾，或反向时在它的
开始；`Arrived` 在它停下的那一帧为 true，如 `Settled(key)` 和 `Arrived(key)`
之于值。

改变 `Run` 可以再次播放时间线，或播放改变后的时间线。构建它时如果不改变
`Run`，播放头会保持在原处，所以视图可以逐帧重新安排它的补间：时间线播放它
这一帧安排的补间。如果安排的补间更长，它会从播放头所在处继续，在安排的时间线
播放完之前不会 `Settled`。视图停止构建元素时，时间线随元素的状态一起消失。
元素每次构建时，对它的每条时间线只会请求一次，它返回的 `*Timeline` 在视图
构建那一帧期间有效。

当桌面要求减少动效时，本会播放的时间线会立即处于它将停下的位置，每个补间
都停在 `To`，如果它反向播放到起点，则停在 `From`，和值、姿态与 `Keyframes`
一样；并且它不请求帧，除非它的 `NonMotion` 说明它只做淡入淡出（见
[减少动效](#减少动效与帧)）。视图自己对播放头做的操作仍然有效，如拖动仍会
移动 `Draggable` 元素：`Seek` 和 `Progress` 把它放到拖动进度条要求的地方，
`Paused` 把它保持在那里。`DisableAnimations` 不改变时间线，它像
`Keyframes` 一样自行播放；要跳转就用 `Seek`。时间线只在播放时绘制帧，由
`c.Now()` 计时。

## 滚动

`InView()` 报告上一帧中，元素是否至少有一部分在周围滚动容器的可见区域内，便于
在元素滚入视口时播放入场动画。相关状态变化时，例如滚动使元素进入视口，
MyGo 会再构建一帧，让视图无需等待下一次输入就能读取新结果。`InViewMargin`
和 `InViewAmount` 也遵循这一规则；不读取这些方法的元素不会因此请求帧。`ScrollState`
（见[滚动](scroll.md)）给出它的容器滚动了多远，用于跟随滚动的效果：

```go
card := ui.Box(c).Key(it.ID).Height(120)
shown := float32(0)
if card.InView() {
	shown = 1
}
card.Opacity(card.AnimateTo("shown", shown, ui.Smooth))

header.Opacity(1 - min(app.scroll.Y/80, 1)) // fades as the page scrolls
```

`InViewMargin(margin...)` 在查看之前把窗口和元素周围的滚动容器扩大 `margin`
个 DIP，如 IntersectionObserver 的 `rootMargin` 和 `scrollMargin` 所做
（Motion 的 `margin`）：写法与 CSS 的 `Padding` 一样，所有边、垂直和水平，或
上、右、下、左。缩略图在滚入前 300 DIP 开始加载，这样它永远不会显示空白：

```go
thumb := ui.Image(c, placeholder).Key(it.ID).Size(160, 120)
if thumb.InViewMargin(300) {
	app.load(it.ID) // once; the next frames show the picture
}
```

负的边距则缩小它们，让元素只有在进入视口那么深之后才算进入视野。它周围的
其他裁剪（`Clip`、`ClipInset`）会按自己的方式裁剪它，不受边距影响。没有边距
的 `InViewMargin()` 就是 `InView()`。`List` 或 `GridView` 只构建靠近其边缘的
行，每边多两行，所以在其中边距不会超出它构建的行。

`InViewAmount(amount, margin...)` 等到元素的这一比例面积（从 0 到 1）显示
出来，如 IntersectionObserver 的 `threshold` 和 Motion 的 `amount` 所做：
一张卡片显示一半才算一次曝光，而不是它在列表底部露头时就算；动画则在它显示
五分之一时开始。`1` 等待全部显示，`0` 等同于 `InViewMargin`；边距和
`InViewMargin` 一样会扩大窗口与滚动容器。它的面积按变换移动后的盒子计算：

```go
card := ui.Column(c).Key(ad.ID)
if card.InViewAmount(0.5) {
	app.counted(ad.ID)
}
```

`ScrollState.ScrollTo(x, y, animation)` 平滑滚动一个容器，带 `Animation`
选项的 `ScrollIntoView` 滚动元素周围的容器（见[滚动](scroll.md)）。

## 变化的内容

`ContentTransition` 为元素内容的变化添加动画：`Text` 的文本、`Icon` 的图标、
`Image` 的图片。默认旧内容淡出、新内容淡入；旧图标缩小消失，新图标放大出现。
`Numeric` 让一行文本中变化的字符滚动，如计数器的数字，不变的字符则保持
不动：向上，或带 `Down` 时向下，如 SwiftUI 的 numericText：

```go
ui.Text(c, strconv.Itoa(app.count)).FontSize(32).Bold().
	ContentTransition(ui.ContentTransition{Numeric: true, Down: app.decreased})
ui.Icon(c, playOrPause).ContentTransition(ui.ContentTransition{})
```

`Morph` 把新旧文本共有的字符从它们原来的位置移到它们现在的位置，只属于旧
文本的字符淡出，只属于新文本的字符淡入，如 Keynote 的 Magic Move 按字符
进行：词语重排、标题改写、字母重新排列。富文本也会变形，它的字符会变成所在
文本段的颜色。用 [`Match`](transitions.md#变形的文字) 飞行的文本，会从它
`Match` 的那个元素的文本变形而来。

```go
ui.Text(c, app.title).ContentTransition(ui.ContentTransition{Morph: true})
```

内容过渡在只绘制的帧中运行。当桌面要求减少动效时，内容淡入淡出而不移动。

## 循环与自己的动效

`Loop` 返回周期性循环动画的进度，可用于加载指示器和脉冲效果（见
[绘制与动画](drawing.md#动画)）。要实现其他动画，可根据 `c.Now()` 计算状态，
并在动画的每一帧调用 `c.AnimationFrame()`。

## 减少动效与帧

当桌面要求减少动效时（`c.Preferences().ReduceMotion`），值和姿态立即到达
目标，序列停在它们的最后一个值。不移动而变化的东西仍然平滑变化：颜色
（`AnimateColor`）、姿态的 `Fade`、内容交叉淡入淡出，以及
[过渡](transitions.md)的颜色和淡入淡出。`Loop` 继续，如系统的加载指示器
那样。

视图需要明确说明动画值是否会让内容移动。不涉及移动的值，例如不透明度、模糊或
视图自行驱动的交叉淡入淡出，可在 `Animation`、`KeyframeOptions` 或
`TimelineOptions` 中设置 `NonMotion`。这样启用减少动效后仍会播放动画，与
元素自身的淡入淡出行为一致：

```go
op := e.AnimateTo("fade", target, ui.Animation{Duration: 200 * time.Millisecond, NonMotion: true})
ui.Image(c, first).Opacity(1 - op)
ui.Image(c, second).Opacity(op)
```

对于位置、尺寸、缩放或滚动，不要设置它，并给会移动的部分单独配一条时间线，
与淡入淡出的时间线并列，它会在减少动效时立即停到终点。

动画只在内容仍有变化时按显示器刷新率绘制帧；窗口不可见时（见
下文）则不绘制。如果变化的值需要由视图读取，动画帧会运行视图；姿态、内容过渡，
以及在 `Draw` 中计算的动画则只重绘。

## 不在视线内

当窗口没有任何部分显示时（macOS 上：隐藏、最小化、在另一个 Space 上，或被
其他窗口完全遮住；浏览器中：后台标签页；Windows 上以及 X11 下的 Linux：
最小化），MyGo 不会为它绘制帧，如浏览器不为不在视线内的页面绘制任何东西，
也如 Apple 对 Mac 应用的要求。视图不运行，`Draw` 不被调用，`After` 和
`Painter.After` 请求的帧会等待。只要窗口有一部分显示，它就绘制一次，带上
期间变化的一切；在那之前，屏幕显示它的最后一帧，大约一个刷新周期。输入之后
（输入到被其他应用窗口遮住的键窗口中的按键）、辅助功能打开时，以及为
`Window.CapturePage`，仍会构建一帧，好让捕获显示窗口现在的样子。Windows 和
X11 只告诉 MyGo 窗口最小化了，别的不说，Wayland 什么都不说，所以被其他窗口
遮住的窗口，或在另一个桌面上的窗口，仍像以往一样在那里绘制。在 macOS 上，
不在视线内的窗口还会在最后一帧之后半秒交还 GPU 为其帧占用的内存，只保留它
显示的那一帧，即 Dock、Mission Control 和窗口截图所显示的那一帧：后台标签页中的
窗口只占用一帧（与窗口一样大）的内存，而不是两到三帧。

这对动效意味着：

- **MyGo 做的动画会按时间继续到相应位置。** 窗口离开视线时正在运行的弹簧、
  途中的 `Animate`、`Loop`、关键帧和时间线，在窗口重新显示时已经走得更远，
  或已经完成。
- **在视线外改变的目标会在窗口显示时动画。** 视图到那时才第一次看到新目标，
  所以值会从它原来所在处移向它，就像 CSS 过渡处理在隐藏标签页中做出的更改
  那样。
- **从时间而不是帧来计算自己的动效。** 在绘制东西的第一帧取的开始时间，或
  帧的计数，会在窗口显示时重新开始，并把期间发生的事当作刚开始一样播放。
  在改变状态的代码里，取状态改变那一刻的时间，视图就会把在视线外做出的改变
  显示为已完成：

```go
// counter eases from one value to the next over 400 ms, by the clock.
type counter struct {
	from, to float32
	changed  time.Time
}

const ease = 400 * time.Millisecond

// at returns the value shown at t.
func (n counter) at(t time.Time) float32 {
	p := min(float32(t.Sub(n.changed))/float32(ease), 1)
	return n.from + (n.to-n.from)*ui.EaseOut(p)
}

// Where the count changes, outside the view:
now := time.Now()
app.count = counter{from: app.count.at(now), to: float32(n), changed: now}
win.Update(func() {})

// In the view:
value := app.count.at(c.Now())
if c.Now().Sub(app.count.changed) < ease {
	c.AnimationFrame()
}
```

- **不要在视图中执行后台工作。** 在视图、`After` 或 `Draw` 中轮询、保存或启动
  任务，都会在窗口不可见时暂停。应在应用中通过 goroutine 或 `time.Ticker` 执行
  这些工作，再用 `Window.Update` 更新界面。
