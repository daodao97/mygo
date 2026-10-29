//go:build android && cgo

package gl

/*
#include <EGL/egl.h>
*/
import "C"

import "unsafe"

// currentContext names the GL context the renderer's textures belong to, so
// that a renderer whose context changed can drop them rather than use
// another context's names (gl.go:533 and :794, present.go:30).
//
// libepoxy answers this through GDK on Linux; on Android it is
// eglGetCurrentContext, which is the context of the calling thread. The
// Android backend makes its EGL surface current on the pump goroutine, which
// is the thread gl.New runs on, so the answer is the renderer's context.
//
// EGL_NO_CONTEXT is (EGLContext)0, so a thread with nothing current gives 0,
// which is what a caller here tests against.
func currentContext() uintptr {
	return uintptr(unsafe.Pointer(C.eglGetCurrentContext()))
}

// contextVersion is what the renderer asks before it compiles shaders: an
// Android context is ES by definition, and load_android.go answers the
// version from GL_VERSION (esVersion).
func contextVersion() (version int32, es bool) { return esVersion(), true }

// hasExtension reports whether the current context has an extension.
func hasExtension(name string) bool { return hasGLExtension(name) }

// versionString describes the current context's version, for errors.
func versionString() string { return goString(glGetString(glVersion)) }

// rendererString names the current context's renderer.
func rendererString() string { return goString(glGetString(glRenderer)) }

// hasContext reports whether EGL has a context on the calling thread.
func hasContext() bool { return currentContext() != 0 }
