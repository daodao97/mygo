# 设备 API 和插件

## 设备

| | |
|---|---|
| `mygo.Haptics` | 选择、碰撞或任务结束方式的触感 |
| `mygo.SystemBars` | 系统栏图标的颜色，以及隐藏系统栏 |
| `mygo.SetOrientations` | 屏幕可以如何旋转 |
| `mygo.Share` | 系统的共享面板，共享文本、链接和文件 |
| `mygo.Network` | 是否在线，以及流量是否计费 |
| `mygo.Power.Battery` | 电池电量，以及低电量模式 |
| `mygo.SetBarItems` | 窗口的系统导航栏中的按钮 |
| `Context.SafeArea` | 窗口边缘被什么遮挡，以便在 `Overlay` 中绘制 |

它们在桌面上什么都不做，或者报告 `errors.ErrUnsupported`。

## 官方插件

需要权限或较大依赖的功能是插件，应用只有导入它才会拥有它，其用途说明和权限会加入应用：

| 包（`plugins/...`） | 作用 |
|---|---|
| `maps` | 原生 UI 中带标注的地图：MapKit，以及使用 OpenStreetMap 瓦片的 osmdroid，不需要密钥 |
| `location` | 设备所在的位置，以及持续跟踪位置，通过 Core Location 和 LocationManager |
| `imagepicker` | 系统的照片和视频选择器，不需要权限，以及它的拍照界面 |
| `camera` | 原生 UI 中的实时预览，以及拍照：AVCaptureSession 和 CameraX |
| `videoplayer` | 原生 UI 中的视频：AVPlayer 和 MediaPlayer |
| `biometrics` | 面容 ID、触控 ID、指纹、设备密码 |
| `sensors` | 加速度计、陀螺仪和磁力计 |
| `push` | APNs 和 Firebase 的令牌、消息，以及被轻点的通知（iOS 设备需要描述文件中有推送通知能力，Android 上需要一个 Firebase 项目） |
| `purchases` | App 内购买和订阅：StoreKit 2 和 Google Play 结算 |
| `permissionflow` | 应用能否使用相机、麦克风、照片、通讯录等，请求授权，以及系统的设置 |
| `preferences` | 在多次运行之间保存的小数据，所有平台可用 |
| `sqlite` | 带全文搜索的 64 位 SQLite，编译进应用 |
| `background` | 应用没有打开时、在系统找到时间时运行的 Go 代码：BGTaskScheduler 和 WorkManager |

## 编写插件

手机上的原生代码最好写成[原生模块](../native-modules.md)：一个 Go 接口，在 iOS 上用 Swift 实现，在 Android 上用 Kotlin 实现，Go 像调用普通 Go 值一样调用它，带有它的类型、错误、流和回调，`mygo dev` 和 `mygo build` 随应用从源码构建它。同一个模块在 macOS 上可以是 Swift，在其他桌面上可以是 Rust 或 Zig：

```sh
mygo native new -lang swift,kotlin torch
```

```go
//mygo:native
//mygo:mainthread
type Torch interface {
	// Set lights the torch, or puts it out.
	Set(on bool) error
}
```

```kotlin
fun makeTorch(): Torch = CameraTorch()

private class CameraTorch : Torch {
    private val camera = Native.context.getSystemService(CameraManager::class.java)

    override fun set(on: Boolean) {
        val id = camera.cameraIdList.firstOrNull { camera.getCameraCharacteristics(it).get(CameraCharacteristics.FLASH_INFO_AVAILABLE) == true }
            ?: throw NativeUnsupportedException()
        camera.setTorchMode(id, on)
    }
}
```

它的 `Info.plist`、权限和 Gradle 依赖会加入应用（[原生模块](../native-modules.md#手机)）。SQLite 插件就是这样一个模块：在 iOS 上是 Swift，在 Android 上是带 C 的 Kotlin，在桌面上是 Zig。

插件也可以带上 Go 通过 JSON 调用的 Swift 和 Kotlin，上面的官方插件就是这样写的：一个 Go 包，旁边是它的 Swift 和 Kotlin，由它的 `mygo-plugin.json` 列出。`mygo build` 在应用导入的包中找到它，把它的代码编译进宿主并注册：

```json
{
  "mobile": {
    "id": "torch",
    "ios": {
      "sources": ["ios/*.swift"],
      "class": "TorchPlugin",
      "frameworks": ["AVFoundation"],
      "infoPlist": {"NSCameraUsageDescription": "Lights the torch."}
    },
    "android": {
      "sources": ["android/*.kt"],
      "class": "TorchPlugin",
      "permissions": ["CAMERA"],
      "dependencies": ["androidx.camera:camera-core:1.6.2"],
      "application": ["<service android:name=\"dev.mygo.TorchService\" />"]
    }
  }
}
```

Go 用 JSON 调用它，并接收它的事件：

```go
func On(ctx context.Context, on bool) error {
	return mygo.Native("torch").Call(ctx, "set", map[string]bool{"on": on}, nil)
}
```

Swift 放在宿主的模块中，Kotlin 放在包 `dev.mygo` 中，在 UI 线程上运行。它们在任意时刻回复一次，`PluginHost.emit` 发送事件（`mygo.Native("torch").On`）：

```swift
final class TorchPlugin: MyGoPlugin {
    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply) {
        switch method {
        case "set": reply(.success(nil))
        default: reply(.failure(PluginFailure("NoMethod", method)))
        }
    }
}
```

```kotlin
package dev.mygo

class TorchPlugin : MyGoPlugin {
    override fun call(method: String, args: JSONObject, reply: PluginReply) {
        when (method) {
            "set" -> reply.success(null)
            else -> reply.failure(PluginFailure("NoMethod", method))
        }
    }
}
```

`PluginHost` 提供它们需要的宿主功能：在 iOS 上是用来弹出界面的 `presenter`，以及传给应用委托的事件 `applicationEvent`；在 Android 上是 `activity`、`startForResult` 和 `requestPermissions`。错误的代码以 `*mygo.NativeError` 到达 Go。

### C 代码

C 通过原生模块进入手机：模块的 Kotlin 用 CMake 构建 C（在它的 `build.gradle.kts` 中用 `externalNativeBuild`），使用 NDK，并为应用所构建的 ABI 构建；它的 Swift 包可以带 C target，或以二进制 framework 引入一个库。SQLite 插件的模块用 SQLite 的 amalgamation 同时做了这两件事，并用哈希固定版本：`plugins/sqlite/internal/sqlitecore`。

### 原生视图

`PluginHost.registerView` 在插件的初始化方法中注册一种视图，`mygo.NewPluginView` 为显示原生 UI 的窗口创建它，由 `ui.WebView` 放置。它显示在内容之下、内容帧中留出的空洞里，与 webview 一样：在它之后绘制的内容显示在它之上，落在这些内容上的手指仍归内容所有。视图的 `ID` 在有关它的调用中指代它。`plugins/videoplayer` 是一个简短的示例，可用于地图或其他由系统绘制的东西。
