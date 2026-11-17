// Windows showing web pages, in a WKWebView.

import UIKit
import WebKit

final class WebWindow: HostWindow, WKNavigationDelegate, WKUIDelegate {
    let webView: WKWebView
    private let messages = MessageHandler()
    private var observers: [NSKeyValueObservation] = []
    /// The load the app started, which the app is not asked about
    /// (Navigation.Unasked).
    private var appLoad = false
    private var pinchZoom = false
    var userAgent = ""

    init(id: Int, _ c: CreateWindowCall) {
        let config = WKWebViewConfiguration()
        config.allowsInlineMediaPlayback = true
        if c.autoplay { config.mediaTypesRequiringUserActionForPlayback = [] }
        if !c.userAgentSuffix.isEmpty { config.applicationNameForUserAgent = c.userAgentSuffix }
        for s in Set(c.schemes) where !WKWebView.handlesURLScheme(s) {
            config.setURLSchemeHandler(SchemeHandler.shared, forURLScheme: s)
        }
        config.userContentController.add(messages, name: "mygo")
        webView = WKWebView(frame: .zero, configuration: config)
        super.init(id: id)
        messages.window = self
        setScripts(c.scripts, c.scriptFlags)
        webView.frame = bounds
        webView.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        webView.navigationDelegate = self
        webView.uiDelegate = self
        webView.allowsBackForwardNavigationGestures = c.swipeNavigation
        webView.isInspectable = c.inspectable
        if !c.userAgent.isEmpty { webView.customUserAgent = c.userAgent }
        pinchZoom = c.pinchZoom
        addSubview(webView)
        observers = [
            webView.observe(\.title) { [weak self] _, _ in self?.reportState() },
            webView.observe(\.url) { [weak self] _, _ in self?.reportState() },
            webView.observe(\.isLoading) { [weak self] _, _ in self?.reportState() },
            webView.observe(\.canGoBack) { [weak self] _, _ in self?.reportState() },
            webView.observe(\.canGoForward) { [weak self] _, _ in self?.reportState() },
        ]
        webView.evaluateJavaScript("navigator.userAgent") { [weak self] v, _ in self?.userAgent = v as? String ?? "" }
    }

    required init?(coder: NSCoder) { fatalError() }

    func setScripts(_ scripts: [String], _ flags: [String]) {
        let ucc = webView.configuration.userContentController
        ucc.removeAllUserScripts()
        for (i, src) in scripts.enumerated() {
            let f = i < flags.count ? flags[i] : ""
            ucc.addUserScript(WKUserScript(source: src, injectionTime: f.contains("e") ? .atDocumentEnd : .atDocumentStart,
                                           forMainFrameOnly: !f.contains("a")))
        }
    }

    func load(_ url: String) {
        guard let u = URL(string: url) else { return }
        appLoad = true
        if u.isFileURL {
            webView.loadFileURL(u, allowingReadAccessTo: u.deletingLastPathComponent())
        } else {
            webView.load(URLRequest(url: u))
        }
    }

    func loadHTML(_ html: String, base: String) {
        appLoad = true
        webView.loadHTMLString(html, baseURL: URL(string: base))
    }

    override func setBackground(_ color: UIColor) {
        super.setBackground(color)
        webView.isOpaque = color.cgColor.alpha >= 1
        webView.backgroundColor = color
        webView.scrollView.backgroundColor = color
        webView.underPageBackgroundColor = color
    }

    override func focus() { webView.becomeFirstResponder() }

    override func close() {
        webView.configuration.userContentController.removeScriptMessageHandler(forName: "mygo")
        webView.stopLoading()
        observers = []
        super.close()
    }

    private func reportState() {
        Go.send(Event.pageState(id: id, url: webView.url?.absoluteString ?? "", title: webView.title ?? "",
                                loading: webView.isLoading, canGoBack: webView.canGoBack, canGoForward: webView.canGoForward))
    }

    // WKNavigationDelegate

