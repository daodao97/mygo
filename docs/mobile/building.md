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
refuses.

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
