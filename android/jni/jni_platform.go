//go:build android && cgo

package jni

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>

// The JNI half of the Android platform features: the clipboard, opening a URL,
// notifications and the accessibility tree.
//
// # Two directions, and why this file needs the JavaVM
//
// jni.go's //export functions are Java calling Go, and each one already gets
// the JNIEnv it needs as its first argument. This file also has to call Java —
// the clipboard, an Intent and a NotificationManager are Java objects — and
// that is the direction JNI does not hand you anything for: a Go goroutine is
// not a thread the JVM knows, so it has no JNIEnv until it is attached to a
// JavaVM, and the JavaVM is only reachable from a thread that already has an
// env. The unlock is that Java calls nativeAttach once, at install, from a
// thread that has both: that call keeps a global reference to the JavaVM and to
// the MygoPlatform object, and every call below is made through them.
//
// # Threads
//
// mygo_platform_env attaches the calling Go thread on first use. On Android the
// caller is the backend's pump goroutine, which is locked to its OS thread for
// the life of the process, so it is attached once and never detached: detaching
// it would leave the backend unable to reach Java afterwards, and a thread that
// lives as long as the process leaks nothing by staying attached.
//
// Every call below is synchronous, which is what the platform.Clipboard and
// platform.Shell contracts want. The Java side is what hops to the Android UI
// thread where Android insists on it (announcements, accessibility events, the
// notification permission dialog) — doing that here would mean this file
// knowing about a Looper, and the Java side already has one.

// The JavaVM and the MygoPlatform object, set by nativeAttach.
static JavaVM *mygo_platform_vm = NULL;
static jobject mygo_platform_obj = NULL;

// The methods of MygoPlatform this file calls. They are looked up once, in
// nativeAttach, and every call checks its slot before using it: a Java method
// that was renamed is then a "the platform is not attached" error, not a crash
// inside JNI.
static jmethodID mygo_mid_read_text = NULL;
static jmethodID mygo_mid_read_html = NULL;
static jmethodID mygo_mid_mimes = NULL;
static jmethodID mygo_mid_write = NULL;
static jmethodID mygo_mid_clear = NULL;
static jmethodID mygo_mid_open = NULL;
static jmethodID mygo_mid_notify_allowed = NULL;
static jmethodID mygo_mid_notify = NULL;
static jmethodID mygo_mid_cancel = NULL;
static jmethodID mygo_mid_cancel_all = NULL;
static jmethodID mygo_mid_announce = NULL;
static jmethodID mygo_mid_set_tree = NULL;
static jmethodID mygo_mid_secret_set = NULL;
static jmethodID mygo_mid_secret_get = NULL;
static jmethodID mygo_mid_secret_delete = NULL;
static jmethodID mygo_mid_scan = NULL;
static jmethodID mygo_mid_scan_cancel = NULL;
static jmethodID mygo_mid_take_scan = NULL;
static jmethodID mygo_mid_exit = NULL;

// mygo_platform_exception clears a pending Java exception and reports whether
// there was one.
//
// Every call below clears one. A pending exception that survives the JNI call
// that raised it poisons the thread: the next CallXxxMethod returns immediately
// with a default value and does nothing, which reads as "the clipboard is
// empty" everywhere. The Java methods are written to catch what they raise and
// answer with a value, so this is the belt to that suspenders.
static int mygo_platform_exception(JNIEnv *env) {
	if ((*env)->ExceptionCheck(env)) {
		(*env)->ExceptionClear(env);
		return 1;
	}
	return 0;
}

// mygo_platform_env returns a JNIEnv for the calling thread, attaching it to the
// JavaVM the first time. NULL means the Java side never attached.
static JNIEnv *mygo_platform_env(void) {
	JNIEnv *env = NULL;
	if (mygo_platform_vm == NULL) {
		return NULL;
	}
	jint rc = (*mygo_platform_vm)->GetEnv(mygo_platform_vm, (void **)&env, JNI_VERSION_1_6);
	if (rc == JNI_EDETACHED) {
		// jni.h's C mapping takes JNIEnv** here, unlike GetEnv's void** a
		// line above: the two really are declared differently.
		rc = (*mygo_platform_vm)->AttachCurrentThread(mygo_platform_vm, &env, NULL);
	}
	if (rc != JNI_OK) {
		return NULL;
	}
	return env;
}

// mygo_platform_ready reports whether nativeAttach has run and every method this
// file calls was found.
static jint mygo_platform_ready(void) {
	if (mygo_platform_vm == NULL || mygo_platform_obj == NULL || mygo_mid_read_text == NULL) {
		return 0;
	}
	return 1;
}

// mygo_platform_java_bytes copies the bytes of a byte[] argument of a //export
// function into memory malloc'd here, with the length as its result. The caller
// frees *out with free(). A NULL array is length 0 and *out NULL.
//
// The length is read here rather than passed as a parameter because the JNI
// signature of a native method taking byte[] has no length in it: the JVM calls
// Java_org_mygo_MygoPlatform_nativeNotificationClicked(JNIEnv*, jobject,
// jbyteArray), and a fourth Go parameter would be reading whatever the caller's
// registers happen to hold. (The generated libmygo.h is what caught this: see
// the //export functions below.)
static int mygo_platform_java_bytes(JNIEnv *env, jbyteArray a, char **out) {
	*out = NULL;
	if (env == NULL || a == NULL) {
		return 0;
	}
	jsize n = (*env)->GetArrayLength(env, a);
	if (n < 0) {
		mygo_platform_exception(env);
		return 0;
	}
	char *buf = (char *)malloc(n > 0 ? (size_t)n : 1);
	if (buf == NULL) {
		return 0;
	}
	if (n > 0) {
		(*env)->GetByteArrayRegion(env, a, 0, n, (jbyte *)buf);
		if (mygo_platform_exception(env)) {
			free(buf);
			return 0;
		}
	}
	*out = buf;
	return (int)n;
}

