import CSQLite
import Foundation

// The implementation of package sqlitecore on iOS: SQLite 3.53.4, which
// CSQLite links in (build-xcframework.sh), called as the Zig does on
// desktops.

func makeEngine() throws -> any Engine {
    if sqlite3_threadsafe() == 0 { throw NativeError.go("sqlite: the library is not threadsafe") }
    return SQLiteEngine()
}

/// SQLITE_TRANSIENT: SQLite copies the value before the bind returns.
private let transient = unsafeBitCast(-1, to: sqlite3_destructor_type.self)

final class SQLiteEngine: Engine {
    func `open`(path: String, readOnly: Bool, busyMillis: Int) throws -> any Conn {
        let flags = (readOnly ? SQLITE_OPEN_READONLY : SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE) | SQLITE_OPEN_FULLMUTEX
        var db: OpaquePointer?
        let rc = sqlite3_open_v2(path, &db, flags, nil)
        guard rc == SQLITE_OK, let db else {
            let err = failure(db, rc)
            if let db { _ = sqlite3_close_v2(db) }
            throw err
        }
        _ = sqlite3_extended_result_codes(db, 1)
        return SQLiteConn(db, timeout: Int32(clamping: max(busyMillis, 0)))
    }

    func version() -> String { String(cString: sqlite3_libversion()) }
}

final class SQLiteConn: Conn {
    /// The connection, nil once closed.
    private var db: OpaquePointer?
    /// The state SQLite's handlers read, which it holds a pointer to.
    private let handlers: UnsafeMutablePointer<Handlers>

    init(_ db: OpaquePointer, timeout: Int32) {
        self.db = db
        handlers = .allocate(capacity: 1)
        handlers.initialize(to: Handlers(db: db, timeout: timeout))
        _ = sqlite3_set_authorizer(db, authorize, handlers)
        _ = sqlite3_busy_handler(db, busy, handlers)
    }

    deinit {
        if let db { _ = sqlite3_close_v2(db) }
        handlers.deinitialize(count: 1)
        handlers.deallocate()
    }

    private func connection() throws -> OpaquePointer {
        guard let db else { throw NativeError.closed }
        return db
    }

    func close() throws {
        guard let db else { return }
        let rc = sqlite3_close_v2(db)
        if rc != SQLITE_OK { throw failure(db, rc) }
        self.db = nil
    }

    func autocommit() -> Bool { db.map { sqlite3_get_autocommit($0) != 0 } ?? true }

    func interrupt() {
        if let db { sqlite3_interrupt(db) }
    }

    func control(sql: String) throws {
        let db = try connection()
        let stmt = try prepare(db, sql, control: true)
        defer { _ = sqlite3_finalize(stmt) }
        let rc = sqlite3_step(stmt)
        if rc != SQLITE_DONE { throw failure(db, rc) }
    }

    func run(sql: String, args: [Value], query: Bool) throws -> Outcome {
        let db = try connection()
        let stmt = try prepare(db, sql, control: false)
        defer { _ = sqlite3_finalize(stmt) }
        let count = Int(sqlite3_bind_parameter_count(stmt))
        if count != args.count { throw NativeError.go("sqlite: statement needs \(count) parameters, got \(args.count)") }
        for (i, arg) in args.enumerated() {
            let rc = bind(stmt, Int32(i + 1), arg)
            if rc != SQLITE_OK { throw failure(db, rc) }
        }
        let n = query ? sqlite3_column_count(stmt) : 0
        var out = Outcome(changes: 0, lastInsertID: 0, columns: [], values: [])
        out.columns = (0..<n).map { sqlite3_column_name(stmt, $0).map { String(cString: $0) } ?? "" }
        let before = sqlite3_total_changes64(db)
        while true {
            let rc = sqlite3_step(stmt)
            if rc == SQLITE_ROW {
                if query { for i in 0..<n { out.values.append(column(stmt, i)) } }
            } else if rc == SQLITE_DONE {
                break
            } else {
                throw failure(db, rc)
            }
        }
        out.lastInsertID = sqlite3_last_insert_rowid(db)
        if sqlite3_total_changes64(db) != before { out.changes = sqlite3_changes64(db) }
        return out
    }

    /// Prepares sql, which must be exactly one statement: it compiles what
    /// follows the first before anything runs, so that extra SQL, even
    /// invalid, never runs a part of an operation.
    private func prepare(_ db: OpaquePointer, _ sql: String, control: Bool) throws -> OpaquePointer {
        handlers.pointee.allowControl = control
        defer { handlers.pointee.allowControl = true }
        var text = Array(sql.utf8)
        let n = text.count
        text.append(0)
        return try text.withUnsafeBufferPointer { buf -> OpaquePointer in
            let base = UnsafeRawPointer(buf.baseAddress!).assumingMemoryBound(to: CChar.self)
            var first: OpaquePointer?
            var tail: UnsafePointer<CChar>?
            var rc = sqlite3_prepare_v2(db, base, Int32(n), &first, &tail)
            if rc != SQLITE_OK {
                if let first { _ = sqlite3_finalize(first) }
                throw failure(db, rc)
            }
            guard let first else { throw NativeError.go("sqlite: SQL contains no statement") }
            var offset = tail.map { base.distance(to: $0) } ?? n
            while offset < n {
                var extra: OpaquePointer?
                rc = sqlite3_prepare_v2(db, base + offset, Int32(n - offset), &extra, &tail)
                if rc != SQLITE_OK || extra != nil {
                    let err: any Error
                    if rc != SQLITE_OK {
                        err = failure(db, rc)
                    } else {
                        err = NativeError.go("sqlite: expected exactly one statement")
                    }
                    if let extra { _ = sqlite3_finalize(extra) }
                    _ = sqlite3_finalize(first)
                    throw err
                }
                let next = tail.map { base.distance(to: $0) } ?? n
                if next <= offset { break }
                offset = next
            }
            return first
        }
    }
}

