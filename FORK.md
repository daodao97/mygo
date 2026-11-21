# Fork 与上游的差异

本仓库是 `egoist/mygo`（上游，remote `upstream`）的 fork，`daodao97/mygo`（remote `origin`；2026-11-09 曾更名为 `daodao97/mygo-mobile`，2026-11-18 改回 `daodao97/mygo`，旧地址重定向）。Go 模块路径仍是 `github.com/egoist/mygo`，不随仓库更名，以便合并上游。本文件记录两者的差异、处理规则和进行中的工作，供维护 fork 的 agent 先读。用文末的命令重新统计差异。

## 快照（2026-11-21）

- fork `main` 已合并 `upstream/main` 到 `2d814ef1`（2026-11-21），含 #940（`6e712c6e`）。
- 分支 `sync-940` 已于 2026-11-18 快进合入 `main`（`964d7b6f`），第二阶段的过程见「第二阶段进度（2026-11-17）」。
- fork 独有 187 个非合并提交，与上游的增删差异约 36,000 行。
- 2026-11-08 同步了上游 26 个提交（`1dfa5a1f..80013ac3`）。冲突只有两处：`go.mod` 取上游新增的 flac/aac 依赖，保留 fork 的 `Heyana/typesetting-go`；`ui/layoutcache_test.go` 的字段清单两边都保留（上游的 `popupSet`、fork 的触摸/输入字段）。上游新增的 `Window.SetInputRegion` 是可选接口 `platform.InputRegionWindow`，iOS/Android 不需要实现；`ListRow.Hovered`/`Pressed`/`PointerPosition`、终端 `Options.OnDrop` 在 fork 中没有对应实现，没有需要按规则 1 处理的重叠。
- 2026-11-09 同步了上游 39 个提交（到 `6a730f12`）。冲突：`docs/architecture.md` 两边都保留（fork 的 iOS 段落、上游的 `ffi`）；`ui/runtime.go` 两边都保留（fork 的帧计时、上游的 `sweepVideos`）；`ui/layoutcache_test.go` 字段清单取并集；`internal/gpu/gl/load_linux.go` 只取上游新增的 `uploadElement`，`setShaderSource`/`shaderLog`/`programLog`/`infoLog` 在 fork 已移到 `load_common.go`（与 Android 共用）；终端 `urlPattern` 按规则 1 改用上游 #1481（URL 在第一个非 ASCII 字符处结束），fork 的写法删除；shader 二进制取上游后重新 `go generate`。上游 #1476 给 `platform.Clipboard.WriteData` 加了 `concealed` 参数，iOS/Android 后端已跟上签名，但还没有标记机密内容（iOS 的 `UIPasteboard` localOnly、Android 的 `EXTRA_IS_SENSITIVE`）。
  合并后对照上游这批新增（`RowClicked`、`WindowDoubleClicked`、`ui/video` 与 `Bitmap.UpdateYUV`、`Power.KeepRunning`、`App.ChosenLocale`、`ClipboardOptions.Concealed`、`ffi`/native modules、文本框的 `ReadOnly`/`Multiline` 无障碍状态、新的 landmark/tabpanel 角色）检查了 fork 新增代码，没有按规则 1 需要删除的重复实现。容易误判的两处：fork 的 `platform` 文本输入状态里的 `ReadOnly`/`Multiline` 是移动端键盘和原生选区的配置，上游 #1493 的是无障碍状态（`AccessReadOnly`/`AccessMultiline`），两者不同层；`ui/text_input_client.go` 的 `TextInputClient` 是上游的，fork 只在其上加了 UTF-16 适配。`desktopcontrol` 用 `purego.Dlopen` 加载的是系统框架，不属于 `ffi` 规定的应用自带库。iOS 还没有为新的 landmark/tabpanel 角色单独映射（落到默认处理）。
- 2026-11-09 第二次同步上游 4 个提交（到 `6dfdff6a`），无冲突；其中 #1530 是 fork 的 PR，按规则 1 以上游为准。
- 2026-11-10 同步上游 56 个提交（到 `a4b6734f`）。冲突 15 处：`app.go` 的监听器字段取并集（fork 的生命周期、上游的 `onQuitCanceled`），`DidBecomeActive` 同时保留上游的 `backgroundShow = false` 和 fork 的 `setLifecycle`；`login.go` 取上游的 `Relaunch(args ...string)`，保留 fork 的 iOS `IsSystemManaged` 判断；`updater.go` 的重启参数按规则 1 改用上游的 `work/relaunch`（删除 fork 把 `os.Args` 传给 helper 的写法），保留 fork 独有的 `keep`（`keepPreviousBundles`）；`internal/gpu/metal` 保留上游的 `MYGO_UNCAPPED`（`setDisplaySyncEnabled:`）和 fork 的 iOS 异步呈现；`ui/framestats.go`、`ui/runtime.go`、`ui/input.go` 两边都保留（fork 的 `FramePhases`、frame-why、触摸仲裁；上游的帧率统计、`askBuild`、悬停只重绘外观）；`ui/context.go`/`ui/paint.go` 保留 fork 的 `uint64` flags 和 router 字段，加上游的 `hoverBuilt`/`hoverLook` 和 `ScrollbarStyle`；终端只加上游的 `barStyle`。richtext 按规则 1 删除 fork 的 `EditorOptions.ContextMenu`，改用上游的 `Menu`（Miya `conversation_view.go` 已改）；fork 的 `OnInput`（所有输入）和上游的 `OnKey`（只有按键）并存，`OnInput` 先问。上游给 `platform.Shell` 加的 `PreviewItem`（Quick Look，#1561）在 Android 返回 unsupported。
- 2026-11-10 第二次同步上游 13 个提交（到 `e98410d0`）。冲突 3 处：`README.md` 取 fork 的（上游只在 fork README 没有的功能列表里加了 RPM）；`cmd/mygo/config_test.go`、`go.sum` 取并集（fork 的 iOS 配置类型与 `Heyana/typesetting-go`，上游的 `LinuxDepends` 与 rpm 依赖）。上游 #1634 给 `platform.Shell` 加了 `DefaultApp`：Android 补成 unsupported，iOS 由内层后端继承。对照这批新增检查了 fork 代码：`examples/ios-native` 的 `revealEditingField` 处理点按聚焦和键盘/旋转导致的视口变化，正是上游 #1629/#1635 的聚焦滚动明确不管的两种情况，不算重复，保留；`Theme.WithAccent`/`WithPreferences`、`TitleBar.Bottom` 在 fork 中没有对应实现（fork 的 `SafeArea` 是另一个问题）。iOS 还没有把 `TitleBar.Bottom` 用于刘海/灵动岛区域。
- 2026-11-12 同步上游 52 个提交（`e98410d0..0d644f4a`）。冲突 4 个文件：`README.md` 保留 fork 的应用列表（上游增加的是赞助链接）；`ui/theme.go` 保留上游的 `DisableAutoDimming` 和 fork 的 iOS 背景说明；终端 `paint.go`、`view.go` 采用上游 #1687 的行缓存复用，保留 fork 的固定网格、光标跟随和自适应颜色缓存，删掉不再使用的 `searchMatches` 字段。自动合并之外，将新增的 `hiddenScrollbars`/`barFlags` 适配为 fork 的 `uint64` flags，inspector 改读新的滚动条轴状态。用 fork 的生成器重建 `ui/api_gen.go`，迁移表检查通过。
  对照新功能检查了 fork 扩展：`FocusOptions.PreventScroll`、`Element.OnUnmount`、`ListState.Choose`、逐轴滚动条、富文本的 `ScrollBy`/`ScrollToParagraph`/`VisibleParagraphs`/`CaretRect`/`NoScrollbar`、代码编辑器的 `EditRemote`/文本替换显示、Markdown `CodeLink` 均没有 fork 的同功能实现。富文本 `OnInput`/`OnPaste`/`OnCopy` 仍是 fork 扩展。新增的平台契约只有 `WindowHandler` 的客户端证书回调，由核心实现，不要求 iOS/Android 新增后端方法；移动端 webview 尚未接入客户端证书选择。#940 尚未合入；#1525、#1571 经查询仍是未合并的草稿 PR。
  验证：macOS/Linux/Windows 的 `CGO_ENABLED=0 go build ./...` 和 `go vet ./...`、生产标签与 js/wasm vet、Go 全仓库测试、Miya 本地依赖编译、iOS 后端 vet、Android `.so` 构建与 16KB 对齐均通过。另修正 fork 原有的跨平台 E2E 编译问题：将公共测试中的 macOS `waitOnScreen` 移入 Darwin 的 `drag` 钩子，真实 GUI 的 `TestContentWindowTextSelection` 回归通过。本机 Bun 1.2.20 的 Response.clone 测试失败，改用项目 `.bun-version` 锁定的 1.4.4 后，构建、268 项测试和类型检查全部通过，生成的 bridge/keys 文件没有差异。
  发布体积（arm64，`-trimpath -tags mygo_noinspector -ldflags '-s -w -X github.com/egoist/mygo.production=1'`，两边加 `-buildvcs=false` 排除独立基线源码的 VCS 元数据差异）：

  | 平台 | counter-native 同步前 → 后（字节） | hello 同步前 → 后（字节） |
  |---|---:|---:|
  | macOS | 9,954,370 → 9,988,258（+33,888） | 9,783,890 → 9,800,978（+17,088） |
  | Linux | 9,502,983 → 9,502,983（不变） | 9,437,447 → 9,437,447（不变） |
  | Windows | 10,055,168 → 10,066,944（+11,776） | 9,666,560 → 9,679,872（+13,312） |

  增长来自本批上游通用改动，按规则 1 保留上游实现。macOS hello 的 `-dumpdep` 和符号比较确认，新增客户端证书路径由 `window.authChallenge → clientCertificate → keychainIdentities → platform.CertificateIssuer` 链入；语言匹配缓存由 `missingTranslations → i18n.Match → i18n.match` 链入。counter-native 还包含新的 UI 布局、滚动条和生命周期逻辑。未引入 fork 的重依赖或新的共享路径反射查找。
- 2026-11-12 第二次同步上游 32 个提交（`0d644f4a..1c40ba32`），无冲突。上游 `3e3c6534`（Linux 全局快捷键描述）把 `platform.Backend.RegisterHotkey` 改为三个参数（新增 `description`），并给 `AppHandler` 加了 `HotkeyBound`；iOS/Android 没有自己的 `RegisterHotkey`，iOS vet 与 Android `.so` 构建均通过，不需要改动。对照这批新增检查了 fork 代码：上游的 `Handle.SelectAll` 是应用层 API，fork 测试里的 `"selectAll"` 是 iOS 编辑菜单发来的平台 `SurfaceCommand`，两者不同层；TokenField `Query`、`PopoverMenuButtonOptions.Item`、拖拽源消失后保持拖拽、编辑器行样式、Markdown 任务复选框与锚点、SVG `var()`/文本/marker 在 fork 中没有同功能实现；`ui/combobox.go` 的 inline combobox 自动合并后测试通过。
  验证：`go build ./...`、`go vet . ./ui`、`go test . ./ui ./internal/text ./plugins/terminal/...`（2843 项）通过；Miya 通过临时 go.work 指向本地 fork 后 `go build ./...`、`go vet ./...` 通过；`go generate ./ui` 无差异。