// mygo_platform_bytes makes a Java byte[] of len bytes at data, or NULL when
// data is NULL (which the callers use to mean "this representation is absent").
//
// # Why bytes and not a jstring
//
// NewStringUTF takes *modified* UTF-8: it cannot carry a NUL and it mangles the
// supplementary characters (emoji, and a good deal of CJK-extended text) that
// JNI hands back as two surrogate halves. Clipboard text is exactly the place
// where those characters show up, so the two sides exchange UTF-8 bytes and the
// Java side decodes them with Charsets.UTF_8 — the one encoding that survives
// the trip whole.
static jbyteArray mygo_platform_bytes(JNIEnv *env, const char *data, int len) {
	if (data == NULL) {
		return NULL;
	}
	jbyteArray a = (*env)->NewByteArray(env, (jsize)len);
	if (a == NULL) {
		return NULL;
	}
	if (len > 0) {
		(*env)->SetByteArrayRegion(env, a, 0, (jsize)len, (const jbyte *)data);
		if (mygo_platform_exception(env)) {
			(*env)->DeleteLocalRef(env, a);
			return NULL;
		}
	}
	return a;
}

// mygo_platform_arg is mygo_platform_bytes for an argument that is required but
// may be empty: a NULL data pointer becomes an empty array rather than "absent".
// An empty string is a value a user can copy, and the two sides must not
// confuse it with no value at all.
static jbyteArray mygo_platform_arg(JNIEnv *env, const char *data, int len) {
	return mygo_platform_bytes(env, data != NULL ? data : "", len);
}

// mygo_platform_grab copies the Java byte[] a into memory malloc'd here, with
// its length in *out_len. NULL means the method returned nothing — no value, or
// a call that failed. The caller frees the result with free().
//
// An empty array copies to a 1-byte buffer so that its non-NULL result keeps
// meaning "the value is here and it is empty", which is how the Go side tells an
// empty string from an absent one.
static char *mygo_platform_grab(JNIEnv *env, jbyteArray a, int *out_len) {
	*out_len = 0;
	if (a == NULL) {
		return NULL;
	}
	jsize n = (*env)->GetArrayLength(env, a);
	if (n < 0) {
		mygo_platform_exception(env);
		return NULL;
	}
	char *out = (char *)malloc(n > 0 ? (size_t)n : 1);
	if (out == NULL) {
		return NULL;
	}
	if (n > 0) {
		(*env)->GetByteArrayRegion(env, a, 0, n, (jbyte *)out);
		if (mygo_platform_exception(env)) {
			free(out);
			return NULL;
		}
	}
	*out_len = (int)n;
	return out;
}

// mygo_platform_call_bytes calls a no-argument method that returns byte[] and
// returns a malloc'd copy of the result.
static char *mygo_platform_call_bytes(jmethodID mid, int *out_len) {
	*out_len = 0;
	JNIEnv *env = mygo_platform_env();
	if (env == NULL || mygo_platform_obj == NULL || mid == NULL) {
		return NULL;
	}
	jbyteArray a = (jbyteArray)(*env)->CallObjectMethod(env, mygo_platform_obj, mid);
	if (mygo_platform_exception(env)) {
		return NULL;
	}
	char *out = mygo_platform_grab(env, a, out_len);
	if (a != NULL) {
		(*env)->DeleteLocalRef(env, a);
	}
	return out;
}

// mygo_platform_call_int calls a method taking one byte[] and returning int.
// -1 means "the call did not happen" (no Java side, or a Java exception).
static jint mygo_platform_call_int(jmethodID mid, const char *data, int len) {
	JNIEnv *env = mygo_platform_env();
	if (env == NULL || mygo_platform_obj == NULL || mid == NULL) {
		return -1;
	}
	jbyteArray a = mygo_platform_arg(env, data, len);
	if (a == NULL) {
		return -1;
	}
	jint rc = (*env)->CallIntMethod(env, mygo_platform_obj, mid, a);
	(*env)->DeleteLocalRef(env, a);
	if (mygo_platform_exception(env)) {
		return -1;
	}
	return rc;
}

// mygo_platform_call_bool calls a method taking one byte[] and returning
// boolean. -1 is "the call did not happen", 1 true and 0 false.
static jint mygo_platform_call_bool(jmethodID mid, const char *data, int len) {
	JNIEnv *env = mygo_platform_env();
	if (env == NULL || mygo_platform_obj == NULL || mid == NULL) {
		return -1;
	}
	jbyteArray a = mygo_platform_arg(env, data, len);
	if (a == NULL) {
		return -1;
	}
	jboolean ok = (*env)->CallBooleanMethod(env, mygo_platform_obj, mid, a);
	(*env)->DeleteLocalRef(env, a);
	if (mygo_platform_exception(env)) {
		return -1;
	}
	return ok == JNI_TRUE ? 1 : 0;
}

