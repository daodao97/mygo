# 表格

<!-- preview table 310 -->

`ui.Table` 把[列表](list.md)的行排成列，放在由 `ui.TableColumn` 组成的表头下，每列都有标题、宽度（设为 0 则分摊其他列剩余的空间）和对齐方式；`cell` 构建某行某列的内容，通常是 `Text`。它和列表一样只构建视口中的行，行的高度等于最高单元格的高度，因此换行的文本会把所在行撑高。

```go
cols := []ui.TableColumn{{Title: "Name"}, {Title: "Size", Width: 90, Align: ui.End}}
app.files.Selected = &app.file
if ui.Table(c, &app.files, cols, len(files), func(row, col int) {
	switch col {
	case 0:
		ui.Text(c, files[row].Name).SingleLine()
	case 1:
		ui.Text(c, files[row].Size())
	}
}).Grow(1).Submitted() {
	app.open(files[app.file])
}
```

## 选择行

表格接受与列表相同的 `ListState`（也可以传 `nil`），并可获得键盘焦点：设置 `Selected` 后，单击或按键可选择一行；设置 `Selection` 后可选择多行，`Submitted` 报告双击或 Enter；`Key`、`Label`、`Reorder`、`DragRows` 等的用法与列表相同。由 `Header` 指定的标题行横跨所有列，交给 `cell` 按第 0 列构建；所属分区的各行滚动时，标题行固定在顶部。

被选中和悬停的行从一边到另一边都是方角，就像 Finder 的列表视图；`RoundRows` 会按主题的 `Radius` 给它们加圆角，和列表的行一样，适合其他列表已经使用圆角的应用。

行高至少为主题的 `Space(8)`，并随最高的单元格增高。`RowHeight` 则让每一行都恰好为该高度，就像 AppKit 的 `rowHeight`，单元格内容垂直居中：表格更紧凑，可以显示更多行，适合日志或分支列表。表头保持原有高度。

## 选择单元格

配合 `Selection`，`CellSelect` 让表格像电子表格一样选择一块单元格：范围是选中的行，以及从 `Cells.Anchor` 到 `Cells.Cursor` 按显示顺序排列的列（`Cells.All` 表示所有列）。只能是一整块：像 Excel 中按住 Cmd 拖动那样添加的不连续单元格范围不会被选中。

- **单击。** 单击某个单元格会选中它所在的行和列；Shift+单击会把行和列扩展到该单元格；Cmd+单击（Linux 和 Windows 上为 Ctrl+单击）会添加或移除它所在的行，同时保持列不变。在块外的单元格上按下时，指针一落下就会选择；在块内按下则要等到松开。
- **拖动** 掠过单元格时会选中经过的单元格，靠近表格边缘时还会向下和向两侧滚动表格。
- **按键。** Left 和 Right 移动 `Cells.Cursor`，配合 Shift 会扩展列；移动行的按键在不按 Shift 时只保留当前列；Cmd+A 会选择所有行和所有列。在从右向左的界面中，Left 会移到屏幕左侧的列。在 `OutlineTable` 中，Left 和 Right 仍用于展开和收起行，列则用指针选择。
- **右键单击。** 在块外的单元格上右键，会先选中该单元格再打开上下文菜单；在块内右键，块保持不变。`MenuKeepsSelection` 会让块保持原样。
- **行标题。** `RowHeader` 列就像电子表格的行号，用于选择整行：单击或拖动它的单元格会选中整行及所有列，单击它的表头则选择所有行。它不参与排序。

选区内的单元格与选中行一样，使用强调色背景及配套的文字颜色；选中整行时，行标题也如此显示。表格有焦点时，键盘导航当前所在行中 `Cells.Cursor` 对应的单元格会显示边框。单元格内部有焦点时（例如正在编辑），则保持原有外观。选中的行不会整行着色。`SelectedColumns` 按显示顺序返回选区的列 ID；应用可用 `SelectColumns` 实现选择所有行和一段列的命令。`CellSelected(row, col)` 告诉单元格构建器当前单元格是否选中，以便自定义文字颜色适应强调色背景。用 `RowClicked` 读取所在行的单击，或用 `RowDoubleClicked` 读取双击，不会消费点击事件，行仍会处理该事件并更新选区。

