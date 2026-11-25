# 分栏视图

<!-- preview split 270 -->

`ui.Split` 将两个窗格并排显示，中间的分隔条可拖动，也可在获得键盘焦点后用方向键调整位置。`*float32` 保存第一个窗格的宽度，分隔条与两侧边缘至少相距 40 DIP。空间不足时，第一个窗格会缩小，为第二个窗格保留 40 DIP；空间充足后再恢复原宽度，而 `*float32` 始终保存用户设置的宽度。通过 `Changed` 可检测分隔条的位置变化。

```go
ui.Split(c, &app.sidebarWidth, func() { app.files(c) }, func() { app.editor(c) }).Fill()
```

`ui.SplitVertical` 把第一个窗格放在第二个上方，`*float32` 表示它的高度：

```go
ui.SplitVertical(c, &app.consoleHeight, func() { app.editor(c) }, func() { app.console(c) }).Grow(1)
```

`ui.SplitPercent` 和 `ui.SplitVerticalPercent` 则取第一个窗格所占空间的百分比，从 0 到 100：两个窗格会随窗口按比例增大和缩小，如同编辑器下方的结果面板。

```go
ui.SplitVerticalPercent(c, &app.editorPercent, func() { app.editor(c) }, func() { app.results(c) }).Grow(1)
```

`SplitSecond` 让 `*float32` 改为表示第二个窗格的尺寸，适合位于窗口末端、尺寸保持不变的面板，第一个窗格则占据剩余空间：例如聊天应用右侧的成员列表、检查器面板，或编辑器下方的终端。第二个窗格在 `Split` 中位于右侧（从右到左布局时位于左侧），在 `SplitVertical` 中位于底部。分隔条的行为不变：方向键按所指方向移动分隔条，并与两侧边缘保持至少 40 DIP。窗口缩小时，第一个窗格先缩小到 40 DIP，之后第二个窗格才会缩小，待空间充足时再恢复原尺寸。对于百分比变体，百分比表示第二个窗格所占的比例；`SplitReset` 也会把第二个窗格恢复为指定值。

```go
ui.Split(c, &app.membersWidth, func() { app.chat(c) }, func() { app.members(c) }).SplitSecond().Fill()
ui.SplitVertical(c, &app.terminalHeight, func() { app.editor(c) }, func() { app.terminal(c) }).SplitSecond().Grow(1)
```

把尺寸保存在应用状态中并持久化，以便应用再次启动时恢复布局。指针在分隔条上会显示调整大小的光标。

设置 `SplitReset` 后，双击分隔条会将第一个窗格恢复为应用指定的尺寸，单位与分栏类型一致：`Split` 和 `SplitVertical` 用 DIP，百分比变体用百分比。未设置时，双击不会改变尺寸。即使窗口太小，无法容纳指定尺寸，也仍会保留该值，待空间充足时恢复，这与按下分隔条调整尺寸时的行为相同。

```go
ui.Split(c, &app.sidebarWidth, func() { app.files(c) }, func() { app.editor(c) }).SplitReset(220).Fill()
```

`SplitGap` 用指定的 DIP 间距隔开两个窗格，不再绘制 1 DIP 的分隔线，适合并排卡片或各自绘制边框的窗格：`SplitGap(0)` 让两个窗格无缝紧贴，正值则在两者之间留出对应空隙，调整大小的拖动手柄位于其中。

```go
ui.Split(c, &app.sidebarWidth, func() { app.files(c) }, func() { app.editor(c) }).SplitGap(8).Fill()
```

## 辅助功能

辅助技术把分隔条看作名为 "Divider" 的分隔条，它可以获得焦点，并随方向键移动。它的值是分隔条距分栏起始边的位置，使用 `SplitSecond` 时也是如此，因此增大该值会让分隔条移向末端。
