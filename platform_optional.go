package mygo

import (
	"runtime"

	"github.com/egoist/mygo/internal/platform"
)

// The fork's services that upstream #940's phone backend does not have
// (FORK.md, "第二阶段"): a backend that has one implements its method, and
// the others report errors.ErrUnsupported until the service is ported to a
// plugin on mygo.Native.

func mobileBackend() platform.Mobile {
	if b, ok := backend().(interface{ Mobile() platform.Mobile }); ok {
		return b.Mobile()
	}
	return platform.UnsupportedMobile{}
}

func authenticatorBackend() platform.Authenticator {
	if b, ok := backend().(interface{ Authenticator() platform.Authenticator }); ok {
		return b.Authenticator()
	}
	return platform.UnsupportedAuthenticator{}
}

func permissionsBackend() platform.Permissions {
	if b, ok := backend().(interface{ Permissions() platform.Permissions }); ok {
		return b.Permissions()
	}
	return platform.UnsupportedPermissions{}
}

func speechBackend() platform.SpeechRecognizer {
	if b, ok := backend().(interface {
		Speech() platform.SpeechRecognizer
	}); ok {
		return b.Speech()
	}
	return platform.UnsupportedSpeech{}
}

// systemManagedLifetime reports whether the system, not the user, ends the
// app, as on phones.
func systemManagedLifetime() bool {
	if b, ok := backend().(interface{ SystemManagedLifetime() bool }); ok {
		return b.SystemManagedLifetime()
	}
	return runtime.GOOS == "ios" || runtime.GOOS == "android"
}
