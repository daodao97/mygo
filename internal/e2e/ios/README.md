# iOS native UI tests

This standalone XCTest target drives the already installed
`com.mygo.iosnative` example through UIKit accessibility and real touch and
keyboard events. It checks counters, editing, scrolling, saving across
launches, background checkpoints and scroll restoration, cold URL entry,
first-frame handoff, close/quit semantics, foreground activation and rotation.
It also checks immersive backgrounds, five keyboard types and return-key
submission, native alerts/action sheets, sharing completion/cancellation and
clipboard contents, camera permission decisions and Settings entry, missing
permission purpose strings, Dynamic Type preferences, and binary Keychain
storage across process termination (including updates, empty values and deletion). Screenshot
attachments remain in the result bundle. It does not require Bun.

The app navigation test drives the three-tab feature showcase: independent
`ui.Router` stacks, a nested navigation editor, tab switching with an active
keyboard, draft retention, explicit saving and list scrolling, including
restoration after terminating the process. It leaves the installed demo
showing the overview.

`testInteractiveEdgeBackAndRestoration` uses real left-edge drags to verify
cancellation with the editor and keyboard retained, completion through two
page levels, catalog scroll restoration, the root guard and checkpointing of
the changed history across cold launch.

`testChineseScrollingAndNavigation` repeats four rounds of Chinese-list
scrolling, page transitions, Back and tab switching. It checks navigation
under repeated gestures; timing evidence is collected separately and the
test itself does not assert a frame-rate target.

`testSystemTextEditingAndPassword` long-presses Unicode text, selects all,
drags the trailing system handle and verifies that copying uses the changed
Go selection. It then cuts and pastes the entire multiline sample, navigates
Back with the keyboard dismissed, and pastes into a password field while
checking masked accessibility and the absence of copy/cut. Screenshots show
word/multiline selection and handle adjustment. The handle coordinates in
this test target the iPhone 12 mini demo layout.

From the repository root, with Xcode signed in and a paired, unlocked device
in Developer Mode:

```sh
IOS_TEAM=YOUR_APPLE_TEAM_ID
IOS_DEVICE=YOUR_DEVICE_UDID

go run ./cmd/mygo build -platform ios/arm64 -debug -ios-team "$IOS_TEAM" -ios-device "$IOS_DEVICE" examples/ios-native
xcrun devicectl device install app --device "$IOS_DEVICE" 'examples/ios-native/build/ios-arm64/MyGo iOS.app'

xcodebuild -project internal/e2e/ios/Tests.xcodeproj -scheme NativeUITests \
  -sdk iphoneos -destination "id=$IOS_DEVICE" \
  -derivedDataPath .mygo/ios-ui-tests \
  -resultBundlePath ".mygo/ios-ui-$(date +%s).xcresult" \
  -collect-test-diagnostics never DEVELOPMENT_TEAM="$IOS_TEAM" \
  -allowProvisioningUpdates -allowProvisioningDeviceRegistration test
```

The URL-opening tests require iOS 16.4 or later. The device needs an English keyboard installed; the test selects it through
the keyboard switcher when necessary. It appends a unique marker to the
example's message and resets the example through its test deep link. Both
explicit saves and automatic background checkpoints are tested in its sandbox. Rotation is restored
to portrait during teardown. Background assertions accept both running and
suspended apps, then verify persisted content and scroll position after relaunch.
The original three baseline tests passed on iPhone 16 / iOS 26.6.2.
All eight tests passed on iPhone 12 mini / iOS 18.2.1 on 2026-10-07,
with zero failures or skips. The result bundle is
`.mygo/ios-mini-mobile-services-3.xcresult`; screenshots are exported to
`.mygo/ios-mini-evidence-3`.
Chinese IME composition and VoiceOver navigation need separate device tests.

After adding the tabbed demo, all nine tests passed on the same iPhone 12 mini
on 2026-10-07 (zero failures/skips): `.mygo/ios-app-full-1.xcresult`.
Attachments are exported to `.mygo/ios-app-full-evidence-1`.

