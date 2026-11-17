// The iOS host of MyGo: the app's delegates, which start Go, and the
// bridge between Go and UIKit. Go runs on a thread of its own and makes
// its calls (HostCalls) through mygo_drain on the main thread; events go
// to Go through mygo_event.

import UIKit
import os

/// The way to Go.
enum Go {
    /// The descriptor of the process's standard error before Go took it,
    /// where the app's output still goes, as Xcode and simctl show it.
    static var stderr: Int32 = -1
    static let log = OSLog(subsystem: Bundle.main.bundleIdentifier ?? "mygo", category: "app")

    /// Sends an event Go does not answer.
    static func send(_ msg: [UInt8]) { _ = ask(msg) }

    /// Sends an event and returns Go's answer, nil when Go answers none
    /// (the event is not one it answers, or Go is not running yet).
    @discardableResult
    static func ask(_ msg: [UInt8]) -> [UInt8]? {
        var msg = msg
        var reply: UnsafeMutablePointer<UInt8>? = nil
        var n = 0
        msg.withUnsafeMutableBufferPointer { p in
            mygo_event(p.baseAddress, p.count, Thread.isMainThread ? 1 : 0, &reply, &n)
        }
        guard let reply else { return nil }
        defer { mygo_free(reply) }
        return Array(UnsafeBufferPointer(start: reply, count: n))
    }

    /// Decodes Go's answer to an event, or nil when there is none.
    static func answer<T>(_ msg: [UInt8], _ decode: (inout WireReader) -> T) -> T? {
        guard let reply = ask(msg) else { return nil }
        return reply.withUnsafeBytes { buf in
            var r = WireReader(buf)
            return decode(&r)
        }
    }

    static func start() {
        stderr = dup(2)
        var host = mygo_host(
            call: { msg, len, reply, replyLen in
                let out = dispatchCall(Host.shared, UnsafeRawBufferPointer(start: msg, count: len))
                guard let out, let reply, let replyLen else {
                    reply?.pointee = nil
                    replyLen?.pointee = 0
                    return
                }
                let p = malloc(max(out.count, 1))!.assumingMemoryBound(to: UInt8.self)
                out.withUnsafeBufferPointer { b in
                    if let base = b.baseAddress { p.update(from: base, count: out.count) }
                }
                reply.pointee = p
                replyLen.pointee = out.count
            },
            post: {
                DispatchQueue.main.async { mygo_drain() }
            },
            log: { isError, line, len in
                guard let line else { return }
                let s = String(decoding: UnsafeBufferPointer(start: line, count: len), as: UTF8.self)
                os_log("%{public}@", log: Go.log, type: isError != 0 ? .error : .default, s)
                if Go.stderr >= 0 {
                    var b = Array(s.utf8)
                    b.append(10)
                    _ = b.withUnsafeBytes { write(Go.stderr, $0.baseAddress, $0.count) }
                }
            })
        mygo_start(&host)
    }
}

@main
final class AppDelegate: UIResponder, UIApplicationDelegate {
    func application(_ application: UIApplication, didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil) -> Bool {
        UNUserNotificationCenter.current().delegate = Host.shared
        registerPlugins()
        Go.start()
        return true
    }

    func application(_ application: UIApplication, configurationForConnecting session: UISceneSession, options: UIScene.ConnectionOptions) -> UISceneConfiguration {
        let c = UISceneConfiguration(name: nil, sessionRole: session.role)
        c.delegateClass = SceneDelegate.self
        return c
    }

    func application(_ application: UIApplication, didRegisterForRemoteNotificationsWithDeviceToken token: Data) {
        PluginHost.applicationEvent(.pushToken(token))
    }

    func application(_ application: UIApplication, didFailToRegisterForRemoteNotificationsWithError error: Error) {
        PluginHost.applicationEvent(.pushError(error))
    }

    func application(_ application: UIApplication, didDiscardSceneSessions sessions: Set<UISceneSession>) {
        Scenes.discarded(sessions)
    }

    func applicationDidReceiveMemoryWarning(_ application: UIApplication) { Go.send(Event.memoryWarning()) }
    func applicationWillTerminate(_ application: UIApplication) { Go.ask(Event.terminating()) }
}

