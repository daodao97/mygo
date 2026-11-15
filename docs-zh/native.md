# 桌面 API

MyGo 封装了桌面应用最常用的操作系统功能，适用于窗口显示网页、原生 UI 或两者兼有的应用。与 MyGo 的其他部分一样，每一项都可以从任意 goroutine 调用。

## 对话框

`mygo.Dialog` 显示系统的文件和消息对话框，调用会阻塞直到用户作出选择：绑定方法在独立的 goroutine 中运行，可以直接调用；事件监听器和菜单项回调需要尽快返回，应另起 goroutine 调用。`Parent` 指定对话框所属的窗口；在 macOS 上，对话框以附着式对话框（sheet）形式附着在父窗口上。

```go
// Pick files.
paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
	Parent:   win,
	Title:    "Import",
	Filters:  []mygo.FileFilter{{Name: "Images", Extensions: []string{"png", "jpg"}}},
	Multiple: true,
}) // nil when canceled

// Choose where to save.
path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
	Parent:      win,
	DefaultPath: "notes.json",
}) // "" when canceled

// Ask a question.
res, err := mygo.Dialog.Message(mygo.MessageOptions{
	Parent:             win,
	Type:               mygo.MessageWarning,
	Message:            "Delete 3 notes?",
	Detail:             "You cannot undo this.",
	Buttons:            []string{"Cancel", "Delete"},
	DestructiveButtons: []int{1},
	CheckboxLabel:      "Don't ask again",
})
if res.Button == 1 { /* Delete */ }

// Report an error.
mygo.Dialog.Error("Import failed", err.Error())
```

`OpenDialogOptions` 还可以选择目录（`Directory`）、显示隐藏文件，以及在 macOS 上显示消息和“新建文件夹”按钮。

过滤器也可以按类型选取文件，类似 HTML 的 `accept`：可以用 `MIMETypes` 指定 `"application/pdf"` 这样的类型，也可以用 `"image/*"` 指定一整类；系统选择器会按自己认识的类型来匹配，而不需要逐一列出所有扩展名：

```go
mygo.FileFilter{Name: "Pictures", MIMETypes: []string{"image/*"}}
```

macOS 11 及更高版本把它们作为文件类型（`UTType`），把 `image/*`、`audio/*`、`video/*`、`text/*` 和 `font/*` 这些种类作为对应的类型（`public.image` 等），更早的版本则只接受这些类型；GTK 会匹配这些类型，也匹配通配符；Windows 按注册表中该类型对应的扩展名过滤，对于整类，则按该种类的每种类型以及 `PerceivedType` 标明属于它的每个文件过滤。

在消息对话框中，Enter 激活 `DefaultButton`，Escape 激活 `CancelButton`，即使它们是同一个按钮，如上面的 Cancel。在 macOS 上 Cmd+. 也会取消，`DefaultButton` 为 -1 时 Enter 不激活任何按钮。`CancelButton` 默认为第一个标有 Cancel 或 No 的按钮，否则为第一个按钮。对于 Delete / Keep，设置 `CancelButton: 1` 以选择 Keep；否则 Escape 选择 Delete。`DestructiveButtons` 用来标出会删除或无法撤销的按钮。

## 通知

```go
n := mygo.NewNotification(mygo.NotificationOptions{
	Title: "Export finished",
	Body:  "notes.json was saved to Documents.",
})
n.OnClick(func() { mainWindow().Focus() })
if err := n.Show(); err != nil {
	log.Println(err)
}
```

`Close` 移除通知。macOS 只显示打包应用的通知，而 `mygo dev` 和 `mygo build` 生成的就是打包应用；`mygo.NotificationsSupported()` 报告应用能否显示通知。Linux 通过桌面的通知服务显示，Windows 显示为通知区域的气泡。`Show` 在系统拿到通知后返回。在 macOS 上，第一次会询问用户是否允许通知，`Show` 会等待回答；如果通知未获允许——无论是这次回答拒绝，还是之后在系统设置中关闭——它就返回 `mygo.ErrNotificationsDenied`。`ShowContext(ctx)` 在 `ctx` 结束时停止等待（例如应用在询问仍未作答时退出），并撤掉之后才因用户允许而显示出来的那条通知。

macOS 在通知被点击后仍会把它留在通知中心，直到应用将其移除：在 `OnClick` 中调用 `Close` 即可撤掉；也可以在应用回到前台时调用 `mygo.ClearNotifications()`，它会移除应用的所有通知，包括更早几次运行留下的通知：

