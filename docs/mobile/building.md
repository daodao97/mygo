# Building and publishing for phones

## Building

```sh
mygo build -platform ios             # dist/ios-arm64/<slug>-<version>.ipa, signed
mygo build -platform ios/simulator   # dist/ios-simulator/<name>.app
mygo build -platform android         # dist/android/<slug>-<version>.apk and .aab
mygo build -platform android/arm64   # one ABI: arm64, arm or amd64
```

The `.ipa` is signed with the installed provisioning profile that best
fits the app's identifier and `ios.distribution` (`app-store` by default,
which is also TestFlight's), whose certificate is in the keychain; it
carries the privacy manifest App Store Connect asks for, made from what the
app's code uses, unless the project has its own
`ios/PrivacyInfo.xcprivacy`. The CLI refuses a build that names Apple's private methods, which App
Review turns down.

The `.aab` is what Google Play takes, the `.apk` what other stores and
devices install. Release builds are signed with `android.keystore`, its
passwords from `MYGO_ANDROID_KEYSTORE_PASSWORD` and
`MYGO_ANDROID_KEY_PASSWORD`, or else with the debug key, which Google Play
refuses. From `minSdk` 24 the build leaves out the JAR signature (v1),
which Android needs no more; `android.v1Signing` adds it for the phones
whose installers still ask for it.

The app's native libraries are built for devices with 16 KB pages, which
Android 15 and later may have: Go and native modules' C link for them, and
the APK stores them on 16 KB boundaries. The build checks the APK as
Android does, and names any library of its 64-bit ABIs that is not, such
as one a plugin's dependency ships: on those devices the app would run in
page size compatibility mode, with a warning on every launch, and Google
Play refuses it.

The configuration's `ios` and `android` sections set the rest (see
[Configuration](../configuration.md)):

```ts
export default defineConfig({
  name: "Todo",
  identifier: "dev.example.todo",
  ios: { team: "ABCDE12345", infoPlist: { NSCameraUsageDescription: "Scan receipts." } },
  android: { permissions: ["CAMERA"], keystore: "upload.jks", keyAlias: "upload" },
});
```

## App extensions

An app extension, as a VPN client's Network Extension (its packet tunnel
provider), is a bundle of its own in the app's `PlugIns/`, with its own
provisioning profile and entitlements. Build it apart, with Xcode, and name
it in `ios.extensions`:

```json
{
  "identifier": "com.example.vpn",
  "ios": {
    "extensions": [
      { "path": "build/PacketTunnel.appex", "provisioningProfile": "PacketTunnel App Store" }
    ]
  }
}
```

The CLI copies each into `PlugIns/` and signs it before the app, with the
app's certificate: its profile goes in it as `embedded.mobileprovision`, and
it is signed with that profile's entitlements. `provisioningProfile` is its
name, UUID or the path of a `.mobileprovision`; without one, the installed
profile that best fits the extension's identifier and `ios.distribution`.
The identifier must start with the app's and a dot, as
`com.example.vpn.tunnel`, or iOS refuses to install it, and its version and
build number should be the app's, which App Store Connect checks. The
simulator's build signs extensions ad hoc. `mygo dev` on a device signs them
with development profiles, and the development app, `com.example.vpn.dev`,
carries the extension as `com.example.vpn.dev.tunnel`: derive the extension's
identifier in the app's code from `Bundle.main.bundleIdentifier`.

## Publishing

`mygo publish` uploads what `mygo build` wrote for this version, or `-file`.

```sh
mygo build -platform ios && mygo publish -platform ios
mygo build -platform android && mygo publish -platform android -track beta
```

**iOS** uploads the `.ipa` to App Store Connect with `xcrun altool`, where it
shows in TestFlight once processed. It takes an API key of the team (App Store
Connect › Users and Access › Integrations), which never goes on the command
line:

| Variable | |
|---|---|
| `MYGO_ASC_KEY_ID` | the key's ID |
| `MYGO_ASC_ISSUER_ID` | the issuer's ID |
| `MYGO_ASC_KEY` | the path of its `.p8` |

**Android** uploads the `.aab` through Google Play's Android Publisher API:
an edit, its bundle, a release on the track, then the commit, or the edit
deleted if anything fails. `MYGO_PLAY_SERVICE_ACCOUNT` holds the path of a
service account's JSON key, or the JSON itself, for secrets of CI; the account
needs access to the app in Play Console. `-track` is `internal` by default, or
`alpha`, `beta`, `production` or a closed testing track's name. Google Play
takes the first upload of an app in its console only, and an app it has not
reviewed yet takes releases only with `-status draft`.

The app's `identifier` is the package it uploads to.

## Links, icons and launch screens

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

**Links**: https URLs of your domains open the app, and reach
`App.OnOpenURL`. `ios.associatedDomains` signs the app with
`applinks:example.com` (a value with its own service, as
`webcredentials:example.com`, stays as it is); the app ID needs the capability
Associated Domains, and the host an `apple-app-site-association` file.
`android.appLinks` adds a verified intent filter per host (`*.example.com`
takes its subdomains), and each host serves
`/.well-known/assetlinks.json` naming the app and its signing key.

**Android's icon**: `android.adaptiveIcon` is the launcher icon of Android 8.0
and later, a PNG of 432×432 or more over a color. Launchers crop to the middle
72/108 of it and may move it, so keep what matters there. `icon` stays what
older systems show.

**Launch screens**: `ios.launchScreen` is a color, an image centered on it, or
both, with no storyboard: the CLI compiles them into the app's asset catalog
and names them in `UILaunchScreen`. The image is a PNG at the size of a 3×
screen, so a logo of 600×600 pixels takes 200×200 points. `android.splash` is
the color of the screen the system shows while the app starts, under the
app's icon.

**Background modes**: `ios.backgroundModes` are the app's
`UIBackgroundModes`, with those its plugins ask for.

## Executables

An app that runs a program of its own beside its Go code, as a VPN client
runs its proxy core, ships it on Android as a native library: a native
module carries it as `kotlin/src/main/jniLibs/arm64-v8a/lib<name>.so`, one per
ABI. Android lets an app exec only its files in `nativeLibraryDir` (W^X, from
targetSdk 29), and by default the app's libraries stay in the APK,
uncompressed, and are loaded from it, so that directory is empty.
`android.extractNativeLibs` has the installer extract them there:

```json
{
  "android": { "extractNativeLibs": true }
}
```

The app then finds the executable at
`context.applicationInfo.nativeLibraryDir + "/lib<name>.so"`. The libraries
ship compressed, and take space again once extracted, so leave it off unless
the app execs one.

Link such an executable for 16 KB pages (`-Wl,-z,max-page-size=16384`), as
Android's own libraries are: on a device or emulator with 16 KB pages, one
linked for 4 KB pages crashes as it starts.

An executable built for some ABIs only, as `arm64-v8a`, limits the app to
them, so that no build for another ABI lacks it:

```json
{
  "android": { "extractNativeLibs": true, "abis": ["arm64"] }
}
```

`mygo build -platform android` then builds those alone, and `mygo dev`
refuses a device or emulator of another ABI. In every build, a native
module's libraries of an ABI that Go wasn't built for stay out of the APK:
a device of that ABI would install them without the app.
