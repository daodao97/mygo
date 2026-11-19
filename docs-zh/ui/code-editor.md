# 代码编辑器

<!-- preview code-editor 320 -->

`ui/editor` 包为原生 UI 提供代码编辑器和[差异查看器](diff-viewer.md)，参考 Zed 设计：支持语法高亮、多个插入符和选区、软换行、折叠、查找与替换、补全、悬停提示，以及应用或语言服务器提供的诊断和内联提示；还能将文本区段显示为其他内容，并提供代码编辑器常用的命令和按键。它只对视口中的行排版和绘制，因此百万行文件也能像十行文件一样滚动和编辑。

```go
import "github.com/egoist/mygo/ui/editor"

// Kept by the app, as its other state.
buf := editor.NewBuffer(source)
buf.SetLanguage(editor.LanguageFor("main.go"))
app.code = &editor.State{Buffer: buf, WrapGuides: []int{100}}

// In the view:
editor.View(c, app.code).Grow(1).FillWidth()
```

`State` 是编辑器的状态，像文本框里的字符串那样跨帧保留：包括缓冲区、选项，以及编辑器自己保留的状态，如选区、折叠和滚动位置。选项是在构建编辑器之前设置的字段；零值即默认值，与 Zed 相同。`View` 构建某个状态的编辑器；它和其他元素一样是可聚焦元素：给它一个大小，再设置 `AutoFocus`，就能立即在里面输入。

[代码编辑器示例](../../examples/code-editor) 可编辑多种语言的文件，提供当前文件的单词补全、悬停提示、TODO 注释诊断，以及行号边栏中的更改标记和差异视图。

## 缓冲区

`Buffer` 是编辑器编辑的文本，带撤销历史和语言。多个编辑器以及差异的两侧可以显示同一个缓冲区：一处的编辑会出现在其他地方，插入符随周围的文本一起移动；撤销撤回的是缓冲区最后一次编辑，不管它由哪个编辑器做出。

```go
buf.Edit(editor.Edit{Start: 0, End: 0, Text: "// Generated.\n"})
buf.Transact(func() { // one step of undo
	buf.Edit(editor.Edit{Start: a, End: b, Text: name})
	buf.Edit(editor.Edit{Start: c, End: d, Text: name})
})
if buf.Modified() {
	_, err := buf.WriteTo(file)
	if err == nil {
		buf.MarkSaved()
	}
}
```

偏移量按 UTF-8 字节计数，`Pos` 是行号和行内字节数，两者都从 0 开始：`Pos` 和 `Offset` 在两者之间转换，`Line`、`Slice` 和 `String` 读取文本。`Set` 以一次编辑替换文本，可以撤销；`Reset` 则作为新文稿替换，不保留历史。

`EditRemote` 应用别处做出的编辑，例如另一台设备的编辑或合并的结果，这些编辑不进历史。撤销和重做仍然只撤回、重做缓冲区自己的步骤，作用在含有这些远程编辑的文本上：

```go
buf.Edit(editor.Edit{Start: 0, End: 0, Text: "// Mine.\n"})
buf.EditRemote(editor.Edit{Start: end, End: end, Text: "// Theirs.\n"})
buf.Undo() // takes back "// Mine.", and "// Theirs." stays
```

与远程编辑重叠的步骤无法再单独撤销或重做，会从历史中移除，但文本保持不变，前后的步骤也会保留。在该步骤涉及的文本边缘插入，或替换紧邻的文本，都不算重叠。因此应比较两份文本，只传入发生变化的区段；不要替换全文，否则会与所有历史步骤重叠。插入符、选区、折叠、诊断和内联提示的位置都会随远程编辑调整，`Modified` 仍通过与已保存文本比较来判断：新文本与磁盘内容一致时，调用 `MarkSaved`。一次远程编辑会调整历史中每个步骤的位置，完整的 10,000 步大约需要一毫秒，与文本大小无关。

