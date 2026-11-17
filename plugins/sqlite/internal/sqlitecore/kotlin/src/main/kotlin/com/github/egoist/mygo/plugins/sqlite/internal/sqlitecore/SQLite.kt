package com.github.egoist.mygo.plugins.sqlite.internal.sqlitecore

// SQLite's C API, as much as the module calls, in libsqlitecore.so
// (src/main/c): connections and statements are their pointers, text is
// UTF-8 bytes, and calls that fail return SQLite's code.
internal object SQLite {
    init {
        System.loadLibrary("sqlitecore")
    }

    const val OK = 0
    const val ROW = 100
    const val DONE = 101
    const val NO_STATEMENT = -1
    const val MORE_STATEMENTS = -2

    @JvmStatic external fun threadsafe(): Int
    @JvmStatic external fun version(): String
    @JvmStatic external fun open(path: ByteArray, readOnly: Boolean, timeout: Int, out: LongArray): Int
    @JvmStatic external fun close(h: Long): Int
    @JvmStatic external fun errcode(h: Long): Int
    @JvmStatic external fun errmsg(h: Long): ByteArray
    @JvmStatic external fun autocommit(h: Long): Boolean
    @JvmStatic external fun interrupt(h: Long)
    @JvmStatic external fun changes(h: Long): Long
    @JvmStatic external fun totalChanges(h: Long): Long
    @JvmStatic external fun lastInsertID(h: Long): Long
    @JvmStatic external fun prepare(h: Long, sql: ByteArray, control: Boolean, out: LongArray): Int
    @JvmStatic external fun finalize(s: Long)
    @JvmStatic external fun step(s: Long): Int
    @JvmStatic external fun parameters(s: Long): Int
    @JvmStatic external fun bindNull(s: Long, i: Int): Int
    @JvmStatic external fun bindInt(s: Long, i: Int, v: Long): Int
    @JvmStatic external fun bindReal(s: Long, i: Int, v: Double): Int
    @JvmStatic external fun bindBytes(s: Long, i: Int, v: ByteArray, text: Boolean): Int
    @JvmStatic external fun columns(s: Long): Int
    @JvmStatic external fun columnName(s: Long, i: Int): ByteArray
    @JvmStatic external fun columnType(s: Long, i: Int): Int
    @JvmStatic external fun columnInt(s: Long, i: Int): Long
    @JvmStatic external fun columnReal(s: Long, i: Int): Double
    @JvmStatic external fun columnBytes(s: Long, i: Int, text: Boolean): ByteArray
}
