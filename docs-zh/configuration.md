# 配置

`mygo.config.ts` 位于项目根目录，描述应用及 CLI 的开发、构建方式。默认导出即为配置，项目依赖的 `mygo-cli` 包通过 `defineConfig` 提供类型，让编辑器能补全字段并显示相应文档：

```ts
import { defineConfig } from "mygo-cli";

export default defineConfig({
  name: "My App",
  slug: "my-app",
  identifier: "com.example.myapp",
  version: "1.2.0",
  copyright: "© 2026 Example Inc.",

  devUrl: "http://localhost:5173",
  devCommand: "bun run dev:web",
  buildCommand: "bun run build:web",
  frontendDist: "dist",
  bindings: "src/mygo.ts",
  out: "build",

  resources: ["third_party/licenses"],
  urlSchemes: ["myapp"],
  fileAssociations: [{ ext: ["md"], name: "Markdown Document", mimeType: "text/markdown" }],

  i18n: { dir: "locales", source: "en" },
  updates: { publicKey: "…", github: "you/my-app" },
  macos: { signingIdentity: "Developer ID Application: Jane Doe (TEAMID)" },
  windows: { certificate: "certs/code-signing.pfx" },
  linux: { maintainer: "Jane Doe <jane@example.com>" },
});
```

每个字段都是可选的。配置也可以是 JSON，放在 [mygo.json](#mygojson) 中，正如[原生 UI](ui/README.md) 的项目那样：它们没有 JavaScript，也没有要开发或构建的前端，所以会省略[前端](#开发与前端)的字段。

## 应用

| 字段 | 默认值 | |
|---|---|---|
| `name` | 目录的名称 | 用户看到的名字：应用、它的可执行文件、菜单、安装程序；Windows 上可以显示另一个名字，[`windows.displayName`](#windows) |
| `slug` | 名称的小写形式，词之间用连字符，例如 `My App` 的 `my-app`；中文字符取最常用的普通话读音（不带声调），每个字各算一个词，假名和谚文则取各自的罗马化，例如 `云音乐` 的 `yun-yin-le`、`ラーメン` 的 `ramen` | 需要纯 ASCII 名称时，应用文件所用的名字：macOS 的磁盘映像和 Windows 的安装程序、Linux 可执行文件、它的桌面条目、图标、Debian 和 RPM 包以及 `/opt` 目录、每个平台的[更新归档](updates.md)、[安装脚本](distribution.md#macos-与-windows-的安装脚本)的变量，以及应用定义的 MIME 类型；由小写字母和数字组成，之间用单个连字符。把它设为另一个名字，例如为 `云音乐` 设 `cloud-music`，并在应用发布后保持不变（见 [Linux](distribution.md#linux)） |
| `identifier` | `com.mygo.` 加上名称的字母和数字，例如 `com.mygo.myapp` | 应用唯一的反向 DNS 名称，例如 `com.example.myapp`；系统按它来索引偏好设置、权限和注册信息 |
| `version` | `0.1.0` | 应用的版本，`App.Version()` 返回它，更新也用它来比较 |
| `copyright` | | 应用的版权声明，写在它的 `Info.plist`（macOS）和文件属性（Windows）中 |
| `publisher` | 去掉“©”和年份的版权声明，例如 `© 2026 Example Inc.` 的 `Example Inc.` | 制作应用的人：Windows 上“设置 > 应用”为已安装应用列出的发布者，以及文件属性中的公司 |
| `license` | package.json 中的 `license` | 应用的 SPDX 许可证表达式，如 `MIT` 或 `Apache-2.0 WITH LLVM-exception`，RPM 包和 Arch Linux 的 PKGBUILD 会记录它 |
| `icon` | 存在时为 `resources/icon.png` | 正方形 PNG，理想情况下为 1024×1024；如果旁边有带 `-dev` 的文件，如 `icon-dev.png`，它就是开发版应用的图标 |
| `main` | `.` | 应用的 Go 包，相对于项目 |

## 开发与前端

| 字段 | 默认值 | |
|---|---|---|
| `devUrl` | | `mygo dev` 使用的开发服务器地址，须为 `http(s)` URL；窗口使用不带 URL 协议名（scheme）的地址（如 `/`）时，从该服务器加载 |
| `devCommand` | | `mygo dev` 运行期间在项目目录中运行，例如 Vite 开发服务器；`devUrl` 有响应后应用才启动 |
| `buildCommand` | | 在 `mygo build` 编译应用之前构建前端 |
| `frontendDist` | | 构建后的前端目录，`mygo build` 把它嵌入应用；没有 `devUrl` 时，`mygo dev` 从磁盘提供它 |
| `bindings` | `frontend/package.json` 依赖 `mygo-runtime` 时是 `frontend/src/mygo.ts`；否则根目录的 package.json 依赖它时是 `src/mygo.ts`；否则配置中有前端（`devUrl`、`devCommand`、`buildCommand` 或 `frontendDist`）时是 `mygo.ts`；没有前端的应用，例如[原生 UI](ui/README.md) 的应用，则没有 | `mygo generate` 写入 TypeScript 客户端的位置 |
| `out` | `dist` | `mygo build` 写入构建产物的位置，每个平台一个目录 |

它们如何配合，见[前端](frontend.md#页面如何加载)。新项目把 `out` 设为 `build`，以区别于 Vite 的 `dist`。

## 集成

| 字段 | |
|---|---|
| `resources` | 复制到每个平台的应用中的文件和目录，与 `resources` 目录的内容放在一起，以各自的基名为名；见[资源](distribution.md#资源)，以及[平台资源](distribution.md#平台资源)，了解某个平台的文件 |
| `resourcesExclude` | `resources` 目录中不放进应用的文件：它们在该目录中路径的模式，如 `["**/*.psd", "design"]`，其中 `**` 匹配任意数量的目录；Go 源文件总是被排除。见[排除文件](distribution.md#排除文件) |
| `urlSchemes` | 应用打开的 URL 协议，例如 `myapp://…` 的 `myapp`，它们会传给 `App.OnOpenURL`；见[深度链接](app.md#深度链接) |
| `fileAssociations` | 应用打开的文件类型，它们会传给 `App.OnOpenFile`；见[文件关联](app.md#文件关联) |

每个文件关联有：

| 字段 | |
|---|---|
| `ext` | 文件扩展名，不带点：`["md", "markdown"]` |
| `name` | 文件是什么：`"Markdown Document"` |
| `mimeType` | 文件的 MIME 类型，Linux 用它识别文件；应用会定义一个未注册的类型（`x-`），未指定时为 `application/x-<app>-<ext>` |
| `role` | macOS 上的 `Editor`（默认）或 `Viewer` |

## i18n

应用的消息目录，每种语言一个 JSON 文件；见[语言](i18n.md)。没有 `i18n` 时，应用只包含英语。

| 字段 | 默认值 | |
|---|---|---|
| `dir` | `locales` | 消息目录所在的目录，应用附带的每种语言一个 `<tag>.json`（`de.json`、`zh-Hant.json`）；它本身是一个 Go 包，`mygo generate` 在其中写入消息的 Go 函数 |
| `source` | `en` | 应用编写时使用的语言，它的消息目录包含所有消息，其他语言以它为回退 |

## updates

带签名的[自动更新](updates.md)。`publicKey` 是必需的，`github` 和 `url` 中还要有一个。

| 字段 | 默认值 | |
|---|---|---|
| `publicKey` | | `mygo-update.pub` 的内容，来自 `mygo keygen` |
| `github` | | 一个公开仓库 `owner/name`，更新放在它的发布版本里 |
| `tagPrefix` | `v` | 发布标签中版本前面的部分；换用其他前缀，如 `desktop-v`，会让应用查找以它标记的最新发布版本，而不是该仓库的最新发布版本 |
| `url` | | 代替 `github`，存放更新的目录的 HTTPS URL |
| `s3` | | 为 `url` 提供内容的存储桶，`mygo build -upload` 会把文件上传到它：`bucket`，以及可选的 `prefix`（它的目录）、`region`（默认：`AWS_REGION`，否则 `us-east-1`）、`endpoint`（兼容服务如 Cloudflare R2 的端点）和 `pathStyle`（见[发布到 S3](updates.md#发布到-s3)） |
| `privateKey` | | `mygo-update.key` 的路径，供 `mygo build` 使用；保存密钥的 `MYGO_UPDATER_PRIVATE_KEY` 环境变量优先 |
| `changelog` | 存在时为 `CHANGELOG.md` | 一个 Markdown 文件，其 `## <version>` 部分成为发布说明 |
| `deltas` | `3` | 有多少个较早的版本会获得[增量更新](updates.md#增量更新)；`0` 表示不生成 |

## macos

| 字段 | 默认值 | |
|---|---|---|
| `icon` | | 以资源目录（asset catalog）、`Assets.car` 或 Icon Composer 的 `.icon` 提供图标，放在 `icon` 指定的 `.icns` 旁边；也可用另一个 `.icns` 替代原图标。见[图标](distribution.md#名称图标与版本) |
| `minimumSystemVersion` | `12.0` | 应用支持的最旧 macOS 版本 |
| `signingIdentity` | `-`，临时签名 | 签署应用及其磁盘映像的身份，例如 `Developer ID Application: Jane Doe (TEAMID)` |
| `entitlements` | | 用于签署应用的权限（entitlements）属性列表 |
| `provisioningProfile` | | 在签名前作为 `Contents/embedded.provisionprofile` 嵌入的描述文件；必须与应用标识符、权限和签名身份匹配；不能与临时签名一起使用。见[描述文件](distribution.md#描述文件) |
| `helperEntitlements` | | 资源中代码的权限属性列表，按它在资源中的路径，例如 `{ "bin/server": "server.entitlements.plist" }`；其他代码保持签署它时使用的权限。见[辅助可执行文件](distribution.md#辅助可执行文件) |
| `infoPlist` | | 添加到应用 `Info.plist` 的键，替换 MyGo 的，例如 `NSCameraUsageDescription` |
| `dmgTitle` | `name` | 磁盘映像的卷名 |
| `notarize` | | 对生产构建的应用和磁盘映像进行公证：`keychainProfile`，即 `xcrun notarytool store-credentials` 的钥匙串描述文件，以及可选的 `keychain`，存放该描述文件的钥匙串 |

见 [macOS](distribution.md#macos)。

## windows

| 字段 | 默认值 | |
|---|---|---|
| `displayName` | `name` | Windows 显示应用时使用的名字：它的开始菜单和桌面快捷方式、它在“设置 > 应用”中的条目、安装程序的标题和消息，以及可执行文件文件属性中的产品名称。可执行文件、它的安装文件夹，以及以 `name` 或 `slug` 命名的文件仍沿用原来的名字，例如名为 `weather` 的应用显示为 `天气助手`，其可执行文件仍是 `weather.exe` |
| `installDirName` | `name` | 安装程序安装应用的文件夹名，`%LOCALAPPDATA%\Programs\<installDirName>`，例如名为 `智链` 的应用用 ASCII 的 `ZhiLian`；可执行文件、快捷方式、“设置 > 应用”中的条目以及用户看到的其他内容也都保持原名。不能有路径分隔符、`:*?"<>\|`，也不能是 `.` 和 `..`。更改它只影响新的安装。见[安装程序](distribution.md#安装程序) |
| `certificate` | | 用于可执行文件、资源中未签名的可执行文件和库，以及安装程序的代码签名证书（`.pfx`）；密码来自 `MYGO_WINDOWS_CERTIFICATE_PASSWORD` |
| `signCommand` | | 代替 `certificate` 的签名命令，`%1` 表示文件路径 |
| `timestampUrl` | `http://timestamp.digicert.com` | `certificate` 的时间戳服务器 |
| `installerInclude` | | 安装程序会包含的 NSIS 脚本，例如 `build/installer.nsh`，其 `customInit`、`customInstall` 和 `customUnInstall` 宏分别在安装程序启动时、安装结束时，以及卸载时、移除应用文件夹之前运行；`customUnPage` 在确认页之前添加一个卸载程序页面，`customUnConfirmShow` 和 `customUnConfirmLeave` 在确认页上运行。见[安装程序钩子](distribution.md#安装程序钩子) |

见 [Windows](distribution.md#windows)。

## linux

| 字段 | 默认值 | |
|---|---|---|
| `maintainer` | package.json 中的 `author`，否则 `name` | `Name <email>`，Debian 和 RPM 包以及 PKGBUILD 的维护者 |
| `comment` | | 简短描述，用于桌面条目和包 |
| `categories` | `["Utility"]` | 桌面条目的类别，决定它在应用菜单中的位置 |
| `depends` | | 应用除 GTK 和 WebKitGTK 之外需要的包，按包格式（`deb`、`rpm` 和 `arch`），或一个 Debian 包列表，MyGo 会把它映射到已知的 Arch Linux 和 RPM 包 |
| `webview` | `true` | `false` 用于不显示网页的应用，例如原生 UI 的应用：Debian 和 RPM 包以及 PKGBUILD 只依赖 GTK，install.sh 和 `mygo doctor` 不会要求 WebKitGTK |
| `command` | 无 | 运行应用的命令，或一组名称，如 `["my-app", "ma"]`，后者会额外加一个短名字：每个名称在 Debian 和 RPM 包及 PKGBUILD 中生成 `/usr/bin/<command>`，在 install.sh 中生成 `~/.local/bin/<command>` |
| `formats` | `["deb", "rpm", "pkgbuild"]` | `mygo build` 写入的包，可选 `deb`、`rpm` 和 `pkgbuild`；带 install.sh 的归档总会生成，因为 install.sh 从它更新应用，PKGBUILD 也打包它，所以设为 `[]` 时只生成它 |

见 [Linux](distribution.md#linux)。

## 计算出的配置

配置本身就是代码，可以计算字段值，例如从 package.json 读取版本，或从 CI 环境中读取签名身份。默认导出也可以是同步或异步函数，参数为当前运行的命令（`"dev"`、`"build"`、`"generate"` 或 `"init"`）：

```ts
import { defineConfig } from "mygo-cli";
import pkg from "./package.json" with { type: "json" };

export default defineConfig(({ command }) => ({
  name: command === "dev" ? "My App (dev)" : "My App",
  version: pkg.version,
  macos: {
    signingIdentity: process.env.MACOS_SIGNING_IDENTITY ?? "-",
  },
}));
```

## 如何运行

CLI 会在项目目录中用 [Bun](https://bun.sh) 求值 `mygo.config.ts`，没有 Bun 就用 Node.js 22.6 或更高版本，然后检查结果。Node.js 通过移除类型来运行 TypeScript，所以文件可以使用类型注解，但不能使用生成代码的语法，如 `enum`。文件打印的内容会进入终端，与 CLI 的输出分开。`mygo dev` 会在该文件变化时重新加载应用，但它导入的文件变化时则不会。

## mygo.json

配置也可以改用 JSON，写在 `mygo.json` 中，读取它不需要 JavaScript 运行时；`mygo init -template native` 会创建一个这样的文件：

```json
{
  "name": "My App",
  "identifier": "com.example.myapp",
  "devUrl": "http://localhost:5173",
  "macos": { "signingIdentity": "Developer ID Application: Jane Doe (TEAMID)" }
}
```

它有相同的字段。一个项目要么有 `mygo.config.ts`，要么有 `mygo.json`，不能两者都有。