```go
n.OnClick(func() {
	mainWindow().Focus()
	n.Close()
})

// What the user has seen in the app does not wait in Notification Center.
mygo.App.OnDidBecomeActive(func() { mygo.ClearNotifications() })
```

Linux 会请求桌面的通知服务关闭它们（该服务可能早已关掉），Windows 则收起自己的气泡。

通知可以比显示它的那次运行存在得更久，在 macOS 的通知中心里，点击它还会再次启动应用。给它一个应用自己的 `ID`，`App.OnNotificationClick` 会在每次点击时收到这个 ID，也包括应用因这次点击而启动的那一次——只要处理器在 `Run` 之前注册。用仍在显示的通知的 `ID` 再显示通知，会替换掉原来那条。`Group` 把通知归入通知中心的同一个堆叠，例如同一段对话的消息（macOS）：

```go
// IDs are "<conversation>/<message>".
mygo.App.OnNotificationClick(func(id string) {
	conversation, _, _ := strings.Cut(id, "/")
	openConversation(conversation)
})

mygo.NewNotification(mygo.NotificationOptions{
	ID:    "ada/" + msg.ID,
	Group: "ada",
	Title: "Ada",
	Body:  msg.Text,
}).Show()
```

`Image` 在通知中显示一张图像，即 PNG 或 JPEG 的字节，例如刚开始播放的曲目的封面。macOS 会把它作为缩略图附上，通知展开时缩略图随之变大；Linux 把它的像素交给桌面的通知服务，并缩小到 256 像素以内；Windows 把它显示为气泡的大图标，代替应用的图标。系统无法解码的图像会使通知不显示图像。`NewNotification` 复制它，因此应用可以重用自己的缓冲区；MyGo 不获取远程图像：

```go
// One notification for what plays, replaced as the track changes.
mygo.NewNotification(mygo.NotificationOptions{
	ID:     "now-playing",
	Title:  track.Title,
	Body:   track.Artist + " — " + track.Album,
	Image:  track.Cover,
	Silent: true,
}).Show()
```

### macOS 上的发送者头像

在 macOS 12 及更高版本上，收到的私信可以提供发送者，从而显示一个醒目的头像，并附上应用的小图标：

```go
mygo.NewNotification(mygo.NotificationOptions{
	ID: "message:7", Group: "conversation:3",
	Title: "Ada", Subtitle: "Lunch", Body: "See you at noon",
	Sender: &mygo.NotificationSender{
		ID: "user:ada", Name: "Ada", Avatar: avatarPNG,
	},
}).Show()
```

`Sender.ID` 是应用内部的发送者标识，MyGo 不会将其当作电子邮件地址或电话号码。`Name` 默认为 `Title`。`Avatar` 保存 PNG、JPEG 或 SVG 等编码后的图像数据，macOS 会先将其光栅化为 PNG，再用于向 SiriKit 提交消息信息（donation）。图像由应用获取，是否将发送者和消息内容共享给系统也由应用决定。`NewNotification` 会复制发送者信息和图像，因此应用可以重用原缓冲区。`Group` 指定对话标识，未指定时使用通知 ID。此 API 仅用于收到的私信，不适用于群聊或发出的消息。通信通知的布局由 macOS 决定，副标题显示在消息正文上方。