- 2026-11-13 同步上游 18 个提交（`1c40ba32..6eaa7fb4`）。冲突 2 处：`ui/element.go` 的 `node` 采用上游 #1728 的字段重排（布局热字段放前面，新增 `special` 标志），保留 fork 的 `uint64` flags 和 `gestureEnabled`/`router`/`routerLevel` 字段；`docs/architecture.md` 保留 fork 的「No cgo on desktop」，加上游关于 macOS `Clipboard.OnChanged` 轮询的例外说明。上游给平台契约加了三个方法：`Window.IgnoresMouseEvents`（`69f0eea4`，窗口创建时即可鼠标穿透）、`Clipboard.WatchChanges`（#342）、`Shell.TrashItemResult`（#1453）。iOS 的 clipboard/shell 内嵌 unsupported 而继承，只补 `window.IgnoresMouseEvents`（返回 false，与空的 `SetIgnoreMouseEvents` 一致）；Android 补 `IgnoresMouseEvents`、`WatchChanges` 和 `TrashItemResult`（后两者返回 `ErrUnsupported`）。Android 还没有接入剪贴板变化通知。对照这批新增检查了 fork 代码：`List` 的 `EndReached`、剪贴板监听、回收站路径在 fork 中没有同功能实现；`special` 只控制 `form`/`colFit`/`list`/`overflow`，这些字段只在上游的构造函数中赋值，且都同时设置了 `special`。
- 2026-11-14 同步上游 38 个提交（`6eaa7fb4..daf9c2e5`）。冲突 5 个文件：`internal/linux/menu.go` 取上游的 `sync.OnceValue`/`indicatorSignal`（删掉 fork 的 `trayActivation`）；`internal/web/web.go` 取上游新的 `NewTray(id string, …)` 签名（#1836，iOS/Android 由内层后端继承，vet 通过），保留 fork 的 `Sharing`/`Speech`/`Permissions`/`Authenticator`/`Mobile`/`SystemManagedLifetime`；终端 `paint.go`、`view.go`、`terminal_test.go`：上游 #1677（折行链接整体悬停、打开）与 fork 的 `linkMatch`（`links.go`，含点按打开、硬换行文件路径、缩进续行）是同一功能，但 fork 的覆盖面更大，保留 fork 的结构，按上游行为补齐两处差异：OSC 8 超链接沿软换行跨行（新增 `view.wraps`），`HoveredLink` 对折行链接返回包住所有行的矩形。上游的 `TestCommandClickWrappedLink` 保留，只把 `linkSpan` 断言改成 `linkMatch` 字段。`ui/api_gen.go` 重新生成无变化，Android `.so` 构建通过。
  验证：macOS/Linux/Windows 的 `go build ./...`，`go vet . ./ui`；`go test . ./ui ./ui/editor ./ui/richtext ./ui/video ./audio ./internal/text` 与 `go test -short ./cmd/mygo` 通过；iOS 后端（Xcode iphoneos SDK）交叉编译、Android `.so` 构建与 16KB 对齐通过；`../Miya` 的 `go build ./...` 通过。
- 2026-11-15 同步上游 15 个提交（`daf9c2e5..3737de09`），无文本冲突，但有一处 git 不提示的重叠：上游 #1870 给 `ListState` 加了 `RowHeight`（Table 每行固定高度），fork 的 `ListState` 早有同名字段（`8b2a42dc`，行等高时未建行的估计高度，`GridView` 自己设置），合并后重复声明编译失败。按规则 1 保留上游字段的位置和说明，删掉 fork 的声明，把估计高度的含义并入上游说明；两种语义一致（Table 设了 `RowHeight` 时每行就是这个高度，用它估计正好准确），`sync` 里 `heights.def` 取 `RowHeight` 的逻辑不变。对照这批新增检查了 fork 代码：`TruncateStart`、`Span.PaddingStart`、popover 复用、路径 mask 图集满时回退到 CPU、Table 行高在 fork 中没有同功能实现；fork 在 `ui/richtext/ime.go` 的 `SetSelection`/`SelectionRectsForRange`（移动端原生选区）与上游 #1859 的空 `SetMarkedText` 修复不重叠。
  验证：macOS/Linux/Windows 的 `CGO_ENABLED=0 go build ./...`，`go vet . ./ui`、`go vet -tags mygo_noinspector ./ui`；`go test . ./ui ./ui/editor ./ui/richtext ./ui/markdown/... ./internal/text ./internal/raster ./plugins/terminal/...`（13,186 项）通过；`go generate ./ui` 无差异；iOS 后端 vet、Android `.so` 构建与 16KB 对齐、`../Miya` 的 `go build ./...` 通过。`go test -short ./cmd/mygo` 唯一的失败 `TestFetchSignedRetry` 是上游 `111c8898` 新加的测试，在干净的 `upstream/main` 上本机同样失败（请求次数比预期多一次，随子用例变化），不是 fork 引入的。
- 2026-11-15 第二次同步上游 8 个提交（`3737de09..13a0b3e7`，含 0.9.1），无冲突。上游给 `platform.Window` 加了 `CornerRadius()`（`6fb5ce9c`，窗口圆角半径）：iOS（`window_stubs.go`）和 Android（`window_gen.go`）补成返回 0，即接口说明的「系统不告诉」。菜单项的 `Badge`/`BadgeAlign`（#738）是 `platform.MenuItem` 的字段，移动端没有用 `MenuItem` 的标签，不需要改。`ui/api_gen.go` 用 fork 的生成器重新生成，`NewRecorder` 的回调改为 nil 时保持 nil（fork 生成器的写法）。对照这批新增检查了 fork 代码：`ui.Recorder`、`plugins/render`、`ui/shader` 的实例绘制、视频 seek、窗口圆角、菜单徽标在 fork 中没有同功能实现；fork 的 `SetBadge` 是应用图标角标，与菜单项徽标不同层。
  验证：macOS/Linux/Windows 的 `CGO_ENABLED=0 go build ./...`，`go vet . ./ui`、`go vet -tags mygo_noinspector ./ui`；`go test` 覆盖根包、`ui`、`ui/editor`、`ui/richtext`、`ui/shader`、`ui/markdown/...`、`internal/text`、`internal/raster`、`internal/scene`、`internal/shader/...`、`internal/shaderir/...`、`internal/platform`、`internal/gpu`、`plugins/render`、`plugins/terminal/...`、`examples/render`，全部通过；`go test -short ./cmd/mygo` 这次全部通过（上次失败的 `TestFetchSignedRetry` 本批没有修改，属于时有时无的失败）；iOS 后端 vet、Android `.so` 构建与 16KB 对齐、`../Miya` 的 `go build ./...` 通过。
- 2026-11-16 同步上游 21 个提交（`13a0b3e7..9db28d4e`）。冲突 5 个文件，都来自上游 #1896/#1907 的多指触点（`Element.Touches`、`Touch`、`platform.TouchDown`/`TouchMove`/`TouchUp`/`TouchCancel`、`SurfaceEvent.Width`/`Height`）：上游新建的 `ui/touch.go`、`ui/touch_test.go` 与 fork 的触摸手势引擎同名（add/add）。两者不是同一功能——上游给视图原始触点（ID、阶段、接触面积），fork 把手指仲裁成滚动、长按选择、收起键盘——都保留：文件名让给上游（以后上游改 `touch.go` 能干净合并），fork 的移到 `ui/touch_engine.go`、`ui/touch_engine_test.go`。`internal/platform/surface.go` 的事件种类取并集（fork 的 `PointerLongPress` 在前，上游的四个 `Touch*` 连续排在后面，`ui/input.go` 按区间判断它们）；`ui/input.go` 先走上游的触点分支（总是返回 false，不影响指针路径），再走 fork 的 `fromWindow` 与手势仲裁；`ui/api_gen.go` 用 fork 的生成器重建（含 `Touches`）。
  对照这批新增检查了 fork 代码：调用的 `AbortSignal` 取消、`ListState.MoveAnimation`、菜单项 `KeepOpen` 与徽标颜色/形状、单实例锁持有到进程退出、`platform.ExitBackend`、半透明 web view 的 `Opacity`、托盘 `SetTitleMonospaced`、`PopoverMenu` 的 Panel/Item 选项在 fork 中没有同功能实现。fork 的 `Gestures`（捏合、旋转、双指平移和点按）是识别好的手势，与上游的原始触点不同层，保留。新的平台契约方法 iOS/Android 都由内层后端继承，不需要补。**缺口**：fork 的 iOS/Android 后端只发 `Pointer*`（`Pointer: PointerTouch`），不发 `TouchDown` 等，所以 `Element.Touches` 在 fork 的手机上暂时拿不到手指；需要时在 `internal/ios/backend.go`、`internal/android` 的触摸入口另外发这四种事件（#940 换后端时再看它是否发）。
  验证：macOS/Linux/Windows 的 `go build ./...`，`go vet . ./ui`；`go test . ./ui ./ui/editor ./ui/richtext ./internal/text ./internal/platform ./internal/linux/... ./internal/windows/... ./plugins/terminal/...`（13,002 项）通过；iOS 后端 vet、Android `.so` 构建与 16KB 对齐、`../Miya` 的 `go build ./...` 通过。
- 2026-11-17 同步上游 18 个提交（`9db28d4e..12e8babd`，含 0.9.2），无冲突。上游 `main` 的下一个提交是 #940（`6e712c6e`，632 个文件，约 5.5 万行）。在临时 worktree 试合并到它，有 65 个文件冲突（55 个 UU、10 个 AA），按决定这次不合，另开分支做第二阶段。平台契约只给 Now Playing 加了 `MediaRaise` 和 `MediaInfo.Artists`，移动端不用补方法。`ui/api_gen.go` 用 fork 的生成器重建，只带进了 PopoverMenu 子菜单指向保持（`menuAimGrace`）的注释。对照这批新增检查了 fork 代码：子菜单指向保持、文本框把单独的 `\r` 当换行、字形偏离基线、Now Playing 多艺术家与 MPRIS Raise、diff 越过末尾滚动、Markdown 增量解析、调用和事件一直等待的修复，在 fork 中都没有同功能实现。
  验证：macOS/Linux/Windows 的 `go build ./...`，`go vet . ./ui`；`go test . ./ui ./ui/editor ./ui/richtext ./ui/markdown/... ./internal/text ./internal/platform ./internal/linux/... ./internal/windows/... ./plugins/terminal/...`（13,163 项）通过；iOS 后端 vet、Android `.so` 构建与 16KB 对齐、`../Miya` 的 `go build ./...` 通过。
- 2026-11-18 第二阶段完成：`sync-940` 快进合入 `main`（`964d7b6f`），随后同步上游 7 个提交（`12e8babd` 之后的 `55b50cfa`），无冲突。上游新增的 `internal/platform/touch.go` 是 Windows 触屏手势（点按、长按右键、拖动滚动、双指缩放，`DragRegionAt`），与 fork 在 `internal/mobile/surface.go` 的手机双指手势分属不同层，不重叠。真机（iPhone 16）上修掉的 #940 问题（编辑菜单弹出键盘、只读文本手柄与菜单位置、编辑菜单不消失与点光标切换、`KeepFocus` 按钮误弹菜单、菜单拷贝后取消只读选区）都直接改在源文件里，可整理为上游 PR。
  验证：macOS/Linux/Windows `CGO_ENABLED=0` 编译与 `go vet ./...`、wasm、iOS/Android 交叉 vet；根包、`ui`、`ui/editor`、`ui/richtext`、`internal/mobile`、`internal/windows`、终端插件、`uimigrate` 测试通过；`go generate ./ui` 无差异。
