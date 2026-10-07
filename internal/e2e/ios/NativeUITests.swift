import XCTest
import UIKit

final class NativeUITests: XCTestCase {
    func testHardwareKeyboardEditingAndFocus() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openService("editing", in: app)
        app.buttons["Reset sample"].tap()
        let field = app.descendants(matching: .any)["Editing sample"].firstMatch
        let sample = "Hello MyGo 👋\nSelect text, then cut, copy or paste.\n中文与 emoji stay intact."
        field.tap()
        XCTAssertTrue(app.keyboards.firstMatch.waitForExistence(timeout: 5))
        app.typeKey("a", modifierFlags: .command)
        field.typeText("replacement")
        XCTAssertEqual(field.value as? String, "replacement")
        var undoSteps = 0
        while field.value as? String != sample && undoSteps < 15 {
            app.typeKey("z", modifierFlags: .command)
            undoSteps += 1
        }
        XCTAssertEqual(field.value as? String, sample, "Command-Z bypassed the shared editor history")
        for _ in 0..<undoSteps { app.typeKey("z", modifierFlags: [.command, .shift]) }
        XCTAssertEqual(field.value as? String, "replacement")
        app.typeKey("\t", modifierFlags: [])
        app.typeText("abc")
        scrollTo(app.staticTexts["Password length: 3"], in: app)
        XCTAssertTrue(app.staticTexts["Password length: 3"].waitForExistence(timeout: 5), "Tab did not focus the next Go editor: \(app.debugDescription)")
        app.typeKey("\t", modifierFlags: .shift)
        app.typeKey("c", modifierFlags: .command)
        scrollTo(app.buttons["Read editing clipboard"], in: app)
        app.buttons["Read editing clipboard"].tap()
        XCTAssertTrue(app.staticTexts["Copied: replacement"].waitForExistence(timeout: 5), "Shift-Tab or Command-C failed")
        screenshot("hardware-keyboard-go-editing", app)
    }
    // XCTest on a physical phone does not deliver Escape to UIPress/keyCommands.
    // This simulator test accepts an actual host keyboard Escape (CUA/interactive).
    func testSimulatorHardwareEscape() throws {
        #if targetEnvironment(simulator)
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openService("editing", in: app)
        let field = app.descendants(matching: .any)["Editing sample"].firstMatch
        field.tap()
        XCTAssertEqual(field.elementType, .textView)
        print("MYGO_SIMULATOR_AWAITING_ESCAPE")
        let unfocused = XCTNSPredicateExpectation(predicate: NSPredicate { _, _ in
            field.exists && field.elementType != .textView && !app.keyboards.firstMatch.exists
        }, object: nil)
        XCTAssertEqual(XCTWaiter.wait(for: [unfocused], timeout: 45), .completed, "Send a host keyboard Escape to Simulator; Go focus and the keyboard must both clear")
        screenshot("hardware-escape-cleared-go-focus", app)
        #else
        throw XCTSkip("Requires host hardware keyboard input in Simulator")
        #endif
    }
    func testBiometricAvailabilityAndCancellation() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openService("biometrics", in: app)
        app.buttons["Check biometrics"].tap()
        let status = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'Biometrics:'")).firstMatch
        XCTAssertTrue(status.waitForExistence(timeout: 10), app.debugDescription)
        XCTAssertTrue(["face-id", "touch-id", "none"].contains { status.label.contains($0) })
        let wasAvailable = status.label.contains("available: true")
        screenshot("biometric-capability", app)
        app.buttons["Cancel authentication after 2 seconds"].tap()
        let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
        let allow = springboard.buttons.matching(NSPredicate(format: "label IN %@", ["OK", "好", "Allow", "允许"])).firstMatch
        if allow.waitForExistence(timeout: 1) { allow.tap() }
        let result = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'Authentication'")).firstMatch
        let completed = XCTNSPredicateExpectation(predicate: NSPredicate { _, _ in
            result.exists && result.label != "Authentication pending"
        }, object: nil)
        XCTAssertEqual(XCTWaiter.wait(for: [completed], timeout: 10), .completed, app.debugDescription)
        // A real device may authenticate before the timeout, or be unenrolled.
        // Record the actual branch; simulator enrolled tests assert timeout.
        XCTAssertTrue(["Authentication timed out", "Authentication succeeded", "Authentication: canceled", "Authentication: unavailable", "Authentication: not-enrolled", "Authentication: passcode-not-set", "Authentication: locked-out"].contains(result.label), result.label)
        print("MYGO_BIOMETRIC_RESULT: \(result.label)")
        #if targetEnvironment(simulator)
        if wasAvailable { XCTAssertEqual(result.label, "Authentication timed out") }
        #endif
        screenshot("biometric-authentication-result", app)
        app.buttons["Check biometrics"].tap()
        XCTAssertTrue(status.waitForExistence(timeout: 10), "Authentication kept a stale request or blocked Go dispatch")
    }
    func testBiometricSimulatorMatchAndBackgroundCancellation() throws {
        #if targetEnvironment(simulator)
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openService("biometrics", in: app)
        app.buttons["Check biometrics"].tap()
        let status = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'Biometrics:'")).firstMatch
        XCTAssertTrue(status.waitForExistence(timeout: 10))
        XCTAssertTrue(status.label.contains("available: true"), "Enable Face ID Enrolled on the owned test simulator")
        app.buttons["Authenticate with biometrics"].tap()
        let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
        let allow = springboard.buttons.matching(NSPredicate(format: "label IN %@", ["OK", "好", "Allow", "允许"])).firstMatch
        if allow.waitForExistence(timeout: 2) { allow.tap() }
        print("MYGO_SIMULATOR_AWAITING_MATCH")
        XCTAssertTrue(app.staticTexts["Authentication succeeded"].waitForExistence(timeout: 45), "Send Features > Face ID > Matching Face on the owned simulator")
        screenshot("biometric-success", app)
        app.buttons["Authenticate with biometrics"].tap()
        XCUIDevice.shared.press(.home)
        app.activate()
        XCTAssertTrue(app.staticTexts["Authentication: canceled"].waitForExistence(timeout: 10), app.debugDescription)
        app.buttons["Check biometrics"].tap()
        XCTAssertTrue(status.waitForExistence(timeout: 10))
        screenshot("biometric-background-canceled", app)
        #else
        throw XCTSkip("Matching Face is a Simulator control")
        #endif
    }
    func testBiometricUnenrolledSimulator() throws {
        #if targetEnvironment(simulator)
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openService("biometrics", in: app)
        app.buttons["Check biometrics"].tap()
        let status = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'Biometrics:'")).firstMatch
        XCTAssertTrue(status.waitForExistence(timeout: 10))
        XCTAssertTrue(status.label.contains("available: false") && status.label.contains("not-enrolled"), status.label)
        app.buttons["Authenticate with biometrics"].tap()
        XCTAssertTrue(app.staticTexts["Authentication: not-enrolled"].waitForExistence(timeout: 10), app.debugDescription)
        screenshot("biometric-not-enrolled", app)
        #else
        throw XCTSkip("Unenrollment is a Simulator fixture")
        #endif
    }
    func testChineseComposition() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openService("editing", in: app)
        scrollTo(app.buttons["Reset composition"], in: app)
        app.buttons["Reset composition"].tap()
        let field = app.descendants(matching: .any)["Composition input"].firstMatch
        scrollTo(field, in: app)
        field.tap()
        XCTAssertTrue(app.keyboards.firstMatch.waitForExistence(timeout: 5))
        let globe = app.buttons.matching(NSPredicate(format: "label IN %@", ["Next keyboard", "下一键盘", "下一个键盘"])).firstMatch
        XCTAssertTrue(globe.waitForExistence(timeout: 5), app.debugDescription)
        globe.press(forDuration: 1)
        let original = app.cells.allElementsBoundByIndex.first(where: { $0.isSelected })?.label
        let pinyin = app.cells.matching(NSPredicate(format: "label CONTAINS '拼音' OR label CONTAINS[c] 'Pinyin'")).firstMatch
        guard pinyin.exists else {
            app.coordinate(withNormalizedOffset: CGVector(dx: 0.95, dy: 0.3)).tap()
            throw XCTSkip("This device has no configured Pinyin keyboard")
        }
        pinyin.tap()
        defer {
            if let original = original {
                if !app.keyboards.firstMatch.exists { field.tap() }
                globe.press(forDuration: 1)
                if app.cells[original].exists { app.cells[original].tap() }
            }
        }
        let committed = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'Committed input:'")).firstMatch
        func typePinyin(_ text: String) {
            // Tap real software keys; typeText("中文") bypasses composition.
            for letter in text {
                var key = app.keyboards.keys[String(letter)].firstMatch
                if !key.exists {
                    // The user's configured Pinyin keyboard may use ten keys.
                    let groups = ["A B C", "D E F", "G H I", "J K L", "M N O", "P Q R S", "T U V", "W X Y Z"]
                    if let group = groups.first(where: { $0.contains(String(letter).uppercased()) }) {
                        key = app.keyboards.keys.matching(NSPredicate(format: "label CONTAINS %@", group)).firstMatch
                    }
                }
                XCTAssertTrue(key.exists, app.debugDescription)
                key.tap()
            }
        }
        typePinyin("zhongwen")
        XCTAssertEqual(committed.label.trimmingCharacters(in: .whitespaces), "Committed input:", app.debugDescription)
        XCTAssertFalse((field.value as? String ?? "").isEmpty, app.debugDescription)
        screenshot("pinyin-marked-before-commit", app)
        let chinese = app.descendants(matching: .any).matching(NSPredicate(format: "label == %@", "中文")).firstMatch
        XCTAssertTrue(chinese.waitForExistence(timeout: 5), app.debugDescription)
        chinese.tap()
        XCTAssertTrue(app.staticTexts["Committed input: 中文"].waitForExistence(timeout: 5), app.debugDescription)
        typePinyin("shu")
        XCTAssertEqual(committed.label, "Committed input: 中文", app.debugDescription)
        let book = app.descendants(matching: .any).matching(NSPredicate(format: "label == %@", "书")).firstMatch
        XCTAssertTrue(book.waitForExistence(timeout: 5), app.debugDescription)
        book.tap()
        XCTAssertTrue(app.staticTexts["Committed input: 中文书"].waitForExistence(timeout: 5))
        typePinyin("x")
        app.keyboards.keys["delete"].tap()
        XCTAssertEqual(committed.label, "Committed input: 中文书", app.debugDescription)
        screenshot("pinyin-commit-and-cancel", app)
    }
    override func tearDown() {
        XCUIDevice.shared.orientation = .portrait
        // Leave the user's device on the feature demo after every test.
        // open(URL:) also delivers to a running process, unlike a launch payload.
        if #available(iOS 16.4, *) {
            let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
            app.open(URL(string: "mygo-native://demo/app/home")!)
            XCTAssertTrue(app.staticTexts["MyGo Mobile"].waitForExistence(timeout: 10),
                          "Device was left on a full-screen diagnostic page")
            for tab in ["Overview", "Features", "Status"] { XCTAssertTrue(app.buttons[tab].exists) }
        }
        super.tearDown()
    }

    private func reset(_ app: XCUIApplication) throws {
        app.terminate()
        if #available(iOS 16.4, *) {
            app.open(URL(string: "mygo-native://demo/reset")!)
        } else {
            throw XCTSkip("Cold URL launch needs XCTest's iOS 16.4 open(URL:) API")
        }
        XCTAssertTrue(app.staticTexts["MyGo on iOS"].waitForExistence(timeout: 15), app.debugDescription)
        XCTAssertTrue(app.staticTexts["Count: 0"].exists, app.debugDescription)
        XCTAssertTrue(app.staticTexts["First frame presented"].waitForExistence(timeout: 5), app.debugDescription)
        XCTAssertTrue(app.staticTexts["Opened: mygo-native://demo/reset"].exists, app.debugDescription)
        XCTAssertFalse(app.staticTexts["MyGo"].exists, "Startup overlay remained after the first presented frame")
    }

    private func launchDiagnostics(_ app: XCUIApplication) throws {
        if #available(iOS 16.4, *) {
            app.open(URL(string: "mygo-native://demo/diagnostics")!)
        } else {
            throw XCTSkip("Cold diagnostic entry requires iOS 16.4")
        }
    }

    func testNormalLaunchAfterDiagnosticsKeepsBottomTabs() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openShowcaseFeature("Images", in: app)
        XCTAssertTrue(app.staticTexts["Raster Images"].waitForExistence(timeout: 5))
        try launchDiagnostics(app)
        XCTAssertTrue(app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'Row '")).firstMatch.waitForExistence(timeout: 10))
        XCTAssertFalse(app.buttons["Features"].exists)
        backgroundForCheckpoint(app)
        app.terminate()
        app.launch()
        XCTAssertTrue(app.staticTexts["Raster Images"].waitForExistence(timeout: 15), app.debugDescription)
        for tab in ["Overview", "Features", "Status"] {
            XCTAssertTrue(app.buttons[tab].exists, "Ordinary launch lost \(tab)")
        }
        screenshot("ordinary-launch-restored-feature-tabs", app)
        if #available(iOS 16.4, *) {
            app.open(URL(string: "mygo-native://demo/app/home")!)
        }
        XCTAssertTrue(app.staticTexts["MyGo Mobile"].waitForExistence(timeout: 5))
        backgroundForCheckpoint(app)
        app.terminate()
        app.launch()
        XCTAssertTrue(app.staticTexts["MyGo Mobile"].waitForExistence(timeout: 15))
        for tab in ["Overview", "Features", "Status"] { XCTAssertTrue(app.buttons[tab].exists) }
        screenshot("ordinary-launch-overview-bottom-tabs", app)
    }

    private func swipe(_ app: XCUIApplication, up: Bool) {
        // Use the scroll padding, so the gesture cannot start editing text
        // or selecting a row while looking for a control.
        let top = app.coordinate(withNormalizedOffset: CGVector(dx: 0.025, dy: 0.20))
        let bottom = app.coordinate(withNormalizedOffset: CGVector(dx: 0.025, dy: 0.70))
        if up { bottom.press(forDuration: 0.05, thenDragTo: top) }
        else { top.press(forDuration: 0.05, thenDragTo: bottom) }
    }

    private func scrollTo(_ element: XCUIElement, in app: XCUIApplication) {
        var up = true
        for _ in 0..<12 {
            let bottom = app.buttons["Features"].exists ? app.buttons["Features"].frame.minY : app.frame.maxY
            let top = app.buttons["Back"].exists ? app.buttons["Back"].frame.maxY + 5 : app.frame.height * 0.13
            if element.exists {
                let frame = element.frame
                if element.isHittable && frame.minY >= top && frame.maxY < bottom - 4 { return }
                up = frame.minY >= top
            }
            let a = app.coordinate(withNormalizedOffset: CGVector(dx: 0.025, dy: up ? 0.55 : 0.40))
            let b = app.coordinate(withNormalizedOffset: CGVector(dx: 0.025, dy: up ? 0.40 : 0.55))
            a.press(forDuration: 0.05, thenDragTo: b, withVelocity: .slow, thenHoldForDuration: 0.2)
        }
        XCTFail("Could not reach \(element): \(app.debugDescription)")
    }

    private func waitForHittable(_ element: XCUIElement) -> Bool {
        let ready = XCTNSPredicateExpectation(predicate: NSPredicate { _, _ in
            element.exists && element.isHittable
        }, object: nil)
        return XCTWaiter.wait(for: [ready], timeout: 5) == .completed
    }

    private func backgroundForCheckpoint(_ app: XCUIApplication) {
        XCUIDevice.shared.press(.home)
        let background = XCTNSPredicateExpectation(predicate: NSPredicate { _, _ in
            app.state == .runningBackground || app.state == .runningBackgroundSuspended
        }, object: nil)
        XCTAssertEqual(XCTWaiter.wait(for: [background], timeout: 5), .completed)
        // SpringBoard becomes foreground before the Scene finishes its
        // background animation. Let sceneDidEnterBackground checkpoint before
        // terminate() force-kills the process without a termination callback.
        let settle = XCTestExpectation(description: "Scene background animation")
        XCTAssertEqual(XCTWaiter.wait(for: [settle], timeout: 1), .timedOut)
    }

    private func scrollToTop(_ app: XCUIApplication) {
        for _ in 0..<10 {
            if app.staticTexts["MyGo on iOS"].isHittable { break }
            swipe(app, up: false)
        }
        swipe(app, up: false) // ensure the top is fully clamped, not partially visible
    }

    private func editMessage(_ app: XCUIApplication) -> String {
        app.descendants(matching: .any)["Message"].firstMatch.tap()
        XCTAssertTrue(app.keyboards.firstMatch.waitForExistence(timeout: 5))
        let keyboard = app.keyboards.firstMatch
        for _ in 0..<3 {
            if keyboard.keys["a"].exists || keyboard.keys["A"].exists { break }
            app.buttons["Next keyboard"].tap()
        }
        let marker = "abc" + String(UUID().uuidString.prefix(6)).lowercased()
        app.typeText(" " + marker)
        XCTAssertTrue(messageLabel(marker, in: app).waitForExistence(timeout: 5), app.debugDescription)
        return marker
    }

    private func messageLabel(_ marker: String, in app: XCUIApplication) -> XCUIElement {
        app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'Message:' AND label CONTAINS %@", marker)).firstMatch
    }

    private func screenshot(_ name: String, _ app: XCUIApplication) {
        let shot = XCTAttachment(screenshot: app.screenshot())
        shot.name = name
        shot.lifetime = .keepAlways
        add(shot)
    }

    private func assertBackground(_ app: XCUIApplication, rgb: [Int]) {
        let image = app.screenshot().image.cgImage!
        let width = image.width, height = image.height
        var pixels = [UInt8](repeating: 0, count: width * height * 4)
        let space = CGColorSpace(name: CGColorSpace.sRGB)!
        pixels.withUnsafeMutableBytes { bytes in
            let context = CGContext(data: bytes.baseAddress, width: width, height: height,
                                    bitsPerComponent: 8, bytesPerRow: width * 4,
                                    space: space, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
            context.draw(image, in: CGRect(x: 0, y: 0, width: width, height: height))
        }
        // Left padding samples avoid text and controls, including the clock
        // and home indicator. Compare the status area, page and bottom inset.
        for y in [0.03, 0.18, 0.985] {
            let offset = (Int(Double(height) * y) * width + Int(Double(width) * 0.025)) * 4
            for channel in 0..<3 {
                XCTAssertEqual(Double(pixels[offset + channel]), Double(rgb[channel]), accuracy: 4,
                               "Background differs at y=\(y), channel=\(channel)")
            }
        }
    }

    func testImmersiveBackground() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try reset(app)
        if #available(iOS 16.4, *) {
            app.open(URL(string: "mygo-native://demo/appearance/dark")!)
            XCTAssertTrue(app.staticTexts["Opened: mygo-native://demo/appearance/dark"].waitForExistence(timeout: 5))
            assertBackground(app, rgb: [20, 33, 61])
            screenshot("native-ui-immersive-dark", app)
            app.open(URL(string: "mygo-native://demo/appearance/light")!)
            XCTAssertTrue(app.staticTexts["Opened: mygo-native://demo/appearance/light"].waitForExistence(timeout: 5))
            assertBackground(app, rgb: [238, 244, 248])
            screenshot("native-ui-immersive-light", app)
        }
    }

    private func openMobile(_ app: XCUIApplication, keyboard: String = "email") throws {
        if #available(iOS 16.4, *) {
            app.open(URL(string: "mygo-native://demo/mobile/input/\(keyboard)")!)
            let heading = keyboard == "secrets" ? "Secure storage" : "Mobile services"
            XCTAssertTrue(app.staticTexts[heading].waitForExistence(timeout: 10), app.debugDescription)
            if keyboard != "secrets" { XCTAssertTrue(app.staticTexts["Keyboard: \(keyboard)"].exists, app.debugDescription) }
        } else { throw XCTSkip("URL opening requires iOS 16.4") }
    }

    func testMobileKeyboardTypesAndSubmit() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try reset(app)
        for kind in ["number", "decimal", "phone", "email", "url"] {
            try openMobile(app, keyboard: kind)
            app.descendants(matching: .any)["Mobile field"].firstMatch.tap()
            let keyboard = app.keyboards.firstMatch
            XCTAssertTrue(keyboard.waitForExistence(timeout: 5), app.debugDescription)
            if ["number", "decimal", "phone"].contains(kind) {
                XCTAssertTrue(keyboard.keys["1"].exists, app.debugDescription)
                XCTAssertFalse(keyboard.keys["a"].exists)
                if kind == "decimal" { XCTAssertTrue(keyboard.keys["."].exists, app.debugDescription) }
                app.typeText("123")
                XCTAssertTrue(app.staticTexts["Input value: 123"].waitForExistence(timeout: 5), app.debugDescription)
            } else {
                for _ in 0..<3 {
                    if keyboard.keys["a"].exists || keyboard.keys["A"].exists { break }
                    app.buttons["Next keyboard"].tap()
                }
                app.typeText(kind == "email" ? "abc@example.com" : "https://example.com")
                if kind == "email" {
                    XCTAssertTrue(app.staticTexts["Input value: abc@example.com"].waitForExistence(timeout: 5), app.debugDescription)
                    let send = keyboard.buttons.matching(NSPredicate(format: "label ==[c] 'send'")).firstMatch
                    XCTAssertTrue(send.exists, app.debugDescription)
                    send.tap()
                    XCTAssertTrue(app.staticTexts["Submissions: 1"].waitForExistence(timeout: 5), app.debugDescription)
                } else {
                    XCTAssertTrue(app.staticTexts["Input value: https://example.com"].waitForExistence(timeout: 5), app.debugDescription)
                }
            }
            screenshot("mobile-keyboard-\(kind)", app)
            app.staticTexts["Mobile services"].tap()
            XCTAssertFalse(app.keyboards.firstMatch.exists, "Keyboard did not dismiss after moving focus")
        }
    }

    func testIPadLayoutAndPresentation() throws {
        guard UIDevice.current.userInterfaceIdiom == .pad else { throw XCTSkip("iPad coverage") }
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openService("gestures", in: app)
        let field = app.descendants(matching: .any)["Gesture input"].firstMatch
        field.tap()
        XCTAssertTrue(app.keyboards.firstMatch.waitForExistence(timeout: 5))
        field.typeText("ipad draft")
        XCUIDevice.shared.orientation = .landscapeLeft
        XCTAssertEqual(field.value as? String, "ipad draft")
        XCTAssertTrue(app.buttons["Features"].exists)
        screenshot("ipad-landscape-keyboard-safe-area", app)
        app.buttons["Back"].tap()
        let hidden = XCTNSPredicateExpectation(predicate: NSPredicate { _, _ in !app.keyboards.firstMatch.exists }, object: nil)
        XCTAssertEqual(XCTWaiter.wait(for: [hidden], timeout: 5), .completed, app.debugDescription)
        try openService("dialogs", in: app)
        app.buttons["Native actions"].tap()
        XCTAssertTrue(app.buttons["Confirm"].waitForExistence(timeout: 5), app.debugDescription)
        screenshot("ipad-action-sheet-anchor", app)
        app.buttons["Confirm"].tap()
        XCTAssertTrue(app.staticTexts["Dialog result: 0"].waitForExistence(timeout: 5))
        app.buttons["Share text"].tap()
        XCTAssertTrue(app.otherElements["PopoverDismissRegion"].waitForExistence(timeout: 5), app.debugDescription)
        screenshot("ipad-share-anchor", app)
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.05, dy: 0.10)).tap()
        XCTAssertTrue(app.staticTexts["Share completed: false"].waitForExistence(timeout: 5))
    }

    private func openService(_ name: String, in app: XCUIApplication) throws {
        guard #available(iOS 16.4, *) else { throw XCTSkip("Service URL requires iOS 16.4") }
        app.terminate()
        app.open(URL(string: "mygo-native://demo/app/feature?name=\(name)")!)
        XCTAssertTrue(app.buttons["Features"].waitForExistence(timeout: 15), app.debugDescription)
    }

    private func authorizeNotifications(_ app: XCUIApplication) {
        app.buttons["Request notification permission"].tap()
        let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
        for source in [springboard, app] {
            let allow = source.alerts.buttons.matching(NSPredicate(format: "label IN %@", ["Allow", "允许"])).firstMatch
            if allow.waitForExistence(timeout: 2) { allow.tap() }
        }
        XCTAssertTrue(app.staticTexts["Permission notifications: granted"].waitForExistence(timeout: 10), app.debugDescription)
    }

    func testLocalNotificationsAndBadge() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openService("notifications", in: app)
        authorizeNotifications(app)
        app.buttons["Cancel demo notification"].tap()
        app.buttons["Show notification now"].tap()
        XCTAssertTrue(app.staticTexts["Delivered: mygo-demo-local"].waitForExistence(timeout: 8), app.debugDescription)
        screenshot("notification-immediate-delivery", app)
        sleep(4)
        app.buttons["Cancel demo notification"].tap()
        app.buttons["Schedule in 5 seconds"].tap()
        XCTAssertTrue(app.staticTexts["Notification submitted: mygo-demo-local"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts["Delivered: mygo-demo-local"].waitForExistence(timeout: 12), app.debugDescription)
        screenshot("notification-foreground-delivery", app)
        // Native banners temporarily cover the upper controls.
        sleep(4)
        app.buttons["Set badge to 1"].tap()
        XCTAssertTrue(app.staticTexts["Badge count: 1"].waitForExistence(timeout: 5))
        app.buttons["Clear badge"].tap()
        XCTAssertTrue(app.staticTexts["Badge count: 0"].waitForExistence(timeout: 5))
        app.buttons["Schedule in 5 seconds"].tap()
        XCTAssertTrue(app.staticTexts["Notification submitted: mygo-demo-local"].waitForExistence(timeout: 5))
        app.buttons["Cancel demo notification"].tap()
        XCTAssertTrue(app.staticTexts["Notification cancelled"].waitForExistence(timeout: 5))
        sleep(7)
        XCTAssertTrue(app.staticTexts["Notification cancelled"].exists, "Cancelled request was delivered")
        screenshot("notification-cancel-and-clear-badge", app)
    }

    func testNotificationColdLaunchEntry() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openService("notifications", in: app)
        authorizeNotifications(app)
        app.buttons["Cancel demo notification"].tap()
        app.buttons["Schedule in 15 seconds"].tap()
        XCTAssertTrue(app.staticTexts["Notification submitted: mygo-demo-local"].waitForExistence(timeout: 5))
        XCUIDevice.shared.press(.home)
        app.terminate()
        let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
        let banner = springboard.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS 'MyGo notification'")).firstMatch
        if !banner.waitForExistence(timeout: 20) {
            // Notification Center also covers devices with banners suppressed.
            let top = springboard.coordinate(withNormalizedOffset: CGVector(dx: 0.40, dy: 0.01))
            let bottom = springboard.coordinate(withNormalizedOffset: CGVector(dx: 0.40, dy: 0.75))
            top.press(forDuration: 0.05, thenDragTo: bottom)
        }
        XCTAssertTrue(banner.waitForExistence(timeout: 8), "MyGo notification was absent from Notification Center")
        let attachment=XCTAttachment(screenshot: banner.screenshot()); attachment.name="notification-cold-launch-banner"; attachment.lifetime = .keepAlways; add(attachment)
        banner.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
        XCTAssertTrue(app.staticTexts["Opened notification: mygo-demo-local"].waitForExistence(timeout: 15), app.debugDescription)
        XCTAssertTrue(app.buttons["Features"].exists)
        XCTAssertTrue(app.staticTexts["Notification route: /features/notifications"].exists)
        app.buttons["Cancel demo notification"].tap()
        screenshot("notification-cold-launch-route", app)
    }

    func testHapticsAndSystemIntegration() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openService("haptics", in: app)
        for kind in ["selection", "light", "medium", "heavy", "soft", "rigid", "success", "warning", "error"] {
            let button = app.buttons["Haptic " + kind]
            scrollTo(button, in: app)
            button.tap()
            let result = app.staticTexts["Haptic requested: " + kind]
            scrollTo(result, in: app)
            XCTAssertTrue(result.exists, app.debugDescription)
        }
        screenshot("haptic-api-patterns", app)
        try openService("integration", in: app)
        for label in ["Status bar light", "Status bar dark", "Status bar auto"] {
            app.buttons[label].tap()
            XCTAssertTrue(app.staticTexts[label].exists)
        }
        app.buttons["Toggle status bar"].tap()
        XCTAssertTrue(app.staticTexts["Status bar hidden: true"].waitForExistence(timeout: 5))
        screenshot("status-bar-hidden", app)
        app.buttons["Toggle status bar"].tap()
        XCTAssertTrue(app.staticTexts["Status bar hidden: false"].waitForExistence(timeout: 5))
        screenshot("status-bar-restored", app)
        app.buttons["Open demo deep link"].tap()
        XCTAssertTrue(app.staticTexts["Deep link opened"].waitForExistence(timeout: 5), app.debugDescription)
        XCTAssertTrue(app.staticTexts["Opened URL: mygo-native://demo/app/feature?name=integration"].exists)
        screenshot("status-bar-and-deep-link", app)
    }

    func testInteractiveKeyboardAndReadOnlySelection() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openService("gestures", in: app)
        let field = app.descendants(matching: .any)["Gesture input"].firstMatch
        field.tap()
        XCTAssertTrue(app.keyboards.firstMatch.waitForExistence(timeout: 5))
        field.typeText("gesture draft")
        let top = app.keyboards.firstMatch.frame.minY
        let dragX = app.frame.maxX - 10 // page padding, outside selectable/editable text
        let start = app.coordinate(withNormalizedOffset: .zero).withOffset(CGVector(dx: dragX, dy: top - 130))
        let shortEnd = app.coordinate(withNormalizedOffset: .zero).withOffset(CGVector(dx: dragX, dy: top - 100))
        start.press(forDuration: 0.1, thenDragTo: shortEnd, withVelocity: 40, thenHoldForDuration: 0.5)
        XCTAssertTrue(app.keyboards.firstMatch.exists, "A drag above the keyboard lost input focus")
        // UIKit completes dismissal when a downward drag crosses its edge
        // and is released. Actual cancellation requires reversing that drag.
        let partialEnd = app.coordinate(withNormalizedOffset: .zero).withOffset(CGVector(dx: dragX, dy: top + 35))
        start.press(forDuration: 0.1, thenDragTo: partialEnd, withVelocity: 40, thenHoldForDuration: 0.5)
        let partialGone = XCTNSPredicateExpectation(predicate: NSPredicate { _, _ in !app.keyboards.firstMatch.exists }, object: nil)
        XCTAssertEqual(XCTWaiter.wait(for: [partialGone], timeout: 5), .completed, app.debugDescription)
        XCTAssertEqual(field.value as? String, "gesture draft")
        scrollTo(field, in: app)
        field.tap()
        XCTAssertTrue(app.keyboards.firstMatch.waitForExistence(timeout: 5))
        let end = app.coordinate(withNormalizedOffset: .zero).withOffset(CGVector(dx: dragX, dy: app.frame.maxY - 20))
        start.press(forDuration: 0.1, thenDragTo: end, withVelocity: .slow, thenHoldForDuration: 0.2)
        let gone = XCTNSPredicateExpectation(predicate: NSPredicate { _, _ in !app.keyboards.firstMatch.exists }, object: nil)
        XCTAssertEqual(XCTWaiter.wait(for: [gone], timeout: 5), .completed, app.debugDescription)
        XCTAssertEqual(field.value as? String, "gesture draft")
        screenshot("interactive-keyboard-dismissed", app)
        let readonly = app.descendants(matching: .any)["Read-only sample"].firstMatch
        scrollTo(readonly, in: app)
        readonly.coordinate(withNormalizedOffset: CGVector(dx: 0, dy: 0.5)).withOffset(CGVector(dx: 60, dy: 0)).press(forDuration: 1)
        XCTAssertFalse(app.keyboards.firstMatch.exists, "Read-only selection opened a keyboard")
        XCTAssertTrue(editingMenu("Copy", in: app).waitForExistence(timeout: 5), app.debugDescription)
        XCTAssertFalse(editingMenu("Cut", in: app).exists)
        XCTAssertFalse(editingMenu("Paste", in: app).exists)
        screenshot("readonly-selection-menu", app)
        tapEditingMenu("Select All", in: app)
        tapEditingMenu("Copy", in: app)
        app.buttons["Read selection clipboard"].tap()
        XCTAssertTrue(app.staticTexts["Selection clipboard: Read-only text: Hello MyGo 👋 中文"].waitForExistence(timeout: 5), app.debugDescription)
        screenshot("readonly-system-copy-no-keyboard", app)
    }

    func testReadOnlySystemSelection() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openService("gestures", in: app)
        let readonly = app.descendants(matching: .any)["Read-only sample"].firstMatch
        scrollTo(readonly, in: app)
        readonly.coordinate(withNormalizedOffset: CGVector(dx: 0, dy: 0.5)).withOffset(CGVector(dx: 60, dy: 0)).press(forDuration: 1)
        XCTAssertFalse(app.keyboards.firstMatch.exists, "Read-only selection opened a keyboard")
        XCTAssertTrue(editingMenu("Copy", in: app).waitForExistence(timeout: 5), app.debugDescription)
        XCTAssertFalse(editingMenu("Cut", in: app).exists)
        XCTAssertFalse(editingMenu("Paste", in: app).exists)
        screenshot("readonly-selection-menu", app)
        tapEditingMenu("Select All", in: app)
        tapEditingMenu("Copy", in: app)
        app.buttons["Read selection clipboard"].tap()
        XCTAssertTrue(app.staticTexts["Selection clipboard: Read-only text: Hello MyGo 👋 中文"].waitForExistence(timeout: 5), app.debugDescription)
        screenshot("readonly-system-copy-no-keyboard", app)
    }

    func testPushRegistrationMissingEntitlement() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openService("notifications", in: app)
        let button=app.buttons["Register for push notifications"]
        scrollTo(button,in:app); button.tap()
        let error=app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'APNs registration:'")).firstMatch
        scrollTo(error,in:app)
        XCTAssertTrue(error.waitForExistence(timeout: 10), app.debugDescription)
        XCTAssertTrue(error.label.contains("aps-environment"), error.label)
        screenshot("push-missing-entitlement-error", app)
    }

    func testPinchAndRotation() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openService("gestures", in: app)
        let pad = app.descendants(matching: .any)["Gesture pad"].firstMatch
        let scale = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'Gesture scale:'")).firstMatch
        let rotation = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'Gesture rotation:'")).firstMatch
        scrollTo(pad, in: app)
        pad.pinch(withScale: 1.5, velocity: 1)
        scrollTo(scale,in:app)
        XCTAssertTrue(scale.exists)
        XCTAssertNotEqual(scale.label, "Gesture scale: 1.00", app.debugDescription)
        scrollTo(pad,in:app)
        pad.rotate(CGFloat.pi / 4, withVelocity: 1)
        scrollTo(rotation,in:app)
        XCTAssertTrue(rotation.exists)
        XCTAssertNotEqual(rotation.label, "Gesture rotation: 0.00", app.debugDescription)
        screenshot("pinch-and-rotation", app)
        scrollTo(app.buttons["Reset gestures"],in:app)
        app.buttons["Reset gestures"].tap()
        XCTAssertTrue(app.staticTexts["Gesture scale: 1.00"].exists)
        XCTAssertTrue(app.staticTexts["Gesture rotation: 0.00"].exists)
    }

    private func editingMenu(_ title: String, in app: XCUIApplication) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(format: "label ==[c] %@", title)).firstMatch
    }

    private func tapEditingMenu(_ title: String, in app: XCUIApplication) {
        for _ in 0..<4 {
            let item = editingMenu(title, in: app)
            if item.exists && item.isHittable { item.tap(); return }
            // The system edit menu paginates actions on compact phones.
            let next = app.buttons["Forward"]
            if next.exists { next.tap() }
            else if item.waitForExistence(timeout: 3) && item.isHittable { item.tap(); return }
        }
        XCTFail("Could not reach edit action \(title): \(app.debugDescription)")
    }

    func testSystemTextEditingAndPassword() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openShowcaseFeature("Text Editing", in: app)
        app.buttons["Reset sample"].tap()
        let field = app.descendants(matching: .any)["Editing sample"].firstMatch
        field.tap()
        XCTAssertTrue(app.keyboards.firstMatch.waitForExistence(timeout: 5))
        let point = field.coordinate(withNormalizedOffset: CGVector(dx: 0.15, dy: 0.16))
        point.press(forDuration: 1)
        XCTAssertTrue(editingMenu("Copy", in: app).waitForExistence(timeout: 5), app.debugDescription)
        screenshot("editing-word-selection-menu", app)
        tapEditingMenu("Select All", in: app)
        XCTAssertTrue(editingMenu("Cut", in: app).waitForExistence(timeout: 5), app.debugDescription)
        screenshot("editing-multiline-selection-handles", app)
        // Drag the trailing system handle back toward earlier text. Copying
        // verifies that UIKit hit testing changed the actual Go selection.
        let trailing = field.coordinate(withNormalizedOffset: .zero).withOffset(CGVector(dx: 166, dy: 62))
        let shorter = field.coordinate(withNormalizedOffset: .zero).withOffset(CGVector(dx: 150, dy: 30))
        trailing.press(forDuration: 0.15, thenDragTo: shorter, withVelocity: .slow, thenHoldForDuration: 0.2)
        screenshot("editing-system-handle-drag", app)
        tapEditingMenu("Copy", in: app)
        app.buttons["Read editing clipboard"].tap()
        let shortened = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'Copied: Hello MyGo'")).firstMatch
        XCTAssertTrue(shortened.waitForExistence(timeout: 5), app.debugDescription)
        XCTAssertFalse(shortened.label.contains("intact."), "Dragging the native handle did not change Go selection")
        app.buttons["Reset sample"].tap()
        field.tap()
        point.press(forDuration: 1)
        tapEditingMenu("Select All", in: app)
        tapEditingMenu("Cut", in: app)
        XCTAssertTrue(app.staticTexts["Editor is empty"].waitForExistence(timeout: 5), app.debugDescription)
        app.buttons["Read editing clipboard"].tap()
        XCTAssertTrue(app.staticTexts["Copied: Hello MyGo 👋\nSelect text, then cut, copy or paste.\n中文与 emoji stay intact."].waitForExistence(timeout: 5))
        field.tap()
        point.press(forDuration: 1)
        XCTAssertTrue(editingMenu("Paste", in: app).waitForExistence(timeout: 5), app.debugDescription)
        tapEditingMenu("Paste", in: app)
        XCTAssertFalse(app.staticTexts["Editor is empty"].exists)
        XCTAssertEqual(field.value as? String, "Hello MyGo 👋\nSelect text, then cut, copy or paste.\n中文与 emoji stay intact.")
        screenshot("editing-unicode-paste", app)
        app.buttons["Back"].tap()
        XCTAssertTrue(app.staticTexts["Features"].waitForExistence(timeout: 5))
        XCTAssertFalse(app.buttons["Back"].exists)
        XCTAssertFalse(app.keyboards.firstMatch.exists)
        app.buttons["Text Editing"].tap()
        let password = app.descendants(matching: .any)["Editing password"].firstMatch
        scrollTo(password, in: app)
        password.tap()
        XCTAssertTrue(app.keyboards.firstMatch.waitForExistence(timeout: 5))
        XCTAssertTrue(waitForHittable(password))
        password.press(forDuration: 1)
        XCTAssertTrue(editingMenu("Paste", in: app).waitForExistence(timeout: 5), app.debugDescription)
        XCTAssertFalse(editingMenu("Copy", in: app).exists)
        XCTAssertFalse(editingMenu("Cut", in: app).exists)
        screenshot("editing-password-paste-only", app)
        tapEditingMenu("Paste", in: app)
        XCTAssertFalse((password.value as? String)?.contains("Hello MyGo") == true)
        screenshot("editing-password-masked", app)
        let passwordLength = app.staticTexts["Password length: 73"]
        scrollTo(passwordLength, in: app)
        XCTAssertTrue(passwordLength.waitForExistence(timeout: 5), app.debugDescription)
    }

    func testNativeDialogsAndShare() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try reset(app)
        try openMobile(app)
        app.buttons["Native alert"].tap()
        XCTAssertTrue(app.alerts["MyGo native dialog"].waitForExistence(timeout: 5), app.debugDescription)
        app.alerts.buttons["Confirm"].tap()
        XCTAssertTrue(app.staticTexts["Dialog result: 0"].waitForExistence(timeout: 5), app.debugDescription)
        app.buttons["Native alert"].tap()
        XCTAssertTrue(app.alerts["MyGo native dialog"].waitForExistence(timeout: 5))
        app.alerts.buttons["Cancel"].tap()
        XCTAssertTrue(app.staticTexts["Dialog result: 1"].waitForExistence(timeout: 5), app.debugDescription)
        app.buttons["Native actions"].tap()
        XCTAssertTrue(app.buttons["Delete"].waitForExistence(timeout: 5), app.debugDescription)
        screenshot("mobile-native-action-sheet", app)
        app.buttons["Delete"].tap()
        XCTAssertTrue(app.staticTexts["Dialog result: 2"].waitForExistence(timeout: 5), app.debugDescription)
        app.buttons["Native actions"].tap()
        XCTAssertTrue(app.buttons["Cancel"].waitForExistence(timeout: 5), app.debugDescription)
        app.buttons["Cancel"].tap()
        XCTAssertTrue(app.staticTexts["Dialog result: 1"].waitForExistence(timeout: 5))
        app.buttons["Share text"].tap()
        let copy = app.cells.matching(NSPredicate(format: "label IN %@", ["Copy", "拷贝", "复制"])).firstMatch
        XCTAssertTrue(copy.waitForExistence(timeout: 5), app.debugDescription)
        screenshot("mobile-share-sheet", app)
        copy.tap()
        XCTAssertTrue(app.staticTexts["Share completed: true"].waitForExistence(timeout: 5), app.debugDescription)
        app.buttons["Read clipboard"].tap()
        XCTAssertTrue(app.staticTexts["Clipboard: MyGo mobile sharing"].waitForExistence(timeout: 5), app.debugDescription)
        app.buttons["Share text"].tap()
        let close = app.buttons.matching(NSPredicate(format: "label IN %@", ["Close", "关闭", "Cancel", "取消"])).firstMatch
        if close.waitForExistence(timeout: 3) {
            close.tap()
        } else {
            let dismiss = app.otherElements.matching(identifier: "PopoverDismissRegion").firstMatch
            XCTAssertTrue(dismiss.waitForExistence(timeout: 5), app.debugDescription)
            // The system's dismiss region extends well beyond the screen.
            // Target visible blank space instead of normalizing its frame.
            app.coordinate(withNormalizedOffset: CGVector(dx: 0.05, dy: 0.10)).tap()
        }
        XCTAssertTrue(app.staticTexts["Share completed: false"].waitForExistence(timeout: 5), app.debugDescription)
        app.buttons["Native alert"].tap()
        XCTAssertTrue(app.alerts["MyGo native dialog"].waitForExistence(timeout: 5), "Go stopped dispatching after cancelled sharing")
        app.alerts.buttons["Confirm"].tap()
    }

    func testDynamicTypePreferences() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try reset(app)
        try openMobile(app)
        let original = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'Text scale:'")).firstMatch.label
        app.terminate()
        app.launchArguments = ["-UIPreferredContentSizeCategoryName", "UICTContentSizeCategoryAccessibilityXXXL"]
        app.launch()
        try openMobile(app)
        let scaled = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'Text scale:'")).firstMatch
        XCTAssertNotEqual(scaled.label, original, app.debugDescription)
        screenshot("mobile-dynamic-type", app)
        app.terminate()
        app.launchArguments = []
    }

    func testNativePermissionsAndMissingPurpose() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        app.resetAuthorizationStatus(for: .camera)
        app.resetAuthorizationStatus(for: .microphone)
        defer { app.resetAuthorizationStatus(for: .camera) }
        try reset(app)
        try openMobile(app)
        let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
        scrollTo(app.buttons["Check camera permission"], in: app)
        app.buttons["Check camera permission"].tap()
        scrollToTopMobile(app)
        XCTAssertTrue(app.staticTexts["Permission camera: not-determined"].waitForExistence(timeout: 5), app.debugDescription)
        scrollTo(app.buttons["Request camera permission"], in: app)
        app.buttons["Request camera permission"].tap()
        let allow = springboard.buttons.matching(NSPredicate(format: "label IN %@", ["Allow", "允许"])).firstMatch
        XCTAssertTrue(allow.waitForExistence(timeout: 5), springboard.debugDescription)
        screenshot("mobile-camera-permission", springboard)
        allow.tap()
        scrollToTopMobile(app)
        XCTAssertTrue(app.staticTexts["Permission camera: granted"].waitForExistence(timeout: 5), app.debugDescription)
        scrollTo(app.buttons["Request microphone permission"], in: app)
        app.buttons["Request microphone permission"].tap()
        scrollToTopMobile(app)
        let missing = "mygo: permission request requires NSMicrophoneUsageDescription in ios.infoPlist"
        XCTAssertTrue(app.staticTexts[missing].waitForExistence(timeout: 5), app.debugDescription)
        XCTAssertFalse(springboard.alerts.firstMatch.exists, "Missing purpose string reached the OS prompt")
        app.terminate()
        app.resetAuthorizationStatus(for: .camera)
        app.launch()
        try openMobile(app)
        scrollTo(app.buttons["Request camera permission"], in: app)
        app.buttons["Request camera permission"].tap()
        let deny = springboard.buttons.matching(NSPredicate(format: "label IN %@", ["Don’t Allow", "Don't Allow", "不允许"])).firstMatch
        XCTAssertTrue(deny.waitForExistence(timeout: 5))
        deny.tap()
        scrollToTopMobile(app)
        XCTAssertTrue(app.staticTexts["Permission camera: denied"].waitForExistence(timeout: 5), app.debugDescription)
        scrollTo(app.buttons["Request camera permission"], in: app)
        app.buttons["Request camera permission"].tap()
        XCTAssertFalse(springboard.alerts.firstMatch.exists, "A denied permission should not prompt again")
        scrollTo(app.buttons["Open app settings"], in: app)
        app.buttons["Open app settings"].tap()
        let settings = XCUIApplication(bundleIdentifier: "com.apple.Preferences")
        XCTAssertTrue(settings.wait(for: .runningForeground, timeout: 10))
        app.activate()
        scrollToTopMobile(app)
        XCTAssertTrue(app.staticTexts["Settings opened"].waitForExistence(timeout: 5), app.debugDescription)
    }

    private func scrollToTopMobile(_ app: XCUIApplication) {
        for _ in 0..<8 {
            if app.staticTexts["Mobile services"].isHittable { break }
            swipe(app, up: false)
        }
    }

    func testKeychainBinaryUpdateAndColdRestoration() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try reset(app)
        try openMobile(app, keyboard: "secrets")
        XCTAssertTrue(app.staticTexts["Secure storage"].waitForExistence(timeout: 5))
        app.buttons["Delete secret"].tap()
        XCTAssertTrue(app.staticTexts["Secret deleted"].waitForExistence(timeout: 5))
        app.buttons["Read secret"].tap()
        XCTAssertTrue(app.staticTexts["Secret absent"].waitForExistence(timeout: 5), app.debugDescription)
        app.buttons["Set secret"].tap()
        XCTAssertTrue(app.staticTexts["Secret saved"].waitForExistence(timeout: 5), app.debugDescription)
        app.buttons["Read secret"].tap()
        XCTAssertTrue(app.staticTexts["Secret: 00ff010203"].waitForExistence(timeout: 5), app.debugDescription)
        app.terminate()
        try openMobile(app, keyboard: "secrets")
        app.buttons["Read secret"].tap()
        XCTAssertTrue(app.staticTexts["Secret: 00ff010203"].waitForExistence(timeout: 5), "Keychain value did not survive process termination")
        app.buttons["Update secret"].tap()
        XCTAssertTrue(app.staticTexts["Secret updated"].waitForExistence(timeout: 5))
        app.buttons["Read secret"].tap()
        XCTAssertTrue(app.staticTexts["Secret: e69bb4e696b020f09f918b"].waitForExistence(timeout: 5), app.debugDescription)
        screenshot("mobile-keychain-restored-and-updated", app)
        app.buttons["Empty secret"].tap()
        XCTAssertTrue(app.staticTexts["Empty secret saved"].waitForExistence(timeout: 5))
        app.buttons["Read secret"].tap()
        XCTAssertTrue(app.staticTexts["Secret: "].waitForExistence(timeout: 5), app.debugDescription)
        app.buttons["Delete secret"].tap()
        XCTAssertTrue(app.staticTexts["Secret deleted"].waitForExistence(timeout: 5))
        app.buttons["Read secret"].tap()
        XCTAssertTrue(app.staticTexts["Secret absent"].waitForExistence(timeout: 5))
    }

    func testNativeUI() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try reset(app)
        app.buttons["Increment"].tap()
        XCTAssertTrue(app.staticTexts["Count: 1"].waitForExistence(timeout: 3))
        app.buttons["Decrement"].tap()
        XCTAssertTrue(app.staticTexts["Count: 0"].waitForExistence(timeout: 3))
        let marker = editMessage(app)
        screenshot("native-ui-keyboard-and-unicode", app)
        app.buttons["Save"].tap()
        XCTAssertTrue(app.staticTexts["Saved"].waitForExistence(timeout: 3))
        XCUIDevice.shared.press(.home)
        app.activate()
        XCTAssertTrue(app.staticTexts["Lifecycle: active"].waitForExistence(timeout: 5), app.debugDescription)
        XCUIDevice.shared.orientation = .landscapeLeft
        XCTAssertTrue(app.buttons["Increment"].waitForExistence(timeout: 5))
        XCUIDevice.shared.orientation = .portrait
        app.terminate()
        try launchDiagnostics(app)
        XCTAssertTrue(messageLabel(marker, in: app).waitForExistence(timeout: 10), app.debugDescription)
        screenshot("native-ui-explicit-save-restored", app)
    }

    func testBackgroundRestorationAndCloseSemantics() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try reset(app)
        app.buttons["Increment"].tap()
        let marker = editMessage(app)
        // No Save tap: the Scene background callback must checkpoint edits.
        app.staticTexts["MyGo on iOS"].tap()
        let rows = app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'Row '"))
        for _ in 0..<3 { swipe(app, up: true) }
        let row = rows.allElementsBoundByIndex.first(where: { $0.isHittable })!
        let rowLabel = row.label
        let selected = rowLabel.replacingOccurrences(of: "Row ", with: "")
        row.tap()
        screenshot("native-ui-before-background", app)
        XCUIDevice.shared.press(.home)
        // iOS may suspend an idle app before XCTest observes the transition.
        // Both states mean the app has left the foreground; restoration below
        // still verifies that the background checkpoint actually persisted.
        let background = XCTNSPredicateExpectation(predicate: NSPredicate { _, _ in
            app.state == .runningBackground || app.state == .runningBackgroundSuspended
        }, object: nil)
        XCTAssertEqual(XCTWaiter.wait(for: [background], timeout: 5), .completed,
                       "App did not enter background: \(app.state.rawValue)")
        let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
        XCTAssertTrue(springboard.wait(for: .runningForeground, timeout: 5))
        // Querying SpringBoard waits for its transition animation to settle;
        // killing at the start of that animation need not enter background.
        XCTAssertTrue(springboard.otherElements.firstMatch.waitForExistence(timeout: 5))
        app.terminate()
        try launchDiagnostics(app)
        XCTAssertTrue(app.buttons[rowLabel].waitForExistence(timeout: 10), app.debugDescription)
        XCTAssertTrue(app.buttons[rowLabel].isHittable, "Scroll position was not restored: \(app.debugDescription)")
        screenshot("native-ui-background-scroll-restored", app)
        scrollToTop(app)
        XCTAssertTrue(app.staticTexts["Count: 1"].exists, app.debugDescription)
        XCTAssertTrue(messageLabel(marker, in: app).exists, app.debugDescription)
        XCTAssertTrue(app.staticTexts["Selected: \(selected)"].exists, app.debugDescription)
        XCTAssertTrue(app.staticTexts["Lifecycle: active"].exists, app.debugDescription)
        scrollTo(app.buttons["Try close"], in: app)
        app.buttons["Try close"].tap()
        scrollToTop(app)
        XCTAssertTrue(app.staticTexts["Close unsupported"].exists, app.debugDescription)
        scrollTo(app.buttons["Try quit"], in: app)
        app.buttons["Try quit"].tap()
        scrollToTop(app)
        XCTAssertTrue(app.staticTexts["Quit unsupported"].exists, app.debugDescription)
        app.buttons["Increment"].tap()
        XCTAssertTrue(app.staticTexts["Count: 2"].waitForExistence(timeout: 3), "Go UI stopped after TryQuit")
        screenshot("native-ui-close-and-quit-kept-alive", app)
        if #available(iOS 16.4, *) {
            app.open(URL(string: "mygo-native://demo/resume?source=ui")!)
            XCTAssertTrue(app.staticTexts["Opened: mygo-native://demo/resume?source=ui"].waitForExistence(timeout: 5), app.debugDescription)
            XCTAssertTrue(app.staticTexts["Count: 2"].exists, "Warm URL delivery discarded live state")
        }
    }

    func testViewAllFeaturesAlwaysOpensCatalog() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openShowcaseFeature("Navigation & State", in: app)
        XCTAssertTrue(app.staticTexts["Router Demo"].waitForExistence(timeout: 5))
        app.buttons["Open Detail"].tap()
        XCTAssertTrue(app.staticTexts["State Editor"].waitForExistence(timeout: 5))
        backgroundForCheckpoint(app)
        app.terminate()
        app.launch()
        XCTAssertTrue(app.staticTexts["State Editor"].waitForExistence(timeout: 15))

        for title in ["Navigation & State", "Dialogs & Sharing", "Images"] {
            app.buttons["Overview"].tap()
            let catalog = app.buttons["View All Features"]
            scrollTo(catalog, in: app)
            catalog.tap()
            XCTAssertTrue(app.staticTexts["Feature Demos"].waitForExistence(timeout: 5),
                          "Catalog button resumed the old detail instead of the list")
            XCTAssertTrue(app.staticTexts["Feature Demos"].isHittable,
                          "Catalog button retained a previous list scroll offset")
            XCTAssertFalse(app.buttons["Back"].exists, "Catalog is not the root page")
            screenshot("view-all-catalog-before-\(title)", app)
            scrollTo(app.buttons[title], in: app)
            app.buttons[title].tap()
            XCTAssertTrue(app.buttons["Back"].waitForExistence(timeout: 5))
            // The bottom tab itself still resumes this detail.
            app.buttons["Overview"].tap()
            app.buttons["Features"].tap()
            XCTAssertTrue(app.buttons["Back"].waitForExistence(timeout: 5))
            XCTAssertFalse(app.staticTexts["Feature Demos"].exists)
        }
        app.buttons["Overview"].tap()
        scrollTo(app.buttons["View All Features"], in: app)
        app.buttons["View All Features"].tap()
        XCTAssertTrue(app.staticTexts["Feature Demos"].waitForExistence(timeout: 5))
        backgroundForCheckpoint(app)
        app.terminate()
        app.launch()
        XCTAssertTrue(app.staticTexts["Feature Demos"].waitForExistence(timeout: 15))
        XCTAssertTrue(app.staticTexts["Feature Demos"].isHittable)
        XCTAssertFalse(app.buttons["Back"].exists)
        screenshot("view-all-catalog-cold-restoration", app)
    }

    func testTabNavigationAndRestoration() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        app.terminate()
        guard #available(iOS 16.4, *) else { throw XCTSkip("URL opening requires iOS 16.4") }
        app.open(URL(string: "mygo-native://demo/app/reset")!)
        XCTAssertTrue(app.staticTexts["MyGo Mobile"].waitForExistence(timeout: 15))
        for tab in ["Overview", "Features", "Status"] { XCTAssertTrue(app.buttons[tab].isHittable) }
        screenshot("feature-overview", app)
        app.buttons["Features"].tap()
        scrollTo(app.buttons["Navigation & State"], in: app)
        screenshot("feature-catalog", app)
        app.buttons["Navigation & State"].tap()
        XCTAssertTrue(app.staticTexts["Router Demo"].waitForExistence(timeout: 5))
        app.buttons["Open Detail"].tap()
        let input = app.descendants(matching: .any)["State input"].firstMatch
        input.tap()
        XCTAssertTrue(app.keyboards.firstMatch.waitForExistence(timeout: 5))
        let keyboard = app.keyboards.firstMatch
        for _ in 0..<3 {
            if keyboard.keys["a"].exists || keyboard.keys["A"].exists { break }
            app.buttons["Next keyboard"].tap()
        }
        let marker = "state" + String(UUID().uuidString.prefix(6)).lowercased()
        app.typeText(marker)
        app.buttons["Overview"].tap()
        XCTAssertTrue(app.staticTexts["MyGo Mobile"].waitForExistence(timeout: 5))
        XCTAssertFalse(app.keyboards.firstMatch.exists)
        app.buttons["Features"].tap()
        XCTAssertTrue(app.staticTexts["State Editor"].waitForExistence(timeout: 5))
        XCTAssertTrue((input.value as? String)?.contains(marker) == true)
        app.buttons["Save State"].tap()
        XCTAssertTrue(app.staticTexts[marker].waitForExistence(timeout: 5))
        screenshot("navigation-saved-state", app)
        app.buttons["Status"].tap()
        XCTAssertTrue(app.staticTexts["Runtime Status"].waitForExistence(timeout: 5))
        app.buttons["Features"].tap()
        XCTAssertTrue(app.staticTexts["Router Demo"].waitForExistence(timeout: 5))
        app.buttons["Back"].tap()
        XCTAssertTrue(waitForHittable(app.buttons["Navigation & State"]), "Feature catalog lost its scroll offset")
        app.buttons["Navigation & State"].tap()
        app.buttons["Open Detail"].tap()
        backgroundForCheckpoint(app)
        app.terminate()
        app.launch()
        XCTAssertTrue(app.staticTexts["State Editor"].waitForExistence(timeout: 15))
        XCTAssertTrue((input.value as? String)?.contains(marker) == true)
        app.buttons["Back"].tap()
        XCTAssertTrue(app.staticTexts[marker].waitForExistence(timeout: 5))
        app.buttons["Back"].tap()
        XCTAssertTrue(waitForHittable(app.buttons["Navigation & State"]), "Cold launch lost catalog scroll position")
        app.open(URL(string: "mygo-native://demo/app/home")!)
        XCTAssertTrue(app.staticTexts["MyGo Mobile"].waitForExistence(timeout: 5))
        screenshot("feature-overview-final", app)
    }

    func testChineseScrollingAndNavigation() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        guard #available(iOS 16.4, *) else { throw XCTSkip("URL opening requires iOS 16.4") }
        app.terminate()
        app.open(URL(string: "mygo-native://demo/app/reset")!)
        XCTAssertTrue(app.buttons["Overview"].waitForExistence(timeout: 10))
        for round in 0..<4 {
            app.buttons["Features"].tap()
            for _ in 0..<2 { swipe(app, up: true) }
            for _ in 0..<2 { swipe(app, up: false) }
            XCTAssertTrue(app.buttons["Text & Scrolling"].isHittable, app.debugDescription)
            app.buttons["Text & Scrolling"].tap()
            XCTAssertTrue(app.staticTexts["Chinese Text"].waitForExistence(timeout: 5))
            swipe(app, up: true)
            if round == 0 {
                // The page's right padding is free of content. A visible
                // scroll thumb would darken this strip after the swipe.
                for y in [0.20, 0.30, 0.40, 0.50, 0.60, 0.70, 0.80] {
                    assertSamplePixel(app, app, x: 0.982, y: y, rgb: [246, 247, 249])
                }
                screenshot("hidden-scrollbars-chinese-list", app)
            }
            swipe(app, up: false)
            app.buttons["Back"].tap()
            app.buttons["Status"].tap()
            app.buttons["Overview"].tap()
        }
        screenshot("chinese-feature-scroll", app)
    }

    private func openShowcaseFeature(_ title: String, in app: XCUIApplication) throws {
        app.terminate()
        guard #available(iOS 16.4, *) else { throw XCTSkip("URL opening requires iOS 16.4") }
        app.open(URL(string: "mygo-native://demo/app/reset")!)
        XCTAssertTrue(app.staticTexts["MyGo Mobile"].waitForExistence(timeout: 15))
        app.buttons["Features"].tap()
        scrollTo(app.buttons[title], in: app)
        app.buttons[title].tap()
    }

    func testFilePhotoPickerCancellation() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openShowcaseFeature("Files & Photos", in: app)
        for (button, result) in [("Import text file", "Files cancelled"),
                                 ("Import multiple files", "Files cancelled"),
                                 ("Choose photo", "Photos cancelled"),
                                 ("Choose photos", "Photos cancelled"),
                                 ("Export sample document", "Export cancelled")] {
            scrollTo(app.buttons[button], in: app)
            app.buttons[button].tap()
            let cancel = app.buttons.matching(NSPredicate(format: "label IN %@", ["Cancel", "取消"])).firstMatch
            if button == "Export sample document" && !cancel.exists {
                XCTAssertTrue(app.navigationBars.firstMatch.waitForExistence(timeout: 10))
                screenshot("picker-export-sheet", app)
                let top = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.09))
                let bottom = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.75))
                top.press(forDuration: 0.1, thenDragTo: bottom)
            } else {
                XCTAssertTrue(cancel.waitForExistence(timeout: 10), app.debugDescription)
                screenshot("picker-\(button)", app)
                cancel.tap()
            }
            scrollTo(app.staticTexts[result], in: app)
            XCTAssertTrue(app.staticTexts[result].waitForExistence(timeout: 10), app.debugDescription)
        }
        screenshot("picker-cancellations-completed", app)
        // Reusing the presenters for all five calls catches stale delegates
        // and callbacks that leave the request waiting after cancellation.
        app.buttons["Back"].tap()
        XCTAssertTrue(app.buttons["Files & Photos"].waitForExistence(timeout: 5))
    }

    private func pickerLocation(_ app: XCUIApplication) {
        // Restrict the round trip to the example's own Documents folder.
        let local = app.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS[c] %@", "iPhone")).firstMatch
        for _ in 0..<5 {
            if local.exists && local.isHittable { break }
            let back = app.navigationBars["FullDocumentManagerViewControllerNavigationBar"].buttons.firstMatch
            XCTAssertTrue(back.exists, app.debugDescription)
            XCTAssertFalse(["Cancel", "取消"].contains(back.label), app.debugDescription)
            back.tap()
        }
        XCTAssertTrue(local.waitForExistence(timeout: 5), app.debugDescription)
        local.tap()
        let folder = app.cells.matching(NSPredicate(format: "label BEGINSWITH %@", "MyGo iOS")).firstMatch
        XCTAssertTrue(folder.waitForExistence(timeout: 5), app.debugDescription)
        folder.tap()
    }

    func testDocumentExportAndImportRoundTrip() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openShowcaseFeature("Files & Photos", in: app)
        app.buttons["Export sample document"].tap()
        XCTAssertTrue(app.descendants(matching: .any).matching(NSPredicate(format: "label IN %@", ["Cancel", "取消"])).firstMatch.waitForExistence(timeout: 10))
        screenshot("document-export-locations", app)
        pickerLocation(app)
        let save = app.buttons.matching(NSPredicate(format: "label IN %@", ["Save", "存储", "保存"])).firstMatch
        XCTAssertTrue(save.waitForExistence(timeout: 5), app.debugDescription)
        save.tap()
        let replace = app.buttons.matching(NSPredicate(format: "label IN %@", ["Replace", "替换"])).firstMatch
        if replace.waitForExistence(timeout: 2) { replace.tap() }
        let completed = app.staticTexts["Export completed: mygo-picker-test.txt"]
        scrollTo(completed, in: app)
        XCTAssertTrue(completed.waitForExistence(timeout: 10), app.debugDescription)
        for button in ["Import text file", "Import multiple files"] {
            scrollTo(app.buttons[button], in: app)
            app.buttons[button].tap()
            let file = app.cells.matching(NSPredicate(format: "label CONTAINS %@", "mygo-picker-test")).firstMatch
            if !file.waitForExistence(timeout: 3) { pickerLocation(app) }
            XCTAssertTrue(file.waitForExistence(timeout: 5), app.debugDescription)
            if button == "Import multiple files" {
                let select = app.buttons.matching(NSPredicate(format: "label IN %@", ["Select", "选择"])).firstMatch
                if select.exists { select.tap() }
                file.tap()
                let second = app.cells.matching(NSPredicate(format: "label CONTAINS %@", "mygo-picker-fixture")).firstMatch
                XCTAssertTrue(second.exists, app.debugDescription)
                second.tap()
                let open = app.buttons.matching(NSPredicate(format: "label IN %@", ["Open", "打开"])).firstMatch
                XCTAssertTrue(open.exists, app.debugDescription)
                open.tap()
            } else { file.tap() }
            let count = button == "Import multiple files" ? 2 : 1
            let imported = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "Files imported: \(count)")).firstMatch
            scrollTo(imported, in: app)
            XCTAssertTrue(imported.waitForExistence(timeout: 10), app.debugDescription)
            XCTAssertTrue(imported.label.contains("MyGo document round trip\nChinese: 你好\nEmoji: 👋"))
            screenshot("document-round-trip-\(button)", app)
        }
        scrollTo(app.buttons["Delete imported copies"], in: app)
        app.buttons["Delete imported copies"].tap()
        XCTAssertTrue(app.staticTexts["Deleted 3 imported copies"].waitForExistence(timeout: 5))
        screenshot("document-import-cleanup", app)
    }

    func testPhotoImportCopiesFromSeededLibrary() throws {
        // Seed only generated fixtures with simctl addmedia; do not choose
        // images from a developer's private physical-device photo library.
#if !targetEnvironment(simulator)
        throw XCTSkip("Photo content test uses a simulator seeded with MyGo fixtures")
#else
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openShowcaseFeature("Files & Photos", in: app)
        for multiple in [false, true] {
            let button = multiple ? "Choose photos" : "Choose photo"
            scrollTo(app.buttons[button], in: app)
            app.buttons[button].tap()
            let onboarding = app.buttons.matching(NSPredicate(format: "label IN %@", ["Close", "关闭"])).firstMatch
            if onboarding.waitForExistence(timeout: 2) {
                onboarding.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
            }
            let photos = app.descendants(matching: .image).matching(identifier: "PXGGridLayout-Info")
            XCTAssertTrue(photos.firstMatch.waitForExistence(timeout: 10), app.debugDescription)
            screenshot("seeded-photo-picker", app)
            // PHPicker's remote image nodes do not advertise a hittable
            // action on this SDK. Their observed frames still locate tiles.
            photos.element(boundBy: 0).coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
            if multiple {
                XCTAssertGreaterThanOrEqual(photos.count, 2, app.debugDescription)
                photos.element(boundBy: 1).coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
                let add = app.buttons["Add"]
                XCTAssertTrue(add.waitForExistence(timeout: 5), app.debugDescription)
                add.tap()
            }
            let count = multiple ? 2 : 1
            let imported = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "Photos imported: \(count)")).firstMatch
            scrollTo(imported, in: app)
            XCTAssertTrue(imported.waitForExistence(timeout: 10), app.debugDescription)
            XCTAssertTrue(imported.label.contains(".png"), imported.label)
            XCTAssertTrue(imported.label.contains("36f32f4a4457058f") || imported.label.contains("0d134da09dcadcb6"), imported.label)
            if multiple {
                XCTAssertTrue(imported.label.contains("36f32f4a4457058f"), imported.label)
                XCTAssertTrue(imported.label.contains("0d134da09dcadcb6"), imported.label)
            }
            screenshot("photo-copies-\(count)", app)
        }
        scrollTo(app.buttons["Delete imported copies"], in: app)
        app.buttons["Delete imported copies"].tap()
        XCTAssertTrue(app.staticTexts["Deleted 3 imported copies"].waitForExistence(timeout: 5))
