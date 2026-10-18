# Native iOS extensions, shared files and system actions

These features are opt-in. Applications without `ios.native` or `ios.extensions`
keep the single-host build. No WidgetKit/AppIntents code is called by `App.Run`.
Desktop applications keep their no-cgo build path.

## Packaging native targets

Declare native sources on the host and on each app-extension target:

```json
{
  "name": "Example",
  "identifier": "org.example.app",
  "version": "1.0.0",
  "ios": {
    "buildNumber": "1",
    "appGroups": ["group.org.example.app"],
    "native": {
      "sources": ["ios/Shared"],
      "support": ["actions", "shared-store", "widgets"]
    },
    "extensions": [{
      "name": "Status",
      "extensionPoint": "com.apple.widgetkit-extension",
      "minimumSystemVersion": "18.0",
      "appGroups": ["group.org.example.app"],
      "native": {
        "sources": ["ios/Status", "ios/Shared"],
        "frameworks": ["Foundation"],
        "swiftConditions": ["STATUS_EXTENSION"],
        "support": ["actions", "shared-store", "widgets"]
      }
    }]
  }
}
```

`mygo build -platform ios/arm64` builds, embeds and signs the extension targets.
Use `-ios-simulator` for Apple Silicon simulators. Native sources, resources,
static libraries and helper implementations are copied into the exported
`MyGoHost`; the exported project does not reference the original source tree.
It has no XcodeGen dependency. Internal relative symlinks are retained; external
symlinks are rejected rather than leaving nonportable references. All targets
share the app version/build number.
A higher extension deployment target is allowed; a lower one is rejected.

Native directories compile `.swift`, `.m`, `.mm`, `.c`, `.cc`, `.cpp` and expose
copied `.h`/`.hpp` directories as header search paths. Configuration files in
those directories are not compiled. `resources` adds resource files/folders;
`frameworks` links SDK frameworks, `linkerFlags` adds individual linker arguments,
and `libraries` links prebuilt `.a` files with `-force_load`. The host embeds
required Swift runtime compatibility libraries for native Swift targets; App
Intents is weak-linked on an iOS 15 host, whose intent types must remain guarded
by iOS 16/18 availability annotations. Build those archives
for the chosen SDK/architecture before invoking MyGo; the CLI does not guess Go
core entry points, download native dependencies or run arbitrary extension build
scripts. A host-only `entryPoint` can replace `main.m` for pre-launch registration;
it must ultimately call `mygo_ios_start()` and must not provide another Go entry.

An extension bundle identifier defaults to `identifier + "." + name`. Override
`bundleIdentifier` only with a unique child identifier. Supply extension-specific
`infoPlist` keys (including principal class under `NSExtension`) and entitlements.
Generated identifier, executable, version, build number and extension point are
authoritative. App Groups are explicit per target; matching groups must exist in
that target's provisioning profile. Do not also supply the raw application-groups
entitlement when using `appGroups`.

Automatic signing uses `-ios-team`/`ios.developmentTeam`. Manual signing requires
`ios.signing.exportProfiles[extensionBundleID]` for every extension, in addition
to the host profile. Builds preserve extension dSYMs under `ExtensionSymbols`;
archives preserve them in their normal dSYMs directory. Preflight checks embedded
identifiers, versions, metadata, arm64 binaries and privacy manifests; strict
signature verification covers embedded code. This is local artifact validation,
not validation of App Store approval or provisioning-service availability.

## Shared containers and snapshots

`mygo.AppGroupDirectory("group.org.example.app")` resolves the entitled iOS
container before or after App.Run, from any goroutine. It returns an error on
missing entitlement/access; it never falls back to private storage. The directory
uses protection until the first unlock after boot. Existing files retain their
own protection. Other Go platforms return `mygo.ErrUnsupported`.

The opt-in `sharedstore` package is pure Go and also works with ordinary private
paths. It provides bounded reads, atomic replacement, 0600 files, nonblocking
cross-process locks, versioned JSON snapshots, expiry and explicit migrations:

```go
home, err := mygo.AppGroupDirectory("group.org.example.app")
if err != nil { return err }
store, err := sharedstore.Open(filepath.Join(home, "Status"), sharedstore.Options{})
if err != nil { return err }
defer store.Close()
now := time.Now()
expiry := now.Add(5 * time.Minute).UnixMilli()
return sharedstore.Write(store, "status.json", sharedstore.Snapshot[Status]{
    Version: 1, UpdatedAt: now.UnixMilli(), ExpiresAt: &expiry, Value: status,
})
```

The size limit defaults to 1 MiB and can be set between 1 byte and 64 MiB.
`ReadBytes`/`WriteBytes` preserve an existing application's file format.
`Read[T]` rejects incompatible versions and expired data without returning its
payload. `Update[T]` coordinates read/modify/write, and `Migrate` changes a schema
under that same per-file lock. Failed encoding or migration leaves the previous
generation intact. `TryLock` returns `ErrBusy` rather than waiting on the UI
thread. `TryLockFile` preserves an existing lock-file convention during upgrades.
Keep lock files stable: never delete/replace them. Close a store after all its
operations and lock leases finish. Locks are supported on Apple, Linux, BSD and
Windows; other targets return `ErrLockUnsupported`, while raw reads/replacement
remain available where the filesystem supports them.

A name addresses one file, never a path. Go's `os.Root` confines filesystem access
to the store, including symlink traversal. Atomic replacement protects complete
file generations; use `Update` or a shared lock for read/modify/write to prevent
lost updates. It is not a multi-file transaction or a guarantee against every
power-loss scenario. All App Group members share trust. Store only the minimal
snapshot the extension needs; do not copy all application settings or secrets.

Swift targets selecting `shared-store` receive `MyGoSharedStore` and
`MyGoSharedSnapshot<Value>`. The read-only helper resolves a group/subdirectory,
limits reads and checks schema/expiry. Its envelope matches Go's snapshot; times
are Unix milliseconds. SwiftUI should additionally mark sensitive view content
with `privacySensitive()`. Shared files do not replace keychain access controls.

## Native system actions

Targets selecting `actions` receive `MyGoNativeAction` and
`MyGoActionExecution`. Keep the actual AppIntent/SetValueIntent types in Swift:
this preserves Apple's metadata extraction, parameter UI, availability and
shortcuts integration. Implement the native handler rather than sending work to
a Go application that may not be running:

```swift
@available(iOS 16.0, *)
struct RefreshAccountIntent: MyGoNativeAction {
    static var title: LocalizedStringResource = "Refresh account"
    static var actionGroup: String { "account" }
    static var openAppWhenRun = false
    @MainActor func run() async throws {
        try await AccountService.refresh() // application-owned native service
    }
}
```

The protocol supplies `perform()`, forwarding native errors through App Intents
and rejecting overlapping actions in the same group in the current process.
A boolean action may conform to both `SetValueIntent` and `MyGoNativeAction`;
its `run()` reads the normal `@Parameter var value: Bool`. Prefer explicit
set-to-value operations over toggling stale widget state. Native handlers must
propagate cancellation and coordinate their own I/O/timeouts. This API neither
starts a Go runtime nor grants background time; it is not a cross-process lock.
Use the shared store or an authoritative service for cross-process coordination.

Targets selecting `widgets` receive
`MyGoWidgetRefresh.request(kinds: ["Example.Status"], controls: true)`.
It requests WidgetKit refreshes, optionally including iOS 18 Controls. Actual
refresh timing remains system-scheduled. Keep snapshot timestamps visible and
define stale/missing-data UI in the application.

## Verification

`go test -race ./sharedstore` tests complete generations, bounded/confined files,
subprocess lock contention, migration rollback, expiry and the Swift wire fixture.
`go test ./cmd/mygo` tests extension configuration and portable native staging.
On an Apple Silicon Mac with Xcode, `scripts/test-ios-native-support.sh` runs
native action/cancellation/concurrency checks and type-checks the helpers with the
iOS simulator SDK. A full application build additionally verifies Xcode metadata
extraction, linker inputs, embedding and signing.

See [opt-in runtime services](runtime-services.md) for network paths, cancellable
work/polling, counter sparklines, generation caches and scheduled background refresh.
