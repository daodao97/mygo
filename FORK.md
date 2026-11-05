# Fork 与上游的差异

本仓库是 `egoist/mygo`（上游，remote `upstream`）的 fork，`daodao97/mygo`（remote `origin`）。本文件记录两者的差异、处理规则和进行中的工作，供维护 fork 的 agent 先读。数字为 2026-11-05 的快照，用文末的命令重新统计。

## 快照（2026-11-05）

- fork `main`：`d2a71494`；`upstream/main`：`977ea57e`；fork 落后上游 0 个提交。
- fork 独有的非合并提交：110 个；`git diff upstream/main main` 约 58,000 行。
- 约 2/3 是 iOS/Android，其余是对上游的扩展和待提交回上游的修复。共享代码中已没有已知的重复实现（见「已清理的重复实现」）。

## 规则

1. **同一功能两套实现时，以上游为准。** 删除 fork 的实现，下游改用上游 API。fork 独有的功能（上游没有的）保留。
2. **上游不需要 iOS/Android 功能。** 移动端代码不作为上游 PR 的候选；从混合提交中提取 PR 时要剥离移动端部分。
3. **合并上游时**，同一功能的冲突按规则 1 解决，再迁移调用方。不要因为 fork 先做了就保留 fork 的设计。
4. 桌面构建不使用 cgo（见 `AGENTS.md`）；移动端可以用 cgo。

## 使用方

| 项目 | 依赖方式 | 影响 |
|---|---|---|
| Miya | `replace github.com/egoist/mygo => ../mygo` | fork `main` 改动立即生效；改公开 API 前先编译 Miya |
| myclash | `replace => ../mygo` | 同上；`build.noindex/` 下有旧的 mygo 源码备份，`go build ./...` 会报错，编译时排除 |
| gorex、retty、gorex-workflows | 固定 fork 的伪版本（`80d5e6a3`、`d30cf320`） | 终端用的是自己的 `retty/internal/terminal` 副本，不依赖本仓库的 `plugins/terminal`；升级 mygo 时再检查 |

在不切换本仓库分支的情况下，用分支验证下游：

```sh
printf 'go 1.27.3\n\nuse %s\n\nreplace github.com/egoist/mygo => %s\n' "$APP" "$BRANCH_WORKTREE" > /tmp/w.work
(cd "$APP" && GOWORK=/tmp/w.work go build ./...)
```

## 差异分类

### A. 移动端（保留，不提交上游）

| 范围 | 内容 |
|---|---|
| `internal/android`、`android/`、`internal/ios`、`native/ios` | 两个平台的后端、JNI/Objective-C 桥接、宿主工程 |
| `push/`（APNs、Live Activity）、`cmd/mygo` 中的 iOS 打包与扩展 | 推送发送端、`mygo push setup/status`、iOS 构建 |
| 根包模块 | `app_group`、`background_refresh`、`biometrics`、`device`（扫码、键盘）、`lifecycle`（`OnDidEnterBackground` 等）、`mobile*`、`mobile_experience`、`permissions.go` 中的 `Permissions` 模块（桌面后端返回 unsupported）、`share`、`speech`、`push.go`、`task.go` |
| 独立包 | `sharedstore`、`generationcache`、`async`、`backgroundtask`、`timeseries`、`ui/chart/sparkline` |
| `ui/` | `touch*`、弹性滚动、`native_text_scope`、`input_accessory`、`input_options`、`text_input_client`、`gesture`、`router_touch`、SafeArea |
| `plugins/terminal` | `software_keys`、`input_context`、`ActiveCursor`、`SelectionAnchor`、`InsertNewline`、`OnSubmit`、触摸选择 |
| `Secrets` 的 iOS/Android 后端 | `internal/ios/secrets.{go,m}`、`internal/android/backend/secrets.go` 实现上游的 `platform.Secrets` |

### B. 对上游的扩展（保留；通用的可以提交上游）

| 功能 | 位置 | 备注 |
|---|---|---|
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
| 终端：TUI 硬换行与缩进续行的路径识别（`hardWrappedFileAt`） | `plugins/terminal/links.go` | 依赖 `vt.RowsJoined`、`RowText` |
| 终端：快照保留软换行和超链接目标 | `internal/vt/snapshot_links.go` | 提交 `c55486a2` |
| `internal/probe` | 性能探针 | 主要用于 Android |

### C. 等上游合并后就会消失的差异

| 差异 | 上游 PR | 本地分支 | 上游合并后 |
|---|---|---|---|
| 窗口空闲时释放文本 shaping 临时缓冲区（`System.ReleaseScratch`），Core Text 属性缓存命中不再构建临时对象 | #1301 | `pr/text-idle-scratch` | 同步时 `internal/text/{engine,layout,coretext_darwin,dwrite_windows,edit}.go`、`ui/window.go` 取上游 |
| 终端链接识别：Unicode 路径、带引号的路径、`(行,列)` 和 `#L行C列` | #1302 | `pr/terminal-file-links` | fork 的 `matchLink` 与上游的 `pathAt` 是同一功能，按规则 1 改用上游；硬换行续接（B 类）保留并改为调用上游 |
| 同步更新时光标隐藏也能在 1 秒后放行（`view.go` 中的 `c.After`） | #1305 | `pr/terminal-sync-update-wake` | 同步时 `view.go` 这一段取上游 |

已被上游合并的 fork 改动：#928（macOS 空闲唤醒）、#1057（CJK 强调）、#1058（链接实线下划线）、#1077（zsh shell integration，上游在其基础上做了修改）。

## 已清理的重复实现（2026-11-05）

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

此外，滚动条策略在更早的合并中已经采用上游的“悬停时显示”和 `KeepScrollbars`。`93e7fb70`（延迟帧计时）优化的是 fork 自己的 API，不是上游的。

## 待提交上游的候选（尚未提交）

- 搜索匹配以宽字符结尾时，上游的 `viewportMatches` 少高亮最后一个单元格（fork 原来的 `Highlights` 处理过）。
- reflow 后上游的搜索保留了匹配，但丢失了选中状态（查找栏显示“0 of 1”）。
- `508037e1`：生成的 facade 把 nil 回调包装成非 nil，导致 `InputModifiers(mods, nil)` panic；需要先确认上游是否已修复。
- `8256d934`：原生 popover 外观跟随应用主题（macOS）。
- `fae3d440`：frame-why 诊断和 `LoopFPS`（提交时去掉 `AGENTS.md` 的改动）。
- Metal 连续阴影捷径：补丁在 `~/.cache/mygo-pr/metal-continuous-shadows.patch`。和上游 #910 的 shader 一起编译后，float16 结果比完整公式差 3 ULP（测试只允许 1 ULP），不是编译器版本造成的，需要调整 shader 写法。

已核实**不是**上游问题、不要再提的：渲染路径日志滞后（`d2a71494`）；`FrameAverage` 计时开销（fork 自己的 API）；滚动条“滚动时才显示”（和上游设计冲突，已放弃）。

## 已知问题

- `GOOS=android CGO_ENABLED=0 go build .` 失败：`linkDMABUFProbe`、`linkNotificationImages` 未定义。合并上游前就已存在，fork 的 Android CI 会失败。
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
git push origin main                                # 快进推送，不用 force
```

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