// mygo_platform_call_void calls a method taking one byte[] and returning void.
// -1 is "the call did not happen".
static jint mygo_platform_call_void(jmethodID mid, const char *data, int len) {
	JNIEnv *env = mygo_platform_env();
	if (env == NULL || mygo_platform_obj == NULL || mid == NULL) {
		return -1;
	}
	jbyteArray a = mygo_platform_arg(env, data, len);
	if (a == NULL) {
		return -1;
	}
	(*env)->CallVoidMethod(env, mygo_platform_obj, mid, a);
	(*env)->DeleteLocalRef(env, a);
	if (mygo_platform_exception(env)) {
		return -1;
	}
	return 0;
}

// mygo_platform_call_two_bool calls writeClipboard(byte[] plain, byte[] html)
// where either argument may be NULL (the representation is absent).
// The secure store: secretSet(namespace, key, value, accessibility),
// secretGet(namespace, key) -> byte[] or NULL, and delete. They are separate
// wrappers rather than one shape because the return differs: two booleans, or
// bytes. All three run the Android Keystore on the Java side, which may take
// milliseconds on the first call — an unwrap — so their Go callers treat them
// like any other native call and do not hold a lock across them.
static jint mygo_platform_call_secret_set(jmethodID mid, const char *ns, int ns_len, const char *key, int key_len, const char *value, int value_len, int accessibility) {
	JNIEnv *env = mygo_platform_env();
	if (env == NULL || mygo_platform_obj == NULL || mid == NULL) {
		return -1;
	}
	jbyteArray a = mygo_platform_arg(env, ns, ns_len);
	jbyteArray b = mygo_platform_arg(env, key, key_len);
	jbyteArray c = mygo_platform_arg(env, value, value_len);
	if (a == NULL || b == NULL || c == NULL) {
		if (a != NULL) (*env)->DeleteLocalRef(env, a);
		if (b != NULL) (*env)->DeleteLocalRef(env, b);
		if (c != NULL) (*env)->DeleteLocalRef(env, c);
		return -1;
	}
	jboolean ok = (*env)->CallBooleanMethod(env, mygo_platform_obj, mid, a, b, c, (jint)accessibility);
	(*env)->DeleteLocalRef(env, a);
	(*env)->DeleteLocalRef(env, b);
	(*env)->DeleteLocalRef(env, c);
	if (mygo_platform_exception(env)) {
		return -1;
	}
	return ok == JNI_TRUE ? 0 : 1;
}
static char *mygo_platform_call_secret_get(jmethodID mid, const char *ns, int ns_len, const char *key, int key_len, int *out_len) {
	*out_len = 0;
	JNIEnv *env = mygo_platform_env();
	if (env == NULL || mygo_platform_obj == NULL || mid == NULL) {
		return NULL;
	}
	jbyteArray a = mygo_platform_arg(env, ns, ns_len);
	jbyteArray b = mygo_platform_arg(env, key, key_len);
	if (a == NULL || b == NULL) {
		if (a != NULL) (*env)->DeleteLocalRef(env, a);
		if (b != NULL) (*env)->DeleteLocalRef(env, b);
		return NULL;
	}
	jbyteArray r = (jbyteArray)(*env)->CallObjectMethod(env, mygo_platform_obj, mid, a, b);
	(*env)->DeleteLocalRef(env, a);
	(*env)->DeleteLocalRef(env, b);
	if (mygo_platform_exception(env)) {
		return NULL;
	}
	char *out = mygo_platform_grab(env, r, out_len);
	if (r != NULL) {
		(*env)->DeleteLocalRef(env, r);
	}
	return out;
}
static jint mygo_platform_call_secret_delete(jmethodID mid, const char *ns, int ns_len, const char *key, int key_len) {
	JNIEnv *env = mygo_platform_env();
	if (env == NULL || mygo_platform_obj == NULL || mid == NULL) {
		return -1;
	}
	jbyteArray a = mygo_platform_arg(env, ns, ns_len);
	jbyteArray b = mygo_platform_arg(env, key, key_len);
	if (a == NULL || b == NULL) {
		if (a != NULL) (*env)->DeleteLocalRef(env, a);
		if (b != NULL) (*env)->DeleteLocalRef(env, b);
		return -1;
	}
	jboolean ok = (*env)->CallBooleanMethod(env, mygo_platform_obj, mid, a, b);
	(*env)->DeleteLocalRef(env, a);
	(*env)->DeleteLocalRef(env, b);
	if (mygo_platform_exception(env)) {
		return -1;
	}
	return ok == JNI_TRUE ? 0 : 1;
}
static jint mygo_platform_call_two_bool(jmethodID mid, int has_one, const char *one, int one_len,
	int has_two, const char *two, int two_len) {
	JNIEnv *env = mygo_platform_env();
	if (env == NULL || mygo_platform_obj == NULL || mid == NULL) {
		return -1;
	}
	jbyteArray a = has_one ? mygo_platform_arg(env, one, one_len) : NULL;
	jbyteArray b = has_two ? mygo_platform_arg(env, two, two_len) : NULL;
	if ((has_one && a == NULL) || (has_two && b == NULL)) {
		if (a != NULL) {
			(*env)->DeleteLocalRef(env, a);
		}
		if (b != NULL) {
			(*env)->DeleteLocalRef(env, b);
		}
		return -1;
	}
	jboolean ok = (*env)->CallBooleanMethod(env, mygo_platform_obj, mid, a, b);
	if (a != NULL) {
		(*env)->DeleteLocalRef(env, a);
	}
	if (b != NULL) {
		(*env)->DeleteLocalRef(env, b);
	}
	if (mygo_platform_exception(env)) {
		return -1;
	}
	return ok == JNI_TRUE ? 1 : 0;
}

