# 应用

`mygo.App` 管理应用的生命周期、身份信息，以及与操作系统的交互。

## 主函数

```go
func main() {
	mygo.Bind(&Notes{})                // services the frontend calls
	mygo.App.WhenReady(func() {        // once the app has launched
		mygo.NewWindow(mygo.WindowOptions{URL: "/"})
	})
	if err := mygo.App.Run(); err != nil { // the event loop, until the app quits
		log.Fatal(err)
	}
}
```

`App.Run` 运行原生事件循环，并在应用退出时返回。它必须从主 goroutine 调用，因为每个平台的 UI 工具包都必须运行在启动进程的线程上；MyGo 把主 goroutine 保持在那个线程上。窗口只在应用就绪后存在：在 `WhenReady` 中创建它们，或在那之后从任意 goroutine 创建。

在 `Run` 之前，`main` 会准备好应用：绑定服务、处理协议、添加监听器，并做 `App.SetName`、`App.SetMenu`、`App.Dock.SetMenu`、`Theme.SetSource` 和 `Theme.SetGTKCSS` 等设置，它们在应用启动时生效。它也可以读取不依赖运行中应用的内容，如 `App.Path`、`App.Locale` 或 `Power.IsOnBattery`，并向系统注册应用（`App.SetOpenAtLogin`、`App.RegisterURLScheme`）。其他一切都需要运行中的应用：如果 `main` 在 `Run` 之前调用剪贴板、显示器、对话框、通知、全局快捷键、托盘、Dock 徽标、`Theme.IsDark`、`Theme.Preferences` 等，它们在每个平台上都会 panic，并在消息中指明是哪个调用。从 `WhenReady` 起使用它们；其他 goroutine 可以更早调用，并等待应用启动。

## 线程

MyGo 的每个函数和方法都可从任意 goroutine 安全调用：在主线程之外发起的调用会被发送到主线程，并等待结果。各部分在哪个线程上运行：

- 事件监听器（如 `OnClose` 或 `OnActivate`）、菜单项点击和全局快捷键的回调在主线程上运行，主线程也负责绘制 UI：回调应尽快返回，耗时任务另起 goroutine。
- 绑定方法在自己的 goroutine 上运行，每次调用一个：它们可以阻塞。
- `mygo.RunOnMain(fn)` 在主线程上运行 `fn`，用于通过 `Window.NativeHandle` 调用的原生 API。

## 生命周期

| 事件 | 何时 |
|---|---|
| `OnReady` | 应用已启动；应用已就绪时，`WhenReady` 也会立即运行它的函数 |
| `OnActivate(func(hasVisibleWindows bool))` | 应用再次被激活，例如点击它的 Dock 图标（macOS） |
| `OnDidBecomeActive`、`OnDidResignActive` | 应用成为或不再是活动应用 |
| `OnWindowCreated(func(w *mygo.Window))` | 创建了一个窗口 |
| `OnWindowAllClosed` | 最后一个窗口关闭，除非应用正在退出 |
| `OnBeforeQuit(func(e *mygo.QuitEvent))` | 退出开始，在窗口关闭之前；`e.PreventDefault()` 取消它，除非操作系统结束会话（`e.SessionEnding`） |
| `OnWillQuit(func(e *mygo.QuitEvent))` | 每个窗口都已关闭，应用即将退出；仍可取消，除非操作系统结束会话 |
| `OnQuitCanceled` | 已开始的退出被监听器或窗口取消；应用继续运行 |
| `OnQuit` | 事件循环已停止，就在 `Run` 返回之前 |

每个都返回一个移除监听器的函数。

### 退出

最后一个窗口关闭时，应用会退出，除非 `OnWindowAllClosed` 有监听器。在 macOS 上，应用通常带着 Dock 图标继续运行，用户点击图标时再打开窗口：

```go
mygo.App.OnWindowAllClosed(func() {
	if runtime.GOOS != "darwin" {
		mygo.App.Quit()
	}
})
mygo.App.OnActivate(func(hasVisibleWindows bool) {
	if !hasVisibleWindows {
		openMainWindow()
	}
})
```

