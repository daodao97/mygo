# Phones and tablets

The same MyGo app runs on iPhones and iPads, and on Android phones and
tablets: windows of web pages show in the system's web view (WKWebView on
iOS, Android's WebView), windows of native UI draw with the GPU (Metal on
iOS, OpenGL ES on Android), and the app's Go code, its bound services and
its events are the same. `mygo dev` runs the app on a simulator, an
emulator or a device as you change it; `mygo build` makes what the App
Store and Google Play take.

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

The other pages of this section cover [building and
publishing](building.md), [interfaces on phones](interface.md), with tabs,
fingers and text, and [device APIs and plugins](plugins.md), with native
code of your own.

## What you need

- **iOS**: a Mac with Xcode. The Go code links with Xcode's clang (cgo,
  which Go needs to link for iOS), and the CLI compiles the app's Swift
  host, its icon and its bundle, and signs it, without an Xcode project.
  Devices and the App Store need an Apple developer team and a
  provisioning profile for the app's identifier, which Xcode makes
  (Settings › Accounts) or developer.apple.com.
- **Android**: Android Studio with the SDK and the NDK (SDK Manager › SDK
  Tools › NDK), on any system. The Go code is a shared library for each
  ABI, linked by the NDK's clang, and Gradle builds the app; the CLI finds
  the SDK in its usual place or `ANDROID_HOME`, the NDK in the SDK or
  `ANDROID_NDK_HOME`, and Android Studio's JDK or `JAVA_HOME`, any JDK
  from 17 on.

Desktop builds still need neither cgo nor these tools.

`mygo doctor` checks them. On a Mac it looks at Xcode and its command line
tools, the iOS SDK, the simulators, the signing identities and the
provisioning profiles; on every system at the Android SDK, adb, the NDK, the
JDK, the emulators and the devices connected. What is missing there is a
warning that says how to get it, never a failure: an app for desktops needs
none of it.

## Devices

`mygo devices` lists what `-device` takes, by name and ID: iOS simulators
with their runtimes, iOS devices, Android devices and emulators, and the
virtual devices (AVDs) of emulators that are not running. `-json` prints it
for scripts.

```
NAME           ID                                    PLATFORM    KIND       STATE
iPhone 18 Pro  629AC4ED-EFB6-4090-9143-E4B57B3220DA  iOS 27.0    simulator  booted
Jane's iPhone  00008140-000E55192283801C             iOS 27.0.1  device     connected
Pixel_9        emulator-5554                         Android 17  emulator   booted
Tablet         Tablet                                Android     emulator   shutdown
```

It asks `xcrun simctl`, `xcrun devicectl` (Xcode 15 and later), `adb` and
`emulator`, and leaves out what it cannot ask.

## Developing