After the iOS Metal presentation change, all ten tests passed on iPhone 12
mini / iOS 18.2.1 on 2026-10-07 (zero failures/skips):
`.mygo/ios-chinese-final-full.xcresult`. This final app excludes temporary
profiling instrumentation. Attachments are in `.mygo/ios-chinese-final-evidence`.

The functional showcase and interactive Back regressions bring the suite to
twelve tests. All twelve passed on iPhone 12 mini / iOS 18.2.1 on 2026-10-07
(zero failures/skips): `.mygo/ios-feature-final-full-1.xcresult`. Screenshots
are exported to `.mygo/ios-feature-final-evidence-1`. The catalog integration
test invokes real input submission, native alerts, permission queries,
Keychain writes/reads/deletion and immersive light/dark changes through the
new feature pages. After adding the nested-transition gesture guard, the
latest installed package also passed the focused edge Back/restoration test:
`.mygo/ios-feature-final-edge-2.xcresult`.

After moving serialization/restoration into `ui.Router`, the three affected
navigation regressions passed again on the same device (zero failures/skips):
`.mygo/ios-router-owned-navigation-1.xcresult`. They cover the demo without a
history mirror, nested/tab navigation, cold restoration, gesture cancellation
and completion, a further successful edge Back after cold launch, and repeated
Chinese scrolling. Attachments are in `.mygo/ios-router-owned-evidence-1`.

The demo interface now uses English navigation, feature names, instructions
and controls, retaining Chinese as intentional Unicode sample data. On the
same iPhone 12 mini, Chinese scrolling and baseline native UI passed in
`.mygo/ios-demo-english-1.xcresult`. The initial catalog run exposed a
submission result below the visible keyboard viewport. After shortening the
English keyboard/editor instructions, catalog integrations, interactive Back
and tab/cold restoration all passed (three tests, zero failures/skips) in
`.mygo/ios-demo-english-2.xcresult`. Final screenshots are exported to
`.mygo/ios-demo-english-evidence-2`; the submission result and Save State
button remain visible with the keyboard open.

Four rendering regressions extend the suite to sixteen tests:

- `testImageRenderingCases` samples actual device screenshot pixels for
  PNG/JPEG/WebP/EXIF/GIF, all five fit modes, grayscale, transparent PNG
  blending and the colored SVG's mask. It also selects and scrolls the
  virtual 200-item thumbnail grid and exercises invalid image data. The GIF
  fixture contains two animation frames; the supported first-frame decoder
  consistently shows the first one.
- `testLinksAndRichText` taps an inline action and the last fragment of a
  wrapping link, follows a relative route with a query string, returns
  through history/edge Back, handles an unavailable scheme, and opens the
  device's default browser (Safari or Chrome), retaining the page on return.
- `testRemoteImageLoadingAndRetry` enters an unsupported URL, verifies the
  failure, retries the official Go sample over HTTPS, checks the loaded
  image and clears it. The device and MyGo app need network access. On this
  test device, the app's Wireless Data setting was Off and caused DNS
  lookups to fail while Safari could load the same URL. Wi-Fi-only access
  was enabled for this test app; the regression does not modify permissions.
- `testRenderingEffects` samples gradient endpoints, opacity over the page,
  the outside/inside of a rounded clip and custom-drawn bars.

The scroll-to helper uses short drags and checks the whole target is above
fixed bottom tabs, avoiding taps on partially visible links or controls.

On iPhone 12 mini / iOS 18.2.1 (2026-10-07), seven tests passed in
`.mygo/ios-rendering-final-1.xcresult`: image pixels, rendering effects,
links/default-browser return, catalog integrations, Chinese scrolling,
interactive Back, and tab/cold restoration. The network test failed because
keyboard resizing hid the URL field; the demo now reveals it only on focus
or viewport changes, preserving subsequent manual scrolling.

After that fix and extending the HTTP timeout to 30 seconds, the final
installed package passes image rendering and remote failure/retry/clear in
`.mygo/ios-rendering-final-images-network-3.xcresult` (two tests, zero
failures/skips). Screenshots are exported to `.mygo/ios-rendering-final-evidence-3`.
The downloaded sample is visible with its attribution. Temporary request
tracing was removed before this build. These are focused regressions across
the two runs; the full sixteen-test suite was not rerun.

