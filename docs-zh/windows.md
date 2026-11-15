# 窗口

MyGo 使用系统原生窗口，窗口内既可以显示从 `URL` 加载的网页，也可以显示通过 `Content` 提供、由 MyGo 绘制的[原生 UI](ui/README.md)。应用就绪后，用 `mygo.NewWindow` 创建窗口：

```go
mygo.App.WhenReady(func() {
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:     "Editor",
		URL:       "/",
		Width:     1200,
		Height:    800,
		MinWidth:  600,
		MinHeight: 400,
		StateKey:  "editor",
	})
	win.OnClosed(func() { log.Println("closed") })
})
```

原生 UI 的窗口用 `Content` 代替 `URL`：

```go
mygo.NewWindow(mygo.WindowOptions{Title: "Inspector", Content: ui.View(app.view)})
```

本指南的一切都适用于这两种窗口，但[页面](#页面)及其相关选项除外——只有显示网页的窗口才有页面。与每个 MyGo API 一样，窗口可以从任何 goroutine 创建和修改；调用会转到驱动 UI 的主线程。

## 选项

零值 `WindowOptions` 是一个可见、可调整大小的 800×600 窗口，居中显示在屏幕上。尺寸和位置以 DIP 计。

| 选项 | |
|---|---|
| `Title` | 窗口标题；默认为应用名，页面的 `<title>` 会替换它 |
| `URL` | 窗口创建后加载；`/` 和其他不带协议前缀的 URL 指向[前端](frontend.md#页面如何加载)的页面 |
| `Content` | 显示 MyGo 绘制的[原生 UI](ui/README.md)，而不是页面 |
| `Width`, `Height` | 窗口的尺寸；使用 `UseContentSize` 时则是其所显示内容的尺寸 |
| `X`, `Y` | 左上角；两者都为 0 时窗口居中 |
| `MinWidth`, `MinHeight`, `MaxWidth`, `MaxHeight` | 调整大小的限制；0 表示没有限制 |
| `FitContent` | 按页面内容的大小设置页面区域，并在上面的限制内跟随它，如同[浮出窗口](#让浮出窗口适应其内容) |
| `StateKey` | 在此键下记住窗口的边界和状态，见[下文](#记住窗口的位置) |
| `Hidden` | 创建窗口但不显示它 |
| `Maximized`, `FullScreen` | 以最大化或全屏创建它 |
| `Frameless` | 没有标题栏和边框：由页面绘制它们，见[自定义标题栏](frontend.md#自定义标题栏)，或[原生 UI](ui/windows.md) |
| `TitleBarStyle` | 隐藏标题栏，但让窗口按钮浮在页面上方，见[自定义标题栏](frontend.md#自定义标题栏)，或浮在[原生 UI](ui/windows.md) 上方 |
| `TrafficLightPosition` | 移动隐藏式标题栏的窗口按钮（macOS） |
| `TitleBarHeight` | 页面在窗口按钮下方绘制的标题栏高度（Linux、Windows） |
| `HideWindowButtons` | 让隐藏式标题栏的窗口按钮不显示，并保留其可调整大小的上边缘、阴影和圆角：页面绘制自己的按钮，并调用 `Minimize`、`Maximize`、`Unmaximize` 和 `Close` |
| `Transparent`, `Vibrancy` | 透明窗口，以及透明页面或[原生 UI](ui/windows.md) 背后的材质（macOS、Windows 11 22H2）：`VibrancyGlass` 和 `VibrancyGlassClear` 是 macOS 26 的 Liquid Glass。在 Windows 上，原生 UI 的透明窗口在未绘制任何内容的地方会透出背后的内容，而带材质的窗口没有菜单栏：Alt 和 F10 会以弹出菜单打开它的菜单 |
| `VibrancyTint` | 为 Liquid Glass 着色，使用 CSS 语法，如 `"rgba(0, 122, 255, 0.3)"` 或 `"light-dark(...)"`；其他材质和平台会忽略它（macOS 26）。`SetVibrancyTint` 之后可以更改它 |
| `CornerRadius` | 给无边框窗口加圆角：在 macOS 上，其内容和材质会被裁切到这些圆角；Windows 11 则会给它加上自己的圆角 |
| `BackgroundColor` | 在页面绘制之前填充窗口，使用 CSS 语法，如 `"#1e1e1e"`，或 `"light-dark(#f5f5f7, #1e1e1e)"` 以跟随外观 |
| `Opacity` | 0 到 1 之间 |
| `IgnoreMouseEvents` | 创建窗口时就让鼠标穿透到它背后的内容，如同之后调用 `SetIgnoreMouseEvents(true)`（见[点击穿透窗口](#点击穿透窗口)） |
| `DisableResize`, `DisableMove`, `DisableMinimize`, `DisableMaximize`, `DisableClose`, `DisableFullScreen`, `DisableShadow` | 去掉相应能力。在 Wayland 下的 Linux 上，`DisableShadow` 让无边框窗口或隐藏式标题栏去掉 GTK 在它们周围绘制的阴影和边框 |
| `AlwaysOnTop` | 让窗口保持在其他窗口之上；在 Wayland 下仅限不在任务栏中的无边框窗口，见[点击穿透窗口](#点击穿透窗口) |
| `SkipTaskbar` | 让窗口不出现在任务栏中（Linux、Windows） |
| `AutoHideMenuBar` | 只在键盘焦点进入菜单栏时（按 Alt 或 F10）显示菜单栏（Linux、Windows），见[菜单栏](menus.md#菜单栏) |
| `Parent`, `Modal` | 子窗口，对其父窗口模态，也可以是[叠在浮出窗口上的对话框](#叠在浮出窗口上的对话框)；对于紧挨其某一部分放置的窗口，见[浮出窗口](#浮出窗口) |
| `Panel` | 启动器或快速输入的窗口，浮在所有应用之上，见[面板](#面板) |
| `Page` | 窗口的[页面](#页面)选项：其预加载脚本、受信任的来源、Web 检查器、缩放、用户代理及其后缀、滑动和捏合 |

## 无闪烁地显示窗口

显示网页的窗口在页面绘制之前会显示其 `BackgroundColor`（默认为白色），所以把它设成页面的背景色。如果页面跟随深色模式，就用 `light-dark(<light>, <dark>)` 同时给出两种颜色：窗口会随外观切换。要只在页面就绪后才显示窗口，可以创建隐藏的窗口，并在 `OnReadyToShow` 中显示它们：

```go
win := mygo.NewWindow(mygo.WindowOptions{URL: "/", Hidden: true, BackgroundColor: "#1e1e1e"})
win.OnReadyToShow(win.Show)
```

原生 UI 的窗口两者都不需要：它们没有要加载的页面，绘制出第一帧就会显示（在 Windows 上，窗口会一直不可见，直到绘制出这一帧）。以 `Hidden` 创建的窗口绘制出第一帧后就可以显示，而这一帧是在隐藏状态下绘制的，所以 `OnReadyToShow(win.Show)` 会让它像页面的窗口一样显示。

原生 UI 的视图在 `NewWindow` 返回后才首次运行，因此它可以使用 `NewWindow` 返回的窗口：

```go
app.win = mygo.NewWindow(mygo.WindowOptions{Content: ui.View(func(c *ui.Context) {
	ui.Text(c, app.win.Title()) // never nil
})})
```

在主线程上调用时（如在 `App.WhenReady` 或事件处理器中），`NewWindow` 会延迟到主循环下一轮、窗口绘制第一帧时显示窗口；若之前调用了 `IsVisible` 或 `Maximize` 等窗口方法，则会先显示窗口再执行该方法。紧接 `NewWindow` 注册的 `OnShow` 能收到这次显示事件，调用 `Hide` 则可阻止显示。从其他 goroutine 调用时，`NewWindow` 显示窗口后返回，主线程同时继续绘制；此时需同步访问保存窗口的变量，或通过 `Window.Update` 为其赋值。

## 记住窗口的位置

带有 `StateKey` 时，窗口会在用户数据目录的 `window-state.json` 中记住自己的位置、尺寸以及最大化和全屏状态，该文件在窗口关闭和应用退出时写入。下一个用相同键创建的窗口（例如下次启动时）会取回这些状态，只要保存的位置能落在已连接的显示器上。它们会覆盖 `X`、`Y`、`Width`、`Height`、`UseContentSize`、`Maximized` 和 `FullScreen`，这些仍是首次启动的默认值。每种窗口用各自的键：`"main"`、`"preferences"`。

在 Windows 上，状态还会以屏幕像素保存窗口的边界（`PixelX`、`PixelY`、`PixelWidth`、`PixelHeight`）：各显示器的缩放不同时，窗口在 DIP 中的位置会随缩放变化，而早期版本的状态无法区分这些显示器。

应用若把数据保存在别处（例如自己的数据库中），可以通过 `WindowStateStore` 将窗口状态一并保存：在创建第一个带 `StateKey` 的窗口之前设置它，就不会写入 `window-state.json`。存储实现需要像 JSON 存储一样保留 `WindowState` 的全部字段：在 Windows 上，如果状态中缺少像素坐标和尺寸，而各显示器的缩放又不同，窗口就会居中显示。

```go
type windowStates struct{ db *sql.DB }

func (s windowStates) LoadWindowStates() (map[string]mygo.WindowState, error) {
	var data []byte
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = 'windows'`).Scan(&data)
	if err != nil {
		return nil, err
	}
	var states map[string]mygo.WindowState
	return states, json.Unmarshal(data, &states)
}

func (s windowStates) SaveWindowStates(states map[string]mygo.WindowState) error {
	data, err := json.Marshal(states)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES ('windows', ?)`, data)
	return err
}

mygo.App.SetWindowStateStore(windowStates{db})
```

MyGo 在创建第一个带 `StateKey` 的窗口时加载一次状态，并在这样的窗口关闭和应用退出时，在主线程上保存全部状态。没有状态或加载失败时，窗口按其选项打开；保存失败会在下一次重试。

退出时，状态会在 `OnBeforeQuit` 监听器运行之后、每个窗口关闭时保存，并在事件循环停止之后、`OnQuit` 监听器运行之前再保存一次。如果“重置设置”操作删除了 `window-state.json`，或删除了自己存储中的状态，就会看到它们又被写回来。`App.ClearWindowStates()` 则会永久遗忘它们：它不保存任何状态，对 `WindowStateStore` 则写入一个空 map，此后窗口不再保存自己的状态，直到应用退出。已打开的窗口保留其边界，而之后创建的窗口（如同在下次启动时）会使用其选项。它可以在任何时候调用，也可以从退出监听器中调用：

```go
mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
	if resetRequested {
		_ = mygo.App.ClearWindowStates()
	}
})
```

### 恢复页面

窗口的位置随其 `StateKey` 恢复；其页面随会话状态恢复：`Page.SessionState` 保存前进/后退列表，以及引擎保存的滚动位置和表单字段，`PageOptions.SessionState` 则在新的窗口或 web view 中恢复它，代替其 `URL`，包括后退。应用把该状态保存在自己存放数据的地方，以便在下次启动时恢复页面，或关掉一个并未显示的页面、之后再恢复它，如同浏览器的休眠标签页：

```go
win.OnClose(func(e *mygo.CloseEvent) {
	if state, err := win.Page().SessionState(); err == nil {
		os.WriteFile(sessionFile, state, 0o600)
	}
})

// At the next launch:
state, _ := os.ReadFile(sessionFile)
win := mygo.NewWindow(mygo.WindowOptions{StateKey: "main", URL: "/", Page: mygo.PageOptions{SessionState: state}})
```

该状态是不透明的字节，其格式由 MyGo 管理版本：页面的 URL 和引擎自身的状态。平台无法读取的状态（在另一个平台上保存，或由以其他方式编码的 Web 引擎保存）只会恢复它当时所在的页面；而根本不是状态的数据（例如文件缺失）会加载 `URL`。它可能包含用户输入表单的内容：要像对待用户数据一样保管它。`LoadHTML` 加载的 HTML 不在其中。

WebKit 会随时保存滚动位置和表单字段：它显示的页面的滚动位置会在用户停止滚动片刻后保存（WebKitGTK 2.38 以及 Safari 16 的 WebKit 或更高版本），其表单字段只在它离开视图时保存（例如其窗口被隐藏），其他条目的滚动位置则在页面离开它们时保存。此后保存的状态会包含这些内容，所以页面在显示期间保存时会回到其滚动位置，但表单字段仍是它上次离开视图时的。

| 平台 | 会话状态 |
|---|---|
| macOS | `WKWebView` 的 `interactionState`，macOS 12 或更高版本；之前只有页面的 URL |
| Linux | `webkit_web_view_get_session_state`，序列化，以及 `webkit_web_view_restore_session_state` |
| Windows | 只有页面的 URL：WebView2 没有用于前进/后退列表的 API |

## 尺寸、位置和状态

```go
win.SetSize(1024, 768)
win.SetContentSize(1024, 700) // the page area
win.SetPosition(100, 100)
win.SetBounds(mygo.Rectangle{X: 100, Y: 100, Width: 1024, Height: 768})
win.Center()

win.Maximize()          // Unmaximize, ToggleMaximize, IsMaximized
win.Minimize()          // Restore, IsMinimized
win.SetFullScreen(true) // ToggleFullScreen, IsFullScreen
win.Hide()              // Show, ShowInactive (without focusing), IsVisible
win.Focus()             // Blur, IsFocused
```

`IsVisible` 告诉你窗口是否被 `Show` 显示过，且此后没有被 `Hide` 隐藏：在任何平台上，最小化的窗口仍然算可见，被其他窗口覆盖或位于另一个桌面的窗口也一样。`IsMinimized` 表示窗口已最小化，而在原生 UI 的窗口中，[`Context.WindowHidden`](ui/windows.md#隐藏的窗口) 则表示它是否有任何部分显示在屏幕上。

尺寸和位置以窗口的 DIP 计，无论其页面（`Page.SetZoomFactor`）或其[原生 UI](ui/windows.md#缩放)的缩放如何：在 1.25 的缩放下，1000 DIP 的页面区域显示页面的 800 个 CSS 像素，或视图的 800 个 DIP。`Bounds`、`ContentBounds`、`Size`、`ContentSize` 和 `Position` 会读回这些值；[`mygo.Screen`](native.md#显示器) 告诉你显示器的位置。`Bounds` 是整个窗口的边界，包括标题栏和边框（在 Linux X11 上是窗口管理器的框架），`ContentBounds` 则是页面或原生 UI 显示的区域（在菜单栏下方）的边界。在 Wayland 下的 Linux 上，合成器决定窗口去向，并忽略应用设置的位置，其装饰也不属于 `Bounds`。

其他属性：

- `SetTitle`、`SetResizable`、`SetMovable`、`SetMinimizable`、`SetMaximizable`、`SetClosable`、`SetMinimumSize`、`SetMaximumSize`、`SetAlwaysOnTop`、`SetOpacity`、`SetHasShadow`、`SetBackgroundColor` 和 `SetVibrancy` 更改选项所设置的内容。在 Windows 上，`SetVibrancy` 更改用材质创建的窗口的材质，在其他窗口中则不显示任何材质。在 Linux 上，窗口保留创建时的阴影（`DisableShadow`）。
- `SetIcon(png)` 给窗口自己的图标，显示在标题栏和任务栏按钮中（Linux、Windows）。
- `SetSkipTaskbar` 让窗口不出现在任务栏中（Linux、Windows）。
- `SetAutoHideMenuBar` 隐藏菜单栏，直到按 Alt 或 F10，或永久重新显示它（Linux、Windows）。
- `SetVisibleOnAllWorkspaces` 在每个 Space 或虚拟桌面上显示它（macOS、Linux）；在 macOS 上还会像浮层一样显示在全屏的其他应用之上，即使设置了 `DisableFullScreen`，只要应用是辅助应用（`ActivationPolicyAccessory`、`App.Dock.Hide`）：macOS 在那里不显示常规应用的窗口，只显示[面板](#面板)。
- `SetContentProtection(true)` 让它不出现在截图和屏幕录制中。
- `SetIgnoreMouseEvents(true)` 让点击穿透到它背后的内容，`IgnoresMouseEvents` 读回这一状态，`SetInputRegion` 则除其某些部分上的点击外全部放行（见[点击穿透窗口](#点击穿透窗口)）。
- `SetActiveAppearance(true)` 在另一个应用位于前面时仍把窗口绘制为活动状态（macOS），使浮在它上面的面板所用的清透 Liquid Glass（`VibrancyGlassClear`）保持清透。它使用一个私有的 AppKit 调用，之后的 macOS 可能会移除它：面向 Mac App Store 的应用应让它保持关闭。此时其材质、页面和原生 UI 也都保持活动，在每个平台上都是如此（见[活动与非活动窗口](#活动与非活动窗口)）。
- `SetWindowButtonsTheme(mygo.ThemeDark)` 在浅色应用中，把隐藏式标题栏的窗口按钮按深色背景绘制，就像它们位于页面顶部的深色视图之上；浅色背景则用 `ThemeLight`；`ThemeSystem` 再次跟随应用。页面和菜单保持应用的外观（Windows；macOS 和 Linux 的按钮保持应用的外观）。
- `SetProgressBar` 在任务栏按钮（Windows）、Dock 图标（macOS）或启动器条目（支持它的 Linux 坞）上显示任务进度：

  ```go
  win.SetProgressBar(mygo.ProgressBar{Value: 0.4})
  win.SetProgressBar(mygo.ProgressBar{State: mygo.ProgressIndeterminate})
  win.SetProgressBar(mygo.ProgressBar{}) // done
  ```

- `FlashFrame(true)` 请求注意：任务栏按钮闪烁（Windows）、窗口被标记为紧急（Linux）或 Dock 图标弹跳（macOS）。

## 活动与非活动窗口

接收用户输入的窗口会以活动状态显示，也就是窗口自身或它的某个浮出窗口具有键盘焦点时。原生应用通过不同的外观区分活动与非活动窗口，让用户一眼看出输入焦点在哪里，MyGo 应用也是如此：

- 在 macOS 上，窗口处于非活动状态时，其材质（`Vibrancy`）会淡化为不透明的灰色，如同 AppKit 的侧边栏；Windows 11 则将其 Mica 或 Acrylic 绘制为纯色填充。面板和浮出窗口则让自己的材质保持活动，浮在任何位于前面的应用之上；用 `SetActiveAppearance(true)` 保持活动的窗口也是如此。
- 窗口非活动时，页面会在 `<html>` 上获得 `data-mygo-window-inactive` 属性（供 CSS 使用），`mygo-runtime` 的 `windowActive()` 也会反映这一点（见[非活动窗口](frontend.md#非活动窗口)）。
- 原生 UI 读取 `c.WindowActive()`，其内置控件把强调色淡化为灰色（见[原生 UI](ui/windows.md#活动与非活动窗口)）。

窗口所属的可聚焦浮出窗口获得键盘焦点时，父窗口仍保持活动状态，在 Windows 上其标题栏和 Mica 材质也保持活动外观。尚未获得过键盘焦点的窗口默认视为活动，以便即将获得焦点的窗口从第一帧起就呈现活动外观；通过 `ShowInactive` 显示的窗口除外。面板和没有父窗口的浮出窗口始终保持活动状态。

## 子窗口和模态窗口

设置 `Parent` 后，子窗口保持在父窗口之上，并随父窗口关闭。在所有平台上，子窗口都可以移到任意显示器，父窗口移动时子窗口不会跟随。在 macOS 上，父窗口每次移到前台，子窗口都会排在其上方；父窗口设为 `AlwaysOnTop` 时也一样，子窗口还会随父窗口进入全屏，在父窗口最小化或隐藏时隐藏。设置 `Modal` 后，子窗口还会像对话框一样阻止用户操作父窗口，直到子窗口关闭：

```go
prefs := mygo.NewWindow(mygo.WindowOptions{
	URL:    "/preferences",
	Parent: win,
	Modal:  true,
	Width:  500,
	Height: 400,
})
```

在 macOS 上，模态窗口显示为附着式对话框（sheet）。父窗口也可以是浮出窗口，见[叠在浮出窗口上的对话框](#叠在浮出窗口上的对话框)。

## 浮出窗口

浮出窗口（flyout）是一种没有边框、背景或任务栏按钮的窗口，由父窗口拥有，紧挨父窗口的一个矩形放置，该矩形就是它的锚点：组合框的列表、悬停卡片、带表单的浮层。与原生 UI 的[浮层](ui/overlays.md)或页面的浮层不同，它可以延伸到父窗口的边缘之外，去哪里由显示器的工作区决定，而不是父窗口的内容。它的页面或[原生 UI](ui/README.md) 绘制其全部内容，包括背景：

```go
fl := mygo.NewFlyout(mygo.FlyoutOptions{
	Parent:    win,
	Anchor:    mygo.Rectangle{X: 20, Y: 40, Width: 120, Height: 28}, // in win's content area
	Placement: mygo.PlacementBottomStart,
	Gap:       4,
	Width:     240,
	Height:    300,
	Content:   ui.View(app.suggestions),
})
```

锚点以相对于父窗口内容区域的 DIP 计，这正是 `ui.Element.Bounds` 为打开浮出窗口的元素给出的值；零尺寸的矩形会把它锚定到一个点。`Placement` 指定浮出窗口去往锚点的哪一侧，以及沿该侧如何对齐，如 `"bottom-start"`、`"right"` 或 `"top-end"`。工作区在那里没有空间时，浮出窗口翻到另一侧或另一端（如果那边有空间），然后滑回工作区，再缩小以适应它，如同 GTK 放置菜单、Wayland 合成器放置弹出窗口那样。它保持在父窗口之上，随父窗口移动并随它关闭。`SetAnchor` 让它紧挨另一个矩形放置，`SetSize` 在它所在的位置调整它的大小；`IsFlyout` 区分浮出窗口与其他窗口。

设置 `Focusable` 后，浮出窗口一显示就获得键盘焦点，并像菜单一样，在用户点击外部、其他窗口获得焦点或应用失去活动状态时关闭。`OnClose` 监听器可以阻止关闭，但 Wayland 上的合成器会直接关闭它。在 Linux 和 Windows 上，外部点击只会关闭浮出窗口，与系统菜单一致；在 macOS 上，点击还会传递给指针下方的内容，但不会传递给锚点，与 AppKit 浮层一致。因此，点击用于打开浮出窗口的按钮只会关闭它，不会再次打开。菜单、选项列表和浮层通常应允许获得焦点，并自行处理方向键和 Enter 等按键。

未设置 `Focusable` 时，键盘焦点始终留在父窗口，浮出窗口会持续显示，直到被主动关闭，适合工具提示、悬停卡片或父窗口中文本框的输入建议。这类浮出窗口由父窗口的视图控制，应用自行决定何时关闭，例如文本框失去焦点时。（在 Wayland 上，只有获得键盘焦点的弹出窗口才能收到外部点击。）

两种浮出窗口都会在按下未被内容处理的 Escape 时关闭，效果与用户主动关闭相同：原生 UI 的元素或快捷键未处理该按键，页面处理器也未通过 `preventDefault()` 取消它，与 Web 浮层的行为一致。

`Shadow` 在 macOS 上给它系统的窗口阴影：阴影跟随浮出窗口所绘制内容的形状，并用细线勾勒它（所以那里不要自己绘制边框）；在 Windows 上同样如此，Windows 11 会像给菜单加圆角那样给浮出窗口加圆角并勾勒它（所以那里既不要绘制边框，也不要绘制圆角）。

在 macOS 上，`Popover` 会改用系统浮层外观：浮出窗口显示在 `NSPopover` 中，由系统绘制材质、圆角、指向锚点中央的箭头及动画。材质和箭头随应用主题变化，打开期间也会更新；不受托盘菜单栏图标的对比度外观影响。AppKit 会选择 `Placement` 指定的一侧或相反一侧，其他对齐设置和 `Gap` 不生效。与 AppKit 浮层一样，它显示期间会获得键盘焦点；设置 `Focusable` 后，点击其他位置会关闭它。内容不应在材质上覆盖背景：原生 UI 的 `c.Vibrancy()` 会报告该材质，视图只需在未使用该材质的场合自行绘制面板背景：

```go
func (app *App) about(c *ui.Context) {
	c.Root().Background(ui.Transparent)
	box := ui.Column(c).Fill() // over the popover's material
	if !c.Vibrancy() {
		box = ui.Column(c).Fill().Background(c.Theme().Surface).Radius(10)
	}
	box.Padding(12).Children(func() { ui.Text(c, "MyGo") })
}
```

浮出窗口可以挂在托盘图标而非窗口上，如同菜单栏应用的面板：给它 `Tray` 而不是 `Parent`。它会紧挨图标，位于 `Placement` 指定的一侧，并浮在其他应用的窗口之上。在 macOS 菜单栏下方，`PlacementBottom` 使它居中于图标下方（使用 `Popover` 时，系统浮层的箭头指向它）；在 Windows 上，同样的放置会翻转到屏幕底部任务栏图标的上方。macOS 会在每个显示器上显示菜单栏项，此时它会去往用户点击的那个图标副本下方。按下图标不会关闭可聚焦的浮出窗口，所以图标的点击可以切换它；使用 `Popover` 时，点击会关闭它（如同 AppKit 的浮层在锚点上关闭），且不会到达 `OnClick`。无论哪种方式，点击图标都会用下面的代码关闭已打开的浮出窗口，而不会再次打开它。在浮出窗口（它唯一的窗口）关闭时让应用继续运行：

```go
mygo.App.OnWindowAllClosed(func() {})
tray.OnClick(func() {
	if panel != nil {
		panel.Close()
		return
	}
	panel = mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:      tray,
		Placement: mygo.PlacementBottom,
		Width:     280,
		Height:    230,
		Focusable: true,
		Popover:   true,
		Content:   ui.View(app.panel),
	})
	panel.OnClosed(func() { panel = nil })
})
```

Linux 的托盘图标既不报告点击，也不报告自己的位置：在那里从图标的菜单项打开浮出窗口，它会显示在主显示器工作区的中央。`examples/menubar-flyout` 就是这样一个应用。

### 叠在浮出窗口上的对话框

原生 UI 的[对话框](ui/dialog.md)绘制在所属窗口内部，因此在浮出窗口中会受到其边界裁切，托盘浮出窗口也可能没有其他可用的窗口。此时应另建一个以浮出窗口为父窗口的模态窗口：将浮出窗口设为 `Parent`，并启用 `Modal`。未指定 `X` 和 `Y` 时，模态窗口在浮出窗口上方居中，并调整位置以落在显示器工作区内；它始终位于浮出窗口之上，父窗口属于托盘时，还会位于其他应用的窗口之上。模态窗口显示期间，浮出窗口不接受输入，也不会因焦点变化而关闭；模态窗口关闭后，可聚焦的浮出窗口重新获得键盘焦点，并恢复失焦关闭的行为。关闭浮出窗口（例如点击托盘图标）也会关闭该模态窗口。

```go
if ui.Button(c, "Quit…").Clicked() {
	mygo.NewWindow(mygo.WindowOptions{
		Title:          "Quit?",
		Parent:         panel,
		Modal:          true,
		Width:          360,
		Height:         140,
		UseContentSize: true,
		DisableResize:  true,
		Content:        ui.View(app.confirmQuit),
	})
}
```

在 macOS 上，该窗口不是附着式对话框（sheet），而是与它关联、处于同一层级的独立窗口。在 Wayland 上，带 `Parent` 的浮出窗口属于弹出窗口，模态窗口获得键盘焦点后是否保留弹出窗口由合成器决定；托盘浮出窗口属于普通窗口，因此会保留。

### 让浮出窗口适应其内容

使用 `FitContent` 时，浮出窗口采用其页面内容的大小，并随内容变化而跟随，如同浏览器为扩展的弹出窗口设置大小：带列表的菜单栏浮层、悬停卡片、启动器的结果。`Width` 和 `Height` 只在页面报告自己的大小之前有效；页面会在 DOM 就绪之前报告。所以以 `Hidden` 创建并从 `OnReadyToShow` 显示的浮出窗口，会按自己的大小显示。`MinWidth`、`MinHeight`、`MaxWidth` 和 `MaxHeight` 约束它，工作区也约束它，如同任何浮出窗口：

```go
fl := mygo.NewFlyout(mygo.FlyoutOptions{
	Tray:       tray,
	Placement:  mygo.PlacementBottom,
	FitContent: true,
	MaxWidth:   400,
	MaxHeight:  600,
	URL:        "/panel",
})
```

页面中 MyGo 的运行时把根元素（`html`）按内容的宽度布局（`width: max-content`），在限制之内，使其文本在 `MaxWidth` 处换行，并用 `ResizeObserver` 观察其大小：高度取内容的高度，包括 body 的外边距；超出 `MaxHeight` 时页面滚动，这时滚动条占用的空间也计入大小。给 `html` 设定自己大小的页面（如 `height: 100%`）会让浮出窗口保持该大小：让根元素和 body 采用其内容的大小。大小以 DIP 计，是页面的 CSS 像素乘以其缩放；不带来变化的大小不会触发调整。

原生 UI 的浮出窗口（`Content`）以同样的方式适应，例如托盘面板中随应用运行而变化的节点列表、表单或错误行，无需猜测大小，也无需调用 `SetSize`：

```go
fl := mygo.NewFlyout(mygo.FlyoutOptions{
	Tray:       tray,
	Placement:  mygo.PlacementBottom,
	FitContent: true,
	MinWidth:   260,
	MaxHeight:  480,
	Content: ui.View(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(12).Gap(8).Children(func() {
			ui.Text(c, "Nodes")
			ui.Scroll(c).Grow(1).Children(func() { app.nodes(c) })
			if app.err != nil {
				ui.Text(c, app.err.Error())
			}
		})
	}),
})
```

每一帧在布局时都会测量根内容的大小，就像 CSS 的 `width: fit-content; height: auto`：宽度取其子元素最宽的一行（max-content），受 `MinWidth`、`MaxWidth` 和工作区限制；高度取该宽度下的高度，受 `MinHeight` 和 `MaxHeight` 限制，超出时其中的 `Scroll` 会滚动。`Fill` 和 `Grow` 在布局时取浮出窗口的大小，并按它们的内容测量。放置为 `Absolute` 的元素（如菜单和提示条）不占其中任何空间。测量出另一个大小的帧会调整浮出窗口的大小，重新对着其锚点放置（在工作区没有空间的地方翻转），下一帧再按该大小布局内容：变化后的第一帧会以浮出窗口原有的大小显示它。隐藏的浮出窗口不绘制帧：隐藏期间更改的内容（用 `Update` 或 `Invalidate`）会在 `Show` 或 `ShowInactive` 显示它时布局，浮出窗口会按该内容所取的大小显示。测量结果与上一帧相同的帧不会调整任何大小，因此 `SetSize` 给出的大小会保持不变，直到内容的大小改变。在窗口大小（`Context.Size`）确定后才构建内容的视图，会稳定在它所取的大小，而不是来回调整大小。

`WindowOptions.FitContent` 对窗口的页面区域做同样的事，例如对[面板](#面板)，在它的 `MinWidth`、`MinHeight`、`MaxWidth` 和 `MaxHeight` 以及其显示器的工作区之内。它只在内容大小改变时跟随内容，且只改变发生变化的那个维度，所以用户给可调整大小的窗口设定的大小会一直保持到那时；窗口最大化或全屏时则不会跟随。

各平台如何显示它们：

| | macOS | Linux | Windows |
|---|---|---|---|
| 窗口 | 一个无边框、不激活的 `NSPanel`：显示期间是其父窗口的子窗口，对托盘而言则在菜单层级 | 一个 `GTK_WINDOW_POPUP`：X11 上是 override-redirect，Wayland 上是其父窗口的 `xdg_popup`；对托盘而言，是一个置顶的无装饰工具窗口 | 一个有所属窗口的 `WS_POPUP`、`WS_EX_TOOLWINDOW`，没有重定向位图；对托盘而言则是置顶且无主的 |
| 放置 | 在屏幕的可用区域内 | X11 上在显示器的工作区；Wayland 上是合成器的定位器（`gdk_window_move_to_rect`） | 在显示器的工作区（`MonitorFromRect`） |
| 键盘 | 可聚焦时是键窗口；父窗口保持为主窗口 | 可聚焦时抓取 seat 和应用输入，如同 GTK 的菜单 | 可聚焦时激活，否则 `WS_EX_NOACTIVATE` |

在 Wayland 上，新的弹出窗口只能叠在当前最上层弹出窗口之上：显示浮出窗口会关闭其他已显示的浮出窗口，但保留其祖先窗口，效果与用户主动关闭相同。`examples/flyout` 演示颜色列表、笔记编辑器和“关于”窗口，后者在 macOS 上显示为浮层。

## 面板

面板适合启动器、命令面板或快速输入窗口，例如 Spotlight 和 Raycast：通过[全局快捷键](native.md#全局快捷键)随时唤出，显示在用户当前的 Space 或虚拟桌面上，浮在所有应用窗口之上，包括全屏应用，且不出现在任务栏、Dock 的窗口列表和应用切换器中。面板不能最小化、最大化或进入全屏，在 Linux 和 Windows 上没有菜单栏。在 macOS 上，面板显示时获得键盘焦点，但不激活所属应用：用户之前使用的应用仍保持活动，其菜单栏也不变，面板隐藏后焦点会回到该应用。点击面板内部也不会改变该应用的活动状态。

```go
mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory) // no Dock icon (macOS)
mygo.App.OnWindowAllClosed(func() {})                     // keep running with no window

mygo.App.WhenReady(func() {
	panel := mygo.NewWindow(mygo.WindowOptions{
		Panel:       true,
		Hidden:      true,
		Frameless:   true,
		Transparent: true,
		Vibrancy:    mygo.VibrancyHUD,
		Width:       680,
		Height:      420,
		URL:         "/launcher",
	})
	// Hide rather than close, so that it shows at once the next time:
	// on Escape, Cmd-W or Alt-F4, and as the user goes elsewhere.
	panel.OnClose(func(e *mygo.CloseEvent) {
		e.PreventDefault()
		panel.Hide()
	})
	panel.OnBlur(panel.Hide)

	mygo.GlobalShortcut.Register("Alt+Shift+Space", func() {
		if panel.IsVisible() {
			panel.Hide()
			return
		}
		// On the display with the pointer, a third of the way down.
		d := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
		width, height := panel.Size()
		panel.SetPosition(d.WorkArea.X+(d.WorkArea.Width-width)/2, d.WorkArea.Y+(d.WorkArea.Height-height)/3)
		panel.Show()
	})
})
```

结果会变多变少的面板（如启动器的）可以用 `FitContent` 采用其页面的大小，如同[浮出窗口](#让浮出窗口适应其内容)：给它 `MaxHeight`，而不是固定的 `Height`。

页面未处理（`preventDefault()`）或原生 UI 未接管的 Escape 会关闭面板，如同用户关闭它，`OnClose` 监听器可以把它变成隐藏面板，如上所述。启动器通常从一开始就是没有 Dock 图标的辅助应用：在 [`mygo.config.ts`](configuration.md) 的 `macos.infoPlist` 中给它 `LSUIElement`，或在 `App.Run` 之前设置其激活策略，如上所述。常规应用在启动时会被激活，如同用户打开的任何应用：它显示的第一个窗口（面板也一样）会把它带到前面。

各平台如何显示它们：

| | macOS | Linux | Windows |
|---|---|---|---|
| 窗口 | 一个带不激活样式的 `NSPanel`，从不是主窗口 | 一个工具窗口（`_NET_WM_WINDOW_TYPE_UTILITY`），保持在上面，不出现在任务栏和分页器中；Wayland 下是覆盖层的绘制表面（gtk-layer-shell） | 一个置顶的工具窗口（`WS_EX_TOOLWINDOW`），由一个隐藏窗口拥有，因此从不会有任务栏按钮，带 Windows 11 的圆角 |
| 工作区 | 每个 Space，作为全屏辅助窗口（`CanJoinAllSpaces`、`FullScreenAuxiliary`），不在 Mission Control 和窗口循环中 | 每个工作区（粘性） | 它显示所在的虚拟桌面 |
| 键盘 | 键窗口，应用保持非活动 | 活动窗口；在层 shell 的绘制表面上，显示期间独占 | 前台窗口，全局快捷键的按键让它取得 |

在 Wayland 下，应用不能自行设置普通窗口的位置或将其移到最前。若合成器支持 layer shell 协议（Sway、Hyprland、KDE Plasma、COSMIC……），且已安装 `libgtk-layer-shell.so.0`，面板会像这些桌面的启动器一样，使用该协议的绘制表面：显示在覆盖层，位于全屏窗口之上，不出现在任务栏中，默认在用户当前显示器上居中，也可通过 `X`、`Y` 或 `SetPosition` 定位。它与这些桌面的其他启动器一样，显示期间始终保持键盘焦点，从一开始就能接收按键；只有隐藏时才失焦，因此应处理 Escape 来关闭它，这也是用户预期的操作。在不支持该协议的桌面（例如 GNOME）上，面板与普通窗口一样，位置和层级由合成器决定。

## 点击穿透窗口

桌面宠物是一个无边框、透明、始终置顶的窗口，显示一个角色，偶尔显示一个对话气泡：点击它们会到达应用，点击其他地方则会落到它下面的窗口或桌面，就像宠物的窗口不存在一样。`SetInputRegion` 给窗口设置接收鼠标的矩形，按从其内容左上角算起的 DIP 计；其他地方的按下、移动和滚轮会穿透到它背后的内容：

```go
pet := mygo.NewWindow(mygo.WindowOptions{
	Frameless:   true,
	Transparent: true,
	AlwaysOnTop: true,
	SkipTaskbar: true,
	Width:       320,
	Height:      240,
	Content:     ui.View(app.pet),
})
character := mygo.Rectangle{X: 200, Y: 120, Width: 110, Height: 110}
pet.SetInputRegion([]mygo.Rectangle{character})

// The bubble shows: it takes clicks too, until it goes.
pet.SetInputRegion([]mygo.Rectangle{character, {X: 10, Y: 10, Width: 220, Height: 90}})
```

在接收点击的内容移动或改变大小时，重新设置区域：矩形固定在它们在窗口中的位置，而窗口可能整体移动（`SetPosition`，或从角色开始 `StartDrag`）。页面用 `getBoundingClientRect` 找到这些矩形，并通过绑定的方法发送它们；原生 UI 用元素的 `Bounds`。`SetInputRegion(nil)` 重新让窗口在所有地方接收鼠标，如同它开始时；空切片让一切都穿透，如同 `SetIgnoreMouseEvents(true)`，后者在设置时优先于区域。原生 UI 中的 `PassThrough` 是另一回事：它让指针穿过一个元素，到达窗口在其下显示的内容，而从不穿到另一个窗口。

各平台如何做到：

| | macOS | Linux | Windows |
|---|---|---|---|
| 区域 | 指针在矩形外时窗口让鼠标穿过（`ignoresMouseEvents`），随指针移动而切换，依据应用内和其他应用中指针移动的监听器 | 窗口的输入形状：X11 的 SHAPE 扩展、Wayland 的输入区域 | 光标在矩形外时窗口让鼠标穿过（分层，`WS_EX_TRANSPARENT`）；窗口显示期间，定时器在系统时钟的每一拍（15.6 ms）询问一次 |

在 macOS 和 Windows 上，按住鼠标键时窗口保持原样：从角色上开始的拖拽在那里结束，从背后内容上开始的拖拽也在那里结束，松开后决定下一次谁接收鼠标。

在会忽略置顶的 Wayland 下，这样的窗口（无边框、`AlwaysOnTop` 且 `SkipTaskbar`，没有父窗口）在合成器支持 layer shell 协议（KDE Plasma、Sway、Hyprland、COSMIC……）且已安装 `libgtk-layer-shell.so.0` 时，会像[面板](#面板)一样使用该协议的绘制表面：显示在覆盖层，位于全屏窗口之上，不出现在任务栏中，按窗口的边界定位，不受桌面面板预留区域的影响；只有被点击时才接收键盘，因此它下面的应用保持键盘焦点。它会一直保持在上层直到关闭：`SetAlwaysOnTop(false)` 无法让 layer shell 表面变回普通窗口，因此允许用户关闭置顶的应用需要重新打开一个不带 `AlwaysOnTop` 的窗口。在其他桌面上（例如没有 layer shell 的 GNOME），它与普通窗口一样，可能被其他窗口遮住；在 XWayland 下（`GDK_BACKEND=x11`），GNOME 会让它保持在其他窗口之上，全屏窗口也不例外。

## 事件

监听器在主线程上运行：保持简短，耗时工作另起 goroutine。每个 `On…` 方法返回一个移除监听器的函数。

| 事件 | 何时 |
|---|---|
| `OnClose(func(e *mygo.CloseEvent))` | 窗口即将关闭，由用户或 `Close`；`e.PreventDefault()` 保持它打开 |
| `OnClosed` | 窗口已关闭 |
| `OnFocus`, `OnBlur` | 它获得或失去键盘焦点 |
| `OnShow`, `OnHide` | 它被显示或隐藏 |
| `OnResize`, `OnMove` | 它被调整大小或移动 |
| `OnMaximize`, `OnUnmaximize`, `OnMinimize`, `OnRestore` | 其状态改变 |
| `OnEnterFullScreen`, `OnLeaveFullScreen` | 它进入或离开全屏 |
| `OnReadyToShow` | 其第一个页面已准备好显示，或其原生 UI 绘制了第一帧 |
| `OnFileDrop` | 文件被拖放到其页面上，见[拖放的文件](frontend.md#拖放的文件)，或拖放到未接收它们的原生 UI 上，见[拖放的文件](ui/input.md#拖入的文件) |

在关闭有未保存更改的窗口之前询问：

```go
win.OnClose(func(e *mygo.CloseEvent) {
	if !doc.Dirty() {
		return
	}
	e.PreventDefault()
	go func() {
		res, _ := mygo.Dialog.Message(mygo.MessageOptions{
			Parent:  win,
			Type:    mygo.MessageQuestion,
			Message: "Save changes before closing?",
			Buttons: []string{"Save", "Don't Save", "Cancel"},
		})
		switch res.Button {
		case 0:
			doc.Save()
			win.Destroy()
		case 1:
			win.Destroy()
		}
	}()
})
```

`Close` 与用户关闭窗口的行为一致，允许 `OnClose` 监听器阻止关闭；`Destroy` 则立即关闭窗口。

## 页面

窗口一次显示一个页面，并像浏览器标签页一样有历史记录。`win.Page()` 返回它；显示[原生 UI](ui/README.md)的窗口返回 nil：

```go
page := win.Page()
page.LoadURL("/settings") // LoadHTML, LoadFile
page.Reload()             // ReloadIgnoringCache, Stop
page.GoBack()             // GoForward, CanGoBack, CanGoForward
page.URL()                // the current page
page.SetZoomFactor(1.25)  // ZoomFactor
```

`WindowOptions.Page` 设置页面：

```go
mygo.NewWindow(mygo.WindowOptions{
	URL:  "/",
	Page: mygo.PageOptions{ZoomFactor: 1.25, DevTools: mygo.DevToolsEnabled},
})
```

| 选项 | |
|---|---|
| `PreloadScript` | 在每个页面自身脚本之前运行的 JavaScript |
| `PreloadAllFrames` | 也在页面的 iframe 中运行 `PreloadScript`，包括任何来源的 iframe，即 `window.mygo` 不存在的地方 |
| `TrustedOrigins` | 其他来源，其页面可以调用 Go，见[谁可以调用](bindings.md#谁能调用) |
| `DevTools` | `DevToolsAuto`（开发构建中的检查器）、`DevToolsEnabled` 或 `DevToolsDisabled`；该选项还会打开或关闭 WebView2 自己的按键，如 F5 重新加载（Windows）。对于原生 UI 的窗口，则是其[检查器](ui/inspector.md) |
| `ZoomFactor`, `UserAgent` | 页面的缩放（1 为 100%）和用户代理 |
| `SafariTabFocus` | 把 Tab 留给文本框和带 tabindex 的元素，如同 Safari 在系统键盘导航关闭时的做法；默认情况下 Tab 也会到达链接、按钮、复选框和下拉框（macOS） |
| `PictureInPicture` | 让页面的视频进入画中画，见[画中画](#画中画)（macOS；WebView2 已经允许） |
| `SwipeNavigation` | 水平滑动会在页面历史中后退和前进，如同浏览器，见[手势](#滑动和捏合) |
| `PinchZoom` | 捏合会放大页面，如同浏览器，见[手势](#滑动和捏合) |
| `UserAgentSuffix` | 附加到用户代理而不是替换它（`UserAgent` 会替换），如 `"MyApp/1.2"`；它也会附加到 `UserAgent`，`SetUserAgent` 会保留它，且会去掉首尾空格 |
| `ContentRules` | 阻止页面加载的内容并隐藏页面一部分的规则，见[阻止内容](#阻止内容) |
| `BackgroundThrottling` | `ThrottlingOn`（web view 自己的）或 `ThrottlingOff`，后者让页面在其窗口隐藏、最小化或被覆盖时继续运行，见[后台页面](#后台页面) |
| `Autoplay` | `AutoplayUserGesture`（web view 自己的策略）或 `AutoplayAllow`，后者让页面在用户点击或输入之前播放可听媒体，在隐藏的窗口中也一样，见[自动播放](#自动播放) |
| `Session` | 页面的 cookie、存储和缓存的存储区，与其他会话的分开，见[会话](app.md#会话)；nil 是默认会话 |

`UserAgentSuffix` 可让网站识别应用，也可用于那些因 web view 默认用户代理未标明浏览器而返回旧版页面的网站：添加 `"Version/17.0 Safari/605.1.15"` 可让网站将其识别为 Safari。

页面事件，位于 `win.Page()` 上：

| 事件 | 何时 |
|---|---|
| `OnWillNavigate(func(e *mygo.NavigateEvent))` | 页面即将导航，例如点击链接之后；`e.PreventDefault()` 取消它。`e.UserInitiated` 表示导航是否由用户的手势开始——点击链接，或点击后由处理器设置 `location.href`——还是页面脚本自行发起。`LoadURL` 及其同类不会调用它，页面的 iframe 也不会，除了 Linux 上 WebKitGTK 2.56 之前的版本，那里无法告知导航来自哪个框架：在那里它也会为 iframe 的导航调用，但 `srcdoc` 的除外。 |
| `OnDidNavigate(func(url string))` | 一次导航已提交 |
| `OnDOMReady` | 页面的 DOM 已就绪（`DOMContentLoaded`） |
| `OnDidFinishLoad` | 页面完成加载 |
| `OnDidFailLoad(func(err *mygo.LoadError))` | 页面加载失败 |
| `OnPageTitleUpdated(func(e *mygo.TitleEvent))` | 页面的 `<title>` 改变；阻止该事件会保持窗口标题不变 |
| `OnThemeColorChanged(func(c color.NRGBA, ok bool))` | 页面的[主题色](#主题色)改变；当它不再有主题色时 `ok` 为 false |
| `OnRenderProcessGone(func(reason string))` | Web 内容进程崩溃或被杀死；`Reload` 可恢复 |
| `OnAudibleChanged(func(audible bool))` | 页面开始或停止[发出声音](#声音) |
| `OnCaptureStateChanged(func(kind mygo.CaptureKind, s mygo.CaptureState))` | 页面开始或停止[捕获](#摄像头和麦克风)摄像头、麦克风或显示器，或捕获被静音或取消静音 |

让窗口停留在应用自己的页面上，并在浏览器中打开其他链接：

```go
win.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
	if u, err := url.Parse(e.URL); err == nil && (u.Scheme == "http" || u.Scheme == "https") && e.UserInitiated {
		e.PreventDefault()
		go mygo.Shell.OpenExternal(e.URL)
	}
})
```

开发期间，应用页面来自开发服务器，使用 `http` URL：如果规则依赖 URL 协议，应改为检查主机，或使用 `mygo.IsDev()` 区分开发环境。

打开新窗口的链接会经过窗口的打开处理器，见[链接和新窗口](frontend.md#链接与新窗口)。

### 滑动和捏合

页面默认不会因滑动而后退，也不会因捏合而缩放，这与桌面应用的惯例一致。设置 `SwipeNavigation` 和 `PinchZoom` 可启用这些手势，窗口页面和 [web view](ui/web-view.md) 页面均适用。

```go
mygo.NewWindow(mygo.WindowOptions{
	URL:  "https://example.com/",
	Page: mygo.PageOptions{SwipeNavigation: true, PinchZoom: true},
})
```

捏合会放大页面显示的内容而不重新布局它，如同浏览器，并保持 `ZoomFactor`（它会以另一个大小布局页面）不变。

| | `SwipeNavigation` | `PinchZoom` |
|---|---|---|
| macOS | 触控板或 Magic Mouse 在页面之间的滑动，按“系统设置”中的设定（`allowsBackForwardNavigationGestures`） | 触控板的捏合和智能缩放（`allowsMagnification`） |
| Linux | 触控板上两根手指、触摸屏上一根手指（`enable-back-forward-navigation-gestures`） | 触摸屏的捏合，以及触控板的捏合，GTK 只在 Wayland 上报告后者。WebKitGTK 会缩放且没有设置可以阻止它：从 WebKitGTK 2.38 起 MyGo 会关闭其缩放手势，更早的版本总是缩放 |
| Windows | 触摸屏上的滑动（`IsSwipeNavigationEnabled`） | 触摸屏的捏合（`IsPinchZoomEnabled`）；没有 `ICoreWebView2Settings5` 的 WebView2 Runtime 总是缩放 |

WebKitGTK 和 WebView2 默认开启捏合，WebView2 还默认开启滑动：除非页面要求，MyGo 会关闭它们。

### 主题色

`ThemeColor` 返回页面的主题色，即其 `<meta name="theme-color">`，以便应用可以按页面给标题栏、工具栏或窗口背景着色，如同 Safari；`OnThemeColorChanged` 在它改变时告知：

```go
page := win.Page()
page.OnThemeColorChanged(func(c color.NRGBA, ok bool) {
	if !ok {
		c = defaultToolbarColor // the page has none
	}
	toolbar.SetBackground(ui.RGB(c.R, c.G, c.B))
})
```

页面可以通过 meta 的 `media` 为浅色和深色外观分别指定颜色：

```html
<meta name="theme-color" content="#ffffff" media="(prefers-color-scheme: light)">
<meta name="theme-color" content="#111111" media="(prefers-color-scheme: dark)">
```

该颜色取自页面 `<head>` 中第一个 `media` 缺失或匹配、且内容是颜色的 `theme-color` meta，如同浏览器挑选的那样。页面的引擎计算它，所以任何 CSS 颜色都可用（`dodgerblue`、`rgb(30 144 255 / 50%)`、`oklch(…)`），并以带直通 alpha 的 8 位 sRGB 给出。事件只在颜色改变时触发：页面添加、移除或更改其 meta；应用的外观（[`Theme`](native.md#深色模式)）改变，且另一个 meta 的 `media` 匹配；或另一个页面有不同的颜色。刚开始加载的页面会保留前一个页面的颜色直到其 DOM 就绪，所以在相同颜色的页面之间切换不会触发任何事件。

页面中 MyGo 的运行时在每个平台上以同样的方式观察这些 meta，适用于任何来源的页面，受信任与否。运行时不在其中运行的页面（如 web view 显示的图像或 PDF）没有主题色。

### 声音

`IsAudible` 表示页面是否有音频正在播放，包括嵌入页面中来自其他网站的视频；`OnAudibleChanged` 在此状态变化时触发，可用于更新标签页或窗口上的扬声器图标，以及媒体应用的状态。`SetAudioMuted` 将整个页面静音，与浏览器静音标签页类似，页面内的媒体元素不会感知到这一静音操作；静音状态在页面导航后仍然保留：

```go
page := win.Page()
page.OnAudibleChanged(func(audible bool) {
	log.Println("playing:", audible) // show a speaker on the window's tab
})

// A menu item that mutes or unmutes the window it was chosen for.
mute := &mygo.MenuItem{Label: "Mute or Unmute Window", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
	if win == nil || win.Page() == nil {
		return // no window, or one showing native UI
	}
	win.Page().SetAudioMuted(!win.Page().IsAudioMuted())
}}
```

在 `NewWindow` 之后立即设置静音，页面就会以静音状态启动，即使此时 web view 尚未创建。静音页面仍可能被报告为正在播放音频，这与浏览器标签页显示带叉的扬声器图标类似，并不表示用户能听到声音。暂停或无声的音频源何时不再计入，由引擎决定，因此声音停止后状态可能稍有延迟。

| 平台 | 发出声音 | 静音 |
|---|---|---|
| macOS | WebKit 私有的 `_isPlayingAudio`，被观察；如果 WebKit 不再回应它则为 false | WebKit 私有的 `_setPageMuted:`，如同 Safari 使标签页静音；如果 WebKit 不再回应它则什么也不做 |
| Windows | WebView2 的 `IsDocumentPlayingAudio`（`ICoreWebView2_8`）；在比它更旧的 runtime 上为 false | 同一个的 `IsMuted`；在更旧的 runtime 上什么也不做 |
| Linux | `webkit_web_view_is_playing_audio` | `webkit_web_view_set_is_muted`，WebKitGTK 2.30 或更高版本；在更旧的版本上什么也不做 |

平台无法静音时，`IsAudioMuted` 为 false。显示原生 UI 的窗口以及窗口关闭之后，两者都为 false。[web view](ui/web-view.md)的页面也有它们。

### 后台页面

Web view 会减慢没人能看到的页面以节省电量：一旦其窗口隐藏、最小化或位于其他窗口之后，其定时器最多每秒触发一次，`requestAnimationFrame` 会等待，macOS 还可能挂起它。在后台播放、同步或计算的页面用 `ThrottlingOff` 保持运行，如同 Electron 的 `backgroundThrottling: false`：

```go
mygo.NewWindow(mygo.WindowOptions{
	URL:  "/player",
	Page: mygo.PageOptions{BackgroundThrottling: mygo.ThrottlingOff},
})
```

无论在窗口中还是在 [web view](ui/web-view.md) 中，其定时器都保持原有频率，其进程也保持可见页面的优先级。被覆盖的窗口的页面在 macOS 和 Windows 上也保留动画帧，但隐藏或最小化窗口的页面在 macOS 和 Linux 上仍会变为隐藏（`document.visibilityState`），Windows 上隐藏窗口的 web view 也是如此，那时它不绘制任何东西：必须继续的工作就用定时器来运行。

| | |
|---|---|
| macOS | `WKPreferences.inactiveSchedulingPolicy` 为 none（macOS 14+），WebKit 私有的隐藏页面定时器节流、CSS 动画暂停和进程抑制关闭，窗口遮挡检测关闭，使被覆盖的窗口算作可见 |
| Linux | WebKitGTK 的 `HiddenPageDOMTimerThrottling` 和 `HiddenPageCSSAnimationSuspension` 特性关闭（WebKitGTK 2.42+；之前没有） |
| Windows | 其浏览器以 `--disable-background-timer-throttling --disable-renderer-backgrounding --disable-backgrounding-occluded-windows` 运行 |

在 Windows 上，这些参数属于 WebView2 环境，应用的所有页面共享它，并由第一个显示页面的窗口或 web view 创建：把选项给那一个。之后应用中没有任何页面的定时器被节流；稍后再请求的页面已经太晚，日志会说明。应用的两个进程也共享浏览器，所以它们必须一致：如果第二个进程的第一个页面要求的与正在运行的进程不同，WebView2 会拒绝给它 web view。隐藏窗口的[原生 UI](ui/web-view.md) 中的 web view 仍会变为隐藏，因为它的窗口要离开屏幕就必须如此。

### 自动播放

WebKitGTK 和 WebView2 只在用户点击或输入之后才让页面开始可听媒体：在此之前，未静音的 `<audio>` 或 `<video>` 的 `play()` 会以 `NotAllowedError` 拒绝，而正在播放的静音媒体在取消静音时会再次暂停。隐藏窗口的页面永远得不到这样的手势，所以它在那里根本无法播放声音。应用若使用[原生 UI](native.md) 作为界面，又在隐藏窗口的页面中播放媒体，可以用 `AutoplayAllow` 让该页面播放：

```go
mygo.NewWindow(mygo.WindowOptions{
	URL:    "/player",
	Hidden: true,
	Page:   mygo.PageOptions{Autoplay: mygo.AutoplayAllow},
})
```

这样 `play()` 会成功返回，`autoplay` 元素会带着声音开始，在窗口和 [web view](ui/web-view.md) 中都是如此。默认值 `AutoplayUserGesture` 让页面遵循其 web view 自己的策略。

| | |
|---|---|
| macOS | `WKWebViewConfiguration.mediaTypesRequiringUserActionForPlayback` 为 none |
| Linux | 页面主框架导航的网站策略允许自动播放（`WEBKIT_AUTOPLAY_ALLOW`，WebKitGTK 2.30+；之前没有） |
| Windows | 其浏览器以 `--autoplay-policy=no-user-gesture-required` 运行 |

在 Windows 上，与 [`ThrottlingOff`](#后台页面)一样，该参数属于 WebView2 环境，应用的所有页面共享它，并由第一个显示页面的窗口或 web view 创建：把选项给那一个。之后应用中的每个页面都可以无手势播放，保持 `AutoplayUserGesture` 的页面也一样；稍后再请求的页面已经太晚，日志会说明。在第一个窗口是原生 UI 的应用中，播放声音的页面所在的窗口通常就是第一个显示页面的窗口，给它选项就够了。第一个窗口就显示页面的应用，也要把选项给该窗口，或先创建要播放的页面。应用的两个进程必须一致，如同 `ThrottlingOff`。

### 摄像头和麦克风

`CaptureState` 表示页面是否在捕获摄像头、麦克风或显示器（`getUserMedia()`、`getDisplayMedia()`），来自其任何框架，以及捕获是否被静音：`CaptureNone`、`CaptureActive` 或 `CaptureMuted`。`OnCaptureStateChanged` 在该状态改变时告知，用于窗口或菜单栏中的录制指示器，如同 Safari 显示的那样。`SetCaptureState` 无需页面配合就能静音、取消静音或停止捕获，如同通话应用的静音按钮：被静音的捕获保留页面的轨道，轨道收到的是无声或黑帧；被停止的捕获则结束它们。

```go
page := win.Page()
page.OnCaptureStateChanged(func(kind mygo.CaptureKind, s mygo.CaptureState) {
	if kind == mygo.CaptureMicrophone {
		log.Println("microphone live:", s == mygo.CaptureActive) // show a recording indicator
	}
})

// A menu item that mutes or unmutes the microphone of the window it was chosen for.
mute := &mygo.MenuItem{Label: "Mute or Unmute Microphone", Accelerator: "CmdOrCtrl+Shift+M", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
	if win == nil || win.Page() == nil {
		return
	}
	p := win.Page()
	switch p.CaptureState(mygo.CaptureMicrophone) {
	case mygo.CaptureActive:
		p.SetCaptureState(mygo.CaptureMicrophone, mygo.CaptureMuted)
	case mygo.CaptureMuted:
		p.SetCaptureState(mygo.CaptureMicrophone, mygo.CaptureActive)
	}
}}
```

页面先请求设备，由[权限](#权限)决定。页面接受更改后，`CaptureState` 和事件会在 `SetCaptureState` 之后片刻反映这一变化；页面没有捕获这类设备时，`SetCaptureState` 什么也不做。页面自己不能取消静音：再次请求设备会像新的捕获一样经过权限处理器。

| 平台 | 捕获状态 | 静音和停止 |
|---|---|---|
| macOS | `WKWebView` 的 `cameraCaptureState` 和 `microphoneCaptureState`（macOS 12+），以及 WebKit 私有的 `_displayCaptureState`（macOS 13+），被观察；如果 WebKit 不回应它们则为 `CaptureNone` | `setCameraCaptureState:completionHandler:`、`setMicrophoneCaptureState:completionHandler:` 和私有的 `_setDisplayCaptureState:completionHandler:`；当摄像头和麦克风只有一个被静音时，静音或取消静音页面的声音（`SetAudioMuted`）会短暂地静音另一个，如同 WebKit 一起静音它们 |
| Linux | `webkit_web_view_get_camera_capture_state` 及其麦克风和显示器同类接口，WebKitGTK 2.34 或更高版本，以及它们的 `notify` 信号 | 它们的设置方法；静音或取消静音页面的声音（`SetAudioMuted`）会取消静音 WebKitGTK 中的每个捕获，MyGo 会立即再次静音，所以被静音的捕获可能短暂处于活动状态 |
| Windows | 总是 `CaptureNone`：WebView2 不报告捕获 | 什么也不做 |

显示原生 UI 的窗口以及窗口关闭之后，`CaptureState` 为 `CaptureNone`。[web view](ui/web-view.md)的页面也有它们。

### 画中画

`PageOptions.PictureInPicture` 让页面的视频浮出到画中画，即一个位于每个应用的窗口之上的小窗口，如同它们在 Safari 中那样：视频控件的按钮、其上下文菜单的项，以及页面脚本中的 `requestPictureInPicture()`。WKWebView 只把它留给 Safari，所以 macOS 上的页面需要该选项；WebView2 则无论此选项如何设置，都允许视频进入画中画。`TogglePictureInPicture` 无需在页面中点击，就能把视频移出，或把已移出的视频移回页面；`IsPictureInPictureActive` 表示是否有视频已移出，无论谁把它放在那里：

```go
win := mygo.NewWindow(mygo.WindowOptions{
	URL:  "/player",
	Page: mygo.PageOptions{PictureInPicture: true},
})

// A menu item that floats the video of the window it was chosen for.
pip := &mygo.MenuItem{Label: "Picture in Picture", Accelerator: "CmdOrCtrl+Shift+P", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
	if win != nil && win.Page() != nil {
		win.Page().TogglePictureInPicture()
	}
}}
```

| 平台 | |
|---|---|
| macOS | WebKit 私有的 `_allowsPictureInPictureMediaPlayback` 偏好，以及 `_togglePictureInPicture`、`_canTogglePictureInPicture` 和 `_isPictureInPictureActive`，用于 WebKit 为其媒体控件挑选的视频；如果 WebKit 不再回应它们，它们都什么也不做，或为 false。在视频处于画中画状态下导航的页面不会进入前进/后退缓存（私有的 `_setUsesPageCache:`），否则 WebKit 从缓存中把它带回时，会以为它的视频仍在画中画中 |
| Windows | Chromium 的，在每个页面中开启。`TogglePictureInPicture` 以用户手势调用页面文档（不是其 iframe）中正在播放的视频的 `requestPictureInPicture()`，否则调用最大的；`IsPictureInPictureActive` 向页面询问 `document.pictureInPictureElement`，iframe 中的视频会让它为 null |
| Linux | 无：WebKitGTK 没有画中画，所以 `TogglePictureInPicture` 什么也不做，`IsPictureInPictureActive` 为 false |

显示原生 UI 的窗口以及窗口关闭之后，两者都什么也不做，或为 false。[web view](ui/web-view.md)的页面也有它们。

### 在页面中查找

`FindInPage` 高亮一段文本的匹配项并报告有多少个，如同浏览器的查找栏；在用户输入时调用它，并用 `FindNext` 前往下一个匹配项：

```go
res, err := win.Page().FindInPage("mygo", mygo.FindOptions{FindNext: true})
fmt.Printf("%d of %d\n", res.Active, res.Matches)
win.Page().StopFindInPage()
```

### 打印、PDF 和截图

```go
win.Page().Print() // the print dialog

pdf, err := win.Page().PrintToPDF(mygo.PDFOptions{
	PageSize:   mygo.PageA4,
	Landscape:  false,
	Margins:    &mygo.Margins{Top: 0.5, Right: 0.5, Bottom: 0.5, Left: 0.5}, // inches
	Background: true, // print background colors and images
})

png, err := win.CapturePage() // what the window shows, as a PNG
```

`PrintToPDF` 为打印而布局页面，所以其打印样式表会生效。零值 `PDFOptions` 打印 Letter 页面，纵向，边距 0.4 英寸，没有背景，如同浏览器的打印对话框。

PDF 的文本会像页面的文本一样复制和搜索。在 macOS 上，WebKit 打印的 PDF 会为许多汉字命名部首，例如用 ⼈（U+2F08）表示 人，系统字体会用相同的字形绘制它们。`PrintToPDF` 向已安装的字体询问其字形是哪些字符，并向 PDF 追加修正后的文本映射，同时不改变 PDF 页面的外观。未安装字体中的文本（如 Web 字体）保留部首。

### 下载

页面在跟随带 `download` 属性的链接，或收到无法显示的响应（如附件）时下载文件。默认情况下文件进入“下载”目录，使用页面或服务器建议的名称。`OnWillDownload` 选择另一个路径或取消下载，`OnDownloadDone` 报告它如何结束：

```go
win.Page().OnWillDownload(func(e *mygo.DownloadEvent) {
	if strings.HasSuffix(e.SuggestedName, ".exe") {
		e.PreventDefault()
		return
	}
	dir, _ := mygo.App.Path(mygo.PathDocuments)
	e.Path = filepath.Join(dir, e.SuggestedName)
})
win.Page().OnDownloadDone(func(d *mygo.Download) {
	if d.Err == nil {
		mygo.Shell.ShowItemInFolder(d.Path)
	}
})
```

### 权限

页面可以请求摄像头、麦克风、位置或通知权限。默认情况下，应用自己的页面（包括使用自定义协议的页面，见[信任](frontend.md#信任)）会获准，其他页面则会被拒绝。设置 `Page.SetPermissionHandler` 后，由该处理器决定是否授权；它在主线程上运行：

```go
win.Page().SetPermissionHandler(func(req mygo.PermissionRequest) bool {
	return req.Origin == "https://meet.example.com" &&
		!slices.Contains(req.Permissions, mygo.PermissionGeolocation)
})
```

操作系统也可能询问用户。macOS 会就摄像头和麦克风向每个应用询问一次，并会结束那些使用它们却未在 `Info.plist` 中说明原因的应用：在 mygo.config.ts 的 `macos.infoPlist` 中添加用途说明：

```ts
export default defineConfig({
  macos: {
    infoPlist: {
      NSCameraUsageDescription: "Scan documents with the camera.",
      NSMicrophoneUsageDescription: "Record voice notes.",
    },
  },
});
```

### 登录和证书

受 HTTP 认证（Basic、Digest、NTLM）保护的页面——无论是服务器还是代理要求的——都会请求用户名和密码。`Page.SetAuthHandler` 在主线程上回答，给出凭据，或给出空 `AuthResponse` 以取消，后者会显示服务器的拒绝：

```go
win.Page().SetAuthHandler(func(req mygo.AuthRequest) mygo.AuthResponse {
	if req.Host == "router.lan" && !req.IsProxy {
		return mygo.AuthResponse{Username: "admin", Password: password}
	}
	return mygo.AuthResponse{}
})
```

web view 会在本次会话余下的时间里保留服务器接受的凭据。没有处理器时，WebKitGTK 和 WebView2 显示自己的登录对话框，macOS 则不显示。要以应用自己的方式询问用户，先取消，再提示，然后在处理器拿到凭据后 `Reload`。

与浏览器一样，服务器证书无法验证时（例如开发服务器使用自签名证书），页面会加载失败。`Page.SetCertificateErrorHandler` 可以允许继续加载，但必须校验 `CertificateError.Certificate` 中的证书（DER 编码），只放行符合预期的证书，绝不要仅凭主机名就无条件接受其提供的任何证书：

```go
win.Page().SetCertificateErrorHandler(func(e mygo.CertificateError) bool {
	sum := sha256.Sum256(e.Certificate)
	return e.Host == "dev.example" && sum == devCertificateSHA256
})
```

在 Linux 上只询问页面本身，不询问它加载的内容；在 Linux 和 Windows 上，为某个主机允许的证书会在每个窗口中保持允许，直到应用退出。

服务器或代理还可以在提供页面之前请求客户端证书（双向 TLS）。`Page.SetClientCertificateHandler` 用平台提供的某个证书回答，或用空 `ClientCertificateResponse` 拒绝，后者会让加载失败，如同用户关闭浏览器的提示：

```go
win.Page().SetClientCertificateHandler(func(req mygo.ClientCertificateRequest) mygo.ClientCertificateResponse {
	for _, c := range req.Certificates {
		if c.Subject == "CN=alice" {
			return mygo.ClientCertificateResponse{Certificate: c}
		}
	}
	return mygo.ClientCertificateResponse{}
})
```

能提供哪些证书取决于平台：Windows 上是 WebView2 的相互信任证书，macOS 上是钥匙串的身份。WebKitGTK 不列出任何证书，所以在 Linux 上 `Certificates` 为空，处理器只能拒绝；没有处理器时，每个平台显示自己的选择（WebView2 的对话框、WebKitGTK 的选择器；macOS 从不询问）。Linux 上的拒绝会以取消结束加载，`OnDidFinishLoad` 会报告它，`OnDidFailLoad` 不会。

### 阻止内容

内容规则阻止页面加载的内容并隐藏其部分，如同广告和跟踪器拦截器。它们用 WebKit 的[内容拦截格式](https://developer.apple.com/documentation/safariservices/creating-a-content-blocker)编写：一个规则 JSON 数组，每条规则由一个触发器和动作组成，触发器用正则表达式匹配要加载的 URL。`mygo.CompileContentRules` 编译它们，页面通过 `PageOptions.ContentRules` 或 `Page.SetContentRules` 应用它们：

```go
rules, err := mygo.CompileContentRules("ads", `[
	{"trigger": {"url-filter": "^[^:]+://+([^:/]+\\.)?ads\\.example\\.com[:/]"}, "action": {"type": "block"}},
	{"trigger": {"url-filter": ".*", "if-domain": ["*news.example"]},
	 "action": {"type": "css-display-none", "selector": ".sponsored"}}]`)
if err != nil {
	return err
}
win := mygo.NewWindow(mygo.WindowOptions{URL: "https://news.example/",
	Page: mygo.PageOptions{ContentRules: []*mygo.ContentRules{rules}}})
```

触发器接受 `url-filter`、`url-filter-is-case-sensitive`，`if-domain`、`unless-domain`、`if-top-url` 和 `unless-top-url` 中的一个，以及 `resource-type` 和 `load-type`（`first-party` 或 `third-party`）。动作是 `block`、`block-cookies`、`make-https`、带 `selector` 的 `css-display-none`，以及 `ignore-previous-rules`，后者撤销它之前的规则对一次加载所做的处理。正则表达式是 WebKit 的：不支持交替（`|`）、计数重复或 `\d` 这样的字符类，且仅限 ASCII。域名是小写的 punycode。引擎无法解析其选择器的规则会被跳过；任何其他错误都会让整个列表失败。

`CompileContentRules` 对长列表要花一些时间，所以在主线程之外调用它。引擎把它编译的内容保存在该 id 下：相同的规则在之后的启动中立即加载，在该 id 下编译的其他规则会替换较早的规则。WebKit 限制一个列表的规则数（150,000 条，旧版本为 50,000 条），所以把长列表分成几个并一起应用；页面按顺序应用其列表，每个列表的 `ignore-previous-rules` 只撤销它自己的规则。`SetContentRules` 替换页面的列表，传入空时移除它们：从那时起的加载会经过它们，而页面在重新加载之前显示它已加载的内容。

macOS 和 Linux（WebKitGTK 2.24 或更新版本）用 WebKit 的内容拦截器应用规则。WebView2 没有，所以在 Windows 上 MyGo 会看到使用规则的页面的每个请求，并让被阻止的请求失败：被阻止的 `fetch` 会拒绝，`XMLHttpRequest` 会出错，如同在 WebKit 中；而被阻止的页面会显示 WebView2 对 403 响应的错误页。它会重定向 `make-https` 匹配的内容，并用每个文档采用的样式表隐藏元素。在那里：

- WebSocket 不会被阻止：WebView2 不向 MyGo 显示它们的请求。
- 弹出窗口（`resource-type` 为 `popup`）不会被匹配。
- `block-cookies` 移除请求的 cookie，而不是响应的 cookie。
- 自身 `style` 写着 `display: ... !important` 的元素会保留：样式表是页面的，而 WebKit 的是用户的，后者获胜。

### Web 检查器

页面的 `OpenDevTools`、`CloseDevTools`、`ToggleDevTools` 和 `IsDevToolsOpened` 在其有检查器时控制它，见 [Web 检查器](frontend.md#网页检查器)。

### 页面的进程

`ProcessID` 返回运行页面的操作系统进程 ID，用于应用自己的任务管理器、内存报告，或判断繁忙的进程属于哪个窗口；未知时为 0：

```go
pid := win.Page().ProcessID()
```

| 平台 | 进程 |
|---|---|
| macOS | 页面的 Web 内容进程（`com.apple.WebKit.WebContent`），来自 WebKit 私有的 `_webProcessIdentifier`；如果 WebKit 不再回应它则为 0。页面导航到另一个网站后，或在其进程崩溃后，可能移到另一个进程，所以再次询问，而不要保留该 ID |
| Windows | WebView2 的浏览器进程（`msedgewebview2.exe`），应用的 web view 共享它，其子进程渲染页面 |
| Linux | 无：WebKitGTK 没有页面进程的 API，所以总是 0 |

在 web view 启动进程之前、窗口关闭之后，以及显示原生 UI 的窗口中，它也是 0。

`GPUProcessID` 返回为页面处理图形和媒体的进程 ID。在 macOS 上，页面的声音从那里输出，所以显示页面所播放内容的应用（例如音乐播放器的可视化器）可以用 Core Audio 进程 tap（`CATapDescription`，macOS 14.2 及更高版本）捕获它，而这需要该进程的 ID：

```go
pid := win.Page().GPUProcessID()
```

| 平台 | 进程 |
|---|---|
| macOS | WebKit 的 GPU 进程（`com.apple.WebKit.GPU`），它播放页面的音频和视频，来自 WebKit 私有的 `_gpuProcessIdentifier`；如果 WebKit 不回应它或尚未启动该进程则为 0，它会在页面首次需要时（播放媒体或用 GPU 绘制）启动该进程 |
| Windows | WebView2 的 GPU 进程（`msedgewebview2.exe --type=gpu-process`），是 `ProcessID` 的浏览器进程的子进程，来自 `ICoreWebView2Environment8` 的 `GetProcessInfos`；没有它的 runtime 上为 0。WebView2 从其音频服务（一个工具进程）播放声音，而不是这个进程：为此捕获 `ProcessID` 进程的进程树（进程环回） |
| Linux | 无：WebKitGTK 没有其 GPU 进程的 API，所以总是 0 |
| 浏览器 | 无：总是 0 |

在 web view 启动进程之前、窗口关闭之后，以及显示原生 UI 的窗口中，它也是 0。应用的 web view 可能共享该进程，而且每次应用运行以及 web view 在其崩溃或退出后重新启动它时，它都会得到新的 ID，所以再次询问，而不要保留该 ID。

## 查找窗口

- `mygo.Windows()` 列出打开的窗口，`mygo.FocusedWindow()` 返回获得焦点的那个，`mygo.WindowByID(id)` 返回 `ID()` 为该值的窗口，页面通过 `runtime().windowId` 认识它。
- `mygo.CallerWindow(ctx)` 返回调用绑定方法的页面所属的窗口，`mygo.CallerPage(ctx)` 返回该页面：可能是窗口的页面，也可能是原生 UI 窗口中 web view 的页面（[Web view](ui/web-view.md)）。传入自定义协议请求的 `r.Context()` 时，它们返回发起该请求的窗口和页面。
- 对每个新窗口调用 `mygo.App.OnWindowCreated`。

## Linux 上的 GTK 样式

在 Linux 上，GTK 会用桌面的 GTK 3 主题自己绘制窗口框架的一部分：桌面交给应用自行装饰的窗口，其标题栏和装饰（Wayland 上的 GNOME）、GTK 在 Wayland 上围绕无边框窗口或隐藏式标题栏绘制的阴影和边框，以及隐藏式标题栏的标题按钮；其菜单和对话框也是如此。`mygo.Theme.SetGTKCSS` 用应用自己的 GTK 3 样式表为它们设置样式，以获得与桌面其余部分匹配的框架，如 GTK 4 和 libadwaita 应用的扁平标题栏和圆形标题按钮：

```go
const gtk4Frame = `
decoration { border-radius: 12px; box-shadow: 0 3px 9px 1px rgba(0, 0, 0, .28); }
.maximized decoration, .fullscreen decoration, .tiled decoration { border-radius: 0; }
.csd.popup decoration, tooltip.csd decoration { border-radius: 5px; }
headerbar { min-height: 46px; padding: 0 6px; background-color: #f9fafb;
            background-image: none; border-width: 0; box-shadow: none; }
button.titlebutton { min-width: 24px; min-height: 24px; margin: 0 3px; padding: 0;
                     border: none; border-radius: 9999px; background-color: transparent; }
`

if err := mygo.Theme.SetGTKCSS(gtk4Frame); err != nil {
	log.Print(err) // the sheet does not parse
}
```

- GTK 以用户的优先级（`GTK_STYLE_PROVIDER_PRIORITY_USER`）采用该样式表：其规则胜过主题的、MyGo 自己的以及用户的 `~/.config/gtk-3.0/gtk.css` 的规则。
- 另一次调用会替换该样式表，如同切换应用主题，`""` 会移除它。无法解析的样式表会返回 GTK 的错误，并指出位置，先前设置的样式表会保留。
- 可以从任何 goroutine 调用，也可以在 `App.Run` 之前调用：其他 goroutine 会等待 Run 加载样式表后再返回；主 goroutine 中设置的样式表则留待 Run 加载，由 Run 记录错误，因为届时原调用已经返回，无法再返回错误。
- 页面和原生 UI 从不会看到它：MyGo 绘制它们，而页面有自己的 CSS。在 macOS、Windows 和浏览器中，它什么也不做并返回 nil。

GTK 3 只给它所装饰窗口的上角加圆角，无论 `decoration` 的 `border-radius` 是多少：它把装饰裁切到该半径，而从不裁切窗口的内容，所以页面或原生 UI 会覆盖下角，下角保持方形。需要四个圆角的窗口自己绘制它们：一个 `Frameless`、`Transparent` 的窗口（GTK 不给它加装饰，也没有阴影），其页面或原生 UI 给自己的背景加圆角，在合成窗口管理器下或在 Wayland 上。

## 原生访问

`win.NativeHandle()` 返回原生窗口：macOS 上是 `NSWindow*`，Linux 上是 `GtkWindow*`，Windows 上是 `HWND`。它用于 MyGo 未覆盖的 API。在主线程上用 `mygo.RunOnMain` 调用原生 API。
