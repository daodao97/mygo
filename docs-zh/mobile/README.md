# 手机和平板

同一个 MyGo 应用可以在 iPhone 和 iPad 上运行，也可以在 Android 手机和平板上运行：网页窗口显示在系统的 webview 中（iOS 上是 WKWebView，Android 上是 Android 的 WebView），原生 UI 的窗口用 GPU 绘制（iOS 上是 Metal，Android 上是 OpenGL ES），应用的 Go 代码、绑定的服务和事件都不变。`mygo dev` 在你修改代码时把应用运行在模拟器或设备上；`mygo build` 生成 App Store 和 Google Play 接受的产物。

```sh
mygo doctor                    # what is there, and what is missing
mygo devices                   # what -device takes
mygo dev -platform ios         # the booted iOS simulator
mygo dev -platform android     # the device or emulator adb reaches
mygo build -platform ios       # a signed .ipa
mygo build -platform android   # an .apk and an .aab
mygo publish -platform ios     # the .ipa to App Store Connect
mygo publish -platform android # the .aab to a track of Google Play
```

本节的其他页面介绍[为手机构建和发布](building.md)、[手机上的界面](interface.md)（标签页、手指和文本），以及[设备 API 和插件](plugins.md)（包括你自己的原生代码）。

## 你需要什么

- **iOS**：一台装有 Xcode 的 Mac。Go 代码用 Xcode 的 clang 链接（Go 为 iOS 链接时需要 cgo），CLI 编译应用的 Swift 宿主、图标和应用包，并为它签名，不需要 Xcode 项目。在设备上运行和上架 App Store 需要 Apple 开发者团队，以及与应用标识符匹配的描述文件（provisioning profile），可由 Xcode（设置 › 账户）或 developer.apple.com 生成。
- **Android**：任意系统上装有 SDK 和 NDK 的 Android Studio（SDK Manager › SDK Tools › NDK）。Go 代码为每个 ABI 构建成一个共享库，由 NDK 的 clang 链接，再由 Gradle 构建应用；CLI 在 SDK 的常用位置或 `ANDROID_HOME` 中查找 SDK，在 SDK 中或 `ANDROID_NDK_HOME` 中查找 NDK，并使用 Android Studio 的 JDK 或 `JAVA_HOME`，JDK 17 或更新版本均可。

桌面构建仍然不需要 cgo，也不需要这些工具。

`mygo doctor` 会检查它们。在 Mac 上，它检查 Xcode 及其命令行工具、iOS SDK、模拟器、签名身份和描述文件；在所有系统上，它检查 Android SDK、adb、NDK、JDK、模拟器和已连接的设备。缺少的项目只是一条说明如何获取的警告，从不算失败：只面向桌面的应用一样都不需要。

## 设备

`mygo devices` 按名称和 ID 列出 `-device` 接受的设备：iOS 模拟器及其运行时、iOS 设备、Android 设备和模拟器，以及尚未运行的模拟器虚拟设备（AVD）。`-json` 以脚本可读的形式输出。

```
NAME           ID                                    PLATFORM    KIND       STATE
iPhone 18 Pro  629AC4ED-EFB6-4090-9143-E4B57B3220DA  iOS 27.0    simulator  booted
Jane's iPhone  00008140-000E55192283801C             iOS 27.0.1  device     connected
Pixel_9        emulator-5554                         Android 17  emulator   booted
Tablet         Tablet                                Android     emulator   shutdown
```

它会询问 `xcrun simctl`、`xcrun devicectl`（Xcode 15 及以上）、`adb` 和 `emulator`，问不到的就不列出。

## 开发

`mygo dev -platform ios` 为模拟器构建应用，安装到已启动的模拟器上（或 `-device` 指定的模拟器，按名称或 UDID，它会启动该模拟器）并运行，应用的输出显示在控制台中。Go 代码每次改动后，几秒内就会重新构建并重新启动应用。前端的 `devUrl` 与桌面上一样可用：模拟器能访问 Mac 上的开发服务器。

`mygo dev -platform android` 对 `adb` 能连接的设备或模拟器做同样的事（或 `-device` 指定的序列号），它通过 `adb reverse` 以同一个 URL 访问开发服务器。没有 `devUrl` 时，应用会嵌入 `frontendDist`，因为设备读不到电脑的磁盘。

Go 代码的改动大约两秒生效：宿主的 Swift 保持已编译状态，iOS 上把新的可执行文件放进已安装的应用，Android 上不经 Gradle 就把新的库传给可调试的应用。宿主、插件、资源或配置的改动则会重新构建并安装整个应用。

Go 无法替换正在运行的程序中的代码，所以每次改动都会重启应用，而应用会回到原来的位置。滚动位置、`ui.Router` 所在的页面，以及控件用 `ui.Local` 保存的内容会自动恢复。一行代码就能恢复应用自己的值，包括未导出的字段，不需要在类型上加标签或方法：

