//go:build android && cgo

package jni

/*
#include <jni.h>
#include <android/native_window.h>
#include <android/native_window_jni.h>
#include <android/log.h>
#include <stdlib.h>

// ANativeWindow_fromSurface lives in libandroid.
#cgo LDFLAGS: -landroid -llog

static void mygo_log(const char *s) {
	__android_log_print(ANDROID_LOG_INFO, "mygo", "%s", s);
}
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/egoist/mygo/internal/android/backend"
)

// theBackend is the library's single backend. The Java host of the app
// loads this library (System.loadLibrary("mygo")) and reaches it only
// through the //export functions below.
var theBackend = backend.New()

// androidLog writes one line to logcat under the tag "mygo". The entry
// points below call it so that a Java-side lifecycle event and the Go-side
// reaction to it both show up in `adb logcat -s mygo:V`; without it the only
// observable is the pixels, which says *that* something is wrong but not
// *which* callback stopped arriving.
//
// Safe from a cgo callback thread: C.CString allocates with malloc (C owns
// the pointer — no Go pointer crosses the boundary) and __android_log_print
// does not re-enter the Go runtime.
func androidLog(format string, args ...any) {
	cs := C.CString(fmt.Sprintf(format, args...))
	C.mygo_log(cs)
	C.free(unsafe.Pointer(cs))
}

// The //export names are the JNI symbols Java binds to
// org.mygo.Native's static native methods, so they are spelled out by the
// Java side and must not change: Java_org_mygo_Native_<method>.
//
// Every one of them runs on whatever thread Java called it on — usually the
// Activity's main thread — and so does no Go work itself: it hands the
// event to the backend, which queues it for its pump goroutine. Touching
// backend state (and, later, package ui) on this thread would enter the Go
// scheduler from a cgo callback and then wait on the main-thread machinery,
// which deadlocks.

// Java_org_mygo_Native_nativeOnSurfaceCreated is nativeOnSurfaceCreated.
//
// It takes the ANativeWindow* of the Surface the Activity's SurfaceView
// created with ANativeWindow_fromSurface and gives it to the backend, which
// owns it from here and releases it in nativeOnSurfaceDestroyed (after the
// EGLSurface is gone). Java never computes pointers: it passes the
// android.view.Surface.
//
//export Java_org_mygo_Native_nativeOnSurfaceCreated
func Java_org_mygo_Native_nativeOnSurfaceCreated(env *C.JNIEnv, clazz C.jclass, surface C.jobject) {
	// A null Surface gives a null window, which says "nothing to adopt"
	// just as well as checking the jobject would.
	window := C.ANativeWindow_fromSurface(env, surface)
	if window == nil {
		androidLog("onSurfaceCreated: null ANativeWindow (Java passed a released Surface?)")
		return
	}
	// %p, not %v: cgo types an opaque C struct as struct{}, so %v prints
	// "&{{{}}}" and the pointer value -- the one useful part -- is lost.
	androidLog("onSurfaceCreated window=%p", window)
	startApp()
	theBackend.OnSurfaceCreated(unsafe.Pointer(window))
}

// Java_org_mygo_Native_nativeOnSurfaceChanged is nativeOnSurfaceChanged:
// w and h are the surface's size in device pixels, dpi its density in dots
// per inch.
//
//export Java_org_mygo_Native_nativeOnSurfaceChanged
func Java_org_mygo_Native_nativeOnSurfaceChanged(env *C.JNIEnv, clazz C.jclass, w, h, dpi C.jint) {
	androidLog("onSurfaceChanged %dx%d dpi=%d", int(w), int(h), int(dpi))
	theBackend.OnSurfaceChanged(int(w), int(h), int(dpi))
}

// Java_org_mygo_Native_nativeOnSurfaceDestroyed is
// nativeOnSurfaceDestroyed. It reports that the Activity lost its Surface;
// the backend keeps the EGL context and drops only the EGLSurface, so the
// app does not flash a blank frame when it comes back (see egl.go).
//
//export Java_org_mygo_Native_nativeOnSurfaceDestroyed
func Java_org_mygo_Native_nativeOnSurfaceDestroyed(env *C.JNIEnv, clazz C.jclass) {
	androidLog("onSurfaceDestroyed")
	theBackend.OnSurfaceDestroyed()
}

// Java_org_mygo_Native_nativeOnTouch is nativeOnTouch: action is Android's
// MotionEvent action, x and y the coordinates in the surface's own pixels,
// eventTimeMs the event time that lets a gesture be reconstructed.
//
//export Java_org_mygo_Native_nativeOnTouch
func Java_org_mygo_Native_nativeOnTouch(env *C.JNIEnv, clazz C.jclass, action C.jint, x, y C.jfloat, eventTimeMs C.jlong) {
	androidLog("onTouch action=%d x=%.1f y=%.1f t=%d", int(action), float32(x), float32(y), int64(eventTimeMs))
	theBackend.OnTouch(int(action), float32(x), float32(y), int64(eventTimeMs))
}

// Java_org_mygo_Native_nativeOnKey is nativeOnKey: keyCode is Android's key
// code, action its ACTION_DOWN/ACTION_UP, metaState the modifier keys held.
//
//export Java_org_mygo_Native_nativeOnKey
func Java_org_mygo_Native_nativeOnKey(env *C.JNIEnv, clazz C.jclass, keyCode, action, metaState C.jint) {
	androidLog("onKey code=%d action=%d meta=%d", int(keyCode), int(action), int(metaState))
	theBackend.OnKey(int(keyCode), int(action), int(metaState))
}

// Java_org_mygo_Native_nativeOnSizeChanged is nativeOnSizeChanged: the
// view's size in device pixels, which Android reports separately from the
// surface's own change.
//
//export Java_org_mygo_Native_nativeOnSizeChanged
func Java_org_mygo_Native_nativeOnSizeChanged(env *C.JNIEnv, clazz C.jclass, w, h C.jint) {
	androidLog("onSizeChanged %dx%d", int(w), int(h))
	theBackend.OnSizeChanged(int(w), int(h))
}

// Java_org_mygo_Native_nativeOnSafeArea is nativeOnSafeArea: the room the
// device's chrome takes from the content, in DIPs — the status bar, a
// notch, the home indicator, a rounded corner.
//
// The host computes it (MygoView.reportSafeArea) because only Android
// knows: the heights differ per device, and the density it has to be
// divided by lives on the Java side. Go's layout works in DIPs and takes
// what it is given.
//
//export Java_org_mygo_Native_nativeOnSafeArea
func Java_org_mygo_Native_nativeOnSafeArea(env *C.JNIEnv, clazz C.jclass, top, right, bottom, left C.jint) {
	androidLog("onSafeArea top=%d right=%d bottom=%d left=%d", int(top), int(right), int(bottom), int(left))
	theBackend.OnSafeArea(int(top), int(right), int(bottom), int(left))
}

// Java_org_mygo_Native_nativeOnLifecycle is nativeOnLifecycle: kind is one
// of the Activity's lifecycle events, numbered as the backend's lifecycle.go
// lists them (pause, resume, low memory, stop, start).
//
//export Java_org_mygo_Native_nativeOnLifecycle
func Java_org_mygo_Native_nativeOnLifecycle(env *C.JNIEnv, clazz C.jclass, kind C.jint) {
	androidLog("onLifecycle kind=%d", int(kind))
	theBackend.OnLifecycle(int(kind))
}

// Java_org_mygo_Native_nativeOnTheme is nativeOnTheme: whether the system's
// night mode is on, reported at start and on every uiMode change.
//
//export Java_org_mygo_Native_nativeOnTheme
func Java_org_mygo_Native_nativeOnTheme(env *C.JNIEnv, clazz C.jclass, dark C.jboolean) {
	androidLog("onTheme dark=%v", dark != 0)
	theBackend.OnTheme(dark != 0)
}
