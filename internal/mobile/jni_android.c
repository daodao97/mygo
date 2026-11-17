// The JNI side of the Android host: dev.mygo.MyGoNative's native methods
// call into Go, and Go's calls reach dev.mygo.Host on the UI thread. Go
// wakes the UI thread through a pipe its looper watches, so that the
// host's calls never attach a thread of Go's to the JVM. Native modules
// do: Go calls their Kotlin (dev.mygo.Native) on the calling thread, as
// it calls a Swift library's functions elsewhere.

#define _GNU_SOURCE
#include <jni.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <fcntl.h>
#include <android/log.h>
#include <android/looper.h>
#include <android/native_window_jni.h>
#include "_cgo_export.h"

static JavaVM *vm;
static jclass host;
static jmethodID host_call;
static int wake[2] = {-1, -1};

// dev.mygo.Native, the runtime of native modules' Kotlin.
static jclass native;
static jmethodID native_init, native_invoke, native_direct, native_op;

JNIEXPORT jint JNI_OnLoad(JavaVM *v, void *reserved) {
	vm = v;
	JNIEnv *env;
	if ((*v)->GetEnv(v, (void **)&env, JNI_VERSION_1_6) != JNI_OK) return JNI_ERR;
	jclass c = (*env)->FindClass(env, "dev/mygo/Host");
	if (c == NULL) return JNI_ERR;
	host = (*env)->NewGlobalRef(env, c);
	host_call = (*env)->GetStaticMethodID(env, host, "call", "([B)[B");
	// Found here, with the app's class loader, which a thread of Go's that
	// attaches later does not have.
	jclass n = (*env)->FindClass(env, "dev/mygo/Native");
	if (n == NULL) {
		(*env)->ExceptionClear(env);
	} else {
		native = (*env)->NewGlobalRef(env, n);
		native_init = (*env)->GetStaticMethodID(env, native, "init", "(Ljava/lang/String;Ljava/lang/String;J)Ljava/lang/String;");
		native_invoke = (*env)->GetStaticMethodID(env, native, "invoke", "(Ljava/lang/String;Ljava/lang/String;J[BJ)[B");
		native_direct = (*env)->GetStaticMethodID(env, native, "direct", "(Ljava/lang/String;Ljava/lang/String;J[J)J");
		native_op = (*env)->GetStaticMethodID(env, native, "op", "(Ljava/lang/String;Ljava/lang/String;JJ[B)V");
		if ((*env)->ExceptionCheck(env)) {
			(*env)->ExceptionClear(env);
			native = NULL;
		}
	}
	return JNI_VERSION_1_6;
}

static JNIEnv *ui_env(void) {
	JNIEnv *env = NULL;
	(*vm)->GetEnv(vm, (void **)&env, JNI_VERSION_1_6);
	return env;
}

void mygo_android_call(uint8_t *msg, size_t len, uint8_t **reply, size_t *reply_len) {
	JNIEnv *env = ui_env();
	*reply = NULL;
	*reply_len = 0;
	jbyteArray a = (*env)->NewByteArray(env, (jsize)len);
	(*env)->SetByteArrayRegion(env, a, 0, (jsize)len, (const jbyte *)msg);
	jbyteArray r = (jbyteArray)(*env)->CallStaticObjectMethod(env, host, host_call, a);
	(*env)->DeleteLocalRef(env, a);
	if ((*env)->ExceptionCheck(env)) {
		(*env)->ExceptionDescribe(env);
		(*env)->ExceptionClear(env);
		return;
	}
	if (r == NULL) return;
	jsize n = (*env)->GetArrayLength(env, r);
	*reply = malloc(n > 0 ? n : 1);
	(*env)->GetByteArrayRegion(env, r, 0, n, (jbyte *)*reply);
	*reply_len = n;
	(*env)->DeleteLocalRef(env, r);
}

// mygo_android_call_here calls the host on the thread of the caller, which
// joins the JVM the first time: the text of native UI, which measures and draws
// with its own objects, whatever the UI thread is doing.
void mygo_android_call_here(uint8_t *msg, size_t len, uint8_t **reply, size_t *reply_len) {
	JNIEnv *env = NULL;
	*reply = NULL;
	*reply_len = 0;
	if ((*vm)->GetEnv(vm, (void **)&env, JNI_VERSION_1_6) != JNI_OK && (*vm)->AttachCurrentThreadAsDaemon(vm, &env, NULL) != JNI_OK) {
		return;
	}
	mygo_android_call(msg, len, reply, reply_len);
}

