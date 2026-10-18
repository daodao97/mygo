import Foundation

public enum MyGoSharedStoreError: Error {
    case invalidName, unavailableContainer, tooLarge, incompatibleVersion, expired
}

/// Uses the same JSON envelope as Go's sharedstore package. Dates are Unix
/// milliseconds, avoiding platform-dependent Codable date strategies.
public struct MyGoSharedSnapshot<Value: Codable>: Codable {
    public var version: Int
    public var updatedAt: Int64
    public var expiresAt: Int64?
    public var value: Value
    public init(version: Int, updatedAt: Int64, expiresAt: Int64? = nil, value: Value) {
        self.version = version; self.updatedAt = updatedAt; self.expiresAt = expiresAt; self.value = value
    }
}

/// A minimal read-only companion to Go's sharedstore. Readers need no lock:
/// writers replace complete files atomically. This object must not be used for
/// security decisions; all participants in an App Group share trust.
public struct MyGoSharedStore {
    public let directory: URL
    public let maxBytes: Int
    public init(group: String, subdirectory: String? = nil, maxBytes: Int = 1_048_576) throws {
        guard group.hasPrefix("group."), !group.contains("/"), !group.contains("\0"), maxBytes > 0, maxBytes <= 67_108_864 else { throw MyGoSharedStoreError.invalidName }
        guard var url = FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: group) else { throw MyGoSharedStoreError.unavailableContainer }
        if let name = subdirectory {
            try Self.validate(name)
            url.appendPathComponent(name, isDirectory: true)
        }
        self.directory = url; self.maxBytes = maxBytes
    }
    public func read<Value: Codable>(_ name: String, version: Int, now: Date = .now) throws -> MyGoSharedSnapshot<Value> {
        try Self.validate(name)
        let url = directory.appendingPathComponent(name)
        // Read at most limit + 1; reject oversized files without mapping them.
        let handle = try FileHandle(forReadingFrom: url)
        defer { try? handle.close() }
        var data = Data()
        while data.count <= maxBytes {
            let chunk = try handle.read(upToCount: min(65_536, maxBytes + 1 - data.count)) ?? Data()
            if chunk.isEmpty { break }
            data.append(chunk)
        }
        guard data.count <= maxBytes else { throw MyGoSharedStoreError.tooLarge }
        let snapshot = try JSONDecoder().decode(MyGoSharedSnapshot<Value>.self, from: data)
        guard snapshot.version == version, snapshot.updatedAt > 0 else { throw MyGoSharedStoreError.incompatibleVersion }
        if let expiry = snapshot.expiresAt, expiry <= Int64(now.timeIntervalSince1970 * 1000) { throw MyGoSharedStoreError.expired }
        return snapshot
    }
    private static func validate(_ name: String) throws {
        guard !name.isEmpty, name != ".", name != "..", !name.contains("/"), !name.contains("\\"), !name.contains("\0") else { throw MyGoSharedStoreError.invalidName }
    }
}
