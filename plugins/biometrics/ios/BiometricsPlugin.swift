import LocalAuthentication
import UIKit

final class BiometricsPlugin: MyGoPlugin {
    // busy is set while a prompt shows; main thread only.
    private var busy = false

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
            DispatchQueue.main.async { self.authenticate(args, reply) }
        default:
            reply(.failure(PluginFailure("NoMethod", method)))
        }
    }

    private func authenticate(_ args: [String: Any], _ reply: @escaping PluginReply) {
        if busy {
            reply(.failure(PluginFailure("Busy")))
            return
        }
        if UIApplication.shared.applicationState != .active {
            reply(.failure(PluginFailure("Inactive")))
            return
        }
        let c = LAContext()
        if c.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: nil),
           c.biometryType == .faceID,
           Bundle.main.object(forInfoDictionaryKey: "NSFaceIDUsageDescription") == nil {
            reply(.failure(PluginFailure("MissingPurpose")))
            return
        }
        busy = true
        let reason = args["reason"] as? String ?? ""
        c.evaluatePolicy(.deviceOwnerAuthentication, localizedReason: reason.isEmpty ? " " : reason) { ok, error in
            DispatchQueue.main.async { self.busy = false }
            if ok {
                reply(.success(nil))
            } else {
                reply(.failure(BiometricsPlugin.failure(error)))
            }
        }
    }

    private static func failure(_ error: Error?) -> PluginFailure {
        let message = error?.localizedDescription ?? ""
        switch (error as? LAError)?.code {
        case .userCancel, .appCancel, .systemCancel, .userFallback:
            return PluginFailure("Canceled")
        case .authenticationFailed:
            return PluginFailure("NotRecognized")
        case .biometryNotAvailable:
            return PluginFailure("Unavailable", message)
        case .biometryNotEnrolled:
            return PluginFailure("NotEnrolled", message)
        case .biometryLockout:
            return PluginFailure("LockedOut", message)
        case .passcodeNotSet:
            return PluginFailure("PasscodeNotSet", message)
        case .notInteractive:
            return PluginFailure("Inactive", message)
        default:
            return PluginFailure("Failed", message)
        }
    }
}