- 2026-11-18 第二次同步上游 5 个提交（`55b50cfa..e88574c8`，含 0.10.0）。冲突 1 处：`internal/darwin/testhooks.go`，上游 #1946 加了 `TestDropFilesFrom`（拖入点与松手点分开，测试拖到网页视图），fork 加了 `TestDropFileReferences`（Finder 的 file-reference URL）。两者不是同一功能，都保留：内部的 `testDropFiles` 同时带 `overX, overY` 和 `references`，三个导出函数都调它。对照这批新增检查了 fork 代码：#1946 让拖到 native UI 中网页视图的文件交给 WebKit，fork 在 `surface_drag.go` 的改动是 native UI 自己的 `DataDrop` 用 NSURL 解析 file-reference URL，路径不同、不重叠；`Painter.RichText` 缓存（#1979）、canvas 字形保留（#1977）、WebView2 创建与 Linux 资源释放（#1978）在 fork 中没有同功能实现。`go generate ./ui` 无差异。
  验证：macOS/Linux/Windows 的 `go build ./...`，`go vet . ./ui ./internal/darwin ./internal/e2e`、Linux/Windows 后端 vet；`go test . ./ui ./internal/darwin ./internal/gpu/metal` 通过；`../Miya` 的 `go build ./...` 通过。
- 2026-11-18 第三次同步上游 6 个提交（`e88574c8..d8fc2b07`）。冲突 5 个文件：
  - `internal/mobile/wire/gen/spec.go` 两边各加了一个 call，都保留：上游的 `SetFrameRate`（`b528dac4`，iOS display link 按内容要的帧率走，`platform.FrameRateSurface`）在前，fork 的 `SetAppBadge` 排到最后，编号由 86 变为 87。
  - `msgs.go`、`Wire.swift`、`Wire.kt` 由 `go generate ./internal/mobile/wire` 重新生成，没有手工挑选。
  - `ui/touch.go`：上游 #2005（接住惯性的手指过了 slop 继续平移，相隔极近的帧不再结束惯性）和 fork 的 `0c497b7e` 是同一修复，按规则 1 改用上游的写法：按下时 `swallow, caught = true, true`，移动时过了 slop 解除，`flingStep` 在位移为 0 时不更新 `f.at`。删掉 fork 的 `fling.rx/ry` 余量、抬起时的 `caught && !panning` 判断和测试 `TestCatchingAFlingPansOn`，上游的 `TestTouchPansAfterCatchingFling`、`TestTouchFlingSurvivesFrameTooSoon` 覆盖同样的行为。

  对照这批新增检查了 fork 代码：`mygo dev` 重建后窗口留在原位（`window_state_dev.go`，只管没有 `StateKey` 的窗口）与 fork 的 `PersistState`（应用自己的数据）不同层；`ios.extensions`、`android.extractNativeLibs` 是 #940 CLI 的配置，fork 的 iOS 打包已在第二阶段删除；Windows/macOS 的 `ShowInactive` 排到当前应用之后在 fork 中没有对应实现。Android 宿主的 `setFrameRate` 是空操作（上游如此）。
  验证：macOS/Linux/Windows 与 `CGO_ENABLED=0` 的 `go build ./...`；`go vet . ./ui ./internal/mobile/... ./cmd/mygo`；`go test . ./ui ./ui/editor ./ui/richtext ./internal/mobile/... ./internal/platform ./internal/text ./plugins/terminal/...`（13,007 项）与 `go test -short ./cmd/mygo`（287 项）通过；`go generate ./ui` 无差异；iOS（iphoneos SDK）与 Android（NDK 28.2）对 `. ./ui ./internal/mobile/... ./plugins/...` 的 vet、`../Miya` 的 `go build ./...` 通过。
- 2026-11-18 第四次同步上游 20 个提交（`d8fc2b07..400ba3b1`）。冲突 1 处：`ui/layoutcache_test.go` 的字段清单取并集（上游 #1993 的 `stateRead`，fork 的 `touchHold`/`touchPointer`/`keyboardDismiss`）。`go generate ./ui` 用 fork 的生成器重建后，fork 独有的 `Element.Gestures`、`LoopFPS` 也带上了 #1993 的 `markStateRead()`（读每帧状态之后再设 `Key`，Tester 里会 panic），保留。
  对照这批新增检查了 fork 代码：#2040 在 Metal 窗口不可见半秒后归还 drawable，与 fork 的 `Renderer.SetPresentationHook`（`ui.Startup`/`OnFirstFrame` 判断首帧呈现）不是同一功能；`Page.Eval` 在手机上执行语句（`23b3b8d5`）、Android 字体注册（#2012 等）、模拟器按 application identifier 签名都在 #940 的代码里，fork 第二阶段后没有自己的版本；`DottedUnderline` 与 fork 的链接悬停整段下划线（`2ba0a0f9`）不同；render 的多核与静止帧跳过、编辑器剪切末行与横向滚动、diff 查找、WSL 文件夹对话框、日期滚轮的月份本地化、`Tester.AccessInfo.Bounds` 裁剪、布局 item 缓存在 fork 中没有同功能实现。
  验证：macOS/Linux/Windows 与 `CGO_ENABLED=0` 的 `go build ./...`；`go vet . ./ui ./internal/darwin ./internal/mobile/... ./internal/gpu/metal ./internal/text`；`go test . ./ui ./ui/editor ./ui/richtext ./internal/text ./internal/platform ./internal/mobile/... ./internal/darwin ./internal/gpu/metal ./internal/uigen ./plugins/render ./plugins/terminal/...`（13,524 项）与 `go test -short ./cmd/mygo`（288 项）通过；iOS（iphoneos SDK）与 Android（NDK 28.2）对 `. ./ui ./internal/mobile/... ./plugins/...` 的 vet、`../Miya` 的 `go build ./...` 通过。
- 2026-11-20 同步上游 42 个提交（`400ba3b1..7ba76ed9`）。冲突 4 个文件：
  - `plugins/terminal/view.go`：两边都保留。fork 的固定网格先按滚动平移（平移了就吃掉事件），之后才是上游 `4b1aa44e` 的「横向为主的滚动交给外层处理器」，固定网格没有吃掉的横向滚动照上游放出去。
  - `ui/keys.go`：`Cmd` 取上游 #2076 的 `platform.MacKeys`。fork 原来写的是 `GOOS == "darwin" || GOOS == "ios"`，是同一功能，按规则 1 以上游为准。
  - `ui/layoutcache_test.go`：字段清单取上游的（`originX`/`originY`/`originSet` 移到布局读取的字段，新增 `pinned`/`pinWorld`），再加上 fork 的 `gestureEnabled`/`router`/`routerLevel`。
  - `ui/window.go`：上游的 `renewed` 与 `platform.FirstFrameSurface` 是核心告诉 surface「首帧已画」（`SurfaceRenew` 后再告诉一次）；fork 的 `firstFramePresented`/`firstFrameOff` 是 `ui.Startup` 经 `conn.OnFirstFrame` 得知「显示器已呈现首帧」。方向相反，不是同一功能，都保留；`detach` 里两边的清理都做。

  对照这批新增检查了 fork 代码。终端里 iOS 上 Ctrl+V 粘贴的分支是 fork 自己的（`c1b0ee50`），与 #2076 不是同一功能，保留。`internal/accelerator` 的 `darwin || ios` 是上游 #2076 自己的写法。`paths.go` 的 iOS 日志路径是 fork 的，管的是路径，#2076 明确不改。平台契约只新增了可选的 `ThumbBarWindow`、`FrameAheadSurface`，以及 `PageOptions.ZoomControl`/`ZoomChanged`，手机后端不用补方法。崩溃写 `crash.log`、`WasOpenedAtLogin`、编辑器光标下方的面板与光标矩形、行间元素、`ui.Table` 的列虚拟化与冻结列、`SplitGap`、Markdown `LinkColor`、`NumberInput` 外部赋值、Windows 缩略图工具栏，在 fork 中都没有同功能实现。iOS/Android 硬件键盘输入改在 #940 的宿主里，fork 第二阶段后没有自己的版本。`go generate ./ui` 无差异。
  验证：macOS/Linux/Windows 的 `CGO_ENABLED=0 go build ./...`；`go vet . ./ui ./plugins/terminal/... ./internal/mobile/... ./internal/darwin`；`go test . ./ui ./ui/editor ./ui/richtext ./ui/markdown/... ./internal/text ./internal/platform ./internal/mobile/... ./internal/darwin ./internal/uigen ./plugins/terminal/...` 与 `go test -short ./cmd/mygo` 通过；iOS（iphoneos SDK）与 Android（NDK 28.2）对 `. ./ui ./internal/mobile/... ./plugins/...` 的 vet、`../Miya` 的 `go build ./...` 通过。
- 2026-11-20 第二次同步上游 3 个提交（`7ba76ed9..71f6c08b`）。冲突 1 个文件，`plugins/terminal/view.go`，来自上游 #2118（`024dd334`：程序接管鼠标时，横向滚轮作为滚轮的左右键报给程序，否则照旧交给外层处理器）：
  - `view` 的字段：fork 早先把 `scrolled`/`zoomed`/`scrollZoom`/`ticking` 挪到了结构体前面，只在那里加上游新增的 `scrolledX`，不取上游重复的那一组。
  - `scroll()`：fork 的固定网格平移仍在最前（平移了就吃掉事件），之后整段取上游的写法。fork 这边的「横向为主一律 `return false`」本来就是上游 `4b1aa44e` 的代码，被 #2118 替换，不是 fork 的设计。固定网格没有吃掉的横向滚轮现在也会报给接管鼠标的程序。

  对照这批新增检查了 fork 代码：fork 在 `ui/editor` 没有自己的改动（与合并基点无差异），#2105（fitted 查看器的查找与跳转行滚入视野）没有重叠；#2119 只改 Windows 回收站的测试；终端里 fork 没有横向滚轮的另一套实现。平台契约没有变化，手机后端不用补方法。`go generate ./ui` 无差异。
  验证：macOS/Linux/Windows 的 `CGO_ENABLED=0 go build ./...`；`go vet . ./ui ./ui/editor ./plugins/terminal/...` 与 Windows 的 `go vet ./internal/windows/...`；`go test . ./ui ./ui/editor ./ui/richtext ./internal/text ./internal/platform ./plugins/terminal/...`（13,321 项）通过；iOS（iphoneos SDK）与 Android（NDK 28.2）对 `. ./ui ./internal/mobile/... ./plugins/...` 的 vet、`../Miya` 的 `go build ./...` 通过。
