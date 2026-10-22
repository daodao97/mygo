//go:build ios || android || (!darwin && !(linux && (amd64 || arm64)) && !(windows && (amd64 || arm64)))

package mygo

func linkContentRules() {}
