# 文本

`ui.Text` 显示在可用宽度内换行的文本，`ui.Textf` 负责格式化：

```go
ui.Text(c, "Settings").FontSize(20).Bold()
ui.Textf(c, "%d files, %s", n, size).TextColor(c.Theme().TextMuted)
```

## 样式

`FontSize`、`FontWeight`、`Bold`、`Italic`、`Font`、`LineHeight`、`FixedLineHeight`（以 DIP 计，无论字号多大）、`TextColor`、`TextAlign`、`Underline`、`WavyUnderline`、`DottedUnderline`（例如链接标示悬停时）、`Strikethrough`、`DecorationColor` 和 `DecorationThickness`（下划线和删除线的颜色与粗细）、`UnderlineOffset`（下划线顶部在基线下方多远，如 CSS 的 `text-underline-offset`）、`TextBackground`（行背后的高亮）、`LetterSpacing` 和 `FontFeatures` 用来设置样式，可以设在文本上，也可以设在它上方的任何元素上，其中的文本会继承这些样式。`FontFeatures` 按标签开启字体的 OpenType 特性，或用 `tag=value` 设置它们：`FontFeatures("tnum")` 让不断变化的数字宽度一致，`FontFeatures("liga=0")` 关闭连字。

链接的下划线比字体默认位置略低，避开下伸笔画，和 Web 上的 `underline-offset-4` 一样：

```go
ui.Link(c, "Read the docs", url).Underline().DecorationThickness(1).UnderlineOffset(4)
```

`TextStroke(width, color)` 沿字形轮廓描边，描边位于填充之下，如同 CSS 的 `-webkit-text-stroke` 配合 `paint-order: stroke fill`：每个字形周围露出一半描边，形成一圈光晕，让标签在背后的任何内容上都清晰可读。一段文本的所有描边都绘制在该文本所有字形之下，所以描边绝不会盖住旁边的字形；内联元素的文本则用自己的描边绘制。图表中位于柱形上方、用背景色描边的标签：

```go
ui.Text(c, "42%").TextColor(t.Text).TextStroke(3, t.Background)
```

`TextShadow(x, y, blur, color)` 投下文本的阴影，如同 CSS 的 `text-shadow`：以字形为形状，按 `x`、`y` 偏移若干 DIP 并用 `blur` 模糊，绘制在字形和描边下方，让照片上的说明文字更醒目。元素内部各文本也会投下这个阴影，或投下自己的阴影；颜色透明时不投阴影。`Span` 的 `Shadow` 为它的文本投下阴影，由 `NewTextShadow` 创建。下划线和删除线不投阴影，而且只会绘制一个阴影，CSS 则接受一个列表：

```go
ui.Text(c, "Golden hour").TextColor(ui.RGB(255, 255, 255)).TextShadow(0, 1, 4, ui.RGBA(0, 0, 0, 0.6))
ui.RichText(c, ui.Span{Text: "Sale", Shadow: ui.NewTextShadow(0, 2, 3, t.Accent)})
```

`TabSize(n)` 从每行开头起，按字体中 `n` 个空格的间隔设置制表位，和 CSS 的 `tab-size` 一样，这样代码块中的制表符会缩进 2 列或 4 列；如果不设置，制表符就停在系统文本栈默认放置的位置，各系统并不相同：

```go
ui.Text(c, source).Font("monospace").TabSize(4)
```

阿拉伯文、希伯来文以及其他这样书写的文字从右向左阅读，它们与从左向右的文本混排时，按 Unicode 的双向算法处理：段落的方向由其中第一个强方向字符决定。`TextAlign` 的 `Start` 指左侧；段落从右向左读时指右侧，在[从右向左](right-to-left.md)布局的元素中，其中任何文本也是如此——在那里，没有强方向字符的段落，例如数字，也从右向左读。

