# Mobile integration and verification

This tracks the complete mobile capability audit requested for integration,
including items that require signing capabilities, services or more devices.
An implementation is complete only with evidence for its public behavior;
building a native bridge alone is not runtime validation.

| Requirement | Implementation | Verification still required |
|---|---|---|
| Navigation: interactive edge back, cancellation, system navigation containers | `ui.Router.InteractiveBack` defaults on for iOS through `NewRouter`; preview/settling, cancellation, shared layouts, Pop/Reset and full JSON history/cursor persistence implemented. Feature demo uses independent per-tab routers; system containers pending | Completion through two levels, cancellation retaining editor/keyboard, root guard, scroll/history cold restoration passed on iPhone 12 mini; system containers pending |
| Input: keyboard types, return actions, autofill, dismissal, selection and edit menus | Keyboard traits, native menus/selection, interactive dismissal and read-only copying implemented; IME snapshots reconcile marked ranges atomically and pin long-document context. Native history and hardware edit/focus commands now route through Go | Real Pinyin candidate commit/cancel passed on iPhone 12 mini; long-context/Unicode/history pass headless tests. Keyboard types, selection/handles and masked paste have earlier iPhone coverage; Command editing/undo and Tab/Shift-Tab/password trait reload passed with public XCTest native key events on iPhone; actual host Escape clears focus/keyboard on the iPad simulator. Physical keyboard, button Return/Space, autofill, alternate IMEs and older-iOS fallback remain pending |
| System dialogs, action sheets, sharing, files/export, photo picker | Native alerts/actions and text/URL/file sharing plus file import, photo selection and existing-file export implemented; persistent external-directory access pending | Alert/actions/share-copy and cancellation passed on iPhone 12 mini; file-picker verification below. Photo single/multiple/copy integrity and cleanup pass with generated PNGs in the simulator; URL/file shares, iCloud/third-party providers pending; action/share anchoring verified on iPad simulator |
| Permissions: query/request/settings | Camera/microphone/location/photos/notifications query and request plus settings entry implemented | Camera grant/deny/re-request, missing microphone purpose, settings round trip passed; other resources and restricted/limited states pending |
| Local/remote notifications, badges and notification launch | Local immediate/delayed notifications, cancellation, badges and typed foreground/cold-response events implemented; APNs registration/token/error bridge exists | Local delivery/cancel/cold entry verified; entitlement-error callback verified. Real remote APNs delivery/token refresh requires push capability and a delivery service |
| Background scheduling, transfers, audio | Bounded state checkpoint exists; worker services pending | OS expiration, cancel, process relaunch, transfer data integrity |
| Keychain and biometrics | Binary device-local Keychain storage plus standalone Face ID/Touch ID query/authentication, typed errors, optional system-passcode fallback and context/background/disconnection cancellation implemented; Keychain biometric access-control policies pending | Keychain binary/empty/update/delete/cold restoration has iPhone coverage. Face ID query/success/timeout passed on iPhone 12 mini; matching, timeout/background cancellation and unenrollment errors passed on iPad simulator. Touch ID, passcode entry, locked-device/lockout policy and biometric-protected Keychain items remain pending |
| Haptics, camera/microphone, location and other device services | Selection, five impact styles and success/warning/error feedback implemented; capture/location services remain pending | Nine UIKit feedback requests succeed on iPhone; physical sensation cannot be asserted by XCTest. Capture/location/interruption remain pending |
| Multi-touch pinch/rotation and mobile gestures | Primary touch, scroll, interactive router edge Back and opt-in captured pinch/rotation events implemented | Changed scale/rotation and reset verified on iPhone; extended gestures remain pending |
| Safe area/keyboard geometry, orientation, status bar control | Automatic safe area/background/contrast, explicit status text/visibility and docked/floating keyboard geometry implemented | Status visibility visually checked on iPhone; iPad simulator rotation/docked keyboard tested. Floating keyboard and physical iPad remain unverified |
| Accessibility preferences and navigation | Dynamic Type, contrast and Reduce Motion bridged, including notifications; basic tree/actions exist | Dynamic Type passed on iPhone; live settings changes, contrast/motion and VoiceOver navigation/actions pending |
| Native packaging: icons, entitlements, Archive/IPA | Development signing/build/install, icons, entitlements/build number and numeric CLI override, Release Archive/local IPA, privacy merging, Go/native dSYMs, artifact checks, manual signing and unsigned/signed packaging CI implemented | Development and automatic App Store IPA exports, unsigned archive, simulator Debug and Go source-address resolution verified locally. Real manual signing and remote CI execution remain unverified. Uploads, TestFlight and remote build management are application-owned |
| Universal Links and document integration | URL/file events plus associated-domain entitlements and standard/custom document UTI packaging implemented; persistent file access pending | Configured self URL tested; document registrations inspected in the built app. Associated-domain OS delivery requires an actual domain/AASA/profile; security-scoped lifetime remains pending |
| Chinese IME, Scene reconnection and device coverage | Real Pinyin and headless long-context snapshots covered; disconnection commits input and clears active focus without an extra background → inactive → background transition | Actual OS Scene disconnection/reconnection, VoiceOver, physical iPad and minimum supported iOS remain unverified |
| Chinese content scrolling and transitions | iOS Metal uses asynchronous command-buffer presentation and three drawables; desktop transactional presentation remains | Four-round iPhone 12 mini comparison: GPU-path frame processing median 31.9 → 5.1 ms, p95 33.9 → 18.6 ms; remaining slow frames and other devices still need profiling |
| Images, rich text and rendering | English Images, Links & Rich Text and Rendering demo pages exercise existing native UI; bounded asynchronous HTTP loading is an app example | Real iPhone pixels cover formats, EXIF, fitting, grayscale, alpha, SVG masks, gradients, opacity and clipping; grid, inline/router/browser links and remote retry pass. Animated image playback, zoom and reusable network caching remain pending |
| Android native UI | Pending | Native build/install/input/lifecycle/rendering on Android devices |
| iPad multiple Scenes | Single Scene only | Independent windows/state, disconnection/reconnection and multitasking |
| Mobile system WebView | Native Go content only | Navigation, IPC, lifecycle, security, keyboard and permissions |

