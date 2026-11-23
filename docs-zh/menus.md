# 菜单与托盘

菜单由 `*mygo.MenuItem` 的模板构建，用作菜单栏、上下文菜单、Dock 菜单和托盘图标的菜单。

## 模板

```go
menu := mygo.NewMenu([]*mygo.MenuItem{
	{Role: mygo.RoleAppMenu},
	{Label: "File", Submenu: []*mygo.MenuItem{
		{Label: "New Note", Accelerator: "CmdOrCtrl+N", Click: func(item *mygo.MenuItem, win *mygo.Window) {
			newNote()
		}},
		{Label: "Export…", Accelerator: "CmdOrCtrl+E", Click: exportNotes},
		mygo.Separator(),
		{Role: mygo.RoleClose},
	}},
	{Role: mygo.RoleEditMenu},
	{Label: "View", Submenu: []*mygo.MenuItem{
		{ID: "sidebar", Label: "Show Sidebar", Type: mygo.MenuItemCheckbox, Checked: true, Click: toggleSidebar},
		mygo.Separator(),
		{Label: "List", Type: mygo.MenuItemRadio, Checked: true},
		{Label: "Grid", Type: mygo.MenuItemRadio},
		mygo.Separator(),
		{Role: mygo.RoleToggleDevTools},
		{Role: mygo.RoleToggleFullScreen},
	}},
	{Role: mygo.RoleWindowMenu},
})
```

`MenuItem` 包含以下字段：

