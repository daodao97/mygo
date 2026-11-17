import MapKit
import UIKit

/// A pin, which knows its ID.
private final class Pin: MKPointAnnotation {
    var id = ""
}

/// Zoom as web maps have it, from MapKit's span of longitude.
private func span(zoom: Double, height: CGFloat, width: CGFloat) -> MKCoordinateSpan {
    let lng = 360 / pow(2, zoom)
    let ratio = width > 0 ? Double(height / width) : 1
    return MKCoordinateSpan(latitudeDelta: min(lng * ratio, 180), longitudeDelta: min(lng, 360))
}

private final class MapHolder: NSObject, MKMapViewDelegate, UIGestureRecognizerDelegate {
    let id: Int
    let map = MKMapView()
    var pins: [String: Pin] = [:]
    /// What to show once the view has a size.
    var pending: (CLLocationCoordinate2D, Double)?
    /// The pin selected by the tap being handled, which has told already.
    weak var justSelected: Pin?

    init(id: Int) {
        self.id = id
        super.init()
        map.delegate = self
        // A tap once a double tap, which zooms, is ruled out.
        let twice = UITapGestureRecognizer()
        twice.numberOfTapsRequired = 2
        twice.delegate = self
        map.addGestureRecognizer(twice)
        let tap = UITapGestureRecognizer(target: self, action: #selector(tapped(_:)))
        tap.delegate = self
        tap.require(toFail: twice)
        map.addGestureRecognizer(tap)
    }

    func show(_ place: CLLocationCoordinate2D, zoom: Double, animated: Bool) {
        // MapKit raises on what's off its map.
        let at = CLLocationCoordinate2D(latitude: max(-85, min(85, place.latitude)), longitude: max(-180, min(180, place.longitude)))
        guard map.bounds.width > 0 else {
            pending = (at, zoom)
            return
        }
        map.setRegion(MKCoordinateRegion(center: at, span: span(zoom: zoom, height: map.bounds.height, width: map.bounds.width)), animated: animated)
    }

    func gestureRecognizer(_ g: UIGestureRecognizer, shouldRecognizeSimultaneouslyWith other: UIGestureRecognizer) -> Bool { true }

    @objc func tapped(_ g: UITapGestureRecognizer) {
        let p = g.location(in: map)
        // A pin's own tap, not the map's: mapView(_:didSelect:) tells of its first,
        // and this of those while it stays selected.
        var v = map.hitTest(p, with: nil)
        while let view = v, !(view is MKAnnotationView) { v = view.superview }
        if let pin = (v as? MKAnnotationView)?.annotation as? Pin {
            if justSelected !== pin { PluginHost.emit("maps", "markerTap", ["id": id, "marker": pin.id]) }
            justSelected = nil
            return
        }
        justSelected = nil
        let at = map.convert(p, toCoordinateFrom: map)
        PluginHost.emit("maps", "tap", ["id": id, "lat": at.latitude, "lng": at.longitude])
    }

    func mapViewDidChangeVisibleRegion(_ m: MKMapView) {
        if let (at, zoom) = pending, m.bounds.width > 0 {
            pending = nil
            show(at, zoom: zoom, animated: false)
        }
    }

    func mapView(_ m: MKMapView, didSelect annotation: MKAnnotation) {
        if let pin = annotation as? Pin {
            justSelected = pin
            PluginHost.emit("maps", "markerTap", ["id": id, "marker": pin.id])
        }
    }

    func mapView(_ m: MKMapView, viewFor annotation: MKAnnotation) -> MKAnnotationView? {
        guard annotation is Pin else { return nil }
        let v = m.dequeueReusableAnnotationView(withIdentifier: "pin") as? MKMarkerAnnotationView
            ?? MKMarkerAnnotationView(annotation: annotation, reuseIdentifier: "pin")
        v.annotation = annotation
        v.canShowCallout = annotation.title != nil
        return v
    }
}

final class MapsPlugin: MyGoPlugin {
    private var maps: [Int: MapHolder] = [:]

    init() {
        PluginHost.registerView("map") { [weak self] id, args in
            let h = MapHolder(id: id)
            self?.maps[id] = h
            h.show(CLLocationCoordinate2D(latitude: args["lat"] as? Double ?? 0, longitude: args["lng"] as? Double ?? 0),
                   zoom: args["zoom"] as? Double ?? 2, animated: false)
            return h.map
        }
    }

    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply) {
        guard let id = args["id"] as? Int, let h = maps[id] else {
            reply(.failure(PluginFailure("NoMap", "no map \(args["id"] ?? "")")))
            return
        }
        let at = CLLocationCoordinate2D(latitude: args["lat"] as? Double ?? 0, longitude: args["lng"] as? Double ?? 0)
        switch method {
        case "setCamera":
            h.show(at, zoom: args["zoom"] as? Double ?? 2, animated: args["animated"] as? Bool ?? false)
            reply(.success(nil))
        case "camera":
            let r = h.map.region
            let zoom = r.span.longitudeDelta > 0 ? log2(360 / r.span.longitudeDelta) : 0
            reply(.success(["Lat": r.center.latitude, "Lng": r.center.longitude, "Zoom": zoom]))
        case "addMarker":
            let name = args["marker"] as? String ?? ""
            if let old = h.pins[name] { h.map.removeAnnotation(old) }
            let pin = Pin()
            pin.id = name
            pin.coordinate = at
            if let t = args["title"] as? String, !t.isEmpty { pin.title = t }
            h.pins[name] = pin
            h.map.addAnnotation(pin)
            reply(.success(nil))
        case "removeMarker":
            let name = args["marker"] as? String ?? ""
            if let old = h.pins.removeValue(forKey: name) { h.map.removeAnnotation(old) }
            reply(.success(nil))
        case "dispose":
            maps[id] = nil
            reply(.success(nil))
        default:
            reply(.failure(PluginFailure("UnknownMethod", method)))
        }
    }
}