Logo packaging verification (2026-10-07): the standard `icon` setting now
generates the iOS AppIcon asset catalog. The installed iPhone 12 mini icon
was retrieved with `devicectl device info appIcon --allow-placeholder false`
and visually checked (`.mygo/ios-logo-installed-icon.png`). The compiled
bundle has iPhone/iPad primary icon metadata, Assets.car and the launch logo.
Native UI and background/restoration/close regressions both pass in
`.mygo/ios-logo-native-1.xcresult` (two tests, zero failures/skips).

A temporary test plan retained screen recordings for these passing tests.
The cold-launch recording confirms the system logo/title, matching overlay
and fade to native content. Attachments are in `.mygo/ios-logo-evidence-1`;
the extracted launch frame is `.mygo/ios-logo-launch.png`. The original test
scheme was restored and the capture plan moved to `.mygo` after verification.
CLI tests cover app icon selection, square PNG validation, 1024-pixel output
and flattening transparent pixels over white. Desktop CGO-free builds and
affected vet checks also pass.

Ordinary-launch regression (2026-10-07): diagnostic mode is no longer part
of demo checkpoints; old `Diagnostics: true` checkpoints also return to the
feature app. Cold diagnostic restoration explicitly opens
`mygo-native://demo/diagnostics`, which does not reset saved content. Every
test teardown now opens Overview through XCTest's URL API and checks all
three bottom tabs, including when the process is already running.

The new `testNormalLaunchAfterDiagnosticsKeepsBottomTabs` brings the suite
to seventeen tests. On iPhone 12 mini / iOS 18.2.1,
`.mygo/ios-demo-bottom-tabs-1.xcresult` contains three passing focused tests
(zero failures/skips): native UI, background/restoration/close, and ordinary
launch after diagnostics. The last verifies both the preserved Images route
and Overview on subsequent ordinary cold launches, with bottom tabs visible.
Screenshots are in `.mygo/ios-demo-bottom-tabs-evidence-1`. The final installed
app was relaunched normally after testing. Demo Go tests, vet and desktop
cross-builds pass; the full seventeen-test suite was not rerun.

Hidden-scrollbar verification (2026-10-07): all demo page containers,
the virtual thumbnail grid and the navigation editor use `HideScrollbars()`.
Chinese scrolling/navigation and image/grid rendering pass on iPhone 12 mini
in `.mygo/ios-hidden-scrollbars-1.xcresult` (two tests, zero failures/skips).
The Chinese test samples the page's right padding after scrolling to ensure
the thumb no longer darkens it; screenshots are in
`.mygo/ios-hidden-scrollbars-evidence-1`. After applying the same option to
the editor, the final installed package passes tab navigation, input and cold
restoration in `.mygo/ios-hidden-scrollbars-editor-2.xcresult` (one test,
zero failures/skips). It was returned to Overview after verification.

Headless tests verify hidden thumb pixels, clicks reaching content at former
thumb positions, horizontal/vertical wheel and touch scrolling, tracked
offsets, and restoring the thumbs when the option is omitted. UI/demo Go
tests and macOS/Linux/Windows CGO-free builds pass; macOS vet and production
inspector-free vet also pass. These are focused regressions, not a full
seventeen-test rerun.

Catalog-entry verification (2026-10-07): “View All Features” now resets the
Features router to `/features` and the catalog scroll offset to zero instead
of merely selecting the tab and resuming its last detail. The new
`testViewAllFeaturesAlwaysOpensCatalog` brings the suite to eighteen tests.
It exercises a restored nested detail, repeated entries after Navigation,
Dialogs and Images, ordinary tab restoration, and catalog cold restoration.
It and `testTabNavigationAndRestoration` pass on iPhone 12 mini / iOS 18.2.1
in `.mygo/ios-view-all-1.xcresult` (two tests, zero failures/skips). Screenshots
are in `.mygo/ios-view-all-evidence-1`. Headless regressions also cover a
previously scrolled catalog and preservation of another tab and editor draft.
UI/demo Go tests, demo vet and desktop CGO-free builds pass. This is focused
validation; the full eighteen-test suite was not rerun.

