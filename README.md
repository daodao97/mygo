# MyGo Mobile

**用 Go 写桌面和移动端应用：原生 UI 或网页前端，同一套代码跑在 macOS、Linux、Windows、iOS 和 Android 上。**

MyGo Mobile 是 [egoist/mygo](https://github.com/egoist/mygo) 的 fork（仓库 `daodao97/mygo-mobile`，原名 `daodao97/mygo`，旧地址自动重定向）。上游提供桌面端的系统 webview 与 Go 原生 UI（无 cgo、单个二进制、类型化 IPC）；本 fork 在此之上增加了 **iOS / Android 原生 UI** 和一批桌面扩展，并持续合并上游。
与上游差异的完整记录、合并规则和进行中的 PR 见 [FORK.md](FORK.md)。

## 基于本 fork 的应用

| 应用 | 平台 | 说明 |
|---|---|---|
| [Miya](https://github.com/daodao97/miya) | macOS + iOS / Android | 在一个工作区里并行管理多个编码 agent 会话（Codex、Claude Code），手机端远程查看、接管和接收推送；`plugins/desktopcontrol`、`Microphone` 等功能都从这里下沉而来 |
| [Retty](https://github.com/daodao97/retty) | macOS + iOS | 终端工具，桌面与手机同构，使用 fork 的终端插件扩展（触摸选择、软键盘、远程固定网格） |

## 为什么有这个 fork

上游只面向桌面（macOS、Linux、Windows）和浏览器。本 fork 把同一套 Go 原生 UI 跑到手机上，让一个应用的界面和逻辑在桌面与移动端同构，例如终端工具 [Retty](https://github.com/daodao97/retty)（macOS + iOS）和 [Miya](https://github.com/daodao97/miya)。

移动端曾提交给上游，被维护者婉拒：

| PR | 内容 | 结果 |
|---|---|---|
| [egoist/mygo#157](https://github.com/egoist/mygo/pull/157) | Add experimental iOS native UI and mobile build tooling：UIKit 后端、触摸/手势/输入法、移动端系统能力、APNs、`mygo build -platform ios`、Xcode 工程生成与签名导出（206 个文件，+19,699 行） | 2026-10-08 关闭，未合并。维护者的答复：*“I don't want to maintain ios/android support, and I also prefer native ui components on mobile device.”* |

2026-11-17 起移动端以上游 #940（`internal/mobile`，Swift/Kotlin 宿主内嵌在 CLI）为准；fork 原来的 iOS/Android 后端已删除，fork 独有、尚未移植到 #940 的移动端扩展见 [FORK.md](FORK.md)。

## 比上游多出的功能

**移动端（仅 fork）**

- **iOS**：UIKit 宿主 + Go 视图，CoreText 排版、Metal 渲染（CPU 回退），安全区、键盘、显示链路帧调度（支持 ProMotion 120 Hz）、场景生命周期与状态恢复；`mygo build -platform ios/arm64`、模拟器构建、签名、IPA 导出与 dSYM。见 [docs/ios.md](docs/ios.md)。
- **Android**：JNI 后端与 Gradle 宿主工程，Surface/EGL 恢复、生命周期与深链、原生编辑菜单与选区手柄、HarfBuzz 文字排版；构建脚本见 `scripts/android-*.sh`。
- **共享 UI 的触摸支持**：触摸滚动与弹性滚动、捏合/旋转手势、交互式路由返回、输入法与输入附件栏、原生选区/光标几何。
- **移动端系统能力**：推送（APNs、Live Activity 及 `push/` 发送端）、生物识别、权限、相机扫码、分享、语音与麦克风 PCM 流、App Group、后台刷新/后台任务、触觉、Keychain/Keystore 上的 `Secrets`。
- 配套包：`sharedstore`、`generationcache`、`async`、`backgroundtask`、`timeseries`、`ui/chart/sparkline`。

**对上游的通用扩展（桌面也可用）**

- `plugins/desktopcontrol`：macOS 显示器/窗口枚举、ScreenCaptureKit 截图、键鼠输入、辅助功能对话框（纯 Go）。
- 网络路径状态 `Network.PathStatus` / `OnPathChanged`。
- 路由 `Router.Reset` / `History` / `Transitioning` 与 JSON 持久化。
- inline combobox、`DecodeBitmapSize`（按显示尺寸解码图片）、`LastInput`。
- 应用状态持久化 `PersistState`、`OnFirstFrame`、`ui.Startup`。
- 诊断：`FrameReading` / `FrameAverage` / `HeapReadingNow`、`MYGO_FRAME_WHY` 帧原因追踪、`LoopFPS`、`internal/probe` 性能探针。
- 终端插件：`AdaptiveColors`、`SelectOnDrag`、`SelectedText` / `SelectAll` / `CopyRawText`、远程固定网格（`FixedCols`、`ReadScreen` 等）、TUI 硬换行路径识别。
- `updater.InstallUpdate`、`automation.Start`。

完整清单与代码位置见 [FORK.md](FORK.md) 的「差异分类」。

## 与上游的同步策略

- **同一功能两套实现时，以上游为准。** 上游落地了 fork 也有的功能，就删掉 fork 的版本，把调用方（Miya、myclash 及 fork 自身）迁到上游 API，不因为 fork 先做了而保留。上游没有的功能继续保留。
- **定期合并**：`git fetch upstream && git merge --no-ff upstream/main`，冲突按上一条处理；合并后编译桌面、iOS、Android 与下游应用，再快进推送 `main`，从不强推。上游给 `platform` 接口新增的方法要在 iOS/Android 后端补上。
- **合并后人工排查**：git 不会提示“同一功能、无文本冲突”的重叠，需要对照上游本次改动找出来按规则处理，并更新 FORK.md。
- **回馈上游**：通用的修复和功能从 `upstream/main` 上的独立 worktree 提取，剥离移动端代码，以草稿 PR 提交。
- 桌面构建始终保持 `CGO_ENABLED=0`；cgo 只用于移动端。

## 方案的可持续性

上游作者婉拒移动端时说更倾向于在手机上使用系统原生控件（UIKit / SwiftUI、Android View / Compose）。本 fork 走的是另一条路：界面由 MyGo 用 Go 自绘（Metal / OpenGL 渲染，CoreText / HarfBuzz 排版），iOS 和 Android 只提供宿主与绘制表面，与 Flutter 同类；同时把弹窗、分享、选择器、通知、钥匙串、编辑菜单与选区手柄等体验敏感的部分交给系统真正的原生组件。

这条路的成本和收益结构不同：

- **追平系统行为主要是前期投入。** 触摸滚动、选区、输入法、键盘避让、无障碍映射做完一遍后，剩下的是每年随系统版本的小幅跟进（弃用 API、新交互、新视觉风格）。
- **一份代码跨平台的收益是持续的。** 应用每加一个页面或功能，桌面、iOS、Android 共用同一份 Go 视图代码，不必各写一遍；应用越多、功能越多越划算。平台行为修在 fork 的后端和 `ui` 层，修一次所有应用一起受益。

为了让持续成本保持在小幅跟进的量级：

- **高频小步同步上游**，并以上游为准消除重复实现，不让分叉累积。
- **移动端代码尽量是纯新增**（`internal/ios`、`internal/android` 等独立目录），对共享代码的扩展优先做成可选接口；能对桌面也有用的挂点争取提交上游，缩小冲突面。
- **每次同步都编译 iOS / Android**：上游给 `platform` 接口加的方法桌面编译发现不了（例如 `Clipboard.WriteData` 新增的 `concealed` 参数），必须在同步当天补上。
- **每年系统大版本后集中清理一次**弃用 API，并适配新的系统行为。
- 在 [FORK.md](FORK.md) 记录每次同步的冲突处理、迁移和已知问题，作为维护的单一信息来源。

## 提交到上游的 PR

| PR | 内容 | 状态 |
|---|---|---|
| [#157](https://github.com/egoist/mygo/pull/157) | iOS 原生 UI 与移动端构建工具链 | 已关闭（上游不维护移动端） |
| [#928](https://github.com/egoist/mygo/pull/928) | macOS 空闲窗口不再唤醒 Go | 已合并 |
| [#1057](https://github.com/egoist/mygo/pull/1057) | Markdown 中 CJK 旁的 `*` / `~` 强调 | 已合并 |
| [#1058](https://github.com/egoist/mygo/pull/1058) | 链接悬停下划线完整穿过下行字母 | 已合并 |
| [#1076](https://github.com/egoist/mygo/pull/1076) | 终端可选的 zsh shell integration | 关闭，上游在此基础上另行实现 |
| [#1301](https://github.com/egoist/mygo/pull/1301) | 窗口空闲时释放文本 shaping 缓冲 | 关闭，上游已有等价的 `System.ReleaseShaping` |
| [#1302](https://github.com/egoist/mygo/pull/1302) | 终端识别中文文本中的路径、带引号路径与行列定位 | 已合并 |
| [#1305](https://github.com/egoist/mygo/pull/1305) | 终端同步更新 1 秒后放行 | 已合并 |
| [#1525](https://github.com/egoist/mygo/pull/1525) | `plugins/desktopcontrol` 官方插件 | 草稿，进行中 |
| [#1530](https://github.com/egoist/mygo/pull/1530) | macOS 原生 popover 跟随应用外观 | 已合并 |

## 使用本 fork

模块路径仍是 `github.com/egoist/mygo`，在应用的 `go.mod` 里用 `replace` 指向本 fork：

```
replace github.com/egoist/mygo => github.com/daodao97/mygo-mobile <version>
```

## 文档

- 框架本身的用法沿用上游文档：[docs/README.md](docs/README.md)，以及 [上游仓库](https://github.com/egoist/mygo)。
- iOS：[docs/ios.md](docs/ios.md)；Android：`AGENTS.md` 的 Runbook 与 `scripts/android-*.sh`。
- 架构、线程模型与添加功能的清单：[docs/architecture.md](docs/architecture.md)。

## License

MIT
