# Markdown

<!-- preview markdown 440 -->

`ui/markdown` 包在原生 UI 中显示 Markdown，支持 CommonMark、GitHub 的表格、任务列表、删除线和自动链接，以及脚注和数学公式。它针对语言模型逐 token 传入的回复设计：只解析新增文本，尚未写完的语法按预期的最终格式显示，文本平滑淡入。

```go
import "github.com/egoist/mygo/ui/markdown"

// Kept in the app's state, shared by every view.
app.md = markdown.Options{}

// In the view:
markdown.View(c, app.readme, &app.md)
```

`View` 显示已完成的文稿。每一帧都给它源文本，就像给 `Text` 一个字符串：它只重新解析自上一帧以来改变的部分，并保留对其余部分的处理结果，包括其段落的文本排版。

## 流式回复

`Stream` 显示仍在写入的 Markdown。把陆续到达的回复 token 追加到字符串中，完成后在同一位置改用 `View`，使视图状态得以延续：

```go
func (app *chat) message(c *ui.Context, m *Message) {
	if m.Done {
		markdown.View(c, m.Text, &app.md)
	} else {
		markdown.Stream(c, m.Text, &app.md)
	}
}
```

追加文本时，只解析追加位置之前仍未闭合的最外层块；如果没有这样的块，就只解析新增的行：一个 token 在长回复末尾的开销与在开头相同，已显示的块既不会重新解析，也不会重新排版。

回复的末尾会显示其语法最可能变成的样子，如聊天应用所显示的那样，而不改变文本：`**bol` 会显示为粗体，未闭合的行内代码段会按代码显示，`$$x_i`（以及启用 `ExtDollarMath` 时的 `$x_i`）会按数学公式显示，其中的 `_` 和 `*` 属于 TeX，而不是强调；`[the docs](https://ex` 是一个目标地址还没传完的链接（尚不可点击），未闭合的代码围栏则按代码显示。还可能变成其他内容的行会等待后面的内容：一行几个标记，如 `-` 或 `==`，会让上面的段落变成标题；表格的表头则会等待分隔行，以免短暂显示成一段竖线。已完成的块永远不会改变，完成后用 `View` 显示的回复，与 CommonMark 读取的完全一致。

### 节奏与淡入

token 会一阵一阵地到达。`Stream` 则以均匀的节奏显示文本，一次一个词，在四分之一秒内跟上已经到达的内容，每个词都淡入。`Options.Reveal` 可以改变方式：

```go
app.md.Reveal = &markdown.Reveal{
	Unit:    markdown.ByLine,         // a line at a time, once complete
	CatchUp: 400 * time.Millisecond,  // text waits at most this long
	Fade:    500 * time.Millisecond,  // and fades in over this
}
```

`ByWord`（默认）、`ByCharacter`、`BySentence`、`ByLine` 和 `ByParagraph` 一次显示一个单位，后三个只在单位完成时显示：句子在句号、问号或感叹号后跟一个空格处结束，或在表意句号（。）或行尾处结束。`AtOnce` 则随到随显示。负的 `Fade` 会关闭淡入，桌面的减弱动态效果也一样。文本通过颜色淡入，块的其他部分（列表标记、代码块、表格的行）则通过不透明度淡入，这只影响绘制：不会为此重新排版任何内容。消息切换到 `View` 时仍待显示的内容会继续到来。

### 回复显示完成之后

回复数据全部到达时，末尾的文字可能还没有显示完：逐步显示仍在继续，淡入也尚未结束。`Revealed` 接受 `View` 或 `Stream` 返回的元素，报告是否已显示全部文本并完成淡入，聊天界面可据此在文字显示完后再展示回复时间和操作按钮：

```go
var md ui.Element
if m.Done {
	md = markdown.View(c, m.Text, &app.md)
} else {
	md = markdown.Stream(c, m.Text, &app.md)
}
if m.Done && markdown.Revealed(md) {
	ui.Text(c, m.Time).TextColor(c.Theme().TextMuted)
}
```

返回值反映当前帧的显示状态：先构建视图，再读取它，并在下方按结果构建相应内容。视图在逐步显示和淡入期间会持续请求帧，因此淡入结束后时间会自动出现。立即显示的文本（`AtOnce`，或没有 `Stream` 遗留待显示内容的 `View`）在第一帧就返回 true；不启用淡入时（负的 `Fade`、减弱动态效果），最后一个词出现时就返回 true。追加文本后会再次变为 false。对于 `Stream`，它只表示目前收到的内容已显示完，因此还需检查回复是否接收完成，如上面的 `m.Done`。

