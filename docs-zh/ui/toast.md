# Toast

<!-- preview toast 300 -->

`c.Toast` 在窗口底部附近显示一条消息几秒钟，用于说明用户刚才操作的结果：

```go
if ui.Button(c, "Save").Clicked() {
	app.save()
	c.Toast("Saved")
}
```

`c.ToastAction` 显示一条带按钮的 toast，例如删除后的“撤销”，显示时间更长：按钮会在主线程上运行该操作（与视图一样），并关闭 toast；Tab 可以到达该按钮。

```go
app.trash(note)
c.ToastAction("Note deleted", "Undo", func() { app.restore(note) })
```

已在显示的消息会重新显示；其他消息堆叠在它上方，显示最新的三条，较旧的消息隐藏等待，但计时仍在继续。指针停在 toast 上或键盘焦点在其中时，toast 的计时暂停；窗口在后台时计时继续（见[下文](#在后台)）。Escape 关闭焦点所在的 toast。Toast 会淡入淡出。

Toast 采用主题的 `Inverse` 和 `InverseText` 颜色；主题未指定这两个值时，会将主题的文字色和背景色对调用于 toast（见[主题](styling.md#主题)）。

## 添加和关闭 toast

`c.AddToast` 显示一个 `ui.Toast`，如同 Base UI 的 toast 管理器添加 toast，并返回其 ID；`c.Toast` 和 `c.ToastAction` 都是以消息作为 ID 和标题的 `AddToast`：

```go
id := c.AddToast(ui.Toast{
	Title:       "Uploading",
	Description: "report.pdf",
	Timeout:     -1, // until closed
})
// ...
c.CloseToast(id)
```

| 字段 | 作用 |
|---|---|
| `ID` | 标识 toast：用相同 ID 再次添加，会更新正在显示的 toast 并重新显示；为空时自动生成 ID |
| `Title`、`Description` | 显示的内容，屏幕阅读器会在它显示时读出 |
| `Type` | toast 的类型，如 `"error"`，用于区分显示样式 |
| `Timeout` | 它显示多久：为零时 4 秒，为负时直到关闭 |
| `Action`、`OnAction` | 一个按钮，运行 `OnAction` 并关闭 toast，除非 `OnAction` 添加了相同 ID 的 toast |
| `OnClose` | 在它关闭时运行：计时结束，或者用户、它的操作或 `CloseToast` 关闭了它 |
| `Data` | 自定义样式所需的其他数据 |

`c.CloseToast("")` 关闭所有 toast。

## 在后台

窗口在后台或隐藏时，toast 的计时继续，因为 toast 传达的是会过时的消息，例如托盘应用在主窗口保持隐藏时完成下载或重启。到窗口再次显示时计时已结束的 toast 会在那一帧关闭，运行它的 `OnClose`，但不会显示。指针停在 toast 上仍会暂停计时；窗口具有键盘焦点时，焦点在 toast 中也会如此。用户必须看到的消息会一直显示到关闭（`Timeout` 为负）。

## 无外观

`ToastViewportBase` 用你自己的外观代替主题外观构建窗口的 toast，如同 Base UI 的 Toast 部件：它的 `fn` 为视口设置样式，视口是覆盖窗口的一列，指针可以穿过，toast 位于它的底部（`Justify` 和 `AlignItems` 移动它们），并用 `ToastBase` 构建每个正在显示的 toast，辅助技术把它的 `Root` 看作一个状态，它的 `ActionButton` 和 `CloseButton` 分别运行操作和关闭 toast。在视图中的任意位置调用一次即可。位于右下角的 toast：

```go
ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
	viewport.Padding(16).AlignItems(ui.End).Gap(8)
	for _, t := range toasts {
		toast := ui.ToastBase(c, t)
		toast.Root.Row().Gap(12).Padding(10, 14).Radius(8).Background(surface).Border(1, border)
		toast.Root.Transition(ui.ElementTransition{Enter: &ui.Motion{Y: 8}, Exit: &ui.Motion{}})
		toast.Root.Children(func() {
			if t.Type == "error" {
				ui.Icon(c, alert).TextColor(danger)
			}
			ui.Text(c, t.Title).Grow(1)
			if t.Action != "" {
				toast.ActionButton().Children(func() { ui.Text(c, t.Action) })
			}
			toast.CloseButton().Label("Close").Children(func() { ui.Icon(c, x) })
		})
	}
})
```

`toasts` 是正在显示的 toast，即最新的三条，最旧的在前。`toast.Left()` 表示一个 toast 还会显示多久，可用于倒计时；一直显示到关闭的 toast 为 0。计时和暂停规则与主题外观相同。没有 toast 显示时视口仍保留，好让最后一个 toast 走完退出过渡。

## 辅助功能

屏幕阅读器在 toast 显示时读出它的标题和描述（见[播报](accessibility.md#播报)）；辅助技术把它看作一个状态，其中包含它的按钮。