- `App.Quit()` 关闭每个窗口（`OnClose` 监听器可以拒绝），然后退出；`OnBeforeQuit` 和 `OnWillQuit` 监听器也可以取消它。Cmd+Q、退出菜单项和终端中的 Ctrl+C（SIGINT 和 SIGTERM）以同样的方式退出；第二次信号会立即退出。
- 任何监听器或窗口取消退出时，都会触发 `OnQuitCanceled`：如果应用在 `OnBeforeQuit` 中设置了退出状态，就应在这里清除，否则应用继续运行时仍会保留该状态。在 `OnWillQuit` 阶段取消退出，不会重新打开已经关闭的窗口。
- 在 Windows 和 macOS 上，操作系统结束会话（注销、重启、关机）时，仍会依次运行退出监听器和窗口的 `OnClose`，然后运行 `OnQuit`，但这些回调无法取消退出：`QuitEvent` 的 `SessionEnding` 已设置，进程可能在这些回调和 `OnQuit` 返回后立即结束。必须保留的数据应在回调中同步保存，不要弹出对话框，也不要依赖需要等待的 goroutine。如果退出流程已运行过这些回调，则只完成剩余流程。Linux 不会向 MyGo 通知会话结束：关机或终止用户进程的注销会发送 SIGTERM，按与 `App.Quit()` 相同的流程退出。
- `App.Exit(code)` 立即退出，不触发事件。
- `App.Relaunch()` 退出，然后用相同的参数再次启动应用，例如在更改了需要重启的设置之后。`App.Relaunch(args...)` 改为传入其他参数，替换应用启动时的参数，让新实例知道它为什么启动，而不用依赖两者共享的文件：

  ```go
  mygo.App.Relaunch("--safe-mode")
  ```

  它们会在 macOS（通过 Launch Services，更新后也一样）、Linux 和 Windows 上，以及 `mygo dev` 下到达新实例。在浏览器中，`Relaunch` 只会退出。

## 单一实例

大多数桌面应用同一时间只运行一个实例，再次启动时会将已有实例切到前台。在第二个实例中，`RequestSingleInstanceLock` 会先把命令行参数传给第一个实例，然后返回 false：

```go
func main() {
	if !mygo.App.RequestSingleInstanceLock() {
		return // the running instance takes over
	}
	mygo.App.OnSecondInstance(func(args []string, workingDir string) {
		if w := mainWindow(); w != nil {
			w.Restore()
			w.Focus()
		}
	})
	// ...
}
```

锁会一直持有到进程退出，因此应用在 `Run` 返回后所做的工作（停止它启动的进程、写入它的文件）不会与新实例同时进行。从退出开始，实例不再接收命令行：在此期间的启动（例如 `Relaunch` 或更新发起的启动）会等待它退出（无论需要多久），然后再运行。进程结束时系统会释放锁，进程崩溃或被终止时也是如此，应用启动的进程不会继续持有它。

## 深度链接

应用可以打开使用自身协议的 URL，如 `myapp://open?note=42`。在 mygo.config.ts 中列出这些协议，安装应用时会将它们注册到系统：

```ts
export default defineConfig({
  urlSchemes: ["myapp"],
});
```

并在 `Run` 之前注册 `OnOpenURL` 来处理 URL，这样它也能收到应用启动时带的 URL：

```go
mygo.App.OnOpenURL(func(rawURL string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	openNote(u.Query().Get("note"))
})
```

