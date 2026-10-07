# iOS native UI (experimental)

MyGo can package a Go `WindowOptions.Content` view in a UIKit application.
The UI is drawn by MyGo with the shared layout, widgets, CoreText engine
and Metal renderer, with a CPU fallback. It does not need a web frontend or
Bun at runtime.

Frames are paced by `CADisplayLink`. On iOS, Metal keeps three drawables and
uses [command-buffer presentation](https://developer.apple.com/documentation/metal/mtlcommandbuffer/present(_:))
without waiting for a Core Animation transaction. On iPhone 12 mini / iOS
18.2.1, four rounds of Chinese-list scrolling, detail transitions and tab
switching reduced GPU-path frame processing time from a 31.9 ms median
(33.9 ms p95) to 5.1 ms (18.6 ms p95). These are application frame processing
times, not measured display FPS; occasional slow frames and initial font/glyph
loading remain. Captures are `.mygo/ios-chinese-phases.log` and
`.mygo/ios-chinese-frames-after.log`.

This initial backend is experimental. The native example passed an XCTest
UI flow on an iPhone 16 running iOS 26.6.2: counter taps, keyboard input,
scrolling and row selection, explicit and background state restoration, cold
URL entry, startup overlay handoff, first-frame notification, unsupported
close/quit, foreground activation and portrait/landscape rotation. Chinese IME
composition now has real Pinyin candidate commit/cancellation coverage on
iPhone 12 mini / iOS 18.2.1, plus headless Unicode and long-context tests.
VoiceOver navigation, iOS 15 and physical iPad remain unverified. An iPad iOS 26.1 simulator additionally covers
rotation, docked keyboard layout, action sheets and share popovers; it uses the
CPU presenter, while physical devices use Metal.

## Build

Use macOS with Xcode selected by `xcode-select`, an iOS SDK, and Go.
The initial targets are iOS 15+ on arm64 devices and arm64 simulators.

```sh
# Complete unsigned app plus an exported Xcode host project.
go run ./cmd/mygo build -platform ios/arm64 -debug examples/ios-native

# Signed development app: use the actual Team ID from your Apple account.
go run ./cmd/mygo build -platform ios/arm64 -debug -ios-team TEAMID examples/ios-native

# Include a specific iPhone/iPad in the development profile.
go run ./cmd/mygo build -platform ios/arm64 -debug -ios-team TEAMID -ios-device UDID examples/ios-native

# Simulator app (requires an installed runtime to run it).
go run ./cmd/mygo build -platform ios/arm64 -debug -ios-simulator examples/ios-native
```

The example's output is `examples/ios-native/build/ios-arm64/MyGo iOS.app`
or `build/ios-simulator-arm64/MyGo iOS.app`. `MyGoHost/MyGoApp.xcodeproj`
beside it includes the archive, entry point, resources and bundle metadata;
it can also be opened in Xcode for signing. Desktop build commands continue
to use `CGO_ENABLED=0`; the CLI enables cgo only for iOS.

An application's configuration can persist the team and bundle settings:

```json
{
  "name": "My App",
  "identifier": "com.example.myapp",
  "icon": "resources/icon.png",
  "ios": {
    "developmentTeam": "TEAMID",
    "minimumSystemVersion": "15.0",
    "infoPlist": {
      "NSCameraUsageDescription": "Scan a document."
    }
  }
}
```

Use an authenticated account in Xcode → Settings → Accounts. The Team ID is
the certificate's organization unit, not the personal identifier displayed
in parentheses in some Apple Development certificate names. Automatic
signing obtains a provisioning profile for your bundle identifier and
registered devices. Pass `-ios-device` with the device's UDID (from
`devicectl device info details`) to register it and obtain a matching profile.
An unsigned `.app` cannot run on a physical iPhone. Xcode's iOS platform
support must be installed in Settings → Components for destination builds.

```sh
xcrun devicectl list devices
xcrun devicectl device install app --device DEVICE 'examples/ios-native/build/ios-arm64/MyGo iOS.app'
xcrun devicectl device process launch --device DEVICE --console com.mygo.iosnative
```

