# 插件

为网页提供功能的插件分为两部分：Go 服务，以及封装了服务调用的 JavaScript 包。应用通过 `mygo.Use` 启用 Go 部分，再通过 npm 引入 JavaScript 部分。

## 官方插件

- [Fetch](plugins/fetch.md)：一个从 Go 发起 HTTP 请求的 `fetch`，没有 CORS 限制，可带任意请求头，支持流式响应体和取消。
- [WebSocket](plugins/websocket.md)：一个由 Go 建立连接的 `WebSocket`，握手时可带请求头。
- [SQLite](plugins/sqlite.md)：本地数据库、参数化查询和原子事务，使用由 Zig 编译的 SQLite C 引擎，不需要 cgo。
- [文件监视](plugins/watch.md)：应用允许页面监视的目录中的文件变化。
- [更新器](plugins/updater.md)：一个仿照 Sparkle 的更新窗口，会检查更新并提示安装。它完全用 Go 实现，窗口是网页，对使用原生 UI 的应用则是原生 UI。
- [终端](plugins/terminal.md)：原生 UI 的终端，一个用 Ghostty 的终端模拟器运行 shell 或任意程序的视图。它完全用 Go 实现，带一个由 CLI 打包进应用的原生库。
- [玻璃](plugins/glass.md)：原生 UI 的 Liquid Glass，在每个平台上都像 macOS 绘制的那样，带 macOS 的滚动边缘和背景模糊。它完全用 Go 实现，带自己的着色器。
- [游戏](plugins/game.md)：普通应用功能之外的游戏功能，按需启用：以固定步长运行的游戏循环及其输入处理、摄像机，以及采用统一标准布局的游戏手柄。完全用 Go 实现。
- [Loading](plugins/loading.md)：用于原生 UI 的 loading.dev 29 种加载指示器，外观和动效与浏览器中一致。它完全用 Go 实现。
- [SF Symbols](plugins/sfsymbols.md)：macOS 上用于原生 UI 的 Apple SF Symbols，由系统绘制。它完全用 Go 实现。
- [自动化](plugins/automation.md)：一个套接字，另一个进程（端到端测试运行器或智能体）通过它驱动运行中的应用：应用的窗口、其原生 UI 中的元素、输入、菜单、页面中的脚本和截图。它完全用 Go 实现，可选启用，且不包含在发布构建中；`mygo-automation` 是它面向智能体的命令行客户端。
- [PermissionFlow](plugins/permissionflow.md)：检查和请求必须由用户授予的系统权限，例如辅助功能、屏幕录制和麦克风，并引导用户前往系统设置，将应用拖入权限列表，做法参照 PermissionFlow。完全用 Go 实现。
- [Render](plugins/render.md)：把原生 UI 做成视频，就像 Remotion 把页面做成视频：视图根据帧号构建每一帧，在 GPU 上绘制，用系统的编码器编码，并带有声音。完全用 Go 实现。

## 使用插件

`mygo.Use` 接收应用要用的插件，须在 `App.Run` 之前调用：

```go
import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/fetch"
	"github.com/egoist/mygo/plugins/websocket"
)

func main() {
	mygo.Use(fetch.Plugin, websocket.Plugin)
	// ...
}
```

前端则导入它们的包：

```sh
bun add @mygo-plugins/fetch @mygo-plugins/websocket
```

```ts
import { fetch } from "@mygo-plugins/fetch";
import { WebSocket } from "@mygo-plugins/websocket";

const res = await fetch("https://api.example.com/me", {
  headers: { authorization: `Bearer ${token}` },
});
const ws = new WebSocket("wss://api.example.com/live", [], {
  headers: { authorization: `Bearer ${token}` },
});
```

官方插件的 `Plugin` 变量带默认选项，其 `New` 函数接受选项，例如 fetch 和 WebSocket 插件的 `http.Client`，以及决定页面可以发起哪些请求的 `Allow` 函数。各插件的页面会逐一列出这些选项。

只使用插件的 JavaScript 包而不启用其 Go 部分，会报错并提示调用 `mygo.Use`。

## 编写插件

Go 部分用 `mygo.Plugin` 定义：包含名称、供页面调用方法的服务，以及可选的 `Setup`，在应用启用插件时运行：

```go
package clock

// Plugin gives pages the time of the Go side.
var Plugin = mygo.Plugin{Name: "clock", Service: service{}}

type service struct{}

func (service) Now() time.Time { return time.Now() }
```

该服务的绑定方式与 `mygo.Bind` 的服务相同，其方法遵循相同的规则（上下文、通道、错误），但以 `plugin:<name>` 的名称绑定，且不在生成的 TypeScript 客户端中。它的 JavaScript 包用 `mygo-runtime` 的 `call` 调用它；`mygo-runtime` 被列为对等依赖（peer dependency），以便使用应用自带的那一份（同时作为开发依赖，供它自己的构建和测试使用）：

```ts
import { call } from "mygo-runtime";

export const now = async () => new Date(await call<string>("plugin:clock.Now"));
```

只有应用受信任的页面可以调用插件，与所有绑定的方法一样。名称被占用，或服务的方法使用了无法传到 JavaScript 的类型时，`mygo.Use` 会 panic，让错误在启动时就暴露出来。
