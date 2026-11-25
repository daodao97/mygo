# 构建与分发

`mygo build` 把项目变成用户可以安装的应用：macOS 上的应用包和磁盘映像，Windows 上的可执行文件和安装程序，Linux 上的可执行文件、桌面条目、带安装脚本的归档、Debian 和 RPM 包，以及 Arch Linux 的 PKGBUILD。

```sh
bun run build        # mygo build, in a project with a web frontend
go tool mygo build   # in a project of native UI
```

它会：

1. 有 Web 前端时，写出 TypeScript 客户端，让前端基于 Go 代码构建，并运行配置里的 `buildCommand`，由它把前端构建到 `frontendDist`；
2. 编译应用的生产版本，有前端时把前端嵌入其中：不带 Web 检查器，并把配置里的名称、标识符和版本链接进去；
3. 为每个平台打包到 `build/<os>-<arch>/`（配置里的 `out`）；
4. 在支持签名的平台上为产物签名；配置了[更新](updates.md)时，还会生成已签名的更新归档，以及基于已发布版本的增量更新。

## 平台

`-platform` 列出 `GOOS/GOARCH` 目标，默认是本机：

```sh
mygo build -platform darwin/universal,windows/amd64,windows/arm64,linux/amd64,linux/arm64
```

`darwin/universal` 把 arm64 与 amd64 合到一个应用里。MyGo 不需要 cgo，所以任何机器都能为每个平台编译。有些步骤需要目标平台的工具，在其他系统上会跳过并给出提示：macOS 应用只在 macOS 上签名并放进磁盘映像；Windows 安装程序在其他系统上则只在安装了 [NSIS](#安装程序) 时制作。

CLI 默认 `CGO_ENABLED=0`。如果应用的依赖需要 cgo，在环境中为 `mygo dev`、`mygo build` 和 `mygo generate` 设置 `CGO_ENABLED=1`。这些构建需要目标平台的 C 编译器；交叉编译时，把 `CC` 设为合适的交叉编译器。

| 平台 | `build/<os>-<arch>/` 中的内容 |
|---|---|
| macOS | `My App.app`，已签名，以及 `my-app-0.1.0-arm64.dmg`（`darwin/universal` 为 `my-app-0.1.0.dmg`），配置了[更新](updates.md)时还有 `install.sh` |
| Windows | `My App.exe`、应用的文件，以及安装程序 `my-app-setup-0.1.0-amd64.exe`，配置了[更新](updates.md)时还有 [`install.ps1`](#macos-与-windows-的安装脚本) |
| Linux | `my-app`、`my-app.desktop`、`my-app.png`、应用定义的文件类型对应的 `my-app.xml`、应用的文件、它们的归档 `my-app-0.1.0-linux-amd64.tar.gz`（含 [`install.sh`](#安装脚本)），以及 [`linux.formats`](#linux) 选出的包，默认三种全有：Debian 包 `my-app_0.1.0_amd64.deb`、RPM 包 `my-app-0.1.0-1.x86_64.rpm`，以及一个 [`PKGBUILD`](#arch-linux) 连同它安装的图标 `my-app-256x256.png` 与 `my-app-512x512.png` |

其他标志：`-debug` 保留检查器等开发功能，`-skip-dmg`、`-skip-notarize` 和 `-skip-installer`（Windows 安装程序）跳过相应步骤，`-sign` 覆盖 macOS 的签名身份，`-o` 改为指定的输出目录。见 [CLI](cli.md#mygo-build)。

## 名称、图标与版本

配置描述应用：

```ts
export default defineConfig({
  name: "My App",
  identifier: "com.example.myapp",
  version: "1.2.0",
  copyright: "© 2026 Example Inc.",
});
```

- `name` 是用户看到的名字：应用包、可执行文件、菜单和安装程序都用它。
- `identifier` 是应用独有的反向 DNS 名称。系统用它来记录偏好设置、权限和注册信息：发布后就不要再改。
- `version` 是应用的版本，`App.Version()` 会返回它，[更新](updates.md)会比较它。

图标是 `resources/icon.png`，或配置里的 `icon`：一个正方形 PNG，最好是 1024×1024。`mygo build` 把它做成 macOS 的 `.icns`、Windows 可执行文件的图标资源，以及 Linux 桌面条目的图标。在同一目录下，同名但带 `-dev` 的文件（`resources/icon-dev.png`）如果存在，就是 `mygo dev` 构建的开发版应用的图标，用来在 Dock 或任务栏中与正式应用区分开；它不会随应用发布。

在 macOS 上，`macos.icon` 会添加一个资源目录，macOS 会优先使用它而不是 `.icns`：一个是 `actool` 编译出的 `Assets.car`，其中应用图标名为 `AppIcon`（或 `macos.infoPlist` 设置的 `CFBundleIconName`）；另一个是 Icon Composer `.icon`，即 macOS 26 的分层图标，由 `mygo build` 用 Xcode 26 或更高版本的 `actool` 编译，并用 `actool` 从中渲染出的 `.icns` 替代 PNG 生成的那份。在其他系统上构建，或者编译失败时，构建会跳过 `.icon` 并给出提示，应用仍使用 PNG 生成的 `.icns`。`mygo dev` 显示 `.icns`。

`macos.icon` 也可以指定自备的 `.icns`，替代从 PNG 生成的图标。适合为不同尺寸单独绘制图标的情况，例如在 16 点和 32 点尺寸下采用更紧凑的裁剪，再用 `iconutil` 从 `.iconset` 生成图标文件。`mygo dev` 仍使用 PNG 生成的图标。

```ts
export default defineConfig({
  macos: { icon: "AppIcon.icon" }, // outside resources, which ship as they are
});
```

在没有 Xcode 的构建中（例如 Linux 上的 CI），把 `actool` 从 `.icon` 编译出的 `Assets.car` 提交到仓库，并把它设为 `macos.icon`。`.icon` 必须命名为 `AppIcon.icon`：`actool` 按文件名给图标命名，`--app-icon` 不一致时不会编译任何内容。

```sh
mkdir -p icon
xcrun actool AppIcon.icon --compile icon --platform macosx --target-device mac \
  --minimum-deployment-target 12.0 --app-icon AppIcon \
  --output-partial-info-plist icon/partial.plist
```

这会写出 `icon/Assets.car`，用于 `macos: { icon: "icon/Assets.car" }`。

## 资源

应用运行时需要读取的文件应随应用一起发布，例如数据库初始数据、辅助程序或托盘图标。项目 `resources` 目录中的内容都会复制进应用，但 Go 源文件和 [`resourcesExclude`](#排除文件) 匹配的内容除外；配置中 `resources` 列出的文件和目录也会复制进去，并保留各自的文件名或目录名：

```ts
export default defineConfig({
  resources: ["third_party/licenses", "bin/helper"],
});
```

运行时，它们位于 `App.Path(mygo.PathResources)`：macOS 应用中是 `Contents/Resources`，其他地方是可执行文件所在目录，`go run` 和 `go test` 下是项目的 `resources` 目录：

```go
dir, _ := mygo.App.Path(mygo.PathResources)
seed := filepath.Join(dir, "seed.db")
```

`mygo dev` 也会把它们复制进开发版应用，并在它们变化时重新构建。资源目录里的点文件会随应用发布：有些包需要读取这类名称，例如 dsh 的提供商目录会读取 `dist/providers/data/.manifest.json`。隐藏目录（`.git`、`.cache`）会被排除，`.DS_Store`、`Thumbs.db`、`desktop.ini` 也不会带上；`resources` 顶层以点开头的条目同样会被排除，因为它们属于仓库，而不是应用。你不想发布的点文件，例如资源旁边的 `.env`，可以用 [`resourcesExclude`](#排除文件) 排除。

### 排除文件

`resources` 中的 Go 源文件不会进入应用：构建过程会读取它们，例如某个包用 `//go:embed` 嵌入旁边的图片，而应用永远不会读取。其他只有项目自己需要的文件，比如设计文件，则用 `resourcesExclude` 排除，它是这些文件在 `resources` 中路径的匹配模式：

```ts
export default defineConfig({
  resourcesExclude: ["**/*.psd", "design"],
});
```

模式的每一段（斜杠之间）按 Go 的 `path.Match` 匹配（`*`、`?`、`[a-z]`），`**` 匹配任意数量的目录，零个也算：`**/*.psd` 排除所有 `.psd` 文件，`*.psd` 只排除 `resources` 顶层的那些。匹配的目录会连同其中的内容一起排除，所以 `design` 和 `design/**` 都会排除 `resources/design`。[平台目录](#平台资源)中的路径以目录名开头，例如 `linux-amd64/bin/server.pdb`，`**/*.pdb` 也会匹配它。构建和 `mygo dev` 会排除相同的文件。`resources` 中列出的文件和目录会完整发布，包括 Go 源文件。

### 平台资源

有些资源属于某个平台，例如为某个系统和处理器构建的程序。`resources` 中以平台命名的目录存放它们，布局与 `resources` 本身相同：

```
resources/
├── data/seed.db                    every app
├── darwin/                         macOS apps
├── darwin-arm64/bin/server         macOS on Apple silicon
├── darwin-amd64/bin/server         macOS on Intel
├── linux-amd64/bin/server          Linux on x86-64
├── linux-arm64/bin/server          Linux on ARM64
└── windows-amd64/bin/server.exe    Windows on x86-64
```

目录名采用与 `-platform` 目标相同的命名：只写系统（`darwin`、`linux` 或 `windows`）时适用于该系统的所有架构；同时指定系统和架构（如 `darwin-arm64` 或 `windows-amd64`）时只适用于对应目标。构建会将当前目标的平台资源与通用资源合并，所以上例的 Linux x86-64 应用包含 `data/seed.db` 和对应的 `bin/server`，不包含其他平台的资源。如果两个目录的文件会安装到同一路径，构建就会失败；使用其他工具的平台命名（如 `darwin-x64` 或 `macos`）也会报错，避免这些目录被误当作通用资源带入所有平台。如果同一系统下已有其他架构的目录，构建还会提示缺少专用目录的目标，例如已有 `linux-amd64` 时提示 `linux/arm64`。

通用 macOS 应用（`darwin/universal`）会带上 `darwin` 和 `darwin-universal`，并合并 `darwin-arm64` 与 `darwin-amd64`：一边的每个文件都必须在另一边有对应文件，程序和库会变成通用二进制文件（如 `lipo` 生成的那样），其他文件必须完全相同。两种架构都要用的内容，例如已经是通用二进制文件的程序，放在 `darwin` 里。

`mygo dev` 只带上它所在机器的平台目录。在 `go run` 和 `go test` 下，`PathResources` 就是 `resources` 目录本身，包括平台目录。

### 辅助可执行文件

应用运行的程序也是资源，例如用其他语言写的服务器，或 `ffmpeg` 这样的工具：把每个平台的构建产物放进它的[平台目录](#平台资源)，并从那里运行。

```go
dir, _ := mygo.App.Path(mygo.PathResources)
cmd := exec.Command(filepath.Join(dir, "bin", "server"))
```

同一份代码在 Windows 上运行的是 `bin\server.exe`：在那里，`exec.Command` 用不带扩展名的绝对路径也能找到程序。

`mygo build` 会随应用一起为它们签名，好让 Gatekeeper 和 SmartScreen 接受它们，公证也能通过：

- 在 macOS 上，资源中的每个 Mach-O 可执行文件和库，以及包含它们的每个应用、框架或插件包，都会用 `signingIdentity` 由内向外签名；使用 Developer ID 时还会启用强化运行时并加上时间戳。默认的 ad hoc 签名只给还没有签名的代码签名：Apple silicon 不运行未签名的代码。
- 在 Windows 上，配置了 `certificate` 或 `signCommand` 时，资源中没有签名的每个可执行文件和 DLL 都会被签名。由发行者签过名的文件保留原签名。

在强化运行时下，有些程序需要授权，例如在运行时编译代码的 JavaScript 运行时需要 `com.apple.security.cs.allow-jit`。代码会保留签名时的授权，所以这类运行时的官方构建可以直接使用；`macos.helperEntitlements` 按代码在应用资源中的路径，为它单独指定授权，例如用 `bin/server` 对应 `resources/darwin-arm64/bin/server`：

```ts
export default defineConfig({
  macos: {
    signingIdentity: "Developer ID Application: Jane Doe (TEAMID)",
    helperEntitlements: { "bin/server": "server.entitlements.plist" },
  },
});
```

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>com.apple.security.cs.allow-jit</key>
  <true/>
</dict>
</plist>
```

### 包的原生库

用 purego 加载原生库的 Go 包（如[终端插件](plugins/terminal.md)加载 libghostty-vt）会在源码旁的 `mygo-plugin.json` 中声明它的各个构建：每个平台一个文件，写明从哪里下载以及它的 SHA-256：

```json
{
  "libraries": [
    {
      "name": "libexample",
      "files": {
        "darwin-arm64": { "name": "libexample.dylib", "url": "https://…/libexample-darwin-arm64.dylib", "sha256": "…" },
        "linux-amd64": { "name": "libexample.so", "url": "https://…/libexample-linux-amd64.so", "sha256": "…" },
        "windows-amd64": { "name": "example.dll", "url": "https://…/example-windows-amd64.dll", "sha256": "…" }
      }
    }
  ]
}
```

`mygo build` 和 `mygo dev` 会在构建应用所用的包中找到这些文件，把每个库下载一次到用户的缓存（`<cache>/mygo/natives/<sha256>/`），校验它的 SHA-256，再以它的名称安装到应用资源的顶层，并和其他代码一样签名；通用 macOS 应用会在一个文件中得到两种架构。包从 `PathResources` 加载它。为包没有对应文件的平台构建会失败。

## macOS

`mygo build` 会生成应用包，为它签名，再放进一个磁盘映像；磁盘映像的窗口会提示用户把应用拖到“应用程序”文件夹。

### 签名与公证

默认使用 ad hoc 签名，应用能在构建它的 Mac 上运行，但会在其他 Mac 上被 Gatekeeper 拦截。要分发应用，应使用 [Apple Developer Program](https://developer.apple.com/programs/) 提供的 Developer ID 签名，并提交 Apple 公证：

```ts
export default defineConfig({
  macos: {
    signingIdentity: "Developer ID Application: Jane Doe (TEAMID)",
    notarize: { keychainProfile: "notary" },
  },
});
```

把公证服务的凭据一次性存入钥匙串：

```sh
xcrun notarytool store-credentials notary
```

随后 `mygo build` 会在签名时启用强化运行时，先将应用提交给公证服务并把票据附加到应用上，再对装有该应用的磁盘映像做同样的事；更新也会包含附加了票据的应用。`security find-identity -v -p codesigning` 可列出签名身份，`mygo doctor` 会统计 Developer ID 的数量。

`codesign` 缺少什么，就在这台 Mac 的屏幕上弹出对话框来要：已锁定钥匙串的密码，或者使用该身份密钥的许可。它会一直等到有人回答，所以没人看着的构建——锁屏之后、通过 SSH 或在 CI 中——就停在那里。15 秒之后，`mygo build` 和 `mygo dev` 会提示 `codesign` 可能正在等待。在没人能回答的地方，先在构建之前解锁钥匙串，并允许 `codesign` 使用密钥，就像 [GitHub Actions 的工作流](github-actions.md)那样：

```sh
security unlock-keychain ~/Library/Keychains/login.keychain-db
security set-key-partition-list -S apple-tool:,apple: -s ~/Library/Keychains/login.keychain-db
```

### Info.plist 与授权

`macos.infoPlist` 向应用的 `Info.plist` 添加键，或替换 MyGo 的 `Info.plist`，例如使用摄像头或麦克风的页面所需的用途说明：

```ts
export default defineConfig({
  macos: {
    minimumSystemVersion: "13.0",
    infoPlist: {
      NSCameraUsageDescription: "Scan documents with the camera.",
      LSApplicationCategoryType: "public.app-category.productivity",
    },
    entitlements: "entitlements.plist",
    dmgTitle: "My App Installer",
  },
});
```

`entitlements` 指定一份授权属性列表，用于为应用签名；`helperEntitlements` 则为[辅助可执行文件](#辅助可执行文件)签名。`minimumSystemVersion` 是应用支持的最旧 macOS，默认 12.0。

### 描述文件

受限授权（如通信通知）需要一份授权它们的描述文件，用 Developer ID 分发也不例外。把 `macos.provisioningProfile` 设为它的路径，可以是相对于项目的路径，也可以是绝对路径。`build` 和 `dev` 在签名前把它复制到应用包的 `Contents/embedded.provisionprofile`，所以签名后的应用、磁盘映像和更新归档都会包含它。

```ts
export default defineConfig({
  macos: {
    signingIdentity: process.env.MACOS_SIGNING_IDENTITY,
    provisioningProfile: process.env.MACOS_PROVISIONING_PROFILE,
    entitlements: "entitlements.plist",
  },
});
```

描述文件必须与签名证书、包标识符和授权匹配，包括应用标识符和团队标识符。MyGo 按提供的内容嵌入它；它不创建描述文件，也不从描述文件中推导授权。Apple 在应用运行时验证它们的授权。描述文件不能与 ad hoc 签名（`-sign -`）一起使用。`dev` 默认用 Apple Development 身份签名，没有这样的身份时用 ad hoc；如果给了 `-sign`，则用 `-sign` 指定的身份（它不使用 `macos.signingIdentity`）。有描述文件但没有 `-sign` 时，它也用 ad hoc。它构建的应用的标识符是 `<identifier>.dev`，而应用的描述文件并不授权这个标识符：用[计算配置](configuration.md#计算出的配置)把描述文件和受限授权排除在开发构建之外，

```ts
export default defineConfig(({ command }) => ({
  macos: {
    signingIdentity: process.env.MACOS_SIGNING_IDENTITY,
    ...(command === "build" && {
      provisioningProfile: process.env.MACOS_PROVISIONING_PROFILE,
      entitlements: "entitlements.plist",
    }),
  },
}));
```

或传入 `-sign` 并为 `dev` 提供 `<identifier>.dev` 的描述文件。它会监视描述文件，在变化时重建。

把描述文件排除在版本控制之外，通过构建环境提供它们的路径。见 Apple 的 [Inside code signing: provisioning profiles](https://developer.apple.com/documentation/technotes/tn3125-inside-code-signing-provisioning-profiles)。

## Windows

可执行文件包含图标、版本信息和应用清单（启用逐显示器 DPI 感知及现代控件），运行时不显示控制台窗口。可以在主包中放入自己的 `.syso` 文件来替换这些内容。

### 安装程序

`mygo build` 还会生成 `my-app-setup-1.2.0-amd64.exe`，名称取自应用的 [`slug`](configuration.md#应用)，并像磁盘映像一样包含架构，方便将不同架构的安装程序上传到同一个发布版本。它将应用安装到当前用户的 `%LOCALAPPDATA%\Programs\My App`，无需管理员权限，因此应用可以[自我更新](updates.md)；还会添加开始菜单快捷方式和“设置”中的卸载项，并注册应用的 URL 协议和文件关联。完成页提供运行应用和创建桌面快捷方式的选项，后者默认勾选；静默安装（`/S`）不创建桌面快捷方式。

如果用户选择的文件夹里有其他文件，例如 `D:\Tools`，安装程序会为应用新建一个文件夹 `D:\Tools\My App`，因为卸载程序会移除应用的文件夹及其中的所有内容。

快捷方式、“设置 > 应用”条目和安装程序标题默认使用应用的 `name`。如果希望可执行文件和文件夹保留简单名称，可以用 [`windows.displayName`](configuration.md#windows) 单独指定显示名称。例如，应用显示为 `天气助手`，可执行文件仍为 `%LOCALAPPDATA%\Programs\weather` 中的 `weather.exe`：

```ts
export default defineConfig({
  name: "weather",
  windows: { displayName: "天气助手" },
});
```

文件名不能包含的字符（如 `:` 和 `?`）只在快捷方式的名称中变成 `-`。设置了 `displayName` 的版本，其安装程序会替换早期版本用应用名称创建的快捷方式。

[`windows.installDirName`](configuration.md#windows) 只命名安装文件夹，适用于在其他地方都保留自己名称的应用，例如为只认 ASCII 路径的工具（如控制台代码页里的脚本）使用一个 ASCII 文件夹。这里应用安装到 `%LOCALAPPDATA%\Programs\ZhiLian`，而它的可执行文件、快捷方式以及“设置 > 应用”中的条目名为 `智链`：

```ts
export default defineConfig({
  name: "智链",
  windows: { installDirName: "ZhiLian" },
});
```

已安装的应用会留在原来的文件夹里：新版本的安装程序会装进已安装版本的文件夹（这个文件夹可能是用户自己选的），[`install.ps1`](#macos-与-windows-的安装脚本)和[更新](updates.md)也是如此。新的 `installDirName` 只适用于新安装，以及卸载并重新安装后的应用。

安装程序用 [NSIS](https://nsis.sourceforge.io) 制作。在 Windows 上，`mygo build` 如果发现已安装 NSIS 就直接使用，否则在第一次需要时下载：官方 NSIS 3.13 的 zip，取自 MyGo 在 GitHub releases 上的副本，没有则取自 SourceForge，按 SHA-256 校验后存放在 `%LOCALAPPDATA%\mygo`。在 macOS 和 Linux 上，安装 NSIS 才能制作 Windows 安装程序（`brew install makensis`、`apt install nsis`）；没有它时，`mygo build` 跳过安装程序。

`mygo build -skip-installer` 不生成安装程序，也不查找或下载 NSIS：构建结果里有应用的文件，由你决定如何发布；配置了[更新](updates.md)时还有更新归档和清单，之前安装的应用会继续从中更新。它也不写 `install.ps1`，该脚本负责运行安装程序，所以一行安装需要一个同时包含两者的发布版本。

### 安装程序钩子

`windows.installerInclude` 向安装程序添加应用自己的步骤：一个相对于项目目录的 NSIS 脚本，安装程序会在 MUI2.nsh 及其语言之后包含它，就像 electron-builder 的 `include`。它定义的宏中，`customInstall` 在安装结束时运行，此时应用的文件已在 `$INSTDIR` 中；`customUnInstall` 在卸载时运行，在卸载程序移除 `$INSTDIR` 之前；`customInit` 在 `.onInit` 中运行，也就是安装程序启动时，例如在安装任何东西之前检查应用需要什么。脚本没有定义的宏会被跳过；脚本不存在时，`mygo build` 会停止。

```ts
export default defineConfig({
  windows: { installerInclude: "build/installer.nsh" },
});
```

下面的脚本将随应用发布的[辅助可执行文件](#辅助可执行文件)（项目中为 `resources/windows/bin/helper.exe`，安装后为 `$INSTDIR\bin\helper.exe`）复制到用户数据目录，并在卸载时删除它：

```nsis
; build/installer.nsh
!macro customInstall
  CreateDirectory "$LOCALAPPDATA\My App\bin"
  CopyFiles /SILENT "$INSTDIR\bin\helper.exe" "$LOCALAPPDATA\My App\bin"
!macroend

!macro customUnInstall
  Delete "$LOCALAPPDATA\My App\bin\helper.exe"
  RMDir "$LOCALAPPDATA\My App\bin"
!macroend
```

这些宏在静默安装和卸载（`/S`）中也会运行，[`install.ps1`](#macos-与-windows-的安装脚本)正是这样运行安装程序的：`IfSilent` 可以区分两者，消息框需要 `/SD` 才能自动应答。[更新](updates.md)不用安装程序就替换应用的文件，所以不会运行任何宏。`customUnInstall` 调用的函数属于卸载程序，以 `un.` 命名。

脚本也可以在卸载程序中询问用户。`customUnPage` 宏用 [nsDialogs](https://nsis.sourceforge.io/Docs/nsDialogs/Readme.html) 创建一个自己的页面，显示在卸载程序的确认页之前；用户离开该页面时运行 `customUnPageLeave`；没有 `customUnPage` 就没有这个页面。`customUnConfirmShow` 和 `customUnConfirmLeave` 分别在确认页显示时和用户离开它时运行，相当于 Modern UI 的 `MUI_PAGE_CUSTOMFUNCTION_SHOW` 和 `_LEAVE` 函数；在 `Leave` 宏中 `Abort` 会让用户留在该页面。静默卸载（`/S`）不显示任何页面，所以页面设置的变量保持默认值。下面的页面让用户选择是否删除应用保存在其文件夹之外的数据，默认不勾选，由 `customUnInstall` 读取这一选择：

```nsis
; build/installer.nsh
!include "nsDialogs.nsh"
Var RemoveData
Var RemoveDataBox

!macro customUnPage
  !insertmacro MUI_HEADER_TEXT "卸载 My App" "选择要删除的内容。"
  nsDialogs::Create 1018
  Pop $0
  ${NSD_CreateCheckbox} 0 0 100% 12u "同时删除我的数据和设置"
  Pop $RemoveDataBox
  nsDialogs::Show
!macroend

!macro customUnPageLeave
  ${NSD_GetState} $RemoveDataBox $RemoveData
!macroend

!macro customUnInstall
  ${If} $RemoveData == ${BST_CHECKED}
    RMDir /r "$LOCALAPPDATA\My App"
  ${EndIf}
!macroend
```

安装程序仅为当前用户安装，无需管理员权限，便于应用自我更新。因此钩子也应只操作用户可写的位置，例如 `$LOCALAPPDATA` 和 `HKCU`。安装程序已提供安装目录选择，卸载程序移除应用文件夹和用户的[登录时启动](app.md#登录时启动)条目，保留应用数据。添加钩子不影响安装程序及卸载程序的签名。

### 代码签名

未签名的应用会让 SmartScreen 警告用户。用证书为可执行文件、[辅助可执行文件](#辅助可执行文件)、安装程序及其卸载程序签名：

```ts
export default defineConfig({
  windows: { certificate: "certs/code-signing.pfx" },
});
```

密码来自 `MYGO_WINDOWS_CERTIFICATE_PASSWORD` 环境变量。在 Windows 上由 Windows SDK 的 `signtool` 签名，其他系统上则用 `osslsigncode`。`timestampUrl` 用于更改时间戳服务器，默认是 `http://timestamp.digicert.com`。

私钥保存在硬件令牌或云服务（如 Azure Trusted Signing）中的证书，要用自己的命令签名，其中 `%1` 是文件：

```ts
export default defineConfig({
  windows: {
    signCommand: "signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 /a %1",
  },
});
```

## Linux

`mygo build` 生成可执行文件（以应用的 [`slug`](configuration.md#应用) 命名，默认为应用名的小写形式）、桌面条目和图标，以及 Debian 和 RPM 包。这些包将应用安装到 `/opt/my-app`，并安装桌面条目、图标，注册 URL 协议和文件类型：

```ts
export default defineConfig({
  linux: {
    maintainer: "Jane Doe <jane@example.com>",
    comment: "Take notes",
    categories: ["Office"],
    depends: {
      deb: ["libayatana-appindicator3-1"],
      rpm: ["libayatana-appindicator-gtk3"],
      arch: ["libayatana-appindicator"],
    },
    command: "my-app",
  },
});
```

这些包依赖 GTK 3 和 WebKitGTK；`depends` 可以为每种格式（`deb`、`rpm` 和 `arch`）添加更多包，也可以接受一个 Debian 包列表（如 `["libayatana-appindicator3-1"]`），由 MyGo 映射到它知道的 Arch Linux 和 RPM 包。
完全不显示网页的应用（窗口和 `ui.WebView` 中都没有网页），例如[原生 UI](native.md) 应用，只需要 GTK。设置 `webview: false` 后，包依赖不再包含 WebKitGTK，`install.sh` 和在项目中运行的 `mygo doctor` 也不再检查它，因为应用不会加载它。
`maintainer` 默认取 package.json 的 `author`，否则取应用名称。
`comment` 在桌面条目和包中描述应用，`categories` 把它放进应用程序菜单（默认 `Utility`）。
应用从应用程序菜单打开。`command` 还会给应用一个命令：来自包的 `/usr/bin/my-app`，以及来自 [`install.sh`](#安装脚本) 的 `~/.local/bin/my-app`；也可以给多个命令，如 `["my-app", "ma"]`，这样就多了一个短名称。不设置它时，两者都不会添加命令，这个名字就留给同名的命令行工具，比如应用自己的命令行工具。

`formats` 选择构建要写出哪些包，可取 `deb`、`rpm` 和 `pkgbuild`，默认三种全有；不认识的名称会报错。带 [`install.sh`](#安装脚本) 的归档总会写出：`install.sh` 从它安装并[更新](updates.md)应用，PKGBUILD 也把它打包。`formats: ["deb"]` 写出 Debian 包和归档，`formats: []` 只写归档。`SHA256SUMS` 和 `-upload` 只处理构建写出的内容，构建也只对它写出的包的 `depends` 发出警告。

```ts
export default defineConfig({
  linux: { formats: ["deb", "rpm"] },
});
```

桌面条目把应用的窗口类 `StartupWMClass` 设为 `command` 的可执行文件名：X11 会话会把窗口和条目归为一组，并在任务栏显示它的图标，Wayland 则用同一个名称作为应用 id 来匹配。用另一个名称的命令启动（如 `ma`）时，应用仍是同一个程序：它的窗口仍属于那个类，它的页面也保留原来的数据，如 `localStorage`。

已发布的应用不要更改 slug。它的 Linux 文件以 slug 命名，更新会替换同名文件：更新到另一个 slug 的版本会把旧可执行文件留在原处，`install.sh` 注册的桌面条目仍会运行它，而新 slug 的 Debian 或 RPM 包是另一个包，会和旧的并排安装。在首次发布前就选好 slug，例如 `云音乐` 用 `cloud-music`，按自己的名字生成则会是 `yun-yin-le`。

### Arch Linux

除了归档，每次 Linux 构建还会得到一个 `PKGBUILD`，除非 [`linux.formats`](#linux) 排除了它。`makepkg` 会用它把旁边的归档打包成 Arch Linux 包，按 SHA-256 校验后用 pacman 安装：

```sh
cd build/linux-amd64
makepkg -si
```

这个包安装的内容与 Debian 包相同：应用位于 `/opt/my-app`，命令位于 `/usr/bin`，并通过 pacman 钩子注册桌面条目、图标、URL 协议和文件类型。图标使用 PKGBUILD 旁的 `my-app-256x256.png` 和 `my-app-512x512.png`，因为归档中只有原始图像。各字段取自配置：

| 字段 | 来源 |
|---|---|
| `pkgname` | slug |
| `pkgver` | 去掉连字符的版本，pkgver 不能包含连字符：`1.2.0-beta.1` 变成 `1.2.0beta.1`，pacman 会正确地把它排在 `1.2.0` 之前（它会把 `1.2.0_beta.1` 排在之后），`1.2.0-1` 变成 `1.2.0pre1`，`1.2.0+build.5` 变成 `1.2.0_build.5` |
| `pkgdesc` | `linux.comment`，否则为名称 |
| `arch` | amd64 为 `x86_64`，arm64 为 `aarch64` |
| `url` | package.json 的 `homepage`，否则为 `updates.github` 的仓库 |
| `license` | mygo.json 中的 `license`，否则 package.json 中的，否则 `LicenseRef-custom` |
| `depends` | `gtk3`；除非 `linux.webview` 为 `false`，否则还有 `webkit2gtk-4.1`；然后是 `linux.depends.arch`。当 `linux.depends` 是 Debian 包列表时，则是其中每个 Debian 包对应的、MyGo 知道的 Arch Linux 包，如 `libnotify4` 对应 `libnotify`；对于找不到对应 Arch Linux 包的 Debian 包，构建会发出警告并将其省略 |

`-upload` 不会带上 PKGBUILD：PKGBUILD 打包的是它旁边的归档，而每个架构都有自己的 PKGBUILD。把它连同图标复制到从发布版本中下载的归档旁边，就能为该发布版本打包。

### 安装脚本

每次 Linux 构建还会生成应用归档 `my-app-1.2.0-linux-amd64.tar.gz` 和 `install.sh`，无需 root 即可安装。脚本将应用安装到 `~/.local/my-app.app`，使其可以[自我更新](updates.md)，并安装桌面条目、图标，注册 URL 协议和文件类型。配置了 `linux.command` 时，还会在 `~/.local/bin` 中创建命令链接，但不会占用其他程序已有的命令名；安装后会提示用成功创建的第一个命令启动应用。配置了[更新](updates.md)时，脚本会下载当前机器对应更新清单中的最新版本，并在安装前像应用更新器一样，校验大小，再用 `updates.publicKey` 校验 Ed25519 签名，因此一行命令即可安装应用：

```sh
curl -fsSL https://github.com/me/my-app/releases/latest/download/install.sh | sh
```

当 [`tagPrefix`](updates.md#设置) 不是 `v` 时，任何发布版本中的脚本（如 `…/releases/download/desktop-v1.2.0/install.sh`）都会安装带该前缀标签的最新发布版本。

它用 `openssl pkeyutl -rawin` 校验签名，OpenSSL 3.0 及更高版本提供该命令（Debian 12、Ubuntu 22.04、Fedora 36、RHEL 9 及更高版本）。没有这个命令时（例如只有 OpenSSL 1.1.1），脚本不会下载任何东西，并会说明这一点。这时可以从发布版本中下载归档再交给脚本；脚本会安装传给它的归档，或者放在它旁边的归档，都不做校验。

`-upload` 会把它随归档一起发布；使用 `updates.url` 时，把它发布在清单旁边。当它和对应版本的归档放在一起时（如 `build/linux-amd64/`），它会改为安装那个归档；它也会安装传给它的归档：

```sh
sh install.sh my-app-1.2.0-linux-amd64.tar.gz
sh install.sh --uninstall   # keeps the settings and data of the app
```

再次运行会在已安装版本之上安装它找到的版本；指向应用已不再使用的名称的链接（slug 除外）会保留，就像用户自己创建的链接一样，直到 `--uninstall` 移除 `~/.local/bin` 中所有指向应用的链接。
应用需要 WebKitGTK，而 `install.sh` 不安装它：系统的库缓存中没有它时，脚本会在安装后说明这一点，并给出在 Debian、Ubuntu、Fedora、Arch 和 openSUSE 上安装它的命令，除非 `linux.webview` 为 `false`。
应用自我更新时，会重新注册新版本的桌面条目和文件类型，因此新添加的 URL 协议或文件类型也会生效。希望通过系统包管理器更新应用时，可使用 Debian 或 RPM 包；其他情况可使用 `install.sh`，以便应用在 Linux 上自我更新。

### macOS 与 Windows 的安装脚本

配置了[更新](updates.md)时，同一个 `install.sh` 会在 macOS 上安装应用，Windows 构建则得到 `install.ps1`，所以在每个系统上一行命令即可安装应用：

```sh
curl -fsSL https://github.com/me/my-app/releases/latest/download/install.sh | sh
```

```powershell
irm https://github.com/me/my-app/releases/latest/download/install.ps1 | iex
```

在 macOS 上，`install.sh` 会下载 Mac 架构的更新清单指定的归档，没有时用 `darwin-universal` 的（Apple silicon 的 `darwin-arm64`，通过 Rosetta 运行的 shell 也一样）。下载后，它像应用的更新器那样，对照 `updates.publicKey` 校验大小和 Ed25519 签名（macOS 没有处理 Ed25519 签名的工具，所以脚本用 `openssl` 和 `shasum` 做哈希，再用 JavaScript for Automation 完成曲线运算）。如果应用正在运行，脚本会先退出它，然后替换 `/Applications` 里的应用包；如果用户无法写入 `/Applications`，或者应用本来就在 `~/Applications` 里，则替换那里的应用包，最后打开应用。它从不请求 root，同名但包标识符不同的应用不会被碰。`curl` 不设置隔离标记，所以打开应用时不会出现 Gatekeeper 对下载应用的提示。
`sh install.sh --uninstall` 移除应用并保留其设置和数据。

`install.ps1` 在 Windows PowerShell 5.1 和 PowerShell 7 中运行。它下载适合 PC 架构（`amd64` 或 `arm64`）的最新版本安装程序，对照 `SHA256SUMS` 校验 SHA-256；如果应用正在运行，就先退出应用，在已安装版本的文件夹中静默运行安装程序（`/S`），然后打开应用。卸载时，仍和往常一样从“设置 > 应用”操作。

带更新的构建会把 `SHA256SUMS`（安装程序、归档和脚本的 SHA-256，格式与 `sha256sum` 相同）写到输出目录，`-upload` 会随它们一起发布，并合并其他平台早先上传的行：同一个 GitHub 发布版本的那些行，或 `updates.url` 为 `updates.s3` 提供的 `SHA256SUMS` 中的那些行，但不包含上传所替换的早期版本的文件。它按构建的方式命名文件；GitHub 会把上传名称中的空格变成点，`install.ps1` 也遵循这一点。使用 `updates.url` 和自己的服务器时，把脚本和 `SHA256SUMS` 发布在清单旁边，其中包含每个平台最新文件对应的行。

`SHA256SUMS` 未签名，只能检测下载损坏或不完整，无法防范发布文件被恶意替换。在 Windows 上应为安装程序签名（[代码签名](#代码签名)），让系统显示其发布者。

测试时，`MY_APP_DOWNLOAD_URL`（应用的 [`slug`](configuration.md#应用) 的大写形式）是两个脚本改为下载的位置，`MY_APP_INSTALL_DIR` 是 macOS 上应用的文件夹，`MY_APP_NO_OPEN=1` 让应用保持关闭。

## 语言

带[消息目录](i18n.md)的应用向每个系统声明它们的语言，源语言在前，系统会回退到它：

- macOS：`CFBundleDevelopmentRegion`、`CFBundleLocalizations`，以及 `Contents/Resources` 中每种语言一个本地化文件夹，按 Xcode 的命名方式命名（`de.lproj`、`zh-Hans.lproj`，`zh-Hant-HK` 为 `zh-HK.lproj`），这样 AppKit 的界面文字（“服务”菜单、各种面板）会使用应用显示的语言，系统设置也会列出应用的语言。
- Linux：桌面条目中的 `Name[xx]` 和 `Comment[xx]`，按 POSIX 区域设置（`de`、`zh_CN`、`zh_TW`、`sr@latin`）。
- Windows：安装程序内置各语言的文本，优先使用用户的显示语言，找不到时使用源语言。仅使用中文的应用（`"i18n": { "source": "zh-Hans" }`）也只有中文安装界面，但 MyGo 提供的少数文本除外：有关应用正在运行的消息仍使用英文。完成页中创建桌面快捷方式的复选框使用 MyGo 的英文字符串 `mygo.desktopShortcut`，属于 [MyGo 自己的语言](i18n.md#mygo-自己的字符串)；应用可以在自己的消息目录中翻译或修改它。NSIS 3 提供约 60 种语言的文件；如果缺少所需语言，构建会停止并列出支持的语言。文件属性中还包含按语言组织的字符串表。

应用在某种语言中的名称和描述来自消息目录的 `mygo.appName` 和 `mygo.appDescription`；配置里的 `name` 和 `linux.comment` 是源语言的，Windows 上如果有 `windows.displayName`，则用它代替 `name`。没有消息目录的应用不声明任何语言。

在 macOS 上，`Info.plist` 的其他字符串（例如系统请求麦克风或系统音频时显示的原因，`NSMicrophoneUsageDescription`、`NSAudioCaptureUsageDescription`）按 Xcode 项目的做法翻译：使用应用[资源](#平台资源)中该语言的 `InfoPlist.strings`；构建会保留这个文件，并在文件里缺少应用名称时补上应用名称。

```
resources/darwin/
├── ja.lproj/InfoPlist.strings        "NSMicrophoneUsageDescription" = "音声メモを録音します。";
└── zh-Hans.lproj/InfoPlist.strings   "NSMicrophoneUsageDescription" = "用麦克风录语音笔记。";
```

该文件是 Xcode 写出的文本格式，使用 UTF-8 或 UTF-16，或是 XML 属性列表。

## URL 协议与文件类型

配置中的 `urlSchemes` 和 `fileAssociations`（见[深度链接](app.md#深度链接)和[文件关联](app.md#文件关联)）由每个包注册：macOS 应用的 `Info.plist`、Windows 安装程序，以及 Linux 桌面条目、安装脚本、Debian 和 RPM 包和 PKGBUILD。

## 发布

配置中设置了 `updates.github` 时，`mygo build -upload` 会把磁盘映像、安装程序、包、Linux 归档、安装脚本以及更新文件上传到该版本的 GitHub 发布版本，标签为 `v1.2.0`，并把该发布版本创建为草稿。检查草稿后发布它。这需要已登录的 [GitHub CLI](https://cli.github.com)（`gh`）。使用 `updates.s3` 时，它会把它们上传到 Amazon S3 或兼容服务（如 Cloudflare R2）的存储桶，凭据放在 `AWS_ACCESS_KEY_ID` 和 `AWS_SECRET_ACCESS_KEY` 中（见[发布到 S3](updates.md#发布到-s3)）。

在 CI 中于每个平台构建，并上传到同一个发布版本：macOS 运行器负责签名、公证并制作磁盘映像；任何运行器都能编译和打包 Windows 与 Linux 应用。把签名密钥保存在 CI 的机密变量中：`MYGO_WINDOWS_CERTIFICATE_PASSWORD`、`MYGO_UPDATER_PRIVATE_KEY`，以及 macOS 运行器钥匙串中的公证凭据。[GitHub Actions](github-actions.md) 有一个完成这一切的工作流。
