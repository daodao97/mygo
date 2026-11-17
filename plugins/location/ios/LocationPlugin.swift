import CoreLocation
import Foundation

final class LocationPlugin: NSObject, MyGoPlugin, CLLocationManagerDelegate {
    private let manager = CLLocationManager()
    private var waitingFix: [PluginReply] = []
    private var waitingAuth: [PluginReply] = []
    private var watching = false

    override init() {
        super.init()
        manager.delegate = self
        manager.desiredAccuracy = kCLLocationAccuracyBest
    }

    private var granted: Bool {
        let s = manager.authorizationStatus
        return s == .authorizedWhenInUse || s == .authorizedAlways
    }

    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply) {
        switch method {
        case "requestPermission":
            if manager.authorizationStatus == .notDetermined {
                waitingAuth.append(reply)
                manager.requestWhenInUseAuthorization()
            } else {
                reply(.success(["granted": granted]))
            }
        case "current":
            waitingFix.append(reply)
            if manager.authorizationStatus == .notDetermined {
                manager.requestWhenInUseAuthorization()
            } else if !granted {
                failFixes(PluginFailure("Denied", "the app may not know where the device is"))
            } else {
                manager.requestLocation()
            }
        case "start":
            watching = true
            if manager.authorizationStatus == .notDetermined { manager.requestWhenInUseAuthorization() }
            manager.startUpdatingLocation()
            reply(.success(nil))
        case "stop":
            watching = false
            manager.stopUpdatingLocation()
            reply(.success(nil))
        default:
            reply(.failure(PluginFailure("NoMethod", method)))
        }
    }

    private func failFixes(_ f: PluginFailure) {
        let list = waitingFix
        waitingFix = []
        for r in list { r(.failure(f)) }
    }

    func locationManagerDidChangeAuthorization(_ m: CLLocationManager) {
        guard m.authorizationStatus != .notDetermined else { return }
        let auth = waitingAuth
        waitingAuth = []
        for r in auth { r(.success(["granted": granted])) }
        if !waitingFix.isEmpty {
            if granted {
                m.requestLocation()
            } else {
                failFixes(PluginFailure("Denied", "the app may not know where the device is"))
            }
        }
    }

    private func encode(_ l: CLLocation) -> [String: Any] {
        ["latitude": l.coordinate.latitude, "longitude": l.coordinate.longitude, "altitude": l.verticalAccuracy >= 0 ? l.altitude : -1,
         "speed": l.speed, "heading": l.course, "accuracy": l.horizontalAccuracy, "time": Int64(l.timestamp.timeIntervalSince1970 * 1000)]
    }

    func locationManager(_ m: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        guard let l = locations.last else { return }
        let fix = encode(l)
        let list = waitingFix
        waitingFix = []
        for r in list { r(.success(fix)) }
        if watching { PluginHost.emit("location", "fix", fix) }
    }

    func locationManager(_ m: CLLocationManager, didFailWithError error: Error) {
        failFixes(PluginFailure("Failed", error.localizedDescription))
    }
}