## 一个聊天

把每条消息显示为[列表](list.md)中跟随末尾的一行：列表在回复增长时保持末尾可见，用 `FollowAnimation` 滑向它，直到用户向上滚动；用户滚回末尾后，它会再次跟随。`AtEnd` 和 `ScrollToEnd` 可以用来做一个回到最新消息的按钮。

```go
app.list.Key = func(i int) any { return app.messages[i].ID }
app.list.FollowEnd = true
app.list.FollowAnimation = &ui.Smooth
ui.List(c, &app.list, len(app.messages), func(i int) {
	app.message(c, app.messages[i])
}).Grow(1).Gap(16).Justify(ui.End)
```

[聊天示例](../../examples/markdown-chat)就是这样流式显示预设回复的。

## 选择和复制

视图中的文本是一个跨所有块的选区，如在浏览器中那样。复制会把选中的文本放到剪贴板，段落之间有空行，列表项各占一行，表格单元格以制表符分隔；它会以 HTML 格式复制，以便连同格式粘贴到[富文本编辑器](rich-text-editor.md)、邮件或文字处理软件中；也会以 Markdown 格式复制（`transfer.Markdown`）。上下文菜单中的 `Copy as Markdown`，以及 Shift+Cmd+C（Linux 和 Windows 上为 Shift+Ctrl+C），会复制选区的源文本，补全它截断的强调、代码和链接；没有选区时则复制整个文稿。回复流入时，文本会保持选区。

`ShareSelection` 把选区交给视图周围的 `Selectable` 容器，例如跨越聊天各条消息的一个选区。

## 链接和图像

指向 `http`、`https` 和 `mailto` 地址的链接按 `ui.SafeToOpen` 判断是否安全，与富文本编辑器一致；这些链接的打开方式与[链接](link.md)控件相同，并在工具提示中显示地址。不带协议前缀的路径由包含该视图的路由器处理；`#a-heading` 会滚动到对应锚点的标题（`Slug`）；其他协议（如 `javascript:`）的链接不执行任何操作。设置 `OnLink` 后，则由它处理所有链接点击：

```go
app.md.OnLink = func(url string) { app.open(url) }
```

`LinkMenu` 给每个链接一个自己的[上下文菜单](context-menu.md)，传入它的目标地址，例如一个在回复中链接文件的聊天，会为右键点击的文件提供 `Copy Path` 和 `Reveal in Finder`。链接的菜单会替换视图周围的菜单，如同其 `Selectable` 容器的菜单，而 `EditItems` 会把文本的 `Copy` 和 `Select All` 放回去；指向 `#a-heading` 的链接、脚注引用，以及目标地址还没传完的链接，会保留它们周围的菜单：

```go
app.md.LinkMenu = func(m *ui.Menu, url string) {
	if m.Item("Copy Path").Chosen() {
		app.copyPath(url)
	}
	if m.Item("Reveal in Finder").Chosen() {
		app.reveal(url)
	}
	m.Separator()
	m.EditItems()
}
```

`Anchor` 在某个标题处打开文稿，传入它的 slug（锚点名），可带或不带 `#`：比如一个文档阅读器从搜索结果打开 `guide.md#install`，或回到用户上次所在的位置。视图会在文稿显示时滚动到它，在另一个文稿替换它时以及 `Anchor` 改变时也是如此；`AnchorVersion` 会再次请求同一个标题。处理 `#a-heading` 链接的 `OnLink` 会设置它：

```go
app.md.OnLink = func(url string) {
	if strings.HasPrefix(url, "#") {
		app.md.Anchor = url
		app.md.AnchorVersion++
		return
	}
	app.open(url)
}
```