行以 `\n` 结尾，和 Zed 一样：带 `\r\n` 或 `\r` 行尾的文本按 `\n` 行尾保留，缓冲区记住每一行原本的行尾，`WriteTo` 按原样写入，因此文件保存后与原来一致，混合行尾也不例外。编辑只改变它拆分或合并的行的行尾：用 Enter 等把一行拆成两行时，两半都沿用原行的行尾；删除换行时保留其后的那个换行；粘贴的换行依次沿用被替换的换行的行尾，其余沿用所在行的行尾，与剪贴板中的写法无关；上下移动行时，行尾随行一起移动。以 `\r` 结尾的行后面若是以 `\n` 结尾的空行，写出后会成为 `\r\n`，读回时只算一个换行，因此该空行的换行改为 `\r\n`。撤销和重做会连同文本一起恢复行尾。`LineEnding` 返回大多数行使用的行尾，`SetLineEnding` 把所有行转换为同一种。

文本是持久化的块树，和 Zed 的 rope 一样：一次编辑只复制几 KB 的数据和树中的一条路径，与文稿大小无关，每个版本都与前一版本共享其余部分。撤销会保留回退每个步骤所需的编辑，`Snapshot` 是缓冲区变化时保持不变的版本，其他 goroutine 无需锁就能读取。缓冲区的方法可以从任何 goroutine 安全调用；修改时用 `Window.Update`，窗口才会显示变化。

## 编辑

编辑器的编辑方式与 Zed 相同，命令绑定到相同的按键（Linux 和 Windows 上用 Control 代替 Command，和 Visual Studio Code 一样）：