- 2026-11-21 同步上游 18 个提交（`71f6c08b..2d814ef1`）。`internal/platform/platform.go` 同时保留 fork 的 `ApplicationHost` 和上游 #1811 的 `ErrNavigated`。`plugins/terminal/links_test.go` 同时保留 fork 的 `openedTarget` 测试辅助函数和上游 #2120 的 Windows 路径测试，并覆盖 fork 的折行链接适配层如何解析盘符与行列后缀。

  终端单行路径识别直接使用 #2120 的 `pathAt`，硬换行路径由现有适配层处理。`Page.Eval` 保留 fork 的取消测试钩子和上游的导航错误处理。浏览器触摸（#2133）、动画构建期间提前请求下一帧、List/GridView 的 Enter 和 Table 表头键盘操作直接采用上游实现。Android 工具栏的主题和标题布局改在 #940 的宿主中。回收站路径、富文本链接命中、托盘修复、激活点击、可变字体字重、RTL 括号与 unified diff 末行修复均使用上游实现。`go generate ./ui` 验证了生成文件。#1525、#1571 仍待合并。

  macOS、Linux、Windows 的 `CGO_ENABLED=0 go build ./...`，根包、UI 和受影响后端的 vet，20 个受影响包的测试，macOS 的 `TestEvalAcrossNavigation` 和 `TestConsumeActivationClick`，Linux 容器的回收站路径单元测试，iOS 与 Android 的 vet，以及 Miya 引用本地 fork 的 `go build ./...` 均通过。Android 用 counter-native 成功构建了 APK 和 AAB。Windows 后端的测试完成交叉编译。js/wasm vet 与 CI 指定的 Node 测试通过。直接在 Node 运行需要浏览器文本度量的通用用例时，断行和截断用例的失败可在同步前的 `d0feb7d5` 复现。CLI 短测试的 `TestFetchSignedRetry` 也在同步前的基线上复现了请求次数超出预期的失败。

  arm64 发布体积按 `scripts/bench.ts` 的参数测量，另加 `-buildvcs=false`。

  | 平台 | counter-native 同步前 → 后（字节） | hello 同步前 → 后（字节） |
  |---|---:|---:|
  | macOS | 10,176,466 → 10,176,498（+32） | 9,919,602 → 9,919,634（+32） |
  | Linux | 9,634,055 → 9,634,055 | 9,502,983 → 9,502,983 |
  | Windows | 10,245,632 → 10,257,408（+11,776） | 9,778,176 → 9,784,320（+6,144） |

  `-dumpdep` 与 `go tool nm` 显示，macOS 的增量包含上游激活点击和托盘定位的改动。Windows 两个示例都链入新的窗口激活和托盘逻辑。counter-native 还包含 DirectWrite 字重选择与列表快捷键逻辑，hello 则包含回收站路径查询和 DevTools 导航错误解析。

## 规则

1. **同一功能两套实现时，以上游为准。** 删除 fork 的实现，下游改用上游 API。fork 独有的功能（上游没有的）保留。
2. **上游不收 fork 的 iOS/Android 代码。** 上游自己的移动端是 #940（见「向 #940 收敛」），fork 的移动端向它收敛。移动端代码不作为上游 PR 的候选；从混合提交中提取 PR 时要剥离移动端部分。
3. **合并上游时**，同一功能的冲突按规则 1 解决，再迁移调用方。不要因为 fork 先做了就保留 fork 的设计。
4. 桌面构建不使用 cgo（见 `AGENTS.md`）；移动端可以用 cgo。

## 使用方

| 项目 | 依赖方式 | 影响 |
|---|---|---|
| Miya | `go.mod` 固定 fork 的伪版本（`replace github.com/egoist/mygo => github.com/daodao97/mygo <pseudo-version>`）；本地未提交的 `go.work` 用 `replace => ../mygo` | 本地开发时 fork `main` 的改动立即生效，CI 和打包用 `go.mod` 里的伪版本：fork 改动推送后要在 Miya 升级伪版本（`GOWORK=off go mod edit -replace …@<commit> && GOWORK=off go mod tidy`），用 `GOWORK=off go build ./...` 验证。改公开 API 前先编译 Miya。2026-11-10 起固定 `c558a806`（`v0.0.0-20261110103125-c558a8060974`，向 #940 收敛的缺口全部过完：Lifecycle 补 `Power` 的 suspend/resume、`plugins/keyboardbar`），Miya `3b594b5` 已提交，`GOWORK=off` 下编译、`internal/mobile` 测试通过 |
| myclash | `replace => ../mygo` | 同上；`build.noindex/` 下有旧的 mygo 源码备份，`go build ./...` 会报错，编译时排除 |
| gorex、retty、gorex-workflows | 固定 fork 的伪版本（`80d5e6a3`、`d30cf320`） | 终端用的是自己的 `retty/internal/terminal` 副本，不依赖本仓库的 `plugins/terminal`；升级 mygo 时再检查 |

在不切换本仓库分支的情况下，用分支验证下游：

```sh
printf 'go 1.27.3\n\nuse %s\n\nreplace github.com/egoist/mygo => %s\n' "$APP" "$BRANCH_WORKTREE" > /tmp/w.work
(cd "$APP" && GOWORK=/tmp/w.work go build ./...)
```

## 向 #940 收敛（2026-11-10 决定）

上游作者的 #940（"Run apps on iOS and Android"，草稿，分支 `mobile`；#1581 把 `main` 合进它）是同一功能的另一套实现：iOS 和 Android 共用一个后端 `internal/mobile`，Go/Swift/Kotlin 通过生成的二进制协议通信（`internal/mobile/wire/gen/spec.go`），宿主内嵌在 CLI 里（`mygo dev/build/publish -platform ios|android`），需要权限或较大依赖的功能做成插件（`mygo.Native(id).Call`、`PluginView`）。它合进 `main` 后按规则 1 以它为准。在那之前分阶段收敛（2026-11-10 选定）：先保留 fork 的 iOS/Android 后端，把公开 API 改成 #940 的名字和形状，Miya 随之迁移一次；fork 独有的功能改写为插件；#940 合进 `main` 后再换后端。在那之前：

- 不在下表「有对应」的领域给 fork 加新的移动端 API。
- fork 独有、Miya 在用的移动端功能，按 #940 的插件形状（`mygo.Native`）改写为 `plugins/<name>`，以便合并后保留。
- `ui/touch.go`、`ui/touch_text.go`、`ui/gpu_android.go` 两边同名，#940 合并时会直接冲突。（2026-11-16 起 `ui/touch.go` 是上游 `main` 的 `Element.Touches`，fork 的手势引擎在 `ui/touch_engine.go`；#940 的 `touch.go` 届时与上游 `main` 自己的冲突由上游解决，fork 删 `touch_engine.go` 取 #940 的引擎。）

用 worktree `~/.cache/mygo-pr/mobile-940`（`pr/1581`，`git fetch upstream pull/1581/head:refs/remotes/pr/1581`）对 Miya 做类型检查：Miya 用到 136 个 #940 和上游 `main` 都没有的符号，几乎都在 Miya 的 `internal/mobile`、`internal/voiceinput`。

| Miya 用到的 fork API | #940 的对应 | 缺口 |
|---|---|---|
| `ui.SafeArea`、`Tester.SetSafeArea` | `Context.SafeArea()`（返回四个值）、`Tester.SetInsets` | **API 已迁移**（`converge-940`，Miya 用本地的 `safeArea`/`safeInsets`）。语义仍不同：#940 计入键盘、窗口根元素自动内缩；fork 只算系统边框，由 `ExtendIntoSafeArea` 控制，换后端时再对齐 |
| `ui.InputOptions`（`Keyboard`、`Return`、`Capitalization`、`Correction`）、`Element.InputOptions` | `Element.Keyboard`、`ReturnKey`、`Capitalize`、`Autocorrect`、`Autofill` | **已迁移**（`converge-940`）：类型和常量照 #940（整数枚举，`CapitalizeAll` → `CapitalizeCharacters`，`Content*` → `Autofill*`），内部仍转成 fork 的 `platform.TextInputOptions` 字符串；删除 `KeyboardText`、`ReturnContinue`。fork 扩展保留 `KeyboardASCII` 和 `Element.KeyboardDismiss`（#940 没有对应） |
| `ui.InputPointerCancel` | `InputPointerUp` 带 `Canceled`、`PressCanceled` | **已迁移**（规则 1，上游 `main` 已有）：删除 `InputPointerCancel`，fork 触摸手势的 `cancelPointer` 保留原有清理（选区、编辑器、滚动拖动、拖放、hover），向元素发 `InputPointerUp{Canceled: true}`；终端插件和 Miya 改认 `Canceled` |
| `HapticLight` 等、`hapticsModule.Play` | `mygo.Haptics.Light()` 等 | **已迁移**（`converge-940`）；#940 没有 `Soft`/`Rigid`，Miya 用 `Light` 代替 |
| `Biometrics`、`Authentication*` 错误码 | `plugins/biometrics`：`Available`、`Authenticate(ctx, reason)` | **已迁移**：fork 新增 `plugins/biometrics`，API 照 #940（默认允许设备密码回退），暂时跑在根包 `mygo.Biometrics` 上；细分错误作为 fork 扩展（`ErrUnavailable`、`ErrNotEnrolled`、`ErrLockedOut`、`ErrPasscodeNotSet`、`ErrBusy`、`ErrInactive`、`ErrMissingPurpose`），同时包着系统的 `AuthenticationError`。根包的 `Biometrics` 换后端时删除 |
| `RegisterPushNotifications`、`OnPushToken`、`OnNotification`、`OnPushRegistrationError` | `plugins/push`：`Register`、`OnToken`、`OnMessage`、`OnOpened` | **已迁移**：本地通知的点击改用上游已有的 `App.OnNotificationClick(id)`（规则 1），Miya 把跳转目标编码进通知 ID；远程推送走 fork 新增的 `plugins/push`（API 照 #940），前台展示策略作为 fork 扩展 `push.SetPresentation`。根包删除 `OnNotification`、`NotificationEvent`、`SetNotificationPresentationHandler`、`OnPushToken`、`OnPushRegistrationError`；`App.RegisterPushNotifications` 标为 Deprecated，只给插件调用（未使用的应用由链接器删掉）。插件经 `internal/pushhub` 的钩子接收事件，不导入插件的应用不链接这部分：counter-native 体积不变，hello 小 336 字节。`Configure` 在 fork 的 Android 返回 `ErrUnsupported`（没有 Firebase）；`NotificationOptions.Data` 暂时保留为 fork 扩展 |
| `Permissions.Query/Request` | `plugins/permissionflow`：`Check`、`Request` | **已迁移**：移植 #940 对 permissionflow 的手机端改动（`Photos`、`Contacts`、`Notifications`、`Location`、`OnPhones`），iOS/Android 暂时跑在根包 `mygo.Permissions` 上（`phone.go` 做映射）；`Contacts`、`Bluetooth`、`Calendars` 在 fork 的手机后端返回 `ErrUnsupported`。根包的 `Permissions` 换后端时删除 |
| `Dialog.Photos` | `plugins/imagepicker`：`Pick` | **已迁移**：fork 新增 `plugins/imagepicker`，API 照 #940，暂时跑在 `Dialog.Photos` 上；只支持 `Images`，`Videos`/`Any`/`Capture` 返回 `ErrUnsupported`，`Limit` 之外的副本删除 |
| `Lifecycle`、`OnDidEnterBackground`、`OnWillEnterForeground` | `OnDidResignActive`/`OnDidBecomeActive`、`IsActive`；进出后台是 `Power.OnSuspend`/`OnResume`（`EventBackground` → `PowerEvent`），状态用 `OnSaveState` | **补齐 #940 的形状，fork API 保留**（2026-11-10 决定：Miya、myclash 仍用 `Lifecycle()`）：进入后台在 `OnDidEnterBackground` 之后、checkpoint 之前发 `Power` 的 suspend，回到前台在 `OnWillEnterForeground` 之后发 resume；Android 的 `App.IsActive` 原先恒为 false（`unsupported`），改为 `onResume`～`onPause` 之间为 true。`Lifecycle` 系列是 #940 没有的 fork 扩展，换后端时用这些钩子重建或让调用方改用 `IsActive`/`Power`。`OnSaveState` 未移植（fork 用 `PersistState`） |
| `SetNotificationBadge` | `App.SetBadgeCount`（上游 `main` 已有，规则 1） | **已迁移**：删除 fork 的方法；`SetBadgeCount` 不返回错误，未授权时角标不显示。换到 #940 的 `internal/mobile` 后端后，`appController` 嵌入 `unsupported.App`，iOS 的图标角标一度成了空操作（旧 `internal/ios` 实现过），2026-11-18 补回：wire 调用 `SetAppBadge`（calls 末尾），iOS 宿主用 `UNUserNotificationCenter.setBadgeCount`；Android 宿主为空操作，启动器角标跟随通知，系统没有设数字的接口 |
| `Router.InteractiveBack`、`Element.TouchScroll/TouchSelection/Gestures`、`GestureEvent` | #940 的触摸引擎（`ui/touch.go`、`touch_back.go`、`touch_text.go`） | **已逐项核实（2026-11-10），换后端时整体替换，现在不改**：`TouchScroll` 是 #940 的默认（平移时指下的 `HandleInput` 元素先收到 `Precise` 的 `InputScroll`，静止点按照常按下/抬起），换后端时删调用（Miya 2 处）；`TouchSelection`/`InputLongPress` 在 #940 没有事件，改为轮询 `Element.LongPressed()`，可编辑/可选文本自动长按选词——Miya 的 `conversation_view`、`mobile_voice`（按住说话要长按开始和抬起结束，`LongPressed` 给不了抬起）和 fork 终端插件（#940 的终端没有触摸选择）要重写或保留扩展；`Gestures`（捏合、双指平移）#940 没有，它只跟踪一根手指、协议无手势消息，Miya 远程桌面的捏合缩放需给 #940 加扩展；`InteractiveBack` 在 #940 自动开启（能后退、`Transition == TransitionAuto`、未减弱动态效果），没有开关，Miya 在抽屉页和 `/desktop` 关掉它，需换 `Transition` 或给 #940 加开关 |
| `WindowOptions.ExtendIntoSafeArea` | 无开关：根元素总把安全区当 padding（`ui/runtime.go` 的 `padInsets`，窗口背景填满），贴底的纵向滚动容器延伸到底部安全区下（`extendUnderBottom`） | **已核实，fork 扩展保留**（Miya 的两个窗口在用）。换后端时 Miya 删掉 `ExtendIntoSafeArea` 和自己按 `safeArea` 做的内缩，否则缩两次；画到系统栏下的背景改由窗口背景色给 |

