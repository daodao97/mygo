import AppIntents
import Foundation

@MainActor private var actionValue = false
struct SimpleTestAction: MyGoNativeAction {
    static var title: LocalizedStringResource = "Test"
    static var actionGroup: String { "test" }
    @MainActor func run() async throws { actionValue = true }
}

@main
struct ActionsTest {
    @MainActor static func main() async throws {
        var held: CheckedContinuation<Void, Never>?
        var first: Task<Void, Error>?
        await withCheckedContinuation { ready in
            first = Task { @MainActor in
                try await MyGoActionExecution.perform(group: "test") {
                    await withCheckedContinuation { finish in held = finish; ready.resume() }
                }
            }
        }
        do {
            try await MyGoActionExecution.perform(group: "test") { fatalError("busy action ran") }
            fatalError("overlapping action was accepted")
        } catch MyGoActionError.busy {}
        held?.resume()
        try await first?.value
        enum Failure: Error { case expected }
        do { try await MyGoActionExecution.perform(group: "test") { throw Failure.expected } }
        catch Failure.expected {}
        _ = try await SimpleTestAction().perform()
        precondition(actionValue, "protocol default did not invoke native handler")
        let cancelled = Task { @MainActor in
            withUnsafeCurrentTask { $0?.cancel() }
            do { try await MyGoActionExecution.perform(group: "test") { fatalError("cancelled action ran") }; fatalError("cancel ignored") }
            catch is CancellationError {}
        }
        try await cancelled.value
        let data = try Data(contentsOf: URL(fileURLWithPath: "native/ios/tests/snapshot-v1.json"))
        let snapshot = try JSONDecoder().decode(MyGoSharedSnapshot<[String: Int]>.self, from: data)
        precondition(snapshot.version == 1 && snapshot.updatedAt == 1_800_000_000_000 && snapshot.value["count"] == 4)
        let encoded = try JSONEncoder().encode(snapshot)
        let wire = try JSONSerialization.jsonObject(with: encoded) as! [String: Any]
        precondition((wire["expiresAt"] as? NSNumber)?.int64Value == 1_800_000_060_000)
        print("PASS: action conformance, overlap rejection, error cleanup, cancellation and snapshot wire format")
    }
}