// mygo_platform_call_notify calls
// postNotification(byte[] id, byte[] title, byte[] body, boolean silent).
static jint mygo_platform_call_notify(const char *id, int id_len, const char *title, int title_len,
	const char *body, int body_len, int silent) {
	JNIEnv *env = mygo_platform_env();
	if (env == NULL || mygo_platform_obj == NULL || mygo_mid_notify == NULL) {
		return -1;
	}
	jbyteArray i = mygo_platform_arg(env, id, id_len);
	jbyteArray t = mygo_platform_arg(env, title, title_len);
	jbyteArray b = mygo_platform_arg(env, body, body_len);
	if (i == NULL || t == NULL || b == NULL) {
		if (i != NULL) {
			(*env)->DeleteLocalRef(env, i);
		}
		if (t != NULL) {
			(*env)->DeleteLocalRef(env, t);
		}
		if (b != NULL) {
			(*env)->DeleteLocalRef(env, b);
		}
		return -1;
	}
	jint rc = (*env)->CallIntMethod(env, mygo_platform_obj, mygo_mid_notify, i, t, b,
		silent ? JNI_TRUE : JNI_FALSE);
	(*env)->DeleteLocalRef(env, i);
	(*env)->DeleteLocalRef(env, t);
	(*env)->DeleteLocalRef(env, b);
	if (mygo_platform_exception(env)) {
		return -1;
	}
	return rc;
}

// mygo_platform_attach records the JavaVM and the MygoPlatform object and looks
// up every method this file calls. It returns 0 on success, -1 when nothing
// could be recorded and -2 when a method is missing (a rename on the Java side).
static jint mygo_platform_attach(JNIEnv *env, jobject thiz) {
	if (env == NULL || thiz == NULL) {
		return -1;
	}
	if (mygo_platform_vm == NULL) {
		if ((*env)->GetJavaVM(env, &mygo_platform_vm) != JNI_OK) {
			return -1;
		}
	}
	if (mygo_platform_obj == NULL) {
		mygo_platform_obj = (*env)->NewGlobalRef(env, thiz);
		if (mygo_platform_obj == NULL) {
			return -1;
		}
	}
	jclass cls = (*env)->GetObjectClass(env, mygo_platform_obj);
	if (cls == NULL) {
		mygo_platform_exception(env);
		return -1;
	}
	struct {
		const char *name;
		const char *sig;
		jmethodID *slot;
	} lookups[] = {
		{"readClipboardText", "()[B", &mygo_mid_read_text},
		{"readClipboardHtml", "()[B", &mygo_mid_read_html},
		{"clipboardMimeTypes", "()[B", &mygo_mid_mimes},
		{"writeClipboard", "([B[B)Z", &mygo_mid_write},
		{"clearClipboard", "()Z", &mygo_mid_clear},
		{"openExternalUrl", "([B)I", &mygo_mid_open},
		{"notificationsAllowed", "()Z", &mygo_mid_notify_allowed},
		{"postNotification", "([B[B[BZ)I", &mygo_mid_notify},
		{"cancelNotification", "([B)Z", &mygo_mid_cancel},
		{"cancelAllNotifications", "()Z", &mygo_mid_cancel_all},
		{"announce", "([B)V", &mygo_mid_announce},
		{"setAccessibilityTree", "([B)V", &mygo_mid_set_tree},
		{"secretSet", "([B[B[BI)Z", &mygo_mid_secret_set},
		{"secretGet", "([B[B)[B", &mygo_mid_secret_get},
		{"secretDelete", "([B[B)Z", &mygo_mid_secret_delete},
		{"startScan", "(J[B)Z", &mygo_mid_scan},
		{"cancelScan", "(J)Z", &mygo_mid_scan_cancel},
		{"takeScanResult", "()Ljava/lang/String;", &mygo_mid_take_scan},
		{"exitApp", "()V", &mygo_mid_exit},
	};
	int count = (int)(sizeof(lookups) / sizeof(lookups[0]));
	for (int n = 0; n < count; n++) {
		*lookups[n].slot = (*env)->GetMethodID(env, cls, lookups[n].name, lookups[n].sig);
		if (*lookups[n].slot == NULL) {
			mygo_platform_exception(env);
			(*env)->DeleteLocalRef(env, cls);
			return -2;
		}
	}
	(*env)->DeleteLocalRef(env, cls);
	return 0;
}