#940 没有对应、要改写为插件的：`InputAccessoryBar`/`InputAction`/`ModifierLatch`（**已改写**为 `plugins/keyboardbar` 的 `Bar`/`Action`/`ModifierLatch`；2026-11-10 决定删掉 iOS 原生的 `inputAccessoryView`：`Element.InputAccessory`、`TextInputState.Accessory`/`AccessoryID`、`SurfaceInputAction` 和 `native.m` 里的 `MyGoInputAccessory`，iOS 也画 Go 栏，靠 `keyboardTop` 把内容排在键盘之上；`InputModifiers` 留在 `ui`）、`Microphone`（**已改写**为 `plugins/microphone`）、`Speech`（含 `RegisterProvider`，**已改写**为 `plugins/speech`）、`Scanner.ScanCode`（**已改写**为 `plugins/scanner`）、`OpenInAppBrowser`（**已改写**为 `plugins/browser`）、`Network.Prepare`/`NetworkError`（**已改写**为 `plugins/network`），以及上面「缺口」列的部分。与移动端无关、不受影响的 B 类扩展：`Router.Reset/History/Transitioning`、`Network.PathStatus/OnPathChanged`、终端 `SelectOnDrag`、`ComboboxParts.Inline`、`Window.OnFirstFrame`、richtext `OnInput`/`OnPaste`/`OnCopy`、`Context.Blur`。

### 当前状态与第二阶段清单（2026-11-10）

**第一阶段（统一公开 API）已完成**，上表「缺口」列全部过完：能提前收敛的已改，其余逐项核实并写明换后端时的处理。Miya 固定在 `23e766f4`（Miya `0713575`），iPhone 16 上验证过键盘栏。

**架构仍是 fork 的，和 #940 不同：**

| | fork（现在） | #940 |
|---|---|---|
| 移动端后端 | 两个：`internal/ios`（cgo + Objective-C）、`internal/android`（cgo + JNI），宿主工程在仓库里（`android/`、`native/ios`），约 1.3 万行 | 一个：`internal/mobile`，Go 与 Swift/Kotlin 走生成的二进制协议（`internal/mobile/wire/gen/spec.go`），宿主内嵌在 CLI（`mygo build -platform ios` 或 `android`），约 1.8 万行 |
| 插件调原生 | 插件是壳，调根包的 fork 模块（`mygo.Biometrics`、`Permissions`、`Dialog.Photos`、`App.RegisterPushNotifications`、`Microphone`、`Speech`、`Scanner`、`OpenInAppBrowser`、`Network.Prepare`）；fork **没有** `mygo.Native` | 插件经 `mygo.Native(id).Call`、`PluginView` 调自己的原生代码 |
| 触摸引擎 | fork 的 `ui/touch*.go`（`TouchScroll`、`InputLongPress`、捏合 `Gestures`、`InteractiveBack`） | `ui/touch.go`、`touch_back.go`、`touch_text.go`（见上表手势一行） |
| 安全区 | 只算系统栏，`ExtendIntoSafeArea` 控制 | 计入键盘，根元素自动内缩 |
| 生命周期 | fork 的 `Lifecycle()`/`OnDidEnterBackground` 等保留为扩展，同时发 `Power` 的 suspend/resume | 只有 active/resign、`Power`、`OnSaveState` |

**第二阶段：#940 合进上游 `main` 后换后端**（规则 1，以它为准）。**#940 已于 2026-11-17 合进上游 `main`（`6e712c6e`）**；fork `main` 停在它之前的 `12e8babd`，第二阶段在单独分支上做：在 `~/.cache/mygo-pr/sync-940` 从 fork `main` 合并 `6e712c6e`，Miya 用临时 go.work 指向那里验证，完成后再合进 `main`。此后同步上游时，在 #940 进 `main` 之前，只合并它之后不依赖它的提交，或者一并等第二阶段。按顺序：

1. 合并上游后，删除 `internal/ios`、`internal/android`、`android/`、`native/ios` 及 `cmd/mygo` 中 fork 的 iOS 打包，改用 `internal/mobile` 和 CLI 内嵌宿主；`ui/touch.go`、`ui/touch_text.go`、`ui/gpu_android.go` 会直接冲突，取 #940 的。
2. 插件改为 `mygo.Native`：`biometrics`、`push`、`imagepicker`、`permissionflow` 换成 #940 自带的实现（保留 fork 扩展：细分错误、`push.SetPresentation`、`NotificationOptions.Data`）；`microphone`、`speech`、`scanner`、`browser`、`network` 是 fork 独有，给它们写 Swift/Kotlin 原生端。然后删根包对应模块（`mygo.Biometrics`、`Permissions`、`Microphone`、`Speech`、`Scanner`、`OpenInAppBrowser`、`Network.Prepare`、`Dialog.Photos` 的手机实现、`App.RegisterPushNotifications`）。
3. 触摸：删 `TouchScroll` 调用；`TouchSelection`/`InputLongPress` 的使用者（Miya `conversation_view`、`mobile_voice` 按住说话，fork 终端插件）重写或给 #940 加扩展；Miya 远程桌面的捏合缩放给 #940 加多指手势；`InteractiveBack` 改用 `Transition`、不开 `Router.SystemStack` 的 Router，或给 #940 加开关（见「2026-11-16 跟踪到 `upstream/mobile` `ea96ba60`」）。
4. 安全区：Miya 删 `ExtendIntoSafeArea` 和自己按 `safeArea` 的内缩；`keyboardbar` 依赖「内容排在键盘之上」，在 #940 上复核（它的 `SafeArea` 计入键盘，根元素自动内缩）。
5. 生命周期：`Lifecycle` 系列在 #940 上用 active/resign 与 `Power` 重建，或让 Miya、myclash 改用 `App.IsActive`/`Power.OnSuspend`。
6. 验证：`go build ./...`、`go vet . ./ui`、`../Miya` 用 `GOWORK=off` 编译与测试、myclash 编译，iPhone 上装 Miya 实测（安装步骤见 Miya 的 `docs/install-and-upgrade.md`；`build-ios.sh -ios-device` 用硬件 UDID，`devicectl` 用 CoreDevice 标识符）。

在那之前：同步上游照常进行（`git fetch upstream` 后 `git merge --no-ff upstream/main`），并跟踪 #940 的变化。**跟踪 `upstream/mobile` 分支，不再跟踪 `pr/1581`**：#1581 的 head 停在 `6702121b`（2026-11-09），之后 #940 的提交都直接推到 `mobile`。查看新增：`git log --no-merges <上次的提交>..upstream/mobile ^upstream/main`。worktree `~/.cache/mygo-pr/mobile-940` 改为检出 `upstream/mobile`。

#### 2026-11-14 跟踪到 `upstream/mobile` `2d314c44`

从 `6702121b` 到 `2d314c44`，#940 自己的提交有 9 个，其余是它合进来的上游 `main`，fork 已经同步过：

- **已提前收敛（通用部分，规则 1）**：iOS/Android 构建不再编译桌面文件。`GOOS=ios` 匹配 `darwin` 标签，`GOOS=android` 匹配 `linux` 标签。
  - 移植了 `51f12a8d`、`7eecf682`。
  - 补上了 #940 更早已有的部分：整个 `internal/darwin` 都标为 `darwin && !ios`，包括 fork 独有的 `trackpad_testhooks.go`；`plugins/automation` 的 `menu_darwin.go`/`menu_other.go` 改成 #940 的标签。只移植那两个提交会让 `internal/darwin` 在 iOS 上只编译一半，而 automation 在 iOS 上会导入它。
  - 冲突处理：`load_android.go` 保留 fork 的版本，因为 fork 的 `load_common.go` 已经给 Android 提供 `uploadElement`；`dmabuf_other.go` 取上游写法。
  - 验证：iOS 全量 `go vet` 通过（除 `cmd/mygo`、`internal/e2e`），Android `.so` 编译通过。
