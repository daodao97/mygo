//go:build android && cgo

package gl

/*
#include <stdlib.h>
#include <EGL/egl.h>
#include <GLES3/gl3.h>
*/
import "C"

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"
)

// load takes the GL entry points from eglGetProcAddress, which is how
// Khronos says the functions of a GLES context are found on Android (the
// EGL spec makes it the one portable way; libepoxy, which load_linux.go
// uses, does not exist there).
//
// 🔴 The context must be current when this runs, and it must be the one the
// renderer will draw in: eglGetProcAddress answers per context for the
// extension functions, and the core names come from libGLESv3.so, which
// Android loads with the context. gl.New is called after eglMakeCurrent
// for exactly this reason.
//
// 🔴 A load with no current context is not a verdict, and caching one as if
// it were cost a device every GPU frame it ever drew.
//
// eglGetProcAddress answers nothing useful with no context current, so the
// first ask comes back with every name missing. loadOnce then latched that
// answer for the life of the process — and on this device the very first
// frame is asked for before any surface is usable (ColorOS hands the launch
// surfaceCreated → surfaceChanged → surfaceDestroyed inside ten
// milliseconds, and the EGLSurface create fails, see
// internal/android/backend/frame.go), so the latch was taken with no
// context and every later renderer, however good its surface, was refused
// with "OpenGL 0.0 is too old".
//
// So: latch success, and latch a failure only when a context was current and
// the names are still missing — that is a driver's real answer. With no
// context, do nothing and let the next frame try again once there is one.
//
// Desktop GL extensions (epoxy's epoxy_* and one desktop-only entry point)
// have no place here: an Android GLES3 context has no such things, and
// asking for them would make load() fail on a device that is behaving.
func load() error {
	if errLoad != nil && errLoadLatched {
		return errLoad
	}
	if currentContext() == 0 {
		// Nothing is current: eglGetProcAddress would answer nothing and
		// register nothing. Leave errLoad unlatched so the next call, made
		// with a context, does the real work.
		return errNoContext
	}
	loadOnce.Do(func() {
		var missing []string
		gl := func(f any, name string) {
			cs := C.CString(name)
			p := C.eglGetProcAddress(cs)
			C.free(unsafe.Pointer(cs))
			if p == nil {
				missing = append(missing, name)
				return
			}
			purego.RegisterFunc(f, uintptr(unsafe.Pointer(p)))
		}
		gl(&glGetString, "glGetString")
		gl(&glGetError, "glGetError")
		gl(&glGetIntegerv, "glGetIntegerv")
		gl(&glCreateShader, "glCreateShader")
		gl(&glShaderSource, "glShaderSource")
		gl(&glCompileShader, "glCompileShader")
		gl(&glGetShaderiv, "glGetShaderiv")
		gl(&glGetShaderInfoLog, "glGetShaderInfoLog")
		gl(&glDeleteShader, "glDeleteShader")
		gl(&glCreateProgram, "glCreateProgram")
		gl(&glAttachShader, "glAttachShader")
		gl(&glLinkProgram, "glLinkProgram")
		gl(&glGetProgramiv, "glGetProgramiv")
		gl(&glGetProgramInfoLog, "glGetProgramInfoLog")
		gl(&glUseProgram, "glUseProgram")
		gl(&glDeleteProgram, "glDeleteProgram")
		gl(&glGetUniformLocation, "glGetUniformLocation")
		gl(&glUniform1i, "glUniform1i")
		gl(&glUniform2f, "glUniform2f")
		gl(&glUniform1f, "glUniform1f")
		gl(&glUniform2i, "glUniform2i")
		gl(&glCopyTexSubImage2D, "glCopyTexSubImage2D")
		gl(&glGenVertexArrays, "glGenVertexArrays")
		gl(&glBindVertexArray, "glBindVertexArray")
		gl(&glDeleteVertexArrays, "glDeleteVertexArrays")
		gl(&glGenBuffers, "glGenBuffers")
		gl(&glBindBuffer, "glBindBuffer")
		gl(&glBufferData, "glBufferData")
		gl(&glDeleteBuffers, "glDeleteBuffers")
		gl(&glEnableVertexAttrib, "glEnableVertexAttribArray")
		gl(&glVertexAttribPointer, "glVertexAttribPointer")
		gl(&glVertexAttribDivisor, "glVertexAttribDivisor")
		gl(&glGenTextures, "glGenTextures")
		gl(&glBindTexture, "glBindTexture")
		gl(&glActiveTexture, "glActiveTexture")
		gl(&glTexImage2D, "glTexImage2D")
		gl(&glTexSubImage2D, "glTexSubImage2D")
		gl(&glTexParameteri, "glTexParameteri")
		gl(&glDeleteTextures, "glDeleteTextures")
		gl(&glPixelStorei, "glPixelStorei")
		gl(&glViewport, "glViewport")
		gl(&glScissor, "glScissor")
		gl(&glEnable, "glEnable")
		gl(&glDisable, "glDisable")
		gl(&glBlendFunc, "glBlendFunc")
		gl(&glBlendFuncSeparate, "glBlendFuncSeparate")
		gl(&glClearColor, "glClearColor")
		gl(&glClear, "glClear")
		gl(&glDrawArraysInstanced, "glDrawArraysInstanced")
		gl(&glReadPixels, "glReadPixels")
		gl(&glGenFramebuffers, "glGenFramebuffers")
		gl(&glBindFramebuffer, "glBindFramebuffer")
		gl(&glFramebufferTexture2D, "glFramebufferTexture2D")
		gl(&glCheckFramebufferState, "glCheckFramebufferStatus")
		gl(&glDeleteFramebuffers, "glDeleteFramebuffers")
		gl(&glBlitFramebuffer, "glBlitFramebuffer")
		if len(missing) > 0 {
			errLoad = fmt.Errorf("gl: eglGetProcAddress lacks %s", strings.Join(missing, ", "))
			// A context was current and the names are still absent: this is
			// the driver's answer, and it will not change. Latch it.
			errLoadLatched = true
			return
		}
		// The GL version and the desktop question are answered here rather
		// than by libepoxy: an Android context is ES by definition, and
		// glGetString(GL_VERSION) says which of them.
		epoxyGLVersion = esVersion
		epoxyIsDesktopGL = func() bool { return false }
		epoxyHasExtension = hasGLExtension
		errLoad = nil
		errLoadLatched = true
	})
	return errLoad
}

// The three functions libepoxy would have provided, on Android: a GLES3
// context answers "ES 3.x" and is never desktop GL.
func esVersion() int32 {
	s := goString(glGetString(C.GL_VERSION))
	// "OpenGL ES 3.2 v1.r1p0..." — the major and minor are the two numbers
	// after "ES ", and the renderer wants them as one number (3.2 -> 32).
	var major, minor int
	if _, err := fmt.Sscanf(s, "OpenGL ES %d.%d", &major, &minor); err != nil {
		return 0
	}
	return int32(major*10 + minor)
}

func hasGLExtension(name string) bool {
	s := goString(glGetString(C.GL_EXTENSIONS))
	for _, f := range strings.Fields(s) {
		if f == name {
			return true
		}
	}
	return false
}