// The wrappers the Go side calls. Each one returns -1 when the platform is not
// attached, so that the Go side can tell "Java said no" from "there is no Java
// side to ask".
static char *mygo_platform_read_text(int *out_len) { return mygo_platform_call_bytes(mygo_mid_read_text, out_len); }
static char *mygo_platform_read_html(int *out_len) { return mygo_platform_call_bytes(mygo_mid_read_html, out_len); }
static char *mygo_platform_mimes(int *out_len) { return mygo_platform_call_bytes(mygo_mid_mimes, out_len); }
static jint mygo_platform_write(int has_plain, const char *plain, int plain_len, int has_html, const char *html, int html_len) {
	return mygo_platform_call_two_bool(mygo_mid_write, has_plain, plain, plain_len, has_html, html, html_len);
}
static jint mygo_platform_clear(void) {
	return mygo_platform_call_bool(mygo_mid_clear, NULL, 0);
}
// mygo_platform_exit calls exitApp(), which takes nothing, so it is not one of
// the byte[] shapes above.
static jint mygo_platform_exit(void) {
	JNIEnv *env = mygo_platform_env();
	if (env == NULL || mygo_platform_obj == NULL || mygo_mid_exit == NULL) {
		return -1;
	}
	(*env)->CallVoidMethod(env, mygo_platform_obj, mygo_mid_exit);
	if (mygo_platform_exception(env)) {
		return -1;
	}
	return 0;
}
static jint mygo_platform_open(const char *url, int url_len) {
	return mygo_platform_call_int(mygo_mid_open, url, url_len);
}
static jint mygo_platform_notify_allowed(void) {
	return mygo_platform_call_bool(mygo_mid_notify_allowed, NULL, 0);
}
static jint mygo_platform_notify(const char *id, int id_len, const char *title, int title_len, const char *body, int body_len, int silent) {
	return mygo_platform_call_notify(id, id_len, title, title_len, body, body_len, silent);
}
static jint mygo_platform_cancel(const char *id, int id_len) {
	return mygo_platform_call_bool(mygo_mid_cancel, id, id_len);
}
static jint mygo_platform_cancel_all(void) {
	return mygo_platform_call_bool(mygo_mid_cancel_all, NULL, 0);
}
static jint mygo_platform_announce(const char *text, int text_len) {
	return mygo_platform_call_void(mygo_mid_announce, text, text_len);
}
static jint mygo_platform_set_tree(const char *json, int json_len) {
	return mygo_platform_call_void(mygo_mid_set_tree, json, json_len);
}
static jint mygo_platform_secret_set(const char *ns, int ns_len, const char *key, int key_len, const char *value, int value_len, int accessibility) {
	return mygo_platform_call_secret_set(mygo_mid_secret_set, ns, ns_len, key, key_len, value, value_len, accessibility);
}
static char *mygo_platform_secret_get(const char *ns, int ns_len, const char *key, int key_len, int *out_len) {
	return mygo_platform_call_secret_get(mygo_mid_secret_get, ns, ns_len, key, key_len, out_len);
}
static jint mygo_platform_secret_delete(const char *ns, int ns_len, const char *key, int key_len) {
	return mygo_platform_call_secret_delete(mygo_mid_secret_delete, ns, ns_len, key, key_len);
}
// startScan(long token, byte[] prompt) opens the picker; the answer comes back
// through Java_org_mygo_MygoPlatform_nativeScanResult, not as a return value.
static jint mygo_platform_scan(jlong token, const char *prompt, int prompt_len) {
	JNIEnv *env = mygo_platform_env();
	if (env == NULL || mygo_platform_obj == NULL || mygo_mid_scan == NULL) {
		return -1;
	}
	jbyteArray p = mygo_platform_arg(env, prompt, prompt_len);
	if (p == NULL) {
		return -1;
	}
	jboolean ok = (*env)->CallBooleanMethod(env, mygo_platform_obj, mygo_mid_scan, token, p);
	(*env)->DeleteLocalRef(env, p);
	if (mygo_platform_exception(env)) {
		return -1;
	}
	return ok == JNI_TRUE ? 0 : 1;
}
// mygo_platform_take_scan_result asks the Java side for the pending scan
// result and copies it out as a malloc'd C string. It returns NULL when there
// is nothing to report; *code is then untouched.
static const char *mygo_platform_take_scan_result(void) {
	JNIEnv *env = mygo_platform_env();
	if (env == NULL || mygo_platform_obj == NULL || mygo_mid_take_scan == NULL) {
		return NULL;
	}
	jstring js = (jstring)(*env)->CallObjectMethod(env, mygo_platform_obj, mygo_mid_take_scan);
	if (mygo_platform_exception(env) || js == NULL) {
		return NULL;
	}
	const char *utf = (*env)->GetStringUTFChars(env, js, NULL);
	if (utf == NULL) {
		(*env)->DeleteLocalRef(env, js);
		return NULL;
	}
	char *out = strdup(utf);
	(*env)->ReleaseStringUTFChars(env, js, utf);
	(*env)->DeleteLocalRef(env, js);
	// 编码格式："<code>\n<token>\n<text>"，三样一次带走。
	return out;
}

static jint mygo_platform_scan_cancel(jlong token) {
	JNIEnv *env = mygo_platform_env();
	if (env == NULL || mygo_platform_obj == NULL || mygo_mid_scan_cancel == NULL) {
		return -1;
	}
	jboolean ok = (*env)->CallBooleanMethod(env, mygo_platform_obj, mygo_mid_scan_cancel, token);
	if (mygo_platform_exception(env)) {
		return -1;
	}
	return ok == JNI_TRUE ? 0 : 1;
}
*/
import "C"

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/egoist/mygo/internal/android/backend"
	"github.com/egoist/mygo/internal/platform"
)

// init installs the bridge before Java has attached.
//
// The bridge is installed here rather than in nativeAttach so that the platform
// features exist from the moment the library is loaded: a call that arrives
// before install() then reports backend.ErrPlatformBridge — "the Java side did
// not attach" — instead of the package answering from the unsupported backend's
// silence, which is a failure with no explanation.
func init() {
	backend.SetPlatformBridge(androidJNI{})
}