Evidence from earlier work: `.mygo/ios-immersive-tests.xcresult` contains
three successful iPhone 16 tests covering native UI, lifecycle/restoration,
close semantics and immersive backgrounds. This is a baseline, not proof
for the missing integrations above.

New device coverage (2026-10-07): iPhone 12 mini, iOS 18.2.1 (22C161),
375 × 812 point viewport. `.mygo/ios-mini-mobile-services-3.xcresult`
contains all eight tests passing, with no failures or skips: native UI,
background restoration/close semantics, immersive backgrounds, keyboard
types/submission, alerts/actions/sharing, Dynamic Type, camera permissions
and Settings, and binary Keychain cold restoration. Attachments are exported
to `.mygo/ios-mini-evidence-3`. This run uses the new device exclusively.

The run exposed shared implicit scroll state between the example's demo
and service pages; independent page keys now prevent restored demo offsets
from hiding the service heading. XCTest background waits now accept both
running and suspended background states while retaining the relaunch
checkpoint assertions. Share cancellation uses the visible system close
button, with an on-screen dismissal fallback, rather than the oversized
accessibility dismissal frame. Minimum supported iOS, iPad, Chinese IME
composition and VoiceOver remain unverified.

Earlier tabbed demo verification (2026-10-07, before the functional showcase): `.mygo/ios-app-full-1.xcresult`
contains all nine iPhone 12 mini tests passing without failures or skips.
The new navigation test exercises independent per-tab `ui.Router` histories,
detail/edit/back, draft retention across tab switches, favorites/notes,
and cold restoration of the selected page, parent history and list scroll.
It also verifies that a tab tap with the keyboard visible completes once,
after deferring keyboard deactivation until touch release. Go tests passed
(696 tests across 46 packages); macOS/Linux/Windows builds and vet with
`CGO_ENABLED=0`, and production inspector-free vet passed.

Chinese-content performance verification (2026-10-07, on the earlier demo layout):
`.mygo/ios-chinese-final-full.xcresult` contains all ten iPhone 12 mini tests
passing, with no failures or skips, including the new four-round Chinese
scrolling/navigation regression. This final app has no temporary profiler or
frame-capture code. Evidence is exported to `.mygo/ios-chinese-final-evidence`;
the timing comparison is `.mygo/ios-chinese-comparison.json`. Go tests,
desktop `CGO_ENABLED=0` builds/vet for macOS/Linux/Windows and production vet
passed. The headless navigation test disables transitions so its state and
click assertions do not depend on wall-clock animation progress.

