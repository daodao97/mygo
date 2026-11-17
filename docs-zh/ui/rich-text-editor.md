# 富文本编辑器

<!-- preview rich-text-editor 300 -->

`ui/richtext` 包编辑带属性的文本，就像 macOS 上的 NSTextView 和绑定了 `AttributedString` 的 SwiftUI `TextEditor`。它支持：

- **格式：** 粗体、斜体、下划线、删除线、行内代码、字体、字号和颜色。
- **链接：** 单击打开，可从应用或按 ⌘K 设置。
- **段落：** 标题、项目符号列表、编号列表和任务列表、引用、代码块和对齐。
- **附件：** 行内的图像。
- **编辑：** 文字处理器的快捷键和“格式”菜单，以及撤销时让格式随文本一起恢复的功能。
- **数据交换：** 保留格式的复制与粘贴、按当前输入样式组合文本的输入法、Markdown 导出，以及能读出格式的屏幕阅读器。

它只对视口中的段落排版和绘制，因此编辑长文稿与短文稿的开销相近。

```go
import "github.com/egoist/mygo/ui/richtext"

// Kept by the app, as its other state.
app.doc = richtext.NewDocument(richtext.NewText("Hello"))

// In the view:
if richtext.Editor(c, app.doc).Fill().Changed() {
	app.save(app.doc.Text())
}
```

[笔记示例](../../examples/notes) 是一个带工具栏、链接浮层和“格式”菜单的笔记应用，它把笔记保存为 JSON。

## 文本

`Text` 是带属性的文本，由多个段落组成：每段有自己的 `ParagraphStyle`，段内的每个文本段有自己的 `Attributes`。它和字符串一样是值类型。它的方法返回新文本，原文本保持不变，新旧文本共享所有未改动的内容。保留一个版本（用于撤销、稍后保存或比较）只需一个指针；即使其他 goroutine 正在创建新版本，也可从任意 goroutine 读取已有版本。

`ParagraphStyle.QuoteDepth` 将嵌套引用的深度与列表项的 `Indent` 层级分开保存。`Quote: true` 且深度为零时仍表示一层引用。段落拆分、合并、撤销以及 JSON、HTML 和 Markdown 转换都会保留该深度，包括嵌套引用中的列表。

```go
note := richtext.TextOf(
	richtext.Paragraph{Style: richtext.ParagraphStyle{Heading: 1}, Runs: []richtext.Run{{Text: "Groceries"}}},
	richtext.Paragraph{Style: richtext.ParagraphStyle{List: richtext.Tasks}, Runs: []richtext.Run{
		{Text: "Milk, "},
		{Text: "oat", Attributes: richtext.Attributes{Weight: 700}},
	}},
)
note = note.Format(0, 9, func(a *richtext.Attributes) { a.Color = ui.RGB(37, 99, 235) })
note = note.ReplaceString(note.Len(), note.Len(), "\nBread", richtext.Attributes{})
```

偏移量按 rune 计数，换行符也算在内，和 `Element.TextSelection` 一样。读取一个文本：

- `String` 和 `PlainText` 给出它的字符；
- `Paragraphs` 和 `Runs` 给出它的结构；
- `AttributesAt` 和 `ParagraphStyleAt` 描述一个 rune。

修改一个文本：

- `Replace` 和 `ReplaceString` 改变它的文本；
- `Format` 改变一个区段的属性；
- `FormatParagraphs` 改变区段所触及段落的样式。

属性为零值时，采用编辑器的字体、字号和颜色。`Code`、`Link` 和 `Tag`（应用为文本段设置的标记，例如提及或评论）只作用于原有内容，在它们边缘输入的文本不会自动并入其中。

段落的样式及其换行符的属性属于段落本身，文字处理器中也是如此：空段落会记住在那里输入会得到什么样式。段落合并或拆分时，每段都采用其第一个 rune 原来所在段落的样式，AppKit 修正段落样式时也是这样：

- 删除标题后的换行符会保留标题。
- 整个删除标题（包括它的换行符）会留下它后面的段落。
- 粘贴在段落开头的文本会带来它自己的样式。

列表是扁平的，和 Quill、Automerge 一样：段落的 `Indent` 就是它的层级，相邻的项组成一个列表，每个层级都从 1 开始编号（第二级起用字母和罗马数字）。

段落的 `ID` 是应用给它起的名字，可以作为应用保存该段落相关数据的键，例如笔记或时间线。编辑器不显示它；编辑时它会跟着段落文本走：

