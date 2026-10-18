//go:build !ios || !cgo

package mygo

func hostAppGroupDirectory(string) (string, error) { return "", ErrUnsupported }
