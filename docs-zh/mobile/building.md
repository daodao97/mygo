# 为手机构建和发布

## 构建

```sh
mygo build -platform ios             # dist/ios-arm64/<slug>-<version>.ipa, signed
mygo build -platform ios/simulator   # dist/ios-simulator/<name>.app
mygo build -platform android         # dist/android/<slug>-<version>.apk and .aab
mygo build -platform android/arm64   # one ABI: arm64, arm or amd64
```

`.ipa` 用已安装的描述文件中最匹配应用标识符和 `ios.distribution`（默认 `app-store`，TestFlight 也用它）的那个签名，其证书在钥匙串中；它带有 App Store Connect 要求的隐私清单，根据应用代码用到的内容生成，除非项目有自己的 `ios/PrivacyInfo.xcprivacy`。如果构建中引用了 Apple 的私有方法，CLI 会拒绝构建，因为 App 审核会拒绝它。

`.aab` 是 Google Play 接受的格式，`.apk` 供其他应用商店和设备安装。正式版用 `android.keystore` 签名，其密码来自 `MYGO_ANDROID_KEYSTORE_PASSWORD` 和 `MYGO_ANDROID_KEY_PASSWORD`；否则用调试密钥签名，而 Google Play 不接受调试密钥。

其余设置在配置的 `ios` 和 `android` 部分中（见[配置](../configuration.md)）：

```ts
export default defineConfig({
  name: "Todo",
  identifier: "dev.example.todo",
  ios: { team: "ABCDE12345", infoPlist: { NSCameraUsageDescription: "Scan receipts." } },
  android: { permissions: ["CAMERA"], keystore: "upload.jks", keyAlias: "upload" },
});
```

## 发布

`mygo publish` 上传 `mygo build` 为当前版本生成的产物，或 `-file` 指定的文件。

```sh
mygo build -platform ios && mygo publish -platform ios
mygo build -platform android && mygo publish -platform android -track beta
```

**iOS** 用 `xcrun altool` 把 `.ipa` 上传到 App Store Connect，处理完成后会出现在 TestFlight 中。上传需要团队的 API 密钥（App Store Connect › 用户和访问 › 集成），密钥从不出现在命令行中：

| 变量 | |
|---|---|
| `MYGO_ASC_KEY_ID` | 密钥的 ID |
| `MYGO_ASC_ISSUER_ID` | 签发者的 ID |
| `MYGO_ASC_KEY` | 其 `.p8` 文件的路径 |

**Android** 通过 Google Play 的 Android Publisher API 上传 `.aab`：创建一次编辑，上传应用包，在轨道上创建发布，然后提交；任何一步失败都会删除这次编辑。`MYGO_PLAY_SERVICE_ACCOUNT` 是服务账号 JSON 密钥的路径，也可以是 JSON 本身，便于在 CI 的密钥中使用；该账号需要在 Play 管理中心中有应用的访问权限。`-track` 默认是 `internal`，也可以是 `alpha`、`beta`、`production` 或某个封闭测试轨道的名称。应用的第一次上传只能在 Google Play 管理中心中完成；尚未经过审核的应用只接受 `-status draft` 的发布。

上传的目标包就是应用的 `identifier`。

## 链接、图标和启动画面

```ts
export default defineConfig({
  ios: {
    associatedDomains: ["example.com"],
    launchScreen: { background: "#102030", image: "resources/logo.png" },
    backgroundModes: ["audio", "remote-notification"],
  },
  android: {
    appLinks: ["example.com"],
    adaptiveIcon: { foreground: "resources/icon-foreground.png", background: "#102030" },
    splash: "#102030",
  },
});
```

**链接**：你的域名下的 https URL 会打开应用，并传给 `App.OnOpenURL`。`ios.associatedDomains` 让应用带上 `applinks:example.com` 签名（自带服务的值，如 `webcredentials:example.com`，保持原样）；应用 ID 需要“关联域”能力，主机需要提供 `apple-app-site-association` 文件。`android.appLinks` 为每个主机添加一个经过验证的 intent filter（`*.example.com` 包括其子域名），每个主机需要提供列出该应用及其签名密钥的 `/.well-known/assetlinks.json`。

**Android 的图标**：`android.adaptiveIcon` 是 Android 8.0 及以上的启动器图标，一张 432×432 或更大的 PNG，叠在一种颜色上。启动器会把它裁成中间的 72/108，并可能移动它，所以重要的内容要放在那里。旧系统仍显示 `icon`。

**启动画面**：`ios.launchScreen` 是一种颜色、居中显示在其上的一张图片，或两者兼有，不需要 storyboard：CLI 把它们编译进应用的资源目录，并在 `UILaunchScreen` 中引用。图片是 3 倍屏尺寸的 PNG，所以 600×600 像素的标志占 200×200 点。`android.splash` 是应用启动时系统所显示画面的颜色，应用图标显示在它上面。

**后台模式**：`ios.backgroundModes` 是应用的 `UIBackgroundModes`，加上插件要求的那些。
