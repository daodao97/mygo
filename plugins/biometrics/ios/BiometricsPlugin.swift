import LocalAuthentication

final class BiometricsPlugin: MyGoPlugin {
    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply) {
        switch method {
        case "available":
            let c = LAContext()
            var kind = ""
            if c.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: nil) {
                switch c.biometryType {
                case .faceID: kind = "face"
                case .touchID: kind = "fingerprint"
                case .opticID: kind = "iris"
                default: kind = "biometric"
                }
            } else if c.canEvaluatePolicy(.deviceOwnerAuthentication, error: nil) {
                kind = "passcode"
            }
            reply(.success(["kind": kind]))
        case "authenticate":
            let reason = args["reason"] as? String ?? ""
            LAContext().evaluatePolicy(.deviceOwnerAuthentication, localizedReason: reason.isEmpty ? " " : reason) { ok, error in
                if ok {
                    reply(.success(nil))
                    return
                }
                switch (error as? LAError)?.code {
                case .userCancel, .appCancel, .systemCancel:
                    reply(.failure(PluginFailure("Canceled")))
                case .authenticationFailed:
                    reply(.failure(PluginFailure("NotRecognized")))
                default:
                    reply(.failure(PluginFailure("Failed", error?.localizedDescription ?? "")))
                }
            }
        default:
            reply(.failure(PluginFailure("NoMethod", method)))
        }
    }
}