```go
a := &app{tab: 0}
mygo.KeepState(a)
```

系统在后台结束应用后，它也会让应用这样恢复。它保留的是数据：函数、通道、接口和指向其他包类型的指针保持程序创建时的样子，类型被这次改动修改过的字段也保持新建时的值。需要更多控制时，可以用 `App.OnSaveState` 和 `App.RestoredState`。这些在桌面上的 `mygo dev` 下同样可用。

开发版应用名为“<name> Dev”，标识符为“<identifier>.dev”，与桌面上相同，因此可以与正式版并存。

### 在 iPhone 或 iPad 上

`mygo dev -platform ios -device <name or UDID>` 也接受设备，设备需要开启开发者模式（设置 › 隐私与安全性）。应用为设备构建，并用开发描述文件签名：从已安装的描述文件中选出最匹配“<identifier>.dev”的那个，通常是 Xcode 维护的通配符描述文件，其证书在钥匙串中，并且列出了该设备。`ios.provisioningProfile` 指定的描述文件用于 `ios.distribution`，所以只有当它是 `development` 时，才会用来为开发版签名。

`xcrun devicectl` 负责安装和启动，应用的输出显示在控制台中；如果 Xcode 的 `devicectl` 没有 `--console`，应用照样运行，只是看不到输出。设备不在 Mac 的网络栈上：`localhost` 上的 `devUrl` 会改用 Mac 在网络中的地址加载，开发服务器必须监听该地址（`vite --host`）。开发版允许访问本地网络，并为系统的提示说明原因。

通配符描述文件不能带关联域（Associated Domains）：使用这种描述文件时，开发版会省略 `ios.associatedDomains` 并给出提示。

### Android 模拟器

`-device` 也接受 AVD 的名称。未运行的 AVD 会在 `mygo dev` 之外启动，`mygo dev` 等待其系统启动完成；`mygo dev` 结束后它继续运行，因为再次启动很慢。

## 应用在手机上的行为

- **一个屏幕**：最后显示的窗口占满屏幕，位于之前显示的窗口之上；隐藏或关闭它会显示它下面的窗口。窗口的大小、位置和标题栏不起作用。`Window.Bounds` 是屏幕的范围。
- **原生 UI 在安全区域内布局**：避开系统栏、屏幕的挖孔，以及显示时的键盘，而窗口背景铺满边缘。网页在系统栏之内布局，在 Android 上还位于键盘之上。
- **触摸**见[手指](interface.md#手指)；`Tester.FingerDown`、`FingerMove` 和 `FingerUp` 用手指测试视图。
- **键盘**在原生 UI 的元素接收文本时显示，带有系统的输入建议和输入法（包括组字），硬件键盘的按键与桌面上一样可用（[文本](interface.md#文本)）。
- **辅助功能**：VoiceOver 和 TalkBack 能朗读原生 UI 的元素并对它们操作。
- **Android 的返回**会弹出最后推入的窗口、在网页中后退、关闭原生 UI 中 Escape 会关闭的东西、在 `ui.Router` 中后退，否则让应用在后台继续运行。系统会提前知道这些，以支持预测性返回：`ui.Router` 的页面跟随手势移动，没有可返回的内容时，系统显示应用离开的动画。
- **生命周期**：应用从不自己退出。进入后台是电源事件 `"suspend"`（`Power.OnSuspend`），回到前台是 `"resume"` 和 `App.OnActivate`。两者之间帧会停止。系统可能在后台结束应用：`App.OnSaveState` 保存恢复所需的内容，下次启动时可以在 `App.RestoredState` 中找到。
- **内存**：系统内存不足时会调用 `App.OnMemoryWarning`。原生 UI 这时会释放渲染器的纹理和文本缓存，下一帧再重新创建。
- **深度链接**（`urlSchemes`）通过 `App.OnOpenURL` 打开应用。
- **对话框**：消息框是系统的警告框；`Dialog.Open` 显示系统的文稿选择器，并在应用的临时文件中提供所选文件的副本。`Dialog.Save` 提供一个用于写入的临时文件；`Dialog.SaveCopy` 让用户选择应用生成的文件存放到哪里。
- **文件**：`WatchFiles` 与桌面上一样监视应用的目录。
- **密钥**在 iOS 上存入钥匙串，在 Android 上存入应用的存储，并用 Android 密钥库中的密钥加密。
- **通知**第一次使用时会请求用户的许可。
- 手机没有的功能什么都不做：菜单栏、托盘、程序坞、全局快捷键、更新器（应用由应用商店更新）。

Android 的 webview 不加载自定义协议：应用的前端（`mygo://localhost`）及其其他协议在 `https://<scheme>.localhost` 上提供，网页除了在 `location.href` 中之外察觉不到。对应用协议发出的 `fetch` 和 `XMLHttpRequest` 请求保留请求体；在 Android 上，提交到应用协议的表单不带请求体到达，因为 Android 不提供它，所以请用 `fetch` 或绑定的方法提交表单。