// androidJNI is the Android implementation of backend.PlatformBridge: every
// method is a JNI call to org.mygo.MygoPlatform.
//
// The Java method names and signatures are the table inside
// mygo_platform_attach, and they are the same strings MygoPlatform.kt declares.
// javap over the compiled class is what checks the two against each other (see
// T34.md).
type androidJNI struct{}

// attached reports whether nativeAttach has run and every method was found.
func attached() bool { return C.mygo_platform_ready() != 0 }

// ClipboardRead reads the clip's text, its markup and the MIME types it
// advertises.
func (androidJNI) ClipboardRead() (backend.ClipboardPayload, []string, error) {
	if !attached() {
		return backend.ClipboardPayload{}, nil, backend.ErrPlatformBridge
	}
	var p backend.ClipboardPayload
	var n C.int
	if s, ok := takeJavaBytes(C.mygo_platform_read_text(&n), n); ok {
		p.Plain, p.HasPlain = s, true
	}
	if s, ok := takeJavaBytes(C.mygo_platform_read_html(&n), n); ok {
		p.HTML, p.HasHTML = s, true
	}
	mimes, ok := takeJavaBytes(C.mygo_platform_mimes(&n), n)
	if !ok {
		return p, nil, nil
	}
	// One MIME type per line: the Java side joins them because a MIME type
	// cannot contain a newline, and a byte[] of lines keeps this side of the
	// JNI to one shape (see mygo_platform_bytes).
	var out []string
	for _, line := range strings.Split(mimes, "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return p, out, nil
}

// ClipboardWrite replaces the clipboard with the given representations.
func (androidJNI) ClipboardWrite(p backend.ClipboardPayload) error {
	if !attached() {
		return backend.ErrPlatformBridge
	}
	plain, plainLen := cBytes(p.Plain)
	defer cFree(plain)
	html, htmlLen := cBytes(p.HTML)
	defer cFree(html)
	rc := C.mygo_platform_write(
		boolToC(p.HasPlain), plain, plainLen,
		boolToC(p.HasHTML), html, htmlLen)
	if rc != 0 {
		return errClipboardWrite
	}
	return nil
}

// ClipboardClear empties the clipboard.
func (androidJNI) ClipboardClear() error {
	if !attached() {
		return backend.ErrPlatformBridge
	}
	if C.mygo_platform_clear() < 0 {
		return backend.ErrPlatformBridge
	}
	return nil
}

// OpenExternalURL hands the URL to the system.
func (androidJNI) OpenExternalURL(url string) error {
	if !attached() {
		return backend.ErrPlatformBridge
	}
	c, n := cBytes(url)
	defer cFree(c)
	switch rc := C.mygo_platform_open(c, n); rc {
	case 0:
		return nil
	case 1:
		// The Java side found no activity for the intent, which is what a
		// device with no browser (or no mail client) looks like.
		return errNoActivity
	default:
		return backend.ErrPlatformBridge
	}
}

// NotificationsAllowed reports whether the user allows notifications.
//
// Before Java attaches this is false: nothing can be posted, and false is the
// honest answer for "not now".
func (androidJNI) NotificationsAllowed() bool {
	return C.mygo_platform_notify_allowed() == 1
}

// PostNotification posts one notification.
func (androidJNI) PostNotification(n *platform.Notification) error {
	if !attached() {
		return backend.ErrPlatformBridge
	}
	if n == nil {
		return errNilNotification
	}
	id, idLen := cBytes(n.ID)
	defer cFree(id)
	title, titleLen := cBytes(n.Title)
	defer cFree(title)
	// The subtitle has nowhere of its own to go on Android (a notification
	// has a title and a body), so it leads the body, the way the Linux
	// backend's D-Bus body does.
	body, bodyLen := cBytes(joinSubtitle(n.Subtitle, n.Body))
	defer cFree(body)
	switch rc := C.mygo_platform_notify(id, idLen, title, titleLen, body, bodyLen, boolToC(n.Silent)); rc {
	case 0:
		return nil
	case 1:
		return platform.ErrNotificationsDenied
	case 2:
		return errPostNotification
	default:
		return backend.ErrPlatformBridge
	}
}

// CancelNotification removes one notification.
func (androidJNI) CancelNotification(id string) error {
	if !attached() {
		return backend.ErrPlatformBridge
	}
	c, n := cBytes(id)
	defer cFree(c)
	if C.mygo_platform_cancel(c, n) < 0 {
		return backend.ErrPlatformBridge
	}
	return nil
}

// CancelAllNotifications removes the app's notifications.
func (androidJNI) CancelAllNotifications() error {
	if !attached() {
		return backend.ErrPlatformBridge
	}
	if C.mygo_platform_cancel_all() < 0 {
		return backend.ErrPlatformBridge
	}
	return nil
}

// AnnounceAs puts text through the platform's accessibility announcement.
func (androidJNI) AnnounceAs(text string) error {
	if !attached() {
		return backend.ErrPlatformBridge
	}
	c, n := cBytes(text)
	defer cFree(c)
	if C.mygo_platform_announce(c, n) < 0 {
		return backend.ErrPlatformBridge
	}
	return nil
}

// SetAccessibilityTreeJSON gives the Java side the tree its node provider
// serves.
func (androidJNI) SetAccessibilityTreeJSON(json string) error {
	if !attached() {
		return backend.ErrPlatformBridge
	}
	c, n := cBytes(json)
	defer cFree(c)
	if C.mygo_platform_set_tree(c, n) < 0 {
		return backend.ErrPlatformBridge
	}
	return nil
}

// SecretSet stores value under namespace/key in the Keystore.
func (androidJNI) SecretSet(namespace, key string, value []byte, accessibility string) error {
	if !attached() {
		return backend.ErrPlatformBridge
	}
	ns, nsLen := cBytes(namespace)
	defer cFree(ns)
	k, kLen := cBytes(key)
	defer cFree(k)
	v, vLen := cBytes(string(value))
	defer cFree(v)
	// 1 is "the Java side stored it already, or refused"; both are final.
	switch C.mygo_platform_secret_set(ns, nsLen, k, kLen, v, vLen, C.int(accessibilityCode(accessibility))) {
	case -1:
		return backend.ErrPlatformBridge
	case 0:
		return nil
	}
	return errSecretStore
}

// SecretGet reads value. A missing one is platform.ErrSecretNotFound.
func (androidJNI) SecretGet(namespace, key string) ([]byte, error) {
	if !attached() {
		return nil, backend.ErrPlatformBridge
	}
	ns, nsLen := cBytes(namespace)
	defer cFree(ns)
	k, kLen := cBytes(key)
	defer cFree(k)
	var n C.int
	s, ok := takeJavaBytes(C.mygo_platform_secret_get(ns, nsLen, k, kLen, &n), n)
	if !ok {
		// The Java side returns null for both "no such secret" and a failed
		// unwrap; attaching is what tells them apart here, and the platform
		// is attached by the check above, so it is the store's answer.
		return nil, platform.ErrSecretNotFound
	}
	return []byte(s), nil
}

// SecretDelete removes value; removing one that is not there is not an error.
func (androidJNI) SecretDelete(namespace, key string) error {
	if !attached() {
		return backend.ErrPlatformBridge
	}
	ns, nsLen := cBytes(namespace)
	defer cFree(ns)
	k, kLen := cBytes(key)
	defer cFree(k)
	switch C.mygo_platform_secret_delete(ns, nsLen, k, kLen) {
	case -1:
		return backend.ErrPlatformBridge
	case 0:
		return nil
	}
	return errSecretStore
}

// ScanCode opens the system's image picker through the bridge.
func (androidJNI) ScanCode(token uint64, prompt string) error {
	if !attached() {
		return backend.ErrPlatformBridge
	}
	p, n := cBytes(prompt)
	defer cFree(p)
	switch C.mygo_platform_scan(C.jlong(token), p, n) {
	case -1:
		return backend.ErrPlatformBridge
	case 0:
		go collectScanResults()
		return nil
	}
	return backend.ErrScanUnavailable
}

// collectScanResults polls the Java side until it hands over an answer.
//
// The bridge is one-way — Go calls Java through jmethodID, and nothing on the
// Java side calls back into Go — so a scan cannot arrive as a JNI entry point.
// Polling is the shape that fits: the wait is a person holding a camera or
// picking a photo (seconds), and one getString every 150ms costs nothing.
// It stops as soon as an answer is taken, or after a minute, which is longer
// than anyone leaves a camera up.
func collectScanResults() {
	const interval = 150 * time.Millisecond
	deadline := time.Now().Add(time.Minute)
	for time.Now().Before(deadline) {
		if code, token, text, ok := takeScanResult(); ok {
			backend.ScanResult(token, text, code)
			return
		}
		time.Sleep(interval)
	}
}

// takeScanResult pulls one pending result from the Java side, if there is one.
// The encoding is "<code>\n<token>\n<text>": code first so a malformed tail
// still says whether the scan failed as canceled or as a decode miss.
func takeScanResult() (code int32, token uint64, text string, ok bool) {
	raw := C.mygo_platform_take_scan_result()
	if raw == nil {
		return 0, 0, "", false
	}
	defer C.free(unsafe.Pointer(raw))
	parts := strings.SplitN(C.GoString(raw), "\n", 3)
	if len(parts) < 2 {
		return 0, 0, "", false
	}
	code64, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil {
		return 0, 0, "", false
	}
	n, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return 0, 0, "", false
	}
	if len(parts) == 3 {
		text = parts[2]
	}
	return int32(code64), n, text, true
}

