import UIKit
import UserNotifications

final class PushPlugin: MyGoPlugin {
    private var waiting: PluginReply?
    private var lastMessage = ""
    private var lastMessageAt = Date.distantPast
    /// A tap that launched the app comes before Go listens.
    private var opened: [[String: Any]] = []
    private var listening = false

    private func plain(_ info: [AnyHashable: Any]) -> [String: Any] {
        var out: [String: Any] = [:]
        for (k, v) in info where JSONSerialization.isValidJSONObject([v]) { out["\(k)"] = v }
        return out
    }

    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply) {
        switch method {
        case "configure":
            reply(.success(nil))
        case "register":
            waiting = reply
            UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge]) { granted, error in
                DispatchQueue.main.async {
                    guard granted else {
                        self.waiting?(.failure(PluginFailure("Denied", error?.localizedDescription ?? "notifications are not allowed")))
                        self.waiting = nil
                        return
                    }
                    UIApplication.shared.registerForRemoteNotifications()
                }
            }
        case "listenOpened":
            listening = true
            for o in opened { PluginHost.emit("push", "opened", o) }
            opened = []
            reply(.success(nil))
        default:
            reply(.failure(PluginFailure("NoMethod", method)))
        }
    }

    func applicationEvent(_ event: ApplicationEvent) {
        switch event {
        case .pushToken(let d):
            let token = d.map { String(format: "%02x", $0) }.joined()
            if let r = waiting {
                r(.success(["token": token]))
                waiting = nil
            }
            PluginHost.emit("push", "token", token)
        case .pushError(let e):
            waiting?(.failure(PluginFailure("Failed", e.localizedDescription)))
            waiting = nil
        case .pushMessage(let info):
            // UIKit tells some twice: as it arrives, and as it would show.
            let key = String(describing: info)
            let now = Date()
            if key == lastMessage && now.timeIntervalSince(lastMessageAt) < 2 { return }
            lastMessage = key
            lastMessageAt = now
            PluginHost.emit("push", "message", plain(info))
        case .pushOpened(let info):
            if listening {
                PluginHost.emit("push", "opened", plain(info))
            } else {
                opened.append(plain(info))
            }
        }
    }
}

/// Only apps with this plugin have it: UIKit warns of a delegate with it and no
/// remote-notification background mode.
extension AppDelegate {
    func application(_ application: UIApplication, didReceiveRemoteNotification info: [AnyHashable: Any],
                     fetchCompletionHandler done: @escaping (UIBackgroundFetchResult) -> Void) {
        PluginHost.applicationEvent(.pushMessage(info))
        done(.newData)
    }
}