`SingleLine` 让文本保持在一行，用省略号截断，`MaxLines` 把它限制在几行，`Ellipsis(" →")` 让截断处改用另一个标记；`NoWrap` 保持整行不折，只在换行符处断开。`Balance` 把文本排成宽度大致相同的行，和 CSS 的 `text-wrap: balance` 一样：需要两行的标题会在中间附近分开，而不是在第二行只留一个词。它会保持按完整宽度换行时的行数，并找出仍能排进这些行的最窄宽度，只在文本排版时计算一次。它适用于 `Text` 和 `RichText`，用于短文本：标题、说明文字、卡片上的标签。

```go
ui.Text(c, "Everything you need to ship a desktop app").FontSize(28).Bold().Balance()
```

`Truncated` 报告上一帧是否用省略号截断了元素的文本，就像 `Bounds` 报告上一帧的盒子：第一帧和文本放得下时为 false。对没有自身文本的元素（例如 `Button`）调用时，它报告其内部的文本是否被截断。可以只在文本被截断时把完整文本显示为工具提示，就像 Web 上 `title` 配合 `text-overflow: ellipsis` 的用法：

```go
t := ui.Text(c, name).MaxLines(1)
if t.Truncated() {
	t.Tooltip(name)
}
```

`Pretty` 避免段落的最后一行只剩一个词，实现了 CSS 的 `text-wrap: pretty` 至少具备的功能：从前一行挪一个词到末行，并尽量利用这两行的宽度，更前面的行保持不变。这样末行就不止一个词，也不影响段落其余部分。如果前一行没有多余的词，例如只有一个长词，就保持原样。`TextShape.Pretty` 为 `ShapeRichTextWith` 提供相同行为。

`Selectable` 让用户用指针、Shift 和方向键选择文本，并复制它，例如错误消息或要粘贴到别处的标识符。

把 `Selectable` 放在容器上，在其 `Text` 和 `RichText` 后代之间共享一个选区：

```go
ui.Column(c).Selectable().Gap(12).Children(func() {
	ui.Text(c, "First paragraph.")
	ui.Text(c, "Second paragraph.")
})
```

拖动可以向任一方向跨过段落。Shift+单击和 Shift+方向键会扩展选区，在滚动容器边缘附近拖动会使其滚动。Cmd+C（Linux 和 Windows 上为 Ctrl+C）、编辑菜单的“复制”和上下文菜单会复制选中的文本，按段落构建的顺序用换行符连接。“全选”会选中整个容器，包括视口外已构建的段落。段落中的内联文本和链接会作为该段落的一部分被复制。

容器上的 `CopyData` 决定复制时如何从所选段落的各部分写出内容，例如文稿应用用空行把它们连成文本，同时写出 HTML 以保留格式；`SelectedTexts` 则返回这些部分，供需要以其他方式复制它们的菜单项使用。段落末尾的文本增长时，选区会保留。

嵌套的 `Selectable` 容器有各自的选区。按钮和文本输入保持自己的交互；它们的标签不会加入容器的选区。在文本或容器上使用 `Unselectable` 可以排除它的子树；内联元素跟随其段落的选区。修改选中的文本，或移除选区的一个端点，会清除共享选区。虚拟化列表只包含已经构建的段落，所以要选择整个文稿时，应使用普通的滚动容器。

选区使用主题的 `Selection` 颜色；`SelectionColor` 为元素及其内部文本设置其他选区颜色，例如让彩色气泡上的选区也能清晰可辨。右键单击可选择的文本会显示“复制”和“全选”；在文本或其 `Selectable` 容器上设置 `ContextMenu` 会替换这个菜单，`Menu.EditItems` 可以把“复制”和“全选”加进去。

文本输入框的插入符使用主题的 `Accent` 颜色；`CaretColor` 可以为单个输入框或元素内的所有输入框设置其他颜色，如同 CSS 的 `caret-color`。

## 富文本

`ui.RichText` 在一个段落中混合样式：每个 `ui.Span` 在文本样式之上设置自己改动的部分（字体、大小、粗细、斜体、颜色、下划线、删除线及其颜色、粗细和偏移、背景、字母间距、特性），各个 span 一起换行。`Radius` 让 span 的背景变圆，背景会贴合字形，而不是填满整行，文本中的代码就是这样：