/// What SQLite's handlers read: the wait for a lock, which Interrupt ends
/// too, and the transaction control a call allows.
private struct Handlers {
    let db: OpaquePointer
    let timeout: Int32
    var waited: Int32 = 0
    var allowControl = true
}

private let busy: @convention(c) (UnsafeMutableRawPointer?, Int32) -> Int32 = { data, previous in
    let h = data!.assumingMemoryBound(to: Handlers.self)
    if previous == 0 { h.pointee.waited = 0 }
    if sqlite3_is_interrupted(h.pointee.db) != 0 || h.pointee.waited >= h.pointee.timeout { return 0 }
    let delay = min(previous < 10 ? previous + 1 : 10, h.pointee.timeout - h.pointee.waited)
    h.pointee.waited += sqlite3_sleep(delay)
    return sqlite3_is_interrupted(h.pointee.db) == 0 ? 1 : 0
}

// Transactions belong to the batch API. ATTACH (including VACUUM INTO)
// could escape the plugin's database directory. Extension loading is
// omitted at compile time, and directory-changing pragmas are deprecated.
private let authorize: @convention(c) (UnsafeMutableRawPointer?, Int32, UnsafePointer<CChar>?, UnsafePointer<CChar>?, UnsafePointer<CChar>?, UnsafePointer<CChar>?) -> Int32 = { data, action, a, _, _, _ in
    let allow = data!.assumingMemoryBound(to: Handlers.self).pointee.allowControl
    switch action {
    case SQLITE_TRANSACTION, SQLITE_SAVEPOINT, SQLITE_DETACH:
        return allow ? SQLITE_OK : SQLITE_DENY
    case SQLITE_ATTACH:
        // Ordinary VACUUM attaches an unnamed temporary database while
        // stepping. User ATTACH is denied while preparing, and VACUUM INTO
        // a named output file is denied here too.
        return allow && a != nil && a!.pointee == 0 ? SQLITE_OK : SQLITE_DENY
    default:
        return SQLITE_OK
    }
}

private func bind(_ stmt: OpaquePointer, _ i: Int32, _ v: Value) -> Int32 {
    switch v.type {
    case .null:
        return sqlite3_bind_null(stmt, i)
    case .integer:
        return sqlite3_bind_int64(stmt, i, v.int)
    case .real:
        return sqlite3_bind_double(stmt, i, v.real)
    case .text:
        var text = v.text
        return text.withUTF8 { buf in
            buf.withMemoryRebound(to: CChar.self) { chars in
                guard let p = chars.baseAddress else { return "".withCString { sqlite3_bind_text(stmt, i, $0, 0, transient) } }
                return sqlite3_bind_text(stmt, i, p, Int32(chars.count), transient)
            }
        }
    case .blob:
        // A pointer that is not null, even for no bytes, keeps an empty BLOB
        // from being NULL.
        return v.blob.withUnsafeBytes { buf in
            guard let p = buf.baseAddress, buf.count > 0 else { return "".withCString { sqlite3_bind_blob(stmt, i, $0, 0, transient) } }
            return sqlite3_bind_blob(stmt, i, p, Int32(buf.count), transient)
        }
    }
}

/// The value of column i of the current row, copied.
private func column(_ stmt: OpaquePointer, _ i: Int32) -> Value {
    var v = Value(type: .null, int: 0, real: 0, text: "", blob: Data())
    switch sqlite3_column_type(stmt, i) {
    case SQLITE_INTEGER:
        v.type = .integer
        v.int = sqlite3_column_int64(stmt, i)
    case SQLITE_FLOAT:
        v.type = .real
        v.real = sqlite3_column_double(stmt, i)
    case SQLITE_TEXT:
        v.type = .text
        let p = sqlite3_column_text(stmt, i)
        let n = Int(sqlite3_column_bytes(stmt, i))
        v.text = p.map { String(decoding: UnsafeBufferPointer(start: $0, count: n), as: UTF8.self) } ?? ""
    case SQLITE_BLOB:
        v.type = .blob
        let p = sqlite3_column_blob(stmt, i)
        let n = Int(sqlite3_column_bytes(stmt, i))
        if let p, n > 0 { v.blob = Data(bytes: p, count: n) }
    default:
        break
    }
    return v
}

/// The error of the last call on db that failed with rc, which Go reports
/// as an sqlite.Error: SQLite's extended code and message.
private func failure(_ db: OpaquePointer?, _ rc: Int32) -> NSError {
    guard let db else { return NSError(domain: "SQLite", code: Int(rc), userInfo: [NSLocalizedDescriptionKey: "could not open database"]) }
    return NSError(domain: "SQLite", code: Int(sqlite3_extended_errcode(db)), userInfo: [NSLocalizedDescriptionKey: String(cString: sqlite3_errmsg(db))])
}
