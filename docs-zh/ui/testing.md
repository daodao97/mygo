# 测试

`ui.NewTester` 无需窗口即可运行视图，速度与单元测试相当：它在内存中渲染帧，按文本查找元素，并执行单击、键入、滚动和按键。

```go
func TestCounter(t *testing.T) {
	app := &counter{}
	tt := ui.NewTester(app.view, 320, 240)
	if err := tt.Click("Increment"); err != nil {
		t.Fatal(err)
	}
	if app.n != 1 || !tt.HasText("1") {
		t.Errorf("count %d, texts %q", app.n, tt.Texts())
	}
}
```

元素按文本查找，没有文本的则按 `Label` 查找，例如图标按钮：`tt.Click("Close")`、`tt.Find("Volume")`、`tt.Focused("Name")`。

`Click`、`ClickWith` 和 `RightClick` 会先检查第一个匹配元素，再发送输入。如果元素没有可见区域，或可见区域的中心实际命中其他控件或空白处，就返回错误；不会自动滚动使其可见，也不会另找未被遮挡的位置。列表行等元素如果没有自己的上下文菜单，`RightClick` 会打开点击位置处子内容的菜单，与实际右键单击一致；但内部能独立接收点击或输入的控件仍视为其他控件。`Find` 仍会返回匹配元素裁剪后的边界框，宽度或高度可能为零；`ClickAt` 直接在指定坐标发送输入，不指定目标名称。

同一父元素下两个元素被赋予相同的 `Key` 时会共享同一个状态，因此本应给其中一个的单击或焦点会去到另一个：测试器会在赋予第二个 key 的位置 panic，而应用只会记录它。

## 测试器做什么

