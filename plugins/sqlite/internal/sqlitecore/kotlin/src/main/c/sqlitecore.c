// The JNI of the module's Kotlin (SQLite.kt): SQLite's C API, as much as
// it calls, with what must be C in C: the connection's handlers, which
// SQLite calls while it runs a statement, and the check that SQL holds
// exactly one statement. Text crosses as UTF-8 bytes, which Java's
// strings are not.

#include <jni.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include "sqlite3.h"

#define EXPORT JNIEXPORT __attribute__((visibility("default")))
#define FN(name) Java_com_github_egoist_mygo_plugins_sqlite_internal_sqlitecore_SQLite_##name

// The results of prepare that are no code of SQLite's.
enum { NO_STATEMENT = -1, MORE_STATEMENTS = -2 };

typedef struct {
	sqlite3 *db;
	int timeout_ms, waited_ms;
	int allow_control;
} conn;

static conn *of(jlong h) { return (conn *)(intptr_t)h; }
static sqlite3_stmt *stmt(jlong s) { return (sqlite3_stmt *)(intptr_t)s; }

// busy waits for another connection's lock, up to the connection's
// timeout, unless Interrupt ends it.
static int busy(void *data, int previous) {
	conn *c = data;
	if (previous == 0) c->waited_ms = 0;
	if (sqlite3_is_interrupted(c->db) || c->waited_ms >= c->timeout_ms) return 0;
	int delay = previous < 10 ? previous + 1 : 10;
	if (delay > c->timeout_ms - c->waited_ms) delay = c->timeout_ms - c->waited_ms;
	c->waited_ms += sqlite3_sleep(delay);
	return !sqlite3_is_interrupted(c->db);
}

// authorize denies transaction control outside Control, which the batch
// API owns, and ATTACH (including VACUUM INTO), which could escape the
// plugin's database directory. Extension loading is omitted at compile
// time, and directory-changing pragmas are deprecated.
static int authorize(void *data, int action, const char *a, const char *b, const char *d, const char *e) {
	conn *c = data;
	(void)b, (void)d, (void)e;
	switch (action) {
	case SQLITE_TRANSACTION:
	case SQLITE_SAVEPOINT:
	case SQLITE_DETACH:
		return c->allow_control ? SQLITE_OK : SQLITE_DENY;
	case SQLITE_ATTACH:
		// Ordinary VACUUM attaches an unnamed temporary database while
		// stepping. User ATTACH is denied while preparing, and VACUUM
		// INTO a named output file is denied here too.
		return c->allow_control && a != NULL && a[0] == 0 ? SQLITE_OK : SQLITE_DENY;
	}
	return SQLITE_OK;
}

// bytes copies b into memory of C's, NUL-terminated, or returns NULL.
static char *bytes(JNIEnv *env, jbyteArray b, jsize *n) {
	*n = (*env)->GetArrayLength(env, b);
	char *p = malloc((size_t)*n + 1);
	if (p == NULL) return NULL;
	(*env)->GetByteArrayRegion(env, b, 0, *n, (jbyte *)p);
	p[*n] = 0;
	return p;
}

static jbyteArray array(JNIEnv *env, const void *p, int n) {
	if (n < 0) n = 0;
	jbyteArray b = (*env)->NewByteArray(env, n);
	if (b != NULL && n > 0) (*env)->SetByteArrayRegion(env, b, 0, n, p);
	return b;
}

EXPORT jint FN(threadsafe)(JNIEnv *env, jclass cls) { return sqlite3_threadsafe(); }

EXPORT jstring FN(version)(JNIEnv *env, jclass cls) { return (*env)->NewStringUTF(env, sqlite3_libversion()); }

// open opens path and puts its connection in out[0], when SQLite made one,
// even when it failed: its error is the connection's then, and closing it
// is the caller's.
EXPORT jint FN(open)(JNIEnv *env, jclass cls, jbyteArray path, jboolean readOnly, jint timeout, jlongArray out) {
	jsize n;
	char *name = bytes(env, path, &n);
	if (name == NULL) return SQLITE_NOMEM;
	int flags = readOnly ? SQLITE_OPEN_READONLY : SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE;
	sqlite3 *db = NULL;
	int rc = sqlite3_open_v2(name, &db, flags | SQLITE_OPEN_FULLMUTEX, NULL);
	free(name);
	if (db == NULL) return rc;
	conn *c = calloc(1, sizeof *c);
	if (c == NULL) {
		sqlite3_close_v2(db);
		return SQLITE_NOMEM;
	}
	c->db = db;
	c->timeout_ms = timeout < 0 ? 0 : timeout;
	c->allow_control = 1;
	if (rc == SQLITE_OK) {
		sqlite3_extended_result_codes(db, 1);
		sqlite3_set_authorizer(db, authorize, c);
		sqlite3_busy_handler(db, busy, c);
	}
	jlong h = (jlong)(intptr_t)c;
	(*env)->SetLongArrayRegion(env, out, 0, 1, &h);
	return rc;
}

// close closes the connection, and frees it unless SQLite could not.
EXPORT jint FN(close)(JNIEnv *env, jclass cls, jlong h) {
	conn *c = of(h);
	int rc = sqlite3_close_v2(c->db);
	if (rc == SQLITE_OK) free(c);
	return rc;
}

EXPORT jint FN(errcode)(JNIEnv *env, jclass cls, jlong h) { return sqlite3_extended_errcode(of(h)->db); }

EXPORT jbyteArray FN(errmsg)(JNIEnv *env, jclass cls, jlong h) {
	const char *m = sqlite3_errmsg(of(h)->db);
	return array(env, m, (int)strlen(m));
}

