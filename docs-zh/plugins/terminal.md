# 终端

终端插件为[原生 UI](../ui/README.md) 应用提供终端：一个视图，在伪终端中运行用户的 shell 或任何程序，使用 [Ghostty](https://ghostty.org) 的终端模拟器 libghostty-vt。全部用 Go 写成，没有 npm 包：模拟器是一个原生库，应用在运行时加载它，无需 cgo，`mygo build` 和 `mygo dev` 会把它打包进应用。

## 设置

创建一个终端（它会启动其程序），并在窗口中显示它：

```go
import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

func main() {
	mygo.App.WhenReady(func() {
		term, err := terminal.New(terminal.Options{})
		if err != nil {
			log.Fatal(err)
		}
		win := mygo.NewWindow(mygo.WindowOptions{
			Title: "Terminal",
			Content: ui.View(func(c *ui.Context) {
				terminal.View(c, term).Fill().AutoFocus()
			}),
		})
		win.OnClosed(func() { term.Close() })
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
```

[终端示例](../../examples/terminal)展示了这个应用。其窗口使用 shell 的标题，并在 shell 退出时关闭。

`terminal.View` 与其他元素一样：用 `Fill` 或 `Grow` 设置它的大小，也可以把它放在其他元素旁边，如同编辑器的窗格。它的大小以字体的单元格为单位决定终端的大小；点击会让它获得键盘焦点。一个终端一次只在一个视图中显示。程序会在视图第一次显示它时启动，以便按视图的大小绘制第一屏；如果没有视图显示它，则在 `New` 之后半秒以 80×24 启动。

终端支持常见终端功能：

- 颜色（16、256 和 24 位）、粗体、斜体、淡色、反色、删除线、上划线和下划线（单线、双线、波浪线、点线、虚线，各自有颜色）；
- Unicode：宽字符、emoji 和字形簇占用其宽度对应的单元格，如同在 Ghostty 中，系统字体会补上终端字体缺少的内容；
- 框线字符、方块和 Powerline 的分隔符由终端绘制而非取自字体，使全屏程序的线条能够相接；
- 在终端调整大小时会重排的回滚缓冲区，以及全屏程序的备用屏幕；
- 按程序要求的键盘：传统序列、xterm 的 modifyOtherKeys 或 Kitty 键盘协议，以及输入法（在光标处组合）。在传统序列中，Ctrl+I、Ctrl+M 和 Ctrl+[ 发送 Tab、Enter 和 Esc，如同 xterm，而 Ghostty 发送 fixterms 的 CSI u，shell 不会读取它；
- 鼠标报告、焦点报告、括号粘贴、同步输出、超链接（OSC 8）、标题（OSC 0 和 2）、工作目录（OSC 7）、在应用允许时复制到剪贴板和读取剪贴板（OSC 52、OSC 5522；见 `ClipboardRead`），以及 Kitty 剪贴板协议的粘贴事件（OSC 5522，模式 5522），后者让程序接收粘贴的图像。

## 使用

- **选择**用指针进行，效果与 Ghostty 相同：拖动选择单元格，双击选择单词，三击选择行，拖过顶部或底部会滚动。Option（其他平台为 Alt）拖动会画出矩形（从 shell 命令之外开始，那里 Option+click 移动光标）。Shift+click 从原有锚点扩展现有选择；之后的 Shift+click 可以扩展或收缩它，包括跨行。当程序接收鼠标时（如编辑器），则用 Shift 选择。
- **移动光标**：在 shell 提示符处正在输入的命令上 Option+click（其他平台为 Alt+click）会把 shell 的光标移到点击的单元格，如同 Ghostty 和 kitty，即使命令跨越多个行：终端发送把光标带到那里的 Left 或 Right 方向键（在应用模式下，当 shell 设置了 DECCKM 时为 `ESC O`）。点击命令末尾之后只会移到其末尾，不会更远，这样 Right 方向键不会接受建议（zsh-autosuggestions、fish）。它需要 shell 用 OSC 133 标记其提示符（shell 集成，如 Ghostty 的、kitty 的或 shell 自己的）。没有它时，在命令之外、全屏程序的屏幕上以及视图显示回滚缓冲区时，Option+click 和 Option 拖拽像以前一样选择；在命令上，点击属于光标，所以矩形从命令之外开始。`Mirror` 也会为另一个终端标记提示符的行，在那里的点击效果相同。
- **指针**是 Ghostty 的：shell 上是 I 形光标，程序接收鼠标时是箭头（按住 Shift 时又变回 I 形，因为那时 Shift 用于选择），以及程序用 OSC 22 请求的形状（`pointer`、`crosshair`、`col-resize`……），直到程序再次接收或离开鼠标。
- **复制和粘贴**在 macOS 上是 Command+C 和 Command+V，编辑菜单的角色也会发送它们，其他地方是 Control+Shift+C 和 Control+Shift+V，以及 Control+Insert 和 Shift+Insert；在 Windows 上是 Control+C 和 Control+V，如同 Windows Terminal（见[按键](#按键)）。程序要求时粘贴是括号粘贴，使粘贴的行不会作为命令运行。上下文菜单有复制、粘贴、全选和清除回滚缓冲区。选项可以添加选择时复制以及其他次要点击行为（见[选项](#选项)）。
- **粘贴图像**：自己读取剪贴板的程序（例如通过模式 5522 请求读取，接受图像的编码代理就是这样）会被告知一次粘贴包含什么：剪贴板的每种格式都表示为 MIME 类型（文本、作为 PNG 的图像、作为 `text/uri-list` 的文件……），然后它读取自己想要的内容，即使在 SSH 上也可以。只有执行粘贴的终端会回答它的读取，内容是粘贴时剪贴板中的内容；显示同一程序的其他终端（如一个会话上的其他窗口）保持沉默。其他程序只会得到文本。如果只有图像，向请求括号粘贴的程序会粘贴一个空的括号粘贴，如同 VS Code 的终端，Claude Code 等代理会以此为契机自己从剪贴板读取图像；对任何其他程序则什么也不粘贴。
- **读取剪贴板**：程序用 OSC 52（`?`）或 OSC 5522 请求，默认被拒绝：OSC 52 得到空剪贴板，OSC 5522 得到 EPERM 状态；`ClipboardRead` 允许它，或先询问应用（见[选项](#选项)）。剪贴板在视图构建一帧时于主线程上读取，答案也在那里写入，所以程序读到的是当时剪贴板中的内容，而不是它请求时的。
- **滚动**：滚轮和触控板滚动回滚缓冲区，Shift+Page Up 和 Shift+Page Down 也是如此。全屏程序会把滚轮作为方向键接收，或在接收鼠标时作为鼠标报告接收。输入内容时视图会滚回底部。
- **缩放**：滚轮加 Control（macOS 上为 Command），以及 Control（Command）配 =、- 和 0，可以让文本变大、变小、恢复原来的大小，每步一个 DIP，与 Ghostty 相同；`OnZoom` 则可以接管缩放。
- **链接**：Command+click（其他地方为 Control+click）会打开程序打印的超链接（OSC 8），或作为文本打印的 URL，即使程序正在接收鼠标也可以；按住 Command 时，指针在它们上面是手形。
- 应用的快捷键优先：macOS 上的 Command 快捷键从不会发送给程序；窗口或终端周围元素用 `Shortcut` 处理的按键，或应用菜单在 macOS 上作为键等价物的按键，都会交给应用处理。

## 选项

```go
term, err := terminal.New(terminal.Options{
	Command:  []string{"htop"},          // the user's login shell when empty
	Dir:      "/tmp",                     // the home directory when empty
	Env:      []string{"EDITOR=vim"},     // added to the app's environment
	Font:     terminal.Font{Family: "JetBrains Mono", Size: 14},
	Theme:    terminal.DarkTheme(),        // follows the window when nil
	Cursor:   terminal.CursorBar,          // a block when zero
	OnTitle:  func(title string) { win.SetTitle(title) },
	OnExit:   func(code int) { win.Close() },
})
```

- `Command` 运行程序而不是 shell。shell 作为登录 shell（`-zsh`）运行，如同终端应用启动它，以便读取用户的配置文件：`$SHELL`，否则 macOS 上是 `/bin/zsh`，其他地方是 `/bin/sh`，Windows 上是 PowerShell。
- `ShellIntegration: true` 启用 zsh 提示符集成。临时的 `.zshenv` 会在加载用户的启动文件之前恢复原来的 `ZDOTDIR`，并在用户的提示符主题之后安装 OSC 133 提示符标记。Powerlevel10k 会被要求为异步重绘发出自己的标记。当列数改变时，在重排之前按原宽度清除已标记的提示符，zsh 在 `SIGWINCH` 之后重绘它；因此右侧和多行提示符不会在回滚缓冲区中累积。命令输出和备用屏幕保留其正常的调整大小行为。用户的文件从不会被修改，临时文件在终端退出或关闭时删除，其他本地 shell 不变。使用 `Conn` 时，数据源必须提供 OSC 133 标记并在调整大小时重绘其提示符；插件不会向远程会话注入 shell 启动配置。集成默认关闭。
- `Env` 添加到环境，环境中已有 `TERM=xterm-256color`、`COLORTERM=truecolor`，在没有设置区域设置时有 `LANG=en_US.UTF-8`，如同从 Finder 启动的应用。
- `Font` 设置字体：默认是 13 DIP 的系统等宽字体（见[字体](#字体)）。
- `Theme` 设置颜色：前景、背景、光标及其下方文本、选择及选中文本，以及 16 色调色板；如果也设置了 `DarkTheme`，它会设置深色窗口的对应颜色（见[主题](#主题)）。`terminal.DarkTheme()` 和 `terminal.LightTheme()` 使用窗口的背景和文本颜色，配上 Visual Studio Code 的调色板；没有主题时，终端会采用窗口当前外观的主题，并随外观变化而更新。程序可以更改颜色（OSC 4、10、11、12）。
- `Cursor` 和 `NoBlink` 设置光标，程序可以更改它。
- `Scrollback` 决定在屏幕上方保留多少字节的输出：默认 10 MB，为负时不保留。
- `ScrollMultiplier` 缩放滚动，如同 Ghostty 的 `mouse-scroll-multiplier`：`Discrete` 是鼠标滚轮一格滚动的行数（默认 3；在 Windows 上它会按“每次滚动行数”设置缩放，相当于把该设置设为 3），`Precision` 缩放触控板的距离（默认 1）。`SetScrollMultiplier` 在运行中的终端里更改它。
- `MinimumContrast` 是文本与其背景之间的最低对比度，如同 Ghostty 的 `minimum-contrast`：WCAG 比率从 1（无，默认）到 21。对比度更低的文本（如程序在浅色主题上的浅灰色）会变暗或变亮到刚好达到它，同时保持其色相。`SetMinimumContrast` 在运行中的终端里更改它。
- `Transparent` 使背景不绘制，如同 Ghostty 的 `background-opacity`，让视图下方的内容透出来：窗口的材质，或元素的半透明背景。有自己背景色的单元格仍会绘制它。
- `Padding` 是网格周围的空间，横向（`X`）和纵向（`Y`）以 DIP 计，如同 Ghostty 的 `window-padding-x` 和 `window-padding-y`：默认横向 6、纵向 4，为负时没有。`PaddingColor` 是内边距显示的颜色，如同 Ghostty 的 `window-padding-color`：默认是背景，或用 `terminal.PaddingExtend` 时是离它最近的单元格的背景，使全屏程序的颜色到达视图边缘，而不是停在网格边缘。如同 Ghostty 的 `extend`，它只延伸每个单元格都有自己的背景的行，使提示符的高亮或程序的标签页和框保持在各自的单元格内；`terminal.PaddingExtendAlways` 延伸每一行。无论哪种方式，反色显示的一个孤立字符（如程序绘制自己的光标）会把自己的背景保留在单元格内。

  ```go
  term, err := terminal.New(terminal.Options{
  	Padding:      terminal.Padding{X: 14, Y: 8},
  	PaddingColor: terminal.PaddingExtend,
  })
  ```
- `OptionAsAlt` 让 macOS 上的 Option 成为程序的 Alt 键（Meta），而不是输入重音字母的键：Option-E 发送 Esc 和 e，而不是原本会输入的 ´。
- `CopyOnSelect: terminal.CopyOnSelectClipboard` 在指针结束选择时以及全选形成选择时，把选择内容复制到剪贴板，如同 Ghostty 的 `copy-on-select = clipboard`。Ghostty 的 `primary`（Linux 的选择剪贴板）在这里没有提供，因为原生 UI 没有这种剪贴板。`SetCopyOnSelect` 在运行中的终端里更改它。
- `Keybinds` 在默认值之上把按键绑定到动作（见[按键](#按键)）。
- `RightClick` 是次要点击所做的事，如同 Ghostty 的 `right-click-action`：默认是上下文菜单，`RightClickPaste`、`RightClickCopy`、`RightClickCopyOrPaste`（复制选择，没有选择时粘贴，如同 Windows Terminal）或 `RightClickIgnore`。复制后会取消选择。接收鼠标的程序仍会收到点击，除非按住 Shift。
- `NoClipboardWrite` 阻止程序写入剪贴板（OSC 52 和 OSC 5522），如同 Ghostty 的 `clipboard-write = deny`；读取由 `ClipboardRead` 管理，而粘贴时向程序提供的内容仍会得到回应。
- `ClipboardRead` 决定如何处理程序的剪贴板读取请求（OSC 52 的“?”，以及 Kitty 剪贴板协议的 OSC 5522）：默认为 `ClipboardDeny`，对应 Ghostty 的 `clipboard-read = deny`（Ghostty 默认询问，但询问需要有视图显示终端）；`ClipboardAllow` 用剪贴板内容响应，`ClipboardAsk` 则先询问应用。视图构建下一帧时，会在主线程上读取剪贴板并写入响应，终端自带的提示框也在这一帧显示：没有视图显示的终端会等待，已关闭的终端则不会再响应读取请求。
- `OnClipboardRequest` 在程序读取剪贴板之前询问应用，当 `ClipboardRead` 为 `ClipboardAsk` 时。它在主线程上运行，参数是一个 `ClipboardRequest`：程序请求的 MIME 类型（`Mimes`）、它请求的是否为可用的类型（`List`，即 Kitty 协议的目标列表，这种请求无需询问就会得到回答）、它想要的选择（`Primary`）以及剪贴板中的文本内容（`Text`），用于向用户提问。`Allow` 和 `Deny` 用来回答请求，可以在钩子中调用，也可以稍后从任何 goroutine 调用，先调用的那个生效；钩子没有回答的请求会继续等待。没有钩子时，终端会在视图中绘制自己的提示框，一次处理一个读取，带一行程序将读取内容的预览，以及 Allow 和 Deny 按钮（Enter 允许，Escape 拒绝）：

  ```go
  OnClipboardRequest: func(req terminal.ClipboardRequest) {
  	if req.Text == "" && !req.List {
  		req.Deny() // nothing to leak
  		return
  	}
  	askTheUser(func(ok bool) {
  		if ok {
  			req.Allow()
  		} else {
  			req.Deny()
  		}
  	})
  },
  ```
- `OnCopy` 在终端把文本写入剪贴板之后于主线程上运行，带上复制的原因：`CopyUser`（复制键、上下文菜单、执行复制的次要点击）、`CopySelect`（`CopyOnSelect`）或 `CopyProgram`（OSC 52）。应用可以告知这一点，如同 Ghostty 的 `app-notifications = clipboard-copy` 用提示条所做：

  ```go
  OnCopy: func(text string, by terminal.CopySource) {
  	if by != terminal.CopyProgram {
  		showToast("Copied to clipboard")
  	}
  },
  ```
- `OnClear` 在终端执行用户的清除操作前于主线程上调用：绑定到 `ActionClearScreen` 的按键和上下文菜单中的清除操作都会传入 `ClearScreen`。返回值表示是否接管操作；若接管，本地终端便不执行清除。显示服务器终端的窗口可接管操作，交由服务器端清除（见[在别处显示服务器的终端](#在别处显示服务器的终端)）。在全屏程序的屏幕上不会调用此回调。
- `OnTitle`、`OnExit`、`OnBell` 和 `OnNotify` 在程序设置标题、结束、响铃或请求桌面通知时，于终端的 goroutine 上运行。它们可能在显示终端的窗口存在之前运行，而窗口由 `NewWindow` 在 `New` 之后创建：使用窗口的回调会等待它，如同终端示例所做（在 `win` 之前 `<-ready`，`NewWindow` 设置 `win` 后 `close(ready)`）。`Done` 在程序退出后关闭，`ExitCode` 返回其代码（无法启动的程序为 -1）；终端随后显示“[Process exited]”（使用 `Conn` 时为“[Disconnected]”）。
- `NoExitMessage` 不写这一行，适用于自己提示程序已结束或直接关闭视图的应用：`OnExit`、`Done` 和 `ExitCode` 不受影响。
- `OpenLink` 在主线程上接收 Command+click 打开的链接；有了它，也能接收作为文本打印的文件路径（`/abs/path`、`~/notes.md`、`./a.go`、`src/app.go:12:3`、`main.go:250`、`app.ts(12,3)`、从盘符开始的 `D:\dev\main.go` 与 `C:/Users/me/a.png`、`app.go#L12C3`、带空格且用引号括起的路径、中文行文中两侧没有空格的路径），无论文件中的位置原本怎样打印，都会以 `path:line:col` 给出，并去掉引号；相对路径相对于程序的 `Dir`。它报告是否打开了链接；它留下的超链接或 URL 会用系统打开，如同没有 `OpenLink` 时每个链接都会那样。应用可以用这种方式在自己的预览中打开远程 shell 的路径，而不是让系统在本地打开它们。
- `OnPaste` 在按键、粘贴菜单项或次要点击触发剪贴板粘贴时于主线程上调用，参数为包含剪贴板所有格式的 `transfer.Data`。返回 true 表示接管操作，终端不再自行粘贴。驱动远程 shell 的应用可借此上传剪贴板中的文件（`d.Files()`）或图像（`d.Read(transfer.PNG)`），改用 `Paste` 粘贴远程路径：

  ```go
  OnPaste: func(d transfer.Data) bool {
  	files, _ := d.Files()
  	if len(files) == 0 {
  		return false // text pastes as usual
  	}
  	go func() { term.Paste(strings.Join(upload(files), " ")) }()
  	return true
  },
  ```

- `OnPasteStatus` 在终端的 goroutine 上，用 `PasteProgress` 告知向自行读取剪贴板的程序粘贴的进展：先是 `PasteOffered`，然后程序读取时是带 `Sent`/`Total` 字节的 `PasteSending`（写入它的字节，按协议发送的方式编码），然后是 `PasteSent`；如果提供的内容写入 5 秒后程序仍未读取，则为 `PasteNotTaken`（它仍可能读取，直到下一次粘贴到来）；如果没有文本的粘贴（如只有图像）发往既不读取剪贴板也不读取括号粘贴的程序，则为 `PasteNoText`，这种情况下什么也不粘贴。应用可以用这种方式在慢速链路上显示大图像的进度，并说明为什么什么也没粘贴。
- `OnKey` 在处理按键绑定及向程序发送按键之前于主线程上调用。返回 true 表示接管操作，按键及其输入的文本都不会发送给程序。与视图周围的 `Shortcut`（下一帧才运行）不同，该回调及其中通过 `Send` 发送的内容会保持与后续按键的先后顺序。只输入文本的按键可能仅以文本形式传入（例如在 Linux 上），不会触发 `OnKey`。
- `OnScroll` 在主线程上、终端滚动回滚缓冲区之前，看到滚轮以整行滚动视图（向上为负），并通过返回 true 接管它：回滚缓冲区保持不动。程序接收滚轮时不会询问它：接收鼠标时滚轮作为鼠标报告（除非按住 Shift），或在全屏程序的屏幕上作为方向键（备用滚动，`\x1b[?1007l` 会关闭它）。历史记录在另一端的 `Conn`（如只流式传输屏幕的远程多路复用器）可以用这种方式把滚轮发送到那里。鼠标滚轮的一格是三行，或在 Windows 上按“每次滚动行数”设置要求的行数（除非更改，否则为 3），与 Windows Terminal 相同；触控板按其移动的距离滚动。
- `OnZoom` 在主线程上看到缩放：滚轮加 Control（macOS 上为 Command），它会在 `OnScroll` 和接收鼠标的程序看到之前先缩放；或绑定到字体大小动作的按键。其 `Zoom` 包含 `Steps`（向上滚动为正；滚轮事件是一格，触控板的移动会累加成格），或为 `Reset`。它通过返回 true 接管缩放，如同一次性设置所有文本大小的应用那样；没有它，或对于它留下的缩放，终端会自己更改字体大小，每步一个 DIP，最小到 1，重置时回到 `Font.Size`。macOS 的捏合手势（原生 UI 不报告）不会缩放。

## 按键

终端把按键绑定到动作，如同 Ghostty 的 `keybind` 选项，使用该平台的 Ghostty 默认值，由 `terminal.DefaultKeybinds` 返回：

| 动作 | macOS | Linux 和 Windows | Windows 另有 |
| --- | --- | --- | --- |
| `ActionCopy` | Cmd+C | Ctrl+Shift+C、Ctrl+Insert | Ctrl+C（仅操作有效时拦截） |
| `ActionPaste` | Cmd+V | Ctrl+Shift+V、Shift+Insert | Ctrl+V（仅操作有效时拦截） |
| `ActionSelectAll` | Cmd+A | Ctrl+Shift+A | |
| `ActionClearScreen` | Cmd+K | | Ctrl+Shift+K |
| `ActionIncreaseFontSize` | Cmd+=、Cmd++ | Ctrl+=、Ctrl++ | |
| `ActionDecreaseFontSize` | Cmd+- | Ctrl+- | |
| `ActionResetFontSize` | Cmd+0 | Ctrl+0 | |
| `ActionScrollPageUp`, `…Down` | Shift+ 或 Cmd+Page Up/Down | Shift+Page Up/Down | Ctrl+Shift+Page Up/Down |
| `ActionScrollToTop`, `…Bottom` | Cmd+Home/End | Shift+Home/End（Linux） | Ctrl+Shift+Home/End |

清除屏幕和滚动的按键也仅在操作有效时拦截，以便全屏程序（如编辑器）接收这些按键。`Options.Keybinds` 在默认绑定上添加或覆盖：同一按键和修饰键的新绑定会替换默认绑定，绑定到 `ActionNone` 则解除绑定，让程序接收该按键；`NoDefaultKeybinds` 禁用所有默认绑定，对应 Ghostty 的 `keybind = clear`：

```go
term, err := terminal.New(terminal.Options{Keybinds: []terminal.Keybind{
	// Windows Terminal's Control+C and Control+V on Linux too.
	{Mods: ui.Ctrl, Key: ui.KeyC, Action: terminal.ActionCopy, Performable: true},
	{Mods: ui.Ctrl, Key: ui.KeyV, Action: terminal.ActionPaste, Performable: true},
	// Control+Shift+A reaches the program.
	{Mods: ui.Ctrl | ui.Shift, Key: ui.KeyA, Action: terminal.ActionNone},
}})
```

`Performable` 绑定对应 Ghostty 的 `performable:` 前缀，仅在操作有效时拦截按键：有选区才复制，剪贴板有内容才粘贴，不在全屏程序的屏幕上才清除屏幕和滚动；否则按键会像未绑定一样传给程序，例如没有选区时 Control+C 会中断程序。这种复制绑定还会取消选择，使下次按键可以中断程序。`ActionClearScreen` 清除回滚缓冲区及光标所在行上方的屏幕，将光标所在行连同 shell 提示符移到顶部，对应 Kitty 的 `clear_terminal to_cursor`；上下文菜单的清除操作及代码中的 `Terminal.ClearScreen` 效果相同，`Terminal.ClearScrollback` 则只清除回滚缓冲区。`ActionIgnore` 拦截按键，不执行任何操作。

按键依次由应用快捷键、`OnKey`、按键绑定和程序处理。上下文菜单会显示各菜单项绑定的按键。

## 字体

`terminal.Font` 保存 Ghostty 的字体选项所设置的内容：

```go
term, err := terminal.New(terminal.Options{Font: terminal.Font{
	Family:     "JetBrains Mono, Menlo, monospace", // tried in order
	Size:       14,                                  // DIPs, 13 when zero
	Weight:     500,                                 // 400 when zero
	Features:   []string{"-calt", "+ss01"},          // as Ghostty's font-feature
	CellWidth:  1.03,                                // wider cells, unchanged glyph size
	LineHeight: 1.2,                                 // as adjust-cell-height = 20%
	Thicken:    true,                                // as font-thicken, on macOS
}})
```

- `Family` 是字体族列表，格式与 `ui.Element.Font` 接受的一样：为空时是系统等宽字体。字体缺少的字符来自系统的回退字体。应用自带而非用户安装的字体用 `ui.RegisterFont` 添加；各样式放在同一个字体族下，让粗体和斜体文本能使用它们：

  ```go
  //go:embed fonts
  var fonts embed.FS

  for _, name := range []string{"Regular", "Bold", "Italic", "BoldItalic"} {
  	data, _ := fonts.ReadFile("fonts/JetBrainsMono-" + name + ".ttf")
  	ui.RegisterFont(data, "JetBrains Mono")
  }
  ```

- `Weight` 设置文本的粗细，范围为 100 到 900，并限定在字体族拥有的字重中；粗体文本比它重 300，至少为 700。
- `Features` 打开和关闭 OpenType 特性：`"ss01"` 或 `"+ss01"` 打开，`"calt=0"` 或 `"-calt"` 关闭，`"cv05=2"` 是替代字形。编码字体的大部分连字由 `calt` 生成，其余的由 `liga` 生成；连字会跨越其字符的单元格。在 macOS 上，除非列表指定 `kern`，否则字距调整关闭，使每个字符保持在其单元格中的位置；`"+kern"` 保留字体的字距。
- `CellWidth` 乘以字体的水平步进，以确定列宽，但不缩放字形，也不改变行高。零或更小表示 `1`；`1.03` 请求宽 3% 的单元格，低于 `1` 时字形会重叠。单元格宽度会舍入到整数个显示像素，所以很小的增量可能共用同一宽度，也可能跳过一个像素。宽字符、光标、选择、鼠标报告和框线字符都遵循调整后的网格；对单元格来说过宽的字形（如 emoji）会缩小到字体自己的单元格宽度。
- `LineHeight` 使行比字体的行高更高或更矮，文本在其中居中；框线字符仍会相接。
- `Thicken` 用更粗的笔画绘制文本，无论颜色和系统设置如何，都使用 Core Text 最强的字体平滑，如同 Ghostty。没有它时，文本如 AppKit 绘制的那样：越浅的文本平滑得越多，只要用户保持平滑开启。它在 Linux 和 Windows 上什么也不改变，如同在 Ghostty 中。

`SetFont` 更改运行中终端的字体；网格随后采用其单元格的新大小。

## 主题

插件包含 Ghostty 附带的数百个主题（来自 [iTerm2-Color-Schemes](https://github.com/mbadolato/iTerm2-Color-Schemes)），`terminal.GhosttyTheme` 按名称返回主题，用法与 Ghostty 的 `theme` 选项相同：

```go
mocha, err := terminal.GhosttyTheme("Catppuccin Mocha")
latte, err := terminal.GhosttyTheme("Catppuccin Latte")
term, err := terminal.New(terminal.Options{
	Theme:     latte, // in light windows
	DarkTheme: mocha, // in dark windows, as theme = light:…,dark:… in Ghostty
})
```

与 Ghostty 一样，用户在 Ghostty 主题目录（`$XDG_CONFIG_HOME/ghostty/themes`，默认 `~/.config/ghostty/themes`）中的主题文件优先，绝对路径则指定一个主题文件。`terminal.GhosttyThemes()` 列出名称（用户的主题与 Ghostty 的），供选择器使用，`terminal.ParseGhosttyTheme` 从字节中读取 Ghostty 格式的主题：

```
palette = 0=#45475a
palette = 1=#f38ba8
background = #1e1e2e
foreground = #cdd6f4
cursor-color = #f5e0dc
cursor-text = #1e1e2e
selection-background = #f5e0dc
selection-foreground = #1e1e2e
```

`SetTheme(theme, dark)` 更改运行中终端的主题。

## 搜索

`Search` 在屏幕和回滚缓冲区中搜索文本，如同 Ghostty 的查找，使用 libghostty-vt 的搜索：匹配项在找到时高亮，每次处理回滚缓冲区中一帧的量，并跟随后续输出；最新的是选中项，并会显示出来。除 ASCII 字母的大小写外，匹配是精确的。`SearchNext` 向更旧的输出选择下一个匹配项，一直到回滚缓冲区中，`SearchPrevious` 则向更新的输出，两者都会回绕，并且除非匹配项已显示，否则会把它滚动到视图中央，避开边缘上的查找栏；`EndSearch`（或 `Search("")`）结束搜索。终端不绘制查找栏：应用绘制一个，并把 Ghostty 的按键绑定到这些方法，macOS 上是 Cmd+F、Cmd+G、Cmd+Shift+G 和 Escape。`OnSearch` 在主线程上告知搜索的进展，用于显示“1 of 3”：

```go
term, err := terminal.New(terminal.Options{
	OnSearch: func(s terminal.SearchStatus) {
		// s.Selected of s.Total matches of s.Text; s.Searching while more
		// of the scrollback is to go through.
		bar.status = fmt.Sprintf("%d of %d", s.Selected, s.Total)
	},
})
```

主题的 `Search` 和 `SearchText` 为匹配项及其文本着色，`SearchSelected` 和 `SearchSelectedText` 为选中的那个着色，如同 Ghostty 的 `search-background`、`search-foreground`、`search-selected-background` 和 `search-selected-foreground`，主题文件也会设置它们：为零时是 Ghostty 的浅琥珀色和浅橙色配黑色文本。

## 方法

终端的方法可以从任何 goroutine 安全调用：

- `Send` 把字节发送给程序，如同输入的一样，`Paste` 粘贴文本。`PasteData` 粘贴 `transfer.Data`，如同粘贴键粘贴剪贴板：自行读取剪贴板的程序会按 MIME 类型收到每种格式，并在请求时读取它想要的（`transfer.Lazy` 表示只在此时才在终端的 goroutine 上生成）；任何其他程序得到文本。`Paste` 是带文本的 `PasteData`，这样的程序会把它读作 `text/plain`。
- `Feed` 显示字节，如同程序写入了它们，包括文本和转义序列。
- `Title`、`Dir`（shell 报告的目录）、`Size`（以单元格计）和 `Text`（屏幕及其回滚缓冲区）读取终端。
- `HoveredLink` 返回视图中按住 Command（其他地方为 Control）悬停的链接，如同 `OpenLink` 接收的那样，它是否为文件路径，以及其单元格相对于视图的矩形：在 `View` 之后读取它，以在视图上方显示链接的预览。
- `SetFont` 和 `SetTheme` 更改字体和颜色；`SetFont` 还会设置重置缩放时恢复的大小，并清除当前缩放。
- `Resize` 设置没有视图显示的终端的大小，`Snapshot` 以转义序列返回终端的显示内容：回滚缓冲区、屏幕及其样式、光标，以及程序设置的模式。将这些序列输入相同大小的新终端，即可恢复相同画面，例如服务器为连接到会话的窗口保留屏幕状态。
- `Close` 挂断程序，它会收到 SIGHUP，并释放终端。

## 没有程序

`Conn` 把终端连接到任何可读写的东西（如 SSH 会话或串口），而不是程序：终端从它读取的内容会显示，输入的内容会写入它。带 `Resize(cols, rows int) error` 方法的 `Conn` 会被告知终端的大小：

```go
term, err := terminal.New(terminal.Options{Conn: session})
```

其 `Conn` 从不发送任何内容的终端会显示应用用 `Feed` 写入的内容，如同带颜色的日志。

## 在别处显示服务器的终端

让会话保持运行的服务器（如多路复用器）会保留每个会话的终端，程序的输出会写入其中，而窗口从另一个进程显示它。服务器终端的 `Mirror` 把它的显示内容分成帧：`Frame` 返回 VT 序列，把显示此前帧的终端更新为服务器当前显示的内容。窗口的终端从它的 `Conn` 读取这些序列，如同程序写入的一样，并逐单元格显示服务器的屏幕和回滚缓冲区，包括光标、改变终端所发送内容的模式（光标键、键盘协议、鼠标、括号粘贴）、程序设置的颜色（OSC 4、10、11、12）以及 shell 提示符的行（OSC 133）；同时它仍像任何终端一样选择、复制、打开链接、组合文本，并在 Option+click 时移动 shell 的光标。窗口端不再自行排版程序输出，因此画面不会与服务器端产生偏差。如果先恢复服务器屏幕快照，再继续输入程序输出，画面就可能出现偏差，尤其是通过相对移动光标来重绘的程序，例如智能体的界面。

```go
// The server, one Mirror for each window, its first frame starting it over:
m, err := term.NewMirror()
frame, due := m.Frame() // after output; due: a synchronized update holds it
```

帧就地绘制各行，滚出屏幕的行按顺序穿过窗口的屏幕，使其回滚缓冲区成为服务器的回滚缓冲区；软换行保持为软换行。每个单元格以自己的样式绘制，所以空单元格在有色单元格之后仍为空。帧像视图一样读取终端，取用它对变化内容的记录：要镜像没有视图显示的终端（如服务器的终端）时，它有多少窗口就用多少个 Mirror。程序持有同步更新（模式 2026）时，最多一秒不会产生帧：`Frame` 随后会报告有一帧待生成。程序不改变光标形状时，窗口的光标采用自己的 `Cursor` 和 `NoBlink`；程序设置时则采用程序的。

窗口端的终端设置 `ResizeByConn` 后，视图只会通过 `Conn` 请求调整为自身可容纳的尺寸，屏幕仍保持原尺寸；窗口收到服务器流中的尺寸变更通知后，再调用 `Resize`，且必须在处理新尺寸的帧之前调用。

```go
term, err := terminal.New(terminal.Options{Conn: stream, ResizeByConn: true})
// Where the server's stream says it resized to cols×rows:
term.Resize(cols, rows)
```

服务器的终端回答程序的问题，所以它需要一些只有窗口才知道的信息。带 `ResizePixels(cols, rows, cellWidth, cellHeight int) error` 的 `Conn` 会被告知窗口单元格的像素大小，带 `UseTheme(theme *terminal.Theme) error` 的会被告知窗口的颜色；服务器用 `ResizePixels` 和 `UseTheme` 把它们交给自己的终端。以像素询问大小的程序，或以像素接收鼠标的程序（模式 1016），随后会按窗口的单元格计算；询问颜色（OSC 4、10、11、12）或是否深色的程序会得到窗口的。已询问主题变化的程序（模式 2031）在它们从浅色变为深色或变回时被告知，如同视图显示的终端在窗口外观改变时告知它。

```go
// The server, where a window's stream says so:
term.ResizePixels(cols, rows, cellWidth, cellHeight)
term.UseTheme(theme)
```

用户的清除应由服务器来做：如果只在窗口中清除，服务器后续的帧仍从满屏的状态绘制，会把行带回到错误的位置。窗口的 `OnClear` 接管它们并询问服务器，由服务器的终端清除；下一帧会显示它。

```go
// The window:
OnClear: func(c terminal.Clear) bool {
	stream.Clear(c) // however the app asks its server
	return true
},
// The server, asked:
term.ClearScreen()
```

程序对剪贴板的写入（OSC 52）随帧一起传递，由窗口的终端按自己的策略写入。服务器的终端设置 `ClipboardRead: terminal.ClipboardForward` 时，程序的读取也会传递，由窗口的终端回答，并像它本来那样询问用户：服务器的终端什么也不回答。程序的其他查询由服务器自己回答，以它们所写入的终端的身份。

`History` 以行的形式返回服务器的回滚缓冲区及屏幕上直到光标处的内容，不含光标和模式，供程序结束后接管会话的终端使用，例如服务器重启之后：将这些内容输入终端，光标会停在显示内容下方，让下一个程序从下方开始，而不覆盖已有内容。全屏程序的屏幕不返回这些历史内容。

## 库

libghostty-vt 是 Ghostty 的原生库，针对每个平台从插件绑定的 Ghostty 版本构建，并随 MyGo 发布。插件的包在 `mygo-plugin.json` 中列出这些构建及其 SHA-256，CLI 会把各平台对应的库放进它构建的应用的资源中，名为 `libghostty-vt.dylib`、`libghostty-vt.so` 或 `ghostty-vt.dll`：macOS 应用会用应用的身份给它签名，通用应用会在一个文件中得到两种架构。

不由 CLI 构建的程序（如在 `go run` 和 `go test` 下）会把库下载一次到用户的缓存（`<cache>/mygo/natives/`），CLI 也把它下载的库保存在那里，并检查其 SHA-256。打包的应用从不下载它。`terminal.LibraryPath` 返回程序加载的库，`$MYGO_GHOSTTY_VT` 指定另一个库。

<!-- repository-only:start -->

要从 Ghostty 的源码构建 libghostty-vt：

```sh
git clone https://github.com/ghostty-org/ghostty && cd ghostty
zig build -Demit-lib-vt -Doptimize=ReleaseFast
MYGO_GHOSTTY_VT=$PWD/zig-out/lib/libghostty-vt.dylib go run ./examples/terminal
```

<!-- repository-only:end -->

`terminal.Load` 加载库，`New` 也会这样做：先调用它以在显示窗口之前报告缺少的库。

该库在 macOS 13 或更高版本上运行，如同 Ghostty，所以带终端的应用把 `minimumSystemVersion` 设为 13.0 或更高；在 Linux 上需要 glibc 2.28 或更高版本（Debian 10、Ubuntu 20.04），支持 x64 和 arm64；在 Windows 10 1809 或更高版本上，其 ConPTY 运行程序，支持 x64 和 arm64。

## 测试

`ui.NewTester` 在没有窗口的情况下运行终端视图：用 `Type` 和 `Key` 向它输入，并用 `Text` 读取程序打印的内容：

```go
func TestShell(t *testing.T) {
	term, err := terminal.New(terminal.Options{Command: []string{"/bin/sh"}})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { terminal.View(c, term).Fill().AutoFocus() }, 640, 400)
	tt.Type("echo hello")
	tt.Key(0, ui.KeyEnter)
	for !strings.Contains(term.Text(), "\nhello") {
		time.Sleep(10 * time.Millisecond)
		tt.Frame()
	}
}
```
