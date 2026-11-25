# 自动化

自动化插件（`plugins/automation`）允许其他进程控制正在运行的应用：端到端测试工具或智能体可以列出窗口、读取原生 UI 的元素信息、操作元素、发送指针、滚轮、按键和文本输入、选择应用菜单项、在页面中执行脚本、将窗口截取为 PNG 图像，并等待界面稳定。macOS、Linux 和 Windows 使用同一套协议，通过 Unix 域套接字传输以换行分隔的 JSON。

元素正是 MyGo 提供给辅助技术的那些元素，因此客户端看到的与屏幕阅读器看到的一致，操作方式也相同；它使用引擎的 ID，以及应用为元素设置的 `Key`。[ui.Tester](../ui/testing.md) 仍然是在 `go test` 中测试视图的方式；这个插件针对的是构建好的应用，从外部驱动。

客户端可以自行实现[协议](#协议)，也可以使用面向智能体的命令行客户端 [`mygo-automation`](#命令行mygo-automation)。

## 开启

应用要启用它，只需导入这个包。用一个构建标签把它排除在应用的发布版本之外，单独放在一个文件里：

```go
//go:build e2e

package main

import _ "github.com/egoist/mygo/plugins/automation"
```

```sh
go build -tags e2e -o app .
MYGO_AUTOMATION=/tmp/app.sock ./app
```

只有当 `MYGO_AUTOMATION` 指定了套接字路径时，它才会监听；否则什么都不做。不含原生 UI 检查器的构建会把它排除在外，正如这些构建也排除检查器：`go build -tags mygo_noinspector` 和 `mygo build` 的发布版本构建都是如此，除非 `-debug` 或 `MYGO_INSPECTOR=1` 保留检查器。这样的构建即使导入了它，也会忽略 `MYGO_AUTOMATION`，并在标准错误上说明这一点。

应用退出时没有关闭套接字，留在该路径上的套接字会被替换；但另一个应用正在监听的套接字不会被替换，不是套接字的文件也不会。应用退出时会删除自己的套接字。

## 协议

客户端连接到套接字并写入请求，每行一个 JSON 对象。应用按顺序逐行回答每个请求，前一个得到回答后才回答下一个；希望请求并行运行的客户端（例如对一个窗口执行 `idle`，同时对另一个窗口执行操作）可以打开更多连接。

```json
{"id": 1, "method": "tree", "params": {"window": 1}}
```

`id` 是任意 JSON 值，回答会原样重复它。回答要么带 `result`，要么带 `error`，其中 `code` 用来判断，`message` 用来阅读：

```json
{"id": 1, "result": {"window": 1, "nodes": [...]}}
{"id": 2, "error": {"code": "no_window", "message": "no window 7"}}
```

| 代码 | 含义 |
| --- | --- |
| `parse_error` | 该行不是 JSON 对象（此时其 `id` 为 `null`）。 |
| `invalid_request` | 没有 `method`，或 `params` 不是该方法接受的，或其中的值无法使用。 |
| `unknown_method` | 没有这个方法。 |
| `no_window` | 没有窗口拥有该 ID，或应用没有窗口。 |
| `no_element` | 窗口最后一帧中没有元素拥有该 ID 或 `key`。 |
| `unsupported` | 窗口无法执行该操作，例如读取页面的元素。 |
| `timeout` | `idle` 等待超过 `timeout`，已超时。 |
| `failed` | 应用无法完成，例如脚本抛出异常。 |

除 `windows` 外，每个方法都接受一个 `window`，即 `windows` 列出的 ID；省略时取获得焦点的窗口，没有则取第一个窗口。坐标和尺寸以 DIP 为单位，相对于窗口的内容，元素的边界也是如此。

| 方法 | 参数 | 结果 |
| --- | --- | --- |
| `windows` | | `{windows: [{id, title, kind, url?, bounds, contentBounds, focused, visible}]}` |
| `tree` | `window` | `{window, focus?, nodes: [...], announcements?}` |
| `act` | `window`, `id` 或 `key`, `action`, `text?`, `from?`, `to?` | `{}` |
| `input` | `window`, `events` | `{}` |
| `menu` | `window`, `path` | `{}` |
| `capture` | `window` | `{png, width, height}` |
| `eval` | `window`, `script` | `{value}` |
| `idle` | `window`, `timeout?` | `{}` |

### windows

列出打开的窗口。原生 UI 窗口的 `kind` 是 `content`，显示网页的窗口是 `page`，并给出其 `url`；`bounds` 是窗口在屏幕上的位置和大小，`contentBounds` 是内容区域的位置和大小，均为 `{x, y, width, height}`。

```json
{"id": 1, "method": "windows"}
{"id": 1, "result": {"windows": [{"id": 1, "title": "Counter", "kind": "content", "bounds": {"x": 440, "y": 362, "width": 400, "height": 300}, "contentBounds": {"x": 440, "y": 387, "width": 400, "height": 275}, "focused": true, "visible": true}]}}
```

### tree

窗口最后一帧的元素，按树序排列，父元素在其子元素之前：

```json
{"id": 2, "method": "tree", "params": {"window": 1}}
{"id": 2, "result": {"window": 1, "focus": "5878896818624359798", "nodes": [
  {"id": "15956256791619451359", "role": "text", "label": "Count: 0", "bounds": {"x": 20, "y": 20, "width": 360, "height": 18}},
  {"id": "5424175759328386547", "role": "button", "key": "add", "label": "Add", "bounds": {"x": 20, "y": 48, "width": 360, "height": 32}, "states": ["focusable"], "actions": ["press", "focus"]},
  {"id": "5878896818624359798", "role": "textField", "key": "name", "label": "Name", "value": "Ada", "bounds": {"x": 20, "y": 90, "width": 360, "height": 32}, "states": ["focusable"], "actions": ["focus", "setValue"], "selection": [3, 3]}
]}}
```

每个节点都有 `id`（除非位于顶层，否则还有 `parent` 的 ID）、`role`，以及它拥有的以下字段：`key`、`label`、`value`、`placeholder`、`description`、`bounds`、`states`、`actions`、`min`、`max`、`now` 和 `step`（用于滑块、进度条、仪表和步进器）、`selection`（文本框的选区，以 rune 计）、`level`、`posInSet`、`setSize`、`rowCount`、`colCount`、`rowIndex` 和 `colIndex`（表格或网格的尺寸，以及其中行或单元格从 1 开始的位置，与应用给出的相同：ARIA 的 `aria-rowcount` 等）、`rowSpan` 和 `colSpan`（单元格跨多少行和列，当多于一行或一列时：`aria-rowspan` 和 `aria-colspan`）、`current`（它是什么的当前项，如 ARIA 的 `aria-current`：`true`、`page`、`step`、`location`、`date` 或 `time`）、`autoComplete`（文本框或组合框的补全建议所补全的内容，如 ARIA 的 `aria-autocomplete`：`inline`、`list` 或 `both`）、`roleDescription`（对应 `aria-roledescription`）、`hasPopup`（它打开的弹出层类型，对应 `aria-haspopup`：`menu`、`listbox`、`tree`、`grid` 或 `dialog`）、`controls`（它控制的元素的 ID，对应 `aria-controls`）、`describedBy`（描述它的元素的 ID，对应 `aria-describedby`）和 `errorMessage`（在它 `invalid` 时保存其错误的元素，对应 `aria-errormessage`）。
ID 是数字字符串：它们是 64 位整数，JavaScript 的数字类型会将其舍入。`keyShortcuts` 是按下或聚焦该元素所用的按键，对应 ARIA 的 `aria-keyshortcuts`，写法与平台菜单相同（macOS 上为 `⌘K`，其他地方为 `Ctrl+K`），以空格分隔。

`key` 是元素的 `Key`，通过 `Element.Key` 给出，或者对于在创建之前就需要 `key` 的控件，通过 `Context.Key` 给出：字符串、布尔值或整数保持原样，其他值则按 Go 打印出来的形式。ID 可能随视图变化而在帧之间改变，`key` 则不同：它是应用给元素起的名字，旨在长期有效；给测试需要查找的元素设置专属的 `key`：

```go
c.Key("name")
ui.TextInput(c, &name).Label("Name")
ui.Button(c, "Add").Key("add")
```

树中只包含辅助技术会读取的元素：用于布局其他元素的行或列没有节点，无论是否设置了 `key`。角色有 `group`、`text`、`button`、`link`、`checkBox`、`radio`、`switch`、`slider`、`progress`、`textField`、`image`、`list`、`scroll`、`dialog`、`popup`、`tooltip`、`popUpButton`、`tabList`、`tab`、`splitter`、`status`、`table`、`row`、`cell`、`columnHeader`、`tree`、`treeItem`、`listItem`、`menuButton`、`toolbar`、`radioGroup`、`toggleButton`、`comboBox`、`disclosure`、`meter`、`stepper`、`colorWell`、`alertDialog`、`menu`、`menuBar`、`menuItem`、`menuItemCheckBox`、`menuItemRadio`、`heading`、`region`、`log`、`timer`、`alert`、`separator`、`rowHeader`、`search`、`article`、`navigation`、`grid`、`treeGrid`、`deletion`、`insertion`、`tabPanel`、`main`、`banner`、`contentInfo`、`complementary` 和 `form`。状态有 `focusable`、`disabled`、`checked`、`mixed`、`selected`、`expanded`、`multiline`、`password`、`readOnly`、`selectable`、`offscreen`、`multiselectable`、`segment`、`search`、`invalid`、`sortAscending`、`sortDescending`、`expandable`、`vertical`、`busy`、`sortOther` 和 `horizontal`。

原生 UI 中的 webview 会显示为一个标记为 `webView` 的节点，不含其页面的元素。显示网页的窗口没有树：`tree` 回答 `unsupported`，客户端用 `eval` 读取页面。

### act

像辅助技术那样对元素执行操作：通过 `id` 找到元素，或者当恰好一个元素拥有某个 `key` 时通过 `key` 找到。

```json
{"id": 3, "method": "act", "params": {"window": 1, "key": "add", "action": "press"}}
{"id": 4, "method": "act", "params": {"window": 1, "key": "name", "action": "setValue", "text": "Grace"}}
```

| 操作 | 作用 |
| --- | --- |
| `press` | 点击按钮、勾选复选框、打开菜单按钮。 |
| `focus` | 把键盘焦点交给该元素。 |
| `increment`、`decrement` | 步进滑块或步进器。 |
| `setValue` | 用 `text` 替换文本框的文本。 |
| `insertText` | 在文本框的插入点输入 `text`。 |
| `selectText` | 选中文本框从 `from` 到 `to` 的 rune。 |
| `scrollIntoView` | 滚动元素周围的容器以显示它。 |
| `expand`、`collapse` | 打开或关闭树中的项。 |

### input

按照系统事件的传递路径，依次向窗口的内容发送事件：

```json
{"id": 5, "method": "input", "params": {"window": 1, "events": [
  {"type": "click", "x": 200, "y": 99},
  {"type": "text", "text": "Grace"},
  {"type": "key", "key": "CmdOrCtrl+A"}
]}}
```

| 类型 | 字段 |
| --- | --- |
| `pointerMove` | `x`、`y`、`mods?` |
| `pointerDown`、`pointerUp` | `x`、`y`、`button?`、`clicks?`、`mods?` |
| `click` | `x`、`y`、`button?`、`clicks?`、`mods?`：先移动，然后每次点击各一次按下和释放 |
| `wheel` | `x`、`y`、`dx`、`dy`、`precise?`、`mods?`：按 `dx`、`dy` 以 DIP 为单位滚动，`precise` 为真时像触摸板一样 |
| `keyDown`、`keyUp`、`key` | `key`：按键及其修饰键，写法与菜单快捷键相同，如 `Enter`、`Shift+Tab` 或 `CmdOrCtrl+Z`；`key` 按下并释放它 |
| `text` | `text`：输入的文本，或由输入法提交的文本 |

`button` 是 `left`（默认）、`right` 或 `middle`；`clicks` 统计双击或三击的按下次数；`mods` 列出 `shift`、`ctrl`、`alt`、`super` 和 `cmdOrCtrl`。按键事件不输入文本：用 `text` 事件发送文本，正如后端对会产生文本的按键同时发送两者。

内容会在绘制下一帧之前依次处理同一个请求中的事件，就像处理同时到达的事件一样。当某个事件依赖前一个事件显示的结果时（例如向点击聚焦的文本框输入），在两者之间用 `idle` 等待。输入发送到原生 UI 的窗口；页面回答 `unsupported`，客户端用 `eval` 驱动它们。

### menu

像菜单栏那样，按路径上的标签选择应用菜单中的菜单项，并在该项的 `Click` 返回后作出回答：

```json
{"id": 6, "method": "menu", "params": {"path": ["File", "New Window"]}}
```

在 Linux 和 Windows 上，它从窗口的菜单栏中选择，菜单栏显示的就是应用菜单。Dock 和托盘图标的菜单，以及打开的上下文菜单，都不在范围内：系统在自己的事件循环中显示上下文菜单，插件并非在每个平台上都能从中选择，所以不要用 `input` 右键打开上下文菜单。用 [ui.Tester](../ui/testing.md) 测试它们，其 `RightClick` 和 `ChooseMenuItem` 会打开视图的上下文菜单并从中选择。

### capture

把窗口截取为 PNG 图像，以 base64 编码，尺寸为 `width` × `height` 设备像素：原生 UI 按其原样渲染，网页则是截图。

### eval

在显示网页的窗口的页面中执行 JavaScript，与 `Page.Eval` 一样：表达式的值（会等待 promise），或语句 `return` 的内容。

```json
{"id": 7, "method": "eval", "params": {"window": 2, "script": "document.title"}}
{"id": 7, "result": {"value": "Todo"}}
```

### idle

在窗口稳定后回答，让客户端等待 UI 而不是休眠：对原生 UI，等内容不再请求它尚未绘制的帧（由 `Tester.FramePending` 告知）；对网页，等它加载完成并响应过一次脚本。它不会等待 `Context.After` 或 `Painter.After` 安排在稍后的事项，测试器的帧也不会。永远不会稳定的内容（例如持续移动的指示器）永远不会回答：给出以毫秒计的 `timeout`，让它改为以 `timeout` 错误失败。

```json
{"id": 8, "method": "idle", "params": {"window": 1, "timeout": 5000}}
```

### 看不见的窗口

窗口完全不可见时（例如隐藏或最小化的窗口、被其他窗口遮挡的窗口，或锁屏时的任何窗口），它会保留自己的帧直到再次显示，而它的绘制表面在此之前可能一帧都不绘制：macOS 对隐藏窗口不绘制，锁屏时对任何窗口也不绘制。在这种情况下，请求会自行构建窗口等待的帧：`act` 和 `input` 立即生效，就像按下操作在构建的这一帧中生效一样；`tree`、`capture` 和 `idle` 则说明窗口当前显示的内容，`idle` 不会等待窗口显示后才绘制的内容。

## 客户端

任何能打开 Unix 域套接字的语言都可以作为客户端。用 Go 写的测试：

```go
conn, err := net.Dial("unix", "/tmp/app.sock")
if err != nil {
	t.Fatal(err)
}
r := bufio.NewReader(conn)
call := func(method string, params any) json.RawMessage {
	line, _ := json.Marshal(map[string]any{"id": 1, "method": method, "params": params})
	conn.Write(append(line, '\n'))
	answer, _ := r.ReadBytes('\n')
	var res struct {
		Result json.RawMessage
		Error  *struct{ Code, Message string }
	}
	json.Unmarshal(answer, &res)
	if res.Error != nil {
		t.Fatalf("%s: %s", method, res.Error.Message)
	}
	return res.Result
}
call("act", map[string]any{"key": "add", "action": "press"})
call("idle", nil)
tree := call("tree", nil)
```

在 shell 中用 `socat`：

```sh
echo '{"id": 1, "method": "windows"}' | socat - UNIX-CONNECT:/tmp/app.sock
```

## 命令行：mygo-automation

AI 编程智能体在控制自己开发的应用时，需要用简短命令查看和操作 UI，并获得简短响应，而不是自行编写套接字客户端，或将整棵 JSON 树和 20 位的元素 ID 读入上下文。`mygo-automation` 就是这样的客户端，使用同一个套接字：

```sh
mygo-automation open --socket /tmp/app.sock
mygo-automation snapshot -i
# @e2 [button] "Add" key=add
# @e3 [textField] "Name" key=name
mygo-automation press @e2 --settle
# - @e1 [text] "Count: 0"
# + @e1 [text] "Count: 1"
# = @e2 [button] "Add" key=add
mygo-automation fill @e3 "Ada" --settle
mygo-automation screenshot /tmp/app.png
mygo-automation close
```

`snapshot` 每行打印窗口的一个元素：短引用（`@e1`）、角色、标签、值、`key` 和状态，为空的部分省略；`-i` 只保留控件，`--json` 打印协议的原始回答。操作接受一个引用、`key=<key>` 或 `id=<digits>`，覆盖协议的方法：`press`、`fill` 及其他操作、`click`、`pointer`、`wheel`、`type`、`key`、`menu`、`eval`、`wait`、`screenshot`（保存到文件）和 `windows`。使用 `--settle` 时，操作会用 `idle` 和超时等待窗口稳定，并打印自上次快照以来的变化：`+` 和 `-` 表示出现、消失或改变的元素，`=` 表示它们周围的元素，并带上各自的引用。

会话（`--session`，或 `MYGO_AUTOMATION_SESSION`）将套接字、窗口和引用信息保存在用户缓存目录下的独立文件中，因此并行运行的智能体不会互相修改引用。只要窗口仍显示对应元素，引用就一直有效，且绝不会分配给其他元素；元素消失后，使用该引用会被拒绝，提示已过期。`close` 结束会话，但不退出应用。

`mygo-automation skills` 打印它的指南，即内置于命令中的 `SKILL.md`：所安装版本的命令、会话、选择器、引用、等待稳定与差异、限制和错误，别无其他。

### 安装

这个命令位于 MyGo 的模块中。在应用的模块目录里，`go install` 会构建应用的 `go.mod` 所要求的 MyGo 版本，使命令及其指南与应用匹配：

```sh
go install github.com/egoist/mygo/plugins/automation/cmd/mygo-automation
```

在其他位置安装时，要指明该版本号；在应用的模块中，可以用 `go list -m github.com/egoist/mygo` 打印出来：

```sh
go install github.com/egoist/mygo/plugins/automation/cmd/mygo-automation@vX.Y.Z
```

`mygo-automation version` 打印它构建时所用的版本。升级应用的 MyGo 不会升级已安装的命令：升级时重新安装它，指南也随之而来。它只需要 Go 的标准库，并且与插件一样可在 macOS、Linux 和 Windows 上运行。

### 面向智能体的技能

指南由命令本身提供，因此智能体的配置只需说明如何获取指南。将以下简短说明保存为技能文件：Claude Code 使用 `.claude/skills/mygo-automation/SKILL.md`，Codex 及从同一位置读取技能的智能体使用 `.agents/skills/mygo-automation/SKILL.md`：

````markdown
---
name: mygo-automation
description: Use when reading a MyGo app's native UI elements and acting on them, sending pointer, wheel, key and text input, choosing items of the application menu, evaluating scripts in pages, or taking screenshots of a running MyGo app.
---

This file is a discovery stub, not the usage guide. Before running a mygo-automation command, load the actual workflow from the CLI:

```bash
mygo-automation skills # start here
```

Do not probe first with --help or --version. Only if the command is missing, install it from the app's module directory with `go install github.com/egoist/mygo/plugins/automation/cmd/mygo-automation`.
````

## 安全

任何打开套接字的人都能像应用的用户一样驱动应用，并在其页面中运行脚本：不要把插件放进对外发布的构建版本中。`mygo-automation` 与其他客户端一样只是套接字的客户端：它不添加监听器，只连接正在监听的应用。它需要两层主动启用：应用只在导入它时才链接它（构建标签让这仅限于测试构建），并且只在设置了 `MYGO_AUTOMATION` 时才监听。不含检查器的构建无论导入了什么都会把它排除。

套接字只属于该用户：在 macOS 和 Linux 上以 `0600` 模式创建；在 Windows 上，套接字的文件没有模式，因此沿用其所在目录的访问控制列表，在用户的配置目录和临时目录中，该目录只有用户自己能访问。把它放在只有该用户可写的目录中，不要放在与他人共享的目录里。

## 平台

Windows 需要 Windows 10 1803 或更高版本，才有 Unix 域套接字。浏览器（`GOOS=js`）没有套接字：插件在那里可以编译，但不会在任何地方监听。
