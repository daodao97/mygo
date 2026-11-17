// Windows showing native UI. Go's Metal renderer draws into a CAMetalLayer
// it adds to the view's layer; the view gives Go the display's refreshes,
// the fingers, the keyboard (UITextInput, with the marked text of input
// methods) and the questions of VoiceOver.

import UIKit
import QuartzCore

final class SurfaceWindow: HostWindow, UITextInput, UIPointerInteractionDelegate {
    private var link: CADisplayLink?
    private var fingers: [ObjectIdentifier: Int] = [:]
    private var nextFinger = 1

    // The input being edited, as Go last told it: the text around the
    // selection, from base, and offsets in UTF-16 units of the document,
    // which is what UITextInput's NSRanges count in.
    private var text: [UInt16] = []
    private var base = 0, length = 0
    private var selStart = 0, selEnd = 0
    private var markStart = -1, markEnd = -1
    private var caret = CGRect.zero
    private var editing = false
    private var hasDocument = false
    private var shows = 0
    private var serial = 0

    // The input's traits, which the keyboard reads as it shows.
    var keyboardType: UIKeyboardType = .default
    var returnKeyType: UIReturnKeyType = .default
    var autocapitalizationType: UITextAutocapitalizationType = .sentences
    var autocorrectionType: UITextAutocorrectionType = .default
    var spellCheckingType: UITextSpellCheckingType = .default
    var isSecureTextEntry = false
    var textContentType: UITextContentType! = nil

    private var elements: [AccessElement]?

    /// What Go's renderer adds its layer to, from its thread. The view's own
    /// layer is UIKit's, which allows changes on the main thread alone.
    let drawHost = CALayer()

    override func layoutSubviews() {
        super.layoutSubviews()
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        drawHost.frame = bounds
        CATransaction.commit()
    }

