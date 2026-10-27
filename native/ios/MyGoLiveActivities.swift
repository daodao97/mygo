import ActivityKit
import Foundation

@available(iOS 16.2, *)
public struct MyGoLiveActivityAttributes: ActivityAttributes {
    public struct ContentState: Codable, Hashable {
        public var title: String
        public var subtitle: String
        public var status: String
        public var url: String
        public var progress: Double?
        public var updatedAt: Int64
    }
    public var id: String
}

#if !MYGO_LIVE_ACTIVITY_EXTENSION
import UIKit
@_silgen_name("goIOSSystemResult") private func activityResult(_ token: UInt64, _ value: UnsafePointer<CChar>, _ code: Int32, _ message: UnsafePointer<CChar>)
@_silgen_name("mygo_ios_live_activity_event") private func activityEvent(_ value: UnsafePointer<CChar>)

@available(iOS 16.2, *)
@MainActor private enum MyGoActivities {
    struct Request: Decodable { var id: String; var state: MyGoLiveActivityAttributes.ContentState }
    struct Info: Encodable { var id: String; var activity: String; var pushToken: String }
    static var watches: [String: Task<Void, Never>] = [:]
    static var canceled: Set<UInt64> = []
    static func emit(_ activity: Activity<MyGoLiveActivityAttributes>, _ token: Data?) {
        let info = Info(id: activity.attributes.id, activity: activity.id, pushToken: token?.map { String(format: "%02x", $0) }.joined() ?? "")
        if let data = try? JSONEncoder().encode(info), let text = String(data: data, encoding: .utf8) { text.withCString { activityEvent($0) } }
    }
    static func watch(_ activity: Activity<MyGoLiveActivityAttributes>) {
        guard watches[activity.id] == nil else { return }
        watches[activity.id] = Task { @MainActor in
            let tokens = Task { @MainActor in
                for await token in activity.pushTokenUpdates { emit(activity, token) }
            }
            for await state in activity.activityStateUpdates {
                if state == .dismissed || state == .ended {
                    emit(activity, nil)
                    tokens.cancel()
                    watches.removeValue(forKey: activity.id)
                    return
                }
            }
            tokens.cancel()
        }
    }
    static func run(_ token: UInt64, _ kind: String, _ json: String) async {
        if canceled.remove(token) != nil { return }
        do {
            guard let bytes = json.data(using: .utf8) else { throw NSError(domain: "mygo", code: 1) }
            if kind == "activity-end" {
                let id = try JSONDecoder().decode(String.self, from: bytes)
                for activity in Activity<MyGoLiveActivityAttributes>.activities where activity.attributes.id == id {
                    await activity.end(nil, dismissalPolicy: .immediate)
                    emit(activity, nil)
                }
                reply(token, "null", 0, "")
                return
            }
            let request = try JSONDecoder().decode(Request.self, from: bytes)
            guard ActivityAuthorizationInfo().areActivitiesEnabled else { reply(token, "null", 2, "Live Activities are disabled in system settings"); return }
            let content = ActivityContent(state: request.state, staleDate: Date().addingTimeInterval(120))
            let activity: Activity<MyGoLiveActivityAttributes>
            if let existing = Activity<MyGoLiveActivityAttributes>.activities.first(where: { $0.attributes.id == request.id && ($0.activityState == .active || $0.activityState == .stale) }) {
                activity = existing
                await activity.update(content)
            } else {
                guard UIApplication.shared.applicationState == .active else { reply(token, "null", 2, "Start Live Activities while the application is active"); return }
                activity = try Activity.request(attributes: MyGoLiveActivityAttributes(id: request.id), content: content, pushType: .token)
            }
            if canceled.remove(token) != nil {
                await activity.end(nil, dismissalPolicy: .immediate)
                return
            }
            watch(activity)
            let info = Info(id: request.id, activity: activity.id, pushToken: activity.pushToken?.map { String(format: "%02x", $0) }.joined() ?? "")
            let text = String(data: try JSONEncoder().encode(info), encoding: .utf8) ?? "null"
            reply(token, text, 0, "")
        } catch { reply(token, "null", 2, error.localizedDescription) }
    }
}
private func reply(_ token: UInt64, _ value: String, _ code: Int32, _ message: String) { value.withCString { data in message.withCString { activityResult(token, data, code, $0) } } }

@_cdecl("mygo_swift_activity") func mygoActivity(_ token: UInt64, _ kind: UnsafePointer<CChar>, _ json: UnsafePointer<CChar>) {
    let k = String(cString: kind), value = String(cString: json)
    Task { @MainActor in
        if #available(iOS 16.2, *) { await MyGoActivities.run(token, k, value) }
        else { reply(token, "null", 1, "") }
    }
}
@_cdecl("mygo_swift_activity_cancel") func mygoActivityCancel(_ token: UInt64) {
    Task { @MainActor in
        if #available(iOS 16.2, *) { if MyGoActivities.canceled.count > 128 { MyGoActivities.canceled.removeAll() }; MyGoActivities.canceled.insert(token) }
        reply(token, "null", 2, "Canceled")
    }
}
#endif
