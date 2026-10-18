import AppIntents
import Foundation

/// Implement run() in Swift so the action also works in an extension while the
/// Go application is stopped. AppIntent/SetValueIntent still declare their own
/// native metadata and parameters, allowing Xcode to extract them normally.
@available(iOS 16.0, *)
public protocol MyGoNativeAction: AppIntent {
    static var actionGroup: String { get }
    @MainActor func run() async throws
}

@available(iOS 16.0, *)
public extension MyGoNativeAction {
    func perform() async throws -> some IntentResult {
        try await MyGoActionExecution.perform(group: Self.actionGroup) { try await run() }
        return .result()
    }
}

public enum MyGoActionError: LocalizedError {
    case busy
    public var errorDescription: String? { "Another operation is already in progress. Please try again." }
}

/// Serializes operations in a named group within this process. Cancellation is
/// cooperative: the native handler must propagate cancellation and cancel its
/// own I/O. This does not provide a cross-process transaction or a time limit.
@MainActor
public enum MyGoActionExecution {
    private static var active: Set<String> = []
    public static func perform(group: String, operation: @MainActor () async throws -> Void) async throws {
        try Task.checkCancellation()
        guard active.insert(group).inserted else { throw MyGoActionError.busy }
        defer { active.remove(group) }
        try await operation()
        try Task.checkCancellation()
    }
}
