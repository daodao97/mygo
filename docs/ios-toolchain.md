# iOS build and release tooling

MyGo builds native Go UI for iOS using macOS, Xcode and a Go c-archive. Desktop
builds continue to use `CGO_ENABLED=0`. App Store Connect export is local;
building never uploads a build or assigns it to TestFlight testers.

The iOS toolchain boundary is native local packaging: app bundles, generated
Xcode projects, archives, IPA exports, signing, privacy and source symbols.
MyGo has no App Store Connect client, credential store or dependency on asc,
Fastlane or Transporter. Each application chooses its own upload/TestFlight
tools and consumes these standard artifacts.

## Privacy declarations

Every generated host includes `PrivacyInfo.xcprivacy` in its root bundle.
MyGo declares the runtime's interval/timer use of `mach_absolute_time`
(`35F9.1`) and file metadata inside the app container (`C617.1`) or files
explicitly selected by the user (`3B52.1`). App and dependency declarations
are merged by API category and collected-data type. Reasons and purposes are
deduplicated; collection/linking/tracking declarations are preserved.

```json
{
  "ios": {
    "privacy": {
      "manifests": ["vendor/PrivacyInfo.xcprivacy"],
      "accessedAPIs": [
        {
          "category": "NSPrivacyAccessedAPICategoryUserDefaults",
          "reasons": ["CA92.1"]
        }
      ]
    }
  }
}
```

Declare additional APIs only when the application actually uses them for
those reasons. `manifests` accepts XML or binary property lists relative to
the project; configuration also supports `tracking`, `trackingDomains` and
`collectedData` with `type`, `linked`, `tracking` and `purposes`. Invalid keys,
types, mismatched required reasons and inconsistent tracking are errors.
MyGo's reason list follows [Apple's approved reasons](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype).

