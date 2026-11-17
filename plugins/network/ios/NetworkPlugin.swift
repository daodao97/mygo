import Foundation

/// One HEAD request through URLSession, which brings up what BSD sockets
/// alone do not: the cellular-data and local-network prompts, VPN and DNS64.
final class NetworkPlugin: MyGoPlugin {
    private var tasks: [Int64: URLSessionDataTask] = [:]

    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply) {
        let id = (args["id"] as? NSNumber)?.int64Value ?? 0
        switch method {
        case "prepare":
            guard let raw = args["url"] as? String, let url = URL(string: raw) else {
                reply(.failure(PluginFailure("failed", "invalid URL")))
                return
            }
            var request = URLRequest(url: url)
            request.httpMethod = "HEAD"
            let timeout = (args["timeout"] as? NSNumber)?.doubleValue ?? 10
            request.timeoutInterval = timeout > 0 ? timeout : 10
            let task = URLSession.shared.dataTask(with: request) { [weak self] _, _, error in
                DispatchQueue.main.async {
                    self?.tasks.removeValue(forKey: id)
                    // Any HTTP response proves the route: only transport failures count.
                    if let error {
                        reply(.failure(PluginFailure(NetworkPlugin.kind(error), error.localizedDescription)))
                    } else {
                        reply(.success(nil))
                    }
                }
            }
            tasks[id] = task
            task.resume()
        case "cancel":
            tasks[id]?.cancel()
            reply(.success(nil))
        default:
            reply(.failure(PluginFailure("NoMethod", method)))
        }
    }

    private static func kind(_ error: Error) -> String {
        guard let e = error as? URLError else { return "failed" }
        switch e.code {
        case .notConnectedToInternet, .dataNotAllowed, .internationalRoamingOff: return "offline"
        case .timedOut: return "timeout"
        case .cancelled: return "canceled"
        default: return "failed"
        }
    }
}
