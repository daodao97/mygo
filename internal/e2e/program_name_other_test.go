//go:build !(linux && !android && (amd64 || arm64)) && !js

package e2e

// programName prints the name GLib has for the program, which other
// systems do not name a program by.
func programName() {}