Functional showcase verification (2026-10-07): the demo now has overview,
feature-catalog and runtime-state tabs, with working input, lifecycle,
dialog/share, permission, Keychain, appearance and navigation integrations.
`.mygo/ios-feature-final-full-1.xcresult` contains all twelve iPhone 12 mini /
iOS 18.2.1 tests passing, with zero failures or skips. In addition to the
existing service regressions, this run exercises the new feature entrances,
real email submission, alert cancellation, permission query results, binary
Keychain read/write/delete and light/dark immersive backgrounds. It also
covers interactive Back completion through two levels, cancellation keeping
the editor and keyboard, the root guard and gesture-history cold restoration.
Attachments are exported to `.mygo/ios-feature-final-evidence-1`.

A subsequent guard prevents starting edge Back while a nested `Route.View`
transition is still active. Shared-layout, nested-transition, velocity,
reversal, cancellation and Reduce Motion behavior pass headless tests.
The latest package was rebuilt and installed; its focused physical-device
edge Back/restoration test passes in `.mygo/ios-feature-final-edge-2.xcresult`.
All Go tests, desktop `CGO_ENABLED=0` builds/vet for macOS/Linux/Windows,
production inspector-free vet and `git diff --check` pass on the final source.

Router-owned history verification (2026-10-07): routers now serialize and
restore complete history and the cursor themselves, including forward
entries. The demo holds `*ui.Router` values directly in its checkpoint, calls
Push/Pop/Reset, and has no separate history mirror, per-frame synchronization
or manual path replay. Gesture completion uses the same Pop/Go path. Legacy
location-array checkpoints remain readable. Headless tests cover duplicate
entries, query sharing, forward branches, history bounds, invalid/atomic
restoration, initialized configuration/host retention, Reset, gesture
checkpoints and migration of the earlier demo format.

`.mygo/ios-router-owned-navigation-1.xcresult` contains three passing iPhone
12 mini tests, with zero failures/skips: tab navigation and cold restoration,
interactive Back cancellation/completion/restoration (including another
successful gesture after cold launch), and repeated Chinese scrolling and
navigation. Attachments are in `.mygo/ios-router-owned-evidence-1`; the copied
device checkpoint is `.mygo/ios-router-owned-checkpoint-1.json`. All Go tests,
macOS/Linux/Windows `CGO_ENABLED=0` builds/vet, production inspector-free vet
and `git diff --check` passed. This is focused validation of the history
refactor; the earlier twelve-test showcase run is recorded above.

Rendering showcase verification (2026-10-07): the new Images, Links & Rich
Text and Rendering pages run on iPhone 12 mini / iOS 18.2.1. Seven tests
passed in `.mygo/ios-rendering-final-1.xcresult`, covering image/rendering
pixels, links and browser return, feature integrations, Chinese scrolling,
interactive Back and tab/cold restoration. Its network test exposed a URL
field hidden by the keyboard. Focus/viewport changes now reveal that field
with `ScrollIntoView`, without pinning subsequent manual scrolling.

The final installed package passes both image rendering and network
failure/retry/clear tests in `.mygo/ios-rendering-final-images-network-3.xcresult`
(two tests, zero failures/skips); screenshots are in
`.mygo/ios-rendering-final-evidence-3`. Wi-Fi-only access was enabled for this
test app after its Wireless Data setting was found Off. Request tracing
confirmed successful Go HTTPS/TLS downloads after some address connection
timeouts; the loader now allows 30 seconds. Temporary diagnostics are absent
from the final package. Format decoding, HTTP limits/errors and keyboard
viewport/manual-scroll behavior pass Go tests; desktop CGO-free checks pass.
This is focused validation across these runs, not a new full sixteen-test run.

