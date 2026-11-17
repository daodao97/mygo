// The sound of package audio, through AVAudioEngine: Go's output writes from a
// thread of its own, which waits here while the engine holds enough.

import AVFoundation

final class Sound {
    static let shared = Sound()

    private let engine = AVAudioEngine()
    private let player = AVAudioPlayerNode()
    private var format: AVAudioFormat?
    private var room: DispatchSemaphore?
    private var attached = false

    func open(_ c: AudioOpenCall) -> AudioOpenReply {
        var r = AudioOpenReply()
        do {
            let s = AVAudioSession.sharedInstance()
            // Sounds follow the side switch, and mix with what else plays,
            // until the app asks for more of the session.
            if s.category == .soloAmbient { try s.setCategory(.ambient) }
            try s.setActive(true)
            guard let f = AVAudioFormat(commonFormat: .pcmFormatFloat32, sampleRate: Double(c.rate), channels: 2, interleaved: false) else {
                r.error = "audio: no such format"
                return r
            }
            format = f
            if !attached {
                engine.attach(player)
                engine.connect(player, to: engine.mainMixerNode, format: f)
                attached = true
            }
            // Writes of 10 ms: as many as hold what was asked for.
            room = DispatchSemaphore(value: max(2, c.ahead / 10))
            try engine.start()
            player.play()
        } catch {
            r.error = "audio: \(error.localizedDescription)"
        }
        return r
    }

    func write(_ c: AudioWriteCall) -> AudioWriteReply {
        var r = AudioWriteReply()
        guard let f = format, let room else {
            r.error = "audio: closed"
            return r
        }
        let frames = c.samples.count / 8
        guard frames > 0, let b = AVAudioPCMBuffer(pcmFormat: f, frameCapacity: AVAudioFrameCount(frames)),
              let left = b.floatChannelData?[0], let right = b.floatChannelData?[1] else { return r }
        b.frameLength = AVAudioFrameCount(frames)
        c.samples.withUnsafeBytes { raw in
            let s = raw.bindMemory(to: Float32.self)
            for i in 0..<frames {
                left[i] = s[2 * i]
                right[i] = s[2 * i + 1]
            }
        }
        // An engine that stopped, as for a phone call, plays nothing more and
        // frees no room: the output opens it again.
        if !engine.isRunning || room.wait(timeout: .now() + 1) == .timedOut {
            r.error = "audio: the engine stopped"
            return r
        }
        player.scheduleBuffer(b) { room.signal() }
        return r
    }

    func close() {
        player.stop()
        engine.stop()
        room = nil
    }
}
