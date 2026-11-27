package mygo

// JavaVM returns the process's JavaVM* on Android, for a C library that the
// app links into its Go code with cgo and that asks for the JVM, to attach
// its threads and call Java. It is 0 on other platforms, and may be 0
// before the app's main runs, as in init functions: call it from main or
// later.
//
// The value is for C: pass it to the library as a pointer and never
// dereference it in Go. A thread of C's gets a JNIEnv from the VM's
// AttachCurrentThread (or GetEnv, once attached) before calling JNI, and
// detaches before it exits; a JNIEnv is never shared between threads.
func JavaVM() uintptr { return javaVM() }

// AndroidContext returns a JNI global reference to the application's
// Context (android.content.Context) on Android, for a C library that asks
// for it beside the JavaVM. The reference stays valid for the life of the
// process: MyGo never deletes it, and C must not delete it either. It is 0
// on other platforms, and before the app's main runs.
//
// As with JavaVM, the value is for C, as a jobject used with a JNIEnv of
// an attached thread, never for Go to dereference.
func AndroidContext() uintptr { return androidContext() }
