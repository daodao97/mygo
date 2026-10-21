//go:build linux && (amd64 || arm64)

package linux

import "github.com/egoist/mygo/internal/platform"

func (*Backend) Speech() platform.SpeechRecognizer { return platform.UnsupportedSpeech{} }