/// The windows of their own scenes, on iPad (WindowOptions.OwnScene): a scene
/// asked for with the window's number, in which it's the whole content. iOS may
/// disconnect a scene in the background and connect it again, keeping its session,
/// which keeps the window; the window closes as the session goes.
enum Scenes {
    static let activity = "dev.mygo.window"
    static var sessions: [Int: UISceneSession] = [:]
    /// Those asked for that haven't connected, and those of them no longer wanted.
    static var pending = Set<Int>()
    static var unwanted = Set<Int>()

    static var available: Bool {
        UIDevice.current.userInterfaceIdiom == .pad && UIApplication.shared.supportsMultipleScenes
    }

    static func open(_ w: HostWindow) {
        unwanted.remove(w.id)
        if pending.contains(w.id) { return }
        let request: UISceneSessionActivationRequest
        if let s = sessions[w.id] {
            request = UISceneSessionActivationRequest(session: s)
        } else {
            let a = NSUserActivity(activityType: activity)
            a.userInfo = ["window": w.id]
            request = UISceneSessionActivationRequest(role: .windowApplication, userActivity: a)
            pending.insert(w.id)
        }
        UIApplication.shared.activateSceneSession(for: request) { error in
            pending.remove(w.id)
            FileHandle.standardError.write(Data("mygo: opening a window: \(error)\n".utf8))
        }
    }

    static func close(_ id: Int) {
        if pending.contains(id) { unwanted.insert(id) }
        guard let s = sessions.removeValue(forKey: id) else { return }
        // Back to the app's first, rather than out of the app, where iPadOS would
        // go from the only window on screen.
        if let main = Host.shared.window?.windowScene?.session {
            UIApplication.shared.activateSceneSession(for: UISceneSessionActivationRequest(session: main)) { _ in }
        }
        UIApplication.shared.requestSceneSessionDestruction(s, options: nil)
    }

    /// The user closed these: their windows close.
    static func discarded(_ all: Set<UISceneSession>) {
        for s in all {
            guard let id = s.userInfo?["window"] as? Int, sessions[id] === s else { continue }
            sessions[id] = nil
            if Host.shared.windows[id] != nil { Go.send(Event.closedByPage(id: id)) }
        }
    }
}

final class SceneDelegate: UIResponder, UIWindowSceneDelegate {
    var window: UIWindow?
    /// The MyGo window this scene is, for one of its own; 0 for the app's first.
    private var ownWindow = 0

    func scene(_ scene: UIScene, willConnectTo session: UISceneSession, options: UIScene.ConnectionOptions) {
        guard let scene = scene as? UIWindowScene else { return }
        let own = options.userActivities.first(where: { $0.activityType == Scenes.activity })?.userInfo?["window"] as? Int
            ?? session.userInfo?["window"] as? Int
        if let id = own {
            Scenes.pending.remove(id)
            // A window of its own, unless it's of an earlier run, which the
            // system brings back, or wanted no more.
            guard let hw = Host.shared.windows[id], Scenes.unwanted.remove(id) == nil else {
                UIApplication.shared.requestSceneSessionDestruction(session, options: nil)
                return
            }
            let w = UIWindow(windowScene: scene)
            let holder = UIViewController()
            let c = Navigation.shared.controller(hw)
            holder.addChild(c)
            c.view.frame = holder.view.bounds
            c.view.autoresizingMask = [.flexibleWidth, .flexibleHeight]
            holder.view.addSubview(c.view)
            c.didMove(toParent: holder)
            w.rootViewController = holder
            w.makeKeyAndVisible()
            window = w
            ownWindow = id
            session.userInfo = ["window": id]
            Scenes.sessions[id] = session
            scene.title = c.title
            hw.reportSize()
            return
        }
        if Host.shared.window != nil {
            // A second of the app's first, which the user asked the system for:
            // the app has one such window.
            UIApplication.shared.requestSceneSessionDestruction(session, options: nil)
            return
        }
        let w = UIWindow(windowScene: scene)
        w.rootViewController = Host.shared.root
        w.makeKeyAndVisible()
        window = w
        Host.shared.window = w
        let urls = options.urlContexts.map { $0.url.absoluteString } + options.userActivities.compactMap { $0.webpageURL?.absoluteString }
        if !urls.isEmpty { Go.send(Event.openURLs(urls: urls)) }
    }

    func scene(_ scene: UIScene, openURLContexts contexts: Set<UIOpenURLContext>) {
        Go.send(Event.openURLs(urls: contexts.map { $0.url.absoluteString }))
    }