Logo verification (2026-10-07): the CLI now packages `icon` through an
AppIcon asset catalog; the demo also configures its launch-screen image.
The iPhone 12 mini returned the installed gradient-ring icon without a
placeholder (`.mygo/ios-logo-installed-icon.png`). Cold-launch video confirms
the logo/title and overlay fade into native content. Two native UI and
lifecycle/restoration/close regressions pass in `.mygo/ios-logo-native-1.xcresult`,
with no failures/skips. Evidence is in `.mygo/ios-logo-evidence-1` and the
launch frame is `.mygo/ios-logo-launch.png`.

Ordinary demo launch verification (2026-10-07): full-screen diagnostic mode
is transient and ignored in older checkpoints. Tab selection, router history
and content still restore normally. UI tests open diagnostics explicitly for
cold diagnostic restoration and return the device to Overview during teardown,
asserting all three bottom tabs. Three focused tests pass on iPhone 12 mini /
iOS 18.2.1 in `.mygo/ios-demo-bottom-tabs-1.xcresult`: native UI, background
restoration/close, and ordinary launch after diagnostics. The new regression
checks restored Images and Overview with bottom navigation on cold launch.
Evidence is in `.mygo/ios-demo-bottom-tabs-evidence-1`.

Hidden-scrollbar verification (2026-10-07): `Element.HideScrollbars()` hides
both scrollbar drawing and pointer targets while retaining content scrolling
and checkpoints. The demo applies it to pages, the virtual grid and editor.
On iPhone 12 mini, Chinese scrolling with margin pixel checks and image/grid
regressions pass in `.mygo/ios-hidden-scrollbars-1.xcresult` (two tests).
The final package's editor/tab navigation and cold restoration pass in
`.mygo/ios-hidden-scrollbars-editor-2.xcresult` (one test). Both runs have zero
failures/skips. Headless coverage includes thumb pixels, underlying clicks,
wheel/touch scrolling and re-enabling the bars; desktop CGO-free checks pass.

Text editing verification (2026-10-07): built-in editors use UIKit selection
highlights/handles on iOS 17+ with Go caret/selection/hit geometry and shared
Go clipboard commands. Visible-paragraph queries retain text-area
virtualization; stale fields, passwords and composition reject native
selection geometry. Custom TextCaret handlers retain their own selection.
Selection proxy relayout no longer emits synthetic resize notifications,
which would cancel interactive Back.

On iPhone 12 mini / iOS 18.2.1, all nineteen tests passed with zero
failures/skips in `.mygo/ios-edit-final-full-2.xcresult`. The new test checks
word/multiline selection, trailing-handle dragging that changes copied Go
text, complete Chinese/emoji cut/paste, Back with keyboard dismissal and
password paste without copy/cut or exposed text. Screenshots are in
`.mygo/ios-edit-final-full-evidence-2`. Full Go tests, focused geometry race
tests and desktop CGO-free builds/vet pass. Chinese IME composition,
hardware-keyboard actions and runtime coverage below iOS 17 remain pending.

After the full run, the final package also keeps Go caret anchoring for
custom TextCaret and marked text, with native selection display disabled for
those paths. Text editing/password and interactive Back pass again on the
same device in `.mygo/ios-edit-final-focus-3.xcresult` (two tests, zero
failures/skips). Final screenshots are in
`.mygo/ios-edit-final-focus-evidence-3`; teardown returns the device to
Overview and verifies all three bottom tabs. Real Chinese composition and
custom-input integration still need separate runtime coverage.

File/photo picker verification (2026-10-07): the final installed package
passes both document export/import round-trip and all five picker cancellation
flows on iPhone 12 mini / iOS 18.2.1 in
`.mygo/ios-picker-final-phone-2.xcresult` (zero failures/skips). Single and
two-file imports preserve Chinese/emoji contents after dismissal; cleanup
removes three cache copies. Export-sheet swipe dismissal returns cancellation.
Screenshots are in `.mygo/ios-picker-final-phone-evidence-2`.

The same installed package also passes native alerts/sharing, text editing
and password paste, and interactive edge Back in
`.mygo/ios-picker-final-phone-1.xcresult`. That earlier run's document test
failed because the test expected a Select button in a picker already in
selection mode; the corrected round trip passes in the final run above.
These are focused regressions, not a complete rerun of the mobile suite.