- **还没对齐的构建标签**：`backend_other.go`、`content_rules_other.go`、`context_menu_other.go`、`audio/`、`internal/download`、`cmd/mygo/term_*`、`internal/e2e` 的其余文件。这些和 fork 自己的后端选择（`ios && cgo` 等）纠缠在一起，换后端时整体取 #940 的。
- **要等第二阶段**：以下都依赖 `internal/mobile` 的 transport/nativehost 和 CLI 内嵌宿主，现在不移植。
  - native 模块上手机：iOS 用 Swift，Android 用 Kotlin，含 `internal/nativegen/kotlingen.go`、`native/lib_android.go`（`07d78460`）。
  - `plugins/sqlite` 上手机（`36cdb116`、`ae204203`）。
  - 手机文档（`1781b743`、`2d314c44`）。
- **已核实无需处理**：#940 对 permissionflow 的本地化已经在上游 `main`，fork 也有。两边 permissionflow 剩下的差别只有 fork 的 `phone.go` 桥接，按第二阶段第 2 步删。

#### 2026-11-16 跟踪到 `upstream/mobile` `ea96ba60`

从 `2d314c44` 到 `ea96ba60`，#940 自己的提交有 4 个，期间没有再合入上游 `main`（`mobile` 落后 `main` 48 个提交）：

- `3607c02e`：**`Router.SystemStack`**。手机上把 Router 到当前页为止的历史交给系统页面栈显示，带系统导航栏、每页标题和返回按钮，仍是一个窗口。
  - iOS：每一页是窗口 `UINavigationController` 的一个 view controller，过渡和侧滑返回由 UIKit 负责。窗口只有一个 surface，移到当前显示的页面里，离开的页面留快照。
  - Android：窗口的 `MaterialToolbar` 跟随页面栈，页面滑动和预测性返回仍由 Router 自己做。
  - 平台契约新增可选的 `platform.StackSurface`：核心在新页面那一帧呈现前发 `SetStack` 并等待，用户在系统里返回时回传 `StackPopped`，新页面呈现后回传 `StackShown`。视图用 `Router.InSystemStack` 判断要不要自己画导航栏。桌面和浏览器没有 `StackSurface`，行为不变；counter-native 增大 288 字节，hello 不变。
- `730d71b2` 与 `bae9c648`：iOS 第一页改成小标题，随即撤回，两者抵消。
- `ea96ba60`：iOS 26 返回到带大标题的页面时，让大标题跟着页面移动。做法是每屏放一个透明、不接收触摸的空 scroll view 作为 `contentScrollView`，surface 的顶部内缩取它调整后的 inset。同时作用于 Router 的页面栈和 `Window.Push`。

处理：

- **要等第二阶段**：实现都在 #940 的宿主里（`internal/mobile/ios/Sources/MyGoHost/Navigation.swift`、`App.swift`、Android 的 `HostImpl.kt`），现在不移植。fork 的 iOS/Android 后端没有 `StackSurface`，单独移植 ui 部分也不会改变行为。
- **换后端时要核对**：
  - `InteractiveBack`：上表那一行写的是 #940 自动开启、没有开关。现在多了 `SystemStack`，可以按 Router 开启，开启后 iOS 的侧滑返回由 UIKit 负责。Miya 在抽屉页和 `/desktop` 关掉侧滑返回的需求，可以改用不开 `SystemStack` 的 Router 来满足，届时复核。
  - fork 的 `Router.Reset`/`History`/`Transitioning`/JSON 持久化（B 类，Miya 用 `Transitioning`）也会改 Router 的历史。要确认它们和 `SystemStack` 的页面栈同步（`noteStack`、`syncStack`、`stackPopped`）能共存：`Reset` 和恢复持久化后，系统页面栈要跟着重建；`Transitioning` 在 UIKit 负责过渡时要有意义。

### 第二阶段进度（2026-11-17）

分支 `sync-940` 合并 #940（`6e712c6e`），按 2026-11-17 的决定「先换平台，再逐个移植」：平台层和核心以 #940 为准，fork 独有的移动端 API 先删掉或在手机上返回 `ErrUnsupported`，之后每项单独移植回来。

**已换成 #940 的（规则 1）：**

- 后端：删除 `internal/ios`、`internal/android`、`android/`、`native/ios` 和根包导入它们的文件；手机走 `internal/mobile`（恢复了 #940 的 `backend_mobile.go`）。`internal/gpu/gl` 删除 fork 的 `context_*.go`、`load_common.go`，Android 文字引擎删除，`coretext_apple.go` 改回 `coretext_darwin.go`（保留 fork 的改动）。
- typesetting 改回 `go-text/typesetting v0.3.5`，删除 `Heyana/typesetting-go`、`third_party/hb-build`、`scripts/android-*.sh`、`.github/workflows/android.yml`，AGENTS.md 的 Runbook 一节删除。
- CLI：`IOS`/`Android` 配置、打包、`devices`/`publish` 取 #940；删除 fork 的 17 个 `ios_*.go` 和 `mygo push setup/status`；`packages/cli/index.d.ts` 取 #940。
- `ui` 顶层以 #940 为底，用三方合并补回 fork 的非移动端扩展：Router `Reset`/`History`/`Transitioning`（读 #940 的 `swiping`）/JSON 持久化（`restoring`，持久化改为沿用 `SystemStack`）/`TransitionStack`，frame-why、诊断计时，`Startup` 的 `firstFramePresented`，`Tester.SetRefreshRate`，`Context.Blur`/`Services.Blur`，`diagnostics.go`、`startup.go`、`bitmap.go`、combobox、gridview 等。`flags` 改回 `uint32`。fork 的 `ui/touch_engine.go`、`touch_text.go`、`gesture.go`、`router_touch.go`、`touch_elastic*`、`input_options.go`、`native_text_scope.go`、`text_geometry.go` 等删除，用 #940 的触摸引擎。
- 平台契约：删除 fork 的 `Window.SafeArea`、`WindowHandler.SafeAreaChanged`、`WindowOptions.ExtendIntoSafeArea`、`surface.Conn.SafeArea`/`ContentBackground`、`TextInputOptions`、`TextGeometry`、`NativeTextSelectionSurface`、`Dialogs.ShowExportDialog`/`ShowPhotoDialog`、`Sharing`（改用 #940 的 `Share`）、`SurfaceMemoryPressure`（改用 #940 的 `SurfaceLowMemory`，测试保留）、`AppHandler.DidEnterBackground`/`WillEnterForeground`。根包删除 fork 的 `Haptics`、`SetStatusBar`（#940 的 `Haptics`、`SystemBars`）、`Dialog.Export`/`Photos`。
- 插件 `biometrics`、`push`、`imagepicker`、`permissionflow_mobile.go` 取 #940（fork 的测试删除）。
- `examples/ios-native` 删除。

**保留、但在 #940 的手机后端上暂时返回不支持：** `Backend` 的 `Authenticator`、`Mobile`、`Permissions`、`Speech`、`SystemManagedLifetime` 改为可选接口（`platform_optional.go`），桌面照旧，#940 手机后端没有实现。受影响：`mygo.Biometrics`、`Permissions`、`Speech`/`plugins/speech`、`Microphone`/`plugins/microphone`、`Device`/`DismissKeyboard`/`Scanner`/`Network.Prepare`（在 `device.go`，供 `plugins/scanner`、`plugins/network`）、推送回调、App Group、后台刷新、Live Activity（`*_other.go` 去掉标签，所有平台都是空实现）。`SystemManagedLifetime` 在手机上回退为 true。

**改接到 #940 上的：** `Lifecycle` 的进出后台改由 `Power` 的 suspend/resume 驱动（只在系统管理生命周期时；重复的 suspend 忽略，resume 只从后台恢复），顺序同 fork：`OnDidEnterBackground` → `Power.OnSuspend` → checkpoint。`keyboardbar` 的长按改为轮询 `Element.LongPressed()`。`automation` 保留 #940 的 `@`/`tcp:` 监听，外层仍是 fork 的 `listenWith`。

**待移植（第二阶段第 2～5 步）：**

1. 触摸：`TouchScroll`、`TouchSelection`/`InputLongPress`（终端长按选择已关闭，`longPress := false`）、捏合 `Gestures`、`InteractiveBack`。
2. 键盘与输入：`Element.InputModifiers`（`keyboardbar` 的修饰键锁定、终端软键盘修饰键，相关测试已删）、`KeyboardDismiss`/`KeyboardDismissInteractive`、`TextCaretFunc`（终端改用 `TextCaret(v.caret())`）、`TextContext`/`InputTextReplace`/`InputSelection`（终端原生替换已删）、原生文字选区（`ui/richtext` 的 `SelectionRectsForRange` 已删）。
3. 用 `mygo.Native` + Swift/Kotlin 重写：`microphone`、`speech`、`scanner`、`browser`、`network`、App Group/widget（`sharedstore` 的 Swift 样本测试已删）、后台刷新、Live Activity、导出对话框、fork 的推送发送端 CLI。
4. Miya：用临时 go.work 指向 `sync-940` 编译，第一轮报 `ui.KeyboardDismissInteractive`、`TouchSelection`、`KeyboardDismiss`（修掉后依赖它们的包可能再报）；按 `ExtendIntoSafeArea` 一行的说明删掉 Miya 自己的安全区内缩。

**验证（2026-11-17，`sync-940`）：** macOS/Linux/Windows 的 `go build ./...`（含 `CGO_ENABLED=0`）与 `go vet ./...`，`go vet -tags mygo_noinspector ./...`，js/wasm vet；iOS（iphoneos SDK）和 Android（NDK r28 clang）对 `. ./ui ./internal/mobile/... ./plugins/...` 的 vet；`go test -short`（除 `internal/e2e`）全部通过，`go generate ./ui` 与 `uimigrate` 已更新。未做：发布体积、e2e、真机。

#### 2026-11-18 移植回 Miya 依赖的功能

在 `sync-940` 上把 Miya 用到的 fork 移动端功能逐项接到 #940 上（`9e11578a..822fd36e`），Miya 迁移在它自己的 `sync-940` 分支（`8b7b842`，未推送）。