    func scene(_ scene: UIScene, continue activity: NSUserActivity) {
        if let url = activity.webpageURL { Go.send(Event.openURLs(urls: [url.absoluteString])) }
    }

    func sceneDidDisconnect(_ scene: UIScene) {
        // Not closed: its session stays, and the system may connect it again, for
        // which the view lets go of the scene's window.
        if ownWindow != 0, let c = window?.rootViewController?.children.first {
            c.willMove(toParent: nil)
            c.view.removeFromSuperview()
            c.removeFromParent()
        }
        window?.rootViewController = nil
        window = nil
        if ownWindow == 0, Host.shared.window?.windowScene === scene {
            Host.shared.window = nil
        }
    }

    func sceneDidBecomeActive(_ scene: UIScene) {
        Go.send(Event.active(active: true))
        if UIApplication.shared.supportsMultipleScenes { Go.send(Event.windowFront(id: ownWindow)) }
    }

    func sceneWillResignActive(_ scene: UIScene) {
        let others = UIApplication.shared.connectedScenes.filter { $0 !== scene && $0.activationState == .foregroundActive }
        if others.isEmpty { Go.send(Event.active(active: false)) }
    }

    // Waits for Go: it must stop drawing before the app is in the background,
    // where iOS ends apps that use the GPU. The app is there once none of its
    // scenes is on screen.
    func sceneDidEnterBackground(_ scene: UIScene) {
        let shown = UIApplication.shared.connectedScenes.contains {
            $0 !== scene && ($0.activationState == .foregroundActive || $0.activationState == .foregroundInactive)
        }
        if !shown { Go.ask(Event.background(background: true)) }
    }

    func sceneWillEnterForeground(_ scene: UIScene) { Go.ask(Event.background(background: false)) }
}

/// The view controller of the app's window, whose view holds the app's
/// windows, the one in front last.
final class RootViewController: UIViewController, UIScrollViewDelegate {
    /// Where the keyboard is, on the screen, nil while it's away.
    var keyboardFrame: CGRect?
    /// UIKit tells a scroll view of a tap on the status bar, the only way it
    /// tells: this one shows nothing, and tells Go.
    private let statusBarTap = UIScrollView()
    /// What the app asked of the system's bars and the screen's orientation.
    var barStyle = "auto"
    var barsHidden = false
    var orientations: UIInterfaceOrientationMask = .all

    override var preferredStatusBarStyle: UIStatusBarStyle {
        switch barStyle {
        case "light": return .lightContent
        case "dark": return .darkContent
        default: return .default
        }
    }
    override var prefersStatusBarHidden: Bool { barsHidden }
    override var prefersHomeIndicatorAutoHidden: Bool { barsHidden }
    override var supportedInterfaceOrientations: UIInterfaceOrientationMask { orientations }
    override var childForStatusBarHidden: UIViewController? { nil }

