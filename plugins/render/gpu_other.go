//go:build !darwin && !windows && !(linux && (amd64 || arm64))

package render

// newGPUDrawer returns none: frames draw on the CPU.
func newGPUDrawer(w, h int, required bool) (drawer, error) { return nil, nil }