- **插件**（`mygo.Native` + Swift/Kotlin，iOS 模拟器和 Android arm64 均编译通过）：`biometrics` 加回细分错误（`ErrUnavailable` 等七个，包着 `NativeError`）；`push.SetPresentation` 经 iOS 宿主新增的可选钩子 `PluginHost.presentation`（`willPresent` 里问，3 秒不答照常显示），本地通知的 Data 由核心按 ID 提供（`pushhub.LocalData`）；`microphone`（AVAudioEngine / AudioRecord，16 kHz PCM 事件）、`network.Prepare`（URLSession HEAD；Android 无事可做）与新的 `network.Interfaces`（Android 经 Java 列网卡，供 Miya 的 netmon）、`browser.Open`（SFSafariViewController / Custom Tab）、`scanner`（AVCaptureMetadataOutput / 拍照或相册 + ZXing）。根包的 `Microphone`、`Scanner`、`Network.Prepare`、`OpenInAppBrowser` 删除。`speech` 走自定义 provider 时不经平台，无需改动；内置 `apple` provider 在手机上仍返回不支持。
- **触摸**（直接改在 `ui/touch.go`、`ui/handler.go` 等源文件里，不改 #940 默认行为）：`InputLongPress` 先交给指下的输入处理器，接了就拿走这根手指（之后 `InputPointerMove`、`InputPointerUp`/取消）；文本要 `TouchSelection()` 才收到，且这时按下推迟到抬起；`TouchPointer()` 让手指像鼠标一样拖动；`KeyboardDismiss` 在开始平移时收起键盘；`Gestures()` 收双指 pinch/rotate/pan/tap，由 `internal/mobile/surface.go` 识别（没有元素要时仍发 `PointerZoom`）；`Router.DisableBackGesture` 关掉边缘侧滑。这些开关存在每帧的 `node` 上、`commitElement` 时复制（共用帧戳会让不再要的开关残留）。终端的长按选择恢复。
- **其他**：`WindowOptions.ExtendIntoSafeArea`（根元素不按 inset 内缩，`Context.SafeArea` 仍按 #940 含键盘）、`KeyboardASCII`（iOS `.asciiCapable`，Android 用 visible-password 变体）作为 fork 扩展加回。
- **Miya**：`InteractiveBack` → `DisableBackGesture`；去掉 `TouchScroll`，远程桌面触控板/指针模式用 `TouchPointer`；`main_android.go` 只剩 netmon（启动、日志由宿主负责）；文件浏览器改为 Miya 自己的插件 `internal/mobile/miyanative`（也声明 `keychain-access-groups`，团队前缀写死，因 #940 CLI 不展开 `$(AppIdentifierPrefix)`）；`mygo.json` 的 `ios` 换成 #940 字段，**最低 iOS 升到 17.0**（#940 宿主本身要求，2026-11-18 决定）；桌面面板的终端排除出手机构建（libghostty-vt 无 iOS/Android 构建）；`build-android.sh` 改调 `mygo build -platform android/arm64`。测试改用 `Tester.Finger*`，`setSafeInsets` 同时开 `SetExtendIntoSafeArea`。
- **验证**：mygo 三桌面 build/vet、wasm vet、iOS/Android vet，`ui`、根包、`internal/mobile`、插件测试通过；Miya `go test -short ./...` 只剩迁移前就失败的 `TestDiscoverProjectWebServicesWithRealListeners`、`TestDesktopPageRendersLiveConnectingAndEnded`；Miya iOS 模拟器包、Android apk 构建通过，模拟器上启动到配对页并弹出通知授权。**未做**：真机（iPhone、Android 设备）实测、发布体积复测、e2e。
- **仍未移植**：App Group/widget、后台刷新、Live Activity、推送发送端 CLI（`mygo push`）、Miya 以外的 fork 移动端功能。

## 差异分类

### A. 移动端（保留，不提交上游）

| 范围 | 内容 |
|---|---|
| `internal/android`、`android/`、`internal/ios`、`native/ios` | 两个平台的后端、JNI/Objective-C 桥接、宿主工程 |
| `push/`（APNs、Live Activity）、`cmd/mygo` 中的 iOS 打包与扩展 | 推送发送端、`mygo push setup/status`、iOS 构建 |
| 根包模块 | `app_group`、`background_refresh`、`biometrics`、`device`（扫码、键盘）、`lifecycle`（`OnDidEnterBackground` 等）、`microphone`（`Microphone.Start`：16 kHz 单声道 PCM 流，供自定义 `SpeechProvider` 使用；2026-11-08 从 Miya 迁入）、`mobile*`、`mobile_experience`、`permissions.go` 中的 `Permissions` 模块（桌面后端返回 unsupported）、`share`、`speech`、`push.go`、`task.go` |
| 独立包 | `sharedstore`、`generationcache`、`async`、`backgroundtask`、`timeseries`、`ui/chart/sparkline` |
| `ui/` | `touch*`、弹性滚动、`native_text_scope`、`input_options`、`text_input_client`、`gesture`、`router_touch`、SafeArea |
| `plugins/terminal` | `software_keys`、`input_context`、`ActiveCursor`、`SelectionAnchor`、`InsertNewline`、`OnSubmit`、触摸选择 |
| `Secrets` 的 iOS/Android 后端 | `internal/ios/secrets.{go,m}`、`internal/android/backend/secrets.go` 实现上游的 `platform.Secrets` |

#### Android 原生支持补齐（2026-11-08）

- Android pump 改为非阻塞 FIFO，修复回调填满队列后向自身投递帧造成的自锁；补齐 Surface/EGL 恢复、启动前生命周期与深链回放，以及 Activity 重建后的去重。
- 增加异步原生服务请求、取消与完成一次的桥接，接通权限、设备认证/PIN 回退、系统对话框、图片选择、文件保存、录音、浏览器/PDF/导出、触觉和角标。
- 增加原生浮动编辑菜单、选区手柄、文字裁切与 UTF-16 映射；纯文本、HTML、Markdown 同时复制，Markdown 通过受限只读 FileProvider URI 供其他应用读取。
- 补齐私有目录、Java 网络接口快照、字号/减少动效、主题系统栏、彩色 emoji 和系统本地时区初始化。共享 UI 的内置编辑器增加直接选区设置与预编辑方向处理，保持既有自定义输入客户端的兼容路径。
- 在 API36/arm64 模拟器完成真实收发、相机/相册二维码识别、选图、录音、文件浏览/导出及选区/剪贴板验证；这不是全功能或全版本验收。连续取景扫码尚未实现，API29/30 彩色字形回退未实机验证。远程推送见下文 2026-11-09。
- 桌面维持无 cgo；生产配置 counter-native / hello 的大小仍为 9,869,042 / 9,750,354 字节。Android 修改保留在 fork，不提交移动端代码到上游。

#### Android 后台运行与推送接口（2026-11-09）

- `Power.KeepRunning`（上游接口）在 Android 上实现为前台服务 `MygoKeepRunningService`（`specialUse`，`START_NOT_STICKY`，按引用计数启停，`internal/android/backend/power_android.go`、`MygoKeepRunning.kt`）。应在前台调用；进程被结束后不自动重启。
- 本地通知点击直接打开 `MygoActivity`（Android 12 起广播跳板无法把应用带回前台），intent 带本机随机密钥；原 `MygoPlatform$NotificationReceiver` 已删除。
- 通用推送接口：`App.OnPushSubscription`（`Service`、`Token`、`Data`；iOS 也发 `{apns, token}`）、可选 `platform.PushSubscriber`。Android 不内置任何通道：应用实现 `org.mygo.MygoPushChannel`，在 Manifest meta-data `org.mygo.push.channels` 中列出类名；`MygoPush.kt` 负责选通道、显示 JSON 消息与点击回传（JNI `Native.onPush` / `Native.onNotification`，Ready 前排队回放）。
- 曾实现并实测过 FCM（`push/fcm`）与 UnifiedPush/Web Push（`push/webpush`）通道，因需要中转服务或额外安装分发器而移除。Miya 改用 `KeepRunning` 保持自己的 tailcat 连接收通知。
- 已在 API36 模拟器验证前台服务、后台保活和本地通知点击；自定义推送通道的注册路径只验证了编译。

### B. 对上游的扩展（保留；通用的可以提交上游）

| 功能 | 位置 | 备注 |
|---|---|---|
| `desktopcontrol` | `plugins/desktopcontrol` | 从 Miya 下沉的 macOS 显示器/窗口、ScreenCaptureKit JPEG、键鼠、AX 对话框与 gamma 遮屏；纯 Go，无网络入口，授权与租约由使用方负责 |
| `Network.PathStatus` / `OnPathChanged` | `network_path.go` | 通过可选接口 `platform.NetworkPathSource` 扩展上游的 `NetworkModule` |
| `Router.Reset` / `History` / `Transitioning` / JSON 持久化 | `ui/router*.go` | 上游只有 `Back`/`Forward`/`Go`/`Push`/`Replace`；Miya 使用 `Transitioning` |
| inline combobox | `ui/combobox.go` | 提交 `0f6c018e`、`bc60b9d5`；上游没有 |
| `DecodeBitmapSize` | `ui/bitmap.go` | 按显示尺寸解码；依赖上游的 `Bitmap.remake` |
| `LastInput` | `input.go` | 本应用最后一次输入；不同于上游的系统空闲时间 `Power.IdleTime` |
| `updater.InstallUpdate` | `plugins/updater/install.go` | 供远程控制端调用 |
| `FrameReading` / `FrameAverage` / `HeapReadingNow` | `ui/diagnostics.go` | 上游没有这些 API |
| frame-why 诊断（`MYGO_FRAME_WHY`）、`LoopFPS` | `internal/framewhy`、`ui/framewhy.go` | 提交 `fae3d440` |
| `PersistState`、`OnFirstFrame`、`ui.Startup` | `app_state.go`、`first_frame.go`、`ui/startup.go` | 上游的 `WindowStateStore` 只保存窗口状态 |
| 终端：`AdaptiveColors`、`OnRenderEvent`、`CopyRawText`、`SelectedText`、`SelectAll` | `plugins/terminal` | `AdaptiveColors` 使用上游的 `contrasted`/`luminance`/`contrast` |
| 终端：`SelectOnDrag` | `plugins/terminal` | **Miya 在用**（`view_panel.go`）；上游的做法是按住 Shift 拖动 |
| 终端：远程固定网格（`FixedCols`、`FitToView`、`ReadScreen`、`SetInputEnabled`、`resume`） | `plugins/terminal` | 当前没有使用方 |
| 终端：TUI 硬换行与缩进续行的路径识别（`hardWrappedFileAt`） | `plugins/terminal/links.go` | 依赖 `vt.RowsJoined`、`RowText`；通过适配层 `matchLink` 调用上游的 `urlAt`/`pathAt` |
| 终端：快照保留软换行和超链接目标 | `internal/vt/snapshot_links.go` | 提交 `c55486a2` |
| `internal/probe` | 性能探针 | 主要用于 Android |
| `automation.Start` | `plugins/automation` | 共享库宿主可在建窗前显式启用自动化；运行时 Unix socket 限于私有目录，复用现有 inspector 开关 |

### C. 等上游合并后就会消失的差异

进行中的上游 PR：

- #1525（2026-11-09 开为草稿，2026-11-21 确认仍待合并）：`plugins/desktopcontrol` 作为纯 Go 官方插件，附 `examples/desktopcontrol` 和 `docs/plugins/desktopcontrol.md`。不含 `RunPrivacy`（gamma 遮屏），它留在 fork，之后单独提交。分支 `pr/desktopcontrol-plugin`，worktree `~/.cache/mygo-pr/desktopcontrol`。合并后按规则 1 改用上游版本，fork 只保留 `privacy*`。
- #1571（2026-11-09 开为草稿，2026-11-21 确认仍待合并）：loading 插件默认 30 fps（`Options.FPS`、`DefaultFPS`），`Painter.AnimationFrameRate` 在窗口没有键盘焦点时请求一半的帧，`ui.Tester.SetRefreshRate`。fork 的提交是 `2035305e`。上游版本的 `AnimationFrameRate` 保留上游的 `p.rt.repainting = true`，fork 用的是 frame-why 诊断的 `repaintFrames(false)`，其余相同。分支 `pr/loading-fps`，worktree `~/.cache/mygo-pr/loading-fps`。合并后同步时 `ui/paint.go` 以上游为准，同时保留 fork 的 `repaintFrames`。