- 拆分段落时，ID 留在前半段；如果在段落开头按 Enter 把段落往下移，则留在后半段。另一半没有 ID。
- 合并两个段落时保留第一个的 ID，除非第一个是空的。
- 粘贴或插入的段落没有 ID：`Replace` 从不采用它插入文本的 ID，因此不会有两个段落得到同一个 ID。

撤销和重做会随文本一起恢复 ID，JSON 也会保存 ID；剪贴板、HTML、Markdown 和 RTF 则不包含 ID。可以在创建文本时设置 ID（`TextOf`、`UnmarshalJSON`），也可以让文稿为编辑中新建的段落分配 ID，并将其计入创建该段落的同一撤销步骤：

```go
app.doc = richtext.NewDocument(chapter)
app.doc.IdentifyParagraphs(func() string { return rand.Text() })

// On save, every paragraph has an ID:
for start, p := range app.doc.Text().Paragraphs() {
	app.store.Save(p.Style.ID, start, p)
}
```

`MarshalJSON` 和 `UnmarshalJSON` 保存文本。`Markdown` 和 `HTML` 导出它，`ParseMarkdown`、`ParseHTML` 和 `ParseRTF` 分别读取 Markdown、HTML 和 RTF，和粘贴时一样。

## 文稿

`Document` 保存编辑器所编辑的文本、选区及当前输入属性，还有撤销历史，由应用跨帧保留。它的方法可以从任意 goroutine 安全调用，因此工具栏、菜单或定时器都能修改正在显示的文稿，编辑器会在下一帧反映变化。从其他 goroutine 修改时，用 `Window.Update` 安排更新，以确保窗口绘制新的一帧。

命令和编辑器快捷键一样作用于选区。有选中文本时，命令修改相应的文本段或段落，并计为一个撤销步骤。只有插入符时，字符格式命令只改变接下来输入文本的属性；移动插入符后，这些临时输入属性就会清除，NSTextView 中的 ⌘B 也是如此。

```go
f := app.doc.Formatting() // the selection's formats, for a toolbar
bold := ui.Button(c, "B")
if f.Bold == richtext.FormatOn {
	bold.Background(t.Accent.Alpha(0.18))
}
if bold.KeepFocus().Clicked() {
	app.doc.ToggleBold()
}
```

`Formatting` 把每种字符格式表示为 `FormatOn`、`FormatOff` 或 `FormatMixed`，同时给出选区所在的链接和它第一个段落的样式。这些方法改变文稿：

| 方法 | 作用 |
|---|---|
| `ToggleBold`, `ToggleItalic`, `ToggleUnderline`, `ToggleStrikethrough`, `ToggleCode` | 开启一种格式；选区全部都有该格式时关闭 |
| `Format(fn)` | 改变任何属性，如字体、字号或颜色 |
| `SetLink(url)` | 为选区添加链接、为插入符处的链接设置地址，或插入 URL；`""` 移除链接 |
| `SetHeading(level)`, `ToggleList(kind)`, `ToggleQuote`, `ToggleCodeBlock`, `Indent(levels)`, `SetAlign(a)` | 改变选中的段落 |
| `FormatParagraphs(fn)` | 改变任何段落样式 |
| `ToggleChecked(i)` | 勾选包含 rune `i` 的任务 |
| `InsertText`, `Insert`, `InsertAttachment` | 替换选区 |
| `Edit(fn)` | 以任意方式修改文稿，作为一步撤销 |
| `Undo`, `Redo`, `BreakUndoCoalescing` | 回退、重做步骤 |
| `SetText` | 替换文本，如同打开另一条笔记，并清除历史 |
| `IdentifyParagraphs(newID)` | 为没有 `ID` 的段落以及编辑新产生的段落命名 |

`Edit` 将一个 `Edit` 对象传给函数，函数内的 `Replace`、`ReplaceString`、`Format`、`FormatParagraphs` 和 `Select` 操作会合并为一个撤销步骤。选区随编辑更新，规则与 AttributedString 跟踪索引相同：在插入符处插入文本，插入符会移到新文本之后；在选区末端插入文本，选区不会扩展。

```go
app.doc.Edit(func(e *richtext.Edit) {
	e.ReplaceString(0, 0, "Dear Ann,\n", richtext.Attributes{})
	e.Format(0, 8, func(a *richtext.Attributes) { a.Italic = true })
})
```

## 编辑器

`Editor` 具有文本区域的外观：内边距、背景、边框和圆角。`EditorBase` 没有这些，适合应用自定义外观的编辑器。