For a simulator, boot one using `simctl` and install and launch the
simulator artifact with `simctl install booted` and `simctl launch booted`.
Device archives and simulator archives are separate targets.
The simulator uses the CPU presenter because its Metal SDK does not expose
drawable presentation callbacks. Physical devices retain the Metal path.

## Application model

Keep the existing `main`, `App.WhenReady`, `App.Run` and `ui.View` API. The
generated host starts UIKit on the OS main thread and enters the Go main
function on a background thread. Readiness, view construction and event
handlers run on UIKit's main thread. Public calls from goroutines dispatch
there as on desktop; callbacks must remain short.

The initial backend supports one fullscreen content window, safe-area
layout, keyboard avoidance, touch taps and scroll gestures with momentum,
text input and composition, basic accessibility actions, theme changes,
text clipboard, external URLs and configured deep links. User data, cache
and logs live in the app sandbox; bundled resources are in `MyGoResources` in
the app. `examples/ios-native` demonstrates mobile integrations with overview,
feature-catalog and runtime-state tabs. Each tab uses a `ui.Router`, including
header Back, interactive left-edge Back, nested pages and restoration of
history, editor values and scroll positions. Feature pages exercise input,
lifecycle, system dialogs/sharing, permissions, Keychain and appearance.
See [the example](../examples/ios-native/README.md).
Desktop UI development can run the
same example with `go run ./examples/ios-native`.

Webview windows, multiple windows/scenes, Android, UIKit navigation containers,
desktop menus and desktop-style automatic updates remain outside this backend.
Desktop window controls are inert or return `ErrUnsupported`. Built-in text
supports the system edit menu; iOS 17+ adds native selection visuals and handles,
including read-only selectable text without a software keyboard. Older versions
retain Go selection visuals. Pinch/rotation are opt-in through `Element.Gestures`.

The iOS archive currently compiles the shared Metal shader source at runtime.
Distribution signing and upload still depend on the application's Apple account,
provisioning capabilities and App Store Connect setup. The CLI can create local
archives and export IPAs; it does not upload them.

## Notifications and device feedback

Existing `NewNotification(...).Show()`, `OnClick`, `App.OnNotificationClick`,
`Close` and `ClearNotifications` work on iOS. Showing requests alert/sound/badge
permission if needed and returns `ErrNotificationsDenied` on denial. Closing or
clearing removes both pending and delivered requests, including earlier runs.

```go
n := mygo.NewNotification(mygo.NotificationOptions{
    ID: "message-42", Title: "New message", Body: "Tap to open the conversation",
    Data: map[string]string{"route": "/messages/42"},
})
err := n.Schedule(5 * time.Second) // one-shot; persists after process termination
// n.Close() cancels this ID. Delayed delivery is currently iOS-only.
```

The minimum iOS trigger is one second. `Badge *int` in notification options sets
an icon badge on delivery; nil leaves it unchanged. `App.SetNotificationBadge`
sets it immediately and returns authorization/native errors (zero clears it).
The existing `App.SetBadgeCount` convenience also works on iOS, retaining its
original void signature.

Register `App.OnNotification` before `App.Run` for foreground delivery and
responses from a cold launch. Its typed event contains ID, Data and Clicked.
Foreground notifications show a system banner/list entry; the system controls
actual presentation, sound and badge delivery. Cold responses are queued until
Go readiness; duplicate Scene/delegate responses are delivered once. Route
navigation remains application policy, using the existing `ui.Router`.

For APNs, configure `ios.entitlements["aps-environment"]` and a matching
provisioning profile, then call `App.RegisterPushNotifications`. Tokens (including
registration updates) and errors arrive through `OnPushToken` and
`OnPushRegistrationError`. Request visible notification permission separately.
Remote custom string fields arrive in event Data; an optional `mygoID` overrides
the OS request identifier. Sending remote pushes needs your APNs delivery service.
Silent background push handling is not provided by this notification API.

`Haptics.Play` supports Selection, Light/Medium/Heavy/Soft/Rigid impacts and
Success/Warning/Error feedback. Native generators are reused. Success means the
request reached UIKit, not proof that hardware vibrated: simulators, hardware
and system settings can suppress feedback. Desktop backends return unsupported.

## Input, gestures and system integration