    func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction,
                 decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        let url = action.request.url
        let main = action.targetFrame?.isMainFrame ?? true
        // Links of schemes no web view loads (mailto:, tel:, other apps')
        // go to the app the system has for them.
        if let url, let scheme = url.scheme?.lowercased(), !WKWebView.handlesURLScheme(scheme),
           webView.configuration.urlSchemeHandler(forURLScheme: scheme) == nil, scheme != "about", scheme != "blob", scheme != "data" {
            decisionHandler(.cancel)
            UIApplication.shared.open(url)
            return
        }
        let kind = action.navigationType
        var unasked = kind == .backForward || kind == .reload
        if main && appLoad && kind == .other {
            appLoad = false
            unasked = true
        }
        let user = kind == .linkActivated || kind == .formSubmitted
        let allow = Go.answer(Event.willNavigate(id: id, url: url?.absoluteString ?? "", mainFrame: main, userInitiated: user,
                                                 unasked: unasked, reload: kind == .reload && main)) { WillNavigateReply(&$0).allow } ?? true
        decisionHandler(allow ? .allow : .cancel)
    }

    func webView(_ webView: WKWebView, didStartProvisionalNavigation navigation: WKNavigation!) {
        Go.send(Event.navigationStarted(id: id, url: webView.url?.absoluteString ?? ""))
    }

    func webView(_ webView: WKWebView, didCommit navigation: WKNavigation!) {
        if !pinchZoom { webView.scrollView.pinchGestureRecognizer?.isEnabled = false }
        Go.send(Event.navigationCommitted(id: id, url: webView.url?.absoluteString ?? ""))
    }

    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        Go.send(Event.loadFinished(id: id))
    }

    func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) { failed(error) }

    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) { failed(error) }

    private func failed(_ error: Error) {
        let e = error as NSError
        if e.domain == NSURLErrorDomain && e.code == NSURLErrorCancelled { return }
        let url = (e.userInfo[NSURLErrorFailingURLStringErrorKey] as? String) ?? webView.url?.absoluteString ?? ""
        Go.send(Event.loadFailed(id: id, url: url, code: e.code, description: e.localizedDescription))
    }

    func webViewWebContentProcessDidTerminate(_ webView: WKWebView) {
        Go.send(Event.processGone(id: id, reason: "crashed"))
    }

    // WKUIDelegate

    func webView(_ webView: WKWebView, createWebViewWith config: WKWebViewConfiguration, for action: WKNavigationAction,
                 windowFeatures: WKWindowFeatures) -> WKWebView? {
        let url = action.request.url?.absoluteString ?? ""
        let handled = Go.answer(Event.newWindow(id: id, url: url, userInitiated: action.navigationType == .linkActivated)) {
            NewWindowReply(&$0).handled
        } ?? false
        if !handled, action.navigationType == .linkActivated, let u = action.request.url {
            UIApplication.shared.open(u)
        }
        return nil
    }

    func webViewDidClose(_ webView: WKWebView) { Go.send(Event.closedByPage(id: id)) }

    func webView(_ webView: WKWebView, runJavaScriptAlertPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo,
                 completionHandler: @escaping () -> Void) {
        let a = UIAlertController(title: nil, message: message, preferredStyle: .alert)
        a.addAction(UIAlertAction(title: NSLocalizedString("OK", comment: ""), style: .default) { _ in completionHandler() })
        Host.shared.present(a)
    }

    func webView(_ webView: WKWebView, runJavaScriptConfirmPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo,
                 completionHandler: @escaping (Bool) -> Void) {
        let a = UIAlertController(title: nil, message: message, preferredStyle: .alert)
        a.addAction(UIAlertAction(title: NSLocalizedString("Cancel", comment: ""), style: .cancel) { _ in completionHandler(false) })
        a.addAction(UIAlertAction(title: NSLocalizedString("OK", comment: ""), style: .default) { _ in completionHandler(true) })
        Host.shared.present(a)
    }

    func webView(_ webView: WKWebView, runJavaScriptTextInputPanelWithPrompt prompt: String, defaultText: String?,
                 initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping (String?) -> Void) {
        let a = UIAlertController(title: nil, message: prompt, preferredStyle: .alert)
        a.addTextField { $0.text = defaultText }
        a.addAction(UIAlertAction(title: NSLocalizedString("Cancel", comment: ""), style: .cancel) { _ in completionHandler(nil) })
        a.addAction(UIAlertAction(title: NSLocalizedString("OK", comment: ""), style: .default) { _ in
            completionHandler(a.textFields?.first?.text ?? "")
        })
        Host.shared.present(a)
    }

    func webView(_ webView: WKWebView, requestMediaCapturePermissionFor origin: WKSecurityOrigin, initiatedByFrame frame: WKFrameInfo,
                 type: WKMediaCaptureType, decisionHandler: @escaping (WKPermissionDecision) -> Void) {
        var kinds: [String] = []
        switch type {
        case .camera: kinds = ["camera"]
        case .microphone: kinds = ["microphone"]
        default: kinds = ["camera", "microphone"]
        }
        let o = "\(origin.protocol)://\(origin.host)" + (origin.port != 0 ? ":\(origin.port)" : "")
        let allow = Go.answer(Event.permissionRequested(id: id, kinds: kinds, origin: o)) { PermissionRequestedReply(&$0).allow } ?? false
        decisionHandler(allow ? .grant : .deny)
    }

    fileprivate func message(_ text: String) { Go.send(Event.message(id: id, text: text)) }
}

