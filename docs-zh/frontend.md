# 前端

网页窗口显示应用的 Web 前端，即用任意工具构建的 HTML、CSS 和 JavaScript。新项目默认使用 TypeScript 和 [Vite](https://vite.dev)，但 MyGo 不限制前端工具：开发时只需提供一个可加载的 URL，构建时只需提供前端产物所在的目录。[原生 UI](ui/README.md) 窗口不需要 Web 前端，纯原生 UI 应用也无需配置前端。

## 页面如何加载

窗口用不带协议的 URL 加载前端页面，例如 `/` 或 `/settings?tab=general`：

```go
mygo.NewWindow(mygo.WindowOptions{URL: "/"})
win.Page().LoadURL("/settings")
```

它们会相对于以下地址解析：

- 在 `mygo dev` 期间，解析到 `mygo.config.ts` 中 `devUrl` 指定的开发服务器，它由 `devCommand` 启动，例如 `http://localhost:5173/`，这样开发服务器的热重载在应用里也能用；
- 在构建出的应用中，解析到 `mygo://localhost/`，由它提供 `frontendDist` 中的文件，`mygo build` 在运行 `buildCommand` 后把这些文件嵌入可执行文件。

```ts
export default defineConfig({
  devUrl: "http://localhost:5173",
  devCommand: "bun run dev:web",
  buildCommand: "bun run build:web",
  frontendDist: "dist",
});
```

没有 `devUrl` 时，`mygo dev` 改为从磁盘提供 `frontendDist` 中的文件。不带文件扩展名、又匹配不到任何文件的路径会返回 `index.html`，因此客户端路由能正常工作。

前端是安全上下文，在 macOS 和 Linux 上它的源是 `mygo://localhost`，在 Windows 上是 `http://mygo.localhost`，因为 WebView2 在 Windows 上把自定义协议放在 `http://<scheme>.localhost` 下提供。`localStorage` 和 IndexedDB 等存储属于各自的源，因此页面在 `mygo dev` 下（开发服务器的源）存储的数据，并不是构建后的应用看到的数据。把重要的数据放在 Go 里，例如放在 `App.Path(mygo.PathUserData)` 下的文件里，再通过绑定方法交给页面。

窗口的页面也可以加载其他内容：

- `win.Page().LoadURL("https://example.com")`，一个网页；
- `win.Page().LoadHTML(html, baseURL)`，一个 HTML 字符串，重新加载时会再次加载它；
- `win.Page().LoadFile("docs/index.html")`，一个本地文件，先相对于工作目录解析，再相对于可执行文件所在目录（以及 macOS 应用的 Resources 目录）。

不属于应用自己的页面不能调用 Go 方法：见[谁可以调用](bindings.md#谁能调用)。

### 不用 CLI

`mygo build` 通过调用 `mygo.SetFrontend` 嵌入前端，用 `go build` 构建的程序也可以用 `embed` 这样做：

```go
//go:embed all:dist
var dist embed.FS

func main() {
	web, _ := fs.Sub(dist, "dist")
	mygo.SetFrontend(web)
	// ...
}
```

## 自定义协议

`mygo.Protocol.Handle` 用 `http.Handler` 处理 URL 协议，提供应用在运行时生成或读取的内容：页面、图片、用户文件。处理器各自在独立的 goroutine 中运行，以流式方式发送响应：webview 读取不及时，写入就会等待，因此无需将整个大文件放入内存。页面可以像普通 Web URL 一样，通过 `fetch`、`<img>`、ES 模块和相对 URL 加载这些内容：

```go
// Thumbnails generated into the cache directory.
cache, _ := mygo.App.Path(mygo.PathCache)
mygo.Protocol.Handle("thumbs", mygo.FileServer(os.DirFS(cache)))

// Anything an http.Handler does.
mygo.Protocol.HandleFunc("api", func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"path": r.URL.Path})
})
```

```html
<img src="thumbs://localhost/photo-42.png" />
```

在创建使用这些协议的窗口之前注册它们。`mygo.FileServer` 为 `fs.FS` 提供 HTTP 服务，支持内容类型和范围请求，并像前端一样回退到 `index.html`。处理 `mygo` 协议会用你的处理器替换前端。自定义协议的页面属于应用自己：它们可以调用 Go 方法，见[信任](#信任)。

在 Windows 上，处理器返回或写入达到 1 MB 后，WebView2 才开始接收响应，其余部分随后流式传输；WebView2 读取不及时，处理器的写入就会等待。WebView2 一次只读取一个自定义协议响应，而且似乎读完整个响应后才向页面交付内容。因此，服务器发送事件或长轮询等持续保持响应连接的处理器，只有返回后才能将内容交给页面；写入超过 1 MB 后，还会阻塞应用的其他响应。这类更新请改用[事件](bindings.md#事件)发送。同样，请求文件剩余部分的 `<video>` 或 `<audio>`（`Range: bytes=X-`）一次最多获取 4 MB：处理器收到的范围会缩小为 `bytes=X-(X+4 MB-1)`，并返回该部分内容（206），`mygo.FileServer` 和 `http.ServeFile` 均如此处理。播放时，元素会继续请求后续内容。WebView2 释放响应后，请求上下文会被取消，处理器便可停止写入。

音频和视频是例外：WebKitGTK 的媒体播放器无法播放自定义协议中的媒体，因此在 Linux 上，来自自定义协议的 `<video>` 或 `<audio>` 会以 `MEDIA_ERR_SRC_NOT_SUPPORTED`（`error.code` 4）失败，无论处理器如何作答。改为从环回 HTTP 服务器提供媒体，各平台做法相同，并把它的 URL 交给页面：

```go
ln, err := net.Listen("tcp", "127.0.0.1:0") // a free port
if err != nil {
	log.Fatal(err)
}
go http.Serve(ln, mygo.FileServer(os.DirFS(videos)))
mediaURL := "http://" + ln.Addr().String() + "/" // <video src="{mediaURL}clip.mp4">
```

机器上的任何程序都能访问这样的服务器：只提供它们有权读取的内容，或在它的路径里放一个应用的密钥。

每个协议都有自己的源，因此页面访问另一个协议会发出跨源请求：前端读取 `api://localhost/`，或者在 `mygo dev` 期间，开发服务器读取 `thumbs://localhost/`。对应用自己的页面（即可以调用 Go 方法的页面）的响应带有 CORS 头，让它们能读取响应，MyGo 也会应答它们的预检请求。其他站点则没有这些响应头，除非窗口信任它们（`PageOptions.TrustedOrigins`）；处理器也可以设置自己的响应头。在 Windows 上，访问协议要用 `http://<scheme>.localhost/`，WebView2 在那里提供它；访问另一个主机要用 `http://<host>.<scheme>.localhost/`（`myapp://abc/` 是 `http://abc.myapp.localhost/`）。这类主机名是 DNS 名称（由字母、数字、连字符和点组成），并以小写形式报告；带其他主机或端口的 URL 不会在那里得到服务。

### 信任

窗口会信任以下页面：应用前端、自定义协议页面、`file:` 和 `about:` 页面（`LoadHTML` 未指定基准 URL 时显示 `about:blank`）、来自 `PageOptions.TrustedOrigins` 中所列源的页面，以及开发构建中由 localhost 提供的页面。无论页面位于窗口还是 [Web 视图](ui/web-view.md)中，信任状态都取决于页面 URL，并随导航变化。受信任的页面可以：

- 调用每个用 `mygo.Bind` 绑定的方法，以及应用使用的插件；
- 调用 `window.mygo.window` 设置标题、最小化、最大化、全屏或关闭窗口；
- 无需权限处理器即可获取摄像头、麦克风、位置和通知；
- 读取每个自定义协议的响应，这些响应带有供它使用的 CORS 头；
- 获知拖放到它上面的文件的路径，以及其窗口文稿的路径。

因此，绝不要在受信任的页面中通过自定义协议提供第三方或用户的 HTML，也不要用未指定基准 URL 的 `LoadHTML` 加载这类内容，否则插件 UI 或用户 HTML 的预览也会获得上述全部能力。应改为使用不受信任的页面，或从窗口不信任的源加载，例如 `https` 站点或 `data:` URL。这些页面不能调用任何 Go 方法，也不能获得权限，除非[权限处理器](windows.md#权限)明确授权。

#### 不受信任的页面

`PageOptions.Untrusted` 将页面明确标记为不受信任，不受 URL 影响，适用于通过自定义协议展示插件 UI 的窗口或 [Web 视图](ui/web-view.md)。此时不信任任何源，`TrustedOrigins` 中的源也不例外，限制也比普通的不受信任源页面更严格：MyGo 默认允许普通页面执行的操作，只有经应用处理器批准后才允许该页面执行。

- 它对绑定方法和 `window.mygo.window` 的调用会被拒绝，它的拖动区域也不起作用。页面发布的消息超过 4 KiB 时，MyGo 会在解码前丢弃；其余的都在主线程上应答，不会为每条消息开一个 goroutine。
- `Broadcast` 不会到达它，对包含它的 Web 视图所在的窗口调用 `Emit`，也到不了它。用 `EmitPage` 向它发送。
- 没有 `SetPermissionHandler`，它就没有权限；没有 `SetWindowOpenHandler`，它打不开窗口，也打不开浏览器；没有 `OnWillDownload` 监听器，它什么也下载不了；没有 `OnWillNavigate` 监听器，除了自身页面的源，它去不了任何其他源。

它仍能像任何页面一样读取自己的协议，它的请求会到达处理器，处理器用 `mygo.CallerPage(r.Context())` 判断是哪个页面在请求。插件的 UI 就是这样与应用通信，应用则用 `EmitPage` 作答：

```go
mygo.Protocol.HandleFunc("plugin", func(w http.ResponseWriter, r *http.Request) {
	p := mygo.CallerPage(r.Context())
	plugin := pluginOf[p] // the plugin whose web view made the request
	if plugin == nil || r.URL.Host != plugin.ID {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodPost {
		msg, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
		...
		PluginReply.EmitPage(p, reply)
		return
	}
	http.FileServerFS(plugin.Files).ServeHTTP(w, r)
})

view, _ := win.NewWebView(mygo.WebViewOptions{
	URL:  "plugin://" + plugin.ID + "/",
	Page: mygo.PageOptions{Untrusted: true},
})
pluginOf[view.Page()] = plugin
```

```js
// In the plugin's page.
await fetch("/message", { method: "POST", body: JSON.stringify(msg) });
window.mygo.on("plugin-reply", (reply) => { ... });
```

页面也可以向其他主机和协议发出请求，但读不到它们的响应：在处理器对请求采取行动之前，要像上面那样检查页面。

有些东西会到达每个页面，无论受信任与否：

- 事件：`Broadcast` 发送到每个窗口的每个页面，`Emit` 发送到原生 UI 窗口的每个 Web 视图。向单个页面发送用 `EmitPage`。
- 发往 Go 的消息：来自不受信任源的页面调用会被拒绝，但应用仍会先接收并解码消息，不限制大小；只有显式标记为[不受信任的页面](#不受信任的页面)才会受到上述消息限制。
- `window.open` 和 `target="_blank"` 链接会在默认浏览器中打开 `http(s)` URL，除非 `SetWindowOpenHandler` 另行决定（[链接与新窗口](#链接与新窗口)）。
- 下载的文件会进入“下载”文件夹，除非 `OnWillDownload` 另行决定（[下载](windows.md#下载)）。
- 页面可以导航到任何地方，除非 `OnWillNavigate` 取消（[页面](windows.md#页面)）。导航到受信任 URL 的页面就是受信任的。

## mygo-runtime 包

MyGo 把它的运行时作为 `window.mygo` 注入每个页面。新项目依赖的 `mygo-runtime` npm 包为它提供类型和导入方式；生成的客户端就建立在它之上。

```ts
import { currentWindow, isMyGo, onFileDrop, runtime } from "mygo-runtime";

if (isMyGo() && runtime().platform === "darwin") {
  document.documentElement.classList.add("mac");
}

await currentWindow.toggleMaximize();
```

| 导出 | 作用 |
|---|---|
| `call(method, ...args)`, `on(event, listener)`, `once(event, listener)`, `event<T>(name)` | 无类型的调用和事件，见[绑定](bindings.md#不用生成的客户端) |
| `Channel` | 来自 Go 方法的值流，见[通道](bindings.md#通道) |
| `isCallError(err)` | 一次拒绝是否来自 Go 的 error |
| `isMyGo()` | 页面是否运行在 MyGo 窗口里，而不是开发服务器的浏览器标签页里 |
| `runtime()` | 运行时：`platform`（`"darwin"`、`"linux"` 或 `"win32"`）、`windowId`（Go 窗口的 `ID()`）和 `version`；在 MyGo 之外会抛出错误 |
| `currentWindow` | 页面的窗口：`minimize`、`maximize`、`unmaximize`、`toggleMaximize`、`isMaximized`、`toggleFullScreen`、`close` 和 `setTitle` |
| `onFileDrop(listener)` | 拖放到窗口上的文件及其路径 |
| `preferences()` | 桌面的强调色和辅助功能设置；在 MyGo 之外会抛出错误 |
| `onPreferences(listener)` | `preferences()` 的变化 |
| `windowActive()`, `onWindowActive(listener)` | 页面的窗口是否被绘制为活动窗口（即接收用户输入的窗口），以及它的变化，见[非活动窗口](#非活动窗口) |
| `locale()`, `direction()`, `onLocaleChange(listener)` | 应用显示的语言（`App.Locale`），例如 `"zh-Hans"`，其方向，`"ltr"` 或 `"rtl"`，以及它们的变化；页面从文档一开始就持有这些值，即 `mygo.locale` 和 `mygo.dir` |

`mygo-runtime/i18n` 格式化 ICU MessageFormat 的消息，正如 Go 中的 `i18n` 包那样。`mygo generate` 为应用的[消息目录](i18n.md#在页面中)生成的模块 `locales.ts` 就建立在它之上，其中包含每种语言的消息，由前端的构建过程打包：页面不必等待请求，就能用应用的语言显示首批文字。

在普通浏览器中打开时（便于调整布局），页面没有运行时：调用会被拒绝，`isMyGo()` 为 false。

## 自定义标题栏

无边框窗口没有标题栏，也没有边框，由页面自己绘制。用 CSS 属性 `--app-region: drag` 标记用来拖动窗口的元素：

```go
mygo.NewWindow(mygo.WindowOptions{URL: "/", Frameless: true})
```

```css
.titlebar {
  --app-region: drag;
  height: 40px;
}
.titlebar .search {
  --app-region: no-drag;
}
```

```ts
import { currentWindow } from "mygo-runtime";

minimizeButton.onclick = () => currentWindow.minimize();
maximizeButton.onclick = () => currentWindow.toggleMaximize();
closeButton.onclick = () => currentWindow.close();
```

该属性会被继承：`no-drag` 可以把区域中的部分重新排除在外，区域内的按钮、链接、输入框和其他交互元素仍然可以点击。双击区域的效果与双击标题栏相同。

无边框窗口仍可从边缘调整大小。在 Wayland 下的 Linux 上，GTK 会像 GNOME 的应用那样，自己装饰无边框窗口和隐藏了标题栏的窗口，但装饰中不含标题栏：它在页面周围绘制阴影和细边框，可以从紧贴页面外侧的位置调整窗口大小，并在窗口最大化或全屏时去掉它们。在 Linux 的其他地方，GTK 不会给没有装饰的窗口加边框（例如 X11，或设置了 `DisableShadow`、`Transparent` 或 `Panel` 的窗口），此时页面最外侧 5 px 充当边框：那里显示调整大小的光标，页面在那里收不到点击或指针移动。

在 Linux 上，无边框窗口还会保留 `App.SetMenu` 或 `Window.SetMenu` 给它的菜单栏，它位于页面之上，因而也位于自定义标题栏之上。给这样的窗口一个自己的空菜单 `win.SetMenu(mygo.NewMenu(nil))`，改从页面显示菜单（`Menu.PopupAt`，在页面调用的绑定方法中）。

### 隐藏标题栏

窗口可以保留窗口控件而只隐藏标题栏，让页面延伸到控件下面并绘制标题栏的其余部分：把 `TitleBarStyle` 设为 `mygo.TitleBarHidden`。控件留在窗口顶部：

- 在 macOS 上，是交通灯按钮。`mygo.TitleBarHiddenInset` 在它们周围留出更多空间，`TrafficLightPosition` 可以移动它们，其值为关闭按钮左上角在窗口中的位置。
- 在 Windows 上，右上角则是最小化、最大化或还原，以及关闭按钮，按照 Windows 11 的样式绘制。它们的高度填满标题栏，即 `TitleBarHeight`（默认 32），将鼠标悬停在最大化按钮上会打开贴靠布局。窗口仍可从上边缘调整大小。
- 在 Linux 上，是 GTK 自己的标题按钮，位置由桌面的按钮布局决定，并在 `TitleBarHeight` 中居中（默认是桌面标题栏的高度）。布局决定显示哪些按钮，可能一个也没有，平铺窗口管理器常常如此设置。自己装饰窗口的平铺式 Wayland 合成器（如 Hyprland 和 Sway）则没有按钮，合成器自己也不显示按钮。在 KDE Plasma 上，KWin 不会在页面上方加标题栏：按钮是 GTK 的，位置由 KDE 的按钮布局决定。页面最外侧的像素用于调整窗口大小，与无边框窗口一样。窗口的菜单栏会隐藏，如同 `AutoHideMenuBar`：在其菜单打开时，Alt 和 F10 会在页面上方显示它，其快捷键始终有效。`Theme.SetGTKCSS` 可以改变按钮的样式，例如改成 GTK 4 的圆形按钮（[GTK 的样式](windows.md#linux-上的-gtk-样式)）。

```go
mygo.NewWindow(mygo.WindowOptions{
	URL:            "/",
	TitleBarStyle:  mygo.TitleBarHidden,
	TitleBarHeight: 40,
})
```

从第一次绘制起，页面就从 `:root` 上的 CSS 变量得知控件占用的空间：

| 变量 | |
|---|---|
| `--mygo-titlebar-height` | 标题栏的高度 |
| `--mygo-titlebar-inset-left`, `--mygo-titlebar-inset-right` | 控件从左右边缘占用的空间 |
| `--mygo-titlebar-controls-bottom` | 控件从顶部边缘算起结束的位置：macOS 上交通灯按钮的底部，Linux 和 Windows 上标题栏的高度 |

在全屏时它们是 `0px`，此时控件隐藏（在 macOS 上，它们会随菜单栏一起回来）。页面的标题栏避开控件，并标记 `--app-region: drag`，以便拖动窗口：

```css
.titlebar {
  --app-region: drag;
  height: var(--mygo-titlebar-height, 40px);
  padding-left: var(--mygo-titlebar-inset-left, 0px);
  padding-right: var(--mygo-titlebar-inset-right, 0px);
}
```

延伸到窗口顶部、上面覆盖着交通灯按钮的侧边栏，其项目从按钮下方开始：

```css
.sidebar {
  padding-top: calc(var(--mygo-titlebar-controls-bottom, 0px) + 8px);
}
```

`Transparent: true` 让背景透明的页面透出桌面，`Vibrancy` 会在 macOS 以及 Windows 11 22H2 及更高版本上，在页面后面加一层模糊材质：

```go
mygo.NewWindow(mygo.WindowOptions{
	URL:           "/",
	TitleBarStyle: mygo.TitleBarHidden,
	Transparent:   true,
	Vibrancy:      mygo.VibrancySidebar, // Mica on Windows 11
})
```

完整的自定义标题栏见 `examples/frameless`。原生 UI 以同样的方式显示材质（[Vibrancy](ui/windows.md#材质)），正如 `examples/vibrancy` 用内嵌标题栏下方的半透明侧边栏所展示的那样。

## 拖放的文件

从 Finder 或 Explorer 拖到页面上的文件，到达页面时会带着路径，而 DOM 的 drop 事件不会提供路径：

```ts
import { onFileDrop } from "mygo-runtime";

onFileDrop(({ paths, x, y }) => {
  const target = document.elementFromPoint(x, y);
  // ...
});
```

Go 侧则用 `win.OnFileDrop`。页面自己的拖放仍然可用：它的 drop 监听器仍会收到 `File` 对象，而把文件拖放到页面不处理的地方，不会再让页面被该文件替换。

## 深色模式

页面通过 `prefers-color-scheme` 媒体查询跟随系统外观。在 CSS 中声明 `color-scheme: light dark`，让表单控件和滚动条也跟随，并给窗口设置与页面在两种外观下都匹配的 `BackgroundColor`，窗口会在页面绘制之前显示它：

```css
:root {
  color-scheme: light dark;
}
```

```go
mygo.NewWindow(mygo.WindowOptions{URL: "/", BackgroundColor: "light-dark(#f5f5f7, #1e1e1e)"})
```

`mygo.Theme.SetSource(mygo.ThemeDark)` 强制使用一种外观，页面通过同一个媒体查询看到它。

## 强调色与辅助功能

`prefers-reduced-motion` 和 `prefers-contrast` 会自动跟随桌面。桌面使用的强调色，以及它要求文本放大的倍数，是媒体查询无法告诉页面的：从第一次绘制起，它们就是 `:root` 上的 CSS 变量。

| 变量 | |
|---|---|
| `--mygo-accent` | 以 `#rrggbb` 表示的强调色；桌面没有强调色时未设置，因此 `var(--mygo-accent, AccentColor)` 会回退到桌面自己的颜色 |
| `--mygo-text-scale` | 文本放大的倍数；`1` 是通常大小，因此写作 `calc(1rem * var(--mygo-text-scale, 1))` |

```css
:root {
  --accent: var(--mygo-accent, AccentColor);
  font-size: calc(1rem * var(--mygo-text-scale, 1));
}
```

`mygo-runtime` 的 `preferences()` 返回这些设置，从文档开始加载时就能读取当前值；`onPreferences` 通知设置变化，这些变化只能由桌面设置触发。组件可以像使用 `matchMedia` 一样，读取前者并订阅后者：

```ts
import { onPreferences, preferences, type ThemePreferences } from "mygo-runtime";

const apply = ({ reduceTransparency }: ThemePreferences) =>
  document.documentElement.classList.toggle("opaque", reduceTransparency);
apply(preferences());
const off = onPreferences(apply);
```

| 字段 | |
|---|---|
| `accent` | 以 `"#rrggbb"` 表示的强调色，桌面没有时为 `null` |
| `reduceMotion` | 桌面要求更少的动效，如同 `prefers-reduced-motion` |
| `reduceTransparency` | 桌面要求不透明的绘制表面：macOS 的“降低透明度”，Windows 关闭透明效果；在 Linux 上为 false，因为它没有这样的设置 |
| `highContrast` | 桌面要求更高的对比度，如同 `prefers-contrast: more` |
| `textScale` | 文本应比通常大多少倍；`1` 为通常大小 |

导航后的页面从当时的取值开始：在文档开始时告知它们取值的脚本，会在取值变化时被替换。`mygo.Theme.Preferences()` 在 Go 侧读取它们，见[原生](native.md#强调色和辅助功能)。

## 非活动窗口

原生应用会用不同的外观区分接收用户输入的窗口和其他窗口：macOS 会把非活动窗口的侧边栏及其选中项的强调色淡化为灰色，Windows 把 Mica 绘制为纯色填充，GTK 用 `:backdrop` 设置样式。页面可通过 `<html>` 判断窗口状态：非活动窗口的 `<html>` 带有 `data-mygo-window-inactive` 属性：

```css
.sidebar .selected {
  background: var(--accent);
  color: white;
}
:root[data-mygo-window-inactive] .sidebar .selected {
  background: #0000001a;
  color: inherit;
}
```

窗口本身或其某个浮出窗口具有键盘焦点时，该窗口就是活动的：窗口的浮出窗口不会让窗口失去活动状态，而在同样的情况下，DOM 焦点会离开页面。面板、没有父窗口的浮出窗口，以及 `SetActiveAppearance` 始终保持活动的窗口，都是活动的。页面一开始是活动的，DOM 就绪后才会被告知相反的状态；`mygo-runtime` 的 `windowActive()` 读取状态，`onWindowActive` 会告知它的变化。透明页面后面的材质会自行跟随（macOS 上在面板和浮出窗口之外的窗口中，Windows 上是 Mica）。显示它的窗口见[原生 UI](ui/windows.md#活动与非活动窗口)。

## 调整自身窗口大小的页面

浮出窗口或带 `FitContent` 的窗口，由页面决定其大小，不需要 `ResizeObserver` 或自己的绑定方法：MyGo 的运行时会在窗口限制范围内，按内容宽度布局 `html`，窗口会随根节点大小变化而调整（见[窗口](windows.md#让浮出窗口适应其内容)）。让 `html` 和 `body` 采用内容的大小：在它们上设置 `height: 100%` 或 `100vh` 会让窗口保持当前大小，而 `html` 上的 `width` 会取代内容自身的宽度。超过最大宽度的文本会换行，超过最大高度的内容会滚动。

## 链接与新窗口

带 `target="_blank"` 的链接和 `window.open()` 会调用窗口的打开处理器。没有处理器时，`http(s)` URL 在默认浏览器中打开，其他一律拒绝；`win.Page().SetWindowOpenHandler` 可以另行决定：

```go
win.Page().SetWindowOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WindowOptions {
	if strings.HasPrefix(req.URL, "https://docs.example.com/") {
		return &mygo.WindowOptions{Width: 900, Height: 700} // a window of the app
	}
	go mygo.Shell.OpenExternal(req.URL) // the default browser
	return nil
})
```

`req.UserInitiated` 区分用户打开的窗口（点击链接，或点击、按键后由处理器调用 `window.open()`）与脚本自行打开的窗口，例如定时器弹出的窗口或在页面加载时打开的窗口。要拒绝这些窗口，例如嵌入页面里的广告：

```go
win.Page().SetWindowOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WindowOptions {
	if !req.UserInitiated {
		return nil // a pop-up
	}
	go mygo.Shell.OpenExternal(req.URL)
	return nil
})
```

在 Linux 上，WebKitGTK 会直接阻止非用户操作触发的 `window.open()`，因此只有用户操作触发的打开请求才会传给处理器。在 macOS 和 Linux 上，WebKit 会将应用通过 `Page.Eval` 执行的脚本视为用户操作。

`win.Page().OnWillNavigate` 可以取消页面自己的导航，例如让应用的窗口留在应用内：见[窗口](windows.md#页面)。

## 预加载脚本

`PageOptions.PreloadScript` 在窗口的每个页面中运行 JavaScript，且早于页面自己的脚本，此时 `window.mygo` 已存在。

它只在顶层框架中运行。`PreloadAllFrames` 让它也在页面的 iframe 中运行，无论 iframe 的源是什么，且在每个 iframe 启动时运行，浏览器需要这样才能看到从其他站点嵌入的播放器加载了什么。iframe 中没有 `window.mygo`：要把 iframe 发现的内容告诉页面，请发送到 `window.top`。

## 网页检查器

开发构建带有网页检查器：右键单击页面并选择“检查元素”（Windows 上为“检查”），或调用 `win.Page().OpenDevTools()`。生产构建没有，除非用 `mygo build -debug` 构建，或窗口的页面在 `WindowOptions.Page` 中设置 `DevTools: mygo.DevToolsEnabled`。

在 Windows 上，应用会像其他 WebView2 应用一样读取 `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS` 和 `WEBVIEW2_USER_DATA_FOLDER`。使用 `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--remote-debugging-port=9222` 时，另一个进程可以通过 DevTools 协议驱动它的页面，如同测试运行器、Playwright 的 `connectOverCDP` 或智能体所做的那样。这在每个构建中都有效，与 Microsoft 的加载器一样：启动应用的人设置它的环境。

在 Windows 上，WebView2 自带的按键随检查器一起提供：F5 和 Ctrl+R 重新加载页面（重新加载不会询问 [`OnWillNavigate`](windows.md#页面)），Ctrl+P 打印，Ctrl+F 查找，Ctrl+加号缩放，这些都只在检查器开启时有效，而 Ctrl+加号还只在允许用户缩放的页面中有效（[`ZoomControl`](windows.md#用户的缩放)）。没有它时，如同生产构建，页面就没有这些按键，与 macOS 和 Linux 一样，只有应用的菜单项接受这些按键，例如“视图”菜单的菜单项（`RoleViewMenu`）。
