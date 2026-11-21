//go:build !windows

package mygo

// contentCorners returns nil: the system rounds a window's corners, or
// keeps them square, on every platform but Windows 10 (corners_windows.go).
func (w *Window) contentCorners() func() float64 { return nil }