`AnchorBlock` 找到锚点所指的块，用于 `Anchor`，以及视图自己跟随的指向 `#an-anchor` 的链接：传入视图的文稿（参见[解析器](#解析器)）和不带 `#` 的锚点，它返回该块的索引，或返回 -1，按该 slug 查找标题，与没有它时一样。笔记应用可以按 ID 跳到一个块，比如 Obsidian 的 `[[note#^block-id]]` 跳到以 `^block-id` 结尾的段落：

```go
app.md.AnchorBlock = func(d *parser.Document, name string) int {
	if !strings.HasPrefix(name, "^") {
		return -1 // a heading's slug
	}
	for i := 1; i < d.Len(); i++ {
		if d.Block(i).Kind == parser.Paragraph && strings.HasSuffix(strings.TrimSpace(d.Lines(i)), " "+name) {
			return i
		}
	}
	return -1
}
```

列表项按它所在的行滚动到视野中，表格的行或单元格则滚动整个表格。文稿属于视图：只读取它，不要修改它。

`LinkIcon` 在链接文本的开头放一个图标，由目标地址和周围文本的大小决定：一个像 `InlineMath` 那样生成的盒子，点击效果与该链接相同，并停留在它第一个词所在的行上。作为文本、HTML 和 Markdown 复制时，它都不产生内容。当它返回 false 时，链接没有图标；仍在流式传入的链接也暂时没有。链接从 `Theme.Link` 获取颜色：

```go
app.md.LinkIcon = func(dest string, size float32) (markdown.InlineObject, bool) {
	if !strings.HasPrefix(dest, "http") {
		return markdown.InlineObject{}, false
	}
	return markdown.InlineObject{
		Width: size * 1.2, Height: size * 0.86, Depth: size * 0.14,
		Draw: func(p *ui.Painter, r ui.Rect) {
			p.Icon(globe, ui.Rect{X: r.X, Y: r.Y, W: size, H: size}, app.md.Theme.Link)
		},
	}, true
}
```

`LinkColor` 按目标地址为链接文本着色，返回 true 时用它给出的颜色代替 `Theme.Link`：比如聊天应用把引用来源显示成弱化的颜色，作为旁注；文档阅读器用颜色区分站外链接和站内链接。提及、引用和代码链接同样适用，悬停时链接照常显示下划线；仍在流式传入的链接要等目标地址完整后才着色，此前保持 `Theme.Link`。视图每次构建时都会为显示的每个链接调用它，因此只应查询应用手头已有的数据；它的返回值变化时，段落会随之更新：

```go
app.md.LinkColor = func(dest string) (ui.Color, bool) {
	return ui.Hex("#6b7280"), app.isCitation(dest)
}
```

`LinkIcon` 的图标颜色由它的 `Draw` 决定，上例用的是 `Theme.Link`。若要与 `LinkColor` 着色的链接一致，就用 `LinkColor` 为同一目标地址给出的颜色绘制图标。颜色变化的段落会重新构建，并再次请求它的图标。

图像在 `LoadImage` 加载它们之前会显示描述文字，加载在各自的 goroutine 上进行；如果没有它，图像就一直只是描述文字，因为视图自身不会从网络加载任何内容。`FileImages(dir)` 加载一个目录中的文件。仅由图像组成的段落按它们原本的大小显示，最宽不超过视图。

```go
app.md.LoadImage = markdown.FileImages(filepath.Dir(app.path))
```

## 任务列表

任务列表的项（`- [ ]` 和 `- [x]`）显示复选框，它们只表示任务是否已完成。`OnTask` 让它们成为应用的控件，比如笔记应用在笔记文件中勾掉一项任务：点击复选框，或在它拥有焦点时按 Space 或 Enter，会以该任务在视图源文本中的行号（从 1 起）和请求的状态调用它。复选框始终显示源文本所写的状态，因此由应用写入这一改动，并把新文本交给视图：

```go
app.md.OnTask = func(line int, checked bool) {
	lines := strings.SplitAfter(app.note, "\n")
	mark := "[ ]"
	if checked {
		mark = "[x]"
	}
	l := lines[line-1]
	i := strings.Index(l, "[") // the item's check box, after its marker
	lines[line-1] = l[:i] + mark + l[i+3:]
	app.note = strings.Join(lines, "")
	app.save()
}
```

行号指交给该视图的 Markdown 中的行：在另一个视图的块中构建的视图，比如代码块中的 Markdown，要在行号上加上该块的行（[`SourceLine`](#代码)）。

## 代码

围栏代码块使用[代码编辑器](code-editor.md)的分词器和配色进行语法高亮，逐行沿用上一行的状态，因此流式传入时只需高亮新增行；代码行分段排版，内容增加时只需重新排版最后一段。每个代码块上方显示语言和复制按钮，代码可横向滚动（`WrapCode` 则改为换行）。复制按钮要等代码块闭合后才可用。

`CodeBlock` 代替 `DefaultCodeBlock` 构建代码块，`Code` 则按它列出的语言构建这些块，例如图表。每个都会获得该块的语言、代码，以及围栏是否仍未闭合，并可以使用默认实现的各部分：

```go
app.md.Code = map[string]func(*ui.Context, *markdown.CodeBlockView){
	"mermaid": func(c *ui.Context, b *markdown.CodeBlockView) {
		if b.Open { // still streaming: show the code until it closes
			markdown.DefaultCodeBlock(c, b)
			return
		}
		ui.Image(c, app.diagram(b.Code()))
	},
}
app.md.CodeBlock = func(c *ui.Context, b *markdown.CodeBlockView) {
	ui.Column(c).Gap(4).Children(func() {
		ui.Text(c, b.Language).Bold()
		b.Body(c)       // the highlighted, selectable code
		b.CopyButton(c) // the copy button
	})
}
```

`SourceLine` 是块在视图源文本中开始的行，从 1 起：它的起始围栏，或缩进代码的第一行。围栏代码块代码的第 k 行是源文本的第 `SourceLine()+k` 行，因此把块构建为其中 Markdown 的视图时，可以把该视图的任务按它们在源文本中的行交给外层视图的 `OnTask`：

```go
app.md.Code = map[string]func(*ui.Context, *markdown.CodeBlockView){
	"tasks": func(c *ui.Context, b *markdown.CodeBlockView) {
		start := b.SourceLine()
		app.inner.OnTask = func(line int, checked bool) { app.md.OnTask(start+line, checked) }
		markdown.View(c, b.Code(), &app.inner)
	},
}
```

## 数学和 HTML

`$$` 与 `$$`、`\[` 与 `\]` 或 `\(` 与 `\)` 之间的数学公式以代码字体显示 TeX 源码，支持语言模型常用的写法；启用 `ExtDollarMath` 扩展后也支持 `$…$`，但表示金额的美元符号除外。`Math` 可自定义行间公式块，例如接入自己的渲染器；仍在流式传入的公式块会向 `Block` 标记为 `Open`，因此可先显示 TeX 源码，等闭合后再渲染。

`InlineMath` 根据 TeX 源码和周围文本的字号，将行内公式构建为段落中的对象：宽度为 `Width`，基线上方高度为 `Height`，下方深度为 `Depth`，随文本换行，由 `Draw` 绘制。复制为文本或 Markdown 时会保留公式源码，如 `$x_i$`。回调返回 false 时，公式显示为 TeX 源码。段落每次变化都会调用它，流式传入时可能每帧调用，因此应缓存生成的内容：

```go
app.md.InlineMath = func(tex string, size float32) (markdown.InlineObject, bool) {
	f, err := app.formula(tex, size) // laid out once, kept by TeX and size
	if err != nil {
		return markdown.InlineObject{}, false
	}
	return markdown.InlineObject{Width: f.W, Height: f.Ascent, Depth: f.Descent, Draw: f.Draw}, true
}
```

原始 HTML 显示为文本，这样回复就无法隐藏任何内容。在文本中，模型所写的格式标签（`<b>`、`<i>`、`<u>`、`<s>`、`<code>`、`<kbd>`、`<mark>`、`<sub>`、`<sup>` 和 `<br>`）会生效；`HTML` 构建 HTML 块，例如用于 `<details>` 或 `<think>`。

## 样式

视图的外观与窗口一致，使用窗口的字体和颜色。`Theme` 用来设置要改的部分，其余部分采用 `DefaultTheme` 针对窗口主题的值，因此只设置一个字段的主题只会改变一件事：

```go
app.md.Theme = &markdown.Theme{
	FontSize:     15,
	Link:         ui.Hex("#7c3aed"),
	CodeFont:     "JetBrains Mono",
	BlockSpacing: 1,
	Headings: [6]markdown.HeadingStyle{
		{Size: 1.8, Weight: 800, Rule: true},
		{Size: 1.4, Rule: true},
	},
}
```

主题可设置字体、代码字体及其行高；文本、次要文本、链接、粗体文本和代码的颜色；代码字号；块与列表项之间的间距；各级标题的字号、字重、颜色、间距和分隔线；列表缩进；引用块的竖条和文本；代码块的背景、边框、圆角、内边距和语法高亮配色（`editor.Theme`）；表格的线条、表头、条纹、内边距和最大列宽；高亮文本；以及复选框。长度以 em 为单位，即字号的倍数，因此主题会随字号缩放。`LinkWeight` 设置链接文本的最小字重，让灰度打印或色盲读者等无法仅靠颜色区分链接的场合也能辨认链接。

`Block` 替换任何块的显示方式：每个块都会先询问它，当它返回 true 时由它构建，使用文稿的块和行内元素（参见[解析器](#解析器)）：

```go
app.md.Block = func(c *ui.Context, d *parser.Document, i int) bool {
	b := d.Block(i)
	if b.Kind != parser.ThematicBreak {
		return false
	}
	ui.Text(c, "⁂").TextAlign(ui.Center).AlignSelf(ui.Stretch)
	return true
}
```

## 辅助功能

标题是相应层级的标题，列表及其项是列表和列表项，链接是链接，表格是包含行、列标题和单元格的表格，任务项是复选框（无论是否勾选），图像是按描述命名的图像，代码块是按语言命名的组，并带有复制代码按钮。任务的复选框是只读的；设置 `OnTask` 后，它以任务的文本命名，可获得焦点；辅助功能按下它时，会像点击一样请求另一种状态。

## 解析器

解析器位于独立的 `ui/markdown/parser` 包中，不依赖 ui，因此只需解析 Markdown 的程序（如使用它的富文本编辑器或渲染 HTML 的服务器）不会引入视图代码。`Parse` 读取字符串，`Append` 追加文本，`Edit` 修改指定范围，每种操作都只重新解析受改动影响的部分。编辑后，从改动前的顶层块开始解析，直到在原有块的起始位置遇到新块，且此处没有尚未闭合的内容；从这里起复用原有块，因此长文稿的预览在输入时也只需解析少数几个块。若链接定义出现在引用位置之后，则只重新解析引用该链接的块中的行内元素。

```go
d := parser.Parse(src)
d.Append(token)
for i := range d.Children(0) { // the top-level blocks
	b := d.Block(i)
	if b.Kind == parser.CodeBlock {
		fmt.Println(b.Language(), d.Lines(i))
	}
}
html := d.HTML()
```

块和行内元素以扁平切片保存，按文稿顺序排列，每项包含其在源文本中的字节范围（`Start` 和 `End`），以及所有子元素之后的索引（`Next`）。`Inlines` 首次调用时才解析相应块的行内元素。`New(ext)` 可指定不同于 `Default` 的扩展组合，`SetStreaming` 则像 `Stream` 一样补全末尾语法。

按 GitHub 规范，`HTML` 会转义那些浏览器会一直读到结束的原始标签——`title`、`textarea`、`style`、`xmp`、`iframe`、`noembed`、`noframes`、`script` 和 `plaintext`——于是把文档当 HTML 复制出去时，其中的 `<script>` 到粘贴它的地方仍是文本。这就是 cmark-gfm 的 tagfilter，即 `ExtTagFilter` 扩展，`GFM` 及其包含的 `Default` 都启用了它；`New(ext &^ parser.ExtTagFilter)` 则按原样写出原始 HTML。其他原始 HTML，例如标签上的事件处理器，仍然是 HTML。

它通过 CommonMark 0.31.2 规范的全部 652 个示例以及 GitHub 扩展的示例，对那些在其他解析器中很慢的输入也能保持线性时间，和 cmark 一样，并且不依赖标准库之外的任何东西。

对中文、日文和韩文放宽了一条规则，因为它们不在强调前后加空格：在 CJK 字符、全角标点或弯引号旁边，`*` 和 `~~` 即使在 CommonMark 中会被标点阻止的地方也能开启和闭合，因此 `**结论。**后文` 是粗体。`_` 和单个 `~` 保持 CommonMark 的规则。规范中那个位置没有包含 CJK 文本的示例，所以它们仍然全部通过。

## 性能

在 Apple M5 上以 `CGO_ENABLED=0 GOMAXPROCS=1` 测量一条 50 KB 的回复，它包含标题、嵌套列表、代码、表格和链接：

| | 时间 | 分配 |
|---|---:|---:|
| 完整解析它，包括行内元素 | 0.3 ms | 0.8 MB，1,300 次分配 |
| 追加一个 4 字节的 token，解析尾部的行内元素 | 2 µs | 600 B，2 次分配 |
| 完整显示它的一帧，没有变化 | 1.3 ms | 16 KB，350 次分配 |
| 追加一个 16 字节 token 的一帧 | 0.8 ms | 36 KB，350 次分配 |

帧会像视图那样构建每个块的元素，同时保留解析结果、段落的区间及其文本排版：比聊天回复长得多的文稿，最好一次只显示一节。

<!-- repository-only:start -->

## 运行基准测试和测试

```sh
CGO_ENABLED=0 GOMAXPROCS=1 go test ./ui/markdown -run '^$' -bench . -benchmem
go test ./ui/markdown -run 'TestCommonMark|TestGFM|TestAppend|TestEdits|TestPathological' -v
```

<!-- repository-only:end -->
