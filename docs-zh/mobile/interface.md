# 手机上的界面

## 标签页和栈

在手机上，用户通过底部的系统标签栏和一层层的屏幕栈在应用中导航，每层屏幕顶部都有一个显示标题和返回按钮的栏。`mygo.NewTabs` 显示标签栏（iOS 上是 UIKit 的，Android 上是 Material 的底部导航），`Window.Push` 把一个窗口显示在另一个之上，放进它所在标签页的栈中：

```go
mygo.App.WhenReady(func() {
	list := mygo.NewWindow(mygo.WindowOptions{Title: "Fruits", Content: ui.View(app.list), Hidden: true})
	web := mygo.NewWindow(mygo.WindowOptions{Title: "Web", URL: "/", Hidden: true})
	tabs, _ := mygo.NewTabs(
		mygo.TabItem{Title: "Fruits", Symbol: "leaf", Icon: leafPNG, Window: list},
		mygo.TabItem{Title: "Web", Symbol: "globe", Icon: globePNG, Window: web},
	)
	tabs.SetBadge(1, "3")
	app.open = func(fruit string) {
		list.Push(mygo.NewWindow(mygo.WindowOptions{Title: fruit, Content: ui.View(app.detail(fruit)), Hidden: true}))
	}
})
```

标签页在 iOS 上指定一个 SF Symbol，在 Android 上（以及 iOS 上没有符号时）提供一张 PNG，按栏的颜色作为模板绘制。返回按钮、iOS 上从左边缘的轻扫和 Android 的返回会弹出推入的窗口，并关闭它；`Window.Pop` 也一样。在 iOS 上，栏浮在原生 UI 之上，列表在标签栏的玻璃下继续滚动；在 Android 上，栏使用窗口背景的颜色。在桌面上，这些窗口就是普通窗口：`Tabs.Select` 显示其中一个，推入的窗口也显示为一个窗口。`examples/tabs` 演示了两者，其中一个标签页是网页。

`ui.Router` 也可以在一个窗口里把它的页面显示为系统的栈：设置 `SystemStack` 后，从第一页到当前页的历史就是栈，顶部是系统的导航栏，显示每页用 `Route.Title` 给出的标题和返回按钮。在 iOS 上由 UIKit 的导航控制器移动页面，并支持从左边缘轻扫返回；在 Android 上导航栏是 Material 的工具栏，带返回箭头，页面由路由器像 Android 应用那样滑动。`InSystemStack` 为真时不要再画自己的栏：

```go
func (app *notes) view(c *ui.Context) {
	app.router.SystemStack = true // 在桌面和浏览器里没有任何变化
	ui.Column(c).Fill().Children(func() {
		if !app.router.InSystemStack(c) {
			ui.Row(c).Children(func() {
				ui.BackButton(c, app.router)
				ui.Text(c, app.router.Title()).Bold()
			})
		}
		app.router.View(c, func(r *ui.Route) {
			switch {
			case r.Match("/"):
				r.Title("Notes")
				app.list(c)
			case r.Match("/notes/{id}"):
				r.Title("Note")
				app.note(c, r.Param("id"))
			}
		})
	})
}
```

路由器的 View 要占满窗口。在 iOS 上窗口只有一个绘制表面，它跟着当前页移动：下面的屏幕保留各自页面最后显示时的快照，手指返回时露出快照，回到该页后页面重新实时显示。`examples/gallery` 在手机上就是这样显示它的页面的。

在 iPad 上，在 `mygo.json` 中设置 `"ios": {"multipleWindows": true}` 后，窗口可以成为系统自己的窗口，与应用的其他窗口并排：

```go
mygo.NewWindow(mygo.WindowOptions{Title: "Draft", OwnScene: true, Content: ui.View(draft)}).Show()
```

在其他地方，它和普通窗口一样：在手机上显示在前一个窗口之上。Android 平板会自行与其他应用分屏。

## 手指

原生 UI 像系统自己的工具包那样处理手指：

- 轻点就是点击。手指能点到它几乎碰到的东西，范围为 14 DIP：滑块的拇指比指尖还窄。
- 移动的手指会锁定到它移动的方向，以及在该方向上滚动的最内层容器：在一个纵向列表中，一行横向照片接收横向的移动，把纵向的移动留给列表。手指抬起时内容继续惯性滑动，手指落下时惯性滑动停止，不会点到任何东西。
- 超过内容的边界后，内容跟随得越来越少，然后弹回，iOS 上弹得远，Android 上弹得近。内容移动时会显示滚动指示器。
- 沿某个方向拖动的元素接收该方向的移动：列表中一个横向的 `Draggable`，即使里面有按钮，也会把纵向的移动留给列表。
- 手指静止按住半秒就是长按：`Element.LongPressed`、上下文菜单、选中文本中的一个词，或拖动一个值，比如 `Reorder` 移动的列表行。设备会给出一下触感反馈。
- 两根手指可以捏合和旋转（`InputEvent` 的 `InputZoom`，以及 `Rotation`）。
- 在 iOS 上，手指从左边缘可以让 `ui.Router` 的页面返回，页面跟随手指移动，就像 UIKit 的栈那样。在 iOS 上轻点状态栏，可滚动的内容会回到顶部。

