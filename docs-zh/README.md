# MyGo 文档

MyGo 让你用 Go 构建桌面应用。每个窗口可以选择以下两种界面，同一个应用也可以混合使用：

- **网页**：显示在操作系统的 webview 中（macOS 上是 WKWebView，Linux 上是 WebKitGTK，Windows 上是 WebView2），而不是打包的浏览器。前端是用任意工具构建的 HTML、CSS 和 JavaScript，通过 MyGo 根据 Go 代码生成的 TypeScript 客户端调用你的 Go 代码；客户端带有 Go 结构体的类型和方法的文档。
- **原生 UI**：由 MyGo 自己在 GPU 上绘制，只用 Go 和 `ui` 包编写，没有 HTML、JavaScript，也没有需要启动的 webview，因此窗口立刻打开，内存占用也很少。

两种窗口共用窗口选项和事件、菜单和桌面 API，整个应用是一个几 MB 的 Go 程序。

一个显示网页、前端调用 Go 服务的窗口：

```go
type Greeter struct{}

// Greet returns a greeting.
func (Greeter) Greet(name string) string { return "Hello, " + name }

func main() {
	mygo.Bind(Greeter{})
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{Title: "Hello", URL: "/"})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
```

```ts
import { Greeter } from "./mygo"; // generated

document.body.textContent = await Greeter.greet("Ada");
```

下面是一个原生 UI 窗口，用 Go 函数根据应用状态生成界面：

```go
type counter struct{ n int }

func (s *counter) view(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Text(c, fmt.Sprint(s.n)).FontSize(40).Bold()
		if ui.PrimaryButton(c, "Increment").Clicked() {
			s.n++
		}
	})
}

func main() {
	s := &counter{}
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:   "Counter",
			Content: ui.View(s.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
```

网页适合展示复杂文稿、复用现有 Web 代码，以及使用浏览器独有的功能；原生 UI 适合各类工具、设置界面和检查器，以及对启动速度要求高的应用。

## 快速开始

- [快速开始](getting-started.md)：安装工具，然后创建、开发和构建应用，可以使用原生 UI 或 Web 前端。

## 构建应用

所有应用共有的部分，无论窗口显示原生 UI 还是网页。

- [窗口](windows.md)：创建和排列两种窗口，它们的事件，以及显示网页的窗口如何处理页面：导航、下载、权限、打印。
- [应用](app.md)：生命周期、退出、单实例、深度链接、文件关联、最近使用的文稿、登录时启动、标准目录、跳转列表、在 Linux 上绘制网页、浏览数据、Cookie 和会话。
- [菜单与托盘](menus.md)：应用菜单、上下文菜单和 Dock 菜单，键盘快捷键和托盘图标。
- [文稿](documents.md)：以文稿为中心的应用，每个文件一个窗口，支持“文件”菜单、自动保存、崩溃后恢复、关闭和退出前询问是否保存，以及处理其他应用对文件的更改。
- [语言](i18n.md)：为应用提供多语言文本，包括 ICU 消息目录、带类型的 Go 和 TypeScript 函数、复数规则、MyGo 内置文本，以及用于测试布局的伪区域设置。
- [桌面 API](native.md)：对话框、通知、剪贴板、shell、显示器、深色模式、电源、网络状态、机密、监视文件和全局快捷键。
- [原生模块](native-modules.md)：用 Swift、Kotlin、Rust 或 Zig 实现 Go 接口的实验性功能，无需 cgo 即可对接 macOS、iOS 和 Android 的框架和视图、Rust crate，以及 Zig 和 C 库；CLI 负责构建、打包到应用中，并发布给使用你的 Go 包的开发者。
- [剪贴板与拖放数据](data-transfer.md)：多种表示形式、文件列表、自定义格式、惰性提供方和剪贴板持久化。
- [音频](audio.md)：音效、流式音乐和应用自己合成的声音，在 Go 中混音，支持 WAV、Ogg Vorbis 和 MP3。
- [启动器](launchers.md)：像 Spotlight 和 Raycast 那样的应用，用快捷键在任何应用之上唤出，用来查找并打开已安装的应用。

## 原生 UI

[原生 UI](ui/README.md) 有独立的文档，包括视图、布局、输入、导航和辅助功能指南；从按钮到表格，每种组件都有单独的页面。

## 网页

- [前端](frontend.md)：页面在开发时和构建出的应用中如何加载、`mygo-runtime` 包、自定义协议、自定义标题栏和拖放的文件。
- [从前端调用 Go](bindings.md)：绑定的服务、通道、类型化的事件和生成的 TypeScript 客户端。
- [插件](plugins.md)：以 Go 包和 npm 包形式提供的功能，例如官方插件，以及如何编写你自己的插件。

## 手机和平板

