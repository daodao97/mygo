import CoreMotion
import Foundation

final class SensorsPlugin: MyGoPlugin {
    private let motion = CMMotionManager()
    private let queue = OperationQueue()

    private func send(_ sensor: String, _ x: Double, _ y: Double, _ z: Double) {
        PluginHost.emit("sensors", sensor, ["X": x, "Y": y, "Z": z, "T": Int64(Date().timeIntervalSince1970 * 1000)])
    }

    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply) {
        let sensor = args["sensor"] as? String ?? ""
        let every = max(Double(args["intervalMs"] as? Int ?? 100), 10) / 1000
        switch (method, sensor) {
        case ("start", "accelerometer"):
            guard motion.isAccelerometerAvailable else { return reply(.failure(PluginFailure("NoSensor"))) }
            motion.accelerometerUpdateInterval = every
            motion.startAccelerometerUpdates(to: queue) { [weak self] d, _ in
                // Core Motion counts in gravities, against the screen's z.
                if let a = d?.acceleration { self?.send(sensor, a.x * 9.81, a.y * 9.81, a.z * 9.81) }
            }
        case ("start", "gyroscope"):
            guard motion.isGyroAvailable else { return reply(.failure(PluginFailure("NoSensor"))) }
            motion.gyroUpdateInterval = every
            motion.startGyroUpdates(to: queue) { [weak self] d, _ in
                if let r = d?.rotationRate { self?.send(sensor, r.x, r.y, r.z) }
            }
        case ("start", "magnetometer"):
            guard motion.isMagnetometerAvailable else { return reply(.failure(PluginFailure("NoSensor"))) }
            motion.magnetometerUpdateInterval = every
            motion.startMagnetometerUpdates(to: queue) { [weak self] d, _ in
                if let m = d?.magneticField { self?.send(sensor, m.x, m.y, m.z) }
            }
        case ("stop", "accelerometer"): motion.stopAccelerometerUpdates()
        case ("stop", "gyroscope"): motion.stopGyroUpdates()
        case ("stop", "magnetometer"): motion.stopMagnetometerUpdates()
        default: return reply(.failure(PluginFailure("NoMethod", method)))
        }
        reply(.success(nil))
    }
}
