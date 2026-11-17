// Package nativehost is how package native reaches what a phone's host
// gives it, through functions the host's Go side (internal/mobile) sets
// as it starts: the UI thread, where Swift's main actor and Android's
// views live, apart from Go's main thread there; and on Android the
// Kotlin implementations of native modules, which Go calls through JNI.
// Apps without native modules link only these variables.
package nativehost

// OnUI runs fn on the UI thread and waits for it, or runs it at once on
// the UI thread. Phones only; nil elsewhere, where Go's main thread is the
// system's.
var OnUI func(fn func())

// PostUI runs fn on the UI thread later. Phones only.
var PostUI func(fn func())

// Kotlin is the JNI side of native modules implemented in Kotlin, on
// Android: what jni_android.c calls in dev.mygo.Native. Every function
// takes the module's name, which its Kotlin class registered under with
// Init.
var Kotlin *KotlinBridge

// KotlinBridge calls the Kotlin side of native modules.
type KotlinBridge struct {
	// Init loads class, the module's generated object, under name, with
	// idBase in the high bits of the ids it makes. It returns an error's
	// message, or "".
	Init func(name, class string, idBase uint64) string
	// Invoke calls the function sym of the module on the object h, with
	// the encoded arguments args and the call's id (0 for one answered at
	// once). The answer is its status, then its encoded result; nil is
	// status 0 with nothing.
	Invoke func(name, sym string, h uint64, args []byte, call uint64) []byte
	// Direct calls the method of numbers sym, with args as their bits.
	Direct func(name, sym string, h uint64, args []uint64) uint64
	// Op runs one of release (a: the object), cancel and next (a: the
	// call), or resume (a: the continuation, with status and data).
	Op func(name, op string, a uint64, status int64, data []byte)
	// Malloc returns the address of n bytes of C memory, which Free frees.
	Malloc func(n int) uintptr
	Free   func(p uintptr)
}

// Handle answers what Kotlin tells Go through the callback modules share
// (package native's handle): kind, id, status and the encoded data, and
// returns the answer of a question, its status and data, and whether
// there is one. Package native sets it.
var Handle func(kind, id uint64, status int64, data []byte) (int64, []byte, bool)
