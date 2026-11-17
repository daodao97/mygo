// Plugins' native code. A plugin brings Swift that implements MyGoPlugin
// (mygo-plugin.json names the class), which mygo build compiles into the app
// and registers by the plugin's id; Go calls it with mygo.Native(id).Call. It
// runs on the main thread.

import UIKit

/// The result of a plugin's method: its answer, which JSON can encode, or an
/// error.
public typealias PluginReply = (Result<Any?, PluginFailure>) -> Void

/// An error of a plugin's native code, with a code Go can tell apart.
public struct PluginFailure: Error {
    public let code: String
    public let message: String
    public init(_ code: String, _ message: String = "") {
        self.code = code
        self.message = message
    }
}

/// A plugin's native code.
public protocol MyGoPlugin: AnyObject {
    /// Runs a method with its arguments, from JSON, on the main thread. It
    /// may answer later: call reply exactly once, from any thread.
    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply)
    /// Hears what the app's delegate hears that no plugin can get otherwise.
    func applicationEvent(_ event: ApplicationEvent)
}

public extension MyGoPlugin {
    func applicationEvent(_ event: ApplicationEvent) {}
}

/// What reaches the app's delegate for plugins.
public enum ApplicationEvent {
    case pushToken(Data)
    case pushError(Error)
    case pushMessage([AnyHashable: Any])
    /// The user tapped a push's notification.
    case pushOpened([AnyHashable: Any])
}

/// What plugins have of the host.
public enum PluginHost {
    private static var plugins: [String: MyGoPlugin] = [:]

    private static var viewMakers: [String: (Int, [String: Any]) -> UIView] = [:]

    public static func register(_ id: String, _ plugin: MyGoPlugin) { plugins[id] = plugin }

    /// Registers a type of view that mygo.NewPluginView makes: make gets the
    /// number that calls about the view name it, and the arguments.
    public static func registerView(_ type: String, _ make: @escaping (_ id: Int, _ args: [String: Any]) -> UIView) {
        viewMakers[type] = make
    }

    static func makeView(_ type: String, id: Int, args: String) -> UIView? {
        // A native module's view, which ui.NativeView shows: its handle, an
        // object its Swift retained for Go.
        if type == "mygo.native" {
            guard let h = UInt(args), let p = UnsafeRawPointer(bitPattern: h) else { return nil }
            return Unmanaged<AnyObject>.fromOpaque(p).takeUnretainedValue() as? UIView
        }
        var a: [String: Any] = [:]
        if let d = args.data(using: .utf8), let o = try? JSONSerialization.jsonObject(with: d) as? [String: Any] { a = o }
        return viewMakers[type]?(id, a)
    }

    /// The view controller to present over.
    public static var presenter: UIViewController { Host.shared.presenter }
    /// Presents a view controller, once the one on screen stops going away.
    public static func present(_ vc: UIViewController) { Host.shared.present(vc) }
    /// The window showing the app.
    public static var window: UIWindow? { Host.shared.window }

    /// Sends an event to Go (mygo.Native(id).On), from any thread.
    public static func emit(_ plugin: String, _ name: String, _ data: Any? = nil) {
        Go.send(Event.pluginEvent(plugin: plugin, name: name, data: encode(data)))
    }

    /// Tells plugins what reached the app's delegate. Public for what a plugin
    /// adds to the delegate itself, in an extension of AppDelegate.
    public static func applicationEvent(_ e: ApplicationEvent) {
        for p in plugins.values { p.applicationEvent(e) }
    }

    static func encode(_ v: Any?) -> String {
        guard let v else { return "null" }
        if JSONSerialization.isValidJSONObject(v), let d = try? JSONSerialization.data(withJSONObject: v) {
            return String(decoding: d, as: UTF8.self)
        }
        // Scalars, which JSONSerialization takes only inside something. What
        // it can't encode it raises for, which Swift cannot catch.
        if JSONSerialization.isValidJSONObject([v]), let d = try? JSONSerialization.data(withJSONObject: [v]), d.count >= 2 {
            return String(decoding: d[1..<(d.count - 1)], as: UTF8.self)
        }
        return "null"
    }

    static func call(_ c: PluginCallCall) {
        guard let p = plugins[c.plugin] else {
            Go.send(Event.pluginResult(req: c.req, result: "", error: "no plugin \(c.plugin) in this app", code: "NoPlugin"))
            return
        }
        var args: [String: Any] = [:]
        if let d = c.args.data(using: .utf8), let o = try? JSONSerialization.jsonObject(with: d) as? [String: Any] { args = o }
        var answered = false
        p.call(c.method, args) { r in
            DispatchQueue.main.async {
                guard !answered else { return }
                answered = true
                switch r {
                case .success(let v):
                    Go.send(Event.pluginResult(req: c.req, result: encode(v), error: "", code: ""))
                case .failure(let f):
                    Go.send(Event.pluginResult(req: c.req, result: "", error: f.message, code: f.code))
                }
            }
        }
    }
}

/// A window showing a plugin's view, which a surface holds.
final class PluginViewWindow: HostWindow {
    init(id: Int, view: UIView) {
        super.init(id: id)
        view.frame = bounds
        view.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        addSubview(view)
    }

    required init?(coder: NSCoder) { fatalError() }
}

/// The windows of the app, for native modules' Swift, which finds this
/// class by its name: a method taking a *mygo.Window gets its view
/// controller.
@objc(MyGoWindows) public final class MyGoWindows: NSObject {
    @MainActor @objc public static func controller(_ id: NSNumber) -> UIViewController? {
        guard let w = Host.shared.windows[id.intValue] else { return nil }
        return Navigation.shared.controller(w)
    }
}