void mygo_android_post(void) {
	char c = 1;
	write(wake[1], &c, 1);
}

void mygo_android_log(int error, uint8_t *line, size_t len) {
	char *s = malloc(len + 1);
	memcpy(s, line, len);
	s[len] = 0;
	__android_log_write(error ? ANDROID_LOG_ERROR : ANDROID_LOG_INFO, "mygo", s);
	free(s);
}

static int on_wake(int fd, int events, void *data) {
	char buf[64];
	while (read(fd, buf, sizeof buf) > 0) {
	}
	mygo_drain();
	return 1;
}

JNIEXPORT void JNICALL Java_dev_mygo_MyGoNative_start(JNIEnv *env, jclass c) {
	if (wake[0] >= 0) return;
	pipe2(wake, O_NONBLOCK | O_CLOEXEC);
	ALooper_addFd(ALooper_forThread(), wake[0], ALOOPER_POLL_CALLBACK, ALOOPER_EVENT_INPUT, on_wake, NULL);
	mygo_start_android();
}

JNIEXPORT jlong JNICALL Java_dev_mygo_MyGoNative_nativeWindow(JNIEnv *env, jclass c, jobject surface) {
	return (jlong)(intptr_t)ANativeWindow_fromSurface(env, surface);
}

JNIEXPORT void JNICALL Java_dev_mygo_MyGoNative_releaseNativeWindow(JNIEnv *env, jclass c, jlong window) {
	if (window != 0) ANativeWindow_release((ANativeWindow *)(intptr_t)window);
}

JNIEXPORT jbyteArray JNICALL Java_dev_mygo_MyGoNative_event(JNIEnv *env, jclass c, jbyteArray msg, jboolean on_ui) {
	jsize n = (*env)->GetArrayLength(env, msg);
	jbyte *b = (*env)->GetByteArrayElements(env, msg, NULL);
	uint8_t *reply = NULL;
	size_t reply_len = 0;
	mygo_event((uint8_t *)b, (size_t)n, on_ui ? 1 : 0, &reply, &reply_len);
	(*env)->ReleaseByteArrayElements(env, msg, b, JNI_ABORT);
	if (reply == NULL) return NULL;
	jbyteArray r = (*env)->NewByteArray(env, (jsize)reply_len);
	(*env)->SetByteArrayRegion(env, r, 0, (jsize)reply_len, (const jbyte *)reply);
	free(reply);
	return r;
}

JNIEXPORT void JNICALL Java_dev_mygo_MyGoNative_drain(JNIEnv *env, jclass c) { mygo_drain(); }

// Native modules.

// any_env is the JNIEnv of this thread, which joins the JVM the first
// time: Go calls a module from any of its threads.
static JNIEnv *any_env(void) {
	JNIEnv *env = NULL;
	if ((*vm)->GetEnv(vm, (void **)&env, JNI_VERSION_1_6) != JNI_OK && (*vm)->AttachCurrentThreadAsDaemon(vm, &env, NULL) != JNI_OK) {
		return NULL;
	}
	return env;
}

// failed reports, and clears, an exception the runtime let through.
static int failed(JNIEnv *env) {
	if (!(*env)->ExceptionCheck(env)) return 0;
	(*env)->ExceptionDescribe(env);
	(*env)->ExceptionClear(env);
	return 1;
}

static jbyteArray bytes(JNIEnv *env, const uint8_t *b, size_t n) {
	jbyteArray a = (*env)->NewByteArray(env, (jsize)n);
	if (a != NULL && n > 0) (*env)->SetByteArrayRegion(env, a, 0, (jsize)n, (const jbyte *)b);
	return a;
}

// mygo_native_init loads a module's Kotlin, by the class of its generated
// object; it returns what went wrong, which the caller frees, or NULL.
char *mygo_native_init(const char *name, const char *cls, uint64_t base) {
	if (native == NULL) return strdup("the app's host has no dev.mygo.Native: build it with mygo build -platform android");
	JNIEnv *env = any_env();
	if (env == NULL) return strdup("the thread could not join the JVM");
	(*env)->PushLocalFrame(env, 8);
	jstring msg = (jstring)(*env)->CallStaticObjectMethod(env, native, native_init, (*env)->NewStringUTF(env, name), (*env)->NewStringUTF(env, cls), (jlong)base);
	char *r = NULL;
	if (failed(env)) {
		r = strdup("dev.mygo.Native.init threw");
	} else if (msg != NULL) {
		const char *c = (*env)->GetStringUTFChars(env, msg, NULL);
		r = strdup(c);
		(*env)->ReleaseStringUTFChars(env, msg, c);
	}
	(*env)->PopLocalFrame(env, NULL);
	return r;
}

