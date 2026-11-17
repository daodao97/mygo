package com.github.egoist.mygo.plugins.sqlite.internal.sqlitecore

import dev.mygo.NativeClosedException
import dev.mygo.NativeException
import dev.mygo.NativeGoException

// The implementation of package sqlitecore on Android: SQLite's
// amalgamation, which the module's C compiles in (src/main/c), as the Zig
// build does on desktops, called as the Zig does.

fun makeEngine(): Engine {
    if (SQLite.threadsafe() == 0) throw NativeGoException("sqlite: the library is not threadsafe")
    return SQLiteEngine()
}

private class SQLiteEngine : Engine {
    override fun open(path: String, readOnly: Boolean, busyMillis: Long): Conn {
        val out = LongArray(1)
        val rc = SQLite.open(path.toByteArray(), readOnly, busyMillis.coerceIn(0, Int.MAX_VALUE.toLong()).toInt(), out)
        if (rc != SQLite.OK) {
            if (out[0] == 0L) throw NativeException("SQLite", rc.toLong(), "could not open database")
            val err = failure(out[0])
            SQLite.close(out[0])
            throw err
        }
        return SQLiteConn(out[0])
    }

    override fun version(): String = SQLite.version()
}

private class SQLiteConn(private var h: Long) : Conn {
    private fun db(): Long = h.takeIf { it != 0L } ?: throw NativeClosedException()

    override fun close() {
        if (h == 0L) return
        if (SQLite.close(h) != SQLite.OK) throw failure(h)
        h = 0
    }

    protected fun finalize() {
        if (h != 0L) SQLite.close(h)
    }

    override fun autocommit(): Boolean = h == 0L || SQLite.autocommit(h)

    override fun interrupt() {
        h.takeIf { it != 0L }?.let { SQLite.interrupt(it) }
    }

    override fun control(sql: String) {
        val db = db()
        val s = prepare(db, sql, true)
        try {
            val rc = SQLite.step(s)
            if (rc != SQLite.DONE) throw failure(db)
        } finally {
            SQLite.finalize(s)
        }
    }

    override fun run(sql: String, args: List<Value>, query: Boolean): Outcome {
        val db = db()
        val s = prepare(db, sql, false)
        try {
            val count = SQLite.parameters(s)
            if (count != args.size) throw NativeGoException("sqlite: statement needs $count parameters, got ${args.size}")
            args.forEachIndexed { i, v -> check(db, bind(s, i + 1, v)) }
            val n = if (query) SQLite.columns(s) else 0
            val columns = List(n) { String(SQLite.columnName(s, it)) }
            val values = ArrayList<Value>()
            val before = SQLite.totalChanges(db)
            while (true) {
                when (val rc = SQLite.step(s)) {
                    SQLite.ROW -> if (query) for (i in 0 until n) values.add(column(s, i))
                    SQLite.DONE -> break
                    else -> throw failure(db)
                }
            }
            val changes = if (SQLite.totalChanges(db) != before) SQLite.changes(db) else 0
            return Outcome(changes, SQLite.lastInsertID(db), columns, values)
        } finally {
            SQLite.finalize(s)
        }
    }
}

/** Prepares sql, which must be exactly one statement, on db. */
private fun prepare(db: Long, sql: String, control: Boolean): Long {
    val out = LongArray(1)
    when (val rc = SQLite.prepare(db, sql.toByteArray(), control, out)) {
        SQLite.OK -> return out[0]
        SQLite.NO_STATEMENT -> throw NativeGoException("sqlite: SQL contains no statement")
        SQLite.MORE_STATEMENTS -> throw NativeGoException("sqlite: expected exactly one statement")
        else -> throw failure(db)
    }
}

private fun bind(s: Long, i: Int, v: Value): Int = when (v.type) {
    Type.NULL -> SQLite.bindNull(s, i)
    Type.INTEGER -> SQLite.bindInt(s, i, v.int)
    Type.REAL -> SQLite.bindReal(s, i, v.real)
    Type.TEXT -> SQLite.bindBytes(s, i, v.text.toByteArray(), true)
    Type.BLOB -> SQLite.bindBytes(s, i, v.blob, false)
}

/** The value of column i of the current row. */
private fun column(s: Long, i: Int): Value = when (SQLite.columnType(s, i)) {
    1 -> Value(Type.INTEGER, SQLite.columnInt(s, i), 0.0, "")
    2 -> Value(Type.REAL, 0, SQLite.columnReal(s, i), "")
    3 -> Value(Type.TEXT, 0, 0.0, String(SQLite.columnBytes(s, i, true)))
    4 -> Value(Type.BLOB, 0, 0.0, "", SQLite.columnBytes(s, i, false))
    else -> Value(Type.NULL, 0, 0.0, "")
}

private fun check(db: Long, rc: Int) {
    if (rc != SQLite.OK) throw failure(db)
}

/**
 * The error of the last call on db that failed, which Go reports as an
 * sqlite.Error: SQLite's extended code and message.
 */
private fun failure(db: Long): NativeException =
    NativeException("SQLite", SQLite.errcode(db).toLong(), String(SQLite.errmsg(db)))
