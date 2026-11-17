import AVFoundation

/// 16 kHz mono signed 16-bit PCM from AVAudioEngine. One capture owns the
/// shared audio session at a time; every method runs on the main thread, the
/// input tap on the engine's render thread.
final class MicrophonePlugin: MyGoPlugin {
    fileprivate var capture: MicrophoneCapture?

    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply) {
        let token = (args["token"] as? NSNumber)?.int64Value ?? 0
        switch method {
        case "start":
            if capture != nil {
                reply(.failure(PluginFailure("Busy", "a capture runs")))
                return
            }
            AVAudioApplication.requestRecordPermission { granted in
                DispatchQueue.main.async {
                    guard granted else {
                        reply(.failure(PluginFailure("Denied", "microphone access is not allowed")))
                        return
                    }
                    if self.capture != nil {
                        reply(.failure(PluginFailure("Busy", "a capture runs")))
                        return
                    }
                    let c = MicrophoneCapture(token: token, owner: self)
                    if let failure = c.start() {
                        reply(.failure(PluginFailure("Failed", failure)))
                        return
                    }
                    self.capture = c
                    reply(.success(nil))
                }
            }
        case "stop":
            if let c = capture, c.token == token {
                c.finish(nil, canceled: args["cancel"] as? Bool ?? false)
            }
            reply(.success(nil))
        default:
            reply(.failure(PluginFailure("NoMethod", method)))
        }
    }
}

fileprivate final class MicrophoneCapture {
    let token: Int64
    weak var owner: MicrophonePlugin?
    let engine = AVAudioEngine()
    var tapped = false
    var active = false
    var done = false
    var interruption: NSObjectProtocol?

    init(token: Int64, owner: MicrophonePlugin) {
        self.token = token
        self.owner = owner
    }

    /// Starts capture, or says why it could not, having released all it took.
    func start() -> String? {
        let audio = AVAudioSession.sharedInstance()
        do {
            try audio.setCategory(.record, mode: .measurement, options: [.allowBluetoothHFP])
            // Audio input mutes system haptics by default, including the
            // feedback an app gives that recording started.
            try? audio.setAllowHapticsAndSystemSoundsDuringRecording(true)
            try audio.setActive(true)
            active = true
        } catch {
            release()
            return error.localizedDescription
        }
        let input = engine.inputNode
        let format = input.outputFormat(forBus: 0)
        guard format.sampleRate > 0, format.channelCount > 0,
              let target = AVAudioFormat(commonFormat: .pcmFormatInt16, sampleRate: 16000, channels: 1, interleaved: true),
              let converter = AVAudioConverter(from: format, to: target) else {
            release()
            return "Microphone unavailable"
        }
        let token = self.token
        input.installTap(onBus: 0, bufferSize: 1600, format: format) { [weak self] buffer, _ in
            let capacity = AVAudioFrameCount((Double(buffer.frameLength) * 16000 / format.sampleRate).rounded(.up)) + 32
            guard let pcm = AVAudioPCMBuffer(pcmFormat: target, frameCapacity: capacity) else { return }
            var supplied = false
            var failure: NSError?
            converter.convert(to: pcm, error: &failure) { _, status in
                if supplied {
                    status.pointee = .noDataNow
                    return nil
                }
                supplied = true
                status.pointee = .haveData
                return buffer
            }
            if let failure {
                let message = failure.localizedDescription
                DispatchQueue.main.async { self?.finish(message, canceled: false) }
                return
            }
            if pcm.frameLength > 0, let samples = pcm.int16ChannelData?[0] {
                let data = Data(bytes: samples, count: Int(pcm.frameLength) * 2)
                PluginHost.emit("microphone", "audio", ["token": token, "pcm": data.base64EncodedString()])
            }
        }
        tapped = true
        engine.prepare()
        do {
            try engine.start()
        } catch {
            release()
            return error.localizedDescription
        }
        interruption = NotificationCenter.default.addObserver(forName: AVAudioSession.interruptionNotification, object: audio, queue: .main) { [weak self] note in
            if let raw = note.userInfo?[AVAudioSessionInterruptionTypeKey] as? UInt,
               AVAudioSession.InterruptionType(rawValue: raw) == .began {
                self?.finish("Recording interrupted", canceled: false)
            }
        }
        return nil
    }

    /// Ends capture: told to Go with its error, unless canceled.
    func finish(_ error: String?, canceled: Bool) {
        guard !done else { return }
        done = true
        release()
        if owner?.capture === self { owner?.capture = nil }
        if !canceled {
            PluginHost.emit("microphone", "audio", ["token": token, "end": true, "error": error ?? ""])
        }
    }

    private func release() {
        engine.stop()
        if tapped {
            engine.inputNode.removeTap(onBus: 0)
            tapped = false
        }
        if active {
            try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
            active = false
        }
        if let interruption { NotificationCenter.default.removeObserver(interruption) }
        interruption = nil
    }
}
