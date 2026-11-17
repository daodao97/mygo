import AVFoundation
import UIKit

/// A full-screen scanner of QR codes, AVCaptureMetadataOutput's.
final class ScannerPlugin: MyGoPlugin {
    private var scanning: (id: Int64, controller: ScannerController)?

    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply) {
        let id = (args["id"] as? NSNumber)?.int64Value ?? 0
        switch method {
        case "scan":
            if scanning != nil {
                reply(.failure(PluginFailure("Busy", "a scan runs")))
                return
            }
            let prompt = args["prompt"] as? String ?? ""
            let cancelLabel = args["cancelLabel"] as? String ?? ""
            AVCaptureDevice.requestAccess(for: .video) { allowed in
                DispatchQueue.main.async {
                    guard allowed else {
                        reply(.failure(PluginFailure("Denied", "camera access is not allowed")))
                        return
                    }
                    if self.scanning != nil {
                        reply(.failure(PluginFailure("Busy", "a scan runs")))
                        return
                    }
                    let controller = ScannerController(prompt: prompt, cancelLabel: cancelLabel) { value, code in
                        self.scanning = nil
                        if let code {
                            reply(.failure(PluginFailure(code)))
                        } else {
                            reply(.success(["value": value ?? ""]))
                        }
                    }
                    controller.modalPresentationStyle = .fullScreen
                    self.scanning = (id, controller)
                    PluginHost.window?.endEditing(true)
                    PluginHost.present(controller)
                }
            }
        case "cancel":
            if let s = scanning, s.id == id { s.controller.cancelScan() }
            reply(.success(nil))
        default:
            reply(.failure(PluginFailure("NoMethod", method)))
        }
    }
}

final class ScannerController: UIViewController, @preconcurrency AVCaptureMetadataOutputObjectsDelegate {
    private let prompt: String
    private let cancelLabel: String
    private var done: ((String?, String?) -> Void)?
    private let capture = AVCaptureSession()
    private var preview: AVCaptureVideoPreviewLayer?
    private let queue = DispatchQueue(label: "mygo.scanner")

    init(prompt: String, cancelLabel: String, done: @escaping (String?, String?) -> Void) {
        self.prompt = prompt
        self.cancelLabel = cancelLabel
        self.done = done
        super.init(nibName: nil, bundle: nil)
    }

    required init?(coder: NSCoder) { fatalError() }

    override func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = .black
        view.accessibilityIdentifier = "MyGo scanner"
        let output = AVCaptureMetadataOutput()
        guard let device = AVCaptureDevice.default(for: .video),
              let input = try? AVCaptureDeviceInput(device: device),
              capture.canAddInput(input), capture.canAddOutput(output) else {
            DispatchQueue.main.async { self.finish(nil, "Unavailable") }
            return
        }
        capture.addInput(input)
        capture.addOutput(output)
        output.setMetadataObjectsDelegate(self, queue: .main)
        output.metadataObjectTypes = [.qr]
        let preview = AVCaptureVideoPreviewLayer(session: capture)
        preview.videoGravity = .resizeAspectFill
        view.layer.addSublayer(preview)
        self.preview = preview

        let label = UILabel()
        label.text = prompt
        label.textColor = .white
        label.textAlignment = .center
        label.numberOfLines = 0
        label.adjustsFontForContentSizeCategory = true
        label.font = .preferredFont(forTextStyle: .headline)
        label.translatesAutoresizingMaskIntoConstraints = false
        label.isHidden = prompt.isEmpty
        view.addSubview(label)
        let cancel = UIButton(type: .system)
        cancel.setTitle(cancelLabel.isEmpty ? NSLocalizedString("Cancel", comment: "") : cancelLabel, for: .normal)
        cancel.setTitleColor(.white, for: .normal)
        cancel.titleLabel?.font = .preferredFont(forTextStyle: .body)
        cancel.translatesAutoresizingMaskIntoConstraints = false
        cancel.addTarget(self, action: #selector(cancelScan), for: .touchUpInside)
        view.addSubview(cancel)
        let safe = view.safeAreaLayoutGuide
        NSLayoutConstraint.activate([
            label.topAnchor.constraint(equalTo: safe.topAnchor, constant: 60),
            label.leadingAnchor.constraint(equalTo: safe.leadingAnchor, constant: 24),
            label.trailingAnchor.constraint(equalTo: safe.trailingAnchor, constant: -24),
            cancel.bottomAnchor.constraint(equalTo: safe.bottomAnchor, constant: -28),
            cancel.centerXAnchor.constraint(equalTo: view.centerXAnchor),
            cancel.heightAnchor.constraint(greaterThanOrEqualToConstant: 48),
            cancel.widthAnchor.constraint(greaterThanOrEqualToConstant: 120),
        ])
        let session = capture
        queue.async { session.startRunning() }
    }

    override func viewDidLayoutSubviews() {
        super.viewDidLayoutSubviews()
        preview?.frame = view.bounds
        let angle: CGFloat
        switch view.window?.windowScene?.effectiveGeometry.interfaceOrientation ?? .portrait {
        case .landscapeLeft: angle = 180
        case .landscapeRight: angle = 0
        case .portraitUpsideDown: angle = 270
        default: angle = 90
        }
        if let c = preview?.connection, c.isVideoRotationAngleSupported(angle) { c.videoRotationAngle = angle }
    }

    override func viewDidDisappear(_ animated: Bool) {
        super.viewDidDisappear(animated)
        let session = capture
        queue.async { session.stopRunning() }
    }

    @objc func cancelScan() { finish(nil, "Canceled") }

    /// Answers once, after the scanner went away, so the app can present again at once.
    private func finish(_ value: String?, _ code: String?) {
        guard let done else { return }
        self.done = nil
        if presentingViewController != nil {
            dismiss(animated: true) { done(value, code) }
        } else {
            done(value, code)
        }
    }

    func metadataOutput(_ output: AVCaptureMetadataOutput, didOutput objects: [AVMetadataObject], from connection: AVCaptureConnection) {
        for object in objects {
            if let code = object as? AVMetadataMachineReadableCodeObject, let value = code.stringValue, !value.isEmpty {
                finish(value, nil)
                return
            }
        }
    }
}