`Element.PullToRefresh` 让手指把滚动容器拉过顶部：

```go
list := ui.Scroll(c).Fill()
if list.PullToRefresh(&app.refresh) {
	go func() {
		items := fetch()
		win.Update(func() { app.items, app.refresh.Refreshing = items, false })
	}()
}
```

## 文本

键盘通过输入元素的 `TextInputClient` 编辑文本，带有系统的输入建议和输入法。元素会说明它装的是什么内容，由此选择键盘：

```go
ui.TextInput(c, &app.email).
	Keyboard(ui.KeyboardEmail).ReturnKey(ui.ReturnNext).
	Capitalize(ui.CapitalizeNone).Autocorrect(false).Autofill(ui.AutofillEmail)
```

`ReturnNext` 把焦点移到下一个元素；`Password` 是安全输入。键盘弹出时会让正在使用的输入元素保持可见。按住一个词会选中它，并显示系统的操作菜单（`UIEditMenuInteraction` 和 Android 的浮动工具栏）；选区两端的手柄可以拖动，并带有系统的放大镜，外观与各系统一致。轻点插入点也会显示菜单。

## 手机的控件

包 `ui/mobile` 提供手机界面有而桌面界面没有的控件，外观与各系统一致，基于 `ui` 的公开 API。不导入它的应用不会为它付出代价。

| | |
|---|---|
| `Sheet`、`ActionSheet` | 从底部升起的面板，带停靠位置，可以用手指拖走 |
| `Drawer` | 从侧边滑入的面板 |
| `PageView`、`PageDots` | 用手指轻扫切换的页面，在接近时才构建 |
| `SwipeRow` | 用手指轻扫到一边以显示操作的行 |
| `Wheel`、`DatePicker`、`TimePicker` | 可转动的滚轮，选择变化时有触感反馈 |
| `TopBar`、`BottomBar`、`FloatingButton` | 由应用绘制的栏，用于在一个窗口内部路由 |
| `ListRow`、`Section` | 行及行的分组，带按下时的外观 |
| `Ink` | Material 的水波纹 |
| `Switch` | 系统的开关 |
| `Zoomable` | 可以用手指捏合、旋转和拖动的内容，比如照片 |

`mobile.SetLook` 可以不论系统，选择一种设计。`examples/mobile` 演示了所有控件。

来自网络的图片用 `ui/netimage` 代替 `ui.Image`：

```go
netimage.Image(c, user.AvatarURL, netimage.Options{MaxSide: 240}).Size(80, 80).Radius(40)
```

占位符先占住它们的位置，图片加载完成后淡入。只要有空间，它们就留在内存中，并按服务器的缓存头在多次运行之间保存在磁盘上，离线时也能显示。屏幕上的图片先于惯性滑动划过的图片下载，轮到时已经划走的图片根本不会下载。桌面和浏览器也能使用它。

## 测试

单元测试用 `ui.Tester` 运行视图，推进它的时钟，并使用 `FingerDown`、`FingerMove`、`FingerUp` 和 `SetInsets`。包 `ui/golden` 把绘制结果与测试旁边的图片比较：

```go
func TestCard(t *testing.T) {
	golden.Match(t, ui.NewTester(card, 320, 200), "testdata/card.png")
}
```

用 `MYGO_UPDATE_GOLDEN=1` 运行即可写入这些图片，要在运行测试的平台上写入：不同平台的文本渲染不同。

`MYGO_FRAME_STATS=1` 记录慢于 8 毫秒的帧，以及耗时所在，在手机上也一样：模拟器用 `SIMCTL_CHILD_MYGO_FRAME_STATS=1`，Android 上用 `adb shell am start ... --es mygo.env MYGO_FRAME_STATS=1`。

端到端测试从另一个进程驱动运行中的应用，在模拟器和设备上都一样，使用[自动化插件](../plugins/automation.md#on-phones)：读取屏幕上的元素，对它们操作，发送手指和文本，截取屏幕，并等待画面稳定。

在开发版中用四根手指轻点，可以打开原生 UI 的检查器：它显示在手机底部，包括元素、样式、帧时间和问题，就像在桌面上按 F12 打开的那样。