`Changed` 报告用户在编辑器中所做的编辑，而不是应用通过文稿所做的编辑。`EditorOptions` 设置：

- `Label` 和 `Placeholder`；
- `Font`，编辑器的字体和字号；
- `ReadOnly`，用户可以选择、复制文本并单击其中链接；
- `NoMarkdown`，关闭 Markdown 输入快捷方式；
- `OnLink`，改为自行打开链接，而不是由系统打开；
- `OnLinkCommand`，按 ⌘K 时由它询问链接的 URL，例如在浮层中；
- `TagStyle`，设置带 `Tag` 的文本段的样式；
- `Menu`，用它代替编辑器自带的菜单构建上下文菜单，适合有自己菜单的应用；
- `OnKey`，在编辑器处理按键前调用，供应用先行处理，例如让位于 `Document.CaretRect` 的补全列表接管方向键、Enter 和 Escape；返回 true 表示已处理该按键；
- `Spans`，接收绘制段落所用的 span，可以返回拆分、改色或调整间距后的 span，但不能改变文本，例如按语言高亮代码块，或调整字距；回调的处理逻辑变化时，修改 `SpansVersion` 以触发段落重新排版；
- `Space`，接收段落序号，返回在主题的间距之外还要在该段上方留出的间距（DIP），例如列表在拖动项将要落下的位置让出空间；返回负数会从主题的间距中扣除，最少扣到零，例如让分隔线所在段落离上下文字更近；
- `Theme`，文本的外观（见[主题](#主题)）。

按键与文字处理器相同，macOS 之外用 Control 代替 Command：

| 按键 | macOS | Windows 和 Linux |
|---|---|---|
| 粗体、斜体、下划线 | ⌘B, ⌘I, ⌘U | Ctrl+B, Ctrl+I, Ctrl+U |
| 删除线 | ⇧⌘X | Ctrl+Shift+X |
| 代码 | ⌘E | Ctrl+Shift+E |
| 链接 | ⌘K | Ctrl+K |
| 编号列表、项目符号列表、任务列表 | ⇧⌘7, ⇧⌘8, ⇧⌘L | Ctrl+Shift+7, 8, L |
| 增加缩进、减少缩进 | ⌘], ⌘[（列表中为 Tab 和 ⇧Tab） | Ctrl+], Ctrl+[ |
| 左对齐、居中、右对齐 | ⌘{, ⌘\|, ⌘} | Ctrl+L, Ctrl+E, Ctrl+R |
| 标题 1 到 6、正文 | ⌥⌘1 到 ⌥⌘6, ⌥⌘0 | Ctrl+Alt+1 到 6, Ctrl+Alt+0 |
| 上一个、下一个段落 | ⌥↑, ⌥↓ | Ctrl+↑, Ctrl+↓ |
| 文本开头、结尾 | ⌘↑, ⌘↓ | Ctrl+Home, Ctrl+End |

段落的行为和 Notes、Google Docs 一样：

- Enter 延续列表，在空项上则离开列表。
- 在标题末尾按 Enter 会开始一个正文段落。
- 在列表项、标题、引用或代码块的开头按 Backspace 会先去掉样式，再把段落与上面一段合并。
- 单击任务的复选框会勾选它，单击链接会打开它：仅限 `http`、`https` 或 `mailto` 地址（`ui.SafeToOpen`），和 Markdown 视图打开链接的方式一样，因为在 Windows 上其他协议和路径会启动程序。`OnLink` 则改为自行打开链接。

Markdown 输入快捷方式会自动设置格式：在段落开头，`# ` 到 `###### ` 生成标题，`- `、`* ` 和 `+ ` 生成项目符号列表，`1. ` 生成编号列表，`[ ] ` 和 `[x] ` 生成任务，`> ` 生成引用，```` ``` ```` 后再按 Enter 生成代码块。在词语两侧输入 `**bold**`、`*italic*`、`_italic_`、`` `code` `` 或 `~~strikethrough~~` 时，结束标记会触发相应格式；词内的下划线（例如 `snake_case` 中的）保持原样。撤销会取消自动格式，恢复实际输入的文本。

[“格式”菜单的角色](../menus.md#角色)（`RoleFormatMenu`，或 `RoleBold`、`RoleAlignCenter` 等）会作用于具有键盘焦点的编辑器。“粘贴并匹配样式”会按输入属性粘贴纯文本。

## 主题

`EditorOptions.Theme` 设置文本的外观：文本、弱化文本、链接、插入符和选区的颜色；代码（行内和代码块中）的字体、字号和颜色，以及代码块的圆角和留白；各级标题的字号、字重、颜色、行高和上下留白；正文段落的首行缩进和段间距；列表的缩进、其标记和复选框的颜色；引用的竖条和文字；以及 `Editor` 的内边距，它会取代窗口主题中的内边距。留作零值的项采用窗口主题下 `DefaultTheme` 的值，因此主题只需设置自己改动的部分，也能跟随窗口进入深色模式：

```go
violet := ui.Hex("#7c3aed")
richtext.Editor(c, doc, richtext.EditorOptions{Theme: &richtext.Theme{
	Link:     violet,
	Checkbox: violet,
	Headings: [6]richtext.HeadingStyle{{Size: 2.2, Weight: 800}},
}})
```

长度以 em 为单位，乘以编辑器字号后得到实际尺寸，因此主题会随 `Font` 缩放；内边距和引用竖条宽度除外，它们以 DIP 计。改变主题会使文本重新排版。编辑器显示的颜色也会作为文本段属性传给屏幕阅读器。

正文段落（不包括标题、列表项和代码）默认不额外留出段间距或首行缩进，与笔记应用相同。排版文章时，可以用以下两种方式区分段落：

- `FirstLineIndent` 设置正文段落的首行缩进，类似印刷小说（约 1.2 em）。标题、列表、代码块、空段落或采用其他对齐方式的段落（例如居中的场景分隔符 “* * *”）之后的第一段不缩进；引用的第一段也不缩进，因为这些位置无需再靠缩进区分。只有按起始侧对齐的段落才会缩进。
- `ParagraphSpacing` 在正文段落之间留出间距，屏幕上排版的散文常这样做。标题保留自己的上下间距。

标题的行高默认是正文行高 `Font.LineHeight` 乘以标题字号；设置标题的 `LineHeight`（以标题字号的 em 计）可以另给行高，例如字号大的标题行距更紧。行高不会小于字体自身的上伸加下伸，而各平台的字体不同：Windows 上 Segoe UI 是 1.33 em，所以 24 号标题设行高 30，在那里实际是 31.9。

```go
// 正文 15 号、行高 22，一级标题 24 号、行高 30。
richtext.Editor(c, doc, richtext.EditorOptions{
	Font: ui.Font{Size: 15, LineHeight: 22.0 / 15},
	Theme: &richtext.Theme{Headings: [6]richtext.HeadingStyle{
		{Size: 24.0 / 15, LineHeight: 30.0 / 24},
	}},
})
```

```go
richtext.EditorBase(c, app.doc, richtext.EditorOptions{Theme: &richtext.Theme{
	FirstLineIndent: 1.2,
}})
```

缩进不是字符：插入符、选区、单击和方向键都会跳过它，文字处理器中也是如此。

## 屏幕上的段落

`Document.VisibleParagraphs` 返回编辑器视口内各段落的索引、起始 rune，以及文本行区域的顶部位置和高度。位置以编辑器元素顶部为基准，单位为 DIP，已计入滚动偏移。在构建视图时读取这些数据，可以让编辑器侧栏中的时间线、页边批注或段落编号与段落对齐。将侧栏与编辑器顶部对齐，再用侧栏顶部加上段落顶部偏移，作为绘制位置：

```go
boxes := app.doc.VisibleParagraphs()
ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
	ui.Box(c).Width(40).Clip().Draw(func(p *ui.Painter, r ui.Rect) {
		for _, b := range boxes {
			p.Fill(ui.Rect{X: r.X + 16, Y: r.Y + b.Top, W: 4, H: b.Height}, t.Accent, 2)
		}
	})
	richtext.EditorBase(c, app.doc).Grow(1)
})
```

读取段落数据时，编辑器会同步完成排版，因此返回的矩形就是这一帧中段落的绘制位置。如果之后位置发生变化，例如编辑器滚动、宽度改变或滚动以显示插入符，就会再构建一帧，让侧栏自动跟随，应用无需监听滚动。矩形不包含标题上下边距或段间距，因此在其顶部绘制标记即可对齐段落第一行。`VisibleParagraphs` 和绘制一样，只对视口内的内容排版，长文稿中的开销也与短文稿相近。在编辑器首次显示文稿前，它返回 nil。

## 屏幕上的插入符

`Document.CaretRect` 返回编辑器显示插入符的位置，不论编辑器有没有焦点：一条与插入符高度相同的线，从编辑器元素左上角算起（以 DIP 为单位，已计入滚动），以及它是否在视口内。在构建视图时读取它，就能把弹出层放在插入符处，例如 `/` 或 `@` 之后的补全，其方向键、Enter 和 Escape 由 `OnKey` 接管。把弹出层和编辑器放在同一个盒子里，位置放在插入符底部：

```go
caret, ok := app.doc.CaretRect()
ui.Box(c).Fill().Children(func() {
	richtext.EditorBase(c, app.doc, richtext.EditorOptions{OnKey: app.completionKey}).Fill()
	if app.completing && ok {
		ui.Column(c).Absolute().Left(caret.X).Top(caret.Y + caret.H + 4).Children(func() {
			for _, s := range app.completions {
				ui.Text(c, s)
			}
		})
	}
})
```

和段落一样，插入符的位置就是编辑器在这一帧绘制它的位置：如果某一帧中位置与读取时不同——例如用户输入、编辑器滚动或宽度变化——编辑器会再构建一帧，因此弹出层会跟着插入符。在编辑器显示文稿之前，以及插入符滚出视口（此时弹出层隐藏）时，`ok` 为 false。

## 滚动

编辑器会自行处理滚动：滚轮、滚动条、将选区拖到边缘，以及让插入符保持可见。`Document.ScrollBy` 和 `Document.ScrollToParagraph` 可滚动上次显示该文稿的编辑器，范围限制在文本以内，并请求一帧来显示结果：

| 方法 | 作用 |
|---|---|
| `ScrollBy(dy)` | 滚动 `dy` DIP，正值向下，并返回是否发生了移动 |
| `ScrollToParagraph(i, align)` | 将段落 `i` 对齐编辑器的顶部、中央或底部（`ui.ScrollStart`、`ui.ScrollCenter`、`ui.ScrollEnd`），或以最小滚动量使其可见（`ui.ScrollNearest`），与元素的 `ScrollIntoView` 相同 |

应用在文本上方拖动自定义内容时，例如通过旁边的拖动柄移动内容块，可以在指针接近编辑器顶部或底部时滚动编辑器，与编辑器处理拖动选区时的行为一致：

```go
// y is the pointer's height in the editor's element, h the element's.
if app.dragging {
	switch y, h := app.dragY, app.editorH; {
	case y < 24:
		app.doc.ScrollBy(y - 24)
		c.After(16 * time.Millisecond)
	case y > h-24:
		app.doc.ScrollBy(y - (h - 24))
		c.After(16 * time.Millisecond)
	}
}
```

这两个方法会在滚动时排版可见内容，因此随后读取 `VisibleParagraphs` 和 `CaretRect`，就能得到滚动后段落和插入符的位置。文稿尚未被任何编辑器显示时，两者均不执行操作。

编辑器以视口中的第一段为基准保持滚动位置：上方内容增高时，例如某段首次排版后变高，屏幕上的内容保持不动；该段顶部以下的内容增高时，只推移其下方内容。编辑器具有键盘焦点、且已向下滚动时，如果插入符所在段落可见，就改以该段为基准，类似浏览器优先以获得焦点的可编辑元素作为滚动锚点（CSS Scroll Anchoring 的优先候选项）：视口顶部与插入符之间的内容增高时，例如调整 `EditorOptions.Space` 留出的间距或编辑文本，插入符上方的文本会向上移动，正在输入的行则保持原位。已滚动到顶部的编辑器仍保持在顶部。一次编辑包含多处替换时，编辑器逐处跟随，同样保持位置：在一次 `Document.Edit` 中把视口上方的段落移进视口，视口中没有移动的段落仍留在原处；撤销和重做这次移动时也一样。这类编辑放在视口第一段开头的内容会放到视口上方，与浏览器的滚动锚定一样，锚点保持在新增内容之下，因此撤销移动后视口能回到原处；只有一处替换的编辑仍像以前一样把它显示在视口顶部。撤销和重做最多逐处跟随一个步骤中的 16 处替换；更长的步骤（例如连续输入一段时间）仍按一个区域处理，视口的去向与以前相同。

## 在格式边缘输入

输入的文本继承插入符前一个字符的属性。在段落开头，它继承后一个字符的属性；替换选区时，继承选区第一个字符的属性。这些是 NSTextView 的规则（在 macOS 27 上实测），ProseMirror、Lexical 和 Quill 也遵循：

- 在粗体末尾输入会延续粗体；在它开头则不会。
- 在链接、代码段或标签末尾输入不会延续它。在它们内部输入则会。
- 删除插入符前的文本后，在插入符移动之前，输入的文本会继承它的属性：删掉一个粗体单词再输入，新文本也是粗体。
- 按 Enter 后，继续输入仍保持按 Enter 前输入的属性。

## 附件

附件用对象替换字符（U+FFFC）表示，字符的 `Attributes.Attachment` 保存一个 `Attachment`，内容可以是图像或应用自行绘制的图形。附件作为底部对齐基线的行内盒子参与排版，像单个字符一样被选择、复制和删除，换行不会拆开附件。附件宽于编辑器时，会缩小到可用宽度。

```go
photo, _ := ui.DecodeBitmap(data)
app.doc.InsertAttachment(&richtext.Attachment{Image: photo, Name: "Beach", Width: 240})
```

没有图像或 `Draw` 函数时，附件会在盒子里显示 `Name`，就像尚未加载的图像。HTML 和 Markdown 会写出附件的 `URL`。

## 复制与粘贴

复制把选区以三种格式放入剪贴板：

- 编辑器自己的格式（`richtext.Format`，JSON），MyGo 应用会原样粘贴，在应用内连附件一起；
- HTML；
- 纯文本。

粘贴时会读取剪贴板中内容最丰富的格式：编辑器自己的格式，然后是 HTML，然后是 RTF，然后是 Markdown（`transfer.Markdown`，Markdown 视图和应用复制出的就是这种），最后是纯文本。[Markdown 视图](markdown.md)的选区会带着其格式粘贴。

- HTML，即浏览器、邮件、Word 和 Google Docs 复制的那种，会保留标题、列表、任务列表、引用、代码块、对齐、粗体、斜体、下划线、删除线、代码和链接。
- RTF，即 TextEdit、Pages 和 WordPad 在没有 HTML 时复制的那种，会保留段落、对齐、列表、粗体、斜体、下划线、删除线和链接。字体、字号和颜色会被略去，Notes 也会略去它们，因此粘贴的内容采用编辑器的样式。“粘贴并匹配样式”（⇧⌥⌘V）会按输入属性粘贴纯文本。

## 输入法

输入法在插入符处按当前输入属性生成组合文本，组合过程中显示下划线，与 AppKit 和 Qt 一致。整次输入法组合及其替换的内容计为一个撤销步骤。

## 辅助功能

屏幕阅读器把编辑器读作文本框，带其文本段的格式：

- **VoiceOver** 读取 `AXAttributedStringForRange`：字体、颜色、下划线、删除线和链接。
- **Orca** 读取 ATK 的文本段属性。
- **讲述人**读取 UI Automation 的 Text 模式，可按字符、格式、单词和行读取，给出每个文本段的字体、字重、斜体、行、颜色和标题级别，以及它们不一致时的混合值。

屏幕阅读器也会移动选区。

## 大型文稿

文本采用分块 rope 结构，段落则保存在可保留旧版本的持久化序列中，因此一次编辑只复制受影响的内容及树中通往该处的路径，开销为 O(log n)。编辑器只对视口中的段落排版，缓存附近段落的布局，并估算其他段落的高度。在包含 100,000 个带格式段落的文稿中输入一个字符，再用 CPU 绘制这一帧，所需时间与 1,000 个段落的文稿相近（`ui/richtext` 中的 `BenchmarkTypeFrame`）。

## Markdown

`Text.Markdown` 写出带 GitHub 扩展的 CommonMark：

- 标题、列表、任务列表、引用和围栏代码块；
- `**bold**`、`*italic*`、`~~strikethrough~~`、`` `code` ``、链接和图像；
- 下划线写成 `<u>`；某个位置无法用 Markdown 分隔符表示格式的开始或结束时，改用 `<strong>`、`<em>` 或 `<del>`，例如粗体 “foo.” 后紧接普通文本 “bar” 的情况。

对齐、字体和颜色没有对应的 Markdown，会被略去。

`ParseMarkdown` 可以读回这些内容，也可以读取模型或其他应用生成的 Markdown。它使用 [Markdown 视图](markdown.md)的解析器（`ui/markdown/parser`），支持 CommonMark、GitHub 扩展和脚注。列表项中从第二段起的段落会缩进对齐到该列表项下方；表格每行转换为一个段落，单元格之间用制表符分隔。数学公式保留原始写法，不作渲染。`Markdown` 导出的内容再解析回来，结果保持一致。
