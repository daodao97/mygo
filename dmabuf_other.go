//go:build !(linux && !android && (amd64 || arm64))

package mygo

func linkDMABUFProbe() {}
