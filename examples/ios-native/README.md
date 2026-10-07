# iOS native feature demo

A showcase of MyGo mobile integrations, built entirely with Go native UI.
The fixed bottom tabs are Overview, Features and Status. Each tab has its own
`ui.Router`, header Back button and page history.
No browser or JavaScript is used. The interface is in English; Chinese text
is retained as sample data for typography, input and Unicode storage tests.

The iOS home-screen icon and startup logo use the MyGo template's gradient
ring. `icon` selects `resources/icon.png`; `ios.launchScreen.image` selects
`assets/launch-logo.png`. The static system launch screen and startup overlay
share the logo and colors, then fade into the first presented Go frame.
On iPhone 12 mini / iOS 18.2.1, the installed icon was retrieved without a
placeholder and cold-launch recordings confirmed the logo/overlay handoff.
The packaged example passes both native UI and lifecycle/restoration/close
regressions in `.mygo/ios-logo-native-1.xcresult` (zero failures/skips).

The feature catalog opens working examples of:

- Chinese and emoji layout, long-list scrolling and item selection.
- PNG/JPEG/WebP/GIF-first-frame decoding, EXIF orientation, all five image
  fit modes, grayscale, transparent PNG, colored SVG and a 200-item virtual
  thumbnail grid.
- Network image loading off the UI thread, placeholder, failure, retry and
  clearing; decoded images stay available while navigating in this process.
- Inline and wrapping links, styled text spans, relative router links with
  query strings, system browser handoff/return and unavailable URL handling.
- Gradients, shadows, group opacity, rounded clipping and custom drawing.
- Scene lifecycle, first-presented-frame notification, background checkpoint,
  explicit saving, cold-launch restoration and close/quit semantics.
- Email, number, decimal, phone, URL and one-time-code keyboard hints and
  return-key submission.
- Native selection handles and edit menus, Unicode cut/copy/paste and
  masked password paste (**Text Editing**, iOS 17+ for selection visuals).
- Native alerts, action sheets, sharing and clipboard access.
- Local notification scheduling/cancellation, badges and notification cold entry;
  APNs registration and entitlement errors (**Notifications**).
- Nine UIKit feedback patterns (**Haptics**).
- Interactive keyboard dismissal, read-only system selection/copy and captured
  pinch/rotation (**Input & Gestures**).
- Status bar styles/visibility, incoming URL/document readouts and self deep
  links (**System Integration**).
- Permission queries/requests and the app's system Settings entry.
- Binary Keychain storage, updates, empty values, reading and deletion.
- Face ID/Touch ID availability, authentication, timeout cancellation and
  optional system passcode fallback (**Biometric Authentication**).
- Real IME composition with a committed-value readout, Tab/Shift-Tab focus,
  Escape and shared Command editing/undo shortcuts (**Text Editing**).
- Light/dark immersive backgrounds and accessibility preference readouts.
- Independent tab histories, nested pages, interactive edge Back and editing
  state retained across navigation and process relaunch.

`ui.NewRouter` enables interactive Back on iOS. Drag from the page's left edge
and release beyond halfway, or flick right, to return. A short or reversed
swipe cancels without losing the page, scroll position, draft or keyboard
focus. The root page cannot go back. Vertical gestures still scroll.

The demo keeps its three `*ui.Router` values directly in state registered with
`mygo.PersistState`. Routers serialize and restore their own history and
cursor, including forward entries; the demo has no separate history mirror,
per-frame synchronization or path replay. Pages call `Push` and `Pop` directly,
and the overview deep link uses `Reset`. Existing location-array checkpoints
are accepted by the router. Tab selection, tracked scroll positions and the
navigation editor value are saved alongside the routers.
The Overview button “View All Features” resets the Features router to its
catalog root and scrolls the list to the top. Selecting the bottom Features
tab resumes its remembered page instead. Other tabs and editor drafts are
retained when opening the catalog.
“Save State” copies the editor value to the parent page and explicitly saves the
checkpoint. The state tab also opens the full interaction diagnostics.

```sh
go run ./cmd/mygo build -platform ios/arm64 -debug \
  -ios-team YOUR_TEAM -ios-device YOUR_DEVICE_UDID examples/ios-native
xcrun devicectl device install app --device YOUR_DEVICE_UDID \
  'examples/ios-native/build/ios-arm64/MyGo iOS.app'
```

Deep links:

- `mygo-native://demo/app/home`: show the overview, retaining other tab histories
  and the navigation editor state.
- `mygo-native://demo/app/reset`: reset the feature demo state for testing.
- `mygo-native://demo/app/feature?name=notifications` (also `haptics`,
  `gestures`, `integration`): open a feature with its catalog Back entry.
- `mygo-native://demo/reset` and `/mobile/input/<kind>`: full-screen diagnostic
  views used by the device regression tests.
- `mygo-native://demo/diagnostics`: open diagnostics without resetting saved
  counter, text or scroll data. Cold restoration tests use this explicit entry.

Full-screen diagnostic mode is transient and omitted from app checkpoints,
including when reading checkpoints written by earlier demo builds. Ordinary
launches restore the feature app's selected tab, router and bottom navigation.
Device tests explicitly return to Overview and check all three bottom tabs
during teardown.

Page, thumbnail-grid and editor scrollbars are hidden with
`HideScrollbars()`. Touch scrolling, input caret scrolling and saved positions
remain available; the hidden bar area also allows taps on its content.

See [the device tests](../../internal/e2e/ios/README.md). Desktop development
can run the same example with `go run ./examples/ios-native`.

The network loader is an application example in `rendering.go`, using Go
HTTP and `ui.DecodeBitmap`, with a 30-second timeout, a 4 MB response limit
and at most four million decoded pixels. It is not an image URL API in the
framework. Offline samples are embedded and decoded once. Demo format/fit,
thumbnail selection and downloaded textures are transient; router and page
scroll checkpoints still use registered app state.