- **指针。** `tt.Click`、`tt.ClickAt`、`tt.ClickWith(ui.Shift, "Row 3")`、`tt.Press`、`tt.Move` 和 `tt.Release`、用 `tt.PressButton` 和 `tt.ReleaseButton` 操作另一个按钮（例如右键拖动用 `tt.PressButton(1, x, y)`）、`tt.CancelPress`（像系统取消按下那样结束它，不产生单击）、`tt.Scroll`（像触摸板那样立即滚动）和 `tt.Wheel`（鼠标滚轮的一格，会随着时钟前进而滑向目标，见 `tt.Advance`）。`tt.UsePointer(ui.PointerPen, 0.6)` 让之后的按下和移动来自以该压力按下的笔，或手指（`ui.PointerTouch`）。
- **键盘。** `tt.Key(ui.Cmd, ui.KeyS)` 按下一个键，`tt.KeyRepeat` 在按住时像自动重复那样再次按下它，`tt.TypeKey` 输入一个键及它产生的文本，`tt.Type` 向具有焦点的元素输入文本。`tt.Compose` 显示输入法的组合状态，`tt.Command` 执行菜单的编辑命令，如 `"copy"`，`tt.TextCaret` 返回输入法将会组合的位置。
- **菜单。** `tt.RightClick` 打开上下文菜单，`tt.Menu` 列出菜单，`tt.ChooseMenuItem("Move to", "Archive")` 从中选择，`tt.CloseMenu` 关闭它：系统的菜单，或[在窗口中绘制](context-menu.md#在窗口中绘制)的菜单，因此测试在两种菜单下都能通过。
- **窗口。** `tt.SetSize`、`tt.SetScale`、`tt.SetZoom`（与 `Context.SetZoom` 一样[缩放](windows.md#缩放)内容；`Find`、`ClickAt`、`Move` 和 `TextCaret` 仍使用窗口坐标）、`tt.SetDark`、`tt.SetPreferences` 和 `tt.SetDirection`（[从右到左](right-to-left.md)布局）可修改窗口和桌面设置。`ui.NewTesterWith(view, w, h, ui.TesterOptions{Preferences: p, Dark: true})` 可指定初始桌面设置，让第一帧就遵循这些设置，类似 Media Queries 5 从首次解析样式时就应用用户偏好：这样在减少动效模式下创建的视图不会在最初几帧启动动画；而使用 `NewTester` 时，还没来得及调用 `SetPreferences`，初始帧就已运行至稳定。`tt.SetFocused` 可移除或恢复窗口的键盘焦点；`tt.SetWindowActive` 可让窗口按非活动或活动状态绘制，模拟其他窗口接收用户输入的情况（[非活动窗口](windows.md#活动与非活动窗口)）；`tt.SetWindowHidden` 可完全隐藏或重新显示窗口（[隐藏的窗口](windows.md#隐藏的窗口)）。
- **它显示的内容。** `tt.Texts`、`tt.HasText`、`tt.Find`（文本所在的盒子）、`tt.Focused`、`tt.Cursor`、`tt.Clipboard`、`tt.OpenedURLs` 和 `tt.Announcements`，即视图请求屏幕阅读器朗读的内容（[实时区域与播报](#实时区域与播报)）；`tt.FailOpenURL` 让它打开的链接失败，如同没有任何应用能打开它们，`tt.FailClipboard` 让它向剪贴板写入文本失败，如同浏览器拒绝该操作（`WriteClipboardThen`）。
- **辅助技术看到什么。** `tt.Access` 和 `tt.AccessByRole` 读取元素的角色、名称、值和状态，`tt.AccessAll` 则读取所有元素的这些信息（[见下文](#辅助技术看到什么)）。
- **帧。** `tt.Frame` 绘制一帧，用于视图之外发生变化时，`tt.Image()` 是最后一帧，用于快照；`ui.Render` 在给定缩放下绘制一次视图。输入和 `tt.Frame` 会运行视图请求的帧，直到它不再请求，或运行 20 帧：`tt.FramePending` 说明它是否仍在请求，例如一个动画不停或永远无法稳定的视图。
- **时间。** 测试器使用独立时钟 `tt.Now()`，只有调用 `tt.Advance(d)` 才会前进 `d`，并按预定时间运行期间到期的帧：动画进行时每次刷新一帧，`c.After` 和 `Painter.After` 请求的帧到期时运行。所有依赖时间的行为都使用该时钟，包括动画、`Context.After`、`Context.Now`、双击判定、连续输入的撤销分组、插入符闪烁、工具提示和短暂提示。这样，无论机器繁忙还是运行很快，测试结果都一致：不要用睡眠等待，而应推进时钟。`Advance` 只运行视图请求的帧；从视图外部修改显示内容后，应先调用 `tt.Frame`。`editor.Buffer` 和 `richtext.Document` 的撤销分组使用系统时钟。

  ```go
  tt := ui.NewTester(app.view, 320, 240)
  tt.Click("More")
  tt.Advance(100 * time.Millisecond) // half way through a 200 ms animation
  tt.Advance(time.Second)            // and done
  if tt.FramePending() {
  	t.Error("the panel keeps asking for frames")
  }
  ```

## 实时区域与播报

`tt.Announcements()` 返回视图通过 `c.Announce` 请求朗读的内容，并清空记录。要测试实时区域的播报，可用 `tt.UseScreenReader()` 启用屏幕阅读器模拟：此后每帧的辅助功能树都会交给它处理。与 ARIA 一样，变化后的 `RoleStatus` 文本和新增的 `RoleLog` 内容会在不打断当前朗读的情况下播报一次，与 `c.Announce` 的文本一起出现在 `tt.Announcements()` 中；`RoleAlert` 则立即播报，记录在 `tt.Alerts()` 中。启用前已显示的内容不会补读，与真实屏幕阅读器开始读取窗口时一致。

```go
tt := ui.NewTester(view, 400, 300)
tt.UseScreenReader()
tt.Click("Save")
if got := tt.Announcements(); !slices.Equal(got, []string{"Saved 2 files"}) {
	t.Errorf("read out %q", got)
}
```

## 辅助技术看到什么

`tt.AccessByRole(role, name)` 返回屏幕阅读器从最后一帧中第一个具有该角色和可访问名称的元素（`""` 表示任意名称）得到的内容，如同 Testing Library 的 `getByRole(role, { name })` 和 Playwright 的查找方式，`tt.Access(name)` 返回第一个具有该名称的元素，无论其角色。它们返回的 `ui.AccessInfo` 包含角色、名称、值、描述和 `RoleDescription`，以及各项状态，如[辅助功能](accessibility.md#状态)列出的：`Checked` 和 `Mixed`（`aria-checked`、`aria-pressed`）、`Selected`（`aria-selected`，被选中的标签页、树项、行或网格单元格，带 `Selected` 或 `AccessSelected`）、`Expanded`、`HasPopup`、`Busy`、`Current`、`AutoComplete`（`aria-autocomplete`）、`Sort`（列标题的 `ui.SortDirection`）、`Disabled`、`Invalid`、`Focused`（焦点所在的元素或选项，包括 `ActiveDescendant`）、`Controls`（它控制的元素的名称，`Access` 能找到它们）、`DescribedBy` 和 `ErrorMessage`（描述它的元素和保存它错误的元素的名称）、`Vertical` 和 `Horizontal`（`aria-orientation`）、`Level`、`PosInSet` 和 `SetSize`（`aria-posinset`、`aria-setsize`，如 `PositionInSet` 给出的）、`Multiselectable`、`RowCount`、`ColumnCount`、`RowIndex` 和 `ColumnIndex`（[网格](accessibility.md#网格)的）、`RowSpan` 和 `ColumnSpan`（其单元格的）、范围的 `Min`、`Max` 和 `Now`，以及它的 `Bounds`（窗口 DIP 中的可见区域，与 `Find` 一样经过裁剪）。这样测试检查的是状态确实到达了辅助技术，而不只是视图设置了它：

```go
tt := ui.NewTester(app.view, 320, 240)
reply, ok := tt.AccessByRole(ui.RoleStatus, "Reply")
if !ok || !reply.Busy {
	t.Errorf("Reply while streaming: %+v, found %v", reply, ok)
}
if docs, _ := tt.AccessByRole(ui.RoleLink, "Docs"); docs.Current != ui.CurrentPage {
	t.Errorf("Docs is not the current page: %+v", docs)
}
```

`tt.AccessAll()` 按屏幕阅读器的读取顺序，返回最后一帧所有元素的上述信息，可用于检查整个视图，例如确保每个控件都有名称：

```go
for _, a := range tt.AccessAll() {
	if a.Role == ui.RoleButton && a.Name == "" {
		t.Errorf("a button at %v has no name", a.Bounds)
	}
}
```

角色是元素被赋予的角色（来自它的控件或 `Role`），或根据它的作用推断出的角色：文本、按钮、组。树按屏幕阅读器会得到的样子构建，无论是否有屏幕阅读器读取窗口，读取它都不会改变任何东西：它不会构建帧。

## 测试构建后的应用

测试器在 `go test` 中运行视图。要像端到端测试运行器或智能体 一样，从另一个进程测试构建后的应用及其真实窗口，可在测试构建中导入[自动化插件](../plugins/automation.md)：它像测试器的 `Find` 一样，按标签或 `Key` 查找各窗口中的元素，执行操作、发送输入，并根据帧是否仍待处理来等待，类似 `FramePending`。