已被上游合并的 fork 改动：#928（macOS 空闲唤醒）、#1057（CJK 强调）、#1058（链接实线下划线）、#1077（zsh shell integration，上游在其基础上做了修改）、#1305（同步更新在 1 秒后放行，2026-11-06 同步时 `view.go` 取上游，两边的测试都保留）、#1530（macOS 原生 popover 跟随应用外观，2026-11-09 合并；同步时自动合并在 `showPopover` 里留下了 fork 和上游各一份 `setAppearance`，删掉 fork 那份，`popover.go` 与上游一致）、#1302（终端链接识别：Unicode 路径、带引号的路径、`(行,列)` 和 `#L行C列`，2026-11-07 同步时改用上游，见下表）。

已关闭、未合并：#1301（窗口空闲时释放文本 shaping 临时缓冲区，2026-11-06 关闭）。上游的 `01dfd8ed` 已经加了 `System.ReleaseShaping`，并在 `windowHost.idle()` 中调用，和 fork 的 `ReleaseScratch` 用的是同一个挂点。idlemem 实测这部分收益不到 1%（59.4 → 58.9 MB）。2026-11-06 同步时已删掉 fork 的 `ReleaseScratch` 和 `ui/window.go` 中的 `textLayouts`，改用上游的实现（规则 1）。Core Text 属性缓存和 `lineCarets` 的改动没有单独提交。

## 已清理的重复实现（2026-11-05、2026-11-06）

| 提交 | 内容 | 迁移注意事项 |
|---|---|---|
| `87bc3515` | caret 停靠点改用上游的 `seen` 掩码 | — |
| `f54b6559` | 终端搜索改用上游的 `Search`/`SearchNext`/`SearchPrevious`/`EndSearch`/`OnSearch`；删除 `SetSearch`、`SearchState`、`vt.Search.Highlights` | 测试通过 `t.find.drive` 推进搜索，和 view 的做法一致 |
| `619927b9` | 最小对比度改用上游的 `Options.MinimumContrast`；删除 `Theme.MinimumContrast` 和 fork 自己的对比度计算 | — |
| `d8cad29b` | 同步更新超时改用上游规则（从开始算起 1 秒），不再每次输出都重新计时 | 保留了主动唤醒（C 类，#1305） |
| `fdd786f2` | 打开链接只走上游的 `OpenLink(string)`；删除 `OnOpenLink`，`Link` 改为内部类型 `linkTarget` | — |
| `229142de` | `SecureStore` 改为上游的 `Secrets`；iOS/Android 实现 `platform.Secrets`；删除 `SecureStorage` | **钥匙串数据没有迁移**：旧数据的 service 是 `<bundleID>.mygo.<namespace>`，新数据用 `Secrets` 的 service。Miya 手机端升级后需要重新配对；Miya 桌面端已按 APNs token 去重推送注册，避免同一部手机重复收到推送 |
| `73681819` | 删除 `Router.Pop`（等同于 `Back`），改用上游的 `Back` | — |
| `d2a71494` | `ui/window.go` 中渲染路径的命名改回上游写法 | 上游没有“日志滞后一帧”的问题 |
| `deccef93` | 指针设备改用上游的 `platform.PointerKind`、`SurfaceEvent.Pointer`（以及 `Pressure`）；删除 `platform.PointerType` 和 `SurfaceEvent.PointerType` | iOS/Android/桌面后端/fake 改写 `Pointer`；mouse 是零值，不用显式写。`Tester.send` 会用 `UsePointer` 的设备覆盖 Move/Down/Up 的 `Pointer`，fork 的触摸测试 helper（`ui/touch_engine_test.go` 的 `touch`）因此先调 `UsePointer(PointerTouch, 0)`，同时在事件里写明 `Pointer`，让 Cancel 等其他 kind 也带上设备 |
| `deccef93` | 删除 fork 重复的 `platform.PointerCancel`；系统发来的取消走上游的 `pointerCancel` | fork 触摸手势内部的 `cancelPointer` 暂未迁移（见「已知问题」） |
| `deccef93` | 窗口空闲释放文本缓冲改用上游的 `System.ReleaseShaping`；删除 `ReleaseScratch` 和各引擎的 `releaseScratch` | #1301 已关闭 |
| `deccef93` | 终端同步更新的唤醒改用上游的 `holdDue` | #1305 已被上游合并 |
| `d298e2d8` | 终端路径识别改用上游 #1302 的 `pathAt`/`pathOf`/`urlAt`/`matchAt`/`balanced`；删除 fork 的 `pathTokenPattern`、`pathLocationPattern`、`linkProseDelimiters`、`trimLinkToken` 和旧的 `matchLink` 主体 | `matchLink` 只剩适配层：把上游返回的列范围换回 rune 下标，从 `path:line:col` 解析出 `Line`/`Column`，`raw` 就是交给 `OpenLink` 的字符串。行为变化：`main.go:0` 现在按上游打开 `main.go` |

此外，滚动条策略在更早的合并中已经采用上游的“悬停时显示”和 `KeepScrollbars`。`93e7fb70`（延迟帧计时）优化的是 fork 自己的 API，不是上游的。

## 待提交上游的候选（尚未提交）

- 搜索匹配以宽字符结尾时，上游的 `viewportMatches` 少高亮最后一个单元格（fork 原来的 `Highlights` 处理过）。
- reflow 后上游的搜索保留了匹配，但丢失了选中状态（查找栏显示“0 of 1”）。
- `fae3d440`：frame-why 诊断和 `LoopFPS`（提交时去掉 `AGENTS.md` 的改动）。
- Metal 连续阴影捷径：补丁在 `~/.cache/mygo-pr/metal-continuous-shadows.patch`。和上游 #910 的 shader 一起编译后，float16 结果比完整公式差 3 ULP（测试只允许 1 ULP），不是编译器版本造成的，需要调整 shader 写法。

已核实**不是**上游问题、不要再提的：渲染路径日志滞后（`d2a71494`）；`FrameAverage` 计时开销（fork 自己的 API）；滚动条“滚动时才显示”（和上游设计冲突，已放弃）；`ea1f3508`（ListState 第二次布局：上游只有 reveal 会在同一遍里再次调用 `remember`，而 reveal 一定会移动滚动偏移，`movedIn` 必然请求下一帧，所以 bug 被掩盖，写不出在上游失败的测试，2026-11-09 核实）；`508037e1`（facade 把 nil 回调包装成非 nil：上游 #239 之后，只有需要 `makeContext`/`wrapElement` 转换的 13 处回调还会包装，对应的核心实现都不把 nil 当作合法输入；出现 panic 的 `InputModifiers` 和 `Services.Blur` 都是 fork 独有的 API，2026-11-09 核实）。

## 已知问题

- `android/jni/jni_textinput.go` 没有经过 gofmt。
- 上游 macOS 的 GUI 测试不稳定（`TestPanel`、`TestContentWindowResizeRightAfterFrame` 等待超时），上游 `main` 本身也会失败；PR 中 macOS 任务失败时先对照上游 `main`。
- 用 `go build` 单独链接 `examples/ios-native` 会缺少 `_MyGoBackgroundRefresh*` 符号，这些符号由 `mygo build` 打包时链接；检查 iOS 代码时用 `go build ./internal/ios` 和 `go vet`。

## 工作流程

环境：非登录 shell 的 PATH 里没有 `go` 和 `gh`，命令前加 `export PATH=/opt/homebrew/bin:$PATH`。zsh 中不加引号的 `$VAR` 不会分词；`ls` 是 `lsd` 的别名，脚本里用 `/bin/ls`。

### 同步上游

```sh
git fetch upstream
git rev-list --count main..upstream/main          # 落后多少
git merge --no-ff upstream/main -m 'Merge upstream/main through <sha>'
# 冲突：同一功能取上游（规则 1）；自动合并后也要编译，上游可能改了名字
# （例如 Bitmap.decode → remake）
go build ./... && go vet . ./ui && go test . ./ui ./internal/text ./plugins/terminal/... -count=1
(cd ../Miya && go build ./...)
# 移动端：上游给 platform 接口加的方法（如 Window.GPUProcessID、SetVibrancyTint、
# Shell.TypeIcon）iOS/Android 后端不会自动有，桌面编译也发现不了
GOOS=ios GOARCH=arm64 CGO_ENABLED=1 CC="$(xcrun --sdk iphoneos -f clang) -isysroot $(xcrun --sdk iphoneos --show-sdk-path) -arch arm64" go vet . ./ui ./internal/mobile/... ./plugins/...
GOOS=android GOARCH=arm64 CGO_ENABLED=1 CC="$NDK/toolchains/llvm/prebuilt/darwin-x86_64/bin/aarch64-linux-android29-clang" go vet . ./ui ./internal/mobile/... ./plugins/...   # sync-940 起，手机后端是 #940 的 internal/mobile
go generate ./ui && git status --short ui/api_gen.go  # 上游新加的 facade 函数要用 fork 的生成器重新生成
git push origin main                                # 快进推送，不用 force
```

生成的二进制（`internal/gpu/metal/shaderlib.go`、`plugins/glass/shaders_darwin.go`）冲突时不要手工挑选：取任一侧后在 macOS 上 `go generate ./internal/gpu/metal ./plugins/glass`。

合并之后，再对照上游这次改动的部分检查 fork 的新增代码：同一功能、但没有文本冲突的重叠，git 不会提示，需要人工找出来，按规则 1 处理。最后更新本文件：把上游已合并的 PR 移到“已被上游合并”，删掉已经消失的差异。

### 向上游提交 PR

1. 在 `upstream/main` 上建 worktree（`git worktree add -b pr/<topic> ~/.cache/mygo-pr/<topic> upstream/main`），只移植通用部分，剥离移动端和 Miya 专用的代码。
2. 先确认上游还没有同样的功能：搜索同名 API，并看相关目录在 fork 分叉后的 `git log`。
3. 测试先在未修复的代码上失败、修复后通过；不依赖墙钟时间（用 `ui.Tester.Advance`、`internal/wait`）。
4. 使用上游的 PR 模板。AI 披露写 “Claude Code with Claude Opus 5.5 (claude-opus-5-5)”；**“A human read every line” 留给用户本人勾选**；先建草稿 PR（`gh pr create -R egoist/mygo --draft --head daodao97:<branch>`）。
5. 上游合并后，下次同步时那部分差异会以上游为准消失；把本文件对应的行移到“已被上游合并”。

### 重新统计本文件的数据

```sh
git fetch upstream
git rev-list --no-merges --count upstream/main..main
git diff --numstat upstream/main main | awk '{a=$1+$2; split($3,p,"/"); k=p[1]; if (k=="internal"||k=="plugins"||k=="native"||k=="cmd"||k=="ui") k=p[1]"/"p[2]; s[k]+=a; t+=a} END {for (k in s) printf "%6d  %s\n", s[k], k; printf "%6d  TOTAL\n", t}' | sort -rn | head -30
git diff --name-status upstream/main main -- '*.go' | awk '$1=="A" && $2 !~ /\// && $2 !~ /_test\.go$/ {print $2}'   # fork 新增的根包文件
gh pr list -R egoist/mygo --author daodao97 --state all
```
