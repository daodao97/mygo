# mygo CLI

`mygo` 创建、开发、打包并发布 MyGo 应用。

## 安装

有 Web 前端的项目把 CLI 作为 `mygo-cli` npm 包依赖。该包包含 macOS、Linux 和 Windows 上 arm64 与 x64 的预编译二进制文件，项目在自己的脚本中运行它：

```sh
bun add -d mygo-cli    # or: npm install -D mygo-cli
bun run dev            # "dev": "mygo dev" in package.json
bunx mygo doctor       # any command, in the project
```

在项目之外，运行 `bunx mygo-cli` 或 `npx mygo-cli`，例如 `bunx mygo-cli init my-app`：npm 上有一个无关的 `mygo` 包，所以在没装 mygo-cli 的地方，`bunx mygo` 会运行那个包。`npm install -g mygo-cli` 会把 `mygo` 放进你的 `PATH`。`MYGO_CLI_BINARY` 让该包运行 CLI 的另一个构建。

[原生 UI](ui/README.md) 项目没有 npm 包：它们的 `go.mod` 把 CLI 作为[工具](https://go.dev/doc/modules/managing-dependencies#tools)，`go tool mygo` 会按 `go.mod` 固定的版本运行它。

CLI 是一个 Go 程序，Go 也能直接运行它，无需安装：

```sh
go run github.com/egoist/mygo/cmd/mygo@latest init my-app
```

`go install` 会把 `mygo` 放进你的 `PATH`：

```sh
go install github.com/egoist/mygo/cmd/mygo@latest
```

无论用哪种方式，构建应用都需要 Go，因为应用由 Go 编译。

接受项目目录 `[dir]` 的命令默认使用当前目录。

## mygo init

```
mygo init [flags] <dir>
```

在新的或空的目录中创建项目：一个 Go 模块与一个用 Vite 构建的 TypeScript 前端并排放置，`resources/icon.png` 中的默认图标，以及带 `dev`、`build` 和 `generate` 脚本的 package.json。它用 Bun 安装依赖并生成 TypeScript 客户端。见 [Web 前端](getting-started.md#web-前端)。

使用 `-template native` 时，项目就是一个 Go 模块，其窗口显示[原生 UI](ui/README.md)：`main.go`、视图的测试、`mygo.json` 和图标。该模块把 CLI 作为[工具](https://go.dev/doc/modules/managing-dependencies#tools)，所以 `go tool mygo dev` 和 `go tool mygo build` 会运行它固定的 CLI 版本，不需要 Bun。见[原生 UI](getting-started.md#原生-ui)。

两个模板都在 `.agents/skills/mygo-maintenance/` 中包含 `mygo-maintenance` 智能体技能，讲解维护应用时的元素生命周期、稳定标识、线程和测试。

| 标志 | |
|---|---|
| `-template` | `web`，TypeScript 前端（默认），或 `native`，用 Go 编写的原生 UI |
| `-name` | 应用的名称（默认：目录的名称） |
| `-module` | Go 模块路径（默认：目录的名称） |
| `-mygo` | 要使用的 MyGo 检出目录，通过 `replace` 指令代替已发布的模块；脚本随后用 `go run` 运行该检出的 CLI；对于 Web 模板，先在检出目录中运行 `bun install && bun run build` |

## mygo install-skills

```sh
mygo install-skills [dir]
```

在现有项目的 `.agents/skills/` 中，安装或更新随你运行的 CLI 版本一起捆绑的智能体技能。目录默认为当前目录；不需要项目配置、Go 构建环境或前端工具。新项目也会通过 `mygo init` 获得这些技能。

该命令会覆盖捆绑的文件，例如 `mygo-maintenance/SKILL.md` 和 `mygo-maintenance/agents/openai.yaml`，这些文件的本地修改也会被覆盖。其他技能和额外的自定义文件会被保留。升级 CLI 后再次运行该命令，即可刷新这些指导内容。

## mygo dev

```
mygo dev [flags] [dir]
```

开发应用，支持实时重载。它写出 TypeScript 客户端，运行配置中的 `devCommand`（例如 Vite 开发服务器），等待 `devUrl` 响应，然后构建一个开发版应用并启动它；这个应用加载 `devUrl`，而不是构建好的前端。没有 `devUrl` 时，应用从磁盘提供 `frontendDist`。

对 Go 代码、配置、图标、资源或[消息目录](i18n.md)的更改会重新构建应用，重新生成 TypeScript 客户端和消息目录的函数，并重启应用：正在运行的构建先退出，新构建再启动，所以两者绝不会同时占用相同的文件、锁或 webview 配置。编译失败时，不会替换正在运行的应用。前端更改由开发服务器处理。退出应用或按 Ctrl+C 会结束 mygo dev（再按一次 Ctrl+C 可以不等应用退出），而 `App.Relaunch` 会用传给它的参数重启应用。

在终端中，每个步骤会显示一个旋转指示器，说明当前在做什么，例如正在编译的 Go 包，随后显示这一步的耗时；开发服务器和应用打印的内容分别标记为 `web │` 和 `app │`，重新构建会列出发生变化的文件。像在 Vite 中一样，按一个键再按 Enter：`r` 重新构建并重启应用，`c` 清空控制台，`q` 退出，`h` 列出这些按键。`NO_COLOR` 关闭颜色；输出到文件或管道时，每个步骤占一行。

开发版应用命名为 `<name> Dev`，标识符为 `<identifier>.dev`，使它的数据、偏好设置和单实例锁与已安装的应用分开。在 macOS 上，它是 `.mygo/dev` 中的真正应用包；在 Windows 上，它的可执行文件带有 `mygo build` 嵌入的图标、清单和版本信息。

在 macOS 上，mygo dev 用用户钥匙串中的 Apple Development 身份为开发版应用签名，就像 Xcode 为 Debug 构建签名一样：有多个身份时，使用 `macos.signingIdentity` 所属团队的那个。Xcode 可以在“设置”>“帐户”>“管理证书”中为任何 Apple ID 免费创建一个身份。这个签名在每次构建之间保持不变，所以 macOS 会保留用户给开发版应用的授权，例如辅助功能、屏幕录制或摄像头，以及通过 [`mygo.Secrets`](native.md#机密) 读取的钥匙串项目的“始终允许”。没有这样的身份时，mygo dev 会改用临时签名，并说明这一点：临时签名每次构建都会改变，macOS 会把每一个都当成不同的应用。`-sign` 可以指定另一个身份，使用描述文件时需要它；`-sign -` 则使用临时签名。

| 标志 | |
|---|---|
使用 `-web` 时，mygo dev 改为开发[浏览器中的原生 UI](ui/browsers.md)应用：它把应用编译成 WebAssembly，在 `http://localhost:5174`（或下一个空闲端口）上提供并打开它，并在 Go 代码变化时重新构建，每次重新构建都会重新加载页面。编译失败时，页面会继续运行上一次的构建，并在页面上显示失败原因，直到修复后编译成功。`o` 再次打开页面。

| 标志 | |
|---|---|
| `-skip-dev-command` | 不运行 `devCommand`，例如开发服务器已运行时 |
| `-sign` | 在 macOS 上为开发版应用签名的身份（默认：一个 Apple Development 身份，见下文，否则为 `-`，即临时签名） |
| `-web` | 在浏览器中开发应用 |
| `-port` | 使用 `-web` 时，供应用使用的端口，或下一个空闲端口（默认：5174） |
| `-open` | 使用 `-web` 时，在浏览器中打开应用（默认：true） |
| `-lang` | 以某种语言运行应用，例如 `de` 或 `zh-Hant`，或伪区域设置 `en-XA` 或 `ar-XB`，但不改变应用保存的语言；见[尝试一种语言](i18n.md#试用一种语言) |
| `-background` | 启动应用，并在每次重新构建后再次启动它，而不把它带到前台：它的窗口显示时不带焦点，直到你点击其中一个，这样你保存时编辑器仍保持焦点（Windows 上最大化的窗口仍会获得焦点） |

## mygo build

```
mygo build [flags] [dir]
```

构建生产应用：运行 `buildCommand`，编译应用并嵌入 `frontendDist`，然后为每个平台打包到 `<out>/<os>-<arch>`。见[构建与分发](distribution.md)。

平台 `web`（或 `js/wasm`）把面向浏览器的原生 UI 应用构建到 `<out>/web`，得到一个可在任何地方提供的静态站点：负责加载应用的 `index.html`、Go 的 `wasm_exec.js`、应用的 WebAssembly（按内容命名，因而可以永久缓存），以及图标。见[浏览器](ui/browsers.md)。

| 标志 | |
|---|---|
| `-platform` | 以逗号分隔的 `GOOS/GOARCH` 目标，例如 `darwin/universal,windows/amd64,linux/amd64`，或 `web`（默认：本机的） |
| `-debug` | 保留开发功能，例如 Web 检查器和[原生 UI 的检查器](ui/inspector.md) |
| `-o` | 输出目录（默认：配置中的 `out`） |
| `-sign` | macOS 签名身份（默认：`macos.signingIdentity`） |
| `-skip-build-command` | 不运行 `buildCommand` |
| `-skip-dmg` | 不制作 macOS 磁盘映像 |
| `-skip-notarize` | 不公证，即使设置了 `macos.notarize` |
| `-skip-installer` | 不制作 Windows 安装程序，也不为它们下载 NSIS（也不生成运行安装程序的 `install.ps1`） |
| `-upload` | 用 `gh` 把安装程序和更新上传到该版本的 GitHub 草稿发布版本（`updates.github`），或上传到 `updates.s3` 的存储桶 |

## mygo generate

```
mygo generate [-o file] [dir]
```

为用 `mygo.Bind` 绑定的服务和用 `mygo.NewEvent` 声明的事件写出类型安全的 TypeScript 客户端，写到配置中的 `bindings`，除非 `-o` 指定另一个文件。`mygo dev` 和 `mygo build` 会为你运行它；没有前端的应用不会得到客户端。见[生成的客户端](bindings.md#生成的客户端)。

配置中有 `i18n` 时，它先检查应用的[消息目录](i18n.md)，再生成它们对应的函数：消息目录所在目录中的 `catalog_gen.go`，以及客户端旁边以该目录命名的 TypeScript 模块（`src/locales.ts`）。消息目录中的错误会让它停止。

## mygo i18n check

```
mygo i18n check [dir]
```

检查应用的[消息目录](i18n.md#检查消息目录)：它列出消息目录的错误，例如无法解析的消息或翻译中添加的参数，以及警告，例如缺少翻译，并在有错误时以状态 1 退出。

## mygo native

与原生模块一样是实验性的：这些命令及其标志可能在任何发布版本中改变。这些命令用于[原生模块](native-modules.md)——用 Swift、Kotlin、Rust 或 Zig 实现的 Go 接口，按平台构建成一个库（在手机上随应用构建）——以及包用 `ffi` 绑定的 C 库（[自己绑定一个 C 库](native-modules.md#自己绑定-c-库)）。`[module dir]` 默认为当前目录。

### mygo native build

```
mygo native build [flags] [dir]
```

构建 `dir` 中的[原生模块](native-modules.md)，每个模块用各平台对应的语言构建：模块有 Swift 实现时，在 macOS 上用 Swift，否则用 Rust，再不行用 Zig。也构建 `dir` 中的包在各自 `mygo-plugin.json` 中命名的库，用 swift、cargo 或 zig 从源代码为每个平台构建。要发布的文件写到该模块的 `.mygo/native/release`，以文件的 SHA-256 命名；它们的名称、URL 和 SHA-256 写入 `mygo-plugin.json`。Swift 在 Mac 上以通用架构构建，打包成 `.tar.gz`，内含它引入的框架（`darwin-universal`）。无法在这里构建的平台（在非 Mac 机器上构建 Rust 和 Swift 的 macOS 库）列在末尾，并保持原样。标志放在目录之前。`mygo dev` 和 `mygo build` 会自行构建应用自己的各个包中的模块和库；这个命令则把某个包的模块和库发布给其他人的应用，以及没有这些语言工具的机器。

| 标志 | |
|---|---|
| `-target` | 平台，以逗号分隔的 `GOOS-GOARCH`，`all`（默认）或 `host`；`host` 构建本机的库，供 `go run` 和 `go test` 使用，并不改动 `mygo-plugin.json` |
| `-lang` | 只构建那些用该语言（`swift`、`rust` 或 `zig`）实现的模块，而不是按各平台的语言构建：与 `-target host` 一起使用，可以测试每个实现 |
| `-release` | 文件发布到的目录 URL，如 `https://github.com/<owner>/<repo>/releases/download/<tag>`（默认：库的 `release`；配合 `-upload` 时为 git 检出所在仓库里该库的 `native-<name>` 发布版本） |
| `-out` | 要发布的文件的目录 |
| `-upload` | 用 GitHub CLI 把文件上传到 `-release` 指定的 GitHub 发布版本，需要时创建它 |
| `-force` | 连那些已发布文件就是用当前源码构建出的平台也会重新构建，否则这些平台会被跳过 |

### mygo native new

```
mygo native new [-lang <languages>] <module name>
```

在 `modules/<module name>` 中创建模块，或者在给出路径而不是名称时，在该路径创建：包含接口的 Go 包、为它生成的 Go 代码、一个 `AGENTS.md`，以及每种语言一个目录。`-lang` 接受以逗号分隔的语言：`swift`（默认）、`kotlin`、`rust`、`zig`，或 `none`，表示只有 Go 部分。

### mygo native add

```
mygo native add <language> [module dir]
```

为现有模块添加另一种语言（`swift`、`kotlin`、`rust` 或 `zig`）的实现，生成每个入口点的工厂函数和返回“不支持”错误的方法，使模块立即可以构建。

### mygo native check

```
mygo native check [-lang <language>] [module dir]
```

生成模块，为本机构建它的实现，或用 `-lang` 指定的实现，并对 Go 包运行 vet。它把每个问题列为 `file:line: message`，并说明要改什么，有问题时以状态 1 退出。

### mygo native call

```
mygo native call [-timeout <duration>] [module dir] <Interface>.<Method> [<arguments>]
```

创建入口点的实现，并用 `<arguments>` 调用其中一个方法；参数以 JSON 对象传入，键为参数的 Go 名称。结果以 JSON 输出，流中的每个值占一行。命令会为方法创建上下文，用 `-timeout` 限制超时，并打印回调调用记录。

`mygo dev` 和 `mygo build` 生成并构建应用的每个原生模块；见[构建、运行和测试](native-modules.md#构建运行和测试)。

## mygo keygen

```
mygo keygen [flags]
```

创建签署[更新](updates.md)的密钥对：私钥 `mygo-update.key` 和公钥 `mygo-update.pub`，并打印要加入配置的 `updates` 部分。

| 标志 | |
|---|---|
| `-o` | 写入密钥的目录（默认：用户配置目录中的 `mygo/update-keys`） |
| `-force` | 替换现有的密钥 |

## mygo doctor

检查开发机器：Go、Bun 和平台的 webview，有 Web 前端的项目需要它们（Linux 上的这类项目还需要 GTK 3；`linux.webview` 为 `false` 的项目则不需要 WebKitGTK）；还有可选功能的工具：用于 Windows 安装程序的 NSIS（`mygo build` 在 Windows 上下载它）、用于为 Windows 应用签名的 `signtool` 或 `osslsigncode`、用于 `-upload` 的 `gh`，以及用于为 macOS 应用签名的 Developer ID 身份。

## mygo version

打印 CLI 的版本。

## 环境变量

| 变量 | |
|---|---|
| `CGO_ENABLED` | 未设置或为空时默认为 `0`；对于需要 cgo 的应用依赖，设为 `1`，包括在 `mygo dev`、`mygo build` 和 `mygo generate` 中 |
| `MYGO_LDFLAGS` | `mygo build` 的构建所用的额外 `-ldflags`，加在它自己使用的 `-ldflags` 之后，例如 `-X example.com/app/internal/crash.DSN=…` 或 `-buildid=`；按引号外的空格拆分，和 `go build -ldflags` 的拆分方式一样 |
| `GOFLAGS` | Go 的参数；`mygo build` 会把其中 `-tags=…` 的标签加到它自己的标签里，例如用 `-tags=mygo_nowebview` 构建[不含 WebView2](ui/windows.md#没有-webview) 的 Windows 应用 |
| `MYGO_INSPECTOR` | `1` 让 `mygo build` 的生产构建保留[原生 UI 的检查器](ui/inspector.md)，默认的生产构建不含它 |
| `MYGO_UPDATER_PRIVATE_KEY` | 用于为更新签名的私钥，供 `mygo build` 使用 |
| `MYGO_WINDOWS_CERTIFICATE_PASSWORD` | `windows.certificate` 的密码，供 `mygo build` 使用 |
| `MYGO_CLI_BINARY` | 供 `mygo-cli` 包运行的 CLI 构建 |
| `MYGO_ENV` | `production` 让应用表现得像生产构建，例如不带 Web 检查器 |
| `MYGO_NATIVE_<NAME>` | `ffi` 包加载的[原生库](native-modules.md#库的查找位置) `<name>`（大写，字母和数字之外的字符用 `_`）的文件，优先于其他文件 |
| `MYGO_LOCALE` | 应用运行时使用的语言，会覆盖它保存的语言，由 `mygo dev -lang` 设置 |

对于依赖需要 cgo 的应用：

```sh
CGO_ENABLED=1 go tool mygo dev
CGO_ENABLED=1 go tool mygo build
```
