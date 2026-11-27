# 文本输入

<!-- preview text-input 250 -->

`ui.TextInput` 创建一个编辑 `*string` 的单行文本输入框，`ui.TextArea` 则创建多行输入框。它们会随着用户输入修改字符串：直接读取即可；需要跟进处理时用 `Changed`，单行输入框中按下 Enter 时用 `Submitted`。

```go
if ui.TextInput(c, &app.query).Placeholder("Search").Label("Search").Changed() {
	app.results = search(app.query)
}

if ui.TextInput(c, &app.name).AutoFocus().Submitted() {
	app.rename()
}

ui.TextArea(c, &app.notes).Height(160)
```

- `Placeholder` 在输入为空时显示一段文本，在 `TextInput` 中只占一行，在文本区域中会换行。在包裹输入框的字段上，如 `SearchField`、`TokenField` 或 `Combobox`，它会显示在该输入框中。`PlaceholderColor` 用不同于主题 `TextMuted` 的颜色绘制它，例如表单里的警告提示。
- `CaretWidth` 可将光标宽度设为大于 1 DIP，例如终端的方块光标，或适合大字号主题的宽光标；设为 0 则隐藏光标。它只影响绘制，不改变其他行为。
- `Password` 隐藏文本，并在调用它的帧中阻止文本传入剪贴板和输入法：实现显示密码的眼睛按钮时，只需在显示期间不调用它。仅 `TextInput` 支持此选项；与各平台的文本控件一样，多行文本区域没有密码模式。
- `AutoFocus` 让输入框一出现就获得键盘焦点，例如对话框的第一个字段。
- `ReadOnly(true)` 显示文本而不让用户修改：它仍能获得焦点，但没有光标，文本可以被选择和复制。
- `MaxLength(n)` 限制用户输入的文本长度不超过 n，计数方式与 HTML 的 `maxlength` 相同，按 UTF-16 代码单元计算：键入、粘贴或由输入法提交的内容都会被裁到放得下的长度。应用设置的值不会被裁剪；n <= 0 表示不限制长度。
- `Disabled(true)` 使它变灰。
- `OnPaste(fn)` 在粘贴内容（Cmd+V、Ctrl+V，或编辑菜单、上下文菜单中的“粘贴”）插入前调用，参数为剪贴板文本；没有文本时（例如只有图像）传入空字符串。返回 true 表示接管本次粘贴，不再执行默认插入，类似 Web 的 `preventDefault`：

  ```go
  ui.TextArea(c, &app.draft).Lines(1, 8).OnPaste(func(text string) bool {
  	if len(text) > 10_000 {
  		app.attach(text) // a large block becomes an attachment
  		return true
  	}
  	if text == "" {
  		if png := mygo.Clipboard.ReadImage(); png != nil {
  			app.attachImage(png)
  			return true
  		}
  	}
  	return false
  })
  ```

- `OnTextKey(fn)` 在输入框处理非文本按键前调用，包括 Enter、Tab、Escape、Backspace、Delete、方向键等光标移动键，以及带 Cmd 或 Ctrl 的组合键，类似 AppKit 的 `doCommandBySelector` 和网页的 `keydown`。fn 在按键到来时执行，此前到来的按键和文本已处理完毕；返回 true 即接管该按键，阻止默认行为，例如文本区域插入换行、`TextInput` 提交或 Tab 移动焦点。未接管的按键照常处理。与 `HandleInput` 一样，焦点周围的 `Shortcut` 会先处理其可处理的按键，但输入框编辑本身需要的键（例如 Enter 和方向键）除外。fn 可能在两帧之间执行，因此应通过传入的 `ui.TextKey` 读写文本，不要使用视图中留存的 `Element`（在 fn 返回后使用会 panic），例如聊天输入框用 Enter 发送、Shift+Enter 换行、Up 恢复上一条消息：

  ```go
  ui.TextArea(c, &app.draft).Lines(1, 8).OnTextKey(func(k ui.TextKey) bool {
  	switch {
  	case k.Key == ui.KeyEnter && k.Mods == 0:
  		app.send(k.Text())
  		k.SetText("")
  		return true
  	case k.Key == ui.KeyUp && k.Mods == 0 && k.Text() == "":
  		k.SetText(app.lastSent())
  		return true
  	}
  	return false // Shift+Enter breaks the line
  })
  ```

`TextInput` 若窄到放不下文本，有焦点时会让光标保持在视图内，没有焦点时则显示文本开头。