The remote sample is the [Go gopher by Renée French](https://go.dev/blog/gopher),
licensed under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/),
shown without modifications apart from display scaling. Its attribution
link is also shown below the downloaded image. Local fixtures are generated
by `go run ./assets/generate.go` from this example directory (Go plus ffmpeg
for WebP encoding, development tooling only).

Rendering showcase verification (2026-10-07): Images, Links & Rich Text
and Rendering are installed on iPhone 12 mini / iOS 18.2.1. Device tests
sample actual screenshot pixels, follow inline/relative/browser links,
select/scroll thumbnails, and exercise network failure, retry and clearing.
Seven relevant regressions passed in `.mygo/ios-rendering-final-1.xcresult`;
the network failure there exposed a keyboard-hidden URL field, now fixed
using `ScrollIntoView` on focus and viewport changes. The final package
passes both image and network tests in
`.mygo/ios-rendering-final-images-network-3.xcresult` (zero failures/skips).
Screenshots are in `.mygo/ios-rendering-final-evidence-3`. This is focused
validation, not a full sixteen-test rerun. Only this app's Wi-Fi access
was enabled during testing. Temporary request tracing is absent from the
final package. Go format/HTTP-limit/keyboard-viewport tests and desktop
CGO-free builds/checks pass.

Text Editing demonstrates long-press selection and the system edit menu.
On iOS 17+, the system caret, highlight and handles follow the Go text
layout. Use **Reset sample**, drag a selection handle, then copy and
**Read editing clipboard** to inspect the result. Cut and paste round-trip
Chinese and emoji; the password field accepts paste and shows only masked
text, with copy/cut unavailable. Both fields scroll into view when focused
or when the keyboard changes the viewport. These sample values are
transient and are not saved in checkpoints.

All nineteen device tests pass on iPhone 12 mini / iOS 18.2.1 in
`.mygo/ios-edit-final-full-2.xcresult` (zero failures/skips), including actual
handle dragging, Unicode clipboard contents and password paste length.
Screenshots are in `.mygo/ios-edit-final-full-evidence-2`.

After the full run, the final package also keeps Go caret anchoring for
custom TextCaret and marked text, with native selection display disabled for
those paths. Text editing/password and interactive Back pass again on the
same device in `.mygo/ios-edit-final-focus-3.xcresult` (two tests, zero
failures/skips). Final screenshots are in
`.mygo/ios-edit-final-focus-evidence-3`; teardown returns the device to
Overview and verifies all three bottom tabs. Real Chinese composition and
custom-input integration still need separate runtime coverage.

Files & Photos demonstrates filtered/single and multiple file import, single
and multiple image selection, export of `mygo-picker-test.txt`, and deletion
of imported copies. Text imports display their contents, including Chinese
and emoji. Export the sample into this app's Documents folder in Files, then
import it again to check the round trip. The demo exposes only its Documents
folder through its Info.plist settings. Imported copies and picker results
are transient; the framework never deletes selected originals. Desktop demo
imports make their own copies before exposing the cleanup button.

The photo picker preserves the provider's image representation (which may
be HEIC); this page reports names and file sizes rather than converting
formats. External folders, videos and persistent document authorization are
not part of this implementation.

Picker verification (2026-10-07): document export, single/two-file import,
Unicode contents, cancellation and cleanup pass on iPhone 12 mini / iOS
18.2.1 in `.mygo/ios-picker-final-phone-2.xcresult` (zero failures/skips).
Photos single/multiple import, fixture hashes and cleanup pass on the iOS
26.1 simulator in `.mygo/ios-picker-photos-simulator-5.xcresult`.
Screenshots are in `.mygo/ios-picker-final-phone-evidence-2` and
`.mygo/ios-picker-photos-evidence-5`. These are focused tests; provider
failures, iCloud/third-party services, HEIC/JPEG and iPad remain untested.
See [the capability audit](../../docs/mobile-progress.md) for regression evidence.

Release archives and local development IPA export use `mygo build -platform
ios/arm64 -ios-team YOUR_TEAM -ios-archive -ios-export-method debugging
examples/ios-native`. See [iOS packaging and service APIs](../../docs/ios.md)
for entitlements, associated domains and distribution export methods.

Service and packaging verification (2026-10-07): all nine focused iPhone
regressions pass in `.mygo/ios-services-phone-final-1.xcresult`; four iPad
simulator regressions pass in `.mygo/ios-services-ipad-final-1.xcresult`. The
final development-signed Release archive/IPA is generated and installed on
“less”. Its four device regressions pass in
`.mygo/ios-services-release-final-2.xcresult`, including immediate notification
delivery without losing the result to the submission callback, native keyboard
dismissal/read-only copying and edge Back. Screenshots are in
`.mygo/ios-services-release-final-evidence-2`. Actual push delivery, domain/AASA
Universal Links, physical iPad and distribution upload remain unverified.


Input/authentication follow-up (2026-10-07): the latest installed Release app
passes nine focused iPhone 12 mini / iOS 18.2.1 regressions in
`.mygo/ios-input-auth-release-final-1.xcresult` (zero failures/skips), including
real Pinyin candidate commit/cancel, native Command undo/redo, Tab/password
trait reload, biometric deadline cancellation, lifecycle, interactive Back,
Unicode selection/password and Keychain cold restoration. Simulator host
Escape and controlled Face ID matching/cancellation/unenrollment have separate
passing runs. See [mobile progress](../../docs/mobile-progress.md) for exact
artifacts and remaining physical keyboard, Scene, accessibility and biometric
policy coverage. The current Release archive/IPA are development signed;
no distribution export or TestFlight upload was performed.
