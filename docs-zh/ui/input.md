# 输入

元素会在你构建它们时报告用户自上一帧以来对它们做了什么：查询一下，然后在
构建该元素的地方处理结果。

一个 `ui.Element` 值只在一次构建中有效。它的零值表示不存在，对该值的
查询返回 false 或零值。用 `Valid` 检查可选控件。开发构建和 `Tester` 会检测
过期元素的使用；生产构建中的方法则返回空结果或忽略操作。

要跨构建保留焦点或快捷键，就保存一个 `ui.Handle`。每个窗口对该句柄各有
自己的绑定；查询时要用该窗口的 Context。

```go
card := ui.Column(c).Padding(12).Radius(8).Focusable()
if card.Hovered() {
	card.Background(t.SurfaceHover)
}
if card.DoubleClicked() {
	app.open(item)
}
```

## 指针

`Hovered`、`Pressed`、`Clicked`、`DoubleClicked`、`RightClicked`、`Dragged`
（按下元素期间，指针自上一帧以来移动了多远）和 `PointerPosition`。
`ClickModifiers` 返回最后一次点击时按住的修饰键，如 Shift 点击时的 Shift。
`c.Modifiers()` 返回当前按住的修饰键，修饰键单独按下或松开时也一样，视图会
在它们变化时重新绘制：一个在按住 Cmd 时显示每行 Cmd+1–9 的列表会读取它。
`PassThrough` 让指针穿过元素到达下面的内容；`HitTest` 根据指针位置的函数
决定哪里可以穿过，如圆环的孔或饼图各段之间的空隙（见[绘制](drawing.md#填充规则与命中)）；
`IgnoreInput` 让指针穿过元素及其内部元素，并阻止它们获得键盘焦点；再配合
[`AccessHidden`](accessibility.md#不挡辅助功能的路) 对辅助技术隐藏这些元素，
即可完全禁用交互。

接收指针的元素会把指针交给它下面最内层的元素：可点击行中的按钮接收自己的
点击。

`PressedInside` 报告在元素上或元素内部的按下，而元素并不接收指针；`Pressed`
和 `Clicked` 则会让它接收指针：表格的单元格能注意到 Alt 点击，而它的行仍然
负责选择：

```go
cell := ui.Box(c).Children(func() { ui.Text(c, value) })
if cell.PressedInside() && c.Modifiers()&ui.Alt != 0 {
	app.inspect(row, col)
}
```

当指针在元素内按住时，`Pressed` 保持为 true。如果一次松开和尚未被任何构建
读取的移动一起到来，那么在读取这些移动的构建里，元素仍算按下，指针停在松开
的位置（`PointerPosition`），到下一次构建才变为松开。观察到按下的开始后，
视图会在绘制前重建，所以由 `if row.Pressed() || row.Clicked()` 选择的行会在
那一帧显示新的选中色和焦点色。要防止这次选择重复选中同一项。`Clicked` 报告
在元素内松开时的激活，或通过键盘或辅助技术触发的激活；按钮使用这种激活。

`Pressed`、`Clicked`、`Dragged`、`DragVelocity` 和 `DragEnded` 只跟随主
按钮，正如 CSS 的 `:active` 和 Web 的拖动手势那样（`event.button === 0`）：
右键或中键拖动不会移动卡片、滑块或分隔条，由此产生的只有右键点击
（`RightClicked`），或它打开的[上下文菜单](context-menu.md)。`PressedButton`
在按下持续期间返回按下元素的按钮，无论是哪个（0 主按钮，1 辅助按钮，2 中
键）；配合 `PointerPosition`，它就能构建其他按钮的手势，如用中键平移的画布：

```go
x, y, _ := canvas.PointerPosition()
if b, ok := canvas.PressedButton(); ok && b == 2 {
	if app.panning {
		app.pan.X += x - app.last.X
		app.pan.Y += y - app.last.Y
	}
	app.panning, app.last = true, ui.Point{X: x, Y: y}
} else {
	app.panning = false
}
```

每次按下都来自主指针：鼠标，或触摸屏上的第一根手指；第二根手指不会按下任何
东西，在浏览器中也是如此（`isPrimary`）。[`HandleInput`](#每个键随到随处理)
能通过 `InputEvent.Button` 接收每个按钮的按下和松开事件，
[`Touches`](#触摸屏上的手指) 则能得到每根手指。

`PressedPointer` 说明是哪种设备在按元素、按得多重，如同 Pointer Events 的
`pointerType` 和 `pressure`：`PointerMouse`（鼠标或触摸板）、`PointerPen`
（数位板或屏幕上的笔）或 `PointerTouch`（触摸屏上的手指），以及 0 到 1 的
压力，取自这次按下最后一次移动。不报告压力的设备，如鼠标，在按钮按下时压力
为 0.5。`HandleInput` 在每个 `InputPointerDown`、`InputPointerMove` 和
`InputPointerUp` 上得到同样的信息（`InputEvent.Pointer` 和 `Pressure`），
每次移动随到随得，而不是每帧一次；笔在原处加重或减轻压力也会作为一次移动
到来。签名板用笔的压力加宽笔画，对鼠标则依据指针的速度：

```go
ui.Box(c).Grow(1).Draw(app.paint).HandleInput(func(ev ui.InputEvent) bool {
	switch {
	case ev.Kind == ui.InputPointerDown, ev.Kind == ui.InputPointerMove && ev.Button == 0:
		width := app.widthBySpeed(ev.X, ev.Y, ev.Time) // a mouse's, or a finger's
		if ev.Pointer == ui.PointerPen {
			width = 1 + 5*ev.Pressure
		}
		app.stroke = append(app.stroke, stroke{ev.X, ev.Y, width})
		return true
	}
	return ev.Kind == ui.InputPointerUp
})
```

各系统都会报告按下和移动事件所用的设备：macOS 上是 AppKit 的数位板事件（鼠标
事件的 subtype 和 `pressure`，以及移动之间的 `tabletPoint:`），Windows 上是
笔和手指的 `WM_POINTER` 消息——Windows 会把它们转成标记为来自这些设备的
鼠标消息，Linux 上是 GDK 的源设备和压力轴，浏览器中是 `PointerEvent`。
`Tester` 在 `UsePointer(ui.PointerPen, 0.6)` 之后以笔或手指按下。

### 系统取消的按下

系统可以在指针没有松开的情况下结束一次按下，如同 Pointer Events 的
`pointercancel`：浏览器把触摸变成页面滚动，按住按钮时菜单或另一个窗口夺走
鼠标输入，或 Windows 停止跟踪笔或手指。窗口失去键盘焦点也会以同样的方式结束按下。
这样的按下不会提交松开本会提交的任何东西：它不点击任何东西（`Clicked`、
`RightClicked`），不放下它拖动的值（`Drop`），不结束拖动（`DragEnded`），
也不打开上下文菜单，`Draggable` 元素会回到按下把它拖走前的位置。
`PressCanceled` 在下一帧报告取消事件，供手势处理逻辑撤销已开始的操作：

```go
dx, _, held := handle.Dragged()
switch {
case held:
	app.preview += dx
case handle.PressCanceled():
	app.preview = 0 // as it was
case app.preview != 0:
	app.width, app.preview = app.width+app.preview, 0
}
```

`HandleInput` 仍会收到 `InputPointerUp`，以便释放按下时保留的状态；
此时 `InputEvent.Canceled` 已置位，应回滚而不是提交操作。

```go
ui.Box(c).Size(120, 40).HandleInput(func(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputPointerDown:
		app.armed = true
	case ui.InputPointerUp:
		if app.armed && !ev.Canceled {
			app.fire()
		}
		app.armed = false
	}
	return true
})
```

`Tester` 用 `CancelPress` 取消一次按下。

指针按下某个元素时，按下开始时指针所在的那些元素——如按钮周围的行——只要
指针还在它们上面，就保持 `Hovered`，和在 CSS 中一样，其他元素则不再悬停：
拖过其他元素不会点亮它们。行被悬停时才显示的按钮，在按下时要保持不变，才能
接住点击：

```go
row := ui.Row(c).Padding(8)
hovered := row.Hovered()
row.Children(func() {
	ui.Text(c, item.Title).Grow(1)
	if hovered && ui.Button(c, "Remove").Clicked() {
		removed = item.ID
	}
})
```

只在行被悬停时构建的按钮，对辅助技术来说并不存在。`IgnoreInput` 让它保持
构建，平时保持透明且不响应指针和键盘输入，但仍能被辅助技术读取和激活（见
[辅助功能](accessibility.md#不挡指针的路)）：

```go
del := ui.Button(c, "Remove").IgnoreInput(!hovered)
if !hovered {
	del.Opacity(0)
}
```

## 键盘焦点

`Focusable` 元素在被点击时获得焦点，Tab 和 Shift+Tab 在它们之间移动焦点，
用键盘移动焦点时还会显示焦点环。`AutoFocus` 在元素出现时给它焦点，如对话框
的第一个字段；`Focus` 在你调用期间让焦点保持在那里；`Focused`、
`FocusVisible` 和 `FocusWithin` 报告它。Enter 和空格按下有焦点的按钮，空格切换有焦点的复选框、
开关、切换按钮或单选按钮。有焦点的链接，或带 `Role(ui.RoleLink)` 的元素，
只接受 Enter，如同网页中那样：空格滚动它周围的容器。

当焦点通过键盘或应用（`Focus`、`AutoFocus`、[句柄](#持久的焦点)的 `Focus`，
或用 [`FocusBind`](#绑定到数据的焦点) 绑定的字段）移到某个元素时，它周围的
滚动容器会以刚好显示它的最小幅度滚动，如 Web 的 `element.focus()` 那样，这样
你自己的键盘导航会让焦点保持在视野中。它只在焦点移动时滚动一次：每帧都调用
`Focus` 不会干扰用户的滚动。点击会让元素在原地获得焦点，按住指针按钮时调用
`Focus` 也一样，例如一行通过按下它的手柄来获得焦点。要以其他方式让它对齐，
就在它获得焦点的那一帧调用 `ScrollIntoView` 并带上它的选项。要让滚动留在
原处，如 Web 的 `focus({ preventScroll: true })`，传入 `ui.FocusOptions`：
`cell.Focus(ui.FocusOptions{PreventScroll: true})` 在对话框关闭时把焦点还给
网格的某个单元格，而不滚动网格。对于把焦点交给内部输入框的字段，这个选项也
同样有效。零值选项等同于不带选项的 `Focus()`。

`TabStop(false)` 让元素可聚焦但不在 Tab 顺序中，如同 HTML 的
`tabindex="-1"`：它仍可由点击、`Focus`、`AutoFocus` 和辅助技术聚焦，
而 Tab 和 Shift+Tab 会越过它，继续到它内部和它之后的控件。卡片或面板通过
这种方式获得焦点，从而能接收自己的按键，又不会排在自己的按钮之前；
`TabStop(true)` 把它放回顺序中。在[焦点组](#焦点组)中，Tab 从它停留的成员进入，而方向键仍能到达其他
成员。

```go
card := ui.Column(c).Padding(12).TabStop(false).Children(func() {
	ui.Text(c, item.Title)
	ui.Button(c, "Open") // Tab goes here, past the card
})
if card.Shortcut(0, ui.KeyDelete) {
	app.remove(item)
}
```

`FocusLeft` 报告自上一帧以来焦点离开了某个元素：它曾在元素上或元素内，现在
在元素外，或没有任何元素获得焦点，例如窗口失去键盘焦点时。这对应 Web 的 `focusout`，其
`relatedTarget` 不在元素内。附加到该元素及其内部元素（`AttachTo`）的浮层，
如组合框的列表或按钮的浮层，都在它内部，`PressedOutside` 也是如此，所以弹出
字段在焦点离开它时关闭，但在焦点移入它自己的列表时不关闭：

```go
var field ui.Element
combo := ui.Column(c).Children(func() {
	field = ui.TextInput(c, &app.query)
})
if combo.FocusLeft() {
	app.open = false
}
if app.open {
	ui.Overlay(c, func() {
		ui.Column(c).AttachTo(field, ui.AnchorBottomLeft, ui.AnchorTopLeft).Children(app.options)
	})
}
```

焦点去了哪里，由元素在构建时报告（`Focused`、`FocusWithin`），以及在控件
构建之前由[句柄](#持久的焦点)的 `FocusWithin(c)` 报告。

`c.FocusTakesText()` 报告当前焦点是否接收文本，无论是键入的还是合成的：
不是只读的文本输入或文本区域、富文本或代码编辑器，或自己接收文本的元素
（`HandleTextInput`、`TextCaret`）。它就是 Web 上单键快捷键所用的判断
（`isContentEditable`、`input` 或 `textarea`）：在字段中输入文本时，命令
面板不应响应字母快捷键。

```go
if !c.FocusTakesText() && c.Shortcut(0, ui.KeyK) {
	app.palette = true
}
```

## 持久的焦点

在每次构建时把应用持有的句柄绑定到一个控件：

```go
ui.TextInput(c.Key("search"), &app.query).Bind(&app.search)
```

在应用状态中声明 `search ui.Handle`。`app.search.Focus()` 请求焦点，并在
控件隐藏时等待。`CancelFocus()` 取消请求。`Focused(c)` 和 `FocusWithin(c)`
可在控件构建前后查询其焦点状态。`c.Resolve(app.search)` 只返回本次构建的
元素。`app.search.Bounds(c)` 从已提交的帧读取控件边界，输入回调中及
两次构建之间也可查询。关闭窗口会取消该窗口的焦点请求。

一个句柄可以在多个窗口中绑定。查询时要用那个窗口的 Context；用 `Focus(c)`
或 `CancelFocus(c)` 可以显式选择窗口。不带参数的焦点调用要求至多有一个
仍有效的窗口绑定。

## 绑定到数据的焦点

```go
type pane int
const (none pane = iota; files; diff)

ui.Column(c.Key("files")).FocusBind(&app.pane, files).Children(func() { app.filesView(c) })
ui.Column(c.Key("diff")).FocusBind(&app.pane, diff).Children(func() { app.diffView(c) })

// In an action:
app.pane = diff

// Actual focus in this window:
focused := ui.FocusedValue(c, &app.pane)
```

`FocusBind` 接受指向可比较字段的指针和一个匹配的值。每个值在窗口中命名一个
控件；把零值保留给无焦点。字段保存期望的焦点。如果它的控件隐藏，请求会等待，
所以等待时用 `FocusedValue` 读取实际焦点。赋零值会清除焦点。没有待处理的
请求时，用户的焦点变化会更新字段。为不同窗口使用不同的字段。

## 焦点组

工具栏、单选按钮组、分段控件或标签页列表的控件是 Tab 的一个停靠点，与原生
控件一样：Tab 把焦点移入组，移到上次拥有它的控件（或被选中的单选按钮或标签
页），然后移出组，而方向键在它们之间移动焦点，Home 和 End 移到第一个和最后
一个。`FocusGroup` 为你自己的容器做这件事，方向键可以是 `ui.Horizontal`、
`ui.Vertical` 或两者；里面的滑块或文本输入保留自己接收的方向键。

```go
ui.Row(c).Gap(4).FocusGroup(ui.Horizontal).Children(func() {
	for _, tool := range tools {
		if ui.Button(c, tool.Name).Clicked() {
			app.tool = tool
		}
	}
})
```

## 快捷键

`c.Shortcut(ui.Cmd, ui.KeyS)` 报告在窗口任意位置以恰好这些修饰键按下的键，
`Element.Shortcut` 只在元素或它内部的某个元素有焦点时才报告，而且它优先。
有焦点的按钮保留 Enter 和空格，链接保留 Enter，切换按钮保留空格，所以
`c.Shortcut(0, ui.KeyEnter)` 可在焦点位于其他位置时按下对话框的默认按钮。
文本输入框同样保留 Enter：单行输入框用 [`Submitted`](text-input.md) 报告它，文本区域则换行。

```go
if c.Shortcut(ui.Cmd, ui.KeyS) {
	app.save()
}
```

执行与快捷键相同操作的按钮，用
[`KeyShortcuts`](accessibility.md#键盘快捷键) 告诉屏幕阅读器它的键，如
`ui.Button(c, "Save").KeyShortcuts(ui.Cmd, ui.KeyS)`。

按住的键会重复，每次重复都再次作为一次按下到来，所以步进的动作在按住键时
持续运行。`c.ShortcutRepeated()` 说明最后报告按键的那个快捷键所接受的键是否
来自这样的一次重复，如同 Web 的 `KeyboardEvent.repeat`，无论那是哪个快捷键
（窗口的、元素的、列表的还是浮层的）；`OnShortcut` 动作可以在运行时询问它。
切换按钮只接受按下：

```go
if c.Shortcut(0, ui.KeyUp) {
	app.count++ // and again while Up is held
}
if c.Shortcut(0, ui.KeyT) && !c.ShortcutRepeated() {
	app.panel = !app.panel // once a press
}
```

动作即使忽略了重复按键，该事件仍会被消费，不会再传给其他快捷键。GTK 不会
标记重复，所以在 Linux 上，一个已经按住的键再次按下（中间没有松开）就是一次
重复，正如 X11（带可检测的自动重复，GDK 会打开它）和 Wayland 发送它们那样。

`ui.Cmd` 在 macOS 和 iOS 上是 Command，在其他平台上是 Ctrl。`ui.KeyBack` 和
`ui.KeyForward` 是鼠标的后退和前进按钮，以及有此键的键盘上的键，
[路由器](navigation.md)会接受它们。[菜单](../menus.md)的快捷键仍然有效，
编辑菜单的角色（剪切、复制、粘贴、全选、撤销、重做）作用于有焦点的文本输入。
如果当前焦点不处理这类命令，例如焦点在没有可编辑内容的列表中，命令会转交给对应按键
的快捷键处理。因此，即使 macOS 的菜单先于窗口处理 Command+V，
`c.Shortcut(ui.Cmd, ui.KeyV)` 也能收到粘贴命令。编辑菜单项只有在当前焦点
或对应快捷键能够处理该角色的命令时才会启用。

快捷键在不能输入拉丁字母的键盘布局上也能用：按住 Cmd、Ctrl、Alt 或 Super
时，某个键输入的不是 US 键位上的字母——如俄语布局上的 ы——它就代表 US 键盘
上同一位置的键，所以 Cmd+ы 是 `ui.KeyS`。不加这些修饰键输入时，它仍是文本。

有焦点的文本输入会先处理平台的编辑键：在 macOS 和 iOS 上，Option 和 Command 配合
方向键和 Backspace，以及 Control 配合 A、E、B、F、N、P、D、H 和 K，如同其他
Mac 应用；在其他平台，它让出 Alt 和方向键（它们用于后退和前进）以及功能键。

快捷键动作可以在绑定其控件之前声明：

```go
app.filesView.OnShortcut(c, ui.Cmd, ui.KeyK, app.openSelected)
ui.List(c.Key("files"), &app.list, len(app.files)).Bind(&app.filesView).
    Rows(func(row ui.ListRow) {
        ui.Text(row.Context, app.files[row.Index].Name)
    })
```

`Handle.OnShortcut` 只在窗口为该句柄构建了启用的控件时，才在构建之后运行。
隐藏的控件不接收命令。`c.OnShortcut` 在当前父作用域中声明一个动作；根
Context 处理有焦点的控件和活动浮层剩下的键。对于元素焦点子树内的动作，使用
`Element.OnShortcut`。`c.OnWindowShortcut` 无论 `c` 在哪里都声明窗口的动作，
正如 `c.Shortcut` 报告它的那样：构建在视图深处的组件，比如一个显示按键并执行
该快捷键的提示组件，无论焦点在其他位置还是没有焦点，都能响应该快捷键。组件在对话框内部构建时，
可在对话框显示期间执行动作；在对话框背后构建时，则暂停响应。

```go
func shortcutHint(c *ui.Context, key ui.Key, label string, run func()) {
	c.OnWindowShortcut(ui.Cmd, key, run)
	ui.Text(c, label) // "⌘K"
}
```

## 按住的键

`c.KeyDown(ui.KeyK)` 报告窗口中是否按住某个键，`c.KeysDown()` 列出按住的键，
按它们按下的顺序，无论哪个元素有焦点，也无论它是否接下了它们：它们只做观察，
所以有焦点的字段仍能输入。读取它们的视图会在它们变化时重新绘制。快捷键速查
表会在它的搜索框有焦点时点亮按住的键帽，并淡化不使用其中任何键的行：

```go
ui.SearchField(c, &app.query).Label("Search shortcuts").AutoFocus()
held, mods := c.KeysDown(), c.Modifiers()
for _, s := range app.shortcuts {
	row := shortcutRow(c, s, held, mods) // lights the caps held
	if (len(held) > 0 || mods != 0) && !s.uses(held, mods) {
		row.Opacity(0.4)
	}
}
```

键的计法与快捷键接收它们时相同，所以俄语布局上的 Cmd+ы 按住的是 `ui.KeyS`；
修饰键则来自 `c.Modifiers()`。按下后会执行菜单项快捷键的键会转到菜单，而不是
窗口。窗口失去键盘焦点时会清除所有按键的按住状态，因为松开事件会发送到
新的窗口或应用；系统不向窗口发送松开事件时，也会清除相应按键状态：在 macOS 上和 Mac 的
浏览器中，是在按住 Command 时松开的键；在 Windows 上，是菜单打开时按住的键，
如 Alt 加字母或 Alt 加空格，它们的松开由菜单接收；以及输入法接收其松开的键，
如它在键按住时开始合成。`Tester` 用 `KeyDown` 和 `KeyUp` 按住键，
`SetFocused(false)` 松开它们。

## 输入法

文本输入会接收用输入法合成的文本，输入法能看到插入点周围的文本：macOS 的
长按会替换要加上变音符号的字母，日语输入法会再次转换已输入的文本，其他输入
法则根据前面的内容预测。

## 拖入的文件

`DroppedFiles` 返回从 Finder、Explorer 或文件管理器拖放到元素上的文件路径，
`FileDragOver` 报告拖到它上面的文件，以显示它会接受它们。窗口只在这类元素上
接受文件；当它有 `OnFileDrop` 监听器时，可以在任何地方接受，监听器得到没有
元素接受的文件，以及它们被放下的位置：

```go
zone := ui.Column(c).Size(260, 80).Border(1, t.Border)
if files := zone.DroppedFiles(); files != nil {
	app.files = files
}
if zone.FileDragOver() {
	zone.Border(2, t.Accent)
}
```

在窗口内或窗口之间拖动的值，以及与其他应用交换的序列化数据，属于
[拖放](drag-and-drop.md)。

## 滚动和捏合

滚动容器自己用滚轮和触摸板滚动。用它们做其他事的元素，如缩放和平移 K 线图
的图表，通过 `HandleInput` 接收它们：`ui.InputScroll` 按 `DX` 和 `DY` DIP
滚动，正值向下、向内容右侧，`X` 和 `Y` 是指针相对元素的位置；`Precise` 用来
区分按像素滚动的触摸板和按格滚动的鼠标滚轮。`ui.InputZoom` 是触摸板的捏合，
位于 `X` 和 `Y`，按 `Scale` 缩放：2 表示内容显示为原来的两倍大。macOS、
Wayland 上的 Linux 和浏览器报告捏合；Windows 把它们作为按住 Ctrl 的滚动
发送，所以在那里，按住 Ctrl 滚动触摸板也会作为 `ui.InputZoom` 到来，而鼠标
滚轮的格仍是 `ui.InputScroll`，`Mods` 中含 `ui.Ctrl`。在 Linux 上，触摸屏
像触摸板那样滚动和捏合：手指拖动会带着它下面的内容一起滚动（`Precise`），
两根手指在它们之间捏合，一起移动时则滚动，点按会点击，手指静止半秒会右键
点击。元素接下的输入，周围的滚动容器不会再跟着滚动；元素放过的输入（返回
false），则由它们来滚动。

```go
ui.Box(c).Fill().HandleInput(func(ev ui.InputEvent) bool {
	switch {
	case ev.Kind == ui.InputZoom:
		app.chart.zoom(ev.X, ev.Scale)
	case ev.Kind == ui.InputScroll && !ev.Precise:
		// A notch, which scrolls further on some platforms than on
		// others, zooms by a quarter.
		if ev.DY < 0 {
			app.chart.zoom(ev.X, 1.25)
		} else if ev.DY > 0 {
			app.chart.zoom(ev.X, 0.8)
		}
	case ev.Kind == ui.InputScroll:
		app.chart.pan(ev.DX)
	default:
		return false
	}
	return true
})
```

`Tester` 用 `Scroll`（触摸板）、`Wheel`（一格）和 `Pinch` 滚动。

## 触摸屏上的手指

`Touches` 返回落在元素或其内部元素上的触摸屏手指：每根按着的手指，以及自上次
构建以来抬起或被系统接管的手指，按落下的顺序排列。每个 `ui.Touch` 从手指落下
到抬起都保持同一个 `ID`，不受其他手指影响，并带有它相对元素盒子的当前位置
（`X`、`Y`）和落下的位置（`StartX`、`StartY`）、按压力度（`Pressure`，系统
不报告时为 0.5）、与屏幕接触的大小（`Width`、`Height`，以元素的单位计，系统
不报告时为 0），以及自上次构建以来的 `Phase`：`ui.TouchBegan`、
`ui.TouchMoved`、`ui.TouchStationary`、`ui.TouchEnded`（在它抬起的位置）或
`ui.TouchCanceled`（被系统接管，如屏幕边缘的系统手势：不是完成的手势）。结束或
被取消的手指只在一次构建中出现，之后就消失。请求它们的元素会在手指落下、移动
或抬起时得到一帧。

手指照常做它们原本做的事：点按会点击，一根手指滚动它下面的内容，两根手指捏合，
在 Linux 上手指静止按住会右键点击。不请求触摸的应用看不出任何区别，窗口也只在
有视图请求之后才跟踪手指：之前已按下的手指不会报告。有了每根手指，视图就能识别
自己的手势，如大屏上的白板为每根手指画一笔，四指滑动翻页，五指点按清屏：

```go
canvas := ui.Box(c).Grow(1).Draw(app.paint)
fingers := canvas.Touches()
down, dx := 0, float32(0)
for _, t := range fingers {
	if t.Phase < ui.TouchEnded {
		down++
	}
	dx += (t.X - t.StartX) / float32(len(fingers))
	if app.most <= 1 && t.Phase != ui.TouchCanceled {
		app.ink.LineTo(t.ID, t.X, t.Y) // a stroke for each finger, by its ID
	}
}
if down > app.most {
	app.most = down // the most fingers down at once in this gesture
	if down == 1 {
		app.began = c.Now()
	}
}
if len(fingers) > 0 && down == 0 { // the last finger lifted
	quick := c.Now().Sub(app.began) < 300*time.Millisecond
	switch {
	case app.most == 4 && dx < -120:
		app.nextPage()
	case app.most == 4 && dx > 120:
		app.previousPage()
	case app.most == 5 && quick && math.Abs(float64(dx)) < 10:
		app.ink.Clear()
	}
	app.most = 0
}
```

Windows（`WM_POINTER`）、Linux（GDK 的触摸序列）和浏览器（`pointerType` 为
`touch` 的 Pointer Events，每根手指由其 `pointerId` 区分）报告触摸。macOS
没有触摸屏，触控板上的手指也不是屏幕上的点：它不报告触摸，它的捏合和轻扫作为
`ui.InputZoom`、`ui.InputScroll` 和历史键到来。`Tester` 用
`TouchDown(id, x, y)` 放下手指，用 `TouchMove`、`TouchUp` 和 `TouchCancel`
移动、抬起和取消它们；它只发送触摸，不发送后端由它们生成的点按和滚动。

Windows 会把三指和四指的轻扫与点按留给它自己的手势（切换应用和桌面），除非
用户在“设置 > 蓝牙和其他设备 > 触摸”中关闭它们：在此之前应用最多只能看到
两根手指。在一次构建之前就落下又抬起的手指，比如快速
点按，会在一次构建中以 `TouchBegan` 出现，在下一次构建中以 `TouchEnded`
出现，因此统计落下手指数的视图会数到每一根。

### 手掌与手指

手掌按在屏幕上的面积比指尖大，大屏上的白板据接触的大小分辨二者，用手掌擦除，
用手指书写：

```go
for _, t := range canvas.Touches() {
	if t.Phase == ui.TouchEnded || t.Phase == ui.TouchCanceled {
		continue
	}
	if t.Width*t.Height > 40*40 { // a palm
		app.ink.Erase(t.X, t.Y, t.Width, t.Height)
	} else {
		app.ink.LineTo(t.ID, t.X, t.Y)
	}
}
```

大小以元素的单位计：在放大 2 倍的元素上减半，与 `X`、`Y` 一样。手指在原地
变成手掌时以 `TouchMoved` 到来。系统报告它的地方：

| 平台 | 接触大小 |
| --- | --- |
| Windows | 手指的 `POINTER_TOUCH_INFO` 的 `rcContact`，触摸屏报告时（`TOUCH_MASK_CONTACTAREA`），以 DIP 计 |
| 浏览器 | Pointer Event 的 `width` 和 `height`；浏览器报告 1×1 时为 0，它无法得知时就这样报告 |
| Linux | 无：GDK 不报告接触大小，`Width` 和 `Height` 为 0 |
| macOS | 没有触摸屏，也就没有触摸 |

为白板制造的大触摸屏通常会在 Windows 上报告接触大小。需要它的应用在 `Width`
和 `Height` 为 0 时应以其他方式分辨手掌与手指。

## 每个键，随到随处理

自己接收每个键的控件，如[终端](../plugins/terminal.md)，通过 `HandleInput`
在下一帧之前随到随得地得到输入：按下和松开的按键，有焦点时键入和合成的文本，
菜单的编辑命令，以及在它们上面按下、移动或滚动的指针。
`InputEvent.PhysicalKey` 是按 US 键盘上的位置表示的键，无论布局输入什么，
用于按位置起作用的键，如游戏中的 WASD。函数会报告自己是否接下了该事件；放过
的事件照常传给快捷键、Tab、上下文菜单和滚动容器，而窗口或焦点周围的元素用
`Shortcut` 处理的键会先转到那里。`TextCaret` 为这样的元素在它有焦点时打开
系统的输入法，在它给出的插入点合成；`InputMethod(false)` 会再次关闭输入法，
但按键仍然输入文本，如同 Vim 的普通模式，浏览器除外，它们做不到。
`c.ReadClipboard`、`c.WriteClipboard` 和 `c.OpenURL` 为它们复制、粘贴和
打开链接；`c.OpenURLThen` 也打开链接，系统打开链接后，它的函数稍后会得到
结果，没有应用能打开时则会得到错误，视图随后重新构建一帧。`c.WriteClipboardThen`
以同样的方式复制文本，如同 `navigator.clipboard.writeText` 成功完成或被拒绝：
文本没能写入时它的函数会得到错误，如另一个应用持有剪贴板时，或浏览器拒绝
用户未主动触发的写入时，这样复制按钮可以显示“已复制”或“复制失败”
（在 `FailClipboard` 之后，`Tester` 会让它们失败）。`Services.ReadClipboardData`
和 `WriteClipboardData` 一次读写多种格式，如富文本编辑器复制它自己的格式、
HTML 和纯文本，而 `ClipboardFormats` 不读取就列出剪贴板的格式，如终端决定
粘贴读取什么。应用自己的文本字段在屏幕阅读器移动其选区时得到 `InputSelect`：
其 `Access` 所描述文本中从 `Start` 到 `End` 的 rune。

```go
ui.Box(c).Fill().Focusable().HandleInput(func(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputKeyDown:
		return app.key(ev.Mods, ev.Key)
	case ui.InputText:
		app.insert(ev.Text)
		return true
	}
	return false
}).TextCaret(app.caretRect())

c.OpenURLThen(url, func(err error) {
	if err != nil {
		c.Toast("No app opens " + url)
	}
})

if ui.Button(c, "Copy").Clicked() {
	c.WriteClipboardThen(link, func(err error) {
		app.copied, app.copyFailed = err == nil, err != nil
	})
}
```

### 键盘上的每个键

`InputEvent.Code` 是按它在任何键盘上的位置表示的键，一个 `ui.KeyCode`：它能
区分左右修饰键（`ui.CodeShiftLeft`、`ui.CodeAltRight`、`ui.CodeSuperLeft`），
并命名 `ui.Key` 没有名称的键，如 `ui.CodeNumpad5` 或
`ui.CodeIntlBackslash`——左 Shift 旁的第 102 个键。它的值是 USB HID 键盘页的
用途 ID，名称沿用 `KeyboardEvent.code`，Meta 用 Super 表示，因此一张映射表就能把
它们转换成客户机的扫描码。

`HandleInput` 和 `ObserveInput` 能收到系统告知窗口的每个按键，如同网页的
监听器那样。这包括单独按下和松开的修饰键（包括单独轻按一下 Super）、Caps
Lock、Num Lock、Scroll Lock、Print Screen 和 Pause，以及布局无法给出 `Key`
的按键，如配合 AltGr 按下的键。它们作为 `ui.InputKeyDown` 和
`ui.InputKeyUp` 到来，`Key` 为 `ui.KeyUnknown`，只有 `Code` 能命名它们。
`Mods` 包含按下时带入的修饰键，不包含松开时带走的修饰键。用 `switch` 选择
按键的处理器永远不会注意到它们；对所有按键都作出反应的处理器应检查 `Key`
不是 `ui.KeyUnknown`。

```go
ui.Box(c).Grow(1).Focusable().AutoFocus().Bind(&app.screen).HandleInput(func(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputKeyDown:
		return app.guest.press(ev.Code)
	case ui.InputKeyUp:
		return app.guest.release(ev.Code)
	}
	return false
})
if !app.screen.Focused(c) {
	app.guest.releaseAll() // what is let go of elsewhere never comes up here
}
```

函数接下的输入不会再往下传，比如 Alt，否则它会在 Windows 和 Linux 上打开
菜单栏，所以对不用的键要返回 false；无论哪种方式，快捷键都仍然有效。
`Tester.CodeDown` 和 `Tester.CodeUp` 在测试中按下和松开这类键。

系统报告的按键事件有以下限制：

- 桌面为自己保留的键永远不会到来，比如用来打开启动器的 Super，或窗口管理器
  绑定的组合键。
- 在 macOS 上，Caps Lock 在它锁定状态变化时作为一次按下和一次松开一起到来，
  而不是随按键动作。Fn 没有 `Code`。
- 在 Windows 上，AltGr 作为 `ui.CodeControlLeft` 然后是
  `ui.CodeAltRight` 到来，因为 Windows 会合成一个左 Control，`Mods` 中含有
  Ctrl 和 Alt。Print Screen 只作为松开到来，因为 Windows 保留了它的按下。
  同时按住两个 Shift 时，只有最后松开的那个键会产生松开事件。Windows 键的松开仍会打开
  开始菜单。
- 在浏览器中，页面得到浏览器留给它的内容：`Code` 是事件的 `code`。
- `WebView` 在有焦点时接受这些键。

## 区域内的输入

`ObserveInput` 让容器看到进入其内部元素的输入，如同祖先监听冒泡到它的
`keydown` 事件，但不接管它：按下和松开的按键（带 `Repeat`），焦点在容器上或
容器内时键入和合成的文本以及菜单的编辑命令，以及在它上面按下和松开的指针。
比如在内部按下第一个键就停止播放的演示，或在某个字段被输入后关闭提示的面板，
就会用到它：

```go
panel := ui.Column(c).Gap(8).ObserveInput(func(ev ui.InputEvent) {
	if ev.Kind == ui.InputKeyDown || ev.Kind == ui.InputText {
		app.hint = false
	}
})
panel.Children(func() {
	if app.hint {
		ui.Text(c, "Type a name and an email")
	}
	ui.TextInput(c, &app.name).Label("Name")
	ui.TextInput(c, &app.email).Label("Email")
})
```

函数在有焦点的元素得到输入之前运行，如同 UI Events 的捕获阶段，最外层的
容器最先，之后会构建一帧。输入随后照常继续：转到有焦点元素的 `HandleInput`、
文本输入、快捷键和 Tab，所以函数也能看到 `Shortcut` 接下的键。它看不到系统
或菜单先于窗口接下的键（这时到来的是一条编辑命令），也看不到容器打开的
浮层、菜单和对话框的输入，因为它们不在它内部。像 Tab 这样移动了焦点的键，
其松开事件会发送到移动后的焦点位置。

## 另见

- [上下文菜单](context-menu.md)和[菜单按钮](menu-button.md)。
- [工具提示](tooltip.md)。
- 自定义标题栏：[原生 UI 窗口](windows.md)。
