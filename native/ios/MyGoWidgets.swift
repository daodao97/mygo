import WidgetKit

/// Requests a system-scheduled refresh; it does not promise an immediate update.
public enum MyGoWidgetRefresh {
    public static func request(kinds: [String], controls: Bool = false) {
        for kind in Set(kinds) { WidgetCenter.shared.reloadTimelines(ofKind: kind) }
        if #available(iOS 18.0, *), controls { ControlCenter.shared.reloadAllControls() }
    }
}