`mygo dev -platform ios` builds the app for the simulator, installs it on
the one booted (or `-device`, a simulator's name or UDID, which it boots)
and launches it, the app's output in the console. Each change of the Go
code rebuilds and relaunches it in a few seconds. A frontend's `devUrl`
works as on the desktop: the simulator reaches the Mac's dev server.

`mygo dev -platform android` does the same with the device or emulator
`adb` reaches (or `-device`, its serial), which reaches the dev server
through `adb reverse`, at the same URL. Without `devUrl`, the app embeds
`frontendDist`, as a device cannot read the computer's disk.

A change of the Go code takes about two seconds: the host's Swift stays
compiled and the new executable goes into the installed app on iOS, and on
Android the new library goes to the debuggable app without Gradle. A change of
the host, a plugin, the resources or the configuration builds and installs the
whole app again.

Go can't swap code in a running program, so a change restarts the app, and the
app comes back where it was. Scroll offsets, where a `ui.Router` is and what
widgets keep with `ui.Local` return by themselves. One line brings back the app's
own values, unexported fields and all, with no tags or methods on its types:

```go
a := &app{tab: 0}
mygo.KeepState(a)
```

It also brings the app back as the system ended it in the background. It keeps
data: functions, channels, interfaces and pointers to other packages' types stay
as the program makes them, and a field whose type the change altered stays as
made. `App.OnSaveState` and `App.RestoredState` are there for more control. All
of it works under `mygo dev` on desktops too.

The development app is "<name> Dev" with the identifier
"<identifier>.dev", as on the desktop, so it installs beside the release.

### On an iPhone or iPad

`mygo dev -platform ios -device <name or UDID>` also takes a device, which
needs Developer Mode on (Settings › Privacy & Security). The app builds for
the device and signs with a development provisioning profile: the installed
one that best fits "<identifier>.dev", usually the wildcard profile Xcode
keeps, whose certificate is in the keychain and which lists the device. A
profile `ios.provisioningProfile` names is for `ios.distribution`, so it signs
development builds only when that is `development`.

`xcrun devicectl` installs and launches it, the app's output in the console;
an Xcode whose `devicectl` has no `--console` still runs the app, without its
output. A device is not on the Mac's network stack: it loads a `devUrl` on
`localhost` at the Mac's address on the network, which the dev server must
listen on (`vite --host`), and development builds allow local networking and
say why for the system's prompt.

A wildcard profile cannot have Associated Domains: with one, the development
build leaves `ios.associatedDomains` out and says so.

### Android emulators

`-device` also takes the name of an AVD. One that is not running starts,
apart from `mygo dev`, which waits for its system to finish booting; it keeps
running after `mygo dev` ends, as starting it again is slow.

## How apps behave on phones

- **One screen**: the window shown last fills the screen, above those
  shown before; hiding or closing it shows the one under it. Window sizes,
  positions and title bars have no say. `Window.Bounds` is the screen's.
- **Native UI lays out inside the safe area**: the system's bars, the
  display's cutout and, while it shows, the keyboard, while the window's
  background fills the edges. Pages lay out inside the bars, and above the
  keyboard on Android.
- **Touch** works as [Fingers](interface.md#fingers) describes;
  `Tester.FingerDown`, `FingerMove` and `FingerUp` test views with a finger.
- **The keyboard** shows while an element of native UI takes text, with
  the system's suggestions and input methods (composition included), and
  a hardware keyboard's keys work as on the desktop ([Text](interface.md#text)).
- **Accessibility**: VoiceOver and TalkBack read native UI's elements and
  act on them.
- **Android's back** pops the window pushed last, goes back in a page,
  closes what Escape closes in native UI, goes back in a `ui.Router`, or
  else leaves the app running in the background. The system knows
  beforehand, for its predictive back: a `ui.Router`'s page follows the
  gesture, and without anything to go back to the system shows the app
  leaving.
- **Lifecycle**: the app never quits itself. Going to the background is a
  power event `"suspend"` (`Power.OnSuspend`), and coming back `"resume"`
  and `App.OnActivate`. Frames stop in between. The system may end an app in
  the background: `App.OnSaveState` keeps what it needs to come back, which
  the next launch finds in `App.RestoredState`.
- **Memory**: `App.OnMemoryWarning` is called as the system runs short. Native
  UI lets go of its renderer's textures and its caches of text then, and the
  next frame makes them again.
- **Deep links** (`urlSchemes`) open the app with `App.OnOpenURL`.
- **Dialogs**: message boxes are the system's alerts; `Dialog.Open` shows
  its document picker and gives copies of the files chosen, in the app's
  temporary files. `Dialog.Save` gives a temporary file to write;
  `Dialog.SaveCopy` has the user choose where a file the app made goes.
- **Files**: `WatchFiles` watches the app's directories as on the
  desktop.
- **Secrets** go to the Keychain on iOS, and on Android to the app's
  storage, encrypted with a key of Android's keystore.
- **Notifications** ask for the user's permission the first time.
- What phones lack does nothing: menu bars, trays, the Dock, global
  shortcuts, the updater (stores update apps).

Android's web view loads no custom schemes: the app's frontend
(`mygo://localhost`) and its other schemes are served at
`https://<scheme>.localhost`, which pages never notice but in
`location.href`. Requests of `fetch` and `XMLHttpRequest` to the app's
schemes keep their bodies; a form posted to one arrives without its body
on Android, which does not give it, so post forms with `fetch` or through
bound methods.