Photo single/multiple import and cleanup pass on the iOS 26.1 simulator in
`.mygo/ios-picker-photos-simulator-5.xcresult` (zero failures/skips), with
SHA-256 prefixes matching both generated PNG fixtures after the provider's
temporary URLs have expired. Screenshots are in
`.mygo/ios-picker-photos-evidence-5`. Simulator native UI, first-frame handoff
and rotation also pass in `.mygo/ios-picker-photos-simulator-3.xcresult`;
its earlier photo test was corrected for remote picker accessibility.
The simulator uses the CPU presenter; the SDK lacks Metal drawable
presentation callbacks. Physical devices retain Metal rendering.

No private photos were selected on the device. iCloud/third-party providers,
HEIC/JPEG photo representations, provider download failure and iPad still
need runtime coverage. Persistent external-folder authorization and video
selection remain unimplemented. Go tests/race checks, desktop CGO-free
builds/vet on macOS/Linux/Windows, and production inspector-free vet pass.

Mobile services verification (2026-10-07): the English catalog adds Notifications,
Haptics, Input & Gestures and System Integration. Local requests retain typed
string data through foreground delivery and notification-triggered cold launch.
APNs registration reports tokens/errors through UI-thread listeners; the demo
profile intentionally has no push entitlement, so the device test verifies the
registration error rather than remote delivery. Associated-domain entitlements
and standard/custom document registration are configurable in packaging.

On iPhone 12 mini / iOS 18.2.1, all nine focused tests pass without failures or
skips in `.mygo/ios-services-phone-final-1.xcresult`: notification scheduling,
cancellation and badge calls; notification cold routing/data; nine haptic calls
and status visibility/self URL entry; pre-edge keyboard focus retention and
interactive dismissal with retained draft; read-only Unicode copying without keyboard,
cut or paste; changed pinch/rotation values; APNs entitlement errors; native
UI/rotation/first-frame; Unicode editing/password; and interactive edge Back
and cold history restoration. Screenshots are in
`.mygo/ios-services-phone-final-evidence-1`.

All four tests pass on the iPad Pro 11-inch iOS 26.1 simulator in
`.mygo/ios-services-ipad-final-1.xcresult`: interactive keyboard/read-only copy,
changed pinch/rotation values, notification cold entry/data, and orientation,
draft retention, Back keyboard dismissal and action/share popover anchoring.
Screenshots are in `.mygo/ios-services-ipad-final-evidence-1`. The simulator uses
the CPU presenter and has been shut down after testing. This is not physical
iPad or floating-keyboard validation.

The input regressions caught two framework defects: the surface's UIView
gesture gate rejected ancestor scroll pans, and selectable Text excluded
native selection changes from its read-only editor queue. Ancestor gestures
now retain UIKit arbitration, and read-only selection/geometry/copy pass both
headless and runtime checks. UIKit scroll pans retain Go's content offsets and
momentum; keyboard disappearance assertions wait for UIKit's transition.

Full Go tests and focused gesture/read-only/keyboard race tests pass. Desktop
`CGO_ENABLED=0` build/vet for macOS, Linux and Windows, production inspector-free
vet and `git diff --check` pass. Remote APNs delivery/token refresh, actual
Universal Link domain delivery, persistent external-file authorization, physical
iPad, Chinese IME/hardware keyboard/VoiceOver, distribution-signed export and
TestFlight upload still require separate coverage. Multi-Scene iPad windows
and mobile WKWebView remain unimplemented.

The stronger follow-up keyboard test crosses the keyboard edge before release,
checks native completion and retained draft, reopens input, then completes a full
drag. It passes on the iPad simulator in
`.mygo/ios-keyboard-partial-ipad-2.xcresult`. Earlier tests only checked that a
short drag above the keyboard retained focus. UIKit cancellation requires pulling
the active drag upward before releasing; a continuous reversal still lacks a
runtime test, and the demo now describes the system behavior accurately.

Final Release packaging verification (2026-10-07):
`.mygo/ios-services-archive-final-3.log` records a development-signed Release app
(11.3 MB), `.xcarchive` (12.3 MB) and local IPA (4.6 MB). Both the archived app
and extracted IPA payload pass strict signature verification. The IPA's bundle
identity, build number and plain-text document registration match the app; the
ZIP passes CRC checks. Outputs remain in `examples/ios-native/build/ios-arm64`.