    override func loadView() {
        view = UIView()
        view.backgroundColor = .systemBackground
        statusBarTap.scrollsToTop = true
        statusBarTap.delegate = self
        statusBarTap.contentSize = CGSize(width: 1, height: 3)
        statusBarTap.contentOffset = CGPoint(x: 0, y: 1)
        statusBarTap.frame = CGRect(x: 0, y: 0, width: 1, height: 1)
        view.addSubview(statusBarTap)
        let nc = NotificationCenter.default
        nc.addObserver(self, selector: #selector(keyboardChanged(_:)), name: UIResponder.keyboardWillChangeFrameNotification, object: nil)
        nc.addObserver(self, selector: #selector(keyboardChanged(_:)), name: UIResponder.keyboardWillHideNotification, object: nil)
        nc.addObserver(self, selector: #selector(settingsChanged), name: UIAccessibility.reduceMotionStatusDidChangeNotification, object: nil)
        nc.addObserver(self, selector: #selector(settingsChanged), name: UIContentSizeCategory.didChangeNotification, object: nil)
        nc.addObserver(self, selector: #selector(localeChanged), name: NSLocale.currentLocaleDidChangeNotification, object: nil)
        nc.addObserver(self, selector: #selector(timeZoneChanged), name: .NSSystemTimeZoneDidChange, object: nil)
        registerForTraitChanges([UITraitUserInterfaceStyle.self]) { (_: RootViewController, _) in
            Go.send(Event.themeChanged())
        }
    }

    func scrollViewShouldScrollToTop(_ s: UIScrollView) -> Bool {
        Go.send(Event.statusBarTapped())
        return false
    }

    @objc func settingsChanged() { Go.send(Event.themeChanged()) }
    @objc func localeChanged() { Go.send(Event.localeChanged()) }
    @objc func timeZoneChanged() {
        NSTimeZone.resetSystemTimeZone()
        Go.send(Event.timeZoneChanged(id: TimeZone.current.identifier))
    }

    @objc func keyboardChanged(_ n: Notification) {
        var frame: CGRect?
        if n.name != UIResponder.keyboardWillHideNotification {
            frame = (n.userInfo?[UIResponder.keyboardFrameEndUserInfoKey] as? NSValue)?.cgRectValue
        }
        if frame != keyboardFrame {
            keyboardFrame = frame
            for w in Host.shared.windows.values { w.reportSize() }
        }
    }

    override func viewDidLayoutSubviews() {
        super.viewDidLayoutSubviews()
        for w in Host.shared.windows.values { w.reportSize() }
    }

    override func viewSafeAreaInsetsDidChange() {
        super.viewSafeAreaInsetsDidChange()
        for w in Host.shared.windows.values { w.reportSize() }
    }

    override var childForStatusBarStyle: UIViewController? { nil }
}

/// A window of the app: a view filling the screen, which shows a page or
/// native UI.
class HostWindow: UIView {
    let id: Int
    /// The surface showing this window under it, as a web view in native UI.
    weak var holder: SurfaceWindow?
    /// It opens in a scene of its own, on iPad.
    var ownScene = false
    private var lastSize: [Double] = []

    init(id: Int) {
        self.id = id
        super.init(frame: Host.shared.root.view.bounds)
        autoresizingMask = [.flexibleWidth, .flexibleHeight]
    }

    required init?(coder: NSCoder) { fatalError() }

    /// How much of this window the keyboard covers: only that of the scene the
    /// user is typing in, of those an iPad may show side by side.
    private var keyboardCover: CGFloat {
        guard let frame = Host.shared.root.keyboardFrame, let w = window,
              w.windowScene?.activationState == .foregroundActive else { return 0 }
        let local = convert(frame, from: w.screen.coordinateSpace)
        return max(0, bounds.maxY - local.minY)
    }

    /// Tells Go the window's size and what covers it, when they changed.
    func reportSize() {
        var s = holder == nil ? safeAreaInsets : .zero
        // A large title, in the scroll view standing for the page's
        // content, covers the page further down than the bars.
        if holder == nil, let top = (superview?.next as? ScreenController)?.topInset { s.top = max(s.top, top) }
        let scale = window?.screen.scale ?? traitCollection.displayScale
        let size = [Double(bounds.width), Double(bounds.height), Double(scale), Double(s.top), Double(s.left),
                    Double(s.bottom), Double(s.right), Double(keyboardCover)]
        if size == lastSize || bounds.width == 0 { return }
        lastSize = size
        Go.send(Event.resized(id: id, width: size[0], height: size[1], scale: size[2], safeTop: size[3],
                              safeLeft: size[4], safeBottom: size[5], safeRight: size[6], keyboard: size[7]))
    }

    override func safeAreaInsetsDidChange() {
        super.safeAreaInsetsDidChange()
        reportSize()
    }

    override func layoutSubviews() {
        super.layoutSubviews()
        reportSize()
    }

    func close() { removeFromSuperview() }
    func focus() {}
    func setBackground(_ color: UIColor) { backgroundColor = color }
}

extension UIColor {
    /// The color of 0xRRGGBBAA.
    convenience init(rgba v: Int) {
        self.init(red: CGFloat((v >> 24) & 0xff) / 255, green: CGFloat((v >> 16) & 0xff) / 255,
                  blue: CGFloat((v >> 8) & 0xff) / 255, alpha: CGFloat(v & 0xff) / 255)
    }

    var rgba: Int {
        var r: CGFloat = 0, g: CGFloat = 0, b: CGFloat = 0, a: CGFloat = 0
        getRed(&r, green: &g, blue: &b, alpha: &a)
        func c(_ v: CGFloat) -> Int { Int((min(max(v, 0), 1) * 255).rounded()) }
        return c(r) << 24 | c(g) << 16 | c(b) << 8 | c(a)
    }
}
