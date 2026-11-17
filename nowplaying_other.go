//go:build !(darwin && !ios) && !(linux && !android && (amd64 || arm64)) && !(windows && (amd64 || arm64))

package mygo

func linkNowPlaying() {}