Review-fix verification (2026-10-07): persistent-state cancellation now
serializes on the UI thread without holding a lock while waiting for it.
Concurrent old cancellation cannot remove a later registration of the same
key. A subprocess regression bounds the timeout if the deadlock returns.
Ordinary touch interruption on SurfaceBlur cancels its handler and clears
contact, scrolling and pointer presence so later taps are accepted.

Router restoration discovers shared layouts from the first Route.View build,
including existing version-1 and legacy checkpoints. Headless tests cover
nested layouts, query variants, independent history branches, navigation
before the first restored frame, and leaf-only interactive Back after restore.
Full Go tests, focused race tests, desktop CGO-free builds and vet pass.

On iPhone 12 mini / iOS 18.2.1, interactive Back, tab/input/cold restoration
and repeated catalog entry pass in `.mygo/ios-review-fixes-1.xcresult` (three
tests, zero failures/skips). The final package also clears pointer presence
when interrupting an ordinary touch and passes background/checkpoint/closing
verification in `.mygo/ios-review-fixes-final-background-2.xcresult` (one test,
zero failures/skips). Screenshots are in `.mygo/ios-review-fixes-evidence-1`
and `.mygo/ios-review-fixes-final-evidence-2`. The installed app was returned
to Overview. These are four focused device regressions, not a full
eighteen-test rerun.

Text editing extends the suite to nineteen tests. All nineteen pass on
iPhone 12 mini / iOS 18.2.1 on 2026-10-07 (zero failures/skips):
`.mygo/ios-edit-final-full-2.xcresult`. Screenshots are exported to
`.mygo/ios-edit-final-full-evidence-2`. The editing test verifies the changed
Go selection after dragging a native handle, full Unicode cut/paste,
keyboard dismissal on Back and masked password paste with a rune-count
check. The full suite also verifies that selection-proxy relayout does not
cancel interactive Back or restore a stale keyboard. Temporary native
selection/touch tracing is absent from this package.

After the full run, the final package also keeps Go caret anchoring for
custom TextCaret and marked text, with native selection display disabled for
those paths. Text editing/password and interactive Back pass again on the
same device in `.mygo/ios-edit-final-focus-3.xcresult` (two tests, zero
failures/skips). Final screenshots are in
`.mygo/ios-edit-final-focus-evidence-3`; teardown returns the device to
Overview and verifies all three bottom tabs. Real Chinese composition and
custom-input integration still need separate runtime coverage.


## File and photo pickers

`testDocumentExportAndImportRoundTrip` confines export and selection to the
example's own Documents directory, checks Chinese/emoji text after single
and multiple import, and deletes cache copies. `testFilePhotoPickerCancellation`
reuses all five presenters in one flow and covers export-sheet swipe dismissal.
The example enables Files access to its Documents directory and creates only
`mygo-picker-test.txt` and `mygo-picker-fixture.txt` as test resources.

Photo content tests use generated PNGs in a simulator, so device tests do not
select private photos. Seed these fixtures after booting a simulator and
installing the `-ios-simulator` build:

```sh
xcrun simctl addmedia SIMULATOR examples/ios-native/assets/pattern.png examples/ios-native/assets/transparent.png
xcodebuild -project internal/e2e/ios/Tests.xcodeproj -scheme NativeUITests \
  -sdk iphonesimulator -destination 'id=SIMULATOR' \
  -derivedDataPath .mygo/ios-ui-tests-simulator \
  -only-testing:NativeUITests/NativeUITests/testPhotoImportCopiesFromSeededLibrary \
  CODE_SIGNING_ALLOWED=NO test
```

Use the two newly seeded images at the front of the picker, with no other
newer images. The test compares each imported copy's SHA-256 prefix, checks
single/multiple results and cleanup after dismissal. It is explicitly skipped
on a physical device; picker presentation and cancellation run there.