#endif
    }

    private func assertSamplePixel(_ app: XCUIApplication, _ element: XCUIElement,
                                   x: Double, y: Double, rgb: [Int], tolerance: Double = 6) {
        XCTAssertTrue(element.exists)
        let frame = element.frame
        let image = app.screenshot().image.cgImage!
        let scale = Double(image.width) / app.frame.width
        let px = (frame.minX + frame.width * x) * scale
        let py = (frame.minY + frame.height * y) * scale
        let pixel = image.cropping(to: CGRect(x: px.rounded(.down), y: py.rounded(.down), width: 1, height: 1))!
        var rgba = [UInt8](repeating: 0, count: 4)
        rgba.withUnsafeMutableBytes { bytes in
            let context = CGContext(data: bytes.baseAddress, width: 1, height: 1,
                                    bitsPerComponent: 8, bytesPerRow: 4,
                                    space: CGColorSpace(name: CGColorSpace.sRGB)!,
                                    bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
            context.draw(pixel, in: CGRect(x: 0, y: 0, width: 1, height: 1))
        }
        for channel in 0..<3 {
            XCTAssertEqual(Double(rgba[channel]), Double(rgb[channel]), accuracy: tolerance,
                           "Pixel differs in \(element.label), channel \(channel)")
        }
    }

    func testImageRenderingCases() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openShowcaseFeature("Images", in: app)
        XCTAssertTrue(app.staticTexts["Raster Images"].waitForExistence(timeout: 5))
        let preview = app.images["Image preview"]
        for format in ["PNG", "JPEG", "WebP", "EXIF", "GIF"] {
            app.buttons[format].tap()
            XCTAssertTrue(app.staticTexts["Format: \(format) · Fit: Contain"].waitForExistence(timeout: 3))
            assertSamplePixel(app, preview, x: 0.25, y: 0.25, rgb: [232, 91, 85])
            assertSamplePixel(app, preview, x: 0.75, y: 0.75, rgb: [50, 165, 130])
        }
        for fit in ["Cover", "Stretch", "Scale Down", "Natural", "Contain"] {
            app.buttons[fit].tap()
            XCTAssertTrue(app.staticTexts["Format: GIF · Fit: \(fit)"].waitForExistence(timeout: 3))
            assertSamplePixel(app, preview, x: 0.25, y: 0.25, rgb: [232, 91, 85])
        }
        screenshot("image-formats-and-fit", app)
        let gray = app.descendants(matching: .any)["Grayscale"].firstMatch
        scrollTo(gray, in: app)
        gray.tap()
        assertSamplePixel(app, preview, x: 0.25, y: 0.25, rgb: [121, 121, 121])
        gray.tap()
        scrollTo(app.links["Open thumbnail grid"], in: app)
        app.links["Open thumbnail grid"].tap()
        XCTAssertTrue(app.staticTexts["Image Grid"].waitForExistence(timeout: 5))
        app.staticTexts["Thumbnail 003"].tap()
        XCTAssertTrue(app.staticTexts["Selected thumbnail: 3"].waitForExistence(timeout: 3))
        let grid = app.descendants(matching: .any)["Thumbnail grid"].firstMatch
        let bottom = grid.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.85))
        let top = grid.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.15))
        for _ in 0..<3 { bottom.press(forDuration: 0.05, thenDragTo: top) }
        XCTAssertFalse(app.staticTexts["Thumbnail 001"].exists, "Grid did not scroll")
        screenshot("thumbnail-grid-scrolled", app)
        app.buttons["Back"].tap()
        scrollTo(app.images["Transparent PNG"], in: app)
        XCTAssertTrue(app.images["Color SVG"].exists)
        assertSamplePixel(app, app.images["Transparent PNG"], x: 0.1, y: 0.1, rgb: [243, 198, 78])
        assertSamplePixel(app, app.images["Transparent PNG"], x: 0.5, y: 0.5, rgb: [154, 163, 138])
        let svg = app.images["Color SVG"]
        let inset = (svg.frame.width - svg.frame.height * 2) / 2
        assertSamplePixel(app, svg, x: (inset + svg.frame.height * 210 / 120) / svg.frame.width,
                          y: 28.0 / 120, rgb: [246, 247, 249])
        screenshot("image-alpha-and-svg", app)
        scrollTo(app.buttons["Decode invalid image"], in: app)
        app.buttons["Decode invalid image"].tap()
        swipe(app, up: true)
        XCTAssertTrue(app.staticTexts["Decode failed: invalid image data"].waitForExistence(timeout: 3))
    }

    func testRemoteImageLoadingAndRetry() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openShowcaseFeature("Images", in: app)
        let url = app.descendants(matching: .any)["Remote image URL"].firstMatch
        scrollTo(url, in: app)
        let sample = "https://go.dev/doc/gopher/frontpage.png"
        func setURL(_ text: String) {
            let old = url.value as? String ?? ""
            url.coordinate(withNormalizedOffset: CGVector(dx: 0.98, dy: 0.5)).tap()
            XCTAssertTrue(app.keyboards.firstMatch.waitForExistence(timeout: 5))
            XCTAssertTrue(waitForHittable(url), "Keyboard hid the URL field")
            app.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: old.count) + text)
            app.staticTexts["Images"].tap()
            XCTAssertFalse(app.keyboards.firstMatch.exists)
        }
        setURL("mygo-invalid://image")
        scrollTo(app.buttons["Load remote image"], in: app)
        app.buttons["Load remote image"].tap()
        swipe(app, up: true)
        XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'Remote image failed:'")).firstMatch.waitForExistence(timeout: 5))
        screenshot("remote-image-failed", app)
        for _ in 0..<2 { swipe(app, up: false) }
        scrollTo(url, in: app)
        setURL(sample)
        scrollTo(app.buttons["Load remote image"], in: app)
        app.buttons["Load remote image"].tap()
        swipe(app, up: true)
        XCTAssertTrue(app.images["Remote image"].waitForExistence(timeout: 35), app.debugDescription)
        swipe(app, up: true)
        XCTAssertTrue(app.staticTexts["Remote image loaded"].waitForExistence(timeout: 3))
        screenshot("remote-image-retry-loaded", app)
        for _ in 0..<2 { swipe(app, up: false) }
        scrollTo(app.buttons["Clear image"], in: app)
        app.buttons["Clear image"].tap()
        swipe(app, up: true)
        XCTAssertTrue(app.staticTexts["Image cleared"].waitForExistence(timeout: 3))
        XCTAssertFalse(app.images["Remote image"].exists)
    }

    func testLinksAndRichText() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openShowcaseFeature("Links & Rich Text", in: app)
        XCTAssertTrue(app.staticTexts["Links & Text"].waitForExistence(timeout: 5))
        app.links["this inline action"].tap()
        XCTAssertTrue(app.staticTexts["Inline actions: 1"].waitForExistence(timeout: 3))
        let longLink = app.links["a long inline link that wraps across multiple lines"]
        let frame = longLink.frame
        XCTAssertGreaterThan(frame.height, 30, "Long link did not wrap")
        // The final line starts at the paragraph's left edge. Exercise its
        // own hit fragment, avoiding the empty area in the union of lines.
        app.coordinate(withNormalizedOffset: .zero).withOffset(CGVector(dx: frame.minX + 6, dy: frame.maxY - 8)).tap()
        XCTAssertTrue(app.staticTexts["Inline actions: 2"].waitForExistence(timeout: 3))
        screenshot("rich-text-inline-links", app)
        app.links["open linked detail"].tap()
        XCTAssertTrue(app.staticTexts["Linked Page"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts["Source: inline"].exists)
        app.links["View image examples"].tap()
        XCTAssertTrue(app.staticTexts["Raster Images"].waitForExistence(timeout: 5))
        app.buttons["Back"].tap()
        XCTAssertTrue(app.staticTexts["Linked Page"].waitForExistence(timeout: 5))
        edgeBack(app, complete: true)
        XCTAssertTrue(app.staticTexts["Links & Text"].waitForExistence(timeout: 5))
        scrollTo(app.buttons["Try unavailable URL"], in: app)
        app.buttons["Try unavailable URL"].tap()
        swipe(app, up: true)
        XCTAssertTrue(app.staticTexts["URL open failed"].waitForExistence(timeout: 3))
        for _ in 0..<2 { swipe(app, up: false) }
        scrollTo(app.links["Open example.com"], in: app)
        app.links["Open example.com"].tap()
        let browsers = [XCUIApplication(bundleIdentifier: "com.apple.mobilesafari"),
                        XCUIApplication(bundleIdentifier: "com.google.chrome.ios")]
        let browser = XCTNSPredicateExpectation(predicate: NSPredicate { _, _ in
            browsers.contains { $0.state == .runningForeground }
        }, object: nil)
        XCTAssertEqual(XCTWaiter.wait(for: [browser], timeout: 10), .completed)
        app.activate()
        XCTAssertTrue(app.buttons["Back"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.links["Open example.com"].isHittable, "Browser return lost the page offset")
        screenshot("external-link-return", app)
    }

    func testRenderingEffects() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        try openShowcaseFeature("Rendering", in: app)
        XCTAssertTrue(app.staticTexts["Rendering Samples"].waitForExistence(timeout: 5))
        assertSamplePixel(app, app.images["Gradient sample"], x: 0.1, y: 0.8, rgb: [62, 121, 207], tolerance: 10)
        assertSamplePixel(app, app.images["Gradient sample"], x: 0.9, y: 0.8, rgb: [124, 97, 213], tolerance: 10)
        app.buttons["Toggle opacity"].tap()
        XCTAssertTrue(app.staticTexts["Opacity: 0.4"].waitForExistence(timeout: 3))
        assertSamplePixel(app, app.images["Opacity sample"], x: 0.98, y: 0.5, rgb: [169, 198, 232])
        screenshot("rendering-gradient-opacity", app)
        scrollTo(app.images["Rounded clip sample"], in: app)
        let clip = app.images["Rounded clip sample"]
        assertSamplePixel(app, clip, x: 0.003, y: 0.01, rgb: [246, 247, 249])
        assertSamplePixel(app, clip, x: 0.25, y: 0.25, rgb: [232, 91, 85])
        scrollTo(app.images["Drawing sample"], in: app)
        assertSamplePixel(app, app.images["Drawing sample"], x: 0.15, y: 0.8, rgb: [50, 165, 130])
        screenshot("rendering-clipping-and-drawing", app)
    }

    private func edgeBack(_ app: XCUIApplication, complete: Bool) {
        let start = app.coordinate(withNormalizedOffset: CGVector(dx: 0.025, dy: 0.38))
        let end = app.coordinate(withNormalizedOffset: CGVector(dx: complete ? 0.82 : 0.22, dy: 0.38))
        start.press(forDuration: 0.05, thenDragTo: end,
                    withVelocity: .slow, thenHoldForDuration: 0.3)
    }

    func testFeatureCatalogIntegrations() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        guard #available(iOS 16.4, *) else { throw XCTSkip("URL opening requires iOS 16.4") }
        app.terminate()
        app.open(URL(string: "mygo-native://demo/app/reset")!)
        XCTAssertTrue(app.staticTexts["MyGo Mobile"].waitForExistence(timeout: 15))
        app.buttons["Features"].tap()

        func openFeature(_ title: String) {
            scrollTo(app.buttons[title], in: app)
            XCTAssertTrue(waitForHittable(app.buttons[title]))
            app.buttons[title].tap()
        }
        func backToCatalog() {
            app.buttons["Back"].tap()
            for _ in 0..<2 { swipe(app, up: false) }
            XCTAssertTrue(waitForHittable(app.buttons["Text & Scrolling"]))
        }

        openFeature("Keyboard & Input")
        XCTAssertTrue(app.staticTexts["Keyboard: email"].waitForExistence(timeout: 5))
        app.descendants(matching: .any)["Mobile field"].firstMatch.tap()
        XCTAssertTrue(app.keyboards.firstMatch.waitForExistence(timeout: 5))
        app.typeText("demo@example.com")
        let send = app.keyboards.firstMatch.buttons.matching(NSPredicate(format: "label ==[c] 'send'")).firstMatch
        XCTAssertTrue(send.exists)
        send.tap()
        XCTAssertTrue(app.staticTexts["Submissions: 1"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts["Input value: demo@example.com"].exists)
        screenshot("feature-keyboard-input", app)
        backToCatalog()

        openFeature("Dialogs & Sharing")
        XCTAssertTrue(waitForHittable(app.buttons["Native alert"]))
        app.buttons["Native alert"].tap()
        XCTAssertTrue(app.alerts.firstMatch.waitForExistence(timeout: 5))
        app.alerts.firstMatch.buttons["Cancel"].tap()
        XCTAssertTrue(app.staticTexts["Dialog result: 1"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.buttons["Share text"].isHittable)
        screenshot("feature-dialog-result", app)
        backToCatalog()

        openFeature("Permissions & Settings")
        XCTAssertTrue(waitForHittable(app.buttons["Check camera permission"]))
        app.buttons["Check camera permission"].tap()
        XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'Permission camera:'")).firstMatch.waitForExistence(timeout: 5))
        screenshot("feature-permission-result", app)
        backToCatalog()

        openFeature("Keychain Storage")
        XCTAssertTrue(app.staticTexts["Secure storage"].waitForExistence(timeout: 5))
        app.buttons["Set secret"].tap()
        XCTAssertTrue(app.staticTexts["Secret saved"].waitForExistence(timeout: 5))
        app.buttons["Read secret"].tap()
        XCTAssertTrue(app.staticTexts["Secret: 00ff010203"].waitForExistence(timeout: 5))
        screenshot("feature-keychain-result", app)
        app.buttons["Delete secret"].tap()
        XCTAssertTrue(app.staticTexts["Secret deleted"].waitForExistence(timeout: 5))
        backToCatalog()

        openFeature("Appearance & Accessibility")
        XCTAssertTrue(waitForHittable(app.buttons["Dark"]))
        app.buttons["Dark"].tap()
        assertBackground(app, rgb: [20, 33, 61])
        screenshot("feature-appearance-dark", app)
        app.buttons["Light"].tap()
        assertBackground(app, rgb: [246, 247, 249])
        app.open(URL(string: "mygo-native://demo/app/home")!)
        XCTAssertTrue(app.staticTexts["MyGo Mobile"].waitForExistence(timeout: 5))
    }

    func testInteractiveEdgeBackAndRestoration() throws {
        continueAfterFailure = false
        let app = XCUIApplication(bundleIdentifier: "com.mygo.iosnative")
        guard #available(iOS 16.4, *) else { throw XCTSkip("URL opening requires iOS 16.4") }
        app.terminate()
        app.open(URL(string: "mygo-native://demo/app/reset")!)
        XCTAssertTrue(app.staticTexts["MyGo Mobile"].waitForExistence(timeout: 15))
        app.buttons["Features"].tap()
        scrollTo(app.buttons["Navigation & State"], in: app)
        app.buttons["Navigation & State"].tap()
        app.buttons["Open Detail"].tap()
        let input = app.descendants(matching: .any)["State input"].firstMatch
        input.tap()
        XCTAssertTrue(app.keyboards.firstMatch.waitForExistence(timeout: 5))
        let keyboard = app.keyboards.firstMatch
        for _ in 0..<3 {
            if keyboard.keys["a"].exists || keyboard.keys["A"].exists { break }
            app.buttons["Next keyboard"].tap()
        }
        let marker = "gesture" + String(UUID().uuidString.prefix(6)).lowercased()
        app.typeText(marker)
        edgeBack(app, complete: false)
        XCTAssertTrue(app.staticTexts["State Editor"].exists, "Cancelled gesture navigated")
        XCTAssertTrue(app.keyboards.firstMatch.exists, "Cancelled gesture dismissed the keyboard")
        XCTAssertTrue((input.value as? String)?.contains(marker) == true)
        screenshot("edge-back-cancelled-editor", app)
        edgeBack(app, complete: true)
        screenshot("edge-back-completion-from-editor", app)
        XCTAssertTrue(app.staticTexts["Router Demo"].waitForExistence(timeout: 5), app.debugDescription)
        XCTAssertFalse(app.keyboards.firstMatch.exists, "Completed back retained the outgoing keyboard")
        app.buttons["Open Detail"].tap()
        XCTAssertTrue((input.value as? String)?.contains(marker) == true, "Gesture lost the draft")
        edgeBack(app, complete: true)
        screenshot("edge-back-completion-from-editor", app)
        XCTAssertTrue(app.staticTexts["Router Demo"].waitForExistence(timeout: 5), app.debugDescription)
        edgeBack(app, complete: true)
        XCTAssertTrue(app.buttons["Navigation & State"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.buttons["Navigation & State"].isHittable, "Back lost catalog scroll offset")
        XCTAssertFalse(app.buttons["Back"].exists)
        edgeBack(app, complete: true)
        XCTAssertFalse(app.buttons["Back"].exists, "Root gesture went past the history root")
        backgroundForCheckpoint(app)
        app.terminate()
        app.launch()
        XCTAssertTrue(app.buttons["Navigation & State"].waitForExistence(timeout: 15), app.debugDescription)
        XCTAssertTrue(app.buttons["Navigation & State"].isHittable, "Cold launch lost scroll offset after gesture navigation")
        XCTAssertFalse(app.buttons["Back"].exists, "Gesture did not checkpoint the updated history")
        screenshot("edge-back-restored-catalog", app)
        app.buttons["Navigation & State"].tap()
        app.buttons["Open Detail"].tap()
        XCTAssertTrue((input.value as? String)?.contains(marker) == true)
        edgeBack(app, complete: true)
        XCTAssertTrue(app.staticTexts["Router Demo"].waitForExistence(timeout: 5), "Restored router lost interactive Back")
        app.open(URL(string: "mygo-native://demo/app/home")!)
    }
}