The final Release app is installed on the new iPhone 12 mini (“less”). All four
regressions pass without failures/skips in
`.mygo/ios-services-release-final-2.xcresult`: native UI/rotation/first-frame;
interactive edge Back and history restoration; pre-edge focus retention plus
partial/full keyboard dismissal and read-only copying; and immediate/delayed
notifications, cancellation and badge calls. Screenshots are in
`.mygo/ios-services-release-final-evidence-2`. The immediate test caught a demo
race where submission completion overwrote an earlier foreground-delivery
status. Request versions and status guards now retain delivery/open status and
ignore stale submission replies. The app returns to Overview with all three
bottom tabs. Full Go tests and desktop CGO-free builds pass after this fix.
No distribution-signed export or upload was performed.


Input and authentication follow-up (2026-10-07): marked-text snapshots now
reconcile committed context and composition in one Go frame, retain a pinned
surrounding-text window until commit/cancel, and change only differing runes.
Headless tests cover Unicode, incremental candidates crossing the ordinary
context boundary, cancellation, undo and native undo/redo availability.
UIKit's undo manager and hardware commands use the shared Go history;
Tab changes reload secure/read-only traits after synchronizing the new field.
Scene disconnection commits pending input and clears active focus without
repeating an already completed inactive transition. Actual OS reconnection
remains unverified.

`Biometrics.Query` and context-cancelable `Biometrics.Authenticate` add standalone
Face ID/Touch ID capability checks, typed results, optional system device
passcode fallback and exact-once background/disconnection cancellation. Native
preflight runs off the UI thread, each authentication uses a fresh context,
and late callbacks cannot complete a newer request. This does not add
biometric access-control policies to Keychain storage. The English catalog
adds Biometric Authentication and an IME committed-value readout. Overview
quick links locate features by stable paths instead of catalog indices.

The development-signed Release app is installed on the new iPhone 12 mini
(“less”) / iOS 18.2.1. All nine focused tests pass with zero failures/skips in
`.mygo/ios-input-auth-release-final-1.xcresult`: real Pinyin marked text,
candidate commits and cancellation; Command editing/undo/redo and Tab/
Shift-Tab/password input/copy; biometric query and deadline cancellation;
background checkpoints and close semantics; interactive edge Back/history
restoration; partial/full keyboard dismissal and read-only copying; binary
Keychain update/cold restoration; native UI/rotation/first frame; and Unicode
selection/handle dragging/cut/paste/password masking. The final biometric
branch was `Authentication timed out`; an earlier individual test verifies
real Face ID success in `.mygo/ios-input-auth-phone-4.xcresult` (that mixed run
also contains an obsolete hardware-test failure, and is not a passing suite).
Screenshots are exported to `.mygo/ios-input-auth-release-final-evidence-1`.
Teardown verifies Overview and all three bottom tabs.

On the owned iPad iOS 26.1 simulator, actual host Escape input clears both Go
text focus and the software keyboard in
`.mygo/ios-hardware-escape-simulator-final-1.xcresult` (one test, no failures/skips).
Public XCTest Escape did not reach UIKit on the physical phone, so those
attempts are not counted as physical-keyboard evidence. Enrollment-controlled
biometric matching, timeout and background cancellation pass in
`.mygo/ios-auth-simulator-3.xcresult` (two tests, no failures/skips); unenrollment
errors pass in `.mygo/ios-auth-unenrolled-1.xcresult` (one test, no failures/skips).
Simulator enrollment and keyboard capture were restored and the simulator
was shut down. Continuous keyboard drag reversal, physical keyboards,
Touch ID, passcode entry/lockout, alternate IMEs, VoiceOver, actual Scene
reconnection, physical iPad and minimum supported iOS remain pending.

`.mygo/ios-input-auth-archive-final-1.log` records the latest Release app
(11.3 MB), archive (12.4 MB) and local debugging IPA (4.6 MB), in
`examples/ios-native/build/ios-arm64`. App, archived app and exported payload
pass strict signature verification; the IPA passes ZIP CRC and metadata/
Face ID purpose checks. No distribution-signed export or upload was performed.
Temporary native hardware tracing is absent. Full Go tests, focused race
checks, desktop CGO-free builds/vet for macOS/Linux/Windows, production
inspector-free vet and whitespace checks pass. These are focused mobile
regressions, not a rerun of every historical device test.