EXPORT jboolean FN(autocommit)(JNIEnv *env, jclass cls, jlong h) { return sqlite3_get_autocommit(of(h)->db) != 0; }

EXPORT void FN(interrupt)(JNIEnv *env, jclass cls, jlong h) { sqlite3_interrupt(of(h)->db); }

EXPORT jlong FN(changes)(JNIEnv *env, jclass cls, jlong h) { return sqlite3_changes64(of(h)->db); }

EXPORT jlong FN(totalChanges)(JNIEnv *env, jclass cls, jlong h) { return sqlite3_total_changes64(of(h)->db); }

EXPORT jlong FN(lastInsertID)(JNIEnv *env, jclass cls, jlong h) { return sqlite3_last_insert_rowid(of(h)->db); }

// prepare prepares sql, which must be exactly one statement, into out[0]:
// it compiles what follows the first before anything runs, so that extra
// SQL, even invalid, never runs a part of an operation. control allows
// transaction control in it.
EXPORT jint FN(prepare)(JNIEnv *env, jclass cls, jlong h, jbyteArray sql, jboolean control, jlongArray out) {
	conn *c = of(h);
	jsize n;
	char *text = bytes(env, sql, &n);
	if (text == NULL) return SQLITE_NOMEM;
	c->allow_control = control;
	sqlite3_stmt *first = NULL;
	const char *tail = NULL;
	int rc = sqlite3_prepare_v2(c->db, text, n, &first, &tail);
	if (rc == SQLITE_OK && first == NULL) rc = NO_STATEMENT;
	size_t offset = tail != NULL ? (size_t)(tail - text) : (size_t)n;
	while (rc == SQLITE_OK && offset < (size_t)n) {
		sqlite3_stmt *extra = NULL;
		rc = sqlite3_prepare_v2(c->db, text + offset, (int)(n - offset), &extra, &tail);
		if (extra != NULL) {
			sqlite3_finalize(extra);
			rc = MORE_STATEMENTS;
		}
		size_t next = tail != NULL ? (size_t)(tail - text) : (size_t)n;
		if (next <= offset) break;
		offset = next;
	}
	c->allow_control = 1;
	free(text);
	if (rc != SQLITE_OK) {
		if (first != NULL) sqlite3_finalize(first);
		return rc;
	}
	jlong s = (jlong)(intptr_t)first;
	(*env)->SetLongArrayRegion(env, out, 0, 1, &s);
	return SQLITE_OK;
}

EXPORT void FN(finalize)(JNIEnv *env, jclass cls, jlong s) { sqlite3_finalize(stmt(s)); }

EXPORT jint FN(step)(JNIEnv *env, jclass cls, jlong s) { return sqlite3_step(stmt(s)); }

EXPORT jint FN(parameters)(JNIEnv *env, jclass cls, jlong s) { return sqlite3_bind_parameter_count(stmt(s)); }

EXPORT jint FN(bindNull)(JNIEnv *env, jclass cls, jlong s, jint i) { return sqlite3_bind_null(stmt(s), i); }

EXPORT jint FN(bindInt)(JNIEnv *env, jclass cls, jlong s, jint i, jlong v) { return sqlite3_bind_int64(stmt(s), i, v); }

EXPORT jint FN(bindReal)(JNIEnv *env, jclass cls, jlong s, jint i, jdouble v) { return sqlite3_bind_double(stmt(s), i, v); }

// bindBytes binds b as text or a BLOB, which SQLite copies. An empty BLOB
// binds a pointer that is not NULL, which keeps it from being NULL.
EXPORT jint FN(bindBytes)(JNIEnv *env, jclass cls, jlong s, jint i, jbyteArray b, jboolean text) {
	jsize n = (*env)->GetArrayLength(env, b);
	jbyte *p = n > 0 ? (*env)->GetByteArrayElements(env, b, NULL) : NULL;
	if (n > 0 && p == NULL) return SQLITE_NOMEM;
	const void *v = p != NULL ? (const void *)p : "";
	int rc = text ? sqlite3_bind_text(stmt(s), i, v, n, SQLITE_TRANSIENT) : sqlite3_bind_blob(stmt(s), i, v, n, SQLITE_TRANSIENT);
	if (p != NULL) (*env)->ReleaseByteArrayElements(env, b, p, JNI_ABORT);
	return rc;
}

EXPORT jint FN(columns)(JNIEnv *env, jclass cls, jlong s) { return sqlite3_column_count(stmt(s)); }

EXPORT jbyteArray FN(columnName)(JNIEnv *env, jclass cls, jlong s, jint i) {
	const char *name = sqlite3_column_name(stmt(s), i);
	return array(env, name, name != NULL ? (int)strlen(name) : 0);
}

EXPORT jint FN(columnType)(JNIEnv *env, jclass cls, jlong s, jint i) { return sqlite3_column_type(stmt(s), i); }

EXPORT jlong FN(columnInt)(JNIEnv *env, jclass cls, jlong s, jint i) { return sqlite3_column_int64(stmt(s), i); }

EXPORT jdouble FN(columnReal)(JNIEnv *env, jclass cls, jlong s, jint i) { return sqlite3_column_double(stmt(s), i); }

EXPORT jbyteArray FN(columnBytes)(JNIEnv *env, jclass cls, jlong s, jint i, jboolean text) {
	const void *p = text ? (const void *)sqlite3_column_text(stmt(s), i) : sqlite3_column_blob(stmt(s), i);
	return array(env, p, p != NULL ? sqlite3_column_bytes(stmt(s), i) : 0);
}
