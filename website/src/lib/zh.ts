/**
 * The site's text in Simplified Chinese, by its English: a text without an
 * entry is shown in English, so the docs' pages, written in English, stay so.
 */
export const zh: Record<string, string> = {
  // Header, footer and the controls of every page.
  Docs: "文档",
  Examples: "示例",
  Benchmarks: "基准测试",
  Showcase: "作品展示",
  Sponsors: "赞助商",
  Releases: "发布版本",
  "MIT License": "MIT 许可证",
  "Navigation menu": "导航菜单",
  Language: "语言",
  Auto: "自动",
  Theme: "主题",
  Light: "浅色",
  Dark: "深色",
  System: "系统",
  Close: "关闭",
  Copy: "复制",
  "Search docs…": "搜索文档…",
  "Search docs": "搜索文档",
  "Search the docs": "搜索文档",
  "Search the docs…": "搜索文档…",
  "Find a page or a section of the docs.": "查找文档中的页面或章节。",
  "The search index could not be loaded.": "无法加载搜索索引。",
  "Loading…": "加载中…",
  "No results for “{query}”": "没有找到与“{query}”相关的结果",

  // The docs' pages, and the titles of their sections.
  Menu: "菜单",
  Documentation: "文档",
  "Native UI": "原生 UI",
  Overview: "概览",
  Introduction: "简介",
  More: "更多",
  Previous: "上一页",
  Next: "下一页",
  "Edit this page on GitHub": "在 GitHub 上编辑此页",

  // Pages that do not exist.
  "Page not found": "找不到页面",
  "This page does not exist": "页面不存在",
  "It may have moved. The docs, or their search, will have what you are looking for.": "页面可能换了位置。你可以浏览或搜索文档，查找所需内容。",
  "Read the docs": "阅读文档",
  Home: "首页",

  // The home page.
  "MyGo — Desktop apps in Go, with native UI or a web frontend": "MyGo — 用 Go 构建桌面应用，支持原生 UI 或 Web 前端",
  "Desktop apps in Go.": "用 Go 构建桌面应用。",
  "Tiny, fast, fully typed.": "小巧、快速、一应俱全",
  "One small binary for macOS, Windows and Linux. Native UI or a web frontend in Go, with installers and auto-updates included.":
    "用 Go 构建桌面应用，界面可选原生 UI 或 Web 前端。在 macOS、Windows 和 Linux 上都只需一个小巧的可执行文件，还支持生成安装包和自动更新。",
  "Get started": "开始使用",
  "Become a sponsor": "成为赞助商",
  "Kind of window": "窗口类型",
  "Web frontend": "Web 前端",
  "is generated from main.go, with the types of your structs and the docs of your methods.": "由 main.go 生成，包含你的结构体类型和方法文档。",
  "No webview": "无 webview",
  "The view is a Go function of your app's state. MyGo calls it to build each frame and draws it on the GPU, with Metal, Direct3D or OpenGL: no HTML, no JavaScript, no bindings.":
    "视图用 Go 函数描述，根据应用状态生成界面。MyGo 每帧调用这个函数，再通过 Metal、Direct3D 或 OpenGL 在 GPU 上绘制，无需 HTML、JavaScript 或绑定代码。",
  "Its windows open at once, and an app of native UI alone needs only Go to build and no webview to run.":
    "窗口一启动就能显示。纯原生 UI 应用只需 Go 即可构建，运行时无需 webview。",
  "Start a project": "创建项目",
  Binary: "二进制文件",
  "Memory, native UI": "内存（原生 UI）",
  "Memory, webview included": "内存（含 webview）",
  "CPU at idle": "空闲时 CPU 占用",
  "Web frontends": "Web 前端",
  "Any web stack, in the webview the OS already has: WKWebView, WebKitGTK, WebView2. No browser bundled.":
    "前端技术栈不限，页面在系统自带的 webview 中运行：WKWebView、WebKitGTK、WebView2。无需打包浏览器。",
  "Interfaces written in Go alone and drawn on the GPU: flexbox and grid layout, widgets, text editing, screen reader support. No webview at all.":
    "仅用 Go 编写界面并由 GPU 绘制：支持 flexbox 和 grid 布局、控件、文本编辑与屏幕阅读器。完全不需要 webview。",
  "Typed IPC": "类型安全的 IPC",
  "Pages call bound Go services, stream values through channels and get typed events. The TypeScript client is generated from your Go code.":
    "页面可以调用已绑定的 Go 服务，通过通道流式接收数据，并订阅带类型的事件。TypeScript 客户端直接从你的 Go 代码生成。",
  "Desktop APIs": "桌面 API",
  "Windows, menus, tray, dialogs, notifications, global shortcuts, deep links, file associations and more, for windows of both kinds.":
    "两种窗口都支持窗口管理、菜单、托盘、对话框、通知、全局快捷键、深度链接、文件关联等桌面功能。",
  "Pure Go, no cgo": "纯 Go，无 cgo",
  "Build for every platform from any machine.": "在任意机器上都能交叉编译各平台的应用。",
  "Ready to ship": "从打包到发布",
  "App bundles and disk images, Windows installers, Debian and RPM packages, code signing, notarization and signed auto-updates.":
    "支持生成应用包、磁盘映像、Windows 安装程序、Debian 和 RPM 软件包，并提供代码签名、公证和带签名校验的自动更新。",

  // /benchmarks.
  "Benchmarks · MyGo": "基准测试 · MyGo",
  "The same apps, in four frameworks.": "同一批应用，四种框架。",
  "The same small apps written with MyGo's native UI, GPUI, GPUI Fast and egui, each the usual way of its framework, showing the same text in windows of the same size, and measured on the same Mac.":
    "用 MyGo 的原生 UI、GPUI、GPUI Fast 和 egui 分别编写的同一批小应用。每个应用都采用其框架的常规写法，在相同尺寸的窗口中显示相同的文本，并在同一台 Mac 上测量。",
  "The apps": "应用源码",
  "How they are measured": "测量方法",
  "The data": "数据",
  "Reading the results": "如何解读结果",
  "Shorter bars are better, but for frame rate. Each bar is the median of the runs: hover it for the lowest and the highest.":
    "帧率越高越好，其余指标都是条形越短越好。图中显示多次运行的中位数，将鼠标悬停在条形上可查看最小值和最大值。",
  "Frame rate stops at the display's refresh rate: an app below it missed frames. Time between frames is taken as each frame starts, not as it reaches the screen.":
    "帧率上限取决于显示器刷新率，低于这一上限说明应用出现了掉帧。帧间隔按相邻两帧开始绘制的时间计算，而不是实际显示到屏幕上的时间。",
  "CPU time leaves out what the GPU does: two apps taking the same CPU may not cost the same to draw.":
    "CPU 时间不包含 GPU 的工作耗时：即使两个应用的 CPU 耗时相同，实际绘制开销也可能不同。",
  "The Mac was in ordinary use during the runs. The frameworks took turns, so that this weighs on them alike.":
    "测试期间，这台 Mac 仍在正常使用。各框架交替运行，以尽量均衡日常使用带来的干扰。",
  "CPU per frame has moved by as much as a third between full runs of the same build, more than the range of one run shows: read small gaps as ties.":
    "即使使用同一个构建版本，不同轮完整测试的每帧 CPU 时间也可能相差三分之一，比单轮测试显示的波动范围更大。因此，细微差距应视为没有明显差别。",
  "MyGo's list measures its rows, which may differ in height, where the uniform lists of GPUI and GPUI Fast and egui's rows take one height for all.":
    "MyGo 的列表会测量每一行，行高可能各不相同；而 GPUI、GPUI Fast 和 egui 的列表则让所有行采用统一的行高。",
  "GPUI Fast is a fork of GPUI that keeps what did not change between frames. Its list scrolls from outside the view, as a wheel does, so that its cached rows apply; the other scenes change every frame.":
    "GPUI Fast 是 GPUI 的一个分支，会缓存帧间未变化的内容。测试通过视图外部的操作驱动列表滚动，模拟滚轮输入，让行缓存能够生效；其他测试场景则每帧都会更新内容。",
  "MyGo, GPUI and GPUI Fast draw text with the system's font and egui with its own. The crates of GPUI and GPUI Fast come without widgets, so their buttons are styled elements, and without scroll bars.":
    "MyGo、GPUI 和 GPUI Fast 使用系统字体，egui 使用自带字体。GPUI 和 GPUI Fast 的 crate 不自带控件，因此测试中的按钮由普通元素设置样式实现，列表也没有滚动条。",
  "higher is better": "越高越好",
  "{label}: {low} to {high} over {runs} runs": "{label}：{low} 至 {high}，共 {runs} 次运行",
  Measured: "测量时间",
  Machine: "测试设备",
  "{cpu}, {cpus} CPUs, {memory} GB of memory, {display}": "{cpu}，{cpus} 个 CPU，{memory} GB 内存，{display}",
  Frameworks: "框架",
  Compilers: "编译器",
  Runs: "测试次数",
  "{runs} of each scene, {frames} frames each, frameworks taking turns": "每个场景 {runs} 次，每次 {frames} 帧，各框架轮流运行",
  "Starting and resting": "启动与空闲",
  "A number and three buttons, then nothing to do: what an app costs before it does anything.":
    "界面只有一个数字和三个按钮，显示后保持空闲，用来测量应用尚未执行实际工作时的基础开销。",
  "App size": "应用体积",
  "A release build, with its symbols stripped.": "发布构建，已去除符号信息。",
  Startup: "启动时间",
  "From starting the process until its first frame is drawn and its window is on screen.": "从进程启动，到绘制出第一帧且窗口显示在屏幕上为止。",
  "Memory at rest": "空闲时内存占用",
  "The physical footprint of the process 5 seconds after its first frame.": "首帧之后 5 秒时进程的物理内存占用。",
  "CPU at rest": "空闲时 CPU 占用",
  "The share of a core the process takes over the next 5 seconds, drawing nothing.": "随后 5 秒内进程的 CPU 占用率，以单个核心为基准，期间不进行绘制。",
  "Scrolling a long list": "滚动长列表",
  "A log viewer: 100,000 virtualized rows of three texts each, scrolling 8 DIPs every frame.": "日志查看器：10 万行虚拟化列表，每行三段文本，每帧滚动 8 个 DIP。",
  "A live dashboard": "实时仪表盘",
  "600 cells, each with its own background and a number that changes every frame.": "600 个单元格，每个都有独立背景，显示的数字每帧更新。",
  "Resizing a document": "调整文档大小",
  "40 paragraphs, about 20,000 characters, wrapped again to a width that changes every frame.": "40 个段落，约 20,000 个字符，每帧都按变化的宽度重新换行。",
  "CPU per frame": "每帧 CPU 时间",
  "The time the whole process spent on the CPU, its threads and the graphics driver's, for each frame.": "绘制每帧时整个进程消耗的 CPU 时间，包括应用线程和图形驱动线程的耗时。",
  "Frame rate": "帧率",
  "Frames a second. It stops at the display's refresh rate.": "每秒帧数，以显示器的刷新率为上限。",
  "Slowest frames": "最慢的帧",
  "The 99th percentile of the time between frames: what the worst 1% of frames look like.": "帧间隔的第 99 百分位数，用于反映最慢 1% 帧的耗时。",
  "Peak memory": "峰值内存",
  "The highest physical footprint of the process, as Activity Monitor shows it.": "进程的最高物理内存占用，与“活动监视器”显示的一致。",

  // /showcase.
  "Showcase · MyGo": "作品展示 · MyGo",
  "Built with MyGo.": "使用 MyGo 构建的应用。",
  "Apps the community has made with MyGo, drawing native UI or showing a web frontend.": "社区使用 MyGo 开发的应用，既有原生 UI，也有 Web 前端。",
  "Add your app": "添加你的应用",
  "Built something with MyGo? Open a pull request that adds it to": "用 MyGo 做了应用？请提交一个 Pull Request，将它添加到",
  "in egoist/mygo-showcase": "（位于 egoist/mygo-showcase 仓库）",
  "The apps could not be loaded. See them in": "应用列表加载失败，可以在这里查看：",
  ", with its name, a short description, a link, and whether it draws native UI or shows a web frontend. A screenshot is optional.":
    "，并注明应用名称、简短介绍、链接，以及使用的是原生 UI 还是 Web 前端，也可以附上截图。",
  "By {author}": "作者：{author}",

  // /sponsors.
  "Sponsors · MyGo": "赞助商 · MyGo",
  "Sponsor MyGo.": "赞助 MyGo。",
  "MyGo is free and open source. Sponsors support its development, and their banner is shown in the README and on this website.":
    "MyGo 免费且开源。你的赞助将支持项目持续开发，赞助横幅会展示在 README 和本网站上。",
  "per month": "每月",
  Benefits: "权益",
  "Banner in the README": "README 中的横幅",
  "Your banner in the Sponsored by section at the top of the README.": "你的横幅将显示在 README 顶部的“赞助商”部分。",
  "Banner on the website": "网站上的横幅",
  "Your banner in the Sponsors section of the home page, and on this page.": "你的横幅将显示在首页的“赞助商”部分以及本页面。",
  "Current sponsors": "当前赞助商",
  "To sponsor MyGo at $500 per month, email": "如需每月赞助 MyGo 500 美元，请发送邮件至",

  // The descriptions of the pages, which their heads give to search engines.
  "Build desktop apps in Go. Windows draw native UI that MyGo paints on the GPU, written in Go alone with package ui, or show a web frontend in the system webview that calls Go through a TypeScript client generated from your code. Pure Go with no cgo, and a few megabytes per app.":
    "用 Go 构建桌面应用：使用 ui 包以纯 Go 编写原生 UI，由 MyGo 在 GPU 上绘制；或选择 Web 前端，在系统 webview 中运行，通过自动生成的 TypeScript 客户端调用 Go 代码。无需 cgo，每个应用仅几 MB。",
  "MyGo's native UI, GPUI, GPUI Fast and egui on the same apps: startup, memory, scrolling a long list, a live dashboard and wrapping text, measured on the same machine.":
    "MyGo 的原生 UI、GPUI、GPUI Fast 和 egui 运行同一批应用的对比：启动、内存、滚动长列表、实时仪表盘与文本换行，均在同一台机器上测量。",
  "Apps built with MyGo by the community, with native UI or a web frontend.": "社区使用 MyGo 构建的应用，包括原生 UI 应用和 Web 前端应用。",
  "Sponsor MyGo for $500 per month: your banner in the README and on the website.": "每月 500 美元赞助 MyGo：你的横幅将显示在 README 和网站上。",
}