- `Label`，以及 `Accelerator`，即它的快捷键（见[快捷键](#快捷键)）；
- `Click`，选中菜单项时在主线程上调用，传入该菜单项和窗口：Linux 和 Windows 的菜单栏或上下文菜单传入所属窗口，其他菜单传入当前焦点窗口，没有则为 nil；
- `Type`：`MenuItemNormal`、`MenuItemCheckbox`、`MenuItemRadio`、`MenuItemSeparator` 或 `MenuItemSubmenu`；带 `Submenu` 的菜单项是子菜单，其他项除非另行设置，都是普通项。相邻的单选按钮项组成一组，选中其中一个会取消选中其他的。复选框和单选按钮项在 `Click` 运行前切换 `Checked`；
- `Disabled`、`Hidden` 和 `ToolTip`；
- `Icon`，显示在标签旁边的图像（见[图标](#图标)）；
- `Subtitle`，标签下方第二行较小的文字。macOS 14 和浏览器这样显示它；更早的 macOS、Windows 和 Linux 把它显示在标签之后，即 `Label — Subtitle`；
- `ID`，用于通过 `Menu.ItemByID` 找到该菜单项；
- `Role`，一种内置行为，带有它的标签和快捷键。

### 角色

角色行为符合用户对标准菜单项的预期，带有通常的标签和快捷键，`Label` 和 `Accelerator` 可以覆盖它们：

- 编辑：`RoleUndo`、`RoleRedo`、`RoleCut`、`RoleCopy`、`RolePaste`、`RoleDelete`、`RoleSelectAll`，以及在 macOS 上的 `RolePasteAndMatchStyle`。它们的按键会先送到内容，如同在浏览器里一样：在 Windows 的页面中，WebView2 用页面自己的事件和剪贴板上的每种格式剪切、复制和粘贴。从菜单选择“粘贴”会把焦点还给页面，并通过 DevTools 运行 Chromium 的粘贴命令，使用相同的粘贴事件和格式。页面还会看到一个没有修饰键的 `Unidentified` 键的 `keydown` 和 `keyup`，因此它的 Ctrl+V 处理器不会被调用。“粘贴并匹配样式”仍然插入纯文本。给编辑角色指定一个页面用不到的按键时，例如在 Ctrl+Shift+C 上设“复制”，它仍会执行自己的命令；
- 格式：`RoleBold`、`RoleItalic`、`RoleUnderline`、`RoleStrikethrough`、`RoleAlignLeft`、`RoleAlignCenter` 和 `RoleAlignRight`，它们格式化原生 UI 的[富文本编辑器](ui/rich-text-editor.md)的选区，或可编辑页面（`document.execCommand`）的选区；“粘贴并匹配样式”把纯文本粘贴到富文本编辑器中；
- 页面：`RoleReload`、`RoleForceReload`、`RoleToggleDevTools`（在原生 UI 窗口中，是其[检查器](ui/inspector.md)）、`RoleResetZoom`、`RoleZoomIn`、`RoleZoomOut`；
- 窗口：`RoleToggleFullScreen`、`RoleMinimize`、`RoleZoom`、`RoleClose`；
- [文稿](documents.md#文件菜单)：`RoleNew`、`RoleOpen`、`RoleOpenRecent`、`RoleSave`、`RoleSaveAs`、`RoleDuplicate`、`RoleRename`、`RoleMoveTo`、`RoleRevert`；
- 应用：`RoleQuit`，以及在 macOS 上的 `RoleAbout`、`RoleHide`、`RoleHideOthers`、`RoleUnhide`、`RoleFront`、`RoleServices`、`RoleStartSpeaking`、`RoleStopSpeaking`；
- 整个菜单：`RoleFileMenu`、`RoleEditMenu`、`RoleFormatMenu`（粗体、斜体、下划线、删除线和各种对齐）、`RoleViewMenu` 和 `RoleWindowMenu`，以及在 macOS 上的 `RoleAppMenu`（以应用命名的菜单）、`RoleWindow`（列出窗口的菜单）和 `RoleHelp`（带搜索框的菜单）。

带 macOS 专用角色的菜单项在其他平台上会被略去，因此同一份模板可用于所有平台。

编辑类角色的菜单项，无论位于菜单栏还是弹出菜单，都与其他应用一样，只在当前焦点支持相应操作时启用：在[原生 UI](ui/README.md) 中，选中文本时可剪切、复制，有可撤销的内容时可撤销，文本输入中可粘贴；接受输入的元素（`HandleInput`）则启用全部编辑操作。在 macOS 上，如果焦点元素或窗口的某个[快捷键](ui/input.md)使用了该角色的按键，对应菜单项也会启用。macOS 会根据焦点验证菜单项：页面的 webview 按 WebKit 的行为启用编辑项和朗读项，不支持朗读的原生 UI 则禁用朗读项。在 Linux 和 Windows 上，页面的菜单项保持应用设置的状态。应用明确禁用的菜单项始终保持禁用。

### 修改菜单项

用菜单项自身的方法修改正在使用的菜单项，让原生菜单随之更新：`SetLabel`、`SetEnabled`、`SetVisible`、`SetChecked`、`SetAccelerator`、`SetSubtitle` 和 `SetIcon`：

```go
item := menu.ItemByID("sidebar")
item.SetChecked(false)
```

`Menu.Append` 和 `Menu.Insert` 添加菜单项。

### 图标

菜单项的 `Icon` 是显示在标签旁边的图像，例如 VPN 托盘菜单中每台服务器的国旗。`NewMenuIcon` 用 PNG 图像按它自己的颜色制作图标，对不是 PNG 图像的数据返回错误；每个图像只制作一次，并交给所有显示它的菜单项：

```go
//go:embed flags/jp.png
var japanPNG []byte

japan, err := mygo.NewMenuIcon(japanPNG, false)
if err != nil {
	log.Fatal(err)
}
servers := mygo.NewMenu([]*mygo.MenuItem{
	{ID: "tokyo", Label: "Tokyo", Type: mygo.MenuItemRadio, Checked: true, Icon: japan},
	{ID: "osaka", Label: "Osaka", Type: mygo.MenuItemRadio, Icon: japan},
	{ID: "direct", Label: "Direct", Type: mygo.MenuItemRadio},
})
```

图像会按比例放入 16×16 点的方框：至少提供 32×32 像素，或 48×48，以便在高分辨率显示器以及 150% 和 200% 缩放下保持清晰。复选框或单选按钮项还会显示它的勾选标记。`SetIcon` 替换正在使用的菜单中菜单项的图标，nil 则移除它：

```go
servers.ItemByID("tokyo").SetIcon(warning)
servers.ItemByID("osaka").SetIcon(nil)
```

`NewMenuIcon` 的第二个参数 `template` 让黑白透明图像采用菜单文字的颜色，如同 macOS 给它的模板图像着色：在深色菜单中和高亮时为白色。Linux 按原样显示模板图标。

`NewMenuIconSVG` 改用 SVG 制作图标，即 `ui.ParseSVG` 解析出的 `*ui.SVG`，这样应用也能在菜单中显示它用 `ui.Icon` 绘制的图标。它用原生 UI 的渲染器（`ui.SVG` 的 `Image`）把 SVG 绘制一次，得到长边 64 像素的图像，在最高 400% 缩放下保持清晰，并对 nil SVG 或没有尺寸的 SVG 返回错误。作为模板时，只看它的形状，无论颜色如何，与 `ui.Icon` 一样：

```go
//go:embed icons/trash.svg
var trashSVG []byte

var trash = ui.MustParseSVG(trashSVG) // ui.Icon(c, trash) in the window

trashIcon, err := mygo.NewMenuIconSVG(trash, true)
```

在原生 UI 绘制的菜单中，菜单项的 [`IconColor`](ui/context-menu.md#菜单项) 给它的模板图标一个独立的颜色。

MyGo 对每个图像只解码和缩放一次，无论多少菜单项显示同一个图像，也无论它们的菜单打开或重建多少次，由同一图像制作的图标也是如此；一旦应用和菜单都不再持有该图标，就释放由它生成的内容；Windows 10 菜单的位图随其菜单一起释放。不制作图标的应用不会链接绘制它们的代码。

原生 UI 的上下文菜单使用相同的图标，在其菜单项上设置 [`Icon`](ui/context-menu.md#菜单项)。

- **macOS** 在每个菜单的勾选标记列之后显示图标（`NSImage`），包括菜单栏自己的菜单项，它们在菜单栏中显示图标。
- **Windows 11** 的菜单由 MyGo 绘制，像系统的菜单那样，在勾选标记和标签之间为图标留出单独的一列，按菜单的显示缩放绘制。**Windows 10** 把图标作为菜单项的位图（32 位，带 alpha）绘制在它的勾选标记列中：已勾选的菜单项显示带勾选边框的图标，代替勾选标记。菜单栏本身的菜单项不显示图标。
- **Linux** 在菜单栏的菜单和上下文菜单（GTK）的菜单项中显示图标；菜单栏本身的菜单项不显示。托盘图标的菜单由面板绘制，面板通过 DBusMenu（`icon-data`）获取图标：GNOME Shell 的 AppIndicator 扩展和 KDE Plasma 会显示它们，其他面板可能不会。

## 菜单栏

```go
mygo.App.SetMenu(menu)
```

在 macOS 上，菜单栏属于应用。应用会得到一个默认菜单栏，包含应用、文件、编辑、视图和窗口菜单，直到应用设置自己的菜单栏：在你的菜单中保留 `RoleAppMenu` 和 `RoleEditMenu`，有了它们，Cmd+Q 以及复制和粘贴才能工作。

在 Linux 和 Windows 上，菜单栏属于窗口。`App.SetMenu` 会把菜单给每个没有自己菜单的窗口，`Window.SetMenu` 则给窗口设置自己的菜单；窗口默认没有菜单，但带[文稿](documents.md)的应用除外，它们会得到文件和编辑菜单。`App.SetMenu(nil)` 移除菜单。

在 Windows 上，菜单栏和菜单会像标题栏一样跟随外观（浅色或深色），见[深色模式](frontend.md#深色模式)。在 Windows 11 上，它们看起来像系统自带的菜单，例如文件资源管理器的：更宽敞的菜单项、带圆角的高亮、更小且颜色更淡的快捷键、系统浮出控件更圆润的圆角，以及从 Windows 11 22H2 起，弹出菜单后面的 Acrylic，如同 WinUI 的菜单，除非透明效果已关闭、节能模式或对比度主题已开启，或当前是远程会话，此时它们会使用 WinUI 菜单回退到的纯色背景，并带一条淡淡的轮廓线。

带 `AutoHideMenuBar` 的窗口会把菜单栏藏起来，把空间留给页面。单独按 Alt 或按 F10 会显示菜单栏：在 Windows 上焦点落在菜单栏中，在 Linux 上则打开它的第一个菜单；用户选中一项或离开后，菜单栏会再次隐藏。它的快捷键始终有效：

```go
mygo.NewWindow(mygo.WindowOptions{URL: "/", AutoHideMenuBar: true})
```

没有标题栏的 Windows 窗口，无论是无边框还是隐藏了标题栏，也没有菜单栏的位置：Alt 和 F10 仍会从左上角、页面绘制的标题栏下方以弹出菜单打开它的菜单。在 Linux 上，隐藏标题栏的窗口会像 `AutoHideMenuBar` 那样隐藏菜单栏，以便窗口按钮留在窗口顶部、页面的标题栏中；无边框窗口则在页面上方显示菜单栏。

全屏窗口把整个屏幕留给页面：它的菜单栏会隐藏，直到它退出全屏。Alt 和 F10 仍会打开它的菜单：在 Windows 上从左上角以弹出菜单打开，在 Linux 上则在菜单栏中打开，菜单打开期间菜单栏会显示。

## 上下文菜单

`Menu.Popup` 在窗口中鼠标所在的位置显示菜单，菜单关闭后返回；`PopupAt` 在页面中的某个位置显示它。从绑定方法中显示菜单，由页面在右键单击时调用：

```go
// ShowNoteMenu shows the context menu of a note.
func (n *Notes) ShowNoteMenu(ctx context.Context, id int64) {
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "Rename", Click: func(*mygo.MenuItem, *mygo.Window) { n.startRename(id) }},
		{Label: "Delete", Click: func(*mygo.MenuItem, *mygo.Window) { n.Delete(id) }},
	})
	menu.Popup(mygo.CallerWindow(ctx))
}
```

```ts
note.addEventListener("contextmenu", (e) => {
  e.preventDefault();
  Notes.showNoteMenu(id);
});
```

没有自己的处理器时，页面会显示 webview 的标准上下文菜单，文本上有复制和粘贴，开发构建中还有检查元素。显示[原生 UI](ui/context-menu.md) 的窗口用 `ContextMenu` 为元素提供上下文菜单，像界面其余部分一样构建。

### webview 的菜单

`Page.OnContextMenu` 在标准菜单即将打开时修改它，如同 Electron 的 `context-menu` 事件。事件会告知右键单击了什么（`LinkURL`、`LinkText`、`ImageURL`、`MediaURL`、`SelectionText`、`Editable`、`FrameURL`），它的 `Menu` 持有 webview 自己的菜单项。隐藏、重命名、禁用或移除这些菜单项，也可以添加应用自己的菜单项，它们会像在其他菜单中一样工作：

```go
page.OnContextMenu(func(e *mygo.ContextMenuEvent) {
	e.Menu.Remove(mygo.ContextMenuReload, mygo.ContextMenuInspect)
	if e.LinkURL != "" {
		e.Menu.Append(mygo.Separator(), &mygo.MenuItem{
			Label: "Open in Browser",
			Click: func(*mygo.MenuItem, *mygo.Window) { go mygo.Shell.OpenExternal(e.LinkURL) },
		})
	}
})
```

`e.PreventDefault()` 不显示菜单，把 `e.Menu` 设为应用的菜单则会改为显示那个菜单。webview 的菜单项无论移到哪里，都继续做 webview 所做的事，除非应用给其中一项设置 `Click`，此时会改为运行它。页面自己的 `contextmenu` 处理器调用 `preventDefault` 时不打开任何菜单，监听器也不会被调用。

webview 的菜单项在每个支持它们的平台上都有相同的 ID：

| ID | 菜单项 |
|---|---|
| `ContextMenuBack`, `ContextMenuForward`, `ContextMenuReload`, `ContextMenuStop` | 页面的导航 |
| `ContextMenuInspect` | 检查元素，在带 DevTools 的构建中 |
| `ContextMenuUndo`, `ContextMenuRedo`, `ContextMenuCut`, `ContextMenuCopy`, `ContextMenuPaste`, `ContextMenuPasteAndMatchStyle`, `ContextMenuDelete`, `ContextMenuSelectAll` | 编辑 |
| `ContextMenuSpellingGuess` | 为拼错的词建议的每个词 |
| `ContextMenuOpenLink`, `ContextMenuOpenLinkInNewWindow`, `ContextMenuDownloadLink`, `ContextMenuCopyLink` | 链接 |
| `ContextMenuOpenImageInNewWindow`, `ContextMenuDownloadImage`, `ContextMenuCopyImage`, `ContextMenuCopyImageURL` | 图像 |
| `ContextMenuOpenMediaInNewWindow`, `ContextMenuDownloadMedia`, `ContextMenuCopyMediaURL`, `ContextMenuMediaControls`, `ContextMenuMediaLoop`, `ContextMenuMediaFullScreen`, `ContextMenuPictureInPicture` | 视频和音频 |
| `ContextMenuOpenFrameInNewWindow`, `ContextMenuLookUp`, `ContextMenuSearchWeb`, `ContextMenuShare`, `ContextMenuEmoji`, `ContextMenuPrint` | 其余 |

其他菜单项以平台自己的名称作为 ID：macOS 上 WebKit 的标识符（`WKMenuItemIdentifierTranslate`），Linux 上标准操作的昵称（`unicode`），Windows 上 WebView2 的名称（`createQrCode`）。分隔符以及平台没有命名的菜单项没有 ID。

各平台的做法：

- **macOS** 在 WebKit 的菜单打开时就地修改它，因此查询、翻译、服务和拼写建议继续工作。WebKit 不会告知单击了什么：MyGo 的运行时从页面自身文档中的 `contextmenu` 事件中获取这一信息，因此在 iframe 中单击不会提供任何信息，`FrameURL` 保持为空。
- **Linux** 就地修改 WebKitGTK 的菜单。WebKitGTK 会告知链接、图像、媒体以及元素是否可编辑，页面则会告知选区。应用的菜单项不显示快捷键或图标，单选按钮项显示为复选框，只有 WebKitGTK 标准操作的菜单项可以重命名。`FrameURL` 保持为空。
- **Windows** 从 WebView2（1.0.1185 及更高版本；更旧的运行时不调用监听器）获得一切。应用修改过的菜单由 MyGo 绘制，与其他弹出菜单一样，其中 WebView2 的菜单项仍做它们原本做的事；保持原样的菜单则是 WebView2 自己的。WebView2 不给拼写建议提供词，因此 MyGo 绘制的菜单会略去它们；它也不提供 `data:` 图像的 URL，这个 URL 改由页面的 `contextmenu` 事件提供。

应用修改过的菜单会隐藏开头、结尾和连续重复的分隔符，避免移除菜单项后留下多余分隔符。

## Dock 菜单

在 macOS 上，`App.Dock.SetMenu` 会把菜单项添加到右键单击 Dock 图标所打开的菜单中标准项的上方。与 `App.SetMenu` 一样，它可以在 `main` 中、`App.Run` 之前调用：

```go
mygo.App.Dock.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
	{Label: "New Window", Click: func(*mygo.MenuItem, *mygo.Window) { openMainWindow() }},
}))
```

## 托盘图标

托盘图标在 macOS 上位于菜单栏，在 Windows 和 Linux 上位于通知区域。在应用就绪后创建它：

```go
//go:embed resources/tray.png
var trayIcon []byte

mygo.App.WhenReady(func() {
	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon:           trayIcon,
		IconIsTemplate: true, // macOS tints it to match the menu bar
		ToolTip:        "Notes",
		Menu: mygo.NewMenu([]*mygo.MenuItem{
			{Label: "Open Notes", Click: func(*mygo.MenuItem, *mygo.Window) { openMainWindow() }},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}),
	})
	if err != nil {
		log.Println("tray:", err)
	}
	_ = tray
})
```

图标是 PNG，理想情况下为 16×16 点（高分辨率显示器用 32×32 的图像）；模板图标为透明背景上的黑色，让 macOS 为浅色和深色菜单栏给它着色。带菜单的托盘在单击时显示菜单。没有菜单时，单击会到达 `OnClick`，`OnRightClick` 接收右键单击，在 macOS 上还会接收 Control 单击，因为在那里按住 Control 单击相当于右键单击。用 `SetIcon`、`SetMenu`、`SetToolTip` 以及在 macOS 上的 `SetTitle`（图标旁边的文字）修改托盘；`Destroy` 移除它。

托盘菜单的菜单项可以显示[图标](#图标)，例如每台服务器旁边的国旗，并像任何菜单的菜单项那样变化：菜单下次打开时显示它们的状态。

显示不断变化的数字的标题，例如网速，应保持宽度，否则菜单项会伸缩，图标和它旁边的菜单项会移动。在 macOS 上，标题中的数字宽度都相同，因此把值填充到固定字符数就能保持宽度：用数字空格（U+2007）填充，它与数字一样宽，因为普通空格更窄。

```go
// "  9.0 MB/s", " 12.5 MB/s", "128.0 MB/s": all as wide.
func speed(mbps float64) string {
	return strings.ReplaceAll(fmt.Sprintf("%5.1f", mbps), " ", "\u2007") + " MB/s"
}

tray.SetTitle(speed(rate))
```

只有数字等宽，字母仍有各自的宽度，因此切换单位的标题（先 `9 KB/s`，后 `1.2 MB/s`）仍会改变宽度。应用可自行保持宽度稳定，例如像上面那样始终使用同一单位。

应用可以显示多个图标，例如菜单栏应用把它的各项指标分别放在自己的菜单项中。在 macOS 上，新图标出现在已有图标的左边，用户按住 ⌘ 拖动图标来移动它们：给每个图标一个在应用的图标中唯一的 `ID`，每当它再次被创建时（下次启动或 `Destroy` 之后），它都会回到用户放置的位置。在 Windows 上，用户选择哪些图标显示在任务栏上，而不是溢出区域中，带 `ID` 的图标会保持这一选择，无论应用以什么顺序创建它的图标。

模板图标会自动适应菜单栏颜色。彩色图标（例如用不同颜色绘制各项指标的图标）则需要根据所在栏的外观调整颜色：`Tray.Dark` 返回该栏是否为深色，可以是受系统外观或壁纸影响的 macOS 菜单栏，也可以是 Windows 任务栏；外观变化时会触发 `OnAppearanceChange`，可在回调中重新设置图标：

```go
draw := func() { tray.SetIcon(meterIcon(tray.Dark()), false) }
tray.OnAppearanceChange(draw)
draw()
```

`Tray.PopUpMenu` 以编程方式打开用 `SetMenu` 设置的菜单。在 macOS 上，AppKit 把它放在状态项下方，并在菜单打开时高亮其按钮。在 Windows 11 上，它像系统托盘菜单那样打开，以图标为中心，位于任务栏上方一点（如果任务栏在另一条边上，则位于它旁边）；在 Windows 10 上，以及对于任务栏溢出区域中的图标，在指针处打开。要只在右键单击时显示菜单而保留主单击可用，在 `OnRightClick` 中设置菜单，调用 `PopUpMenu`，然后在调用返回后用 `SetMenu(nil)` 清除它。

`Tray.Bounds` 是图标在屏幕上的位置。在 macOS 上它是状态项的框架；在 Linux 上它总是空的，因为面板从不报告图标的位置；在 Windows 上，被用户收进通知区域溢出区的图标，在那个浮出区关闭时 `Bounds` 为空。`Bounds` 为空并不表示图标没了：Explorer 重启时，MyGo 会重新添加每个图标，所以应用从不需要重建托盘。

托盘图标不会让应用保持运行：应用在最后一个窗口关闭时退出（[退出](app.md#退出)）。要把窗口关闭到托盘，改为隐藏它，这会保留窗口和应用。退出（例如通过托盘菜单的 `RoleQuit`）会在窗口关闭之前开始，因此 `OnClose` 能区分退出造成的关闭和用户自己的关闭：

```go
var quitting bool // OnBeforeQuit and OnClose run on the main thread
mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { quitting = true })

win.OnClose(func(e *mygo.CloseEvent) {
	if !quitting {
		e.PreventDefault()
		win.Hide()
	}
})
// The icon's click, without a menu, or an item of its menu, brings it back.
tray.OnClick(win.Show)
```

在 Linux 上，托盘图标使用 AppIndicator，它需要 `libayatana-appindicator3`：它们显示菜单，带 `OnClick` 监听器的图标会响应激活（需要包含 `activate` 信号的库版本；更旧的版本不会报告单击），GNOME 的 AppIndicator 扩展在双击时发送激活，而超过双击时间间隔后的单击会打开菜单。没有该库时 `NewTray` 返回错误。当桌面偏好深色时，模板托盘图标为白色，保留其 alpha，并在设置门户的配色方案变化时重绘。`Tray.Dark` 读取该桌面偏好，独立于 `Theme.SetSource`；没有门户时返回 false。AppIndicator 无法报告面板自己的主题，它可能与桌面的不同。

常驻菜单栏的应用在 macOS 上通常会去掉 Dock 图标：

```go
mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
```

关于能打开原生 UI 窗口的图标，见[菜单栏示例](https://github.com/egoist/mygo/tree/main/examples/menubar-native)。它在 macOS 和 Windows 上不设置托盘菜单，用 `OnClick` 切换一个用 `Tray.Bounds` 定位的无边框窗口，并在 `OnBlur` 或 Escape 时隐藏它。在 Linux 上，一个 AppIndicator 菜单项打开同一个窗口。

## 快捷键

快捷键是修饰键与按键用 `+` 连接的组合，例如 `CmdOrCtrl+N` 或 `Alt+Shift+F4`，不区分大小写。

修饰键：

| 修饰键 | 按键 |
|---|---|
| `CmdOrCtrl` (`CommandOrControl`) | macOS 和 iOS 上为 Command，其他地方为 Control |
| `Cmd` (`Command`), `Super`, `Meta`, `Win` | macOS 和 iOS 上为 Command，其他地方为 Windows 或 Super 键 |
| `Ctrl` (`Control`) | Control |
| `Alt` (`Option`, `AltGr`) | Alt，macOS 和 iOS 上为 Option |
| `Shift` | Shift |

按键：

- 一个字符：字母、数字或 `,`、`/` 之类的符号（`+` 写作 `Plus`）；
- `F1` 到 `F24`；
- `Enter`（`Return`）、`Tab`、`Space`、`Backspace`、`Delete`（`Del`）、`Insert`（`Ins`）、`Escape`（`Esc`）；
- `Up`、`Down`、`Left`、`Right`、`Home`、`End`、`PageUp`、`PageDown`；
- `Num0` 到 `Num9`、`NumDec`、`NumAdd`、`NumSub`、`NumMult`、`NumDiv`（数字键盘）；
- `VolumeUp`、`VolumeDown`、`VolumeMute`、`MediaPlayPause`、`MediaNextTrack`、`MediaPreviousTrack`、`MediaStop`、`PrintScreen`、`CapsLock`、`NumLock`、`ScrollLock`。

同样的语法用于注册[全局快捷键](native.md#全局快捷键)。