The import check detects direct timestamp, boot-time and disk-space API
symbols. It cannot discover every Objective-C selector, transitive dynamic
library call or runtime-loaded SDK. Application declarations must reflect
actual use. Listed binary SDKs still need their own manifests/signatures;
app-level merging does not replace [Apple's SDK packaging requirements](https://developer.apple.com/support/third-party-SDK-requirements/).

## Source symbols

iOS Go archives retain DWARF, including in Release. Native cgo objects compile
with debug information. Xcode creates matching dSYMs; Release strips the
installed application's symbols after collecting its dSYM, while Debug keeps
them and disables Go optimization/inlining for source debugging.

Archives contain `dSYMs/MyGoApp.app.dSYM`. Ordinary app builds include a sibling
`<name>.app.dSYM`. Preserve these with the exact corresponding binary/build;
each build verifies UUID equality and resolves a Go function address to its
source line with `atos`. dSYMs are separate from the installed app/IPA.

## Artifact checks

```sh
go run ./cmd/mygo ios check -archive 'build/ios-arm64/MyApp.xcarchive'
go run ./cmd/mygo ios check -ipa 'build/ios-arm64/Export/MyApp.ipa' \
  -symbols 'build/ios-arm64/MyApp.xcarchive/dSYMs/MyGoApp.app.dSYM'
go run ./cmd/mygo ios check -archive 'build/ios-arm64/MyApp.xcarchive' \
  -distribution -json
```

Use `-app` for an app bundle, `-archive` for its archive or `-ipa` for an export.
Archive symbol paths are inferred; other checks accept `-symbols`. `-unsigned`
explicitly permits unsigned local/CI builds. `-distribution` requires an App
Store profile, distribution identity, matching signed application/team
entitlements, production APNs entitlement if present, unexpired provisioning,
compiled icon, source symbols and an encryption declaration in Info.plist.
Set `ITSAppUsesNonExemptEncryption` through `ios.infoPlist` according to actual
app use; any required portal documentation remains an application task.

Distribution checks also require purpose strings for detected imports from
MyGo's protected-resource APIs: camera, microphone, location while in use,
photo library, add-only photo access and Face ID. Put these usage descriptions
in `ios.infoPlist`, using nonempty strings shorter than 4000 bytes that explain
the application's actual purpose. The iOS native demo includes examples.
Apple checks linked SDK references even when the app does not request access
at runtime, so runtime permission guards alone do not satisfy upload validation.
This local check covers the bridge's known direct imports; other SDK APIs and
localized descriptions still need application-specific review. MyGo does not
invent usage descriptions for an application.

Every build runs metadata, architecture, privacy, signature and symbol checks
before replacing the previous output. IPA exports are extracted and checked
again; ZIP CRC errors, unsafe paths/symlinks and excessive expansion fail.
`BuildReport.json` and `ExportReport.json` retain verification evidence.
App Store Connect exports additionally run distribution checks. These local
checks do not replace Apple's upload validation or server processing.

## Signing

Automatic signing uses `ios.developmentTeam` or `-ios-team` with the Xcode
account. Manual signing can use existing certificates/profiles:

```json
{
  "identifier": "com.example.myapp",
  "ios": {
    "developmentTeam": "TEAMID",
    "signing": {
      "style": "manual",
      "identity": "Apple Distribution",
      "provisioningProfile": "MyApp Store",
      "exportCertificate": "Apple Distribution",
      "exportProfiles": { "com.example.myapp": "MyApp Store" }
    }
  }
}
```

`exportProfiles` overrides the profile used for each bundle during export;
without an override the app uses `provisioningProfile`. Manual operations do
not ask Xcode to create/update profiles. Certificate/profile installation and
App Store Connect application setup remain external account configuration.

## Build numbers and local environment

`ios.buildNumber` is independent of the marketing `version`. Override it for
one build without changing the configuration file:

```sh
go run ./cmd/mygo build -platform ios/arm64 -ios-archive \
  -ios-team TEAMID -ios-build-number 42 \
  -ios-export-method app-store-connect ./myapp

go run ./cmd/mygo ios doctor
go run ./cmd/mygo ios doctor -release -json
```

The override accepts numeric strings with up to three components. A CI job can
supply its build counter; remote uniqueness and number reservation belong to
the application's release pipeline. `ios doctor` checks Go, Xcode SDKs,
packaging/symbol utilities and device tools. `-release` also requires a local
Apple Distribution identity with its private key; it does not create one.
Neither command contacts App Store Connect. iOS builds reject the desktop
`-upload` flag and only export local artifacts.

## CI and current release coverage

`.github/workflows/ios.yml` builds an unsigned Release archive and a simulator
Debug app, checks Go source symbols/privacy, and preserves archives, dSYMs and
reports. It needs no signing account.

`.github/workflows/ios-package.yml` is a manually triggered, main-branch-only
signed packaging job. It accepts a native project with `mygo.json` and an
optional numeric build number (default: the GitHub run number). Configure the
`ios-signing` environment with these signing secrets:

- `IOS_TEAM_ID`: Apple team ID.
- `IOS_CERTIFICATE_BASE64`: base64-encoded distribution `.p12`, including its private key.
- `IOS_CERTIFICATE_PASSWORD`: the `.p12` password (can be empty).
- `IOS_PROFILE_BASE64`: base64-encoded App Store provisioning profile for the app.

The project must declare its actual encryption use through
`ios.infoPlist.ITSAppUsesNonExemptEncryption`. The job verifies profile/team/app
matching, uses a temporary signing keychain, generates a manual-signing
configuration in the disposable checkout, builds/checks the archive and IPA,
and retains artifacts for 30 days. Cleanup restores the original keychain
search list and removes the job's profile and private signing material. It
uses Go, Xcode and macOS utilities, with no ASC credentials or upload steps.
The unsigned job's retention is 14 days. These workflows have not been run
remotely yet.

Current local checks cover development-signed Release archive/debugging IPA,
an automatically signed App Store Connect IPA, unsigned Release archive,
simulator Debug app, source-address resolution,
old-symbol/manifest rejection and development-profile rejection for App Store
use. The installed app has focused iPhone startup/authentication coverage.
See [mobile progress](mobile-progress.md) for exact runs. A separate certificate
comparison confirms that the App Store export's embedded profile contains
its distribution signing certificate. Real manual signing remains unverified.
Development IPAs remain for local device testing; App Store uploads require
the separately exported distribution IPA and the same team as the app record.

App Store validation, uploads, TestFlight metadata/groups/review and delivery
status are owned by the application release process. Tools such as asc may be
used independently after `mygo build`; MyGo does not call or depend on them.
