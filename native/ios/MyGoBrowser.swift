import Foundation
import SafariServices
import UIKit

@_silgen_name("goIOSSystemResult") private func browserResult(_ token: UInt64, _ value: UnsafePointer<CChar>, _ code: Int32, _ message: UnsafePointer<CChar>)

@MainActor private final class MyGoBrowser: NSObject, @preconcurrency SFSafariViewControllerDelegate, UIAdaptivePresentationControllerDelegate {
    static var requests: [UInt64: MyGoBrowser] = [:]
    static var canceled: Set<UInt64> = []
    let token: UInt64
    var controller: SFSafariViewController?
    init(_ token: UInt64) { self.token = token }
    func finish() {
        guard Self.requests.removeValue(forKey: token) != nil else { return }
        controller?.dismiss(animated: true)
        controller = nil
        "null".withCString { value in "".withCString { browserResult(token, value, 0, $0) } }
    }
    func safariViewControllerDidFinish(_ controller: SFSafariViewController) { finish() }
    func presentationControllerDidDismiss(_ presentationController: UIPresentationController) { finish() }
    static func open(_ token: UInt64, _ data: String) {
        if canceled.remove(token) != nil { return }
        guard let bytes = data.data(using: .utf8), let raw = try? JSONDecoder().decode(String.self, from: bytes), let url = URL(string: raw), ["http", "https"].contains(url.scheme ?? "") else { fail(token, "Invalid browser URL"); return }
        let windows = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.filter { $0.activationState == .foregroundActive }.flatMap { $0.windows }
        guard let root = windows.first(where: { $0.isKeyWindow })?.rootViewController, root.presentedViewController == nil else { fail(token, "Another system presentation is active"); return }
        let request = MyGoBrowser(token)
        let controller = SFSafariViewController(url: url)
        controller.delegate = request
        request.controller = controller
        requests[token] = request
        root.present(controller, animated: true)
        controller.presentationController?.delegate = request
    }
    static func fail(_ token: UInt64, _ message: String) { "null".withCString { value in message.withCString { browserResult(token, value, 2, $0) } } }
}

@_cdecl("mygo_swift_browser_open") func mygoBrowserOpen(_ token: UInt64, _ json: UnsafePointer<CChar>) {
    let value = String(cString: json)
    Task { @MainActor in MyGoBrowser.open(token, value) }
}
@_cdecl("mygo_swift_browser_cancel") func mygoBrowserCancel(_ token: UInt64) {
    Task { @MainActor in
        if let request = MyGoBrowser.requests[token] { request.finish() }
        else { if MyGoBrowser.canceled.count > 128 { MyGoBrowser.canceled.removeAll() }; MyGoBrowser.canceled.insert(token); MyGoBrowser.fail(token, "Canceled") }
    }
}