Next implementation priorities: camera/location services with cancellation
and lifecycle handling, persistent security-scoped directory access, and
biometric-protected Keychain policies. Distribution signing/TestFlight,
APNs delivery and Universal Links require the corresponding account/service
configuration; device/accessibility coverage remains a separate test track.


## Build/release toolchain follow-up (2026-10-07)

The iOS release boundary is native local packaging. Per the user's clarified
scope, App Store Connect uploads/TestFlight integration and dependencies on
external release CLIs are outside MyGo. The initial asc integration was
removed before completion; application release pipelines can use standard
MyGo artifacts with any tool independently.

| Item | Current state | Verification / boundary |
|---|---|---|
| Privacy manifest | Root manifest packages framework/runtime reasons and merges app/dependency APIs, data collection and tracking declarations; schema/reason checks and direct import checks implemented | Broader SDK/runtime-call auditing is application-specific; Apple's server validation belongs to its release pipeline |
| Source/crash symbols | Go/native DWARF retained; Debug disables optimization/inlining; Release strips installed app symbols after dSYM generation; UUID and actual Go source-line resolution enforced | Matching dSYMs included with local apps/archives; real production-crash coverage remains unverified |
| Signing and release preflight | Automatic development signing, manual identity/profile/export mapping, privacy/metadata/signature/UUID and App Store-profile/entitlement checks implemented | Development and automatic App Store exports verified locally; manual signing remains unverified |
| Native packaging | App, generated Xcode project, `.xcarchive`, local IPA export and reports | Development and App Store IPAs verified locally; no App Store upload operation in MyGo |
| Upload and TestFlight | Application-owned release process | No ASC client, credential store, asc dependency or TestFlight management commands |
| Build numbering | Independent configured number plus numeric `-ios-build-number` override without changing the config file | CI may supply a counter; remote uniqueness/reservation is application-owned |
| CI | Unsigned archive/simulator workflow and manual signed archive/IPA workflow preserving reports/archives/dSYMs | Remote execution and signed job need repository/Apple signing configuration; no upload steps |
| Local environment / development tools | `mygo ios doctor` checks SDKs, signing, symbols and device utilities; manual devicectl/simctl usable | Integrated mobile dev/debug commands remain a separate development track, outside this packaging follow-up |

`mygo ios check` checks an app, archive or safely extracted IPA and emits text
or JSON. Builds automatically check the app and exported IPA before replacing
the prior output; `BuildReport.json`/`ExportReport.json` keep evidence. Old
archives without privacy or Go symbols fail, and development-profile builds
fail `-distribution` instead of being treated as upload-ready.

The latest development-signed Release archive/debugging IPA is in
`.mygo/ios-toolchain-p0-final/ios-arm64`; build log
`.mygo/ios-toolchain-p0-final-2.log`. The app is 11.3 MB, archive 32.7 MB (including
source symbols), IPA 4.6 MB. Both reports prove strict signatures, matching
UUIDs and `main.main` resolved to `main.go:132`. Independent IPA checks pass in
`.mygo/ios-toolchain-p0-ipa-check-2.json`. Unsigned Release and simulator Debug
checks pass in `.mygo/ios-toolchain-p0-unsigned-check-2.json` and
`.mygo/ios-toolchain-p0-simulator-check-1.json`; these mirror local CI commands.
CI has not been pushed/run remotely. Rejection evidence is
`.mygo/ios-toolchain-p0-old-rejected-1.log` and
`.mygo/ios-toolchain-p0-distribution-rejected-2.log`.

Full Go tests and CLI tests pass, as do desktop CGO-free builds/vet on macOS,
Linux and Windows and production inspector-free vet. Focused tests cover
privacy merging/validation, manual-profile configuration, path traversal,
symlinks, ambiguous IPA payloads, size bounds and CRC failures. See
[the toolchain guide](ios-toolchain.md) for configuration and commands.
No App Store upload or TestFlight assignment has occurred. Uploads, remote
numbering and TestFlight management are intentionally left to the application
release process; MyGo native packaging does not require ASC credentials.

