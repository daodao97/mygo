# APNs push notifications

MyGo provides iOS notification delivery and an APNs sender for desktop apps or
servers. A new application enables push in project configuration, imports its
Apple key once on the sender host, and opens the configured provider. It does
not need its own config loader, JWT implementation or macOS Keychain commands.

## Configure a project

Add the push switch alongside the application's existing signing identity:

```json
{
  "name": "My App",
  "identifier": "dev.example.app",
  "ios": {
    "developmentTeam": "YOURTEAMID",
    "pushNotifications": true
  }
}
```

The same `ios.pushNotifications` field is available in `mygo.config.ts`.
The CLI generates `aps-environment=development` unless `ios.entitlements`
explicitly supplies a value. The switch configures signing; it does not request
notification permission or automatically send device tokens anywhere.

In Apple Developer, enable Push Notifications for this App ID, use a matching
provisioning profile, and create an APNs key authorized for this topic and
its environment. Apple account configuration is separate from MyGo; the CLI
never creates keys or changes capabilities on your behalf.

## Import the sender key once

Run in the project directory on the desktop/server that sends notifications:

```sh
go tool mygo push setup /private/path/AuthKey_YOURKEYID0.p8
go tool mygo push status
```

The command reads Bundle ID and Team ID from project configuration. It infers
Key ID from Apple's `AuthKey_KEYID.p8` filename. If the file was renamed, use
`push setup -key-id YOURKEYID0 /private/path/key.p8`.

On macOS the key is saved in the login Keychain, under `<bundle-id>.apns` with
Key ID as the account. On Linux it is stored in an owner-only file. Managed key
import is not implemented on Windows or iOS; the low-level `apns.New` sender
can still use credentials supplied by a host's own secret manager.

Provider metadata lives in `push-config.json` in the application's user-data
directory (`~/Library/Application Support/My App` on macOS). It contains no
private key bytes. Setup does not copy the key into resources, project config,
build output or the mobile bundle. Keep the downloaded key in a secure backup.

Flags precede the key path. Use `-team TEAMID` to override the project's team,
`-dev` for the separate `My App Dev` data directory used by `mygo dev`, or
`-data-dir /private/sender-data` to configure a headless/remote host. Run setup
on that host; importing on the build Mac does not provision another computer.

## Send from a desktop or server

In a packaged MyGo application, the user-data directory and bundle identity
are already known:

```go
provider, err := mygo.App.PushProvider()
if err != nil {
    return err
}
_, err = provider.Send(ctx, apns.Notification{
    DeviceToken: token,
    Payload: apns.Payload{
        ID: "message-42", Title: "New message", Body: "Tap to open",
        Data: map[string]string{"item": "42"},
    },
})
return err
```

Imports are `github.com/egoist/mygo` and `github.com/egoist/mygo/push/apns`.
No `App.Run` or native event loop is needed for the sender. Retain the provider
instead of recreating it for every message. Topic comes from imported metadata;
a different explicitly supplied Topic is rejected. `App.PushProvider` also
checks it against the packaged application's identifier (allowing the desktop
development app's `.dev` suffix for the same mobile topic). It respects
`App.SetPath(mygo.PathUserData, dir)`.

For a plain Go server, or an app with an existing private data directory:

```go
provider, err := apns.OpenProvider("/private/sender-data")
```

`provider.Reload()` picks up changed metadata or a rotated key. Unchanged
credentials retain their JWT and HTTP/2 connections. A failed reload disables
sending until a successful reload. Reload cannot switch the provider to another
application; open a separate provider for that application. Neither constructor
starts a background worker or sends a test message.

## Register and receive on iOS

Before `App.Run`, register `App.OnPushToken`, `App.OnPushRegistrationError` and
`App.OnNotification`. After the app becomes Active, request visible notification
permission with `Permissions.Request(PermissionNotifications)`, then call
`App.RegisterPushNotifications`. Permission requests may fail during
`WhenReady` because UIKit has not activated the app yet. Use
`App.OnDidBecomeActive`, guard concurrent requests, and retry on later activation.

Forward the latest token to your own authenticated subscription endpoint on every
launch/update. Bind it to your user/device and notification preference. The
provider key stays on the sender. On notification clicks, `event.Clicked` is true
and `event.Data` contains the custom string fields; route to the corresponding
application item once login/storage are ready. MyGo queues cold-launch responses
until Go readiness, but application storage may still be loading asynchronously.

`App.SetNotificationPresentationHandler` controls foreground banner/list/sound
presentation and may return zero for an already visible item. It does not suppress
system presentation in the background. Subscription authorization, opt-out,
deduplication, routing and stale-token removal remain application policy.
See [iOS notification APIs](ios.md) for callbacks and platform details.

## Production and checks

Development-signed device builds use Sandbox. TestFlight/App Store distribution
uses Production. Configure a key authorized for the production environment and
import it explicitly:

```sh
go tool mygo push setup -environment production /private/path/AuthKey_YOURKEYID0.p8
```

An explicit `ios.entitlements["aps-environment"]="production"` also changes the
setup default. The final provisioning profile/signature determines the token's
environment; a Release build alone does not imply Production. The managed store
holds one environment per application data directory. Use separate `-data-dir`
locations and `apns.OpenProvider` instances when running both environments.

`push status` verifies local metadata and credential loading without contacting
APNs. Complete real-device verification separately: permission and registration,
foreground presentation/deduplication, background receipt, and notification click
while the app is terminated. APNs acceptance is not proof that the device displayed
or the user opened a notification. `*apns.Error` exposes provider failures; remove
an Unregistered token only if it has not been refreshed after the error timestamp.
Silent background execution callbacks are not implemented by MyGo.

Apple references: [device registration](https://developer.apple.com/documentation/usernotifications/registering-your-app-with-apns),
[token authentication](https://developer.apple.com/documentation/usernotifications/establishing-a-token-based-connection-to-apns),
and [APNs environments](https://developer.apple.com/documentation/bundleresources/entitlements/aps-environment).
