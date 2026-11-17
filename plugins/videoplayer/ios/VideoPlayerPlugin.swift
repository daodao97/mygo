import AVFoundation
import UIKit

private final class PlayerView: UIView {
    override class var layerClass: AnyClass { AVPlayerLayer.self }
    let player = AVPlayer()
    var loop = false
    var id = 0
    var observers: [NSKeyValueObservation] = []
    var timeObserver: Any?
    var ended = false

    func send() {
        let item = player.currentItem
        let dur = item?.duration.seconds ?? 0
        let size = item?.presentationSize ?? .zero
        PluginHost.emit("videoplayer", "state", [
            "id": id, "ready": item?.status == .readyToPlay, "playing": player.rate != 0, "ended": ended,
            "positionMs": max(0, player.currentTime().seconds * 1000), "durationMs": dur.isFinite ? dur * 1000 : 0,
            "width": Int(size.width), "height": Int(size.height), "error": item?.error?.localizedDescription ?? "",
        ] as [String: Any])
    }
}

final class VideoPlayerPlugin: NSObject, MyGoPlugin {
    private var views: [Int: PlayerView] = [:]

    override init() {
        super.init()
        PluginHost.registerView("videoplayer") { [weak self] id, args in
            let v = PlayerView()
            v.id = id
            (v.layer as! AVPlayerLayer).player = v.player
            self?.views[id] = v
            self?.start(v, args)
            return v
        }
    }

    private func start(_ v: PlayerView, _ args: [String: Any]) {
        let source = args["source"] as? String ?? ""
        let url = source.contains("://") ? URL(string: source) : URL(fileURLWithPath: source)
        guard let url else { return }
        let item = AVPlayerItem(url: url)
        v.loop = args["loop"] as? Bool ?? false
        v.player.isMuted = args["muted"] as? Bool ?? false
        v.player.replaceCurrentItem(with: item)
        // Plays with the sound on, whatever the side switch says.
        try? AVAudioSession.sharedInstance().setCategory(.playback)
        try? AVAudioSession.sharedInstance().setActive(true)
        v.observers = [
            item.observe(\.status) { [weak v] _, _ in v?.send() },
            v.player.observe(\.rate) { [weak v] _, _ in v?.send() },
        ]
        v.timeObserver = v.player.addPeriodicTimeObserver(forInterval: CMTime(value: 1, timescale: 4), queue: .main) { [weak v] _ in v?.send() }
        NotificationCenter.default.addObserver(forName: .AVPlayerItemDidPlayToEndTime, object: item, queue: .main) { [weak v] _ in
            guard let v else { return }
            if v.loop {
                v.player.seek(to: .zero)
                v.player.play()
            } else {
                v.ended = true
                v.send()
            }
        }
        if args["autoPlay"] as? Bool ?? false { v.player.play() }
    }

    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply) {
        guard let id = args["id"] as? Int, let v = views[id] else {
            reply(.failure(PluginFailure("NoView")))
            return
        }
        switch method {
        case "play":
            v.ended = false
            v.player.play()
        case "pause": v.player.pause()
        case "seek":
            v.ended = false
            let ms = args["ms"] as? Double ?? Double(args["ms"] as? Int ?? 0)
            v.player.seek(to: CMTime(seconds: ms / 1000, preferredTimescale: 600), toleranceBefore: .zero, toleranceAfter: .zero)
        case "volume": v.player.volume = Float(args["volume"] as? Double ?? 1)
        case "rate": v.player.rate = Float(args["rate"] as? Double ?? 1)
        case "dispose":
            v.player.pause()
            if let t = v.timeObserver { v.player.removeTimeObserver(t) }
            v.observers = []
            views[id] = nil
        default:
            reply(.failure(PluginFailure("NoMethod", method)))
            return
        }
        reply(.success(nil))
    }
}