```go
ui.RichText(c,
	ui.Span{Text: "Saved "},
	ui.Span{Text: "report.pdf", Weight: 600},
	ui.Span{Text: " to "},
	ui.Span{Text: "Documents", Color: t.Accent, Underline: true},
)

ui.RichText(c,
	ui.Span{Text: "Run "},
	ui.Span{Text: "go test", Font: "monospace", Background: t.Surface, Radius: 4},
)

ui.RichText(c,
	ui.Span{Text: "Did you mean "},
	ui.Span{Text: "recieve", WavyUnderline: true, DecorationColor: t.Danger},
	ui.Span{Text: "? "},
	ui.Span{Text: "match", Background: ui.RGBA(250, 204, 21, 0.4)},
)
```

`PaddingX` 和 `PaddingY` 给 span 的背景加内边距，背景同样会贴合字形，就像 CSS 给内联盒加内边距：例如句子中的键帽或提及标签。和 Web 上一样，`PaddingX` 会占用行内空间，让 span 的字形内缩、后面的词后移，换行时也会计入这部分宽度；`PaddingY` 只影响绘制，背景会延伸到上下行的区域，但不改变行高。span 换行后，每行的片段两端都有内边距和圆角，和 `box-decoration-break: clone` 一样：

```go
ui.RichText(c,
	ui.Span{Text: "Press "},
	ui.Span{Text: "Ctrl K", Background: t.Surface, Radius: 4, PaddingX: 6, PaddingY: 2},
	ui.Span{Text: " to search"},
)
```

span 的 `LetterSpacing` 为零时继承文本的字距；`NoLetterSpacing` 则显式设置零字距，如同内联盒上的 CSS `letter-spacing: 0`，让数字或代码在拉大字距的大写字母中保持正常间距。句子中的元素可以用 `LetterSpacing(0)` 实现同样的效果：

```go
ui.RichText(c,
	ui.Span{Text: "BUILD "},
	ui.Span{Text: "4.2.1", NoLetterSpacing: true},
).LetterSpacing(2)
```

span 设置 `ObjectWidth` 后，其中的对象替换字符（U+FFFC）就成了内联对象：盒子宽 `ObjectWidth`，基线上方高 `ObjectHeight`，基线下方深 `ObjectDescent`，它们随文字一起换行，行会为它们留出空间，公式的下标正需要如此。`DrawObjects` 在字形之后、文本的裁剪区内绘制这些对象：它收到每个对象在段落中的 rune 及其盒子。

```go
ui.RichText(c,
	ui.Span{Text: "where "},
	ui.Span{Text: "\uFFFC", ObjectWidth: f.W, ObjectHeight: f.Ascent, ObjectDescent: f.Descent},
	ui.Span{Text: " is the mean"},
).DrawObjects(func(p *ui.Painter, at int, r ui.Rect) { f.Draw(p, r) })
```

span 的 `BaselineShift` 把文本在基线上方抬高这么多 DIP，为负时降低，就像上标和下标，不管字体本身有没有对应字形；行会为它留出空间，span 的下划线和删除线也跟着移动：

```go
ui.RichText(c,
	ui.Span{Text: "H"},
	ui.Span{Text: "2", Size: 10, BaselineShift: -3},
	ui.Span{Text: "O boils at 100 °C"},
	ui.Span{Text: "1", Size: 10, BaselineShift: 6},
)
```

文本中的软连字符（U+00AD）是行可以断开的位置：行在那里断开时它显示为连字符，在其他位置则不显示，和浏览器中一样。如果连字符本身放不下，而行除了词内又没有别处可断，连字符就会溢出宽度。

