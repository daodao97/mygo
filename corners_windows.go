//go:build windows

package mygo

// contentCorners returns the function that tells the radius, in DIPs, the
// window's content rounds its own corners to, for a transparent frameless
// window with a CornerRadius, or nil for any other: Windows 10 keeps the
// corners square, where Windows 11 rounds them itself (at 8 DIPs). The
// radius is 0 while the window is maximized or full screen, square on
// either system, and once Windows rounds the corners.
func (w *Window) contentCorners() func() float64 {
	if !w.transparent || !w.frameless || w.cornerRadius <= 0 {
		return nil
	}
	r := float64(w.cornerRadius)
	return func() float64 {
		nw := w.native
		if nw == nil || nw.CornerRadius() != 0 || nw.IsMaximized() || nw.IsFullScreen() {
			return 0
		}
		return r
	}
}
