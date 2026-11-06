//go:build android || !(linux && (amd64 || arm64))

package mygo

func linkDMABUFProbe() {}