The final stripped Release app is installed on the new iPhone 12 mini “less”.
Native UI/startup/first-frame/rotation and biometric availability/deadline
cancellation both pass in `.mygo/ios-toolchain-p0-phone-2.xcresult` (two tests,
zero failures/skips). Screenshots are exported to
`.mygo/ios-toolchain-p0-phone-evidence-2`. Teardown verifies Overview and all
three bottom tabs. No distribution or upload action was performed.

Native-only packaging verification after removing asc integration (2026-10-07):
`-ios-build-number 73` produces an unsigned Release archive in
`.mygo/ios-toolchain-native-unsigned/ios-arm64`; its independent check passes in
`.mygo/ios-toolchain-native-unsigned-check-1.json`. Development-signed archive
and debugging IPA build 74 are in `.mygo/ios-toolchain-native-ipa/ios-arm64`
(build log `.mygo/ios-toolchain-native-ipa-1.log`): app 11.3 MB, archive 32.7 MB,
IPA 4.6 MB. The exported IPA passes all independent checks in
`.mygo/ios-toolchain-native-ipa-check-1.json`, including signatures, privacy,
UUID matching and `main.main` resolved to `main.go:132`. The project's
configuration hash remains unchanged after both builds. These are local
packaging checks; the newly numbered app was not installed on the phone.

The current `ios doctor` passes all ten native checks, with no publishing-tool
check, in `.mygo/ios-toolchain-native-doctor-1.json`. Focused CLI tests and full
Go tests pass in `.mygo/ios-toolchain-native-cli-1.log` and
`.mygo/ios-toolchain-native-go-test-1.log`; desktop CGO-free builds/vet for
macOS/Linux/Windows and production inspector-free vet also pass. Both CI
workflows pass actionlint, and the signed job's five shell/four Python blocks
pass syntax checks. The signed CI has only local archive/IPA export and GitHub
artifact preservation; it has not been executed remotely. The task-local asc
executable/reference checkout was removed, and there is no asc code, command,
API credential or dependency in MyGo's iOS toolchain. At that verification,
distribution identity count was zero and App Store signing was unverified.

App Store signing follow-up (2026-10-07): build 75 exports a separate
distribution IPA in `.mygo/ios-app-store-75/ios-arm64/Export/MyGo iOS.ipa`.
Xcode automatic signing succeeds after logging in to the same personal team
that owns the app record. The prior build 74 debugging IPA remains a
development artifact and cannot be uploaded as a distribution build.
The App Store export and independent distribution check pass in
`ExportReport.json` and `.mygo/ios-app-store-75-check.json`. A separate
certificate comparison in `.mygo/ios-app-store-75-signature/verification.json`
confirms that the actual Apple Distribution signing certificate is present
in the embedded App Store profile, with matching team/application identifiers,
`get-task-allow=false`, `beta-reports-active=true` and no device restriction.
This build uses a task-local copy of the demo with its export-compliance
declaration; it does not put an account/team dependency in the framework or
change the example's configuration. Build/source logs are in
`.mygo/ios-app-store-75-login-retry.log` and `.mygo/ios-store-source-75`.
This verifies native local distribution packaging, not Apple's acceptance or
TestFlight processing of the new IPA; those remain in the app's release flow.

Protected-resource purpose-string follow-up (2026-10-07): Apple's validation
reported ITMS-90683 for build 75's missing location usage description. The demo
configuration now includes English usage descriptions for location while in
use, microphone, photo library, add-only photo access, camera and Face ID.
Distribution preflight checks these known direct API imports against the
compiled app's Info.plist and rejects missing, blank, incorrectly typed or
overlong purpose strings. It does not supply generic descriptions for other
applications or replace review of third-party SDKs/localizations.
The old build is rejected in `.mygo/ios-purpose-build-75-rejected.json`.
Build 76's new App Store IPA is in
`.mygo/ios-app-store-76/ios-arm64/Export/MyGo iOS.ipa`; all ten independent
distribution checks pass in `.mygo/ios-app-store-76-check.json`. The actual IPA
purpose strings and profile/signing-certificate match are independently
verified in `.mygo/ios-app-store-76-signature/verification.json`.
CLI tests pass in `.mygo/ios-purpose-cli-tests.log`, and CLI builds with cgo
disabled pass for macOS, Linux and Windows, along with native CLI vet.
The new IPA still needs Apple's upload validation and server processing.
