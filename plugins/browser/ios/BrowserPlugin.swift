import SafariServices
import UIKit

/// SFSafariViewController, answering as the user closes it.
final class BrowserPlugin: NSObject, MyGoPlugin, @preconcurrency SFSafariViewControllerDelegate, @preconcurrency UIAdaptivePresentationControllerDelegate {
    private var showing: (id: Int64, controller: SFSafariViewController, reply: PluginReply)?

    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply) {
        let id = (args["id"] as? NSNumber)?.int64Value ?? 0
        switch method {
        case "open":
            guard let raw = args["url"] as? String, let url = URL(string: raw),
                  ["http", "https"].contains(url.scheme?.lowercased() ?? "") else {
                reply(.failure(PluginFailure("Invalid", "not an http or https URL")))
                return
            }
            if showing != nil {
                reply(.failure(PluginFailure("Busy", "a page shows")))
                return
            }
            let controller = SFSafariViewController(url: url)
            controller.delegate = self
            controller.presentationController?.delegate = self
            showing = (id, controller, reply)
            PluginHost.present(controller)
        case "close":
            if let s = showing, s.id == id {
                s.controller.dismiss(animated: true)
                finish()
            }
            reply(.success(nil))
        default:
            reply(.failure(PluginFailure("NoMethod", method)))
        }
    }

    private func finish() {
        guard let s = showing else { return }
        showing = nil
        s.reply(.success(nil))
    }

    func safariViewControllerDidFinish(_ controller: SFSafariViewController) { finish() }
    func presentationControllerDidDismiss(_ presentationController: UIPresentationController) { finish() }
}