为应用的 Apple App ID 启用 **Communication Notifications**，并把 `INSendMessageIntent` 添加到 `macos.infoPlist.NSUserActivityTypes`。使用 `com.apple.developer.usernotifications.communication` 签名，并嵌入匹配的描述文件，即使通过 Developer ID 分发也要如此：设置 `macos.provisioningProfile`，`mygo build` 会在签名前把它复制到 `Contents/embedded.provisionprofile`（见[描述文件](distribution.md#描述文件)）。描述文件必须授权应用的标识符、证书和各项权限。不要给 ad-hoc 构建添加这项受限权限：macOS 可能拒绝启动它。

MyGo 会先检查签名中的授权，再向 SiriKit 提交收到的消息信息，并更新通知内容。如果缺少所需授权、发送者信息不完整、头像无效，或提交信息、转换内容失败，则回退为普通通知。Windows、Linux 和其他不支持的平台仍按原有方式显示通知。无论采用哪种方式，ID、声音、分组、点击处理器、`Close` 和 `ClearNotifications` 都遵循同一套通知生命周期。

## 剪贴板

```go
mygo.Clipboard.WriteText("hello")
text := mygo.Clipboard.ReadText()

mygo.Clipboard.WriteHTML("<b>hello</b>")
mygo.Clipboard.WriteImage(png) // a PNG
img := mygo.Clipboard.ReadImage()

formats := mygo.Clipboard.AvailableFormats()
mygo.Clipboard.Clear()
```

页面也可以使用 Web 的 `navigator.clipboard`。

`Clipboard.Write(transfer.Data)` 和 `Clipboard.Read(formats...)` 使用与原生拖放相同的数据模型：有序的数据项、同一数据的多种表示、文件列表、自定义 MIME 格式，以及按需提供数据的提供者。`Formats` 无需读取数据就能列出跨平台的格式名称；`ReadFormat`、`ReadFiles`、`WriteFiles` 和 `Flush` 用于显式协商格式、传输文件和持久保存数据。关于提供者的生命周期和原生格式映射，见[剪贴板和拖放数据](data-transfer.md)。[剪贴板示例](../examples/clipboard)演示了这些操作。

## Shell

```go
mygo.Shell.OpenExternal("https://example.com") // the default browser
mygo.Shell.OpenPath("/Users/ada/report.pdf")   // the default app for the file
mygo.Shell.ShowItemInFolder(path)              // Finder, Explorer, the file manager
mygo.Shell.TrashItem(path)                     // to the trash
mygo.Shell.PreviewItem(path)                   // Quick Look, as the space bar does
mygo.Shell.Beep()
```

`TrashItem` 从不永久删除：某项无法移入废纸篓时，它就会失败，并把该项留在原处。在 Windows 上，这指的是大到回收站放不下的项，或位于没有回收站的驱动器上的项（网络共享和许多可移动驱动器就是如此），否则 Windows 会不经询问就删除它。

`PreviewItem` 在 Quick Look 面板中显示文件，即 Finder 用空格键打开的预览：面板会一直打开，直到用户关闭它；再次请求会替换它当前显示的项。它只在 macOS 上可用；其他平台返回 `ErrUnsupported`。面板不是模态的：应用继续运行，调用在项设置好后返回。

`FileIcon` 返回文件管理器为文件、文件夹或应用显示的图标，以 PNG 形式返回，大小就是请求的像素数：传入的尺寸应为 DIP 值乘以显示器的缩放系数。

```go
png, err := mygo.Shell.FileIcon("/Applications/Safari.app", 64)
```

在 macOS 上它是 Finder 的图标（`NSWorkspace`），从最适合该尺寸的表示绘制；符号链接则使用其指向对象的图标，不带 Finder 的箭头。在 Windows 上它是资源管理器的图标（`IShellItemImageFactory`，仅图标，无缩略图），路径也可以指向 shell 项，例如用 `shell:AppsFolder\<AppUserModelID>` 指定开始菜单中的应用。在 Linux 上，它是图标主题中该文件内容类型对应的图标；对于桌面条目（`.desktop`），则是其应用的图标，主题没有时则用桌面条目的图标。

`TypeIcon` 按扩展名返回某文件类型的图标，扩展名带不带点都行，磁盘上不需要有该类型的文件：比如尚未下载的附件的图标。它的绘制方式与 `FileIcon` 相同，在浏览器中会失败。

```go
png, err := mygo.Shell.TypeIcon("pdf", 64)
```

`InstalledApps` 列出计算机上安装的应用，按名称排序，就像启动器那样；`LaunchApp` 启动其中一个，或把它带到前台：

```go
go func() {
	apps, err := mygo.Shell.InstalledApps() // reads the disk: off the main thread
	if err != nil {
		return
	}
	for _, app := range apps {
		fmt.Println(app.Name, app.ID)
	}
	icon, _ := mygo.Shell.FileIcon(apps[0].Path, 64)
	_ = mygo.Shell.LaunchApp(apps[0])
}()
```

每个应用有系统显示的名称 `Name`（使用用户的语言）、一个 `ID`，以及 `FileIcon` 为其绘制图标的 `Path`：

| | 应用 | `ID` | `Path` |
|---|---|---|---|
| macOS | `/Applications`、`/System/Applications`、`~/Applications` 和 CoreServices 的 `Applications` 中的 `.app` 应用包，以及它们各文件夹（Utilities）中的应用包，外加 Finder；通过符号链接指向的应用只在链接处列出一次，如 `/Applications/Safari.app` | 应用包标识符，没有标识符的应用包则为路径 | 该应用包 |
| Windows | 开始菜单的“所有应用”，桌面应用和打包应用都在内（shell 的 Applications 文件夹） | AppUserModelID，桌面应用没有时则为已知文件夹下的路径 | `shell:AppsFolder\<ID>` |
| Linux | 菜单显示的桌面条目（不是 `NoDisplay`、`Hidden` 或为其他桌面准备的） | 桌面文件 ID，如 `org.gnome.Nautilus.desktop` | 该桌面条目 |

从其他 goroutine 调用 `InstalledApps` 不会阻塞主线程，通常几十毫秒即可返回。每个图标的绘制需要占用主线程几毫秒，因此只应绘制可见项的图标，并缓存结果。

## 显示器

`mygo.Screen` 描述已连接的显示器，以设备无关像素为单位，原点在主显示器的左上角：

```go
primary := mygo.Screen.PrimaryDisplay()
fmt.Println(primary.Bounds, primary.WorkArea, primary.ScaleFactor)

for _, d := range mygo.Screen.Displays() { /* ... */ }

cursor := mygo.Screen.CursorScreenPoint()
d := mygo.Screen.DisplayNearestPoint(cursor)

mygo.Screen.OnDisplaysChanged(func() { /* rearrange windows */ })
```

`WorkArea` 不包含菜单栏、Dock 和任务栏。

在 Windows 上，显示器可以有不同的缩放。每个显示器的坐标从其左上角开始，起点是像素值除以所有显示器中最小的缩放比，然后按自身的缩放比延伸，因此各显示器的坐标区域不会重叠：缩放比更大的显示器会在其后留下空隙。所有显示器的缩放比相同时，坐标就是像素值除以该缩放比。窗口的边界采用它大部分所在显示器的缩放比。`DisplayMatching(rect)` 返回一个矩形重叠最多的显示器，即窗口所在的显示器。

## 深色模式

`mygo.Theme` 是浅色或深色外观，页面通过 `prefers-color-scheme` 媒体查询跟随它：

```go
dark := mygo.Theme.IsDark()
mygo.Theme.OnUpdated(func() { log.Println("dark:", mygo.Theme.IsDark()) })

mygo.Theme.SetSource(mygo.ThemeDark) // or ThemeLight, or ThemeSystem to follow the system
```

`SetSource` 可以在 `App.Run` 之前调用，例如使用用户在应用偏好设置中保存的外观：应用会以该外观启动。

## 强调色和辅助功能

`mygo.Theme.Preferences()` 返回桌面自身控件的强调色以及它们遵循的辅助功能设置，一次性全部读取：

```go
p := mygo.Theme.Preferences()
p.Accent             // a color.NRGBA (image/color), A 0 where the desktop has none
p.ReduceMotion       // the desktop asks for less motion
p.ReduceTransparency // the desktop asks for opaque surfaces, e.g. no vibrancy
p.HighContrast       // the desktop asks for more contrast
p.TextScale          // how many times larger text should be; 1 is the usual size
```

强调色是带非预乘 alpha 的 8 位 sRGB；`ui.RGB(p.Accent.R, p.Accent.G, p.Accent.B)` 把它变成 `ui.Color`，供你自己的绘制使用。

`ReduceMotion` 对应 macOS 的“减弱动态效果”、Windows 关闭动画效果，以及 GNOME 关闭动画；`ReduceTransparency` 对应 macOS 的“降低透明度”和 Windows 关闭透明效果——Linux 桌面没有这些，因此在那里为 false；`HighContrast` 对应 macOS 的“增强对比度”、Windows 的对比度主题，以及 Linux 上设置门户提供的更高对比度（或 HighContrast GTK 主题）。macOS 没有文本缩放，因此那里的 `TextScale` 为 1。在设置门户不提供强调色的 Linux 桌面上，以及浏览器中，没有强调色。

`Theme.OnUpdated` 在桌面改变其中任何一项时触发，调用 `SetSource` 时也是如此。读取这些设置可能需要请求桌面（Linux 上是设置门户），因此 MyGo 只读取一次，并一直保留到下次变化。包 `ui` 的默认主题已经会跟随强调色、对比度和文本缩放，`Animate` 则跟随减弱动态效果。与其他需要应用正在运行的调用一样，在 `App.Run` 之前的 `main` 中调用 `Preferences` 会 panic；从其他 goroutine 调用则会等待应用启动。

页面从文档一开始就能拿到这些设置，途径是 `mygo-runtime` 的 `preferences()` 以及 `--mygo-accent` 和 `--mygo-text-scale` CSS 变量，见[前端指南](frontend.md#强调色与辅助功能)；`prefers-reduced-motion` 和 `prefers-contrast` 已经跟随桌面。通过事件发送或由绑定方法返回的 `ThemePreferences` 就是同一个 JSON，生成的客户端用 mygo-runtime 的 `ThemePreferences` 为它标注类型。

## 电源

```go
// Keep the computer awake during a long task, and the display on with true.
release := mygo.Power.KeepAwake("Exporting the video", false)
defer release()

// Keep the app at full speed while none of its windows shows.
stop := mygo.Power.KeepRunning("Serving the phones")
defer stop()

mygo.Power.OnSuspend(func() { pauseSync() })
mygo.Power.OnResume(func() { resumeSync() })
mygo.Power.OnLockScreen(func() { hideSecrets() })
mygo.Power.OnUnlockScreen(func() { /* ... */ })

onBattery := mygo.Power.IsOnBattery()
idle := mygo.Power.IdleTime() // since the last keyboard or mouse input
```

传给 `KeepAwake` 的原因会显示在系统列出保持唤醒项的地方。

macOS 会在一分钟内把没有任何窗口显示的应用——窗口被隐藏、最小化、被遮挡，或位于锁屏之后——置于 App Nap：它降低应用的优先级，合并计时器并限制 I/O，这会拖慢在后台提供网络服务、串流或同步的应用。托盘图标只要还显示在菜单栏上，就算作一个正在显示的窗口。`KeepRunning` 会在释放之前一直阻止 App Nap，并让计算机在空闲时仍能休眠，正如整天待在菜单栏里的应用所希望的那样；`KeepAwake` 也会阻止 App Nap，但同时让计算机保持唤醒。其他系统上 `KeepRunning` 不做任何事。

在 Linux 上，`OnLockScreen` 会在屏幕保护程序启动时被调用（它通常会锁定屏幕）；在不上报空闲时间的桌面上，`IdleTime` 为 0。

## 正在播放和媒体键

`mygo.NowPlaying` 在系统显示媒体的位置显示应用播放的内容：macOS 上控制中心和菜单栏中的“正在播放”，Windows 上音量弹出控件和锁屏的媒体控制，以及 Linux 上的 MPRIS 客户端，例如 GNOME 和 KDE 的媒体控制以及 `playerctl`。在它显示内容期间，键盘和耳机的媒体键以及那些控件会向应用发送命令：

```go
mygo.NowPlaying.Set(mygo.NowPlayingInfo{
	Title: "Spring", Artist: "Vivaldi", Album: "The Four Seasons",
	Artwork:  cover, // PNG or JPEG
	Duration: 3*time.Minute + 20*time.Second,
	Commands: mygo.MediaPlay | mygo.MediaPause | mygo.MediaNext | mygo.MediaPrevious | mygo.MediaSeek,
})
mygo.NowPlaying.OnCommand(func(cmd mygo.MediaCommand, pos time.Duration) {
	switch cmd {
	case mygo.MediaPause:
		player.Pause()
		mygo.NowPlaying.SetPlayback(mygo.PlaybackPaused, pos) // where playback is
	case mygo.MediaSeek:
		player.Seek(pos)
		mygo.NowPlaying.SetPlayback(mygo.PlaybackPlaying, pos)
	case mygo.MediaNext:
		playNext() // and Set the next track's info
	}
})
mygo.NowPlaying.Clear() // the app no longer shows in the controls
```

状态为 `PlaybackPlaying` 时，系统会推进位置，因此应用只在播放跳转时重新设置位置。播放/暂停键在状态为播放时发送 `MediaPause`，否则发送 `MediaPlay`；只有信息中提供的命令会到达 `OnCommand`（`Commands` 为 0 时提供播放、暂停和停止）。`pos` 是 `MediaSeek` 要跳转到的位置；对于其他命令，`pos` 是命令到来时的播放位置。如果应用用包 `audio` 的播放器播放，可以让 `Control` 播放、暂停、停止它，并跳转播放位置，还能显示它的状态：

```go
release := mygo.NowPlaying.Control(music) // an *audio.Player
defer release()
```

`Control` 会监听播放器的播放结束；应用自己播放、暂停或跳转播放位置之后，用 `NowPlaying.Sync` 把状态显示出来。在浏览器中，`NowPlaying` 就是页面的 Media Session。

## 网络

`mygo.Network` 说明计算机是否在线以及连接是否按流量计费——比如手机热点，或会计算流量的套餐，这时应用应暂缓大文件下载：

```go
status := mygo.Network.Status()
if status.Online && !status.Metered {
	syncNow()
}

mygo.Network.OnChanged(func(s mygo.NetworkStatus) {
	log.Printf("online: %v, metered: %v", s.Online, s.Metered)
})
```

`Online` 表示系统有到计算机之外某个网络的连接，如同浏览器的 `navigator.onLine`，但这并不证明服务器会应答：位于强制门户之后的网络，或通往互联网的路径中断的网络，也算在线。macOS 和 Linux 检查是否有可用的网络路由（一条已连通的路径，GLib 的 `network-available`），Windows 则看连接级别是否为本地访问或更高。`Metered` 在 macOS 上是高费用路径（蜂窝网络、手机热点）或低数据模式，在 Linux 上是按流量计费的连接，在 Windows 上是费用固定或可变、或会漫游、已超出或接近数据上限的连接。在浏览器中 `Online` 就是 `navigator.onLine`，`Metered` 为 false。

`Status` 可以从任何 goroutine 安全调用；在 macOS 上，第一次调用会等待系统的第一个回答，它会在几毫秒内到来。从第一次调用 `OnChanged` 起开始监视变化；它的监听器只在状态变化时运行，且运行在主线程上。`OnChanged` 返回一个用于移除监听器的函数。

页面已经有 `navigator.onLine`。要在 UI 中显示原生状态，从监听器广播一个类型化事件，使生成的客户端收到它：

```go
// Connectivity is pushed to every window when the connection changes, as
// {online, metered}.
var Connectivity = mygo.NewEvent[mygo.NetworkStatus]("connectivity")

func init() {
	mygo.Network.OnChanged(func(s mygo.NetworkStatus) { Connectivity.Broadcast(s) })
}
```

### 代理

网页通过 webview 访问网络，webview 遵循用户在系统设置中设置的代理，包括其绕过列表和设置脚本（PAC 文件）：WebView2 如同 Chromium，从 Windows 的代理设置读取；WKWebView 从 macOS 的代理设置读取；WebKitGTK 通过 GIO 的默认代理解析器读取 GNOME 的设置，在 KDE 上则通过 libproxy 读取其设置。

Go 的 `http.DefaultClient` 只读取 `HTTP_PROXY`、`HTTPS_PROXY` 和 `NO_PROXY`。包 `netproxy` 让 Go 的请求像页面一样遵循系统设置，MyGo 自己的请求也使用它：[更新器](updates.md)的请求，以及 [fetch](plugins/fetch.md) 和 [websocket](plugins/websocket.md) 插件的请求（除非它们的选项指定了客户端）。你自己的客户端可以用它的传输层接入：

```go
import "github.com/egoist/mygo/netproxy"

client := &http.Client{Transport: netproxy.Transport(), Timeout: time.Minute}
```

或者，如果你想用自己的传输层，则设置 `t.Proxy = netproxy.Proxy`。对每个请求：

1. 回环主机（`localhost`、`*.localhost`、`127.0.0.0/8`、`::1`）直接访问。
2. 如果设置了 `HTTP_PROXY`、`HTTPS_PROXY`、`ALL_PROXY` 或 `NO_PROXY`（或其小写名称），则由环境变量决定，与 `http.ProxyFromEnvironment` 相同，因此只为应用某次运行设置的代理会优先。
3. 否则由系统针对该 URL 的设置决定：在 Windows 上是由 WinHTTP 读取的用户设置，带自动检测（WPAD）和设置脚本；在 macOS 上是由 CFNetwork 读取的“系统设置 > 网络 > 代理”中的设置，带设置脚本；在 Linux 上是 GIO 的默认代理解析器，如同 WebKitGTK。系统没有答案时，请求直接连接。

系统针对特定 URL 协议 和主机返回的代理设置会缓存 5 秒，因此应用运行期间开启、关闭或修改代理后，后续请求会采用新设置。正在进行的下载或流等请求仍使用原连接。Go 的传输层 支持 HTTP、HTTPS 代理和 SOCKS5，不支持 SOCKS4；只有系统返回的设置中包含代理密码时，才会发送密码，GNOME 的设置就可能包含密码。在浏览器中，Go 的请求由浏览器通过自己的代理发出。

## 机密

`mygo.Secrets` 把小型机密，例如访问令牌和 API 密钥，保存在系统的凭据存储中，由系统代为加密，不像用户数据目录里的文件：macOS 上的登录钥匙串，Linux 上的 Secret Service（GNOME Keyring、KWallet、KeePassXC……，通过 libsecret）以及 Windows 上的凭据管理器。

```go
if err := mygo.Secrets.Set("github-token", []byte(token)); err != nil {
	log.Println(err)
}

token, err := mygo.Secrets.Get("github-token")
if errors.Is(err, mygo.ErrSecretNotFound) {
	// Ask the user to sign in.
}

mygo.Secrets.Delete("github-token") // on sign out; nil when there is none
```

一个机密最多为 `mygo.MaxSecretSize`（2560）字节，这也是凭据管理器保存的上限，键最多 256 字节。要保存更多，可以用保存在 `Secrets` 中的密钥加密后写入用户数据目录的一个文件。这些方法可以从任何 goroutine 安全调用，在 `Run` 之前也可以；它们可能在系统向用户询问什么时阻塞，例如解锁钥匙串，此时主线程上的应用会继续处理事件。要在页面中使用它们，绑定一个调用它们的后端方法，这样跨到页面里的就只有页面需要的东西。

凭据存储用服务和账户来标识一个机密：`Secrets` 用键作为账户，用应用的标识符（`mygo.config.ts` 的 `identifier`）作为服务。`mygo dev` 的开发应用是 `<identifier>.dev`，因此它把自己的机密与已安装应用的机密分开保存；没有标识符运行的应用，例如用 `go run`，则使用其名称。`Secrets.Service` 可以选择另一个服务，以便与应用的其他程序（例如其命令行工具）共享机密，或读取 Electron 应用用 keytar 保存的机密——keytar 像 `Secrets` 一样，把它们保存在应用传给它的服务下：

```go
// keytar.getPassword("MyApp", "refresh-token") in the Electron app
token, err := mygo.Secrets.Service("MyApp").Get("refresh-token")
```

机密能防的是其他用户和被盗磁盘，而不是同一用户的其他应用：macOS 在另一个应用读取它们之前会询问用户，重新构建的应用没有用相同身份签名时也会询问；Linux 和 Windows 则允许它们读取。`mygo dev` 用用户的 Apple Development 身份为每次构建签名，因此“始终允许”在多次重新构建后依然有效；以 ad hoc 方式签名的应用，例如没有可用身份时，或使用 `-sign -`、`go run` 时，在每次构建后都会被再次询问。

在没有凭据存储的地方，这些方法会失败，并返回满足 `errors.Is(err, errors.ErrUnsupported)` 的错误：在浏览器中，以及在 Linux 上没有 libsecret（Debian 和 Ubuntu 上的 `libsecret-1-0` 包）、没有会话总线或会话总线上没有 Secret Service 时都是如此——服务器和精简桌面就是这种情况。应用可以退而求其次，例如每次都向用户询问机密：

```go
token, err := mygo.Secrets.Get("github-token")
switch {
case errors.Is(err, errors.ErrUnsupported):
	// No credential store: ask the user, keep it in memory.
case errors.Is(err, mygo.ErrSecretNotFound):
	// Ask the user to sign in.
}
```

## 监视文件

`mygo.WatchFiles` 监视一个文件或目录，加上 `Recursive` 还会监视目录中的子目录，以发现变化。等到所有内容都进入监视后它才返回；`Next` 等待下一批变化，并把它们作为一批返回，应放在单独的 goroutine 上使用：

```go
w, err := mygo.WatchFiles(ctx, dir, mygo.FileWatchOptions{
	Recursive: true,
	Ignore: func(p string, isDir bool) bool {
		return isDir && (path.Base(p) == "node_modules" || path.Base(p) == ".git")
	},
})
if err != nil {
	return err
}
go func() {
	defer w.Close()
	for {
		events, err := w.Next(ctx)
		if err != nil {
			return // io.EOF once closed, or once dir went away
		}
		for _, e := range events {
			log.Println(e.Op, e.Path)
		}
	}
}()
```

事件的 `Path` 相对于被监视的内容（`w.Root()`，即解析符号链接后的路径），用斜杠分隔，自身为 `.`；`Op` 是 `mygo.FileCreate`、`FileWrite`、`FileRemove` 或 `FileRename`，重命名时会用 `OldPath` 给出原位置。已经存在的内容不会被报告。变化要等到 50 毫秒没有动静（`Debounce`，为负数时立即）后才报告，最长不超过 500 毫秒（`MaxDelay`）。监视会把这次看到的内容和上次看到的作比较，因此事件只说明差异，而不是造成差异的每一步：应把它们当作需要重新读取文件的提示。写入指的是文件大小或修改时间变了，或系统说该文件被写入过，或原路径上的文件被另一个文件替换（编辑器和原子保存就是这样做的：临时文件重命名覆盖原文件）；权限和其他元数据（索引器和杀毒软件会改动它们）不会被报告，目录被写入也不会——目录的变化体现为其中条目的变化。

在 `Next` 取走上一批之前，监视不会再次查看：当应用繁忙时，期间发生的变化会进入下一批，因此什么也不会丢失，什么也不会堆积。监视无法查看的内容，例如没有权限的目录，会被略过，而不是结束监视。`Ignore` 按路径略过文件和目录：它忽略的目录永远不会被打开，也不计入 `MaxEntries`。

文件是通过其所在目录来监视的，这是 [fsnotify](https://github.com/fsnotify/fsnotify) 推荐的做法：文件被替换（如编辑器保存时）表现为对 `.` 的写入，被移除表现为移除，再次出现表现为创建。目录本身（或文件所在的目录）被移除、重命名或替换后，最后一批事件中会有针对 `.` 的 `FileRemove`，随后 `Next` 返回 `io.EOF`。

调用 `Close` 或取消传给 `WatchFiles` 的上下文都会停止监视：`Next` 返回 `io.EOF`，监视释放完句柄后，`Done` 关闭。应用退出时会停止所有监视。目录中的文件和目录超过 `MaxEntries` 时，监视会以 `mygo.ErrWatchLimit` 停止：macOS 上为 65536，因为一个 FSEvents 流覆盖整棵树；其他地方为 8192，因为那里的后端可能为每个条目占用一个句柄。[监视插件](plugins/watch.md)为页面提供对应用所选择的目录和文件的监视，并说明每个平台如何监视。

`HintsOnly` 用上述全部精确性来换取更低的开销。监视不保留快照，也不 stat 任何文件：它只报告 `mygo.FileHint` 事件，每个事件只给出一个目录，系统报告该目录中或目录下有内容变化，系统漏掉变化后则给出 `.`。哪个文件变了、怎么变的、是否被重命名，永远不得而知。它适合开发时的重载器——只要有变化就重新构建：

```go
w, err := mygo.WatchFiles(ctx, dir, mygo.FileWatchOptions{
	Recursive: true,
	HintsOnly: true, // only "something under this directory changed"
	Ignore: func(p string, isDir bool) bool {
		return isDir && path.Base(p) == "node_modules"
	},
})
```

只要文件内容重要，就绝不要用它：跟踪文稿、同步，或任何需要根据它无法得知的路径去操作的场景。使用它时，只会针对目录调用 `Ignore`（即使变化来自它本会忽略的文件，也仍然会提示该文件所在的目录），`MaxEntries` 统计的是纳入监视的目录数。在 macOS 和 Windows 上，递归的仅提示监视只纳入自己的目录，由系统整体监视该目录；在 Linux 上它纳入每个目录，读取其中条目的类型而不 stat 它们。符号链接仍然不会被跟随，目录之外的内容不会被监视；被监视的目录若消失，仍会以一个针对 `.` 的 `FileRemove` 结束监视。

## 全局快捷键

全局快捷键在应用位于后台时工作，例如把它带到前台：

```go
err := mygo.GlobalShortcut.Register("CmdOrCtrl+Shift+Space", func() {
	win.Show()
	win.Focus()
})
```

函数在主线程上运行。快捷键使用[加速键](menus.md#快捷键)的语法；`Unregister`、`UnregisterAll` 和 `IsRegistered` 管理它们。快捷键已被占用时 `Register` 会失败：占用者可能是应用自己，也可能是另一个应用（如果系统会告知的话）。

在 X11 下的 Linux 上，应用自己抓取按键。在 Wayland 下应用无法这样做，桌面会通过 XDG 桌面门户为应用绑定快捷键，KDE Plasma 6 和 GNOME 48 及更高版本提供该门户；在 Wayland 会话中通过 XWayland 运行的应用（`GDK_BACKEND=x11`）也是如此，因为它自己抓取的按键只有在它拥有焦点时才能收到：桌面可能要求用户确认新快捷键，并允许他们改选其他按键；它需要应用已安装，因为它按桌面条目识别应用。快捷键在 `Register` 返回后不久生效。函数显示或聚焦的窗口，在桌面提供激活令牌时（GNOME 就是如此）会借助按键的激活令牌获得焦点；在 XWayland 下，GNOME 会显示这些窗口但不给它们焦点，焦点留在用户正在使用的窗口上。
