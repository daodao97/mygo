//go:build linux && (amd64 || arm64) && !android

package gl

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"
)

// GTK's functions of a GtkGLArea's context, when GTK is loaded.
var (
	gdkClearCurrent    func()
	gtkAreaContext     func(area uintptr) uintptr
	gtkAreaMakeCurrent func(area uintptr)
	gtkAreaAttach      func(area uintptr)
)

// load takes the GL entry points from libepoxy, which GTK resolves its own
// through: libepoxy exports a pointer per function, to a stub that finds the
// one of the current context when first called. That indirection is what
// lets the same binary draw in two windows whose contexts differ.
//
// Android has no libepoxy; load_android.go asks EGL instead.
func load() error {
	loadOnce.Do(func() {
		lib, err := purego.Dlopen("libepoxy.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			errLoad = fmt.Errorf("gl: cannot load libepoxy: %w", err)
			return
		}
		var missing []string
		fn := func(f any, name string) {
			sym, err := purego.Dlsym(lib, name)
			if err != nil || sym == 0 {
				missing = append(missing, name)
				return
			}
			purego.RegisterFunc(f, sym)
		}
		// libepoxy exports a pointer for each GL function, to a stub that
		// finds the function of the current context when first called.
		gl := func(f any, name string) {
			sym, err := purego.Dlsym(lib, "epoxy_"+name)
			if err != nil || sym == 0 {
				missing = append(missing, name)
				return
			}
			purego.RegisterFunc(f, **(**uintptr)(unsafe.Pointer(&sym)))
		}
		fn(&epoxyGLVersion, "epoxy_gl_version")
		fn(&epoxyIsDesktopGL, "epoxy_is_desktop_gl")
		fn(&epoxyHasExtension, "epoxy_has_gl_extension")
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
			errLoad = fmt.Errorf("gl: libepoxy lacks %s", strings.Join(missing, ", "))
			return
		}
		// GDK is loaded in apps, where its context tells when GTK
		// realized the area again; tests make contexts of their own.
		if sym, err := purego.Dlsym(purego.RTLD_DEFAULT, "gdk_gl_context_get_current"); err == nil && sym != 0 {
			purego.RegisterFunc(&currentGDKContext, sym)
		}
		gtk := map[string]any{
			"gdk_gl_context_clear_current": &gdkClearCurrent, "gtk_gl_area_get_context": &gtkAreaContext,
			"gtk_gl_area_make_current": &gtkAreaMakeCurrent, "gtk_gl_area_attach_buffers": &gtkAreaAttach,
		}
		for name, f := range gtk {
			sym, err := purego.Dlsym(purego.RTLD_DEFAULT, name)
			if err != nil || sym == 0 {
				gdkClearCurrent = nil // all or none
				break
			}
			purego.RegisterFunc(f, sym)
		}
	})
	return errLoad
}

// MakeAreaCurrent makes the context of a GtkGLArea current again, with the
// area's framebuffer bound, when it no longer is during the area's render
// signal, and reports whether OpenGL has a current context after. GTK
// makes the area's context current before the signal, but what a frame
// runs before it draws, as a window that a button's action creates, shows
// or hides, can leave another context current, or none: GDK's
// gdk_window_hide clears the current context of the window it hides, and
// disposing a context makes EGL's current one none. GDK remembers the
// context it last made current and does nothing when asked for it again,
// so the area's context is made current again past that memory: cleared
// first, then made current.
func MakeAreaCurrent(area uintptr) bool {
	if load() != nil {
		return false
	}
	if area == 0 || gdkClearCurrent == nil || currentGDKContext == nil {
		return hasContext()
	}
	ctx, has := gtkAreaContext(area), hasContext()
	if !needsRestore(ctx, currentGDKContext(), has) {
		return has
	}
	gdkClearCurrent()
	gtkAreaMakeCurrent(area)
	gtkAreaAttach(area)
	return hasContext()
}

// needsRestore reports whether the context of a GtkGLArea, area, must be
// made current again before drawing into the area: GDK has another
// context current, current, or OpenGL none at all, has false, whatever
// GDK thinks. An area without a context has none to make current.
func needsRestore(area, current uintptr, has bool) bool {
	return area != 0 && (current != area || !has)
}

// contextVersion returns the version of the current context, major × 10 +
// minor, and whether it is OpenGL ES.
func contextVersion() (version int32, es bool) { return epoxyGLVersion(), !epoxyIsDesktopGL() }

// hasExtension reports whether the current context has an extension.
func hasExtension(name string) bool { return epoxyHasExtension(name) }

// hasContext reports whether a context is current, which GDK may think one
// is when OpenGL does not: libepoxy aborts the first call of most GL
// functions without one. glGetString it finds without.
func hasContext() bool { return glGetString(glVersion) != 0 }

// versionString describes the current context's version, for errors.
func versionString() string { return goString(glGetString(glVersion)) }

// rendererString names the current context's renderer.
func rendererString() string { return goString(glGetString(glRenderer)) }
