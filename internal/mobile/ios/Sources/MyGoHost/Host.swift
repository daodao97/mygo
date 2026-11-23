// The calls Go makes of the host (HostCalls), on the main thread.

import UIKit
import WebKit
import UserNotifications
import UniformTypeIdentifiers
import Security
import Network

final class Host: NSObject, HostCalls, UNUserNotificationCenterDelegate, UIDocumentPickerDelegate {
    static let shared = Host()

    let root = RootViewController()
    var window: UIWindow?
    var windows: [Int: HostWindow] = [:]
    /// The windows on screen, the one in front last.
    var pickers: [UIDocumentPickerViewController: Int] = [:]

    /// The view controller to present over: the root, or what it presents.
    var presenter: UIViewController {
        // Over the window the user is in, which an iPad may have several of.
        let active = UIApplication.shared.connectedScenes.lazy
            .compactMap { $0 as? UIWindowScene }
            .first { $0.activationState == .foregroundActive }
        var vc: UIViewController = active?.keyWindow?.rootViewController ?? root
        while let p = vc.presentedViewController { vc = p }
        return vc
    }

    /// Presents over what shows, once what shows stops going away: UIKit only
    /// logs a presentation it refuses, whose answer would never come.
    func present(_ vc: UIViewController, tries: Int = 20, then done: (() -> Void)? = nil) {
        let top = presenter
        if (top.isBeingDismissed || top.isBeingPresented) && tries > 0 {
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.05) { self.present(vc, tries: tries - 1, then: done) }
            return
        }
        top.present(vc, animated: true, completion: done)
    }

    func timeZone() -> TimeZoneReply {
        var r = TimeZoneReply()
        r.id = TimeZone.current.identifier
        return r
    }

    func firstFrame(_ c: FirstFrameCall) {} // iOS's launch screen goes by itself

    func appInfo() -> AppInfoReply {
        var r = AppInfoReply()
        r.background = UIApplication.shared.applicationState == .background // the system started it for a task
        let b = Bundle.main
        r.name = (b.object(forInfoDictionaryKey: "CFBundleDisplayName") ?? b.object(forInfoDictionaryKey: "CFBundleName")) as? String ?? ""
        r.version = b.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? ""
        r.identifier = b.bundleIdentifier ?? ""
        let fm = FileManager.default
        r.filesDir = fm.urls(for: .applicationSupportDirectory, in: .userDomainMask).first?.path ?? ""
        r.cacheDir = fm.urls(for: .cachesDirectory, in: .userDomainMask).first?.path ?? ""
        r.tempDir = NSTemporaryDirectory()
        r.documentsDir = fm.urls(for: .documentDirectory, in: .userDomainMask).first?.path ?? ""
        r.resourcesDir = b.resourcePath ?? ""
        r.languages = Locale.preferredLanguages
        r.regionLocale = Locale.current.identifier
        r.firstWeekday = Calendar.current.firstWeekday - 1
        let j = DateFormatter.dateFormat(fromTemplate: "j", options: 0, locale: .current) ?? ""
        r.hourCycle = j.contains("a") ? 12 : 24
        r.system = UIDevice.current.systemName
        r.systemVersion = UIDevice.current.systemVersion
        r.model = UIDevice.current.model
        // iOS does not say: what the app kept goes if it was force-quit, which
        // the system tells by the scene's session not coming back.
        r.restoring = true
        return r
    }

    func theme() -> ThemeReply {
        var r = ThemeReply()
        let traits = window?.traitCollection ?? root.traitCollection
        r.dark = traits.userInterfaceStyle == .dark
        r.accent = (window?.tintColor ?? .systemBlue).resolvedColor(with: traits).rgba
        r.reduceMotion = UIAccessibility.isReduceMotionEnabled
        r.reduceTransparency = UIAccessibility.isReduceTransparencyEnabled
        r.increaseContrast = UIAccessibility.isDarkerSystemColorsEnabled
        r.textScale = Double(UIFontMetrics.default.scaledValue(for: 17) / 17)
        return r
    }

    func setAppearance(_ c: SetAppearanceCall) {
        let style: UIUserInterfaceStyle = c.source == "dark" ? .dark : c.source == "light" ? .light : .unspecified
        window?.overrideUserInterfaceStyle = style
    }

    func screen() -> ScreenReply {
        var r = ScreenReply()
        let s = window?.windowScene?.screen ?? UIScreen.main
        r.width = Double(s.bounds.width)
        r.height = Double(s.bounds.height)
        r.scale = Double(s.scale)
        return r
    }

    func log(_ c: LogCall) { NSLog("%@", c.line) }

    // Windows.

    func createWindow(_ c: CreateWindowCall) -> CreateWindowReply {
        var r = CreateWindowReply()
        let w: HostWindow
        if c.surface {
            let s = SurfaceWindow(id: c.id)
            r.layer = Int(bitPattern: Unmanaged.passUnretained(s.drawHost).toOpaque())
            w = s
        } else if !c.viewType.isEmpty {
            guard let v = PluginHost.makeView(c.viewType, id: c.id, args: c.viewArgs) else {
                r.error = "mygo: no view \(c.viewType) in this app"
                return r
            }
            w = PluginViewWindow(id: c.id, view: v)
        } else {
            w = WebWindow(id: c.id, c)
        }
        if c.hasBackground { w.setBackground(UIColor(rgba: c.background)) }
        w.ownScene = c.ownScene && Scenes.available
        windows[c.id] = w
        if c.parent != 0 { surface(c.parent)?.hold(w) }
        return r
    }

    func placeViews(_ c: PlaceViewsCall) { surface(c.id)?.placeViews(c.places) }

    func showWindow(_ c: ShowWindowCall) {
        guard let w = windows[c.id] else { return }
        if w.ownScene {
            Scenes.open(w)
            return
        }
        Navigation.shared.show(w)
        w.reportSize()
    }

    func hideWindow(_ c: HideWindowCall) {
        guard let w = windows[c.id] else { return }
        if w.ownScene {
            Scenes.close(w.id)
            return
        }
        Navigation.shared.hide(w)
    }

    func closeWindow(_ c: CloseWindowCall) {
        guard let w = windows.removeValue(forKey: c.id) else { return }
        if let holder = w.holder {
            holder.release(w)
            w.close()
            return
        }
        if w.ownScene { Scenes.close(w.id) }
        Navigation.shared.hide(w)
        Navigation.shared.forget(w)
        w.close()
    }

    func setTitle(_ c: SetTitleCall) { windows[c.id].map { Navigation.shared.controller($0).title = c.title } }
    func createTabs(_ c: CreateTabsCall) { Navigation.shared.createTabs(c.id) }
    func addTab(_ c: AddTabCall) {
        guard let w = windows[c.window] else { return }
        Navigation.shared.addTab(c.id, w, title: c.title, symbol: c.symbol, icon: c.icon)
    }
    func showTabs(_ c: ShowTabsCall) { Navigation.shared.showTabs(c.id) }
    func selectTab(_ c: SelectTabCall) { Navigation.shared.tabs[c.id]?.selectedIndex = c.index }
    func setTabBadge(_ c: SetTabBadgeCall) {
        Navigation.shared.tabs[c.id]?.viewControllers?[safe: c.index]?.tabBarItem.badgeValue = c.badge.isEmpty ? nil : c.badge
    }
    func closeTabs(_ c: CloseTabsCall) { Navigation.shared.closeTabs(c.id) }
    func pushWindow(_ c: PushWindowCall) {
        guard let p = windows[c.parent], let w = windows[c.child] else { return }
        Navigation.shared.push(w, over: p, title: c.title, animated: c.animated)
    }
    func setStack(_ c: SetStackCall) -> SetStackReply {
        if let w = windows[c.id] { Navigation.shared.setStack(w, titles: c.titles, keys: c.keys) }
        return SetStackReply()
    }
    func stackShown(_ c: StackShownCall) { windows[c.id].map { Navigation.shared.stackShown($0) } }
    func popWindow(_ c: PopWindowCall) {
        guard let w = windows[c.id] else { return }
        Navigation.shared.pop(w, animated: c.animated)
    }

    func setBackground(_ c: SetBackgroundCall) { windows[c.id]?.setBackground(UIColor(rgba: c.color)) }
    func focus(_ c: FocusCall) { windows[c.id]?.focus() }

    // Pages.

    func web(_ id: Int) -> WebWindow? { windows[id] as? WebWindow }

    func loadURL(_ c: LoadURLCall) { web(c.id)?.load(c.url) }
    func loadHTML(_ c: LoadHTMLCall) { web(c.id)?.loadHTML(c.html, base: c.baseURL) }
    func reload(_ c: ReloadCall) {
        if c.ignoreCache { web(c.id)?.webView.reloadFromOrigin() } else { web(c.id)?.webView.reload() }
    }
    func stopLoading(_ c: StopLoadingCall) { web(c.id)?.webView.stopLoading() }
    func goBack(_ c: GoBackCall) { web(c.id)?.webView.goBack() }
    func goForward(_ c: GoForwardCall) { web(c.id)?.webView.goForward() }
    func eval(_ c: EvalCall) { web(c.id)?.webView.evaluateJavaScript(c.js, completionHandler: nil) }

    func callAsync(_ c: CallAsyncCall) {
        guard let w = web(c.id) else {
            Go.send(Event.asyncResult(req: c.req, result: "", error: "mygo: the window is closed"))
            return
        }
        w.webView.callAsyncJavaScript(c.body, arguments: [:], in: nil, in: .page) { result in
            switch result {
            case .success(let v):
                Go.send(Event.asyncResult(req: c.req, result: (v as? String) ?? "", error: ""))
            case .failure(let e):
                // The exception's own message, as "SyntaxError: …", which Eval
                // looks for to run a script of statements as a function body;
                // the description is WebKit's "A JavaScript exception occurred".
                let msg = (e as NSError).userInfo["WKJavaScriptExceptionMessage"] as? String
                Go.send(Event.asyncResult(req: c.req, result: "", error: msg ?? e.localizedDescription))
            }
        }
    }

    func setZoom(_ c: SetZoomCall) { web(c.id)?.webView.pageZoom = CGFloat(c.factor) }
    func setUserAgent(_ c: SetUserAgentCall) { web(c.id)?.webView.customUserAgent = c.userAgent.isEmpty ? nil : c.userAgent }
    func setUserScripts(_ c: SetUserScriptsCall) { web(c.id)?.setScripts(c.scripts, c.scriptFlags) }

    func capturePage(_ c: CapturePageCall) {
        guard let w = web(c.id) else {
            Go.send(Event.captured(req: c.req, png: [], error: "mygo: the window is closed"))
            return
        }
        w.webView.takeSnapshot(with: nil) { image, error in
            if let png = image?.pngData() {
                Go.send(Event.captured(req: c.req, png: Array(png), error: ""))
            } else {
                Go.send(Event.captured(req: c.req, png: [], error: error?.localizedDescription ?? "mygo: no snapshot"))
            }
        }
    }

    func userAgent(_ c: UserAgentCall) -> UserAgentReply {
        var r = UserAgentReply()
        r.userAgent = web(c.id)?.userAgent ?? ""
        return r
    }

    func schemeRespond(_ c: SchemeRespondCall) { SchemeHandler.respond(c) }
    func schemeData(_ c: SchemeDataCall) { SchemeHandler.data(c) }
    func schemeFinish(_ c: SchemeFinishCall) { SchemeHandler.finish(c.req) }
    func schemeFail(_ c: SchemeFailCall) { SchemeHandler.fail(c.req, c.message) }

    // Dialogs.

    func messageBox(_ c: MessageBoxCall) {
        let title = c.title.isEmpty ? c.message : c.title
        var message = c.title.isEmpty ? c.detail : c.message
        if !c.title.isEmpty, !c.detail.isEmpty { message += "\n\n" + c.detail }
        let a = UIAlertController(title: title, message: message.isEmpty ? nil : message, preferredStyle: .alert)
        for (i, label) in c.buttons.enumerated() {
            let style: UIAlertAction.Style = i == c.cancelID && c.buttons.count > 1 ? .cancel : .default
            let action = UIAlertAction(title: label, style: style) { _ in
                Go.send(Event.messageBoxResult(req: c.req, response: i))
            }
            a.addAction(action)
            if i == c.defaultID { a.preferredAction = action }
        }
        present(a)
    }

    func openFiles(_ c: OpenFilesCall) {
        var types: [UTType] = c.extensions.compactMap { UTType(filenameExtension: $0) }
        if c.directories { types = [.folder] } else if types.isEmpty { types = [.item] }
        let p = UIDocumentPickerViewController(forOpeningContentTypes: types, asCopy: !c.directories)
        p.allowsMultipleSelection = c.multiple
        p.delegate = self
        pickers[p] = c.req
        present(p)
    }

    func documentPicker(_ p: UIDocumentPickerViewController, didPickDocumentsAt urls: [URL]) {
        guard let req = pickers.removeValue(forKey: p) else { return }
        for u in urls where !u.isFileURL || u.hasDirectoryPath { _ = u.startAccessingSecurityScopedResource() }
        Go.send(Event.filesResult(req: req, paths: urls.map { $0.path }, error: ""))
    }

    func documentPickerWasCancelled(_ p: UIDocumentPickerViewController) {
        guard let req = pickers.removeValue(forKey: p) else { return }
        Go.send(Event.filesResult(req: req, paths: [], error: ""))
    }

    func shareFile(_ c: ShareFileCall) {
        let vc = UIActivityViewController(activityItems: [URL(fileURLWithPath: c.path)], applicationActivities: nil)
        vc.popoverPresentationController?.sourceView = root.view
        vc.popoverPresentationController?.sourceRect = CGRect(x: root.view.bounds.midX, y: root.view.bounds.midY, width: 0, height: 0)
        present(vc)
    }

    // The system.

    func clipboardText() -> ClipboardTextReply {
        var r = ClipboardTextReply()
        let pb = UIPasteboard.general
        if pb.hasStrings {
            r.text = pb.string ?? ""
            r.ok = true
        }
        return r
    }

    func setClipboardText(_ c: SetClipboardTextCall) { UIPasteboard.general.string = c.text }

    func openURL(_ c: OpenURLCall) -> OpenURLReply {
        var r = OpenURLReply()
        guard let u = URL(string: c.url) else { return r }
        UIApplication.shared.open(u)
        r.ok = true
        return r
    }

    func secretQuery(_ service: String, _ account: String) -> [String: Any] {
        [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: account]
    }

    func secretGet(_ c: SecretGetCall) -> SecretGetReply {
        var r = SecretGetReply()
        var q = secretQuery(c.service, c.account)
        q[kSecReturnData as String] = true
        q[kSecMatchLimit as String] = kSecMatchLimitOne
        var out: CFTypeRef?
        let status = SecItemCopyMatching(q as CFDictionary, &out)
        switch status {
        case errSecSuccess:
            r.value = Array((out as? Data) ?? Data())
            r.found = true
        case errSecItemNotFound:
            break
        default:
            r.error = "mygo: keychain error \(status)"
        }
        return r
    }

    func secretSet(_ c: SecretSetCall) -> SecretSetReply {
        var r = SecretSetReply()
        let q = secretQuery(c.service, c.account)
        let attrs: [String: Any] = [kSecValueData as String: Data(c.value),
                                    kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlock]
        var status = SecItemUpdate(q as CFDictionary, attrs as CFDictionary)
        if status == errSecItemNotFound {
            status = SecItemAdd(q.merging(attrs) { $1 } as CFDictionary, nil)
        }
        if status != errSecSuccess { r.error = "mygo: keychain error \(status)" }
        return r
    }

    func secretDelete(_ c: SecretDeleteCall) -> SecretDeleteReply {
        var r = SecretDeleteReply()
        let status = SecItemDelete(secretQuery(c.service, c.account) as CFDictionary)
        if status != errSecSuccess, status != errSecItemNotFound { r.error = "mygo: keychain error \(status)" }
        return r
    }

    func notify(_ c: NotifyCall) {
        let center = UNUserNotificationCenter.current()
        center.requestAuthorization(options: [.alert, .sound, .badge]) { granted, error in
            guard granted else {
                Go.send(Event.notifyDone(req: c.req, error: error?.localizedDescription ?? "mygo: notifications are not allowed"))
                return
            }
            let content = UNMutableNotificationContent()
            content.title = c.title
            content.subtitle = c.subtitle
            content.body = c.body
            if !c.group.isEmpty { content.threadIdentifier = c.group }
            if !c.silent { content.sound = .default }
            let id = c.notificationID.isEmpty ? UUID().uuidString : c.notificationID
            center.add(UNNotificationRequest(identifier: id, content: content, trigger: nil)) { error in
                Go.send(Event.notifyDone(req: c.req, error: error?.localizedDescription ?? ""))
            }
        }
    }

    func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent n: UNNotification,
                                withCompletionHandler done: @escaping (UNNotificationPresentationOptions) -> Void) {
        // The app on screen hears of a push here: UIKit's other way needs a
        // background mode and a flag in the push.
        if n.request.trigger is UNPushNotificationTrigger {
            PluginHost.applicationEvent(.pushMessage(n.request.content.userInfo))
        }
        done([.banner, .list, .sound])
    }

    func userNotificationCenter(_ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse,
                                withCompletionHandler done: @escaping () -> Void) {
        let n = response.notification
        if n.request.trigger is UNPushNotificationTrigger {
            PluginHost.applicationEvent(.pushOpened(n.request.content.userInfo))
        }
        Go.send(Event.notificationClicked(notificationID: n.request.identifier))
        done()
    }

    func removeNotification(_ c: RemoveNotificationCall) {
        let center = UNUserNotificationCenter.current()
        center.removeDeliveredNotifications(withIdentifiers: [c.notificationID])
        center.removePendingNotificationRequests(withIdentifiers: [c.notificationID])
    }

    func removeAllNotifications() {
        UNUserNotificationCenter.current().removeAllDeliveredNotifications()
        UNUserNotificationCenter.current().removeAllPendingNotificationRequests()
    }

    func keepAwake(_ c: KeepAwakeCall) { UIApplication.shared.isIdleTimerDisabled = c.on }

    // Network and power.

    private var pathMonitor: NWPathMonitor?
    private var path: NWPath?
    private let pathQueue = DispatchQueue(label: "mygo.network")

    private func monitor() -> NWPathMonitor {
        if let m = pathMonitor { return m }
        let m = NWPathMonitor()
        pathMonitor = m
        let ready = DispatchSemaphore(value: 0)
        var first = true
        m.pathUpdateHandler = { [weak self] p in
            DispatchQueue.main.async { self?.path = p }
            if first {
                first = false
                self?.path = p
                ready.signal()
            } else if self?.watchingNetwork == true {
                Go.send(Event.networkChanged(online: p.status == .satisfied, metered: p.isExpensive || p.isConstrained))
            }
        }
        m.start(queue: pathQueue)
        // Its first answer comes within milliseconds.
        _ = ready.wait(timeout: .now() + 0.5)
        return m
    }

    private var watchingNetwork = false

    func networkStatus() -> NetworkStatusReply {
        _ = monitor()
        var r = NetworkStatusReply()
        if let p = path {
            r.online = p.status == .satisfied
            r.metered = p.isExpensive || p.isConstrained
        }
        return r
    }

    func watchNetwork() {
        watchingNetwork = true
        _ = monitor()
    }

    func battery() -> BatteryReply {
        let d = UIDevice.current
        d.isBatteryMonitoringEnabled = true
        var r = BatteryReply()
        r.level = Double(d.batteryLevel)
        r.charging = d.batteryState == .charging || d.batteryState == .full
        r.lowPower = ProcessInfo.processInfo.isLowPowerModeEnabled
        return r
    }

    // The system's bars and the screen.

    func setSystemBars(_ c: SetSystemBarsCall) {
        root.barStyle = c.style
        root.barsHidden = c.hidden
        root.setNeedsStatusBarAppearanceUpdate()
        root.setNeedsUpdateOfHomeIndicatorAutoHidden()
    }

    func setOrientations(_ c: SetOrientationsCall) {
        var m: UIInterfaceOrientationMask = []
        if c.mask & 1 != 0 { m.insert(.portrait) }
        if c.mask & 2 != 0 { m.insert(.portraitUpsideDown) }
        if c.mask & 4 != 0 { m.insert(.landscapeLeft) }
        if c.mask & 8 != 0 { m.insert(.landscapeRight) }
        root.orientations = m.isEmpty ? .all : m
        root.setNeedsUpdateOfSupportedInterfaceOrientations()
        window?.windowScene?.requestGeometryUpdate(.iOS(interfaceOrientations: root.orientations))
    }

    func share(_ c: ShareCall) {
        var items: [Any] = []
        if !c.text.isEmpty { items.append(c.text) }
        if let u = URL(string: c.url), !c.url.isEmpty { items.append(u) }
        items += c.paths.map { URL(fileURLWithPath: $0) }
        guard !items.isEmpty else {
            Go.send(Event.shareResult(req: c.req, error: "mygo: nothing to share"))
            return
        }
        let vc = UIActivityViewController(activityItems: items, applicationActivities: nil)
        popover(vc, x: c.x, y: c.y)
        present(vc) { Go.send(Event.shareResult(req: c.req, error: "")) }
    }

    /// Where a sheet points on an iPad, which shows it as a popover.
    private func popover(_ vc: UIViewController, x: Double, y: Double) {
        guard let p = vc.popoverPresentationController else { return }
        p.sourceView = root.view
        let at = x == 0 && y == 0 ? CGPoint(x: root.view.bounds.midX, y: root.view.bounds.midY) : CGPoint(x: x, y: y)
        p.sourceRect = CGRect(origin: at, size: .zero)
    }

    func saveFile(_ c: SaveFileCall) {
        // Exporting the copy under the name asked for.
        let dir = NSTemporaryDirectory() + UUID().uuidString
        let named = URL(fileURLWithPath: dir).appendingPathComponent(c.name)
        do {
            try FileManager.default.createDirectory(atPath: dir, withIntermediateDirectories: true)
            try FileManager.default.copyItem(at: URL(fileURLWithPath: c.path), to: named)
        } catch {
            Go.send(Event.filesResult(req: c.req, paths: [], error: error.localizedDescription))
            return
        }
        let p = UIDocumentPickerViewController(forExporting: [named], asCopy: true)
        p.delegate = self
        pickers[p] = c.req
        present(p)
    }

    func clipboardImage() -> ClipboardImageReply {
        var r = ClipboardImageReply()
        if UIPasteboard.general.hasImages, let png = UIPasteboard.general.image?.pngData() { r.png = Array(png) }
        return r
    }

    func setClipboardImage(_ c: SetClipboardImageCall) {
        UIPasteboard.general.image = UIImage(data: Data(c.png))
    }

    /// A menu shown by the app, as a context menu: an action sheet, from the
    /// point on an iPad.
    func popupMenu(_ c: PopupMenuCall) {
        let sheet = UIAlertController(title: nil, message: nil, preferredStyle: .actionSheet)
        c.items.withUnsafeBytes { buf in
            var r = WireReader(buf)
            while r.at < buf.count {
                let id = r.int(), label = r.string(), enabled = r.bool(), checked = r.bool()
                if id == 0 { continue }
                let a = UIAlertAction(title: label, style: .default) { _ in
                    Go.send(Event.menuPicked(req: c.req, item: id))
                }
                a.isEnabled = enabled
                if checked { a.setValue(true, forKey: "checked") }
                sheet.addAction(a)
            }
        }
        sheet.addAction(UIAlertAction(title: NSLocalizedString("Cancel", comment: ""), style: .cancel) { _ in
            Go.send(Event.menuPicked(req: c.req, item: 0))
        })
        popover(sheet, x: c.x, y: c.y)
        present(sheet)
    }

    func setBarItems(_ c: SetBarItemsCall) {
        guard let w = windows[c.id] else { return }
        var items: [UIBarButtonItem] = []
        for (i, title) in c.titles.enumerated() {
            let id = c.id
            let action = UIAction(title: title) { _ in Go.send(Event.barItemClicked(id: id, index: i)) }
            var image = c.symbols[i].isEmpty ? nil : UIImage(systemName: c.symbols[i])
            if image == nil, let d = Data(base64Encoded: c.icons[i]), !d.isEmpty {
                image = UIImage(data: d, scale: 3)?.withRenderingMode(.alwaysTemplate)
            }
            let item = UIBarButtonItem(primaryAction: action)
            item.image = image
            item.accessibilityLabel = title
            items.append(item)
        }
        Navigation.shared.controller(w).navigationItem.rightBarButtonItems = items.reversed()
    }

    func pluginCall(_ c: PluginCallCall) { PluginHost.call(c) }

    // Sound, called on the audio's thread.
    func audioOpen(_ c: AudioOpenCall) -> AudioOpenReply { Sound.shared.open(c) }
    func audioWrite(_ c: AudioWriteCall) -> AudioWriteReply { Sound.shared.write(c) }
    func audioClose() -> AudioCloseReply {
        Sound.shared.close()
        return AudioCloseReply()
    }

    func haptic(_ c: HapticCall) {
        switch c.kind {
        case "selection": UISelectionFeedbackGenerator().selectionChanged()
        case "light": UIImpactFeedbackGenerator(style: .light).impactOccurred()
        case "heavy": UIImpactFeedbackGenerator(style: .heavy).impactOccurred()
        case "success": UINotificationFeedbackGenerator().notificationOccurred(.success)
        case "warning": UINotificationFeedbackGenerator().notificationOccurred(.warning)
        case "error": UINotificationFeedbackGenerator().notificationOccurred(.error)
        default: UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        }
    }

    // Native UI.

    func surface(_ id: Int) -> SurfaceWindow? { windows[id] as? SurfaceWindow }

    func requestFrame(_ c: RequestFrameCall) { surface(c.id)?.requestFrame() }
    func setFrameRate(_ c: SetFrameRateCall) { surface(c.id)?.setFrameRate(c.fps) }
    func setTextInput(_ c: SetTextInputCall) { surface(c.id)?.setTextInput(c) }
    func setCursor(_ c: SetCursorCall) { surface(c.id)?.setCursor(c.cursor) }
    func accessibilityChanged(_ c: AccessibilityChangedCall) { surface(c.id)?.accessibilityChanged(focus: c.focus) }
    func announce(_ c: AnnounceCall) { UIAccessibility.post(notification: .announcement, argument: c.text) }
    func showMenu(_ c: ShowMenuCall) { surface(c.id)?.showMenu(c) }
    func hideMenu(_ c: HideMenuCall) { surface(c.id)?.hideMenu() }
    // iOS's back is the swipe from the edge the content handles itself.
    func setBackAvailable(_ c: SetBackAvailableCall) {}
    func showMagnifier(_ c: ShowMagnifierCall) { surface(c.id)?.showMagnifier(c) }
    func hideMagnifier(_ c: HideMagnifierCall) { surface(c.id)?.hideMagnifier() }
    // Go lays out and draws text with Core Text on iOS.
    func textMetrics(_ c: TextMetricsCall) -> TextMetricsReply { TextMetricsReply() }
    func measureText(_ c: MeasureTextCall) -> MeasureTextReply { MeasureTextReply() }
    func textAdvances(_ c: TextAdvancesCall) -> TextAdvancesReply { TextAdvancesReply() }
    func measureTextRun(_ c: MeasureTextRunCall) -> MeasureTextRunReply { MeasureTextRunReply() }
    func drawText(_ c: DrawTextCall) -> DrawTextReply { DrawTextReply() }
    func registerFont(_ c: RegisterFontCall) -> RegisterFontReply { RegisterFontReply() }
}