// ScanCancel takes the picker down, if it is still up.
func (androidJNI) ScanCancel(token uint64) error {
	if !attached() {
		return backend.ErrPlatformBridge
	}
	if C.mygo_platform_scan_cancel(C.jlong(token)) < 0 {
		return backend.ErrPlatformBridge
	}
	return nil
}

// accessibilityCode maps mygo's accessibility to the int the Java side takes.
// The Keystore has no per-item lifetime — an entry is readable whenever the
// device is unlocked — so only the after-first-unlock value has a meaning here
// (readable from the first unlock after boot, which is what the Java side
// implements by never holding a key that needs the screen on).
func accessibilityCode(s string) int {
	if s == "after-first-unlock" {
		return 1
	}
	return 0
}

// The errors this file adds. None of them is platform.ErrNotificationsDenied,
// which says the user said no: these say the call or the platform failed.
var (
	errClipboardWrite   = errors.New("mygo: Android refused the clipboard write")
	errNoActivity       = errors.New("mygo: no app on this device handles that URL")
	errNilNotification  = errors.New("mygo: nil notification")
	errPostNotification = errors.New("mygo: Android refused the notification")
	errSecretStore      = errors.New("mygo: Android refused the secure store write")
)

// joinSubtitle is the body a notification shows when it carries a subtitle.
func joinSubtitle(subtitle, body string) string {
	if subtitle == "" {
		return body
	}
	if body == "" {
		return subtitle
	}
	return subtitle + "\n" + body
}

