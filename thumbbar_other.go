//go:build !(windows && (amd64 || arm64))

package mygo

// linkThumbBars is a no-op away from 64-bit Windows.
func linkThumbBars() {}
