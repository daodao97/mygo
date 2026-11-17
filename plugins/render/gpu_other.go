//go:build !(darwin && !ios) && !windows && !(linux && !android && (amd64 || arm64))

package render

// newGPUDrawer returns none: frames draw on the CPU.
func newGPUDrawer(w, h int, required bool) (drawer, error) { return nil, nil }