- **多个插入符。** Cmd+D 选中选区的下一处出现，或选中插入符处的单词；Cmd+Shift+L 选中所有出现位置；Cmd+Alt+Up 和 Down 在上方或下方一行添加插入符；Alt+单击添加插入符，Alt+Shift+拖动选择一列。输入、删除和每条命令都在每个插入符处生效，Escape 回到单个插入符。Cmd+U 撤销对选区的更改。
- **按结构选择。** Ctrl+Shift+Right 选中单词，然后是它周围的字符串或括号，并继续向外；Ctrl+Shift+Left 返回。
- **行。** Alt+Up 和 Down 移动行，Alt+Shift+Up 和 Down 复制行，Cmd+Shift+K 删除行，Ctrl+J 合并行，Cmd+/ 注释掉这些行，Cmd+] 和 Cmd+[ 增加、减少缩进；Cmd+Enter 在下方打开一行。
- **输入。** 括号和引号在输入时自动闭合，并包裹住选区；Enter 保持缩进，在开括号之后和一对括号之间增加缩进，并延续行注释；闭括号或 `end` 之类的单词在输入时减少其所在行的缩进。撤销会一次撤回一整段连续输入。
- **折叠。** 行号边栏的标记折叠一行下面缩进的行，直到它的闭括号，或折叠连续的一段导入，连同它们之间的空行；Cmd+Alt+[ 和 ] 折叠、展开，Cmd+K Cmd+0 折叠全部，Cmd+K Cmd+J 展开全部，Cmd+K Cmd+1 到 9 按层级折叠。
- **查找。** Cmd+F 打开查找栏，可设置大小写、全词、正则表达式，也可只在选区内查找；Cmd+Alt+F 替换。Cmd+G 转到下一个匹配项，Ctrl+G 转到某一行。
- **指针。** 双击选中一个单词，三击选中一行，拖动会按单词或行扩展选区；在边缘附近拖动会滚动；Cmd+单击转到定义。

`Run` 运行一条命令，效果和菜单项或应用自己的按键一样，`Bindings` 绑定按键：先给 Mods 和 Key，组合键（如 Cmd+K Cmd+0）再给 ThenMods 和 ThenKey。应用的绑定排在默认绑定（`DefaultBindings`）之前，绑定到 `NoCommand` 会从编辑器拿走一个按键，留给应用自己的快捷键。`CommandNamed` 按键位映射给命令的名称找到命令，例如 “selectNext”。

```go
app.code.Bindings = []editor.Binding{
	{Mods: ui.Cmd, Key: ui.KeyP, Command: editor.NoCommand}, // the app's
	{Mods: ui.Cmd | ui.Shift, Key: ui.KeyD, Command: editor.DuplicateSelection},
}
if ui.Button(c, "Format").Clicked() {
	app.code.Run(editor.SortLines)
}
```

`Selections`、`SetSelections`、`Select`、`Cursor` 和 `CursorPos` 用于读取、设置插入符和选区，`Insert` 在每个插入符处输入，`GoTo` 移动插入符，`ScrollTo` 将指定行滚动到可见区域。`Find` 用指定查询打开查找栏，`FindMatches` 统计匹配项；`Fold`、`Unfold` 和 `Folded` 管理行折叠，`FoldImports` 折叠第一组连续的导入语句，可在打开文件时调用；`Focused` 报告编辑器是否有键盘焦点，`VisibleLines` 返回上一帧显示的首末行，可据此实现 Vim 的半页滚动。`ScrollY` 和 `SetScrollY` 读取、设置以 DIP 为单位的滚动距离，`ViewportHeight` 返回视口高度，`LineTop` 返回指定行相对于第一行的起始位置；将总行数传给 `LineTop`，可得到全部行的总高度。这些信息可用于同步滚动并排的编辑器、绘制合并视图各窗格之间连接对应行的条带，或在编辑器旁的代码缩略图中标出当前视口。与滚动容器一样，滚动条滑块只在指针位于编辑器上或拖动滑块时显示；设置 `KeepScrollbars` 可让它始终显示。垂直滚动条上的更改、匹配项和诊断标记始终可见。水平滚动条在编辑器显示过的某一行比编辑器宽时出现；编辑、内联提示、替换或制表符宽度的变化让各行变窄后，它随之消失，文本也滚回可见范围。`NoScrollbars` 移除滚动条和标记，将其空间留给文本。

`OnInput` 会在编辑器之前看到每个输入事件：按键、输入的文本、编辑命令和指针。返回 true 表示取走该事件，编辑器不再处理它；其余事件则照常交给编辑器。应用自己的编辑模式，例如 Vim 的普通模式，就以这种方式取走按键，同时编辑器仍然显示和移动插入符。在插入符处输入的字母以 `InputText` 到达：在某些系统上，它们的按键也会先以 `InputKeyDown` 到达，但取走按键并不会取走它对应的文本，所以这种模式要从 `InputText` 读取字母。输入法组合、提交或替换的内容——例如在 macOS 上按住按键，重音符号会替换插入符前的字母——只交给编辑器。`Bindings` 中组合键的第一个键之后的按键（例如 Cmd+K Cmd+0 中的第二个）归该组合键所有，就像 VS Code 会先解析组合键，再交给扩展的按键。

编辑器有键盘焦点时，`NoInputMethod` 会禁用系统输入法，类似 Neovide 的 Vim 普通模式：按键直接输入键盘布局对应的字符，以 `InputText` 传给 `OnInput`，不经过输入法组字，所以即使启用了中文输入法，按 j 得到的也是 j。把字母作为命令的模式应启用此选项，插入模式则关闭它。浏览器无法禁用输入法，仍会进行组字。

```go
app.code.CursorShape = editor.CursorBlock // normal mode
app.code.NoInputMethod = true
app.code.OnInput = func(ev ui.InputEvent) bool {
	if ev.Kind == ui.InputText {
		return app.vim.normalKeys(ev.Text) // j, k, dd, …
	}
	return false
}
```

## 选项

- `SoftWrap` 在编辑器宽度处换行，宽度更窄时以 `WrapColumn` 列为界换行；换行后的行保留该行的缩进。CJK 文本可以在任意两个字符之间换行，但 `，`、`。` 这类收尾标点不会出现在行首，`（`、`「` 这类开头标点不会出现在行尾。
- `LineNumbers` 设置行号边栏：显示绝对行号、相对插入符所在行的行号，或隐藏行号；`GutterMarks` 在行旁绘制彩色圆点，例如断点标记，`OnGutterClick` 报告对行号的单击。
- `LineBackgrounds` 在文本和选区下方，用指定颜色填充从 `First` 到 `Last` 的行，例如标出按键将要执行的语句。行号边栏保留自己的背景；被折叠的行不占空间，也不绘制背景。与随文本移动的诊断和内联提示不同，这些背景在编辑后仍固定在原行号上；缓冲区的 `Version` 变化时，需要重新计算。
- `TextBackgrounds` 用颜色填充文本的区段，从字节 `Start` 到 `End`，颜色位于文本和选区之下、行背景之上，就像合并时在行内改动过的单词。它们跟随编辑器的布局：一行换行成的各行、制表符、宽字符和横向滚动；折叠隐藏的行不显示任何内容。与行背景一样，文本编辑时它们留在原处，被裁剪到文本末尾，直到缓冲区的 `Version` 变化时重新计算。
- `TabSize` 和 `Indent` 设置缩进：制表符或空格，为 0 时采用该语言文件通常的方式（Go 用制表符）。
- `Whitespace` 把空格和制表符显示为点和箭头：默认只在选区内显示，也可以到处显示、只在行尾显示，或完全不显示。
- `WrapGuides` 在某些列绘制参考线，例如 80 和 100；`NoIndentGuides` 隐藏缩进的参考线。
- `ReadOnly` 让文本可以被选择和复制，但不能编辑。
- `FitContent` 让编辑器的高度随行数增长，而不是在内部滚动，便于在滚动视图或列表中展示代码。
- `KeepScrollbars` 在指针离开时仍显示滚动条的滑块，和滚动容器的 `KeepScrollbars` 一样；`NoScrollbars` 去掉滚动条及其占用的空间，滚轮和按键仍可滚动。
- `Scrollbar` 设置滚动条的样式，和主题的 `ScrollbarStyle` 一样：轨道和滑块的颜色、宽度、圆角以及最短滑块。零值字段跟随主题；`TrackWidth` 为 0 时是 10 DIP，除非主题样式另有设置——编辑器的滚动条一向如此。和 VS Code 的编辑器一样：

  ```go
  st.Scrollbar = ui.ScrollbarStyle{
      TrackWidth: 14, ThumbWidth: 14, Square: true, Flush: true, MinLength: 20,
      Thumb: ui.RGBA(121, 121, 121, 0.4),
      Hover: ui.RGBA(100, 100, 100, 0.7),
      Active: ui.RGBA(191, 191, 191, 0.4),
  }
  ```
- `Font` 和 `LineHeight` 设置文本：默认是系统的等宽字体，13 DIP，行高为字号的 1.5 倍。
- `NoAutoClose`、`NoFolding`、`NoActiveLine` 和 `NoScrollPastEnd` 关闭这些行为。`Placeholder` 在缓冲区为空时显示。
- `ScrollMargin` 设置将插入符滚动到可见区域时，上下保留的行数。0 使用默认的 3 行，正数指定行数，负数表示不留空间。视口较小时，留白随之缩小。到达滚动边界时不再延伸。

  ```go
  st.ScrollMargin = -1 // 不留空间
  ```
- `CursorShape` 决定插入符的画法：竖线（`CursorBar`）；覆盖其后字符的块（`CursorBlock`，Vim 的普通模式如此，块半透明以便看见字符，编辑器没有键盘焦点时带描边）；或下划线（`CursorUnderline`，Vim 的待定操作符如此）。
- `DiffBase` 是行号边栏中对照显示缓冲区更改的文本，比如 Git 中的文件版本：竖条标出新增和更改的行，标记表示删除发生在哪里。Cmd+F8 转到下一处更改，Cmd+Alt+Z 还原插入符处的更改。
- `ContextMenu` 在编辑器自己的上下文菜单项之后添加菜单项。

## 语言与高亮

`LanguageFor` 按文件名找到文件的语言，`LanguageForText` 按文件名，或按文本开头的 `#!` 行（如 `#!/usr/bin/env ruby`）找到它，`LanguageNamed` 按语言自己的名称找到它；`Languages` 列出这些语言。编辑器认识 Go、JavaScript、TypeScript、JSON、Python、Rust、C、C++、Java、Kotlin、Scala、Swift、Dart、C#、shell 脚本、YAML、TOML、SQL、R、Ruby、Lua、PHP、CSS、HTML、ERB、Svelte、XML、Markdown、差异、Dockerfile、Makefile、Protocol Buffers、GraphQL、Zig、Nix、Jinja 模板和 HCL（Terraform）。模板文本的语言由模板扩展名之前的名称决定：`LanguageFor` 把 `compose.yaml.j2` 高亮为 YAML 中的 Jinja，把 `config.json.erb` 高亮为 JSON 中的 ERB，把 `notice.text.erb` 高亮为纯文本中的 ERB。没有这样的名称时，ERB 的文本按 HTML 处理，Jinja 的按纯文本处理。`Language` 说明如何高亮、注释、匹配括号和缩进文本，并用 `Imports`（导入行的模式，Go 是 `^import\b`）说明哪些行可以作为一段一起折叠：可以为另一种语言构建一个，也可以复制一个再修改。

语言默认用 `Tokenizer` 高亮文本：一个函数，输入一行和前一行结束时的状态，返回该行的 token 和该行结束时的状态，工作方式类似 TextMate 语法和 Monaco 的分词器。编辑器只对显示的行分词；编辑后只重新分词编辑处以下的行，直到某一行开始时的状态与编辑前相同，每帧只做时间预算内的工作。

token 是行内一段同属一个 `TokenKind` 的文本，主题为它着色：关键字、类型、函数、字符串、注释等。

```go
// Lines starting with # are comments.
lang := &editor.Language{
	Name:        "Notes",
	Extensions:  []string{".notes"},
	LineComment: "#",
	Tokenizer: editor.TokenizerFunc(func(line string, state uint64, tokens []editor.Token) ([]editor.Token, uint64) {
		kind := editor.TokenText
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			kind = editor.TokenComment
		}
		return append(tokens, editor.Token{End: len(line), Kind: kind}), 0
	}),
}
```

### 高亮器

语言也可以自带高亮器，例如某个解析器（如 tree-sitter）的高亮器：`Language.Highlighter` 为该语言的每个缓冲区创建 `Highlighter`，编辑器用它代替分词器。分词器本身就是一个高亮器（`NewTokenHighlighter`），因此接入高亮器无需改动编辑器。补丁一侧的文本（`DiffState.SetPatch`）由各个差异块的行首尾相接组成：分词器让每个差异块都从状态 0 开始，而自带的高亮器看到的是整段文本，可以自行决定如何高亮。

```go
type Highlighter interface {
	// Edit follows an edit of the text, old and new its text before and
	// after, and changes what it replaced, in order.
	Edit(old, new editor.Snapshot, changes []editor.Change)
	// Highlight works toward the tokens of the lines up to last, for at
	// most budget, and reports whether they are ready.
	Highlight(last int, budget time.Duration) bool
	// Tokens appends the tokens of line i to dst, and reports whether
	// they are final rather than a guess made before Highlight is done.
	Tokens(i int, text string, dst []editor.Token) ([]editor.Token, bool)
}
```

编辑器在主线程上调用这些方法：缓冲区变化时调用 `Edit`；绘制前调用 `Highlight`，传入它要显示的行，之后只要它还返回 false，就在后续帧中继续调用；每绘制一行，再为该行调用 `Tokens`。`Change` 给出一次编辑在旧文本和新文本中替换掉的区段，增量解析器据此修改自己的语法树。解析器也可以改为在后台工作：快照可以从任何 goroutine 安全读取，`Chunks` 一次读取一块；解析完成后，调用创建高亮器时传入的 `invalidate` 函数，就会让显示该缓冲区的编辑器重绘。

知道文本结构的高亮器还可以通过实现以下任一接口告诉编辑器更多信息：

- `SyntaxSelector`：`Enclosing` 返回包含给定区段的最小节点（该节点大于区段本身），“选中更大范围”命令会选它，而不是周围的单词、字符串或括号。
- `Folder`：`FoldEnd` 返回折叠某一行时隐藏的最后一行，而不是由缩进和括号推断出的那一行。
- `BracketMatcher`：`EnclosingBrackets` 返回围绕区段的最内层括号对，编辑器会高亮它们并在其间移动，如 `do` 和 `end`。
- `Indenter`：`IndentAfter` 返回按 Enter 新开的一行要多缩进几级。

## 补全、悬停与诊断

编辑器显示应用或语言服务器所知道的代码信息；请求由应用来应答，可以立即应答，也可以稍后从另一个 goroutine 通过 `Window.Update` 应答：

```go
app.code.OnCompletion = func(r editor.CompletionRequest) {
	go func() {
		items := app.lsp.Complete(r.Offset) // a language server's
		app.win.Update(func() { app.code.ShowCompletions(r.Offset-len(r.Prefix), items) })
	}()
}
app.code.OnHover = func(r editor.HoverRequest) {
	if doc, start, end, ok := app.lsp.Hover(r.Offset); ok {
		app.code.ShowHover(start, end, doc, nil)
	}
}
app.code.SetDiagnostics(buf.Version(), app.lsp.Diagnostics())
```

- `OnCompletion` 在输入单词或按下 Ctrl+Space 时请求补全。`ShowCompletions` 在插入符下方列出补全项，随着输入继续，按已输入的单词过滤和排序，并显示种类、详细信息和文档；Enter 或 Tab 插入选中的项。
- `OnHover` 在指针停在文本上时（由 `HoverDelay` 控制，默认 600 ms），或在插入符处按下 Cmd+K Cmd+I 时，请求要显示的内容。`NoPointerHover` 关闭指针停留触发的悬停，诊断的悬停也一并关闭，如同 VS Code 的 `editor.hover.enabled`；用按键仍可显示悬停。`ShowHover` 在区段上方的浮层中显示文本，或显示由函数在该处构建的内容。指针可以移入悬停浮层；只要指针还在其中，浮层就保持显示，以便选择、复制其中的文本。指针离开文本和浮层后，浮层会稍等片刻再消失，Escape 也能关闭它。
- `SetDiagnostics` 根据问题严重程度绘制波浪下划线，在悬停提示中显示消息，并在滚动条上添加标记。传入诊断所对应的缓冲区版本号，编辑器会根据该版本之后的编辑调整诊断位置。
- `SetInlays` 显示内联提示，即行内并非原文的文本，例如变量的类型；`SetSuggestion` 在插入符之后显示一段文本作为建议，按 Tab 接受，类似于 AI 给出的补全。
- 在单词上 Cmd+单击和 F12 会调用 `OnDefinition`。
- `ShowPopup` 在偏移处的插入符下方显示一个由应用构建的面板，例如跳转到定义找到的多处用法或实现，供从中选择一项。面板打开时会获得键盘，用于应用自己的按键（`panel.Shortcut`）；Escape、在面板外按下、一次编辑或 `ClosePopup` 会关闭它，键盘随之回到编辑器。文本之外的偏移按最近的一端处理。
- `Rect` 返回字节偏移处的插入符显示的位置，单位为 DIP，相对于编辑器框的左上角，取自上一帧的布局，同时返回该行是否显示（被折叠隐藏的行不显示）。应用可以根据文本区段首尾偏移的 `Rect` 绘制属于该区段的内容，例如在元素的 `DrawOver` 中为其画一个边框。

## 替换区段

`SetReplacements` 把文本的区段显示为其他内容，和 Zed 的折叠显示占位符一样：`Text`、由 `Content` 构建的元素，或者什么都不显示。文本仍属于缓冲区：它照原样保存、复制、查找和撤销，辅助技术读到的也是原文。Markdown 编辑器的实时预览会隐藏标记、绘制复选框，但插入符所在的行例外，那里显示源文本以便编辑：

```go
var rs []editor.Replacement
for _, m := range marks(buf) { // the app's parser
	if !onCaretLine(m) {
		rs = append(rs, editor.Replacement{Start: m.Start, End: m.End})
	}
}
rs = append(rs, editor.Replacement{
	Start: box.Start, End: box.End, Label: "Not done", Interactive: true,
	Content: func(c *ui.Context, e ui.Element, r editor.Replacement) {
		e.Children(func() {
			if ui.ButtonBase(c).Fill().KeepFocus().Clicked() {
				buf.Edit(editor.Edit{Start: r.End - 2, End: r.End - 1, Text: "x"})
			}
		})
	},
})
app.code.SetReplacements(rs)
editor.View(c, app.code).Grow(1).FillWidth()
```

替换项和内联提示一样随编辑移动，在它们边缘输入的内容不算在内；它们的文本消失时，替换项也随之消失。文本或选区变化时重新设置即可：此时只会重新布置替换项有变化的行，因此在每一帧中开销都很小。

- **插入符** 会把替换项显示的内容当作一个字符跨过，在它旁边删除则会删掉整个替换项。行内不显示任何内容的区段就像不存在：不会为它多按一次键，Backspace 删除的是它前面的字符，而不是隐藏的文本。
- **`Text`** 以编辑器字体、`Color` 指定的颜色显示；`Color` 为空时用被替换文本的颜色。
- **`Content`** 会拿到编辑器放置的元素，宽 `Width` DIP（为 0 时是与行等高的正方形），高与行相同，和其他部件一样（见[栏与浮层](#栏与浮层)）。宽度由应用指定，以便软换行在尚未布置的行中把它计算在内。除非设置 `Interactive`，否则在它上面按下会落到文本上；`Label` 是给辅助技术的名称。
- **跨行的区段** 显示在它开始的那一行：从起点到其换行符所覆盖的各行不占行高，和折叠的行一样——但它要显示内容时，第一行除外；区段结束的那一行则另起一行显示它后面的内容。

空区段以及与前面区段重叠的区段会被略去。`Replacements` 按编辑移动后的位置返回它们。只布置视口中的行，也只构建这些行的元素，所以长文本可以有多少替换项都不成问题。

[代码编辑器示例](../../examples/code-editor) 有一个 README.md 的实时预览。

## 代码片段

补全的 `Insert` 始终是纯文本，即使设置了 `Kind: editor.CompletionSnippet`（这只决定图标）。要插入带可编辑占位符的代码模板，先解析模板，再设置 `Snippet`：

```go
snippet, err := editor.ParseSnippet("for ${1:index}, ${2:value} := range ${3:items} {\n\t$0\n}")
if err != nil {
    // Offer a plain-text completion instead when a server uses
    // snippet syntax the editor does not support.
    return err
}
code.ShowCompletions(start, []editor.Completion{{
    Label: "forrange", Kind: editor.CompletionSnippet, Snippet: snippet,
}})
// Or insert at every current selection, outside the completion list:
code.InsertSnippet(snippet)
```

模板只需解析一次；`Snippet` 不可变，可由多个编辑器共享。`Text()` 返回初始文本。解析器支持 `$1`、`${1}`、`${1:default}`、转义的美元符号、花括号和反斜杠，以及最终跳转位置 `$0`（省略时为文本末尾）。Tab 按编号从小到大跳转，0 最后访问；Shift+Tab 反向跳转，到第一个字段为止。相同编号共享默认值，前向引用也不例外，并通过编辑器的多选区机制同步编辑。同编号、同位置的空跳转点会像插入符一样合并。滚动位置和输入法位置以原主插入点处该字段的首次出现为准。

补全列表会先于片段的跳转接管 Tab，因此字段本身也可以被补全。Escape 会先关闭打开的补全列表或其他编辑器浮层，然后结束这次片段会话。移动到活动字段之外、从外部编辑缓冲区、撤销/重做或更换缓冲区也会结束会话。每个字段的输入各自构成一个撤销组；撤销插入会恢复旧文本和选区。重做恢复文本和选区，但不重新开始片段导航。在镜像字段处，输入法的组合和提交与普通插入符处一样。新的片段会取代之前的会话。

这里仅支持 TextMate/LSP 语法中用于 Tab 跳转的子集：嵌套占位符、变量、候选项、文本变换、同一编号的冲突默认值，以及无效 UTF-8 都会返回错误。文本保留模板的缩进，CRLF 统一为 LF。`InsertSnippet` 不会修改只读编辑器。状态方法（包括片段插入）与编辑器的其他交互一样，都在 UI 线程上运行。

运行 `go run ./examples/code-editor`，在 Go 文件中输入 `funcdef` 或 `forrange`，然后按 Tab 试用。

## 样式

### 主题

`Theme` 决定编辑器的配色：背景、文本、行号边栏、选区、插入符、参考线、诊断、更改以及每种 token。留作零值的颜色取自 `DefaultTheme`，它会跟随窗口的主题和外观（浅色或深色）。

```go
th := editor.DefaultTheme(c.Theme())
th.Tokens[editor.TokenKeyword] = editor.TokenStyle{Color: ui.RGB(0xc6, 0x78, 0xdd), Bold: true}
app.code.Theme = th
```

### 栏与浮层

查找栏、转到行栏、补全菜单和悬停卡片都是应用可以自行构建的部件，做法和给 ui 的[基础控件](custom-widgets.md)设置样式一样：`RenderFindBar`、`RenderGoToLine`、`RenderCompletions` 和 `RenderHover` 各自收到编辑器放置的元素（位于右上角的工具栏、顶部的栏、插入符下方的列表、被悬停文本上方的卡片），以及一个句柄，用来访问该部件所显示的内容并执行其操作。这个元素会挡住下方文本的按下事件，并在部件需要时保留键盘焦点（补全显示时焦点留在编辑器里，选择悬停文本时留在悬停里）。给这个元素设置样式，构建它的内容；它的位置也可以改变。编辑器自带的外观由 `DefaultFindBar`、`DefaultGoToLine`、`DefaultCompletions` 和 `DefaultHover` 实现，渲染函数可以先调用它们，再重新设置样式。

```go
app.code.RenderFindBar = func(c *ui.Context, bar ui.Element, f *editor.FindBar) {
	t := c.Theme()
	bar.Row().Gap(8).Padding(6).Background(t.Surface).Radius(12).Shadow(0, 4, 12, 0, ui.RGBA(0, 0, 0, 0.2))
	bar.Children(func() {
		f.QueryInput(ui.TextInputBase(c, f.Query()).Placeholder("Search").Width(180))
		ui.Toggle(c, f.Regexp(), ".*").KeepFocus()
		ui.Text(c, f.Status()).TextColor(t.TextMuted)
		if ui.Button(c, "Next").KeepFocus().Clicked() {
			f.Next()
		}
	})
}
app.code.RenderHover = func(c *ui.Context, panel ui.Element, h *editor.HoverCard) {
	editor.DefaultHover(c, panel, h) // the editor's look, on another background
	panel.Background(app.hoverColor)
}
```

- `FindBar` 把字段和开关绑定起来：`Query` 和 `Replacement`，以及 `CaseSensitive`、`WholeWord`、`Regexp` 和 `InSelection` 都是指针，匹配项会随它们变化。`QueryInput` 和 `ReplacementInput` 给输入框装上查找栏的按键：栏打开时查询获得焦点，Enter 和 Shift+Enter 遍历匹配项，Alt+Enter 全选它们，在替换框中按 Enter 和 Cmd+Enter 分别替换一个或全部，Escape 关闭。`Matches` 和 `Status` 统计匹配项，`Next`、`Previous`、`SelectAll`、`Replace`、`ReplaceAll` 和 `Close` 执行操作；`Replacing` 说明这个栏是否也执行替换，Cmd+Alt+F 打开的就是这种。
- `GoToLineBar` 用 `Text` 作为输入框内容（`Input` 给它装上该栏的按键），用 `Cursor`、`LineCount` 和 `Target` 告诉它跳到哪里，另有 `Go` 和 `Close`。
- `CompletionMenu` 有要显示的 `Items`（最优的在前）和当前选中的那一项 `Selected`。`Item(i)` 生成某个菜单项对应的行：方向键移动时它被选中，并滚动到视图中，单击则插入该项；可以设置它的样式、给它内容。`Font`、`LineHeight` 和 `Theme` 让代码的显示效果与编辑器一致；每个 `CompletionKind` 有 `Icon`，以及决定其色调的 `Token`。
- `HoverCard` 包含被悬停文本的 `Diagnostics`、应用传给 `ShowHover` 的 `Text`，以及 `Content`，其中的内容由对应函数构建。

## 辅助功能

辅助技术看到一个以 `Label` 命名的多行文本框，可读取缓冲区全文（包括显示为替换内容的区段），并移动最新选区，还能知道视图中各行的位置，因此 VoiceOver 可以逐行移动并框出正在朗读的行；替换项中的元素以各自的 `Label` 命名；补全显示为列表，查找栏显示为报告匹配数量的工具栏。编辑器菜单、查找栏、行跳转栏及其名称使用窗口语言（[MyGo 自己的字符串](../i18n.md#mygo-自己的字符串)）。输入法与其他文本框一样在编辑器中组字，候选窗口位于插入符处。