Picker verification (2026-10-07): iPhone 12 mini / iOS 18.2.1 passes both
new document/cancellation tests in `.mygo/ios-picker-final-phone-2.xcresult`
(zero failures/skips). Screenshots: `.mygo/ios-picker-final-phone-evidence-2`.
The iOS 26.1 simulator passes photo single/multiple selection, generated-PNG
hashes and cleanup in `.mygo/ios-picker-photos-simulator-5.xcresult` (zero
failures/skips). Screenshots: `.mygo/ios-picker-photos-evidence-5`.
The simulator's CPU presenter also passes native UI/first-frame/rotation in
`.mygo/ios-picker-photos-simulator-3.xcresult`; that run's photo-test selectors
were corrected in the final photo run. See [the capability audit](../../../docs/mobile-progress.md)
for related physical-device regressions and remaining provider/device coverage.

## Notifications, haptics, gestures and release packaging

The service tests open native feature pages through
`mygo-native://demo/app/feature?name=notifications` (or `haptics`, `gestures`,
`integration`). They verify local immediate/delayed delivery, cancellation,
badges and notification-driven cold navigation; missing APNs entitlement errors;
nine UIKit feedback requests; status visibility and self URL entry; interactive
keyboard focus retention/dismissal; read-only copy without keyboard/cut/paste;
and changed pinch/rotation values followed by reset. Haptic calls do not prove
physical vibration, and a real APNs service/entitled profile is still required
for remote delivery and token-refresh coverage.

`testIPadLayoutAndPresentation` additionally verifies docked keyboard rotation,
draft retention, Back dismissal and action/share popover behavior. It requires
an iPad target; the current coverage is a simulator, not a physical iPad.
Keyboard disappearance waits for UIKit's transition to finish.

Release packaging is tested separately with `-ios-archive
-ios-export-method debugging`, signature verification of the archive app and
exported payload, and installation/regression of the resulting Release app.
Distribution-account methods are configurable; tests do not upload to TestFlight.
See [the capability audit](../../../docs/mobile-progress.md) for exact runs.

Current evidence (2026-10-07): nine iPhone service regressions pass in
`.mygo/ios-services-phone-final-1.xcresult`, four iPad simulator tests pass in
`.mygo/ios-services-ipad-final-1.xcresult`, and the final installed Release app
passes all four packaging regressions in
`.mygo/ios-services-release-final-2.xcresult`. Immediate delivery and both partial
and full keyboard dismissal are included in the last run. Continuous keyboard
reversal/cancellation still needs a separate runtime test; XCTest's public drag
API used here provides a single straight path. Screenshots are exported beside
the runs as documented in the capability audit.


## Composition, hardware keys and biometrics

`testChineseComposition` uses the device's configured Pinyin keyboard and
real software-key taps. It checks that marked text leaves the committed Go
value unchanged, then accepts Chinese candidates and cancels another mark.
It supports full and ten-key Pinyin and restores the originally selected
keyboard. A missing Pinyin keyboard is an explicit skip.

`testHardwareKeyboardEditingAndFocus` sends public XCTest Command-A/Z/Shift-Z,
Tab/Shift-Tab and Command-C events to the physical app. It checks the shared
Go undo/redo history, password trait reload after Tab, and copied contents.
This is native key-event coverage, not a physical Bluetooth/USB keyboard test.

`testSimulatorHardwareEscape` requires interactive host keyboard input. Run
only this test on the owned simulator, temporarily enable **Capture Keyboard**,
and send Escape after the log prints `MYGO_SIMULATOR_AWAITING_ESCAPE`.
The test waits for both Go text focus and the software keyboard to clear;
restore Capture Keyboard after the run. XCTest Escape did not reach UIKit on
the tested iPhone, so that attempt is not counted as device validation.

`testBiometricAvailabilityAndCancellation` records the actual capability and
authentication result. A physical Face ID device may authenticate before the
two-second deadline. An enrolled simulator must time out. The interactive
`testBiometricSimulatorMatchAndBackgroundCancellation` requires Face ID
**Enrolled**, then **Matching Face** after `MYGO_SIMULATOR_AWAITING_MATCH`;
it verifies success followed by cancellation when backgrounded and a fresh
capability query. `testBiometricUnenrolledSimulator` requires **Enrolled** off
and asserts typed `not-enrolled` results. Run these simulator fixture tests
separately, restoring enrollment afterward. They do not enter a device
passcode, alter Keychain access control, or exercise Touch ID.