// mygo_native_invoke calls a function of a module's Kotlin. Its answer,
// which the caller frees, is the status and then the data; it returns 0
// when the call threw past the runtime.
int mygo_native_invoke(const char *name, const char *sym, uint64_t h, const uint8_t *args, size_t n, uint64_t call, uint8_t **reply, size_t *reply_len) {
	*reply = NULL;
	*reply_len = 0;
	JNIEnv *env = any_env();
	if (env == NULL || native == NULL) return 0;
	(*env)->PushLocalFrame(env, 8);
	jbyteArray r = (jbyteArray)(*env)->CallStaticObjectMethod(env, native, native_invoke, (*env)->NewStringUTF(env, name), (*env)->NewStringUTF(env, sym), (jlong)h, bytes(env, args, n), (jlong)call);
	int ok = !failed(env);
	if (ok && r != NULL) {
		jsize k = (*env)->GetArrayLength(env, r);
		*reply = malloc(k > 0 ? k : 1);
		(*env)->GetByteArrayRegion(env, r, 0, k, (jbyte *)*reply);
		*reply_len = k;
	}
	(*env)->PopLocalFrame(env, NULL);
	return ok;
}

// mygo_native_direct calls a method of numbers.
uint64_t mygo_native_direct(const char *name, const char *sym, uint64_t h, const uint64_t *args, size_t n) {
	JNIEnv *env = any_env();
	if (env == NULL || native == NULL) return 0;
	(*env)->PushLocalFrame(env, 8);
	jlongArray a = (*env)->NewLongArray(env, (jsize)n);
	if (n > 0) (*env)->SetLongArrayRegion(env, a, 0, (jsize)n, (const jlong *)args);
	jlong r = (*env)->CallStaticLongMethod(env, native, native_direct, (*env)->NewStringUTF(env, name), (*env)->NewStringUTF(env, sym), (jlong)h, a);
	if (failed(env)) r = 0;
	(*env)->PopLocalFrame(env, NULL);
	return (uint64_t)r;
}

// mygo_native_op releases an object, cancels a call, asks a stream for its
// next value, or resumes a continuation of Kotlin's.
void mygo_native_op(const char *name, const char *op, uint64_t a, int64_t status, const uint8_t *data, size_t n) {
	JNIEnv *env = any_env();
	if (env == NULL || native == NULL) return;
	(*env)->PushLocalFrame(env, 8);
	(*env)->CallStaticVoidMethod(env, native, native_op, (*env)->NewStringUTF(env, name), (*env)->NewStringUTF(env, op), (jlong)a, (jlong)status, data != NULL ? bytes(env, data, n) : NULL);
	failed(env);
	(*env)->PopLocalFrame(env, NULL);
}

// What Kotlin tells Go: the callback every module shares. The answer of a
// question is its status and then its data, null when there is none.
JNIEXPORT jbyteArray JNICALL Java_dev_mygo_Native_send(JNIEnv *env, jclass c, jlong kind, jlong id, jlong status, jbyteArray data) {
	jsize n = data != NULL ? (*env)->GetArrayLength(env, data) : 0;
	jbyte *b = data != NULL ? (*env)->GetByteArrayElements(env, data, NULL) : NULL;
	uint8_t *reply = NULL;
	size_t reply_len = 0;
	int answered = mygo_native_handle((uint64_t)kind, (uint64_t)id, (int64_t)status, (uint8_t *)b, (size_t)n, &reply, &reply_len);
	if (b != NULL) (*env)->ReleaseByteArrayElements(env, data, b, JNI_ABORT);
	if (!answered) return NULL;
	jbyteArray r = bytes(env, reply, reply_len);
	free(reply);
	return r;
}

// The memory of a *native.Buffer, which Kotlin reads and writes.
JNIEXPORT jobject JNICALL Java_dev_mygo_Native_buffer(JNIEnv *env, jclass c, jlong p, jlong n) {
	return (*env)->NewDirectByteBuffer(env, (void *)(intptr_t)p, (jlong)n);
}