`CellsStyle` 用自定义颜色而非强调色绘制块，就像电子表格的两级配色和条纹：`Cell` 填充块内的单元格，`CellText` 为其中文本着色，`Row` 填充选中行的其余部分，`Stripe` 填充隔行。颜色为零时不绘制。游标的边框在块内使用 `CellText`，在块外使用 `Cell`；这些颜色为零时，则使用强调色及其文字颜色。

```go
t := c.Theme()
app.grid.CellSelect = true
app.grid.CellsStyle = ui.CellsStyle{
	Cell:     t.Accent,
	CellText: t.AccentText,
	Row:      t.Accent.Alpha(0.12),
	Stripe:   t.Text.Alpha(0.04),
}
```

```go
cols := []ui.TableColumn{{Title: "#", RowHeader: true, Width: 48, Fixed: true}, {Title: "Name"}, {Title: "Size", Width: 90}}
app.grid.Selection, app.grid.CellSelect = &app.chosen, true
ui.Table(c, &app.grid, cols, len(rows), func(row, col int) {
	text, color := rows[row][col], c.Theme().Text
	if text == "" {
		text, color = "empty", c.Theme().TextMuted
	}
	if app.grid.CellSelected(row, col) {
		color = c.Theme().AccentText // muted does not show on the accent
	}
	ui.Text(c, text).TextColor(color).SingleLine()
}).Grow(1)
```

## 排列列

用户可以像在 Finder 中一样排列列：

- **调整大小。** 拖动列标题的右边缘可以调整该列宽度，范围在 `MinWidth` 和 `MaxWidth` 之间；双击该处会让列宽适应单元格。它前面那些分摊剩余空间的列会保持宽度，使边缘跟随指针。
- **移动。** 拖动标题可以把该列移到其他列之间。
- **横向滚动。** 当列的总宽超过表格时，可以横向滚动，标题也一起滚动。
- **固定列。** `Fixed` 列固定在原处，宽度不变。
- **冻结列。** `Frozen` 列固定在表格的起始端（从右到左时在右侧），其他列在它下面横向滚动。它不同于 `Fixed`：用户仍然可以移动它并调整它的大小。只有从第一列起连续排列的列才会被冻结，每列都要有 `Width`（或用户调整出的宽度），被冻结的列跟随列的顺序。请给它们留足空间：滚动的列只能在被冻结的列留下的空间里看到，所以被冻结的列和表格一样宽或更宽时，其他列就看不到了，表格不会自动取消冻结。

`ListState.VirtualColumns` 让表格在每一行中只构建视野内的列，以及两侧各半个表格宽度的列，就像它只构建视口中的行一样；其余的列用同宽的占位块代替，所以表格的滚动和构建了全部列时一样。它要求每一列都有 `Width`，否则表格会构建全部列。标题始终完整构建，视野外的列不会调用单元格构建函数。

`ListState.Columns` 按列的 `ID`（默认是标题）保留顺序和宽度：可以从保存的设置中设置它来恢复，表格的 `Changed` 报告变化时再保存它。`Columns` 会随指针移动而变化，但 `Changed` 只在用户松开标题或边缘、且顺序或宽度确实变化时报告一次，双击让列宽适应单元格后也会报告一次，这样每次手势只写入一次设置：

```go
if ui.Table(c, &app.files, cols, len(files), cell).Grow(1).Changed() {
	app.settings.Columns = app.files.Columns
	app.saveSettings()
}
```

如果同时设置了 `Sort` 或 `SortBy`，`Changed` 也会报告新的排序：它报告其中任一变化时，两者都要保存。

## 排序