韩文文本（应用语言为韩文，或由 `Lang("ko")` 设定时）在词之间、空格处和标点之后断行，和 CSS 的 `word-break: keep-all` 一样，而不是在任意两个音节之间断开；比一行长的词仍会在词内断开（见[语言](../i18n.md#原生-ui)）。

## 句子中的元素

在文本的 `Children` 中构建的文本元素会延续该段落，如同 HTML 的内联元素：每个元素在段落样式之上为自己的文本设置样式，并保留元素的行为（`Clicked`、`Hovered`、键盘焦点、`Tooltip`、`ContextMenu`）；它的词落到哪一行，哪一行就属于它的区域。内部的[链接](link.md)是句子中的链接，Tab 能到达它，辅助技术也会把它读作链接：

```go
ui.RichText(c).Children(func() {
	ui.Text(c, "Read ")
	ui.Link(c, "the guide", "https://example.com/guide")
	ui.Text(c, " or ")
	if ui.Text(c, "show an example").TextColor(t.Accent).Clicked() {
		app.example = true
	}
	ui.Text(c, ".")
})
```

文本中只能放文本元素（`Text`、`Link`、`RichText`）；它们的大小、内边距、边框和圆角都不起作用，`Background` 会高亮它们的文本。

`Opacity` 让元素在句子中的词整体变淡，连同其内部元素的颜色一起；`TextColor` 为透明时则什么都不绘制。`Invisible` 像 CSS 的 `visibility: hidden` 一样隐藏它们：它们在行中占的空间仍然保留，因此逐词流式写入回复的文字可以淡入，占位符也可以隐藏，而不会移动周围的文本：

```go
ui.Text(c, "").Children(func() {
	for i, word := range reply.words {
		ui.Text(c, word).Opacity(reply.fadeIn(i)) // 0 to 1 as each arrives
	}
})
```

## 字体

文本由系统自己的文本引擎（Windows 上的 DirectWrite、macOS 上的 Core Text、Linux 上的 Pango）使用系统字体（Segoe UI、SF，或桌面环境的界面字体，GTK 应用用的就是后者）排版和绘制；其他文字和 emoji 像原生应用那样回退到系统字体，从右向左的文本也按正确顺序排列。`Font("monospace")` 选择系统的等宽字体，`ui.RegisterFont` 添加你自己的字体：

```go
//go:embed Inter.ttf
var inter []byte

func init() {
	if err := ui.RegisterFont(inter, "Inter"); err != nil {
		log.Fatal(err)
	}
}
```

之后 `Font("Inter")` 会使用它，也可以在主题的 `Font` 中为所有元素设置它。以同一家族名注册的字体共同组成该家族，不混入它们原属家族的其他字体：在桌面上，会按字重和样式匹配这些字体，不受注册先后顺序影响。用字体自身的家族名仍然能找到它，除非应用在该名下注册了其他字体。因此，两个字体变体声明了相同字重时（例如某个字体的 Light 和 Semilight），要把其中一个注册到单独的家族名下才能区分。

家族列表（如 `Font("Inter, Noto Sans JP")`）会先用系统或应用拥有的第一个家族绘制，缺少的字形再用下一个拥有它的字体补上，之后才轮到系统自己的选择。

`ui.FontFamilies()` 列出文本可用的家族，就像字体设置中列出的那样：先列应用注册的，用应用给它们的名称（没有给就用它们自己的名称），再列系统的，每个只出现一次，排序时不区分大小写。每个 `ui.FontFamily` 会说明它是否为 `Monospaced`（供终端或编辑器的字体设置使用），以及应用是否 `Registered` 了它：

```go
for _, f := range ui.FontFamilies() {
	if f.Monospaced {
		fonts = append(fonts, f.Name)
	}
}
```

第一次调用会查询系统，安装的字体很多时会花些时间（几百个字体要几十毫秒），但不会阻塞窗口中的文本；之后的调用会复用系统的列表，直到调用 `RegisterFont`，因此在应用运行期间安装的字体，要重启后才会出现。浏览器不会透露自己有哪些字体：在浏览器里，列表只包含注册过的家族，而且一个家族要等字体加载完成后才算等宽。

对于控件自行排版的文本（例如终端单元格中的文本），可以用 `ui.Shape` 一次完成字形排版：见[绘制](drawing.md#排版文本)。