/// The handler of the bridge's messages, apart from the window, which
/// WebKit's user content controller keeps.
private final class MessageHandler: NSObject, WKScriptMessageHandler {
    weak var window: WebWindow?

    func userContentController(_ ucc: WKUserContentController, didReceive message: WKScriptMessage) {
        guard message.frameInfo.isMainFrame, let text = message.body as? String else { return }
        window?.message(text)
    }
}

/// The handler of the app's schemes: their requests go to Go, which
/// answers them with SchemeRespond, SchemeData and SchemeFinish.
final class SchemeHandler: NSObject, WKURLSchemeHandler {
    static let shared = SchemeHandler()
    private static var tasks: [Int: WKURLSchemeTask] = [:]
    private static var urls: [Int: URL] = [:]
    private static var next = 0

    func webView(_ webView: WKWebView, start task: WKURLSchemeTask) {
        SchemeHandler.next += 1
        let req = SchemeHandler.next
        SchemeHandler.tasks[req] = task
        SchemeHandler.urls[req] = task.request.url
        let window = Host.shared.windows.values.first { ($0 as? WebWindow)?.webView === webView }
        var header: [String] = []
        for (k, v) in task.request.allHTTPHeaderFields ?? [:] { header += [k, v] }
        var body: [UInt8] = []
        var hasBody = false
        if let data = task.request.httpBody {
            body = Array(data)
            hasBody = true
        } else if let stream = task.request.httpBodyStream {
            stream.open()
            var buf = [UInt8](repeating: 0, count: 65536)
            while stream.hasBytesAvailable {
                let n = stream.read(&buf, maxLength: buf.count)
                if n <= 0 { break }
                body += buf[0..<n]
            }
            stream.close()
            hasBody = true
        }
        Go.send(Event.schemeRequest(id: window?.id ?? 0, req: req, method: task.request.httpMethod ?? "GET",
                                    url: task.request.url?.absoluteString ?? "", header: header, body: body, hasBody: hasBody))
    }

    func webView(_ webView: WKWebView, stop task: WKURLSchemeTask) {
        guard let req = SchemeHandler.tasks.first(where: { $0.value === task })?.key else { return }
        SchemeHandler.tasks[req] = nil
        SchemeHandler.urls[req] = nil
        Go.send(Event.schemeCancel(req: req))
    }

    static func respond(_ c: SchemeRespondCall) {
        guard let task = tasks[c.req], let url = urls[c.req] else { return }
        var fields: [String: String] = [:]
        var i = 0
        while i + 1 < c.header.count {
            let k = c.header[i], v = c.header[i + 1]
            fields[k] = fields[k].map { $0 + ", " + v } ?? v
            i += 2
        }
        if let r = HTTPURLResponse(url: url, statusCode: c.status, httpVersion: "HTTP/1.1", headerFields: fields) {
            task.didReceive(r)
        }
    }

    static func data(_ c: SchemeDataCall) { tasks[c.req]?.didReceive(Data(c.data)) }

    static func finish(_ req: Int) {
        tasks.removeValue(forKey: req)?.didFinish()
        urls[req] = nil
    }

    static func fail(_ req: Int, _ message: String) {
        tasks.removeValue(forKey: req)?.didFailWithError(NSError(domain: "mygo", code: 1, userInfo: [NSLocalizedDescriptionKey: message]))
        urls[req] = nil
    }
}