在 Windows 和 Linux 上，打开 URL 会启动应用的新实例：请求[单一实例锁](#单一实例)后，后续实例收到的 URL 会传给第一个实例的 `OnOpenURL`。没有通过 `mygo build` 生成的安装包安装的应用，例如便携式可执行文件，可在运行时用 `App.RegisterURLScheme("myapp")` 注册协议；每次启动时调用的开销很小，还可配合 `UnregisterURLScheme` 和 `IsURLSchemeRegistered` 使用。在 macOS 上，协议必须在配置中列出。

## 文件关联

应用从文件管理器打开其类型的文件。在 mygo.config.ts 中声明这些类型：

```ts
export default defineConfig({
  fileAssociations: [
    {
      ext: ["md", "markdown"],
      name: "Markdown Document",
      mimeType: "text/markdown",
    },
  ],
});
```

并在 `Run` 之前注册 `OnOpenFile` 来打开它们，这样它也能收到应用启动时带的文件：

```go
mygo.App.OnOpenFile(func(path string) {
	openDocument(path)
})
```

它也会收到拖放到应用 Dock 图标上的文件（macOS），以及命令行传入的文件。与深度链接一样，单一实例锁让第一个实例在 Windows 和 Linux 上收到后续实例的文件。

每个窗口各显示一个文件的应用，可以通过[文稿](documents.md)功能实现上述行为，以及文稿类应用的其他功能。

## 最近使用的文稿

记录用户打开的文稿，让应用和系统再次提供它们：

```go
if err := mygo.App.AddRecentDocument(path); err != nil {
	log.Println(err)
}
```

`App.RecentDocuments()` 返回它们，最新的在前，最多十个，也包括更早运行时的那些，但不包含已不存在的文件；“打开最近使用”菜单（`RoleOpenRecent`，见[菜单](menus.md)）列出它们。macOS 也会在应用的 Dock 菜单中列出它们，Linux 在用户最近使用的文件（`~/.local/share/recently-used.xbel`，文件选择器和文件管理器的“最近使用”）中列出，Windows 在应用[跳转列表](#跳转列表)的“最近”类别中列出，前提是应用注册了打开这些文件类型（见[文件关联](#文件关联)）。在“打开最近使用”菜单、Dock 菜单或跳转列表中选择一项，会用应用打开该文件，应用通过 `OnOpenFile` 收到它。[文稿](documents.md)记录它们打开和保存的文件，并自己打开选中的那些；其他应用，例如打开文件夹的应用，记录它们打开的内容。

`ClearRecentDocuments` 清空最近使用记录，例如用户要求清除应用历史时调用，与“打开最近使用”菜单中的“清除菜单”作用相同。在 Linux 上，不影响用户其他文件的最近使用记录。

## 登录时启动

```go
if err := mygo.App.SetOpenAtLogin(true); err != nil {
	log.Println(err)
}
enabled := mygo.App.OpenAtLogin()
```

把它作为设置提供。应用在 macOS 13 及更高版本上成为用户的登录项（这需要打包后的应用），在 macOS 12 上成为启动代理，在 Linux 上成为 XDG 自启动条目，在 Windows 上成为用户的 Run 条目，系统设置会列出它并可以关闭它。`WasOpenedAtLogin` 区分是否在登录时启动，以便决定在后台启动：

```go
mygo.NewWindow(mygo.WindowOptions{URL: "/", Hidden: mygo.App.WasOpenedAtLogin()})
```

在 Windows 上，Run 条目以用户的普通（未提权）令牌启动应用，即使用户是管理员：应用无法创建网络适配器、更改系统代理或写入受保护的文件夹。需要管理员权限的应用改为在任务计划程序中注册登录任务，运行级别为 `HighestAvailable`（`schtasks /Create /SC ONLOGON /RL HIGHEST`，或任务计划程序 API），这会在不询问的情况下以提权方式启动它，并且不调用 `SetOpenAtLogin`。创建任务本身需要管理员权限，所以应用在用户打开该设置时请求一次。`WasOpenedAtLogin` 不能区分这种启动：为此给任务的命令加一个应用自己的参数。

## 目录

`App.Path` 返回标准目录：

| 名称 | 目录 |
|---|---|
| `PathUserData` | 应用的数据：`~/Library/Application Support/<Name>`、`~/.config/<Name>`（`$XDG_CONFIG_HOME`）或 `%AppData%\<Name>`；首次使用时创建 |
| `PathCache` | 应用的缓存：`~/Library/Caches/<Name>`、`~/.cache/<Name>` 或 `%LocalAppData%\<Name>`；首次使用时创建 |
| `PathLogs` | 写入日志的位置：`~/Library/Logs/<Name>`，否则用户数据目录中的 `logs`；首次使用时创建 |
| `PathResources` | 应用随附的文件，见[资源](distribution.md#资源) |
| `PathAppData` | 每用户配置目录，不带应用名称 |
| `PathHome`、`PathTemp`、`PathExe` | 主目录、临时目录、可执行文件 |
| `PathDesktop`、`PathDocuments`、`PathDownloads`、`PathMusic`、`PathPictures`、`PathVideos` | 用户的文件夹 |

```go
dir, err := mygo.App.Path(mygo.PathUserData)
db := filepath.Join(dir, "notes.db")
```

`App.SetPath` 覆盖某个目录，例如把测试数据分开存放。

## 崩溃

没有被 recover 的 panic 或其他致命错误会结束应用，Go 运行时把经过连同各 goroutine 的栈写到标准错误。打包后的应用的标准错误哪里也不去：macOS 给从访达打开的应用 `/dev/null`，Windows 不给 GUI 应用标准错误。所以 `App.Run` 让运行时同时写到 `PathLogs` 中的 `crash.log`，每次崩溃接在之前的后面；日志超过 1 MB 时，应用启动时从头开始。`mygo dev`、`go run` 和 `go test` 运行的构建会显示标准错误，不写崩溃日志。要把崩溃记到别处，在应用就绪后（`App.WhenReady`）调用 `debug.SetCrashOutput`。

在手机上，`mygo dev` 的控制台会显示崩溃：iOS 上和 Xcode 显示的一样，Android 上来自系统日志，运行时以标签 `Go` 写在那里。

## 名称与版本

打包后的应用从配置得知自己的名称和版本，`mygo build` 把它们链接进去：`App.Name()` 和 `App.Version()`。用 `go run` 运行的程序具有可执行文件的名称，没有版本，除非它在 `Run` 之前调用 `App.SetName` 和 `App.SetVersion`。名称决定用户数据、缓存和日志目录以及单一实例锁。`mygo dev` 的开发构建命名为 `<Name> Dev`，以便它们的数据与已安装应用分开；调用 `SetName` 会把它们重新关联到一起。

`mygo.IsDev()` 报告应用是否为开发构建：`go run`、`go build` 和 `mygo dev` 的构建是，`mygo build` 的构建不是，`MYGO_ENV=production` 让任何构建表现得像生产构建。`App.IsPackaged()` 报告它是否从应用包运行。`App.Locale()` 返回应用显示的语言，例如 `"zh-Hans"`，取自应用所用的语言，`App.PreferredLanguages()` 返回用户阅读的语言，按系统列出的顺序，`App.ChosenLocale()` 返回 `App.SetLocale` 选择的语言，应用跟随用户时为 `""`；见[应用显示的语言](i18n.md#应用显示的语言)。0.5 之前，`App.Locale()` 返回用户的首选语言，无论应用用什么语言写成。

## Dock 与任务栏

- `App.SetBadgeCount(n)` 在应用图标上显示计数：Dock（macOS）或启动器条目（支持它的 Linux Dock）；零会清除它。
- `App.Dock` 控制 macOS 上的 Dock 图标：`SetBadge` 显示任意文本，`Bounce` 请求注意，`SetIcon` 替换图标，`SetMenu` 向它的菜单添加项（见 [Dock 菜单](menus.md#dock-菜单)），`Hide` 和 `Show` 移除并恢复它。
- `App.SetActivationPolicy(mygo.ActivationPolicyAccessory)` 移除驻留菜单栏的应用的 Dock 图标，见[托盘图标](menus.md#托盘图标)；它的窗口仍可显示。`App.ActivationPolicy` 返回该策略。
- `App.Focus` 把应用带到前面，`App.IsActive` 报告它是否在前面：活动应用，或在 Windows 和 Linux 上拥有焦点窗口的应用。在 macOS 上，`App.Hide` 和 `App.Show` 隐藏并显示它的所有窗口，`App.ShowAboutPanel` 显示标准的关于面板。

### 跳转列表

在 Windows 上，右键点击应用的任务栏按钮会显示它的跳转列表：用户最近打开的文稿（见[最近使用的文稿](#最近使用的文稿)）和任务，即带参数运行的程序，通常是应用自身，例如文本编辑器的“新建笔记”。`App.JumpList.SetTasks` 设置任务；Windows 在应用退出后保留它们，所以启动时设置一次就够：

```go
mygo.App.OnOpenURL(func(rawURL string) {
	if rawURL == "myapp://new-note" {
		newNote()
	}
})
// ... once the app is ready:
if err := mygo.App.JumpList.SetTasks([]mygo.JumpListTask{
	{Title: "New note", Description: "Write a new note", Args: []string{"myapp://new-note"}},
	{Title: "Notepad", Path: `C:\Windows\notepad.exe`},
}); err != nil {
	log.Println(err)
}
```

运行应用的任务会用 `Args` 再次启动它：应用未运行时，新实例收到这些参数；应用正在运行时，如果用了[单一实例](#单一实例)，则由运行中的实例收到。使用应用已注册[协议](#深度链接)的 URL 在两种情况下都会传给 `OnOpenURL`，已关联类型的文件则传给 `OnOpenFile`，所以任务最好使用这两种参数。其他参数会传入新实例的 `os.Args` 或运行中实例的 `OnSecondInstance`：两处都需要处理。每个参数均按原样传递，MyGo 会为命令行添加所需的引号。

除非另行设置，`Path` 是应用自己的可执行文件，`WorkingDir` 是它运行的目录，`Description` 是工具提示，图标是程序自己的，除非 `IconPath`（连同 `IconIndex`）指定另一个。不设置任何任务则会移除它们。应用更改跳转列表的一部分时，Windows 会替换整个跳转列表，所以 `App.JumpList` 保留各部分并一起提交。它的方法在 macOS 和 Linux 上不做任何事。

## 在 Linux 上绘制网页

WebKitGTK 在可能时用 DMA-BUF 渲染器在 GPU 上绘制页面，而该渲染器在 NVIDIA 驱动、虚拟机和容器中会显示空白窗口。所以 MyGo 默认关闭它：在 GTK 启动之前，`App.Run` 设置 `WEBKIT_DISABLE_DMABUF_RENDERER=1`，除非环境已有该变量。页面随后在 CPU 上绘制，这在渲染器本可工作的地方会损失帧率：在 Intel GPU 的 Wayland 桌面上，长页面（4,600 个元素的聊天记录）以每秒 11–19 帧滚动，一个 CPU 核心满载，而使用渲染器时为 56 帧，CPU 占用减半。

如果应用的运行环境已知，可以在 `App.Run` 之前设置为保留渲染器：

```go
mygo.App.SetDMABUFRenderer(mygo.DMABUFRendererAuto)
```

| 设置 | 渲染器 |
|---|---|
| `DMABUFRendererOff`（默认） | 关闭 |
| `DMABUFRendererAuto` | 在已知主机能用它绘制的地方开启，其他地方关闭 |
| `DMABUFRendererOn` | 交给 WebKitGTK，它在能用的地方都使用 |

当 GTK 将打开 Wayland 显示（`WAYLAND_DISPLAY` 指定一个存在的套接字，且 `GDK_BACKEND` 没有把另一个后端放在前面）并且系统有带 Mesa 驱动的 GPU（驱动带 3D 的渲染节点）时，`DMABUFRendererAuto` 会保持它开启：驱动不是 NVIDIA 的（`/sys/module/nvidia`、`/proc/driver/nvidia/version`），不是软件 OpenGL（`LIBGL_ALWAYS_SOFTWARE`、`GALLIUM_DRIVER=llvmpipe`），也不在容器（`/.dockerenv`、`/run/.containerenv`）、WSL 或虚拟机（固件的 `product_name` 或 `sys_vendor` 指明 VirtualBox、VMware、QEMU、KVM、Hyper-V 等）中。只在了解这些机器的情况下使用 `DMABUFRendererOn`，例如在自助终端中。

用户设置的 `WEBKIT_DISABLE_DMABUF_RENDERER` 总是优先，无论应用选择了什么：`0` 开启渲染器，`1` 关闭。其他系统忽略该设置。

## 浏览数据

`App.ClearBrowsingData()` 删除应用页面存储的内容：Cookie、本地和会话存储、IndexedDB、Service Worker 和缓存，例如用户退出登录时。打开的页面会把内存中的内容保留到重新加载。它清除默认会话；`Session.ClearData` 清除另一个会话，或只清除某些类型的数据。

### Cookie

`mygo.Cookies` 读取和更改应用页面的 Cookie，包括 `HttpOnly` 的，即默认的 Cookie 存储——所有窗口共享它，系统 webview 在多次运行之间保留它。例如，在第一页加载之前用后端获得的令牌让页面登录，或读取页面登录时设置的会话：

```go
err := mygo.Cookies.Set(mygo.Cookie{
	Name:     "session",
	Value:    token,
	Domain:   "example.com",
	Secure:   true,
	HTTPOnly: true,
	Expires:  time.Now().Add(30 * 24 * time.Hour),
})

// The cookies a request to the URL sends.
sent, err := mygo.Cookies.List(mygo.CookieFilter{URL: "https://example.com/account"})
// Those named session of example.com and its subdomains.
sessions, err := mygo.Cookies.List(mygo.CookieFilter{Name: "session", Domain: "example.com"})
// Every cookie, sorted by domain, path and name.
all, err := mygo.Cookies.List(mygo.CookieFilter{})

err = mygo.Cookies.Delete(mygo.Cookie{Name: "session", Domain: "example.com"})
err = mygo.Cookies.Clear() // every cookie, but not the pages' storage
```

Cookie 的名称、域和路径标识它：`Set` 替换同名、同域、同路径的 Cookie，`Delete` 删除它，无论其他字段如何，所以它能删除 `List` 返回的 Cookie。不带前导点的域属于某个主机，只发送给该主机；带前导点（`.example.com`）时，Cookie 也发送给它的子域。域是 ASCII（其他用 punycode），空路径是 `/`。空的 `SameSite` 是 `Lax`，`None` 需要 `Secure`。名称和值接受浏览器接受的内容：除控制字符、`;` 和两端空格之外的任何内容，名称不能有 `=`。零值 `Expires` 让 Cookie 持续到应用退出，过去的 `Expires` 会删除它。

过滤器的字段缩小列表范围：`URL` 是一个 http 或 https URL，保留向它发出的请求会携带的 Cookie，尽可能由引擎匹配（在 macOS 上按 RFC 6265 的域、路径和 `Secure`）；`Name` 保留某个名称的；`Domain` 保留某个域及其子域的，带不带前导点都可以；`Path` 只保留路径完全相同的那些。

应用运行后，这些方法可从任意 goroutine 安全调用，在等待时主线程仍继续处理事件。Cookie 可能携带用户的凭据：与其把模块绑定到页面，不如绑定一个完成页面所需操作的后端方法。

在 Windows 上，WebView2 只能通过 web 视图提供 Cookie：如果没有窗口或原生 UI 中的 web 视图显示页面，这些方法以及 `App.ClearBrowsingData` 会创建隐藏的 web 视图，并启动 WebView2 浏览器进程。窗口创建首个页面的 web 视图时，会等待正在进行的调用完成。页面显示后，或应用不显示页面且最后一次调用结束几秒后，隐藏的 web 视图会关闭，WebView2 进程也随之退出；但如果会话中有仅在 WebView2 进程运行期间保留的 Cookie，隐藏视图就会保持打开，包括 `Expires` 为零值的 Cookie，以及临时会话中的所有 Cookie。WebView2 在调用返回后才应用更改，因此这些方法会等待更改生效；如果两秒内未生效（例如引擎拒绝了某个 Cookie），则返回错误。在 Linux 上，这些方法需要 WebKitGTK 2.42 或更新版本，`Set` 还需要 libsoup 2.70 或更新版本；WebKitGTK 将 Cookie 保存在应用用户数据目录（`~/.config/<Name>`）下的 `cookies.sqlite` 中。

### 会话

每个页面都显示在默认会话中，除非 `PageOptions.Session` 指定另一个：它有自己的 Cookie、本地和会话存储、IndexedDB、缓存和 Service Worker，其他会话的页面看不到它们，就像 Electron 的分区。同一服务的两个账号同时使用、一个私密窗口、一个不能看到用户登录状态的预览，或一个与应用自身存储隔离的第三方页面，各用一个会话：

```go
work := mygo.NewSession("work")        // kept on disk from one run to the next
private := mygo.NewEphemeralSession()  // in memory only

win := mygo.NewWindow(mygo.WindowOptions{
	URL:  "https://mail.example.com",
	Page: mygo.PageOptions{Session: work},
})

list, err := work.Cookies.List(mygo.CookieFilter{})
err = work.Cookies.Set(mygo.Cookie{Name: "session", Value: token, Domain: "example.com"})
err = work.ClearData(mygo.SiteDataAll)                             // signing out
err = work.ClearData(mygo.SiteDataCookies | mygo.SiteDataCaches)  // some kinds
```

`NewSession` 为同一名称返回同一个会话，其数据像默认会话一样留在磁盘上。`NewEphemeralSession` 每次都返回新会话，不向磁盘写入任何内容：最后一个使用它的页面关闭时，它的数据消失，下一个页面从空开始。第一页打开之前设置的 Cookie 会进入该页面。`mygo.Cookies` 和 `App.ClearBrowsingData` 属于默认会话，`mygo.DefaultSession()` 也返回它。页面用 `window.open` 或 `target="_blank"` 链接打开的窗口位于打开者的会话中，无论打开处理器返回的选项中的 `Session` 是什么。自定义协议（`mygo.Protocol`）为每个会话的页面提供服务，[原生 UI](ui/web-view.md) 中的 web 视图也在其 `PageOptions` 中接受一个会话。

`ClearData` 接受 `SiteDataCookies`、`SiteDataLocalStorage`（连同会话存储）、`SiteDataIndexedDB`（连同 WebSQL）、`SiteDataCaches`（HTTP 缓存和 Cache Storage）、`SiteDataServiceWorkers`，或 `SiteDataAll`：引擎为站点保留的所有数据，包括这些类型和它拥有的其他类型。

| | 持久会话 | 临时会话 |
|---|---|---|
| macOS | 从 macOS 14 起，`WKWebsiteDataStore dataStoreForIdentifier:`，使用由名称生成的 UUID；之前是持续到应用退出的内存存储 | `nonPersistentDataStore` |
| Linux | 自己的 web 上下文，数据在用户数据目录的 `Sessions/<UUID>` 中，缓存在缓存目录的对应位置；WebKitGTK 2.16 或更新版本 | 带临时网站数据管理器的 web 上下文 |
| Windows | WebView2 配置文件 `mygo-<UUID>`，位于应用的 WebView2 用户数据中；WebView2 Runtime 1.0.1185 或更新版本 | InPrivate 配置文件 `mygo-private-<n>`，WebView2 在最后一个 web 视图关闭时丢弃它 |

在 Windows 上，会话的 Cookie 来自其配置文件的 web 视图，与默认会话一样：在它的某个窗口显示页面之前，这些调用会创建一个隐藏的 web 视图。