使用 `ListState.Sort` 时，单击 `Sortable` 列的标题会按该列升序排列行，再次单击则反向：标题会显示箭头，辅助技术能读出排序顺序，表格的 `Changed` 也会报告。表格显示行的顺序，就是 `cell` 收到行的顺序，所以要按这个顺序排序，并用 `Key` 让选择跟随对应的行：

```go
cols := []ui.TableColumn{{Title: "Name", Sortable: true}, {Title: "Size", Width: 90, Align: ui.End, Sortable: true}}
app.files.Sort = &app.sort
sorted := sortFiles(app.files, app.sort) // by app.sort.Column, Descending or not
app.files.Key = func(i int) any { return sorted[i].ID }
```

要按多列排序，设置 `ListState.SortBy`。普通单击仍只按一列排序；如果该列原本就是首要排序列，则反转方向。Cmd+单击（Linux 和 Windows 上为 Ctrl+单击）或 Shift+单击会像 Web 数据网格一样，将该列按升序追加为次要排序依据；再次操作改为降序，第三次移除该列。标题显示排序方向；按多列排序时，还会显示优先次序。`SortBy` 每次变化都会创建新切片，不会改变应用保留的旧切片。同时设置 `Sort` 时，它始终对应 `SortBy` 的第一项；没有排序列时则为零值 `SortOrder`，因此原先读取首要排序列的代码仍可使用 `Sort`：

```go
app.files.SortBy = &app.sortBy // []ui.SortOrder, the first first
sorted := sortFiles(app.files, app.sortBy...)
```

## 表头菜单

`ListState.HeaderMenu` 给表头一个上下文菜单，构建方式与 [ContextMenu](context-menu.md) 的相同。`col` 是被单击的列在传给 `Table` 的列中的索引，不管用户把它们移成了什么顺序；如果是表头超出列范围的部分，则为 -1，和 AppKit 的 `clickedColumn` 一样。没有构建任何菜单项时，菜单不会打开：

```go
app.files.HeaderMenu = func(m *ui.Menu, col int) {
	if col >= 0 && m.Item("Hide Column").Chosen() {
		app.hide(cols[col].ID)
	}
	if m.Item("Show All Columns").Chosen() {
		app.showAll()
	}
}
```

## 表头单元格

表格构建每列的表头单元格时，会调用 `ListState.HeaderCell`，传入该单元格和列索引，索引的含义与 `HeaderMenu` 相同。回调构建的内容放在该单元格的标题和排序箭头之后，也可为单元格设置自定义工具提示或卡片：

```go
app.files.HeaderCell = func(cell ui.Element, col int) {
	cell.Tooltip(app.columnTypes[col])
}
```

## 单元格菜单

`ListState.CellMenu` 为单元格提供一个覆盖整个单元格（包括内边距）的上下文菜单，构建方式与 [ContextMenu](context-menu.md) 的相同。`row` 和 `col` 与单元格构建器收到的值相同。没有构建任何菜单项时，菜单不会打开；如果单元格里的内容自带上下文菜单，则会改为由它接管右键单击：

```go
app.grid.CellMenu = func(m *ui.Menu, row, col int) {
	if m.Item("Copy").Chosen() {
		app.copy(row, col)
	}
}
```

## 就地重命名

单元格中的[可编辑文本](editable-text.md)可以就地重命名，就像 Finder 中的文件名：在 macOS 上按 Return，其他平台按 F2，即可编辑选中行的文本；行被选中后再单击它，也一样。

```go
ui.Table(c, &app.files, cols, len(files), func(row, col int) {
	if col == 0 && ui.EditableText(c, &files[row].Name).Changed() {
		app.rename(files[row])
	}
})
```

## 辅助功能

辅助技术看到一个表格，名称来自 `Label`；每行的名称来自其中的文本，并会说明自己在所有行中的位置；列标题会说明行的排序顺序，`RowHeader` 列的单元格会作为行标题为其所在行命名。启用 `CellSelect` 时，块中的单元格会说明自己已被选中。

列中的行树见[大纲](outline.md#在列中)。