- [手机和平板](mobile/README.md)：同一个应用在 iOS 和 Android 上运行，需要什么，在模拟器或设备上开发，以及应用在手机上的行为。
- [为手机构建和发布](mobile/building.md)：签名的 `.ipa`、`.apk` 和 `.aab` 文件，App Store Connect 和 Google Play，链接、图标和启动画面。
- [手机上的界面](mobile/interface.md)：系统的标签页和栈、手指、键盘、手机的控件，以及测试。
- [设备 API 和插件](mobile/plugins.md)：触感反馈、分享、电池，地图、相机、定位等官方插件，以及用 Swift 和 Kotlin 编写你自己的插件。

## 发布

- [构建与分发](distribution.md)：为 macOS、Windows 和 Linux 打包应用、签名、安装程序和磁盘映像。
- [自动更新](updates.md)：来自 GitHub 发布版本、S3 存储桶、你自己的服务器或应用自己的更新服务的签名更新，增量更新，以及类似 Sparkle 的更新窗口。
- [GitHub Actions](github-actions.md)：推送标签时，构建、签名并公证各平台应用，并连同更新一起发布的工作流。

## 官方插件

- [Fetch](plugins/fetch.md)：从 Go 发出 HTTP 请求的 `fetch`，没有 CORS 限制，可带任意请求头，支持流式响应体和取消。
- [WebSocket](plugins/websocket.md)：由 Go 建立连接的 `WebSocket`，握手时可带请求头。
- [SQLite](plugins/sqlite.md)：本地数据库，带参数化查询和原子事务，无需 cgo。
- [文件监视](plugins/watch.md)：应用允许页面监视的目录中的文件变化。
- [更新器](plugins/updater.md)：类似 Sparkle 的更新窗口，检查更新并提示安装。
- [终端](plugins/terminal.md)：原生 UI 的终端，用 Ghostty 的终端模拟器运行 shell 或任意程序。
- [玻璃](plugins/glass.md)：原生 UI 的 Liquid Glass，在每个平台上都像 macOS 绘制的那样，带 macOS 的滚动边缘和背景模糊。
- [游戏](plugins/game.md)：按需启用的游戏功能，包括固定步长的游戏循环及输入处理、摄像机，以及采用统一标准布局的游戏手柄。
- [Loading](plugins/loading.md)：loading.dev 的 29 种原生 UI 加载指示器。
- [SF Symbols](plugins/sfsymbols.md)：macOS 上用于原生 UI 的 Apple SF Symbols，由系统绘制。
- [自动化](plugins/automation.md)：供端到端测试工具和智能体从其他进程控制运行中的应用，包括窗口、元素、输入、菜单和截图，以及面向智能体的命令行客户端 `mygo-automation`。
- [PermissionFlow](plugins/permissionflow.md)：检查并请求需要用户授权的系统权限，例如“辅助功能”和“屏幕录制”，并引导用户前往“系统设置”，将应用拖入对应的权限列表。
- [Render](plugins/render.md)：把原生 UI 做成视频，每一帧在 GPU 上绘制，用系统的编码器编码，并带有声音。

## 参考

- [配置](configuration.md)：`mygo.config.ts`，或 `mygo.json`。
- [mygo CLI](cli.md)：它的命令和标志。
- Go API：`go doc -all github.com/egoist/mygo`，包含每个类型和方法的文档。
- [架构](architecture.md)：MyGo 内部如何工作，面向贡献者，附各平台差异的表格。

## 要求

开发应用需要 [Go](https://go.dev/dl/) 1.27 或更高版本；带 Web 前端的项目还需要前端工具 [Bun](https://bun.sh)，原生 UI 项目只需 Go。MyGo 不使用 cgo，无需安装 C 工具链，在任意机器上都能交叉编译各平台的应用；macOS 应用的签名和磁盘映像制作仍需在 Mac 上完成。

应用运行在：

| 平台 | 带网页的窗口需要 | 原生 UI 的窗口需要 |
|---|---|---|
| macOS 12 或更高版本 | 什么都不需要：WKWebView 是 macOS 的一部分 | 什么都不需要：它们用 Metal 绘制 |
| Linux（x64、arm64） | GTK 3 和 WebKitGTK 4.1（或 4.0）：Debian 和 Ubuntu 上 `libwebkit2gtk-4.1-0`，Fedora 上 `webkit2gtk4.1`，Arch Linux 上 `webkit2gtk-4.1` | 仅 GTK 3 |
| Windows 10 和 11（x64、arm64） | [WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)，Windows 11 已包含 | 什么都不需要：它们用 Direct3D 11 绘制 |

所有窗口都显示原生 UI 的应用既不需要 WebKitGTK，也不需要 WebView2 Runtime。Linux 上的托盘图标还需要 `libayatana-appindicator3`。在 Linux 上，除非应用主动启用，MyGo 会关闭 WebKitGTK 的 DMA-BUF 渲染器，见[在 Linux 上绘制网页](app.md#在-linux-上绘制网页)。

显示网页的应用可以通过 `mygo.WebViewAvailable()` 检查系统是否有可用的 webview，在 `App.Run` 之前也能调用；如果没有，可改为打开原生 UI 窗口，其运行要求见表格最后一列。

运行 `mygo doctor` 可检查开发环境是否就绪。