`TextRanges` 在调用它的帧中为文本中的连续片段着色并设置字重，例如消息输入框在输入内容中显示提及：

```go
in := ui.TextAreaBase(c, &app.draft).Lines(1, 8)
for _, m := range mentions(app.draft) {
	in.TextRanges(ui.TextRange{Start: m.start, End: m.end, Color: m.color, Weight: 600})
}
```

范围以文本中的 rune 偏移指定，每帧应用于当前文本。范围可以重叠和嵌套，类似属性字符串（`NSAttributedString`）的属性：`Color` 在 alpha 非零时生效，`Weight` 在字重非零时生效；重叠处的颜色和字重分别取本帧最后一次设置该属性的值。因此，粗体范围内的彩色提及会同时显示为粗体和彩色：

```go
in.TextRanges(
	ui.TextRange{Start: 0, End: 40, Weight: 700},
	ui.TextRange{Start: 10, End: 16, Color: t.Accent},
)
```

密码输入框不显示任何范围。输入法组合时，组合文本以无样式显示在光标处，范围在它周围保持原有样式。

文本区域至少有数行高，并随文本增长；指定高度后，它就在其中滚动，可用滚轮和滚动条，光标移动时保持在视图内。`Lines(min, max)` 让它按文本在当前宽度下换行后的高度伸展，从 `min` 行到 `max` 行，超过后就滚动，例如消息输入框随输入内容增长：

```go
ui.TextArea(c, &app.draft).Lines(1, 8)
```

文本区域按自身宽度自动换行。`NoWrap` 则让每一行保持完整，只在文本的换行符处断行，适合每行一项的文本，例如 URL 列表、配置文件片段或代码片段。比文本区域更宽的行可以横向滚动，用触控板、按住 Shift 滚动鼠标滚轮或拖动水平滚动条都可以；用户输入、移动光标或把选区拖过边缘时，光标都保持在视图内：

```go
ui.TextArea(c, &app.urls).NoWrap().Height(160)
```

文本区域只排版视口中的段落，并在段落变化前复用排版结果，因此即使容纳数十万行的日志或源文件，输入仍然流畅。

## 编辑

文本输入框的编辑方式和平台的文本框一样：用指针选择（双击选择一个词，三击选择一行），用 Shift 和方向键选择，用平台按键按词和行选择；撤销和重做；剪切、复制和粘贴，也可以由“编辑”菜单中的角色执行；还有编辑命令的上下文菜单。它们接受输入法组合的文本，输入法能看到光标周围的文本，因此长按输入、日语转换和预测都像在其他应用中一样工作。输入法组合期间按下的键归它所有，例如 Enter 选择候选、Escape 放弃组合：这些键不会提交任何内容，不会触发快捷键，也不会关闭对话框。各平台的输入法会在应用之前接收这些键，`Tester` 也同样处理，因此先 `Compose` 再 `Key` 就能看到用户得到的结果。输入法不处理的 Ctrl 或 Command 组合键照常触发快捷键。`Composing` 报告它是否正在组合，此时组合文本尚未进入字符串。

输入框会在帧之间保留正在编辑的文本、选区和撤销历史；从别处设置字符串会替换文本。它会保留最后一个撤销步骤，因此应用可以重新格式化用户键入的内容而不丢失那一步。把字符串设为 `""` 不会丢弃历史：Undo 可以恢复清除之前的文本。

如果要用应用的文本替换用户的部分或全部文本，应改为在输入框上调用 `ReplaceText(start, end, s)`。它替换从 `start` 到 `end` 的 rune，作为单独的一步撤销，因此撤销能恢复用户写过的内容，重做则再次换成应用的文本。偏移量会被限制在文本范围内，因此 `0, math.MaxInt` 会替换全部内容，包括同一帧中键入的文本：

```go
msg := ui.TextArea(c, &app.message).Label("Message")
if text, ok := app.generated(); ok {
	msg.ReplaceText(0, math.MaxInt, text) // Undo restores what the user wrote
}

in := ui.TextInput(c, &app.draft)
if name, ok := app.mentionPicked(); ok {
	_, caret := in.TextSelection()
	in.ReplaceText(app.mentionStart, caret, "@"+name+" ")
}
```

`ReplaceText` 把光标放到文本末尾，并像键入一样通过 `Changed` 报告这次编辑。单行输入框会把换行当作空格，`MaxLength` 也不会裁剪它。它同样适用于 `TextInputBuffer` 和 `TextAreaBuffer`。