    override init(id: Int) {
        super.init(id: id)
        layer.addSublayer(drawHost)
        drawHost.frame = bounds
        isMultipleTouchEnabled = true
        backgroundColor = .systemBackground
        let l = CADisplayLink(target: self, selector: #selector(tick))
        l.preferredFrameRateRange = CAFrameRateRange(minimum: 30, maximum: 120, preferred: 120)
        l.isPaused = true
        l.add(to: .main, forMode: .common)
        link = l
        // A mouse's or a trackpad's pointer, on an iPad: it hovers, it scrolls
        // with the wheel and two fingers, and it takes the content's shape.
        let hover = UIHoverGestureRecognizer(target: self, action: #selector(hovered(_:)))
        addGestureRecognizer(hover)
        let wheel = UIPanGestureRecognizer(target: self, action: #selector(wheeled(_:)))
        wheel.allowedScrollTypesMask = .all
        wheel.allowedTouchTypes = []
        wheel.maximumNumberOfTouches = 0
        addGestureRecognizer(wheel)
        addInteraction(UIPointerInteraction(delegate: self))
    }

    @objc private func hovered(_ g: UIHoverGestureRecognizer) {
        let p = g.location(in: self)
        let phase: String
        switch g.state {
        case .ended, .cancelled, .failed: phase = "cancel"
        default: phase = "move"
        }
        Go.send(Event.touch(id: id, pointer: 0, phase: phase, kind: "mouse", x: Double(p.x), y: Double(p.y), pressure: 0, buttons: 0, time: CACurrentMediaTime()))
    }

    private var wheelLast = CGPoint.zero

    @objc private func wheeled(_ g: UIPanGestureRecognizer) {
        if g.state == .began { wheelLast = .zero }
        let t = g.translation(in: self)
        let p = g.location(in: self)
        // Against the fingers, as what it moves is the view down the content.
        Go.send(Event.scroll(id: id, x: Double(p.x), y: Double(p.y), dx: Double(wheelLast.x - t.x), dy: Double(wheelLast.y - t.y), precise: true, phase: ""))
        wheelLast = t
    }

    private var pointerShape = "default"

    func setCursor(_ name: String) {
        pointerShape = name
        for i in interactions { (i as? UIPointerInteraction)?.invalidate() }
    }

    func pointerInteraction(_ i: UIPointerInteraction, styleFor region: UIPointerRegion) -> UIPointerStyle? {
        switch pointerShape {
        case "text": return UIPointerStyle(shape: .verticalBeam(length: 20))
        case "none": return .hidden()
        case "pointer": return UIPointerStyle(shape: .roundedRect(CGRect(x: -12, y: -12, width: 24, height: 24), radius: 12))
        default: return .system()
        }
    }

    func pointerInteraction(_ i: UIPointerInteraction, regionFor r: UIPointerRegionRequest, defaultRegion: UIPointerRegion) -> UIPointerRegion? {
        // Asked again as it moves: the content changes under it.
        UIPointerRegion(rect: CGRect(x: r.location.x - 1, y: r.location.y - 1, width: 2, height: 2))
    }

    required init?(coder: NSCoder) { fatalError() }

    override func didMoveToWindow() {
        super.didMoveToWindow()
        guard let w = window else { return }
        let hz = Double(w.screen.maximumFramesPerSecond)
        Go.send(Event.surfaceChanged(id: id, handle: Int(bitPattern: Unmanaged.passUnretained(drawHost).toOpaque()),
                                     width: Double(bounds.width), height: Double(bounds.height),
                                     scale: Double(w.screen.scale), refresh: hz))
    }

    override func close() {
        link?.invalidate()
        link = nil
        if isFirstResponder { resignFirstResponder() }
        super.close()
    }

    func requestFrame() { link?.isPaused = false }

    @objc private func tick() {
        // One frame per request: Go asks again while it animates.
        link?.isPaused = true
        Go.send(Event.frame(id: id))
    }

    // The views it holds, as web views: under its layer, whose frames are
    // transparent where the content shows them, each in a view that clips it.

    private final class Held {
        let window: HostWindow
        let clip = UIView()
        var box = CGRect.zero
        var covers: [CGRect] = []
        init(_ w: HostWindow) { window = w }

        func takes(_ p: CGPoint) -> Bool {
            !clip.isHidden && box.contains(p) && !covers.contains { $0.contains(p) }
        }
    }

    private var held: [Held] = []
    /// What holds them, below the layer Go's renderer adds to this view's.
    private lazy var underlay: UIView = {
        let v = UIView(frame: bounds)
        v.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        v.layer.zPosition = -1
        insertSubview(v, at: 0)
        return v
    }()

    func hold(_ w: HostWindow) {
        w.holder = self
        w.autoresizingMask = []
        let h = Held(w)
        h.clip.clipsToBounds = true
        h.clip.isHidden = true
        h.clip.addSubview(w)
        underlay.addSubview(h.clip)
        held.append(h)
    }

    func release(_ w: HostWindow) {
        guard let i = held.firstIndex(where: { $0.window === w }) else { return }
        held[i].clip.removeFromSuperview()
        held.remove(at: i)
    }

    func placeViews(_ places: [UInt8]) {
        var shown: [Held] = []
        places.withUnsafeBytes { buf in
            var r = WireReader(buf)
            while r.at < buf.count {
                let id = r.int()
                let frame = CGRect(x: r.float(), y: r.float(), width: r.float(), height: r.float())
                let clip = CGRect(x: r.float(), y: r.float(), width: r.float(), height: r.float())
                var covers: [CGRect] = []
                for _ in 0..<r.int() { covers.append(CGRect(x: r.float(), y: r.float(), width: r.float(), height: r.float())) }
                guard let h = held.first(where: { $0.window.id == id }) else { continue }
                h.clip.frame = clip
                h.window.frame = frame.offsetBy(dx: -clip.minX, dy: -clip.minY)
                h.window.reportSize()
                h.box = clip
                h.covers = covers
                h.clip.isHidden = false
                underlay.bringSubviewToFront(h.clip)
                shown.append(h)
            }
        }
        for h in held where !shown.contains(where: { $0 === h }) { h.clip.isHidden = true }
    }

    private var pressedAt: TimeInterval = -1

    // A finger on a view it holds goes to the view, which the content hears
    // of, as it closes what a press outside closes.
    override func hitTest(_ p: CGPoint, with e: UIEvent?) -> UIView? {
        guard self.point(inside: p, with: e) else { return nil }
        for h in held.reversed() where h.takes(p) {
            if let t = e?.timestamp, t != pressedAt {
                pressedAt = t
                Go.send(Event.viewPressed(id: id, x: Double(p.x), y: Double(p.y)))
            }
            return h.window.hitTest(convert(p, to: h.window), with: e) ?? h.window
        }
        return self
    }

    // Fingers.

    private func send(_ touches: Set<UITouch>, _ phase: String) {
        for t in touches {
            let key = ObjectIdentifier(t)
            var n = fingers[key]
            if n == nil {
                n = nextFinger
                nextFinger += 1
                fingers[key] = n
            }
            let p = t.location(in: self)
            let kind: String
            switch t.type {
            case .pencil: kind = "pen"
            case .indirectPointer: kind = "mouse"
            default: kind = "touch"
            }
            Go.send(Event.touch(id: id, pointer: n!, phase: phase, kind: kind, x: Double(p.x), y: Double(p.y),
                                pressure: Double(t.force), buttons: 1, time: t.timestamp))
            if phase == "up" || phase == "cancel" { fingers[key] = nil }
        }
    }

    override func touchesBegan(_ t: Set<UITouch>, with e: UIEvent?) {
        // A finger whose end UIKit never told of, as the view left the window
        // under it, would turn every later one into a pinch.
        if let all = e?.allTouches {
            let live = Set(all.map { ObjectIdentifier($0) })
            for (key, n) in fingers where !live.contains(key) {
                Go.send(Event.touch(id: id, pointer: n, phase: "cancel", kind: "touch", x: 0, y: 0, pressure: 0, buttons: 0, time: CACurrentMediaTime()))
                fingers[key] = nil
            }
        }
        send(t, "down")
    }
    override func touchesMoved(_ t: Set<UITouch>, with e: UIEvent?) { send(t, "move") }
    override func touchesEnded(_ t: Set<UITouch>, with e: UIEvent?) { send(t, "up") }
    override func touchesCancelled(_ t: Set<UITouch>, with e: UIEvent?) { send(t, "cancel") }

    // A hardware keyboard's keys.

    private static let keyNames: [UIKeyboardHIDUsage: String] = [
        .keyboardReturnOrEnter: "Enter", .keyboardEscape: "Escape", .keyboardDeleteOrBackspace: "Backspace",
        .keyboardTab: "Tab", .keyboardSpacebar: "Space", .keyboardDeleteForward: "Delete", .keyboardHome: "Home",
        .keyboardEnd: "End", .keyboardPageUp: "PageUp", .keyboardPageDown: "PageDown", .keyboardLeftArrow: "ArrowLeft",
        .keyboardRightArrow: "ArrowRight", .keyboardUpArrow: "ArrowUp", .keyboardDownArrow: "ArrowDown",
    ]

    private func keys(_ presses: Set<UIPress>, down: Bool) -> Set<UIPress> {
        var rest = Set<UIPress>()
        for p in presses {
            guard let k = p.key else { rest.insert(p); continue }
            let name = SurfaceWindow.keyNames[k.keyCode] ?? k.charactersIgnoringModifiers
            var mods = 0
            if k.modifierFlags.contains(.shift) { mods |= 1 }
            if k.modifierFlags.contains(.control) { mods |= 2 }
            if k.modifierFlags.contains(.alternate) { mods |= 4 }
            if k.modifierFlags.contains(.command) { mods |= 8 }
            let handled = Go.answer(Event.key(id: id, down: down, key: name, text: k.characters, mods: mods, repeated: false)) {
                KeyReply(&$0).handled
            } ?? false
            // Keys typing text while editing reach insertText too.
            if !handled && !(editing && SurfaceWindow.keyNames[k.keyCode] == nil) { rest.insert(p) }
        }
        return rest
    }

    override func pressesBegan(_ presses: Set<UIPress>, with e: UIPressesEvent?) {
        let rest = keys(presses, down: true)
        if !rest.isEmpty { super.pressesBegan(rest, with: e) }
    }

    override func pressesEnded(_ presses: Set<UIPress>, with e: UIPressesEvent?) {
        let rest = keys(presses, down: false)
        if !rest.isEmpty { super.pressesEnded(rest, with: e) }
    }

    // The keyboard.

    override var canBecomeFirstResponder: Bool { true }

    func setTextInput(_ c: SetTextInputCall) {
        let wasEditing = editing
        editing = c.active
        let another = c.serial != serial
        serial = c.serial
        hasDocument = c.document
        caret = CGRect(x: c.x, y: c.y, width: max(c.width, 2), height: max(c.height, 1))
        let traitsChanged = setTraits(c)
        let selChanged = c.selStart != selStart || c.selEnd != selEnd
        let textChanged = c.base != base || c.length != length || Array(c.text.utf16) != text || c.markStart != markStart
        if textChanged { inputDelegate?.textWillChange(self) }
        if selChanged { inputDelegate?.selectionWillChange(self) }
        apply(text: c.text, base: c.base, length: c.length, selStart: c.selStart, selEnd: c.selEnd, markStart: c.markStart, markEnd: c.markEnd)
        if selChanged { inputDelegate?.selectionDidChange(self) }
        if textChanged { inputDelegate?.textDidChange(self) }
        if editing && (!isFirstResponder || c.show != shows) {
            becomeFirstResponder()
        } else if !editing && wasEditing && isFirstResponder {
            resignFirstResponder()
        } else if editing && (traitsChanged || another) {
            reloadInputViews()
        }
        shows = c.show
    }

    private func setTraits(_ c: SetTextInputCall) -> Bool {
        let kb: UIKeyboardType
        switch c.keyboard {
        case "email": kb = .emailAddress
        case "number": kb = .numberPad
        case "decimal": kb = .decimalPad
        case "phone": kb = .phonePad
        case "url": kb = .URL
        case "search": kb = .webSearch
        default: kb = .default
        }
        let ret: UIReturnKeyType
        switch c.returnKey {
        case "done": ret = .done
        case "go": ret = .go
        case "next": ret = .next
        case "search": ret = .search
        case "send": ret = .send
        default: ret = .default
        }
        let caps: UITextAutocapitalizationType
        switch c.capitalize {
        case "none": caps = .none
        case "words": caps = .words
        case "characters": caps = .allCharacters
        case "sentences": caps = .sentences
        default: caps = (kb == .default || kb == .webSearch) && !c.secure ? .sentences : .none
        }
        let correct: UITextAutocorrectionType = c.autocorrect == "off" || c.secure ? .no : c.autocorrect == "on" ? .yes : .default
        let content: UITextContentType?
        switch c.autofill {
        case "username": content = .username
        case "password": content = .password
        case "newPassword": content = .newPassword
        case "oneTimeCode": content = .oneTimeCode
        case "email": content = .emailAddress
        case "name": content = .name
        case "phone": content = .telephoneNumber
        case "address": content = .fullStreetAddress
        case "postalCode": content = .postalCode
        case "creditCard": content = .creditCardNumber
        default: content = nil
        }
        let changed = kb != keyboardType || ret != returnKeyType || caps != autocapitalizationType || correct != autocorrectionType ||
            c.secure != isSecureTextEntry || content != textContentType
        keyboardType = kb
        returnKeyType = ret
        autocapitalizationType = caps
        autocorrectionType = correct
        spellCheckingType = correct == .no ? .no : .default
        isSecureTextEntry = c.secure
        textContentType = content
        return changed
    }

    private func apply(text t: String, base b: Int, length l: Int, selStart s: Int, selEnd e: Int, markStart ms: Int, markEnd me: Int) {
        text = Array(t.utf16)
        base = b
        length = l
        selStart = s
        selEnd = e
        markStart = ms
        markEnd = me
    }

    /// Sends an edit and takes the state after it, which the keyboard goes
    /// on to read at once.
    private func edit(_ msg: [UInt8]) {
        guard let st = Go.answer(msg, { TextStateReply(&$0) }) else { return }
        apply(text: st.text, base: st.base, length: st.length, selStart: st.selStart, selEnd: st.selEnd, markStart: st.markStart, markEnd: st.markEnd)
        caret = CGRect(x: st.x, y: st.y, width: max(st.width, 2), height: max(st.height, 1))
    }

    override func resignFirstResponder() -> Bool {
        let r = super.resignFirstResponder()
        if editing { Go.send(Event.textInputEnded(id: id)) }
        return r
    }

    var hasText: Bool { length > 0 }

    func insertText(_ s: String) { edit(Event.textReplace(id: id, hasRange: false, start: 0, end: 0, text: s)) }

    func deleteBackward() {
        if markStart >= 0 {
            edit(Event.textMark(id: id, hasRange: false, start: 0, end: 0, text: "", selStart: 0, selEnd: 0))
            return
        }
        edit(Event.textCommand(id: id, command: "deleteBackward"))
    }

    // UITextInput: the document, by UTF-16 offsets.

    private func pos(_ i: Int) -> Position { Position(max(0, min(i, length))) }

    var selectedTextRange: UITextRange? {
        get { Range(pos(selStart), pos(selEnd)) }
        set {
            guard let r = newValue as? Range, r.lo.i != selStart || r.hi.i != selEnd else { return }
            edit(Event.textSelect(id: id, start: r.lo.i, end: r.hi.i))
        }
    }

    var markedTextRange: UITextRange? { markStart < 0 ? nil : Range(pos(markStart), pos(markEnd)) }

    var markedTextStyle: [NSAttributedString.Key: Any]?

    func setMarkedText(_ s: String?, selectedRange: NSRange) {
        let m = s ?? ""
        edit(Event.textMark(id: id, hasRange: false, start: 0, end: 0, text: m, selStart: selectedRange.location,
                            selEnd: selectedRange.location + selectedRange.length))
    }

    func unmarkText() { edit(Event.textUnmark(id: id)) }

    var beginningOfDocument: UITextPosition { pos(0) }
    var endOfDocument: UITextPosition { pos(length) }

    func text(in range: UITextRange) -> String? {
        guard let r = range as? Range else { return nil }
        let lo = r.lo.i - base, hi = r.hi.i - base
        if lo >= 0 && hi <= text.count {
            return String(decoding: text[lo..<hi], as: UTF16.self)
        }
        return Go.answer(Event.textRange(id: id, start: r.lo.i, end: r.hi.i)) { TextRangeReply(&$0).text }
    }

    func replace(_ range: UITextRange, withText s: String) {
        guard let r = range as? Range else { return }
        edit(Event.textReplace(id: id, hasRange: true, start: r.lo.i, end: r.hi.i, text: s))
    }

    func textRange(from: UITextPosition, to: UITextPosition) -> UITextRange? {
        guard let a = from as? Position, let b = to as? Position else { return nil }
        return Range(a, b)
    }

    func position(from p: UITextPosition, offset: Int) -> UITextPosition? {
        guard let p = p as? Position else { return nil }
        let i = p.i + offset
        return i < 0 || i > length ? nil : pos(i)
    }

    func position(from p: UITextPosition, in direction: UITextLayoutDirection, offset: Int) -> UITextPosition? {
        guard let p = p as? Position else { return nil }
        switch direction {
        case .left: return position(from: p, offset: -offset)
        case .right: return position(from: p, offset: offset)
        default:
            // A line up or down from the caret's place.
            let r = rect(p.i, p.i)
            let step = max(r.height, 1) * CGFloat(offset) * (direction == .up ? -1 : 1)
            return closestPosition(to: CGPoint(x: r.midX, y: r.midY + step))
        }
    }

    func compare(_ a: UITextPosition, to b: UITextPosition) -> ComparisonResult {
        let x = (a as? Position)?.i ?? 0, y = (b as? Position)?.i ?? 0
        return x < y ? .orderedAscending : x > y ? .orderedDescending : .orderedSame
    }

    func offset(from: UITextPosition, to: UITextPosition) -> Int {
        ((to as? Position)?.i ?? 0) - ((from as? Position)?.i ?? 0)
    }

    weak var inputDelegate: UITextInputDelegate?
    lazy var tokenizer: UITextInputTokenizer = UITextInputStringTokenizer(textInput: self)

    func position(within range: UITextRange, farthestIn direction: UITextLayoutDirection) -> UITextPosition? {
        guard let r = range as? Range else { return nil }
        return direction == .left || direction == .up ? r.lo : r.hi
    }

    func characterRange(byExtending p: UITextPosition, in direction: UITextLayoutDirection) -> UITextRange? {
        guard let p = p as? Position else { return nil }
        return direction == .left || direction == .up ? Range(pos(0), p) : Range(p, pos(length))
    }

    func baseWritingDirection(for p: UITextPosition, in d: UITextStorageDirection) -> NSWritingDirection { .natural }
    func setBaseWritingDirection(_ d: NSWritingDirection, for r: UITextRange) {}

    /// Where the range's first line is, the caret if Go does not tell.
    private func rect(_ start: Int, _ end: Int) -> CGRect {
        guard hasDocument, let b = Go.answer(Event.textBounds(id: id, start: start, end: end), { TextBoundsReply(&$0) }), b.ok else { return caret }
        return CGRect(x: b.x, y: b.y, width: b.width, height: max(b.height, 1))
    }

    func firstRect(for range: UITextRange) -> CGRect {
        guard let r = range as? Range else { return caret }
        return rect(r.lo.i, r.hi.i)
    }

    func caretRect(for p: UITextPosition) -> CGRect {
        guard let p = p as? Position else { return caret }
        var r = rect(p.i, p.i)
        r.size.width = max(r.width, 2)
        return r
    }

    func selectionRects(for range: UITextRange) -> [UITextSelectionRect] {
        guard let r = range as? Range else { return [] }
        var out: [SelectionRect] = []
        var at = r.lo.i
        while at < r.hi.i || out.isEmpty {
            guard hasDocument, let b = Go.answer(Event.textBounds(id: id, start: at, end: r.hi.i), { TextBoundsReply(&$0) }), b.ok else { break }
            out.append(SelectionRect(CGRect(x: b.x, y: b.y, width: b.width, height: b.height)))
            if b.end <= at { break }
            at = b.end
        }
        out.first?.starts = true
        out.last?.ends = true
        return out
    }

    func closestPosition(to point: CGPoint) -> UITextPosition? {
        guard hasDocument, let r = Go.answer(Event.textIndexAt(id: id, x: Double(point.x), y: Double(point.y)), { TextIndexAtReply(&$0) }), r.ok else { return pos(selEnd) }
        return pos(r.index)
    }

    func closestPosition(to point: CGPoint, within range: UITextRange) -> UITextPosition? {
        guard let r = range as? Range, let p = closestPosition(to: point) as? Position else { return nil }
        return pos(max(r.lo.i, min(p.i, r.hi.i)))
    }

    func characterRange(at point: CGPoint) -> UITextRange? {
        guard let p = closestPosition(to: point) as? Position else { return nil }
        return Range(p, pos(p.i + 1))
    }

    // VoiceOver.

    func accessibilityChanged(focus: Int) {
        elements = nil
        let target = focus != 0 ? accessibilityElements?.first(where: { ($0 as? AccessElement)?.node == focus }) : nil
        UIAccessibility.post(notification: .layoutChanged, argument: target)
    }

    override var isAccessibilityElement: Bool {
        get { false }
        set {}
    }

    override var accessibilityElements: [Any]? {
        get {
            if let e = elements { return e }
            guard let nodes = Go.answer(Event.accessibilityTree(id: id), { AccessibilityTreeReply(&$0).nodes }) else { return [] }
            elements = AccessElement.decode(nodes, in: self)
            return elements
        }
        set {}
    }

    // VoiceOver's two-finger scrub goes back, as Android's back does.
    override func accessibilityPerformEscape() -> Bool {
        Go.answer(Event.backPressed(id: id)) { BackPressedReply(&$0).handled } ?? false
    }

    func accessAction(_ node: Int, _ action: String) -> Bool {
        Go.answer(Event.accessibilityAction(id: id, node: node, action: action)) { AccessibilityActionReply(&$0).handled } ?? false
    }

    // The system's edit menu: UIEditMenuInteraction asks this view, the first
    // responder, what it can do, and calls it.

    private var menuActions: [String] = []
    private lazy var editMenu: UIEditMenuInteraction = {
        let i = UIEditMenuInteraction(delegate: nil)
        addInteraction(i)
        return i
    }()

    func showMenu(_ c: ShowMenuCall) {
        menuActions = c.actions
        if !isFirstResponder { becomeFirstResponder() }
        let cfg = UIEditMenuConfiguration(identifier: nil, sourcePoint: CGPoint(x: c.x, y: c.y))
        cfg.preferredArrowDirection = .down
        editMenu.presentEditMenu(with: cfg)
    }

    override func canPerformAction(_ action: Selector, withSender sender: Any?) -> Bool {
        switch action {
        case #selector(UIResponderStandardEditActions.cut(_:)): return menuActions.contains("cut")
        case #selector(UIResponderStandardEditActions.copy(_:)): return menuActions.contains("copy")
        case #selector(UIResponderStandardEditActions.paste(_:)): return menuActions.contains("paste")
        case #selector(UIResponderStandardEditActions.selectAll(_:)): return menuActions.contains("selectAll")
        default: return false
        }
    }

    override func cut(_ sender: Any?) { edit(Event.textCommand(id: id, command: "cut")) }
    override func copy(_ sender: Any?) { edit(Event.textCommand(id: id, command: "copy")) }
    override func paste(_ sender: Any?) { edit(Event.textCommand(id: id, command: "paste")) }
    override func selectAll(_ sender: Any?) { edit(Event.textCommand(id: id, command: "selectAll")) }

    // The system's magnifier, while a finger drags a handle.

    private var loupe: UITextLoupeSession?

    func showMagnifier(_ c: ShowMagnifierCall) {
        let at = CGPoint(x: c.x, y: c.y)
        if let l = loupe {
            l.move(to: at, withCaretRect: caret, trackingCaret: true)
        } else {
            loupe = UITextLoupeSession.begin(at: at, fromSelectionWidgetView: nil, in: self)
        }
    }

    func hideMagnifier() {
        loupe?.invalidate()
        loupe = nil
    }
}

/// A position in the text being edited, in characters.
private final class Position: UITextPosition {
    let i: Int
    init(_ i: Int) { self.i = i }
}

private final class Range: UITextRange {
    let lo: Position, hi: Position
    init(_ a: Position, _ b: Position) {
        lo = a.i <= b.i ? a : b
        hi = a.i <= b.i ? b : a
    }
    override var start: UITextPosition { lo }
    override var end: UITextPosition { hi }
    override var isEmpty: Bool { lo.i == hi.i }
}

/// A line of a selection, for the system's text interactions.
private final class SelectionRect: UITextSelectionRect {
    let box: CGRect
    var starts = false, ends = false
    init(_ box: CGRect) { self.box = box }
    override var rect: CGRect { box }
    override var writingDirection: NSWritingDirection { .natural }
    override var containsStart: Bool { starts }
    override var containsEnd: Bool { ends }
    override var isVertical: Bool { false }
}

/// An element of native UI, as VoiceOver reads it.
final class AccessElement: UIAccessibilityElement {
    var node = 0
    var actions = 0
    var box = CGRect.zero
    weak var surface: SurfaceWindow?

    /// What makes VoiceOver say "text field", which UIKit's text fields have
    /// and names no constant for.
    private static let textEntry = UIAccessibilityTraits(rawValue: 1 << 18)

    // platform.AccessRole and its states, as Go numbers them.
    private enum Role: Int {
        case group = 0, text, button, link, checkBox, radio, toggle, slider, progress, textField, image
        case tab = 18, toggleButton = 31, menuItem = 40, heading = 43
    }

    static func decode(_ b: [UInt8], in surface: SurfaceWindow) -> [AccessElement] {
        var out: [AccessElement] = []
        b.withUnsafeBytes { buf in
            var r = WireReader(buf)
            while r.at < buf.count {
                let e = AccessElement(accessibilityContainer: surface)
                e.surface = surface
                e.node = r.int()
                _ = r.int() // the parent: VoiceOver reads the elements in order
                let role = r.int()
                let label = r.string(), value = r.string(), hint = r.string()
                e.box = CGRect(x: r.float(), y: r.float(), width: r.float(), height: r.float())
                let states = r.int()
                e.actions = r.int()
                let level = r.int()
                e.accessibilityLabel = label
                e.accessibilityValue = value.isEmpty ? nil : value
                e.accessibilityHint = hint.isEmpty ? nil : hint
                e.accessibilityFrameInContainerSpace = e.box
                var traits: UIAccessibilityTraits = []
                switch Role(rawValue: role) {
                case .button, .toggleButton, .menuItem, .tab, .checkBox, .radio, .toggle: traits.insert(.button)
                case .link: traits.insert(.link)
                case .slider: traits.insert(.adjustable)
                case .image: traits.insert(.image)
                case .heading: traits.insert(.header)
                case .textField: traits.insert(AccessElement.textEntry)
                case .text: traits.insert(.staticText)
                default: break
                }
                if role == Role.text.rawValue || role == Role.heading.rawValue { _ = level }
                if states & 2 != 0 { traits.insert(.notEnabled) }
                if states & (4 | 16) != 0 { traits.insert(.selected) }
                e.accessibilityTraits = traits
                // Groups without a name of their own hold others, which
                // VoiceOver reads instead.
                if role == Role.group.rawValue && label.isEmpty && value.isEmpty { continue }
                out.append(e)
            }
        }
        return out
    }

    override func accessibilityActivate() -> Bool { surface?.accessAction(node, "press") ?? false }
    override func accessibilityIncrement() { _ = surface?.accessAction(node, "increment") }
    override func accessibilityDecrement() { _ = surface?.accessAction(node, "decrement") }
}
