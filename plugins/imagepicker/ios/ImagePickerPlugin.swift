import PhotosUI
import UIKit
import UniformTypeIdentifiers

final class ImagePickerPlugin: NSObject, MyGoPlugin, PHPickerViewControllerDelegate, UIImagePickerControllerDelegate, UINavigationControllerDelegate {
    private var picking: PluginReply?
    private var capturing: PluginReply?

    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply) {
        switch method {
        case "pick":
            picking = reply
            var cfg = PHPickerConfiguration()
            switch args["kind"] as? String {
            case "videos": cfg.filter = .videos
            case "any": cfg.filter = .any(of: [.images, .videos])
            default: cfg.filter = .images
            }
            let limit = args["limit"] as? Int ?? 1
            cfg.selectionLimit = limit < 0 ? 0 : limit
            let vc = PHPickerViewController(configuration: cfg)
            vc.delegate = self
            PluginHost.present(vc)
        case "capture":
            guard UIImagePickerController.isSourceTypeAvailable(.camera) else {
                reply(.failure(PluginFailure("NoCamera", "the device has no camera")))
                return
            }
            capturing = reply
            let vc = UIImagePickerController()
            vc.sourceType = .camera
            vc.delegate = self
            PluginHost.present(vc)
        default:
            reply(.failure(PluginFailure("NoMethod", method)))
        }
    }

    func picker(_ vc: PHPickerViewController, didFinishPicking results: [PHPickerResult]) {
        vc.dismiss(animated: true)
        guard let reply = picking else { return }
        picking = nil
        let group = DispatchGroup()
        let dir = NSTemporaryDirectory() + UUID().uuidString
        try? FileManager.default.createDirectory(atPath: dir, withIntermediateDirectories: true)
        var paths = [String?](repeating: nil, count: results.count)
        let lock = NSLock()
        for (i, r) in results.enumerated() {
            let p = r.itemProvider
            guard let type = p.registeredTypeIdentifiers.first else { continue }
            group.enter()
            p.loadFileRepresentation(forTypeIdentifier: type) { url, _ in
                defer { group.leave() }
                guard let url else { return }
                let to = URL(fileURLWithPath: dir).appendingPathComponent("\(i)-" + url.lastPathComponent)
                if (try? FileManager.default.copyItem(at: url, to: to)) != nil {
                    lock.lock()
                    paths[i] = to.path
                    lock.unlock()
                }
            }
        }
        group.notify(queue: .main) { reply(.success(["paths": paths.compactMap { $0 }])) }
    }

    func imagePickerController(_ vc: UIImagePickerController, didFinishPickingMediaWithInfo info: [UIImagePickerController.InfoKey: Any]) {
        vc.dismiss(animated: true)
        guard let reply = capturing else { return }
        capturing = nil
        guard let image = info[.originalImage] as? UIImage, let jpeg = image.jpegData(compressionQuality: 0.9) else {
            reply(.failure(PluginFailure("Failed", "no photo")))
            return
        }
        let path = NSTemporaryDirectory() + UUID().uuidString + ".jpg"
        do {
            try jpeg.write(to: URL(fileURLWithPath: path))
            reply(.success(["path": path]))
        } catch {
            reply(.failure(PluginFailure("Failed", error.localizedDescription)))
        }
    }

    func imagePickerControllerDidCancel(_ vc: UIImagePickerController) {
        vc.dismiss(animated: true)
        capturing?(.success(["path": ""]))
        capturing = nil
    }
}