要开始新的编辑会话，清空字符串并更改输入框的[键](views.md#键)。键改变后会创建新的编辑器，不保留旧的选区、输入法组合状态或撤销历史：

```go
// Keep secret and secretSession in app state.
ui.TextInput(c.Key(app.secretSession), &app.secret).Password().Label("Secret")

if ui.Button(c, "Cancel").Clicked() {
	app.secret = ""
	app.secretSession++ // the next build creates a fresh editor
}
```

如需按索引存储，可以用 `ui.TextInputBuffer` 和 `ui.TextAreaBuffer` 编辑 `*ui.TextBuffer`。从外部用 `buffer.Set("")` 替换其内容时，控件会在下次与缓冲区同步时丢弃撤销和重做历史以及组合状态；选区则被限制到新文本范围内。无论是更改键还是替换缓冲区，都不保证旧文本已从内存中抹去；已有的 `TextSnapshot` 值仍保留其内容。

单行输入框会把粘贴文本中的换行替换为空格。可以用 `OnPaste` 在多行内容被改写或插入之前拒绝它：

```go
ui.TextInput(c, &app.secret).Password().OnPaste(func(text string) bool {
	return strings.ContainsAny(text, "\r\n") // import "strings"
})
```

## 光标

`TextSelection` 以 rune 偏移返回选区的两个端点；端点相等时，该位置就是光标。`SetTextSelection` 可移动光标或设置选区，例如补全正在输入的词后，将光标放在词后：

```go
input := ui.TextArea(c, &app.draft)
if start, _ := input.TextSelection(); app.completed != "" {
	app.draft, start = complete(app.draft, start, app.completed)
	input.SetTextSelection(start, start)
}
```

`TextSelection` 按从小到大的顺序返回选区端点。`TextAnchorCaret` 则返回选择起点和光标位置，以区分正向和反向选择：编辑器用于扩展选区的按键（例如 Shift-Home 选到行首）始终以起点为锚点扩展选区：

```go
anchor, caret := input.TextAnchorCaret()
input.SetTextSelection(anchor, lineStart(app.draft, caret))
```

## 行旁

`TrackLines` 将文本区域各行的布局位置记录到 `ui.TextLines`，供应用在行旁绘制内容，例如在每个逻辑行的首个显示行旁绘制行号，或标记行内片段。逻辑行指换行符之间的文本，自动换行后可能占多个显示行。`Top(i)` 是第 `i` 行在内容中的顶部位置，不含内边距和滚动偏移；`At(y)` 返回指定高度处的行；`Rows(i)` 返回第 `i` 行各显示行起始位置的 rune 偏移；`Height` 是文本总高度。配合文本区域的 [`ScrollState`](scroll.md#保持偏移)，即可在行号栏中绘制可见行的行号：

```go
const pad = 8 // the text area's top padding
ui.Row(c).Grow(1).Children(func() {
	ui.Box(c).Width(48).FillHeight().Draw(func(p *ui.Painter, r ui.Rect) {
		top := r.Y + pad - app.scroll.Y // the top of the area's content
		p.Clip(r, 0, func() {
			for i := app.lines.At(app.scroll.Y); i < app.lines.Count() && top+app.lines.Top(i) < r.Y+r.H; i++ {
				p.Text(r.X+8, top+app.lines.Top(i), strconv.Itoa(i+1), 12, p.Theme().TextMuted)
			}
		})
	})
	ui.TextArea(c, &app.source).Padding(pad, 12).Grow(1).FillHeight().
		TrackLines(&app.lines).TrackScroll(&app.scroll)
})
```

尚未排版的行会使用估算高度，滚动条也依据这些估算值，直到对应行进入显示范围。在 `Draw` 中读取的是当前帧的行布局，构建视图时读取的则是上一帧的布局。

## 错误

在[字段](form.md)中，`Error` 将值标记为无效：输入框使用主题的 `Danger` 颜色绘制边框，辅助技术会在读取输入框时一并读出错误消息。

```go
ui.Field(c, "Email", func() {
	ui.TextInput(c, &app.email)
}).Error(app.emailError)
```

## 无外观

`ui.TextInputBase` 和 `ui.TextAreaBase` 是没有内边距、背景、边框和圆角的文本输入框，适合自行设计的输入框：见[自定义控件](custom-widgets.md)。

## 辅助功能

辅助技术将其识别为文本框或文本区域，名称来自 `Label` 或所在字段，可读取文本值、光标和选区；非只读时，可像键入一样编辑。密码框不会暴露文本值。