`InputOptions{Dismiss: ui.KeyboardDismissOnDrag}` dismisses on Go touch scrolling.
`KeyboardDismissInteractive` uses UIKit's public interactive scroll behavior,
while Go retains its own content offsets. A completed tap outside still dismisses;
a native pan keeps the editor until UIKit ends editing. UIKit supports reversing
the drag before release to cancel ([Apple documentation](https://developer.apple.com/documentation/uikit/uiscrollview/keyboarddismissmode-swift.enum/interactive));
that reversal still needs a separate runtime test. The wrapper reserves
vertical, single-contact pans outside the focused editor and left back edge.
Read-only `Text(...).Selectable()` and read-only editors use native copy/selection
on iOS 17+ without presenting the software keyboard; cut/paste are disabled.

`Element.Gestures()` opts an element into pinch and rotation. Events include kind,
phase, local position, multiplicative scale and delta rotation in radians. A
recognized gesture cancels its ordinary press and keeps its original element
through completion, even after moving outside. Removing/ disabling the element
stops delivery. App policy decides zoom/rotation bounds and renders the result.

`App.SetStatusBar(style, hidden)` accepts Auto, Light or Dark text plus visibility.
Auto follows the immersive background. Preferences survive Scene reconnection.
On iPad, floating keyboards no longer collapse the entire window's viewport;
docked keyboards still reduce the available content height.

Configure Universal Links with `ios.associatedDomains`, such as
`["applinks:example.com"]`. The CLI generates and signs the associated-domain
entitlement. UIKit cold/foreground browsing activities already reach
`App.OnOpenURL`. A corresponding hosted `apple-app-site-association` file and
matching provisioning capability are required for OS delivery.

`fileAssociations` generates iOS document registrations. Standard extensions
use system UTIs; unrecognized formats get imported declarations with extension
and MIME tags. Opened documents reach `App.OnOpenFile`. External security-scoped
access currently lasts for that callback; use the import picker for an owned
cache copy. Persistent external-directory/bookmark access remains separate work.

For privacy declarations, Go dSYMs, manual signing and artifact checks, see
[the iOS toolchain guide](ios-toolchain.md).

## Archive and IPA export

```sh
# Release archive, inspectable in Xcode Organizer. Do not use -debug/-ios-device.
go run ./cmd/mygo build -platform ios/arm64 -ios-team TEAMID -ios-archive examples/ios-native

# Local IPA for devices in a development profile (also creates an archive).
go run ./cmd/mygo build -platform ios/arm64 -ios-team TEAMID -ios-export-method debugging examples/ios-native

# Distribution-signed IPA for App Store Connect/TestFlight, when the account is configured.
go run ./cmd/mygo build -platform ios/arm64 -ios-team TEAMID -ios-export-method app-store-connect examples/ios-native
```

Outputs include an `.xcarchive`, the host project and app, plus `Export/*.ipa`
when requested. Methods are `debugging`, `release-testing`, `app-store-connect`
and `enterprise`; availability depends on the account/profile. Export options
explicitly use `destination=export`, never upload. Use Xcode Organizer or your
own upload tooling after reviewing the artifact. `ios.buildNumber` controls
CFBundleVersion independently from the marketing version; `-ios-build-number 42`
overrides it for one build without editing the configuration. Archive versions must
be numeric. `ios.entitlements` accepts signing capabilities, merged with
`ios.associatedDomains`. Release archives use production Go tags and a Release
Xcode configuration; simulator/debug archives are rejected.

`mygo ios doctor` checks local Xcode/SDK/signing/symbol tools; `-release` requires
a distribution identity. The iOS toolchain exports standard local artifacts.
Upload/TestFlight tooling is chosen independently by the application and is
not a MyGo dependency.

## Files and photos

`Dialog.Open` presents the iOS document picker, with extension filters and
optional multiple selection. It briefly holds security-scoped access while
coordinating a read, then imports regular files into private cache
copies, so their paths remain readable after the picker dismisses. The
caller owns these copies and should use `os.Remove` when finished. Copy to
`App.Path(PathUserData)` for persistent storage: iOS can purge caches.
Directory selection and `Dialog.Save` return `ErrUnsupported`; persistent
external-directory access and security-scoped bookmarks are not implemented.

`Dialog.Export(ExportDialogOptions{Files: []string{path}})` exports existing
regular files to a location selected in Files. It returns `false, nil` on
cancellation and `true, nil` on completion. Keep the sources alive until it
returns. This API does not expose an externally writable destination path.

`Dialog.Photos(PhotoDialogOptions{Multiple: true})` uses
[PHPicker](https://developer.apple.com/documentation/photokit/selecting-photos-and-videos-in-ios)
for images without requiring full photo-library access. It also returns
caller-owned cache copies; cancellation returns `nil, nil`. Representations
are preserved, including HEIC, so the caller must use an appropriate decoder.
Video selection, Live Photo pairs and format conversion are not supported.

All three calls are safe from any goroutine and wait for completion. File
copying runs off the UI thread; failed batches and copies made after Scene
disconnection are removed. Presenting while another system dialog is active
or without a foreground Scene returns an error. Desktop-specific open-panel
options such as custom buttons, hidden files and initial directories are
ignored on iOS. `Export` and `Photos` return `ErrUnsupported` on desktop.

An app may set `ios.infoPlist.UIFileSharingEnabled` and
`ios.infoPlist.LSSupportsOpeningDocumentsInPlace` to `true` to expose its
Documents directory in Files, as the functional demo does. This is separate
from registering document types or opening external documents in place.

## App icon

The shared `icon` setting also configures the iOS home-screen icon. It must
be a square PNG, ideally 1024 × 1024 pixels; `resources/icon.png` is used
automatically when present. The CLI generates `Assets.xcassets/AppIcon.appiconset`
in the exported host and selects it with `ASSETCATALOG_COMPILER_APPICON_NAME`.
Xcode produces iPhone/iPad sizes and bundle icon metadata from the single
1024-pixel source, following [Apple's asset-catalog workflow](https://developer.apple.com/documentation/xcode/configuring-your-app-icon).
Transparent input pixels are composited over white for the default iOS
appearance. Supply a full square background; iOS applies its own corner mask.
Dark/tinted variants and alternate runtime icons are not configured yet.

The startup logo is configured separately through `ios.launchScreen.image`.
It can have transparency and is shared by the system launch storyboard and
the matching overlay. The native example uses the template's blue-violet
gradient and white ring for both locations.

## Background and status bar

Native UI's opaque root background automatically fills the top status-bar
area and bottom home-indicator area. The host updates the status-bar text
style for contrast with that color. This works with the default light/dark
themes, `Context.SetTheme` and a solid `Context.Root().Background`.
Content still stays inside the safe area, so controls avoid the notch and
home indicator. It is the background that extends to the screen edges.

```go
func view(c *ui.Context) {
    theme := ui.DarkTheme()
    theme.Background = ui.Hex("#14213d")
    c.SetTheme(theme)
    // Build the page normally; its surrounding safe areas match automatically.
}
```

An explicit `WindowOptions.BackgroundColor` or `Window.SetBackgroundColor`
takes precedence for the native backdrop and safe areas. Set the Go theme's
background to the same color when using this override. Automatic matching
uses a solid, opaque root fill; arbitrary gradients, images and differently
colored headers need an explicit surrounding color. On Scene reconnection
the retained native backdrop is reapplied to the new controller.

The startup overlay retains its launch-screen contrast until its fade
completes, then the page's background determines the status-bar text style.
The example accepts `mygo-native://demo/appearance/dark`, `/appearance/light`
and `/appearance/system` to exercise this behavior.

## Mobile input and accessibility preferences

Configure software keyboard hints on each frame:

```go
field := ui.TextInput(c, &email).InputOptions(ui.InputOptions{
    Keyboard:       ui.KeyboardEmail,
    Return:         ui.ReturnSend,
    Content:        ui.ContentEmail,
    Correction:     ui.CorrectionOff,
    Capitalization: ui.CapitalizeNone,
})
if field.Submitted() {
    // Submit the form. Return captions do not change Go editing policy.
}
```

Number, decimal, phone, URL and ASCII keyboards are also available. Input
options are hints, not validation: paste and composition may contain any
Unicode text. Text areas keep inserting newlines, including when their return
caption is changed. `ContentUsername`, `ContentPassword`, `ContentNewPassword`
and `ContentOneTimeCode` describe autofill semantics; password fields still
need `.Password()`. Autofill suggestions depend on system configuration.
Omitting `InputOptions` on a later frame restores system defaults.

The default Go theme follows iOS Dynamic Type, darker system colors and
Reduce Motion through `Context.Preferences`, including change notifications.
Explicit font sizes remain application-controlled. Complete VoiceOver
navigation still needs more device coverage.

### Text selection and editing

Long-press a focused `TextInput` or `TextArea` to select a word and open
the system menu. Cut, copy, paste and select-all edit the Go value and use
its existing undo history. Select-all covers the entire Go document,
including text outside the keyboard proxy's bounded surrounding-text window.
On iOS 17 and later, system caret, selection highlights and handles use
geometry from the Go layout rather than an independent TextKit layout.
Scrolling stays owned by Go. Highlight geometry only inspects visible
paragraphs, so selecting a long document keeps text-area virtualization.

Password fields accept paste but never export surrounding text or expose
copy/cut actions. Composition and custom `TextCaret` handlers retain Go
selection rendering. The **Text Editing** feature page demonstrates Unicode
selection, clipboard round trips, masked password paste and a composition
field showing its committed Go value separately from marked text. Native
composition snapshots reconcile committed prefix/suffix and the marked range
in one Go frame. They keep the proxy's context fixed until composition ends,
including incremental candidate commits in long documents. Ordinary snapshots
change only differing runes and share the Go undo history.

Hardware key commands support Tab/Shift-Tab between Go controls, Escape to
dismiss editor focus, and Command-A/C/X/V/Z/Shift-Z through the shared editor.
Return/Space activate a focused Go button while the surface has keyboard focus.
Input methods retain candidate-key handling while marked text exists. `ui.Cmd`
uses Command on iOS as on macOS. Command editing and Tab focus traversal are
tested through public XCTest key events on iPhone. An actual host Escape sent
to the iPad simulator clears both Go focus and the keyboard; physical-phone
XCTest Escape did not reach UIKit, so it is not evidence for a connected
keyboard. Physical keyboards, alternate IMEs, button Return/Space activation
and older-iOS selection fallbacks still need runtime coverage.

## System dialogs, sharing and permissions

`Dialog.Message` presents a native alert. `MessageOptions.Style` may be
`MessageActionSheet`; `DestructiveButtons` identifies delete/discard actions.
The host anchors popovers on iPad and uses ordinary action sheets on iPhone.
Checkbox message options return `ErrUnsupported` on iOS. File dialogs and
photo pickers are not implemented yet.

`Share.Show(ShareOptions{Text: "Hello", URL: "https://example.com"})` opens the
native share sheet and waits for completion or cancellation. `Files` accepts
existing regular files; keep them alive until `Show` returns. `ShareResult`
reports `Completed` and the system activity identifier. Both sharing and
dialogs return presentation errors when another system sheet is active or
the Scene is not active. They accept calls from any goroutine and continue
pumping native events while waiting on the main thread. When calling from a
view's build function, start a goroutine and publish the result with
`window.Update`, so the view can finish building its frame.

`Permissions.Query` and `Permissions.Request` report the OS authorization
decision separately from web origin policy. The supported permissions are
camera, microphone, geolocation (while in use), notifications, photo library
and add-only photo access. States include not-determined, granted, denied,
restricted, limited photo access and provisional notifications. Denial is a
status rather than an error. Required purpose strings must be nonempty in
`ios.infoPlist`; the backend checks them before requesting authorization.
App Store distribution preflight also checks the known protected-resource
imports in the compiled app. Apple's validation requires purpose strings for
linked permission APIs even when they are not requested at runtime; see the
[toolchain guide](ios-toolchain.md#artifact-checks) and the native demo's
`mygo.json` for the required usage descriptions.
`Permissions.OpenSettings` opens the application's settings. Query again
when the app returns to the foreground to observe changes.

See [mobile progress](mobile-progress.md) for the remaining integrations and
their verification requirements.

## Secure storage

`NewSecureStore("account", SecureStoreOptions{})` creates an application-scoped
Keychain namespace. `Set`, `Get` and `Delete` accept binary values and are safe
from any goroutine. `Get` returns `ErrSecretNotFound` for an absent key;
`Delete` succeeds for an already absent key. Empty values are supported.

The default policy is `SecretWhenUnlocked` (WhenUnlockedThisDeviceOnly).
`SecretAfterFirstUnlock` opts into reads after the device's first unlock.
Neither policy synchronizes with iCloud or migrates the item to a different
device. Items may survive application deletion, so use `Delete` when signing
out. Native Security calls run off the UI thread and return errors for locked
or unavailable protected data. There is no plaintext fallback. Keychain
user-presence/biometry access-control policies remain pending; standalone
authentication below does not enforce access control on these items.

See Apple's [Keychain guide](https://developer.apple.com/documentation/security/using-the-keychain-to-manage-user-secrets)
and [accessibility policy](https://developer.apple.com/documentation/security/ksecattraccessiblewhenunlockedthisdeviceonly)
for the system behavior behind these options.

## Biometric authentication

`Biometrics.Query()` returns the current Face ID/Touch ID kind, biometric
availability and its failure reason, plus device-owner authentication
availability with a passcode fallback. It does not present a prompt. Recheck
before use rather than persisting the result.

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
err := mygo.Biometrics.Authenticate(ctx, mygo.AuthenticationOptions{
    Reason: "Verify the device owner before continuing.",
    AllowDevicePasscode: true,
})
```

The zero value requires biometrics alone. Opting into passcode fallback uses
Apple's system prompt; MyGo receives neither passcodes nor biometric data.
Every request uses a new [LAContext](https://developer.apple.com/documentation/localauthentication/lacontext).
Face ID requires a nonempty `NSFaceIDUsageDescription` in `ios.infoPlist`.
Requests require an active Scene and reject overlapping authentication or
other system presentations. Native replies return to the main thread once;
context cancellation, background entry and Scene disconnection dismiss pending
authentication. Temporary inactivity caused by Face ID UI does not cancel it.

Use `errors.As(err, &native)` with `*mygo.AuthenticationError` to distinguish
`AuthenticationCanceled`, `AuthenticationFailed`, `AuthenticationNotEnrolled`,
`AuthenticationLockedOut`, `AuthenticationPasscodeNotSet`, missing purpose or
other native errors. Context cancellation returns `ctx.Err()`. Other backends
return `ErrUnsupported`. This checks the local device owner; applications
still own service login and protected-storage policies.

## Scene lifecycle and state

UIKit owns one `UIWindowScene`. A temporary interruption changes activity to
`inactive`; entering the background changes it to `background`. Returning
passes through `inactive` before `active`. Register `App.OnLifecycleChanged`,
`App.OnDidEnterBackground` and `App.OnWillEnterForeground` before `App.Run`.
`App.Lifecycle()` reads the current state from any goroutine. Readiness is
separate: `App.WhenReady` constructs content once for the Go process.

Backgrounding pauses display ticks and cancels the active touch. If UIKit
disconnects a Scene, MyGo keeps the Go window and view state, releases render
resources and reattaches the view when the Scene reconnects. Multiple Scenes
are not supported yet. Neither backgrounding nor disconnecting is a quit.

Persist application state explicitly, including drafts, routes and scroll
positions. Values must support JSON and expose the fields to save:

```go
type State struct {
    Draft string
    Router *ui.Router
    Scroll ui.ScrollState
}
state := State{}
mygo.App.WhenReady(func() {
    if _, err := mygo.PersistState("main", &state); err != nil {
        // Report or handle an incompatible/corrupt snapshot before building UI.
        log.Fatal(err)
    }
    if state.Router == nil {
        state.Router = ui.NewRouter("/home")
    }
    mygo.NewWindow(mygo.WindowOptions{Content: ui.View(func(c *ui.Context) {
        state.Router.View(c, func(route *ui.Route) {
            route.Title("Home")
            ui.Scroll(c).TrackScroll(&state.Scroll).Fill().Children(func() {
                ui.TextInput(c, &state.Draft)
            })
        })
    })})
})
mygo.App.OnStateSaveError(func(err error) { log.Printf("save failed: %v", err) })
```

A `*ui.Router` owns and serializes its complete history and current index,
including forward entries. Page code calls `Push`, `Replace`, `Pop`/`Back`,
`Forward`, `Go` or `Reset`; it does not copy or replay history. See
[pages and navigation](ui/navigation.md). Editor and scroll values remain
separate application state.

Mutate registered values on the UI thread. `PersistState` restores before
returning; its cancellation function unregisters the live value while
retaining its saved snapshot. Background callbacks run before the automatic
checkpoint, within a bounded UIKit background task. The snapshot is written
atomically as `app-state.json` in `PathUserData`; unchanged snapshots are not
rewritten. Corrupt or incompatible data returns an error without modifying
the live value or overwriting the file. Unknown saved keys are retained.

Call `App.SaveState()` after critical edits as well. iOS may terminate a
suspended app without any termination callback, and a force kill need not
allow another checkpoint. An app must not depend on `OnQuit` to save data.
Callbacks should remain short; the checkpoint is not a general background
worker service.

## Launch screen and first frame

Configure the system launch screen and matching startup overlay together:

```json
{
  "ios": {
    "launchScreen": {
      "title": "My App",
      "image": "assets/launch-logo.png",
      "backgroundColor": "#112233",
      "foregroundColor": "#ffffff",
      "fadeDurationMs": 250
    }
  }
}
```

The CLI generates a static `LaunchScreen.storyboard`, compiled by Xcode.
The optional image must be PNG. By default the title is the application name
and colors follow the system; the default fade duration is zero. The UIKit
host shows an overlay with the same title, image, colors and layout while
Go initializes. After the first Go frame is presented it fades the overlay
away. Reduce Motion disables the fade. The system launch screen itself is
static; the fade is performed by the running application.

`window.OnFirstFrame(func() { ... })` fires once for a native UI window.
On iOS it waits for Metal drawable presentation, or the software layer's
transaction completion. Readiness, first frame and fade completion are
separate events. A late subscriber receives the notification asynchronously.
The notification is not repeated on background/foreground transitions or
Scene reconnection. Desktop native UI reports a frame committed for display.
This API does not report webview page loading.

## Cold-start entry and closing

Register `App.OnOpenURL` and `App.OnOpenFile` before `App.Run`. The host queues
URLs and file openings received before Go is ready, then delivers them after
`WhenReady` has restored state and constructed the window. Later Scene URL
callbacks use the same handlers. Configure custom schemes with `urlSchemes`.
Browsing activities route Universal Links to `OnOpenURL`, but associated-domain
entitlements and server association files must be configured separately.

A file opening activates its security-scoped resource for the synchronous
`OnOpenFile` callback. Copy or read the file there before starting asynchronous
work; access is relinquished when the callback returns. Document-type bundle
metadata must be provided by the application through `ios.infoPlist`.

`App.IsSystemManaged()` is true on iOS. `window.TryClose()` and `App.TryQuit()`
return `mygo.ErrUnsupported` without emitting desktop close/quit events,
destroying the view or stopping Go dispatch. The existing `Close`, `Destroy`,
`Quit`, `Exit` and `Relaunch` methods do nothing on iOS. Applications should
navigate between pages in their content view and let UIKit manage the Scene
and process lifetime. Desktop cancelable close/quit behavior is preserved.

## Tests

The standalone XCTest project in `internal/e2e/ios` exercises the installed
`examples/ios-native` application and saves screenshots in its `.xcresult`.
See [the test instructions](../internal/e2e/ios/README.md) for development
signing and device commands. The background restoration test waits for the
Scene transition to settle before force-terminating the app; an immediate kill
during the Home animation need not deliver a background callback. The regular `go test ./...` suite also covers
touch cancellation, scroll-versus-tap arbitration, rune-based text snapshots
and selection, composition, keyboard dismissal without moving a pressed
control, and renderer release on memory pressure. The demo's navigation test
checks independent tab histories, editing across tab switches, nested Back,
saved editor state, scroll retention and cold navigation restoration. Device
tests also cover interactive edge Back completion/cancellation, keyboard
retention on cancellation, the root guard and restored history after a gesture.
