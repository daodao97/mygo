# 上下文菜单

<!-- preview context-menu 180 -->

`ContextMenu` 为元素提供系统上下文菜单，右键单击元素（macOS 上也可按住 Control 单击）时在指针位置打开；元素或其后代具有焦点时，按菜单键或 Shift+F10 则在元素下方打开；列表具有焦点且选中某行时，也可用这两个按键打开该行的菜单。函数在菜单打开时构建菜单项，并在选中某项后的下一帧再次运行，由 `Chosen` 报告所选项：

```go
row.ContextMenu(func(m *ui.Menu) {
	if m.Item("Rename").Shortcut(0, ui.KeyF2).Chosen() {
		app.renaming = i
	}
	if m.Item("Pinned").Checked(note.pinned).Chosen() {
		note.pinned = !note.pinned
	}
	m.Submenu("Move to", func(m *ui.Menu) {
		for _, f := range app.folders {
			if m.Item(f.name).Chosen() {
				moveTo, moved = f, i
			}
		}
	})
	m.Separator()
	if m.Item("Delete").Disabled(note.locked).Chosen() {
		deleted = i
	}
})
```

函数在元素构建时运行，位于构建各行的循环中：如果某个选择会把这一行从集合中移走（例如移动或删除它），就先记下它，等循环结束后再做修改（见
[视图](views.md#事件与动作)）。

## 菜单项

- `Item(label)` 添加一个菜单项；`Chosen` 报告它是否被选中。
- `Checked(on)` 在它旁边显示对勾，用于选中该项即可切换的设置；`Radio(on)` 把它
  显示为在一组中选定一个设置的项之一，on 为 true 时标记为选中的那个。
- `Disabled(d)` 把它变灰：无法选中它。
- `Tooltip(s)` 在指针停留其上时显示 `s`，说明用途。macOS 和 Linux 的系统菜单
  支持此提示，`PopoverMenu` 则在所有平台上支持，键盘高亮停留在该项时也会显示。
- `Icon(icon)` 在标签旁显示图像，用
  [`mygo.NewMenuIcon`](../menus.md#图标) 从 PNG 图像创建一次，或用
  `mygo.NewMenuIconSVG` 从 SVG 创建。系统的菜单像显示 `mygo.MenuItem` 的
  图标那样显示它，浏览器不显示，`PopoverMenu` 在所有平台上绘制它。
- `IconColor(color)` 用 `color` 而不是文本颜色绘制它的模板图标，例如 Delete
  旁的红色垃圾桶；高亮时它采用高亮状态的文本颜色，与标签相同。只有自绘菜单
  （`PopoverMenu`、`PopoverMenuButton`，以及
  [绘制它们](#窗口的每个菜单)的窗口的每个菜单）显示它：系统的菜单按文本颜色为
  模板图标着色。
- `Shortcut(mods, key)` 在它旁边显示按键，作为执行同一操作的快捷键。它只显示
  按键：要在上下文或元素上用 `Shortcut` 处理它。
- `Subtitle(s)` 在标签下方显示第二行更小的文字，与
  [`mygo.MenuItem` 的](../menus.md)相同：在 macOS 14 和浏览器中位于标签下方，
  其他平台位于标签之后（`Label — Subtitle`），在 `PopoverMenu` 中所有平台都位于
  下方。辅助技术把它读作菜单项的描述。
- `ID(id)` 指定独立标识，`Chosen` 据此匹配菜单项，而不再根据位置和标签匹配：即使
  菜单显示期间标签变化（如 `Sync 3 Files`），或前面的菜单项被添加、移除，也能
  正确识别所选项。
- `Separator()` 在菜单项组之间添加一条分隔线，`Submenu(label, build)` 添加一个
  打开子菜单的菜单项，子菜单由 `build` 填充。

## 哪个元素

单击由最内层带菜单的元素处理，例如行和行内按钮各有菜单时，会打开按钮的菜单。[文本输入](text-input.md)提供所属平台文本框的编辑命令，可选择的
[文本](text.md)有 Copy 和 Select All。输入框、文本或 `Selectable` 容器上的
`ContextMenu` 替换那个菜单，`EditItems` 把这些命令加入自己的菜单，作用于
被单击的文本，例如聊天消息的文本在 Copy 上方提供 Reply：

```go
message.Selectable().ContextMenu(func(m *ui.Menu) {
	if m.Item("Reply").Chosen() {
		app.reply(msg)
	}
	m.Separator()
	m.EditItems()
})
```

文本中的元素（如段落里的链接）会在那段文本上打开菜单：链接的 `ContextMenu`
中的 `EditItems` 会加入段落的 Copy 和 Select All，它们作用于选区。

用 `Element.Menu` 可以在按钮下方打开同样的菜单：见[菜单按钮](menu-button.md)。在
测试中，`tt.RightClick` 打开上下文菜单，`tt.Menu` 列出它，`tt.ChooseMenuItem`
从中选择。

## 在窗口中绘制

`PopoverMenu` 接受同样的函数，MyGo 会在系统显示自身菜单的位置，以主题外观把菜单
作为[浮层](popover.md)绘制在窗口中：在应用自己设计的浮层和按钮
旁边，系统菜单可能显得不协调。

```go
card.PopoverMenu(func(m *ui.Menu) {
	if m.Item("Play Next").Chosen() {
		app.playNext(album)
	}
	if m.Item("Pinned").Checked(album.pinned).Chosen() {
		album.pinned = !album.pinned
	}
	m.Submenu("Sort by", func(m *ui.Menu) {
		for _, by := range []string{"Title", "Artist", "Year"} {
			if m.Item(by).Radio(app.sort == by).Chosen() {
				app.sort = by
			}
		}
	})
	m.Separator()
	if m.Item("Remove").Shortcut(0, ui.KeyDelete).Disabled(album.locked).Chosen() {
		app.remove(album)
	}
})
```

打开位置与系统菜单一致：右键单击时以指针位置为起点，按菜单键或 Shift+F10 时以
元素下方为起点，通常向后展开，空间不足时则向前展开；窗口滚动时菜单随元素移动。
打开时获得焦点，关闭时将焦点还给原来的元素，并支持系统菜单的按键操作：

- Up 和 Down 在菜单项之间移动，越过分隔符和禁用的项，并循环；Home 和 End 跳到
  第一项和最后一项，输入菜单项的前几个字母会跳到该项。
- Right 打开菜单项的子菜单，Left 关闭它，在
  [从右到左](right-to-left.md)中方向相反；指针移到菜单项上时也会打开它。
- Enter 和 Space 选中菜单项，与单击相同；Escape 关闭最上层的子菜单，然后
  关闭菜单，Tab 关闭菜单。
- 在菜单外按下鼠标会关闭菜单，按下事件继续传给指针下的内容。

它的菜单项在标签后显示 `Shortcut`，带 `Checked` 或 `Radio` 标记的项旁显示
对勾或圆点。辅助技术看到的是一个由菜单项、复选框项和单选按钮项组成的菜单，
并知道它们是否勾选，与它看到系统菜单时一样。

自绘菜单的菜单项是元素：在测试中，`tt.RightClick` 打开它，`tt.HasText` 找到
它的菜单项，`tt.Click` 选中一个。`tt.Menu`、`tt.ChooseMenuItem` 和
`tt.CloseMenu` 在它上面的工作方式与系统菜单相同，因此测试在两种菜单下都能
通过。

`PopoverMenuButton` 以同样方式在按钮下方绘制
[菜单按钮](menu-button.md#在窗口中绘制)的菜单，并接受同样的函数：一行的
右键单击和它的 ⋯ 按钮可以共享一个菜单，绘制方式相同。

`PopoverMenuOptions` 像 `PopoverMenuButtonOptions` 设置菜单按钮那样设置菜单的
样式：`Panel` 在主题外观之后设置菜单及每个子菜单的面板，`Item` 设置每个菜单项
所在的行。这样右键菜单就能像应用的 ⋯ 菜单一样展开：

```go
text.PopoverMenu(app.messageMenu, ui.PopoverMenuOptions{
	Panel: func(panel ui.Element) {
		panel.Background(app.menuColor).Radius(12)
	},
})
```

### 更丰富的菜单项

自绘菜单的菜单项比系统菜单项支持更多内容，无需自己实现菜单
（[无外观](#无外观)）；系统的菜单只显示这些菜单项的标签：

```go
more.PopoverMenuButton(func(m *ui.Menu) {
	m.MaxWidth(320)
	for _, f := range app.files {
		if m.Item(f.Name).ID(f.Path).Trailing(f.Size).Chosen() {
			app.open(f)
		}
	}
	m.Separator()
	item := m.Item("Download").ID("download")
	item.Content(func(x ui.MenuItemContent) {
		ui.Text(x.Context, "Downloading "+app.download.Name)
		ui.Progress(x.Context, app.download.Progress)
	})
	m.Separator()
	if m.Item("Delete…").Danger().Chosen() {
		app.confirmDelete()
	}
})
```

- `Danger()` 用主题的 `Danger` 颜色显示菜单项，用于删除或丢弃之类的项。高亮时它
  保留主题的高亮色，若高亮上的 `Danger` 清晰可读，标签就用它，否则用
  `AccentText`。辅助技术听不到颜色：让标签说明菜单项做什么。
- `Trailing(s)` 在菜单项末尾、快捷键之前显示一个值，例如文件大小；辅助技术会在
  菜单项的描述中读出它。
- `Content(fn)` 构建取代菜单项标签和副标题显示的内容，位于它的标记和图标
  与尾部文本和快捷键之间。`fn` 会得到上下文、菜单项是否高亮或禁用，以及其
  文本的颜色（`MenuItemContent`）；其中的文本采用菜单项的颜色。菜单项保留
  标签，辅助技术把它读作名称，输入的文字也按它定位。内容仍是菜单项的一部分：单击
  它会选中菜单项。其中的控件（例如过滤其后菜单项的文本输入）会自行接收
  按下事件，并在指针移过所在菜单项时保持焦点，处理相应按键（输入的文本、
  Space、Enter、Home 和 End）；Up 和 Down 移动高亮并把焦点交还菜单，Escape 关闭
  菜单。
- `Highlighted()` 在函数构建时报告：菜单项是否是指针或按键所在的那一项，或通过
  子菜单通向它，以菜单最后一轮构建它时为准：函数会在高亮移动时重新构建，
  因此可以显示与该菜单项相配的内容，例如高亮主题的预览。先调用 `ID`，并在
  构建出的菜单项随高亮变化时给菜单项设置 ID：自绘菜单项的 ID 也用作其所在行的
  键，该行的高亮便会留在它上面。
- `Menu.MaxWidth(w)` 让菜单最多 `w` 宽，子菜单也是如此，除非它们自行设置：过长
  的标签、副标题和尾部文本以省略号结尾，而快捷键、图标和标记仍保留各自的空间。
  辅助技术、`tt.Menu` 和键盘输入看到的是每个标签的完整内容，`Tooltip` 也可以
  显示完整标签。

### 窗口的每个菜单

`Context.SetMenuPresentation(ui.PopoverMenus)` 以这种方式绘制窗口中的每个弹出菜单，
而不只是用 `PopoverMenu` 给出的菜单：`ContextMenu` 和 `Menu` 的菜单、MyGo 的
控件借助它们构建的菜单（[表格](table.md)的表头和单元格菜单、工具栏的溢出菜单、
代码编辑器的菜单），以及文本输入和可选文本的编辑菜单。这样，拥有自己外观的应用
在自己菜单旁边不再显示桌面的菜单，它的菜单函数也无需改动。该设置会一直保持，直到
再次设置，所以视图可以跟随应用的一项设置：

```go
func (app *App) View(c *ui.Context) {
	if app.settings.systemMenus {
		c.SetMenuPresentation(ui.SystemMenus) // the default
	} else {
		c.SetMenuPresentation(ui.PopoverMenus)
	}
	// ...
}
```

无论该设置是什么，`PopoverMenu` 和 `PopoverMenuButton` 都会在窗口中绘制它们的菜单。
只有 MyGo 构建的菜单遵循该设置：菜单栏、托盘菜单和 Dock 菜单仍由系统显示，
[webview](web-view.md) 的菜单仍由 webview 自己显示。

## 无外观

`ui.ContextMenuBase` 为元素提供一个无外观的上下文菜单，在 `*bool` 为 true 时
显示：它像 `PopoverMenu` 的菜单一样打开，位置在元素被右键单击处，按菜单键时则在
元素下方，并具备相同的按键，它的 `Popup` 像
[`MenuBase`](menu-button.md#无外观) 的那样构建菜单，菜单项由你自己的
布局构成：

```go
m := ui.ContextMenuBase(c, row, &app.rowMenu)
m.Popup(func(panel ui.Element) {
	panel.Padding(4).Radius(10).Background(t.Background).Border(1, t.Border)
	for _, label := range app.labels {
		item := m.CheckItem(note.has(label)).Gap(8).Padding(6, 10).Radius(6)
		if item.Highlighted() {
			item.Background(t.SurfaceHover)
		}
		item.Children(func() {
			ui.Box(c).Size(10, 10).Radius(5).Background(label.Color)
			ui.Text(c, label.Name)
		})
		if item.Clicked() {
			note.toggle(label)
		}
	}
})
```
