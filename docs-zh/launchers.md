# 启动器

Spotlight、Raycast、Alfred、PowerToys Run 和 Ulauncher 这类启动器，都能通过快捷键随时唤出，在当前应用上方接收输入，用完即可快速收起。MyGo 提供了构建启动器所需的功能：用于显示界面的[面板](windows.md#面板)、唤出面板的[全局快捷键](native.md#全局快捷键)、可供搜索的[已安装应用](native.md#shell)列表及图标，以及不显示 Dock 图标、驻留在[托盘](menus.md#托盘图标)中的应用模式。[`examples/launcher`](https://github.com/egoist/mygo/tree/main/examples/launcher) 演示了如何用原生 UI 将它们组合起来：

```sh
go run ./examples/launcher
```

## 后台应用

启动器没有 Dock 图标，也不出现在应用切换器中，运行时不显示窗口，而且只运行一个实例：

```go
func main() {
	if !mygo.App.RequestSingleInstanceLock() {
		return // the launcher that runs shows itself
	}
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnSecondInstance(func([]string, string) { show() })
	mygo.App.WhenReady(ready)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
```

打包后的应用可以从一开始就声明这一点，让 macOS 从不把它显示在 Dock 中：在 [`mygo.json`](configuration.md#mygojson) 中设置 `LSUIElement`：

```json
{
  "macos": { "infoPlist": { "LSUIElement": true } }
}
```

托盘图标提供快捷键之外的打开方式，也方便用户退出；[`App.SetOpenAtLogin`](app.md#登录时启动) 让启动器随会话启动，可根据 `App.WasOpenedAtLogin` 判断是否应在登录时保持隐藏。

## 面板

面板显示在用户当前的 Space 或虚拟桌面，浮在所有应用之上，包括全屏应用；在 macOS 上，面板获得键盘焦点，但不激活启动器：原来的应用保留菜单栏，面板隐藏后键盘焦点也会返回该应用。面板只需以隐藏状态创建一次；用完后隐藏而不关闭，再次显示时即可恢复原样：

```go
panel := mygo.NewWindow(mygo.WindowOptions{
	Panel:         true,
	Hidden:        true,
	Frameless:     true,
	Transparent:   true,
	Vibrancy:      mygo.VibrancyGlass, // Liquid Glass on macOS 26, Acrylic on Windows 11
	CornerRadius:  16,
	Width:         680,
	Height:        420,
	DisableResize: true,
	Content:       ui.View(app.view), // or URL: "/launcher"
})
var quitting bool // a quit closes the panel
mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { quitting = true })
panel.OnClose(func(e *mygo.CloseEvent) { // Escape, Cmd-W, Alt-F4
	if !quitting {
		e.PreventDefault()
		panel.Hide()
	}
})
panel.OnBlur(panel.Hide) // the user went elsewhere
```

页面或原生 UI 未处理的 Escape 会关闭面板，`OnClose` 监听器将关闭改为隐藏。如果 Escape 需要先执行其他操作，就自行处理，如示例中先清空查询。退出应用时也会关闭面板，而保持打开的面板会取消退出：监听器放行这些关闭，和隐藏到[托盘](menus.md#托盘图标)的窗口一样。

根据指针位置确定用户当前使用的显示器，像 Spotlight 那样将面板放在从上往下三分之一处，并让搜索框获得键盘焦点：

```go
func show() {
	d := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
	w, h := panel.Size()
	panel.SetPosition(d.WorkArea.X+(d.WorkArea.Width-w)/2, d.WorkArea.Y+(d.WorkArea.Height-h)/3)
	app.query = ""
	app.input.Focus()
	panel.Show()
}
```

## 快捷键

```go
err := mygo.GlobalShortcut.Register("Alt+Shift+Space", func() {
	if panel.IsVisible() {
		panel.Hide()
	} else {
		show()
	}
})
```

选择未被其他启动器占用的快捷键：Raycast 和 PowerToys Run 占用 Alt+Space，Spotlight 占用 Cmd+Space，而 macOS 会将快捷键发送给所有注册它的应用，因此两个应用都会打开。允许用户在启动器设置中更换快捷键：用 `Unregister` 注销旧快捷键，用 `Register` 注册新快捷键，并在 `Register` 因快捷键已被占用而失败时提示用户。在 Windows 上，按键操作允许面板切换到前台；在 Wayland 上，由桌面绑定快捷键，见[全局快捷键](native.md#全局快捷键)。

## 查找应用

`Shell.InstalledApps` 列出计算机上安装的应用，按名称排序，名称采用系统以用户语言显示的形式。它会读取磁盘，耗时几十毫秒：面板显示时，在 goroutine 上列出它们，并用 `Window.Update`（或通过绑定服务交给页面）把它们交给视图：

```go
go func() {
	apps, err := mygo.Shell.InstalledApps()
	if err != nil {
		log.Print(err)
	}
	panel.Update(func() {
		app.apps = apps
		app.filter()
	})
}()
```

`Shell.FileIcon(app.Path, size)` 把应用图标绘制为 PNG，尺寸以像素计：DIP 尺寸乘以显示器的缩放因子。调用耗时几毫秒，因此只绘制可见行的图标，并按路径缓存。`Shell.LaunchApp(app)` 启动所选应用，或将其切换到前台；先隐藏面板，让键盘焦点回到原来的应用，直到启动的应用获得焦点。

## 平台

| | macOS | Windows | Linux |
|---|---|---|---|
| 没有 Dock 图标，没有任务栏按钮 | `LSUIElement`、`ActivationPolicyAccessory` | 面板没有 | 面板没有 |
| 面板 | 不激活应用的 `NSPanel`，可浮在常规应用或辅助应用的全屏窗口之上 | 置顶的工具窗口 | X11 上置顶的实用工具窗口；在支持的 Wayland 合成器上使用层表面（layer surface，需要 `libgtk-layer-shell.so.0`） |
| 快捷键 | Carbon 热键 | `RegisterHotKey` | X11 抓取，Wayland 下是 XDG desktop portal |
| 应用 | 各 Applications 文件夹 | 开始菜单的“所有应用” | 桌面条目 |

在 Wayland 上，使用层表面的面板与其他启动器一样，在显示期间始终保持键盘焦点，只在隐藏时失焦，因此应使用 Escape 关闭。
