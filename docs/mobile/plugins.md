# Device APIs and plugins

## The device

| | |
|---|---|
| `mygo.Haptics` | The feel of a selection, an impact or how a task ended |
| `mygo.SystemBars` | The color of the icons of the system's bars, and hiding them |
| `mygo.SetOrientations` | How the screen may turn |
| `mygo.Share` | The system's share sheet of text, a link and files |
| `mygo.Network` | Whether it is online, and whether that costs |
| `mygo.Power.Battery` | The battery's level, and the low power mode |
| `mygo.SetBarItems` | The buttons of a window's system navigation bar |
| `Context.SafeArea` | What covers the window's edges, to draw in the `Overlay` |

They do nothing on desktops, or report `errors.ErrUnsupported`.

## Official plugins

What needs permissions or a large dependency is a plugin, which an app has only
if it imports it, with its usage descriptions and permissions added to the app:

| Package (`plugins/...`) | What it does |
|---|---|
| `maps` | A map in native UI, with pins: MapKit, and osmdroid with OpenStreetMap's tiles, which needs no key |
| `location` | Where the device is, and following it, through Core Location and LocationManager |
| `imagepicker` | The system's picker of photos and videos, which needs no permission, and its camera screen |
| `camera` | A live preview in native UI, and photos: AVCaptureSession and CameraX |
| `videoplayer` | Video in native UI: AVPlayer and MediaPlayer |
| `biometrics` | Face ID, Touch ID, fingerprints, the passcode |
| `sensors` | The accelerometer, the gyroscope and the magnetometer |
| `push` | APNs and Firebase tokens, messages and tapped notifications (iOS devices need the Push Notifications capability in the profile, Firebase on Android a project) |
| `purchases` | In-app purchases and subscriptions: StoreKit 2 and Google Play Billing |
| `permissionflow` | Whether the app may use the camera, the microphone, photos, contacts…, asking, and the system's settings |
| `preferences` | Small values kept between runs, on every platform |
| `sqlite` | SQLite with full-text search, 64-bit, compiled into the app |
| `background` | Go code that runs while the app isn't open, as the system finds the time: BGTaskScheduler and WorkManager |

## Writing a plugin

Native code for phones is best a [native module](../native-modules.md): a Go
interface, implemented in Swift on iOS and in Kotlin on Android, which Go
calls as any Go value, with its types, errors, streams and callbacks, and
which `mygo dev` and `mygo build` build with the app from its sources. The
same module can be Swift on macOS, and Rust or Zig on other desktops:

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

Its `Info.plist`, entitlements and Gradle dependencies join the app's
([native modules](../native-modules.md#phones)). The SQLite plugin is one:
its module is Swift on iOS, Kotlin with C on Android, and Zig on desktops.

A plugin can also bring Swift and Kotlin that Go calls with JSON, which is
how the official plugins above are written: a Go
package beside its Swift and Kotlin, which its `mygo-plugin.json` names. `mygo build` finds it among the packages the app
imports, compiles its code into the host and registers it:

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

Go calls it with JSON, and hears its events:

```go
func On(ctx context.Context, on bool) error {
	return mygo.Native("torch").Call(ctx, "set", map[string]bool{"on": on}, nil)
}
```

The Swift goes in the host's module, the Kotlin in package `dev.mygo`, on the UI
thread. They answer once, at any time, and `PluginHost.emit` sends events
(`mygo.Native("torch").On`):

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

`PluginHost` has what they need of the host: on iOS the `presenter` to present
over and `applicationEvent` for what reaches the app's delegate; on Android the
`activity`, `startForResult` and `requestPermissions`. An error's code comes to
Go as a `*mygo.NativeError`.

### C code

C reaches phones in a native module: its Kotlin builds C with CMake
(`externalNativeBuild` in its `build.gradle.kts`), with the NDK and for the
ABIs the app builds, and its Swift package takes C targets, or a library
as a binary framework. The SQLite plugin's module does both with SQLite's
amalgamation, pinned by its hash: `plugins/sqlite/internal/sqlitecore`.

### Native views

`PluginHost.registerView` registers a type of view, in the plugin's initializer,
which `mygo.NewPluginView` makes for a window showing native UI, and `ui.WebView`
places. It shows under the content, in a hole of its frames, as a web view does:
what the view paints after it shows over it, and fingers on that stay the
content's. The view's `ID` names it in calls about it. `plugins/videoplayer` is
a short example, for a map or anything else the system draws.
