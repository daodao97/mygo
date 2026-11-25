# 辅助功能

辅助技术，例如 macOS 上的 VoiceOver、Linux 上的 Orca，以及 Windows 上的
Narrator 或 NVDA，会像读取其他应用一样读取窗口的原生 UI：控件及其角色、
名称、值和状态、文本，以及内容变化时获得焦点的元素。辅助技术也能完成键盘和
指针的操作：按下按钮、勾选复选框、移动滑块、编辑文本输入框并移动焦点。

## 名称

控件会描述自己，元素内的文本会为它命名，就像按钮的文本那样。没有文本的元素
需要 `Label`，`Label` 也能让[测试](testing.md)找到它们：

```go
ui.Slider(c, &app.volume, 0, 100).Label("Volume")
ui.TextInput(c, &app.query).Placeholder("Search").Label("Search")
ui.Box(c).Size(24, 24).Draw(drawIcon).Label("Unread messages")
```

[字段](form.md)用标签为控件命名；`LabelledBy` 则用视图绘制的一段说明文字为
控件命名，点击这段文字等同于点击控件（[自己的标签](form.md#自己的标签)）。

## 角色

其他元素根据它们的行为获得角色：可点击并获得焦点的元素是按钮，滚动容器是滚动
区域，带 `Label` 或获得焦点的元素是组。对于被绘制成某种控件、却并非由该控件
构建的元素，可以用 `Role` 设置角色；`ui.RoleNone` 把元素本身排除在树外，但保留
它的子元素（[`AccessHidden`](#不挡辅助功能的路) 会把它们也排除）：

```go
toggle := ui.Box(c).Size(36, 20).Focusable().Role(ui.RoleSwitch).Label("Wi-Fi")
```

除 MyGo 控件自带的角色之外，`ui.RoleHeading` 是章节的标题，用 `Level` 分级；
`ui.RoleNavigation` 是由在应用或文稿中跳转的链接构成的地标，如面包屑导航或
导航栏，屏幕阅读器会列出这些地标并支持跳转（[`Breadcrumbs`](breadcrumbs.md) 就是
如此）；`ui.RoleMenu`、`ui.RoleMenuBar`、`ui.RoleMenuItem`、
`ui.RoleMenuItemCheckBox` 和 `ui.RoleMenuItemRadio` 让画在窗口中的菜单具备
菜单角色，如下拉菜单或你自己的菜单栏（MyGo 画在窗口中的每个菜单，
[`PopoverMenu`](context-menu.md#在窗口中绘制) 的、
[`PopoverMenuButton`](menu-button.md#在窗口中绘制) 的，以及[窗口
的](context-menu.md#窗口的每个菜单)，都有这些角色，
[`MenuBase`](menu-button.md#无外观) 和 `ContextMenuBase` 也有，它们
是无外观、保留键盘操作和焦点行为的菜单）：

```go
ui.Text(c, "Appearance").Bold().Role(ui.RoleHeading).Level(2)
ui.Row(c).Role(ui.RoleNavigation).Label("Sections").Children(sectionLinks)
```

自己构建表格时，用 `ui.RoleRow` 构建行，用 `ui.RoleCell` 构建单元格，用
`ui.RoleColumnHeader` 构建各列的表头。表头由其中的文本命名，并像 ARIA 的
`columnheader` 那样保留其中的元素：表头里的排序按钮和列菜单按钮，能像其他
按钮一样被访问和按下。

```go
ui.Row(c).Role(ui.RoleColumnHeader).Children(func() { // named "Sort Options"
	if ui.Button(c, "Sort").Clicked() {
		app.sortBy("name")
	}
	ui.Button(c, "Options")
})
```

`ui.RoleGrid` 和 `ui.RoleTreeGrid` 是你自己构建的表格，单元格可以获得焦点，
并用方向键移动焦点，像电子表格那样；`ui.RoleTable` 则是只供阅读的表格：见
[网格](#网格)。

`ui.RoleListBox` 是可供选择的选项列表，`ui.RoleOption` 是其中一个选项，用
`Checked` 选中，正如 ARIA 的 `listbox`、`option` 和 `aria-selected`：在
Windows 上是列表框（UI Automation 的 List，带 `Selection` 和
`SelectionItem`），在 Linux 上是列表框（ATK 的列表框，带 `AtkSelection`），
在 macOS 上则是行可被选中的表格，正如 AppKit 的列表那样，
[`SelectBase`](select.md#无外观) 和 [`ComboboxBase`](combobox.md) 的
弹出层也是如此。文本区域插入点旁为 @ 提及显示的建议就是一个列表框，
文本区域保留焦点，并用 `ActiveDescendant` 指向高亮的选项：

```go
area := ui.TextArea(c, &app.draft)
if len(app.matches) > 0 {
	ui.Column(c).Role(ui.RoleListBox).Label("People").Children(func() {
		for i, p := range app.matches {
			opt := ui.Row(c).Padding(4, 8).Role(ui.RoleOption).Checked(i == app.hi)
			if opt.Clicked() {
				app.mention(p)
			}
			opt.Children(func() { ui.Text(c, p.Name) })
			if i == app.hi {
				area.ActiveDescendant(opt)
			}
		}
	})
}
```

按下选项——辅助技术选择它时就是这样——会点击它；只有它是 `Focusable` 时，
它才会获得焦点。

## 状态

各个基类会告诉辅助技术，它们的控件显示了什么。你自己的控件则用下面这些方法
来说明，就像 Web 上 ARIA 的状态那样：

| 方法 | 它说明什么 |
|---|---|
| `Checked(on)`, `Mixed()` | 复选框、开关、单选按钮、切换按钮或菜单项是否开启，或部分开启，或选项是否被选中 |
| `AccessSelected(on)` | 标签页、选项、行、单元格或树项是否被选中，但不绘制 `Selected` 所用的强调色（[下文](#选中但不画出来)） |
| `Expanded(open)` | 元素展开的内容是否显示，如弹出层或区块 |
| `Value(s)` | 它的值，例如打开弹出层的按钮所显示的选择项 |
| `Range(lo, hi, value)`, `Step(step)` | 滑块、进度条、量度计或步进器的范围和值，以及按键每次移动多远 |
| `Level(n)` | 标题的层级，或树中某项的深度 |
| `PositionInSet(pos, size)` | 说明列表、树、菜单或标签页列表中的某一项是共多少项中的第几项，从 1 起，用于已构建内容本身未说明的情况（[下文](#在集合中的位置)） |
| `ActiveDescendant(e)` | 元素保留键盘焦点时，作为当前活动项的选项，如菜单或列表中方向键选中的选项 |
| `Busy(on)` | 元素的内容即将变化，如正在加载的区域：屏幕阅读器会等它加载完再读取 |
| `Current(kind)` | 元素是周围各项中当前的一项：`ui.CurrentPage`（用户所在的页面）、`CurrentStep`、`CurrentLocation`、`CurrentDate`、`CurrentTime` 或 `CurrentItem` |
| `AutoComplete(kind)` | 用户输入时，文本框的建议以何种方式补全：`ui.AutoCompleteList`（弹出层中列出建议）、`AutoCompleteInline`（就地补全文本）或 `AutoCompleteBoth`（[下文](#建议)） |
| `RoleDescription(s)` | 用来称呼其角色的名称，代替角色本身的名称，如把一组幻灯片称为“carousel”，把每一张称为“slide” |
| `Controls(e)` | 元素控制 `e` 的内容或是否显示，如展开按钮控制打开的区块；每个受控对象调用一次 |
| `HasPopup(kind)` | 它打开的弹出层类型：`ui.PopupMenu`、`PopupListBox`、`PopupTree`、`PopupGrid` 或 `PopupDialog` |
| `Sort(dir)` | 它作为表头的这一列对表格行排序的顺序：`ui.SortAscending`、`SortDescending`、`SortOther` 或 `SortNone` |
| `Orientation(o)` | 你自己的滑块、分隔线、分隔条、工具栏、标签页列表或菜单的方向：`ui.Vertical` 或 `ui.Horizontal`（[下文](#方向)） |
| `DescribedBy(e)`, `Invalid(on)`, `ErrorMessage(e)` | 用文本描述它的元素 `e`、它的值是否无效，以及存放其错误消息的元素（[下文](#描述和错误)） |
| `Multiselectable(on)` | 用户可在你自己的列表、网格、树或树网格中选择多项 |
| `ReadOnly(on)` | 你自己的元素显示用户不能更改的值，如 `aria-readonly`：文本框、组合框、复选框、开关、滑块、步进器、单选按钮组、列表框、网格或树网格，以及网格或树网格的单元格和表头（不是表格的）；辅助技术随后既不会设置也不会步进它的值。它在 macOS 上是不可设置的 `AXValue`，在 Linux 上是 ATK 的 `read-only` 状态，在 Windows 上是 UI Automation 的 `IsReadOnly` |
| `Multiline(on)` | 你自己的文本框（`RoleTextField`）可接受多行，如 `aria-multiline` |
| `RowCount(n)`, `ColumnCount(n)` | 表格或网格有多少行和列，无论是否已构建 |
| `RowIndex(i)`, `ColumnIndex(i)` | 某一行或单元格在所有行中的行号，以及某个单元格在所有列中的列号，从 1 起 |

```go
all := ui.CheckboxBase(c, &app.all).Label("Select all")
if app.some {
	all.Mixed()
}

date := ui.ButtonBase(c).Role(ui.RolePopUpButton).Label("Due").Value(app.due.Format("Jan 2")).Expanded(app.picking)
```

区域正在加载、由占位符顶替时，会这样说明；导航栏中指向当前页面的链接则说明
自己是当前页面：

```go
ui.Column(c).Label("Messages").Busy(app.loading).Children(func() {
	if app.loading {
		ui.Spinner(c).Label("Loading")
		return
	}
	messageList(c, app.messages)
})

ui.Row(c).Role(ui.RoleNavigation).Label("Main").Children(func() {
	for _, p := range pages {
		l := ui.Link(c, p.Title, p.Path)
		if p.Path == app.path {
			l.Current(ui.CurrentPage)
		}
	}
})
```

既没有自己的角色也没有 `Label` 的元素，在 `Busy` 或 `Current` 标记它之后，
仍会作为一个组出现在辅助功能树中，就像 Web 上带 ARIA 状态的通用元素一样。
`Busy` 是 AppKit 的 `AXElementBusy`、ATK 的 `busy` 状态和 UI Automation 的
`AriaProperties`（`busy=true`）；`Current` 是 AppKit 的 `AXARIACurrent`、
ATK 带 `active` 状态的 `current` 属性，以及 `AriaProperties`
（`current=page`）。

网格的单元格用 `AccessSelected` 或 `Checked` 说明它是否被选中，就像 ARIA 在
`gridcell` 上的 `aria-selected`，但不绘制它，如日历的日期或你自己的电子表格
中的单元格；`Selected` 还会用强调色绘制它。两个都不调用的单元格（`RoleCell`、
`RoleColumnHeader`、`RoleRowHeader`）不参与彼此之间的选择；而如果表格或
[网格](#网格)（`RoleTable`、`RoleGrid`、`RoleTreeGrid`）中的单元格调用了
其中之一，就由它负责这些单元格的选择。下面是一个轮播、一个展开按钮和一个
打开列表框的按钮，都建立在其他角色之上：

```go
ui.Column(c).Role(ui.RoleGroup).Label("Highlights").RoleDescription("carousel").Children(func() {
	for i, s := range slides {
		ui.Column(c).Role(ui.RoleGroup).Label(fmt.Sprintf("%d of %d", i+1, len(slides))).
			RoleDescription("slide").AccessHidden(i != app.slide).Children(s)
	}
})

toggle := ui.Button(c, "Details").Expanded(app.open)
if app.open {
	toggle.Controls(ui.Column(c).Role(ui.RoleRegion).Label("Details").Children(details))
}

ui.ButtonBase(c).Role(ui.RolePopUpButton).Label("Fruit").Value(app.fruit).
	HasPopup(ui.PopupListBox).Expanded(app.picking)

day := ui.Box(c).Role(ui.RoleCell).Label(d.Format("January 2")).AccessSelected(d.Equal(app.picked))
```

它们按 WAI-ARIA 的 Core Accessibility API Mappings 的规定映射到各平台。
`RoleDescription` 是 AppKit 的 `AXRoleDescription`、ATK 的 `roledescription`
属性和 UI Automation 的 `LocalizedControlType`。`Controls` 是 AppKit 的
`AXLinkedUIElements`、ATK 的 `controller-for` 关系（在它控制的内容上则是
`controlled-by`）和 UI Automation 的 `ControllerFor`，屏幕阅读器可让用户移到
它（VoiceOver 的 VO-J）。`HasPopup` 是 AppKit 的 `AXPopupValue` 及
`AXShowMenu` 动作（该动作会按下元素）、ATK 的 `has-popup` 状态和
`haspopup` 属性，以及 UI Automation 的 ExpandCollapse 模式（配合
`Expanded`）和 `AriaProperties`（`haspopup=listbox`）。`Sort` 在 macOS 上是
排序按钮（`AXSortButton`）表头上的 `AXSortDirection`（其他排序顺序时为
`unknown`），就像 NSTableHeaderView 的表头那样；在 Linux 上是 ATK 的 `sort`
属性；在 Windows 上是 UI Automation 的 `ItemStatus` 和 `AriaProperties`
（`sort=other`），正如 [`Table`](table.md) 的表头根据 `ListState.Sort` 所
说明的那样。选中的单元格是 AppKit 的 `AXSelected`，其表格的
`AXSelectedCells` 也包含它，它的行则在表格的 `AXSelectedRows` 中，正如
NSTableView 报告自己的选择；由于 VoiceOver 既不读取单元格的状态，也不读取
它的变化，因此会在单元格名称之后用应用的语言读出“已选中”，不在焦点处被选中的
单元格会被朗读出来。在 Linux 上它是 ATK 的 selectable 和 selected 状态，在
Windows 上则是表格 Selection 中的 UI Automation SelectionItem。

[测试](testing.md#辅助技术看到什么)用 `Tester.AccessByRole`
读取它们，得到的正是辅助技术看到的内容：

```go
if a, _ := tt.AccessByRole(ui.RoleGroup, "Messages"); !a.Busy {
	t.Error("Messages is not busy while loading")
}
if day, _ := tt.AccessByRole(ui.RoleCell, "November 2"); !day.Selected {
	t.Error("the day picked is not selected")
}
```

经常处于混合状态的复选框，最好建立在
[`CheckboxStateBase`](checkbox.md#无外观) 上，它会根据自身的
`ui.CheckState` 说明混合状态，并在按下时勾选它。

## 建议

`AutoComplete(kind)` 告诉屏幕阅读器，用户输入时文本框的建议以何种方式补全，
就像 ARIA 的 `aria-autocomplete`，好让屏幕阅读器说明该字段有建议：
`ui.AutoCompleteList` 表示弹出层中列出与已输入内容匹配的值，
`AutoCompleteInline` 表示在插入点之后补上值的其余部分并选中，
`AutoCompleteBoth` 表示两者都有。带建议的[组合框](combobox.md)、
[自动补全](autocomplete.md)，以及带建议或设置了 `Query` 的
[标记字段](token-field.md)会自己说明 `AutoCompleteList`；
你自己的组合框如果有弹出层和方向键所在的选项，用 `Expanded` 和
`ActiveDescendant` 说明它们，也要说明这一点：

```go
in := ui.TextInputBase(c, &app.query).Role(ui.RoleComboBox).Label("Search")
in.Expanded(len(app.matches) > 0).AutoComplete(ui.AutoCompleteList)
```

它只提供辅助功能信息，建议仍由视图显示。默认的 `AutoCompleteNone` 不提供补全信息，
适用于无论输入什么都显示相同选项的组合框。它在 macOS 上是 AppKit 的
`AXAutocompleteValue`（如 WebKit 所给），在 Linux 上是 ATK 的 `autocomplete`
对象属性及 supports-autocompletion 状态（如 Core-AAM 所映射），在 Windows
上是 UI Automation 的 `AriaProperties` 中的 `autocomplete=list`——即 UI
Automation 没有的那些 ARIA 属性的存放处。[测试](testing.md#辅助技术看到什么)
在 `Tester.AccessByRole` 结果的 `AutoComplete` 中读取它，
[自动化插件](../plugins/automation.md)在节点的 `autoComplete` 中读取它。

## 描述和错误

`Description` 说明名称之外的信息，会作为帮助文本在名称之后朗读，就像
[工具提示](tooltip.md)那样；`Error` 把值标记为无效，并附上随它一起朗读的
消息；[字段](form.md)把它下方的文本交给控件。

视图单独绘制成元素的提示或帮助文本，可以用 `DescribedBy` 来描述一个控件，就像
ARIA 的 `aria-describedby`：屏幕阅读器会在控件的名称、值和 `Description` 之后
朗读提示文本，含链接的句子也会连贯朗读。即使提示对辅助技术隐藏，这一关联仍然
有效，因此可以将提示排除在阅读顺序之外，同时保留聚焦字段时的朗读：

```go
hint := ui.Text(c, "16 digits, no spaces").TextColor(t.TextMuted)
ui.TextInput(c, &app.card).Label("Card number").DescribedBy(hint)
```

`Invalid` 把值标记为无效而不带消息，就像单独的 `aria-invalid`：用户在必填
字段为空时离开，该字段就会被标记，其消息等到表单提交时才出现。文本输入随后用
主题的 `Danger` 颜色绘制边框，它的描述保持不变。`ErrorMessage` 指向存放消息
的元素，就像 `aria-errormessage`：控件无效时，先朗读它的文本，然后才读描述，
遵循该关系的屏幕阅读器会转到它；控件有效时，它不说明任何内容：

```go
in := ui.TextInput(c, &app.email).Label("Email").Invalid(app.emailTouched && app.email == "")
if app.submitted && app.email == "" {
	in.ErrorMessage(ui.Text(c, "Enter your email").TextColor(t.Danger))
}
```

描述是 AppKit 的 `AXHelp`、ATK 带 `described-by` 关系的描述（提示上有
`description-for`），以及 UI Automation 带 `DescribedBy` 的
`FullDescription`。消息是 WebKit 的 `AXErrorMessageElements`、ATK 的
`error-message` 关系（消息上有 `error-for`），以及 UI Automation 的
`ControllerFor`，如 Core-AAM 所映射；无效是 `AXInvalid`、ATK 的
`invalid-entry` 状态和 UI Automation 的 `IsDataValidForForm`。

## 方向

`Orientation` 说明你自己的元素的方向，就像 `aria-orientation`：上下绘制、
用上下方向键操作的滑块、分隔线、分隔条、工具栏、标签页列表或菜单，方向是
`ui.Vertical`，屏幕阅读器会随它一起朗读。它只负责说明：视图负责布局该元素并
接收它的按键（[`FocusGroup`](input.md#焦点组) 用方向键移动焦点）。没有说明方向
的滑块或分隔线默认为横向；高度大于宽度的分隔线为纵向；竖直方向（`Vertical`）的
[`SliderBase`](slider.md) 会自己说明方向：

```go
ui.Box(c).Size(6, 160).Draw(drawTrack).Focusable().
	Role(ui.RoleSlider).Label("Volume").Range(0, 100, app.volume).
	Orientation(ui.Vertical)
ui.Column(c).Role(ui.RoleToolbar).Label("Tools").Orientation(ui.Vertical).Children(tools)
```

它是 AppKit 的 `AXOrientation`、ATK 的 vertical 或 horizontal 状态，以及 UI
Automation 的 `Orientation`。

## 键盘快捷键

`KeyShortcuts(mods, key)` 告诉屏幕阅读器哪个按键会按下或聚焦该元素，就像
ARIA 的 `aria-keyshortcuts`：命令由窗口快捷键执行的工具栏按钮，或打开命令
面板的按钮，会在名称之后连同按键一起被朗读。它只负责说明，按键本身由视图
处理，就像 [`c.Shortcut`](input.md#快捷键)；每个起作用的按键调用一次：

```go
if c.Shortcut(ui.Cmd, ui.KeyK) {
	app.palette = true
}
ui.Toolbar(c, func() {
	if ui.Button(c, "Search").KeyShortcuts(ui.Cmd, ui.KeyK).Clicked() {
		app.palette = true
	}
	ui.Button(c, "Redo").KeyShortcuts(ui.Cmd|ui.Shift, ui.KeyZ).KeyShortcuts(ui.Cmd, ui.KeyY)
})
```

这些键按平台菜单的写法书写，以空格分隔：macOS 上是 `⌘K` 和 `⇧⌘Z ⌘Y`，
位于 AppKit 的 `AXKeyShortcutsValue` 中，如 WebKit 所给；Linux 上是 `Ctrl+K`
和 `Ctrl+Shift+Z Ctrl+Y`，位于 ATK 的 `keyshortcuts` 对象属性中；Windows 上
位于 UI Automation 的 `AcceleratorKey` 中，其中 Super 是 `Win`。
[测试](testing.md#辅助技术看到什么)在 `Tester.AccessByRole`
结果的 `KeyShortcuts` 中读取它们，[自动化插件](../plugins/automation.md)在
节点的 `keyShortcuts` 中读取它们。

## 文本格式

[富文本编辑器](rich-text-editor.md)会告诉屏幕阅读器各文本段的格式，屏幕
阅读器朗读时会带上它们：粗体、斜体、下划线、删除线、链接、字体、颜色和标题。
VoiceOver 通过 `AXAttributedStringForRange` 读取它们，Orca 通过 ATK 的文本段
属性，Narrator 通过 UI Automation 的 Text 模式——每个文本字段都有该模式——
按字符、格式、单词和行读取。屏幕阅读器也会移动文本字段的选区。

## 列表和表格

[列表](list.md)的行是列表项；[表格](table.md)的行同样由其中的文本命名，每一
行都说明自己在所有行中是第几行，如“10,000 行中的第 5 行”，即使列表只构建
视口内的行。支持行选择的列表会将辅助技术焦点置于选中行：上下
方向键改变选中行时会朗读新行，聚焦一行也会将其选中。屏幕阅读器移到视口之外时，会
请求列表滚动到那里，这会构建它到达的行。`Label` 为列表或表格命名。你自己的
表格若需要通过方向键在单元格间移动焦点，应使用[网格](#网格)。

## 在集合中的位置

屏幕阅读器会连同位置朗读一项，“50 项中的第 3 项”，这个位置由该项周围的项数
决定。你自己的列表、列表框、树、菜单或标签页列表，如果项没有全部构建（如只
显示视口内的项），或各项不是同级（如平铺在一列中的树），就用
`PositionInSet(pos, size)` 说明每一项的位置，就像 ARIA 的 `aria-posinset` 和
`aria-setsize`：`pos` 从 1 起，`size` 是集合中的项数，无论是否已构建。任一值
为 0 或更小则不说明。

```go
ui.Column(c).Role(ui.RoleListBox).Label("Results").Children(func() {
	for i := app.first; i < app.last; i++ { // only those in view
		ui.Row(c).Role(ui.RoleOption).PositionInSet(i+1, len(app.results)).
			Children(func() { ui.Text(c, app.results[i]) })
	}
})

ui.Column(c).Role(ui.RoleTree).Label("Files").Children(func() {
	for _, f := range app.flat { // each item at its depth, one below the other
		ui.Row(c).Role(ui.RoleTreeItem).Label(f.Name).Level(f.Depth+1).PositionInSet(f.Index+1, f.Siblings)
	}
})
```

应用给出的位置优先于 `ComboboxBase`、`SelectBase`、`MenuBase`、`Tree` 或
`GridView` 给其项的位置，后者只计算自己构建的项，例如显示 1,000 个结果中
前 50 个的组合框；[List](list.md) 和 [Table](table.md) 的行保留自己的位置，
它们本来就会说明自己在所有行中是第几行。你自己的[网格](#网格)的行和单元格用
`RowIndex` 和 `ColumnIndex` 说明位置。在 Linux 上这是 ATK 的 `posinset` 和
`setsize` 属性，在 Windows 上是 UI Automation 的 `PositionInSet` 和
`SizeOfSet`，在 macOS 上是 AppKit 的 `AXIndex`；列表自己没有设置大小时，
`AXRowCount` 就是其项给出的大小。
[测试](testing.md#辅助技术看到什么)把它们读作 `PosInSet` 和
`SetSize`。

## 选中，但不画出来

`Selected(true)` 用强调色把元素显示为已选中，文字用与强调色相配的颜色，同时
告诉辅助技术它已被选中。标签页、选项、行、单元格或树项若以自己的方式显示
选择——下划线、粗体或对勾——则只用 `AccessSelected(on)` 说明状态，即 ARIA 的
`aria-selected`：

```go
ui.Row(c).Role(ui.RoleTabList).Label("Views").Children(func() {
	for i, name := range views {
		tab := ui.ButtonBase(c).Role(ui.RoleTab).Label(name).AccessSelected(i == app.view)
		if tab.Clicked() {
			app.view = i
		}
		tab.Children(func() {
			label := ui.Text(c, name)
			if i == app.view {
				label.Bold()
			}
		})
	}
})
```

它与 `Selected` 设置相同的状态，`Checked` 对这些角色也会设置该状态，因此它
与 `Selected` 一样映射到各平台：行或单元格一旦说明是否选中（`false`
或 `true`），就成为所在表格或网格的可选项，选择状态由表格或网格管理。
它只提供状态信息，选中效果的绘制和点击后的状态更新仍由视图完成。
[测试](testing.md#辅助技术看到什么)把它读作 `Selected`。

## 网格

网格是一种表格，单元格可以获得焦点，并用方向键移动焦点，如电子表格或数据
网格；`ui.RoleTable` 则是只供阅读的表格。`ui.RoleTreeGrid` 是一种网格，行像
树项那样展开和收起，行上有 `Level` 和 `Expanded`。它们的行、单元格和表头是
`ui.RoleRow`、`ui.RoleCell`、`ui.RoleColumnHeader` 和 `ui.RoleRowHeader`，
和表格一样：直接在网格内构建行（网格与行之间没有角色的元素会被排除），并直接
在行内构建单元格。

有数千行的网格只构建视口内的行，因此它要用 `RowCount` 和 `ColumnCount`
告诉屏幕阅读器行数和列数（ARIA 的 `aria-rowcount` 和 `aria-colcount`，
包含表头行），每一行再用从 1 起的 `RowIndex`（`aria-rowindex`）说明
在所有行中的位置；列未全部构建，或单元格前有列被省略时，用
`ColumnIndex`（`aria-colindex`）说明列号，后续单元格的列号依次递增。屏幕
阅读器随后朗读“第 4,121 行，共 10,000 行”。跨越多列或多行的单元格，如两列
下方的合计，用 `ColumnSpan` 和 `RowSpan`（`aria-colspan`、`aria-rowspan`）
说明；如果所在行是 [Grid](grid.md)，这些属性还会使单元格跨列布局：同一行中
后续单元格排在所跨列之后；若单元格跨行，下方各行中位于它之后的
单元格用 `ColumnIndex` 说明自己的列。`Multiselectable(true)` 说明用户可以
选择多行（`aria-multiselectable`），每一行用 `Selected` 或 `Checked` 说明它
是否被选中，后者不绘制它：

```go
ui.Column(c).Role(ui.RoleGrid).Label("Sheet").RowCount(len(app.rows)+1).ColumnCount(len(cols)).Multiselectable(true).Children(func() {
	ui.Row(c).Role(ui.RoleRow).RowIndex(1).Children(func() {
		for _, col := range cols {
			ui.Box(c).Role(ui.RoleColumnHeader).Children(func() { ui.Text(c, col.Title) })
		}
	})
	for _, r := range app.visibleRows() { // 4120 to 4160
		ui.Row(c).Role(ui.RoleRow).RowIndex(r + 2).Selected(app.chosen[r]).Children(func() {
			for j, col := range cols {
				ui.Box(c).Role(ui.RoleCell).Focusable().Children(func() { ui.Text(c, app.cell(r, j)) })
			}
		})
	}
})
```

以网格显示的 [List](list.md) 自己只构建视口内的行：`Role(ui.RoleGrid)` 会让
它的行成为行，而不是列表项；这些行本来就会说明自己在所有行中是第几行，而它的
`Selection` 也能选择多项：

```go
ui.List(c, &app.rows, len(app.data), func(i int) {
	ui.Row(c).Children(func() {
		for j := range cols {
			ui.Box(c).Role(ui.RoleCell).Children(func() { ui.Text(c, app.cell(i, j)) })
		}
	})
}).Role(ui.RoleGrid).Label("Sheet").ColumnCount(len(cols))
```

手工构建的数据网格中跨两列的单元格，其行把单元格排进网格的各列：

```go
ui.Grid(c).Role(ui.RoleRow).ColumnTracks(ui.Fixed(120), ui.Fr(1), ui.Fr(1), ui.Fr(1)).Children(func() {
	ui.Box(c).Role(ui.RoleRowHeader).Children(func() { ui.Text(c, "Design") })
	ui.Box(c).Role(ui.RoleCell).ColumnSpan(2).Children(func() { ui.Text(c, "First half: 9") })
	ui.Box(c).Role(ui.RoleCell).Children(func() { ui.Text(c, "Total: 14") }) // the fourth column
})
```

[Table](table.md) 的行自己说明它们在所有行中是第几行，列表的行也一样：计数
和索引是为你自己的网格和表格准备的。在 macOS 上，网格是 AppKit 的 `AXTable`，
它的行和单元格像 WebKit 的那样，在 `AXARIARowCount`、`AXARIAColumnCount`、
`AXARIARowIndex` 和 `AXARIAColumnIndex` 中说明计数和索引，它的行由此得到
`AXIndex`；在 Linux 上，它是 ATK 的表格（树网格则是树表格），带 `xml-roles`
属性 `grid`（`treegrid`）和 `rowcount`、`colcount`、`rowindex` 与 `colindex`
属性，如 Chromium 所给；在 Windows 上，它是 UI Automation 的 DataGrid，其
`Grid` 和 `Table` 模式，以及单元格的 `GridItem` 和 `TableItem`，从 0 起说明
每个单元格在所有单元格中的位置，以及哪些列表头和行表头为它命名，和表格一样。
单元格的跨度在 macOS 上是其 `AXColumnIndexRange` 和 `AXRowIndexRange` 的
长度，在 Linux 上是它的 `colspan` 和 `rowspan` 属性，在 Windows 上是
`GridItem` 的 `ColumnSpan` 和 `RowSpan`；网格会找到它覆盖的每个位置上的
单元格。单元格也用 [`Checked`](#状态) 说明自己是否被选中，选择由网格持有，
和表格一样。选择多行的网格在 Linux 上是 `multiselectable`，在 Windows 上是
`CanSelectMultiple`，其行在这些平台上具有 `selectable` 和 `selected` 状态，在
macOS 上则位于它的 `AXSelectedRows` 中。

## 不可用，但仍可到达

`Disabled(true)` 做三件事：元素被读作已禁用；它会离开焦点和 Tab 顺序，也不
接受点击；它还会以一半不透明度显示（除非 `DimDisabled(false)`）。因此用 Tab
移动的屏幕阅读器永远到不了它。`AccessDisabled(true)` 只做第一件事，如同 HTML
的 `aria-disabled="true"`，用于用户在无法使用时仍须找到的内容，例如 APG 所
要求的、操作被阻止的菜单项，被禁用但仍留在方向键顺序中的标签页，或表单无效
时的提交按钮：该元素及其内部的元素会被读作已禁用（变暗、不可用），并保留
焦点、Tab 顺序中的位置、指针交互和外观。

```go
del := ui.Button(c, "Delete project").AccessDisabled(app.locked)
if app.locked {
	del.Description("Unlock the project to delete it")
}
if del.Clicked() && !app.locked {
	app.deleteProject()
}
```

它只提供状态信息：点击、Enter 和空格仍会触发元素，不可用时视图须忽略这些操作，
与 Web 上一样；外观仍由视图的样式决定。辅助技术可以聚焦并朗读元素及其
`Description`，但既不会按下也不会编辑它。它在 macOS 上是 AppKit 的
`AXEnabled` 为 false，在 Linux 上则是 ATK 省略了 enabled 和 sensitive 状态，
在 Windows 上是 UI Automation 的 `IsEnabled` 为 false，而
`IsKeyboardFocusable` 为 true，其 `SetFocus` 被接受，如 Chromium 的
aria-disabled。[测试](testing.md#辅助技术看到什么)在
`Tester.AccessByRole` 的结果中把它读作带 `Focusable` 的 `Disabled`。

## 不挡指针的路

`IgnoreInput(true)` 把一个元素及其内部的元素只留给辅助技术：指针穿过它到达
它下面的内容，Tab 和点击都不会给它焦点，但辅助技术仍能找到它，连同它的角色
和名称，并按下它，`Clicked` 会报告这次按下。这样，在行被悬停时才显示的按钮
可以在未悬停时仍被构建，并设为 `Opacity(0)`，屏幕阅读器仍能找到它，而点击这一行
仍然算作点击该行：

```go
row := ui.Row(c).Padding(8)
hovered := row.Hovered()
if row.Clicked() {
	app.open(item)
}
row.Children(func() {
	ui.Text(c, item.Title).Grow(1)
	del := ui.Button(c, "Delete").IgnoreInput(!hovered)
	if !hovered {
		del.Opacity(0)
	}
	if del.Clicked() {
		app.remove(item)
	}
})
```

辅助技术既不能聚焦它，也不能移动它里面的滑块，因为两者都要经过键盘焦点；它
可以设置文本输入的值。与 HTML 的 `inert`（它还会对辅助技术隐藏元素）不同，
它仍留在树中：`Invisible` 和 `Hidden` 把它移出树，`AccessHidden` 也会在它
仍显示时把它移出；`Disabled` 把它留在树中，但不接受按下；`AccessDisabled`
则在禁用操作的同时仍允许聚焦。

## 不挡辅助功能的路

`AccessHidden(true)` 把一个元素及其内部的元素排除在辅助技术读取的内容之外，
但仍正常绘制，如同 HTML 的 `aria-hidden="true"`：这些元素不在辅助功能树中，
文本不会用于为外层元素命名，辅助技术也不能按下、聚焦或编辑它们。它用于显示出来
但不需要读取，或已经读取过的内容：

- 在替代它的页面或面板旁边滑出的页面或面板，两者都还在显示；
- 从堆叠中前面那张卡片后面露出的卡片，轮播中当前幻灯片旁边的幻灯片；
- 同一个文本绘制两次的副本，如一个标签淡出、下一个标签淡入。

配合 `IgnoreInput(true)`，它就是 HTML 的 `inert`：指针穿过它们，Tab 略过
它们，它们失去焦点，它们的文本不能被选中，例如[正在离开的
元素](transitions.md#出现与离开)或[正在离开的页面](navigation.md#过渡)
的副本：

```go
ui.Box(c).Fill().Children(func() {
	if app.leaving >= 0 { // the tab that was selected, sliding out
		page(c, app.leaving).Absolute().Translate(-x, 0).IgnoreInput(true).AccessHidden(true)
	}
	page(c, app.tab).Translate(width-x, 0)
})
```

单独使用时，元素仍接受指针和键盘操作，文本仍可选择，如同
`aria-hidden`：应避免键盘焦点进入这些元素，因为辅助技术无法跟随。`RoleNone`
只排除元素本身，适合装饰性图标等叶子元素。

## 播报

不移动焦点的消息，如搜索完成或文件已保存，通过 `c.Announce(text)` 到达屏幕
阅读器，它们会朗读一次，在正在朗读的内容之后：即 VoiceOver 和 Orca 的播报，
以及 Windows 上 Narrator 和 NVDA 会读取的实时区域。[Toast](toast.md) 播报
它们的文本，[路由器](navigation.md)播报在焦点留在页面之外时显示的页面标题。

```go
if ui.Button(c, "Search").Clicked() {
	app.results = search(app.query)
	c.Announce(fmt.Sprintf("%d results", len(app.results)))
}
```

MyGo 只在辅助技术请求后才描述帧，因此应用平时不必为此付出任何代价。

## 地标、实时区域和分隔线

这些角色是 ARIA 中同名的角色，它们按 WAI-ARIA 的 Core Accessibility API
Mappings 的规定映射到各平台：

| 角色 | 是什么 | macOS | Linux（ATK） | Windows（UI Automation） |
|---|---|---|---|---|
| `ui.RoleRegion` | 带 `Label` 的地标，如活动流或面板；没有 `Label` 时则是一个组 | `AXGroup`, `AXLandmarkRegion` | landmark, `xml-roles:region` | Group，自定义地标类型 "region" |
| `ui.RoleForm` | 表单的地标，如登录面板或设置表单，如同 HTML 的 `<form>`；有 `Label` 是地标，没有则是一个组 | `AXGroup`, `AXLandmarkForm` | landmark, `xml-roles:form` | Group，表单地标类型 |
| `ui.RoleSearch` | 容纳搜索应用或文稿所需控件的地标，如窗口顶部的搜索框及其按钮 | `AXGroup`, `AXLandmarkSearch` | landmark, `xml-roles:search` | Group，搜索地标类型 |
| `ui.RoleArticle` | 独立完整的一段内容，如信息流中的帖子或讨论串中的评论，由它的 `Label` 命名；不是地标 | `AXGroup`, `AXDocumentArticle` | article | Group "article" |
| `ui.RoleLog` | 按顺序添加的消息的实时区域，如聊天的记录 | `AXGroup`, `AXApplicationLog` | log | Group "log"，`LiveSetting` polite |
| `ui.RoleTimer` | 时间的计数，如倒计时，只在用户转到它时朗读 | `AXGroup`, `AXApplicationTimer` | timer | Group "timer" |
| `ui.RoleAlert` | 重要消息的实时区域，如字段的错误，会立即朗读 | `AXGroup`, `AXApplicationAlert` | notification | Group "alert"，`LiveSetting` assertive |
| `ui.RoleStatus` | 提示信息的实时区域，如“已选择 3 行”或“已保存”，文本变化后会等待当前朗读结束，再完整朗读 | `AXGroup`, `AXApplicationStatus` | notification，旧版 ATK 不支持时为状态栏 | StatusBar，`LiveSetting` polite |
| `ui.RoleSeparator` | 内容组或控件组之间的分隔线，高度大于宽度时为纵向 | `AXSplitter` | separator，vertical 或 horizontal | Separator，带 `Orientation` |
| `ui.RoleRowHeader` | 为表格中它的行命名的单元格，由它的内容命名，其中的按钮和其他元素像普通单元格那样仍可访问 | `AXCell` | row header | HeaderItem "row header" |
| `ui.RoleDeletion` | 从文稿中移除的内容，如差异中删除的行或划掉的旧价格，如同 HTML 的 `<del>`；它的文本在它内部 | `AXGroup`, `AXDeleteStyleGroup` | content deletion，`xml-roles:deletion` | Text "deletion" |
| `ui.RoleInsertion` | 添加到文稿中的内容，如差异中新增的行，如同 HTML 的 `<ins>` | `AXGroup`, `AXInsertStyleGroup` | content insertion，`xml-roles:insertion` | Text "insertion" |
| `ui.RoleMain` | 窗口的主要内容，不包括它的栏和侧边栏，如同 HTML 的 `<main>`；一个窗口只有一个 | `AXGroup`, `AXLandmarkMain` | landmark, `xml-roles:main` | Group，主地标类型 |
| `ui.RoleBanner` | 窗口的顶栏，带应用名称或窗口标题，如同网页的 `<header>`；一个窗口至多有一个 | `AXGroup`, `AXLandmarkBanner` | landmark, `xml-roles:banner` | Group，自定义地标类型 "banner" |
| `ui.RoleContentInfo` | 页脚或状态区域，带关于内容的信息，如同网页的 `<footer>`；一个窗口至多有一个 | `AXGroup`, `AXLandmarkContentInfo` | landmark, `xml-roles:contentinfo` | Group，自定义地标类型 "content information" |
| `ui.RoleComplementary` | 补充主要内容并独立存在的内容，如侧边栏或检查器，如同 HTML 的 `<aside>`；有多个时用 `Label` 为每个命名 | `AXGroup`, `AXLandmarkComplementary` | landmark, `xml-roles:complementary` | Group，自定义地标类型 "complementary" |
| `ui.RoleTabPanel` | 标签页的内容，用 `Label` 按对应的标签页命名，并由该标签页的 `Controls` 指向，如同 ARIA 的 Tabs 模式；不是地标 | `AXGroup`，角色描述 "tab panel" | scroll pane，`xml-roles:tabpanel` | Pane "tab panel" |

实时区域会说明自己如何被朗读（`AXARIALive`、ATK 的 `live` 和
`container-live` 属性、`LiveSetting`），MyGo 像 Web 那样朗读它们：日志显示
之后，添加到其中的内容会在屏幕阅读器正在朗读的内容之后播报，如同
`c.Announce` 那样；警报的文本在它出现时和每次变化时立即朗读，并打断当前
朗读（VoiceOver 的高优先级播报，ATK 的 `notification` 信号——ATK 2.50 支持
它时为 assertive，否则改用一次播报——以及 Windows 上的 assertive 实时
区域）；状态（`ui.RoleStatus`）的文本变化时，会按 `aria-atomic` 的规定，
等待当前朗读结束，再完整朗读。辅助技术接入时已经显示的内容，或日志、状态刚出现时已有的内容，
都不会被朗读；计时器本身则不朗读任何内容：

```go
ui.Column(c).Role(ui.RoleLog).Label("Messages").Children(func() {
	for _, m := range app.messages {
		ui.Row(c).Key(m.ID).Gap(4).Children(func() {
			ui.Text(c, m.From+":").Bold()
			ui.Text(c, m.Text)
		})
	}
})
if app.nameErr != "" {
	ui.Text(c, app.nameErr).TextColor(t.Danger).Role(ui.RoleAlert)
}
ui.Row(c).Role(ui.RoleToolbar).Label("Format").Children(func() {
	boldAndItalic(c)
	ui.Divider(c).Role(ui.RoleSeparator)
	alignment(c)
})
```

日志会朗读它新增的元素，每一行的元素一起朗读；警报会朗读它的文本；状态在
变化后朗读全部文本（清空时不朗读）。状态区域通过元素身份与上一帧对应，
因此，若前面可能插入其他元素，应为它设置 `Key`；嵌套在日志、警报或其他
状态区域中的状态会随外层区域一起朗读，不会单独朗读。为行设置键（`Key`）后，
即使在已有行之前插入新行，也能区分日志中的原有内容与新增内容。

屏幕阅读器会列出地标并跳转到它们，所以窗口顶部的搜索字段及其按钮放在搜索
地标中，如同 HTML 的 `<search>`：

```go
ui.Row(c).Role(ui.RoleSearch).Label("Search mail").Gap(6).Children(func() {
	ui.SearchField(c, &app.query).Label("Search mail").Grow(1)
	if ui.Button(c, "Search").Clicked() {
		app.search()
	}
})
```

信息流中的每个帖子，或讨论串中的每条评论，都是一篇文章，如同 HTML 的
`<article>`，屏幕阅读器会逐个浏览：

```go
for _, p := range app.posts {
	ui.Column(c).Key(p.ID).Role(ui.RoleArticle).Label(p.Title).Gap(4).Children(func() {
		ui.Text(c, p.Title).Bold().Role(ui.RoleHeading).Level(3)
		ui.Text(c, p.Body)
	})
}
```

被移除和添加的内容，如差异中的行或新价格旁的旧价格，会说明这一点，而不是被
当作纯文本朗读：

```go
ui.Row(c).Gap(6).Children(func() {
	ui.Text(c, "$40").Strikethrough().TextColor(t.TextMuted).Role(ui.RoleDeletion)
	ui.Text(c, "$30").Role(ui.RoleInsertion)
})
```

每个组件的页面会说明辅助技术能看到它的哪些信息。
