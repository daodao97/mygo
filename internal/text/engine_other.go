//go:build !(windows && (amd64 || arm64)) && !darwin && !(linux && !android && (amd64 || arm64)) && !js && !android

package text

func newEngine() engine { return &stubEngine{} }