// takeJavaBytes copies what a C wrapper returned into a Go string. ok is false
// when the Java method reported no value (an absent clipboard representation,
// say), which is not the same as an empty one: mygo_platform_grab returns a
// 1-byte buffer for an empty array precisely so that the two stay apart.
func takeJavaBytes(p *C.char, n C.int) (string, bool) {
	if p == nil {
		return "", false
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoStringN(p, n), true
}

// cBytes makes a malloc'd copy of s for the C side, with its length. The pointer
// is nil for the empty string — the C side treats an empty argument as present
// and empty (mygo_platform_arg), so nothing is lost — and every caller frees it.
func cBytes(s string) (*C.char, C.int) {
	if s == "" {
		return nil, 0
	}
	return (*C.char)(C.CBytes([]byte(s))), C.int(len(s))
}

// cFree releases what cBytes allocated.
func cFree(p *C.char) {
	if p != nil {
		C.free(unsafe.Pointer(p))
	}
}

// boolToC turns a Go bool into the int the C side takes.
func boolToC(v bool) C.int {
	if v {
		return 1
	}
	return 0
}

// javaBytes copies the bytes of a byte[] argument of a //export function into a
// Go string. A null array is the empty string: a value the Java side sends is
// always an array (empty when the value is empty), so null means the caller
// passed nothing at all.
func javaBytes(env *C.JNIEnv, data C.jbyteArray) string {
	var buf *C.char
	n := C.mygo_platform_java_bytes(env, data, &buf)
	if buf == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(buf))
	return C.GoStringN(buf, n)
}

// The //export functions below are what Java calls into this library
// (org.mygo.MygoPlatform's external methods). Their names are the JNI symbols
// Java binds to, so they are spelled out by MygoPlatform.kt and must not
// change: Java_org_mygo_MygoPlatform_<method>.
//
// MygoPlatform is a Kotlin object, so its external members are *instance*
// methods of the singleton class: the second argument is the object, not a
// jclass. That is what keeps the symbol exactly Java_org_mygo_MygoPlatform_x —
// a companion object with @JvmStatic would have moved the native method to
// MygoPlatform$Companion and renamed the symbol with it.

// Java_org_mygo_MygoPlatform_nativeAttach is MygoPlatform.nativeAttach: the
// Activity's install() calls it once, and it is what makes the JavaVM and the
// MygoPlatform object reachable from Go.
//
//export Java_org_mygo_MygoPlatform_nativeAttach
func Java_org_mygo_MygoPlatform_nativeAttach(env *C.JNIEnv, thiz C.jobject) {
	rc := C.mygo_platform_attach(env, thiz)
	switch rc {
	case 0:
		androidLog("MygoPlatform.attach ok")
	case -2:
		// A method of MygoPlatform was renamed: the platform features are
		// off until both sides are in step. This is the one failure the Java
		// side cannot see coming, so it is loud.
		androidLog("MygoPlatform.attach FAILED: a method this library calls is missing (renamed?)")
	default:
		androidLog("MygoPlatform.attach failed rc=%d", int(rc))
	}
}

// Java_org_mygo_MygoPlatform_nativeNotificationClicked is
// MygoPlatform.nativeNotificationClicked: the user tapped a notification, and
// id is the identifier the app gave it.
//
//export Java_org_mygo_MygoPlatform_nativeNotificationClicked
func Java_org_mygo_MygoPlatform_nativeNotificationClicked(env *C.JNIEnv, thiz C.jobject, id C.jbyteArray) {
	clicked := javaBytes(env, id)
	if !backend.DispatchNotificationClicked(clicked) {
		// No sink: the tap is not lost silently, it is reported as
		// undelivered — the state of the wiring, visible in logcat.
		androidLog("notification %q tapped with no click sink installed", clicked)
	}
}

// Java_org_mygo_MygoPlatform_nativeAccessibilityAction is
// MygoPlatform.nativeAccessibilityAction: TalkBack performed an action on the
// element id, named as the Go side names it ("press", "focus", "setvalue").
//
//export Java_org_mygo_MygoPlatform_nativeAccessibilityAction
func Java_org_mygo_MygoPlatform_nativeAccessibilityAction(env *C.JNIEnv, thiz C.jobject, id C.jlong, action C.jbyteArray) {
	name := javaBytes(env, action)
	kind, ok := backend.ParseAccessActionKind(name)
	if !ok {
		androidLog("accessibility: unknown action %q on element %d", name, int64(id))
		return
	}
	if !backend.DispatchAccessAction(uint64(id), kind) {
		androidLog("accessibility: %s on element %d with no action sink installed", name, int64(id))
	}
}

// exitHost ends the app once its main has returned. The backend is a
// process-wide singleton whose quit cannot be undone, and the app's main runs
// once per process (see start.go), so a process that outlived its app would
// show a dead Activity the next time the user opens it from Recents. Asking
// Java to finish the task and end the process makes the next launch a cold
// one, as gomobile does. Without the Java side there is nothing to finish, and
// the process just exits.
func exitHost() {
	androidLog("start: ending the process")
	if attached() && C.mygo_platform_exit() == 0 {
		return
	}
	os.Exit(0)
}
